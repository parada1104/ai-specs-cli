"""Doctor WARN for active changes missing a ## Tracker link section."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class DoctorTrackerCardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doctor_mod = load_module(DOCTOR_PY, "doctor_tracker_card_under_test")

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

    def _run(self, root: Path):
        """Run only the tracker-card check (isolate from bundled-asset ERRORs)."""
        doc = self.doctor_mod.Doctor(root)
        doc._check_tracker_card_link()
        # Doctor exit is ERROR-only; WARN must keep exit 0.
        rc = 1 if any(
            c.severity == self.doctor_mod.Severity.ERROR for c in doc.checks
        ) else 0
        return rc, doc.checks

    def _tracker_checks(self, checks):
        return [c for c in checks if c.name == "tracker-card"]

    def test_missing_tracker_warns_exit_zero(self):
        root = self._project(changes=[("no-card", {})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(tc)
        self.assertEqual(tc[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn("no-card", tc[0].message)

    def test_valid_tracker_ok(self):
        root = self._project(changes=[("good", {"tracker": True})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(tc)
        self.assertEqual(tc[0].severity, self.doctor_mod.Severity.OK)

    def test_valid_card_without_url_emits_one_info_and_terminal_ok(self):
        root = self._project(changes=[("no-url", {"tracker": True, "url": False})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertEqual(sum(c.severity == self.doctor_mod.Severity.INFO for c in tc), 1)
        self.assertIn("no-url", tc[0].message)
        self.assertEqual(tc[-1].severity, self.doctor_mod.Severity.OK)

    def test_noncanonical_card_id_emits_info_without_warn(self):
        root = self._project(changes=[("short-id", {"tracker": True, "card_id": "short"})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(any(c.severity == self.doctor_mod.Severity.INFO and "non-canonical" in c.message for c in tc))
        self.assertFalse(any(c.severity == self.doctor_mod.Severity.WARN for c in tc))
    def test_tracker_none_no_missing_warn(self):
        root = self._project(changes=[("exempt", {"tracker_none": True})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(tc)
        self.assertEqual(tc[0].severity, self.doctor_mod.Severity.OK)

    def test_recipe_disabled_silent(self):
        root = self._project(recipe_enabled=False, changes=[("no-card", {})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        self.assertEqual(self._tracker_checks(checks), [])

    def test_marker_absent_silent(self):
        root = self._project(marker=False, changes=[("no-card", {})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        self.assertEqual(self._tracker_checks(checks), [])

    def test_archive_only_ignored(self):
        root = self._project(changes=[("old", {"archive": True})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(tc)
        self.assertEqual(tc[0].severity, self.doctor_mod.Severity.OK)

    def test_empty_card_id_warns(self):
        root = self._project(changes=[("bad", {"empty_card": True})])
        rc, checks = self._run(root)
        self.assertEqual(rc, 0)
        tc = self._tracker_checks(checks)
        self.assertTrue(tc)
        self.assertEqual(tc[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn("bad", tc[0].message)

    def test_doctor_is_read_only(self):
        root = self._project(changes=[("no-card", {})])

        def walk(base: Path):
            return sorted(
                str(p.relative_to(base))
                for p in base.rglob("*")
                if p.is_file()
            )

        before = walk(root)
        self._run(root)
        after = walk(root)
        self.assertEqual(before, after)


if __name__ == "__main__":
    unittest.main()
