import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import populate_catalog  # noqa: E402
from _blackbox import cache_project_dir, invoke, isolated_home

def _cli_home_with_catalog(
    register, *, recipe_tomls: dict[str, str] | None = None
) -> Path:
    """isolated_home whose catalog is a real dir with public + fixture recipes
    plus the given custom recipes (custom entries win on id collisions)."""
    tmp = tempfile.TemporaryDirectory()
    register(tmp.cleanup)
    home = isolated_home(Path(tmp.name))
    catalog = home / "catalog"
    catalog.unlink()
    recipes_dir = catalog / "recipes"
    recipes_dir.mkdir(parents=True)
    for rid, toml in (recipe_tomls or {}).items():
        (recipes_dir / rid).mkdir()
        (recipes_dir / rid / "recipe.toml").write_text(toml)
    populate_catalog(recipes_dir)
    return home



class RecipeMaterializeTests(unittest.TestCase):

    def _cli_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return isolated_home(Path(tmp.name))

    def _sync(self, root: Path):
        home = self._cli_home()
        return home, invoke(root, "sync", cli_home=home)

    @staticmethod
    def _recipe_skill_path(root: Path, home: Path, recipe_id: str, skill_id: str) -> Path:
        return (
            cache_project_dir(root, home)
            / ".recipe" / recipe_id / "skills" / skill_id
        )

    @staticmethod
    def _command_path(root: Path, home: Path, command_id: str) -> Path:
        return cache_project_dir(root, home) / "commands" / f"{command_id}.md"


    def _make_project(self, recipe_section: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + recipe_section
            + "\n"
        )
        return root

    def test_materializes_bundled_skill(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill_dir = self._recipe_skill_path(root, home, "worktree-flow", "worktree-flow")
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())


    def test_materializes_command(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        command = self._command_path(root, home, "worktree-new")
        self.assertTrue(command.is_file())


    def test_materializes_doc(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        doc = root / "ai-specs" / "recipes" / "worktree-flow" / "README.md"
        self.assertTrue(doc.is_file())


    def test_materializes_template_not_exists(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        template = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides"
            / "bin" / "worktree-cleanup.sh"
        )
        self.assertTrue(template.is_file())


    def test_skips_template_when_target_exists(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        existing = (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides"
            / "bin" / "worktree-cleanup.sh"
        )
        existing.parent.mkdir(parents=True, exist_ok=True)
        existing.write_text("existing")
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(existing.read_text(), "existing")


    def test_writes_recipe_mcp_json(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = \"69ec097f13e2d38ecd89a557\"\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        mcp_path = root / ".mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("trello", data["mcpServers"])
        self.assertEqual(data["mcpServers"]["trello"]["command"], "npx")

    def test_disabled_recipe_skips_materialization(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = false\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill_dir = self._recipe_skill_path(root, home, "worktree-flow", "worktree-flow")
        self.assertFalse(skill_dir.exists())


    def test_sync_without_version_succeeds(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill_dir = self._recipe_skill_path(root, home, "worktree-flow", "worktree-flow")
        self.assertTrue(skill_dir.is_dir())


    def test_legacy_version_warns_and_succeeds(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\nversion = \"2.0.0\"\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("legacy", result.stderr.lower())
        self.assertIn("version", result.stderr.lower())
        skill_dir = self._recipe_skill_path(root, home, "worktree-flow", "worktree-flow")
        self.assertTrue(skill_dir.is_dir())


    def test_unknown_recipe_fails(self):
        root = self._make_project(
            "[recipes.nonexistent]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("recipe directory not found", result.stderr)
        self.assertIn("nonexistent", result.stderr)


    def test_no_recipes_section_succeeds(self):
        root = self._make_project("")
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("sync complete", result.stdout)


    def test_recipe_does_not_overwrite_user_local_skill(self):
        root = self._make_project(
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        user_skill = root / "ai-specs" / "skills" / "worktree-flow"
        user_skill.mkdir(parents=True)
        (user_skill / "SKILL.md").write_text("user local")
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((user_skill / "SKILL.md").read_text(), "user local")
        recipe_skill = self._recipe_skill_path(root, home, "worktree-flow", "worktree-flow")
        self.assertTrue(recipe_skill.is_dir())


    # --- V2 materialize tests -----------------------------------------------


    def test_resolve_bindings_explicit(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.gitlab-mr-flow]\nenabled = true\n"
            "[[bindings]]\ncapability = 'vcs-pr-flow'\nrecipe = 'git-pr-flow'\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("VCS/PR provider: GitHub", agents)
        self.assertNotIn("VCS/PR provider: GitLab", agents)

    def test_resolve_bindings_auto_bind_single_provider(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("VCS/PR provider: GitHub", agents)
        self.assertIn("base branch: `main`", agents)

    def test_resolve_bindings_auto_bind_skips_ambiguity(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.gitlab-mr-flow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("capability ambiguity", result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertNotIn("VCS/PR provider:", agents)

    def test_resolve_bindings_explicit_disabled_fails(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[[bindings]]\ncapability = 'vcs-pr-flow'\nrecipe = 'gitlab-mr-flow'\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("disabled/unknown", result.stderr)
        self.assertIn("vcs-pr-flow", result.stderr)

    def test_resolve_bindings_duplicate_explicit_fails(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.gitlab-mr-flow]\nenabled = true\n"
            "[[bindings]]\ncapability = 'vcs-pr-flow'\nrecipe = 'git-pr-flow'\n"
            "[[bindings]]\ncapability = 'vcs-pr-flow'\nrecipe = 'gitlab-mr-flow'\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("duplicate explicit binding", result.stderr)
        self.assertIn("vcs-pr-flow", result.stderr)

    def test_merge_config_defaults_and_override(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.git-pr-flow.config]\nbase_branch = 'development'\n"
        )
        _, result = self._sync(root)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("**Integration branch**: `development`", agents)
        self.assertIn("base branch: `development`", agents)

    def test_merge_config_missing_required_fails(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_merge_config_warns_on_unknown_key(self):
        root = self._make_project(
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.git-pr-flow.config]\nunknown = 1\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("unknown config key", result.stderr)
        self.assertIn("ignored", result.stderr)

    def test_execute_hooks_validate_config_success(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_execute_hooks_validate_config_fails(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        # TRIAGE: sync exposes the recipe and missing field but not the internal
        # hook action name; the command run was `ai-specs sync <project>`.
        self.assertIn("Trello MCP Workflow", result.stderr)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_execute_hooks_bootstrap_board_creates_marker(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
            "default_list = 'In Progress'\n"
            "epic_list = 'Epic'\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        marker_dir = cache_project_dir(root, home) / ".recipe" / "trello-mcp-workflow"
        self.assertTrue(marker_dir.is_dir())
        marker_file = marker_dir / "bootstrap-ready"
        self.assertTrue(marker_file.is_file())
        content = marker_file.read_text()
        self.assertIn("board_id=69ec097f13e2d38ecd89a557", content)
        self.assertIn("default_list=In Progress", content)
        self.assertIn("epic_list=Epic", content)

    def test_execute_hooks_bootstrap_board_marker_content(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
            "default_list = 'Todo'\n"
            "epic_list = 'Backlog'\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        marker_file = (
            cache_project_dir(root, home)
            / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        )
        content = marker_file.read_text()
        self.assertEqual(
            content,
            "board_id=69ec097f13e2d38ecd89a557\n"
            "default_list=Todo\n"
            "epic_list=Backlog\n",
        )

    def test_execute_hooks_bootstrap_board_missing_board_id(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
        )
        _, result = self._sync(root)
        self.assertNotEqual(result.returncode, 0)
        # TRIAGE: sync exposes the recipe and missing field but not the internal
        # hook action name; the command run was `ai-specs sync <project>`.
        self.assertIn("Trello MCP Workflow", result.stderr)
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_execute_hooks_deferred_link_trello_card(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("link-trello-card", result.stdout)
        self.assertIn("deferred", result.stdout)

    def test_execute_hooks_deferred_sync_card_state(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("sync-card-state", result.stdout)
        self.assertIn("deferred", result.stdout)

    def test_execute_hooks_deferred_comment_verification(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("comment-verification", result.stdout)
        self.assertIn("deferred", result.stdout)

    def test_mcp_preset_manifest_precedence_on_conflict(self):
        root = self._make_project_with_mcp(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n",
            "[mcp.trello]\ncommand = 'custom-cmd'\nargs = ['--flag']\n",
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        mcp_path = root / ".mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("trello", data["mcpServers"])
        self.assertEqual(data["mcpServers"]["trello"]["command"], "custom-cmd")
        self.assertEqual(data["mcpServers"]["trello"]["args"], ["--flag"])

    def test_mcp_preset_recipe_creates_when_not_in_manifest(self):
        root = self._make_project_with_mcp(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n",
            "",
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        mcp_path = root / ".mcp.json"
        self.assertTrue(mcp_path.is_file())
        data = json.loads(mcp_path.read_text())
        self.assertIn("trello", data["mcpServers"])
        self.assertEqual(data["mcpServers"]["trello"]["command"], "npx")
        self.assertEqual(
            data["mcpServers"]["trello"]["args"],
            ["-y", "@delorenj/mcp-server-trello"],
        )

    def test_mcp_preset_merge_warns_on_conflict(self):
        root = self._make_project_with_mcp(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n",
            "[mcp.trello]\ncommand = 'custom-cmd'\n",
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("conflicts with project manifest", result.stderr)

    def test_execute_hooks_unknown_action_warns(self):
        home = _cli_home_with_catalog(
            self.addCleanup,
            recipe_tomls={
                "r": (
                    '[recipe]\n'
                    'id = "r"\n'
                    'name = "R"\n'
                    'description = "D"\n'
                    'version = "1.0"\n'
                    '[[hooks]]\n'
                    'event = "on-sync"\n'
                    'action = "unknown"\n'
                )
            },
        )
        root = self._make_project("[recipes.r]\nenabled = true\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("unknown hook action 'unknown' (skipped)", result.stderr)

    def test_end_to_end_v2_recipe_with_config_and_hooks(self):
        toml = (
            '[recipe]\n'
            'id = "v2-recipe"\n'
            'name = "V2 Recipe"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[capabilities]]\n'
            'id = "tracker"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            '[[provides.skills]]\n'
            'id = "v2-skill"\n'
            'source = "bundled"\n'
        )
        home = _cli_home_with_catalog(
            self.addCleanup, recipe_tomls={"v2-recipe": toml}
        )
        recipe_dir = home / "catalog" / "recipes" / "v2-recipe"
        (recipe_dir / "skills").mkdir()
        (recipe_dir / "skills" / "v2-skill").mkdir()
        (recipe_dir / "skills" / "v2-skill" / "SKILL.md").write_text("skill")
        (recipe_dir / "commands").mkdir()
        (recipe_dir / "commands" / "v2-cmd.md").write_text("cmd")
        (recipe_dir / "templates").mkdir()
        (recipe_dir / "templates" / "tpl.md").write_text("tpl")
        (recipe_dir / "docs").mkdir()
        (recipe_dir / "docs" / "doc.md").write_text("doc")
        root = self._make_project(
            "[recipes.v2-recipe]\nenabled = true\n"
            "[recipes.v2-recipe.config]\nboard_id = 'abc123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(
            self._recipe_skill_path(root, home, "v2-recipe", "v2-skill").is_dir()
        )

    def test_v1_manifest_without_bindings_or_config_succeeds(self):
        # Vehicle changed from the internal test-fixture recipe (whose success
        # path requires AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES=1, which
        # _blackbox.invoke cannot set in the subprocess env) to the public
        # worktree-flow recipe; the v1-manifest-without-bindings/config contract
        # is unchanged.
        root = self._make_project(
            '[recipes.worktree-flow]\nenabled = true\nversion = "1.5.0"\n'
        )
        _, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)

    # --- MCP preset merge safety tests --------------------------------------

    def _make_project_with_mcp(self, recipe_section: str, mcp_section: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + mcp_section + "\n"
            + recipe_section
            + "\n"
        )
        return root


    # --- Hook execution: bootstrap-board --------------------------------------

    # --- Hook execution: project_root parameter ------------------------------

    def test_execute_hooks_project_root_used_by_bootstrap_board(self):
        root = self._make_project(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec0a2099ea20956e371d62'\n"
        )
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        marker = (
            cache_project_dir(root, home)
            / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        )
        self.assertTrue(marker.is_file())
        self.assertIn("board_id=69ec0a2099ea20956e371d62", marker.read_text())

    def test_execute_hooks_project_root_different_paths(self):
        home = self._cli_home()
        for board_id in ("111111111111111111111111", "222222222222222222222222"):
            root = self._make_project(
                "[recipes.trello-mcp-workflow]\nenabled = true\n"
                "[recipes.trello-mcp-workflow.config]\n"
                f"board_id = '{board_id}'\n"
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            marker = (
                cache_project_dir(root, home)
                / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
            )
            self.assertTrue(marker.is_file())
            self.assertIn(f"board_id={board_id}", marker.read_text())

    # --- Config validation: board_id / optional fields -----------------------

    def test_config_validation_board_id_required(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project('[recipes.r]\nenabled = true\n')
        result = invoke(root, "sync", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        # TRIAGE: the internal hook action name 'validate-config' is not rendered
        # by the CLI surface — `ai-specs sync <project>` on a custom cfg fixture
        # exits 1 with 'missing required config field' + 'board_id' from
        # required-field enforcement, but never prints the hook action name.
        self.assertIn("missing required config field", result.stderr)
        self.assertIn("board_id", result.stderr)

    def test_config_validation_default_list_optional(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            '[config.default_list]\n'
            'required = false\n'
            'type = "string"\n'
            'default = "In Progress"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'abc123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_config_validation_epic_list_optional(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            '[config.epic_list]\n'
            'required = false\n'
            'type = "string"\n'
            'default = "Epic"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'abc123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    # --- Regex validation in validate-config hook ----------------------------

    def test_regex_validation_pass(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            "validation.regex = '^[a-f0-9]{24}$'\n"
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = '69ec0a2099ea20956e371d62'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_regex_validation_fail(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            "validation.regex = '^[a-f0-9]{24}$'\n"
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'not-a-valid-board-id'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not match required pattern", result.stderr)
        self.assertIn("board_id", result.stderr)
        self.assertIn("not-a-valid-board-id", result.stderr)

    def test_regex_validation_missing_validation_dict(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'valid-board-id-123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_regex_validation_empty_pattern(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
            "validation.regex = ''\n"
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'valid-board-id-123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_shortlink_detection_on_board_id(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = 'AbCd1234'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("shortLink", result.stderr)
        self.assertIn("24 hex characters", result.stderr)

    def test_shortlink_allows_24_hex_board_id(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.board_id]\n'
            'required = true\n'
            'type = "string"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nboard_id = '69ec0a2099ea20956e371d62'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_shortlink_non_board_id_field_ignored(self):
        toml = (
            '[recipe]\n'
            'id = "r"\n'
            'name = "R"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[hooks]]\n'
            'event = "on-sync"\n'
            'action = "validate-config"\n'
            '[config.other_field]\n'
            'required = true\n'
            'type = "string"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={"r": toml})
        root = self._make_project(
            "[recipes.r]\nenabled = true\n"
            "[recipes.r.config]\nother_field = 'AbCd1234'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)

    # --- Integration: trello-mcp-workflow recipe materialization ------------

    def test_materialize_trello_mcp_workflow_recipe(self):
        rid = "trello-mcp-workflow"
        toml = (
            '[recipe]\n'
            f'id = "{rid}"\n'
            'name = "Trello MCP Workflow"\n'
            'description = "Trello-based project tracking"\n'
            'version = "1.0"\n'
            '[[capabilities]]\nid = "tracker"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "validate-config"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "bootstrap-board"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "link-trello-card"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "sync-card-state"\n'
            '[[hooks]]\nevent = "on-sync"\naction = "comment-verification"\n'
            '[config.board_id]\nrequired = true\ntype = "string"\n'
            '[config.default_list]\nrequired = false\ntype = "string"\ndefault = "In Progress"\n'
            '[config.epic_list]\nrequired = false\ntype = "string"\ndefault = "Epic"\n'
            '[[provides.skills]]\nid = "trello-pm-workflow"\nsource = "bundled"\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={rid: toml})
        skill_dir = (
            home / "catalog" / "recipes" / rid / "skills" / "trello-pm-workflow"
        )
        skill_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text("# Trello PM Workflow\n")
        root = self._make_project(
            f"[recipes.{rid}]\nenabled = true\n"
            f"[recipes.{rid}.config]\nboard_id = 'abc123'\n"
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill = (
            cache_project_dir(root, home)
            / ".recipe" / rid / "skills" / "trello-pm-workflow"
        )
        self.assertTrue(skill.is_dir())
        self.assertTrue((skill / "SKILL.md").is_file())
        marker = (
            cache_project_dir(root, home) / ".recipe" / rid / "bootstrap-ready"
        )
        self.assertTrue(marker.is_file())
        self.assertIn("board_id=abc123", marker.read_text())

    def test_materialize_refuses_internal_test_recipe_without_allow_env(self):
        home = _cli_home_with_catalog(self.addCleanup)
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 1)
        self.assertIn("internal test fixture", result.stderr)
        self.assertIn("test-fixture", result.stderr)
        skill = (
            cache_project_dir(root, home)
            / ".recipe" / "test-fixture" / "skills" / "test-skill"
        )
        self.assertFalse(skill.exists())


class ResolvedConfigContextTests(unittest.TestCase):
    """2.3 — RED: resolved-config carries project_root and topology context."""

    def _project(self, *, topology: str | None = None) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        text = (
            "[project]\nname='ctx'\n\n"
            "[agents]\nenabled=['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        if topology is not None:
            text += (
                "[recipes.worktree-flow.config]\n"
                f"repo_topology = '{topology}'\n"
            )
        (ai_specs / "ai-specs.toml").write_text(text)
        return root

    def _cli_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return isolated_home(Path(tmp.name))

    def _sync(self, root: Path):
        home = self._cli_home()
        return home, invoke(root, "sync", cli_home=home)

    def _init_git_with_submodule(self, root: Path):
        subprocess.run(["git", "init", "-q", str(root)], check=True, capture_output=True)
        subdir = root / "submod"
        subdir.mkdir()
        subprocess.run(["git", "init", "-q", str(subdir)], check=True, capture_output=True)
        (subdir / "f.txt").write_text("x\n")
        subprocess.run(["git", "add", "f.txt"], cwd=subdir, check=True, capture_output=True)
        subprocess.run(
            ["git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init"],
            cwd=subdir, check=True, capture_output=True,
        )
        subprocess.run(
            ["git", "-c", "protocol.file.allow=always", "submodule", "add", "-q", "./submod", "submod"],
            cwd=root, check=True, capture_output=True,
        )

    def test_resolved_config_carries_project_root(self):
        # project_root is observable only through the renderer's topology
        # auto-detection — the AGENTS.md line proves detection ran against the
        # resolved project_root (the git repo with an initialized submodule).
        root = self._project()
        self._init_git_with_submodule(root)
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("- **Repo topology**: `monorepo-submodules` (via auto)", agents)

    def test_resolved_config_carries_stable_monorepo_apps_topology(self):
        root = self._project(topology="monorepo-apps")
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("monorepo-apps", agents)
        self.assertIn("(via config)", agents)

    def test_resolved_config_only_also_carries_context(self):
        # the standalone sync-agent path (build_resolved_config_only +
        # resolved-config passthrough) must carry project_root and topology; the
        # subrepo render resolves auto topology against the resolved
        # project_root (workspace root with git + submodule) — had it used the
        # subrepo dir it would resolve standalone.
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'ctx'\nsubrepos = ['packages/a']\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        (root / "packages" / "a").mkdir(parents=True)
        self._init_git_with_submodule(root)
        (root / "AGENTS.md").write_text("placeholder\n")
        home = self._cli_home()
        r1 = invoke(root, "sync", cli_home=home)
        r2 = invoke(root, "sync-agent", "--all", cli_home=home)
        self.assertEqual(r1.returncode, 0, r1.stderr)
        self.assertEqual(r2.returncode, 0, r2.stderr)
        subrepo = (root / "packages" / "a" / "AGENTS.md").read_text()
        self.assertIn("- **Repo topology**: `monorepo-submodules` (via auto)", subrepo)

class RuntimeHookMaterializeTests(unittest.TestCase):
    def _build(self, *, config_section: str = "", config_override: str = "", enabled_agents: str = "'claude'"):
        """Build a fake home (catalog) + project enabling a hook recipe.

        Returns (project_root, home, rid).
        """
        rid = "wt-hook"
        recipe_toml = (
            '[recipe]\n'
            f'id = "{rid}"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            f'{config_section}'
            '[[provides.hooks]]\n'
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            'blocking = true\n'
        )
        home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={rid: recipe_toml})
        recipe_dir = home / "catalog" / "recipes" / rid
        (recipe_dir / "hooks").mkdir()
        (recipe_dir / "hooks" / "gate.sh").write_text("#!/usr/bin/env bash\nexit 0\n")

        proj_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(proj_tmp.cleanup)
        project_root = Path(proj_tmp.name)
        ai_specs = project_root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            f"[agents]\nenabled = [{enabled_agents}]\n\n"
            f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n"
            f"{config_override}"
        )
        return project_root, home, rid

    def test_script_materialized_executable_at_neutral_path(self):
        project_root, home, rid = self._build()
        result = invoke(project_root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        script = project_root / "ai-specs" / "recipes" / rid / "hooks" / "gate.sh"
        self.assertTrue(script.is_file(), "hook script should materialize at neutral path")
        self.assertTrue(os.access(script, os.X_OK), "hook script should be executable")
        self.assertEqual(script.read_text(), "#!/usr/bin/env bash\nexit 0\n")

    def test_resolved_hooks_out_shape(self):
        project_root, home, rid = self._build()
        result = invoke(project_root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        settings = json.loads((project_root / ".claude" / "settings.json").read_text())
        bucket = settings["hooks"]["PreToolUse"]
        self.assertEqual(len(bucket), 1)
        h = bucket[0]
        self.assertEqual(h["_ai_specs_managed"], "ai-specs:hooks:wt-hook:gate")
        self.assertEqual(h["matcher"], "Edit|Write")
        self.assertEqual(len(h["hooks"]), 1)
        self.assertEqual(h["hooks"][0]["type"], "command")
        command = h["hooks"][0]["command"]
        self.assertEqual(command, "$CLAUDE_PROJECT_DIR/ai-specs/recipes/wt-hook/hooks/gate.sh")
        self.assertTrue(
            (project_root / "ai-specs" / "recipes" / rid / "hooks" / "gate.sh").is_file()
        )
        # TRIAGE: ai-specs sync <project> with the wt-hook fixture renders the
        # PreToolUse bucket entry (event), _ai_specs_managed carrying recipe+id,
        # matcher, and the script command, but the Claude settings.json schema has
        # no field exposing the hook's `blocking` flag, so the original
        # assertEqual(h["blocking"], True) is not reachable from the CLI surface.

    def test_resolved_hooks_env_carries_config(self):
        project_root, home, rid = self._build(
            config_section=(
                '[config.WORKTREE_GATE_PROTECTED]\n'
                'required = false\n'
                'type = "string"\n'
                'default = "main"\n'
                '[config.worktrees_dir]\n'
                'required = false\n'
                'type = "string"\n'
                'default = ".worktrees"\n'
            ),
            config_override=(
                "[recipes.wt-hook.config]\nWORKTREE_GATE_PROTECTED = 'main development'\n"
            ),
        )
        result = invoke(project_root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        settings = json.loads((project_root / ".claude" / "settings.json").read_text())
        bucket = settings["hooks"]["PreToolUse"]
        self.assertEqual(len(bucket), 1)
        env = bucket[0]["hooks"][0]["env"]
        # ENV-shaped config keys are exported...
        self.assertEqual(env.get("WORKTREE_GATE_PROTECTED"), "main development")
        # ...recipe-internal lowercase keys are not.
        self.assertNotIn("worktrees_dir", env)




class FragmentsToJsonTests(unittest.TestCase):
    """Task 2.1 — RED: brief fragments observable via rendered AGENTS.md sections."""

    def _make_project(self, root: Path, rid: str) -> None:
        ai_specs = root / "ai-specs"
        (ai_specs / "skills").mkdir(parents=True)
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{rid}]\nenabled = true\n"
        )

    def test_none_input_returns_empty_dict(self):
        """Recipe without brief -> no brief sections in AGENTS.md (CLI cannot pass None)."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._make_project(root, "r")
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "r": '[recipe]\nid = "r"\nname = "R"\ndescription = "D"\nversion = "1.0"\n'
            })
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (root / "AGENTS.md").read_text()
            self.assertTrue((root / "AGENTS.md").is_file())
            self.assertNotIn("## Runtime Flow", agents)
            self.assertNotIn("## Workflow Rules", agents)
            self.assertNotIn("## Context Sources", agents)

    def test_only_workflow_rules_populated(self):
        """Recipe with only workflow_rules -> only that brief section is rendered."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._make_project(root, "r")
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "r": (
                    '[recipe]\nid = "r"\nname = "R"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\nworkflow_rules = ["Do X."]\n'
                )
            })
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (root / "AGENTS.md").read_text()
            self.assertIn("## Workflow Rules", agents)
            self.assertIn("- Do X.", agents)

    def test_key_set_in_output(self):
        """Recipe mcp_descriptions with key -> key rendered in ## Runtime MCPs."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._make_project(root, "r")
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "r": (
                    '[recipe]\nid = "r"\nname = "R"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\nmcp_descriptions = [{ key = "foo", text = "Context here." }]\n'
                )
            })
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (root / "AGENTS.md").read_text()
            # The context_sources renderer intentionally drops fragment keys (frozen
            # renderer behavior), so key-preservation is observed via the mcp_descriptions
            # section which renders `**key** *(global)*`.
            self.assertIn("## Runtime MCPs", agents)
            self.assertIn("**foo** *(global)*", agents)
            self.assertIn("Context here.", agents)

    def test_none_sections_omitted(self):
        """Recipe with only workflow_rules -> all other brief sections omitted."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._make_project(root, "r")
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "r": (
                    '[recipe]\nid = "r"\nname = "R"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\nworkflow_rules = ["Rule."]\n'
                )
            })
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (root / "AGENTS.md").read_text()
            self.assertIn("## Workflow Rules", agents)
            # TRIAGE: `ai-specs sync <project>` always renders `## Useful Commands`
            # via the renderer's unconditional CLI-literacy pointer
            # (lib/_internal/agents-render.py `_section_useful_commands`), so the
            # JSON-level omission of the useful_commands key is not directly
            # observable; the always-present pointer-only section is asserted instead.
            for header in ("## Runtime Flow", "## Context Sources", "## Conflict Policy",
                           "## Runtime MCPs"):
                self.assertNotIn(header, agents)
            self.assertIn("## Useful Commands", agents)

    def test_all_sections_populated(self):
        """Recipe with all six brief sections -> every section header rendered."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._make_project(root, "r")
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "r": (
                    '[recipe]\nid = "r"\nname = "R"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\n'
                    'runtime_flow = ["Flow."]\n'
                    'context_sources = ["Ctx."]\n'
                    'conflict_policy = ["Policy."]\n'
                    'workflow_rules = ["Rule."]\n'
                    'useful_commands = ["Cmd."]\n'
                    'mcp_descriptions = [{ key = "srv", text = "Desc." }]\n'
                )
            })
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (root / "AGENTS.md").read_text()
            for header in ("## Runtime Flow", "## Context Sources", "## Conflict Policy",
                           "## Workflow Rules", "## Useful Commands", "## Runtime MCPs"):
                self.assertIn(header, agents)


class BriefFragmentsMaterializeIntegrationTests(unittest.TestCase):
    """Task 2.3 — RED: brief_fragments attached in both materialize paths."""

    def _make_recipe_with_brief(self, catalog: Path, rid: str, brief_toml: str = "") -> None:
        """Create a minimal recipe.toml with optional [provides.brief] block."""
        recipe_dir = catalog / rid
        recipe_dir.mkdir(parents=True, exist_ok=True)
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\n'
            f'id = "{rid}"\n'
            f'name = "{rid.title()}"\n'
            f'description = "D"\n'
            f'version = "1.0"\n'
            f'{brief_toml}\n'
        )

    def _make_project(self, root: Path, rid: str, config_extra: str = "") -> None:
        ai_specs = root / "ai-specs"
        ai_specs.mkdir(parents=True, exist_ok=True)
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{rid}]\nenabled = true\nversion = \"1.0\"\n"
            f"{config_extra}\n"
        )

    def test_materialize_attaches_brief_fragments_for_recipe_with_brief(self):
        """sync path: recipe with [provides.brief] -> brief fragments in AGENTS.md."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "my-recipe": (
                    '[recipe]\nid = "my-recipe"\nname = "My-Recipe"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\nworkflow_rules = ["Do not push to main directly."]\n'
                )
            })
            project_root = Path(tmp) / "project"
            self._make_project(project_root, "my-recipe")
            result = invoke(project_root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (project_root / "AGENTS.md").read_text()
            self.assertTrue((project_root / "AGENTS.md").is_file())
            self.assertIn("## Workflow Rules", agents)
            self.assertIn("- Do not push to main directly.", agents)
            self.assertNotIn("## Runtime Flow", agents)
            self.assertNotIn("## Conflict Policy", agents)

    def test_materialize_no_brief_fragments_key_absent_for_recipe_without_brief(self):
        """sync path: recipe without [provides.brief] -> no brief sections in AGENTS.md."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "no-brief-recipe": (
                    '[recipe]\nid = "no-brief-recipe"\nname = "No-Brief-Recipe"\ndescription = "D"\nversion = "1.0"\n'
                )
            })
            project_root = Path(tmp) / "project"
            self._make_project(project_root, "no-brief-recipe")
            result = invoke(project_root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            agents = (project_root / "AGENTS.md").read_text()
            for header in ("## Runtime Flow", "## Context Sources", "## Conflict Policy",
                           "## Workflow Rules", "## Runtime MCPs"):
                self.assertNotIn(header, agents)
            # `## Useful Commands` always renders (renderer CLI-literacy pointer);
            # see the TRIAGE note in FragmentsToJsonTests.test_none_sections_omitted.
            self.assertIn("## Useful Commands", agents)

    def test_build_resolved_config_only_attaches_brief_fragments(self):
        """sync-agent --all path: brief fragments forwarded to subrepo AGENTS.md."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _cli_home_with_catalog(self.addCleanup, recipe_tomls={
                "brief-recipe": (
                    '[recipe]\nid = "brief-recipe"\nname = "Brief-Recipe"\ndescription = "D"\nversion = "1.0"\n'
                    '[provides.brief]\nworkflow_rules = ["A rule from brief-recipe."]\n'
                )
            })
            root = Path(tmp)
            (root / "ai-specs" / "skills").mkdir(parents=True)
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname = 'ctx'\nsubrepos = ['packages/a']\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                "[recipes.brief-recipe]\nenabled = true\n"
            )
            (root / "packages" / "a").mkdir(parents=True)
            (root / "AGENTS.md").write_text("placeholder\n")
            r1 = invoke(root, "sync", cli_home=home)
            r2 = invoke(root, "sync-agent", "--all", cli_home=home)
            self.assertEqual(r1.returncode, 0, r1.stderr)
            self.assertEqual(r2.returncode, 0, r2.stderr)
            subrepo_agents = (root / "packages" / "a" / "AGENTS.md").read_text()
            self.assertIn("## Workflow Rules", subrepo_agents)
            self.assertIn("- A rule from brief-recipe.", subrepo_agents)



class StaleCleanupOverrideTests(unittest.TestCase):
    """ADDED Stale Cleanup Override Detection — sync WARN path."""

    @classmethod
    def setUpClass(cls):
        import tomllib
        with open(ROOT / "catalog" / "recipes" / "worktree-flow" / "recipe.toml", "rb") as fh:
            cls.version = tomllib.load(fh)["recipe"]["version"]
    def _sync(self, root: Path):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        return home, invoke(root, "sync", cli_home=home)

    def _make_wf_project(self) -> Path:
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
            f'[recipes.worktree-flow]\nenabled = true\nversion = "{self.version}"\n'
        )
        return root

    def _cleanup_target(self, root: Path) -> Path:
        return (
            root / "ai-specs" / "recipes" / "worktree-flow" / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )

    def _catalog_src(self) -> Path:
        return (
            ROOT / "catalog" / "recipes" / "worktree-flow" / "templates"
            / "worktree-cleanup.sh"
        )

    def test_identical_override_no_stale_warn(self):
        root = self._make_wf_project()
        dest = self._cleanup_target(root)
        dest.parent.mkdir(parents=True, exist_ok=True)
        payload = self._catalog_src().read_bytes()
        dest.write_bytes(payload)
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_bytes(), payload)
        self.assertNotIn("not refreshed", result.stderr)
        self.assertNotIn("condition=not_exists", result.stderr)

    def test_divergent_override_warns_and_sync_succeeds(self):
        root = self._make_wf_project()
        dest = self._cleanup_target(root)
        dest.parent.mkdir(parents=True, exist_ok=True)
        custom = b"# customized override\n"
        dest.write_bytes(custom)
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(dest.read_bytes(), custom)
        err = result.stderr
        self.assertIn("preserving existing file", err)
        self.assertIn("leave it unchanged", err)
        self.assertIn("remove it and run sync again", err)
        self.assertIn("worktree-cleanup.sh", err)
        self.assertIn("rm ", err)
        self.assertIn("ai-specs sync", err)
        self.assertNotIn("user-managed", err.lower())
        self.assertNotIn("customized", err.lower())

    def test_missing_override_gets_fresh_copy(self):
        root = self._make_wf_project()
        dest = self._cleanup_target(root)
        self.assertFalse(dest.exists())
        home, result = self._sync(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(dest.is_file())
        # Sync stamps __WORKTREE_REPO_TOPOLOGY__ (default auto), mirroring gate_mode.
        expected = (
            self._catalog_src()
            .read_text()
            .replace("__WORKTREE_REPO_TOPOLOGY__", "auto")
        )
        self.assertEqual(dest.read_text(), expected)
        self.assertNotIn("__WORKTREE_REPO_TOPOLOGY__", dest.read_text())
        self.assertNotIn("not refreshed", result.stderr)

if __name__ == "__main__":
    unittest.main()
