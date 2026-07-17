"""Live vcs-pr-flow sibling recipe evals. Requires EVALS_LIVE=1."""

from __future__ import annotations

import fnmatch
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    SUPPORTED_RUNTIMES,
    api_key_present,
    git_paths_changed,
    init_git_repo,
    live_enabled,
    load_scenario,
    materialize_project,
    run_prompt,
)
from tests.evals.lib.project_fixture import (  # noqa: E402
    recipe_version,
    seed_project_files,
    setup_runtime_skills,
)

SCENARIOS_ROOT = REPO_ROOT / "tests" / "evals" / "scenarios"

# (recipe_id, scenario_id)
LIVE_SCENARIOS: tuple[tuple[str, str], ...] = (
    ("git-pr-flow", "ac_protected_head_no_delete"),
    ("git-pr-flow", "ac_feature_head_cleanup"),
    ("git-pr-flow", "ac_delete_branch_on_merge_warn"),
    ("git-pr-flow", "ac_release_head_preferred"),
    ("gitlab-mr-flow", "ac_protected_head_no_delete"),
    ("gitlab-mr-flow", "ac_feature_head_cleanup"),
    ("gitlab-mr-flow", "ac_release_head_preferred"),
    ("bitbucket-pr-flow", "ac_protected_head_no_delete"),
    ("bitbucket-pr-flow", "ac_feature_head_cleanup"),
    ("bitbucket-pr-flow", "ac_release_head_preferred"),
)


def _glob_exists(root: Path, pattern: str) -> bool:
    return any(root.glob(pattern))


def _matches_any(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatch(path, pat) for pat in patterns)


def _selected_runtimes() -> list[str]:
    raw = os.environ.get("EVALS_RUNTIMES") or os.environ.get("EVALS_RUNTIME", "")
    if raw.strip():
        names = [r.strip() for r in raw.split(",") if r.strip()]
    else:
        names = [
            n.strip()
            for n in os.environ.get("EVALS_PREFER", "opencode,pi,omp,claude").split(",")
            if n.strip()
        ]
    out: list[str] = []
    for name in names:
        if name not in SUPPORTED_RUNTIMES:
            continue
        if not shutil.which(name):
            continue
        if not api_key_present(name):
            continue
        out.append(name)
    return out


def _selected_scenarios() -> list[tuple[str, str]]:
    raw = os.environ.get("EVALS_SCENARIOS", "")
    if not raw.strip():
        return list(LIVE_SCENARIOS)
    selected: list[tuple[str, str]] = []
    for token in raw.split(","):
        token = token.strip()
        if not token:
            continue
        if "/" in token:
            recipe_id, scenario_id = token.split("/", 1)
            selected.append((recipe_id, scenario_id))
            continue
        # Bare scenario id: all recipes that define it
        for recipe_id, scenario_id in LIVE_SCENARIOS:
            if scenario_id == token:
                selected.append((recipe_id, scenario_id))
    return selected


def _n_of_m(passed: int, trials: int) -> int:
    return trials if trials == 1 else max(2, (trials * 2) // 3)


def _key(recipe_id: str, scenario_id: str) -> str:
    return f"{recipe_id}/{scenario_id}"


@unittest.skipUnless(
    live_enabled() and bool(_selected_runtimes()),
    "Set EVALS_LIVE=1 with a supported runtime on PATH to run live VCS evals",
)
class VcsPrFlowLiveEvals(unittest.TestCase):
    def _run_scenario(self, recipe_id: str, scenario_id: str, runtime: str):
        scenario_dir = SCENARIOS_ROOT / recipe_id / scenario_id
        scenario = load_scenario(scenario_dir)
        meta = scenario.meta
        self.assertEqual(scenario.recipe_id, recipe_id)

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", recipe_id)
        materialize_project(root, recipe_id, version)
        seed_project_files(root)
        setup_runtime_skills(
            root, runtime, recipe_id, catalog_root=REPO_ROOT / "catalog"
        )

        init_git_repo(root)
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(["git", "commit", "-qm", "baseline"], cwd=root, check=True)

        prompt = scenario.prompt_path.read_text()
        old = os.environ.get("EVALS_RUNTIME")
        os.environ["EVALS_RUNTIME"] = runtime
        try:
            result = run_prompt(root, prompt, runtime=runtime, mode=scenario.mode)
        finally:
            if old is None:
                os.environ.pop("EVALS_RUNTIME", None)
            else:
                os.environ["EVALS_RUNTIME"] = old

        label = f"{runtime}/{_key(recipe_id, scenario_id)}"
        changed = git_paths_changed(root)

        soft_ok = False
        if result.get("timed_out"):
            required_ok = all(
                _glob_exists(root, required)
                for required in meta.get("required_path_globs", [])
            )
            soft_ok = bool(meta.get("required_path_globs")) and required_ok

        if not soft_ok:
            self.assertEqual(
                result["returncode"],
                0,
                msg=(
                    f"{label} failed rc={result.get('returncode')} "
                    f"timed_out={result.get('timed_out')}\n"
                    f"stderr={result.get('stderr')}\n"
                    f"stdout_tail={(result.get('stdout') or '')[-2000:]}\n"
                    f"cmd={result.get('cmd')}\n"
                    f"changed={changed}"
                ),
            )

        forbidden = meta.get("forbidden_path_globs", [])
        for path in changed:
            self.assertFalse(
                _matches_any(path, forbidden),
                f"{label}: forbidden path modified: {path}",
            )

        for required in meta.get("required_path_globs", []):
            self.assertTrue(
                _glob_exists(root, required),
                f"{label}: missing {required}; changed={changed}",
            )

        for rule in meta.get("required_content", []):
            path = root / rule["path"]
            self.assertTrue(path.is_file(), f"{label}: missing {rule['path']}")
            text = path.read_text(encoding="utf-8", errors="replace").lower()
            needles = [str(n).lower() for n in rule.get("contains_any", [])]
            self.assertTrue(
                any(n in text for n in needles),
                f"{label}: {rule['path']} missing any of {needles}\n---\n{text[:2000]}",
            )

        for rule in meta.get("forbidden_content", []):
            path = root / rule["path"]
            if not path.is_file():
                continue
            text = path.read_text(encoding="utf-8", errors="replace").lower()
            for needle in rule.get("contains_any", []):
                self.assertNotIn(
                    str(needle).lower(),
                    text,
                    f"{label}: {rule['path']} must not contain {needle!r}",
                )

    def _run_named(self, recipe_id: str, scenario_id: str):
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        runtimes = _selected_runtimes()
        self.assertTrue(runtimes, "no runtimes selected")
        failures: list[str] = []
        for runtime in runtimes:
            passed = 0
            last_err: Exception | None = None
            for _ in range(trials):
                try:
                    self._run_scenario(recipe_id, scenario_id, runtime)
                    passed += 1
                except AssertionError as exc:
                    last_err = exc
            needed = _n_of_m(passed, trials)
            if passed < needed:
                failures.append(
                    f"{runtime}:{_key(recipe_id, scenario_id)} "
                    f"{passed}/{trials} — {last_err}"
                )
        self.assertFalse(failures, msg="; ".join(failures))

    def test_vcs_scenarios(self):
        selected = _selected_scenarios()
        self.assertTrue(selected, "no VCS scenarios selected")
        for recipe_id, scenario_id in selected:
            with self.subTest(scenario=_key(recipe_id, scenario_id)):
                self._run_named(recipe_id, scenario_id)


if __name__ == "__main__":
    unittest.main()
