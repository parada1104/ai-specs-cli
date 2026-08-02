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

    def test_one_deficient_among_several_blocks(self):
        self._seed_change("good", with_tracker=True)
        self._seed_change("bad", with_tracker=False)
        r = self._run(self._event("Edit", str(self.repo / "catalog" / "x.toml")), mode="always")
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("bad", r.stderr)

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
