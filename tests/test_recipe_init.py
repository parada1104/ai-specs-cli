import importlib.util
import contextlib
import io
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RECIPE_INIT_PATH = ROOT / "lib" / "_internal" / "recipe-init.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeInitTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_INIT_PATH, "recipe_init_internal")

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
        (recipe_dir / "docs").mkdir()
        (recipe_dir / "templates").mkdir()
        (recipe_dir / "docs" / "init.md").write_text("# Tracker init\nChoose a board and list mapping.\n", encoding="utf-8")
        (recipe_dir / "templates" / "mapping.toml").write_text("[mapping]\n", encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(
            '[recipe]\nid = "tracker"\nname = "Tracker"\ndescription = "Tracker setup"\nversion = "1.0"\n\n'
            '[init]\nprompt = "docs/init.md"\ndescription = "Configure tracker"\nneeds_manifest = true\nneeds_mcp = ["trello", "missing-mcp"]\n\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[config.timeout]\nrequired = false\ntype = "integer"\ndefault = 30\n\n'
            '[[provides.mcp]]\nid = "trello"\ncommand = "npx"\nargs = ["-y", "@recipe/trello"]\nenv = { API_TOKEN = "recipe-secret" }\n\n'
            '[[provides.templates]]\nsource = "templates/mapping.toml"\ntarget = "ai-specs/trello-mapping.toml"\ncondition = "not_exists"\n'
            + recipe_extra,
            encoding="utf-8",
        )
        return root

    def _make_cli_home(self, recipe_id: str = "tracker", recipe_extra: str = "", include_runtime: bool = False) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name)
        if include_runtime:
            shutil.copytree(ROOT / "lib", home / "lib")
        recipe_dir = home / "catalog" / "recipes" / recipe_id
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "docs").mkdir()
        (recipe_dir / "templates").mkdir()
        (recipe_dir / "docs" / "init.md").write_text("# Tracker init\nChoose a board and list mapping.\n", encoding="utf-8")
        (recipe_dir / "templates" / "mapping.toml").write_text("[mapping]\n", encoding="utf-8")
        (recipe_dir / "recipe.toml").write_text(
            '[recipe]\nid = "tracker"\nname = "Tracker"\ndescription = "Tracker setup"\nversion = "1.0"\n\n'
            '[init]\nprompt = "docs/init.md"\ndescription = "Configure tracker"\nneeds_manifest = true\nneeds_mcp = ["trello", "missing-mcp"]\n\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n\n'
            '[config.timeout]\nrequired = false\ntype = "integer"\ndefault = 30\n\n'
            '[[provides.mcp]]\nid = "trello"\ncommand = "npx"\nargs = ["-y", "@recipe/trello"]\nenv = { API_TOKEN = "recipe-secret" }\n\n'
            '[[provides.templates]]\nsource = "templates/mapping.toml"\ntarget = "ai-specs/trello-mapping.toml"\ncondition = "not_exists"\n'
            + recipe_extra,
            encoding="utf-8",
        )
        return home

    def _set_ai_specs_home(self, home: Path) -> None:
        old_home = os.environ.get("AI_SPECS_HOME")
        os.environ["AI_SPECS_HOME"] = str(home)

        def restore() -> None:
            if old_home is None:
                os.environ.pop("AI_SPECS_HOME", None)
            else:
                os.environ["AI_SPECS_HOME"] = old_home

        self.addCleanup(restore)

    def test_init_brief_for_installed_recipe_is_read_only_and_context_rich(self):
        root = self._make_project(config='board_id = "abc123"\n')
        self._set_ai_specs_home(self._make_cli_home())
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            rc = self.mod.init_recipe(root, "tracker")
        self.assertEqual(rc, 0)
        self.assertIn("# Recipe Init Brief", out.getvalue())

    def test_build_init_brief_reports_existing_config_and_does_not_duplicate_keys(self):
        root = self._make_project(config='board_id = "abc123"\n')
        self._set_ai_specs_home(self._make_cli_home())
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("Install state: installed", brief)
        self.assertIn("Existing config keys: board_id", brief)
        self.assertIn("Update existing key `board_id`", brief)
        self.assertNotIn("board_id =", brief)

    def test_available_recipe_before_add_succeeds_with_reviewable_manifest_guidance(self):
        root = self._make_project(installed=False)
        self._set_ai_specs_home(self._make_cli_home())
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("Install state: available (not installed)", brief)
        self.assertIn("[recipes.tracker]", brief)

    def test_missing_recipe_fails(self):
        root = self._make_project()
        self._set_ai_specs_home(self._make_cli_home())
        with self.assertRaises(self.mod.RecipeInitError) as ctx:
            self.mod.build_init_brief(root, "missing")
        self.assertIn("Recipe 'missing' no encontrada", str(ctx.exception))

    def test_uninitialized_project_fails_without_mutating(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with self.assertRaises(self.mod.RecipeInitError) as ctx:
                self.mod.build_init_brief(root, "tracker")
            self.assertIn("Proyecto no inicializado", str(ctx.exception))
            self.assertEqual(list(root.iterdir()), [])

    def test_recipe_without_init_workflow_fails(self):
        root = self._make_project(recipe_extra="")
        home = self._make_cli_home()
        self._set_ai_specs_home(home)
        recipe_toml = home / "catalog" / "recipes" / "tracker" / "recipe.toml"
        text = recipe_toml.read_text(encoding="utf-8")
        recipe_toml.write_text(text.replace('[init]\nprompt = "docs/init.md"\ndescription = "Configure tracker"\nneeds_manifest = true\nneeds_mcp = ["trello", "missing-mcp"]\n\n', ""), encoding="utf-8")
        with self.assertRaises(self.mod.RecipeInitError) as ctx:
            self.mod.build_init_brief(root, "tracker")
        self.assertIn("has no init workflow", str(ctx.exception))

    def test_mcp_discovery_redacts_secrets_and_mentions_manifest_precedence(self):
        root = self._make_project(config='board_id = "abc123"\n')
        self._set_ai_specs_home(self._make_cli_home())
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("trello: configured", brief)
        self.assertIn("missing-mcp: missing", brief)
        self.assertIn("${TRELLO_TOKEN}", brief)
        self.assertIn("literal_secret: ***", brief)
        self.assertIn("Authorization: ***", brief)
        self.assertIn("API_TOKEN: ***", brief)
        self.assertNotIn("super-secret", brief)
        self.assertNotIn("literal-auth", brief)
        self.assertNotIn("recipe-secret", brief)
        self.assertIn("project manifest values take precedence", brief)

    def test_unknown_config_keys_are_reported_without_claiming_sync_success(self):
        root = self._make_project(config='board_id = "abc123"\nunknown = "value"\n')
        self._set_ai_specs_home(self._make_cli_home())
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("Unknown config keys: unknown", brief)
        self.assertIn("sync still validates", brief)

    def test_template_preview_reports_existing_targets_without_overwrite(self):
        root = self._make_project()
        self._set_ai_specs_home(self._make_cli_home())
        target = root / "ai-specs" / "trello-mapping.toml"
        target.write_text("existing", encoding="utf-8")
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("ai-specs/trello-mapping.toml", brief)
        self.assertIn("exists", brief)
        self.assertIn("review update/skip/diff", brief)
        self.assertEqual(target.read_text(encoding="utf-8"), "existing")

    def test_cli_dispatch_success_and_usage_errors(self):
        root = self._make_project(config='board_id = "abc123"\n')
        home = self._make_cli_home(include_runtime=True)
        proc = subprocess.run(
            [str(CLI), "recipe", "init", "tracker", str(root)],
            capture_output=True,
            text=True,
            check=False,
            env={**os.environ, "AI_SPECS_HOME": str(home)},
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("# Recipe Init Brief", proc.stdout)
        self.assertEqual(proc.stderr, "")

        missing_id = subprocess.run(
            [str(CLI), "recipe", "init"],
            capture_output=True,
            text=True,
            check=False,
            env={**os.environ, "AI_SPECS_HOME": str(home)},
        )
        self.assertEqual(missing_id.returncode, 2)
        self.assertIn("missing recipe id", missing_id.stderr)

    def test_init_does_not_sync_or_materialize(self):
        root = self._make_project(config='board_id = "abc123"\n')
        self._set_ai_specs_home(self._make_cli_home())
        before = sorted(p.relative_to(root).as_posix() for p in root.rglob("*"))
        brief = self.mod.build_init_brief(root, "tracker")
        after = sorted(p.relative_to(root).as_posix() for p in root.rglob("*"))
        self.assertIn("No files were changed", brief)
        self.assertEqual(after, before)
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
            self._set_ai_specs_home(ROOT)
            brief = self.mod.build_init_brief(root, "trello-mcp-workflow")
            self.assertIn("- ID: trello-mcp-workflow", brief)
            self.assertIn("- Install state: available (not installed)", brief)
            self.assertIn("Configure Trello board and list mappings before sync", brief)
            self.assertIn("board_id", brief)
            self.assertIn("trello: configured", brief)
            self.assertIn("# Trello Recipe Init", brief)

    def test_init_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        root = self._make_project(config='board_id = "abc123"\n')
        local_recipe_toml = root / "catalog" / "recipes" / "tracker" / "recipe.toml"
        text = local_recipe_toml.read_text(encoding="utf-8")
        local_recipe_toml.write_text(text.replace('name = "Tracker"', 'name = "Local Tracker"').replace('version = "1.0"', 'version = "9.9"'), encoding="utf-8")
        self._set_ai_specs_home(self._make_cli_home())
        brief = self.mod.build_init_brief(root, "tracker")
        self.assertIn("- Name: Tracker", brief)
        self.assertIn("- Version: 1.0", brief)
        self.assertNotIn("Local Tracker", brief)
        self.assertNotIn("9.9", brief)


if __name__ == "__main__":
    unittest.main()
