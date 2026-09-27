"""Tests for ai-specs skills list (lib/skills-list.sh)."""

import hashlib
import os
import re
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
        """Vendored deps (real layout: ai-specs/.deps/<id>/) must NOT appear in
        the Local skills section; only genuine local skills do."""
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(
            manifest=manifest,
            skills={"local-skill": "Real local skill"},
        )
        deps_dir = project / "ai-specs" / ".deps" / "vendored-skill" / "skills" / "vendored-skill"
        deps_dir.mkdir(parents=True)
        (deps_dir / "SKILL.md").write_text(
            SKILL_MD.format(name="vendored-skill", desc="Vendored desc"),
            encoding="utf-8",
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        out = proc.stdout
        local_section = out.split("Local skills")[1].split("Available catalog")[0]
        self.assertNotIn("vendored-skill", local_section)
        self.assertIn("local-skill", local_section)

    # ── D7: dep installed status must check the real .deps layout ──

    def test_dep_status_installed_reports_real_deps_layout(self):
        """A dep synced to ai-specs/.deps/<id>/skills/<id>/ reports installed."""
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(manifest=manifest)
        deps_dir = project / "ai-specs" / ".deps" / "vendored-skill" / "skills" / "vendored-skill"
        deps_dir.mkdir(parents=True)
        (deps_dir / "SKILL.md").write_text(
            SKILL_MD.format(name="vendored-skill", desc="Vendored desc"),
            encoding="utf-8",
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        out = proc.stdout
        self.assertIn("vendored-skill", out)
        self.assertIn("✓ installed", out)
        self.assertNotIn("✗ not synced", out)

    def test_dep_status_not_synced_ignores_stale_skills_dir(self):
        """A legacy copy under ai-specs/skills/<id>/ does NOT satisfy the
        installed status; only the real .deps layout counts."""
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'scope = ["root"]\n'
        )
        project = self._project(
            manifest=manifest,
            skills={"vendored-skill": "Stale legacy copy"},
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("✗ not synced", proc.stdout)
        self.assertNotIn("✓ installed", proc.stdout)

    def test_dep_listing_shows_pinned_ref(self):
        """A [[deps]] entry with a pinned ref surfaces it (D16)."""
        manifest = (
            '[project]\nname = "test"\n'
            "\n[[deps]]\n"
            'id = "vendored-skill"\n'
            'source = "https://github.com/test/repo.git"\n'
            'ref = "v1.2.3"\n'
            'scope = ["root"]\n'
        )
        project = self._project(manifest=manifest)
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("v1.2.3", proc.stdout)

    # ── D8: bundled section must scan the cache tier ──

    @staticmethod
    def _cache_key(project: Path) -> str:
        """Mirror project-cache.cache_key without importing lib/_internal
        (new tests stay black-box): sha256(realpath)[:12]-<sanitized name>."""
        real = str(project.resolve())
        digest = hashlib.sha256(real.encode("utf-8")).hexdigest()[:12]
        safe = re.sub(r"[^A-Za-z0-9._-]+", "-", Path(real).name).strip("-._") or "project"
        return f"{digest}-{safe}"

    def test_bundled_section_scans_cache_bundled_skills(self):
        """CLI-shipped bundled skills live in {cache}/.bundled/skills/ — the
        bundled section must list them from there, not from the project."""
        bundled = self.home / "bundled-skills" / "skill-creator"
        bundled.mkdir(parents=True)
        (bundled / "SKILL.md").write_text(
            SKILL_MD.format(name="skill-creator", desc="Bundled desc"),
            encoding="utf-8",
        )
        project = self._project(manifest='[project]\nname = "test"\n')
        cache_bundled = (
            self.home / "cache" / "projects" / self._cache_key(project)
            / ".bundled" / "skills" / "skill-creator"
        )
        cache_bundled.mkdir(parents=True)
        (cache_bundled / "SKILL.md").write_text(
            SKILL_MD.format(name="skill-creator", desc="Bundled desc"),
            encoding="utf-8",
        )
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        bundled_section = proc.stdout.split("Bundled skills")[1].split("Local skills")[0]
        self.assertIn("skill-creator", bundled_section)
        self.assertIn("Bundled desc", bundled_section)
        self.assertNotIn("(none)", bundled_section)

    def test_bundled_section_empty_when_cache_missing(self):
        """Without a flattened cache the bundled section reports (none)."""
        bundled = self.home / "bundled-skills" / "skill-creator"
        bundled.mkdir(parents=True)
        (bundled / "SKILL.md").write_text(
            SKILL_MD.format(name="skill-creator", desc="Bundled desc"),
            encoding="utf-8",
        )
        project = self._project(manifest='[project]\nname = "test"\n')
        proc = self._run(str(project))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        bundled_section = proc.stdout.split("Bundled skills")[1].split("Local skills")[0]
        self.assertIn("(none)", bundled_section)
        self.assertNotIn("skill-creator", bundled_section)

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
