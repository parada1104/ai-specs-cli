"""Group 8.1: catalog trello recipe declares `mode = "shared"` end-to-end.

Materializes the real `catalog/recipes/trello-mcp-workflow/` against a tmp
git-root fixture. Asserts that `.ai-specs/run/proxy.named-config.json` is
created and contains the trello server with the expected command/args/env
— proving that `mode = "shared"` is active in the recipe (otherwise the
named-config file is not written; see materialize_recipes split).
"""
from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"


def _load(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class TrelloRecipeSharedTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = _load(RECIPE_MATERIALIZE_PATH, "recipe_materialize_trello_shared")

    def test_trello_recipe_materializes_named_config_with_shared_server(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        subprocess.run(
            ["git", "init", "-q", str(root)], check=True, capture_output=True
        )

        ai_specs = root / "ai-specs"
        (ai_specs / "skills").mkdir(parents=True)
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture-trello'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\nversion = \"1.0.0\"\n"
            "config = { board_id = \"507f1f77bcf86cd799439011\" }\n"
        )

        # Point ai_specs_home at the worktree so the real trello recipe
        # under catalog/recipes/trello-mcp-workflow/ is materialized.
        rc = self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(rc, 0)

        named = root / ".ai-specs" / "run" / "proxy.named-config.json"
        self.assertTrue(
            named.is_file(),
            f"expected {named} to exist — recipe must declare mode = 'shared'",
        )
        data = json.loads(named.read_text())
        self.assertIn("mcpServers", data)
        self.assertIn(
            "trello",
            data["mcpServers"],
            f"trello server missing from named-config: {list(data['mcpServers'])}",
        )
        entry = data["mcpServers"]["trello"]
        self.assertEqual(entry["command"], "npx")
        self.assertEqual(entry["args"], ["-y", "@delorenj/mcp-server-trello"])
        self.assertEqual(entry["env"]["TRELLO_API_KEY"], "$TRELLO_API_KEY")
        self.assertEqual(entry["env"]["TRELLO_TOKEN"], "$TRELLO_TOKEN")
        # `mode` must NOT appear in the rendered named-config: it is a
        # routing hint stripped before serialisation.
        self.assertNotIn("mode", entry)


if __name__ == "__main__":
    unittest.main()
