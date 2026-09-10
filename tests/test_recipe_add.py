import contextlib
import importlib.util
import io
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from unittest import mock
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_ADD_PATH = ROOT / "lib" / "_internal" / "recipe-add.py"
RECIPE_READ_PATH = ROOT / "lib" / "_internal" / "recipe-read.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
TOML_READ_PATH = ROOT / "lib" / "_internal" / "toml-read.py"
CATALOG = ROOT / "catalog" / "recipes"


class _TtyStringIO(io.StringIO):
    """Captured stdout that still reports itself as a TTY.

    ``redirect_stdout`` replaces ``sys.stdout``, so a plain StringIO would make
    TTY-gated code paths look non-interactive. Reporting ``isatty() == True``
    keeps the interactive path under test while still capturing the output.
    """

    def isatty(self) -> bool:
        return True


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeAddTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_ADD_PATH, "recipe_add_internal")

    def _make_project(self, manifest_content: str, catalog_recipes: dict | None = None) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs_dir = project / "ai-specs"
        ai_specs_dir.mkdir()
        (ai_specs_dir / "ai-specs.toml").write_text(manifest_content, encoding="utf-8")
        if catalog_recipes:
            catalog_dir = project / "catalog" / "recipes"
            catalog_dir.mkdir(parents=True)
            for rid, content in catalog_recipes.items():
                rdir = catalog_dir / rid
                rdir.mkdir()
                (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        return project

    def _make_cli_home(self, catalog_recipes: dict[str, str]) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name)
        catalog_dir = home / "catalog" / "recipes"
        catalog_dir.mkdir(parents=True)
        for rid, content in catalog_recipes.items():
            rdir = catalog_dir / rid
            rdir.mkdir()
            (rdir / "recipe.toml").write_text(content, encoding="utf-8")
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

    def test_add_appends_recipe_without_version(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "2.1.0"\n'
        )
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.my-recipe]", manifest_text)
        self.assertIn("enabled = true", manifest_text)
        self.assertNotIn("version =", manifest_text)

    def test_add_aborts_when_recipe_already_exists(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.my-recipe]\nenabled = true\nversion = \"1.0.0\"\n"
        )
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 1)

    def test_add_fails_when_recipe_not_in_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        rc = self.mod.add_recipe(project, "nonexistent")
        self.assertEqual(rc, 1)

    def test_add_rejects_internal_test_recipe(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "test-fixture"\nname = "Test Fixture"\n'
            'description = "internal"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"test-fixture": recipe_toml}))
        before = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        rc = self.mod.add_recipe(project, "test-fixture")
        self.assertEqual(rc, 1)
        after = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertEqual(before, after)
        self.assertNotIn("[recipes.test-fixture]", after)

    def test_add_does_not_mutate_other_files(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        other_file = project / "other.txt"
        other_file.write_text("original", encoding="utf-8")

        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)
        self.assertEqual(other_file.read_text(encoding="utf-8"), "original")

    def test_add_shows_preview_of_primitives(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[provides]
skills = [
    { id = "my-skill", source = "bundled" },
]
commands = [
    { id = "my-cmd", path = "commands/my-cmd.md" },
]
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

    def test_add_writes_config_placeholders(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.board_id]
required = true
type = "string"

[config.default_list]
required = false
type = "string"
default = "In Progress"

[config.epic_list]
required = false
type = "string"
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.my-recipe.config]", manifest_text)
        self.assertIn('board_id = ""  # REQUIRED', manifest_text)
        self.assertIn('default_list = "In Progress"', manifest_text)
        self.assertIn('# epic_list = ""  # optional', manifest_text)

    def test_double_add_is_idempotent(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc1 = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc1, 0)
        rc2 = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc2, 1)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        count = manifest_text.count("[recipes.my-recipe]")
        self.assertEqual(count, 1)

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = subprocess.run(
                ["python3", str(RECIPE_ADD_PATH), tmp, "my-recipe"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertIn("Project not initialized", proc.stderr)

    def test_add_uses_cli_catalog_when_project_has_no_local_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        self._set_ai_specs_home(ROOT)
        rc = self.mod.add_recipe(project, "trello-mcp-workflow")
        self.assertEqual(rc, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.trello-mcp-workflow]", manifest_text)

    def test_add_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        manifest = '[project]\nname = "test"\n'
        cli_recipe = (
            '[recipe]\nid = "shared-recipe"\nname = "CLI Recipe"\n'
            'description = "Desc"\nversion = "2.0.0"\n'
        )
        local_recipe = (
            '[recipe]\nid = "shared-recipe"\nname = "Local Recipe"\n'
            'description = "Desc"\nversion = "9.9.9"\n'
        )
        project = self._make_project(manifest, {"shared-recipe": local_recipe})
        self._set_ai_specs_home(self._make_cli_home({"shared-recipe": cli_recipe}))
        rc = self.mod.add_recipe(project, "shared-recipe")
        self.assertEqual(rc, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.shared-recipe]", manifest_text)
        self.assertIn("enabled = true", manifest_text)
        self.assertNotIn("version =", manifest_text)


    def test_boolean_default_serializes_as_lowercase_toml(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.auto_remove]
required = false
type = "boolean"
default = true

[config.dry_run]
required = false
type = "boolean"
default = false
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("auto_remove = true", manifest_text)
        self.assertIn("dry_run = false", manifest_text)
        self.assertNotIn("True", manifest_text)
        self.assertNotIn("False", manifest_text)

    def test_list_default_serializes_as_valid_toml(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.tags]
required = false
type = "list"
default = ["alpha", "beta"]
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('tags = ["alpha", "beta"]', manifest_text)

    def test_manifest_remains_valid_toml_after_add_with_non_string_defaults(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.auto_remove]
required = false
type = "boolean"
default = true

[config.retries]
required = false
type = "integer"
default = 3

[config.tags]
required = false
type = "list"
default = ["alpha", "beta"]
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 0)

        manifest_path = project / "ai-specs" / "ai-specs.toml"
        with manifest_path.open("rb") as fh:
            parsed = tomllib.load(fh)
        cfg = parsed["recipes"]["my-recipe"]["config"]
        self.assertEqual(cfg["auto_remove"], True)
        self.assertEqual(cfg["retries"], 3)
        self.assertEqual(cfg["tags"], ["alpha", "beta"])

    def test_add_rolls_back_when_result_is_invalid_toml(self):
        # A manifest already corrupted (e.g. by the old buggy serializer) must
        # not be compounded: the post-write guard reverts and reports failure.
        broken_manifest = '[project]\nname = "test"\n\n[recipes.old.config]\nflag = True\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "1.0.0"\n'
        )
        project = self._make_project(broken_manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        rc = self.mod.add_recipe(project, "my-recipe")
        self.assertEqual(rc, 1)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertEqual(manifest_text, broken_manifest)
        self.assertNotIn("[recipes.my-recipe]", manifest_text)

    def test_tty_missing_interactive_deps_does_not_mutate_manifest(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.board_id]
required = true
type = "string"
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        before = manifest_path.read_text(encoding="utf-8")
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = 3
        fake_util.vendor_dir.return_value = vendor

        with mock.patch.object(self.mod, "_load_sibling", return_value=fake_util), mock.patch.object(
            self.mod.sys.stdin, "isatty", return_value=True
        ), mock.patch.object(self.mod.sys.stdout, "isatty", return_value=True), mock.patch.object(
            self.mod.sys, "stderr", new_callable=io.StringIO
        ) as stderr:
            rc = self.mod.add_recipe(project, "my-recipe")

        self.assertIn("Recipe not added:", stderr.getvalue())

        self.assertEqual(rc, 3)
        self.assertEqual(manifest_path.read_text(encoding="utf-8"), before)
        fake_util.ensure_deps.assert_called_once_with(vendor)

    def test_tty_available_interactive_deps_use_vendor_gate(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.board_id]
required = true
type = "string"
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = None
        fake_util.vendor_dir.return_value = vendor
        config_wizard = mock.Mock()
        questionary = mock.Mock()
        questionary.confirm.return_value.ask.return_value = True

        def load_sibling(name):
            return {"util": fake_util, "config_wizard": config_wizard}[name]

        with mock.patch.object(self.mod, "_load_sibling", side_effect=load_sibling), mock.patch.dict(
            sys.modules, {"questionary": questionary}
        ), mock.patch.object(self.mod.sys.stdin, "isatty", return_value=True), mock.patch.object(
            self.mod.sys.stdout, "isatty", return_value=True
        ):
            rc = self.mod.add_recipe(project, "my-recipe")

        self.assertEqual(rc, 0)
        self.assertIn("[recipes.my-recipe]", (project / "ai-specs" / "ai-specs.toml").read_text())
        fake_util.ensure_deps.assert_called_once_with(vendor)
        config_wizard.configure_selected_recipes.assert_called_once_with(
            project, ["my-recipe"], project / "ai-specs" / "ai-specs.toml"
        )

    def test_non_tty_does_not_call_ensure_deps(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "my-recipe"
name = "My Recipe"
description = "Desc"
version = "1.0.0"

[config.board_id]
required = true
type = "string"
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"my-recipe": recipe_toml}))
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = 3
        fake_util.vendor_dir.return_value = vendor

        with mock.patch.object(self.mod, "_load_sibling", return_value=fake_util), mock.patch.object(
            self.mod.sys.stdin, "isatty", return_value=False
        ), mock.patch.object(self.mod.sys.stdout, "isatty", return_value=False):
            rc = self.mod.add_recipe(project, "my-recipe")

        self.assertEqual(rc, 0)
        self.assertIn("[recipes.my-recipe]", manifest_path.read_text(encoding="utf-8"))
        fake_util.ensure_deps.assert_not_called()

    def test_mcp_env_deps_gate(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "mcp-recipe"
name = "MCP Recipe"
description = "Desc"
version = "1.0.0"

[[provides.mcp]]
id = "test-mcp"
command = "test-cmd"
env = { VAR1 = "$VAR1" }
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"mcp-recipe": recipe_toml}))
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        before = manifest_path.read_text(encoding="utf-8")
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = 3
        fake_util.vendor_dir.return_value = vendor

        with mock.patch.object(self.mod, "_load_sibling", return_value=fake_util), mock.patch.object(
            self.mod.sys.stdin, "isatty", return_value=True
        ), mock.patch.object(self.mod.sys.stdout, "isatty", return_value=True), mock.patch.object(
            self.mod.sys, "stderr", new_callable=io.StringIO
        ) as stderr:
            rc = self.mod.add_recipe(project, "mcp-recipe")

        self.assertEqual(rc, 3)
        self.assertEqual(manifest_path.read_text(encoding="utf-8"), before)
        fake_util.ensure_deps.assert_called_once_with(vendor)
        self.assertIn("Recipe not added:", stderr.getvalue())

    def test_mcp_env_non_tty_gate(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = """[recipe]
id = "mcp-recipe"
name = "MCP Recipe"
description = "Desc"
version = "1.0.0"

[[provides.mcp]]
id = "test-mcp"
command = "test-cmd"
env = { VAR1 = "$VAR1" }
"""
        project = self._make_project(manifest)
        self._set_ai_specs_home(self._make_cli_home({"mcp-recipe": recipe_toml}))
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = 3
        fake_util.vendor_dir.return_value = vendor

        with mock.patch.object(self.mod, "_load_sibling", return_value=fake_util), mock.patch.object(
            self.mod.sys.stdin, "isatty", return_value=False
        ), mock.patch.object(self.mod.sys.stdout, "isatty", return_value=False):
            rc = self.mod.add_recipe(project, "mcp-recipe")

        self.assertEqual(rc, 0)
        self.assertIn("[recipes.mcp-recipe]", manifest_path.read_text(encoding="utf-8"))
        fake_util.ensure_deps.assert_not_called()

    def _enable_vendor_path(self) -> bool:
        """Make vendored rich/questionary importable (mirrors util.ensure_deps)."""
        vendor = ROOT / "lib" / "_vendor"
        if vendor.is_dir() and str(vendor) not in sys.path:
            sys.path.insert(0, str(vendor))
        try:
            import rich.console  # noqa: F401
        except ImportError:
            return False
        return True

    def _jinna_recipe_toml(self) -> str:
        return (
            '[recipe]\n'
            'id = "jinna-flow"\n'
            'name = "Jinna Flow"\n'
            'description = "Desc"\n'
            'version = "1.0.0"\n\n'
            '[[deps.cli]]\n'
            'binary = "jinna"\n'
            'purpose = "Jinna provider CLI"\n'
            'required = true\n'
            'install_url = "https://github.com/example/jinna/releases/latest"\n\n'
            '[[provides.mcp]]\n'
            'id = "jinna"\n'
            'command = "jinna"\n'
            'env = { JINNA_TOKEN = "$JINNA_TOKEN" }\n'
        )

    def _run_add_with_stubs(
        self,
        project: Path,
        *,
        dep_gate_result: bool,
        stdout=None,
    ):
        """Mock the interactive deps for a jinna recipe and capture the dep gate call."""
        vendor = project / "cli-vendor"
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = None
        fake_util.vendor_dir.return_value = vendor
        config_wizard = mock.Mock()
        seen = {}

        def dep_gate(recipe, console):
            seen["recipe"] = recipe
            seen["console"] = console
            return dep_gate_result

        config_wizard._dep_gate.side_effect = dep_gate
        config_wizard.configure_selected_recipes = mock.Mock()
        env_scaffold = mock.Mock()
        env_scaffold.collect_env_vars.return_value = {
            "JINNA_TOKEN": "required by jinna (jinna-flow)"
        }
        dep_install = mock.Mock()
        dep_install.resolve_install_plan.return_value = mock.Mock(
            binary="jinna",
            display="https://github.com/example/jinna/releases/latest",
            command=[],
            kind="guidance",
        )
        questionary = mock.Mock()
        questionary.confirm.return_value.ask.return_value = True

        def load_sibling(name):
            return {
                "util": fake_util,
                "config_wizard": config_wizard,
                "env_scaffold": env_scaffold,
                "dep_install": dep_install,
            }[name]

        stack = contextlib.ExitStack()
        stack.enter_context(
            mock.patch.object(self.mod, "_load_sibling", side_effect=load_sibling)
        )
        stack.enter_context(mock.patch.dict(sys.modules, {"questionary": questionary}))
        stack.enter_context(mock.patch.object(self.mod.sys.stdin, "isatty", return_value=True))
        stack.enter_context(mock.patch.object(self.mod.sys.stdout, "isatty", return_value=True))
        if stdout is not None:
            stack.enter_context(contextlib.redirect_stdout(stdout))
        with stack:
            rc = self.mod.add_recipe(project, "jinna-flow")
        return rc, {
            "dep_gate": seen,
            "config_wizard": config_wizard,
            "env_scaffold": env_scaffold,
            "dep_install": dep_install,
            "util": fake_util,
        }

    def test_add_routes_cli_deps_through_dep_gate(self):
        """A recipe with cli_deps must reach config_wizard._dep_gate on a real Console."""
        if not self._enable_vendor_path():
            self.skipTest("vendored rich unavailable")
        from rich.console import Console

        project = self._make_project('[project]\nname = "test"\n')
        self._set_ai_specs_home(
            self._make_cli_home({"jinna-flow": self._jinna_recipe_toml()})
        )
        rc, stubs = self._run_add_with_stubs(project, dep_gate_result=True)

        self.assertEqual(rc, 0)
        stubs["config_wizard"]._dep_gate.assert_called_once()
        recipe_arg = stubs["dep_gate"]["recipe"]
        console_arg = stubs["dep_gate"]["console"]
        self.assertEqual(recipe_arg.id, "jinna-flow")
        self.assertEqual([d.binary for d in recipe_arg.cli_deps], ["jinna"])
        self.assertIsInstance(console_arg, Console)
        # Env setup is scoped to the added recipe only.
        stubs["env_scaffold"].collect_env_vars.assert_called_once_with(
            project, recipe_ids=["jinna-flow"]
        )
        stubs["env_scaffold"].offer_harness_env.assert_called_once_with(
            project, recipe_ids=["jinna-flow"]
        )

    def test_add_reports_install_guidance_when_dep_gate_unresolved(self):
        """Unresolved CLI deps: explicit install plan surfaced, env setup still runs."""
        if not self._enable_vendor_path():
            self.skipTest("vendored rich unavailable")

        project = self._make_project('[project]\nname = "test"\n')
        self._set_ai_specs_home(
            self._make_cli_home({"jinna-flow": self._jinna_recipe_toml()})
        )
        out = _TtyStringIO()
        rc, stubs = self._run_add_with_stubs(project, dep_gate_result=False, stdout=out)

        self.assertEqual(rc, 0)
        stubs["config_wizard"]._dep_gate.assert_called_once()
        # Never install silently: recipe-add must not invoke an installer itself.
        stubs["dep_install"].offer_and_install.assert_not_called()
        stubs["dep_install"].resolve_install_plan.assert_called_with(
            "jinna", install_url="https://github.com/example/jinna/releases/latest"
        )
        guidance = out.getvalue()
        self.assertIn("https://github.com/example/jinna/releases/latest", guidance)
        self.assertIn("required CLI dependencies are still missing", guidance)
        # Env setup still permitted after an unresolved dep gate.
        stubs["env_scaffold"].offer_harness_env.assert_called_once_with(
            project, recipe_ids=["jinna-flow"]
        )

    def test_add_non_tty_cli_deps_is_guidance_only(self):
        """Non-TTY must not open the dep gate or prompt for env values."""
        project = self._make_project('[project]\nname = "test"\n')
        self._set_ai_specs_home(
            self._make_cli_home({"jinna-flow": self._jinna_recipe_toml()})
        )
        fake_util = mock.Mock()
        fake_util.is_internal_test_recipe.return_value = False
        fake_util.ensure_deps.return_value = 3
        fake_util.vendor_dir.return_value = project / "cli-vendor"
        config_wizard = mock.Mock()
        env_scaffold = mock.Mock()

        def load_sibling(name):
            return {
                "util": fake_util,
                "config_wizard": config_wizard,
                "env_scaffold": env_scaffold,
            }[name]

        out = io.StringIO()
        with mock.patch.object(
            self.mod, "_load_sibling", side_effect=load_sibling
        ), mock.patch.object(
            self.mod.sys.stdin, "isatty", return_value=False
        ), mock.patch.object(
            self.mod.sys.stdout, "isatty", return_value=False
        ), contextlib.redirect_stdout(out):
            rc = self.mod.add_recipe(project, "jinna-flow")

        self.assertEqual(rc, 0)
        config_wizard._dep_gate.assert_not_called()
        env_scaffold.offer_harness_env.assert_not_called()
        self.assertIn("ai-specs configure-recipes", out.getvalue())


if __name__ == "__main__":
    unittest.main()
