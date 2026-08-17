"""Errexit contract for `run_step` in sync.sh and sync-agent.sh.

`run_step` disables errexit to capture the wrapped command's exit status. When
it restores errexit matters:

- restore too early and a failure inside the helper's own output handling
  (SIGPIPE on an early-closed stdout, a full disk) aborts the script from
  inside the helper — leaking both temp files, skipping the caller's error
  handling, and returning bash's status instead of the command's;
- never restore and errexit stays off for the rest of the script, because
  `set` options are shell-global rather than function-local.

Both helpers are intentionally twins, so every case runs against both.
"""

from __future__ import annotations

import shutil
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path

from test_sync_output_verbosity import (  # reuse the established harness
    SYNC_AGENT_SH,
    SYNC_SH,
    _extract_bash_functions,
)


SCRIPTS = (SYNC_SH, SYNC_AGENT_SH)


def run_harness(script: Path, body: str, *, verbose: int = 0):
    """Run run_step + print_step_output from `script` against `body`.

    The prelude mirrors the real scripts: both guard `shopt -s inherit_errexit`
    behind a support check before defining `run_step` (sync.sh:84, sync-agent.sh
    :200). Omitting it here would let an inherit_errexit-dependent regression
    pass unnoticed in the harness while failing in production.
    """
    fns = _extract_bash_functions(script, ("print_step_output", "run_step"))
    bash = (
        "set -euo pipefail\n"
        'if [[ -n "$(shopt -p inherit_errexit 2>/dev/null)" ]]; then\n'
        "    shopt -s inherit_errexit\n"
        "fi\n"
        f"VERBOSE={verbose}\n"
        f"{fns}\n"
        f"{body}\n"
    )
    return subprocess.run(
        ["bash", "-c", bash], text=True, capture_output=True, check=False
    )


class RunStepErrexitTests(unittest.TestCase):
    def test_errexit_is_active_after_a_successful_step(self):
        body = textwrap.dedent(
            """
            run_step "ok" true
            false
            echo "REACHED"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertNotIn(
                    "REACHED",
                    result.stdout,
                    "errexit was left disabled after a successful step",
                )
                self.assertNotEqual(result.returncode, 0)

    def test_errexit_is_active_after_a_handled_failing_step(self):
        body = textwrap.dedent(
            """
            if ! run_step "boom" bash -c 'exit 3'; then
                echo "HANDLED"
            fi
            false
            echo "REACHED"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertIn("HANDLED", result.stdout)
                self.assertNotIn(
                    "REACHED",
                    result.stdout,
                    "errexit was left disabled after a handled failure",
                )

    def test_a_bare_failing_step_still_aborts(self):
        body = textwrap.dedent(
            """
            run_step "boom" bash -c 'exit 7'
            echo "REACHED"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertNotIn("REACHED", result.stdout)
                self.assertEqual(
                    result.returncode, 7, "the wrapped command's status must survive"
                )

    def test_a_guarded_failing_step_yields_the_real_status(self):
        body = textwrap.dedent(
            """
            rc=0
            run_step "boom" bash -c 'exit 42' || rc=$?
            echo "RC=$rc"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertIn("RC=42", result.stdout)

    def _leak_probe(self, script: Path, step: str, *, extra: str = "") -> str:
        """Track the exact temp paths run_step creates, not a directory count.

        A `ls "$TMPDIR" | wc -l` delta is a shared-directory oracle: any other
        process writing to the same TMPDIR at that instant flips the result.
        Shadowing `mktemp` to hand out known paths makes the assertion exact.
        """
        probe_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, probe_dir, True)
        body = textwrap.dedent(
            f"""
            PROBE_DIR="{probe_dir}"
            _n=0
            mktemp() {{
                _n=$((_n+1))
                {extra}
                : > "$PROBE_DIR/temp$_n"
                echo "$PROBE_DIR/temp$_n"
            }}
            {step}
            for f in "$PROBE_DIR"/temp*; do
                [[ -e "$f" ]] && echo "SURVIVED=$(basename "$f")"
            done
            echo "PROBE_DONE"
            """
        )
        return run_harness(script, body).stdout

    def test_no_temporary_files_survive_a_successful_step(self):
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                out = self._leak_probe(script, 'run_step "ok" bash -c \'echo hello\'')
                self.assertIn("PROBE_DONE", out)
                self.assertNotIn("SURVIVED", out)

    def test_no_temporary_files_survive_a_failing_step(self):
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                out = self._leak_probe(
                    script,
                    "run_step \"boom\" bash -c 'echo out; echo err >&2; exit 5' || true",
                )
                self.assertIn("PROBE_DONE", out)
                self.assertNotIn("SURVIVED", out)

    def test_partial_mktemp_failure_does_not_leak_the_first_file(self):
        """JD: the branch where the FIRST mktemp succeeds and the second fails.

        `! A || ! B` short-circuits, so a stub that always fails never reaches
        the second call — that asymmetric branch, the only one where a real
        temp file must be cleaned up by `rm -f "${out_file:-}"`, was untested.
        """
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                out = self._leak_probe(
                    script,
                    "run_step \"step\" bash -c 'echo RAN' || true",
                    extra='[[ $_n -ge 2 ]] && return 1',
                )
                self.assertIn("RAN", out, "the step must still run")
                self.assertIn("PROBE_DONE", out)
                self.assertNotIn(
                    "SURVIVED", out, "the first temp file leaked when the second failed"
                )

    def test_mktemp_failure_still_forwards_a_failing_status(self):
        """The degraded path must not swallow the wrapped command's failure."""
        body = textwrap.dedent(
            """
            mktemp() { return 1; }
            rc=0
            run_step "boom" bash -c 'exit 17' || rc=$?
            echo "RC=$rc"
            false
            echo "REACHED"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertIn("RC=17", result.stdout)
                self.assertNotIn(
                    "REACHED",
                    result.stdout,
                    "errexit was not restored on the degraded path",
                )

    def test_degraded_path_output_is_documented_as_unfiltered(self):
        """Compact filtering cannot apply to output that is never captured.

        When temp files are unavailable the step runs unbuffered, so detail
        markers reach the terminal. That is a deliberate trade — the warning
        must say so rather than leaving the raw output unexplained.
        """
        body = textwrap.dedent(
            """
            mktemp() { return 1; }
            run_step "step" bash -c 'echo "  ✓ detail line"' || true
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                combined = result.stdout + result.stderr
                self.assertIn("✓ detail line", combined, "output must not be lost")
                self.assertIn(
                    "unfiltered",
                    combined.lower(),
                    "the warning must state that output is unfiltered",
                )

    def test_a_failing_step_still_prints_its_full_output(self):
        """The existing contract must not regress while moving the restore."""
        body = textwrap.dedent(
            """
            run_step "boom" bash -c 'echo STDOUT_MARK; echo STDERR_MARK >&2; exit 1' || true
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertIn("STDOUT_MARK", result.stdout)
                self.assertIn("STDERR_MARK", result.stderr)

    def test_a_failing_cat_does_not_abort_from_inside_the_helper(self):
        """The actual defect: errexit restored before the helper's own `cat`.

        `[[ -s f ]] && cat f` does not trip errexit when `[[` fails — a
        non-final command in an && list is exempt. It DOES trip when `cat`
        itself fails, which is reachable via SIGPIPE on an early-closed stdout
        or a full disk. With errexit already restored, that aborts the script
        from inside the helper: the wrapped command's status is lost and the
        temp files leak.

        A guarded call (`if ! run_step`) is exempt — bash suspends errexit for
        the whole invocation — so the defect is only reachable from a BARE call
        site, which is 5 of the 6 in sync.sh and all 4 in sync-agent.sh.
        """
        body = textwrap.dedent(
            """
            cat() { return 1; }          # simulate SIGPIPE / full disk
            before="$(ls "${TMPDIR:-/tmp}" | wc -l)"
            run_step "boom" bash -c 'echo out; exit 5'
            echo "NOT_REACHED"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertNotIn("NOT_REACHED", result.stdout)
                self.assertEqual(
                    result.returncode,
                    5,
                    "aborted from inside the helper on the failing cat, losing "
                    "the wrapped command's status",
                )

    def test_a_failing_cat_still_returns_the_real_status(self):
        body = textwrap.dedent(
            """
            cat() { return 1; }
            rc=0
            run_step "boom" bash -c 'echo out; exit 9' || rc=$?
            echo "RC=$rc"
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                self.assertIn("RC=9", result.stdout)

    def test_mktemp_failure_names_itself(self):
        """A TMPDIR problem must not masquerade as the wrapped command failing.

        `mktemp` is shadowed rather than pointing TMPDIR at a bad path: macOS
        `mktemp` ignores TMPDIR entirely (it uses the Darwin confstr temp dir),
        so a TMPDIR-based test would silently pass without ever exercising the
        guard.
        """
        body = textwrap.dedent(
            """
            mktemp() { return 1; }
            run_step "step" bash -c 'echo RAN' || true
            """
        )
        for script in SCRIPTS:
            with self.subTest(script=script.name):
                result = run_harness(script, body)
                combined = (result.stdout + result.stderr).lower()
                self.assertTrue(
                    "temporary" in combined or "tmpdir" in combined,
                    msg=f"mktemp failure was not named:\n{result.stdout}\n{result.stderr}",
                )
                self.assertIn("RAN", result.stdout, "the step must still run")


if __name__ == "__main__":
    unittest.main()
