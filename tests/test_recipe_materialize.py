import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeMaterializeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")

    def _make_project(self, recipe_section: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + recipe_section
            + "\n"
        )
        return root

    def test_materializes_bundled_skill(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        skill_dir = root / "ai-specs" / "skills" / "test-skill"
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        cmd = root / "ai-specs" / "commands" / "test-command.md"
        self.assertTrue(cmd.is_file())

    def test_materializes_doc(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        doc = root / "docs" / "test-doc-output.md"
        self.assertTrue(doc.is_file())

    def test_materializes_template_not_exists(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        tpl = root / "docs" / "test-template-output.md"
        self.assertTrue(tpl.is_file())

    def test_skips_template_when_target_exists(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        existing = root / "docs" / "test-template-output.md"
        existing.parent.mkdir(parents=True, exist_ok=True)
        existing.write_text("existing")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertEqual(existing.read_text(), "existing")

    def test_writes_recipe_mcp_json(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        mcp_path = root / "ai-specs" / ".recipe-mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("test-mcp", data)
        self.assertEqual(data["test-mcp"]["command"], "npx")

    def test_disabled_recipe_skips_materialization(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = false\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertFalse((root / "ai-specs" / "skills" / "test-skill").exists())

    def test_version_mismatch_fails(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "2.0.0"\n'
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.materialize_recipes(root, ROOT)
        self.assertIn("version mismatch", str(ctx.exception))

    def test_unknown_recipe_fails(self):
        root = self._make_project(
            '[recipes.nonexistent]\nenabled = true\nversion = "1.0.0"\n'
        )
        with self.assertRaises(Exception):
            self.mod.materialize_recipes(root, ROOT)

    def test_no_recipes_section_succeeds(self):
        root = self._make_project("")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

    def test_recipe_overrides_user_local_skill_with_warning(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        # Pre-create a user-local skill with the same ID
        user_skill = root / "ai-specs" / "skills" / "test-skill"
        user_skill.mkdir(parents=True)
        (user_skill / "SKILL.md").write_text("user local")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        # Recipe version should have replaced the local one
        self.assertNotEqual((user_skill / "SKILL.md").read_text(), "user local")


if __name__ == "__main__":
    unittest.main()
