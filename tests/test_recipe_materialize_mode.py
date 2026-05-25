"""Tests for `mode` preservation in `build_recipe_mcp` shallow merge.

Group 1.3 of mcp-compartido-por-proyecto: per the spec
`specs/mcp-preset-merge/spec.md`, the `mode` key MUST flow through the
existing shallow merge with the same precedence as any other field —
manifest wins over preset, preset value inherited when manifest omits
`mode`, and a conflict on `mode` MUST emit the standard merge warning.

The existing implementation in `lib/_internal/recipe-materialize.py`
loops over preset config items generically, so these tests are expected
to pass without code changes (verifying the merge already satisfies the
spec). If a regression appears the suite catches it.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
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


def _write_recipe(catalog_dir: Path, recipe_id: str, mcp_block: str) -> None:
    recipe_dir = catalog_dir / recipe_id
    recipe_dir.mkdir(parents=True, exist_ok=True)
    (recipe_dir / "recipe.toml").write_text(
        '[recipe]\n'
        f'id = "{recipe_id}"\n'
        f'name = "{recipe_id}"\n'
        'description = "fixture"\n'
        'version = "1.0.0"\n'
        '\n'
        + mcp_block
    )


class RecipeMaterializeModeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_mode_internal")

    def _make_catalog(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def test_manifest_mode_overrides_recipe_mode_with_warning(self):
        """recipe shared + manifest stdio -> merged stdio, warn emitted on mode key conflict."""
        catalog = self._make_catalog()
        _write_recipe(
            catalog,
            "trello-recipe",
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'mode = "shared"\n'
            'command = "uvx"\n',
        )
        manifest_mcp = {"trello": {"mode": "stdio", "command": "npx"}}

        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            merged = self.mod.build_recipe_mcp(catalog, ["trello-recipe"], manifest_mcp)

        self.assertEqual(merged["trello"]["mode"], "stdio")
        self.assertIn("mode", stderr.getvalue())
        self.assertIn("trello", stderr.getvalue())

    def test_recipe_mode_inherited_when_manifest_omits_mode(self):
        """recipe shared + manifest entry without `mode` -> merged inherits shared."""
        catalog = self._make_catalog()
        _write_recipe(
            catalog,
            "trello-recipe",
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'mode = "shared"\n'
            'command = "uvx"\n',
        )
        manifest_mcp = {"trello": {"command": "uvx"}}

        merged = self.mod.build_recipe_mcp(catalog, ["trello-recipe"], manifest_mcp)

        self.assertEqual(merged["trello"]["mode"], "shared")

    def test_new_mcp_from_recipe_preserves_mode(self):
        """recipe shared + manifest has no entry for this MCP -> merged carries `mode = "shared"`."""
        catalog = self._make_catalog()
        _write_recipe(
            catalog,
            "trello-recipe",
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'mode = "shared"\n'
            'command = "uvx"\n',
        )
        manifest_mcp: dict = {}

        merged = self.mod.build_recipe_mcp(catalog, ["trello-recipe"], manifest_mcp)

        self.assertEqual(merged["trello"]["mode"], "shared")
        self.assertEqual(merged["trello"]["command"], "uvx")

    def test_mode_conflict_warning_format_matches_other_keys(self):
        """Warning on mode conflict uses the same `'<key>' conflicts with project manifest` template."""
        catalog = self._make_catalog()
        _write_recipe(
            catalog,
            "trello-recipe",
            '[[provides.mcp]]\n'
            'id = "trello"\n'
            'mode = "shared"\n',
        )
        manifest_mcp = {"trello": {"mode": "stdio"}}

        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            self.mod.build_recipe_mcp(catalog, ["trello-recipe"], manifest_mcp)

        output = stderr.getvalue()
        self.assertIn("'mode'", output)
        self.assertIn("conflicts with project manifest", output)
        self.assertIn("manifest wins", output)


if __name__ == "__main__":
    unittest.main()
