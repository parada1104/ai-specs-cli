"""Tests for ai-specs skills add (lib/skills-add.sh)."""

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS_ADD_SCRIPT = ROOT / "lib" / "skills-add.sh"
KEPANO_URL = "https://github.com/kepano/obsidian-skills"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"


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

    def test_add_refuses_traversal_and_separator_ids(self):
        """Registration must refuse ids that could escape the managed .deps
        tree (C2-style guard): '..', separators, absolute paths — with a
        clear refusal message and NO manifest write."""
        for bad_id in ("../pwned", "sub/dir", "/abs", "..", "."):
            with tempfile.TemporaryDirectory() as tmp:
                project = Path(tmp)
                ai = project / "ai-specs"
                ai.mkdir()
                (ai / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
                proc = self._run(
                    "https://github.com/test/repo.git", str(project),
                    "--id", bad_id,
                )
                self.assertNotEqual(proc.returncode, 0, f"id {bad_id!r} must be refused")
                self.assertIn("invalid dep id", proc.stderr)
                # No manifest write: the [[deps]] block must be absent.
                self.assertNotIn("[[deps]]", (ai / "ai-specs.toml").read_text(encoding="utf-8"))


class AutoSyncRouteTests(unittest.TestCase):
    """GO-07 S19a: which route the add-dep auto-sync takes.

    The seam (`AI_SPECS_GO_BIN`) is set by the native dispatcher when it execs a
    legacy script. Both directions are observed through a stub's own argv log, so
    neither leg needs the network: with a usable seam the stub IS the sync, and
    with an unusable one the shell route runs against the offline vendoring
    fixture.
    """

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text('[project]\nname = "test"\n', encoding="utf-8")
        return project

    def _stub(self, *, executable: bool) -> tuple[Path, Path]:
        base = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        log = base / "argv.log"
        stub = base / "ai-specs-stub"
        stub.write_text(f'#!/bin/sh\nprintf \'%s\\n\' "$*" >> {log}\nexit 0\n', encoding="utf-8")
        stub.chmod(0o755 if executable else 0o644)
        return stub, log

    def test_auto_sync_prefers_the_native_dispatcher(self):
        """A usable seam means the ported spine runs and no Python does."""
        project = self._project()
        stub, log = self._stub(executable=True)
        env = {**os.environ, "AI_SPECS_HOME": str(ROOT), "AI_SPECS_GO_BIN": str(stub)}
        proc = subprocess.run(
            ["bash", str(SKILLS_ADD_SCRIPT), KEPANO_URL, str(project), "--subdir", "skills/defuddle"],
            capture_output=True, text=True, env=env, check=False, timeout=120,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("▸ ai-specs sync", proc.stdout)
        self.assertTrue(log.exists(), "the native route must run when the seam names an executable")
        self.assertEqual(log.read_text(encoding="utf-8").strip(), f"sync {project}")

    def test_auto_sync_keeps_the_shell_route_without_a_usable_seam(self):
        """An unset, empty or non-executable seam falls back, never fails."""
        project = self._project()
        stub, log = self._stub(executable=False)
        env = {
            **os.environ,
            "AI_SPECS_HOME": str(ROOT),
            "AI_SPECS_GO_BIN": str(stub),
            "AI_SPECS_GATE_OFFLINE": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        }
        proc = subprocess.run(
            ["bash", str(SKILLS_ADD_SCRIPT), KEPANO_URL, str(project), "--subdir", "skills/defuddle"],
            capture_output=True, text=True, env=env, check=False, timeout=120,
        )
        self.assertFalse(log.exists(), "an unusable seam must never be executed")
        self.assertIn("▸ ai-specs sync", proc.stdout, proc.stderr)
        self.assertEqual(proc.returncode, 0, proc.stderr)


if __name__ == "__main__":
    unittest.main()
