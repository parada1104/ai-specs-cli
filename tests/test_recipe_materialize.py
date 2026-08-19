import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import (  # noqa: E402
    ALLOW_INTERNAL_TEST_RECIPES_ENV,
    allow_internal_test_recipes_env,
    populate_catalog,
    unit_catalog,
)
from _blackbox import cache_project_dir, invoke, isolated_home


RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
CATALOG = unit_catalog()
FIXTURE_HOME: Path | None = None


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module

def _cache_mod():
    path = ROOT / "lib" / "_internal" / "project-cache.py"
    return load_module(path, "project_cache_for_tests")


def _home() -> Path:
    assert FIXTURE_HOME is not None
    return FIXTURE_HOME


def cache_recipe_skill(project_root: Path, recipe_id: str, skill_id: str, cli_home: Path | None = None) -> Path:
    pc = _cache_mod()
    home = _home() if cli_home is None else cli_home
    return pc.recipe_skills_root(project_root, cli_home=home) / recipe_id / "skills" / skill_id


def cache_command(project_root: Path, cmd_id: str, cli_home: Path | None = None) -> Path:
    pc = _cache_mod()
    home = _home() if cli_home is None else cli_home
    return pc.commands_dir(project_root, cli_home=home) / f"{cmd_id}.md"


def cache_dep_skill(project_root: Path, dep_id: str, cli_home: Path | None = None) -> Path:
    pc = _cache_mod()
    home = _home() if cli_home is None else cli_home
    return pc.deps_skills_root(project_root, cli_home=home) / dep_id / "skills" / dep_id


class RecipeMaterializeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        global FIXTURE_HOME
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")
        cls._home_tmpdir = tempfile.TemporaryDirectory()
        FIXTURE_HOME = Path(cls._home_tmpdir.name)
        populate_catalog(FIXTURE_HOME / "catalog" / "recipes")

    @classmethod
    def tearDownClass(cls):
        global FIXTURE_HOME
        cls._home_tmpdir.cleanup()
        FIXTURE_HOME = None

    def setUp(self):
        self._allow = mock.patch.dict(os.environ, allow_internal_test_recipes_env())
        self._allow.start()
        self.addCleanup(self._allow.stop)

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

    def _make_v2_recipe(self, tmp: str, rid: str, caps: list[str] = None, hooks: list[dict] = None, config: dict = None, skills: list[str] = None):
        recipe_dir = Path(tmp) / rid
        recipe_dir.mkdir()
        cap_lines = "".join(f'[[capabilities]]\nid = "{c}"\n' for c in (caps or []))
        hook_lines = "".join(f'[[hooks]]\nevent = "{h["event"]}"\naction = "{h["action"]}"\n' for h in (hooks or []))
        config_lines = ""
        for key, field in (config or {}).items():
            config_lines += f"[config.{key}]\n"
            for fk, fv in field.items():
                if isinstance(fv, bool):
                    config_lines += f"{fk} = {str(fv).lower()}\n"
                elif isinstance(fv, str):
                    config_lines += f'{fk} = "{fv}"\n'
                else:
                    config_lines += f"{fk} = {fv}\n"
        skill_lines = ""
        for sid in (skills or []):
            skill_lines += f'[[provides.skills]]\nid = "{sid}"\nsource = "bundled"\n'
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\ndescription = "D"\nversion = "1.0"\n'
            + cap_lines + hook_lines + config_lines + skill_lines
        )
        # Create dummy skill dirs for bundled skills
        for sid in (skills or []):
            (recipe_dir / "skills" / sid).mkdir(parents=True)
            (recipe_dir / "skills" / sid / "SKILL.md").write_text("skill")

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
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="unknown")]
        )
        # Should warn but not raise
        self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))

    def test_end_to_end_v2_recipe_with_config_and_hooks(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            ai_specs_home = Path(tmp)
            catalog = ai_specs_home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            self._make_v2_recipe(str(catalog), "v2-recipe", caps=["tracker"], hooks=[{"event": "on-sync", "action": "validate-config"}],
                config={"board_id": {"required": True, "type": "string"}}, skills=["v2-skill"])
            # Also create command
            (catalog / "v2-recipe" / "commands").mkdir()
            (catalog / "v2-recipe" / "commands" / "v2-cmd.md").write_text("cmd")
            (catalog / "v2-recipe" / "templates").mkdir()
            (catalog / "v2-recipe" / "templates" / "tpl.md").write_text("tpl")
            (catalog / "v2-recipe" / "docs").mkdir()
            (catalog / "v2-recipe" / "docs" / "doc.md").write_text("doc")

            root = Path(tmp) / "project"
            ai_specs = root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            manifest = ai_specs / "ai-specs.toml"
            manifest.write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                "[recipes.v2-recipe]\nenabled = true\nversion = \"1.0\"\n"
                "[recipes.v2-recipe.config]\nboard_id = 'abc123'\n"
            )
            self.assertEqual(self.mod.materialize_recipes(root, ai_specs_home), 0)
            self.assertTrue(
                cache_recipe_skill(root, "v2-recipe", "v2-skill", cli_home=ai_specs_home).is_dir()
            )

    def test_v1_manifest_without_bindings_or_config_succeeds(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, _home()), 0)

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
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp)
            self.mod.execute_hooks(recipe, {"board_id": "b1"}, project_root)
            marker = _cache_mod().recipe_skills_root(project_root, cli_home=ROOT) / "r" / "bootstrap-ready"
            self.assertTrue(marker.is_file())
            self.assertIn("board_id=b1", marker.read_text())

    def test_execute_hooks_project_root_different_paths(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            hooks=[Hook(event="on-sync", action="bootstrap-board")]
        )
        with tempfile.TemporaryDirectory() as tmp1:
            with tempfile.TemporaryDirectory() as tmp2:
                root1 = Path(tmp1)
                root2 = Path(tmp2)
                cfg = {"board_id": "board-1", "default_list": "List1", "epic_list": "Epic1"}
                self.mod.execute_hooks(recipe, cfg, root1)
                cfg2 = {"board_id": "board-2", "default_list": "List2", "epic_list": "Epic2"}
                self.mod.execute_hooks(recipe, cfg2, root2)
                m1 = _cache_mod().recipe_skills_root(root1, cli_home=ROOT) / "r" / "bootstrap-ready"
                m2 = _cache_mod().recipe_skills_root(root2, cli_home=ROOT) / "r" / "bootstrap-ready"
                self.assertTrue(m1.is_file())
                self.assertTrue(m2.is_file())
                self.assertIn("board_id=board-1", m1.read_text())
                self.assertIn("board_id=board-2", m2.read_text())

    # --- Config validation: board_id / optional fields -----------------------

    def test_config_validation_board_id_required(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={"board_id": ConfigField(required=True)}),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.execute_hooks(recipe, {}, Path(tempfile.gettempdir()))
        self.assertIn("validate-config", str(ctx.exception))

    def test_config_validation_default_list_optional(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
                "default_list": ConfigField(required=False, default="In Progress"),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        self.mod.execute_hooks(recipe, {"board_id": "b1"}, Path(tempfile.gettempdir()))

    def test_config_validation_epic_list_optional(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
                "epic_list": ConfigField(required=False, default="Epic"),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        self.mod.execute_hooks(recipe, {"board_id": "b1"}, Path(tempfile.gettempdir()))

    # --- Regex validation in validate-config hook ----------------------------

    def test_regex_validation_pass(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True, validation={"regex": "^[a-f0-9]{24}$"}),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # Valid 24-char hex string
        self.mod.execute_hooks(recipe, {"board_id": "69ec0a2099ea20956e371d62"}, Path(tempfile.gettempdir()))

    def test_regex_validation_fail(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True, validation={"regex": "^[a-f0-9]{24}$"}),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.execute_hooks(recipe, {"board_id": "not-a-valid-board-id"}, Path(tempfile.gettempdir()))
        self.assertIn("does not match required pattern", str(ctx.exception))
        self.assertIn("board_id", str(ctx.exception))
        self.assertIn("not-a-valid-board-id", str(ctx.exception))

    def test_regex_validation_missing_validation_dict(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),  # no validation dict
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # Should not raise even though board_id has no regex
        self.mod.execute_hooks(recipe, {"board_id": "valid-board-id-123"}, Path(tempfile.gettempdir()))

    def test_regex_validation_empty_pattern(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True, validation={"regex": ""}),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # Pattern is empty string — regex should be skipped
        self.mod.execute_hooks(recipe, {"board_id": "valid-board-id-123"}, Path(tempfile.gettempdir()))

    def test_shortlink_detection_on_board_id(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        with self.assertRaises(RuntimeError) as ctx:
            # 8 alphanumeric chars looks like a Trello shortLink
            self.mod.execute_hooks(recipe, {"board_id": "AbCd1234"}, Path(tempfile.gettempdir()))
        self.assertIn("shortLink", str(ctx.exception))
        self.assertIn("24 hex characters", str(ctx.exception))

    def test_shortlink_allows_24_hex_board_id(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "board_id": ConfigField(required=True),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # Full 24 hex char board ID should pass shortLink detection
        self.mod.execute_hooks(recipe, {"board_id": "69ec0a2099ea20956e371d62"}, Path(tempfile.gettempdir()))

    def test_shortlink_non_board_id_field_ignored(self):
        import tempfile
        from lib._internal.recipe_schema import Recipe, ConfigSchema, ConfigField, Hook
        recipe = Recipe(id="r", name="R", description="D", version="1.0",
            config_schema=ConfigSchema(fields={
                "other_field": ConfigField(required=True),
            }),
            hooks=[Hook(event="on-sync", action="validate-config")]
        )
        # 8 chars on a non-board_id field should not trigger shortLink error
        self.mod.execute_hooks(recipe, {"other_field": "AbCd1234"}, Path(tempfile.gettempdir()))

    # --- Integration: trello-mcp-workflow recipe materialization ------------

    def test_materialize_trello_mcp_workflow_recipe(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            ai_specs_home = Path(tmp)
            catalog = ai_specs_home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            rid = "trello-mcp-workflow"
            recipe_dir = catalog / rid
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
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
            skill_dir = recipe_dir / "skills" / "trello-pm-workflow"
            skill_dir.mkdir(parents=True)
            (skill_dir / "SKILL.md").write_text("# Trello PM Workflow\n")
            project_root = Path(tempfile.mkdtemp())
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'test-project'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n"
                f"[recipes.{rid}.config]\nboard_id = 'abc123'\n"
            )
            result = self.mod.materialize_recipes(project_root, ai_specs_home)
            self.assertEqual(result, 0)
            skill_path = cache_recipe_skill(
                project_root, rid, "trello-pm-workflow", cli_home=ai_specs_home
            )
            self.assertTrue(skill_path.is_dir())
            self.assertTrue((skill_path / "SKILL.md").is_file())
            marker = (
                _cache_mod().recipe_skills_root(project_root, cli_home=ai_specs_home)
                / rid
                / "bootstrap-ready"
            )
            self.assertTrue(marker.is_file())
            marker_content = marker.read_text()
            self.assertIn("board_id=abc123", marker_content)

    def test_materialize_refuses_internal_test_recipe_without_allow_env(self):
        self._allow.stop()
        os.environ.pop(ALLOW_INTERNAL_TEST_RECIPES_ENV, None)
        public_home = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(public_home, ignore_errors=True))
        populate_catalog(public_home / "catalog" / "recipes", include_fixtures=False)
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        with self.assertRaises(SystemExit) as ctx:
            self.mod.materialize_recipes(root, public_home)
        self.assertEqual(ctx.exception.code, 1)
        skill = cache_recipe_skill(root, "test-fixture", "test-skill", cli_home=public_home)
        self.assertFalse(skill.exists())


class ResolvedConfigContextTests(unittest.TestCase):
    """2.3 — RED: resolved-config carries project_root and topology context."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_ctx")

    def _project(self, *, topology: str | None = None) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        text = "[project]\nname='ctx'\n\n[agents]\nenabled=['claude']\n"
        if topology is not None:
            text += (
                "[recipes.worktree-flow]\nenabled = true\n"
                "[recipes.worktree-flow.config]\n"
                f"repo_topology = '{topology}'\n"
            )
        (ai_specs / "ai-specs.toml").write_text(text)
        return root

    def test_resolved_config_carries_project_root(self):
        root = self._project()
        out = root / "resolved.json"
        self.assertEqual(self.mod.materialize_recipes(root, ROOT, resolved_config_out=out), 0)
        data = json.loads(out.read_text())
        self.assertEqual(data["project_root"], str(root.resolve()))

    def test_resolved_config_carries_stable_monorepo_apps_topology(self):
        root = self._project(topology="monorepo-apps")
        out = root / "resolved.json"
        self.assertEqual(self.mod.materialize_recipes(root, ROOT, resolved_config_out=out), 0)
        data = json.loads(out.read_text())
        self.assertEqual(data["topology"]["resolved"], "monorepo-apps")
        self.assertEqual(data["topology"]["via"], "config")

    def test_resolved_config_only_also_carries_context(self):
        root = self._project()
        out = root / "resolved-only.json"
        self.assertEqual(
            self.mod.build_resolved_config_only(root, out, ROOT), 0
        )
        data = json.loads(out.read_text())
        self.assertEqual(data["project_root"], str(root.resolve()))
        self.assertIn("topology", data)


class RuntimeHookMaterializeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal_rh")

    def _build(self, *, config_section: str = "", config_override: str = "", enabled_agents: str = "'claude'"):
        """Build a fake home (catalog) + project enabling a hook recipe.

        Returns (project_root, ai_specs_home).
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name)
        catalog = home / "catalog" / "recipes"
        rid = "wt-hook"
        recipe_dir = catalog / rid
        (recipe_dir / "hooks").mkdir(parents=True)
        (recipe_dir / "recipe.toml").write_text(
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
        (recipe_dir / "hooks" / "gate.sh").write_text("#!/usr/bin/env bash\nexit 0\n")

        proj_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(proj_tmp.cleanup)
        project_root = Path(proj_tmp.name)
        ai_specs = project_root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            f"[agents]\nenabled = [{enabled_agents}]\n\n"
            f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n"
            f"{config_override}"
        )
        return project_root, home, rid

    def test_script_materialized_executable_at_neutral_path(self):
        project_root, home, rid = self._build()
        self.assertEqual(self.mod.materialize_recipes(project_root, home), 0)
        script = project_root / "ai-specs" / "recipes" / rid / "hooks" / "gate.sh"
        self.assertTrue(script.is_file(), "hook script should materialize at neutral path")
        import os
        self.assertTrue(os.access(script, os.X_OK), "hook script should be executable")

    def test_resolved_hooks_out_shape(self):
        project_root, home, rid = self._build()
        out = project_root / "hooks.json"
        self.assertEqual(
            self.mod.materialize_recipes(project_root, home, resolved_hooks_out=out), 0
        )
        data = json.loads(out.read_text())
        self.assertIn("claude", data["enabled_agents"])
        self.assertEqual(len(data["hooks"]), 1)
        h = data["hooks"][0]
        self.assertEqual(h["recipe"], rid)
        self.assertEqual(h["id"], "gate")
        self.assertEqual(h["event"], "pre-tool-use")
        self.assertEqual(h["matcher"], "Edit|Write")
        self.assertEqual(h["blocking"], True)
        self.assertEqual(
            h["script_path"], f"ai-specs/recipes/{rid}/hooks/gate.sh"
        )

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
        out = project_root / "hooks.json"
        self.assertEqual(
            self.mod.materialize_recipes(project_root, home, resolved_hooks_out=out), 0
        )
        data = json.loads(out.read_text())
        env = data["hooks"][0]["env"]
        # ENV-shaped config keys are exported...
        self.assertEqual(env.get("WORKTREE_GATE_PROTECTED"), "main development")
        # ...recipe-internal lowercase keys are not.
        self.assertNotIn("worktrees_dir", env)


class FragmentsToJsonTests(unittest.TestCase):
    """Task 2.1 — RED: _fragments_to_json helper."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_frag_json")
        # Load recipe_schema to build BriefFragment/BriefFragments objects
        schema_path = ROOT / "lib" / "_internal" / "recipe_schema.py"
        cls.schema = load_module(schema_path, "recipe_schema_frag_json")

    def test_none_input_returns_empty_dict(self):
        """_fragments_to_json(None) -> {}."""
        result = self.mod._fragments_to_json(None)
        self.assertEqual(result, {})

    def test_only_workflow_rules_populated(self):
        """BriefFragments with only workflow_rules -> {'workflow_rules': [{...}]}."""
        bf = self.schema.BriefFragments(
            workflow_rules=[self.schema.BriefFragment(text="Do X.")]
        )
        result = self.mod._fragments_to_json(bf)
        self.assertIn("workflow_rules", result)
        self.assertEqual(result["workflow_rules"], [{"key": None, "text": "Do X."}])

    def test_key_set_in_output(self):
        """BriefFragments with key set -> key appears in output dict."""
        bf = self.schema.BriefFragments(
            context_sources=[self.schema.BriefFragment(text="Context here.", key="foo")]
        )
        result = self.mod._fragments_to_json(bf)
        self.assertIn("context_sources", result)
        self.assertEqual(result["context_sources"], [{"key": "foo", "text": "Context here."}])

    def test_none_sections_omitted(self):
        """Sections with None value are omitted from output dict."""
        bf = self.schema.BriefFragments(
            workflow_rules=[self.schema.BriefFragment(text="Rule.")],
            # all others default to None
        )
        result = self.mod._fragments_to_json(bf)
        self.assertIn("workflow_rules", result)
        # None sections should NOT appear
        for section in ("runtime_flow", "context_sources", "conflict_policy",
                        "useful_commands", "mcp_descriptions"):
            self.assertNotIn(section, result, f"Expected {section} to be omitted")

    def test_all_sections_populated(self):
        """All sections populated -> all appear in output."""
        bf = self.schema.BriefFragments(
            runtime_flow=[self.schema.BriefFragment(text="Flow.")],
            context_sources=[self.schema.BriefFragment(text="Ctx.", key="k1")],
            conflict_policy=[self.schema.BriefFragment(text="Policy.")],
            workflow_rules=[self.schema.BriefFragment(text="Rule.")],
            useful_commands=[self.schema.BriefFragment(text="Cmd.")],
            mcp_descriptions=[self.schema.BriefFragment(text="Desc.", key="srv")],
        )
        result = self.mod._fragments_to_json(bf)
        self.assertEqual(len(result), 6)


class BriefFragmentsMaterializeIntegrationTests(unittest.TestCase):
    """Task 2.3 — RED: brief_fragments attached in both materialize paths."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_bf_integ")

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
        """materialize_recipes path: recipe with [provides.brief] -> resolved has brief_fragments."""
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp) / "home"
            home.mkdir()
            catalog = home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            self._make_recipe_with_brief(
                catalog, "my-recipe",
                '[provides.brief]\n'
                'workflow_rules = ["Do not push to main directly."]\n'
            )
            project_root = Path(tmp) / "project"
            self._make_project(project_root, "my-recipe")

            resolved_out = project_root / "resolved.json"
            result = self.mod.materialize_recipes(
                project_root, home,
                resolved_config_out=resolved_out
            )
            self.assertEqual(result, 0)
            self.assertTrue(resolved_out.is_file())
            data = json.loads(resolved_out.read_text())
            recipe_entry = data["recipes"].get("my-recipe", {})
            self.assertIn("brief_fragments", recipe_entry)
            bf = recipe_entry["brief_fragments"]
            self.assertIn("workflow_rules", bf)
            self.assertEqual(bf["workflow_rules"][0]["text"], "Do not push to main directly.")
            self.assertIsNone(bf["workflow_rules"][0]["key"])

    def test_materialize_no_brief_fragments_key_absent_for_recipe_without_brief(self):
        """materialize_recipes path: recipe without [provides.brief] -> brief_fragments absent or {}."""
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp) / "home"
            home.mkdir()
            catalog = home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            self._make_recipe_with_brief(catalog, "no-brief-recipe", "")
            project_root = Path(tmp) / "project"
            self._make_project(project_root, "no-brief-recipe")

            resolved_out = project_root / "resolved.json"
            result = self.mod.materialize_recipes(
                project_root, home,
                resolved_config_out=resolved_out
            )
            self.assertEqual(result, 0)
            data = json.loads(resolved_out.read_text())
            recipe_entry = data["recipes"].get("no-brief-recipe", {})
            # brief_fragments should be absent or empty dict (both are acceptable)
            bf = recipe_entry.get("brief_fragments", {})
            self.assertEqual(bf, {})

    def test_build_resolved_config_only_attaches_brief_fragments(self):
        """build_resolved_config_only path: recipe with [provides.brief] -> brief_fragments in output."""
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp) / "home"
            home.mkdir()
            catalog = home / "catalog" / "recipes"
            catalog.mkdir(parents=True)
            self._make_recipe_with_brief(
                catalog, "brief-recipe",
                '[provides.brief]\n'
                'workflow_rules = ["A rule from brief-recipe."]\n'
            )
            project_root = Path(tmp) / "project"
            self._make_project(project_root, "brief-recipe")

            resolved_out = project_root / "resolved.json"
            result = self.mod.build_resolved_config_only(project_root, resolved_out, home)
            self.assertEqual(result, 0)
            data = json.loads(resolved_out.read_text())
            recipe_entry = data["recipes"].get("brief-recipe", {})
            self.assertIn("brief_fragments", recipe_entry)
            bf = recipe_entry["brief_fragments"]
            self.assertIn("workflow_rules", bf)
            self.assertEqual(bf["workflow_rules"][0]["text"], "A rule from brief-recipe.")



class StaleCleanupOverrideTests(unittest.TestCase):
    """ADDED Stale Cleanup Override Detection — sync WARN path."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_stale_override")
        import tomllib
        with open(ROOT / "catalog" / "recipes" / "worktree-flow" / "recipe.toml", "rb") as fh:
            cls.version = tomllib.load(fh)["recipe"]["version"]

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
        buf = io.StringIO()
        with mock.patch.object(self.mod.sys, "stderr", buf):
            rc = self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(rc, 0)
        self.assertEqual(dest.read_bytes(), payload)
        self.assertNotIn("not refreshed", buf.getvalue())
        self.assertNotIn("condition=not_exists", buf.getvalue())

    def test_divergent_override_warns_and_sync_succeeds(self):
        root = self._make_wf_project()
        dest = self._cleanup_target(root)
        dest.parent.mkdir(parents=True, exist_ok=True)
        custom = b"# customized override\n"
        dest.write_bytes(custom)
        buf = io.StringIO()
        with mock.patch.object(self.mod.sys, "stderr", buf):
            rc = self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(rc, 0)
        self.assertEqual(dest.read_bytes(), custom)
        err = buf.getvalue()
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
        buf = io.StringIO()
        with mock.patch.object(self.mod.sys, "stderr", buf):
            rc = self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(rc, 0)
        self.assertTrue(dest.is_file())
        # Sync stamps __WORKTREE_REPO_TOPOLOGY__ (default auto), mirroring gate_mode.
        expected = (
            self._catalog_src()
            .read_text()
            .replace("__WORKTREE_REPO_TOPOLOGY__", "auto")
        )
        self.assertEqual(dest.read_text(), expected)
        self.assertNotIn("__WORKTREE_REPO_TOPOLOGY__", dest.read_text())
        self.assertNotIn("not refreshed", buf.getvalue())

if __name__ == "__main__":
    unittest.main()
