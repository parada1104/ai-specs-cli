"""Unit tests for brief-render-policy.py."""
import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
POLICY_PATH = ROOT / "lib" / "_internal" / "brief-render-policy.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class BriefRenderPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(POLICY_PATH, "brief_render_policy_unit")

    def _write_toml(self, content: str) -> Path:
        tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".toml", delete=False)
        self.addCleanup(lambda: Path(tmp.name).unlink(missing_ok=True))
        tmp.write(content)
        tmp.flush()
        return Path(tmp.name)

    def test_no_brief_table_defaults_true(self):
        self.assertTrue(self.mod.brief_render_enabled({}))

    def test_brief_without_render_defaults_true(self):
        self.assertTrue(self.mod.brief_render_enabled({"brief": {}}))

    def test_render_true(self):
        self.assertTrue(self.mod.brief_render_enabled({"brief": {"render": True}}))

    def test_render_false(self):
        self.assertFalse(self.mod.brief_render_enabled({"brief": {"render": False}}))

    def test_render_string_raises(self):
        with self.assertRaises(ValueError) as ctx:
            self.mod.brief_render_enabled({"brief": {"render": "false"}})
        self.assertIn("[brief].render", str(ctx.exception))

    def test_render_int_raises(self):
        with self.assertRaises(ValueError):
            self.mod.brief_render_enabled({"brief": {"render": 1}})

    def test_load_from_toml_file(self):
        path = self._write_toml("[brief]\nrender = false\n")
        self.assertFalse(self.mod.load_brief_render_enabled(path))

    def test_cli_prints_false(self):
        path = self._write_toml("[brief]\nrender = false\n")
        result = subprocess.run(
            [sys.executable, str(POLICY_PATH), str(path)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "false")

    def test_cli_validate_rejects_string(self):
        path = self._write_toml('[brief]\nrender = "false"\n')
        result = subprocess.run(
            [sys.executable, str(POLICY_PATH), str(path), "--validate"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[brief].render", result.stderr)

    def test_has_dead_recipe_fragments_true(self):
        resolved = {
            "enabled": ["session-context"],
            "recipes": {
                "session-context": {
                    "brief_fragments": {"workflow_rules": [{"key": "x", "text": "y"}]}
                }
            },
        }
        self.assertTrue(self.mod.has_dead_recipe_fragments(resolved))

    def test_has_dead_recipe_fragments_false(self):
        resolved = {
            "enabled": ["session-context"],
            "recipes": {"session-context": {}},
        }
        self.assertFalse(self.mod.has_dead_recipe_fragments(resolved))


if __name__ == "__main__":
    unittest.main()
