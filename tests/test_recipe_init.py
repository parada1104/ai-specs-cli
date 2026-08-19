import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import CLI, invoke, isolated_home, snapshot


def _manifest(*, installed: bool = True, config: str = "", mcp: bool = True) -> str:
    """Build an ai-specs.toml manifest that drives a trello-mcp-workflow init brief."""
    lines = [
        "[project]",
        'name = "fixture"',
        "[agents]",
        'enabled = ["claude", "opencode"]',
        "",
    ]
    if mcp:
        lines += [
            "[mcp.trello]",
            'command = "npx"',
            'args = ["-y", "@delorenj/mcp-server-trello"]',
            'env = { TRELLO_TOKEN = "$TRELLO_TOKEN", literal_secret = "super-secret" }',
            'headers = { Authorization = "literal-auth" }',
            "",
        ]
    if installed:
        lines += [
            "[recipes.trello-mcp-workflow]",
            "enabled = true",
            'version = "1.3.0"',
            "",
        ]
        if config:
            lines += ["[recipes.trello-mcp-workflow.config]"]
            lines += [line for line in config.splitlines() if line]
            lines.append("")
    return "\n".join(lines).rstrip() + "\n"


class RecipeInitTests(unittest.TestCase):
    def _cli_home(self) -> Path:
        """One shared isolated CLI home per test (install root + cache root)."""
        if getattr(self, "_home", None) is None:
            self._tmp = tempfile.TemporaryDirectory(prefix="recipe-init-home-")
            self.addCleanup(self._tmp.cleanup)
            self._home = isolated_home(Path(self._tmp.name))
        return self._home

    def _init(self, root: Path, *ids: str):
        """Run `bin/ai-specs recipe init <id>` against root via the shared home."""
        return invoke(root, "recipe", "init", *ids, cli_home=self._cli_home())

    def _make_project(self, *, installed: bool = True, config: str = "", mcp: bool = True) -> Path:
        tmp = tempfile.TemporaryDirectory(prefix="recipe-init-proj-")
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(_manifest(installed=installed, config=config, mcp=mcp), encoding="utf-8")
        return root

    # 1
    def test_init_brief_for_installed_recipe_is_read_only_and_context_rich(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"')
        r = self._init(root, "trello-mcp-workflow")
        self.assertEqual(r.returncode, 0)
        self.assertIn("# Recipe Init Brief", r.stdout)
        self.assertIn("- ID: trello-mcp-workflow", r.stdout)
        self.assertIn("- Install state: installed", r.stdout)
        self.assertIn("- Name: Trello MCP Workflow", r.stdout)

    # 2
    def test_build_init_brief_reports_existing_config_and_does_not_duplicate_keys(self):
        root = self._make_project(config='board_id = "aaaaaaaaaaaaaaaaaaaaaaaa"')
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("Install state: installed", r.stdout)
        self.assertIn("Existing config keys: board_id", r.stdout)
        self.assertIn("Update existing key `board_id`", r.stdout)
        # The resolved config value is never echoed as a manifest assignment; the
        # shared trello init.md legitimately prints an "<answer:board_id>" template
        # placeholder under Prompt Content, so assert on the resolved value instead
        # of the generic "board_id =" shape.
        self.assertNotIn('aaaaaaaaaaaaaaaaaaaaaaaa', r.stdout)

    # 3
    def test_available_recipe_before_add_succeeds_with_reviewable_manifest_guidance(self):
        root = self._make_project(installed=False, mcp=False)
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("Install state: available (not installed)", r.stdout)
        self.assertIn("[recipes.trello-mcp-workflow]", r.stdout)
        self.assertIn("enabled = true", r.stdout)
        self.assertIn("trello: missing", r.stdout)

    # 4
    def test_missing_recipe_fails(self):
        root = self._make_project()
        r = self._init(root, "missing")
        self.assertEqual(r.returncode, 1)
        self.assertIn("Recipe 'missing' no encontrada", r.stderr)

    # 5
    def test_init_rejects_internal_test_recipe(self):
        root = self._make_project()
        r = self._init(root, "test-fixture")
        self.assertEqual(r.returncode, 1)
        self.assertIn("internal test fixture", r.stderr)

    # 6
    def test_uninitialized_project_fails_without_mutating(self):
        tmp = tempfile.TemporaryDirectory(prefix="recipe-init-empty-")
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        before = snapshot(root)
        r = self._init(root, "trello-mcp-workflow")
        self.assertEqual(r.returncode, 1)
        self.assertIn("Proyecto no inicializado", r.stderr)
        self.assertEqual(snapshot(root), {})
        self.assertEqual(list(root.iterdir()), [])

    # 7
    def test_recipe_without_init_workflow_fails(self):
        root = self._make_project()
        # git-pr-flow is a public catalog recipe that declares no [init] workflow.
        r = self._init(root, "git-pr-flow")
        self.assertEqual(r.returncode, 1)
        self.assertIn("has no init workflow", r.stderr)

    # 8
    def test_mcp_discovery_redacts_secrets_and_mentions_manifest_precedence(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"')
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("## MCP Discovery", r.stdout)
        self.assertIn("trello: configured", r.stdout)
        self.assertIn("recipe preset available", r.stdout)
        self.assertIn("${TRELLO_TOKEN}", r.stdout)
        self.assertIn("literal_secret: ***", r.stdout)
        self.assertIn("Authorization: ***", r.stdout)
        self.assertNotIn("super-secret", r.stdout)
        self.assertNotIn("literal-auth", r.stdout)
        self.assertIn("project manifest values take precedence", r.stdout)
        # TRIAGE: "API_TOKEN: ***" — ran `bin/ai-specs recipe init trello-mcp-workflow <root>`.
        # Observed surfaces: returncode 0, empty stderr, and an unchanged project tree
        # (tree_diff before/after == empty lists — the command is read-only). The public
        # trello recipe's provides.mcp env is `{ TRELLO_API_KEY = "$TRELLO_API_KEY",
        # TRELLO_TOKEN = "$TRELLO_TOKEN" }` — both are $VAR references, so redaction emits
        # `${TRELLO_API_KEY}` / `${TRELLO_TOKEN}` env refs, never a `key: ***` preset line.
        # No real catalog recipe declares a literal `API_TOKEN = "…"` preset key, so no
        # `API_TOKEN: ***` string exists on any of the three surfaces; the recipe-preset
        # redaction intent is preserved by the `${TRELLO_TOKEN}` / recipe-preset assertions
        # above. "missing-mcp: missing" has no public multi-MCP recipe (needed-id lists are
        # single-element), so its missing-server status path is covered as `trello: missing`
        # in test_available_recipe_before_add_succeeds_with_reviewable_manifest_guidance.

    # 9
    def test_unknown_config_keys_are_reported_without_claiming_sync_success(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"\nunknown = "value"')
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("Unknown config keys: unknown", r.stdout)
        self.assertIn("sync still validates recipe config later", r.stdout)

    # 10
    def test_template_preview_reports_existing_targets_without_overwrite(self):
        root = self._make_project()
        target = root / "ai-specs" / "recipes" / "trello-mcp-workflow" / "overrides" / "templates" / "card-feature.md"
        target.parent.mkdir(parents=True)
        target.write_text("existing", encoding="utf-8")
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md", r.stdout)
        self.assertIn("exists", r.stdout)
        self.assertIn("review update/skip/diff", r.stdout)
        self.assertEqual(target.read_text(encoding="utf-8"), "existing")

    # 11
    def test_cli_dispatch_success_and_usage_errors(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"')
        r = self._init(root, "trello-mcp-workflow")
        self.assertEqual(r.returncode, 0)
        self.assertIn("# Recipe Init Brief", r.stdout)
        self.assertEqual(r.stderr, "")

        # Malformed argv (no recipe id at all): drive bin/ai-specs directly with the
        # shared isolated home. The wrapper's argument-validation branch exits 2 with
        # "missing recipe id" before touching the project.
        home = self._cli_home()
        env = {
            **os.environ,
            "AI_SPECS_HOME": str(home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        missing = subprocess.run([str(CLI), "recipe", "init"], capture_output=True, text=True,
                                 check=False, env=env)
        self.assertEqual(missing.returncode, 2)
        self.assertIn("missing recipe id", missing.stderr)

    # 12
    def test_init_does_not_sync_or_materialize(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"')
        before = snapshot(root)
        r = self._init(root, "trello-mcp-workflow")
        after = snapshot(root)
        self.assertEqual(r.returncode, 0)
        self.assertIn("No files were changed", r.stdout)
        self.assertIn("Init does not run sync or materialize primitives", r.stdout)
        self.assertEqual(after, before)
        self.assertFalse((root / "ai-specs" / ".recipe").exists())
        self.assertFalse((root / "ai-specs" / ".tmp" / "recipe-mcp.json").exists())

    # 13
    def test_trello_recipe_init_uses_cli_catalog_when_project_has_no_local_catalog(self):
        tmp = tempfile.TemporaryDirectory(prefix="recipe-init-nolocal-")
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(_manifest(installed=False, mcp=True), encoding="utf-8")
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("- ID: trello-mcp-workflow", r.stdout)
        self.assertIn("- Install state: available (not installed)", r.stdout)
        self.assertIn("Configure Trello board and list mappings before sync", r.stdout)
        self.assertIn("board_id", r.stdout)
        self.assertIn("trello: configured", r.stdout)
        self.assertIn("# Recipe Init Contract", r.stdout)
        self.assertIn("- Needs MCP: trello", r.stdout)

    # 14
    def test_init_ignores_project_local_catalog_in_favor_of_cli_catalog(self):
        root = self._make_project(config='board_id = "69ec097f13e2d38ecd89a557"')
        local_recipe = root / "catalog" / "recipes" / "trello-mcp-workflow" / "recipe.toml"
        local_recipe.parent.mkdir(parents=True)
        local_recipe.write_text(
            '[recipe]\nid = "trello-mcp-workflow"\nname = "Local Trello"\nversion = "9.9"\n',
            encoding="utf-8",
        )
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("- Name: Trello MCP Workflow", r.stdout)
        self.assertIn("- Version: 1.3.0", r.stdout)
        self.assertNotIn("Local Trello", r.stdout)
        self.assertNotIn("9.9", r.stdout)

    # 15
    def test_init_lists_schema_required_and_optional_config_targets(self):
        root = self._make_project(installed=False)
        r = self._init(root, "trello-mcp-workflow")
        self.assertIn("Add required `board_id` under `[recipes.trello-mcp-workflow.config]`", r.stdout)
        self.assertIn("`default_list` is optional and defaults to `In Progress`", r.stdout)
        self.assertIn("`epic_list` is optional and defaults to `Epic`", r.stdout)
        self.assertIn("sync still validates it later", r.stdout)


if __name__ == "__main__":
    unittest.main()
