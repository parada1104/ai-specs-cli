"""Guard docs/recipes-catalog.md against drift from catalog recipe.toml manifests."""

from __future__ import annotations

import re
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG_DOC = ROOT / "docs" / "recipes-catalog.md"
RECIPES_DIR = ROOT / "catalog" / "recipes"

# Recipe ids with bundled MCP presets — must appear in the "Installs MCP" column.
MCP_RECIPES: dict[str, str] = {
    "trello-mcp-workflow": "trello",
    "vault-canonical-store": "vault-canonical",
}

# User-facing config keys that must be documented in the per-recipe section.
CONFIG_KEYS_IN_CATALOG: dict[str, list[str]] = {
    "worktree-flow": ["WORKTREE_GATE_PROTECTED"],
    "trello-mcp-workflow": [
        "board_id",
        "forbidden_tools",
        "card_validation_required",
    ],
}


def _catalog_recipe_dirs() -> list[Path]:
    return sorted(
        p
        for p in RECIPES_DIR.iterdir()
        if p.is_dir() and not p.name.startswith("test-")
    )


class RecipesCatalogDriftTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()

    def test_at_a_glance_table_has_installs_mcp_column(self):
        self.assertIn("| Installs MCP |", self.catalog)

    def test_mcp_recipes_documented_in_catalog(self):
        for recipe_id, mcp_id in MCP_RECIPES.items():
            with self.subTest(recipe=recipe_id, mcp=mcp_id):
                self.assertIn(f"| `{mcp_id}` |", self.catalog)
                recipe_toml = RECIPES_DIR / recipe_id / "recipe.toml"
                data = tomllib.loads(recipe_toml.read_text())
                provides_mcp = (data.get("provides") or {}).get("mcp") or []
                if isinstance(provides_mcp, dict):
                    provides_mcp = [provides_mcp]
                mcp_ids = {entry["id"] for entry in provides_mcp}
                self.assertIn(mcp_id, mcp_ids)

    def test_documented_config_keys_exist_in_recipe_toml(self):
        for recipe_id, keys in CONFIG_KEYS_IN_CATALOG.items():
            recipe_toml = RECIPES_DIR / recipe_id / "recipe.toml"
            text = recipe_toml.read_text()
            section = self._recipe_section(recipe_id)
            for key in keys:
                with self.subTest(recipe=recipe_id, key=key):
                    self.assertIn(key, text)
                    self.assertIn(key, section)

    def test_worktree_gate_runtime_hook_documented(self):
        section = self._recipe_section("worktree-flow")
        self.assertIn("worktree-gate", section)
        self.assertIn("runtime-hooks.md", section)

    def test_vault_recipe_no_longer_claims_mcp_is_external_only(self):
        section = self._recipe_section("vault-canonical-store")
        self.assertNotIn("does **not** declare the MCP server", section)
        self.assertIn("vault-canonical", section)

    def _recipe_section(self, recipe_id: str) -> str:
        pattern = rf"## {re.escape(recipe_id)}\n(.*?)(?=\n## |\Z)"
        match = re.search(pattern, self.catalog, re.DOTALL)
        self.assertIsNotNone(match, f"missing ## {recipe_id} section")
        return match.group(1)


if __name__ == "__main__":
    unittest.main()
