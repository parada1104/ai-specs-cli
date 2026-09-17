"""RED/GREEN tests for the ledger evidence bridge (``lib/_internal/ledger_bridge.py``).

The bridge is thin acquisition/JSON only (foundation A8/D5): it builds the
``--evidence`` file's three produced sides from local facts and never grades, never
runs ``gh``, never touches MCP, and never reaches the network. These tests pin:

  * the four evidence keys with ``local``/``remote`` deliberately unwired;
  * ``code`` = the change's ``## Tracker`` ``card_id``, ``git`` = the same id only
    when ``pr:`` is recorded;
  * ``tracker.none`` blanks the code side and yields its exemption reason;
  * a missing/malformed artifact fails open with empty sides and no exception;
  * ``recipe_id`` reads the durable witness with the legacy literal as fallback;
  * the change slug resolves active-then-archived;
  * no network/MCP/``gh`` surface exists in the module.
"""
from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BRIDGE_PY = ROOT / "lib" / "_internal" / "ledger_bridge.py"
LEGACY_RECIPE = "trello-mcp-workflow"

TRACKER_SECTION = (
    "## Tracker\n\n"
    "- **card_id**: `6aa703fdcf61a90ec702d58b`\n"
    "- **url**: https://trello.com/c/ie4mQykZ/127-feature\n"
)
TRACKER_WITH_PR = TRACKER_SECTION + "- **pr**: https://github.com/o/r/pull/128\n"


def git(cwd: Path, *args: str) -> str:
    return subprocess.run(
        ["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True
    ).stdout.strip()


def load_bridge():
    if not BRIDGE_PY.is_file():
        raise AssertionError(f"bridge module missing: {BRIDGE_PY}")
    spec = importlib.util.spec_from_file_location("ledger_bridge_under_test", BRIDGE_PY)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class LedgerBridgeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.bridge = load_bridge()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "repo"
        self.root.mkdir()
        git(self.root, "init", "-q")
        git(self.root, "config", "user.email", "t@t.t")
        git(self.root, "config", "user.name", "t")

    def _change(self, slug: str, proposal: str | None = TRACKER_SECTION) -> Path:
        folder = self.root / "openspec" / "changes" / slug
        folder.mkdir(parents=True, exist_ok=True)
        if proposal is not None:
            (folder / "proposal.md").write_text(proposal, encoding="utf-8")
        return folder

    def _witness(self, payload: object) -> Path:
        common = Path(git(self.root, "rev-parse", "--path-format=absolute", "--git-common-dir"))
        ledger_dir = common / "ai-specs" / "ledger"
        ledger_dir.mkdir(parents=True, exist_ok=True)
        path = ledger_dir / "witness.json"
        path.write_text(
            payload if isinstance(payload, str) else json.dumps(payload), encoding="utf-8"
        )
        return path

    # --- evidence payload ---

    def test_evidence_payload_has_four_keys_with_local_and_remote_unwired(self):
        self._change("demo")
        payload = self.bridge.evidence_payload(self.root, "demo")
        self.assertEqual(
            sorted(payload), ["code", "git", "local", "remote"], "exactly the four model sides"
        )
        self.assertEqual(payload["local"], "", "local is the ledger's own snapshot, never the bridge's")
        self.assertEqual(payload["remote"], "", "remote has no producer in this slice (L3)")

    def test_code_side_is_card_id_and_git_side_needs_pr(self):
        self._change("demo", TRACKER_SECTION)
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "demo"),
            {"local": "", "remote": "", "code": "6aa703fdcf61a90ec702d58b", "git": ""},
        )
        self._change("with-pr", TRACKER_WITH_PR)
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "with-pr")["git"],
            "6aa703fdcf61a90ec702d58b",
            "a recorded pr: makes the git side the same native id",
        )

    def test_tracker_none_blanks_the_code_side_and_reports_its_reason(self):
        folder = self._change("none-change", TRACKER_SECTION)
        (folder / "tracker.none").write_text(
            "\nno tracker card for this spike\nsecond line\n", encoding="utf-8"
        )
        self.assertEqual(
            self.bridge.tracker_none_reason(self.root, "none-change"),
            "no tracker card for this spike",
            "the first non-empty line is the persisted reason",
        )
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "none-change"),
            {"local": "", "remote": "", "code": "", "git": ""},
            "an exempt change carries no code side, so it cannot conflict",
        )

    def test_tracker_none_without_text_uses_the_literal_reason(self):
        folder = self._change("blank-none")
        (folder / "tracker.none").write_text("\n\n", encoding="utf-8")
        self.assertEqual(self.bridge.tracker_none_reason(self.root, "blank-none"), "tracker.none")

    def test_tracker_none_absent_or_unreadable_is_none(self):
        self._change("plain")
        self.assertIsNone(self.bridge.tracker_none_reason(self.root, "plain"))
        self.assertIsNone(self.bridge.tracker_none_reason(self.root, "does-not-exist"))
        self.assertIsNone(self.bridge.tracker_none_reason(self.root, None))
        # A directory where the file is expected is unreadable, not a reason.
        (self._change("weird") / "tracker.none").mkdir()
        self.assertIsNone(self.bridge.tracker_none_reason(self.root, "weird"))

    def test_missing_or_malformed_artifact_fails_open(self):
        empty = {"local": "", "remote": "", "code": "", "git": ""}
        self.assertEqual(self.bridge.evidence_payload(self.root, "absent"), empty)
        self.assertEqual(self.bridge.evidence_payload(self.root, None), empty)
        self._change("no-section", "# proposal\n\nnothing here\n")
        self.assertEqual(self.bridge.evidence_payload(self.root, "no-section"), empty)
        # A malformed section (no card_id) is empty, not an exception.
        self._change("malformed", "## Tracker\n\n- **url**: https://trello.com/c/x\n")
        self.assertEqual(self.bridge.evidence_payload(self.root, "malformed"), empty)
        # An unreadable artifact must not raise either: a directory named
        # proposal.md is unreadable as text.
        folder = self._change("dir-artifact", None)
        (folder / "proposal.md").mkdir()
        self.assertEqual(self.bridge.evidence_payload(self.root, "dir-artifact"), empty)

    def test_archived_change_still_provides_the_code_side(self):
        archived = (
            self.root / "openspec" / "changes" / "archive" / "2026-09-13-done-change"
        )
        archived.mkdir(parents=True)
        (archived / "proposal.md").write_text(TRACKER_SECTION, encoding="utf-8")
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "done-change")["code"],
            "6aa703fdcf61a90ec702d58b",
            "pre-merge grades an archived change, so the code side must resolve there",
        )

    # --- change slug resolution ---

    def test_change_slug_resolves_the_single_active_change(self):
        self._change("only-change")
        self.assertEqual(self.bridge.change_slug(self.root), "only-change")
        self._change("second-change")
        self.assertEqual(
            self.bridge.change_slug(self.root), "",
            "two active changes are ambiguous, so the host must not guess one",
        )

    def test_change_slug_is_empty_without_a_planning_tree(self):
        self.assertEqual(self.bridge.change_slug(self.root), "")

    # --- R3: an unvalidated slug may never borrow an artifact outside the tree ---

    def _outside_change(self, name: str = "outside") -> Path:
        """A change-shaped folder OUTSIDE the planning tree, exemption included."""
        outside = Path(self.tmp.name) / name
        outside.mkdir(parents=True, exist_ok=True)
        (outside / "proposal.md").write_text(TRACKER_SECTION, encoding="utf-8")
        (outside / "tracker.none").write_text("outside exemption\n", encoding="utf-8")
        return outside

    def test_invalid_slugs_are_refused_before_any_path_is_built(self):
        self._change("demo")
        for slug in (None, "", ".", "..", "../outside", "../../../outside",
                     "/etc", "demo/sub", "demo\\sub", " demo", "demo\n"):
            with self.subTest(slug=slug):
                self.assertIsNone(
                    self.bridge.change_dir(self.root, slug),
                    "only one plain relative segment may be joined to the change root",
                )
                self.assertIsNone(self.bridge.tracker_none_reason(self.root, slug))
                self.assertEqual(
                    self.bridge.evidence_payload(self.root, slug),
                    {"local": "", "remote": "", "code": "", "git": ""},
                )

    def test_traversal_and_absolute_slugs_cannot_borrow_an_outside_change(self):
        outside = self._outside_change()
        self._change("demo")
        for slug in ("../../../outside", str(outside)):
            with self.subTest(slug=slug):
                self.assertIsNone(
                    self.bridge.tracker_none_reason(self.root, slug),
                    "an outside tracker.none is not this repository's exemption",
                )
                self.assertEqual(
                    self.bridge.evidence_payload(self.root, slug)["code"], "",
                    "an outside ## Tracker card_id must never become the code side",
                )

    def test_symlinked_change_escaping_the_tree_is_refused(self):
        empty = {"local": "", "remote": "", "code": "", "git": ""}
        outside = self._outside_change()
        changes = self.root / "openspec" / "changes"
        archive = changes / "archive"
        archive.mkdir(parents=True, exist_ok=True)
        (changes / "evil").symlink_to(outside)
        (archive / "2026-01-02-dated-evil").symlink_to(outside)
        for slug in ("evil", "dated-evil"):
            with self.subTest(slug=slug):
                self.assertIsNone(
                    self.bridge.change_dir(self.root, slug),
                    "a symlink out of the planning tree is not a change folder",
                )
                self.assertIsNone(self.bridge.tracker_none_reason(self.root, slug))
                self.assertEqual(self.bridge.evidence_payload(self.root, slug), empty)

    def test_symlink_inside_the_planning_tree_still_resolves(self):
        real = self._change("real-change")
        (self.root / "openspec" / "changes" / "alias-change").symlink_to(real)
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "alias-change")["code"],
            "6aa703fdcf61a90ec702d58b",
            "an in-tree symlink stays a legitimate change folder",
        )

    def test_an_active_change_wins_over_an_escaping_archive_link(self):
        real = self._change("demo-change")
        outside = self._outside_change()
        archive = self.root / "openspec" / "changes" / "archive"
        archive.mkdir(parents=True, exist_ok=True)
        (archive / "2026-01-02-demo-change").symlink_to(outside)
        self.assertEqual(
            self.bridge.change_dir(self.root, "demo-change"), real,
            "a legitimate active change is still resolved",
        )
        self.assertEqual(self.bridge.evidence_payload(self.root, "demo-change")["code"],
                         "6aa703fdcf61a90ec702d58b")

    def test_legacy_undated_archive_still_resolves(self):
        legacy = self.root / "openspec" / "changes" / "archive" / "old-change"
        legacy.mkdir(parents=True)
        (legacy / "proposal.md").write_text(TRACKER_SECTION, encoding="utf-8")
        self.assertEqual(
            self.bridge.evidence_payload(self.root, "old-change")["code"],
            "6aa703fdcf61a90ec702d58b",
            "the undated legacy archive branch must survive slug validation",
        )

    # --- witness recipe id ---

    def test_recipe_id_reads_the_bound_witness(self):
        self._witness({
            "v": 1, "capability": "tracker", "state": "bound",
            "recipe_id": "fixture-tracker", "candidates": [],
        })
        self.assertEqual(self.bridge.recipe_id(self.root), "fixture-tracker")

    def test_recipe_id_falls_back_to_the_legacy_literal(self):
        self.assertEqual(self.bridge.recipe_id(self.root), LEGACY_RECIPE, "missing witness")
        self._witness("{ not json")
        self.assertEqual(self.bridge.recipe_id(self.root), LEGACY_RECIPE, "corrupt witness")
        self._witness({"v": 1, "capability": "tracker", "state": "bound", "recipe_id": ""})
        self.assertEqual(self.bridge.recipe_id(self.root), LEGACY_RECIPE, "no recipe id")
        self._witness({"v": 2, "capability": "tracker", "state": "bound", "recipe_id": "future"})
        self.assertEqual(self.bridge.recipe_id(self.root), LEGACY_RECIPE, "unknown version")

    def test_recipe_id_falls_back_outside_a_git_repository(self):
        outside = Path(self.tmp.name) / "not-a-repo"
        outside.mkdir()
        self.assertEqual(self.bridge.recipe_id(outside), LEGACY_RECIPE)

    # --- acquisition-only contract ---

    def test_bridge_is_acquisition_only_and_offline(self):
        source = BRIDGE_PY.read_text(encoding="utf-8")
        for token in ("socket", "urllib", "http.client", "requests", "xmlrpc"):
            self.assertNotIn(token, source, f"{token} would put the hot path on the network")
        self.assertNotIn('"gh"', source, "the bridge must never compose a gh command")
        self.assertNotIn("'gh'", source, "the bridge must never compose a gh command")
        self.assertIn("parse_tracker_section", source, "the existing pure parser stays the only parser")

    def test_bridge_never_grades(self):
        source = BRIDGE_PY.read_text(encoding="utf-8")
        # The bridge may *describe* the Go grader; it must never invoke it or
        # re-implement its predicate.
        for token in ("is_valid_link", "card_id_looks_canonical", "worktree-gate", "Grade("):
            self.assertNotIn(token, source, f"{token} would make the bridge a second grader")


if __name__ == "__main__":
    unittest.main()
