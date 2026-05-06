"""Integration tests for agents-md-render.py config-field rendering.

Verifies that the renderer emits config schema tables for enabled recipes
when catalog recipe.toml files declare config fields.
"""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
AGENTS_MD_RENDER_PATH = ROOT / "lib" / "_internal" / "agents-md-render.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class AgentsMdRenderConfigTableTests(unittest.TestCase):
    """Unit tests for the render_recipe_config_table helper."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_MD_RENDER_PATH, "agents_md_render_internal")

    def test_render_table_with_fields(self):
        fields = {
            "board_id": {"required": True, "type": "string"},
            "timeout": {"required": False, "type": "integer", "default": 30},
        }
        table = self.mod.render_recipe_config_table(fields)
        self.assertIn("board_id", table)
        self.assertIn("timeout", table)
        self.assertIn("yes", table)
        self.assertIn("no", table)
        self.assertIn("integer", table)
        self.assertIn("30", table)
        self.assertIn("| Field |", table)

    def test_render_table_empty(self):
        self.assertEqual(self.mod.render_recipe_config_table({}), "")

    def test_render_table_none(self):
        self.assertEqual(self.mod.render_recipe_config_table(None), "")

    def test_render_table_with_validation(self):
        """validation dict is carried through as an opaque column value."""
        fields = {
            "board_id": {
                "required": True,
                "type": "string",
                "validation": {"regex": "^[0-9a-fA-F]{24}$"},
            },
        }
        table = self.mod.render_recipe_config_table(fields)
        self.assertIn("board_id", table)
        self.assertIn("yes", table)

    def test_render_table_bool_default(self):
        fields = {
            "flag": {"required": False, "type": "boolean", "default": True},
        }
        table = self.mod.render_recipe_config_table(fields)
        self.assertIn("flag", table)
        self.assertIn("true", table)

    def test_render_table_none_default_shows_dash(self):
        fields = {
            "key": {"required": True, "type": "string"},
        }
        table = self.mod.render_recipe_config_table(fields)
        self.assertIn("key", table)
        # No default → em-dash
        self.assertIn("—", table)

    def test_render_table_empty_type_is_empty_string(self):
        fields = {
            "key": {"required": True},
        }
        table = self.mod.render_recipe_config_table(fields)
        self.assertIn("key", table)
        self.assertIn("yes", table)


class AgentsMdRenderRecipesSectionTests(unittest.TestCase):
    """Tests for render_recipes_section with recipe schemas."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_MD_RENDER_PATH, "agents_md_render_recipes")

    def test_section_with_config_table(self):
        recipes = {
            "trello-mcp-workflow": {"enabled": True, "version": "1.0.0"},
        }
        schemas = {
            "trello-mcp-workflow": {
                "board_id": {"required": True, "type": "string"},
            },
        }
        result = self.mod.render_recipes_section(recipes, schemas)
        self.assertIn("trello-mcp-workflow", result)
        self.assertIn("board_id", result)
        self.assertIn("yes", result)
        self.assertIn("| Field |", result)

    def test_section_mixed_recipes(self):
        """One recipe with schema, another without."""
        recipes = {
            "with-config": {"enabled": True, "version": "1.0.0"},
            "no-config": {"enabled": True, "version": "2.0.0"},
        }
        schemas = {
            "with-config": {
                "key": {"required": True, "type": "string"},
            },
        }
        result = self.mod.render_recipes_section(recipes, schemas)
        self.assertIn("with-config", result)
        self.assertIn("no-config", result)
        self.assertIn("| Field |", result)
        # The line for no-config should NOT be immediately followed by a table
        lines = result.splitlines()
        idx = next(i for i, l in enumerate(lines) if "no-config" in l)
        # After the no-config line, the next non-empty line should NOT be a table header
        next_non_empty = next((l.strip() for l in lines[idx + 1:] if l.strip()), "")
        self.assertNotIn("| Field |", next_non_empty,
                         "no-config recipe should not show a config table")

    def test_section_empty_recipes(self):
        self.assertEqual(self.mod.render_recipes_section({}), "")

    def test_section_no_schemas(self):
        """Backward-compatible call without schemas."""
        recipes = {"test": {"enabled": True, "version": "1.0.0"}}
        result = self.mod.render_recipes_section(recipes)
        self.assertIn("test", result)
        self.assertNotIn("| Field |", result)

    def test_section_schemas_none(self):
        """Explicit None schemas."""
        recipes = {"test": {"enabled": True, "version": "1.0.0"}}
        result = self.mod.render_recipes_section(recipes, None)
        self.assertIn("test", result)
        self.assertNotIn("| Field |", result)

    def test_section_disabled_recipe_skipped(self):
        recipes = {
            "disabled": {"enabled": False, "version": "1.0.0"},
        }
        schemas = {
            "disabled": {"key": {"required": True}},
        }
        result = self.mod.render_recipes_section(recipes, schemas)
        self.assertNotIn("disabled", result)


class AgentsMdRenderCatalogLoadingTests(unittest.TestCase):
    """Integration tests: catalog recipe config loaded and rendered."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_MD_RENDER_PATH, "agents_md_render_catalog")

    def test_load_recipe_config_from_catalog(self):
        """Verify trello-mcp-workflow recipe config can be loaded."""
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        schema = self.mod._load_recipe_config_schema(catalog_path, "trello-mcp-workflow")
        self.assertIsNotNone(schema)
        self.assertIn("board_id", schema)
        self.assertIn("default_list", schema)
        self.assertIn("epic_list", schema)
        self.assertTrue(schema["board_id"]["required"])
        self.assertEqual(schema["board_id"]["type"], "string")

    def test_load_nonexistent_recipe_returns_none(self):
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        schema = self.mod._load_recipe_config_schema(catalog_path, "nonexistent-recipe")
        self.assertIsNone(schema)

    def test_get_ai_specs_home_detects_root(self):
        home = self.mod._get_ai_specs_home()
        # This test only runs from within the ai-specs-cli repo
        self.assertIsNotNone(home)
        self.assertTrue((home / "catalog" / "recipes").is_dir())

    def test_catalog_recipe_renders_config_in_section(self):
        """Full integration: render_recipes_section with real catalog data."""
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        recipes = {
            "trello-mcp-workflow": {"enabled": True, "version": "1.0.0"},
        }
        schemas = {}
        for rid, cfg in recipes.items():
            if cfg.get("enabled"):
                s = self.mod._load_recipe_config_schema(catalog_path, rid)
                if s:
                    schemas[rid] = s
        self.assertIn("trello-mcp-workflow", schemas)
        result = self.mod.render_recipes_section(recipes, schemas)
        self.assertIn("trello-mcp-workflow", result)
        self.assertIn("board_id", result)
        self.assertIn("default_list", result)
        self.assertIn("epic_list", result)

    def test_renderer_main_integration(self):
        """Simulate a full render run: create a temp manifest and invoke main()."""
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\n"
                "name = 'test-integration'\n"
                "\n"
                "[agents]\n"
                "enabled = ['claude']\n"
                "\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
            )
            output = project_root / "AGENTS.md"
            ret = self.mod.main()
            # main() reads sys.argv — we need to patch, but instead test
            # end-to-end via subprocess
            self.skipTest("main() uses sys.argv; tested via end-to-end below")

    def test_end_to_end_render_script(self):
        """Run agents-md-render.py as a subprocess with a temp project."""
        import subprocess
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\n"
                "name = 'test-e2e'\n"
                "\n"
                "[agents]\n"
                "enabled = ['claude']\n"
                "\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = true\n"
                "version = '1.0.0'\n"
            )
            output = project_root / "AGENTS.md"
            result = subprocess.run(
                [sys.executable, str(AGENTS_MD_RENDER_PATH), str(project_root), str(output)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            self.assertTrue(output.is_file())
            content = output.read_text()
            # Verify config fields appear
            self.assertIn("trello-mcp-workflow", content)
            self.assertIn("board_id", content)
            self.assertIn("default_list", content)
            self.assertIn("epic_list", content)
            self.assertIn("| Field |", content)
            self.assertIn("| `board_id` | yes | string |", content)
            self.assertIn("| `default_list` | no | string | In Progress", content)
            self.assertIn("| `epic_list` | no | string | Epic", content)

    def test_end_to_render_no_recipes_has_no_config_table(self):
        """Render without any recipes should not contain config tables."""
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\n"
                "name = 'no-recipes'\n"
            )
            output = project_root / "AGENTS.md"
            result = subprocess.run(
                [sys.executable, str(AGENTS_MD_RENDER_PATH), str(project_root), str(output)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            content = output.read_text()
            self.assertNotIn("| Field |", content)
            self.assertNotIn("Active Recipes", content)

    def test_end_to_end_disabled_recipe_no_config(self):
        """A disabled recipe should not have its config rendered."""
        import subprocess
        catalog_path = CATALOG
        if not catalog_path.is_dir():
            self.skipTest(f"catalog not found at {catalog_path}")
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\n"
                "name = 'disabled-recipe'\n"
                "\n"
                "[agents]\n"
                "enabled = ['claude']\n"
                "\n"
                "[recipes.trello-mcp-workflow]\n"
                "enabled = false\n"
                "version = '1.0.0'\n"
            )
            output = project_root / "AGENTS.md"
            result = subprocess.run(
                [sys.executable, str(AGENTS_MD_RENDER_PATH), str(project_root), str(output)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, msg=result.stderr)
            content = output.read_text()
            self.assertNotIn("trello-mcp-workflow", content)
            self.assertNotIn("board_id", content)
            self.assertNotIn("| Field |", content)


if __name__ == "__main__":
    unittest.main()
