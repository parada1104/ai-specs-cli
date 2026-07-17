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
        self.assertTrue(
            DEFAULT_MODELS["opencode"].startswith("cursorapi/"),
            DEFAULT_MODELS["opencode"],
        )
        self.assertEqual(DEFAULT_MODELS["opencode"], DEFAULT_MODELS["pi"])
        self.assertEqual(DEFAULT_MODELS["opencode"], DEFAULT_MODELS["omp"])
        self.assertEqual(default_model("opencode"), "cursorapi/composer-2.5")

    def test_opencode_family_rejects_anthropic_model_override(self):
        import os
        from unittest import mock

        from tests.evals.lib.harness import default_model

        with mock.patch.dict(os.environ, {"EVALS_MODEL": "anthropic/claude-sonnet-4-6"}):
            with self.assertRaises(RuntimeError) as ctx:
                default_model("opencode")
            self.assertIn("cursorapi/", str(ctx.exception))

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
            with mock.patch.object(live.shutil, "which", return_value="/bin/claude"):
                with mock.patch.object(live, "api_key_present", return_value=False):
                    self.assertEqual(live._selected_runtimes(), [])
                with mock.patch.object(live, "api_key_present", return_value=True):
                    self.assertEqual(live._selected_runtimes(), ["claude"])

    def test_live_scripts_are_capability_scoped(self):
        plan = (REPO_ROOT / "tests" / "evals" / "run-live.sh").read_text()
        vcs = (REPO_ROOT / "tests" / "evals" / "run-live-vcs.sh").read_text()
        self.assertIn("eval_plan_build_flow_live", plan)
        self.assertNotIn("eval_vcs_pr_flow_live", plan)
        self.assertIn("eval_vcs_pr_flow_live", vcs)
        self.assertNotIn("eval_plan_build_flow_live", vcs)

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
