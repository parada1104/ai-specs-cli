"""Errexit, cleanup and capture contract for the recipe-materialize block.

The spine captures `recipe-materialize.py` into temp files, derives the recipe
names from its stdout, and replays it filtered on success / raw on failure.
Two cleanup properties matter and are easy to break in opposite directions:

- every temporary must be covered by the cleanup from the moment it exists —
  registering cleanup after the last `mktemp` leaves the earlier files
  unprotected across further fallible calls;
- a failure after the block must still propagate, not be swallowed by the
  capture handling.

The original suite sliced the Bash block out of `lib/sync.sh`. That subject is
Bash source structure, so it stopped gating the port. The contracts are
re-driven here through the real CLI with a PATH `python3` stub (scripted
materialize output/rc) and a PATH `mktemp` stub (recorded probe paths).
"""

from __future__ import annotations

import shlex
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

MATERIALIZE = "recipe-materialize.py"
RECIPE_FLAGS = (
    "--recipe-mcp-out",
    "--resolved-config-out",
    "--resolved-hooks-out",
)


class RecipeCaptureContractTests(unittest.TestCase):
    def stub_sync(self, intercepts=(), *, args=(), fail_from=None):
        base = Path(tempfile.mkdtemp(prefix="ai-specs-recipe-"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        stubs = _sync_stub.SyncStubs(base)
        project = _sync_stub.minimal_project(base)
        for module, kwargs in intercepts:
            stubs.intercept(module, **kwargs)
        if fail_from is not None:
            stubs.mktemp_fail_from(fail_from)
        proc = subprocess.run(
            [str(CLI), "sync", str(project), *args],
            env=stubs.env(),
            text=True,
            capture_output=True,
            check=False,
        )
        return proc, stubs

    def _materialize_invocation(self, stubs) -> list[str]:
        lines = [
            line for line in stubs.invocations() if MATERIALIZE in line
        ]
        self.assertEqual(len(lines), 1, f"expected one materialize call: {lines}")
        return shlex.split(lines[0])

    def _recipe_paths(self, stubs) -> list[str]:
        tokens = self._materialize_invocation(stubs)
        return [tokens[tokens.index(flag) + 1] for flag in RECIPE_FLAGS]

    # ── recipe name extraction ───────────────────────────────────────────

    def test_recipe_names_are_extracted_from_materialize_stdout(self):
        proc, _ = self.stub_sync(
            [(MATERIALIZE, {"rc": 0, "stdout": "  ▸ recipe alpha\n  ▸ recipe beta\n"})]
        )
        self.assertIn("  syncing recipes → alpha, beta\n", proc.stdout)

    def test_no_recipes_prints_a_bare_syncing_line(self):
        proc, _ = self.stub_sync([(MATERIALIZE, {"rc": 0, "stdout": ""})])
        self.assertIn("  syncing recipes\n", proc.stdout)
        self.assertNotIn("  syncing recipes →", proc.stdout)

    # ── success / failure replay ─────────────────────────────────────────

    def test_success_replays_filtered_output(self):
        proc, _ = self.stub_sync(
            [
                (
                    MATERIALIZE,
                    {
                        "rc": 0,
                        "stdout": "    ✓ detail\nplain-out\n",
                        "stderr": "notice\n",
                    },
                )
            ]
        )
        self.assertIn("plain-out\n", proc.stdout)
        self.assertNotIn("✓ detail", proc.stdout, "compact must drop the glyph")
        self.assertIn("notice\n", proc.stderr, "stderr must be replayed")

    def test_failing_capture_returns_its_own_status_and_full_output(self):
        proc, _ = self.stub_sync(
            [
                (
                    MATERIALIZE,
                    {
                        "rc": 3,
                        "stdout": "    ✓ detail\n",
                        "stderr": "err-detail\n",
                    },
                )
            ]
        )
        self.assertEqual(proc.returncode, 3, proc.stderr)
        # Failure prints the capture raw in BOTH streams.
        self.assertIn("    ✓ detail\n", proc.stdout)
        self.assertIn("err-detail\n", proc.stderr)

    # ── cleanup ──────────────────────────────────────────────────────────

    def test_capture_files_are_removed_on_success_and_failure(self):
        proc, stubs = self.stub_sync([(MATERIALIZE, {"rc": 0, "stdout": ""})])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "success path stranded captures")

        proc, stubs = self.stub_sync([(MATERIALIZE, {"rc": 3})])
        self.assertEqual(proc.returncode, 3, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "failure path stranded captures")

    def test_trap_covers_temps_from_the_moment_they_exist(self):
        """Failing at the 4th mktemp must still leave zero survivors."""
        proc, stubs = self.stub_sync([(MATERIALIZE, {"rc": 0})], fail_from=4)
        self.assertEqual(proc.returncode, 1, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "temporaries were stranded")

    def test_recipe_temps_are_not_stranded_when_the_fourth_recipe_capture_fails(
        self,
    ):
        """The genuine late-registration defect: cleanup after the third temp.

        `mktemp_fail_from=4` fails during an earlier run_step, so it does not
        reach the recipe block's `-t` temps at all. Here the boundary is
        derived from a control run, so the failure lands exactly on the fourth
        recipe temp (the first capture file) and the three already-created
        `-t` temps are the ones at risk if cleanup is registered too late.
        """
        _, control = self.stub_sync([(MATERIALIZE, {"rc": 0})])
        boundary = max(
            int(path.rsplit("temp", 1)[1]) for path in self._recipe_paths(control)
        ) + 1
        proc, stubs = self.stub_sync([(MATERIALIZE, {"rc": 0})], fail_from=boundary)
        self.assertEqual(proc.returncode, 1, proc.stderr)
        self.assertEqual(
            stubs.survivors(),
            [],
            "the three `-t` temps created before cleanup registration were "
            "stranded",
        )

    # ── propagation, flags and path distinctness ─────────────────────────

    def test_failures_after_the_block_still_propagate(self):
        proc, _ = self.stub_sync(
            [
                (MATERIALIZE, {"rc": 0, "stdout": ""}),
                ("brief-render-policy.py", {"rc": 0, "stdout": "true\n"}),
                ("agents-render.py", {"rc": 9, "stdout": "oops\n"}),
            ]
        )
        self.assertEqual(proc.returncode, 9, proc.stderr)

    def test_refresh_gates_is_forwarded(self):
        proc, stubs = self.stub_sync(
            [(MATERIALIZE, {"rc": 0, "stdout": ""})], args=("--refresh-gates",)
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("--refresh-gates", self._materialize_invocation(stubs))

        _, stubs = self.stub_sync([(MATERIALIZE, {"rc": 0, "stdout": ""})])
        self.assertNotIn("--refresh-gates", self._materialize_invocation(stubs))

    def test_five_temps_are_distinct(self):
        proc, stubs = self.stub_sync([(MATERIALIZE, {"rc": 0, "stdout": ""})])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        # The probe reached at least the five recipe-related temps.
        self.assertGreaterEqual(stubs.created_count(), 5)
        paths = self._recipe_paths(stubs)
        self.assertTrue(all(paths), f"empty recipe temp path: {paths}")
        self.assertEqual(
            len(set(paths)), 3, f"recipe capture paths collided: {paths}"
        )


if __name__ == "__main__":
    unittest.main()
