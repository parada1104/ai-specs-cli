"""End-to-end tests for shared MCP handling in `materialize_recipes()`.

Group 2.3 of `mcp-compartido-por-proyecto`. Verifies that:
- With at least one shared MCP, `.ai-specs/run/proxy.named-config.json`
  is created at the git-root.
- Without any shared MCP, the file is NOT created and `.ai-specs/run/`
  may not even exist.
- Changing a shared MCP's effective config between syncs produces a
  different SHA-256, which Group 3 will use to drive restart detection.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import subprocess
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


def _git_init(path: Path) -> None:
    subprocess.run(
        ["git", "init", "-q", str(path)], check=True, capture_output=True
    )


def _write_recipe(catalog: Path, recipe_id: str, mcp_block: str) -> None:
    rdir = catalog / recipe_id
    rdir.mkdir(parents=True, exist_ok=True)
    (rdir / "recipe.toml").write_text(
        f'[recipe]\n'
        f'id = "{recipe_id}"\n'
        f'name = "{recipe_id}"\n'
        f'description = "fixture"\n'
        f'version = "1.0.0"\n'
        f'author = "ai-specs"\n'
        f'license = "MIT"\n'
        f'\n'
        f'[provides]\n'
        f'skills = []\n'
        f'\n'
        + mcp_block
        + '\n'
    )


def _make_workspace(recipe_id: str, mcp_block: str, recipe_section: str):
    tmp = tempfile.TemporaryDirectory()
    root = Path(tmp.name)
    _git_init(root)
    home = root / "_ai_specs_home"
    catalog = home / "catalog" / "recipes"
    catalog.mkdir(parents=True)
    _write_recipe(catalog, recipe_id, mcp_block)

    ai_specs = root / "ai-specs"
    ai_specs.mkdir()
    (ai_specs / "skills").mkdir()
    (ai_specs / "commands").mkdir()
    (ai_specs / "ai-specs.toml").write_text(
        "[project]\nname = 'fixture'\n\n"
        "[agents]\nenabled = ['claude']\n\n"
        + recipe_section
        + "\n"
    )
    return tmp, root, home


class MaterializeSharedIntegrationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_shared_int")

    def test_shared_mcp_creates_named_config_at_git_root(self):
        mcp_block = (
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'command = "uvx"\n'
            'args = ["mcp-server-trello"]\n'
            'mode = "shared"\n'
            '[provides.mcp.env]\n'
            'TRELLO_TOKEN = "$TRELLO_TOKEN"\n'
        )
        recipe_section = (
            '[recipes.shared-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        tmp, root, home = _make_workspace("shared-fixture", mcp_block, recipe_section)
        self.addCleanup(tmp.cleanup)

        rc = self.mod.materialize_recipes(root, home)
        self.assertEqual(rc, 0)

        named = root / ".ai-specs" / "run" / "proxy.named-config.json"
        self.assertTrue(named.is_file(), f"expected {named} to exist")
        data = json.loads(named.read_text())
        self.assertIn("trello", data["mcpServers"])
        entry = data["mcpServers"]["trello"]
        self.assertEqual(entry["command"], "uvx")
        self.assertNotIn("mode", entry)
        self.assertEqual(entry["env"]["TRELLO_TOKEN"], "$TRELLO_TOKEN")

    def test_no_shared_mcp_does_not_create_named_config(self):
        mcp_block = (
            '[[provides.mcp]]\n'
            'id = "github"\n'
            'command = "npx"\n'
            'args = ["-y", "@modelcontextprotocol/server-github"]\n'
        )
        recipe_section = (
            '[recipes.stdio-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        tmp, root, home = _make_workspace("stdio-fixture", mcp_block, recipe_section)
        self.addCleanup(tmp.cleanup)

        rc = self.mod.materialize_recipes(root, home)
        self.assertEqual(rc, 0)

        named = root / ".ai-specs" / "run" / "proxy.named-config.json"
        self.assertFalse(
            named.exists(), f"unexpected {named} created when no shared MCPs declared"
        )

    def test_named_config_sha256_changes_when_shared_mcp_changes(self):
        mcp_v1 = (
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'command = "uvx"\n'
            'args = ["mcp-server-trello"]\n'
            'mode = "shared"\n'
        )
        recipe_section = (
            '[recipes.hash-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        tmp, root, home = _make_workspace("hash-fixture", mcp_v1, recipe_section)
        self.addCleanup(tmp.cleanup)
        rc = self.mod.materialize_recipes(root, home)
        self.assertEqual(rc, 0)

        named = root / ".ai-specs" / "run" / "proxy.named-config.json"
        first = hashlib.sha256(named.read_bytes()).hexdigest()

        # Rewrite recipe with an altered args list. SHA-256 of the
        # canonical JSON MUST change — Group 3 keys restart detection
        # off this hash.
        catalog = home / "catalog" / "recipes"
        _write_recipe(
            catalog,
            "hash-fixture",
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'command = "uvx"\n'
            'args = ["mcp-server-trello", "--verbose"]\n'
            'mode = "shared"\n',
        )
        rc = self.mod.materialize_recipes(root, home)
        self.assertEqual(rc, 0)
        second = hashlib.sha256(named.read_bytes()).hexdigest()
        self.assertNotEqual(
            first, second, "named-config hash must change when shared MCP config changes"
        )


if __name__ == "__main__":
    unittest.main()
