"""Errexit contract for the sync spine, driven at the CLI boundary.

`run_step` disables errexit to capture the wrapped command's exit status. When
it restores errexit matters: restore too early and a failure in the helper's
own output handling aborts from inside the helper (leaking temp files and
returning the wrong status); never restore and errexit stays off for the rest
of the pipeline.

The original suite extracted `run_step`/`print_step_output` bodies out of
`lib/sync.sh` and `lib/sync-agent.sh` and ran them under bash. That subject is
Bash source structure, which stops being authoritative now that `sync` is
native: the tests silently stopped gating the port. They are replaced here by
the same observable contracts exercised through the real CLI, with PATH stubs
for `python3` (scripted module output/rc) and `mktemp` (recorded probe paths).
Both spines resolve those two names through PATH, so the seam is
implementation-independent — `test_compact_filter_is_implementation_independent`
proves that explicitly by running the same scenario against both.
"""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))

import _go_cli  # noqa: E402
import _sync_stub  # noqa: E402

CLI = _go_cli.cli()

# The first two sync steps (and the label of the third), used to observe
# whether errexit aborted the pipeline at the right point.
STEP_ONE = "gitignore-render.py"
STEP_TWO = "gitignore-root-refresh.py"
STEP_ONE_LABEL = "  syncing ai-specs/.gitignore"
STEP_TWO_LABEL = "  syncing root .gitignore (agent block)"
STEP_THREE_LABEL = "  syncing bundled skills + commands"

# The compact filter's exact classification: drop these leading glyphs, keep
# the notices, on their original streams.
GLYPH_LINES = (
    "    ✓ ok-detail",
    "    · dot-noise",
    "    ⇢ arrow-noise",
    "    ▸ recipe-noise",
)
NOTICE_LINES = ("  ! warning", "  ✗ error", "  ℹ notice")


class _SyncCliCase(unittest.TestCase):
    """Build a temp project, install the PATH stubs, run one `sync`."""

    def stub_sync(self, intercepts=(), *, args=(), fail_from=None, cli=None):
        base = Path(tempfile.mkdtemp(prefix="ai-specs-errc-"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        stubs = _sync_stub.SyncStubs(base)
        project = _sync_stub.minimal_project(base)
        for module, kwargs in intercepts:
            stubs.intercept(module, **kwargs)
        if fail_from is not None:
            stubs.mktemp_fail_from(fail_from)
        proc = subprocess.run(
            [str(cli or CLI), "sync", str(project), *args],
            env=stubs.env(),
            text=True,
            capture_output=True,
            check=False,
        )
        return proc, stubs


class RunStepErrexitTests(_SyncCliCase):
    def test_errexit_is_active_after_a_successful_step(self):
        """A later step's failure must abort the pipeline, not be swallowed."""
        proc, _ = self.stub_sync(
            [
                (STEP_ONE, {"rc": 0}),
                (STEP_TWO, {"rc": 7}),
            ]
        )
        self.assertEqual(proc.returncode, 7, proc.stderr)
        self.assertNotIn(
            STEP_THREE_LABEL,
            proc.stdout,
            "errexit was left disabled after a successful step",
        )

    def test_a_bare_failing_step_keeps_the_wrapped_status(self):
        """The wrapped command's status must survive, not collapse to 1."""
        proc, _ = self.stub_sync([(STEP_ONE, {"rc": 7})])
        self.assertEqual(proc.returncode, 7, proc.stderr)
        self.assertNotIn(STEP_TWO_LABEL, proc.stdout)

    def test_a_failing_step_prints_its_full_output(self):
        """Failure replays the full captured stdout/stderr, unfiltered."""
        proc, _ = self.stub_sync(
            [
                (
                    STEP_ONE,
                    {
                        "rc": 5,
                        "stdout": "STDOUT_MARK\n    ✓ detail\n",
                        "stderr": "STDERR_MARK\n",
                    },
                )
            ]
        )
        self.assertEqual(proc.returncode, 5, proc.stderr)
        self.assertIn("STDOUT_MARK", proc.stdout)
        self.assertIn("STDERR_MARK", proc.stderr)
        # The glyph line is printed RAW: compact filtering only applies to the
        # success path.
        self.assertIn("    ✓ detail\n", proc.stdout)

    def test_no_temporary_files_survive_success_or_failure(self):
        proc, stubs = self.stub_sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "success path stranded temps")

        proc, stubs = self.stub_sync([(STEP_ONE, {"rc": 7})])
        self.assertNotEqual(proc.returncode, 0)
        self.assertEqual(stubs.survivors(), [], "failure path stranded temps")

    def test_partial_mktemp_failure_runs_the_step_and_removes_the_first_temp(self):
        """First mktemp ok, second fails: the step still runs, no leak."""
        proc, stubs = self.stub_sync(
            [(STEP_ONE, {"rc": 0, "stdout": "STEP_RAN\n"})], fail_from=2
        )
        self.assertIn(STEP_ONE_LABEL, proc.stdout)
        self.assertIn("STEP_RAN", proc.stdout, "the step must still run")
        self.assertIn(
            "  ! cannot create temporary files",
            proc.stderr,
            "the degraded path must announce itself",
        )
        self.assertEqual(
            stubs.survivors(), [], "the first temp leaked when the second failed"
        )

    def test_degraded_path_forwards_the_wrapped_status(self):
        """A mktemp failure must not swallow the wrapped command's rc."""
        proc, _ = self.stub_sync([(STEP_ONE, {"rc": 17})], fail_from=1)
        self.assertEqual(proc.returncode, 17, proc.stderr)
        self.assertNotIn(
            STEP_TWO_LABEL,
            proc.stdout,
            "errexit was not honored on the degraded path",
        )

    def test_degraded_path_output_is_documented_as_unfiltered(self):
        """No captures means no filtering; the warning must say so."""
        proc, _ = self.stub_sync(
            [(STEP_ONE, {"rc": 0, "stdout": "  ✓ detail line\n"})], fail_from=1
        )
        self.assertIn("  ✓ detail line", proc.stdout, "output must not be lost")
        self.assertIn(
            "unfiltered",
            (proc.stdout + proc.stderr).lower(),
            "the warning must state that output is unfiltered",
        )

    def test_mktemp_failure_names_itself(self):
        """A TMPDIR problem must not masquerade as the wrapped command failing."""
        proc, _ = self.stub_sync([(STEP_ONE, {"rc": 0})], fail_from=1)
        combined = (proc.stdout + proc.stderr).lower()
        self.assertTrue(
            "temporary" in combined or "tmpdir" in combined,
            msg=f"mktemp failure was not named:\n{proc.stdout}\n{proc.stderr}",
        )

    def test_compact_filter_is_implementation_independent(self):
        """The PATH seam makes one test cover both spines identically."""
        payload = (
            "\n".join(GLYPH_LINES)
            + "\n\n   \nkeep-me\n"
            + "\n".join(NOTICE_LINES)
            + "\n"
        )
        results: dict[str, tuple[list[str], list[str]]] = {}
        stdouts: dict[str, str] = {}
        for name, cli in (("go", _go_cli.cli()), ("legacy", _go_cli.legacy_cli())):
            proc, _ = self.stub_sync(
                [(STEP_ONE, {"rc": 0, "stdout": payload})], cli=cli
            )
            self.assertEqual(proc.returncode, 0, f"{name}: {proc.stderr}")
            stdouts[name] = proc.stdout
            dropped = [line for line in GLYPH_LINES if line not in proc.stdout]
            kept = [line for line in NOTICE_LINES if line in proc.stdout]
            results[name] = (dropped, kept)
        self.assertEqual(results["go"][0], list(GLYPH_LINES), "glyph lines not dropped")
        self.assertEqual(results["go"][1], list(NOTICE_LINES), "notices not kept")
        self.assertIn("keep-me", stdouts["go"], "ordinary lines must survive")
        self.assertEqual(
            results["go"],
            results["legacy"],
            "the two spines classify the compact output differently",
        )


if __name__ == "__main__":
    unittest.main()
