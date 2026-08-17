"""Contract tests for the non-interactive recipe configure helper."""
from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
HELPER_PATH = ROOT / "lib" / "_internal" / "recipe-configure.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class RecipeConfigureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(HELPER_PATH, "recipe_configure_internal")

    def _project(self, config: str = "") -> tuple[tempfile.TemporaryDirectory, Path, Path]:
        tmp = tempfile.TemporaryDirectory()
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

    def test_inspect_json_is_deterministic_and_contains_schema_state(self):
        tmp, root, _manifest = self._project("integration_branch = 'main'\nkeep_me = 'x'\n")
        self.addCleanup(tmp.cleanup)
        first = self.mod.inspect_project(root, "worktree-flow")
        second = self.mod.inspect_project(root, "worktree-flow")
        self.assertEqual(json.dumps(first, sort_keys=False), json.dumps(second, sort_keys=False))
        self.assertEqual(first["schema_version"], 1)
        self.assertEqual(first["current_config"]["integration_branch"], "main")
        self.assertIn("repo_topology", {field["key"] for field in first["schema"]["fields"]})
        self.assertEqual(first["unknown_keys"], ["keep_me"])

    def test_topology_grounding_uses_resolution_without_init_contract(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        resolution = self.mod._util.TopologyResolution(
            "monorepo-submodules", "auto", "auto", ("libs/core",), True
        )
        with patch.object(self.mod._util, "resolve_repo_topology", return_value=resolution):
            doc = self.mod.inspect_project(root, "worktree-flow")
        self.assertEqual(doc["grounding"]["topology"]["resolved"], "monorepo-submodules")
        self.assertEqual(doc["grounding"]["topology"]["submodules"], ["libs/core"])

    def test_apply_rejects_unknown_key_without_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, code = self.mod.apply_project(root, "worktree-flow", {"not_in_schema": "x"})
        self.assertEqual(code, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_pin_violation_blocks_before_writer_and_sync(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        before = manifest.read_bytes()
        with patch.object(self.mod._config_write, "update_recipe_config") as writer, patch.object(
            self.mod, "_run_command"
        ) as command:
            report, code = self.mod.apply_project(root, "worktree-flow", {"integration_branch": "dev"}, sync=True)
        self.assertEqual(code, 4)
        self.assertEqual(report["status"], "blocked")
        self.assertEqual(manifest.read_bytes(), before)
        writer.assert_not_called()
        command.assert_not_called()

    def test_sync_failure_is_partial_after_successful_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        with patch.object(self.mod, "_run_command", return_value=(1, "syncing materialize\nERROR: failed\n")):
            report, code = self.mod.apply_project(
                root, "worktree-flow", {"integration_branch": "dev"}, sync=True
            )
        self.assertEqual(code, 1)
        self.assertEqual(report["status"], "partial")
        self.assertEqual(report["sync"]["failed_step"], "materialize")
        self.assertFalse(report["sync"]["rolled_back"])
        self.assertFalse(report["sync"]["lock_stamped"])
        self.assertIn('integration_branch = "dev"', manifest.read_text())

    def test_unparsed_doctor_summary_is_not_zero(self):
        parsed = self.mod.parse_doctor_summary("doctor output without summary")
        self.assertFalse(parsed["parsed"])
        self.assertIsNone(parsed["warn"])
        self.assertIsNone(parsed["error"])

    def test_secret_literal_is_rejected_and_env_reference_allowed(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        with patch.object(self.mod, "_schema_for") as schema:
            schema.return_value.config_schema.fields = {
                "api_token": self.mod._recipe_schema.ConfigField(
                    required=False, type="string"
                )
            }
            report, code = self.mod.apply_project(root, "worktree-flow", {"api_token": "literal"})
        self.assertEqual(code, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertNotIn("literal", manifest.read_text())

    def test_no_gitmodules_surfaces_monorepo_apps_question(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        doc = self.mod.inspect_project(root, "worktree-flow")
        self.assertTrue(any("monorepo-apps" in item for item in doc["assumptions"]))

    def test_enum_value_is_rejected_without_write(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, code = self.mod.apply_project(root, "worktree-flow", {"gate_mode": "invalid"})
        self.assertEqual(code, 3)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_lock_staleness_is_informational_gap(self):
        tmp, root, _manifest = self._project()
        self.addCleanup(tmp.cleanup)
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            "[meta]\ncli_version = '0.0.1'\n", encoding="utf-8"
        )
        report, code = self.mod.apply_project(root, "worktree-flow", {"integration_branch": "dev"})
        self.assertEqual(code, 0)
        self.assertEqual(report["status"], "ok")
        self.assertTrue(any("0.0.1" in gap for gap in report["gaps"]))

    def test_ignore_cli_version_is_recorded_and_forwarded(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        with patch.object(
            self.mod, "_run_command", side_effect=[(0, "sync ok"), (0, "Summary: 1 OK, 0 INFO, 0 WARN, 0 ERROR")]
        ) as command:
            report, code = self.mod.apply_project(
                root, "worktree-flow", {"integration_branch": "dev"}, sync=True, ignore_cli_version=True
            )
        self.assertEqual(code, 0)
        self.assertTrue(report["preflight"]["ignore_cli_version"])
        self.assertIn("--ignore-cli-version", command.call_args_list[0].args[0])

    def test_noop_report_has_no_changed_keys(self):
        tmp, root, manifest = self._project("integration_branch='main'\n")
        self.addCleanup(tmp.cleanup)
        before = manifest.read_bytes()
        report, code = self.mod.apply_project(root, "worktree-flow", {"integration_branch": "main"})
        self.assertEqual(code, 0)
        self.assertEqual(report["status"], "no-op")
        self.assertEqual(report["applied"]["changed"], [])
        self.assertEqual(manifest.read_bytes(), before)

    def test_recipe_subcommand_help_lists_configure(self):
        import subprocess

        proc = subprocess.run(
            ["bash", str(ROOT / "lib" / "recipe.sh"), "--help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("configure <id>", proc.stdout)


    def test_trello_inspect_surfaces_init_and_secret_env_names(self):
        tmp, root, manifest = self._project()
        self.addCleanup(tmp.cleanup)
        manifest.write_text(
            "[project]\nname='fixture'\n\n"
            "[recipes.trello-mcp-workflow]\nenabled=true\nversion='1.3.0'\n\n"
            "[recipes.trello-mcp-workflow.config]\n"
        )
        doc = self.mod.inspect_project(root, "trello-mcp-workflow")
        self.assertTrue(doc["grounding"]["init"]["present"])
        self.assertEqual(doc["grounding"]["init"]["needs_mcp"], ["trello"])
        self.assertIn("TRELLO_API_KEY", doc["grounding"]["mcp"]["env_vars"])
        self.assertNotIn("$TRELLO_API_KEY", json.dumps(doc))

if __name__ == "__main__":
    unittest.main()
