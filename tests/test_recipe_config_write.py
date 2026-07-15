"""Tests for recipe-config-write.py surgical config updater."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
WRITE_PATH = ROOT / "lib" / "_internal" / "recipe-config-write.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class RecipeConfigWriteTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(WRITE_PATH, "recipe_config_write_internal")

    def _manifest(self, text: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        path = Path(tmp.name) / "ai-specs.toml"
        path.write_text(text, encoding="utf-8")
        return path

    def test_replace_existing_key(self):
        path = self._manifest(
            '[project]\nname = "p"\n\n'
            "[recipes.git-pr-flow]\n"
            "enabled = true\n"
            'version = "1.0"\n\n'
            "[recipes.git-pr-flow.config]\n"
            'base_branch = "main"  # keep-me-comment\n'
            "# other comment\n"
        )
        self.mod.update_recipe_config(path, "git-pr-flow", {"base_branch": "develop"})
        text = path.read_text(encoding="utf-8")
        self.assertIn('base_branch = "develop"', text)
        self.assertIn("# other comment", text)
        self.assertNotIn('base_branch = "main"', text)

    def test_insert_missing_key(self):
        path = self._manifest(
            "[recipes.trello-mcp-workflow]\n"
            "enabled = true\n"
            'version = "1.0"\n\n'
            "[recipes.trello-mcp-workflow.config]\n"
            'default_list = "In Progress"\n\n'
            "[recipes.other]\n"
            "enabled = false\n"
        )
        self.mod.update_recipe_config(
            path, "trello-mcp-workflow", {"board_id": "0123456789abcdef01234567"}
        )
        text = path.read_text(encoding="utf-8")
        self.assertIn('board_id = "0123456789abcdef01234567"', text)
        self.assertIn('default_list = "In Progress"', text)
        data = tomllib.loads(text)
        self.assertEqual(
            data["recipes"]["trello-mcp-workflow"]["config"]["board_id"],
            "0123456789abcdef01234567",
        )

    def test_comments_preserved(self):
        path = self._manifest(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
            "# keep this exact comment\n"
            'a = "1"\n'
        )
        before_comment = "# keep this exact comment\n"
        self.mod.update_recipe_config(path, "x", {"a": "2"})
        self.assertIn(before_comment, path.read_text(encoding="utf-8"))

    def test_insert_config_block_when_absent(self):
        path = self._manifest(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.y]\nenabled = false\n"
        )
        self.mod.update_recipe_config(path, "x", {"base_branch": "main"})
        data = tomllib.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(data["recipes"]["x"]["config"]["base_branch"], "main")

    def test_append_full_block_when_recipe_absent(self):
        path = self._manifest('[project]\nname = "p"\n')
        self.mod.update_recipe_config(path, "x", {"k": "v"})
        data = tomllib.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(data["recipes"]["x"]["config"]["k"], "v")
        self.assertEqual(data["recipes"]["x"]["enabled"], True)
        self.assertNotIn("version", data["recipes"]["x"])
        self.assertNotIn("version =", path.read_text(encoding="utf-8"))

    def test_bool_serialization(self):
        path = self._manifest(
            "[recipes.worktree-flow]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.worktree-flow.config]\n"
        )
        self.mod.update_recipe_config(path, "worktree-flow", {"auto_remove_merged": True})
        text = path.read_text(encoding="utf-8")
        self.assertIn("auto_remove_merged = true", text)
        self.assertNotIn("True", text)

    def test_invalid_write_restores_original(self):
        path = self._manifest(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
            'a = "1"\n'
        )
        original = path.read_text(encoding="utf-8")

        def bad_value(_v):
            return "[[[not-valid"

        with patch.object(self.mod._toml_write, "toml_value", side_effect=bad_value):
            with self.assertRaises(self.mod.RecipeConfigWriteError):
                self.mod.update_recipe_config(path, "x", {"a": "2"})
        self.assertEqual(path.read_text(encoding="utf-8"), original)

    def test_empty_values_is_noop(self):
        path = self._manifest(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
            'a = "1"\n'
        )
        before = path.read_text(encoding="utf-8")
        mtime_before = path.stat().st_mtime_ns
        self.mod.update_recipe_config(path, "x", {})
        self.assertEqual(path.read_text(encoding="utf-8"), before)
        self.assertEqual(path.stat().st_mtime_ns, mtime_before)

    def test_quoted_key_id(self):
        path = self._manifest('[project]\nname = "p"\n')
        self.mod.update_recipe_config(path, "my.recipe", {"base_branch": "main"})
        text = path.read_text(encoding="utf-8")
        self.assertIn('[recipes."my.recipe"]', text)
        self.assertIn('[recipes."my.recipe".config]', text)
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["my.recipe"]["config"]["base_branch"], "main")


if __name__ == "__main__":
    unittest.main()
