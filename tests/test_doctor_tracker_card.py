"""Doctor WARN for active changes missing a ## Tracker link section."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke


class DoctorTrackerCardTests(unittest.TestCase):
    def _project(
        self,
        *,
        recipe_enabled: bool = True,
        marker: bool = True,
        changes: list[tuple[str, dict]] | None = None,
    ) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "prj"
        root.mkdir()
        ai = root / "ai-specs"
        ai.mkdir()
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            "[recipes.trello-mcp-workflow]\n"
            f"enabled = {'true' if recipe_enabled else 'false'}\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        (root / "AGENTS.md").write_text("# agents\n")
        if marker:
            m = root / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
            m.parent.mkdir(parents=True)
            m.write_text("ready\n")
        for slug, opts in changes or []:
            d = root / "openspec" / "changes" / slug
            if opts.get("archive"):
                d = root / "openspec" / "changes" / "archive" / slug
            d.mkdir(parents=True, exist_ok=True)
            body = "# proposal\n"
            if opts.get("tracker"):
                body += (
                    "\n## Tracker\n\n"
                    f"- **card_id**: `{opts.get('card_id', '6a622e6ad8dd4cefb8c09b81')}`\n"
                )
                if opts.get("url", True):
                    body += "- **url**: https://trello.com/c/demo\n"
            (d / "proposal.md").write_text(body)
            if opts.get("tracker_none"):
                (d / "tracker.none").write_text("reason\n")
            if opts.get("empty_card"):
                (d / "proposal.md").write_text(
                    "## Tracker\n\n- **card_id**: ``\n- **url**: https://x\n"
                )
        return root

    def _cli_home(self, root: Path):
        """One shared CLI home; refresh-bundled so doctor is not poisoned by an empty cache."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        home = isolated_home(Path(td.name))
        invoke(root, "refresh-bundled", cli_home=home)
        return home

    def _run(self, root: Path):
        """Doctor through bin/ai-specs with one shared CLI home."""
        home = self._cli_home(root)
        result = invoke(root, "doctor", cli_home=home)
        return result.returncode, result.stdout

    def _tracker_checks(self, stdout: str) -> list[str]:
        return [ln for ln in stdout.splitlines() if "tracker-card" in ln]

    def test_missing_tracker_warns_exit_zero(self):
        root = self._project(changes=[("no-card", {})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(tc)
        self.assertIn("WARN", tc[0])
        self.assertIn("no-card", tc[0])

    def test_valid_tracker_ok(self):
        root = self._project(changes=[("good", {"tracker": True})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(tc)
        self.assertIn("OK", tc[0])

    def test_valid_card_without_url_emits_one_info_and_terminal_ok(self):
        root = self._project(changes=[("no-url", {"tracker": True, "url": False})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertEqual(sum("INFO" in ln for ln in tc), 1)
        self.assertIn("no-url", tc[0])
        self.assertIn("OK", tc[-1])

    def test_noncanonical_card_id_emits_info_without_warn(self):
        root = self._project(changes=[("short-id", {"tracker": True, "card_id": "short"})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(any("INFO" in ln and "non-canonical" in ln for ln in tc))
        self.assertFalse(any("WARN" in ln for ln in tc))

    def test_tracker_none_no_missing_warn(self):
        root = self._project(changes=[("exempt", {"tracker_none": True})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(tc)
        self.assertIn("OK", tc[0])

    def test_recipe_disabled_silent(self):
        root = self._project(recipe_enabled=False, changes=[("no-card", {})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        self.assertEqual(self._tracker_checks(stdout), [])

    def test_marker_absent_silent(self):
        root = self._project(marker=False, changes=[("no-card", {})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        self.assertEqual(self._tracker_checks(stdout), [])

    def test_archive_only_ignored(self):
        root = self._project(changes=[("old", {"archive": True})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(tc)
        self.assertIn("OK", tc[0])

    def test_empty_card_id_warns(self):
        root = self._project(changes=[("bad", {"empty_card": True})])
        rc, stdout = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(stdout)
        self.assertTrue(tc)
        self.assertIn("WARN", tc[0])
        self.assertIn("bad", tc[0])

    def test_doctor_is_read_only(self):
        root = self._project(changes=[("no-card", {})])
        home = self._cli_home(root)

        def walk(base: Path):
            return sorted(
                str(p.relative_to(base))
                for p in base.rglob("*")
                if p.is_file()
            )

        before = walk(root)
        invoke(root, "doctor", cli_home=home)
        after = walk(root)
        self.assertEqual(before, after)


if __name__ == "__main__":
    unittest.main()
