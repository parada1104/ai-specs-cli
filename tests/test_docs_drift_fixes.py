"""Black-box contract tests for the docs-drift defect card (D12, D25-D34).

Every test drives a CLI entry (`bin/ai-specs` or a `lib/*.sh` wrapper) and
asserts on exit codes, stdout/stderr, or emitted file trees. No new coupled
imports of `lib/_internal` modules — the single exception is the interactive
agents-submenu seam (D12), which has no CLI surface; it is driven in a
subprocess via the same loader `tests/test_hub.py` already uses, and the
assertions are on the observable file effect (the rewritten manifest).
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BIN_AI_SPECS = ROOT / "bin" / "ai-specs"
LIB_UPGRADE = ROOT / "lib" / "upgrade.sh"
LIB_VERSION = ROOT / "lib" / "version.sh"
LIB_SKILLS_LIST = ROOT / "lib" / "skills-list.sh"
LIB_SKILLS_REMOVE = ROOT / "lib" / "skills-remove.sh"
LIB_RECIPE_LIST = ROOT / "lib" / "recipe-list.sh"
LIB_RECIPE_ADD = ROOT / "lib" / "recipe-add.sh"
LIB_RECIPE_INIT = ROOT / "lib" / "recipe-init.sh"
LIB_RECIPE_REMOVE = ROOT / "lib" / "recipe-remove.sh"
HUB_PY = ROOT / "lib" / "_internal" / "hub.py"


def run(args, cwd=None, env=None, check=True):
    result = subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True)
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode, args, output=result.stdout, stderr=result.stderr
        )
    return result


def tmp_dir() -> Path:
    tmp = tempfile.TemporaryDirectory()
    return Path(tmp.name)


class HelpDiscoverabilityTests(unittest.TestCase):
    """D25 — every reachable subcommand appears in `ai-specs help`."""

    def test_help_lists_hub(self):
        result = run([str(BIN_AI_SPECS), "help"])
        self.assertIn("hub [path]", result.stdout)

    def test_help_lists_recipe_configure(self):
        result = run([str(BIN_AI_SPECS), "help"])
        self.assertIn("recipe configure", result.stdout)

    def test_help_hub_line_does_not_imply_read_only(self):
        """D26 — hub rewrites config and runs commands; the help must not
        imply a passive status view."""
        result = run([str(BIN_AI_SPECS), "help"])
        hub_lines = [
            line for line in result.stdout.splitlines() if "hub [path]" in line
        ]
        self.assertEqual(len(hub_lines), 1)
        self.assertNotIn("read-only", hub_lines[0].lower())


class ReadOnlyClaimTests(unittest.TestCase):
    """D26 — no help text claims a behavior the code contradicts."""

    def test_top_level_help_does_not_claim_doctor_read_only(self):
        result = run([str(BIN_AI_SPECS), "help"])
        doctor_lines = [
            line for line in result.stdout.splitlines() if "doctor" in line
        ]
        self.assertTrue(doctor_lines)
        for line in doctor_lines:
            self.assertNotIn("read-only", line.lower())

    def test_doctor_help_does_not_claim_read_only(self):
        result = run([str(BIN_AI_SPECS), "doctor", "--help"])
        self.assertNotIn("read-only", result.stdout.lower())

    def test_top_level_help_does_not_claim_refresh_bundled_no_in_project_writes(self):
        result = run([str(BIN_AI_SPECS), "help"])
        rb_lines = [
            line for line in result.stdout.splitlines() if "refresh-bundled" in line
        ]
        self.assertTrue(rb_lines)
        for line in rb_lines:
            self.assertNotIn("no in-project writes", line.lower())

    def test_refresh_bundled_help_does_not_claim_zero_in_project_writes(self):
        result = run([str(BIN_AI_SPECS), "refresh-bundled", "--help"])
        self.assertNotIn("zero in-project writes", result.stdout.lower())


class InitForceDocTests(unittest.TestCase):
    """D27 — `init --force` only refreshes the root .gitignore agent block."""

    def test_init_help_force_flag_mentions_gitignore(self):
        result = run([str(BIN_AI_SPECS), "init", "--help"])
        lines = result.stdout.splitlines()
        force_idx = [
            i for i, line in enumerate(lines) if line.strip().startswith("--force")
        ]
        self.assertEqual(len(force_idx), 1)
        block = "\n".join(lines[force_idx[0] : force_idx[0] + 3])
        self.assertIn(".gitignore", block)

    def test_init_help_force_flag_does_not_claim_agents_md_regen(self):
        result = run([str(BIN_AI_SPECS), "init", "--help"])
        self.assertNotIn("Re-render templates", result.stdout)
        self.assertNotIn("Regenerate AGENTS.md", result.stdout)


class UpgradeMessageAndExitCodeTests(unittest.TestCase):
    """D28 (literal \\n) and D30 (unknown-argument exit code) for `upgrade`."""

    def fake_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def setup_install(self, home: Path) -> Path:
        """Minimal fake global install, mirroring tests/test_upgrade.py."""
        ai_specs = home / ".ai-specs"
        (ai_specs / "bin").mkdir(parents=True)
        (ai_specs / "lib" / "_internal").mkdir(parents=True)
        (ai_specs / "bin" / "ai-specs").write_text(BIN_AI_SPECS.read_text())
        (ai_specs / "lib" / "upgrade.sh").write_text(LIB_UPGRADE.read_text())
        (ai_specs / "VERSION").write_text("0.21.0\n")
        run(["git", "init", "-b", "main"], cwd=ai_specs)
        run(["git", "config", "user.email", "t@t.com"], cwd=ai_specs)
        run(["git", "config", "user.name", "T"], cwd=ai_specs)
        run(["git", "add", "."], cwd=ai_specs)
        run(["git", "commit", "-m", "init"], cwd=ai_specs)
        local_bin = home / ".local" / "bin"
        local_bin.mkdir(parents=True)
        (local_bin / "ai-specs").symlink_to(ai_specs / "bin" / "ai-specs")
        return ai_specs

    def make_env(self, home: Path) -> dict:
        env = os.environ.copy()
        env["HOME"] = str(home)
        env["AI_SPECS_HOME"] = str(home / ".ai-specs")
        return env

    def test_dirty_tree_abort_renders_real_newline(self):
        home = self.fake_home()
        self.setup_install(home)
        (home / ".ai-specs" / "foo.txt").write_text("dirty")
        script = home / ".ai-specs" / "lib" / "upgrade.sh"
        result = run(["bash", str(script)], env=self.make_env(home), check=False)
        self.assertEqual(result.returncode, 3, msg=result.stderr)
        # The porcelain listing must start on its own line, not glued onto the
        # sentence behind a literal backslash-n.
        self.assertIn("use --force.\n?? foo.txt", result.stderr)
        self.assertNotIn("\\n", result.stderr)

    def test_unknown_argument_exits_2_not_1(self):
        """Exit 1 is reserved for 'broken or missing installation'."""
        home = self.fake_home()
        self.setup_install(home)
        script = home / ".ai-specs" / "lib" / "upgrade.sh"
        result = run(
            ["bash", str(script), "--nonsense"], env=self.make_env(home), check=False
        )
        self.assertEqual(result.returncode, 2, msg=result.stderr)
        self.assertIn("Unknown argument", result.stderr)


class VersionMissingFileTests(unittest.TestCase):
    """D32 — a missing VERSION file degrades to 'unknown', never a raw cat error."""

    def test_missing_version_prints_unknown_exit_0(self):
        home = tmp_dir()
        shutil.rmtree(home, ignore_errors=True)
        self.addCleanup(shutil.rmtree, home, ignore_errors=True)
        lib = home / "lib"
        lib.mkdir(parents=True)
        shutil.copy(LIB_VERSION, lib / "version.sh")
        result = run(["bash", str(lib / "version.sh")], check=False)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertEqual(result.stdout.strip(), "unknown")


class AddDepNamingTests(unittest.TestCase):
    """D34 — `add-dep` names itself in its own help and hints."""

    def test_add_dep_help_names_add_dep(self):
        result = run([str(BIN_AI_SPECS), "add-dep", "--help"])
        self.assertIn("Usage: ai-specs add-dep", result.stdout)
        self.assertNotIn("Usage: ai-specs skills add", result.stdout)

    def test_add_dep_missing_url_hint_names_add_dep(self):
        result = run([str(BIN_AI_SPECS), "add-dep"], check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Run 'ai-specs add-dep --help' for usage.", result.stderr)

    def test_skills_add_help_still_names_skills_add(self):
        result = run([str(BIN_AI_SPECS), "skills", "add", "--help"])
        self.assertIn("Usage: ai-specs skills add", result.stdout)


class DoubleDashPositionalTests(unittest.TestCase):
    """D29 — `--` must not swallow positional arguments."""

    def setUp(self):
        self.cwd = tmp_dir()
        self.addCleanup(shutil.rmtree, self.cwd, ignore_errors=True)

    def project(self, manifest: str | None = None) -> Path:
        proj = self.cwd / "proj"
        (proj / "ai-specs").mkdir(parents=True)
        if manifest is not None:
            (proj / "ai-specs" / "ai-specs.toml").write_text(manifest)
        return proj

    def test_skills_list_accepts_path_after_double_dash(self):
        proj = self.project()
        result = run(["bash", str(LIB_SKILLS_LIST), "--", str(proj)], cwd=self.cwd)
        self.assertIn(f"Project: {proj}", result.stdout)

    def test_recipe_list_accepts_path_after_double_dash(self):
        proj = self.project('[agents]\nenabled = []\n\n[meta]\nname = "p"\n')
        result = run(
            ["bash", str(LIB_RECIPE_LIST), "--", str(proj)],
            cwd=self.cwd,
            check=False,
        )
        # cwd is not an ai-specs project; only the passed path may be used.
        combined = result.stdout + result.stderr
        self.assertNotIn("Project not initialized", combined, msg=combined)

    def test_skills_remove_accepts_id_and_path_after_double_dash(self):
        proj = self.project()  # no manifest
        result = run(
            ["bash", str(LIB_SKILLS_REMOVE), "--", "some-dep", str(proj)],
            cwd=self.cwd,
            check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("missing skill id", result.stderr)
        self.assertIn(str(proj), result.stderr)

    def test_recipe_add_accepts_id_and_path_after_double_dash(self):
        proj = self.project('[agents]\nenabled = []\n\n[meta]\nname = "p"\n')
        result = run(
            ["bash", str(LIB_RECIPE_ADD), "--", "no-such-recipe", str(proj)],
            cwd=self.cwd,
            check=False,
        )
        self.assertNotIn("Project not initialized", result.stdout + result.stderr)
        self.assertIn("not found in catalog", result.stdout + result.stderr)

    def test_recipe_init_accepts_id_and_path_after_double_dash(self):
        proj = self.project('[agents]\nenabled = []\n\n[meta]\nname = "p"\n')
        result = run(
            ["bash", str(LIB_RECIPE_INIT), "--", "no-such-recipe", str(proj)],
            cwd=self.cwd,
            check=False,
        )
        combined = result.stdout + result.stderr
        self.assertNotIn("Project not initialized", combined)
        self.assertNotIn("missing recipe id", combined)

    def test_recipe_remove_accepts_id_and_path_after_double_dash(self):
        proj = self.project('[agents]\nenabled = []\n\n[meta]\nname = "p"\n')
        result = run(
            ["bash", str(LIB_RECIPE_REMOVE), "--", "no-such-recipe", str(proj)],
            cwd=self.cwd,
            check=False,
        )
        combined = result.stdout + result.stderr
        self.assertNotIn("Project not initialized", combined)
        self.assertNotIn("missing recipe id", combined)


class SkillsListTomlParseTests(unittest.TestCase):
    """D33 — `skills list` parses TOML for dep ids instead of grepping."""

    SKILL_MD = "---\nname: {name}\ndescription: {desc}\n---\n# {name}\n\nBody.\n"

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.home = Path(tmp.name)

    def _env(self):
        return {**os.environ, "AI_SPECS_HOME": str(self.home)}

    def test_local_skill_named_like_a_top_level_id_is_listed(self):
        proj_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(proj_tmp.cleanup)
        proj = Path(proj_tmp.name)
        manifest = (
            "[meta]\n"
            'id = "phantom"\n'
            "\n"
            "[[deps]]\n"
            'id = "real-dep"\n'
            'source = "https://example.com/real-dep"\n'
        )
        (proj / "ai-specs").mkdir(parents=True)
        (proj / "ai-specs" / "ai-specs.toml").write_text(manifest)
        skills = proj / "ai-specs" / "skills"
        for name in ("phantom", "other-local"):
            (skills / name).mkdir(parents=True)
            (skills / name / "SKILL.md").write_text(
                self.SKILL_MD.format(name=name, desc="d")
            )

        result = run(
            ["bash", str(LIB_SKILLS_LIST), str(proj)], env=self._env()
        )
        out = result.stdout
        # The dep section must still list real-dep (behavior unchanged).
        self.assertIn("real-dep", out)
        # The [meta] id must NOT exclude the local skill of the same name.
        local_section = out.split("Local skills")[1].split("Available catalog")[0]
        self.assertIn("phantom", local_section)
        self.assertIn("other-local", local_section)


class AgentsSubmenuManifestTests(unittest.TestCase):
    """D12 — the interactive agents submenu must not corrupt the manifest.

    Justified seam: `_run_agents_submenu` has no CLI surface (interactive-only,
    TTY required). The test drives it in a subprocess — the same loader
    `tests/test_hub.py` already uses — and asserts the observable file effect.
    """

    DRIVER = """
import importlib.util, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("hub_under_test", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules["hub_under_test"] = mod
spec.loader.exec_module(mod)
mod.pick_many = lambda message, options: sys.argv[3].split(",")
mod.pause = lambda message="Press Enter to return\\u2026": True
rc = mod._run_agents_submenu(None, Path(sys.argv[2]))
sys.exit(rc if rc is not None else 0)
"""

    def _run_submenu(self, manifest: str, selection: str) -> tuple[int, str]:
        proj_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(proj_tmp.cleanup)
        proj = Path(proj_tmp.name)
        (proj / "ai-specs").mkdir(parents=True)
        toml = proj / "ai-specs" / "ai-specs.toml"
        toml.write_text(manifest)
        result = subprocess.run(
            ["python3", "-c", self.DRIVER, str(HUB_PY), str(proj), selection],
            capture_output=True,
            text=True,
        )
        return result.returncode, toml.read_text()

    def test_agents_table_without_enabled_is_completed_not_duplicated(self):
        manifest = '[meta]\nname = "p"\n\n[agents]\n'
        rc, text = self._run_submenu(manifest, "claude,pi")
        self.assertEqual(rc, 0)
        self.assertEqual(text.count("[agents]"), 1, msg=text)
        self.assertIn('enabled = ["claude", "pi"]', text)

    def test_agents_table_last_with_other_keys_gets_enabled(self):
        manifest = '[agents]\nlayout = "compact"\n'
        rc, text = self._run_submenu(manifest, "claude")
        self.assertEqual(rc, 0)
        self.assertEqual(text.count("[agents]"), 1, msg=text)
        self.assertIn('enabled = ["claude"]', text)
        self.assertIn('layout = "compact"', text)

    def test_existing_enabled_line_is_replaced(self):
        manifest = '[agents]\nenabled = ["old"]\n'
        rc, text = self._run_submenu(manifest, "claude")
        self.assertEqual(rc, 0)
        self.assertEqual(text.count("enabled ="), 1, msg=text)
        self.assertIn('enabled = ["claude"]', text)

    def test_missing_agents_table_is_appended_once(self):
        manifest = '[meta]\nname = "p"\n'
        rc, text = self._run_submenu(manifest, "claude")
        self.assertEqual(rc, 0)
        self.assertEqual(text.count("[agents]"), 1, msg=text)
        self.assertIn('enabled = ["claude"]', text)


if __name__ == "__main__":
    unittest.main()
