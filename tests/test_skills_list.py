"""Tests for ai-specs skills list (lib/skills-list.sh)."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS_LIST_SCRIPT = ROOT / "lib" / "skills-list.sh"
SKILLS_DISPATCH_SCRIPT = ROOT / "lib" / "skills.sh"


SKILL_MD = (
    "---\n"
    "name: {name}\n"
    'description: {desc}\n'
    "---\n"
    "# {name}\n\nBody.\n"
)


class SkillsListTests(unittest.TestCase):
    """Test skills-list.sh via subprocess (hermetic)."""

    def setUp(self):
        # A fake AI_SPECS_HOME with a synthetic catalog so the catalog section
        # is hermetic and does not depend on the real shipped catalog.
        self._home_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._home_tmp.cleanup)
        self.home = Path(self._home_tmp.name)
        catalog = self.home / "catalog" / "skills" / "cat-skill"
        catalog.mkdir(parents=True)
        (catalog / "SKILL.md").write_text(
            SKILL_MD.format(name="cat-skill", desc="A catalog skill"),
            encoding="utf-8",
        )

    def _env(self):
        return {**os.environ, "AI_SPECS_HOME": str(self.home)}

    def _project(self, manifest: str | None = None, skills: dict | None = None) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        if manifest is not None:
            (ai_specs / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        if skills is not None:
            sdir = ai_specs / "skills"
            sdir.mkdir()
            for name, desc in skills.items():
                d = sdir / name
                d.mkdir()
                if desc is not None:
                    (d / "SKILL.md").write_text(
                        SKILL_MD.format(name=name, desc=desc), encoding="utf-8"
                    )
        return project

    def _run(self, *args, cwd=None) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(SKILLS_LIST_SCRIPT), *args],
            capture_output=True, text=True, cwd=str(cwd or Path.cwd()),
            env=self._env(), check=False,
        )

    def test_lists_deps_local_and_catalog(self):
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(
            manifest=manifest,
            skills={"local-skill": "A local skill description"},
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        out = proc.stdout
        # Registered deps
        self.assertIn("vendored-skill", out)
        self.assertIn("https://github.com/test/repo.git", out)
        # Local skills + description rendered
        self.assertIn("local-skill", out)
        self.assertIn("A local skill description", out)
        # Catalog skills + description rendered
        self.assertIn("cat-skill", out)
        self.assertIn("A catalog skill", out)

    def test_empty_deps(self):
        project = self._project(manifest='[project]\nname = "test"\n')
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("Registered deps", proc.stdout)
        self.assertIn("(none)", proc.stdout)

    def test_missing_skills_dir(self):
        project = self._project(manifest='[project]\nname = "test"\n')
        # No ai-specs/skills dir created.
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("Local skills", proc.stdout)
        self.assertIn("(not found)", proc.stdout)

    def test_malformed_manifest_no_traceback(self):
        # Invalid TOML: duplicate-style/garbage that tomllib rejects.
        bad = '[project]\nname = "test"\n[[deps]]\nid = \n'
        project = self._project(manifest=bad)
        proc = self._run(str(project))
        combined = proc.stdout + proc.stderr
        # No Python stacktrace leaked.
        self.assertNotIn("Traceback (most recent call last)", combined)
        self.assertNotIn("tomllib.TOMLDecodeError", combined)
        # A one-line diagnostic instead.
        self.assertIn("manifest invalid", proc.stderr)

    def test_dispatcher_unknown_subcommand_exits_2(self):
        proc = subprocess.run(
            ["bash", str(SKILLS_DISPATCH_SCRIPT), "bogus-subcommand"],
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(proc.returncode, 2)
        self.assertIn("unknown subcommand", proc.stderr)

    def test_local_skills_excludes_registered_deps(self):
        """Vendored deps (synced) must NOT appear in the Local skills section."""
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(
            manifest=manifest,
            skills={
                "vendored-skill": "Should NOT appear in local",
                "local-skill": "Real local skill",
            },
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        out = proc.stdout
        # Vendored dep should appear in "Registered deps"
        self.assertIn("vendored-skill", out)
        # Split out the Local skills section
        if "Local skills" in out:
            local_section = out.split("Local skills")[1].split("Available catalog")[0]
            self.assertNotIn("vendored-skill", local_section)
        # Local skill should still appear
        self.assertIn("local-skill", out)

    def test_help_exits_zero(self):
        proc = subprocess.run(
            ["bash", str(SKILLS_LIST_SCRIPT), "--help"],
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("ai-specs skills list", proc.stdout)

    def test_dispatcher_list_path(self):
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(manifest=manifest)
        # Dispatcher resolves LIB_DIR from AI_SPECS_HOME, so point it at the
        # real repo root (which contains lib/ and a catalog).
        env = {**os.environ, "AI_SPECS_HOME": str(ROOT)}
        proc = subprocess.run(
            ["bash", str(SKILLS_DISPATCH_SCRIPT), "list", str(project)],
            capture_output=True, text=True, env=env, check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("vendored-skill", proc.stdout)


if __name__ == "__main__":
    unittest.main()
