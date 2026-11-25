"""Unit coverage for the plan-build eval runner's assertion primitives.

No model, no network: temp roots with real files and a real `git init` for the
git-config and commit-trailer cases. Each primitive has one conforming fixture
that must pass and one violating fixture that must fail.
"""
from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from tests.evals.lib.harness import (  # noqa: E402
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
        result = {"json": {"part": {"state": {"input": {"command": "gh pr merge 12"}}}}}
        executed = collect_executed_commands(result)
        self.assertEqual(["gh pr merge 12"], executed)
        with self.assertRaises(AssertionError):
            assert_no_forbidden_commands(executed, ["gh pr merge"])


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


if __name__ == "__main__":
    unittest.main()
