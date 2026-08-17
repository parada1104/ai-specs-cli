"""Errexit and cleanup contract for the recipe-materialize capture block.

`lib/sync.sh` captures `recipe-materialize.py` with hand-rolled code that
predates `run_step`, and carried the same defect `run_step` was fixed for:
errexit restored before the block printed its own captured output.

Two cleanup properties matter here and are easy to break in opposite
directions:

- every temporary must be covered by the `trap … EXIT` from the moment it
  exists — registering the trap after the last `mktemp` leaves the earlier
  files unprotected across further fallible calls;
- the trap must survive `set -u`, since a trap naming an unset variable dies
  mid-cleanup and replaces the script's exit status with its own.

The block is inline top-level script, not a function, so it cannot be extracted
the way `run_step` is. These tests slice it out of the real source — starting
at the FIRST temp file, so every name the trap references exists in the harness
— and drive it with a stubbed materialize command.
"""

from __future__ import annotations

import re
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SYNC_SH = ROOT / "lib" / "sync.sh"


def capture_block() -> str:
    """The real block, from the FIRST temp file through its final cleanup.

    Starting at `RECIPE_OUT_FILE=` would exclude the three temporaries created
    before it, leaving those trap references to expand to empty strings — the
    suite could then not detect a trap registered too late, nor a typo in any
    of those three names.
    """
    text = SYNC_SH.read_text(encoding="utf-8")
    start = text.index('RECIPE_MCP_TEMP="$(mktemp')
    end = text.index("sync_agents_render()", start)
    return text[start:end]


class RecipeCaptureContractTests(unittest.TestCase):
    def _run(self, body: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", "-c", body], text=True, capture_output=True, check=False
        )

    def _harness(
        self, *, sabotage: str, rc: int = 0, trailer: str = ""
    ) -> subprocess.CompletedProcess:
        """Drive the real block with a stubbed materialize command."""
        probe = tempfile.mkdtemp()
        block = capture_block()
        # Replace the real python3 invocation with a controllable stub.
        block = re.sub(
            r"python3 \"\$RECIPE_MATERIALIZE_PY\".*?2>\"\$RECIPE_ERR_FILE\"",
            f"bash -c 'echo out; echo err >&2; exit {rc}' "
            '>"$RECIPE_OUT_FILE" 2>"$RECIPE_ERR_FILE"',
            block,
            flags=re.S,
        )
        script = textwrap.dedent(
            f"""
            set -euo pipefail
            VERBOSE=0
            print_step_output() {{ :; }}
            PROBE_DIR="{probe}"
            # A counter would not work here: `VAR="$(mktemp)"` runs the stub in
            # a command-substitution subshell, so an incremented counter never
            # escapes and every call would hand back the SAME path — which
            # silently made the cleanup assertions vacuous.
            mktemp() {{ command mktemp "$PROBE_DIR/tempXXXXXX"; }}
            {sabotage}
            {block}
            echo "REACHED_END"
            {trailer}
            """
        )
        result = self._run(script)
        survivors = sorted(p.name for p in Path(probe).iterdir())
        result.survivors = survivors  # type: ignore[attr-defined]
        return result

    def test_trap_covers_every_temp_file_from_the_moment_it_exists(self):
        """JD re-judgment: a late trap strands the temporaries created before it.

        Registering the trap only after ALL the `mktemp` calls leaves the
        earlier files unprotected across the remaining fallible calls. Under
        errexit, a failure at the fourth aborts before the trap exists.

        Measured on the two shapes: 3 stranded with a late trap, 0 with the
        trap registered up front.
        """
        probe = tempfile.mkdtemp()
        counter = Path(probe) / ".count"
        counter.write_text("0")
        block = capture_block()
        script = textwrap.dedent(
            f"""
            set -euo pipefail
            VERBOSE=0
            print_step_output() {{ :; }}
            PROBE_DIR="{probe}"
            CNT="{counter}"
            # A counter must live in a file: `VAR="$(mktemp)"` runs the stub in
            # a command-substitution subshell, so a shell variable never
            # escapes and no call would ever reach the failing branch.
            mktemp() {{
                n=$(( $(cat "$CNT") + 1 ))
                echo $n > "$CNT"
                [ $n -ge 4 ] && return 1
                command mktemp "$PROBE_DIR/tempXXXXXX"
            }}
            {block}
            echo "REACHED_END"
            """
        )
        self._run(script)
        stranded = sorted(
            p.name for p in Path(probe).iterdir() if p.name != ".count"
        )
        self.assertEqual(
            stranded,
            [],
            f"temp files created before the trap were stranded: {stranded}",
        )

    def test_exit_trap_cannot_clobber_the_exit_status(self):
        """A `set -u` trap referencing an unset name replaces the exit code.

        Observed directly: with a bare `$VAR` in the EXIT trap, a clean
        `exit 3` became exit 1 because the trap itself died. Every name in the
        trap must therefore be `:-` expanded.
        """
        block = capture_block()
        trap_lines = [l for l in block.splitlines() if l.startswith("trap ")]
        self.assertTrue(trap_lines, "the block should register an EXIT trap")
        for line in trap_lines:
            for name in re.findall(r"\$\{?([A-Z_][A-Z0-9_]*)", line):
                self.assertIn(
                    f"${{{name}:-}}",
                    line,
                    f"{name} is not :- expanded; the trap can die under set -u",
                )

    def test_block_is_still_present(self):
        """Guards the fixture: if the block moves, these tests must be updated."""
        block = capture_block()
        self.assertIn("RECIPE_RC", block)
        self.assertIn("rm -f", block)

    def test_a_failing_print_does_not_abort_before_cleanup(self):
        """The defect: errexit restored before the block's own cat calls."""
        result = self._harness(sabotage="cat() { return 1; }", rc=3)
        self.assertEqual(
            result.returncode,
            3,
            f"expected the step's own status, got {result.returncode}",
        )

    def test_capture_files_are_not_stranded_on_the_failure_path(self):
        result = self._harness(sabotage="cat() { return 1; }", rc=3)
        stranded = result.survivors
        self.assertEqual(
            stranded, [], f"capture files stranded: {result.survivors}"
        )

    def test_capture_files_are_removed_on_the_success_path(self):
        result = self._harness(sabotage="", rc=0)
        self.assertIn("REACHED_END", result.stdout)
        stranded = result.survivors
        self.assertEqual(
            stranded, [], f"capture files stranded: {result.survivors}"
        )

    def test_errexit_survives_the_block(self):
        """Nothing after the block may run with errexit silently disabled.

        Driven through `_harness` rather than a hand-built script: an earlier
        version rebuilt the prelude here and reintroduced the very
        counter-in-a-subshell stub this file warns about, so both capture files
        resolved to the same path — production always has two distinct ones.
        """
        # A marker that is not a substring of REACHED_END — an earlier version
        # used "REACHED", which the block's own "REACHED_END" always matched,
        # so the assertion could never pass regardless of behavior.
        result = self._harness(
            sabotage="", rc=0, trailer="false\necho ERREXIT_LEAKED"
        )
        self.assertIn("REACHED_END", result.stdout)
        self.assertNotIn(
            "ERREXIT_LEAKED",
            result.stdout,
            "errexit was left disabled after the block",
        )

    def test_capture_files_are_distinct_paths(self):
        """Guards the fixture itself against the same-path regression."""
        result = self._harness(
            sabotage="",
            rc=0,
            trailer='echo "OUT=$RECIPE_OUT_FILE"; echo "ERR=$RECIPE_ERR_FILE"',
        )
        paths = dict(
            line.split("=", 1)
            for line in result.stdout.splitlines()
            if line.startswith(("OUT=", "ERR="))
        )
        self.assertEqual(len(paths), 2, result.stdout)
        self.assertNotEqual(
            paths["OUT"], paths["ERR"], "both capture files got the same path"
        )


if __name__ == "__main__":
    unittest.main()
