"""Live assisted-configure eval client; opt-in and excluded from unit tests."""
from __future__ import annotations

import hashlib
import json
import os
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
    default_model,
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
    recipe_version,
    setup_bundled_skills,
    setup_runtime_skills,
)

SCENARIOS = REPO_ROOT / "tests" / "evals" / "scenarios" / "assisted-configure"
LIVE_SCENARIOS = (
    "ac_recommend_stops_before_apply",
    "ac_topology_grounded_without_initmd",
    "ac_apply_sync_verify_report",
    "ac_noop_reapply_preserves_bytes",
    "ac_blocked_cli_version_pin",
)


def _selected_runtimes() -> list[str]:
    raw = os.environ.get("EVALS_RUNTIMES") or os.environ.get("EVALS_RUNTIME", "")
    names = [n.strip() for n in raw.split(",") if n.strip()] if raw else [
        n.strip() for n in os.environ.get("EVALS_PREFER", "claude,cursor-agent,opencode,pi,omp").split(",") if n.strip()
    ]
    return [n for n in names if n in SUPPORTED_RUNTIMES and runtime_available(n) and api_key_present(n)]


def _selected_scenarios() -> list[str]:
    raw = os.environ.get("EVALS_SCENARIOS", "")
    return [s.strip() for s in raw.split(",") if s.strip()] if raw else list(LIVE_SCENARIOS)


def _hash_manifest(root: Path) -> str:
    return hashlib.sha256((root / "ai-specs" / "ai-specs.toml").read_bytes()).hexdigest()


@unittest.skipUnless(live_enabled() and bool(_selected_runtimes()), "Set EVALS_LIVE=1 with a supported runtime")
class AssistedConfigureLiveEvals(unittest.TestCase):
    def _run_scenario(self, name: str, runtime: str) -> None:
        scenario = load_scenario(SCENARIOS / name)
        meta = scenario.meta
        root_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(root_tmp.cleanup)
        root = Path(root_tmp.name)
        recipe_id = scenario.recipe_id
        version = recipe_version(REPO_ROOT / "catalog", recipe_id)
        extra = str(meta.get("manifest_extra", ""))
        materialize_project(root, recipe_id, version, extra=extra)
        init_git_repo(root)
        if meta.get("fixture") == "submodule_one":
            add_initialized_submodule(root, path="libs/core", label="core", sources_dir=Path(root_tmp.name) / "remotes")
        setup_runtime_skills(root, runtime, recipe_id, catalog_root=REPO_ROOT / "catalog")
        setup_bundled_skills(root, runtime, ["harness-recipes", "harness-lifecycle"], bundled_root=REPO_ROOT / "bundled-skills")
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(["git", "commit", "-qm", "baseline"], cwd=root, check=True)
        before = _hash_manifest(root)
        old_runtime = os.environ.get("EVALS_RUNTIME")
        os.environ["EVALS_RUNTIME"] = runtime
        try:
            result = run_prompt(root, scenario.prompt_path.read_text(), runtime=runtime, mode=scenario.mode)
        finally:
            if old_runtime is None:
                os.environ.pop("EVALS_RUNTIME", None)
            else:
                os.environ["EVALS_RUNTIME"] = old_runtime
        raw_transcript = result.get("result_text") or result.get("stdout") or ""
        transcript = raw_transcript.lower()
        sha = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=root, capture_output=True, text=True, check=True
        ).stdout.strip()
        print(
            "EVAL_EVIDENCE:"
            + json.dumps(
                {
                    "scenario": name,
                    "runtime": runtime,
                    "model": default_model(runtime),
                    "trial": 1,
                    "cli_version": (REPO_ROOT / "VERSION").read_text().strip(),
                    "worktree_sha": sha,
                    "exit": result.get("returncode"),
                    "timed_out": bool(result.get("timed_out")),
                    "helper_report_present": "report_version" in raw_transcript,
                },
                sort_keys=True,
            )
        )
        self.assertEqual(result.get("returncode"), 0, f"{runtime}/{name}: {result}")
        changed = git_paths_changed(root)
        if name in {"ac_recommend_stops_before_apply", "ac_blocked_cli_version_pin", "ac_noop_reapply_preserves_bytes"}:
            self.assertEqual(_hash_manifest(root), before, f"{runtime}/{name} changed manifest")
        if name == "ac_recommend_stops_before_apply":
            self.assertTrue(any(word in transcript for word in ("recommend", "approval", "inspect")))
        elif name == "ac_topology_grounded_without_initmd":
            self.assertTrue(any(word in transcript for word in ("monorepo-submodules", "submodule")))
        elif name == "ac_apply_sync_verify_report":
            text = (root / "ai-specs" / "ai-specs.toml").read_text()
            self.assertIn("board_id", text)
            self.assertTrue(any(word in transcript for word in ("report", "sync", "verify")))
        elif name == "ac_noop_reapply_preserves_bytes":
            self.assertTrue(any(word in transcript for word in ("no-op", "no op", "unchanged")))
        elif name == "ac_blocked_cli_version_pin":
            self.assertTrue(any(word in transcript for word in ("blocked", "version", "preflight")))
        self.assertFalse(any(path.startswith(".worktrees/") for path in changed))

    def _run_named(self, name: str) -> None:
        failures: list[str] = []
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        for runtime in _selected_runtimes():
            passed = 0
            last: Exception | None = None
            for _ in range(trials):
                try:
                    self._run_scenario(name, runtime)
                    passed += 1
                except AssertionError as exc:
                    last = exc
            required = trials if trials == 1 else max(2, (trials * 2) // 3)
            if passed < required:
                failures.append(f"{runtime}/{name}: {passed}/{trials} ({last})")
        self.assertFalse(failures, "; ".join(failures))

    def test_ac_recommend_stops_before_apply(self):
        if "ac_recommend_stops_before_apply" in _selected_scenarios():
            self._run_named("ac_recommend_stops_before_apply")

    def test_ac_topology_grounded_without_initmd(self):
        if "ac_topology_grounded_without_initmd" in _selected_scenarios():
            self._run_named("ac_topology_grounded_without_initmd")

    def test_ac_apply_sync_verify_report(self):
        if "ac_apply_sync_verify_report" in _selected_scenarios():
            self._run_named("ac_apply_sync_verify_report")

    def test_ac_noop_reapply_preserves_bytes(self):
        if "ac_noop_reapply_preserves_bytes" in _selected_scenarios():
            self._run_named("ac_noop_reapply_preserves_bytes")

    def test_ac_blocked_cli_version_pin(self):
        if "ac_blocked_cli_version_pin" in _selected_scenarios():
            self._run_named("ac_blocked_cli_version_pin")


if __name__ == "__main__":
    unittest.main()
