from __future__ import annotations

import hashlib
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))

from _blackbox import CLI, cache_project_dir, invoke, isolated_home  # noqa: E402

# Runtime-hook gate shipped by the custom `wt-hook` fixture recipe.
GATE_REL = "ai-specs/recipes/wt-hook/hooks/gate.sh"
# The only runtime override template shipped by the *real* worktree-flow recipe.
TPL_REL = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
# Template target used by the custom `example` fixture recipe.
TPL_TARGET = "out/template.md"
# Raw (unrendered) source for the real worktree-flow override template.
TEMPLATE_SRC = ROOT / "catalog" / "recipes" / "worktree-flow" / "templates" / "worktree-cleanup.sh"


def _sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


class OverrideOwnershipTests(unittest.TestCase):
    """Black-box coverage of override ownership through `bin/ai-specs`.

    Every command drives the real CLI (`invoke`) and asserts exit code, the
    emitted override/lock bytes, and stdout/stderr. No internal module is
    imported. One `isolated_home()` is shared across each command sequence.
    """

    # ---------------------------------------------------------------- builders

    def _custom_home(self, base: Path, rel: str, recipe_toml: str, files: dict[str, bytes]) -> Path:
        """Isolated install home whose catalog carries ONE fixture recipe."""
        home = isolated_home(base)
        catalog = home / "catalog"
        catalog.unlink()  # isolated_home symlinks catalog to the repo
        catalog.mkdir()
        recipe_dir = catalog / "recipes" / rel
        recipe_dir.mkdir(parents=True)
        for name, content in files.items():
            target = recipe_dir / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(content)
        (recipe_dir / "recipe.toml").write_text(recipe_toml)
        return home

    def _project(self, manifest: str) -> Path:
        proj_tmp = tempfile.TemporaryDirectory(prefix="oo-proj-")
        self.addCleanup(proj_tmp.cleanup)
        root = Path(proj_tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(manifest)
        return root

    @staticmethod
    def _manifest(recipe: str, version: str | None = None, config: str = "") -> str:
        ver = f"\nversion = '{version}'" if version else ""
        return (
            "[project]\nname = 'p'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{recipe}]\nenabled = true{ver}\n{config}"
        )

    @staticmethod
    def _template_toml(policy: str | None = None) -> str:
        lines = [
            "[recipe]",
            'id = "example"',
            'name = "Example"',
            'description = "D"',
            'version = "1"',
            "[[provides.templates]]",
            'source = "template.txt"',
            'target = "out/template.md"',
            'condition = "not_exists"',
        ]
        if policy:
            lines.append(f'update_policy = "{policy}"')
        return "\n".join(lines) + "\n"

    def _template_project(self, payload: bytes = b"v1\n", *, policy: str | None = None) -> tuple[Path, Path]:
        """Custom `example` template recipe + a project enabling it."""
        home_tmp = tempfile.TemporaryDirectory(prefix="oo-home-")
        self.addCleanup(home_tmp.cleanup)
        home = self._custom_home(Path(home_tmp.name), "example", self._template_toml(policy),
                                 {"template.txt": payload})
        root = self._project(self._manifest("example"))
        return root, home

    def _hook_project(self, gate_bytes: bytes = b"v1\n") -> tuple[Path, Path]:
        """Custom `wt-hook` runtime-hook recipe + a project enabling it."""
        home_tmp = tempfile.TemporaryDirectory(prefix="oo-home-")
        self.addCleanup(home_tmp.cleanup)
        recipe_toml = (
            "[recipe]\n"
            'id = "wt-hook"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            "[[provides.hooks]]\n"
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            "blocking = true\n"
        )
        home = self._custom_home(Path(home_tmp.name), "wt-hook", recipe_toml,
                                 {"hooks/gate.sh": gate_bytes})
        root = self._project(self._manifest("wt-hook", version="1.0"))
        return root, home

    def _worktree_project(self, config: str = "") -> tuple[Path, Path]:
        """Real-catalog worktree-flow project (doctor reads the real catalog)."""
        home_tmp = tempfile.TemporaryDirectory(prefix="oo-home-")
        self.addCleanup(home_tmp.cleanup)
        home = isolated_home(Path(home_tmp.name))
        root = self._project(self._manifest("worktree-flow", config=config))
        return root, home

    def _lock(self, root: Path) -> dict:
        return tomllib.loads((root / "ai-specs" / ".ai-specs.lock").read_text())

    def _managed(self, root: Path) -> dict:
        return self._lock(root).get("managed", {})

    def _hook_entry(self, root: Path) -> dict | None:
        return self._managed(root).get(GATE_REL)

    def _template_entry(self, root: Path) -> dict | None:
        return self._managed(root).get(TPL_TARGET)

    def _customize_then_refresh(self, root: Path, home: Path, custom: bytes,
                                catalog_bytes: bytes = b"v2\n") -> None:
        """Customize the gate and run an explicit `--refresh-gates`."""
        (root / GATE_REL).write_bytes(custom)
        (home / "catalog" / "recipes" / "wt-hook" / "hooks" / "gate.sh").write_bytes(catalog_bytes)
        result = invoke(root, "sync", "--refresh-gates", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def _backup_path(self, root: Path, home: Path, custom: bytes) -> Path:
        rel_key = hashlib.sha256(GATE_REL.encode("utf-8")).hexdigest()
        return cache_project_dir(root, home) / "backups" / rel_key / f"{_sha(custom)}.sh"

    # --------------------------------------------------------------- round-trip

    def test_managed_lock_round_trip_preserves_provenance(self):
        """A template rendered by `sync` survives into the lock with full provenance."""
        root, home = self._template_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = root / TPL_TARGET
        self.assertEqual(dest.read_text(), "v1\n")
        text = (root / "ai-specs" / ".ai-specs.lock").read_text()
        self.assertIn(f'[managed."{TPL_TARGET}"]', text)
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        entry = self._template_entry(root)
        self.assertEqual(entry["sha256"], _sha(dest.read_bytes()))
        self.assertEqual(entry["recipe"], "example")
        self.assertEqual(entry["source"], "template.txt")
        self.assertEqual(entry["kind"], "template")
        self.assertEqual(entry["policy"], "auto")

    # --------------------------------------------------------------- classifier

    def test_classifier_covers_ownership_states(self):
        """Missing / current / stale / user-modified / untracked-adopt all surface."""
        root, home = self._template_project()
        dest = root / TPL_TARGET
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(dest.is_file(), "missing state must seed the override")
        self.assertEqual(dest.read_text(), "v1\n", "seeded override matches the catalog")
        self.assertIn(TPL_TARGET, self._managed(root), "seeded override is now managed")

        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "v1\n", "managed_current stays byte-identical")
        self.assertEqual(self._template_entry(root)["sha256"], _sha(dest.read_bytes()))
        self.assertNotIn("user-modified", result.stderr)

        (home / "catalog" / "recipes" / "example" / "template.txt").write_bytes(b"v2\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "v2\n", "managed_stale (auto) refreshes")
        self.assertEqual(self._template_entry(root)["sha256"], _sha(dest.read_bytes()))

        dest.write_text("user edit\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "user edit\n", "user edit is preserved")
        self.assertIn("user-modified", result.stderr, "user-modified is surfaced")

        root2, home2 = self._template_project(payload=b"same\n")
        dest2 = root2 / TPL_TARGET
        dest2.parent.mkdir(parents=True, exist_ok=True)
        dest2.write_text("same\n")
        result = invoke(root2, "sync", cli_home=home2)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest2.read_text(), "same\n", "untracked match is not rewritten")
        self.assertEqual(self._managed(root2)[TPL_TARGET]["sha256"], _sha(dest2.read_bytes()),
                         "untracked match is adopted under its real sha")

    # -------------------------------------------------------------- materialize

    def test_materialize_seeds_and_refreshes_managed_templates(self):
        """sync seeds a missing override, then refreshes it when the catalog evolves."""
        root, home = self._template_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = root / TPL_TARGET
        self.assertEqual(dest.read_text(), "v1\n")
        self.assertEqual(self._template_entry(root)["sha256"], _sha(dest.read_bytes()))
        (home / "catalog" / "recipes" / "example" / "template.txt").write_bytes(b"v2\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "v2\n", "catalog evolution refreshes the override")
        self.assertEqual(self._template_entry(root)["sha256"], _sha(dest.read_bytes()))

    def test_materialize_preserves_user_modified_and_untracked_diverged(self):
        """sync preserves user edits and, later, an untracked diverged file as-is."""
        root, home = self._template_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = root / TPL_TARGET
        dest.write_text("user edit\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "user edit\n", "user-modified is preserved")
        self.assertIn("user-modified", result.stderr)

        dest.unlink()
        (root / "ai-specs" / ".ai-specs.lock").unlink()
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("custom before migration\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "custom before migration\n",
                         "untracked diverged override is preserved byte-for-byte")
        warning = result.stderr
        self.assertIn("missing", warning)
        self.assertIn("preserving existing file", warning)
        self.assertIn("leave it unchanged", warning)
        self.assertIn("remove it and run sync again", warning)
        self.assertNotIn("user-managed", warning.lower())
        self.assertNotIn("customized", warning.lower())
        self.assertNotIn(TPL_TARGET, self._managed(root),
                         "a preserved untracked override gains no managed entry")
    def test_untracked_matching_catalog_seeds_without_rewrite(self):
        """An override that already matches the catalog is adopted, never rewritten."""
        root, home = self._template_project(payload=b"same\n")
        dest = root / TPL_TARGET
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("same\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_text(), "same\n", "matching unowned bytes are left untouched")
        self.assertEqual(self._managed(root)[TPL_TARGET]["sha256"], _sha(dest.read_bytes()),
                         "the adopted override is recorded under its real sha")

    # ------------------------------------------------------------------ doctor

    def test_doctor_describes_untracked_divergence_neutrally(self):
        """doctor warns about an unowned override neutrally (no claim of authorship)."""
        root, home = self._worktree_project()
        dest = root / TPL_REL
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("local bytes without metadata\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        result = invoke(root, "doctor", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        warning = next(line for line in result.stdout.splitlines() if "missing ownership metadata" in line)
        self.assertIn("missing ownership metadata", warning)
        self.assertIn("preserve", warning)
        self.assertIn("remove", warning)
        self.assertIn("sync", warning)
        self.assertNotIn("user-managed", warning.lower())
        self.assertNotIn("user-owned", warning.lower())

    def test_doctor_silences_legacy_catalog_seed_with_rendered_config(self):
        """A raw legacy seed is adopted silently even when config changes rendered bytes."""
        root, home = self._worktree_project(
            config='[recipes.worktree-flow.config]\nrepo_topology = "standalone"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = root / TPL_REL
        (root / "ai-specs" / ".ai-specs.lock").unlink()  # legacy seed has no ownership metadata
        dest.write_bytes(TEMPLATE_SRC.read_bytes())
        result = invoke(root, "doctor", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([line for line in result.stdout.splitlines() if "stale-override" in line], [],
                         "a legacy seed matching the raw catalog must not warn")
        self.assertEqual([line for line in result.stdout.splitlines() if "missing ownership" in line], [],
                         "a legacy seed matching the raw catalog must not claim unowned")

    # --------------------------------------------------------------- policies

    def test_template_policy_is_validated_and_defaults_auto(self):
        """never-force is legal, auto is the default, an invalid policy errors."""
        root, home = self._template_project(policy="never-force")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self._template_entry(root)["policy"], "never-force")

        root, home = self._template_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self._template_entry(root)["policy"], "auto",
                         "an omitted update_policy defaults to auto")

        root, home = self._template_project(policy="sometimes")
        result = invoke(root, "sync", cli_home=home)
        self.assertNotEqual(result.returncode, 0, "an invalid policy must fail sync")
        self.assertIn("update_policy", result.stderr)

    def test_confirm_policy_preserves_managed_stale(self):
        """A confirm-required managed override is preserved when stale."""
        root, home = self._template_project(policy="confirm")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((root / TPL_TARGET).read_text(), "v1\n")
        (home / "catalog" / "recipes" / "example" / "template.txt").write_bytes(b"v2\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((root / TPL_TARGET).read_text(), "v1\n", "confirm blocks the refresh")
        self.assertIn("managed-stale", result.stderr)

    def test_doctor_warns_user_modified_but_not_managed_auto_stale(self):
        """doctor reports a user edit once, with no separate auto-stale warning."""
        root, home = self._worktree_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = root / TPL_REL
        dest.write_text("user edited bytes\n")
        result = invoke(root, "doctor", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        stale = [line for line in result.stdout.splitlines() if "stale-override" in line]
        self.assertEqual(len(stale), 1, "only the user-modified override warns")
        self.assertIn("user-modified", stale[0])
        self.assertTrue(any(line.startswith("Summary") and "0 ERROR" in line
                            for line in result.stdout.splitlines()))

    # ------------------------------------------------------------------- gates

    def test_gate_baseline_match_refreshes_and_records_baseline(self):
        """A matching baseline lets a catalog gate update refresh the override."""
        root, home = self._hook_project(gate_bytes=b"v1\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        gate = root / GATE_REL
        baseline = self._hook_entry(root)
        self.assertEqual(baseline["kind"], "gate")
        self.assertEqual(baseline["policy"], "auto")
        self.assertEqual(baseline["sha256"], _sha(gate.read_bytes()))
        self.assertEqual(gate.read_bytes(), b"v1\n")

        (home / "catalog" / "recipes" / "wt-hook" / "hooks" / "gate.sh").write_bytes(b"v2\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(gate.read_bytes(), b"v2\n", "a matching baseline force-refreshes")
        self.assertNotIn("user-modified", result.stderr)
        self.assertEqual(self._hook_entry(root)["sha256"], _sha(gate.read_bytes()))

    def test_gate_byte_mismatch_preserves_with_warning(self):
        """A byte mismatch preserves the customized gate and warns."""
        root, home = self._hook_project(gate_bytes=b"v1\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        gate = root / GATE_REL
        gate.write_bytes(b"# custom user gate\n")
        (home / "catalog" / "recipes" / "wt-hook" / "hooks" / "gate.sh").write_bytes(b"v2\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(gate.read_bytes(), b"# custom user gate\n", "the gate keeps its custom bytes")
        self.assertIn("gate.sh", result.stderr)
        self.assertIn("user-modified", result.stderr)
        self.assertIn("refresh", result.stderr.lower())

    def test_gate_missing_provenance_preserves_without_seeding(self):
        """No baseline means preserve + warn, never seed a fake baseline."""
        root, home = self._hook_project()
        gate = root / GATE_REL
        gate.parent.mkdir(parents=True, exist_ok=True)
        gate.write_bytes(b"# pre-existing without provenance\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(gate.read_bytes(), b"# pre-existing without provenance\n")
        self.assertIn("gate.sh", result.stderr)
        self.assertIn("provenance", result.stderr.lower())
        self.assertIsNone(self._hook_entry(root),
                          "no baseline may be seeded when the CLI did not render the gate")

    def test_explicit_refresh_backs_up_pre_refresh_bytes_immutably(self):
        """--refresh-gates stores the exact pre-refresh bytes as a cache-only backup."""
        root, home = self._hook_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        custom = b"# customized user gate\n"
        self._customize_then_refresh(root, home, custom)
        gate = root / GATE_REL
        self.assertEqual(gate.read_bytes(), b"v2\n", "the explicit refresh replaces the gate")
        self.assertEqual(self._hook_entry(root)["sha256"], _sha(gate.read_bytes()))
        backup = self._backup_path(root, home, custom)
        self.assertTrue(backup.is_file(), f"backup missing at {backup}")
        self.assertEqual(backup.read_bytes(), custom, "backup preserves the exact pre-refresh bytes")
        self.assertNotIn("cache", str(root),
                         "backup must live in the CLI cache, not in the project")

    def test_repeated_refresh_is_collision_safe(self):
        """Repeated refreshes keep every original snapshot intact."""
        root, home = self._hook_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        custom_a = b"# custom A\n"
        self._customize_then_refresh(root, home, custom_a)
        backup_a = self._backup_path(root, home, custom_a)
        self.assertTrue(backup_a.is_file())
        custom_b = b"# custom B\n"
        self._customize_then_refresh(root, home, custom_b, catalog_bytes=b"v3\n")
        backup_b = self._backup_path(root, home, custom_b)
        self.assertNotEqual(backup_a, backup_b, "distinct content must not collide")
        self.assertTrue(backup_a.is_file(), "the original snapshot must remain intact")
        self.assertTrue(backup_b.is_file())
        self.assertEqual(backup_a.read_bytes(), custom_a)

    def test_failed_backup_write_leaves_gate_and_lock_unchanged(self):
        """A failed backup aborts the refresh atomically: gate and lock stay put."""
        root, home = self._hook_project()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        gate = root / GATE_REL
        custom = b"# custom gate\n"
        gate.write_bytes(custom)
        before_lock = (root / "ai-specs" / ".ai-specs.lock").read_bytes()
        (home / "catalog" / "recipes" / "wt-hook" / "hooks" / "gate.sh").write_bytes(b"v9\n")
        rel_key = hashlib.sha256(GATE_REL.encode("utf-8")).hexdigest()
        blocking = cache_project_dir(root, home) / "backups" / rel_key
        blocking.parent.mkdir(parents=True, exist_ok=True)
        blocking.write_text("blocking file\n")  # rel-key backup dir position is a file
        result = invoke(root, "sync", "--refresh-gates", cli_home=home)
        self.assertNotEqual(result.returncode, 0, "the refresh must fail when the backup cannot be written")
        self.assertEqual(gate.read_bytes(), custom,
                         "gate must remain unchanged when the backup write fails")
        self.assertEqual((root / "ai-specs" / ".ai-specs.lock").read_bytes(), before_lock,
                         "lock must not be partially updated on refresh failure")

    def test_refresh_absent_or_disabled_provider_parity(self):
        """Refresh behaves identically whether external orchestration is absent or disabled."""
        outcomes = []
        for extra_env in ({}, {"GENTLE_AI_MODE": "disabled", "GENTLE_AI_ABSENT": "1"}):
            root, home = self._hook_project()
            tmp = home / "tmp"
            tmp.mkdir(parents=True, exist_ok=True)
            env = dict(os.environ, AI_SPECS_HOME=str(home), AI_SPECS_NO_NETWORK="1",
                       HOME=str(tmp), TMPDIR=str(tmp))
            env.update(extra_env)
            gate = root / GATE_REL
            subprocess.run([CLI, "sync", str(root)], cwd=ROOT, env=env,
                           capture_output=True, text=True, check=False)
            gate.write_bytes(b"# custom before parity\n")
            (home / "catalog" / "recipes" / "wt-hook" / "hooks" / "gate.sh").write_bytes(b"v3\n")
            proc = subprocess.run([CLI, "sync", "--refresh-gates", str(root)], cwd=ROOT, env=env,
                                  capture_output=True, text=True, check=False)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(gate.read_bytes(), b"v3\n", "the gate adopts the refreshed catalog bytes")
            outcomes.append(gate.read_bytes())
        self.assertEqual(len(set(outcomes)), 1,
                         "absent and disabled external orchestration must refresh identically")

    def test_gate_provenance_policy_is_documented(self):
        """The changed recipe docs preserve the gate provenance contract."""
        docs = (
            ROOT / "catalog" / "recipes" / "worktree-flow" / "README.md",
            ROOT / "catalog" / "recipes" / "trello-mcp-workflow" / "README.md",
            ROOT / "docs" / "recipes-catalog.md",
        )
        surface = "\n".join(path.read_text().lower() for path in docs)
        for phrase in (
            "gate provenance",
            "records a baseline",
            "byte mismatch",
            "missing baseline",
            "ai-specs sync --refresh-gates",
            "cache-only immutable backup",
            "runtime hook scripts are no longer rewritten unconditionally",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, surface)
        self.assertNotIn("always rewritten", surface)


if __name__ == "__main__":
    unittest.main()