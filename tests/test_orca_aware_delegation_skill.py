import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILL_DIR = ROOT / "ai-specs" / "skills" / "orca-aware-delegation"
SKILL_PATH = SKILL_DIR / "SKILL.md"
COMMANDS_PATH = SKILL_DIR / "references" / "orca-lifecycle-commands.md"
CONTRACT_PATH = ROOT / "lib" / "_internal" / "skill_contract.py"


def load_contract_module():
    spec = importlib.util.spec_from_file_location("skill_contract", CONTRACT_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class OrcaAwareDelegationSkillTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.text = SKILL_PATH.read_text()
        cls.commands = COMMANDS_PATH.read_text()
        cls.contract = load_contract_module()

    def test_skill_has_complete_metadata_and_explicit_triggers(self):
        skill = self.contract.from_local_skill(SKILL_PATH, compatibility=False)

        self.assertEqual(skill["name"], "orca-aware-delegation")
        self.assertLessEqual(len(skill["description"]), 250)
        self.assertTrue(skill["description"].startswith("Trigger:"))
        self.assertEqual(skill["metadata"]["scope"], ["root"])
        self.assertEqual(
            skill["metadata"]["auto_invoke"],
            [
                "Explicitly naming Orca CLI, Orca orchestration, or an Orca worktree",
                "Explicitly requesting delegation through Orca",
            ],
        )

    def test_activation_and_normal_route_boundary_are_explicit(self):
        for phrase in (
            "Activate only when the request explicitly names Orca CLI",
            "Do not activate because `orca` exists",
            "an environment variable exists",
            "worktree-flow is enabled",
            "request says only \"delegate\"",
            "Keep the normal non-Orca route unchanged",
            "Standalone/direct worktrees created by `ai-specs` are discoverable by Orca",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_hydration_precedes_orca_dispatch(self):
        worktree = self.text.index("use `/worktree-new`")
        sync = self.text.index("Run `ai-specs sync <absolute-worktree>`")
        dispatch = self.text.index("Hand Orca the existing selector")

        self.assertLess(worktree, sync)
        self.assertLess(sync, dispatch)
        self.assertIn("do not substitute `sync-agent`", self.text)
        self.assertIn("git -C <absolute-worktree> rev-parse --show-toplevel", self.text)
        self.assertIn("path:<absolute-worktree>", self.text)
        self.assertIn("Never copy generated files or secrets", self.text)

    def test_ownership_subrepo_boundary_and_failure_behavior_are_pinned(self):
        for phrase in (
            "canonical/main agent is human-facing",
            "Orca owns runtime, Run, Task, Dispatch, and terminal control",
            "A worker owns change content only",
            "must not create, remove, reassign, or independently manage",
            "For `monorepo-submodules`, fail closed",
            "Superrepo discovery does not imply subrepo worktree discovery",
            "preserve the worktree for inspection",
            "Never silently create a duplicate",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_every_launch_is_a_visible_interactive_orca_tui_session(self):
        for phrase in (
            "visible interactive TUI session created through Orca",
            "`worker-start` or an Orca terminal surface",
            "Never launch a worker with `claude -p`",
            "`opencode run`",
            "a provider API call",
            "a background/headless provider process",
            "any equivalent programmatic invocation",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

        launch_rule = self.text.index("visible interactive TUI session created through Orca")
        dispatch = self.text.index("Hand Orca the existing selector")
        self.assertLess(launch_rule, dispatch)

    def test_worker_done_does_not_imply_terminal_closure(self):
        for phrase in (
            "`worker_done` reports task completion only",
            "it never implies terminal closure",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_retain_is_asked_recorded_and_release_is_never_automatic(self):
        for phrase in (
            "Ask the human whether the TUI stays retained for inspection or continuation",
            "record the answer",
            "use `worker-retain` when it does",
            "Use `worker-release` only for an explicit close/cleanup decision",
            "after judging the worker finished",
            "Never release automatically as the default after `worker_done`",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

        retain = self.text.index("`worker-retain`")
        release = self.text.index("`worker-release`")
        self.assertLess(retain, release)

        self.assertIn("worker-retain", self.commands)
        release_lines = [
            line
            for line in self.commands.splitlines()
            if "orca orchestration worker-release" in line
        ]
        self.assertTrue(release_lines)
        for line in release_lines:
            with self.subTest(line=line):
                self.assertIn("only on an explicit human close decision", line)

    def test_examples_use_existing_worktree_without_unsupported_creation_flags(self):
        self.assertIn(
            "orca orchestration worker-start --task <task_id> "
            "--worktree path:<absolute-worktree> --agent codex --json",
            self.commands,
        )
        self.assertIn(
            "orca terminal send --terminal <handle> --text",
            self.commands,
        )
        self.assertNotIn("orca worktree create", self.text)
        self.assertNotIn("orca worktree create", self.commands)

    def test_existing_worktree_reuse_omits_setup(self):
        for phrase in (
            "reuses that exact path and rejects `--setup`",
            "omit `--setup`",
            "setup is `not_applicable`",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

        worker_start_lines = [
            line
            for line in self.commands.splitlines()
            if "orca orchestration worker-start" in line
        ]
        self.assertTrue(worker_start_lines)
        for line in worker_start_lines:
            with self.subTest(line=line):
                self.assertIn("--worktree path:<absolute-worktree>", line)
                self.assertNotIn("--setup", line)
                self.assertNotIn("new-child", line)
                self.assertNotIn("new-top-level", line)

    def test_supervised_lifecycle_sequence_is_pinned(self):
        run_create = self.commands.index("orca orchestration run-create")
        task_create = self.commands.index("orca orchestration task-create")
        worker_start = self.commands.index("orca orchestration worker-start")
        wait = self.commands.index("orca orchestration check --wait")
        retain = self.commands.index("orca orchestration worker-retain")
        release = self.commands.index("orca orchestration worker-release")

        self.assertLess(run_create, task_create)
        self.assertLess(task_create, worker_start)
        self.assertLess(worker_start, wait)
        self.assertLess(wait, retain)
        self.assertLess(retain, release)
        self.assertIn("--types worker_done,escalation,question", self.commands)
        self.assertIn(
            "canonical orchestrator keeps lifecycle ownership",
            self.text,
        )

    def test_readiness_is_not_proof_and_retry_reuses_task_and_path(self):
        for phrase in (
            "is not proof the worker executed the task",
            "`ready`",
            "`input_accepted`",
            "meaningful readiness/activity",
            "final `worker_done`",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

        for phrase in (
            "orca orchestration worker-show --dispatch",
            "--retry-of",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.commands)

        self.assertIn("never create a duplicate worktree", self.text)

        retry_lines = [
            line
            for line in self.commands.splitlines()
            if "--retry-of" in line and line.startswith("orca orchestration")
        ]
        self.assertTrue(retry_lines)
        for line in retry_lines:
            with self.subTest(line=line):
                self.assertIn("--task <task_id>", line)
                self.assertIn("--worktree path:<absolute-worktree>", line)

    def test_pre_and_post_sync_baselines_are_both_captured(self):
        pre = self.text.index("pre-sync baseline")
        post = self.text.index("post-sync baseline")
        dispatch = self.text.index("Hand Orca the existing selector")

        self.assertLess(pre, post)
        self.assertLess(post, dispatch)
        self.assertIn(
            "capture the pre-sync baseline with "
            "`git -C <absolute-worktree> status --short`",
            self.text,
        )
        self.assertIn(
            "capture the post-sync baseline with "
            "`git -C <absolute-worktree> status --short`",
            self.text,
        )

    def test_post_sync_baseline_separates_provisioning_files_from_worker_content(self):
        for phrase in (
            "`AGENTS.md`",
            "`ai-specs/.ai-specs.lock`",
            "managed recipe overrides",
            "generated runtime/recipe files",
            "worker-owned change content",
            "preserve unrelated pre-existing changes",
            "Attribute provisioning-owned generated files to sync, not to the worker",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_commit_boundary_excludes_sync_output(self):
        for phrase in (
            "Before any commit or staging",
            "revert or exclude only the sync-generated provisioning paths "
            "back to their pre-sync state",
            "stage only the worker-owned change paths",
            "Never commit sync output merely because it is tracked or changed",
            "Only an explicit separate authorization can make a provisioning change "
            "part of the deliverable",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

        baseline = self.text.index("pre-sync baseline")
        boundary = self.text.index("Before any commit or staging")
        self.assertLess(baseline, boundary)

    def test_worker_never_commits_and_orchestrator_owns_the_decision(self):
        for phrase in (
            "must not stage, commit, push, or merge",
            "the canonical orchestrator owns the final staging and commit decision",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_worker_must_verify_repo_context_before_writing(self):
        for phrase in (
            "git rev-parse --show-toplevel",
            "git branch --show-current",
            "git worktree list",
            "before its first write",
            "defense in depth",
            "OpenCode subagent/MCP, Pi/OMP, and Cursor",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, self.text)

    def test_body_stays_within_llm_first_budget(self):
        body = self.text.split("---", 2)[2]
        self.assertLessEqual(len(body.split()), 950)

    def test_commands_reference_is_linked_from_the_skill(self):
        self.assertIn("references/orca-lifecycle-commands.md", self.text)

    def test_required_sections_are_in_style_guide_order(self):
        sections = [
            "## Activation Contract",
            "## Hard Rules",
            "## Decision Gates",
            "## Execution Steps",
            "## Output Contract",
            "## References",
        ]
        positions = [self.text.index(section) for section in sections]
        self.assertEqual(positions, sorted(positions))


if __name__ == "__main__":
    unittest.main()
