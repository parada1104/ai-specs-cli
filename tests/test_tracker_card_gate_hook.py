"""Integration tests for trello-mcp-workflow tracker-card-gate.sh.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open. Bootstrap seam for hermetic tests is
the project-local fallback marker
``repo/.recipe/trello-mcp-workflow/bootstrap-ready``.
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

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "trello-mcp-workflow" / "hooks" / "tracker-card-gate.sh"
TRELLO_LINK = ROOT / "lib" / "_internal" / "trello_link.py"


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-C", str(cwd), *args],
        check=True,
        capture_output=True,
        text=True,
    )


def _load_trello_link():
    name = "trello_link_gate_parity"
    spec = importlib.util.spec_from_file_location(name, TRELLO_LINK)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


class TrackerCardGateHookTests(unittest.TestCase):
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
        # Project-local bootstrap seam (design Decision 3 §5).
        marker = self.repo / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        marker.parent.mkdir(parents=True)
        marker.write_text("ready\n")

    def _stamped_gate(self, mode: str, cli_home: str = "") -> Path:
        self.assertTrue(GATE.is_file(), f"gate script missing: {GATE}")
        stamped = Path(self.tmp.name) / f"tracker-card-gate-{mode}.sh"
        text = (
            GATE.read_text()
            .replace("__TRACKER_CARD_GATE_MODE__", mode)
            .replace("__TRACKER_CLI_HOME__", cli_home)
        )
        stamped.write_text(text)
        stamped.chmod(0o755)
        return stamped

    def _run(
        self,
        event: dict | str,
        *,
        mode: str = "always",
        gate: Path | None = None,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = dict(os.environ)
        env.pop("TRACKER_CARD_GATE_MODE", None)
        env.pop("TRACKER_CARD_GATE_PATHS", None)
        env.pop("AI_SPECS_HOME", None)
        if extra_env:
            env.update(extra_env)
        payload = event if isinstance(event, str) else json.dumps(event)
        return subprocess.run(
            ["bash", str(gate or self._stamped_gate(mode))],
            input=payload,
            capture_output=True,
            text=True,
            env=env,
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

    def _cursor_shell_event(self, command: str, cwd: str | None = None) -> dict:
        return {
            "command": command,
            "cwd": cwd or str(self.repo),
        }

    def _seed_change(
        self,
        slug: str = "demo-change",
        *,
        with_tracker: bool = False,
        tracker_none: bool = False,
        artifacts: tuple[str, ...] = ("proposal.md",),
        card_id: str = "6a622e6ad8dd4cefb8c09b81",
    ) -> Path:
        d = self.repo / "openspec" / "changes" / slug
        d.mkdir(parents=True, exist_ok=True)
        for name in artifacts:
            body = f"# {name}\n"
            if with_tracker and name in ("proposal.md", "tasks.md"):
                body += (
                    "\n## Tracker\n\n"
                    f"- **card_id**: `{card_id}`\n"
                    "- **url**: https://trello.com/c/demo\n"
                )
            (d / name).write_text(body)
        if tracker_none:
            (d / "tracker.none").write_text("no card needed\n")
        return d

    def _seed_archived(self, slug: str = "old-change") -> None:
        d = self.repo / "openspec" / "changes" / "archive" / slug
        d.mkdir(parents=True)
        (d / "proposal.md").write_text("# archived\n")

    # --- Phase 2 path-mode matrix ---

    def test_missing_card_blocks_prod_write(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)
        err = r.stderr.lower()
        self.assertTrue(
            "tracker" in err or "card" in err,
            r.stderr,
        )
        self.assertIn("demo-change", r.stderr)
        self.assertIn("## Tracker", r.stderr)

    def test_with_card_allows_prod_write(self):
        self._seed_change(with_tracker=True)
        r = self._run(self._event("Write", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_openspec_paths_never_blocked(self):
        self._seed_change(with_tracker=False)
        target = self.repo / "openspec" / "changes" / "demo-change" / "proposal.md"
        r = self._run(self._event("Write", str(target)), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_marker_absent_fail_open(self):
        self._seed_change(with_tracker=False)
        marker = self.repo / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        marker.unlink()
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_mode_off_allows(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="off")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_recipe_disabled_or_mode_off_allows_via_env(self):
        # Mode off via env override beats stamped always.
        self._seed_change(with_tracker=False)
        gate = self._stamped_gate("always")
        r = self._run(
            self._event("Edit", str(self.repo / "lib" / "foo.py")),
            gate=gate,
            extra_env={"TRACKER_CARD_GATE_MODE": "off"},
        )
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_warn_mode_allows_with_stderr(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="warn")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue(r.stderr.strip(), "expected non-empty stderr warning")

    def test_tracker_none_allows_prod_write(self):
        self._seed_change(with_tracker=False, tracker_none=True)
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_no_active_change_allows(self):
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_non_production_path_allows(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "tests" / "x.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_malformed_stdin_fail_open(self):
        r = self._run("not json", mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_missing_file_path_fail_open(self):
        r = self._run(
            {"event": "pre-tool-use", "tool_name": "Write", "tool_input": {}, "cwd": str(self.repo)},
            mode="always",
        )
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_archive_only_tree_ignored(self):
        self._seed_archived()
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_stray_dir_without_artifacts_ignored(self):
        d = self.repo / "openspec" / "changes" / "stray"
        d.mkdir(parents=True)
        (d / "notes.txt").write_text("hi\n")
        r = self._run(self._event("Edit", str(self.repo / "lib" / "foo.py")), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_deficient_slug_remediation_has_clean_paths(self):
        self._seed_change("aaa", with_tracker=False)
        self._seed_change("bbb", with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "catalog" / "x.toml")), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("aaa, bbb", r.stderr)
        self.assertNotRegex(r.stderr, r"openspec/changes/[^/]*,[^/]*/tracker\.none")

    def test_single_deficient_slug_remediation_has_exact_path(self):
        self._seed_change("only-one", with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "catalog" / "x.toml")), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("openspec/changes/only-one/tracker.none", r.stderr)

    def test_three_deficient_slugs_remediation_list_is_readable(self):
        for slug in ("aaa", "bbb", "ccc"):
            self._seed_change(slug, with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "catalog" / "x.toml")), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("aaa, bbb, ccc", r.stderr)
        self.assertNotIn("bbb ccc", r.stderr)

    def test_heredoc_pr_create_body_is_not_gated(self):
        self._seed_change(with_tracker=False)
        command = "cat > docs/x.md <<'EOF'\ngh pr create --fill\nEOF"
        r = self._run(self._shell_event(command), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_heredoc_archive_body_is_not_gated(self):
        self._seed_change(with_tracker=False)
        command = "cat > docs/x.md <<'EOF'\nopenspec archive needs-card\nEOF"
        r = self._run(self._shell_event(command), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_shell_preprocessing_matches_bash_truth_matrix(self):
        self._seed_change("needs-card", with_tracker=False)
        cases = (
            ("comment-only-pr", "# create the PR\ngh pr create --fill", 2),
            ("inline-comment-pr", "echo hi  # note\ngh pr create --fill", 2),
            ("comment-only-archive", "set -e\n# archive\nopenspec archive needs-card", 2),
            ("herestring", 'cat <<< "hello"\ngh pr create --fill', 2),
            ("quoted-shift", 'echo "a << b"\ngh pr create --fill', 2),
            ("python-shift", "python3 -c 'print(1 << 2)'\nopenspec archive needs-card", 2),
            ("quoted-heredoc-text", "echo '<<EOF'\ngh pr create --fill\nEOF", 2),
            ("comment-heredoc-text", "# heredoc note <<EOF\ngh pr create --fill", 2),
            ("gated-opener-command", "gh pr create --fill --body-file - <<'EOF'\nbody\nEOF", 2),
            ("docs-heredoc-pr-body", "cat > docs/x.md <<'EOF'\ngh pr create --fill\nEOF", 0),
            ("docs-heredoc-shift-body", "cat > docs/x.md <<-EOF\n\tgh pr create --fill\n\tEOF", 0),
            ("multiple-heredocs", "cat > docs/a <<'A'\ngh pr create --fill\nA\ncat > docs/b <<-B\n\topenspec archive needs-card\n\tB", 0),
            ("inline-comment-semicolon", "echo hi  # note ; gh pr create --fill", 0),
            ("inline-comment-and", "make test  # lint && gh pr create", 0),
            ("inline-comment-pipe", "ls  # pipe | openspec archive x", 0),
            ("comment-fallback", "echo a\n# fallback ; gh pr create", 0),
            ("mid-word-hash-gated", "echo foo#bar; gh pr create --fill", 2),
            ("escaped-hash-gated", "echo foo\\#bar; gh pr create --fill", 2),
            ("mid-word-hash-allow", "echo foo#bar; echo ok", 0),
            ("quoted-hash-gh", 'git commit -m "fix #123 and gh pr create"', 0),
            ("quoted-gh", "echo 'gh pr create'", 0),
            ("lone-cr-comment", "echo hi # note\rgh pr create --fill", 0),
            ("vertical-tab-comment", "echo hi # note\vgh pr create --fill", 0),
            ("form-feed-comment", "echo hi # note\fgh pr create --fill", 0),
            ("next-line-comment", "echo hi # note\x85gh pr create --fill", 0),
            ("line-separator-comment", "echo hi # note\u2028gh pr create --fill", 0),
            ("paragraph-separator-comment", "echo hi # note\u2029gh pr create --fill", 0),
            ("crlf-heredoc-unquoted-then-gated", "cat > docs/x <<EOF\r\nbody\r\nEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-heredoc-strip-tabs-then-gated", "cat > docs/x <<-EOF\r\n\tbody\r\n\tEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-archive-heredoc-then-gated", "cat > docs/x <<EOF\r\nbody\r\nEOF\r\nopenspec archive needs-card\r\n", 2),
            ("crlf-quoted-single-heredoc", "cat > docs/x <<'EOF'\r\nbody\r\nEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-quoted-double-heredoc", "cat > docs/x <<\"EOF\"\r\nbody\r\nEOF\r\ngh pr create --fill\r\n", 2),
            ("crlf-quoted-strip-tabs-heredoc", "cat > docs/x <<-'EOF'\r\n\tbody\r\n\tEOF\r\ngh pr create --fill\r\n", 2),
            ("mixed-endings-heredoc-swallowed", "cat > docs/x <<EOF\r\nbody\r\nEOF\ngh pr create --fill\n", 0),
            ("quoted-delimiter-prefix-suffix", "cat > docs/x <<'EO'F\nbody\nEOF\ngh pr create --fill", 2),
            ("quoted-delimiter-suffix", "cat > docs/x <<'EOF'X\nbody\nEOFX\ngh pr create --fill", 2),
            ("vertical-tab-delimiter", "cat > docs/x <<\vEOF\nbody\nEOF\ngh pr create --fill", 0),
            ("vertical-tab-word-hash-gated", "echo a\v#note; gh pr create --fill", 2),
            ("form-feed-word-hash-gated", "echo a\f#note; gh pr create --fill", 2),
            ("next-line-word-hash-gated", "echo a\x85#note; gh pr create --fill", 2),
            ("line-separator-word-hash-gated", "echo a\u2028#note; gh pr create --fill", 2),
            ("paragraph-separator-word-hash-gated", "echo a\u2029#note; gh pr create --fill", 2),
            ("cr-word-hash-gated", "echo a\r#note; gh pr create --fill", 2),
            ("nbsp-word-hash-gated", "echo a\u00a0#note; gh pr create --fill", 2),
            ("fold-midword-hash-continuation", "echo foo\\\n#bar \\\ngh pr create --fill", 0),
            ("fold-midword-hash-continuation-archive", "echo foo\\\n#bar \\\nopenspec archive needs-card", 0),
            ("comment-continuation-inline", "echo hi # note \\\ngh pr create --fill", 2),
            ("comment-continuation-only", "# note \\\ngh pr create --fill", 2),
            ("comment-continuation-archive", "set -e\n# archive \\\nopenspec archive needs-card", 2),
            ("comment-continuation-draft", "# gh pr create --draft \\\ngh pr create --fill", 2),
            ("unquoted-heredoc-body-fold", "cat > docs/x <<EOF\nbody\\\nEOF\ngh pr create --fill", 0),
            ("quoted-heredoc-body-literal", "cat > docs/x <<'EOF'\nbody\\\nEOF\ngh pr create --fill", 2),
            ("joined-gated-command", "gh pr \\\ncreate --fill", 2),
            ("joined-argv-not-command", "printf '%s\n' \\\n  gh pr create --fill", 0),
            ("single-quoted-backslash-newline-anti-over-fold", "echo 'a\\\nb'; gh pr create --fill", 2),
            ("backslash-quoted-delimiter", "cat > docs/x <<\\EOF\nbody\nEOF\ngh pr create --fill", 2),
            ("backslash-mid-delimiter", "cat > docs/x <<E\\OF\nbody\nEOF\ngh pr create --fill", 2),
            ("arith-shift-unquoted", "echo $((1<<2))\ngh pr create --fill", 2),
            ("trailing-backslash-gated", "gh pr create --fill \\", 2),
            ("unbalanced-quote-after-gated", "gh pr create --fill\necho 'oops", 2),
        )
        for label, command, expected_rc in cases:
            with self.subTest(label=label):
                result = self._run(self._shell_event(command), mode="always")
                self.assertEqual(result.returncode, expected_rc, f"{label}: {command}\n{result.stderr}")

    def test_comments_do_not_bypass_shell_gate(self):
        self._seed_change("needs-card", with_tracker=False)
        cases = (
            "# create the PR\ngh pr create --fill",
            "echo hi  # note\ngh pr create --fill",
            "set -e\n# archive\nopenspec archive needs-card",
        )
        for command in cases:
            with self.subTest(command=command):
                blocked = self._run(self._shell_event(command), mode="always")
                self.assertEqual(blocked.returncode, 2, f"{command}\n{blocked.stderr}")
                warning = self._run(self._shell_event(command), mode="warn")
                self.assertEqual(warning.returncode, 0, f"{command}\n{warning.stderr}")
                self.assertTrue(warning.stderr.strip())

    def test_paths_override_includes_ai_specs(self):
        self._seed_change(with_tracker=False)
        # Default excludes ai-specs → allow
        allowed = self._run(
            self._event("Edit", str(self.repo / "ai-specs" / "ai-specs.toml")),
            mode="always",
        )
        self.assertEqual(allowed.returncode, 0, allowed.stderr)
        # Override includes ai-specs → block
        blocked = self._run(
            self._event("Edit", str(self.repo / "ai-specs" / "ai-specs.toml")),
            mode="always",
            extra_env={"TRACKER_CARD_GATE_PATHS": "lib catalog bin src ai-specs"},
        )
        self.assertEqual(blocked.returncode, 2, blocked.stderr)
    def test_shell_command_separators_are_detected(self):
        self._seed_change("needs-card", with_tracker=False)
        cases = (
            "cd sub; gh pr create --fill",
            "cd sub\ngh pr create --fill",
            "set -e\nopenspec archive needs-card",
            "git add -A; openspec archive needs-card",
        )
        for cmd in cases:
            with self.subTest(cmd=cmd):
                for mode, expected_rc in (("always", 2), ("warn", 0)):
                    r = self._run(self._shell_event(cmd), mode=mode)
                    self.assertEqual(r.returncode, expected_rc, f"{mode}: {cmd}\n{r.stderr}")
                    if mode == "warn":
                        self.assertTrue(r.stderr.strip())

    # --- Phase 4 shell-mode ---

    def test_shell_gh_pr_create_blocked_without_card(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._shell_event("gh pr create --title t --body b"), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_shell_gh_pr_create_warn_allows(self):
        self._seed_change(with_tracker=False)
        r = self._run(self._shell_event("gh pr create --title t --body b"), mode="warn")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue(r.stderr.strip())

    def test_shell_gh_pr_create_allowed_when_carded(self):
        self._seed_change(with_tracker=True)
        r = self._run(self._shell_event("gh pr create --title t --body b"), mode="always")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_archive_command_blocked_for_deficient_slug(self):
        self._seed_change("needs-card", with_tracker=False)
        cases = [
            "openspec archive needs-card",
            "ai-specs archive needs-card",
            "mv openspec/changes/needs-card openspec/changes/archive/needs-card",
            "git mv openspec/changes/needs-card openspec/changes/archive/needs-card",
        ]
        for cmd in cases:
            with self.subTest(cmd=cmd):
                r = self._run(self._shell_event(cmd), mode="always")
                self.assertEqual(r.returncode, 2, f"{cmd}\n{r.stderr}")
    def test_archive_unresolved_slug_falls_back_to_any_deficient(self):
        self._seed_change("needs-card", with_tracker=False)
        for mode, expected_rc in (("always", 2), ("warn", 0)):
            with self.subTest(mode=mode):
                r = self._run(
                    self._shell_event("openspec archive unknown-slug"),
                    mode=mode,
                )
                self.assertEqual(r.returncode, expected_rc, r.stderr)
                self.assertIn("needs-card", r.stderr)


    def test_ambiguous_shell_command_fail_open(self):
        self._seed_change(with_tracker=False)
        for cmd in ("gh pr view 1", "git status", "ls lib"):
            with self.subTest(cmd=cmd):
                r = self._run(self._shell_event(cmd), mode="always")
                self.assertEqual(r.returncode, 0, f"{cmd}\n{r.stderr}")

    def test_cursor_native_shell_pr_create_blocked(self):
        self._seed_change(with_tracker=False)
        r = self._run(
            self._cursor_shell_event("gh pr create --fill"),
            mode="always",
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    # --- Phase 3.3 parser parity ---

    def test_parser_parity_with_trello_link(self):
        """Gate validity equals trello_link.is_valid_link on the Phase 1 matrix."""
        self.assertTrue(GATE.is_file(), "gate script required for parity")
        link = _load_trello_link()
        fixtures = [
            (
                "bold",
                "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"
                "- **url**: https://trello.com/c/x\n",
                True,
            ),
            ("plain", "## Tracker\n\ncard_id: abc\n", True),
            ("empty", "## Tracker\n\n- **card_id**: ``\n", False),
            ("missing", "# no section\n", False),
            (
                "comment",
                "## Tracker\n\n- **card_id**: `deadbeef` # note\n",
                True,
            ),
            (
                "fenced-inside",
                "## Tracker\n\n```markdown\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n```\n",
                False,
            ),
        ]
        for name, body, expect in fixtures:
            with self.subTest(name=name):
                change = self.repo / "openspec" / "changes" / f"parity-{name}"
                change.mkdir(parents=True, exist_ok=True)
                proposal = change / "proposal.md"
                proposal.write_text(body)
                # Clear sibling changes so only this one is evaluated.
                for other in (self.repo / "openspec" / "changes").iterdir():
                    if other.is_dir() and other.name != change.name and other.name != "archive":
                        for child in other.iterdir():
                            if child.is_file():
                                child.unlink()
                        other.rmdir()
                py_valid = link.is_valid_link([proposal, change / "tasks.md"])
                self.assertEqual(py_valid, expect)
                r = self._run(
                    self._event("Edit", str(self.repo / "lib" / "foo.py")),
                    mode="always",
                )
                if expect:
                    self.assertEqual(r.returncode, 0, r.stderr)
                else:
                    self.assertEqual(r.returncode, 2, r.stderr)


if __name__ == "__main__":
    unittest.main()
