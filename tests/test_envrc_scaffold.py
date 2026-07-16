"""Tests for envrc-scaffold.py."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ENVRC_PATH = ROOT / "lib" / "_internal" / "envrc-scaffold.py"
VENDOR = ROOT / "lib" / "_vendor"


def _ensure_vendor_path() -> None:
    if VENDOR.is_dir() and str(VENDOR) not in sys.path:
        sys.path.insert(0, str(VENDOR))


def load_module(path: Path, name: str):
    _ensure_vendor_path()
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class EnvrcScaffoldTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _ensure_vendor_path()
        cls.mod = load_module(ENVRC_PATH, "envrc_scaffold_internal")

    def _project_with_recipe(
        self,
        root: Path,
        *,
        recipe_id: str,
        recipe_toml: str,
        enabled: bool = True,
    ) -> Path:
        catalog = root / "catalog" / "recipes" / recipe_id
        catalog.mkdir(parents=True)
        (catalog / "recipe.toml").write_text(recipe_toml, encoding="utf-8")
        project = root / "project"
        (project / "ai-specs").mkdir(parents=True)
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "p"\n\n'
            f"[recipes.{recipe_id}]\nenabled = {flag}\nversion = \"1.0\"\n",
            encoding="utf-8",
        )
        return project

    def test_collect_from_mcp_env(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                vars_map = self.mod.collect_env_vars(project)
            self.assertIn("TRELLO_API_KEY", vars_map)
            self.assertIn("TRELLO_TOKEN", vars_map)
            self.assertIn("trello-mcp-workflow", vars_map["TRELLO_API_KEY"])

    def test_non_reference_env_ignored(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="literal-env",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "literal-env"\n'
                    'name = "L"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "svc"\n'
                    'command = "echo"\n'
                    'env = { MODE = "production", TOKEN = "$TOKEN" }\n'
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                vars_map = self.mod.collect_env_vars(project)
            self.assertNotIn("MODE", vars_map)
            self.assertIn("TOKEN", vars_map)

    def test_generate_writes_export_lines(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                path = self.mod.generate_envrc_example(project)
            text = path.read_text(encoding="utf-8")
            self.assertIn('export TRELLO_API_KEY=""', text)
            self.assertIn('export TRELLO_TOKEN=""', text)
            self.assertTrue(text.index("TRELLO_API_KEY") < text.index("TRELLO_TOKEN"))
            self.assertIn("required by trello", text)

    def test_envrc_never_written(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n"
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                self.mod.generate_envrc_example(project)
            self.assertFalse((project / "ai-specs" / ".envrc").exists())
            self.assertTrue((project / "ai-specs" / ".envrc.example").is_file())

    def test_existing_example_backed_up(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n"
                ),
            )
            example = project / "ai-specs" / ".envrc.example"
            example.write_text("OLD CONTENT\n", encoding="utf-8")
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                self.mod.generate_envrc_example(project)
            bak = project / "ai-specs" / ".envrc.example.bak"
            self.assertTrue(bak.is_file())
            self.assertEqual(bak.read_text(encoding="utf-8"), "OLD CONTENT\n")
            self.assertIn("TRELLO_API_KEY", example.read_text(encoding="utf-8"))

    def test_no_enabled_mcp_recipes_writes_empty_template(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="session-context",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "session-context"\n'
                    'name = "Session"\n'
                    'description = "D"\n'
                    'version = "1.0"\n'
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                path = self.mod.generate_envrc_example(project)
            text = path.read_text(encoding="utf-8")
            self.assertIn("no env vars required", text)
            self.assertFalse((project / "ai-specs" / ".envrc").exists())

    def test_disabled_recipe_excluded(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                enabled=False,
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\" }\n"
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                vars_map = self.mod.collect_env_vars(project)
            self.assertEqual(vars_map, {})

    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        """Regression: password= kwarg crashes questionary 2.x; use password()."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", MODE = \"$MODE\" }\n"
                ),
            )
            import os
            from unittest.mock import MagicMock, patch

            password = MagicMock()
            password.return_value.ask.return_value = "secret-key"
            text = MagicMock()
            text.return_value.ask.return_value = "plain"
            confirm = MagicMock()
            confirm.return_value.ask.return_value = True
            q = MagicMock(password=password, text=text, confirm=confirm)

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.dict(
                "sys.modules", {"questionary": q}
            ):
                result = self.mod.prompt_env_vars(project)

            self.assertEqual(result["TRELLO_API_KEY"], "secret-key")
            self.assertEqual(result["MODE"], "plain")
            password.assert_called()
            # Secrets must not go through text(..., password=...)
            for call in text.call_args_list:
                self.assertNotIn("password", call.kwargs)

    def test_generate_includes_env_var_help_comments(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root,
                recipe_id="trello-mcp-workflow",
                recipe_toml=(
                    "[recipe]\n"
                    'id = "trello-mcp-workflow"\n'
                    'name = "Trello"\n'
                    'description = "D"\n'
                    'version = "1.0"\n\n'
                    "[[provides.mcp]]\n"
                    'id = "trello"\n'
                    'command = "npx"\n'
                    "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
                ),
            )
            import os
            from unittest.mock import patch

            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                path = self.mod.generate_envrc_example(project)
            text = path.read_text(encoding="utf-8")
            self.assertIn("trello.com/power-ups/admin", text)
            self.assertIn("TRELLO_API_KEY", text)
            self.assertIn("TRELLO_TOKEN", text)

    def test_env_var_help_map_has_known_vars(self):
        self.assertIn("TRELLO_API_KEY", self.mod.ENV_VAR_HELP)
        self.assertIn("TRELLO_TOKEN", self.mod.ENV_VAR_HELP)
        self.assertIn("CANONICAL_VAULT_PATH", self.mod.ENV_VAR_HELP)
        self.assertIn("https://trello.com/power-ups/admin", self.mod.ENV_VAR_HELP["TRELLO_API_KEY"])

    def test_catalog_config_fields_have_help_text(self):
        """Key catalog ConfigFields must ship wizard help_text."""
        schema = load_module(
            ROOT / "lib" / "_internal" / "recipe_schema.py",
            "recipe_schema_help_check",
        )
        catalog = ROOT / "catalog" / "recipes"
        required = {
            "trello-mcp-workflow": ["board_id", "default_list", "epic_list"],
            "worktree-flow": ["integration_branch", "worktrees_dir", "gate_mode"],
            "git-pr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "gitlab-mr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "bitbucket-pr-flow": ["base_branch", "expected_owner", "auto_switch_account"],
            "vault-canonical-store": ["vault_scope", "decisions_folder", "sessions_folder"],
            "tdd-flow": ["test_command"],
        }
        for recipe_id, keys in required.items():
            recipe = schema.load_recipe_toml(catalog / recipe_id / "recipe.toml")
            for key in keys:
                field = recipe.config_schema.fields[key]
                self.assertTrue(
                    field.help_text and field.help_text.strip(),
                    f"{recipe_id}.{key} missing help_text",
                )


if __name__ == "__main__":
    unittest.main()
