"""Tests for ai-specs skills remove (lib/skills-remove.sh)."""

import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS_REMOVE_SCRIPT = ROOT / "lib" / "skills-remove.sh"
SKILLS_ADD_SCRIPT = ROOT / "lib" / "skills-add.sh"


def _manifest_with_dep(dep_id: str = "my-skill") -> str:
    return (
        '[project]\nname = "test"\n'
        "\n"
        "[[deps]]\n"
        f'id = "{dep_id}"\n'
        'source = "https://github.com/test/repo.git"\n'
        'scope = ["root"]\n'
        'auto_invoke = ["When working on my-skill"]\n'
        "\n"
        "[[deps]]\n"
        'id = "other-skill"\n'
        'source = "https://github.com/test/other.git"\n'
        'scope = ["root"]\n'
        "\n"
    )


class SkillsRemoveCliTests(unittest.TestCase):
    """Test skills-remove.sh via subprocess."""

    def _env(self):
        return {**os.environ, "AI_SPECS_HOME": str(ROOT)}

    def _project_with_manifest(self, content: str = _manifest_with_dep()) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(content, encoding="utf-8")
        return project

    def _empty_project(self) -> Path:
        """Project with a minimal manifest and no deps, ready for skills-add."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            '[project]\nname = "test"\n', encoding="utf-8"
        )
        return project

    def _add(self, project: Path, dep_id: str, url: str):
        proc = subprocess.run(
            [
                "bash", str(SKILLS_ADD_SCRIPT), url, str(project),
                "--id", dep_id, "--no-sync",
            ],
            capture_output=True, text=True, cwd=str(project),
            env=self._env(), check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

    def _run(self, *args, cwd=None) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(SKILLS_REMOVE_SCRIPT), *args],
            capture_output=True, text=True, cwd=str(cwd or Path.cwd()),
            env=self._env(), check=False,
        )

    def _toml_path(self, project: Path) -> Path:
        return project / "ai-specs" / "ai-specs.toml"

    def _load(self, project: Path) -> dict:
        with open(self._toml_path(project), "rb") as f:
            return tomllib.load(f)

    # ── Real-world fixtures built via skills-add ──

    def test_remove_last_dep_with_array_fields(self):
        """Remove the SECOND/last dep built by skills-add (with scope/auto_invoke)."""
        project = self._empty_project()
        self._add(project, "first-skill", "https://github.com/test/first.git")
        self._add(project, "second-skill", "https://github.com/test/second.git")

        proc = self._run("second-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        # Must still be valid TOML.
        data = self._load(project)
        deps = data.get("deps", []) or []
        ids = [d.get("id") for d in deps]
        self.assertEqual(ids, ["first-skill"])

        # Surviving dep intact with all its fields.
        survivor = deps[0]
        self.assertEqual(survivor["source"], "https://github.com/test/first.git")
        self.assertEqual(survivor["scope"], ["root"])
        self.assertEqual(survivor["auto_invoke"], ["When working on first-skill"])

        # No leaked fields into [project] or other tables.
        self.assertNotIn("scope", data.get("project", {}))
        self.assertNotIn("auto_invoke", data.get("project", {}))
        self.assertNotIn("vendor_attribution", data.get("project", {}))

        # Removed id absent.
        raw = self._toml_path(project).read_text(encoding="utf-8")
        self.assertNotIn("second-skill", raw)

    def test_remove_first_dep_with_array_fields(self):
        """Remove the FIRST dep built by skills-add; the second survives intact."""
        project = self._empty_project()
        self._add(project, "first-skill", "https://github.com/test/first.git")
        self._add(project, "second-skill", "https://github.com/test/second.git")

        proc = self._run("first-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        data = self._load(project)
        ids = [d.get("id") for d in (data.get("deps", []) or [])]
        self.assertEqual(ids, ["second-skill"])
        self.assertNotIn("scope", data.get("project", {}))

    def test_remove_middle_dep(self):
        """Remove a MIDDLE dep (3 deps); both neighbors stay intact."""
        project = self._empty_project()
        self._add(project, "alpha", "https://github.com/test/alpha.git")
        self._add(project, "beta", "https://github.com/test/beta.git")
        self._add(project, "gamma", "https://github.com/test/gamma.git")

        proc = self._run("beta", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        data = self._load(project)
        deps = data.get("deps", []) or []
        ids = [d.get("id") for d in deps]
        self.assertEqual(ids, ["alpha", "gamma"])
        # Neighbors intact.
        by_id = {d["id"]: d for d in deps}
        self.assertEqual(by_id["alpha"]["source"], "https://github.com/test/alpha.git")
        self.assertEqual(by_id["alpha"]["auto_invoke"], ["When working on alpha"])
        self.assertEqual(by_id["gamma"]["source"], "https://github.com/test/gamma.git")
        self.assertEqual(by_id["gamma"]["auto_invoke"], ["When working on gamma"])

    def test_remove_substring_id_does_not_remove_superstring(self):
        """Removing 'foo' must not remove 'foo-bar'."""
        project = self._empty_project()
        self._add(project, "foo", "https://github.com/test/foo.git")
        self._add(project, "foo-bar", "https://github.com/test/foobar.git")

        proc = self._run("foo", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        data = self._load(project)
        ids = [d.get("id") for d in (data.get("deps", []) or [])]
        self.assertEqual(ids, ["foo-bar"])

    # ── Original simple-fixture cases ──

    def test_remove_removes_deps_block(self):
        project = self._project_with_manifest()
        proc = self._run("my-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        data = self._load(project)
        ids = [d.get("id") for d in (data.get("deps", []) or [])]
        self.assertNotIn("my-skill", ids)

    def test_remove_preserves_other_deps(self):
        project = self._project_with_manifest()
        proc = self._run("my-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        data = self._load(project)
        deps = data.get("deps", []) or []
        ids = [d.get("id") for d in deps]
        self.assertNotIn("my-skill", ids)
        self.assertIn("other-skill", ids)
        survivor = next(d for d in deps if d["id"] == "other-skill")
        self.assertEqual(survivor["source"], "https://github.com/test/other.git")

    def test_remove_fails_when_dep_not_found(self):
        project = self._project_with_manifest()
        proc = self._run("nonexistent", str(project))
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("not found", proc.stderr)

    def test_remove_fails_without_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = self._run("my-skill", tmp)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("not found", proc.stderr)

    def test_remove_fails_without_id(self):
        project = self._project_with_manifest()
        proc = subprocess.run(
            ["bash", str(SKILLS_REMOVE_SCRIPT)],
            capture_output=True, text=True, cwd=str(project), check=False,
        )
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("missing", proc.stderr)

    def test_remove_help_exits_zero(self):
        project = self._project_with_manifest()
        proc = subprocess.run(
            ["bash", str(SKILLS_REMOVE_SCRIPT), "--help"],
            capture_output=True, text=True, cwd=str(project), check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("ai-specs skills remove", proc.stdout)

    def test_remove_preserves_unknown_flags_rejected(self):
        project = self._project_with_manifest()
        proc = subprocess.run(
            ["bash", str(SKILLS_REMOVE_SCRIPT), "--unknown-flag"],
            capture_output=True, text=True, cwd=str(project), check=False,
        )
        self.assertNotEqual(proc.returncode, 0)


if __name__ == "__main__":
    unittest.main()
