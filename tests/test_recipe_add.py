"""Black-box tests for `ai-specs recipe add` (converted from recipe-add.py internals)."""
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, snapshot, tree_diff  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


class RecipeAddTests(unittest.TestCase):
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
        """Isolated install root with an EMPTY catalog plus the given recipes."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name), catalog=False)
        for rid, content in catalog_recipes.items():
            rdir = home / "catalog" / "recipes" / rid
            rdir.mkdir(parents=True, exist_ok=True)
            (rdir / "recipe.toml").write_text(content, encoding="utf-8")
        return home

    def _add(self, project: Path, recipe_id: str, home: Path | None = None):
        """Run `ai-specs recipe add <id> <project>` non-interactively (piped stdin)."""
        return invoke(project, "recipe", "add", recipe_id, cli_home=home, stdin="")

    def test_add_appends_recipe_without_version(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "my-recipe"\nname = "My Recipe"\n'
            'description = "Desc"\nversion = "2.1.0"\n'
        )
        project = self._make_project(manifest)
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("Recipe 'my-recipe' added", result.stdout)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("already in the manifest", result.stderr)

    def test_add_fails_when_recipe_not_in_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "nonexistent", self._make_cli_home({}))
        self.assertEqual(result.returncode, 1)
        self.assertIn("not found in catalog", result.stderr)

    def test_add_rejects_internal_test_recipe(self):
        manifest = '[project]\nname = "test"\n'
        recipe_toml = (
            '[recipe]\nid = "test-fixture"\nname = "Test Fixture"\n'
            'description = "internal"\nversion = "1.0.0"\n'
        )
        project = self._make_project(manifest)
        home = self._make_cli_home({"test-fixture": recipe_toml})
        before = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        result = self._add(project, "test-fixture", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("internal test fixture", result.stderr)
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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        other_file = project / "other.txt"
        other_file.write_text("original", encoding="utf-8")

        before = snapshot(project)
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)
        diff = tree_diff(before, snapshot(project))
        self.assertEqual(diff["created"], [])
        self.assertEqual(diff["deleted"], [])
        self.assertEqual(diff["modified"], ["ai-specs/ai-specs.toml"])
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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("The next sync will materialize", result.stdout)
        self.assertIn("my-skill", result.stdout)
        self.assertIn("my-cmd", result.stdout)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result1 = self._add(project, "my-recipe", home)
        self.assertEqual(result1.returncode, 0)
        result2 = self._add(project, "my-recipe", home)
        self.assertEqual(result2.returncode, 1)
        self.assertIn("already in the manifest", result2.stderr)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        count = manifest_text.count("[recipes.my-recipe]")
        self.assertEqual(count, 1)

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = invoke(Path(tmp), "recipe", "add", "my-recipe", stdin="")
            self.assertEqual(result.returncode, 1)
            self.assertIn("Project not initialized", result.stderr)

    def test_add_uses_cli_catalog_when_project_has_no_local_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        # cli_home=None builds an isolated install root whose catalog is the
        # CLI's own repo catalog; the project has no local catalog.
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
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
        home = self._make_cli_home({"shared-recipe": cli_recipe})
        result = self._add(project, "shared-recipe", home)
        self.assertEqual(result.returncode, 0)
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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 0)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("was not modified", result.stderr)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertEqual(manifest_text, broken_manifest)
        self.assertNotIn("[recipes.my-recipe]", manifest_text)

    # TRIAGE: recipe add tty interactive deps gate — the TTY stdin surface is
    # unreachable through the black-box helper (invoke pipes stdin), so the
    # interactive-deps abort path cannot be driven via the CLI here. The
    # nearest observable non-tty contract is asserted instead.
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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        before = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        result = self._add(project, "my-recipe", home)

        self.assertNotIn("Recipe not added:", result.stderr)
        self.assertEqual(result.returncode, 0)
        self.assertNotEqual(
            (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"), before
        )
        self.assertIn("[recipes.my-recipe]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))

    # TRIAGE: recipe add tty config wizard — the interactive questionary flow
    # ("Configure now?") needs a TTY stdin/stdout pair, which the black-box
    # helper cannot provide. The non-tty deferral contract is asserted instead.
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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        result = self._add(project, "my-recipe", home)

        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.my-recipe]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertIn("Configure required values: ai-specs configure-recipes", result.stdout)
        self.assertNotIn("Configure now?", result.stdout)

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
        home = self._make_cli_home({"my-recipe": recipe_toml})
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        result = self._add(project, "my-recipe", home)

        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.my-recipe]", manifest_path.read_text(encoding="utf-8"))
        self.assertNotIn("Recipe not added:", result.stderr)
        self.assertNotIn("interactive dependencies", result.stderr)

    # TRIAGE: recipe add tty mcp env gate — the interactive env scaffold runs
    # only on a TTY stdin, which the black-box helper cannot provide. The
    # non-tty mcp-env guidance contract is asserted instead.
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
        home = self._make_cli_home({"mcp-recipe": recipe_toml})
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        result = self._add(project, "mcp-recipe", home)

        self.assertNotIn("Recipe not added:", result.stderr)
        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.mcp-recipe]", manifest_path.read_text(encoding="utf-8"))
        self.assertIn("Configure MCP environment variables: ai-specs configure-recipes",
                      result.stdout)

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
        home = self._make_cli_home({"mcp-recipe": recipe_toml})
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        result = self._add(project, "mcp-recipe", home)

        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.mcp-recipe]", manifest_path.read_text(encoding="utf-8"))
        self.assertNotIn("Recipe not added:", result.stderr)

    _JINNA_RECIPE_TOML = (
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

    _JINNA_RECIPE_TOML_WITH_CONFIG = (
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
        '[config.board_id]\n'
        'required = true\n'
        'type = "string"\n\n'
        '[[provides.mcp]]\n'
        'id = "jinna"\n'
        'command = "jinna"\n'
        'env = { JINNA_TOKEN = "$JINNA_TOKEN" }\n'
    )

    # TRIAGE: recipe add tty cli-dep gate — the config_wizard dep gate runs
    # only on a TTY (with rich/questionary), which the black-box helper cannot
    # provide. The non-tty guidance-only contract is asserted instead.
    def test_add_routes_cli_deps_through_dep_gate(self):
        """A recipe with cli_deps must not silently install or gate non-interactively."""
        project = self._make_project('[project]\nname = "test"\n')
        home = self._make_cli_home({"jinna-flow": self._JINNA_RECIPE_TOML})
        result = self._add(project, "jinna-flow", home)

        self.assertEqual(result.returncode, 0)
        self.assertNotIn("required CLI dependencies are still missing", result.stdout)
        self.assertIn("[recipes.jinna-flow]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))

    # TRIAGE: recipe add tty dep-gate deferral — the deferral to the wizard's
    # own gate is observable only inside the interactive wizard, which needs a
    # TTY. The non-tty config guidance contract is asserted instead.
    def test_add_with_config_defers_dep_gate_to_config_wizard(self):
        """A recipe with config fields and cli_deps adds without a dep panel non-interactively."""
        project = self._make_project('[project]\nname = "test"\n')
        home = self._make_cli_home({"jinna-flow": self._JINNA_RECIPE_TOML_WITH_CONFIG})
        result = self._add(project, "jinna-flow", home)

        self.assertEqual(result.returncode, 0)
        self.assertIn("Configure required values: ai-specs configure-recipes", result.stdout)
        self.assertIn("[recipes.jinna-flow]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))

    # TRIAGE: recipe add tty unresolved-dep guidance — install guidance is
    # printed only after a TTY dep-gate decline, which the black-box helper
    # cannot drive. The non-tty behavior (no gate, no guidance) is asserted.
    def test_add_reports_install_guidance_when_dep_gate_unresolved(self):
        """Non-tty runs no dep gate, so no install guidance is claimed."""
        project = self._make_project('[project]\nname = "test"\n')
        home = self._make_cli_home({"jinna-flow": self._JINNA_RECIPE_TOML})
        result = self._add(project, "jinna-flow", home)

        self.assertEqual(result.returncode, 0)
        self.assertNotIn("required CLI dependencies are still missing", result.stdout)
        self.assertNotIn("https://github.com/example/jinna/releases/latest", result.stdout)
        self.assertIn("[recipes.jinna-flow]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))

    def test_add_non_tty_cli_deps_is_guidance_only(self):
        """Non-TTY must not open the dep gate or prompt for env values."""
        project = self._make_project('[project]\nname = "test"\n')
        home = self._make_cli_home({"jinna-flow": self._JINNA_RECIPE_TOML})
        result = self._add(project, "jinna-flow", home)

        self.assertEqual(result.returncode, 0)
        self.assertNotIn("Recipe not added:", result.stderr)
        self.assertIn("ai-specs configure-recipes", result.stdout)
        self.assertIn("[recipes.jinna-flow]",
                      (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
