"""Live plan-build-flow behavioral evals (AC3–AC7). Requires EVALS_LIVE=1."""

import fnmatch
import os
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    api_key_present,
    claude_available,
    git_paths_changed,
    init_git_repo,
    live_enabled,
    load_scenario,
    materialize_project,
    run_claude_prompt,
)
from tests.evals.lib.project_fixture import recipe_version  # noqa: E402

SCENARIOS = REPO_ROOT / "tests" / "evals" / "scenarios" / "plan-build-flow"


def _glob_exists(root: Path, pattern: str) -> bool:
    return any(root.glob(pattern))


def _matches_any(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatch(path, pat) for pat in patterns)


@unittest.skipUnless(
    live_enabled() and claude_available() and api_key_present(),
    "Set EVALS_LIVE=1 with claude CLI and ANTHROPIC_API_KEY to run live evals",
)
class PlanBuildFlowLiveEvals(unittest.TestCase):
    def _run_scenario(self, name: str):
        scenario_dir = SCENARIOS / name
        scenario = load_scenario(scenario_dir)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", scenario.recipe_id)
        materialize_project(root, scenario.recipe_id, version)
        init_git_repo(root)
        subprocess_git_add = __import__("subprocess").run
        subprocess_git_add(["git", "add", "-A"], cwd=root, check=True)
        subprocess_git_add(["git", "commit", "-qm", "baseline"], cwd=root, check=True)

        prompt = scenario.prompt_path.read_text()
        result = run_claude_prompt(root, prompt)
        self.assertEqual(
            result["returncode"],
            0,
            msg=f"claude failed: {result.get('stderr')}",
        )

        changed = git_paths_changed(root)
        forbidden = scenario.meta.get("forbidden_path_globs", [])
        for path in changed:
            self.assertFalse(
                _matches_any(path, forbidden),
                f"forbidden path modified: {path}",
            )

        for required in scenario.meta.get("required_path_globs", []):
            self.assertTrue(
                _glob_exists(root, required),
                f"missing required artifact for pattern {required}",
            )

    def test_ac3_plan_stops_before_apply(self):
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        passed = 0
        last_err = None
        for _ in range(trials):
            try:
                self._run_scenario("ac3_plan_stops_before_apply")
                passed += 1
            except AssertionError as exc:
                last_err = exc
        needed = trials if trials == 1 else max(2, (trials * 2) // 3)
        self.assertGreaterEqual(
            passed,
            needed,
            msg=f"N-of-M failed ({passed}/{trials}); last error: {last_err}",
        )


if __name__ == "__main__":
    unittest.main()
