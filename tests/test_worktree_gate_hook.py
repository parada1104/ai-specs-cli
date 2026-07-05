"""Integration tests for the worktree-flow worktree-gate.sh runtime hook.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open.
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


class WorktreeGateHookTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name) / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")

    def _stamped_gate(self, mode: str) -> Path:
        stamped = Path(self.tmp.name) / f"worktree-gate-{mode}.sh"
        stamped.write_text(GATE.read_text().replace("__WORKTREE_GATE_MODE__", mode))
        stamped.chmod(0o755)
        return stamped

    def _run(
        self,
        event: dict,
        *,
        protected: str = "main development",
        gate: Path | None = None,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = dict(os.environ, WORKTREE_GATE_PROTECTED=protected)
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            ["bash", str(gate or GATE)],
            input=json.dumps(event),
            capture_output=True, text=True, env=env,
        )

    def _checkout(self, branch: str) -> None:
        _git(self.repo, "checkout", "-q", "-B", branch)

    def _event(self, tool: str, file_path: str) -> dict:
        return {
            "event": "pre-tool-use",
            "tool_name": tool,
            "tool_input": {"file_path": file_path},
            "cwd": str(self.repo),
        }

    # 1. Write on a protected branch in the main worktree → block (exit 2).
    def test_block_write_on_protected_branch_main_worktree(self):
        self._checkout("main")
        r = self._run(self._event("Write", str(self.repo / "src.py")))
        self.assertEqual(r.returncode, 2)
        self.assertIn("worktree-gate", r.stderr)

    # 2. Write on a non-protected branch → allow (exit 0).
    def test_allow_write_on_feature_branch(self):
        self._checkout("feature-x")
        r = self._run(self._event("Write", str(self.repo / "src.py")))
        self.assertEqual(r.returncode, 0)

    # 3. Custom protected list honored (development blocked).
    def test_custom_protected_branch_blocks(self):
        self._checkout("development")
        r = self._run(self._event("Edit", str(self.repo / "a.txt")))
        self.assertEqual(r.returncode, 2)

    # 4. .claude/settings.json is always allowed (local machine config).
    def test_allow_claude_settings_on_protected_branch(self):
        self._checkout("main")
        target = self.repo / ".claude" / "settings.json"
        r = self._run(self._event("Write", str(target)))
        self.assertEqual(r.returncode, 0)

    # 5. Empty / missing file_path → fail-open allow.
    def test_missing_file_path_fail_open(self):
        self._checkout("main")
        r = self._run({"event": "pre-tool-use", "tool_name": "Write", "tool_input": {}})
        self.assertEqual(r.returncode, 0)

    # 6. Malformed JSON on stdin → fail-open allow.
    def test_malformed_stdin_fail_open(self):
        env = dict(os.environ, WORKTREE_GATE_PROTECTED="main")
        r = subprocess.run(["bash", str(GATE)], input="not json",
                           capture_output=True, text=True, env=env)
        self.assertEqual(r.returncode, 0)

    # 7. Edit inside a linked worktree under a protected branch → allow.
    def test_allow_edit_inside_linked_worktree(self):
        self._checkout("main")
        wt = Path(self.tmp.name) / "wt"
        _git(self.repo, "worktree", "add", "-q", "-b", "feat", str(wt))
        r = self._run(self._event("Write", str(wt / "x.py")))
        self.assertEqual(r.returncode, 0)

    def test_gate_always_blocks_protected(self):
        self._checkout("development")
        gate = self._stamped_gate("always")
        r = self._run(self._event("Edit", str(self.repo / "a.txt")), gate=gate)
        self.assertEqual(r.returncode, 2)
        self.assertIn("development", r.stderr)

    def test_gate_off_self_disables(self):
        self._checkout("main")
        gate = self._stamped_gate("off")
        r = self._run(self._event("Write", str(self.repo / "src.py")), gate=gate)
        self.assertEqual(r.returncode, 0)

    def test_gate_ask_blocks_with_bypass_hint(self):
        self._checkout("development")
        gate = self._stamped_gate("ask")
        r = self._run(self._event("Edit", str(self.repo / "a.txt")), gate=gate)
        self.assertEqual(r.returncode, 2)
        self.assertIn("WORKTREE_GATE_MODE=off", r.stderr)

    def test_env_override_beats_stamped(self):
        self._checkout("main")
        gate = self._stamped_gate("always")
        r = self._run(
            self._event("Write", str(self.repo / "src.py")),
            gate=gate,
            extra_env={"WORKTREE_GATE_MODE": "off"},
        )
        self.assertEqual(r.returncode, 0)

    def test_empty_env_keeps_stamped(self):
        self._checkout("development")
        gate = self._stamped_gate("ask")
        env = dict(os.environ, WORKTREE_GATE_PROTECTED="main development")
        env.pop("WORKTREE_GATE_MODE", None)
        r = subprocess.run(
            ["bash", str(gate)],
            input=json.dumps(self._event("Edit", str(self.repo / "a.txt"))),
            capture_output=True,
            text=True,
            env=env,
        )
        self.assertEqual(r.returncode, 2)
        self.assertIn("WORKTREE_GATE_MODE=off", r.stderr)

    def test_linked_worktree_always_allowed_in_always(self):
        self._checkout("main")
        wt = Path(self.tmp.name) / "wt"
        _git(self.repo, "worktree", "add", "-q", "-b", "feat", str(wt))
        gate = self._stamped_gate("always")
        r = self._run(self._event("Write", str(wt / "x.py")), gate=gate)
        self.assertEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
