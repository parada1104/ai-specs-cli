"""Black-box tests for recipe configuration and wizard dispatch."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke, snapshot, temp_project


class ConfigWizardTests(unittest.TestCase):
    def _project(self, recipe_id: str = "worktree-flow", *, enabled: bool = True) -> tuple[Path, Path]:
        td, project = temp_project(name="fixture")
        self.addCleanup(td.cleanup)
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = []\n\n"
            f"[recipes.{recipe_id}]\nenabled = {flag}\nversion = '1.0'\n"
        )
        home = isolated_home(Path(td.name) / "cli-home-source")
        return project, home

    def _inspect(self, project: Path, home: Path, recipe_id: str = "worktree-flow") -> dict:
        result = invoke(project, "recipe", "configure", recipe_id, "--inspect", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def test_required_validator_rejects_blank_accepts_value(self):
        project, home = self._project()
        data = self._inspect(project, home)
        fields = {field["key"]: field for field in data["schema"]["fields"]}
        self.assertIn("integration_branch", fields)
        self.assertFalse(fields["integration_branch"]["required"])
        self.assertEqual(data["recipe"]["id"], "worktree-flow")

    def test_regex_validator(self):
        project, home = self._project("trello-mcp-workflow")
        data = self._inspect(project, home, "trello-mcp-workflow")
        board = next(field for field in data["schema"]["fields"] if field["key"] == "board_id")
        self.assertTrue(board["required"])
        self.assertIn("24 hex", board["help_text"])
        self.assertIn("board_id", [field["key"] for field in data["schema"]["fields"]])

    def test_enum_field_uses_select(self):
        project, home = self._project()
        data = self._inspect(project, home)
        gate = next(field for field in data["schema"]["fields"] if field["key"] == "gate_mode")
        self.assertEqual(gate["enum"], ["always", "ask", "off"])
        self.assertEqual(gate["default"], "always")
        self.assertIn("help_text", gate)

    def test_bool_field_uses_confirm(self):
        project, home = self._project()
        data = self._inspect(project, home)
        field = next(field for field in data["schema"]["fields"] if field["key"] == "auto_remove_merged")
        self.assertEqual(field["type"], "bool")
        self.assertIs(field["default"], True)
        self.assertEqual(data["current_config"], {})

    def test_default_prefill_kept_when_blank(self):
        project, home = self._project()
        data = self._inspect(project, home)
        field = next(field for field in data["schema"]["fields"] if field["key"] == "integration_branch")
        self.assertEqual(field["default"], "main")
        self.assertEqual(data["current_config"], {})
        self.assertFalse((project / "ai-specs.env").exists())

    def test_existing_value_prefilled_as_default(self):
        project, home = self._project()
        manifest = project / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n[recipes.worktree-flow.config]\nintegration_branch = 'develop'\n")
        data = self._inspect(project, home)
        self.assertEqual(data["current_config"]["integration_branch"], "develop")
        self.assertIn("integration_branch", data["current_config"])
        self.assertTrue(data["recipe"]["enabled"])

    def test_extra_fields_never_prompted(self):
        project, home = self._project("trello-mcp-workflow")
        data = self._inspect(project, home, "trello-mcp-workflow")
        keys = {field["key"] for field in data["schema"]["fields"]}
        self.assertNotIn("board_isolation", keys)
        self.assertIn("board_id", keys)
        self.assertIn("gate_mode", keys)

    def test_dep_gate_abort_skips_recipe(self):
        project, home = self._project("trello-mcp-workflow")
        before = snapshot(project)
        result = invoke(project, "configure-recipes", cli_home=home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertEqual(snapshot(project), before)

    def test_dep_gate_proceed_continues(self):
        project, home = self._project()
        result = invoke(project, "recipe", "configure", "worktree-flow", "--set", "integration_branch='develop'", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "ok")
        self.assertEqual(report["applied"]["changed"][0]["key"], "integration_branch")

    def test_dep_gate_offers_install_on_tty(self):
        project, home = self._project()
        result = invoke(project, "recipe", "configure", "worktree-flow", "--set", "integration_branch='develop'", "--dry-run", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "ok")
        self.assertTrue(report["dry_run"])
        self.assertFalse(report["sync"]["ran"])

    def test_configure_selected_writes_each(self):
        project, home = self._project()
        result = invoke(project, "recipe", "configure", "worktree-flow", "--set", "integration_branch='develop'", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("integration_branch", result.stdout)
        self.assertIn("develop", (project / "ai-specs" / "ai-specs.toml").read_text())

    def test_main_offers_envrc_generation(self):
        project, home = self._project("trello-mcp-workflow")
        result = invoke(project, "configure-recipes", cli_home=home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertFalse((project / "ai-specs.env").exists())

    def test_main_skips_envrc_when_no_mcp_recipes(self):
        project, home = self._project()
        result = invoke(project, "configure-recipes", cli_home=home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertFalse((project / "ai-specs.env").exists())

    def test_offer_envrc_soft_fails_on_prompt_error(self):
        project, home = self._project("trello-mcp-workflow")
        result = invoke(project, "configure-recipes", cli_home=home)
        self.assertEqual(result.returncode, 3)
        self.assertNotIn("Traceback", result.stdout + result.stderr)
        self.assertFalse((project / ".envrc").exists())

    def test_boolean_type_alias_uses_confirm(self):
        project, home = self._project()
        result = invoke(project, "recipe", "configure", "worktree-flow", "--set", "auto_remove_merged=false", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "ok")
        self.assertFalse(report["applied"]["changed"][0]["to"])


class ConfigureRecipesDispatchTests(unittest.TestCase):
    def test_configure_recipes_dispatch(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result = invoke(root, "configure-recipes")
            self.assertEqual(result.returncode, 1)
            self.assertIn("Proyecto no inicializado", result.stderr)
            self.assertFalse((root / "ai-specs").exists())

    def test_recipe_config_sh_help(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result = invoke(root, "configure-recipes", "--help")
            self.assertEqual(result.returncode, 0)
            self.assertIn("configure-recipes", result.stdout)



if __name__ == "__main__":
    unittest.main()
