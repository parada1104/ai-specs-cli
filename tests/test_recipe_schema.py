"""Tests for recipe_schema.py dataclasses and validation."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeSchemaTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.read_mod = load_module(RECIPE_READ_PATH, "recipe_read_internal")

    def test_recipe_without_sdd_section_parses(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "no-sdd"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "no-sdd"\n'
                'name = "No Sdd"\n'
                'description = "D"\n'
                'version = "1.0"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(data.sdd.threshold, "")

    def test_recipe_with_valid_threshold_parses(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "valid-th"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "valid-th"\n'
                'name = "Valid Threshold"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[sdd]\n'
                'threshold = "behavior_change"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertEqual(data.sdd.threshold, "behavior_change")

    def test_recipe_with_invalid_threshold_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "invalid-th"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "invalid-th"\n'
                'name = "Invalid Threshold"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[sdd]\n'
                'threshold = "major_change"\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("invalid value 'major_change'", str(ctx.exception))

    def test_recipe_to_dict_includes_sdd(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "dict-sdd"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "dict-sdd"\n'
                'name = "Dict Sdd"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[sdd]\n'
                'threshold = "local_fix"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            d = self.read_mod.recipe_to_dict(data)
            self.assertIn("sdd", d)
            self.assertEqual(d["sdd"], {"threshold": "local_fix"})

    def test_recipe_read_defensively_rejects_invalid_threshold(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "defensive-th"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "defensive-th"\n'
                'name = "Defensive"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[sdd]\n'
                'threshold = "unknown_level"\n'
            )
            catalog = Path(tmp)
            with self.assertRaises(self.read_mod.RecipeValidationError) as ctx:
                self.read_mod.read_recipe(catalog, "defensive-th")
            self.assertIn("invalid value 'unknown_level'", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
