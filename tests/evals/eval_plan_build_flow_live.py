"""Live plan-build-flow behavioral evals. Requires EVALS_LIVE=1."""

from __future__ import annotations

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
    allow_cli_commands,
    api_key_present,
    assert_resolved_mode,
    assert_scenario_outputs,
    git_paths_changed,
    glob_exists,
    init_git_repo,
    install_root,
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
    seed_fixture_branch,
    seed_project_files,
    setup_runtime_skills,
    write_manifest,
)

SCENARIOS = REPO_ROOT / "tests" / "evals" / "scenarios" / "plan-build-flow"
LIVE_SCENARIOS = (
    "ac3_plan_stops_before_apply",
    "ac4_build_after_auth",
    "ac5_archive_before_merge",
    "ac_none_no_ceremony",
    "ac_light_plan_only",
    "ac_standard_plan_plus_delta",
    "ac_tier_is_not_declared",
    "ac_ceremony_none_openspec",
    "ac_plan_shape",
    "ac_delta_only_for_observable_rule",
    "ac_archive_with_verb_then_ask_before_merge",
    "ac_archive_marker_without_folder",
    "ac_high_risk_path_odd",
    "ac_specs_write_without_delta",
    "ac_mode_odd_artifacts",
    "ac_mode_openspec_artifacts",
    "ac_brief_reports_the_mode",
    "ac_delivery_contract_artifact_store",
)

# `fixture_mode = "both"` runs the scenario once per artifact mode; the mode is
# declared in the manifest the fixture seeds, so a case asserts the tree its
# mode owns. Per-mode assertion overrides live under `[modes.<mode>]`.
ALL_MODES = ("openspec", "odd")


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


def _fixture_modes(meta: dict) -> tuple[str, ...]:
    declared = str(meta.get("fixture_mode", "openspec"))
    if declared == "both":
        return ALL_MODES
    return (declared,)


def _meta_for_mode(meta: dict, mode: str) -> dict:
    """Scenario assertions for one artifact mode: `[modes.<mode>]` over the top
    level, so a case that must assert a different tree per mode stays one case."""
    overrides = (meta.get("modes") or {}).get(mode) or {}
    return {**meta, **overrides}


def _n_of_m(passed: int, trials: int) -> int:
    return trials if trials == 1 else max(2, (trials * 2) // 3)


SEEDED_IMPLEMENTATION = (
    '"""Signup form handler."""\n'
    "\n"
    "\n"
    "def signup(email: str, password: str) -> dict:\n"
    '    if "@" not in email:\n'
    '        raise ValueError("invalid email")\n'
    "    if len(password) < 8:\n"
    '        raise ValueError("invalid password")\n'
    '    return {"email": email, "password": password}\n'
)


SEEDED_CANONICAL_SPEC = (
    "# forms\n"
    "\n"
    "## Requirements\n"
    "\n"
    "### Requirement: Signup accepts an email\n"
    "\n"
    "The signup form accepts an email and a password.\n"
)


def _write_evidence(
    name: str,
    runtime: str,
    fixture_mode: str,
    prompt: str,
    result: dict,
    changed: list[str],
    root: Path,
) -> None:
    """Keep what the agent actually did, so a verdict can be explained.

    Written on pass and on fail: a failure whose transcript is gone is not a
    finding about the skill, it is a number. Inert unless EVALS_EVIDENCE_DIR is
    set.
    """
    target = os.environ.get("EVALS_EVIDENCE_DIR")
    if not target:
        return
    out = Path(target)
    out.mkdir(parents=True, exist_ok=True)
    lines = [
        f"scenario: {name}",
        f"runtime: {runtime}",
        f"mode: {fixture_mode}",
        f"model: {result.get('model')}",
        f"returncode: {result.get('returncode')}",
        f"timed_out: {result.get('timed_out')}",
        f"cost: {result.get('cost')}",
        "",
        "== prompt ==",
        prompt,
        "",
        "== agent answer ==",
        str(result.get("result_text") or ""),
        "",
        "== changed paths ==",
        *(changed or ["(none)"]),
        "",
        "== tree ==",
        *(
            str(p.relative_to(root))
            for p in sorted(root.rglob("*"))
            if p.is_file() and ".git/" not in str(p.relative_to(root))
        ),
        "",
        "== stdout tail ==",
        (result.get("stdout") or "")[-4000:],
        "",
        "== stderr ==",
        (result.get("stderr") or "")[-2000:],
    ]
    name_part = f"{runtime}-{fixture_mode}-{name}.txt"
    (out / name_part).write_text("\n".join(lines), encoding="utf-8")


@unittest.skipUnless(
    live_enabled() and bool(_selected_runtimes()),
    "Set EVALS_LIVE=1 with a supported runtime on PATH to run live evals",
)
class PlanBuildFlowLiveEvals(unittest.TestCase):
    def _run_scenario(self, name: str, runtime: str, fixture_mode: str):
        scenario_dir = SCENARIOS / name
        scenario = load_scenario(scenario_dir)
        meta = scenario.meta
        slug = str(meta.get("slug", "signup-validation"))
        label = f"{runtime}/{fixture_mode}/{name}"

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", scenario.recipe_id)
        manifest_extra = str(meta.get("manifest_extra", ""))
        materialize_project(root, scenario.recipe_id, version, extra=manifest_extra)
        if fixture_mode != "openspec":
            # materialize_project renders the default mode; the mode under test is
            # declared in the manifest and re-rendered by the final sync below.
            write_manifest(
                root,
                recipe_id=scenario.recipe_id,
                version=version,
                extra_recipes=manifest_extra,
                mode=fixture_mode,
            )
        # The mode under test has to be the mode the CLI resolves. The manifest
        # key alone proves nothing: the recipe-config validator ignores it.
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
        # The fixture's own settings file: the skill needs `ai-specs <verb>` and a
        # headless session cannot approve it. Nothing wider, and never the repo's
        # or the user's settings.
        allow_cli_commands(root)
        init_git_repo(root)
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(["git", "commit", "-qm", "baseline"], cwd=root, check=True)
        # The branch pairs the change, so the derived slug is the fixture slug and
        # `branch.<branch>.*` / the commit range resolve for the assertions.
        seed_fixture_branch(root)
        # Checked here, before any plan exists: with a plan present both modes
        # allow the write and the probe would prove nothing.
        assert_resolved_mode(root, fixture_mode)
        if meta.get("seed_plan"):
            # Seeded after the branch exists and committed on it: a plan written
            # into the baseline is not a branch change, which left the archive
            # cases with nothing to archive and made the branch look untouched.
            seed_authorized_plan(
                root,
                slug=slug,
                mode=fixture_mode,
                # The fixture's only seed knob: `light` seeds a plan with no
                # delta, `standard` a plan plus its delta.
                tier="standard" if meta.get("seed_plan_delta", True) else "light",
            )
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", f"plan: authorize {slug}"], cwd=root, check=True
            )

        if meta.get("seed_canonical_spec"):
            # The canonical-spec case asks the agent to hand-write a rule that
            # already lives somewhere. Without a canonical spec there is nothing
            # to write into, and the agent rightly stops.
            canonical = root / "openspec" / "specs" / "forms" / "spec.md"
            canonical.parent.mkdir(parents=True, exist_ok=True)
            canonical.write_text(SEEDED_CANONICAL_SPEC, encoding="utf-8")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "specs: seed the canonical forms spec"],
                cwd=root,
                check=True,
            )

        if meta.get("seed_implementation"):
            # The archive prompts say the change is ready; a plan with no code is
            # not, and every agent in the P0 pass refused to archive it for
            # exactly that reason -- a false premise in the eval, not a defect in
            # the skill.
            target = root / "src" / "forms" / "signup.py"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(SEEDED_IMPLEMENTATION, encoding="utf-8")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", "feat: implement the change"],
                cwd=root,
                check=True,
            )

        if meta.get("seed_promoted_delta"):
            # `changes archive` refuses while a delta is unpromoted, so seed the
            # change already promoted: the case measures the archive, not the
            # promotion it never asked for.
            promote = subprocess.run(
                [
                    str(install_root() / "dist" / "ai-specs"),
                    "specs",
                    "promote",
                    slug,
                    "--root",
                    str(root),
                ],
                text=True,
                capture_output=True,
                check=False,
                env={**os.environ, "AI_SPECS_HOME": str(install_root())},
            )
            self.assertEqual(promote.returncode, 0, promote.stderr or promote.stdout)
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            subprocess.run(
                ["git", "commit", "-qm", f"specs: promote {slug}"], cwd=root, check=True
            )

        prompt = scenario.prompt_path.read_text()
        old = os.environ.get("EVALS_RUNTIME")
        # The skill tells the agent to run `ai-specs changes start --none`,
        # `specs promote` and `changes archive`; without the binary on PATH those
        # instructions are unexecutable and the case measures the PATH, not the
        # skill.
        old_path = os.environ.get("PATH", "")
        os.environ["PATH"] = str(install_root() / "dist") + os.pathsep + old_path
        os.environ["EVALS_RUNTIME"] = runtime
        try:
            result = run_prompt(root, prompt, runtime=runtime, mode=scenario.mode)
        finally:
            os.environ["PATH"] = old_path
            if old is None:
                os.environ.pop("EVALS_RUNTIME", None)
            else:
                os.environ["EVALS_RUNTIME"] = old

        changed = git_paths_changed(root)
        _write_evidence(name, runtime, fixture_mode, prompt, result, changed, root)
        # If the agent wrote required artifacts then hung (MCP/etc), accept timeout.
        soft_ok = False
        if result.get("timed_out"):
            required_ok = all(
                glob_exists(root, required)
                for required in meta.get("required_path_globs", [])
            )
            any_ok = all(
                any(glob_exists(root, pat) for pat in group)
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
                    f"{label} failed rc={result.get('returncode')} "
                    f"timed_out={result.get('timed_out')}\n"
                    f"stderr={result.get('stderr')}\n"
                    f"stdout_tail={(result.get('stdout') or '')[-2000:]}\n"
                    f"cmd={result.get('cmd')}\n"
                    f"changed={changed}"
                ),
            )
        # Every scenario.toml assertion key lives in the harness, so the live
        # module and the primitive tests share one implementation.
        assert_scenario_outputs(
            root, _meta_for_mode(meta, fixture_mode), result, changed, label=label
        )

    def _run_named(self, name: str):
        trials = int(os.environ.get("EVALS_TRIALS", "1"))
        runtimes = _selected_runtimes()
        self.assertTrue(runtimes, "no runtimes selected")
        meta = load_scenario(SCENARIOS / name).meta
        requires_hook = bool(meta.get("requires_hook"))
        modes = _fixture_modes(meta)
        failures: list[str] = []
        for runtime in runtimes:
            # A gate scenario asserts only where the gate can be enforced; skip
            # runtimes with no pre-file-write hook event (e.g. cursor-agent).
            if requires_hook and runtime in NO_FILE_WRITE_HOOK_RUNTIMES:
                continue
            for mode in modes:
                passed = 0
                last_err: Exception | None = None
                for _ in range(trials):
                    try:
                        self._run_scenario(name, runtime, mode)
                        passed += 1
                    except AssertionError as exc:
                        last_err = exc
                needed = _n_of_m(passed, trials)
                if passed < needed:
                    failures.append(f"{runtime}:{mode}:{name} {passed}/{trials} — {last_err}")
        self.assertFalse(failures, msg="; ".join(failures))


def _scenario_test(name: str):
    def test(self: PlanBuildFlowLiveEvals):
        if name not in _selected_scenarios():
            self.skipTest(f"{name} not selected via EVALS_SCENARIOS")
        self._run_named(name)

    test.__name__ = f"test_{name}"
    test.__doc__ = f"Live golden: {name}"
    return test


for _name in LIVE_SCENARIOS:
    setattr(PlanBuildFlowLiveEvals, f"test_{_name}", _scenario_test(_name))


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
