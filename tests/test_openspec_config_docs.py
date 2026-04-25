import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CONFIG = ROOT / "openspec" / "config.yaml"
DOC = ROOT / "docs" / "ai" / "openspec-config.md"


class OpenSpecConfigDocsTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_config_keeps_schema_rules_as_artifact_string_lists(self):
        text = CONFIG.read_text()
        rules_text = text.split("\nai_specs_guidance:\n", 1)[0]

        self.assertNotRegex(rules_text, re.compile(r"^  apply:\n    guidance:", re.MULTILINE))
        self.assertNotRegex(rules_text, re.compile(r"^  verify:\n    guidance:", re.MULTILINE))
        self.assertNotRegex(rules_text, re.compile(r"^  archive:", re.MULTILINE))
        self.assertContainsAll(
            text,
            [
                "schema: spec-driven",
                "  apply:\n    - Follow existing code patterns and conventions",
                "    - Use ./tests/run.sh for focused test feedback during implementation",
                "  verify:\n    - Run ./tests/validate.sh before final verification",
                "ai_specs_guidance:",
                "    test_command: ./tests/run.sh",
                "    test_command: ./tests/validate.sh",
                "  archive:\n    - Warn before merging destructive deltas (large removals)",
            ],
        )

    def test_doc_records_supported_config_shape_and_validation_commands(self):
        text = DOC.read_text()

        self.assertContainsAll(
            text,
            [
                "# OpenSpec Config",
                "valid for the installed `spec-driven` schema",
                "each artifact value",
                "should be a list of strings",
                "Do not put nested objects under `rules.apply` or `rules.verify`.",
                "Do not add",
                "`rules.archive`",
                "Richer project-specific metadata",
                "`ai_specs_guidance`",
                "`./tests/validate.sh`",
                "`./tests/run.sh`",
            ],
        )


if __name__ == "__main__":
    unittest.main()
