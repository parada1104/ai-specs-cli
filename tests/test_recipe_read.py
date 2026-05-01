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
        self.assertIsNone(data.init)

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

    # --- Init workflow tests -------------------------------------------------

    def _write_init_recipe(self, root: Path, recipe_id: str, init_body: str, prompt_name: str = "init.md") -> Path:
        recipe_dir = root / recipe_id
        recipe_dir.mkdir()
        prompt_dir = recipe_dir / "docs"
        prompt_dir.mkdir()
        (prompt_dir / prompt_name).write_text("# Init prompt\nConfigure this recipe.\n", encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\nid = "{recipe_id}"\nname = "Init Recipe"\ndescription = "D"\nversion = "1.0"\n'
            + init_body,
            encoding="utf-8",
        )
        return recipe_dir

    def test_init_workflow_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(
                Path(tmp),
                "init-recipe",
                '[init]\nprompt = "docs/init.md"\ndescription = "Configure"\nneeds_manifest = true\nneeds_mcp = ["trello", "openmemory"]\n',
            )
            data = self.mod.read_recipe(Path(tmp), "init-recipe")
            self.assertIsNotNone(data.init)
            self.assertEqual(data.init.prompt, "docs/init.md")
            self.assertEqual(data.init.description, "Configure")
            self.assertEqual(data.init.needs_manifest, True)
            self.assertEqual(data.init.needs_mcp, ["trello", "openmemory"])

    def test_init_metadata_in_json(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "init-recipe", '[init]\nprompt = "docs/init.md"\nneeds_mcp = ["trello"]\n')
            data = self.mod.read_recipe(Path(tmp), "init-recipe")
            payload = self.mod.recipe_to_dict(data)
            self.assertEqual(
                payload["init"],
                {
                    "prompt": "docs/init.md",
                    "description": "",
                    "needs_manifest": False,
                    "needs_mcp": ["trello"],
                },
            )

    def test_recipe_without_init_has_null_json_init(self):
        data = self.mod.read_recipe(CATALOG, "test-fixture")
        payload = self.mod.recipe_to_dict(data)
        self.assertIn("init", payload)
        self.assertIsNone(payload["init"])

    def test_init_missing_prompt_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "bad-init"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "bad-init"\nname = "Bad"\ndescription = "D"\nversion = "1.0"\n[init]\ndescription = "Missing prompt"\n',
                encoding="utf-8",
            )
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("[init].prompt", str(ctx.exception))

    def test_init_unknown_field_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "docs/init.md"\nextra = "nope"\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("unsupported init field 'extra'", str(ctx.exception))

    def test_init_invalid_needs_mcp_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "docs/init.md"\nneeds_mcp = ["trello", ""]\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("needs_mcp", str(ctx.exception))

    def test_init_absolute_prompt_path_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "/tmp/init.md"\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("relative", str(ctx.exception))

    def test_init_parent_traversal_prompt_path_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "../init.md"\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("inside the recipe directory", str(ctx.exception))

    def test_init_directory_prompt_path_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "docs"\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("must be a file", str(ctx.exception))

    def test_init_missing_prompt_file_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._write_init_recipe(Path(tmp), "bad-init", '[init]\nprompt = "docs/missing.md"\n')
            with self.assertRaises(self.mod.RecipeValidationError) as ctx:
                self.mod.read_recipe(Path(tmp), "bad-init")
            self.assertIn("init prompt file not found", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
