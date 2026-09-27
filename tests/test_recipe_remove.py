"""Black-box tests for ai-specs recipe remove (lib/recipe-remove.sh).

Covers the manifest/lock write guarantees from Trello card
6a84ed305c0de21af3d2c224:
- D1: recipe remove must not destroy unrelated lock sections
  ([managed."..."], [agents."..."]) when it cleans its own entries.
- D2: removal must not rewrite untouched regions of the manifest
  (no whole-file ``\\n{3,}`` collapse, including inside multi-line strings).
- Validation + restore: a deletion that would break a previously valid
  manifest is refused with the original bytes intact, and no temp files leak
  (atomic write, mirroring lock.py / recipe-config-write.py guarantees).
"""

import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_REMOVE_SCRIPT = ROOT / "lib" / "recipe-remove.sh"


MANIFEST = (
    "[project]\n"
    'name = "test"\n'
    "\n"
    "[recipes.bar]\n"
    "enabled = true\n"
    'notes = """\n'
    "first\n"
    "\n"
    "\n"
    "third\n"
    '"""\n'
    "\n"
    "[recipes.foo]\n"
    "enabled = true\n"
    "\n"
    "[recipes.foo.config]\n"
    'key = "value"\n'
)

# A valid manifest whose segment deletion (both [recipes.foo] segments) cuts a
# multi-line basic string open, so the naive result is invalid TOML.
MANIFEST_DELETION_BREAKS_TOML = (
    "[project]\n"
    'name = "test"\n'
    "\n"
    "[recipes.bar]\n"
    'doc = """\n'
    "[recipes.foo]\n"
    "inner\n"
    '"""\n'
    "\n"
    "[recipes.foo]\n"
    "enabled = true\n"
)

LOCK_CONTENT = (
    "# Managed by ai-specs. Do not edit by hand.\n"
    "\n"
    "[meta]\n"
    'cli_version = "0.24.0"\n'
    'synced_at = "2026-01-01T00:00:00Z"\n'
    "\n"
    "[recipes.foo]\n"
    '"skills/a.md" = "deadbeef"\n'
    "\n"
    '[managed."AGENTS.md"]\n'
    'sha256 = "aaaa"\n'
    'recipe = "bar"\n'
    'kind = "runtime-brief"\n'
    'policy = "never-force"\n'
    "\n"
    '[managed."ai-specs/recipes/foo/hooks/gate.sh"]\n'
    'sha256 = "bbbb"\n'
    'recipe = "foo"\n'
    'source = "hooks/gate.sh"\n'
    'kind = "gate"\n'
    'policy = "auto"\n'
    "\n"
    '[agents."pi"]\n'
    '"AGENTS.md" = "cccc"\n'
)


class RecipeRemoveCliTests(unittest.TestCase):
    """Drive lib/recipe-remove.sh (the `recipe remove` CLI entry) via subprocess."""

    def _env(self):
        return {**os.environ, "AI_SPECS_HOME": str(ROOT)}

    def _project(self, manifest: str = MANIFEST, lock: str | None = LOCK_CONTENT) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        if lock is not None:
            (ai_specs / ".ai-specs.lock").write_text(lock, encoding="utf-8")
        return project

    def _run(self, *args) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(RECIPE_REMOVE_SCRIPT), *args],
            capture_output=True, text=True, env=self._env(), check=False,
        )

    def _toml_path(self, project: Path) -> Path:
        return project / "ai-specs" / "ai-specs.toml"

    def _lock_path(self, project: Path) -> Path:
        return project / "ai-specs" / ".ai-specs.lock"

    def _no_temp_leak(self, project: Path):
        leftovers = [
            entry.name
            for entry in (project / "ai-specs").iterdir()
            if entry.name.startswith(".") and entry.name.endswith(".tmp")
        ]
        self.assertEqual(leftovers, [], "temp files leaked by removal")

    # ── D1: lock data loss ──

    def test_remove_preserves_managed_and_agents_in_lock(self):
        """Removing a recipe must not discard [managed.*] / [agents.*] lock data."""
        project = self._project()
        proc = self._run("foo", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        with open(self._lock_path(project), "rb") as f:
            lock = tomllib.load(f)

        managed = lock.get("managed") or {}
        self.assertIn("AGENTS.md", managed, "[managed.\"AGENTS.md\"] entry destroyed by remove")
        self.assertIn(
            "ai-specs/recipes/foo/hooks/gate.sh", managed,
            "[managed.\"...\"] entry destroyed by remove",
        )
        self.assertEqual(managed["AGENTS.md"]["sha256"], "aaaa")
        self.assertEqual(managed["AGENTS.md"]["kind"], "runtime-brief")
        self.assertEqual(
            (lock.get("agents") or {}).get("pi"), {"AGENTS.md": "cccc"},
            "[agents.*] destroyed by remove",
        )
        self.assertEqual(
            (lock.get("meta") or {}).get("cli_version"), "0.24.0",
            "[meta] destroyed by remove",
        )
        # The removed recipe's own lock entries are gone.
        self.assertNotIn("foo", lock.get("recipes") or {})

    # ── D2: whole-file newline collapse ──

    def test_remove_preserves_multiline_string_blank_lines(self):
        """Untouched manifest regions (even inside multi-line strings) survive byte-identical."""
        project = self._project()
        proc = self._run("foo", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        with open(self._toml_path(project), "rb") as f:
            data = tomllib.load(f)
        self.assertEqual(
            data["recipes"]["bar"]["notes"], "first\n\n\nthird\n",
            "blank lines inside an untouched multi-line string were collapsed",
        )
        # The removed recipe is gone.
        self.assertNotIn("foo", data.get("recipes") or {})
        self._no_temp_leak(project)

    # ── Validation + restore (no partial/atomic-write violation) ──

    def test_remove_invalid_result_restores_manifest(self):
        """A deletion that breaks a valid manifest is refused; original bytes intact."""
        project = self._project(manifest=MANIFEST_DELETION_BREAKS_TOML, lock=None)
        before = self._toml_path(project).read_bytes()

        proc = self._run("foo", str(project))
        self.assertNotEqual(
            proc.returncode, 0,
            "removal that would produce invalid TOML must fail, not write the broken result",
        )
        after = self._toml_path(project).read_bytes()
        self.assertEqual(before, after, "manifest bytes were modified by a refused removal")
        self._no_temp_leak(project)

    def test_remove_tolerates_already_invalid_manifest(self):
        """FROZEN parity: removal still works on a manifest that is not valid TOML."""
        invalid = "[project]\nname = \"test\"\nbroken line\n[recipes.foo]\nenabled = true\n"
        project = self._project(manifest=invalid, lock=None)
        proc = self._run("foo", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        raw = self._toml_path(project).read_text(encoding="utf-8")
        self.assertNotIn("[recipes.foo]", raw)


if __name__ == "__main__":
    unittest.main()
