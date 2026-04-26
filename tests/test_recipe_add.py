import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_ADD_PATH = ROOT / "lib" / "_internal" / "recipe-add.py"
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
TOML_READ_PATH = ROOT / "lib" / "_internal" / "toml-read.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeAddTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_ADD_PATH, "recipe_add_internal")

    def _make_project(self, manifest_content: str, catalog_recipes: dict | None = None) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs_dir = project / "ai-specs"
        ai_specs_dir.mkdir()
        (ai_specs_dir / "ai-specs.toml").write_text(manifest_content, encoding="utf-8")
        if catalog_recipes:
            catalog_dir = project / "catalog" / "recipes"
            catalog_dir.mkdir(parents=True)
            for rid, content in catalog_recipes.items():
                rdir = catalog_dir / rid
                rdir.mkdir()
                (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        return project

    def test_add_appends_recipe_with_exact_version(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "2.1.0"\n'
        )
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.my-recipe]", manifest_text)
        self.assertIn('enabled = true', manifest_text)
        self.assertIn('version = "2.1.0"', manifest_text)

    def test_add_aborts_when_recipe_already_exists(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = true\nversion = \"1.0.0\"\n"
        )
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 1)

    def test_add_fails_when_recipe_not_in_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        rc = self.mod.add_recipe(project, "nonexistent")
        self.assertEqual(rc, 1)

    def test_add_does_not_mutate_other_files(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        other_file = project / "other.txt"
        other_file.write_text("original", encoding="utf-8")

        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)
        self.assertEqual(other_file.read_text(encoding="utf-8"), "original")

    def test_add_shows_preview_of_primitives(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[provides]
skills = [
    { id = "my-skill", source = "bundled" },
]
commands = [
    { id = "my-cmd", path = "commands/my-cmd.md" },
]
"""
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

    def test_double_add_is_idempotent(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        rc1 = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc1, 0)
        rc2 = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc2, 1)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        count = manifest_text.count("[recipes.my-recipe]")
        self.assertEqual(count, 1)

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = subprocess.run(
                ["python3", str(RECIPE_ADD_PATH), tmp, "my-recipe"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertIn("Proyecto no inicializado", proc.stderr)


if __name__ == "__main__":
    unittest.main()
