"""Tests for `split_mcps_by_mode` in `lib/_internal/recipe-materialize.py`.

Group 2.1 of `mcp-compartido-por-proyecto`. Pure function over the merged
MCP dict produced by `build_recipe_mcp`. Splits into (shared, stdio) such that
MCPs without an explicit `mode` field default to the stdio bucket
(consistent with the `mcp-mode-shared` spec).
"""
from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class SplitMcpsByModeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_split")

    def test_mixed_input_splits_into_shared_and_stdio(self):
        merged = {
            "trello": {"command": "uvx", "args": ["mcp-server-trello"], "mode": "shared"},
            "github": {"command": "npx", "args": ["-y", "github-mcp"]},
        }
        shared, stdio = self.mod.split_mcps_by_mode(merged)
        self.assertEqual(set(shared.keys()), {"trello"})
        self.assertEqual(set(stdio.keys()), {"github"})
        self.assertEqual(shared["trello"]["command"], "uvx")
        self.assertEqual(stdio["github"]["command"], "npx")

    def test_empty_input_returns_two_empty_dicts(self):
        shared, stdio = self.mod.split_mcps_by_mode({})
        self.assertEqual(shared, {})
        self.assertEqual(stdio, {})

    def test_only_stdio_returns_empty_shared(self):
        merged = {
            "a": {"command": "npx", "mode": "stdio"},
            "b": {"command": "npx"},
        }
        shared, stdio = self.mod.split_mcps_by_mode(merged)
        self.assertEqual(shared, {})
        self.assertEqual(set(stdio.keys()), {"a", "b"})

    def test_only_shared_returns_empty_stdio(self):
        merged = {
            "trello": {"command": "uvx", "mode": "shared"},
            "github": {"command": "uvx", "mode": "shared"},
        }
        shared, stdio = self.mod.split_mcps_by_mode(merged)
        self.assertEqual(set(shared.keys()), {"trello", "github"})
        self.assertEqual(stdio, {})

    def test_mcp_without_mode_falls_in_stdio_bucket(self):
        merged = {"plain": {"command": "npx", "args": ["-y", "plain-mcp"]}}
        shared, stdio = self.mod.split_mcps_by_mode(merged)
        self.assertEqual(shared, {})
        self.assertIn("plain", stdio)


if __name__ == "__main__":
    unittest.main()
