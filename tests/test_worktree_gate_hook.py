"""Integration tests for the worktree-flow worktree-gate.sh runtime hook.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open.

Covers path-mode (Edit/Write) regression and shell-mode write-bypass heuristics
(Bash/Shell + Cursor native top-level command payloads).
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
        # Default branch name varies by git version; normalize after first commit.
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        # Ensure a predictable "main" exists for protected-branch tests.
        try:
            _git(self.repo, "checkout", "-q", "-B", "main")
        except subprocess.CalledProcessError:
            pass

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

    def _shell_event(self, command: str, tool: str = "Bash", cwd: str | None = None) -> dict:
        return {
            "event": "pre-tool-use",
            "tool_name": tool,
            "tool_input": {"command": command},
            "cwd": cwd or str(self.repo),
        }

    def _cursor_shell_event(self, command: str, cwd: str | None = None) -> dict:
        # Cursor beforeShellExecution native payload: top-level command + cwd.
        return {
            "command": command,
            "cwd": cwd or str(self.repo),
        }

    def _src(self, name: str = "src.py") -> str:
        return str(self.repo / name)

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

    # --- Shell dual-input foundation ---

    def test_shell_missing_command_fail_open(self):
        self._checkout("main")
        r = self._run({
            "event": "pre-tool-use",
            "tool_name": "Bash",
            "tool_input": {},
            "cwd": str(self.repo),
        })
        self.assertEqual(r.returncode, 0)

    def test_cursor_native_payload_command_extracted(self):
        """Cursor top-level {command,cwd} must be recognized (block when write)."""
        self._checkout("main")
        src = self._src()
        r = self._run(self._cursor_shell_event(f"echo x > {src}"))
        self.assertEqual(r.returncode, 2)
        self.assertIn("worktree-gate", r.stderr)

    # --- Pass 1 heuristics ---

    def test_shell_redirect_gt_blocks_protected_main(self):
        self._checkout("main")
        src = self._src()
        r = self._run(self._shell_event(f"echo x > {src}"))
        self.assertEqual(r.returncode, 2)
        self.assertIn("worktree-gate", r.stderr)

    def test_shell_redirect_append_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("notes.md")
        r = self._run(self._shell_event(f"echo x >> {src}"))
        self.assertEqual(r.returncode, 2)

    def test_shell_tee_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("out.log")
        r = self._run(self._shell_event(f"echo x | tee {src}"))
        self.assertEqual(r.returncode, 2)
        r2 = self._run(self._shell_event(f"echo x | tee -a {src}"))
        self.assertEqual(r2.returncode, 2)

    def test_shell_sed_i_blocks_protected_main(self):
        self._checkout("main")
        # relative path resolved via event cwd
        (self.repo / "cfg.yaml").write_text("a\n")
        r = self._run(self._shell_event("sed -i 's/a/b/' cfg.yaml"))
        self.assertEqual(r.returncode, 2)

    def test_shell_perl_i_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("Makefile")
        r = self._run(self._shell_event(f"perl -i -pe 's/a/b/' {src}"))
        self.assertEqual(r.returncode, 2)

    def test_shell_cp_dest_blocks_protected_main(self):
        self._checkout("main")
        src = self._src()
        r = self._run(self._shell_event(f"cp /tmp/src.py {src}"))
        self.assertEqual(r.returncode, 2)

    def test_shell_mv_dest_blocks_protected_main(self):
        self._checkout("main")
        src = self._src()
        r = self._run(self._shell_event(f"mv /tmp/src.py {src}"))
        self.assertEqual(r.returncode, 2)

    # --- Pass 2 interpreter body ---

    def test_shell_python_c_open_w_blocks_protected_main(self):
        self._checkout("main")
        src = self._src()
        r = self._run(self._shell_event(f"python3 -c \"open('{src}','w').write('x')\""))
        self.assertEqual(r.returncode, 2)

    def test_shell_python_heredoc_write_text_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("gen.py")
        cmd = f"python3 <<'PY'\nfrom pathlib import Path\nPath('{src}').write_text('x')\nPY"
        r = self._run(self._shell_event(cmd))
        self.assertEqual(r.returncode, 2)

    def test_shell_node_writeFileSync_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("dist.js")
        r = self._run(self._shell_event(
            f"node -e \"require('fs').writeFileSync('{src}','x')\""
        ))
        self.assertEqual(r.returncode, 2)

    def test_shell_ruby_file_write_blocks_protected_main(self):
        self._checkout("main")
        src = self._src("x.txt")
        r = self._run(self._shell_event(f"ruby -e \"File.write('{src}','s')\""))
        self.assertEqual(r.returncode, 2)

    # --- Fail-open / true negatives ---

    def test_shell_non_write_git_status_allowed(self):
        self._checkout("main")
        r = self._run(self._shell_event("git status --porcelain"))
        self.assertEqual(r.returncode, 0)

    def test_shell_non_write_ls_allowed(self):
        self._checkout("main")
        r = self._run(self._shell_event("ls -la"))
        self.assertEqual(r.returncode, 0)

    def test_shell_non_write_cat_allowed(self):
        self._checkout("main")
        r = self._run(self._shell_event(f"cat {self._src('README.md')}"))
        self.assertEqual(r.returncode, 0)

    def test_shell_quoted_false_redirect_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event("echo 'a > b'"))
        self.assertEqual(r.returncode, 0)

    def test_shell_redirect_dev_null_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event("echo x > /dev/null"))
        self.assertEqual(r.returncode, 0)

    def test_shell_fd_dup_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event("echo hi 2>&1"))
        self.assertEqual(r.returncode, 0)

    def test_shell_ambiguous_python_variable_path_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event("python3 -c \"open(dst,'w')\""))
        self.assertEqual(r.returncode, 0)

    def test_shell_write_outside_repo_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event("echo x > /tmp/out.txt"))
        self.assertEqual(r.returncode, 0)

    def test_shell_write_inside_linked_worktree_allowed(self):
        self._checkout("main")
        wt = Path(self.tmp.name) / "wt"
        _git(self.repo, "worktree", "add", "-q", "-b", "feat-shell", str(wt))
        target = wt / "x.py"
        r = self._run(self._shell_event(f"echo x > {target}"))
        self.assertEqual(r.returncode, 0)

    def test_shell_unbalanced_quote_non_write_fail_open(self):
        self._checkout("main")
        r = self._run(self._shell_event('echo "unterminated'))
        self.assertEqual(r.returncode, 0)

    def test_shell_read_only_heredoc_allowed(self):
        self._checkout("main")
        cmd = "python3 <<'PY'\nprint(open('README.md').read())\nPY"
        r = self._run(self._shell_event(cmd))
        self.assertEqual(r.returncode, 0)

    def test_cursor_shell_redirect_blocks_protected_main(self):
        self._checkout("main")
        r = self._run(self._cursor_shell_event(f"echo x > {self._src()}"))
        self.assertEqual(r.returncode, 2)

    # --- Message + gate_mode parity ---

    def test_shell_block_message_names_bash_bypass_and_worktree_new(self):
        self._checkout("main")
        r = self._run(self._shell_event(f"echo x > {self._src()}"))
        self.assertEqual(r.returncode, 2)
        self.assertIn("bash/shell", r.stderr)
        self.assertIn("/worktree-new", r.stderr)

    def test_shell_gate_ask_includes_bypass_hint(self):
        self._checkout("main")
        gate = self._stamped_gate("ask")
        r = self._run(self._shell_event(f"echo x > {self._src()}"), gate=gate)
        self.assertEqual(r.returncode, 2)
        self.assertIn("WORKTREE_GATE_MODE=off", r.stderr)

    def test_shell_gate_off_disables_shell_gating(self):
        self._checkout("main")
        gate = self._stamped_gate("off")
        r = self._run(self._shell_event(f"echo x > {self._src()}"), gate=gate)
        self.assertEqual(r.returncode, 0)

    def test_shell_write_on_feature_branch_allowed(self):
        self._checkout("feature-shell")
        r = self._run(self._shell_event(f"echo x > {self._src()}"))
        self.assertEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
