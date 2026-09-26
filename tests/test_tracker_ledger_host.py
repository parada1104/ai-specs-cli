"""Direct host-mode contract for the Tracker ledger lifecycle checkpoints.

The `pre-merge` / `archive-close` lifecycle used to be hosted by a dedicated
Python module (``lib/_internal/tracker_ledger_host.py``). That host is retired:
the contract now lives in the existing shell bridge
``catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`` as a direct,
argv-selected host mode, and the merge skills/commands call it there.

These tests pin the direct mode:

  * ``--root <root> --checkpoint pre-merge|archive-close`` grades through the one
    verified Go predicate with bridge-built ``--evidence``;
  * the ``--stage pre-merge|pre-archive`` compatibility alias, and the rule that
    contradictory flags grade nothing;
  * ask/no-TTY, fail-open binary resolution, and the ``off`` skip;
  * every Tracker verdict is advisory: a ``block``/``ask`` is reported on
    stderr and the direct host still exits 0; only usage errors exit non-zero;
  * host mode is selected by argv, never by a piped hook payload;
  * ``archive-close`` is tracker item closure, never an OpenSpec archive, and the
    host never infers a tracker write.

A stub ``worktree-gate`` binary records its argv, so no test re-implements the go
predicate. `tests.test_tracker_card_gate_hook` keeps the pre-tool-use hook
contract (path/shell) for the same script.
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "trello-mcp-workflow" / "hooks" / "tracker-card-gate.sh"
LIB_INTERNAL = ROOT / "lib" / "_internal"
RETIRED_HOST = ROOT / "lib" / "_internal" / "tracker_ledger_host.py"

STUB_BINARY = """#!/usr/bin/env bash
# Mode resolution is Go-owned now: the shell host forwards the raw
# `--ledger-gate-mode` hint instead of skipping `off` itself. So this double
# mirrors Go's resolved-off short-circuit - a stderr note and exit 0 with no
# checkpoint verdict, grading or log entry - before it records any argv.
gate_mode=""
want_mode=0
for arg in "$@"; do
  if [ "$want_mode" = 1 ]; then gate_mode="$arg"; want_mode=0
  elif [ "$arg" = "--ledger-gate-mode" ]; then want_mode=1
  fi
done
if [ "$gate_mode" = off ]; then
  printf 'worktree-gate: ledger_mode off; skipping checkpoint\\n' >&2
  exit 0
fi
printf '%s\\n' "$*" >> "${STUB_LOG}"
decision="${STUB_DECISION:-allow}"
reason="${STUB_REASON:-}"
checkpoint=""
wrote=0
while [ $# -gt 0 ]; do
  case "$1" in
    --checkpoint) checkpoint="$2"; shift 2 ;;
    --decide) printf 'DECIDE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    --write) printf 'WRITE %s\\n' "$2" >> "${STUB_LOG}"; wrote=1; shift 2 ;;
    --evidence) printf 'EVIDENCE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    *) shift ;;
  esac
done
if [ "$wrote" = 1 ] && [ "${STUB_WRITE_EXIT:-0}" = 2 ]; then
  exit 2
fi
prompt='null'
if [ "$decision" = ask ]; then
  prompt='{"reason":"needs-item","evidence":{"local":"","remote":"","code":"","git":""},"choices":["continue","local"]}'
fi
printf '{"capability":"tracker","active":true,"checkpoint":"%s","mode":"warn","decision":"%s","reason":"%s","identity":{"common_dir":"","branch":"","change":null,"key":""},"item":null,"conflict":null,"prompt":%s,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}\\n' "$checkpoint" "$decision" "$reason" "$prompt"
[ "$decision" = block ] && exit 2
exit 0
"""


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True)


class TrackerLedgerHostDirectModeTests(unittest.TestCase):
    """`tracker-card-gate.sh` grades lifecycle checkpoints as a direct host."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.repo = base / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        self.stub_log = base / "stub.log"
        self.stub = base / "worktree-gate"
        self.stub.write_text(STUB_BINARY)
        self.stub.chmod(0o755)

    def _stamped_host(self, mode: str = "warn") -> Path:
        stamped = Path(self.tmp.name) / f"tracker-card-gate-host-{mode}.sh"
        stamped.write_text(
            GATE.read_text()
            .replace("__TRACKER_CARD_GATE_MODE__", mode)
            .replace("__TRACKER_CLI_HOME__", "")
            .replace("__TRACKER_LIB_INTERNAL__", str(LIB_INTERNAL))
        )
        stamped.chmod(0o755)
        return stamped

    def _env(self, *, decision: str = "allow", reason: str = "", **extra: str) -> dict:
        env = dict(os.environ)
        for key in ("TRACKER_CARD_GATE_MODE", "TRACKER_LEDGER_MODE",
                    "TRACKER_CARD_GATE_PATHS", "AI_SPECS_HOME"):
            env.pop(key, None)
        env["WORKTREE_GATE_BIN"] = str(self.stub)
        env["STUB_LOG"] = str(self.stub_log)
        env["STUB_DECISION"] = decision
        env["STUB_REASON"] = reason
        env.update(extra)
        return env

    def _change(self, slug: str = "demo-change") -> Path:
        folder = self.repo / "openspec" / "changes" / slug
        folder.mkdir(parents=True, exist_ok=True)
        (folder / "proposal.md").write_text(
            "## Tracker\n\n- **card_id**: `6aa703fdcf61a90ec702d58b`\n", encoding="utf-8"
        )
        return folder

    def _run_host(
        self,
        *args: str,
        mode: str = "warn",
        decision: str = "allow",
        reason: str = "",
        env: dict | None = None,
        stdin: str = "",
    ) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(self._stamped_host(mode)), *args],
            input=stdin,
            capture_output=True,
            text=True,
            # A new session has no controlling terminal, so the ask prompt's
            # /dev/tty open fails deterministically instead of blocking.
            start_new_session=True,
            env=env or self._env(decision=decision, reason=reason),
        )

    def _logged(self) -> list[str]:
        return self.stub_log.read_text().splitlines() if self.stub_log.exists() else []

    def _logged_checkpoints(self) -> list[str]:
        checkpoints = []
        for line in self._logged():
            parts = line.split()
            for i, token in enumerate(parts):
                if token == "--checkpoint" and i + 1 < len(parts):
                    checkpoints.append(parts[i + 1])
        return checkpoints

    def _grade_lines(self) -> list[str]:
        return [l for l in self._logged() if "--checkpoint" in l and "--write" not in l]

    def _write_lines(self) -> list[str]:
        return [l for l in self._logged() if l.startswith("WRITE ")]

    # --- the retired Python host ---

    def test_the_python_lifecycle_host_is_gone(self):
        """No Python host file backs `pre-merge`/`archive-close` any more."""
        self.assertFalse(RETIRED_HOST.exists(),
                         "the tracker lifecycle host is the shell bridge's direct mode")

    # --- direct checkpoint grading ---

    def test_pre_merge_grades_with_bridge_evidence(self):
        self._change()
        r = self._run_host("--root", str(self.repo), "--checkpoint", "pre-merge", "demo-change")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["pre-merge"])
        self.assertIn("--evidence", self._grade_lines()[0])

    def test_archive_close_grades_without_inferring_a_write(self):
        self._change()
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "archive-close", "demo-change"
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["archive-close"])
        self.assertEqual(self._write_lines(), [],
                         "the lifecycle host never infers a tracker close write")

    def test_stage_alias_maps_pre_archive_to_archive_close(self):
        r = self._run_host("--root", str(self.repo), "--stage", "pre-archive")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["archive-close"])

    def test_default_checkpoint_is_pre_merge(self):
        r = self._run_host("--root", str(self.repo))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["pre-merge"])

    def test_conflicting_checkpoint_and_stage_grade_nothing(self):
        r = self._run_host(
            "--root", str(self.repo),
            "--checkpoint", "archive-close", "--stage", "pre-merge",
        )
        self.assertNotEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [],
                         "a contradictory lifecycle request must not grade anything")

    def test_root_is_required(self):
        r = self._run_host("--checkpoint", "pre-merge")
        self.assertNotEqual(r.returncode, 0, r.stderr)
        self.assertIn("--root", r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_root_resolves_to_the_git_toplevel(self):
        nested = self.repo / "nested" / "deeper"
        nested.mkdir(parents=True)
        r = self._run_host("--root", str(nested), "--checkpoint", "pre-merge")
        self.assertEqual(r.returncode, 0, r.stderr)
        parts = self._grade_lines()[0].split()
        root = parts[parts.index("--project-root") + 1]
        self.assertEqual(os.path.realpath(root), os.path.realpath(str(self.repo)))

    # --- verdict mapping ---

    def test_block_verdict_reported_advisory_without_blocking(self):
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "pre-merge", decision="block"
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("pre-merge", r.stderr)
        self.assertIn("advisory", r.stderr.lower())

    def test_ask_without_tty_reports_pending_without_recording(self):
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "pre-merge", decision="ask"
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("no terminal", r.stderr)
        self.assertNotIn("DECIDE", self.stub_log.read_text())

    def test_ask_identity_unavailable_reports_and_proceeds(self):
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "pre-merge",
            decision="ask", reason="identity_unavailable",
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("identity_unavailable", r.stderr)
        self.assertNotIn("DECIDE", self.stub_log.read_text())

    def test_gate_mode_off_skips_the_checkpoint(self):
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "pre-merge",
            mode="off", decision="block",
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_missing_verified_binary_fails_open(self):
        env = self._env(decision="block")
        env.pop("WORKTREE_GATE_BIN", None)
        env["AI_SPECS_HOME"] = str(Path(self.tmp.name) / "cold-home")
        r = self._run_host("--root", str(self.repo), "--checkpoint", "pre-merge", env=env)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    # --- argv selection, not stdin ---

    def test_host_mode_is_selected_by_argv_not_a_hook_payload(self):
        stdin = json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Edit",
            "tool_input": {"file_path": str(self.repo / "lib" / "foo.py")},
            "cwd": str(self.repo),
        })
        r = self._run_host(
            "--root", str(self.repo), "--checkpoint", "archive-close",
            decision="block", stdin=stdin,
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["archive-close"],
                         "the requested checkpoint wins over any piped hook payload")


if __name__ == "__main__":
    unittest.main()
