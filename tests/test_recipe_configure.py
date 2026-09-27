"""Black-box tests for `ai-specs recipe configure` (converted from recipe-configure.py internals).

Exit-code contract (parity contract §2, FROZEN):
  0 ok/no-op/dry-run; 1 write/sync/doctor failure; 2 argparse;
  3 ConfigureError validation/unknown key/secret literal; 4 blocked by [tool] CLI version policy.
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


class RecipeConfigureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # One shared isolated install root for the whole class: the repo catalog
        # stays symlinked so worktree-flow / trello-mcp-workflow resolve, and the
        # per-test "x" recipe is materialized into a real directory by
        # populate_catalog without touching the repository catalog.
        cls._home_tmp = tempfile.TemporaryDirectory(prefix="ai-specs-home-class-")
        cls.addClassCleanup(cls._home_tmp.cleanup)
        cls.home = isolated_home(Path(cls._home_tmp.name))

    def _project(self, config: str = "") -> tuple[tempfile.TemporaryDirectory, Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.worktree-flow]\nenabled = true\nversion = '1.4.0'\n\n"
            "[recipes.worktree-flow.config]\n"
            + config,
            encoding="utf-8",
        )
        return tmp, root, manifest

    def _configure(self, root: Path, *args: str):
        return invoke(root, "recipe", "configure", *args, cli_home=self.home)

    def _configure_json(self, root: Path, *args: str) -> tuple[dict, object]:
        result = self._configure(root, *args, "--json")
        return json.loads(result.stdout), result

    def _sync_fail_home(self) -> Path:
        """A fresh install root whose cold cache is unwritable.

        The sync's first write step (bundled skills + commands) must then fail,
        deterministically, regardless of test order.
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        os.chmod(home / "cache", 0o500)
        self.addCleanup(os.chmod, home / "cache", 0o755)
        return home

    @staticmethod
    def _git(*args: str, cwd: Path) -> None:
        subprocess.run(
            ["git", *args], cwd=cwd, check=True, capture_output=True, text=True
        )

    def _git_project_with_submodule(self) -> tuple[tempfile.TemporaryDirectory, Path]:
        """A real git repo with one initialized submodule (libs/core).

        `auto` topology resolution only reports monorepo-submodules when
        `git submodule status` shows an initialized entry, so the fixture
        registers the gitlink and runs `git submodule init`.
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        identity = ("-c", "user.email=test@example.test", "-c", "user.name=test")
        self._git("init", "-q", cwd=root)
        (root / "libs" / "core").mkdir(parents=True)
        self._git("init", "-q", cwd=root / "libs" / "core")
        (root / "libs" / "core" / "fixture.txt").write_text("x", encoding="utf-8")
        self._git(*identity, "add", "-A", cwd=root / "libs" / "core")
        self._git(*identity, "commit", "-qm", "init", cwd=root / "libs" / "core")
        sha = subprocess.run(
            ["git", "-C", str(root / "libs" / "core"), "rev-parse", "HEAD"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        self._git("update-index", "--add", "--cacheinfo", f"160000,{sha},libs/core", cwd=root)
        (root / ".gitmodules").write_text(
            '[submodule "libs/core"]\n\tpath = libs/core\n'
            "\turl = https://example.test/libs/core.git\n",
            encoding="utf-8",
        )
        self._git("submodule", "init", cwd=root)
        return tmp, root

    def test_inspect_json_is_deterministic_and_contains_schema_state(self):
        tmp, root, _manifest = self._project("integration_branch = 'main'\nkeep_me = 'x'\n")
        self.addCleanup(tmp.cleanup)
        first = self._configure(root, "worktree-flow", "--inspect", "--json")
        second = self._configure(root, "worktree-flow", "--inspect", "--json")
        self.assertEqual(first.returncode, 0)
        self.assertEqual(first.stdout, second.stdout)
        doc = json.loads(first.stdout)
        self.assertEqual(doc["schema_version"], 1)
        self.assertEqual(doc["current_config"]["integration_branch"], "main")
        self.assertIn("repo_topology", {field["key"] for field in doc["schema"]["fields"]})
        self.assertEqual(doc["unknown_keys"], ["keep_me"])

    def test_topology_grounding_uses_resolution_without_init_contract(self):
        tmp, root = self._git_project_with_submodule()
        (root / "ai-specs").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.worktree-flow]\nenabled = true\nversion = '1.4.0'\n",
            encoding="utf-8",
        )
        result = self._configure(root, "worktree-flow", "--inspect", "--json")
        self.assertEqual(result.returncode, 0)
        topology = json.loads(result.stdout)["grounding"]["topology"]
        self.assertEqual(topology["resolved"], "monorepo-submodules")
        self.assertEqual(topology["via"], "auto")
        self.assertEqual(topology["submodules"], ["libs/core"])

    def test_apply_routes_repo_topology_to_project_field(self):
        """T4 — the project owns topology; recipe config must not gain it."""
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "repo_topology=monorepo-apps"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "ok")
        data = tomllib.loads(manifest.read_text())
        self.assertEqual(data["project"]["repo_topology"], "monorepo-apps")
        self.assertNotIn("repo_topology", data["recipes"]["worktree-flow"].get("config") or {})

    def test_grounding_reports_project_source(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        lines = manifest.read_text().splitlines()
        lines.insert(1, 'repo_topology = "monorepo-apps"')
        manifest.write_text("\n".join(lines) + "\n")
        result = self._configure(root, "worktree-flow", "--inspect", "--json")
        self.assertEqual(result.returncode, 0)
        topology = json.loads(result.stdout)["grounding"]["topology"]
        self.assertEqual(topology["resolved"], "monorepo-apps")
        self.assertEqual(topology["source"], "project")

    def test_apply_rejects_unknown_key_without_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "not_in_schema=x"
        )
        self.assertEqual(result.returncode, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_pin_violation_blocks_before_writer_and_sync(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "integration_branch=dev", "--sync"
        )
        self.assertEqual(result.returncode, 4)
        self.assertEqual(report["status"], "blocked")
        self.assertEqual(manifest.read_bytes(), before)
        self.assertFalse(report["sync"]["ran"])

    def test_sync_failure_is_partial_after_successful_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        home = self._sync_fail_home()
        result = invoke(root, "recipe", "configure", "worktree-flow",
                        "--set", "integration_branch=dev", "--sync", "--json", cli_home=home)
        report = json.loads(result.stdout)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(report["status"], "partial")
        self.assertTrue(report["sync"]["ran"])
        self.assertNotEqual(report["sync"]["exit_code"], 0)
        self.assertTrue(report["sync"]["failed_step"])
        self.assertFalse(report["sync"]["rolled_back"])
        self.assertFalse(report["sync"]["lock_stamped"])
        self.assertIn('integration_branch = "dev"', manifest.read_text())

    # TRIAGE: recipe configure missing surface — parse_doctor_summary's
    # arbitrary-unparsable-doctor-output branch is not reachable through the
    # CLI (the doctor always emits a Summary line when it runs). The closest
    # observable equivalent is the partial-sync path, where no doctor summary
    # is parsed and the verify block must carry no fabricated counts.
    def test_unparsed_doctor_summary_is_not_zero(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        home = self._sync_fail_home()
        result = invoke(root, "recipe", "configure", "worktree-flow",
                        "--set", "integration_branch=dev", "--sync", "--json", cli_home=home)
        report = json.loads(result.stdout)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(report["status"], "partial")
        self.assertFalse(report["verify"]["parsed"])
        self.assertIsNone(report["verify"]["warn"])
        self.assertIsNone(report["verify"]["error"])

    def test_secret_literal_is_rejected_and_env_reference_allowed(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        secret_home = isolated_home(Path(tmp.name), catalog=False)
        populate_catalog(
            secret_home,
            "x",
            '[recipe]\nid = "x"\nname = "X"\ndescription = "d"\nversion = "1.0.0"\n\n'
            '[config.api_token]\nrequired = false\ntype = "string"\n',
        )
        tmp2 = tempfile.TemporaryDirectory()
        self.addCleanup(tmp2.cleanup)
        root = Path(tmp2.name)
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.parent.mkdir()
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.x]\nenabled = true\nversion = '1.0.0'\n\n"
            "[recipes.x.config]\n",
            encoding="utf-8",
        )
        result = invoke(root, "recipe", "configure", "x", "--set", "api_token=literal",
                        "--json", cli_home=secret_home)
        report = json.loads(result.stdout)
        self.assertEqual(result.returncode, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertNotIn("literal", manifest.read_text())
        result = invoke(root, "recipe", "configure", "x",
                        "--set", 'api_token="${env:TRELLO_API_KEY}"',
                        "--json", cli_home=secret_home)
        report = json.loads(result.stdout)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "ok")

    def test_parse_assignment_accepts_structured_table(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        reconcile = (
            'reconcile={scope_field="board_id",max_age_seconds=900,'
            'expectations=[{event="delivery",property="list",config_field="default_list"}]}'
        )
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", reconcile, "--dry-run"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["applied"]["changed"][0]["key"], "reconcile")
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "reconcile=not_toml", "--dry-run"
        )
        self.assertEqual(result.returncode, 3)
        self.assertIn("invalid TOML value", report["reason"])

    def test_no_gitmodules_surfaces_monorepo_apps_question(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        result = self._configure(root, "worktree-flow", "--inspect", "--json")
        self.assertEqual(result.returncode, 0)
        assumptions = json.loads(result.stdout)["assumptions"]
        self.assertTrue(any("monorepo-apps" in item for item in assumptions))

    def test_enum_value_is_rejected_without_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "gate_mode=invalid"
        )
        self.assertEqual(result.returncode, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_lock_staleness_is_informational_gap(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            "[meta]\ncli_version = '0.0.1'\n", encoding="utf-8"
        )
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "integration_branch=dev"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "ok")
        self.assertTrue(any("0.0.1" in gap for gap in report["gaps"]))

    def test_ignore_cli_version_is_recorded_and_forwarded(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        # Without the flag the pin blocks the whole apply (exit 4).
        report, result = self._configure_json(root, "worktree-flow", "--set", "integration_branch=dev")
        self.assertEqual(result.returncode, 4)
        self.assertFalse(report["preflight"]["ignore_cli_version"])
        # With the flag the apply proceeds and sync itself runs to completion:
        # the flag is recorded in preflight and forwarded to the sync command
        # (the final doctor verification then fails on the pin, exit 1).
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "integration_branch=dev",
            "--sync", "--ignore-cli-version",
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(report["status"], "failed")
        self.assertTrue(report["preflight"]["ignore_cli_version"])
        self.assertTrue(report["sync"]["ran"])
        self.assertEqual(report["sync"]["exit_code"], 0)

    def test_noop_report_has_no_changed_keys(self):
        tmp, root, manifest = self._project("integration_branch='main'\n")
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "worktree-flow", "--set", "integration_branch=main"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "no-op")
        self.assertEqual(report["applied"]["changed"], [])
        self.assertEqual(manifest.read_bytes(), before)

    def test_recipe_subcommand_help_lists_configure(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        result = invoke(root, "recipe", "--help", cli_home=self.home, append_root=False)
        self.assertEqual(result.returncode, 0)
        self.assertIn("configure <id>", result.stdout)

    def test_trello_inspect_surfaces_init_and_secret_env_names(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(
            "[project]\nname='fixture'\n\n"
            "[recipes.trello-mcp-workflow]\nenabled=true\nversion='1.3.0'\n\n"
            "[recipes.trello-mcp-workflow.config]\n"
        )
        result = self._configure(root, "trello-mcp-workflow", "--inspect", "--json")
        self.assertEqual(result.returncode, 0)
        doc = json.loads(result.stdout)
        self.assertTrue(doc["grounding"]["init"]["present"])
        self.assertEqual(doc["grounding"]["init"]["needs_mcp"], ["trello"])
        self.assertIn("TRELLO_API_KEY", doc["grounding"]["mcp"]["env_vars"])
        self.assertNotIn("$TRELLO_API_KEY", result.stdout)

    _TRELLO_BASE = (
        "[project]\nname='fixture'\n\n"
        "[recipes.trello-mcp-workflow]\nenabled=true\nversion='1.3.0'\n\n"
        "[recipes.trello-mcp-workflow.config]\n"
        'board_id = "69ec097f13e2d38ecd89a557"\n'
    )

    def test_trello_inspect_lists_reconcile_table_and_not_unknown(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(
            self._TRELLO_BASE
            + "\n[recipes.trello-mcp-workflow.config.reconcile]\n"
            'scope_field = "board_id"\n'
            "max_age_seconds = 900\n\n"
            "[[recipes.trello-mcp-workflow.config.reconcile.expectations]]\n"
            'event = "delivery"\nproperty = "list"\nconfig_field = "default_list"\n'
        )
        result = self._configure(root, "trello-mcp-workflow", "--inspect", "--json")
        self.assertEqual(result.returncode, 0)
        doc = json.loads(result.stdout)
        types = {field["key"]: field["type"] for field in doc["schema"]["fields"]}
        self.assertEqual(types.get("reconcile"), "table")
        self.assertNotIn("reconcile", doc["unknown_keys"])
        self.assertEqual(
            doc["current_config"]["reconcile"]["scope_field"], "board_id"
        )

    def test_parse_assignment_accepts_dotted_structured_key(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "reconcile.max_age_seconds=1200", "--dry-run"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["applied"]["changed"][0]["key"], "reconcile.max_age_seconds")
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set",
            'reconcile.expectations=[{event="merge",property="list",config_field="done_list"}]',
            "--dry-run",
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(
            report["applied"]["changed"][0]["to"],
            [{"event": "merge", "property": "list", "config_field": "done_list"}],
        )
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "reconcile.max_age_seconds=not_a_number",
            "--dry-run",
        )
        self.assertEqual(result.returncode, 3)
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "not_a_table.thing=1", "--dry-run"
        )
        self.assertEqual(result.returncode, 3)

    _TRELLO_RECONCILE_INLINE = (
        _TRELLO_BASE + 'reconcile = { scope_field = "board_id", max_age_seconds = 900, '
        'expectations = [{ event = "delivery", property = "list", '
        'config_field = "default_list" }] }\n'
    )

    def test_apply_dotted_update_edits_existing_reconcile_table(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(self._TRELLO_RECONCILE_INLINE)
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "reconcile.max_age_seconds=1200"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "ok")
        self.assertEqual(
            report["applied"]["changed"][0]["key"], "reconcile.max_age_seconds"
        )
        cfg = tomllib.loads(manifest.read_text())["recipes"]["trello-mcp-workflow"]["config"]
        self.assertEqual(cfg["reconcile"]["max_age_seconds"], 1200)
        self.assertEqual(cfg["reconcile"]["scope_field"], "board_id")
        self.assertEqual(
            cfg["reconcile"]["expectations"][0]["config_field"], "default_list"
        )

    def test_apply_dotted_update_rejects_wrong_type_without_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(self._TRELLO_RECONCILE_INLINE)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", 'reconcile.max_age_seconds="soon"'
        )
        self.assertEqual(result.returncode, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_apply_dotted_noop_preserves_bytes(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(self._TRELLO_RECONCILE_INLINE)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set", "reconcile.max_age_seconds=900"
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "no-op")
        self.assertEqual(manifest.read_bytes(), before)

    def test_apply_accepts_structured_table_key(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(self._TRELLO_BASE)
        reconcile = {
            "scope_field": "board_id",
            "max_age_seconds": 900,
            "expectations": [
                {"event": "delivery", "property": "list", "config_field": "default_list"}
            ],
        }
        report, result = self._configure_json(
            root, "trello-mcp-workflow", "--set",
            'reconcile={scope_field="board_id",max_age_seconds=900,'
            'expectations=[{event="delivery",property="list",config_field="default_list"}]}',
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(report["status"], "ok")
        written = tomllib.loads(manifest.read_text())
        self.assertEqual(
            written["recipes"]["trello-mcp-workflow"]["config"]["reconcile"], reconcile
        )

    def test_apply_rejects_malformed_structured_table(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(self._TRELLO_BASE)
        before = manifest.read_bytes()
        report, result = self._configure_json(
            root,
            "trello-mcp-workflow",
            "--set",
            'reconcile={scope_field="board_id",bogus=1}',
        )
        self.assertEqual(result.returncode, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
