import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"
DOC = ROOT / "docs" / "ai" / "context-precedence.md"
README = ROOT / "README.md"

ORDER = "canonical docs > project skills > packs > handoffs > session memory > proposed context"


class ContextPrecedenceDocsTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_context_precedence_doc_states_order_sources_examples_and_boundary(self):
        text = DOC.read_text()

        self.assertEqual(text.count(ORDER), 1)
        self.assertContainsAll(
            text,
            [
                "# Context Precedence",
                "## Canonical Rule",
                "## Source Classes",
                "## Conflict Examples",
                "## Audit Checklist",
                "**canonical docs**",
                "**project skills**",
                "**packs**",
                "**handoffs**",
                "**session memory**",
                "**proposed context**",
                "Docs vs session memory",
                "Project skills vs packs",
                "Handoffs vs session memory",
                "Proposed context vs existing sources",
                "This MVP is a decision policy, not a runtime merge engine.",
                "MUST NOT require `[memory]`, `[precedence]`, `[packs]`, or any other new manifest section.",
            ],
        )

    def test_readme_points_to_context_precedence_doc_without_duplicating_rule(self):
        readme = README.read_text()

        self.assertContainsAll(
            readme,
            [
                "## Context precedence",
                "The canonical conflict-resolution rule lives in [`docs/ai/context-precedence.md`](docs/ai/context-precedence.md).",
                "README only points to that document so the precedence policy has one source of truth.",
            ],
        )
        self.assertEqual(readme.count(ORDER), 0)

    def test_sync_renders_agents_reference_when_context_precedence_doc_exists(self):
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-precedence-"))
        workspace = tmp / "workspace"
        try:
            shutil.copytree(FIXTURE_ROOT, workspace)
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-precedence'\n"
            )
            doc = workspace / "docs" / "ai" / "context-precedence.md"
            doc.parent.mkdir(parents=True)
            doc.write_text("# Context Precedence\n")

            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()
            self.assertContainsAll(
                agents,
                [
                    "## Context Precedence",
                    "[`docs/ai/context-precedence.md`](docs/ai/context-precedence.md)",
                    ORDER,
                    "decision policy, not an automatic merge engine",
                ],
            )
        finally:
            shutil.rmtree(tmp)


if __name__ == "__main__":
    unittest.main()
