"""Black-box tests for recipe-read behavior, driving `bin/ai-specs <verb>`.

Converted from direct recipe-read.py / recipe_schema.py imports to subprocess
invocations (see openspec/changes/blackbox-test-conversion). The process
boundary is `bin/ai-specs sync` / `bin/ai-specs recipe list` /
`bin/ai-specs recipe init`, and assertions target exit codes, emitted file
tree, stdout and stderr — including the current (FROZEN) behavior.
"""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402
from _fixture_catalog import populate_catalog, unit_catalog  # noqa: E402

# Coupled-only leftovers (see RecipeReadCoupledTests): recipe-read and
# recipe_schema TDD fields that no CLI surface prints.
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"
CATALOG = unit_catalog()


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def recipe_toml(rid: str, extra: str = "", *, name: str = "Test", version: str = "1.0") -> str:
    return (
        f'[recipe]\n'
        f'id = "{rid}"\n'
        f'name = "{name}"\n'
        f'description = "D"\n'
        f'version = "{version}"\n'
        f'{extra}'
    )


class RecipeReadTests(unittest.TestCase):
    """Black-box conversions driving bin/ai-specs."""

    def _cli_home(self, recipes: dict[str, str]) -> Path:
        """isolated_home whose catalog merges public recipes + custom tomls."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        catalog = home / "catalog"
        catalog.unlink()
        recipes_dir = catalog / "recipes"
        recipes_dir.mkdir(parents=True)
        for rid, toml in recipes.items():
            recipe_dir = recipes_dir / rid
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(toml)
        populate_catalog(recipes_dir, include_fixtures=False)
        return home

    def _make_project(self, recipe_section: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + recipe_section
        )
        return root

    def _sync(self, root: Path, home: Path):
        return invoke(root, "sync", cli_home=home)

    def _recipe_skill_path(self, root: Path, home: Path, rid: str, skill_id: str) -> Path:
        return cache_project_dir(root, home) / ".recipe" / rid / "skills" / skill_id

    def test_reads_valid_recipe(self):
        rid, skill_id = "sample", "test-skill"
        home = self._cli_home(
            {rid: recipe_toml(rid, "[[provides.skills]]\nid = \"test-skill\"\nsource = \"bundled\"\n",
                              name="Sample Recipe", version="1.0.0")}
        )
        skill_src = home / "catalog" / "recipes" / rid / "skills" / "test-skill" / "SKILL.md"
        skill_src.parent.mkdir(parents=True)
        skill_src.write_text("# sample skill\n")
        root = self._make_project(f"[recipes.{rid}]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        materialized = self._recipe_skill_path(root, home, rid, "test-skill") / "SKILL.md"
        self.assertTrue(materialized.is_file())
        self.assertEqual(materialized.read_text(), skill_src.read_text())
        listing = invoke(root, "recipe", "list", cli_home=home)
        self.assertEqual(listing.returncode, 0, listing.stderr)
        self.assertIn(rid, listing.stdout)
        self.assertIn("1.0.0", listing.stdout)
        self.assertIn("Sample Recipe", listing.stdout)

    def test_fails_on_missing_recipe_dir(self):
        home = self._cli_home({})
        root = self._make_project("[recipes.nonexistent]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("recipe directory not found", result.stderr)
        self.assertIn("nonexistent", result.stderr)

    def test_fails_on_missing_required_field(self):
        home = self._cli_home({"bad-recipe": '[recipe]\nname = "Bad"\ndescription = "Missing id and version"\n'})
        root = self._make_project("[recipes.bad-recipe]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing or invalid required field", result.stderr)

    def test_cli_outputs_recipe_identity(self):
        rid = "sample"
        home = self._cli_home({rid: recipe_toml(rid, name="Sample Recipe", version="1.0.0")})
        root = self._make_project()
        result = invoke(root, "recipe", "list", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertIn(rid, result.stdout)
        self.assertIn("1.0.0", result.stdout)

    # --- V2 schema tests through the CLI --------------------------------

    def test_v2_duplicate_capability_fails(self):
        toml = recipe_toml("dup-cap", "[[capabilities]]\nid = \"tracker\"\n"
                           "[[capabilities]]\nid = \"tracker\"\n")
        home = self._cli_home({"dup-cap": toml})
        root = self._make_project("[recipes.dup-cap]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("duplicate capability id", result.stderr)
        self.assertIn("tracker", result.stderr)

    def test_v2_missing_capability_id_fails(self):
        toml = recipe_toml("bad-cap", "[[capabilities]]\nid = \"\"\n")
        home = self._cli_home({"bad-cap": toml})
        root = self._make_project("[recipes.bad-cap]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing or invalid required field", result.stderr)
        self.assertIn("'id'", result.stderr)

    def test_v2_hook_parsing_observable_via_sync_success(self):
        # TRIAGE: the parsed [[hooks]] event/action VALUES ('on-sync' /
        # 'validate-config') never render on any CLI surface — `bin/ai-specs
        # sync` prints materialization steps but no hook rows, `recipe list`
        # prints id/name/version, and `recipe init` prints only the init
        # workflow. The parse contract is therefore asserted as: a recipe
        # declaring that exact hook pair is accepted and sync exits 0 (a
        # missing/unknown hook field would be rejected at load time, rc 1).
        toml = recipe_toml("hook-recipe", '[[hooks]]\nevent = "on-sync"\naction = "validate-config"\n')
        home = self._cli_home({"hook-recipe": toml})
        root = self._make_project("[recipes.hook-recipe]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_v2_missing_hook_event_fails(self):
        toml = recipe_toml("bad-hook", '[[hooks]]\naction = "validate-config"\n')
        home = self._cli_home({"bad-hook": toml})
        root = self._make_project("[recipes.bad-hook]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing or invalid required field 'event'", result.stderr)

    def test_v2_missing_hook_action_fails(self):
        toml = recipe_toml("bad-hook", '[[hooks]]\nevent = "on-sync"\n')
        home = self._cli_home({"bad-hook": toml})
        root = self._make_project("[recipes.bad-hook]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing or invalid required field 'action'", result.stderr)

    def test_v2_config_schema_parsing(self):
        toml = recipe_toml(
            "cfg-recipe",
            "[init]\nprompt = \"init.md\"\n"
            "[config.timeout]\nrequired = false\ntype = \"integer\"\ndefault = 30\n"
            "[config.board_id]\nrequired = true\ntype = \"string\"\n",
        )
        home = self._cli_home({"cfg-recipe": toml})
        (home / "catalog" / "recipes" / "cfg-recipe" / "init.md").write_text("# init\n")
        root = self._make_project()
        result = invoke(root, "recipe", "init", "cfg-recipe", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("`timeout` is optional and defaults to `30`", result.stdout)
        self.assertIn("Add required `board_id`", result.stdout)

    def test_v2_invalid_config_field_fails(self):
        toml = recipe_toml("bad-cfg", "[config.timeout]\nrequired = \"not-a-bool\"\n")
        home = self._cli_home({"bad-cfg": toml})
        root = self._make_project("[recipes.bad-cfg]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing or invalid 'required'", result.stderr)

    # --- Init workflow tests through the CLI ----------------------------

    def _write_init_recipe(self, home: Path, rid: str, init_body: str) -> None:
        recipe_dir = home / "catalog" / "recipes" / rid
        recipe_dir.mkdir()
        (recipe_dir / "init.md").write_text("# Init prompt\nConfigure this recipe.\n", encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(
            recipe_toml(rid, init_body, name="Init Recipe"), encoding="utf-8"
        )

    def test_init_workflow_parsing(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home,
            "init-recipe",
            '[init]\nprompt = "init.md"\ndescription = "Configure"\n'
            'needs_manifest = true\nneeds_mcp = ["trello", "openmemory"]\n',
        )
        root = self._make_project()
        result = invoke(root, "recipe", "init", "init-recipe", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("- Prompt: init.md", result.stdout)
        self.assertIn("- Description: Configure", result.stdout)
        self.assertIn("- Needs manifest: true", result.stdout)
        self.assertIn("- Needs MCP: trello, openmemory", result.stdout)

    def test_init_metadata_in_brief(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "init-recipe", '[init]\nprompt = "init.md"\nneeds_mcp = ["trello"]\n'
        )
        root = self._make_project()
        result = invoke(root, "recipe", "init", "init-recipe", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("- Prompt: init.md", result.stdout)
        self.assertIn("- Description: (none)", result.stdout)
        self.assertIn("- Needs manifest: false", result.stdout)
        self.assertIn("- Needs MCP: trello", result.stdout)

    def test_recipe_without_init_brief_fails(self):
        home = self._cli_home({"no-init": recipe_toml("no-init", name="No Init")})
        root = self._make_project()
        result = invoke(root, "recipe", "init", "no-init", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no init workflow", result.stderr)

    def test_init_missing_prompt_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\ndescription = "Missing prompt"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[init].prompt", result.stderr)

    def test_init_unknown_field_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "init.md"\nextra = "nope"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported init field 'extra'", result.stderr)

    def test_init_invalid_needs_mcp_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "init.md"\nneeds_mcp = ["trello", ""]\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("needs_mcp", result.stderr)
        self.assertIn("expected non-empty string", result.stderr)

    def test_init_absolute_prompt_path_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "/tmp/init.md"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must be relative", result.stderr)

    def test_init_parent_traversal_prompt_path_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "../init.md"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("inside the recipe directory", result.stderr)

    def test_init_directory_prompt_path_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "docs"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("init prompt file not found", result.stderr)
        self.assertIn("docs", result.stderr)

    def test_init_missing_prompt_file_fails(self):
        home = self._cli_home({})
        self._write_init_recipe(
            home, "bad-init", '[init]\nprompt = "docs/missing.md"\n'
        )
        root = self._make_project("[recipes.bad-init]\nenabled = true\n")
        result = self._sync(root, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("init prompt file not found", result.stderr)
        self.assertIn("docs/missing.md", result.stderr)


class RecipeReadCoupledTests(unittest.TestCase):
    """Field-level parse assertions with no observable CLI equivalent.

    Each # TRIAGE: marker names the assertion, the exact command run, and what
    that surface did not expose.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_READ_PATH, "recipe_read_coupled")

    def test_v1_recipe_has_empty_v2_fields(self):
        data = self.mod.read_recipe(CATALOG, "test-fixture")
        # TRIAGE: run `bin/ai-specs sync` and `bin/ai-specs recipe list` on
        # test-fixture; neither surface emits capabilities/hooks/config
        # presence or emptiness — only materialization outcomes and the
        # id/name/version columns print, so the empty-list/default-None v2
        # fields stay asserted on the parsed dataclass.
        self.assertEqual(data.capabilities, [])
        self.assertEqual(data.hooks, [])
        self.assertEqual(data.config_schema.fields, {})
        self.assertIsNone(data.init)

    def test_v2_capability_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp) / "cap-recipe"
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                '[recipe]\nid = "cap-recipe"\nname = "Cap"\ndescription = "D"\nversion = "1.0"\n'
                '[[capabilities]]\nid = "tracker"\n'
                '[[capabilities]]\nid = "canonical-memory"\n'
            )
            data = self.mod.read_recipe(Path(tmp), "cap-recipe")
            # TRIAGE: ran `bin/ai-specs sync` and `bin/ai-specs recipe list`
            # with cap-recipe enabled; both succeed (rc 0) but print only
            # aggregate/materialization outcome — the capability ids
            # 'tracker'/'canonical-memory' never appear on any CLI surface.
            self.assertEqual(len(data.capabilities), 2)
            self.assertEqual(data.capabilities[0].id, "tracker")
            self.assertEqual(data.capabilities[1].id, "canonical-memory")


if __name__ == "__main__":
    unittest.main()