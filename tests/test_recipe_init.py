"""Black-box tests for `ai-specs recipe init` (converted from recipe-init.py internals)."""
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, snapshot, tree_diff  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

TRACKER_RECIPE_BODY = (
    '[recipe]\nid = "tracker"\nname = "Tracker"\ndescription = "Tracker setup"\nversion = "1.0"\n\n'
    '[init]\nprompt = "init.md"\ndescription = "Configure tracker"\nneeds_manifest = true\nneeds_mcp = ["trello", "missing-mcp"]\n\n'
    '[config.board_id]\nrequired = true\ntype = "string"\n\n'
    '[config.timeout]\nrequired = false\ntype = "integer"\ndefault = 30\n\n'
    '[[provides.mcp]]\nid = "trello"\ncommand = "npx"\nargs = ["-y", "@recipe/trello"]\nenv = { API_TOKEN = "recipe-secret" }\n\n'
    '[[provides.templates]]\nsource = "templates/mapping.toml"\ntarget = "ai-specs/trello-mapping.toml"\ncondition = "not_exists"\n'
)

TRACKER_INIT_MD = "# Tracker init\nChoose a board and list mapping.\n"
TRACKER_MAPPING_TOML = "[mapping]\n"


class RecipeInitTests(unittest.TestCase):
    def _make_project(self, *, installed: bool = True, config: str = "", recipe_extra: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        manifest = (
            '[project]\nname = "fixture"\n\n'
            '[agents]\nenabled = ["claude", "opencode"]\n\n'
            '[mcp.trello]\ncommand = "npx"\nargs = ["-y", "@trello/mcp"]\nenv = { TRELLO_TOKEN = "$TRELLO_TOKEN", literal_secret = "super-secret" }\nheaders = { Authorization = "literal-auth" }\n\n'
        )
        if installed:
            manifest += '[recipes.tracker]\nenabled = true\nversion = "1.0"\n'
            if config:
                manifest += '\n[recipes.tracker.config]\n' + config + '\n'
        (ai_specs / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        recipe_dir = root / "catalog" / "recipes" / "tracker"
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "templates").mkdir()
        (recipe_dir / "init.md").write_text(TRACKER_INIT_MD, encoding="utf-8")
        (recipe_dir / "templates" / "mapping.toml").write_text(TRACKER_MAPPING_TOML, encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(TRACKER_RECIPE_BODY + recipe_extra, encoding="utf-8")
        return root

    def _make_cli_home(self, recipe_id: str = "tracker", recipe_extra: str = "") -> Path:
        """Isolated install root with an EMPTY catalog plus one tracker recipe."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name), catalog=False)
        recipe_dir = home / "catalog" / "recipes" / recipe_id
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "templates").mkdir()
        (recipe_dir / "init.md").write_text(TRACKER_INIT_MD, encoding="utf-8")
        (recipe_dir / "templates" / "mapping.toml").write_text(TRACKER_MAPPING_TOML, encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(TRACKER_RECIPE_BODY + recipe_extra, encoding="utf-8")
        return home

    def _init(self, project: Path, recipe_id: str, home: Path | None = None, **kwargs):
        """Run `ai-specs recipe init <id> <project>`; cli_home=None uses a fresh isolated home."""
        return invoke(project, "recipe", "init", recipe_id, cli_home=home, **kwargs)

    def test_init_brief_for_installed_recipe_is_read_only_and_context_rich(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("# Recipe Init Brief", result.stdout)

    def test_build_init_brief_reports_existing_config_and_does_not_duplicate_keys(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("Install state: installed", result.stdout)
        self.assertIn("Existing config keys: board_id", result.stdout)
        self.assertIn("Update existing key `board_id`", result.stdout)
        self.assertNotIn("board_id =", result.stdout)

    def test_available_recipe_before_add_succeeds_with_reviewable_manifest_guidance(self):
        root = self._make_project(installed=False)
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("Install state: available (not installed)", result.stdout)
        self.assertIn("[recipes.tracker]", result.stdout)

    def test_missing_recipe_fails(self):
        root = self._make_project()
        home = self._make_cli_home()
        result = self._init(root, "missing", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("Recipe 'missing' not found", result.stderr)

    def test_init_rejects_internal_test_recipe(self):
        root = self._make_project()
        home = self._make_cli_home()
        result = self._init(root, "test-fixture", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("internal test fixture", result.stderr)

    def test_uninitialized_project_fails_without_mutating(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result = self._init(root, "tracker")
            self.assertEqual(result.returncode, 1)
            self.assertIn("Project not initialized", result.stderr)
            self.assertEqual(list(root.iterdir()), [])

    def test_recipe_without_init_workflow_fails(self):
        root = self._make_project(recipe_extra="")
        home = self._make_cli_home()
        recipe_toml = home / "catalog" / "recipes" / "tracker" / "recipe.toml"
        text = recipe_toml.read_text(encoding="utf-8")
        recipe_toml.write_text(text.replace('[init]\nprompt = "init.md"\ndescription = "Configure tracker"\nneeds_manifest = true\nneeds_mcp = ["trello", "missing-mcp"]\n\n', ""), encoding="utf-8")
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("has no init workflow", result.stderr)

    def test_mcp_discovery_redacts_secrets_and_mentions_manifest_precedence(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("trello: configured", result.stdout)
        self.assertIn("missing-mcp: missing", result.stdout)
        self.assertIn("${TRELLO_TOKEN}", result.stdout)
        self.assertIn("literal_secret: ***", result.stdout)
        self.assertIn("Authorization: ***", result.stdout)
        self.assertIn("API_TOKEN: ***", result.stdout)
        self.assertNotIn("super-secret", result.stdout)
        self.assertNotIn("literal-auth", result.stdout)
        self.assertNotIn("recipe-secret", result.stdout)
        self.assertIn("project manifest values take precedence", result.stdout)

    def test_unknown_config_keys_are_reported_without_claiming_sync_success(self):
        root = self._make_project(config='board_id = "abc123"\nunknown = "value"\n')
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("Unknown config keys: unknown", result.stdout)
        self.assertIn("sync still validates", result.stdout)

    def test_template_preview_reports_existing_targets_without_overwrite(self):
        root = self._make_project()
        home = self._make_cli_home()
        target = root / "ai-specs" / "trello-mapping.toml"
        target.write_text("existing", encoding="utf-8")
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("ai-specs/trello-mapping.toml", result.stdout)
        self.assertIn("exists", result.stdout)
        self.assertIn("review update/skip/diff", result.stdout)
        self.assertEqual(target.read_text(encoding="utf-8"), "existing")

    def test_cli_dispatch_success_and_usage_errors(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("# Recipe Init Brief", result.stdout)
        self.assertEqual(result.stderr, "")

        missing_id = invoke(root, "recipe", "init", cli_home=home, append_root=False)
        self.assertEqual(missing_id.returncode, 2)
        self.assertIn("missing recipe id", missing_id.stderr)

    def test_init_does_not_sync_or_materialize(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home()
        before = snapshot(root)
        result = self._init(root, "tracker", home)
        diff = tree_diff(before, snapshot(root))
        self.assertEqual(result.returncode, 0)
        self.assertIn("No files were changed", result.stdout)
        self.assertEqual(diff["created"], [])
        self.assertEqual(diff["deleted"], [])
        self.assertEqual(diff["modified"], [])
        self.assertFalse((root / "ai-specs" / ".recipe").exists())
        self.assertFalse((root / "ai-specs" / ".tmp" / "recipe-mcp.json").exists())

    def test_trello_recipe_init_uses_cli_catalog_when_project_has_no_local_catalog(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            ai_specs = root / "ai-specs"
            ai_specs.mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                '[project]\nname = "fixture"\n\n[agents]\nenabled = ["claude"]\n\n[mcp.trello]\ncommand = "npx"\nargs = ["-y", "@trello/mcp"]\n',
                encoding="utf-8",
            )
            # cli_home=None builds an isolated install root whose catalog is
            # the CLI's own repo catalog; the project has no local catalog.
            result = self._init(root, "trello-mcp-workflow")
            self.assertEqual(result.returncode, 0)
            self.assertIn("- ID: trello-mcp-workflow", result.stdout)
            self.assertIn("- Install state: available (not installed)", result.stdout)
            self.assertIn("Configure Trello board and list mappings before sync", result.stdout)
            self.assertIn("board_id", result.stdout)
            self.assertIn("trello: configured", result.stdout)
            self.assertIn("# Recipe Init Contract", result.stdout)

    def test_init_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        root = self._make_project(config='board_id = "abc123"\n')
        local_recipe_toml = root / "catalog" / "recipes" / "tracker" / "recipe.toml"
        text = local_recipe_toml.read_text(encoding="utf-8")
        local_recipe_toml.write_text(text.replace('name = "Tracker"', 'name = "Local Tracker"').replace('version = "1.0"', 'version = "9.9"'), encoding="utf-8")
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("- Name: Tracker", result.stdout)
        self.assertIn("- Version: 1.0", result.stdout)
        self.assertNotIn("Local Tracker", result.stdout)
        self.assertNotIn("9.9", result.stdout)

    def test_trello_recipe_init_uses_cli_catalog_when_project_has_no_local_catalog_v2(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            ai_specs = root / "ai-specs"
            ai_specs.mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                '[project]\nname = "fixture"\n\n[agents]\nenabled = ["claude"]\n\n[mcp.trello]\ncommand = "npx"\nargs = ["-y", "@trello/mcp"]\n',
                encoding="utf-8",
            )
            # cli_home=None builds an isolated install root whose catalog is
            # the CLI's own repo catalog; the project has no local catalog.
            result = self._init(root, "trello-mcp-workflow")
            self.assertEqual(result.returncode, 0)
            self.assertIn("- ID: trello-mcp-workflow", result.stdout)
            self.assertIn("- Install state: available (not installed)", result.stdout)
            self.assertIn("Configure Trello board and list mappings before sync", result.stdout)
            self.assertIn("board_id", result.stdout)
            self.assertIn("trello: configured", result.stdout)
            self.assertIn("# Recipe Init Contract", result.stdout)

    def test_init_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        root = self._make_project(config='board_id = "abc123"\n')
        local_recipe_toml = root / "catalog" / "recipes" / "tracker" / "recipe.toml"
        text = local_recipe_toml.read_text(encoding="utf-8")
        local_recipe_toml.write_text(text.replace('name = "Tracker"', 'name = "Local Tracker"').replace('version = "1.0"', 'version = "9.9"'), encoding="utf-8")
        home = self._make_cli_home()
        result = self._init(root, "tracker", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("- Name: Tracker", result.stdout)
        self.assertIn("- Version: 1.0", result.stdout)
        self.assertNotIn("Local Tracker", result.stdout)
        self.assertNotIn("9.9", result.stdout)


if __name__ == "__main__":
    unittest.main()
