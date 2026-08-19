import tempfile
import tomllib
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home, snapshot, tree_diff


class RecipeAddTests(unittest.TestCase):
    """Black-box conversions of the original `recipe-add` tests.

    Each test drives `bin/ai-specs recipe add <id> <project_root>` through the
    shared `invoke` helper (non-TTY), so the interactive dependency gate and
    config wizard never run. Where an original assertion only exercised a
    TTY-only branch or mocked an internal call, the closest observable non-TTY
    surface (exit code + emitted manifest + stdout guidance + no vendor
    mutation) replaces it and a `# TRIAGE:` comment documents the uncovered
    internal branch.
    """

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for sequences)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _add(self, root: Path, recipe_id: str):
        """Single shared helper wrapping the CLI invocation for this class."""
        return invoke(root, "recipe", "add", recipe_id, cli_home=self._cli_home())

    def _make_project(self, manifest_content: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs_dir = project / "ai-specs"
        ai_specs_dir.mkdir()
        (ai_specs_dir / "ai-specs.toml").write_text(manifest_content, encoding="utf-8")
        return project

    def _vended_tree(self) -> dict:
        """Snapshot the CLI vendor dependency area (lib/_vendor) for mutation checks."""
        return snapshot(self._cli_home() / "lib" / "_vendor")

    def test_add_appends_recipe_without_version(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "playwright-mcp")
        self.assertEqual(result.returncode, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.playwright-mcp]", manifest_text)
        self.assertIn("enabled = true", manifest_text)
        self.assertNotIn("version =", manifest_text)

    def test_add_aborts_when_recipe_already_exists(self):
        manifest = (
            '[project]\nname = "test"\n'
            "[recipes.git-pr-flow]\nenabled = true\nversion = \"1.0.0\"\n"
        )
        project = self._make_project(manifest)
        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 1)
        self.assertIn("ya está en el manifest", result.stderr)

    def test_add_fails_when_recipe_not_in_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "nonexistent")
        self.assertEqual(result.returncode, 1)
        self.assertIn("no encontrada en catalog/recipes/", result.stderr)

    def test_add_rejects_internal_test_recipe(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        manifest_path = project / "ai-specs" / "ai-specs.toml"
        before = manifest_path.read_text(encoding="utf-8")
        result = self._add(project, "test-fixture")
        self.assertEqual(result.returncode, 1)
        after = manifest_path.read_text(encoding="utf-8")
        self.assertEqual(before, after)
        self.assertNotIn("[recipes.test-fixture]", after)

    def test_add_does_not_mutate_other_files(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        other_file = project / "other.txt"
        other_file.write_text("original", encoding="utf-8")

        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(other_file.read_text(encoding="utf-8"), "original")

    def test_add_shows_preview_of_primitives(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 0)
        self.assertIn("Próximo sync materializará:", result.stdout)
        self.assertIn("git-merge-workflow", result.stdout)
        self.assertIn("pr-create", result.stdout)

    def test_add_writes_config_placeholders(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)

        # trello-mcp-workflow has a REQUIRED field and a default-valued field.
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.trello-mcp-workflow.config]", manifest_text)
        self.assertIn('board_id = ""  # REQUIRED', manifest_text)
        self.assertIn('default_list = "In Progress"', manifest_text)

        # tdd-flow has an optional field with no default -> placeholder comment.
        result = self._add(project, "tdd-flow")
        self.assertEqual(result.returncode, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('# test_command = ""  # optional', manifest_text)

    def test_double_add_is_idempotent(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        rc1 = self._add(project, "playwright-mcp")
        self.assertEqual(rc1.returncode, 0)
        rc2 = self._add(project, "playwright-mcp")
        self.assertEqual(rc2.returncode, 1)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        count = manifest_text.count("[recipes.playwright-mcp]")
        self.assertEqual(count, 1)

    def test_cli_uninitialized_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = invoke(Path(tmp), "recipe", "add", "git-pr-flow", cli_home=self._cli_home())
            self.assertEqual(result.returncode, 1)
            self.assertIn("Proyecto no inicializado", result.stderr)

    def test_add_uses_cli_catalog_when_project_has_no_local_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.trello-mcp-workflow]", manifest_text)

    def test_add_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        # A deliberately broken project-local catalog would fail IF the CLI
        # consulted it; recipe add only resolves recipes from the CLI catalog.
        local = project / "catalog" / "recipes" / "git-pr-flow"
        local.mkdir(parents=True)
        (local / "recipe.toml").write_text("this is ::: not [valid toml\n", encoding="utf-8")

        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 0)
        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("[recipes.git-pr-flow]", manifest_text)
        self.assertIn("enabled = true", manifest_text)
        self.assertNotIn("version =", manifest_text)

    def test_boolean_default_serializes_as_lowercase_toml(self):
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        # git-pr-flow has a bool default false; worktree-flow a bool default true.
        self.assertEqual(self._add(project, "git-pr-flow").returncode, 0)
        self.assertEqual(self._add(project, "worktree-flow").returncode, 0)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn("auto_switch_account = false", manifest_text)
        self.assertIn("auto_remove_merged = true", manifest_text)
        self.assertNotIn("True", manifest_text)
        self.assertNotIn("False", manifest_text)

    def test_list_default_serializes_as_valid_toml(self):
        # TRIAGE: no public catalog recipe declares a `type = "list"` config
        # default, so the original list-values ("tags = [\"alpha\", \"beta\"]")
        # serialization path is not reachable via `bin/ai-specs recipe add`.
        # The observable intent — non-string defaults serialize as valid TOML
        # and round-trip to their real type — is covered with git-pr-flow's
        # boolean default instead.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 0)

        manifest_path = project / "ai-specs" / "ai-specs.toml"
        with manifest_path.open("rb") as fh:
            parsed = tomllib.load(fh)
        cfg = parsed["recipes"]["git-pr-flow"]["config"]
        self.assertIs(cfg["auto_switch_account"], False)
        self.assertEqual(cfg["base_branch"], "main")

    def test_manifest_remains_valid_toml_after_add_with_non_string_defaults(self):
        # TRIAGE: integer- and list-typed config defaults appear in no public
        # catalog recipe, so those specific default types are not reachable via
        # `recipe add`; the covered intent — the manifest stays valid TOML after
        # appending non-string (boolean) defaults — is asserted with
        # worktree-flow's bool default instead.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "worktree-flow")
        self.assertEqual(result.returncode, 0)

        manifest_path = project / "ai-specs" / "ai-specs.toml"
        with manifest_path.open("rb") as fh:
            parsed = tomllib.load(fh)
        cfg = parsed["recipes"]["worktree-flow"]["config"]
        self.assertIs(cfg["auto_remove_merged"], True)
        self.assertEqual(cfg["gate_mode"], "always")

    def test_add_rolls_back_when_result_is_invalid_toml(self):
        # A manifest already corrupted (e.g. by the old buggy serializer) must
        # not be compounded: the post-write guard reverts and reports failure.
        broken_manifest = '[project]\nname = "test"\n\n[recipes.old.config]\nflag = True\n'
        project = self._make_project(broken_manifest)
        result = self._add(project, "git-pr-flow")
        self.assertEqual(result.returncode, 1)

        manifest_text = (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertEqual(manifest_text, broken_manifest)
        self.assertNotIn("[recipes.git-pr-flow]", manifest_text)

    def test_tty_missing_interactive_deps_does_not_mutate_manifest(self):
        # TRIAGE: the TTY-only branch (`ensure_deps` returning 3 before the
        # write, leaving the manifest unmutated) has no non-TTY surface because
        # `invoke` runs with stdin/stdout not a TTY, so the dependency gate is
        # never entered. The equivalent observable non-TTY surface is captured:
        # the recipe with config fields is added (exit 0), the manifest is
        # written, and the vendor dependency area is left untouched (no
        # `ensure_deps` install happened).
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        before = self._vended_tree()
        result = self._add(project, "trello-mcp-workflow")
        after = self._vended_tree()
        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.trello-mcp-workflow]", (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertEqual(tree_diff(before, after)["created"], [])

    def test_tty_available_interactive_deps_use_vendor_gate(self):
        # TRIAGE: the config-wizard invocation (`config_wizard
        # .configure_selected_recipes`) and the questionary confirm prompt are
        # TTY-only and never run under `invoke`; no non-TTY surface exposes
        # them. The non-TTY recipe-with-config surface — success plus the which
        # next-step guidance line — is asserted instead.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        self.assertIn("Siguientes pasos:", result.stdout)
        self.assertIn("Configurar valores requeridos: ai-specs configure-recipes", result.stdout)
        self.assertIn("Configurar variables de entorno MCP: ai-specs configure-recipes", result.stdout)

    def test_non_tty_does_not_call_ensure_deps(self):
        # TRIAGE: the mock-level assertion "ensure_deps was not called" is
        # internal; its observable non-TTY equivalent is that the recipe with
        # config fields is still added (exit 0) and written — had the
        # non-TTY dependency gate fired it would have returned 3 instead.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.trello-mcp-workflow]", (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))

    def test_mcp_env_deps_gate(self):
        # TRIAGE: the TTY-only MCP `ensure_deps` gate (rc 3 + no mutation when
        # deps are unavailable) is not entered in non-TTY. Its observable
        # equivalent is that an MCP-env recipe (vault-canonical-store declares
        # MCP env vars) is added successfully with the MCP env guidance printed
        # and no vendor mutation.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        before = self._vended_tree()
        result = self._add(project, "vault-canonical-store")
        after = self._vended_tree()
        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.vault-canonical-store]", (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertIn("Configurar variables de entorno MCP: ai-specs configure-recipes", result.stdout)
        self.assertEqual(tree_diff(before, after)["created"], [])

    def test_mcp_env_non_tty_gate(self):
        # TRIAGE: the internal "ensure_deps was not called" mock assertion for
        # an MCP-env recipe is unobservable; its non-TTY observable equivalent
        # is that the recipe is added (exit 0) and written.
        manifest = '[project]\nname = "test"\n'
        project = self._make_project(manifest)
        result = self._add(project, "trello-mcp-workflow")
        self.assertEqual(result.returncode, 0)
        self.assertIn("[recipes.trello-mcp-workflow]", (project / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
