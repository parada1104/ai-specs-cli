"""Integration tests for the worktree-flow worktree-gate.sh runtime hook.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open.

Covers path-mode (Edit/Write) regression and shell-mode write-bypass heuristics
(Bash/Shell + Cursor native top-level command payloads).
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
GO_BINARY = ROOT / "dist" / "worktree-gate-current"


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


class _GoGate:
    """Go-side stand-in for a stamped Bash gate.

    The Bash stamp substitution (__WORKTREE_GATE_MODE__ etc.) maps 1:1 to the
    binary's --gate-mode / --gate-scope / --repo-topology flags, so a stamped
    gate materialized for the Bash implementation is represented for the Go
    implementation by this value object (task 2.17).
    """

    def __init__(self, mode: str = "always", scope: str = "auto",
                 topology: str = "auto") -> None:
        self.mode = mode
        self.scope = scope
        self.topology = topology


class WorktreeGateHookTests(unittest.TestCase):
    """Integration suite for the worktree gate, parameterized over impl.

    impl="bash" runs the frozen reference script (the pre-change behavior);
    impl="go" runs the same scenarios against the Go binary through the
    launcher-equivalent flags, so a behavioral differential (mode/scope
    precedence, URI allowlist, extraction, messages) fails here first
    (task 2.17). The Go half skips loudly when no binary is built yet.
    """

    impl = "bash"

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

    def _stamped_gate(self, mode: str):
        """Materialize a gate stamped with the given mode for this impl.

        The Bash path only replaces the mode sentinel; scope and topology stay
        as the literal __WORKTREE_GATE_SCOPE__ / __WORKTREE_REPO_TOPOLOGY__
        (invalid), which emits the fallback warnings. The Go path mirrors that
        by passing the same invalid sentinels so the warning contract is
        identical across impls.
        """
        if self.impl == "go":
            return _GoGate(mode=mode,
                           scope="__WORKTREE_GATE_SCOPE__",
                           topology="__WORKTREE_REPO_TOPOLOGY__")
        stamped = Path(self.tmp.name) / f"worktree-gate-{mode}.sh"
        stamped.write_text(GATE.read_text().replace("__WORKTREE_GATE_MODE__", mode))
        stamped.chmod(0o755)
        return stamped

    def _gate_command(self, gate=None) -> list[str]:
        """Build the invocation for the active implementation.

        Bash: ["bash", path] with stamped env. Go: the binary with explicit
        flags carrying the stamped values (the Phase 3 launcher performs the
        same mapping). A plain _GoGate or None means the effective defaults
        (always / auto / auto).
        """
        if self.impl == "go":
            if gate is None:
                gate = _GoGate()
            return [
                str(GO_BINARY),
                "--gate-mode", gate.mode,
                "--gate-scope", gate.scope,
                "--repo-topology", gate.topology,
                "--protected", "main development",
            ]
        return ["bash", str(gate or GATE)]

    def _run(
        self,
        event: dict,
        *,
        protected: str = "main development",
        gate=None,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = dict(os.environ, WORKTREE_GATE_PROTECTED=protected)
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            self._gate_command(gate),
            input=json.dumps(event),
            capture_output=True, text=True, env=env,
        )
    def _run_in(
        self,
        event: dict,
        run_cwd: Path,
        *,
        event_cwd: Path | None = None,
        protected: str = "main development",
    ) -> subprocess.CompletedProcess:
        """Run the hook with a controlled process cwd (for fallback tests)."""
        if event_cwd is not None:
            event["cwd"] = str(event_cwd)
        env = dict(os.environ, WORKTREE_GATE_PROTECTED=protected)
        return subprocess.run(
            self._gate_command(None),
            input=json.dumps(event),
            capture_output=True, text=True, env=env, cwd=str(run_cwd),
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
        r = subprocess.run(self._gate_command(None), input="not json",
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
            self._gate_command(gate),
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


    def test_scope_override_invalid_falls_back_to_stamp(self):
        superrepo, subrepo = self._make_superrepo_fixture()
        gate = self._scope_gate("superrepo")
        target = subrepo / "production.py"
        r = self._run(self._path_event(target, subrepo), gate=gate,
                      extra_env={"WORKTREE_GATE_SCOPE": "repository"})
        self.assertEqual(r.returncode, 0)
        self.assertIn("invalid WORKTREE_GATE_SCOPE", r.stderr)

    def test_missing_scope_stamp_warns_and_falls_back_to_auto(self):
        self._checkout("main")
        gate = self._stamped_gate("always")
        r = self._run(self._event("Write", self._src()), gate=gate)
        self.assertEqual(r.returncode, 2)
        self.assertIn("scope", r.stderr.lower())
    def _make_superrepo_fixture(self):
        module = Path(self.tmp.name) / "module-source"
        module.mkdir()
        _git(module, "init", "-q")
        _git(module, "config", "user.email", "t@t.t")
        _git(module, "config", "user.name", "t")
        (module / "README.md").write_text("module\n")
        _git(module, "add", "-A")
        _git(module, "commit", "-qm", "init")
        _git(module, "checkout", "-q", "-B", "main")
        superrepo = Path(self.tmp.name) / "superrepo"
        superrepo.mkdir()
        _git(superrepo, "init", "-q")
        _git(superrepo, "config", "user.email", "t@t.t")
        _git(superrepo, "config", "user.name", "t")
        (superrepo / "ROOT").write_text("super\n")
        _git(superrepo, "add", "-A")
        _git(superrepo, "commit", "-qm", "root")
        subprocess.run(
            ["git", "-C", str(superrepo), "-c", "protocol.file.allow=always", "submodule", "add", "-q", str(module), "apps/api"],
            check=True, capture_output=True, text=True,
        )
        _git(superrepo, "commit", "-qam", "add module")
        _git(superrepo, "checkout", "-q", "-B", "main")
        return superrepo, superrepo / "apps" / "api"

    def test_proven_superrepo_central_path_allowed_and_production_blocked(self):
        superrepo, subrepo = self._make_superrepo_fixture()
        central = superrepo / "openspec" / "changes" / "demo" / "tasks.md"
        event = self._event("Write", str(central))
        event["cwd"] = str(superrepo)
        result = self._run(event)
        self.assertEqual(result.returncode, 0)
        production = superrepo / "src" / "generated.py"
        event = self._event("Write", str(production))
        event["cwd"] = str(superrepo)
        self.assertEqual(self._run(event).returncode, 2)
        sub_event = self._event("Write", str(subrepo / "src.py"))
        sub_event["cwd"] = str(subrepo)
        self.assertEqual(self._run(sub_event).returncode, 2)
    def _scope_gate(self, scope: str, topology: str = "monorepo-submodules"):
        """Materialize a gate stamped with scope/topology for this impl."""
        if self.impl == "go":
            return _GoGate(scope=scope, topology=topology)
        stamped = Path(self.tmp.name) / f"worktree-gate-{scope}-{topology}.sh"
        content = GATE.read_text()
        content = content.replace("__WORKTREE_GATE_MODE__", "always")
        content = content.replace("__WORKTREE_GATE_SCOPE__", scope)
        content = content.replace("__WORKTREE_REPO_TOPOLOGY__", topology)
        stamped.write_text(content)
        stamped.chmod(0o755)
        return stamped

    def _path_event(self, target: Path, cwd: Path) -> dict:
        event = self._event("Write", str(target))
        event["cwd"] = str(cwd)
        return event

    def test_scope_matrix_all_values_preserves_central_and_production_floor(self):
        superrepo, subrepo = self._make_superrepo_fixture()
        central = superrepo / "openspec" / "changes" / "matrix" / "tasks.md"
        super_production = superrepo / "src" / "generated.py"
        sub_production = subrepo / "src.py"
        for scope in ("auto", "superrepo", "subrepo"):
            gate = self._scope_gate(scope)
            self.assertEqual(self._run(self._path_event(central, superrepo), gate=gate).returncode, 0)
            self.assertEqual(self._run(self._path_event(super_production, superrepo), gate=gate).returncode, 2 if scope != "subrepo" else 0)
            self.assertEqual(self._run(self._path_event(sub_production, subrepo), gate=gate).returncode, 2 if scope != "superrepo" else 0)

    def test_uninitialized_module_does_not_prove_central_scope(self):
        superrepo, subrepo = self._make_superrepo_fixture()
        _git(superrepo, "submodule", "deinit", "-f", "--", "apps/api")
        if subrepo.exists():
            shutil.rmtree(subrepo)
        target = superrepo / "openspec" / "changes" / "uninitialized" / "tasks.md"
        self.assertEqual(self._run(self._path_event(target, superrepo), gate=self._scope_gate("auto")).returncode, 2)

    def test_symlink_escape_does_not_receive_central_exception(self):
        superrepo, _ = self._make_superrepo_fixture()
        outside = Path(self.tmp.name) / "outside"
        outside.mkdir()
        _git(outside, "init", "-q")
        _git(outside, "config", "user.email", "t@t.t")
        _git(outside, "config", "user.name", "t")
        (outside / "README").write_text("outside\n")
        _git(outside, "add", "-A")
        _git(outside, "commit", "-qm", "init")
        _git(outside, "checkout", "-q", "-B", "main")
        central_root = superrepo / "openspec" / "changes"
        central_root.mkdir(parents=True)
        (central_root / "escape").symlink_to(outside, target_is_directory=True)
        target = central_root / "escape" / "tasks.md"
        self.assertEqual(self._run(self._path_event(target, superrepo), gate=self._scope_gate("auto")).returncode, 2)

    def test_ambiguous_and_nested_registrations_fail_safe(self):
        superrepo, _ = self._make_superrepo_fixture()
        gm = superrepo / ".gitmodules"
        gm.write_text(gm.read_text() + "\n[submodule.duplicate]\n\tpath = apps/api\n[submodule.nested]\n\tpath = apps/api/nested\n")
        central = superrepo / "openspec" / "changes" / "ambiguous" / "tasks.md"
        self.assertEqual(self._run(self._path_event(central, superrepo), gate=self._scope_gate("superrepo")).returncode, 2)

    def test_nested_registration_fails_safe_even_when_outer_is_initialized(self):
        superrepo, _ = self._make_superrepo_fixture()
        gm = superrepo / ".gitmodules"
        gm.write_text(gm.read_text() + "\n[submodule.nested]\n\tpath = apps/api/nested\n")
        central = superrepo / "openspec" / "changes" / "nested" / "tasks.md"
        self.assertEqual(self._run(self._path_event(central, superrepo), gate=self._scope_gate("auto")).returncode, 2)
    def test_relative_git_common_fallback_anchors_to_owner_root(self):
        superrepo, _ = self._make_superrepo_fixture()
        real_git = shutil.which("git")
        self.assertIsNotNone(real_git)
        fake_bin = Path(self.tmp.name) / "fake-bin"
        fake_bin.mkdir()
        fake_git = fake_bin / "git"
        fake_git.write_text(
            "#!/bin/sh\n"
            "if [ \"$1\" = \"-C\" ] && [ \"$3\" = \"rev-parse\" ] && [ \"$4\" = \"--path-format=absolute\" ]; then\n"
            f"  root=\"$2\"; shift 4; exec {real_git} -C \"$root\" rev-parse \"$@\"\n"
            "fi\n"
            f"exec {real_git} \"$@\"\n"
        )
        fake_git.chmod(0o755)
        other_cwd = Path(self.tmp.name) / "unrelated-cwd"
        other_cwd.mkdir()
        target = superrepo / "openspec" / "changes" / "legacy-git" / "tasks.md"
        result = self._run(
            self._path_event(target, other_cwd),
            gate=self._scope_gate("auto"),
            extra_env={"PATH": str(fake_bin) + os.pathsep + os.environ["PATH"]},
        )
        self.assertEqual(result.returncode, 0)
    def test_explicit_vendored_topology_disables_scope_classification(self):
        superrepo, _ = self._make_superrepo_fixture()
        target = superrepo / "openspec" / "changes" / "vendored" / "tasks.md"
        for topology in ("standalone", "monorepo-apps"):
            gate = self._scope_gate("auto", topology)
            self.assertEqual(self._run(self._path_event(target, superrepo), gate=gate).returncode, 2)

    def test_valid_scope_override_selects_subrepo_enforcement(self):
        superrepo, _ = self._make_superrepo_fixture()
        target = superrepo / "src" / "generated.py"
        gate = self._scope_gate("superrepo")
        result = self._run(self._path_event(target, superrepo), gate=gate,
                           extra_env={"WORKTREE_GATE_SCOPE": "subrepo"})
        self.assertEqual(result.returncode, 0)

    def test_invalid_scope_override_uses_actually_stamped_superrepo(self):
        superrepo, subrepo = self._make_superrepo_fixture()
        gate = self._scope_gate("superrepo")
        target = subrepo / "production.py"
        result = self._run(self._path_event(target, subrepo), gate=gate,
                           extra_env={"WORKTREE_GATE_SCOPE": "repository"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("invalid WORKTREE_GATE_SCOPE", result.stderr)

    def test_shell_central_and_noncentral_share_scope_decision(self):
        superrepo, _ = self._make_superrepo_fixture()
        central = superrepo / "openspec" / "changes" / "shell" / "tasks.md"
        noncentral = superrepo / "src" / "generated.py"
        gate = self._scope_gate("auto")
        self.assertEqual(self._run(self._shell_event(f"echo x > {central}", cwd=str(superrepo)), gate=gate).returncode, 0)
        self.assertEqual(self._run(self._shell_event(f"echo x > {noncentral}", cwd=str(superrepo)), gate=gate).returncode, 2)

    # --- Internal URI allowlist ---

    def test_uri_xd_resolve_allowed_on_protected_branch(self):
        self._checkout("development")
        r = self._run(self._event("Write", "xd://resolve"))
        self.assertEqual(r.returncode, 0)

    def test_uri_artifact_allowed_on_protected_branch(self):
        self._checkout("development")
        r = self._run(self._event("Write", "artifact://abc123"))
        self.assertEqual(r.returncode, 0)

    def test_uri_local_allowed_on_protected_branch(self):
        self._checkout("main")
        r = self._run(self._event("Write", "local://plan.md"))
        self.assertEqual(r.returncode, 0)

    def test_uri_vault_allowed_on_protected_branch(self):
        self._checkout("development")
        r = self._run(self._event("Write", "vault://hermes-vault/doc.md"))
        self.assertEqual(r.returncode, 0)

    def test_uri_skill_allowed_on_protected_branch(self):
        self._checkout("main")
        r = self._run(self._event("Write", "skill://testing/init"))
        self.assertEqual(r.returncode, 0)

    def test_uri_representative_schemes_allowed_on_protected_branch(self):
        self._checkout("main")
        for uri in (
            "rule://worktree-gate",
            "agent://abc123",
            "history://abc123",
            "mcp://trello/get_health",
            "issue://42",
            "pr://42",
            "omp://",
        ):
            r = self._run(self._event("Write", uri))
            self.assertEqual(r.returncode, 0, uri)

    def test_uri_shell_mode_literal_target_stays_gated(self):
        # A URI-looking token in a shell command is a literal write target
        # (redirection/argument), not a tool interface: it must be classified.
        self._checkout("main")
        event = self._shell_event(f"echo x > xd://{self.repo}/src.py")
        r = self._run(event)
        self.assertEqual(r.returncode, 2)
    def test_uri_shell_mode_bare_uri_literal_stays_gated(self):
        # Discriminator: the URI allowlist applies in PATH mode only. A bare
        # URI-looking token (no absolute path after the scheme) in a shell
        # command is a literal write target and must be classified — dropping
        # the mode guard would allow it through the allowlist bypass.
        self._checkout("main")
        event = self._shell_event("echo x > xd://out.txt")
        r = self._run(event)
        self.assertEqual(r.returncode, 2)

    def test_uri_traversal_masked_path_stays_gated(self):
        # A known scheme with ../ traversal resolves into the repository and
        # must be classified, not bypassed as a genuine internal URI.
        self._checkout("main")
        r = self._run(self._event("Write", f"xd://{self.repo.name}/../{self.repo.name}/src/app.py"))
        self.assertEqual(r.returncode, 2)

    def test_uri_absolute_path_after_scheme_stays_gated(self):
        # A known scheme followed by an absolute filesystem path must be
        # classified like the raw absolute path it masks.
        self._checkout("main")
        r = self._run(self._event("Write", f"xd://{self.repo}/src/app.py"))
        self.assertEqual(r.returncode, 2)

    # --- Unknown URI schemes stay gated ---

    def test_unknown_uri_https_stays_gated(self):
        self._checkout("main")
        r = self._run(self._event("Write", "https://example.com/src.py"))
        self.assertEqual(r.returncode, 2)

    def test_unknown_uri_file_scheme_stays_gated(self):
        self._checkout("main")
        r = self._run(self._event("Write", "file:///etc/hosts"))
        self.assertEqual(r.returncode, 2)

    def test_unknown_uri_custom_stays_gated(self):
        self._checkout("main")
        r = self._run(self._event("Write", "custom://thing"))
        self.assertEqual(r.returncode, 2)

    # --- Event cwd precedence ---

    def test_relative_path_uses_event_cwd_not_process_pwd(self):
        # Process PWD is inside the repo; the event cwd is external; the
        # relative candidate resolves outside the repository and must be allowed.
        self._checkout("main")
        external = Path(self.tmp.name) / "external"
        external.mkdir()
        r = self._run_in(self._event("Write", "out.txt"), run_cwd=self.repo,
                         event_cwd=external)
        self.assertEqual(r.returncode, 0)

    def test_relative_path_event_cwd_inside_protected_repo_blocks(self):
        self._checkout("main")
        r = self._run_in(self._event("Write", "src/app.py"), run_cwd=Path(self.tmp.name),
                         event_cwd=self.repo)
        self.assertEqual(r.returncode, 2)

    def test_missing_event_cwd_falls_back_to_process_pwd(self):
        self._checkout("main")
        event = self._event("Write", "src/app.py")
        event.pop("cwd")
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)

    def test_absolute_candidate_unchanged_by_event_cwd(self):
        self._checkout("main")
        external = Path(self.tmp.name) / "external"
        external.mkdir()
        r = self._run_in(self._event("Write", str(self.repo / "src.py")), run_cwd=self.repo,
                         event_cwd=external)
        self.assertEqual(r.returncode, 2)

    def test_relative_shell_write_uses_event_cwd(self):
        self._checkout("main")
        external = Path(self.tmp.name) / "external"
        external.mkdir()
        event = self._shell_event("echo x > out.log")
        r = self._run_in(event, run_cwd=self.repo, event_cwd=external)
        self.assertEqual(r.returncode, 0)

    def test_relative_shell_write_falls_back_to_process_pwd(self):
        self._checkout("main")
        event = self._shell_event("echo x > out.log")
        event.pop("cwd")
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)

    def test_relative_event_cwd_falls_back_to_process_pwd(self):
        # A relative event cwd is unusable: resolution falls back to the
        # process PWD (the protected repo), so a relative write blocks.
        self._checkout("main")
        event = self._event("Write", "src/app.py")
        event["cwd"] = "relative/dir"
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)
    def test_relative_event_cwd_traversal_falls_back_to_process_pwd(self):
        # Discriminator: a relative event cwd is unusable even when it
        # resolves (via ..) to an existing directory beside the process PWD.
        # Resolution must fall back to the process PWD so the relative write
        # still blocks; a mutant that accepts relative cwds resolved against
        # the process PWD would resolve outside the repo and allow.
        sibling = Path(self.tmp.name) / "outside"
        sibling.mkdir(exist_ok=True)
        self._checkout("main")
        event = self._event("Write", "src/app.py")
        event["cwd"] = "../outside"
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)

    def test_nonexistent_event_cwd_falls_back_to_process_pwd(self):
        # A nonexistent event cwd is unusable: resolution falls back to the
        # process PWD (the protected repo), so a relative write blocks.
        self._checkout("main")
        event = self._event("Write", "src/app.py")
        event["cwd"] = str(Path(self.tmp.name) / "does-not-exist")
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)

    def test_relative_shell_write_falls_back_on_nonexistent_event_cwd(self):
        self._checkout("main")
        event = self._shell_event("echo x > out.log")
        event["cwd"] = str(Path(self.tmp.name) / "does-not-exist")
        r = self._run_in(event, run_cwd=self.repo)
        self.assertEqual(r.returncode, 2)
if __name__ == "__main__":
    unittest.main()


class WorktreeGateGoHookTests(WorktreeGateHookTests):
    """The same 78-scenario suite run against the Go binary.

    Activates exactly when dist/worktree-gate-current exists (built by
    scripts/build-gate.sh); skips loudly otherwise so a machine without Go
    keeps the Bash half green (task 1.17 / 2.17).
    """

    impl = "go"

    def setUp(self):
        if not GO_BINARY.exists():
            self.skipTest(
                "no Go gate binary in dist/ (run scripts/build-gate.sh); "
                "Bash parameterization still ran")
        super().setUp()
