"""Black-box coverage for Trello tracker-link validation."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke, snapshot


class TrelloLinkParserTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.root = base / "project"
        ai_specs = self.root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (self.root / "AGENTS.md").write_text("# agents\n")
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        marker = self.root / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        marker.parent.mkdir(parents=True)
        marker.write_text("ready\n")
        self.home = isolated_home(base / "cli-home-source")
        prepared = invoke(self.root, "refresh-bundled", cli_home=self.home)
        self.assertEqual(prepared.returncode, 0, prepared.stderr)

    def _change(self, name: str, proposal: str, tasks: str | None = None) -> None:
        change = self.root / "openspec" / "changes" / name
        change.mkdir(parents=True, exist_ok=True)
        (change / "proposal.md").write_text(proposal)
        if tasks is not None:
            (change / "tasks.md").write_text(tasks)

    def _doctor(self) -> tuple[int, list[str], dict]:
        before = snapshot(self.root)
        result = invoke(self.root, "doctor", cli_home=self.home)
        after = snapshot(self.root)
        tracker = [line for line in result.stdout.splitlines() if "tracker-card" in line]
        return result.returncode, tracker, {"created": sorted(set(after) - set(before)), "deleted": sorted(set(before) - set(after))}

    def test_bold_key_list_form_lowercases_keys(self):
        self._change(
            "bold",
            "## Tracker\n\n"
            "- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"
            "- **shortLink**: `5UIKk5jp`\n"
            "- **url**: https://trello.com/c/5UIKk5jp/48-demo\n",
        )
        rc, tracker, diff = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(tracker)
        self.assertIn("all active changes", tracker[-1])
        self.assertEqual(diff["created"], [])
        self.assertEqual(diff["deleted"], [])

    def test_plain_key_value_form(self):
        self._change(
            "plain",
            "## Tracker\n\ncard_id: abc123\nurl: https://example.test/c/x\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("non-canonical" in line for line in tracker))
        self.assertFalse(any("WARN" in line for line in tracker))

    def test_backticks_and_trailing_hash_comment_stripped(self):
        self._change(
            "comments",
            "## Tracker\n\n"
            "- **card_id**: `6a622e6ad8dd4cefb8c09b81` # comment\n"
            "- **url**: `https://trello.com/c/x` # trailing\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertEqual(len(tracker), 1)
        self.assertIn("OK", tracker[0])

    def test_duplicate_keys_first_wins(self):
        self._change(
            "duplicate",
            "## Tracker\n\n"
            "- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"
            "- **card_id**: `not-a-card`\n"
            "- **url**: https://a.test\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertEqual(len(tracker), 1)
        self.assertIn("OK", tracker[0])

    def test_headings_unknown_keys_blank_lines_ignored(self):
        self._change(
            "nested",
            "## Tracker\n\n### Nested\n\n"
            "- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"
            "- **unknown_key**: skip-me\n"
            "- **url**: https://ok.test\n"
            "not a pair line\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertEqual(len(tracker), 1)
        self.assertIn("OK", tracker[0])

    def test_missing_file_returns_empty_invalid(self):
        self._change("missing", "# change\n")
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("WARN" in line and "missing" in line for line in tracker))

    def test_no_tracker_section_invalid(self):
        self._change("no-section", "# Title\n\n## Scope\n\n- **card_id**: `x`\n")
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("WARN" in line and "no-section" in line for line in tracker))

    def test_empty_card_id_invalid_nonempty_valid_without_url(self):
        self._change(
            "empty",
            "## Tracker\n\n- **card_id**: ``\n- **url**: https://x.test\n",
        )
        self._change("no-url", "## Tracker\n\n- **card_id**: `present`\n")
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("WARN" in line and "empty" in line for line in tracker))
        self.assertTrue(any("missing url" in line and "no-url" in line for line in tracker))

    def test_proposal_wins_over_tasks_fallback(self):
        self._change(
            "precedence",
            "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n",
            "## Tracker\n\n- **card_id**: ``\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertEqual(len(tracker), 2)
        self.assertIn("missing url", tracker[0])
        self.assertIn("OK", tracker[-1])

    def test_tasks_fallback_when_proposal_has_no_section(self):
        self._change(
            "fallback",
            "# No tracker here\n",
            "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("OK" in line for line in tracker))
        self.assertFalse(any("WARN" in line for line in tracker))

    def test_section_ends_at_next_h2(self):
        self._change(
            "section-end",
            "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n\n"
            "## Next\n\n- **card_id**: `outside`\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("OK" in line for line in tracker))
        self.assertFalse(any("WARN" in line for line in tracker))

    def test_card_id_looks_canonical_24_hex_only(self):
        for card_id in ("6a622e6ad8dd4cefb8c09b81", "6A622E6AD8DD4CEFB8C09B81"):
            self._change(card_id[:4], f"## Tracker\n\n- **card_id**: `{card_id}`\n")
        for card_id in ("5UIKk5jp", "", "6a622e6ad8dd4cefb8c09b8", "6a622e6ad8dd4cefb8c09b811"):
            value = f"`{card_id}`" if card_id else "``"
            self._change(f"bad-{len(card_id)}", f"## Tracker\n\n- **card_id**: {value}\n")
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertEqual(sum("non-canonical" in line for line in tracker), 3)
        self.assertTrue(any("WARN" in line and "bad-0" in line for line in tracker))
        self.assertFalse(any("WARN" in line and "bad-8" in line for line in tracker))
        self.assertFalse(any("WARN" in line and "bad-23" in line for line in tracker))
        self.assertFalse(any("WARN" in line and "bad-24" in line for line in tracker))
        self.assertFalse(any("WARN" in line for line in tracker if "bad-0" not in line))

    def test_fenced_sample_tracker_ignored(self):
        self._change(
            "fenced",
            "# Title\n\nSample:\n\n```markdown\n"
            "## Tracker\n\n- **card_id**: `<24-hex>`\n```\n\n"
            "## Scope\n\nNo real tracker yet.\n",
        )
        rc, tracker, _ = self._doctor()
        self.assertEqual(rc, 0)
        self.assertTrue(any("WARN" in line and "fenced" in line for line in tracker))


if __name__ == "__main__":
    unittest.main()
