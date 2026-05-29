"""Tests for ai-specs skills add (lib/skills-add.sh)."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS_ADD_SCRIPT = ROOT / "lib" / "skills-add.sh"


class SkillsAddCliTests(unittest.TestCase):
    """Test skills-add.sh via subprocess (bash script)."""

    def _project_with_manifest(self, content: str = '[project]\nname = "test"\n') -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(content, encoding="utf-8")
        return project

    def _run(self, *args, cwd=None) -> subprocess.CompletedProcess:
        # Registration is the unit under test; vendoring/sync runs a real network
        # clone and is covered elsewhere. Force --no-sync for hermetic tests unless
        # the caller already passed it.
        env = {**os.environ, "AI_SPECS_HOME": str(ROOT)}
        run_args = list(args)
        if "--no-sync" not in run_args:
            run_args.append("--no-sync")
        return subprocess.run(
            ["bash", str(SKILLS_ADD_SCRIPT), *run_args],
            capture_output=True, text=True, cwd=str(cwd or Path.cwd()), env=env, check=False,
        )

    def test_add_appends_deps_block(self):
        project = self._project_with_manifest()
        url = "https://github.com/anthropics/skills"
        proc = self._run(url, str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[[deps]]", manifest)
        self.assertIn('id = "skills"', manifest)
        self.assertIn(f'source = "{url}"', manifest)

    def test_add_with_custom_id(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--id", "my-custom-id",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('id = "my-custom-id"', manifest)

    def test_add_with_subdir(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--subdir", "skills/pdf-processing",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('path = "skills/pdf-processing"', manifest)

    def test_add_with_scope(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--scope", "root,subrepo",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('scope = ["root", "subrepo"]', manifest)

    def test_add_with_trigger(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--trigger", "Custom trigger phrase",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('auto_invoke = ["Custom trigger phrase"]', manifest)

    def test_add_with_license_and_attribution(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--license", "MIT",
            "--attribution", "Anthropic",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('license = "MIT"', manifest)
        self.assertIn('vendor_attribution = "Anthropic"', manifest)

    def test_add_duplicate_fails(self):
        project = self._project_with_manifest()
        url = "https://github.com/anthropics/skills"
        proc1 = self._run(url, str(project))
        self.assertEqual(proc1.returncode, 0, proc1.stderr)

        proc2 = self._run(url, str(project))
        self.assertNotEqual(proc2.returncode, 0)
        self.assertIn("already exists", proc2.stderr)

    def test_add_fails_without_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = subprocess.run(
                ["bash", str(SKILLS_ADD_SCRIPT), "https://example.com/repo.git", tmp],
                capture_output=True, text=True, check=False,
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("not found", proc.stderr)

    def test_add_fails_without_url(self):
        project = self._project_with_manifest()
        proc = subprocess.run(
            ["bash", str(SKILLS_ADD_SCRIPT)],
            capture_output=True, text=True, cwd=str(project), check=False,
        )
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("required", proc.stderr)

    def test_add_help_exits_zero(self):
        project = self._project_with_manifest()
        proc = subprocess.run(
            ["bash", str(SKILLS_ADD_SCRIPT), "--help"],
            capture_output=True, text=True, cwd=str(project), check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("ai-specs skills add", proc.stdout)

    def test_add_prints_summary_before_sync(self):
        project = self._project_with_manifest()
        proc = self._run("https://github.com/anthropics/skills", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("ai-specs skills add", proc.stdout)
        self.assertIn("url:", proc.stdout)
        self.assertIn("id:", proc.stdout)

    def test_add_no_sync_flag(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/anthropics/skills",
            "--no-sync",
            str(project),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("dep registered", proc.stdout)
        # The sync-execution banner ("▸ ai-specs sync") must be absent. The
        # "Run 'ai-specs sync ...'" hint is expected and allowed.
        self.assertNotIn("▸ ai-specs sync", proc.stdout)

    def test_add_derives_id_from_url(self):
        project = self._project_with_manifest()
        proc = self._run("https://github.com/some-org/my-skill-repo.git", str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        manifest = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('id = "my-skill-repo"', manifest)

    def test_add_rejects_invalid_id(self):
        project = self._project_with_manifest()
        proc = self._run(
            "https://github.com/test/My-Uppercase-Skill",
            "--id", "Uppercase-Is-Not-Kebab",
            str(project),
        )
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("kebab-case", proc.stderr)


if __name__ == "__main__":
    unittest.main()
