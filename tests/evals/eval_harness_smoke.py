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
    seed_project_files,
    setup_runtime_skills,
)


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
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "plan-build-flow")
        materialize_project(root, "plan-build-flow", version)
        skill = (
            root
            / "ai-specs"
            / ".recipe"
            / "plan-build-flow"
            / "skills"
            / "plan-build-flow"
            / "SKILL.md"
        )
        plan_cmd = root / "ai-specs" / "commands" / "plan.md"
        build_cmd = root / "ai-specs" / "commands" / "build.md"
        self.assertTrue(skill.is_file())
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
        self.assertEqual(DEFAULT_MODELS["claude"], "opus")
        self.assertIn("deepseek", DEFAULT_MODELS["opencode"])

    def test_live_gate_requires_env(self):
        if live_enabled() and runtime_available():
            self.skipTest("live eval env present — see eval_plan_build_flow_live.py")
        self.assertFalse(live_enabled())


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
