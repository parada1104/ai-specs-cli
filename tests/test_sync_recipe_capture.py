"""Errexit, cleanup and capture contract for the recipe-materialize block.

GO-07 S14c moved the materializer in-process: the spine no longer spawns
``recipe-materialize.py``, so the old PATH ``python3`` stub cannot script that
step any more. The contract is re-driven here through the real CLI against the
real native step: the fixtures enable catalog recipes under ``AI_SPECS_HOME``,
and the ``mktemp`` stub still records the temp sequence (the spine keeps
execcing ``mktemp`` through PATH), which is what locates each temp.

Two cleanup properties matter and are easy to break in opposite directions:

- every temporary must be covered by the cleanup from the moment it exists —
  registering cleanup after the last ``mktemp`` leaves the earlier files
  unprotected across further fallible calls;
- a failure after the block must still propagate, not be swallowed by the
  capture handling.
"""

from __future__ import annotations

import re
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

# A light catalog recipe that materializes offline, used to make the native step
# print recipe names worth extracting.
RECIPE = "session-context"
# An enabled recipe whose id trips the internal-test guard. The guard runs
# before any catalog read, so this fixture is deterministic and its stderr is
# byte-identical between the native core and the Python authority.
FATAL_RECIPE = "test-ghost"


class RecipeCaptureContractTests(unittest.TestCase):
    def stub_sync(self, *, recipes=(), intercepts=(), args=(), fail_from=None):
        base = Path(tempfile.mkdtemp(prefix="ai-specs-recipe-"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        stubs = _sync_stub.SyncStubs(base)
        project = _sync_stub.minimal_project(base)
        if recipes:
            manifest = project / "ai-specs" / "ai-specs.toml"
            manifest.write_text(
                manifest.read_text(encoding="utf-8")
                + "".join(f"\n[recipes.{rid}]\nenabled = true\n" for rid in recipes),
                encoding="utf-8",
            )
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

    def _recipe_temp_numbers(self, stubs) -> list[int]:
        """The global ``mktemp`` call numbers of the recipe block's five temps.

        The block creates three ``-t`` temps (recipe MCP, resolved-config,
        resolved-hooks) and then two bare capture files, in that order, so the
        first one anchors the five.
        """
        calls = stubs.mktemp_calls()
        anchors = [i for i, args in enumerate(calls) if "ai-specs-recipe-mcp-" in args]
        self.assertEqual(len(anchors), 1, f"recipe temp anchor not unique: {anchors}")
        return [anchors[0] + n for n in range(1, 6)]

    def _normalize_paths(self, text: str, base: Path) -> str:
        return text.replace(str(base), "<BASE>")

    # ── recipe name extraction ───────────────────────────────────────────

    def test_recipe_names_are_extracted_from_the_native_stdout(self):
        proc, _ = self.stub_sync(recipes=[RECIPE])
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn(f"  syncing recipes → {RECIPE}\n", proc.stdout)

    def test_no_recipes_prints_a_bare_syncing_line(self):
        proc, _ = self.stub_sync()
        self.assertIn("  syncing recipes\n", proc.stdout)
        self.assertNotIn("  syncing recipes →", proc.stdout)

    # ── success / failure replay ─────────────────────────────────────────

    def test_success_replays_filtered_output(self):
        proc, _ = self.stub_sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn(
            "  (no [recipes.*] enabled — skipping)\n",
            proc.stdout,
            "the plain notice must survive the compact filter",
        )
        self.assertNotIn(
            "✓ wrote resolved-config",
            proc.stdout,
            "compact must drop the glyph line",
        )
        self.assertEqual(proc.stderr, "", "a successful native step writes no stderr")

    def test_failing_capture_returns_status_one_and_replays_both_streams(self):
        proc, _ = self.stub_sync(recipes=[FATAL_RECIPE])
        self.assertEqual(proc.returncode, 1, proc.stderr)
        # Failure replays the capture raw in BOTH streams: the partial stdout
        # and the fatal line, which compact does not filter.
        self.assertIn("  syncing recipes\n", proc.stdout)
        self.assertEqual(
            proc.stderr,
            "  ✗ Recipe 'test-ghost' is an internal test fixture and is not "
            "part of the public catalog.\n",
        )

    # ── cleanup ──────────────────────────────────────────────────────────

    def test_capture_files_are_removed_on_success_and_failure(self):
        proc, stubs = self.stub_sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "success path stranded captures")

        proc, stubs = self.stub_sync(recipes=[FATAL_RECIPE])
        self.assertEqual(proc.returncode, 1, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "failure path stranded captures")

    def test_trap_covers_temps_from_the_moment_they_exist(self):
        """Failing at the 4th mktemp must still leave zero survivors."""
        proc, stubs = self.stub_sync(fail_from=4)
        self.assertEqual(proc.returncode, 1, proc.stderr)
        self.assertEqual(stubs.survivors(), [], "temporaries were stranded")

    def test_recipe_temps_are_not_stranded_when_the_first_capture_fails(self):
        """The genuine late-registration defect: cleanup after the third temp.

        The failure lands exactly on the first capture file — the fourth temp of
        the recipe block — so the three already-created ``-t`` temps are the
        ones at risk if cleanup is registered too late.
        """
        _, control = self.stub_sync()
        boundary = self._recipe_temp_numbers(control)[3]
        proc, stubs = self.stub_sync(fail_from=boundary)
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
            intercepts=[
                ("brief-render-policy.py", {"rc": 0, "stdout": "true\n"}),
                ("agents-render.py", {"rc": 9, "stdout": "oops\n"}),
            ]
        )
        self.assertEqual(proc.returncode, 9, proc.stderr)

    def test_refresh_gates_is_accepted_and_a_fresh_sync_is_unchanged(self):
        """The flag no longer reaches an argv (the step is in-process).

        What stays observable at this boundary is acceptance: a fresh project
        syncs identically with and without it. The forwarding of the flag into
        the materializer request is pinned by the per-recipe loop differential
        (internal/sync/materialize_loop_test.go, --refresh-gates scenario).
        """
        base = Path(tempfile.mkdtemp(prefix="ai-specs-recipe-"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        pair = []
        for args in ((), ("--refresh-gates",)):
            local = Path(tempfile.mkdtemp(prefix="ai-specs-recipe-"))
            self.addCleanup(shutil.rmtree, local, ignore_errors=True)
            stubs = _sync_stub.SyncStubs(local)
            project = _sync_stub.minimal_project(local)
            proc = subprocess.run(
                [str(CLI), "sync", str(project), *args],
                env=stubs.env(),
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            pair.append(self._normalize_paths(proc.stdout, local))
        self.assertEqual(
            pair[0], pair[1], "a fresh project must sync identically with and without the flag"
        )

    def test_five_recipe_temps_are_distinct(self):
        proc, stubs = self.stub_sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        numbers = self._recipe_temp_numbers(stubs)
        self.assertGreaterEqual(stubs.created_count(), numbers[-1])
        paths = [str(stubs.probe / f"temp{n}") for n in numbers]
        self.assertTrue(all(paths), f"empty recipe temp path: {paths}")
        self.assertEqual(
            len(set(paths)), 5, f"the recipe temps collided: {paths}"
        )


if __name__ == "__main__":
    unittest.main()
