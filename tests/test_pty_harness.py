"""Regression for the PTY staging helper (tests/test_init_tui.py::_spawn_pty).

A child may print two prompt markers in one write. The helper must still
answer the second one: a marker that arrived in the same chunk as the first
is consumed, not lost, and its stage fires.
"""
import os
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

import tests.test_init_tui as m

# One write carries both markers; the child then reads one line and exits 0.
FIXTURE = (
    "import sys\n"
    "sys.stdout.write('Skipped dep-tail\\r\\nWrite ai-specs.toml with these choices? (Y/n) ')\n"
    "sys.stdout.flush()\n"
    "line = sys.stdin.readline()\n"
    "sys.exit(0 if line else 3)\n"
)


class PtyHelperCoalescedChunksTests(unittest.TestCase):
    def test_second_marker_in_the_same_chunk_still_fires(self):
        base = Path(tempfile.mkdtemp(prefix="ai-specs-pty-harness-", dir=os.environ.get("TMPDIR")))
        self.addCleanup(shutil.rmtree, base, True)
        home = base / "home"
        home.mkdir()
        target = base / "target"
        target.mkdir()
        stages = [
            (b"Skipped dep-tail", b"qqq"),
            (b"Write ai-specs", b"\n"),
        ]
        rc, out = m._spawn_pty(
            target, home, base, stages=stages, timeout=8,
            argv=[sys.executable, "-c", FIXTURE],
        )
        self.assertEqual(rc, 0, f"second stage never fired; output: {out!r}")


if __name__ == "__main__":
    unittest.main()
