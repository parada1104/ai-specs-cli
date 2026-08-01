"""Live worktree-flow recipe evals. Requires EVALS_LIVE=1."""

from __future__ import annotations

import fnmatch
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    forbidden_phrase_violations,
    git_paths_changed,
    init_git_repo,
    live_enabled,
    load_scenario,
    materialize_project,
    run_prompt,
    runtime_available,
)
from tests.evals.lib.project_fixture import (  # noqa: E402
    add_initialized_submodule,
    add_initialized_submodules,
    recipe_version,
    seed_monorepo_apps,
    seed_project_files,
    setup_runtime_commands,
    setup_runtime_skills,
)

SCENARIOS_ROOT = REPO_ROOT / "tests" / "evals" / "scenarios"

LIVE_SCENARIOS: tuple[str, ...] = (
    "ac_submodule_create_uses_subrepo_contract",
    "ac_monorepo_apps_no_subrepo_needed",
    "ac_cleanup_scans_all_submodules",
    "ac_gate_blocked_write_creates_worktree_not_bash_fallback",
)

RECIPE_ID = "worktree-flow"


def _glob_exists(root: Path, pattern: str) -> bool:
    return any(root.glob(pattern))


def _matches_any(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatch(path, pat) for pat in patterns)


def _selected_runtimes() -> list[str]:
    raw = os.environ.get("EVALS_RUNTIMES") or os.environ.get("EVALS_RUNTIME", "")
    prefer = os.environ.get("EVALS_PREFER", "claude,cursor-agent,opencode,pi,omp")
    order = [x.strip() for x in (raw or prefer).split(",") if x.strip()]
    out: list[str] = []
    for name in order:
        if name in out:
            continue
        if runtime_available(name):
            out.append(name)
    return out


def _selected_scenarios() -> list[str]:
    raw = os.environ.get("EVALS_SCENARIOS", "")
    if not raw.strip():
        return list(LIVE_SCENARIOS)
    selected: list[str] = []
    for token in raw.split(","):
        token = token.strip()
        if not token:
            continue
        if "/" in token:
            recipe_id, scenario_id = token.split("/", 1)
            if recipe_id != RECIPE_ID:
                continue
        else:
            scenario_id = token
        if scenario_id in LIVE_SCENARIOS and scenario_id not in selected:
            selected.append(scenario_id)
    return selected


def _n_of_m(passed: int, trials: int) -> int:
    return trials if trials == 1 else max(2, (trials * 2) // 3)


def _prepare_fixture(root: Path, fixture: str) -> None:
    """Shape the temp project for the scenario before the agent runs."""
    if fixture == "submodule_one":
        add_initialized_submodule(
            root,
            path="alquimia-front-web",
            name="alquimia-front-web",
            label="alquimia-front-web",
        )
        return
    if fixture == "submodule_two":
        add_initialized_submodules(
            root,
            [
                {
                    "path": "alquimia-front-web",
                    "name": "alquimia-front-web",
                    "label": "alquimia-front-web",
                },
                {
                    "path": "alquimia-api",
                    "name": "alquimia-api",
                    "label": "alquimia-api",
                },
            ],
        )
        return
    if fixture == "monorepo_apps":
        seed_monorepo_apps(root, ("admin-dashboard", "api"))
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(
            ["git", "commit", "-qm", "seed apps monorepo"],
            cwd=root,
            check=True,
        )
        return
    if fixture == "protected_main":
        app = root / "src" / "app.py"
        app.parent.mkdir(parents=True, exist_ok=True)
        if not app.is_file():
            app.write_text('"""App entry (eval fixture)."""\n\nVALUE = 1\n')
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        dirty = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=root,
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
        if dirty:
            subprocess.run(
                ["git", "commit", "-qm", "seed app.py"],
                cwd=root,
                check=True,
            )
        # Main worktree on a protected branch name.
        subprocess.run(["git", "branch", "-M", "development"], cwd=root, check=True)
        return
    raise ValueError(f"unknown worktree-flow fixture: {fixture}")


@unittest.skipUnless(
    live_enabled() and bool(_selected_runtimes()),
    "set EVALS_LIVE=1 and ensure a supported runtime is on PATH",
)
class WorktreeFlowLiveEvals(unittest.TestCase):
    def _run_scenario(self, scenario_id: str, runtime: str):
        scenario_dir = SCENARIOS_ROOT / RECIPE_ID / scenario_id
        scenario = load_scenario(scenario_dir)
        meta = scenario.meta
        self.assertEqual(scenario.recipe_id, RECIPE_ID)

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", RECIPE_ID)
        # Prefer development as integration branch so gate scenarios match prompts.
        extra = (
            "\n[recipes.worktree-flow.config]\n"
            'integration_branch = "development"\n'
        )
        materialize_project(root, RECIPE_ID, version, extra=extra)
        seed_project_files(root)
        setup_runtime_skills(
            root, runtime, RECIPE_ID, catalog_root=REPO_ROOT / "catalog"
        )
        setup_runtime_commands(
            root, runtime, RECIPE_ID, catalog_root=REPO_ROOT / "catalog"
        )

        init_git_repo(root)
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(["git", "commit", "-qm", "baseline"], cwd=root, check=True)

        fixture = str(meta.get("fixture", "")).strip()
        if fixture:
            _prepare_fixture(root, fixture)

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

        label = f"{runtime}/{RECIPE_ID}/{scenario_id}"
        changed = git_paths_changed(root)

        soft_ok = False
        if result.get("timed_out"):
            required_ok = all(
                _glob_exists(root, required)
                for required in meta.get("required_path_globs", [])
            )
            content_ok = (
                all(
                    (root / rule["path"]).is_file()
                    for rule in meta.get("required_content", [])
                )
                if meta.get("required_content")
                else False
            )
            soft_ok = (bool(meta.get("required_path_globs")) and required_ok) or content_ok

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

        for pattern in meta.get("absent_path_globs", []):
            # Existence smoke: agent must honor "don't execute" (no real worktrees).
            matches = list(root.glob(pattern)) if any(ch in pattern for ch in "*?[") else (
                [root / pattern] if (root / pattern).exists() else []
            )
            self.assertFalse(
                matches,
                f"{label}: path should be absent ({pattern}): {matches[:5]}",
            )

        for required in meta.get("required_path_globs", []):
            self.assertTrue(
                _glob_exists(root, required),
                f"{label}: missing {required}; changed={changed}",
            )

        for rule in meta.get("required_content", []):
            path = root / rule["path"]
            self.assertTrue(path.is_file(), f"{label}: missing {rule['path']}")
            raw = path.read_text(encoding="utf-8", errors="replace").lower()
            text = raw.replace("`", "")
            needles = [str(n).lower().replace("`", "") for n in rule.get("contains_any", [])]
            self.assertTrue(
                any(n in text for n in needles),
                f"{label}: {rule['path']} missing any of {needles}\n---\n{raw[:2000]}",
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

        for rule in meta.get("forbidden_phrases", []):
            path = root / rule["path"]
            self.assertTrue(path.is_file(), f"{label}: missing {rule['path']}")
            hits = forbidden_phrase_violations(
                path.read_text(encoding="utf-8", errors="replace"),
                [str(p) for p in rule.get("phrases", [])],
            )
            if hits:
                self.fail(
                    f"{label}: affirmative forbidden phrase in {rule['path']}: {hits[0]}"
                )

    def _run_named(self, scenario_id: str):
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        runtimes = _selected_runtimes()
        self.assertTrue(runtimes, "no runtimes selected")
        failures: list[str] = []
        for runtime in runtimes:
            passed = 0
            last_err: Exception | None = None
            for _ in range(trials):
                try:
                    self._run_scenario(scenario_id, runtime)
                    passed += 1
                except AssertionError as exc:
                    last_err = exc
            needed = _n_of_m(passed, trials)
            if passed < needed:
                failures.append(
                    f"{runtime}:{RECIPE_ID}/{scenario_id} "
                    f"{passed}/{trials} — {last_err}"
                )
        self.assertFalse(failures, msg="; ".join(failures))

    def test_worktree_flow_scenarios(self):
        selected = _selected_scenarios()
        self.assertTrue(selected, "no worktree-flow scenarios selected")
        for scenario_id in selected:
            with self.subTest(scenario=f"{RECIPE_ID}/{scenario_id}"):
                self._run_named(scenario_id)


if __name__ == "__main__":
    unittest.main()
