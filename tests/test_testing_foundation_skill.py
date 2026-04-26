import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILL = ROOT / "catalog" / "skills" / "testing-foundation" / "SKILL.md"
README = ROOT / "README.md"


class TestingFoundationSkillTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_testing_foundation_skill_defines_mvp_policy_commands_layers_and_deferred_tooling(self):
        text = SKILL.read_text()

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

    def test_readme_points_to_testing_foundation_skill(self):
        readme = README.read_text()

        self.assertContainsAll(
            readme,
            [
                "## Testing foundation",
                "[`ai-specs/skills/testing-foundation/SKILL.md`](ai-specs/skills/testing-foundation/SKILL.md)",
                "Use `./tests/validate.sh` as the default final verification command",
            ],
        )


if __name__ == "__main__":
    unittest.main()
