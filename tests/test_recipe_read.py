import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeReadTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_READ_PATH, "recipe_read_internal")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")

    def test_reads_valid_recipe(self):
        data = self.mod.read_recipe(CATALOG, "test-fixture")
        self.assertEqual(data.id, "test-fixture")
        self.assertEqual(data.name, "Test Fixture Recipe")
        self.assertEqual(data.version, "1.0.0")
        self.assertEqual(len(data.skills), 1)
        self.assertEqual(data.skills[0].id, "test-skill")
        self.assertEqual(data.skills[0].source, "bundled")

    def test_fails_on_missing_recipe_dir(self):
        with self.assertRaises(self.mod.RecipeValidationError) as ctx:
            self.mod.read_recipe(CATALOG, "nonexistent")
        self.assertIn("recipe directory not found", str(ctx.exception))

    def test_fails_on_missing_required_field(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-recipe"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nname = "Bad"\ndescription = "Missing id and version"\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-recipe")
            self.assertIn("missing or invalid required field", str(ctx.exception))

    def test_cli_outputs_json(self):
        import subprocess
        proc = subprocess.run(
            ["python3", str(RECIPE_READ_PATH), str(CATALOG), "test-fixture"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn('"id": "test-fixture"', proc.stdout)


if __name__ == "__main__":
    unittest.main()
