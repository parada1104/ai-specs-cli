import re
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home

_RECIPE_ID_RE = re.compile(r"\]\s+(\S+)")


def _extract_ids(output: str) -> list[str]:
    """Extract recipe IDs from `recipe list` output lines."""
    ids = []
    for line in output.strip().splitlines():
        m = _RECIPE_ID_RE.search(line)
        if m:
            ids.append(m.group(1))
    return ids


class RecipeListTests(unittest.TestCase):
    def setUp(self):
        self._home_td = tempfile.TemporaryDirectory(prefix="recipe-list-home-")
        self.addCleanup(self._home_td.cleanup)

    def _cli_home(self, catalog_recipes: dict[str, str] | None = None) -> Path:
        home = isolated_home(Path(self._home_td.name))
        if catalog_recipes is not None:
            catalog_link = home / "catalog"
            catalog_link.unlink()
            catalog_dir = home / "catalog" / "recipes"
            catalog_dir.mkdir(parents=True)
            for rid, content in catalog_recipes.items():
                rdir = catalog_dir / rid
                rdir.mkdir()
                (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        return home

    def _make_project(self, manifest_content: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs_dir = project / "ai-specs"
        ai_specs_dir.mkdir()
        (ai_specs_dir / "skills").mkdir()
        (ai_specs_dir / "commands").mkdir()
        (ai_specs_dir / "ai-specs.toml").write_text(manifest_content, encoding="utf-8")
        return project

    def _invoke(self, project: Path, home: Path):
        return invoke(project, "recipe", "list", cli_home=home)

    def test_list_shows_available_when_not_in_manifest(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest)
        home = self._cli_home({"my-recipe": recipe_toml})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        ids = _extract_ids(r.stdout)
        self.assertEqual(len(ids), 1)
        self.assertEqual(ids[0], "my-recipe")
        self.assertIn("available", r.stdout)

    def test_list_shows_installed_when_enabled_true(self):
        manifest = (
            '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
            "[recipes.my-recipe]\nenabled = true\n"
        )
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest)
        home = self._cli_home({"my-recipe": recipe_toml})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        lines = [l for l in r.stdout.splitlines() if "my-recipe" in l]
        self.assertEqual(len(lines), 1)
        self.assertIn("installed", lines[0])

    def test_list_catalog_version_info_only_not_outdated(self):
        manifest = (
            '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
            "[recipes.my-recipe]\nenabled = true\n"
        )
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "3.1.4"\n'
        )
        project = self._make_project(manifest)
        home = self._cli_home({"my-recipe": recipe_toml})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        lines = [l for l in r.stdout.splitlines() if "my-recipe" in l]
        self.assertEqual(len(lines), 1)
        self.assertIn("3.1.4", lines[0])
        self.assertIn("installed", lines[0])
        self.assertNotIn("outdated", lines[0])
        self.assertNotIn("outdated", r.stdout)

    def test_list_shows_disabled_when_enabled_false(self):
        manifest = (
            '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
            "[recipes.my-recipe]\nenabled = false\n"
        )
        recipe_toml = '[recipe]\nid = "my-recipe"\nname = "My Recipe"\ndescription = "Desc"\nversion = "1.0.0"\n'
        project = self._make_project(manifest)
        home = self._cli_home({"my-recipe": recipe_toml})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        lines = [l for l in r.stdout.splitlines() if "my-recipe" in l]
        self.assertEqual(len(lines), 1)
        self.assertIn("disabled", lines[0])

    def test_empty_catalog(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        project = self._make_project(manifest)
        home = self._cli_home({})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(_extract_ids(r.stdout), [])

    def test_list_hides_internal_test_recipes(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        public = (
            '[recipe]\nid = "public-recipe"\nname = "Public"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        internal = (
            '[recipe]\nid = "test-fixture"\nname = "Test Fixture"\n'
            'description = "internal"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        home = self._cli_home({"public-recipe": public, "test-fixture": internal})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        ids = _extract_ids(r.stdout)
        self.assertEqual(ids, ["public-recipe"])
        self.assertFalse(any(rid.startswith("test-") for rid in ids))

    def test_list_uses_cli_catalog_when_project_has_no_local_catalog(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        project = self._make_project(manifest)
        home = self._cli_home()
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("trello-mcp-workflow", r.stdout)
        ids = _extract_ids(r.stdout)
        self.assertFalse(any(r.startswith("test-") for r in ids))

    def test_invalid_recipe_toml_shows_error(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        bad_toml = '[recipe]\nname = "Bad"\ndescription = "Missing id"\n'
        project = self._make_project(manifest)
        home = self._cli_home({"bad-recipe": bad_toml})
        r = self._invoke(project, home)
        self.assertEqual(r.returncode, 0, r.stderr)
        lines = [l for l in r.stdout.splitlines() if "bad-recipe" in l]
        self.assertEqual(len(lines), 1)
        self.assertIn("error", lines[0])

    def test_list_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        manifest = '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        cli_recipe = '[recipe]\nid = "shared-recipe"\nname = "CLI Recipe"\ndescription = "Desc"\nversion = "2.0.0"\n'
        project = self._make_project(manifest)
        local_catalog = project / "catalog" / "recipes" / "shared-recipe"
        local_catalog.mkdir(parents=True)
        (local_catalog / "recipe.toml").write_text(
            '[recipe]\nid = "shared-recipe"\nname = "Local Recipe"\ndescription = "Desc"\nversion = "9.9.9"\n'
        )
        home = self._cli_home({"shared-recipe": cli_recipe})
        r = self._invoke(project, home)
        lines = [l for l in r.stdout.splitlines() if "shared-recipe" in l]
        self.assertIn("CLI Recipe", lines[0])
        self.assertIn("2.0.0", lines[0])

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = self._cli_home()
            r = invoke(Path(tmp), "recipe", "list", cli_home=home)
            self.assertEqual(r.returncode, 1)
            self.assertIn("Proyecto no inicializado", r.stderr)

    def test_cli_produces_output(self):
        home = self._cli_home()
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        (root / "ai-specs").mkdir()
        (root / "ai-specs" / "skills").mkdir()
        (root / "ai-specs" / "commands").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "test"\n\n[agents]\nenabled = ["claude"]\n'
        )
        r = invoke(root, "recipe", "list", cli_home=home)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("trello-mcp-workflow", r.stdout)
        self.assertNotIn("test-fixture", r.stdout)
        for line in r.stdout.splitlines():
            parts = line.split()
            if len(parts) >= 3 and parts[2].startswith("test-"):
                self.fail(f"internal test recipe leaked into CLI list: {line!r}")


if __name__ == "__main__":
    unittest.main()
