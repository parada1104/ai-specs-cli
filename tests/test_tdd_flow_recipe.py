import importlib.util
import re
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "tdd-flow"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _recipe_version() -> str:
    """Read the recipe version dynamically from recipe.toml."""
    text = (CATALOG / RECIPE_ID / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, "could not find version in recipe.toml"
    return match.group(1)


class TddFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_for_tdd_flow")

    def test_recipe_validates_and_declares_capability(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)
        cap_ids = [c.id for c in recipe.capabilities]
        self.assertIn("test-runner", cap_ids)
        # Bundled skill is declared
        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertIn(("tdd-flow", "bundled"), skill_ids)
        # Command is declared
        cmd_ids = [c.id for c in recipe.commands]
        self.assertIn("tdd", cmd_ids)
        # test_command config is declared, optional, no default (project-specific)
        fields = recipe.config_schema.fields
        self.assertIn("test_command", fields)
        self.assertFalse(fields["test_command"].required)
        self.assertEqual(fields["test_command"].type, "string")
        self.assertIsNone(fields["test_command"].default)

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        version = _recipe_version()
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{version}"\n'
        )
        return root

    def test_materialize_produces_skill_command_and_doc(self):
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        skill = (
            root / "ai-specs" / ".recipe" / RECIPE_ID
            / "skills" / "tdd-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = root / "ai-specs" / "commands" / "tdd.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")


if __name__ == "__main__":
    unittest.main()
