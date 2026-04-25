import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOC = ROOT / "docs" / "ai" / "testing-foundation.md"
README = ROOT / "README.md"


class TestingFoundationDocsTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_testing_foundation_doc_defines_mvp_policy_commands_layers_and_deferred_tooling(self):
        text = DOC.read_text()

        self.assertContainsAll(
            text,
            [
                "# Testing Foundation",
                "Testing is part of the ai-specs MVP as a cross-cutting quality foundation.",
                "`./tests/run.sh` runs the Python unittest suite.",
                "`./tests/validate.sh` runs syntax checks and then `./tests/run.sh`.",
                "Unit tests for pure parsing, normalization, rendering, and validation logic.",
                "Integration-style CLI or sync tests for generated files and command flows.",
                "Documentation/generated artifact tests when README, AGENTS.md, templates, or",
                "Smoke validation through `./tests/validate.sh` before a change is considered",
                "record RED/GREEN and final validation in",
                "Coverage, linting, type checking, and formatting are desirable but not blocking",
            ],
        )

    def test_readme_points_to_testing_foundation_doc(self):
        readme = README.read_text()

        self.assertContainsAll(
            readme,
            [
                "## Testing foundation",
                "[`docs/ai/testing-foundation.md`](docs/ai/testing-foundation.md)",
                "Use `./tests/validate.sh` as the default final verification command",
            ],
        )


if __name__ == "__main__":
    unittest.main()
