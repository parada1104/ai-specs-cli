"""Validation + materialization tests for the worktree-flow catalog recipe."""

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class WorktreeFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.materialize = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal_wtf"
        )

    def test_recipe_validates(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "worktree-flow")
        cap_ids = {c.id for c in recipe.capabilities}
        self.assertIn("worktree-isolation", cap_ids)

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            '[recipes.worktree-flow]\nenabled = true\nversion = "1.0.0"\n'
        )
        return root

    def test_materializes_skill_commands_and_script(self):
        root = self._make_project()
        self.assertEqual(self.materialize.materialize_recipes(root, ROOT), 0)

        skill = (
            root / "ai-specs" / ".recipe" / "worktree-flow" / "skills"
            / "worktree-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), "bundled skill should materialize")

        for cmd in ("worktree-new", "worktree-clean"):
            path = root / "ai-specs" / "commands" / f"{cmd}.md"
            self.assertTrue(path.is_file(), f"command {cmd} should materialize")

        script = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertTrue(script.is_file(), "cleanup script should materialize")


if __name__ == "__main__":
    unittest.main()
