"""Black-box tests for `ai-specs recipe list` (converted from recipe-list.py internals)."""
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, temp_project  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

RECIPE_TOML = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'


class RecipeListTests(unittest.TestCase):
    def _list(self, manifest: str, catalog_recipes: dict[str, str] | None = None,
              project_catalog: dict[str, str] | None = None, name: str = "test"):
        """Run `ai-specs recipe list` in an isolated home with an exact catalog set."""
        td, project = temp_project(name=name)
        self.addCleanup(td.cleanup)
        (project / "ai-specs" / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        for rid, content in (project_catalog or {}).items():
            rdir = project / "catalog" / "recipes" / rid
            rdir.mkdir(parents=True)
            (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        home_base = tempfile.TemporaryDirectory()
        self.addCleanup(home_base.cleanup)
        home = isolated_home(Path(home_base.name), catalog=False)
        for rid, content in (catalog_recipes or {}).items():
            rdir = home / "catalog" / "recipes" / rid
            rdir.mkdir(parents=True, exist_ok=True)
            (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        result = invoke(project, "recipe", "list", cli_home=home)
        return result

    def _list_full_catalog(self, manifest: str, name: str = "test"):
        """Run `ai-specs recipe list` with the repo catalog (default isolated home)."""
        td, project = temp_project(name=name)
        self.addCleanup(td.cleanup)
        (project / "ai-specs" / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        result = invoke(project, "recipe", "list")
        return result

    def test_list_shows_available_when_not_in_manifest(self):
        result = self._list('[project]\nname = "test"\n', {"my-recipe": RECIPE_TOML})
        self.assertEqual(result.returncode, 0)
        self.assertIn("[available   ]  my-recipe", result.stdout)

    def test_list_shows_installed_when_enabled_true(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = true\nversion = \"1.0.0\"\n"
        )
        result = self._list(manifest, {"my-recipe": RECIPE_TOML})
        self.assertEqual(result.returncode, 0)
        self.assertIn("[installed   ]  my-recipe", result.stdout)

    def test_list_catalog_version_info_only_not_outdated(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = true\n"
        )
        result = self._list(manifest, {"my-recipe": RECIPE_TOML})
        self.assertEqual(result.returncode, 0)
        self.assertIn("[installed   ]  my-recipe                 1.0.0", result.stdout)
        self.assertNotIn("[outdated", result.stdout)

    def test_list_shows_disabled_when_enabled_false(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = false\nversion = \"1.0.0\"\n"
        )
        result = self._list(manifest, {"my-recipe": RECIPE_TOML})
        self.assertEqual(result.returncode, 0)
        self.assertIn("[disabled    ]  my-recipe", result.stdout)

    def test_empty_catalog(self):
        result = self._list('[project]\nname = "test"\n', catalog_recipes={})
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "No recipes available.")

    def test_list_hides_internal_test_recipes(self):
        public = (
            '[recipe]\nid = "public-recipe"\nname = "Public"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        internal = (
            '[recipe]\nid = "test-fixture"\nname = "Test Fixture"\n'
            'description = "internal"\nversion = "1.0.0"\n'
        )
        result = self._list(
            '[project]\nname = "test"\n',
            {"public-recipe": public, "test-fixture": internal},
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("public-recipe", result.stdout)
        self.assertNotIn("test-fixture", result.stdout)
        recipe_rows = [line for line in result.stdout.splitlines() if line.startswith("[")]
        self.assertEqual(len(recipe_rows), 1)
        self.assertFalse(any(rid.startswith("test-") for rid in recipe_rows))

    def test_list_uses_cli_catalog_when_project_has_no_local_catalog(self):
        result = self._list_full_catalog('[project]\nname = "test"\n')
        self.assertEqual(result.returncode, 0)
        self.assertIn("trello-mcp-workflow", result.stdout)
        self.assertNotIn("test-fixture", result.stdout)
        for line in result.stdout.splitlines():
            parts = line.split()
            if len(parts) >= 2 and parts[1].startswith("test-"):
                self.fail(f"internal test recipe leaked into CLI list: {line!r}")

    def test_invalid_recipe_toml_shows_error(self):
        bad_toml = '[recipe]\nname = "Bad"\ndescription = "Missing id"\n'
        result = self._list('[project]\nname = "test"\n', {"bad-recipe": bad_toml})
        self.assertEqual(result.returncode, 0)
        self.assertRegex(result.stdout, r"\[error .*\]  bad-recipe")

    def test_list_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        cli_recipe = '[recipe]\nid = "shared-recipe"\nname = "CLI Recipe"\ndescription = "Desc"\nversion = "2.0.0"\n'
        local_recipe = '[recipe]\nid = "shared-recipe"\nname = "Local Recipe"\ndescription = "Desc"\nversion = "9.9.9"\n'
        result = self._list(
            '[project]\nname = "test"\n',
            catalog_recipes={"shared-recipe": cli_recipe},
            project_catalog={"shared-recipe": local_recipe},
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("CLI Recipe", result.stdout)
        self.assertIn("2.0.0", result.stdout)
        self.assertNotIn("9.9.9", result.stdout)

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = invoke(Path(tmp), "recipe", "list")
            self.assertEqual(result.returncode, 1)
            self.assertIn("Project not initialized", result.stderr)

    def test_cli_produces_output(self):
        result = invoke(ROOT, "recipe", "list")
        self.assertEqual(result.returncode, 0)
        self.assertIn("trello-mcp-workflow", result.stdout)
        self.assertNotIn("test-fixture", result.stdout)
        for line in result.stdout.splitlines():
            # status column then id — reject any catalog id starting with test-
            parts = line.split()
            if len(parts) >= 2 and parts[1].startswith("test-"):
                self.fail(f"internal test recipe leaked into CLI list: {line!r}")


if __name__ == "__main__":
    unittest.main()
