"""Tests for the vault-canonical-store catalog recipe."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_DIR = ROOT / "catalog" / "recipes" / "vault-canonical-store"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class VaultCanonicalStoreRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.mat = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal_vcs"
        )

    def test_recipe_validates_and_provides_canonical_store(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "vault-canonical-store")
        self.assertIn("canonical-store", {c.id for c in recipe.capabilities})
        self.assertEqual({s.id for s in recipe.skills}, {"vault-context"})

    def test_materializes_vault_context_skill(self):
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            version = tomllib.load(fh)["recipe"]["version"]
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp) / "project"
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.vault-canonical-store]\nenabled = true\nversion = \"{version}\"\n"
            )
            self.assertEqual(self.mat.materialize_recipes(project_root, ROOT), 0)
            skill = (
                project_root / "ai-specs" / ".recipe" / "vault-canonical-store"
                / "skills" / "vault-context" / "SKILL.md"
            )
            self.assertTrue(skill.is_file())


if __name__ == "__main__":
    unittest.main()
