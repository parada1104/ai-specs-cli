"""Live vault-canonical-store recipe evals. Requires EVALS_LIVE=1."""

from __future__ import annotations

import fnmatch
import os
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT))

# Offline vendor for kepano deps when materializing the recipe in fixtures.
os.environ.setdefault(
    "AI_SPECS_VENDOR_FIXTURE_ROOT",
    str(REPO_ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"),
)

from tests.evals.lib.harness import (  # noqa: E402
    SUPPORTED_RUNTIMES,
    api_key_present,
    live_enabled,
    load_scenario,
    materialize_project,
    run_prompt,
    runtime_available,
)
from tests.evals.lib.project_fixture import (  # noqa: E402
    recipe_version,
    seed_project_files,
    setup_runtime_skills,
)

SCENARIOS_ROOT = REPO_ROOT / "tests" / "evals" / "scenarios" / "vault-canonical-store"

LIVE_SCENARIOS: tuple[str, ...] = (
    "ac_kepano_skills_present",
    "ac_mcp_path_with_spaces",
    "ac_vault_context_guidance",
)


def _glob_exists(root: Path, pattern: str) -> bool:
    return any(root.glob(pattern))


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
    if not raw.strip():
        return list(LIVE_SCENARIOS)
    selected: list[str] = []
    for token in raw.split(","):
        token = token.strip()
        if not token:
            continue
        if "/" in token:
            _, scenario_id = token.split("/", 1)
            selected.append(scenario_id)
        else:
            selected.append(token)
    return [s for s in selected if s in LIVE_SCENARIOS]


def _n_of_m(passed: int, trials: int) -> int:
    return trials if trials == 1 else max(2, (trials * 2) // 3)


def _content_ok(root: Path, rules: list[dict]) -> bool:
    for rule in rules:
        path = root / rule["path"]
        if not path.is_file():
            return False
        text = path.read_text()
        needles = rule.get("contains_any") or []
        if needles and not any(n.lower() in text.lower() for n in needles):
            return False
    return True


@unittest.skipUnless(
    live_enabled() and bool(_selected_runtimes()),
    "Set EVALS_LIVE=1 with a supported runtime on PATH to run live vault evals",
)
class VaultCanonicalLiveEvals(unittest.TestCase):
    def _run_scenario(self, scenario_id: str, runtime: str):
        scenario_dir = SCENARIOS_ROOT / scenario_id
        scenario = load_scenario(scenario_dir)
        meta = scenario.meta
        self.assertEqual(scenario.recipe_id, "vault-canonical-store")

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(REPO_ROOT / "catalog", "vault-canonical-store")
        materialize_project(root, "vault-canonical-store", version)
        seed_project_files(root)
        setup_runtime_skills(
            root,
            runtime,
            "vault-canonical-store",
            catalog_root=REPO_ROOT / "catalog",
        )
        (root / "ai-specs" / "eval-notes").mkdir(parents=True, exist_ok=True)

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

        label = f"{runtime}/vault-canonical-store/{scenario_id}"
        soft_ok = False
        if result.get("timed_out"):
            required_ok = all(
                _glob_exists(root, required)
                for required in meta.get("required_path_globs", [])
            )
            content_ok = _content_ok(root, meta.get("required_content", []))
            soft_ok = (bool(meta.get("required_path_globs")) and required_ok) or content_ok

        if not soft_ok:
            self.assertEqual(
                result["returncode"],
                0,
                msg=(
                    f"{label} failed rc={result.get('returncode')} "
                    f"timed_out={result.get('timed_out')}\n"
                    f"stderr={result.get('stderr')}\n"
                    f"stdout={result.get('stdout', '')[:2000]}"
                ),
            )

        for required in meta.get("required_path_globs", []):
            self.assertTrue(
                _glob_exists(root, required),
                f"{label}: missing required path {required}",
            )

        for rule in meta.get("required_content", []):
            path = root / rule["path"]
            self.assertTrue(path.is_file(), f"{label}: missing {rule['path']}")
            text = path.read_text()
            needles = rule.get("contains_any") or []
            self.assertTrue(
                any(n.lower() in text.lower() for n in needles),
                f"{label}: {rule['path']} missing any of {needles}\n---\n{text[:2000]}",
            )

        for pat in meta.get("forbidden_path_globs", []):
            hits = [str(p.relative_to(root)) for p in root.glob(pat)]
            # also match via fnmatch on changed set if needed
            self.assertEqual(hits, [], f"{label}: forbidden paths matched {pat}: {hits}")
            for p in root.rglob("*"):
                if p.is_file() and fnmatch.fnmatch(str(p.relative_to(root)), pat):
                    self.fail(f"{label}: forbidden path {p.relative_to(root)}")

    def test_live_scenarios(self):
        runtimes = _selected_runtimes()
        scenarios = _selected_scenarios()
        self.assertTrue(runtimes, "no eligible runtimes")
        self.assertTrue(scenarios, "no scenarios selected")
        trials = max(1, int(os.environ.get("EVALS_TRIALS", "1")))
        need = _n_of_m(0, trials)
        for runtime in runtimes:
            for scenario_id in scenarios:
                passed = 0
                errors: list[str] = []
                for _ in range(trials):
                    try:
                        self._run_scenario(scenario_id, runtime)
                        passed += 1
                    except AssertionError as exc:
                        errors.append(str(exc))
                self.assertGreaterEqual(
                    passed,
                    need if trials > 1 else 1,
                    msg=(
                        f"{runtime}/vault-canonical-store/{scenario_id}: "
                        f"passed {passed}/{trials} (need {need if trials > 1 else 1})\n"
                        + "\n---\n".join(errors[:3])
                    ),
                )


if __name__ == "__main__":
    unittest.main()
