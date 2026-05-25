"""Tests for [[provides.mcp]] `mode` enum validation in recipe_schema.py.

Group 1.1 of mcp-compartido-por-proyecto: the schema MUST accept
`mode = "shared"` and `mode = "stdio"`, treat absence as stdio, and
reject any other value with an explicit error listing the valid options.
"""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_RECIPE_HEADER = (
    '[recipe]\n'
    'id = "fixture"\n'
    'name = "Fixture"\n'
    'description = "D"\n'
    'version = "1.0"\n'
    '\n'
)


class RecipeSchemaModeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_mode_internal")

    def _write_recipe(self, mcp_block: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        recipe_dir = Path(tmp.name) / "fixture"
        recipe_dir.mkdir()
        (recipe_dir / "recipe.toml").write_text(_RECIPE_HEADER + mcp_block)
        return recipe_dir / "recipe.toml"

    def test_provides_mcp_with_mode_shared_validates(self):
        """Recipe with `mode = "shared"` in [[provides.mcp]] passes validation."""
        path = self._write_recipe(
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'mode = "shared"\n'
            'command = "uvx"\n'
        )
        recipe = self.schema.load_recipe_toml(path)
        self.assertEqual(len(recipe.mcp), 1)
        self.assertEqual(recipe.mcp[0].id, "trello")
        self.assertEqual(recipe.mcp[0].config.get("mode"), "shared")

    def test_provides_mcp_with_mode_stdio_validates(self):
        """Recipe with explicit `mode = "stdio"` passes validation."""
        path = self._write_recipe(
            '[[provides.mcp]]\n'
            'id = "github"\n'
            'mode = "stdio"\n'
            'command = "npx"\n'
        )
        recipe = self.schema.load_recipe_toml(path)
        self.assertEqual(recipe.mcp[0].config.get("mode"), "stdio")

    def test_provides_mcp_without_mode_validates(self):
        """Recipe without `mode` (legacy shape) passes validation unchanged."""
        path = self._write_recipe(
            '[[provides.mcp]]\n'
            'id = "legacy"\n'
            'command = "node"\n'
        )
        recipe = self.schema.load_recipe_toml(path)
        self.assertEqual(recipe.mcp[0].id, "legacy")
        self.assertNotIn("mode", recipe.mcp[0].config)

    def test_provides_mcp_with_unknown_mode_rejected(self):
        """Recipe with `mode = "proxy"` (out-of-enum) raises with valid values listed."""
        path = self._write_recipe(
            '[[provides.mcp]]\n'
            'id = "broken"\n'
            'mode = "proxy"\n'
            'command = "uvx"\n'
        )
        with self.assertRaises(self.schema.RecipeValidationError) as ctx:
            self.schema.load_recipe_toml(path)
        msg = str(ctx.exception)
        self.assertIn("mode", msg)
        self.assertIn("proxy", msg)
        self.assertIn("shared", msg)
        self.assertIn("stdio", msg)


if __name__ == "__main__":
    unittest.main()
