"""Errexit and cleanup contract for the recipe-materialize capture block.

`lib/sync.sh:210-234` captures `recipe-materialize.py` with hand-rolled code
that predates `run_step`. It carries the same defect `run_step` was fixed for:
errexit restored before the block prints its own captured output.

It also has a gap `run_step` never had — its two temporary files are outside
the `trap … EXIT` registered a few lines above, so an abort strands them.

The block is inline top-level script, not a function, so it cannot be extracted
the way `run_step` is. These tests reproduce its exact shape instead, driven by
the real source so the fixture cannot silently drift from production.
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
    """The real block, from `RECIPE_OUT_FILE=` through its final `rm -f`."""
    text = SYNC_SH.read_text(encoding="utf-8")
    start = text.index('RECIPE_OUT_FILE="$(mktemp')
    end = text.index("sync_agents_render()", start)
    return text[start:end]


class RecipeCaptureContractTests(unittest.TestCase):
    def _run(self, body: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", "-c", body], text=True, capture_output=True, check=False
        )

    def _harness(self, *, sabotage: str, rc: int = 0) -> subprocess.CompletedProcess:
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
            """
        )
        result = self._run(script)
        survivors = sorted(p.name for p in Path(probe).iterdir())
        result.survivors = survivors  # type: ignore[attr-defined]
        return result

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
        result = self._harness(sabotage="", rc=0)
        self.assertIn("REACHED_END", result.stdout)
        # Nothing after the block may run with errexit silently disabled.
        follow = self._run(
            "set -euo pipefail\n"
            "VERBOSE=0\n"
            "print_step_output() { :; }\n"
            f"PROBE_DIR={tempfile.mkdtemp()}\n"
            "_n=0\n"
            'mktemp() { _n=$((_n+1)); : > "$PROBE_DIR/t$_n"; echo "$PROBE_DIR/t$_n"; }\n'
            + re.sub(
                r"python3 \"\$RECIPE_MATERIALIZE_PY\".*?2>\"\$RECIPE_ERR_FILE\"",
                'bash -c \'true\' >"$RECIPE_OUT_FILE" 2>"$RECIPE_ERR_FILE"',
                capture_block(),
                flags=re.S,
            )
            + "\nfalse\necho REACHED\n"
        )
        self.assertNotIn(
            "REACHED", follow.stdout, "errexit was left disabled after the block"
        )


if __name__ == "__main__":
    unittest.main()
