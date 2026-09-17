"""Integration tests for trello-mcp-workflow tracker-card-gate.sh.

The host is a thin acquisition/JSON bridge to the verified Go `--ledger`
predicate (the only grader). These tests drive it with a stub ``worktree-gate``
binary (``WORKTREE_GATE_BIN``) that records its argv and returns a controlled
verdict, and assert:

- a production path write grades ``apply-start`` and ``gh pr create`` grades
  ``pr-review`` through the stub;
- exit 0 allow / exit 2 block, and fail-open on a missing binary or bad input;
- ``openspec/**`` and non-production writes never reach the ledger;
- the shell tokenizer detects only ``gh pr create`` (archive-close moved to the
  pre-merge guardian, so archive shell actions are not gated here);
- the script still parses and runs under bash 3.2.
"""
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "trello-mcp-workflow" / "hooks" / "tracker-card-gate.sh"
LIB_INTERNAL = ROOT / "lib" / "_internal"

STUB_BINARY = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> "${STUB_LOG}"
decision="${STUB_DECISION:-allow}"
reason="${STUB_REASON:-}"
checkpoint=""
wrote=0
while [ $# -gt 0 ]; do
  case "$1" in
    --checkpoint) checkpoint="$2"; shift 2 ;;
    --decide) printf 'DECIDE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    --write) printf 'WRITE %s\\n' "$2" >> "${STUB_LOG}"; wrote=1; shift 2 ;;
    --evidence) printf 'EVIDENCE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    *) shift ;;
  esac
done
if [ "$wrote" = 1 ] && [ "${STUB_WRITE_EXIT:-0}" = 2 ]; then
  exit 2
fi
prompt='null'
if [ "$decision" = ask ]; then
  prompt='{"reason":"needs-item","evidence":{"local":"","remote":"","code":"","git":""},"choices":["continue","local"]}'
fi
printf '{"capability":"tracker","active":true,"checkpoint":"%s","mode":"warn","decision":"%s","reason":"%s","identity":{"common_dir":"","branch":"","change":null,"key":""},"item":null,"conflict":null,"prompt":%s,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}\\n' "$checkpoint" "$decision" "$reason" "$prompt"
[ "$decision" = block ] && exit 2
exit 0
"""


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True)


class TrackerCardGateHookTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.repo = base / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        self.stub_log = base / "stub.log"
        self.stub = base / "worktree-gate"
        self.stub.write_text(STUB_BINARY)
        self.stub.chmod(0o755)

    def _stamped_gate(self, mode: str, cli_home: str = "",
                      lib_internal: str = "__TRACKER_LIB_INTERNAL__") -> Path:
        self.assertTrue(GATE.is_file(), f"gate script missing: {GATE}")
        stamped = Path(self.tmp.name) / f"tracker-card-gate-{mode}.sh"
        stamped.write_text(
            GATE.read_text()
            .replace("__TRACKER_CARD_GATE_MODE__", mode)
            .replace("__TRACKER_CLI_HOME__", cli_home)
            .replace("__TRACKER_LIB_INTERNAL__", lib_internal)
        )
        stamped.chmod(0o755)
        return stamped

    def _stamped_gate_bridged(self, mode: str = "warn") -> Path:
        """A gate whose __TRACKER_LIB_INTERNAL__ stamp resolves the real bridge."""
        return self._stamped_gate(mode, lib_internal=str(LIB_INTERNAL))

    def _change(self, slug: str = "demo-change", tracker_none: str | None = None) -> Path:
        folder = self.repo / "openspec" / "changes" / slug
        folder.mkdir(parents=True, exist_ok=True)
        (folder / "proposal.md").write_text(
            "## Tracker\n\n- **card_id**: `6aa703fdcf61a90ec702d58b`\n", encoding="utf-8"
        )
        if tracker_none is not None:
            (folder / "tracker.none").write_text(tracker_none, encoding="utf-8")
        return folder

    def _logged(self) -> list[str]:
        if not self.stub_log.exists():
            return []
        return self.stub_log.read_text().splitlines()

    def _grade_lines(self) -> list[str]:
        """Logged grade invocations: they carry --checkpoint but never --write."""
        return [l for l in self._logged() if "--checkpoint" in l and "--write" not in l]

    def _write_lines(self) -> list[str]:
        return [l for l in self._logged() if l.startswith("WRITE ")]

    def _env(self, *, decision: str = "allow", reason: str = "", **extra: str) -> dict:
        env = dict(os.environ)
        for key in ("TRACKER_CARD_GATE_MODE", "TRACKER_LEDGER_MODE",
                    "TRACKER_CARD_GATE_PATHS", "AI_SPECS_HOME"):
            env.pop(key, None)
        env["WORKTREE_GATE_BIN"] = str(self.stub)
        env["STUB_LOG"] = str(self.stub_log)
        env["STUB_DECISION"] = decision
        env["STUB_REASON"] = reason
        env.update(extra)
        return env

    def _run(
        self,
        event: dict | str,
        *,
        mode: str = "warn",
        decision: str = "allow",
        reason: str = "",
        gate: Path | None = None,
        env: dict | None = None,
    ) -> subprocess.CompletedProcess:
        payload = event if isinstance(event, str) else json.dumps(event)
        return subprocess.run(
            ["bash", str(gate or self._stamped_gate(mode))],
            input=payload,
            capture_output=True,
            text=True,
            # A new session has no controlling terminal, so the ask prompt's
            # /dev/tty open fails deterministically instead of blocking on a
            # developer's real terminal.
            start_new_session=True,
            env=env or self._env(decision=decision, reason=reason),
        )

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

    def _logged_checkpoints(self) -> list[str]:
        if not self.stub_log.exists():
            return []
        checkpoints = []
        for line in self.stub_log.read_text().splitlines():
            parts = line.split()
            for i, token in enumerate(parts):
                if token == "--checkpoint" and i + 1 < len(parts):
                    checkpoints.append(parts[i + 1])
        return checkpoints

    # --- path mode ---

    def test_prod_write_grades_apply_start(self):
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["apply-start"])

    def test_prod_write_blocks_on_block_verdict(self):
        r = self._run(
            self._event("Write", str(self.repo / "lib" / "foo.py")),
            decision="block",
            gate=self._stamped_gate("always"),
        )
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("apply-start", r.stderr)

    def test_warn_verdict_reports_on_stderr_without_blocking(self):
        r = self._run(
            self._event("Edit", str(self.repo / "catalog" / "x.toml")),
            decision="dormant",
            gate=self._stamped_gate("warn"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue(r.stderr.strip(), "expected the verdict on stderr")
        self.assertIn("apply-start", r.stderr)

    def test_ask_without_tty_blocks_without_fabricating_a_decision(self):
        r = self._run(
            self._event("Edit", str(self.repo / "lib" / "foo.py")),
            decision="ask",
            gate=self._stamped_gate("warn"),
        )
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("no terminal", r.stderr)
        self.assertIn("apply-start", r.stderr)
        self.assertNotIn("DECIDE", self.stub_log.read_text())

    def test_ask_identity_unavailable_reports_and_proceeds(self):
        r = self._run(
            self._event("Edit", str(self.repo / "lib" / "foo.py")),
            decision="ask",
            reason="identity_unavailable",
            gate=self._stamped_gate("warn"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("identity_unavailable", r.stderr)
        self.assertNotIn("DECIDE", self.stub_log.read_text())

    def test_openspec_paths_never_grade(self):
        target = self.repo / "openspec" / "changes" / "demo" / "proposal.md"
        r = self._run(self._event("Write", str(target)), decision="block")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_non_production_path_never_grades(self):
        r = self._run(self._event("Edit", str(self.repo / "tests" / "x.py")), decision="block")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_claude_settings_never_grade(self):
        r = self._run(
            self._event("Write", str(self.repo / ".claude" / "settings.json")),
            decision="block",
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_malformed_stdin_fail_open(self):
        r = self._run("not json", decision="block", gate=self._stamped_gate("always"))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_missing_file_path_fail_open(self):
        r = self._run(
            {"event": "pre-tool-use", "tool_name": "Write", "tool_input": {}, "cwd": str(self.repo)},
            decision="block",
            gate=self._stamped_gate("always"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_gate_mode_off_skips_the_ledger(self):
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      decision="block", gate=self._stamped_gate("off"))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_missing_binary_fail_open(self):
        env = self._env(decision="block")
        env.pop("WORKTREE_GATE_BIN", None)
        env["AI_SPECS_HOME"] = str(Path(self.tmp.name) / "cold-home")
        r = self._run(
            self._event("Edit", str(self.repo / "lib" / "foo.py")),
            gate=self._stamped_gate("always"),
            env=env,
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [])

    def test_no_bootstrap_marker_required(self):
        # The old marker seam is gone: activation is the durable witness, which
        # the binary itself reads. The host must still grade.
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["apply-start"])

    # --- shell mode ---

    def test_gh_pr_create_grades_pr_review(self):
        r = self._run(self._shell_event("gh pr create --fill"))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["pr-review"])

    def test_gh_pr_create_blocks_on_block_verdict(self):
        r = self._run(
            self._shell_event("gh pr create --title t --body b"),
            decision="block",
            gate=self._stamped_gate("always"),
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_cursor_native_shell_pr_create_grades(self):
        r = self._run(
            {"command": "gh pr create --fill", "cwd": str(self.repo)},
            decision="block",
            gate=self._stamped_gate("always"),
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_ambiguous_shell_commands_fail_open(self):
        for cmd in ("gh pr view 1", "git status", "ls lib"):
            with self.subTest(cmd=cmd):
                r = self._run(self._shell_event(cmd), decision="block")
                self.assertEqual(r.returncode, 0, f"{cmd}\n{r.stderr}")
                self.assertEqual(self._logged_checkpoints(), [])

    def test_archive_commands_are_not_gated_here(self):
        commands = (
            "openspec archive needs-card",
            "ai-specs archive needs-card",
            "mv openspec/changes/needs-card openspec/changes/archive/needs-card",
            "git mv openspec/changes/needs-card openspec/changes/archive/needs-card",
            "mv -t openspec/changes/archive/ openspec/changes/needs-card",
            "set -e\n# archive\nopenspec archive needs-card",
        )
        for cmd in commands:
            with self.subTest(cmd=cmd):
                r = self._run(self._shell_event(cmd), decision="block",
                              gate=self._stamped_gate("always"))
                self.assertEqual(r.returncode, 0, f"{cmd}\n{r.stderr}")
                self.assertEqual(self._logged_checkpoints(), [])

    def test_heredoc_bodies_are_not_gated(self):
        commands = (
            "cat > docs/x.md <<'EOF'\ngh pr create --fill\nEOF",
            "cat > docs/a <<'A'\ngh pr create --fill\nA\ncat > docs/b <<-B\n\topenspec archive x\n\tB",
            "printf '%s\n' \\\n  gh pr create --fill",
        )
        for cmd in commands:
            with self.subTest(cmd=cmd):
                r = self._run(self._shell_event(cmd), decision="block",
                              gate=self._stamped_gate("always"))
                self.assertEqual(r.returncode, 0, f"{cmd}\n{r.stderr}")
                self.assertEqual(self._logged_checkpoints(), [])

    def test_shell_tokenizer_pr_create_matrix(self):
        cases = (
            ("comment-only-pr", "# create the PR\ngh pr create --fill", 2),
            ("inline-comment-pr", "echo hi  # note\ngh pr create --fill", 2),
            ("herestring", 'cat <<< "hello"\ngh pr create --fill', 2),
            ("quoted-shift", 'echo "a << b"\ngh pr create --fill', 2),
            ("quoted-heredoc-text", "echo '<<EOF'\ngh pr create --fill\nEOF", 2),
            ("comment-heredoc-text", "# heredoc note <<EOF\ngh pr create --fill", 2),
            ("gated-opener-command", "gh pr create --fill --body-file - <<'EOF'\nbody\nEOF", 2),
            ("docs-heredoc-pr-body", "cat > docs/x.md <<'EOF'\ngh pr create --fill\nEOF", 0),
            ("docs-heredoc-shift-body", "cat > docs/x.md <<-EOF\n\tgh pr create --fill\n\tEOF", 0),
            ("inline-comment-semicolon", "echo hi  # note ; gh pr create --fill", 0),
            ("inline-comment-and", "make test  # lint && gh pr create", 0),
            ("comment-fallback", "echo a\n# fallback ; gh pr create", 0),
            ("mid-word-hash-gated", "echo foo#bar; gh pr create --fill", 2),
            ("escaped-hash-gated", "echo foo\\#bar; gh pr create --fill", 2),
            ("quoted-hash-gh", 'git commit -m "fix #123 and gh pr create"', 0),
            ("quoted-gh", "echo 'gh pr create'", 0),
            ("lone-cr-comment", "echo hi # note\rgh pr create --fill", 0),
            ("vertical-tab-comment", "echo hi # note\vgh pr create --fill", 0),
            ("crlf-heredoc-unquoted-then-gated",
             "cat > docs/x <<EOF\r\nbody\r\nEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-heredoc-strip-tabs-then-gated",
             "cat > docs/x <<-EOF\r\n\tbody\r\n\tEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-quoted-single-heredoc",
             "cat > docs/x <<'EOF'\r\nbody\r\nEOF\r\ngh pr create --fill\r\n", 2),
            ("mixed-endings-heredoc-swallowed",
             "cat > docs/x <<EOF\r\nbody\r\nEOF\ngh pr create --fill\n", 0),
            ("quoted-delimiter-prefix-suffix",
             "cat > docs/x <<'EO'F\nbody\nEOF\ngh pr create --fill", 2),
            ("vertical-tab-delimiter", "cat > docs/x <<\vEOF\nbody\nEOF\ngh pr create --fill", 0),
            ("vertical-tab-word-hash-gated", "echo a\v#note; gh pr create --fill", 2),
            ("fold-midword-hash-continuation", "echo foo\\\n#bar \\\ngh pr create --fill", 0),
            ("fold-space-backslash-comment-continuation",
             "echo foo \\\n#bar \\\ngh pr create --fill", 2),
            ("comment-continuation-inline", "echo hi # note \\\ngh pr create --fill", 2),
            ("comment-continuation-draft", "# gh pr create --draft \\\ngh pr create --fill", 2),
            ("unquoted-heredoc-body-fold",
             "cat > docs/x <<EOF\nbody\\\nEOF\ngh pr create --fill", 0),
            ("quoted-heredoc-body-literal",
             "cat > docs/x <<'EOF'\nbody\\\nEOF\ngh pr create --fill", 2),
            ("joined-gated-command", "gh pr \\\ncreate --fill", 2),
            ("joined-argv-not-command", "printf '%s\n' \\\n  gh pr create --fill", 0),
            ("backslash-quoted-delimiter", "cat > docs/x <<\\EOF\nbody\nEOF\ngh pr create --fill", 2),
            ("backslash-dq-escape-preserved",
             "cat > docs/x <<\"EO\\OF\"\nbody\nEO\\OF\ngh pr create --fill", 2),
            ("backslash-dq-escape-literal",
             "cat > docs/x <<\"EO\\\\OF\"\nbody\nEOOF\ngh pr create --fill", 0),
            ("eol-semicolon-pr", "echo a;\ngh pr create --fill", 2),
            ("eol-ampersand-pr", "echo hi &\ngh pr create --fill", 2),
            ("eol-pipe-pr", "echo hi |\ngh pr create --fill", 2),
            ("eol-and-pr", "echo hi &&\ngh pr create --fill", 2),
            ("bare-amp-inline-pr", "echo a & gh pr create --fill", 2),
            ("compound-separator-pipe-amp", "echo a |& gh pr create --fill", 2),
            ("compound-separator-case-amp",
             "case x in x) echo hit;& *) gh pr create --fill;; esac", 2),
            ("compound-separator-case-inline-double-semicolon",
             "case y in x) echo hit;; *) gh pr create --fill;; esac", 2),
            ("subshell-pr", "( gh pr create --fill )", 2),
            ("subshell-unspaced-pr", "(gh pr create --fill)", 2),
            ("coproc-named-subshell-pr", "coproc CO ( gh pr create --fill )", 2),
            ("compound-separator-inline-double-semicolon", "echo a ;; gh pr create --fill", 0),
            ("compound-separator-inline-semicolon-amp", "echo a ;& gh pr create --fill", 0),
            ("case-reserved-word-pr", "case x in x) gh pr create --fill;; esac", 2),
            ("case-glued-pattern-pr", "case x in x)gh pr create --fill;; esac", 2),
            ("if-reserved-word-pr", "if true; then gh pr create --fill; fi", 2),
            ("coproc-named-pr", "coproc CO { gh pr create --fill; }", 2),
            ("coproc-gh-pr-create", "coproc gh pr create --fill", 2),
            ("arith-command-shift", "((1<<2))\ngh pr create --fill", 2),
            ("single-quote-backslash-pr", "echo 'a\\'\ngh pr create --fill\necho \"oops", 2),
            ("single-quote-open-after-gated", "x='a\\'\ngh pr create --fill\ny='oops", 2),
            ("dollar-bracket-shift-midword", "x=$[1<<2]\ngh pr create --fill", 2),
            ("same-unit-unbalanced-double", 'gh pr create --title "My PR --body x', 0),
            ("same-unit-unbalanced-single", "gh pr create --fill '", 0),
            ("earlier-line-unbalanced-quote",
             "echo start\ngh pr create --title 'unterminated", 0),
            ("trailing-backslash-gated", "gh pr create --fill \\", 2),
            ("unbalanced-quote-after-gated", "gh pr create --fill\necho 'oops", 2),
        )
        for label, command, expected_rc in cases:
            with self.subTest(label=label):
                expected = expected_rc
                r = self._run(
                    self._shell_event(command),
                    decision="block",
                    gate=self._stamped_gate("always"),
                )
                self.assertEqual(r.returncode, expected, f"{label}: {command}\n{r.stderr}")

    # --- ledger evidence bridge and tracker.none exemption ---

    def test_gate_comments_name_the_tracker_ledger_host(self):
        """W6: the host comments must not still claim the artifact guardian grades it."""
        text = GATE.read_text(encoding="utf-8")
        self.assertNotIn("pre-merge guardian", text)
        self.assertNotIn("premerge_guardian", text)
        self.assertIn("tracker_ledger_host.py", text)

    def test_graded_argv_carries_the_bridge_evidence_file(self):
        self._change("demo-change")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      gate=self._stamped_gate_bridged())
        self.assertEqual(r.returncode, 0, r.stderr)
        grades = self._grade_lines()
        self.assertEqual(len(grades), 1, self.stub_log.read_text())
        self.assertIn("--evidence", grades[0], "the host must pass bridge-built evidence")
        self.assertEqual(self._write_lines(), [], "no exemption means no write")

    def test_tracker_none_never_becomes_a_host_owned_write(self):
        change = self._change("demo-change", tracker_none="\nno tracker card for this spike\n")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      gate=self._stamped_gate_bridged())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(
            self._write_lines(), [],
            "the tracker.none file must never authorize the host to write an exemption",
        )
        grades = self._grade_lines()
        self.assertEqual(len(grades), 1, self.stub_log.read_text())
        self.assertIn("--evidence", grades[0], "the host still grades the checkpoint")
        self.assertTrue((change / "tracker.none").is_file(),
                        "the host never creates or deletes the human-authored file")

    def test_tracker_none_never_reaches_the_write_surface(self):
        self._change("demo-change", tracker_none="no tracker card for this spike\n")
        env = self._env(decision="allow", STUB_WRITE_EXIT="2")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      gate=self._stamped_gate_bridged(), env=env)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._write_lines(), [],
                         "a failing write surface proves the host issues no exempt write")
        self.assertEqual(len(self._grade_lines()), 1, self.stub_log.read_text())

    def test_tracker_none_cannot_unlock_a_blocking_checkpoint(self):
        self._change("demo-change", tracker_none="no tracker card for this spike\n")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      decision="block", gate=self._stamped_gate_bridged())
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertEqual(self._write_lines(), [],
                         "writing the exemption the file describes is not the host's call")

    def test_materialize_stamps_the_bridge_directory(self):
        materialize_path = ROOT / "lib" / "_internal" / "recipe-materialize.py"
        spec = importlib.util.spec_from_file_location("recipe_materialize_stamp", materialize_path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        assert spec.loader is not None
        spec.loader.exec_module(module)

        base = Path(self.tmp.name)
        project = base / "project"
        recipe_dir = base / "recipe"
        hooks = recipe_dir / "hooks"
        hooks.mkdir(parents=True)
        (hooks / "tracker-card-gate.sh").write_text(GATE.read_text())
        hook = SimpleNamespace(script="hooks/tracker-card-gate.sh", id="tracker-card-gate")
        rel = module.materialize_hook_script(
            recipe_dir, hook, project, "trello-mcp-workflow", {"gate_mode": "warn"},
            cli_home=ROOT,
        )
        text = (project / rel).read_text()
        self.assertNotIn("__TRACKER_LIB_INTERNAL__", text, "the stamp must be substituted")
        self.assertIn(str((ROOT / "lib" / "_internal").resolve()), text)
        # No CLI home means no bridge directory: the host then skips both evidence
        # and the exemption write (fail open).
        rel2 = module.materialize_hook_script(
            recipe_dir, hook, base / "project2", "trello-mcp-workflow",
            {"gate_mode": "warn"}, cli_home=None,
        )
        text2 = (base / "project2" / rel2).read_text()
        self.assertNotIn("__TRACKER_LIB_INTERNAL__", text2)
        self.assertIn('stamped_lib_internal=""', text2)

    def test_unstamped_bridge_skips_evidence_and_exempt_write(self):
        self._change("demo-change", tracker_none="no tracker card for this spike\n")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")),
                      gate=self._stamped_gate("warn"))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._write_lines(), [])
        grades = self._grade_lines()
        self.assertEqual(len(grades), 1, self.stub_log.read_text())
        self.assertNotIn("--evidence", grades[0])
        self.assertEqual(self._logged_checkpoints(), ["apply-start"])

    # --- bash 3.2 compatibility ---

    class Bash32Tests(unittest.TestCase):
        BASH32 = "/bin/bash"

        @classmethod
        def setUpClass(cls):
            try:
                ver = subprocess.run(
                    [cls.BASH32, "-c", "echo $BASH_VERSION"],
                    capture_output=True, text=True, check=True,
                ).stdout.strip()
            except (OSError, subprocess.CalledProcessError):
                ver = ""
            if not ver.startswith("3.2"):
                raise unittest.SkipTest(f"{cls.BASH32} is not bash 3.2 (got {ver!r})")

        def setUp(self):
            self.tmp = tempfile.TemporaryDirectory()
            self.addCleanup(self.tmp.cleanup)
            base = Path(self.tmp.name)
            self.repo = base / "repo"
            self.repo.mkdir()
            _git(self.repo, "init", "-q")
            _git(self.repo, "config", "user.email", "t@t.t")
            _git(self.repo, "config", "user.name", "t")
            (self.repo / "README.md").write_text("x\n")
            _git(self.repo, "add", "-A")
            _git(self.repo, "commit", "-qm", "init")
            self.stub_log = base / "stub.log"
            self.stub = base / "worktree-gate"
            self.stub.write_text(STUB_BINARY)
            self.stub.chmod(0o755)

        def _run32(self, event: dict) -> subprocess.CompletedProcess:
            stamped = Path(self.tmp.name) / "gate-bash32.sh"
            stamped.write_text(
                GATE.read_text()
                .replace("__TRACKER_CARD_GATE_MODE__", "always")
                .replace("__TRACKER_CLI_HOME__", "")
            )
            stamped.chmod(0o755)
            env = dict(os.environ)
            for key in ("TRACKER_CARD_GATE_MODE", "TRACKER_LEDGER_MODE",
                        "TRACKER_CARD_GATE_PATHS", "AI_SPECS_HOME"):
                env.pop(key, None)
            env["WORKTREE_GATE_BIN"] = str(self.stub)
            env["STUB_LOG"] = str(self.stub_log)
            env["STUB_DECISION"] = "block"
            return subprocess.run(
                [self.BASH32, str(stamped)],
                input=json.dumps(event), capture_output=True, text=True, env=env,
            )

        def test_script_parses_under_bash_3_2(self):
            parsed = subprocess.run([self.BASH32, "-n", str(GATE)],
                                    capture_output=True, text=True)
            self.assertEqual(parsed.returncode, 0, parsed.stderr)

        def test_prod_write_blocks_under_bash_3_2(self):
            r = self._run32({
                "event": "pre-tool-use",
                "tool_name": "Edit",
                "tool_input": {"file_path": str(self.repo / "lib" / "foo.py")},
                "cwd": str(self.repo),
            })
            self.assertEqual(r.returncode, 2, r.stderr)

        def test_shell_pr_create_blocks_under_bash_3_2(self):
            r = self._run32({
                "event": "pre-tool-use",
                "tool_name": "Bash",
                "tool_input": {"command": "gh pr create --title t --body b"},
                "cwd": str(self.repo),
            })
            self.assertEqual(r.returncode, 2, r.stderr)


if __name__ == "__main__":
    unittest.main()
