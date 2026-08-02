"""Unit tests for lib/_internal/trello_link.py — ## Tracker section parser."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MOD_PATH = ROOT / "lib" / "_internal" / "trello_link.py"


def load_module():
    if not MOD_PATH.is_file():
        return None
    name = "trello_link_under_test"
    spec = importlib.util.spec_from_file_location(name, MOD_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class TrelloLinkParserTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.assertIsNotNone(
            self.mod,
            "lib/_internal/trello_link.py must exist (RED until GREEN)",
        )
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)

    def _write(self, name: str, text: str) -> Path:
        p = self.dir / name
        p.write_text(text)
        return p

    def test_bold_key_list_form_lowercases_keys(self):
        p = self._write(
            "proposal.md",
            "# Title\n\n## Tracker\n\n"
            "- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"
            "- **shortLink**: `5UIKk5jp`\n"
            "- **url**: https://trello.com/c/5UIKk5jp/48-demo\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "6a622e6ad8dd4cefb8c09b81")
        self.assertEqual(got["shortlink"], "5UIKk5jp")
        self.assertEqual(got["url"], "https://trello.com/c/5UIKk5jp/48-demo")
        self.assertTrue(self.mod.is_valid_link([p]))

    def test_plain_key_value_form(self):
        p = self._write(
            "proposal.md",
            "## Tracker\n\ncard_id: abc123\nurl: https://example.test/c/x\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "abc123")
        self.assertEqual(got["url"], "https://example.test/c/x")
        self.assertTrue(self.mod.is_valid_link([p]))

    def test_backticks_and_trailing_hash_comment_stripped(self):
        p = self._write(
            "proposal.md",
            "## Tracker\n\n"
            "- **card_id**: `deadbeef` # comment\n"
            "- **url**: `https://trello.com/c/x` # trailing\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "deadbeef")
        self.assertEqual(got["url"], "https://trello.com/c/x")

    def test_duplicate_keys_first_wins(self):
        p = self._write(
            "proposal.md",
            "## Tracker\n\n"
            "- **card_id**: `first`\n"
            "- **card_id**: `second`\n"
            "- **url**: https://a.test\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "first")

    def test_headings_unknown_keys_blank_lines_ignored(self):
        p = self._write(
            "proposal.md",
            "## Tracker\n\n"
            "### Nested\n"
            "\n"
            "- **card_id**: `ok`\n"
            "- **unknown_key**: skip-me\n"
            "- **url**: https://ok.test\n"
            "not a pair line\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "ok")
        self.assertNotIn("unknown_key", got)
        self.assertEqual(set(got), {"card_id", "url"})

    def test_missing_file_returns_empty_invalid(self):
        missing = self.dir / "missing.md"
        self.assertEqual(self.mod.parse_tracker_section([missing]), {})
        self.assertFalse(self.mod.is_valid_link([missing]))

    def test_no_tracker_section_invalid(self):
        p = self._write("proposal.md", "# Title\n\n## Scope\n\n- **card_id**: `x`\n")
        self.assertEqual(self.mod.parse_tracker_section([p]), {})
        self.assertFalse(self.mod.is_valid_link([p]))

    def test_empty_card_id_invalid_nonempty_valid_without_url(self):
        empty = self._write(
            "empty.md",
            "## Tracker\n\n- **card_id**: ``\n- **url**: https://x.test\n",
        )
        self.assertFalse(self.mod.is_valid_link([empty]))

        no_url = self._write(
            "nourl.md",
            "## Tracker\n\n- **card_id**: `present`\n",
        )
        self.assertTrue(self.mod.is_valid_link([no_url]))

    def test_proposal_wins_over_tasks_fallback(self):
        proposal = self._write(
            "proposal.md",
            "## Tracker\n\n- **card_id**: `from-proposal`\n",
        )
        tasks = self._write(
            "tasks.md",
            "## Tracker\n\n- **card_id**: `from-tasks`\n",
        )
        got = self.mod.parse_tracker_section([proposal, tasks])
        self.assertEqual(got["card_id"], "from-proposal")

    def test_tasks_fallback_when_proposal_has_no_section(self):
        proposal = self._write("proposal.md", "# No tracker here\n")
        tasks = self._write(
            "tasks.md",
            "## Tracker\n\n- **card_id**: `from-tasks`\n",
        )
        got = self.mod.parse_tracker_section([proposal, tasks])
        self.assertEqual(got["card_id"], "from-tasks")
        self.assertTrue(self.mod.is_valid_link([proposal, tasks]))

    def test_section_ends_at_next_h2(self):
        p = self._write(
            "proposal.md",
            "## Tracker\n\n- **card_id**: `in-section`\n\n"
            "## Next\n\n- **card_id**: `outside`\n",
        )
        got = self.mod.parse_tracker_section([p])
        self.assertEqual(got["card_id"], "in-section")

    def test_card_id_looks_canonical_24_hex_only(self):
        self.assertTrue(
            self.mod.card_id_looks_canonical("6a622e6ad8dd4cefb8c09b81")
        )
        self.assertTrue(
            self.mod.card_id_looks_canonical("6A622E6AD8DD4CEFB8C09B81")
        )
        self.assertFalse(self.mod.card_id_looks_canonical("5UIKk5jp"))
        self.assertFalse(self.mod.card_id_looks_canonical(""))
        self.assertFalse(self.mod.card_id_looks_canonical("6a622e6ad8dd4cefb8c09b8"))
        self.assertFalse(
            self.mod.card_id_looks_canonical("6a622e6ad8dd4cefb8c09b811")
        )


if __name__ == "__main__":
    unittest.main()
