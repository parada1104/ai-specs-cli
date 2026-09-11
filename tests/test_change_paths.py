"""Tests for tests/_change_paths.py: archive-aware change-artifact resolution.

A guard that reads a real change artifact from ``openspec/changes/<slug>/`` breaks the
moment that change is archived. These tests pin the resolver that keeps such guards
working after archive, and they invoke the real guard so its protection cannot be
removed without turning a test red.

Judgment Day (two blind judges, informational rows) flagged the earlier version of this
module for asserting the guard's *rule* against content the test had authored itself,
which stayed green even if the guard's own assertion was deleted. The pair
``test_guard_rejects_absolute_paths_in_a_resolved_artifact`` /
``test_guard_accepts_a_clean_resolved_artifact`` replaces that with a real invocation.
"""

from __future__ import annotations

import importlib.util
import re
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _change_paths import change_artifact, change_dir

ROOT = Path(__file__).resolve().parents[1]
SLUG = "bitbucket-bb-cli-alignment"
GUARD_TEST_NAME = "test_apply_progress_omits_absolute_host_and_worktree_paths"
ABSOLUTE_PATH_RE = r"(?m)(/Users/|/home/|/opt/homebrew/)"
DIRTY_ARTIFACT = "worktree: /Users/someone/dev\n"
CLEAN_ARTIFACT = "worktree: .worktrees/bitbucket-golden-archive-aware\n"

BITBUCKET_TEST_PATH = Path(__file__).resolve().parent / "test_bitbucket_pr_flow_recipe.py"


def _load_guard_module():
    """Load the bitbucket test module so its real guard can be invoked."""
    name = "bitbucket_guard_under_test"
    if name in sys.modules:
        return sys.modules[name]
    spec = importlib.util.spec_from_file_location(name, BITBUCKET_TEST_PATH)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class ChangePathsTests(unittest.TestCase):
    def _root_with(self, root: Path, *layouts: str) -> Path:
        (root / "openspec" / "changes").mkdir(parents=True, exist_ok=True)
        for layout in layouts:
            (root / layout).mkdir(parents=True, exist_ok=True)
        return root

    def _write_artifact(self, root: Path, layout: str, content: str) -> Path:
        target = root / layout / "apply-progress.md"
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
        return target

    def test_resolves_active_change_folder(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(Path(tmp), f"openspec/changes/{SLUG}")
            expected = root / "openspec" / "changes" / SLUG
            self.assertEqual(change_dir(root, SLUG), expected)
            self.assertEqual(
                change_artifact(root, SLUG, "apply-progress.md"),
                expected / "apply-progress.md",
            )

    def test_resolves_dated_archive_entry(self):
        """The archived form is `archive/<ISO-date>-<slug>/`."""
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(Path(tmp), f"openspec/changes/archive/2026-09-07-{SLUG}")
            self.assertEqual(
                change_dir(root, SLUG),
                root / "openspec" / "changes" / "archive" / f"2026-09-07-{SLUG}",
            )

    def test_resolves_legacy_undated_archive_entry(self):
        """The pre-date archive form `archive/<slug>/` stays supported."""
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(Path(tmp), f"openspec/changes/archive/{SLUG}")
            self.assertEqual(
                change_dir(root, SLUG),
                root / "openspec" / "changes" / "archive" / SLUG,
            )

    def test_prefers_active_over_archived(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(
                Path(tmp),
                f"openspec/changes/{SLUG}",
                f"openspec/changes/archive/2026-09-07-{SLUG}",
            )
            self.assertEqual(change_dir(root, SLUG), root / "openspec" / "changes" / SLUG)

    def test_newest_dated_archive_entry_wins(self):
        """With several archives of one slug the most recent date must be chosen."""
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(
                Path(tmp),
                f"openspec/changes/archive/2026-01-05-{SLUG}",
                f"openspec/changes/archive/2026-09-07-{SLUG}",
                f"openspec/changes/archive/2025-12-31-{SLUG}",
            )
            self.assertEqual(
                change_dir(root, SLUG),
                root / "openspec" / "changes" / "archive" / f"2026-09-07-{SLUG}",
            )

    def test_invalid_calendar_date_prefix_is_ignored(self):
        """A shape-matching but impossible date must not outrank a real archive entry."""
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(
                Path(tmp),
                f"openspec/changes/archive/2026-09-07-{SLUG}",
                f"openspec/changes/archive/2026-99-99-{SLUG}",
            )
            self.assertEqual(
                change_dir(root, SLUG),
                root / "openspec" / "changes" / "archive" / f"2026-09-07-{SLUG}",
            )

    def test_falls_back_to_active_shaped_path_when_absent(self):
        """An unknown slug yields a usable path so callers keep an informative failure."""
        with tempfile.TemporaryDirectory() as tmp:
            root = self._root_with(Path(tmp))
            self.assertEqual(change_dir(root, SLUG), root / "openspec" / "changes" / SLUG)
            self.assertFalse(change_artifact(root, SLUG, "apply-progress.md").exists())

    def test_missing_openspec_root_is_not_an_error(self):
        """The documented no-raise contract covers a root without openspec/ at all."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.assertEqual(change_dir(root, SLUG), root / "openspec" / "changes" / SLUG)

    def test_missing_changes_root_is_not_an_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "openspec").mkdir()
            self.assertEqual(change_dir(root, SLUG), root / "openspec" / "changes" / SLUG)

    def test_guard_rejects_absolute_paths_in_a_resolved_artifact(self):
        """The guard's own assertion, invoked for real, must fail on a dirty artifact.

        Deleting the guard's ``assertIsNone`` turns this test red, so the guard has a
        regression tripwire instead of relying on the resolver's tests to imply it.
        """
        module = _load_guard_module()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._write_artifact(
                root, f"openspec/changes/archive/2026-09-07-{SLUG}", DIRTY_ARTIFACT
            )
            guard = module.BitbucketPrFlowGoldenContentTests(GUARD_TEST_NAME)
            with mock.patch.object(module, "ROOT", root):
                with self.assertRaises(AssertionError):
                    guard.test_apply_progress_omits_absolute_host_and_worktree_paths()

    def test_guard_accepts_a_clean_resolved_artifact(self):
        """The same invocation must pass on a clean artifact, so the pair is not one-sided."""
        module = _load_guard_module()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self._write_artifact(
                root, f"openspec/changes/archive/2026-09-07-{SLUG}", CLEAN_ARTIFACT
            )
            guard = module.BitbucketPrFlowGoldenContentTests(GUARD_TEST_NAME)
            with mock.patch.object(module, "ROOT", root):
                guard.test_apply_progress_omits_absolute_host_and_worktree_paths()

    def test_real_bitbucket_guard_target_resolves_and_is_clean(self):
        """The bitbucket guard's real target resolves to an existing, clean artifact.

        This intentionally does not assert *where* it resolved: that would couple the
        test to live repository bookkeeping, so restoring the change folder (a
        permitted operation) would redden the suite for everyone.
        """
        target = change_artifact(ROOT, SLUG, "apply-progress.md")
        self.assertTrue(target.is_file(), f"missing {target}")
        self.assertIsNone(re.search(ABSOLUTE_PATH_RE, target.read_text()))


if __name__ == "__main__":
    unittest.main()
