import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ORCHESTRATOR = ROOT / "ai-specs" / "skills" / "openspec-phase-orchestrator" / "SKILL.md"
WORKFLOW = ROOT / "ai-specs" / "skills" / "openspec-sdd-workflow" / "SKILL.md"


class OpenSpecSddWorkflowSkillTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_orchestrator_defines_subagent_requirement_and_sdd_modes(self):
        text = ORCHESTRATOR.read_text()

        self.assertContainsAll(
            text,
            [
                "When the harness exposes a `task`/subagent capability, every SDD phase MUST run",
                "Inline execution is only a fallback for harnesses that do not support subagents.",
                "| `interactive` | Execute one phase at a time and ask before advancing. | After every phase. |",
                "| `auto-artifacts` | Automatically run artifact phases through `tasks.md`. | Stop after `tasks.md`; do not apply or verify. |",
                "| `auto` | Automatically run artifacts, apply, and verify. | Stop after verification (or on error/blocker). |",
                "Default for this project: `auto-artifacts`.",
                "Never enter `apply`, `verify`, `archive`, `merge`, PR creation, or push/merge to",
                "artifact-cycle review",
                "`interactive apply`, `auto apply`, or `apply only`",
            ],
        )

    def test_sdd_workflow_defaults_to_auto_artifacts_and_stops_before_apply(self):
        text = WORKFLOW.read_text()

        self.assertContainsAll(
            text,
            [
                "Default for this project: `auto-artifacts`.",
                "When a user says “execute the SDD cycle” for a card or change without specifying",
                "stop before `apply`.",
                "When the harness supports subagents, every SDD phase MUST be executed by a",
                "phase-specialized subagent through `openspec-phase-orchestrator`.",
                "After the final artifact (`tasks.md`) in `auto-artifacts`, provide an exhaustive",
                "Clear options: `interactive apply`, `auto apply`, or `apply only`.",
                "Else if tasks exist with pending checkboxes → `apply` only when mode is `auto` or the user explicitly requested apply",
            ],
        )


if __name__ == "__main__":
    unittest.main()
