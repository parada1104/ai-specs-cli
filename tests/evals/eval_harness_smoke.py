"""Deterministic smoke tests for the eval harness (no LLM)."""

import importlib.util
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    api_key_present,
    claude_available,
    init_git_repo,
    live_enabled,
    load_scenario,
    materialize_project,
)
from tests.evals.lib.project_fixture import recipe_version  # noqa: E402


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
        self.assertTrue(scenario.prompt_path.is_file())

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

    def test_live_gate_requires_env(self):
        if live_enabled() and claude_available() and api_key_present():
            self.skipTest("live eval env present — see eval_plan_build_flow_live.py")
        self.assertFalse(live_enabled())


class HarnessPathTests(unittest.TestCase):
    def test_claude_binary_optional(self):
        # Document expectation; never fail CI for missing CLI
        if not shutil.which("claude"):
            self.skipTest("claude CLI not installed")


if __name__ == "__main__":
    unittest.main()
