"""Unit tests for the CHANGELOG parser used by `ai-specs upgrade`.

The parser feeds two surfaces: the version-crossing summary and the
version-keyed upgrade notices. Both read the same sections, so both are
covered here. Every degradation path must return data rather than raise —
the upgrade has already succeeded by the time this parser runs, so a parse
failure must never abort it.
"""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CHANGELOG_PATH = ROOT / "lib" / "_internal" / "changelog.py"


def load_module(path: Path, name: str):
    internal_dir = str(path.parent)
    if internal_dir not in sys.path:
        sys.path.insert(0, internal_dir)
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


SAMPLE = """# Changelog

All notable changes are documented in this file.

## [Unreleased]

## [0.22.0] — 2026-08-17

### Added
- Autocontained Go worktree gate.
- Subrepo planning context propagation.

### Upgrade notes
Run `ai-specs sync` in each project to acquire the verified Go worktree-gate
binary. Until you do, the gate falls back to Bash.

### Fixed
- Gate binary acquisition no longer 404s.

## [0.21.0] — 2026-08-05

### Added
- Topology-aware worktree gate scope.

## [0.20.1] — 2026-08-05

### Fixed
- Remove untouched legacy recipe command copies.

### Upgrade notes
Re-run `ai-specs sync` to drop stale command copies.

## [0.20.0] — 2026-08-05

### Added
- Cross-repo worktree artifact scope.
"""


class ChangelogParseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(CHANGELOG_PATH, "changelog_internal")

    def _write(self, text: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        path = Path(tmp.name) / "CHANGELOG.md"
        path.write_text(text, encoding="utf-8")
        return path

    # --- section extraction -------------------------------------------------

    def test_parses_released_versions_and_skips_unreleased(self):
        sections = self.mod.parse_sections(SAMPLE)
        self.assertEqual(
            [s.version for s in sections],
            ["0.22.0", "0.21.0", "0.20.1", "0.20.0"],
        )

    def test_section_carries_its_date_and_body(self):
        sections = self.mod.parse_sections(SAMPLE)
        latest = sections[0]
        self.assertEqual(latest.date, "2026-08-17")
        self.assertIn("Autocontained Go worktree gate", latest.body)
        # The body stops at the next version heading.
        self.assertNotIn("Topology-aware", latest.body)

    # --- range selection ----------------------------------------------------

    def test_range_is_exclusive_of_current_and_inclusive_of_new(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.20.0", "0.22.0")
        self.assertEqual([s.version for s in crossed], ["0.22.0", "0.21.0", "0.20.1"])

    def test_single_version_step(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.21.0", "0.22.0")
        self.assertEqual([s.version for s in crossed], ["0.22.0"])

    def test_summary_order_is_newest_first(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.20.0", "0.22.0")
        self.assertEqual(crossed[0].version, "0.22.0")
        self.assertEqual(crossed[-1].version, "0.20.1")

    def test_versions_are_ordered_by_semver_not_string(self):
        text = SAMPLE.replace("## [0.21.0]", "## [0.9.0]")
        crossed = self.mod.crossed_versions(text, "0.8.0", "0.22.0")
        versions = [s.version for s in crossed]
        # "0.9.0" sorts after "0.22.0" as a string but before it as semver.
        self.assertEqual(versions, ["0.22.0", "0.20.1", "0.20.0", "0.9.0"])

    def test_same_version_crosses_nothing(self):
        self.assertEqual(self.mod.crossed_versions(SAMPLE, "0.22.0", "0.22.0"), [])

    def test_downgrade_crosses_nothing(self):
        self.assertEqual(self.mod.crossed_versions(SAMPLE, "0.22.0", "0.21.0"), [])

    # --- upgrade notices ----------------------------------------------------

    def test_extracts_upgrade_notice_from_a_section(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.21.0", "0.22.0")
        notice = self.mod.upgrade_notice(crossed[0])
        self.assertIn("ai-specs sync", notice)
        self.assertIn("falls back to Bash", notice)

    def test_notice_stops_at_the_next_subsection(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.21.0", "0.22.0")
        notice = self.mod.upgrade_notice(crossed[0])
        # "### Fixed" follows the notice and must not bleed into it.
        self.assertNotIn("404", notice)
        self.assertNotIn("Fixed", notice)

    def test_section_without_a_notice_returns_none(self):
        crossed = self.mod.crossed_versions(SAMPLE, "0.20.1", "0.21.0")
        self.assertIsNone(self.mod.upgrade_notice(crossed[0]))

    def test_notices_replay_oldest_first(self):
        notices = self.mod.crossed_notices(SAMPLE, "0.20.0", "0.22.0")
        self.assertEqual([version for version, _ in notices], ["0.20.1", "0.22.0"])

    def test_no_notices_in_range_returns_empty(self):
        notices = self.mod.crossed_notices(SAMPLE, "0.20.1", "0.21.0")
        self.assertEqual(notices, [])

    # --- degradation --------------------------------------------------------

    def test_missing_file_returns_empty_without_raising(self):
        missing = Path(tempfile.gettempdir()) / "definitely-absent-CHANGELOG.md"
        self.assertEqual(self.mod.read_sections(missing), [])

    def test_unparseable_text_returns_empty_without_raising(self):
        self.assertEqual(self.mod.parse_sections("no headings at all"), [])

    def test_malformed_heading_is_skipped_not_fatal(self):
        text = "## [not-a-version] — whenever\n\n- x\n\n## [0.22.0] — 2026-08-17\n\n- y\n"
        sections = self.mod.parse_sections(text)
        self.assertEqual([s.version for s in sections], ["0.22.0"])

    def test_unknown_current_version_still_reports_the_target(self):
        # A user on an untagged or hand-edited VERSION must not lose the summary.
        crossed = self.mod.crossed_versions(SAMPLE, "not-a-version", "0.22.0")
        self.assertEqual([s.version for s in crossed], ["0.22.0"])

    def test_heading_without_a_date_is_still_parsed(self):
        text = "## [0.22.0]\n\n### Added\n- x\n"
        sections = self.mod.parse_sections(text)
        self.assertEqual(sections[0].version, "0.22.0")
        self.assertIsNone(sections[0].date)

    # --- summary bullets ----------------------------------------------------

    def test_bullets_come_from_added_and_changed(self):
        sections = self.mod.parse_sections(SAMPLE)
        bullets = self.mod.summary_bullets(sections[0])
        self.assertIn("Autocontained Go worktree gate.", bullets)
        self.assertIn("Subrepo planning context propagation.", bullets)

    def test_bullets_exclude_the_upgrade_notice_prose(self):
        sections = self.mod.parse_sections(SAMPLE)
        bullets = self.mod.summary_bullets(sections[0])
        joined = " ".join(bullets)
        self.assertNotIn("falls back to Bash", joined)

    def test_bullets_are_capped(self):
        body = "### Added\n" + "".join(f"- item {i}.\n" for i in range(10))
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        bullets = self.mod.summary_bullets(section, limit=3)
        self.assertEqual(len(bullets), 3)

    def test_bullet_markup_is_stripped_to_plain_text(self):
        body = "### Added\n- **Bold thing**: does `stuff`.\n"
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        self.assertEqual(self.mod.summary_bullets(section), ["Bold thing: does stuff."])

    def test_multiline_bullet_is_joined(self):
        body = "### Added\n- a thing that wraps\n  onto a second line.\n"
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        self.assertEqual(
            self.mod.summary_bullets(section), ["a thing that wraps onto a second line."]
        )

    def test_remaining_count_reports_what_was_dropped(self):
        body = "### Added\n" + "".join(f"- item {i}.\n" for i in range(10))
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        self.assertEqual(self.mod.remaining_count(section, limit=3), 7)

    def test_remaining_count_is_zero_when_nothing_dropped(self):
        sections = self.mod.parse_sections(SAMPLE)
        self.assertEqual(self.mod.remaining_count(sections[0], limit=50), 0)

    def test_bullet_is_cut_at_the_first_sentence(self):
        body = (
            "### Added\n"
            "- **Thing**: the short claim. Then a long elaboration that a user "
            "scanning an upgrade does not need to read right now.\n"
        )
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        self.assertEqual(self.mod.summary_bullets(section), ["Thing: the short claim."])

    def test_long_bullet_without_a_sentence_break_is_truncated(self):
        body = "### Added\n- " + ("word " * 60) + "\n"
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        bullet = self.mod.summary_bullets(section)[0]
        self.assertLessEqual(len(bullet), 100)
        self.assertTrue(bullet.endswith("…"), msg=bullet)

    def test_truncation_does_not_split_a_word(self):
        body = "### Added\n- " + ("alpha " * 60) + "\n"
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        bullet = self.mod.summary_bullets(section)[0]
        self.assertNotIn("alph…", bullet)

    def test_short_bullet_is_left_alone(self):
        body = "### Added\n- A short one.\n"
        section = self.mod.Section(version="1.0.0", date=None, body=body)
        self.assertEqual(self.mod.summary_bullets(section), ["A short one."])

    def test_real_changelog_bullets_stay_scannable(self):
        real = ROOT / "CHANGELOG.md"
        sections = self.mod.read_sections(real)
        for section in sections:
            for bullet in self.mod.summary_bullets(section):
                self.assertLessEqual(
                    len(bullet), 100, msg=f"{section.version}: {bullet!r}"
                )

    def test_section_without_bullets_returns_empty(self):
        section = self.mod.Section(version="1.0.0", date=None, body="prose only\n")
        self.assertEqual(self.mod.summary_bullets(section), [])

    # --- judgment-day round 1 -----------------------------------------------

    def test_en_dash_separator_is_accepted(self):
        """JD S3: an en dash (U+2013) must not silently drop a whole section."""
        text = "## [0.22.0] – 2026-08-17\n\n### Added\n- a thing.\n"
        sections = self.mod.parse_sections(text)
        self.assertEqual([s.version for s in sections], ["0.22.0"])
        self.assertEqual(sections[0].date, "2026-08-17")

    def test_all_dash_separators_are_accepted(self):
        for dash in ("—", "–", "-"):
            with self.subTest(dash=dash):
                text = f"## [1.2.3] {dash} 2026-01-01\n\n### Added\n- x.\n"
                sections = self.mod.parse_sections(text)
                self.assertEqual([s.version for s in sections], ["1.2.3"])

    def test_duplicate_version_headings_are_collapsed(self):
        """JD S2: the real CHANGELOG has a duplicated 0.12.4 heading."""
        text = (
            "## [1.0.0] — 2026-01-02\n\n### Added\n- second copy.\n\n"
            "## [1.0.0] — 2026-01-02\n\n### Added\n- first copy.\n"
        )
        sections = self.mod.parse_sections(text)
        self.assertEqual([s.version for s in sections], ["1.0.0"])

    def test_duplicate_version_is_rendered_once_in_a_range(self):
        text = (
            "## [1.1.0] — 2026-01-03\n\n### Added\n- newer.\n\n"
            "## [1.0.0] — 2026-01-02\n\n### Added\n- dup a.\n\n"
            "## [1.0.0] — 2026-01-02\n\n### Added\n- dup b.\n"
        )
        crossed = self.mod.crossed_versions(text, "0.9.0", "1.1.0")
        self.assertEqual([s.version for s in crossed], ["1.1.0", "1.0.0"])

    def test_real_changelog_has_no_duplicate_versions_after_parsing(self):
        real = ROOT / "CHANGELOG.md"
        versions = [s.version for s in self.mod.read_sections(real)]
        self.assertEqual(len(versions), len(set(versions)), f"duplicates: {versions}")

    def test_heading_inside_a_code_fence_is_not_a_section_boundary(self):
        """JD S5: a '##' line inside a fence must not truncate the section."""
        text = (
            "## [1.0.0] — 2026-01-01\n\n"
            "### Added\n"
            "- documented a markdown sample.\n\n"
            "```markdown\n"
            "## [9.9.9] — not a real release\n"
            "```\n\n"
            "### Fixed\n"
            "- a real fix.\n"
        )
        sections = self.mod.parse_sections(text)
        self.assertEqual([s.version for s in sections], ["1.0.0"])
        self.assertIn("a real fix", sections[0].body)

    def test_notice_is_not_truncated_by_a_fenced_heading(self):
        text = (
            "## [1.0.0] — 2026-01-01\n\n"
            "### Upgrade notes\n"
            "Run this:\n\n"
            "```sh\n"
            "### not a heading\n"
            "```\n\n"
            "Then you are done.\n\n"
            "### Fixed\n"
            "- unrelated.\n"
        )
        section = self.mod.parse_sections(text)[0]
        notice = self.mod.upgrade_notice(section)
        self.assertIn("Then you are done.", notice)
        self.assertNotIn("unrelated", notice)

    def test_cli_notices_branch_uses_the_shared_helper(self):
        """JD S7: main() must delegate, not re-derive the notice pairs."""
        import inspect

        source = inspect.getsource(self.mod.main)
        self.assertTrue(
            "_notices_for" in source or "crossed_notices" in source,
            msg="main() should call the shared helper",
        )
        self.assertNotIn(
            "upgrade_notice(", source, "main() re-derives notices instead of delegating"
        )

    def test_cli_and_helper_agree(self):
        """The delegation must actually produce the same pairs."""
        sections = self.mod.parse_sections(SAMPLE)
        selected = self.mod.select_range(sections, "0.20.0", "0.22.0")
        self.assertEqual(
            self.mod._notices_for(selected),
            self.mod.crossed_notices(SAMPLE, "0.20.0", "0.22.0"),
        )

    # --- against the real changelog ----------------------------------------

    def test_parses_the_repository_changelog(self):
        real = ROOT / "CHANGELOG.md"
        sections = self.mod.read_sections(real)
        versions = [s.version for s in sections]
        self.assertIn("0.22.0", versions)
        self.assertIn("0.21.0", versions)
        self.assertNotIn("Unreleased", versions)

    def test_real_changelog_crossing_is_ordered(self):
        real = ROOT / "CHANGELOG.md"
        sections = self.mod.read_sections(real)
        crossed = self.mod.select_range(sections, "0.19.0", "0.22.0")
        self.assertEqual(
            [s.version for s in crossed],
            ["0.22.0", "0.21.0", "0.20.1", "0.20.0"],
        )


if __name__ == "__main__":
    unittest.main()
