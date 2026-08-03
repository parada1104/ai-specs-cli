"""Deterministic smoke tests for the eval harness (no LLM)."""

import shutil
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    DEFAULT_MODELS,
    assert_natural_prompt,
    claude_available,
    detect_runtime,
    live_enabled,
    load_scenario,
    materialize_project,
    runtime_available,
)
from tests.evals.lib.project_fixture import (  # noqa: E402
    recipe_version,
    resolve_recipe_skill,
    seed_project_files,
    setup_runtime_skills,
)

VCS_RECIPES = ("git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow")
VCS_SKILL_IDS = {
    "git-pr-flow": "git-merge-workflow",
    "gitlab-mr-flow": "gitlab-merge-workflow",
    "bitbucket-pr-flow": "bitbucket-merge-workflow",
}


class HarnessSmokeTests(unittest.TestCase):
    def test_scenario_fixture_loads(self):
        scenario_dir = (
            REPO_ROOT
            / "tests"
            / "evals"
            / "scenarios"
            / "plan-build-flow"
            / "ac3_plan_stops_before_apply"
        )
        scenario = load_scenario(scenario_dir)
        self.assertEqual(scenario.id, "ac3_plan_stops_before_apply")
        self.assertEqual(scenario.recipe_id, "plan-build-flow")
        self.assertEqual(scenario.mode, "plan")
        self.assertTrue(scenario.prompt_path.is_file())
        prompt = scenario.prompt_path.read_text()
        assert_natural_prompt(prompt)

    def test_ac3_prompt_rejects_meta_coaching(self):
        with self.assertRaises(AssertionError):
            assert_natural_prompt("Run the /plan command for this change")

    def test_materialize_plan_build_flow_fixture(self):
        from tests._cache_paths import recipe_skill_dir

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "plan-build-flow")
        materialize_project(root, "plan-build-flow", version)
        skill = (
            recipe_skill_dir(root, "plan-build-flow", "plan-build-flow") / "SKILL.md"
        )
        plan_cmd = root / "ai-specs" / "commands" / "plan.md"
        build_cmd = root / "ai-specs" / "commands" / "build.md"
        self.assertTrue(skill.is_file(), f"missing skill at {skill}")
        self.assertFalse(plan_cmd.exists())
        self.assertFalse(build_cmd.exists())

    def test_setup_runtime_skills_claude(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "plan-build-flow")
        materialize_project(root, "plan-build-flow", version)
        dest = setup_runtime_skills(
            root, "claude", "plan-build-flow", catalog_root=REPO_ROOT / "catalog"
        )
        self.assertTrue(dest.is_file())
        self.assertEqual(dest, root / ".claude" / "skills" / "plan-build-flow" / "SKILL.md")

    def test_setup_runtime_skills_opencode(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        dest = setup_runtime_skills(
            root, "opencode", "plan-build-flow", catalog_root=REPO_ROOT / "catalog"
        )
        self.assertTrue(dest.is_file())

    def test_seed_project_files(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        seed_project_files(root)
        self.assertTrue((root / "src" / "forms" / "signup.py").is_file())

    def test_default_models(self):
        from tests.evals.lib.harness import default_model

        self.assertEqual(DEFAULT_MODELS["claude"], "opus")
        self.assertEqual(DEFAULT_MODELS["cursor-agent"], "composer-2.5")
        self.assertTrue(
            DEFAULT_MODELS["opencode"].startswith("cursorapi/"),
            DEFAULT_MODELS["opencode"],
        )
        self.assertEqual(DEFAULT_MODELS["opencode"], DEFAULT_MODELS["pi"])
        self.assertEqual(DEFAULT_MODELS["opencode"], DEFAULT_MODELS["omp"])
        self.assertEqual(default_model("opencode"), "cursorapi/composer-2.5")
        self.assertEqual(default_model("cursor-agent"), "composer-2.5")

    def test_opencode_family_rejects_anthropic_model_override(self):
        import os
        from unittest import mock

        from tests.evals.lib.harness import default_model

        with mock.patch.dict(os.environ, {"EVALS_MODEL": "anthropic/claude-sonnet-4-6"}):
            with self.assertRaises(RuntimeError) as ctx:
                default_model("opencode")
            self.assertIn("cursorapi/", str(ctx.exception))

    def test_cursor_agent_rejects_cursorapi_model_override(self):
        import os
        from unittest import mock

        from tests.evals.lib.harness import default_model

        with mock.patch.dict(os.environ, {"EVALS_MODEL": "cursorapi/composer-2.5"}):
            with self.assertRaises(RuntimeError) as ctx:
                default_model("cursor-agent")
            self.assertIn("composer-2.5", str(ctx.exception))

    def test_cursor_agent_model_env_uses_underscore(self):
        import os
        from unittest import mock

        from tests.evals.lib.harness import default_model

        env = {
            k: v
            for k, v in os.environ.items()
            if k not in {"EVALS_MODEL", "EVALS_MODEL_CURSOR_AGENT"}
        }
        env["EVALS_MODEL_CURSOR_AGENT"] = "composer-2.5-fast"
        with mock.patch.dict(os.environ, env, clear=True):
            self.assertEqual(default_model("cursor-agent"), "composer-2.5-fast")

    def test_setup_runtime_skills_cursor_agent(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        dest = setup_runtime_skills(
            root, "cursor-agent", "git-pr-flow", catalog_root=REPO_ROOT / "catalog"
        )
        self.assertEqual(
            dest,
            root / ".cursor" / "skills" / "git-merge-workflow" / "SKILL.md",
        )
        self.assertTrue(dest.is_file())

    def test_resolve_runtime_binary_cursor_agent(self):
        import os
        from unittest import mock

        from tests.evals.lib.harness import resolve_runtime_binary, runtime_available

        with mock.patch("tests.evals.lib.harness.shutil.which") as which:
            which.side_effect = lambda name: {
                "cursor-agent": "/bin/cursor-agent",
                "agent": "/bin/agent",
            }.get(name)
            self.assertEqual(resolve_runtime_binary("cursor-agent"), "/bin/cursor-agent")
            self.assertTrue(runtime_available("cursor-agent"))

        with mock.patch("tests.evals.lib.harness.shutil.which") as which:
            which.side_effect = lambda name: {
                "agent": "/bin/agent",
            }.get(name)
            self.assertEqual(resolve_runtime_binary("cursor-agent"), "/bin/agent")

        with mock.patch("tests.evals.lib.harness.shutil.which", return_value=None):
            self.assertIsNone(resolve_runtime_binary("cursor-agent"))
            self.assertFalse(runtime_available("cursor-agent"))

    def test_selected_runtimes_accepts_cursor_agent(self):
        import os
        from unittest import mock

        from tests.evals import eval_vcs_pr_flow_live as live

        with mock.patch.dict(os.environ, {"EVALS_RUNTIMES": "cursor-agent"}, clear=False):
            with mock.patch.object(live, "runtime_available", return_value=True):
                with mock.patch.object(live, "api_key_present", return_value=True):
                    self.assertEqual(live._selected_runtimes(), ["cursor-agent"])

    def test_forbidden_command_line_allows_counterexample_emoji(self):
        from tests.evals.eval_vcs_pr_flow_live import forbidden_command_line_violations

        text = (
            "# merge plan\n"
            "gh pr merge 42 --squash\n"
            "gh pr merge --squash --delete-branch   # ❌\n"
        )
        self.assertEqual(
            forbidden_command_line_violations(
                text,
                starts_with="gh pr merge",
                must_not_contain="--delete-branch",
            ),
            [],
        )
        bad = "gh pr merge 42 --squash --delete-branch\n"
        self.assertEqual(
            forbidden_command_line_violations(
                bad,
                starts_with="gh pr merge",
                must_not_contain="--delete-branch",
            ),
            ["gh pr merge 42 --squash --delete-branch"],
        )

    def test_live_gate_requires_env(self):
        if live_enabled() and runtime_available():
            self.skipTest("live eval env present — see eval_plan_build_flow_live.py")
        self.assertFalse(live_enabled())

    def test_evals_runtimes_honors_credential_gate(self):
        """Forced EVALS_RUNTIMES must still require PATH + credentials."""
        import os
        from unittest import mock

        from tests.evals import eval_plan_build_flow_live as live

        with mock.patch.dict(os.environ, {"EVALS_RUNTIMES": "claude"}, clear=False):
            with mock.patch.object(live, "runtime_available", return_value=True):
                with mock.patch.object(live, "api_key_present", return_value=False):
                    self.assertEqual(live._selected_runtimes(), [])
                with mock.patch.object(live, "api_key_present", return_value=True):
                    self.assertEqual(live._selected_runtimes(), ["claude"])

    def test_live_scripts_are_capability_scoped(self):
        plan = (REPO_ROOT / "tests" / "evals" / "run-live.sh").read_text()
        vcs = (REPO_ROOT / "tests" / "evals" / "run-live-vcs.sh").read_text()
        vault = (REPO_ROOT / "tests" / "evals" / "run-live-vault.sh").read_text()
        self.assertIn("eval_plan_build_flow_live", plan)
        self.assertNotIn("eval_vcs_pr_flow_live", plan)
        self.assertIn("eval_vcs_pr_flow_live", vcs)
        self.assertNotIn("eval_plan_build_flow_live", vcs)
        self.assertIn("eval_vault_canonical_live", vault)
        self.assertNotIn("eval_plan_build_flow_live", vault)
        self.assertNotIn("eval_vcs_pr_flow_live", vault)

    def test_vault_canonical_scenario_fixtures_load(self):
        from tests.evals import eval_vault_canonical_live as vault

        for scenario_id in vault.LIVE_SCENARIOS:
            scenario_dir = (
                REPO_ROOT
                / "tests"
                / "evals"
                / "scenarios"
                / "vault-canonical-store"
                / scenario_id
            )
            scenario = load_scenario(scenario_dir)
            self.assertEqual(scenario.recipe_id, "vault-canonical-store")
            self.assertEqual(scenario.id, scenario_id)
            assert_natural_prompt(scenario.prompt_path.read_text())

    def test_materialize_vault_canonical_store(self):
        from tests._cache_paths import recipe_skill_dir

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "vault-canonical-store")
        materialize_project(root, "vault-canonical-store", version)
        skill = (
            recipe_skill_dir(root, "vault-canonical-store", "vault-context")
            / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing skill at {skill}")
        for kepano_id in (
            "obsidian-markdown",
            "obsidian-bases",
            "json-canvas",
            "obsidian-cli",
            "defuddle",
        ):
            from tests._cache_paths import deps_skill_dir

            dep = deps_skill_dir(root, kepano_id) / "SKILL.md"
            self.assertTrue(dep.is_file(), f"missing kepano dep {dep}")

    def test_vcs_scenario_fixtures_load(self):
        from tests.evals import eval_vcs_pr_flow_live as vcs

        for recipe_id, scenario_id in vcs.LIVE_SCENARIOS:
            scenario_dir = (
                REPO_ROOT / "tests" / "evals" / "scenarios" / recipe_id / scenario_id
            )
            scenario = load_scenario(scenario_dir)
            self.assertEqual(scenario.recipe_id, recipe_id)
            self.assertEqual(scenario.id, scenario_id)
            assert_natural_prompt(scenario.prompt_path.read_text())

    def test_resolve_vcs_skill_ids(self):
        for recipe_id, skill_id in VCS_SKILL_IDS.items():
            path, resolved = resolve_recipe_skill(
                recipe_id, catalog_root=REPO_ROOT / "catalog"
            )
            self.assertEqual(resolved, skill_id)
            self.assertTrue(path.is_file())

    def test_materialize_and_setup_vcs_recipes(self):
        for recipe_id in VCS_RECIPES:
            with self.subTest(recipe=recipe_id):
                tmp = tempfile.TemporaryDirectory()
                self.addCleanup(tmp.cleanup)
                root = Path(tmp.name)
                version = recipe_version(REPO_ROOT / "catalog", recipe_id)
                materialize_project(root, recipe_id, version)
                dest = setup_runtime_skills(
                    root, "claude", recipe_id, catalog_root=REPO_ROOT / "catalog"
                )
                skill_id = VCS_SKILL_IDS[recipe_id]
                self.assertEqual(
                    dest,
                    root / ".claude" / "skills" / skill_id / "SKILL.md",
                )
                self.assertTrue(dest.is_file())


    def test_trello_mcp_workflow_scenario_fixtures_load(self):
        from tests.evals import eval_trello_mcp_workflow_live as trello

        for scenario_id in trello.LIVE_SCENARIOS:
            scenario_dir = (
                REPO_ROOT
                / "tests"
                / "evals"
                / "scenarios"
                / "trello-mcp-workflow"
                / scenario_id
            )
            scenario = load_scenario(scenario_dir)
            self.assertEqual(scenario.recipe_id, "trello-mcp-workflow")
            self.assertEqual(scenario.id, scenario_id)
            assert_natural_prompt(scenario.prompt_path.read_text())

    def test_run_live_trello_script_points_at_client(self):
        script = (REPO_ROOT / "tests" / "evals" / "run-live-trello.sh").read_text()
        self.assertIn("eval_trello_mcp_workflow_live", script)
        self.assertIn("client=trello-mcp-workflow", script)
        self.assertNotIn("eval_worktree_flow_live", script)


class VaultMcpLiveHelperTests(unittest.TestCase):
    def test_create_scoped_vault_and_sync_mcp_config(self):
        from tests.evals.lib.vault_mcp_live import (
            cleanup_vault,
            create_scoped_vault,
            sync_vault_mcp,
        )

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        vault = create_scoped_vault()
        self.addCleanup(cleanup_vault, vault)
        self.assertTrue((vault["scoped"] / "MARKER.md").is_file())
        self.assertIn("VAULT_LIVE_", vault["token"])
        self.assertNotEqual(vault["token"], vault["sibling"])

        version = recipe_version(REPO_ROOT / "catalog", "vault-canonical-store")
        materialize_project(root, "vault-canonical-store", version)
        cfg = sync_vault_mcp(root, "claude")
        self.assertEqual(cfg, root / ".mcp.json")
        text = cfg.read_text()
        self.assertIn("vault-canonical", text)
        self.assertIn("vault-fs-mcp.sh", text)
        self.assertIn("CANONICAL_VAULT_PATH", text)


class HarnessPathTests(unittest.TestCase):
    def test_runtime_binary_optional(self):
        if not detect_runtime():
            self.skipTest("no supported runtime CLI installed")
        self.assertTrue(runtime_available())

    def test_claude_binary_optional(self):
        if not shutil.which("claude"):
            self.skipTest("claude CLI not installed")
        self.assertTrue(claude_available())



if __name__ == "__main__":
    unittest.main()
