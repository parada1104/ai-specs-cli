"""Live vault-canonical-store recipe evals. Requires EVALS_LIVE=1."""

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
from tests.evals.lib.vault_mcp_live import (  # noqa: E402
    cleanup_vault,
    claude_session_mcp_evidence,
    create_scoped_vault,
    mcp_tool_evidence,
    register_claude_local_mcp,
    sync_vault_mcp,
    unregister_claude_local_mcp,
    write_eval_mcp_config,
)

SCENARIOS_ROOT = REPO_ROOT / "tests" / "evals" / "scenarios" / "vault-canonical-store"

LIVE_SCENARIOS: tuple[str, ...] = (
    "ac_kepano_skills_present",
    "ac_mcp_path_with_spaces",
    "ac_vault_context_guidance",
    "ac_mcp_live_scope",
)

# Headless MCP connect is validated on subscription CLIs first.
MCP_LIVE_RUNTIMES = frozenset({"claude", "cursor-agent"})
MCP_LIVE_SCENARIO = "ac_mcp_live_scope"


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
    def _run_guidance_scenario(self, scenario_id: str, runtime: str):
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
            self.assertEqual(hits, [], f"{label}: forbidden paths matched {pat}: {hits}")
            for p in root.rglob("*"):
                if p.is_file() and fnmatch.fnmatch(str(p.relative_to(root)), pat):
                    self.fail(f"{label}: forbidden path {p.relative_to(root)}")

    def _run_mcp_live_scenario(self, runtime: str):
        if runtime not in MCP_LIVE_RUNTIMES:
            self.skipTest(
                f"{MCP_LIVE_SCENARIO} is validated on {sorted(MCP_LIVE_RUNTIMES)}; "
                f"skipping {runtime}"
            )

        scenario = load_scenario(SCENARIOS_ROOT / MCP_LIVE_SCENARIO)
        self.assertEqual(scenario.recipe_id, "vault-canonical-store")

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)

        version = recipe_version(REPO_ROOT / "catalog", "vault-canonical-store")
        materialize_project(root, "vault-canonical-store", version)
        seed_project_files(root)
        # Scope inside project so roots-capable filesystem MCP (Claude) can see
        # MARKER.md; sibling stays outside the project for denial checks.
        vault = create_scoped_vault(project_root=root)
        self.addCleanup(cleanup_vault, vault)
        setup_runtime_skills(
            root,
            runtime,
            "vault-canonical-store",
            catalog_root=REPO_ROOT / "catalog",
        )
        (root / "ai-specs" / "eval-notes").mkdir(parents=True, exist_ok=True)

        sync_vault_mcp(root, runtime)
        eval_mcp = write_eval_mcp_config(root, scoped_path=vault["scoped"])
        if runtime == "claude":
            # Avoid dual registration: project .mcp.json stays "pending" and
            # collisions make local-scope "tools fetch failed". Prefer local add.
            project_mcp = root / ".mcp.json"
            if project_mcp.is_file():
                project_mcp.rename(root / ".mcp.json.eval-backup")
            register_claude_local_mcp(root, scoped_path=vault["scoped"])
            self.addCleanup(unregister_claude_local_mcp, root)
        elif runtime == "cursor-agent":
            # Point Cursor at absolute-scope MCP config; enable for headless.
            cursor_mcp = root / ".cursor" / "mcp.json"
            cursor_mcp.parent.mkdir(parents=True, exist_ok=True)
            cursor_mcp.write_text(eval_mcp.read_text(encoding="utf-8"), encoding="utf-8")
            from tests.evals.lib.harness import resolve_runtime_binary

            cursor_bin = resolve_runtime_binary("cursor-agent") or "cursor-agent"
            subprocess.run(
                [cursor_bin, "mcp", "enable", "vault-canonical"],
                cwd=root,
                capture_output=True,
                text=True,
                check=False,
            )

        # Inherit process env for CLI auth. Inject vault scope for Cursor/env
        # expansion; Claude local MCP embeds the concrete path at add time.
        old_turns = os.environ.get("EVALS_MAX_TURNS")
        old_rt = os.environ.get("EVALS_RUNTIME")
        old_canonical = os.environ.get("CANONICAL_VAULT_PATH")
        old_obsidian = os.environ.get("OBSIDIAN_VAULT_PATH")
        old_bypass = os.environ.get("EVALS_CLAUDE_BYPASS")
        os.environ["EVALS_MAX_TURNS"] = os.environ.get("EVALS_MCP_MAX_TURNS", "20")
        os.environ["EVALS_RUNTIME"] = runtime
        os.environ["CANONICAL_VAULT_PATH"] = str(vault["scoped"])
        os.environ["EVALS_CLAUDE_BYPASS"] = "1"
        os.environ.pop("OBSIDIAN_VAULT_PATH", None)
        try:
            result = run_prompt(
                root,
                scenario.prompt_path.read_text(),
                runtime=runtime,
                mode=scenario.mode,
                # Claude: local MCP registration (not --mcp-config pending gate).
                mcp_config=None,
                approve_mcps=(runtime == "cursor-agent"),
                # Roots-capable filesystem MCP: scope must be in Claude roots.
                add_dirs=[vault["scoped"]] if runtime == "claude" else None,
            )
        finally:
            if old_turns is None:
                os.environ.pop("EVALS_MAX_TURNS", None)
            else:
                os.environ["EVALS_MAX_TURNS"] = old_turns
            if old_rt is None:
                os.environ.pop("EVALS_RUNTIME", None)
            else:
                os.environ["EVALS_RUNTIME"] = old_rt
            if old_canonical is None:
                os.environ.pop("CANONICAL_VAULT_PATH", None)
            else:
                os.environ["CANONICAL_VAULT_PATH"] = old_canonical
            if old_obsidian is None:
                os.environ.pop("OBSIDIAN_VAULT_PATH", None)
            else:
                os.environ["OBSIDIAN_VAULT_PATH"] = old_obsidian
            if old_bypass is None:
                os.environ.pop("EVALS_CLAUDE_BYPASS", None)
            else:
                os.environ["EVALS_CLAUDE_BYPASS"] = old_bypass

        label = f"{runtime}/vault-canonical-store/{MCP_LIVE_SCENARIO}"
        note = root / "ai-specs" / "eval-notes" / "vault-mcp-live.md"
        soft_ok = note.is_file() and f"token={vault['token']}" in note.read_text()
        # Claude often hits max-turns after the note is already written.
        if not soft_ok and not note.is_file():
            self.fail(
                f"{label}: missing note {note}\n"
                f"rc={result.get('returncode')} timed_out={result.get('timed_out')}\n"
                f"stderr={result.get('stderr')}\n"
                f"stdout={result.get('stdout', '')[:3000]}"
            )
        if not soft_ok:
            self.assertEqual(
                result["returncode"],
                0,
                msg=(
                    f"{label} failed rc={result.get('returncode')} "
                    f"timed_out={result.get('timed_out')}\n"
                    f"stderr={result.get('stderr')}\n"
                    f"stdout={result.get('stdout', '')[:3000]}"
                ),
            )

        self.assertTrue(note.is_file(), f"{label}: missing {note}")
        text = note.read_text()
        self.assertIn(
            f"token={vault['token']}",
            text,
            f"{label}: note missing scoped token\n---\n{text[:2000]}\n"
            f"stdout[:2000]={result.get('stdout', '')[:2000]}",
        )
        self.assertNotIn(
            vault["sibling"],
            text,
            f"{label}: sibling secret leaked into note (scope breach)\n---\n{text[:2000]}",
        )
        self.assertRegex(
            text,
            r"(?im)mcp_used\s*=\s*yes",
            f"{label}: expected mcp_used=yes\n---\n{text[:2000]}",
        )
        # Soft preference: agent reports denial. If they never tried, still OK
        # as long as sibling secret did not leak.
        if "sibling_access=allowed" in text.lower().replace(" ", ""):
            self.fail(
                f"{label}: agent reported sibling_access=allowed "
                f"(MCP must not read outside AllowedDirectories)\n---\n{text[:2000]}"
            )
        # Scope evidence: note should mention the scoped directory (spaces OK).
        scope_needle = vault["scoped"].name  # scoped-project
        self.assertIn(
            scope_needle,
            text,
            f"{label}: note should mention scoped dir {scope_needle!r}\n---\n{text[:2000]}",
        )

        # Tool evidence: Claude session jsonl must show mcp__vault-canonical__*.
        # Cursor Agent text output often omits tool names — accept when the note
        # has the random token + sibling denial (token is outside Bash-guessable
        # workspace reads only if MCP scoped correctly; marker is in-project so
        # also require sibling_access=denied language).
        require_tool = os.environ.get("EVALS_MCP_REQUIRE_TOOL_EVIDENCE", "1")
        if require_tool.lower() in {"1", "true", "yes"}:
            if runtime == "claude":
                evidence = claude_session_mcp_evidence(root)
            else:
                evidence = mcp_tool_evidence(result) or (
                    soft_ok
                    and "sibling_access=denied" in text.lower().replace(" ", "")
                )
            self.assertTrue(
                evidence,
                msg=(
                    f"{label}: no MCP tool evidence "
                    f"(Claude: need mcp__vault-canonical__* in session; "
                    f"cursor-agent: need token + sibling_access=denied; "
                    f"set EVALS_MCP_REQUIRE_TOOL_EVIDENCE=0 to soften)\n"
                    f"stdout[:2500]={result.get('stdout', '')[:2500]}"
                ),
            )

    def _run_scenario(self, scenario_id: str, runtime: str):
        if scenario_id == MCP_LIVE_SCENARIO:
            self._run_mcp_live_scenario(runtime)
            return
        self._run_guidance_scenario(scenario_id, runtime)

    def test_live_scenarios(self):
        runtimes = _selected_runtimes()
        scenarios = _selected_scenarios()
        self.assertTrue(runtimes, "no eligible runtimes")
        self.assertTrue(scenarios, "no scenarios selected")
        trials = max(1, int(os.environ.get("EVALS_TRIALS", "1")))
        need = _n_of_m(0, trials)
        for runtime in runtimes:
            for scenario_id in scenarios:
                if (
                    scenario_id == MCP_LIVE_SCENARIO
                    and runtime not in MCP_LIVE_RUNTIMES
                ):
                    continue
                passed = 0
                errors: list[str] = []
                for _ in range(trials):
                    try:
                        self._run_scenario(scenario_id, runtime)
                        passed += 1
                    except unittest.SkipTest:
                        raise
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
