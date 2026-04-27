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

    # --- V2 schema tests -----------------------------------------------------

    def test_v1_recipe_has_empty_v2_fields(self):
        data = self.mod.read_recipe(CATALOG, "test-fixture")
        self.assertEqual(data.capabilities, [])
        self.assertEqual(data.hooks, [])
        self.assertEqual(data.config_schema.fields, {})

    def test_v2_capability_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "cap-recipe"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "cap-recipe"\nname = "Cap"\ndescription = "D"\nversion = "1.0"\n'
                '[[capabilities]]\nid = "tracker"\n'
                '[[capabilities]]\nid = "canonical-memory"\n'
            )
            data = self.mod.read_recipe(Path(tmp), "cap-recipe")
            self.assertEqual(len(data.capabilities), 2)
            self.assertEqual(data.capabilities[0].id, "tracker")
            self.assertEqual(data.capabilities[1].id, "canonical-memory")

    def test_v2_duplicate_capability_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "dup-cap"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "dup-cap"\nname = "Dup"\ndescription = "D"\nversion = "1.0"\n'
                '[[capabilities]]\nid = "tracker"\n'
                '[[capabilities]]\nid = "tracker"\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "dup-cap")
            self.assertIn("duplicate capability id", str(ctx.exception))

    def test_v2_missing_capability_id_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-cap"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "bad-cap"\nname = "Bad"\ndescription = "D"\nversion = "1.0"\n'
                '[[capabilities]]\nid = ""\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-cap")
            self.assertIn("missing or invalid required field 'id'", str(ctx.exception))

    def test_v2_hook_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "hook-recipe"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "hook-recipe"\nname = "Hook"\ndescription = "D"\nversion = "1.0"\n'
                '[[hooks]]\nevent = "on-sync"\naction = "validate-config"\n'
            )
            data = self.mod.read_recipe(Path(tmp), "hook-recipe")
            self.assertEqual(len(data.hooks), 1)
            self.assertEqual(data.hooks[0].event, "on-sync")
            self.assertEqual(data.hooks[0].action, "validate-config")

    def test_v2_missing_hook_event_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-hook"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "bad-hook"\nname = "Bad"\ndescription = "D"\nversion = "1.0"\n'
                '[[hooks]]\naction = "validate-config"\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-hook")
            self.assertIn("missing or invalid required field 'event'", str(ctx.exception))

    def test_v2_missing_hook_action_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-hook"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "bad-hook"\nname = "Bad"\ndescription = "D"\nversion = "1.0"\n'
                '[[hooks]]\nevent = "on-sync"\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-hook")
            self.assertIn("missing or invalid required field 'action'", str(ctx.exception))

    def test_v2_config_schema_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "cfg-recipe"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "cfg-recipe"\nname = "Cfg"\ndescription = "D"\nversion = "1.0"\n'
                '[config.timeout]\nrequired = false\ntype = "integer"\ndefault = 30\n'
                '[config.board_id]\nrequired = true\ntype = "string"\n'
            )
            data = self.mod.read_recipe(Path(tmp), "cfg-recipe")
            self.assertIn("timeout", data.config_schema.fields)
            self.assertIn("board_id", data.config_schema.fields)
            self.assertEqual(data.config_schema.fields["timeout"].required, False)
            self.assertEqual(data.config_schema.fields["timeout"].default, 30)
            self.assertEqual(data.config_schema.fields["board_id"].required, True)

    def test_v2_invalid_config_field_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-cfg"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "bad-cfg"\nname = "Bad"\ndescription = "D"\nversion = "1.0"\n'
                '[config.timeout]\nrequired = "not-a-bool"\n'
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-cfg")
            self.assertIn("missing or invalid 'required'", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
