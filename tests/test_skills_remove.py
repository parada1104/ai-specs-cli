"""Tests for ai-specs skills remove (lib/skills-remove.sh)."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS_REMOVE_SCRIPT = ROOT / "lib" / "skills-remove.sh"


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

    def _project_with_manifest(self, content: str = _manifest_with_dep()) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(content, encoding="utf-8")
        return project

    def _run(self, *args, cwd=None) -> subprocess.CompletedProcess:
        env = {**os.environ, "AI_SPECS_HOME": str(ROOT)}
        return subprocess.run(
            ["bash", str(SKILLS_REMOVE_SCRIPT), *args],
            capture_output=True, text=True, cwd=str(cwd or Path.cwd()), env=env, check=False,
        )

    def test_remove_removes_deps_block(self):
        project = self._project_with_manifest()
        proc = self._run("my-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertNotIn('id = "my-skill"', manifest)

    def test_remove_preserves_other_deps(self):
        project = self._project_with_manifest()
        proc = self._run("my-skill", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertNotIn('id = "my-skill"', manifest)
        self.assertIn('id = "other-skill"', manifest)
        self.assertIn('source = "https://github.com/test/other.git"', manifest)

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
