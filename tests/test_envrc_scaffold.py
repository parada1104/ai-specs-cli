"""Black-box coverage for configure-recipes env scaffolding."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke, snapshot, temp_project


class EnvrcScaffoldTests(unittest.TestCase):
    def _project_with_recipe(self, *, recipe_id: str, enabled: bool = True) -> Path:
        td, project = temp_project(name="p")
        self.addCleanup(td.cleanup)
        manifest = project / "ai-specs" / "ai-specs.toml"
        flag = "true" if enabled else "false"
        manifest.write_text(
            "[project]\nname = 'p'\n\n[agents]\nenabled = []\n\n"
            f"[recipes.{recipe_id}]\nenabled = {flag}\nversion = '1.0'\n"
        )
        return project

    def _home(self, project: Path) -> Path:
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _run(self, project: Path, home: Path):
        return invoke(project, "configure-recipes", cli_home=home)

    def test_collect_from_mcp_env(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        before = snapshot(project)
        home = self._home(project)
        listed = invoke(project, "recipe", "configure", "trello-mcp-workflow", "--inspect", "--json", cli_home=home)
        self.assertEqual(listed.returncode, 0)
        self.assertIn("TRELLO_API_KEY", listed.stdout)
        self.assertIn("TRELLO_TOKEN", listed.stdout)
        self.assertIn("env_vars", listed.stdout)
        self.assertEqual(snapshot(project), before)

    def test_non_reference_env_ignored(self):
        project = self._project_with_recipe(recipe_id="session-context")
        home = self._home(project)
        listed = invoke(project, "recipe", "configure", "session-context", "--inspect", "--json", cli_home=home)
        self.assertEqual(listed.returncode, 0)
        self.assertNotIn("TRELLO_API_KEY", listed.stdout)
        self.assertNotIn("MODE", listed.stdout)
        self.assertIn("env_vars", listed.stdout)

    def test_generate_writes_export_lines(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        home = self._home(project)
        before = snapshot(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertEqual(snapshot(project), before)
        self.assertNotIn("export ", result.stdout + result.stderr)

    def test_envrc_never_written(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        home = self._home(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertFalse((project / ".envrc").exists())
        self.assertFalse((project / "ai-specs.env.example").exists())
        self.assertFalse((project / "ai-specs" / ".envrc.example").exists())

    def test_existing_example_backed_up(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        example = project / "ai-specs.env.example"
        example.write_text("OLD CONTENT\n")
        home = self._home(project)
        before = snapshot(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertEqual(example.read_text(), "OLD CONTENT\n")
        self.assertFalse((project / "ai-specs.env.example.bak").exists())
        self.assertEqual(snapshot(project), before)

    def test_no_enabled_mcp_recipes_writes_empty_template(self):
        project = self._project_with_recipe(recipe_id="session-context")
        home = self._home(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertFalse((project / "ai-specs.env.example").exists())
        self.assertFalse((project / ".envrc").exists())

    def test_disabled_recipe_excluded(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow", enabled=False)
        home = self._home(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertIn("requires an interactive TTY", result.stderr)
        self.assertNotIn("No enabled recipes found", result.stdout)
        self.assertFalse((project / "ai-specs.env.example").exists())
        self.assertFalse((project / ".envrc").exists())

    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        home = self._home(project)
        result = self._run(project, home)
        self.assertEqual(result.returncode, 3)
        self.assertNotIn("password=", result.stdout + result.stderr)
        self.assertFalse((project / "ai-specs.env").exists())

    def test_generate_includes_env_var_help_comments(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        home = self._home(project)
        inspected = invoke(project, "recipe", "init", "trello-mcp-workflow", cli_home=home)
        self.assertEqual(inspected.returncode, 0)
        self.assertIn("TRELLO_API_KEY", inspected.stdout)
        self.assertIn("TRELLO_TOKEN", inspected.stdout)
        self.assertFalse((project / "ai-specs.env.example").exists())

    def test_env_var_help_map_has_known_vars(self):
        project = self._project_with_recipe(recipe_id="trello-mcp-workflow")
        home = self._home(project)
        result = invoke(project, "recipe", "configure", "trello-mcp-workflow", "--inspect", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("TRELLO_API_KEY", result.stdout)
        self.assertIn("TRELLO_TOKEN", result.stdout)
        vault = invoke(project, "recipe", "configure", "vault-canonical-store", "--inspect", "--json", cli_home=home)
        self.assertIn("CANONICAL_VAULT_PATH", vault.stdout)

    def test_catalog_config_fields_have_help_text(self):
        project = self._project_with_recipe(recipe_id="worktree-flow")
        home = self._home(project)
        result = invoke(project, "recipe", "configure", "worktree-flow", "--inspect", "--json", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("help_text", result.stdout)
        self.assertIn("integration_branch", result.stdout)
        self.assertIn("gate_mode", result.stdout)



if __name__ == "__main__":
    unittest.main()
