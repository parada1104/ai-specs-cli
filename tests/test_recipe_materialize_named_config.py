"""Tests for `write_named_server_config` in `lib/_internal/recipe-materialize.py`.

Group 2.2 of `mcp-compartido-por-proyecto`. Writes the named-server-config
file consumed by `mcp-proxy`. Spec:
`openspec/changes/mcp-compartido-por-proyecto/specs/mcp-named-config-materialization/spec.md`.
"""
from __future__ import annotations

import importlib.util
import json
import os
import stat
import sys
import tempfile
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


class WriteNamedServerConfigTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_named")

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.out = Path(self._tmp.name) / "proxy.named-config.json"

    def test_output_has_mcp_servers_top_level_shape(self):
        shared = {"trello": {"command": "uvx", "args": ["mcp-server-trello"], "mode": "shared"}}
        self.mod.write_named_server_config(shared, self.out)
        data = json.loads(self.out.read_text())
        self.assertEqual(list(data.keys()), ["mcpServers"])
        self.assertIn("trello", data["mcpServers"])

    def test_mode_key_is_not_present_in_any_server_entry(self):
        shared = {
            "trello": {"command": "uvx", "args": ["x"], "mode": "shared"},
            "github": {"command": "uvx", "args": ["y"], "mode": "shared", "env": {"K": "v"}},
        }
        self.mod.write_named_server_config(shared, self.out)
        data = json.loads(self.out.read_text())
        for name, entry in data["mcpServers"].items():
            self.assertNotIn("mode", entry, f"mode leaked into server '{name}'")

    def test_env_var_references_preserved_literally(self):
        shared = {
            "trello": {
                "command": "uvx",
                "args": ["mcp-server-trello"],
                "mode": "shared",
                "env": {"TRELLO_TOKEN": "$TRELLO_TOKEN", "API_KEY": "${SOME_API_KEY}"},
            }
        }
        self.mod.write_named_server_config(shared, self.out)
        # Raw text MUST keep the dollar-prefixed forms verbatim — no shell
        # expansion at materialization time.
        text = self.out.read_text()
        self.assertIn('"TRELLO_TOKEN": "$TRELLO_TOKEN"', text)
        self.assertIn('"API_KEY": "${SOME_API_KEY}"', text)
        data = json.loads(text)
        env = data["mcpServers"]["trello"]["env"]
        self.assertEqual(env["TRELLO_TOKEN"], "$TRELLO_TOKEN")
        self.assertEqual(env["API_KEY"], "${SOME_API_KEY}")

    def test_multiple_shared_servers_all_appear_under_mcp_servers(self):
        shared = {
            "trello": {"command": "uvx", "args": ["a"], "mode": "shared"},
            "github": {"command": "uvx", "args": ["b"], "mode": "shared"},
            "linear": {"command": "uvx", "args": ["c"], "mode": "shared"},
        }
        self.mod.write_named_server_config(shared, self.out)
        data = json.loads(self.out.read_text())
        self.assertEqual(
            set(data["mcpServers"].keys()), {"trello", "github", "linear"}
        )

    def test_file_is_written_with_mode_0600(self):
        shared = {"trello": {"command": "uvx", "args": ["x"], "mode": "shared"}}
        self.mod.write_named_server_config(shared, self.out)
        st_mode = os.stat(self.out).st_mode
        perms = stat.S_IMODE(st_mode)
        self.assertEqual(
            perms,
            0o600,
            f"expected 0o600, got {oct(perms)} — named-config may contain secrets",
        )


if __name__ == "__main__":
    unittest.main()
