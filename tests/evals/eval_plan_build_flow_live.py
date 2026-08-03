"""Live plan-build-flow behavioral evals (AC3–AC7). Requires EVALS_LIVE=1."""

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
    NO_FILE_WRITE_HOOK_RUNTIMES,
    SUPPORTED_RUNTIMES,
    api_key_present,
    detect_runtime,
    git_paths_changed,
    init_git_repo,
    live_enabled,
    load_scenario,
    materialize_project,
    run_prompt,
    runtime_available,
    wire_runtime_hooks,
)
from tests.evals.lib.project_fixture import (  # noqa: E402
    recipe_version,
    seed_authorized_plan,
    seed_project_files,
    setup_runtime_skills,
)

SCENARIOS = REPO_ROOT / "tests" / "evals" / "scenarios" / "plan-build-flow"
LIVE_SCENARIOS = (
    "ac3_plan_stops_before_apply",
    "ac4_build_after_auth",
    "ac5_archive_before_merge",
    "ac7_light_gitignore_file_store",
    "ac8_approval_verb_without_folder",
    "ac_delivery_contract_artifact_store",
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
            for n in os.environ.get(
                "EVALS_PREFER", "claude,cursor-agent,opencode,pi,omp"
            ).split(",")
            if n.strip()
        ]
    out: list[str] = []
    for name in names:
        if name not in SUPPORTED_RUNTIMES:
            continue
        if not runtime_available(name):
            continue
        if not api_key_present(name):
            continue
        out.append(name)
    return out


def _selected_scenarios() -> list[str]:
    raw = os.environ.get("EVALS_SCENARIOS", "")
    if raw.strip():
        return [s.strip() for s in raw.split(",") if s.strip()]
    return list(LIVE_SCENARIOS)


def _n_of_m(passed: int, trials: int) -> int:
    return trials if trials == 1 else max(2, (trials * 2) // 3)


@unittest.skipUnless(
    live_enabled() and bool(_selected_runtimes()),
    "Set EVALS_LIVE=1 with a supported runtime on PATH to run live evals",
)
class PlanBuildFlowLiveEvals(unittest.TestCase):
    def _run_scenario(self, name: str, runtime: str):
        scenario_dir = SCENARIOS / name
        scenario = load_scenario(scenario_dir)
        meta = scenario.meta
        slug = str(meta.get("slug", "signup-validation"))

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", scenario.recipe_id)
        manifest_extra = str(meta.get("manifest_extra", ""))
        materialize_project(root, scenario.recipe_id, version, extra=manifest_extra)
        if meta.get("sync_before_prompt"):
            sync = subprocess.run(
                [str(REPO_ROOT / "bin" / "ai-specs"), "sync", str(root)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(sync.returncode, 0, sync.stderr)
        seed_project_files(root)
        setup_runtime_skills(
            root, runtime, scenario.recipe_id, catalog_root=REPO_ROOT / "catalog"
        )
        # Wire the recipe's runtime hooks into this runtime's native channel so
        # the scenario exercises the gate hook end-to-end, not just the advisory
        # skill/brief layer.
        wire_runtime_hooks(root, runtime)
        if meta.get("seed_plan"):
            seed_authorized_plan(
                root, slug=slug, tier=str(meta.get("tier", "standard"))
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

        changed = git_paths_changed(root)
        # If the agent wrote required artifacts then hung (MCP/etc), accept timeout.
        soft_ok = False
        if result.get("timed_out"):
            required_ok = all(
                _glob_exists(root, required)
                for required in meta.get("required_path_globs", [])
            )
            any_ok = all(
                any(_glob_exists(root, pat) for pat in group)
                for group in meta.get("required_any_path_globs", [])
            )
            soft_ok = (bool(meta.get("required_path_globs")) and required_ok) or (
                bool(meta.get("required_any_path_globs")) and any_ok
            )

        if not soft_ok:
            self.assertEqual(
                result["returncode"],
                0,
                msg=(
                    f"{runtime}/{name} failed rc={result.get('returncode')} "
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
                f"{runtime}/{name}: forbidden path modified: {path}",
            )

        for required in meta.get("required_path_globs", []):
            self.assertTrue(
                _glob_exists(root, required),
                f"{runtime}/{name}: missing {required}; changed={changed}",
            )

        for group in meta.get("required_any_path_globs", []):
            self.assertTrue(
                any(_glob_exists(root, pat) for pat in group),
                f"{runtime}/{name}: expected any of {group}; changed={changed}",
            )

        for pattern in meta.get("required_changed_globs", []):
            self.assertTrue(
                any(_matches_any(path, [pattern]) for path in changed)
                or _glob_exists(root, pattern),
                f"{runtime}/{name}: expected change matching {pattern}; changed={changed}",
            )

        for rule in meta.get("required_content", []):
            path = root / rule["path"]
            self.assertTrue(path.is_file(), f"{runtime}/{name}: missing {rule['path']}")
            text = path.read_text(encoding="utf-8", errors="replace").lower()
            needles = [str(n).lower() for n in rule.get("contains_any", [])]
            self.assertTrue(
                any(n in text for n in needles),
                f"{runtime}/{name}: {rule['path']} missing any of {needles}",
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
                    f"{runtime}/{name}: {rule['path']} must not contain {needle!r}",
                )

        transcript = (result.get("result_text") or result.get("stdout") or "").lower()
        for needle in meta.get("required_transcript_any", []):
            # Soft: only enforce when no required_content path was written
            if meta.get("required_content"):
                break
            self.assertIn(
                str(needle).lower(),
                transcript,
                f"{runtime}/{name}: transcript missing {needle!r}",
            )

        if meta.get("assert_active_change_gone"):
            active = root / "openspec" / "changes" / slug
            self.assertFalse(
                active.exists(),
                f"{runtime}/{name}: active change folder still present at {active}",
            )

    def _run_named(self, name: str):
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        runtimes = _selected_runtimes()
        self.assertTrue(runtimes, "no runtimes selected")
        requires_hook = bool(load_scenario(SCENARIOS / name).meta.get("requires_hook"))
        failures: list[str] = []
        for runtime in runtimes:
            # A gate scenario asserts only where the gate can be enforced; skip
            # runtimes with no pre-file-write hook event (e.g. cursor-agent).
            if requires_hook and runtime in NO_FILE_WRITE_HOOK_RUNTIMES:
                continue
            passed = 0
            last_err: Exception | None = None
            for _ in range(trials):
                try:
                    self._run_scenario(name, runtime)
                    passed += 1
                except AssertionError as exc:
                    last_err = exc
            needed = _n_of_m(passed, trials)
            if passed < needed:
                failures.append(f"{runtime}:{name} {passed}/{trials} — {last_err}")
        self.assertFalse(failures, msg="; ".join(failures))

    def test_ac3_plan_stops_before_apply(self):
        if "ac3_plan_stops_before_apply" not in _selected_scenarios():
            self.skipTest("ac3 not selected via EVALS_SCENARIOS")
        self._run_named("ac3_plan_stops_before_apply")

    def test_ac4_build_after_auth(self):
        if "ac4_build_after_auth" not in _selected_scenarios():
            self.skipTest("ac4 not selected via EVALS_SCENARIOS")
        self._run_named("ac4_build_after_auth")

    def test_ac5_archive_before_merge(self):
        if "ac5_archive_before_merge" not in _selected_scenarios():
            self.skipTest("ac5 not selected via EVALS_SCENARIOS")
        self._run_named("ac5_archive_before_merge")

    def test_ac7_light_gitignore_file_store(self):
        if "ac7_light_gitignore_file_store" not in _selected_scenarios():
            self.skipTest("ac7 not selected via EVALS_SCENARIOS")
        self._run_named("ac7_light_gitignore_file_store")

    def test_ac8_approval_verb_without_folder(self):
        if "ac8_approval_verb_without_folder" not in _selected_scenarios():
            self.skipTest("ac8 not selected via EVALS_SCENARIOS")
        self._run_named("ac8_approval_verb_without_folder")
    def test_ac_delivery_contract_artifact_store(self):
        if "ac_delivery_contract_artifact_store" not in _selected_scenarios():
            self.skipTest("delivery contract not selected via EVALS_SCENARIOS")
        self._run_named("ac_delivery_contract_artifact_store")


class PlanBuildDeliveryContractHermetic(unittest.TestCase):
    """Offline materialization eval for the repository-owned store contract."""

    def test_both_override_is_injected_into_agents(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "plan-build-flow")
        materialize_project(
            root,
            "plan-build-flow",
            version,
            extra="\n[recipes.plan-build-flow.config]\nartifact_store_default = 'both'\n",
        )
        result = subprocess.run(
            [str(REPO_ROOT / "bin" / "ai-specs"), "sync", str(root)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        content = (root / "AGENTS.md").read_text()
        self.assertIn("Default artifact store", content)
        self.assertIn("`both`", content)
        self.assertNotIn("{config.artifact_store_default}", content)


if __name__ == "__main__":
    unittest.main()
