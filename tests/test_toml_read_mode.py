"""Tests for [mcp.<name>] `mode` enum validation in toml-read.py.

Group 1.2 of mcp-compartido-por-proyecto: the manifest normalizer MUST
accept `mode = "shared"` and `mode = "stdio"`, treat absence as stdio,
and reject any other value with an explicit error listing valid options.
"""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TOML_READ_PATH = ROOT / "lib" / "_internal" / "toml-read.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TomlReadModeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(TOML_READ_PATH, "toml_read_mode_internal")

    def write_manifest(self, text: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        manifest = root / "ai-specs.toml"
        manifest.write_text(text)
        return manifest

    def test_mcp_with_mode_shared_preserved(self):
        """Manifest [mcp.trello] with `mode = "shared"` normalizes with mode field intact."""
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[mcp.trello]\n"
            "command = 'uvx'\n"
            "args = ['mcp-trello']\n"
            "mode = 'shared'\n"
        )
        data = self.mod.load_toml(manifest)
        mcp = self.mod.read_mcp(data)
        self.assertEqual(mcp["trello"]["mode"], "shared")

    def test_mcp_with_mode_stdio_preserved(self):
        """Manifest [mcp.trello] with explicit `mode = "stdio"` normalizes with mode field intact."""
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[mcp.trello]\n"
            "command = 'npx'\n"
            "mode = 'stdio'\n"
        )
        data = self.mod.load_toml(manifest)
        mcp = self.mod.read_mcp(data)
        self.assertEqual(mcp["trello"]["mode"], "stdio")

    def test_mcp_without_mode_no_breaking_change(self):
        """Manifest [mcp.example] without `mode` normalizes unchanged (no `mode` key emitted)."""
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[mcp.example]\n"
            "command = 'node'\n"
            "args = ['server.js']\n"
        )
        data = self.mod.load_toml(manifest)
        mcp = self.mod.read_mcp(data)
        self.assertNotIn("mode", mcp["example"])

    def test_mcp_with_unknown_mode_rejected(self):
        """Manifest [mcp.broken] with `mode = "foo"` raises with valid values listed."""
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[mcp.broken]\n"
            "command = 'uvx'\n"
            "mode = 'foo'\n"
        )
        data = self.mod.load_toml(manifest)
        with self.assertRaises(ValueError) as ctx:
            self.mod.read_mcp(data)
        msg = str(ctx.exception)
        self.assertIn("mode", msg)
        self.assertIn("foo", msg)
        self.assertIn("shared", msg)
        self.assertIn("stdio", msg)


if __name__ == "__main__":
    unittest.main()
