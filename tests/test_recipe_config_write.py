"""Black-box tests for `ai-specs recipe configure` surgical manifest writes.

The writer under test (`recipe-config-write.py`) has no direct CLI surface of
its own; every test drives it through
`bin/ai-specs recipe configure <id> --set KEY=VALUE` against an isolated
install root whose catalog declares the exact schema fields each scenario
needs. Assertion intent is byte-level manifest surgery observed on
`ai-specs/ai-specs.toml`.
"""
from __future__ import annotations

import json
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

# Test recipe with exactly the config fields the scenarios need. `branches` is
# declared as an unconstrained "array" type so list values pass schema
# validation and reach the writer (whose multiline rejection is what the
# corresponding tests observe).
_RECIPE_X_TOML = """[recipe]
id = "x"
name = "X"
description = "test recipe x"
version = "1.0.0"

[config.a]
required = false
type = "string"

[config.branch]
required = false
type = "string"

[config.base_branch]
required = false
type = "string"

[config.board_id]
required = false
type = "string"

[config.url]
required = false
type = "string"

[config.k]
required = false
type = "string"

[config.branches]
required = false
type = "array"

[config.auto_remove_merged]
required = false
type = "boolean"

[config.reconcile]
scope_field = "board_id"
max_age_seconds = 900

[[config.reconcile.expectations]]
event = "delivery"
property = "list"
config_field = "default_list"
"""

_MY_RECIPE_TOML = """[recipe]
id = "my.recipe"
name = "My Recipe"
description = "dotted recipe id fixture"
version = "1.0.0"

[config.base_branch]
required = false
type = "string"
"""


class RecipeConfigWriteTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._home_tmp = tempfile.TemporaryDirectory(prefix="ai-specs-home-class-")
        cls.addClassCleanup(cls._home_tmp.cleanup)
        cls.home = isolated_home(Path(cls._home_tmp.name), catalog=False)
        populate_catalog(cls.home, "x", _RECIPE_X_TOML)
        populate_catalog(cls.home, "my.recipe", _MY_RECIPE_TOML)

    def _project(self, text: str) -> tuple[tempfile.TemporaryDirectory, Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.parent.mkdir(parents=True)
        manifest.write_text(text, encoding="utf-8")
        return tmp, root, manifest

    def _configure(self, root: Path, *args: str):
        return invoke(root, "recipe", "configure", *args, cli_home=self.home)

    def _configure_json(self, root: Path, *args: str) -> tuple[dict, object]:
        result = self._configure(root, *args, "--json")
        return json.loads(result.stdout), result

    def test_replace_existing_key(self):
        tmp, root, manifest = self._project(
            '[project]\nname = "p"\n\n'
            "[recipes.x]\n"
            "enabled = true\n"
            'version = "1.0"\n\n'
            "[recipes.x.config]\n"
            'base_branch = "main"  # keep-me-comment\n'
            "# other comment\n"
        )
        result = self._configure(root, "x", "--set", "base_branch=develop")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn('base_branch = "develop"', text)
        self.assertIn("# other comment", text)
        self.assertNotIn('base_branch = "main"', text)

    def test_insert_missing_key(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\n"
            "enabled = true\n"
            'version = "1.0"\n\n'
            "[recipes.x.config]\n"
            'default_list = "In Progress"\n\n'
            "[recipes.other]\n"
            "enabled = false\n"
        )
        result = self._configure(root, "x", "--set", "board_id=0123456789abcdef01234567")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn('board_id = "0123456789abcdef01234567"', text)
        self.assertIn('default_list = "In Progress"', text)
        data = tomllib.loads(text)
        self.assertEqual(
            data["recipes"]["x"]["config"]["board_id"],
            "0123456789abcdef01234567",
        )

    def test_comments_preserved(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
            "# keep this exact comment\n"
            'a = "1"\n'
        )
        result = self._configure(root, "x", "--set", 'a="2"')
        self.assertEqual(result.returncode, 0)
        self.assertIn("# keep this exact comment\n", manifest.read_text(encoding="utf-8"))

    def test_insert_config_block_when_absent(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.y]\nenabled = false\n"
        )
        result = self._configure(root, "x", "--set", "board_id=abc")
        self.assertEqual(result.returncode, 0)
        data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        self.assertEqual(data["recipes"]["x"]["config"]["board_id"], "abc")

    def test_append_full_block_when_recipe_absent(self):
        tmp, root, manifest = self._project('[project]\nname = "p"\n')
        result = self._configure(root, "x", "--set", "k=v")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["x"]["config"]["k"], "v")
        self.assertEqual(data["recipes"]["x"]["enabled"], True)
        self.assertNotIn("version", data["recipes"]["x"])
        self.assertNotIn("version =", text)

    def test_bool_serialization(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
        )
        result = self._configure(root, "x", "--set", "auto_remove_merged=true")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn("auto_remove_merged = true", text)
        self.assertNotIn("True", text)

    # TRIAGE: recipe configure missing surface — the writer's
    # validate-and-restore fault path (a corrupt serialized value) cannot be
    # induced through the CLI because values are always valid TOML by the time
    # they reach the writer. This converts the observable half of the contract:
    # a writer-level rejection leaves the manifest byte-identical.
    def test_invalid_write_restores_original(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\nversion = \"1\"\n\n"
            "[recipes.x.config]\n"
            "branches = [\n"
            "  \"main\",\n"
            "  \"develop\",\n"
            "]\n"
            'a = "1"\n'
        )
        original = manifest.read_text(encoding="utf-8")
        result = self._configure(root, "x", "--set", 'branches=["release"]')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(manifest.read_text(encoding="utf-8"), original)

    # TRIAGE: recipe configure missing surface — the CLI rejects an empty
    # --set list at the argparse boundary (exit 2), so the writer's
    # empty-values no-op (byte and mtime preservation on {}) is not reachable;
    # the semantic no-op tests below cover byte preservation instead.
    def test_empty_values_is_noop(self):
        tmp, root, _manifest = self._project("[recipes.x]\nenabled = true\n")
        result = self._configure(root, "x", "--json")
        self.assertEqual(result.returncode, 2)
        self.assertIn("one of --inspect or --set is required", result.stderr)

    def test_quoted_key_id(self):
        tmp, root, manifest = self._project('[project]\nname = "p"\n')
        result = self._configure(root, "my.recipe", "--set", "base_branch=main")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn('[recipes."my.recipe"]', text)
        self.assertIn('[recipes."my.recipe".config]', text)
        data = tomllib.loads(text)
        self.assertEqual(data["recipes"]["my.recipe"]["config"]["base_branch"], "main")

    def test_inline_comment_survives_replacement(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'branch = "main"  # team decision\n'
        )
        result = self._configure(root, "x", "--set", "branch=develop")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn('branch = "develop"  # team decision', text)

    def test_hash_inside_string_is_not_comment(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'url = "https://example.test/#fragment"\n'
            'branch = "main"  # keep\n'
        )
        result = self._configure(root, "x", "--set", "branch=develop")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn('url = "https://example.test/#fragment"', text)
        self.assertIn('branch = "develop"  # keep', text)

    def test_semantic_noop_preserves_original_bytes(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            "branch='main' # formatted\n"
        )
        before = manifest.read_bytes()
        result = self._configure(root, "x", "--set", "branch=main")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(manifest.read_bytes(), before)

    _RECONCILE_INLINE = (
        '[recipes.x]\nenabled = true\n\n[recipes.x.config]\n'
        'board_id = "b1"\n'
        'reconcile = { scope_field = "board_id", max_age_seconds = 900, '
        'expectations = [{ event = "delivery", property = "list", '
        'config_field = "default_list" }] }  # recipe-owned\n'
    )

    def test_dotted_update_of_inline_table_preserves_siblings_and_comment(self):
        tmp, root, manifest = self._project(self._RECONCILE_INLINE)
        result = self._configure(root, "x", "--set", "reconcile.max_age_seconds=1200")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn("# recipe-owned", text)
        self.assertIn('board_id = "b1"', text)
        data = tomllib.loads(text)
        cfg = data["recipes"]["x"]["config"]
        self.assertEqual(cfg["reconcile"]["max_age_seconds"], 1200)
        self.assertEqual(cfg["reconcile"]["scope_field"], "board_id")
        self.assertEqual(
            cfg["reconcile"]["expectations"][0]["config_field"], "default_list"
        )

    def test_dotted_nested_structured_value_assignment(self):
        tmp, root, manifest = self._project(self._RECONCILE_INLINE)
        replacement = [
            {"event": "merge", "property": "list", "config_field": "done_list"}
        ]
        result = self._configure(
            root, "x",
            "--set", 'reconcile.expectations=[{event="merge",property="list",config_field="done_list"}]',
        )
        self.assertEqual(result.returncode, 0)
        data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        self.assertEqual(
            data["recipes"]["x"]["config"]["reconcile"]["expectations"], replacement
        )

    def test_dotted_update_of_header_table_replaces_leaf_line(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'board_id = "b1"\n\n'
            "[recipes.x.config.reconcile]\n"
            'scope_field = "board_id"\n'
            "max_age_seconds = 900  # keep\n\n"
            "[[recipes.x.config.reconcile.expectations]]\n"
            'event = "delivery"\nproperty = "list"\nconfig_field = "default_list"\n'
        )
        result = self._configure(root, "x", "--set", "reconcile.max_age_seconds=1200")
        self.assertEqual(result.returncode, 0)
        text = manifest.read_text(encoding="utf-8")
        self.assertIn("max_age_seconds = 1200  # keep", text)
        self.assertIn('scope_field = "board_id"', text)
        data = tomllib.loads(text)
        cfg = data["recipes"]["x"]["config"]["reconcile"]
        self.assertEqual(cfg["max_age_seconds"], 1200)
        self.assertEqual(len(cfg["expectations"]), 1)

    def test_dotted_update_creates_inline_table_when_absent(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'board_id = "b1"\n'
        )
        result = self._configure(root, "x", "--set", "reconcile.max_age_seconds=1200")
        self.assertEqual(result.returncode, 0)
        cfg = tomllib.loads(manifest.read_text(encoding="utf-8"))["recipes"]["x"]["config"]
        self.assertEqual(cfg["reconcile"]["max_age_seconds"], 1200)
        self.assertEqual(cfg["board_id"], "b1")

    def test_dotted_and_whole_table_for_same_root_is_rejected(self):
        tmp, root, manifest = self._project(self._RECONCILE_INLINE)
        before = manifest.read_bytes()
        result = self._configure(
            root,
            "x",
            "--set", 'reconcile={scope_field="board_id"}',
            "--set", "reconcile.max_age_seconds=1200",
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(manifest.read_bytes(), before)

    def test_dotted_leaf_colliding_with_subtable_is_rejected_without_rewrite(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'board_id = "b1"\n\n'
            "[recipes.x.config.reconcile]\n"
            'scope_field = "board_id"\n'
            "[[recipes.x.config.reconcile.expectations]]\n"
            'event = "delivery"\nproperty = "list"\nconfig_field = "default_list"\n'
        )
        before = manifest.read_bytes()
        result = self._configure(
            root,
            "x",
            "--set",
            'reconcile.expectations=[{event="delivery",property="list",config_field="other"}]',
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(manifest.read_bytes(), before)

    def test_dotted_semantic_noop_preserves_original_bytes(self):
        tmp, root, manifest = self._project(self._RECONCILE_INLINE)
        before = manifest.read_bytes()
        result = self._configure(root, "x", "--set", "reconcile.max_age_seconds=900")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(manifest.read_bytes(), before)

    def test_multiline_value_is_rejected_without_rewrite(self):
        tmp, root, manifest = self._project(
            "[recipes.x]\nenabled = true\n\n[recipes.x.config]\n"
            'branches = [\n  "main",\n  "develop",\n]\n'
            'other = "keep"\n'
        )
        before = manifest.read_bytes()
        result = self._configure(root, "x", "--set", 'branches=["release"]', "--json")
        self.assertEqual(result.returncode, 1)
        self.assertIn("multiline", result.stdout)
        self.assertEqual(manifest.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
