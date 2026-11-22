"""Release checks for the CLI worktree gate."""
from __future__ import annotations

import re
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class WorktreeGateReleaseTests(unittest.TestCase):
    def test_release_workflow_pins_canonical_toolchain_without_broken_cache(self):
        workflow = (ROOT / ".github" / "workflows" / "release-binaries.yml").read_text(
            encoding="utf-8")
        self.assertIn("go-version:", workflow)
        match = re.search(r'go-version:\s*"([0-9]+\.[0-9]+\.[0-9]+)"', workflow)
        self.assertIsNotNone(
            match, "release workflow must pin an EXACT Go version (no .x), "
                  "because the CLI trust root is toolchain-specific")
        self.assertIsNone(
            re.search(r"^\s*cache-dependency-path:", workflow, re.MULTILINE),
            "release workflow must not key setup-go's cache on a go.sum: the "
            "module has zero third-party deps and no go.sum exists")

    def test_local_toolchain_matches_pinned_release_toolchain(self):
        workflow = (ROOT / ".github" / "workflows" / "release-binaries.yml").read_text(
            encoding="utf-8")
        match = re.search(r'go-version:\s*"([0-9]+\.[0-9]+\.[0-9]+)"', workflow)
        if match is None:
            self.fail("workflow must pin an exact Go version")
        proc = subprocess.run(["go", "version"], capture_output=True, text=True,
                              timeout=30)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        local = re.search(r"go([0-9]+\.[0-9]+\.[0-9]+)", proc.stdout)
        if local is None:
            self.fail(f"cannot parse local go version: {proc.stdout!r}")
        self.assertEqual(
            local.group(1), match.group(1),
            "local Go toolchain must match the release workflow pin")

    def test_release_workflow_runs_gate_parity_before_publishing(self):
        workflow = (ROOT / ".github" / "workflows" / "release-binaries.yml").read_text(
            encoding="utf-8")
        self.assertIn("worktree-gate-parity:", workflow)
        self.assertIn("python3 -m unittest tests.test_worktree_gate_parity tests.test_worktree_gate_hook -q", workflow)
        self.assertRegex(workflow, r"needs: \[build-cli, worktree-gate-parity\]")

    def test_release_workflow_vets_and_tests_nested_gate_module(self):
        workflow = (ROOT / ".github" / "workflows" / "release-binaries.yml").read_text(
            encoding="utf-8")
        gate_job = workflow.split("\n  worktree-gate-parity:\n", 1)[1].split("\n  checksum:\n", 1)[0]
        for command in ("vet", "test"):
            with self.subTest(command=command):
                self.assertIn(f"run: go -C catalog/recipes/worktree-flow/gate {command} ./...", gate_job)


if __name__ == "__main__":
    unittest.main()
