"""Unit coverage for the plan-build eval runner's assertion primitives.

No model, no network: temp roots with real files and a real `git init` for the
git-config and commit-trailer cases. Each primitive has one conforming fixture
that must pass and one violating fixture that must fail.
"""
from __future__ import annotations

import ast
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from tests.evals.lib.harness import (  # noqa: E402
    CLAUDE_ALLOWED_COMMANDS,
    allow_cli_commands,
    assert_absent_path_globs,
    assert_active_change_gone,
    assert_commit_trailer,
    assert_git_config,
    assert_no_forbidden_commands,
    assert_scenario_outputs,
    assert_transcript,
    collect_executed_commands,
    current_branch,
    init_git_repo,
    run_claude_prompt,
)

BRANCH = "fixture-branch"


class _Fixture(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)

    def git(self, *args: str) -> str:
        return subprocess.run(
            ["git", "-C", str(self.root), *args],
            capture_output=True,
            text=True,
            check=True,
        ).stdout

    def git_repo(self) -> None:
        init_git_repo(self.root)
        self.git("checkout", "-q", "-b", BRANCH)

    def commit(self, message: str) -> None:
        self.git("add", "-A")
        self.git("commit", "-qm", message)


class RequiredAbsentPathGlobsTests(_Fixture):
    def test_passes_when_no_path_matches(self):
        (self.root / "src").mkdir()
        (self.root / "src" / "app.py").write_text("x = 1\n")
        assert_absent_path_globs(self.root, ["openspec/changes/**", "odd/changes/**"])

    def test_fails_when_a_path_matches(self):
        change = self.root / "openspec" / "changes" / "signup-validation"
        change.mkdir(parents=True)
        (change / "tasks.md").write_text("# Tasks\n")
        with self.assertRaises(AssertionError) as ctx:
            assert_absent_path_globs(self.root, ["openspec/changes/**"])
        self.assertIn("openspec/changes/**", str(ctx.exception))


class RequiredGitConfigTests(_Fixture):
    def test_passes_on_actual_branch_with_expected_value(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        self.assertEqual(current_branch(self.root), BRANCH)
        self.git("config", f"branch.{BRANCH}.aiSpecsCeremony", "none")
        assert_git_config(self.root, {"branch.<branch>.aiSpecsCeremony": "none"})

    def test_fails_when_key_is_missing(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        with self.assertRaises(AssertionError) as ctx:
            assert_git_config(self.root, {"branch.<branch>.aiSpecsCeremony": "none"})
        message = str(ctx.exception)
        self.assertIn(f"branch.{BRANCH}.aiSpecsCeremony", message)
        self.assertIn("expected 'none'", message)

    def test_fails_when_value_differs(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        self.git("config", f"branch.{BRANCH}.aiSpecsCeremony", "standard")
        with self.assertRaises(AssertionError) as ctx:
            assert_git_config(self.root, {"branch.<branch>.aiSpecsCeremony": "none"})
        message = str(ctx.exception)
        self.assertIn("expected 'none'", message)
        self.assertIn("got 'standard'", message)


class ForbiddenCommandsTests(_Fixture):
    def test_passes_when_only_safe_commands_ran(self):
        assert_no_forbidden_commands(["go test ./..."], ["git merge", "gh pr merge"])

    def test_fails_case_insensitively_when_a_command_ran(self):
        with self.assertRaises(AssertionError) as ctx:
            assert_no_forbidden_commands(
                ["GH PR Merge --squash"], ["git merge", "gh pr merge"]
            )
        self.assertIn("gh pr merge", str(ctx.exception))

    def test_fails_closed_when_the_runtime_recorded_nothing(self):
        with self.assertRaises(AssertionError) as ctx:
            assert_no_forbidden_commands(None, ["git merge"])
        self.assertIn("no tool-call evidence", str(ctx.exception))

    def test_prose_is_not_evidence(self):
        # Saying it will not merge neither proves nor disproves anything.
        result = {"result_text": "I will never run git merge.", "stdout": ""}
        self.assertIsNone(collect_executed_commands(result))

    def test_reads_the_command_from_the_tool_call_record(self):
        result = {
            "json": {
                "type": "tool",
                "part": {"state": {"input": {"command": "gh pr merge 12"}}},
            }
        }
        executed = collect_executed_commands(result)
        self.assertEqual(["gh pr merge 12"], executed)
        with self.assertRaises(AssertionError):
            assert_no_forbidden_commands(executed, ["gh pr merge"])

    def test_a_final_result_object_is_not_tool_call_evidence(self):
        # Parseable JSON is not proof the runtime reported its tool calls; a
        # prohibited command could hide behind a payload with no events at all.
        result = {"json": {"result": "all done", "num_turns": 3}, "stdout": ""}
        self.assertIsNone(collect_executed_commands(result))
        with self.assertRaises(AssertionError) as ctx:
            assert_no_forbidden_commands(
                collect_executed_commands(result), ["git merge"]
            )
        self.assertIn("no tool-call evidence", str(ctx.exception))


class RequiredCommitTrailerTests(_Fixture):
    def test_passes_when_a_commit_carries_the_trailer(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        (self.root / "b.txt").write_text("b\n")
        self.commit("feat: touch high-risk path\n\nRisk: item 2 (touches auth path)\n")
        assert_commit_trailer(self.root, "Risk: item")

    def test_fails_when_no_commit_carries_the_trailer(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        (self.root / "b.txt").write_text("b\n")
        self.commit("feat: touch high-risk path\n")
        with self.assertRaises(AssertionError):
            assert_commit_trailer(self.root, "Risk: item")

    def test_fails_outside_the_item_range(self):
        self.git_repo()
        (self.root / "a.txt").write_text("a\n")
        self.commit("baseline")
        (self.root / "b.txt").write_text("b\n")
        self.commit("feat: touch high-risk path\n\nRisk: item 9 (unknown item)\n")
        with self.assertRaises(AssertionError):
            assert_commit_trailer(self.root, "Risk: item")


class ModeAwareActiveChangeGoneTests(_Fixture):
    def test_true_keeps_openspec_behavior(self):
        assert_active_change_gone(self.root, "signup-validation", True)
        change = self.root / "openspec" / "changes" / "signup-validation"
        change.mkdir(parents=True)
        with self.assertRaises(AssertionError):
            assert_active_change_gone(self.root, "signup-validation", True)

    def test_odd_mode_reads_the_odd_tree(self):
        # An openspec folder is irrelevant in odd mode.
        (self.root / "openspec" / "changes" / "signup-validation").mkdir(parents=True)
        assert_active_change_gone(self.root, "signup-validation", "odd")
        (self.root / "odd" / "changes" / "signup-validation").mkdir(parents=True)
        with self.assertRaises(AssertionError) as ctx:
            assert_active_change_gone(self.root, "signup-validation", "odd")
        self.assertIn("odd/changes/signup-validation", str(ctx.exception).replace("\\", "/"))

    def test_openspec_mode_string_matches_true(self):
        (self.root / "odd" / "changes" / "signup-validation").mkdir(parents=True)
        assert_active_change_gone(self.root, "signup-validation", "openspec")
        (self.root / "openspec" / "changes" / "signup-validation").mkdir(parents=True)
        with self.assertRaises(AssertionError):
            assert_active_change_gone(self.root, "signup-validation", "openspec")

    def test_unknown_mode_is_rejected(self):
        with self.assertRaises(ValueError):
            assert_active_change_gone(self.root, "signup-validation", "both")


class TranscriptIsNotSoftTests(_Fixture):
    """The old runner skipped transcript keys whenever `required_content` was
    present; that made "asked before merging" ungradable."""

    def _meta(self) -> dict:
        plan = self.root / "openspec" / "changes" / "signup-validation" / "tasks.md"
        plan.parent.mkdir(parents=True)
        plan.write_text("## Tasks\n\nArchived the change.\n")
        return {
            "required_content": [
                {"path": "openspec/changes/signup-validation/tasks.md", "contains_any": ["Archived"]}
            ],
            "required_transcript_all": ["archive"],
            "required_transcript_one_of": ["merge?", "before merging"],
        }

    def test_passes_when_content_and_transcript_both_conform(self):
        result = {"result_text": "I ran archive. Merge? I will wait before merging."}
        assert_scenario_outputs(self.root, self._meta(), result, [])

    def test_fails_when_content_conforms_but_transcript_lacks_the_needle(self):
        result = {"result_text": "I ran archive."}
        with self.assertRaises(AssertionError) as ctx:
            assert_scenario_outputs(self.root, self._meta(), result, [])
        self.assertIn("transcript missing any of", str(ctx.exception))

    def test_fails_without_required_content_too(self):
        with self.assertRaises(AssertionError):
            assert_transcript("plain answer", all_of=["archive"])


class ClaudeAllowListTests(_Fixture):
    """The one permission the eval fixture grants, pinned.

    The agent under test may run `ai-specs` because the skill tells it to. The
    allow-list is written into the sandbox's own settings file only, and it is
    deliberately one command family rather than a blanket wildcard.
    """

    def test_allows_exactly_one_command_family(self) -> None:
        self.assertEqual(("Bash(ai-specs:*)",), CLAUDE_ALLOWED_COMMANDS)

    def test_never_widens_to_a_blanket_allow(self) -> None:
        for forbidden in ("Bash", "Bash(*)", "*", "Bash(ai-specs)", "Bash(rm:*)"):
            self.assertNotIn(forbidden, CLAUDE_ALLOWED_COMMANDS)

    def test_writes_the_allow_list_without_clobbering_hooks(self) -> None:
        settings = self.root / ".claude" / "settings.json"
        settings.parent.mkdir(parents=True)
        settings.write_text(json.dumps({"hooks": {"PreToolUse": []}}), encoding="utf-8")
        allow_cli_commands(self.root)
        data = json.loads(settings.read_text(encoding="utf-8"))
        self.assertEqual(["Bash(ai-specs:*)"], data["permissions"]["allow"])
        self.assertEqual({"PreToolUse": []}, data["hooks"])

    def test_is_idempotent(self) -> None:
        allow_cli_commands(self.root)
        allow_cli_commands(self.root)
        settings = self.root / ".claude" / "settings.json"
        data = json.loads(settings.read_text(encoding="utf-8"))
        self.assertEqual(["Bash(ai-specs:*)"], data["permissions"]["allow"])

    def test_does_not_touch_anything_outside_the_sandbox(self) -> None:
        allow_cli_commands(self.root)
        written = {
            p.relative_to(self.root)
            for p in self.root.rglob("*")
            if p.is_file()
        }
        self.assertEqual({Path(".claude/settings.json")}, written)

    def test_the_runtime_is_invoked_with_that_same_allow_list(self) -> None:
        # The settings file alone is not honoured by `claude -p`, so the same
        # single family must reach the command line -- and nothing wider with it.
        from tests.evals.lib import harness

        captured: list[list[str]] = []

        def fake_run(cmd, *args, **kwargs):
            captured.append(list(cmd))
            return {"returncode": 0, "stdout": '{"result": "ok"}', "stderr": "", "cmd": cmd}

        real = harness._run
        harness._run = fake_run  # type: ignore[assignment]
        try:
            run_claude_prompt(self.root, "hola")
        finally:
            harness._run = real  # type: ignore[assignment]

        cmd = captured[-1]
        self.assertIn("--allowedTools", cmd)
        start = cmd.index("--allowedTools") + 1
        self.assertEqual(
            list(CLAUDE_ALLOWED_COMMANDS),
            cmd[start : start + len(CLAUDE_ALLOWED_COMMANDS)],
        )
        self.assertNotIn("--dangerously-skip-permissions", cmd)


class SeededImplementationTests(unittest.TestCase):
    def test_the_seeded_implementation_parses(self) -> None:
        # A seed that does not compile turns every agent into a bug reporter: the
        # archive case failed because the agent found a SyntaxError in the fixture.
        from tests.evals.eval_plan_build_flow_live import SEEDED_IMPLEMENTATION

        ast.parse(SEEDED_IMPLEMENTATION)
        self.assertIn("ValueError", SEEDED_IMPLEMENTATION)
        self.assertIn('"email"', SEEDED_IMPLEMENTATION)


if __name__ == "__main__":
    unittest.main()
