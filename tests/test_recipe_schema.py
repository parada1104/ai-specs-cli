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

    def test_config_field_without_validation_parses(self):
        """Config field without 'validation' sub-table parses successfully."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "no-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "no-val"\n'
                'name = "No Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.my_field]\n'
                'required = true\n'
                'type = "string"\n'
                'default = "hello"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            field = data.config_schema.fields["my_field"]
            self.assertEqual(field.required, True)
            self.assertEqual(field.type, "string")
            self.assertEqual(field.default, "hello")
            self.assertEqual(field.validation, {})

    def test_config_field_with_validation_parses(self):
        """Config field with valid 'validation.regex' parses successfully."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "with-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "with-val"\n'
                'name = "With Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                '\n'
                '[config.board_id.validation]\n'
                'regex = "^[0-9a-fA-F]{24}$"\n'
            )
            data = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            field = data.config_schema.fields["board_id"]
            self.assertEqual(field.required, True)
            self.assertEqual(field.validation, {"regex": "^[0-9a-fA-F]{24}$"})

    def test_config_field_with_unknown_validation_key_fails(self):
        """Config field with unknown key inside 'validation' table raises error."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-val"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bad-val"\n'
                'name = "Bad Val"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                '\n'
                '[config.board_id.validation]\n'
                'regex = "^[0-9a-fA-F]{24}$"\n'
                'min_length = 5\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("unknown key", str(ctx.exception))
            self.assertIn("min_length", str(ctx.exception))

    def test_config_field_with_validation_non_table_rejected(self):
        """Config field with 'validation' set to a non-table value raises error."""
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-val-type"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\n'
                'id = "bad-val-type"\n'
                'name = "Bad Val Type"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                '\n'
                '[config.board_id]\n'
                'required = true\n'
                'type = "string"\n'
                'validation = "not_a_table"\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
            self.assertIn("expected table", str(ctx.exception))
            self.assertIn("validation", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
