import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_LIST_PATH = ROOT / "lib" / "_internal" / "recipe-list.py"
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


class RecipeListTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_LIST_PATH, "recipe_list_internal")

    def _make_project(self, manifest_content: str, catalog_recipes: dict | None = None) -> Path:
        """Create a temporary project with ai-specs.toml and optional catalog."""
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

    def test_list_shows_available_when_not_in_manifest(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        results = self.mod.list_recipes(project)
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0]["id"], "my-recipe")
        self.assertEqual(results[0]["status"], "available")

    def test_list_shows_installed_when_enabled_true(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = true\nversion = \"1.0.0\"\n"
        )
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        results = self.mod.list_recipes(project)
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0]["status"], "installed")

    def test_list_shows_disabled_when_enabled_false(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = false\nversion = \"1.0.0\"\n"
        )
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest, {"my-recipe": recipe_toml})
        results = self.mod.list_recipes(project)
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0]["status"], "disabled")

    def test_empty_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        # catalog dir doesn't exist
        results = self.mod.list_recipes(project)
        self.assertEqual(len(results), 0)

    def test_invalid_recipe_toml_shows_error(self):
        manifest = '[project]\nname = "test"\n'
        bad_toml = '[recipe]\nname = "Bad"\ndescription = "Missing id"\n'
        project = self._make_project(manifest, {"bad-recipe": bad_toml})
        results = self.mod.list_recipes(project)
        self.assertEqual(len(results), 1)
        self.assertIn("error", results[0]["status"])

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = subprocess.run(
                ["python3", str(RECIPE_LIST_PATH), tmp],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertIn("Proyecto no inicializado", proc.stderr)

    def test_cli_produces_output(self):
        proc = subprocess.run(
            ["python3", str(RECIPE_LIST_PATH), str(ROOT)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("test-fixture", proc.stdout)


if __name__ == "__main__":
    unittest.main()
