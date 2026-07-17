"""RED/GREEN tests for pre-merge archive/artifact guardian."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "premerge_guardian.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class PremergeGuardianTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "premerge_guardian_test")

    def _repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes").mkdir(parents=True)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        return root

    def test_blocks_when_active_change_folder_exists(self):
        root = self._repo()
        slug = "demo-change"
        active = root / "openspec" / "changes" / slug
        active.mkdir()
        (active / "tasks.md").write_text("Depth: standard\n")
        (active / "specs").mkdir()
        (active / "specs" / "x" / "spec.md").parent.mkdir(parents=True)
        (active / "specs" / "x" / "spec.md").write_text("# x\n")

        result = self.mod.check_premerge(root, slug, tier="standard")
        self.assertFalse(result.ok)
        self.assertTrue(any("active" in b.lower() or "archive" in b.lower() for b in result.blockers))

    def test_blocks_when_archive_missing(self):
        root = self._repo()
        result = self.mod.check_premerge(root, "missing-slug", tier="standard")
        self.assertFalse(result.ok)
        self.assertTrue(any("archive" in b.lower() for b in result.blockers))

    def test_blocks_when_archive_missing_tier_files_standard(self):
        root = self._repo()
        slug = "std-change"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: standard\n")
        # no specs/

        result = self.mod.check_premerge(root, slug, tier="standard")
        self.assertFalse(result.ok)
        self.assertTrue(any("spec" in b.lower() for b in result.blockers))

    def test_passes_when_archived_with_standard_minimum(self):
        root = self._repo()
        slug = "ok-std"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: standard\n")
        (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
        (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")

        result = self.mod.check_premerge(root, slug, tier="standard")
        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.blockers, [])

    def test_light_tier_requires_only_tasks(self):
        root = self._repo()
        slug = "ok-light"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n")

        result = self.mod.check_premerge(root, slug, tier="light")
        self.assertTrue(result.ok, result.blockers)

    def test_full_tier_requires_proposal_or_design(self):
        root = self._repo()
        slug = "full-change"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: full\n")
        (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
        (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")

        missing = self.mod.check_premerge(root, slug, tier="full")
        self.assertFalse(missing.ok)

        (archived / "design.md").write_text("# design\n")
        ok = self.mod.check_premerge(root, slug, tier="full")
        self.assertTrue(ok.ok, ok.blockers)

    def test_infer_tier_from_tasks_depth_line(self):
        root = self._repo()
        slug = "infer"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n- [x] done\n")

        result = self.mod.check_premerge(root, slug, tier=None)
        self.assertTrue(result.ok, result.blockers)

    def test_omitted_tier_defaults_to_inference_not_standard(self):
        """Omitting tier must infer Depth from tasks.md (light → no specs required)."""
        root = self._repo()
        slug = "omit-tier"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n- [x] done\n")

        result = self.mod.check_premerge(root, slug)  # no tier kwarg
        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.tier, "light")


if __name__ == "__main__":
    unittest.main()
