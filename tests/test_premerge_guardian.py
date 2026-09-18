"""RED/GREEN tests for pre-merge archive/artifact guardian."""

from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "premerge_guardian.py"

# The preflight-resolved store (config artifact_store_default) must never change
# a guardian verdict. STORE_ENV_KEY is a test-only fixture naming the env a
# store-aware preflight would set; the guardian is store-blind and reads only
# the filesystem change tree, so every context must yield the baseline verdict.
STORE_ENUM = ["openspec", "engram", "both"]
STORE_ENV_KEY = "PLAN_BUILD_ARTIFACT_STORE"


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
        (archived / "proposal.md").write_text("# proposal\n")
        (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
        (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")
        (archived / "verify-report.md").write_text(
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
        )

        result = self.mod.check_premerge(root, slug, tier="standard")
        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.blockers, [])

    def test_light_tier_requires_proposal(self):
        root = self._repo()
        slug = "ok-light"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n")
        (archived / "proposal.md").write_text("# proposal\n")

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

        (archived / "design.md").write_text("# design\n\n## Success Criteria\n- [ ] Criterion fixture\n")
        (archived / "verify-report.md").write_text(
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n## Success-criteria mapping\n"
            "- Criterion 1: PASS — covered\n"
        )
        ok = self.mod.check_premerge(root, slug, tier="full")
        self.assertTrue(ok.ok, ok.blockers)

    def test_infer_tier_from_tasks_depth_line(self):
        root = self._repo()
        slug = "infer"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n- [x] done\n")
        (archived / "proposal.md").write_text("# proposal\n")

        result = self.mod.check_premerge(root, slug, tier=None)
        self.assertTrue(result.ok, result.blockers)

    def test_omitted_tier_defaults_to_inference_not_standard(self):
        """Omitting tier infers Depth from tasks.md (light needs proposal only)."""
        root = self._repo()
        slug = "omit-tier"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n- [x] done\n")
        (archived / "proposal.md").write_text("# proposal\n")

        result = self.mod.check_premerge(root, slug)
        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.tier, "light")

    def _archive_with(self, slug: str, tasks: str, *, proposal: bool = False,
                      design: bool = False, spec: bool = False,
                      report: str | None = None, root: Path | None = None,
                      archive_name: str | None = None) -> Path:
        root = root or self._repo()
        archive_name = archive_name or slug
        archived = root / "openspec" / "changes" / "archive" / archive_name
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text(tasks)
        if proposal:
            (archived / "proposal.md").write_text(
                "# proposal\n\n## Success Criteria\n- [ ] Criterion fixture\n"
            )
        if design:
            (archived / "design.md").write_text("# design\n")
        if spec:
            (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
            (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")
        if report is not None:
            (archived / "verify-report.md").write_text(report)
        return archived

    def test_passes_when_archived_with_canonical_dated_openspec_name(self):
        root = self._repo()
        slug = "dated-change"
        archived = self._archive_with(
            slug,
            "Depth: standard\n",
            proposal=True,
            spec=True,
            report=(
                "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
                "- Exit: 0\n- Date: 2026-08-16\n- Commit: 1234567\n"
            ),
            root=root,
            archive_name="2026-08-16-" + slug,
        )

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.archive_path, archived)

    def test_undated_archive_remains_legacy_compatible(self):
        root = self._repo()
        slug = "legacy-change"
        archived = self._archive_with(
            slug,
            "Depth: light\n",
            proposal=True,
            root=root,
        )

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.archive_path, archived)

    def test_multiple_dated_archives_block_as_ambiguous(self):
        root = self._repo()
        slug = "ambiguous-change"
        for archive_name in ("2026-08-15-" + slug, "2026-08-16-" + slug):
            self._archive_with(slug, "Depth: light\n", proposal=True, root=root, archive_name=archive_name)

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("ambiguous", joined)
        self.assertIn("2026-08-15-" + slug, joined)
        self.assertIn("2026-08-16-" + slug, joined)

    def test_dated_and_undated_archives_block_as_ambiguous(self):
        root = self._repo()
        slug = "mixed-change"
        self._archive_with(slug, "Depth: light\n", proposal=True, root=root)
        self._archive_with(
            slug,
            "Depth: light\n",
            proposal=True,
            root=root,
            archive_name="2026-08-16-" + slug,
        )

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        self.assertTrue(any("ambiguous" in blocker.lower() for blocker in result.blockers))

    def test_dated_archive_requires_valid_calendar_date(self):
        root = self._repo()
        slug = "invalid-date-change"
        self._archive_with(
            slug,
            "Depth: light\n",
            proposal=True,
            root=root,
            archive_name="2026-02-30-" + slug,
        )

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        self.assertTrue(any("archive" in blocker.lower() for blocker in result.blockers))

    def test_dated_near_match_is_rejected(self):
        root = self._repo()
        slug = "near-match-change"
        candidate = self._archive_with(
            slug,
            "Depth: light\n",
            proposal=True,
            root=root,
            archive_name="2026-08-16-" + slug + "-extra",
        )

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("near-match", joined)
        self.assertIn(candidate.name, joined)

    def test_blocks_when_dated_archive_is_symlink_to_external_dir(self):
        """A dated archive symlink must not let external files satisfy gates."""
        root = self._repo()
        slug = "symlink-change"
        # A well-formed archive living OUTSIDE the planning tree.
        external = root / "outside-planning-tree"
        external.mkdir()
        (external / "tasks.md").write_text("Depth: light\n")
        (external / "proposal.md").write_text("# proposal\n")

        archive = root / "openspec" / "changes" / "archive"
        link = archive / ("2026-08-16-" + slug)
        os.symlink(external, link)

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("symlink", joined)
        self.assertIn(slug, joined)

    def test_blocks_when_legacy_archive_is_symlink_to_external_dir(self):
        """The undated legacy archive must also reject a symlinked directory."""
        root = self._repo()
        slug = "legacy-symlink-change"
        external = root / "outside-planning-tree"
        external.mkdir()
        (external / "tasks.md").write_text("Depth: light\n")
        (external / "proposal.md").write_text("# proposal\n")

        archive = root / "openspec" / "changes" / "archive"
        link = archive / slug
        os.symlink(external, link)

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("symlink", joined)

    def test_unrelated_dated_symlink_does_not_poison_requested_slug(self):
        """A dated symlink for a different slug must not block the requested slug.

        The guardian evaluates only the requested slug: an unrelated date-shaped
        symlink to external content must not poison resolution of a slug that has
        a valid real dated archive of its own.
        """
        root = self._repo()
        slug = "target-change"
        other = "other-change"
        archived = self._archive_with(
            slug,
            "Depth: light\n",
            proposal=True,
            root=root,
            archive_name="2026-08-16-" + slug,
        )

        external = root / "outside-planning-tree"
        external.mkdir()
        (external / "tasks.md").write_text("Depth: light\n")
        (external / "proposal.md").write_text("# proposal\n")

        archive = root / "openspec" / "changes" / "archive"
        os.symlink(external, archive / ("2026-08-16-" + other))

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.archive_path, archived)

    def test_light_minimum_requires_proposal_but_not_verify_evidence(self):
        root = self._repo()
        slug = "light-proposal"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n")

        result = self.mod.check_premerge(root, slug, tier="light")

        self.assertFalse(result.ok)
        self.assertTrue(any("proposal.md" in blocker for blocker in result.blockers))
        self.assertFalse(any("verify" in blocker.lower() for blocker in result.blockers))

    def test_standard_requires_proposal_and_dedicated_verify_report(self):
        root = self._repo()
        slug = "standard-report"
        archived = self._archive_with(slug, "Depth: standard\n", spec=True, root=root)

        missing = self.mod.check_premerge(root, slug, tier="standard")
        self.assertFalse(missing.ok)
        self.assertTrue(any("proposal.md" in blocker for blocker in missing.blockers))
        self.assertTrue(any("verify-report.md" in blocker for blocker in missing.blockers))

        archived.joinpath("proposal.md").write_text("# proposal\n")
        archived.joinpath("tasks.md").write_text(
            "Depth: standard\n\nVerify evidence\nCommand: ./tests/run.sh\nExit: 0\n"
            "Date: 2026-08-07\nCommit: 1234567\nVerdict: PASS\n"
        )
        archived.joinpath("verify-report.md").write_text(
            "## Verify evidence\n\n- Verdict: PASS (all focused checks)\n"
            "- Command: `./tests/run.sh`\n- Exit: 0\n- Date: 2026-08-07\n"
            "- Commit: 1234567\n"
        )
        self.assertTrue(self.mod.check_premerge(root, slug, tier="standard").ok)

    def test_standard_evidence_inside_tasks_does_not_count(self):
        root = self._repo()
        slug = "tasks-evidence"
        archived = self._archive_with(
            slug,
            "Depth: standard\n\n## Verify evidence\n- Verdict: PASS\n"
            "- Command: ./tests/run.sh\n- Exit: 0\n- Date: 2026-08-07\n"
            "- Commit: 1234567\n",
            proposal=True,
            spec=True,
            root=root,
        )

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("verify-report.md" in blocker for blocker in result.blockers))
        self.assertFalse((archived / "verify-report.md").exists())

    def test_standard_report_requires_all_auditable_fields_and_zero_exit(self):
        root = self._repo()
        slug = "bad-report"
        report = "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 1\n"
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("exit", joined)
        self.assertIn("date", joined)
        self.assertIn("commit", joined)
    def test_report_ignores_labels_outside_canonical_evidence_block(self):
        root = self._repo()
        slug = "outside-labels"
        report = (
            "- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 0\n"
            "- Date: 2026-08-07\n- Commit: 1234567\n"
            "## Notes\n"
            "Example labels: Verdict: PASS; Exit: 0; Date: 2026-08-07; Commit: 1234567\n"
            "## Verify evidence\n"
            "Evidence is pending.\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        for field in ("verdict", "command", "exit", "date", "commit"):
            self.assertIn(field, joined)
    def test_report_requires_canonical_evidence_heading(self):
        root = self._repo()
        slug = "missing-canonical-heading"
        report = (
            "- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 0\n"
            "- Date: 2026-08-07\n- Commit: 1234567\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("canonical" in blocker.lower() for blocker in result.blockers))
    def test_report_ignores_fenced_code_block_evidence(self):
        root = self._repo()
        slug = "fenced-evidence"
        report = (
            "```markdown\n"
            "## Verify evidence\n"
            "- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 0\n"
            "- Date: 2026-08-07\n- Commit: 1234567\n"
            "```\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("canonical" in blocker.lower() for blocker in result.blockers))
    def test_report_ignores_indented_code_block_evidence(self):
        root = self._repo()
        slug = "indented-evidence"
        report = (
            "    ## Verify evidence\n"
            "    - Verdict: PASS\n    - Command: ./tests/run.sh\n"
            "    - Exit: 0\n    - Date: 2026-08-07\n"
            "    - Commit: 1234567\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("canonical" in blocker.lower() for blocker in result.blockers))

    def test_report_rejects_duplicate_canonical_labels(self):
        root = self._repo()
        slug = "duplicate-labels"
        report = (
            "## Verify evidence\n"
            "- Verdict: PASS\n- Status: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("duplicate" in blocker.lower() for blocker in result.blockers))

    def test_report_rejects_impossible_calendar_date(self):
        root = self._repo()
        slug = "impossible-date"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-99-99\n- Commit: 1234567\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("date" in blocker.lower() for blocker in result.blockers))

    def test_nested_heading_cannot_supply_evidence_fields(self):
        root = self._repo()
        slug = "nested-evidence"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "### Notes\n- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
        )
        self._archive_with(slug, "Depth: standard\n", proposal=True, spec=True, report=report, root=root)

        result = self.mod.check_premerge(root, slug, tier="standard")

        self.assertFalse(result.ok)
        joined = " ".join(result.blockers).lower()
        self.assertIn("exit", joined)
        self.assertIn("date", joined)
        self.assertIn("commit", joined)

    def test_full_requires_strict_pass_and_ready_for_archive(self):
        root = self._repo()
        slug = "full-report"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "\n## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        self._archive_with(
            slug,
            "Depth: full\n",
            proposal=True,
            spec=True,
            report=report,
            root=root,
        )
        missing_ready = self.mod.check_premerge(root, slug, tier="full")
        self.assertFalse(missing_ready.ok)
        self.assertTrue(any("ready_for_archive" in blocker for blocker in missing_ready.blockers))

        archive = root / "openspec" / "changes" / "archive" / slug
        archive.joinpath("verify-report.md").write_text(
            report.replace("\n## Success-criteria mapping", "- ready_for_archive: true\n\n## Success-criteria mapping")
        )

        archive.joinpath("verify-report.md").write_text(
            report.replace("PASS", "WARN") + "- ready_for_archive: true\n"
        )
        failed = self.mod.check_premerge(root, slug, tier="full")
        self.assertFalse(failed.ok)
        self.assertTrue(any("pass" in blocker.lower() for blocker in failed.blockers))
    def test_full_ignores_indented_nested_success_criteria_bullets(self):
        root = self._repo()
        slug = "full-nested-criteria"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        archived = self._archive_with(
            slug,
            "Depth: full\n",
            proposal=True,
            spec=True,
            report=report,
            root=root,
        )
        archived.joinpath("proposal.md").write_text(
            "# proposal\n\n## Success Criteria\n"
            "- [ ] Top-level criterion\n"
            "  - Nested implementation detail\n"
        )

        result = self.mod.check_premerge(root, slug, tier="full")
        self.assertTrue(result.ok, result.blockers)

    def test_full_accepts_top_level_unordered_and_numbered_criteria(self):
        root = self._repo()
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        cases = {
            "dash": "- [ ] Unordered criterion",
            "star": "* [ ] Unordered criterion",
            "plus": "+ [ ] Unordered criterion",
            "period": "1. Numbered criterion",
            "paren": "1) Numbered criterion",
        }
        for marker, criterion in cases.items():
            with self.subTest(marker=marker):
                slug = f"full-top-level-{marker}"
                archived = self._archive_with(
                    slug,
                    "Depth: full\n",
                    proposal=True,
                    spec=True,
                    report=report,
                    root=root,
                )
                archived.joinpath("proposal.md").write_text(
                    f"# proposal\n\n## Success Criteria\n{criterion}\n"
                )
                result = self.mod.check_premerge(root, slug, tier="full")
                self.assertTrue(result.ok, result.blockers)

    def test_full_requires_deterministic_mapping_for_every_success_criterion(self):
        root = self._repo()
        slug = "full-mapping"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n"
            "- Criterion 1: PASS — first criterion is covered\n"
        )
        archived = self._archive_with(
            slug,
            "Depth: full\n",
            proposal=True,
            spec=True,
            report=report,
            root=root,
        )
        archived.joinpath("proposal.md").write_text(
            "# proposal\n\n## Success Criteria\n"
            "- [ ] First criterion\n"
            "- [ ] Second criterion\n"
        )

        missing = self.mod.check_premerge(root, slug, tier="full")
        self.assertFalse(missing.ok)
        self.assertTrue(any("criterion 2" in blocker.lower() for blocker in missing.blockers))

        archived.joinpath("verify-report.md").write_text(
            report + "- Criterion 2: PASS — second criterion is covered\n"
        )
        complete = self.mod.check_premerge(root, slug, tier="full")
        self.assertTrue(complete.ok, complete.blockers)
    def test_full_does_not_fallback_to_design_when_proposal_criteria_missing_or_empty(self):
        root = self._repo()
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        for proposal_text in ("# proposal\n", "# proposal\n\n## Success Criteria\n\n"):
            with self.subTest(proposal_text=proposal_text):
                slug = "proposal-criteria-required-" + str(len(proposal_text))
                archived = self._archive_with(
                    slug,
                    "Depth: full\n",
                    proposal=True,
                    spec=True,
                    report=report,
                    root=root,
                )
                archived.joinpath("proposal.md").write_text(proposal_text)
                archived.joinpath("design.md").write_text(
                    "# design\n\n## Success Criteria\n- [ ] Design criterion\n"
                )

                result = self.mod.check_premerge(root, slug, tier="full")

                self.assertFalse(result.ok)
                self.assertTrue(any("proposal.md" in blocker for blocker in result.blockers))

    def test_full_rejects_duplicate_success_criteria_headings(self):
        root = self._repo()
        slug = "duplicate-success-criteria"
        report = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        archived = self._archive_with(
            slug,
            "Depth: full\n",
            proposal=True,
            spec=True,
            report=report,
            root=root,
        )
        archived.joinpath("proposal.md").write_text(
            "# proposal\n\n## Success Criteria\n- [ ] First criterion\n\n"
            "## Success Criteria\n- [ ] Second criterion\n"
        )

        result = self.mod.check_premerge(root, slug, tier="full")

        self.assertFalse(result.ok)
        self.assertTrue(any("duplicate" in blocker.lower() for blocker in result.blockers))


    def test_full_does_not_require_explore_when_report_conforms(self):
        root = self._repo()
        slug = "full-no-explore"
        report = (
            "## Verify evidence\n- Status: PASS\n- Command: ./tests/validate.sh\n"
            "- Exit status: 0\n- Date: 2026-08-07\n- SHA: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
        )
        self._archive_with(
            slug,
            "Depth: full\n",
            proposal=True,
            spec=True,
            report=report,
            root=root,
        )
        result = self.mod.check_premerge(root, slug, tier="full")
        self.assertTrue(result.ok, result.blockers)
        self.assertFalse((root / "openspec" / "changes" / slug / "explore.md").exists())

    def test_prearchive_checks_active_folder_without_active_blocker(self):
        root = self._repo()
        slug = "active-prearchive"
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: standard\n")
        (active / "proposal.md").write_text("# proposal\n")
        (active / "specs" / "cap").mkdir(parents=True)
        (active / "specs" / "cap" / "spec.md").write_text("# cap\n")

        blocked = self.mod.check_prearchive(root, slug, tier="standard")
        self.assertFalse(blocked.ok)
        self.assertTrue(any("verify-report.md" in blocker for blocker in blocked.blockers))
        self.assertFalse(any("active" in blocker.lower() for blocker in blocked.blockers))

        (active / "verify-report.md").write_text(
            "## Verify evidence\n- Overall: PASS\n- Command: ./tests/run.sh\n"
            "- Exit code: 0\n- Date: 2026-08-07\n- Revision: 1234567\n"
        )
        self.assertTrue(self.mod.check_prearchive(root, slug, tier="standard").ok)

    def test_guardian_evaluates_only_requested_slug(self):
        root = self._repo()
        self._archive_with(
            "target",
            "Depth: light\n",
            proposal=True,
            root=root,
        )
        self._archive_with("old-nonconforming", "Depth: standard\n", root=root)

        result = self.mod.check_premerge(root, "target", tier="light")

        self.assertTrue(result.ok, result.blockers)

    def _assert_store_invariant(self, fn, *args, **kwargs):
        """Run fn with no store env and with every store value; assert identical verdicts.

        A store-aware preflight would set STORE_ENV_KEY before invoking the
        guardian. The guardian must be store-blind: ok/blockers are identical
        across baseline and `openspec|engram|both`. An Engram mirror cannot be
        materialized inside this unit test; the invariant under test is that the
        store selection never influences the verdict, so a memory-only presence
        can never substitute for missing repository files.
        """
        baseline = fn(*args, **kwargs)
        for value in STORE_ENUM:
            with mock.patch.dict(os.environ, {STORE_ENV_KEY: value}):
                context = fn(*args, **kwargs)
            self.assertEqual(context.ok, baseline.ok, f"store={value}")
            self.assertEqual(context.blockers, baseline.blockers, f"store={value}")
        return baseline

    def test_guardian_blocks_missing_tier_files_under_any_store(self):
        """Engram memory-only cannot satisfy tier minima: verdict is store-blind."""
        root = self._repo()
        slug = "mem-only-tier"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: standard\n")

        result = self._assert_store_invariant(
            self.mod.check_premerge, root, slug, tier="standard"
        )

        self.assertFalse(result.ok)
        self.assertTrue(
            any("proposal.md" in b or "spec" in b.lower() for b in result.blockers)
        )

    def test_guardian_blocks_missing_verify_evidence_under_any_store(self):
        """Engram mirror cannot satisfy verify evidence: verdict is store-blind."""
        root = self._repo()
        slug = "mem-only-verify"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: standard\n")
        (archived / "proposal.md").write_text("# proposal\n")
        (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
        (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")

        result = self._assert_store_invariant(
            self.mod.check_premerge, root, slug, tier="standard"
        )

        self.assertFalse(result.ok)
        self.assertTrue(any("verify-report.md" in b for b in result.blockers))

    def test_guardian_verdict_invariant_across_stores_for_conforming_archive(self):
        """A conforming archive passes identically under every store selection."""
        root = self._repo()
        slug = "ok-every-store"
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: standard\n")
        (archived / "proposal.md").write_text("# proposal\n")
        (archived / "specs" / "cap" / "spec.md").parent.mkdir(parents=True)
        (archived / "specs" / "cap" / "spec.md").write_text("# cap\n")
        (archived / "verify-report.md").write_text(
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
        )

        result = self._assert_store_invariant(
            self.mod.check_premerge, root, slug, tier="standard"
        )

        self.assertTrue(result.ok, result.blockers)
        self.assertEqual(result.blockers, [])

    def test_cli_prearchive_stage_accepts_active_folder(self):
        root = self._repo()
        slug = "cli-prearchive"
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: light\n")
        (active / "proposal.md").write_text("# proposal\n")

        self.assertEqual(self.mod.main([slug, "--root", str(root), "--stage", "pre-archive"]), 0)

    def test_cli_requires_explicit_root_and_never_falls_back_to_cwd(self):
        """2.3 — RED: the guardian must not depend on the process cwd."""
        root = self._repo()
        slug = "no-root"
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: light\n")
        (active / "proposal.md").write_text("# proposal\n")
        with mock.patch("os.chdir", return_value=None), mock.patch.object(
            self.mod, "check_premerge", side_effect=AssertionError("must not resolve cwd")
        ) as check:
            with self.assertRaises(SystemExit) as ctx:
                self.mod.main([slug])
        self.assertEqual(ctx.exception.code, 2)
        check.assert_not_called()
        # A valid explicit root still evaluates normally (archived light change).
        archive = root / "openspec" / "changes" / "archive" / slug
        archive.parent.mkdir(parents=True, exist_ok=True)
        active.rename(archive)
        self.assertEqual(self.mod.main([slug, "--root", str(root)]), 0)

    def test_cli_prearchive_stage_blocks_standard_before_archive(self):
        root = self._repo()
        slug = "cli-standard-prearchive"
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: standard\n")
        (active / "proposal.md").write_text("# proposal\n")
        (active / "specs" / "cap").mkdir(parents=True)
        (active / "specs" / "cap" / "spec.md").write_text("# cap\n")

        self.assertEqual(
            self.mod.main([slug, "--root", str(root), "--stage", "pre-archive"]),
            1,
        )
        (active / "verify-report.md").write_text(
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
        )
        self.assertEqual(
            self.mod.main([slug, "--root", str(root), "--stage", "pre-archive"]),
            0,
        )


LEDGER_STUB_BINARY = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> "${STUB_LOG}"
decision="${STUB_DECISION:-allow}"
reason="${STUB_REASON:-stub}"
checkpoint=""
wrote=0
while [ $# -gt 0 ]; do
  case "$1" in
    --checkpoint) checkpoint="$2"; shift 2 ;;
    --decide) printf 'DECIDE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    --write) printf 'WRITE %s\\n' "$2" >> "${STUB_LOG}"; wrote=1; shift 2 ;;
    --evidence) printf 'EVIDENCE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    *) shift ;;
  esac
done
if [ "$wrote" = 1 ] && [ "${STUB_WRITE_EXIT:-0}" = 2 ]; then
  exit 2
fi
printf '{"capability":"tracker","active":true,"checkpoint":"%s","mode":"warn","decision":"%s","reason":"%s","identity":{"common_dir":"","branch":"","change":null,"key":""},"item":null,"conflict":null,"prompt":null,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}\\n' "$checkpoint" "$decision" "$reason"
[ "$decision" = block ] && exit 2
exit 0
"""


class GuardianTrackerIsolationTests(unittest.TestCase):
    """W1: the artifact guardian owns artifacts; the tracker host owns the ledger.

    These regressions pin the ownership split: the guardian module exposes no
    tracker grader, and running the guardian against a blocking ledger stub never
    reaches that stub.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "premerge_guardian_isolation")

    def _repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        return root

    def _archive_light(self, root: Path, slug: str = "done") -> Path:
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n")
        (archived / "proposal.md").write_text("# proposal\n")
        return archived

    def _env(self) -> dict:
        env = dict(os.environ)
        for key in ("TRACKER_LEDGER_MODE", "TRACKER_CARD_GATE_MODE", "AI_SPECS_HOME"):
            env.pop(key, None)
        return env

    def test_artifact_guardian_exposes_no_tracker_grader(self):
        for symbol in (
            "ledger_blockers", "resolve_ledger_mode", "_ledger_binary",
            "_ledger_bridge", "_ledger_evidence_args",
        ):
            self.assertFalse(
                hasattr(self.mod, symbol),
                f"the artifact guardian must not own tracker symbol {symbol!r}",
            )

    def test_artifact_guardian_never_invokes_the_tracker_ledger(self):
        root = self._repo()
        self._archive_light(root)
        binary = root / "stub-worktree-gate"
        binary.write_text(LEDGER_STUB_BINARY)
        binary.chmod(0o755)
        log = root / "stub.log"
        env = self._env()
        env["WORKTREE_GATE_BIN"] = str(binary)
        env["STUB_LOG"] = str(log)
        # A blocking verdict would fail the guardian if it graded the ledger.
        env["STUB_DECISION"] = "block"
        env["STUB_REASON"] = "missing tracked item"

        r = subprocess.run(
            [sys.executable, str(MODULE_PATH), "done", "--root", str(root),
             "--stage", "pre-merge", "--tier", "light"],
            capture_output=True, text=True, env=env,
        )

        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(
            log.exists(),
            "the artifact guardian must not invoke the tracker ledger predicate",
        )


def _requirement(name: str, body: str) -> str:
    return (
        f"### Requirement: {name}\n\n{body}\n\n"
        f"#### Scenario: {name}\n\n- **WHEN** {name}\n- **THEN** {name}\n"
    )


CANONICAL_SPEC = (
    "# capability Specification\n\n## Purpose\n\nExisting purpose.\n\n"
    "## Requirements\n\n" + _requirement("Existing requirement", "Existing body.")
)

PROMOTED_DELTA = (
    "# Delta for capability\n\n## ADDED Requirements\n\n"
    + _requirement("Added requirement", "Added body.")
)

UNRESOLVED_DELTA = (
    "# Delta for capability\n\n## MODIFIED Requirements\n\n"
    + _requirement("Absent requirement", "New body.")
)

STANDARD_EVIDENCE = (
    "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
    "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
)


class SpecPromotionParityTests(unittest.TestCase):
    """W3: the read-only guardian blocks Standard/Full unpromoted deltas.

    Promotion is a separate, explicit writer. The guardian only validates
    canonical/delta parity and must never mutate ``openspec/specs``.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "premerge_guardian_promotion_parity")

    def _repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        (root / "openspec" / "specs").mkdir(parents=True)
        return root

    def _active(
        self,
        root: Path,
        slug: str,
        delta: str | None = None,
        *,
        tier: str = "standard",
        evidence: str | None = STANDARD_EVIDENCE,
    ) -> Path:
        folder = root / "openspec" / "changes" / slug
        (folder / "specs" / "capability").mkdir(parents=True, exist_ok=True)
        (folder / "tasks.md").write_text(f"Depth: {tier}\n", encoding="utf-8")
        (folder / "proposal.md").write_text("# proposal\n", encoding="utf-8")
        (folder / "specs" / "capability" / "spec.md").write_text(
            delta if delta is not None else PROMOTED_DELTA, encoding="utf-8"
        )
        if evidence:
            (folder / "verify-report.md").write_text(evidence, encoding="utf-8")
        return folder

    def _archive(self, root: Path, slug: str, delta: str | None = None, **kwargs) -> Path:
        active = self._active(root, slug, delta, **kwargs)
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.parent.mkdir(parents=True, exist_ok=True)
        active.rename(archived)
        return archived

    def _write_canonical(self, root: Path, text: str) -> Path:
        path = root / "openspec" / "specs" / "capability" / "spec.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return path

    @staticmethod
    def _promotion_blockers(blockers: list[str]) -> list[str]:
        return [b for b in blockers if "spec_promotion" in b or "canonical" in b.lower()]

    def test_prearchive_blocks_an_unpromoted_standard_delta(self):
        root = self._repo()
        self._active(root, "unpromoted", PROMOTED_DELTA)

        result = self.mod.check_prearchive(root, "unpromoted", tier="standard")

        self.assertFalse(result.ok)
        promotion = self._promotion_blockers(result.blockers)
        self.assertTrue(promotion, result.blockers)
        self.assertEqual(
            self.mod.main(["unpromoted", "--root", str(root), "--stage", "pre-archive"]),
            1,
        )

    def test_prearchive_passes_when_the_delta_is_promoted(self):
        root = self._repo()
        self._promote(root, "promoted", PROMOTED_DELTA)
        self._active(root, "promoted", PROMOTED_DELTA)

        result = self.mod.check_prearchive(root, "promoted", tier="standard")

        self.assertTrue(result.ok, result.blockers)

    def test_premerge_blocks_an_unpromoted_archived_delta(self):
        root = self._repo()
        self._archive(root, "merged-unpromoted", PROMOTED_DELTA)

        result = self.mod.check_premerge(root, "merged-unpromoted", tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(self._promotion_blockers(result.blockers), result.blockers)
        self.assertEqual(
            self.mod.main(["merged-unpromoted", "--root", str(root)]),
            1,
        )

    def test_premerge_passes_for_a_promoted_standard_change(self):
        root = self._repo()
        self._promote(root, "merged-promoted", PROMOTED_DELTA)
        self._archive(root, "merged-promoted", PROMOTED_DELTA)

        result = self.mod.check_premerge(root, "merged-promoted", tier="standard")

        self.assertTrue(result.ok, result.blockers)

    def test_premerge_passes_for_a_promoted_full_change(self):
        root = self._repo()
        evidence = (
            "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n"
            "- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
            "- ready_for_archive: true\n\n"
            "## Success-criteria mapping\n- Criterion 1: PASS — criterion one\n"
        )
        proposal = (
            "# proposal\n\n## Success Criteria\n\n- first criterion\n"
        )
        self._promote(root, "merged-full", PROMOTED_DELTA)
        archived = self._archive(
            root, "merged-full", PROMOTED_DELTA, tier="full", evidence=evidence
        )
        (archived / "proposal.md").write_text(proposal, encoding="utf-8")

        result = self.mod.check_premerge(root, "merged-full", tier="full")

        self.assertTrue(result.ok, result.blockers)

    def test_unresolved_delta_blocks_even_with_a_conforming_report(self):
        root = self._repo()
        self._active(root, "unresolved", UNRESOLVED_DELTA)
        self._write_canonical(root, CANONICAL_SPEC)

        result = self.mod.check_prearchive(root, "unresolved", tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(self._promotion_blockers(result.blockers), result.blockers)

    def test_renamed_delta_is_reported_as_unresolved(self):
        root = self._repo()
        renamed = (
            "# Delta for capability\n\n## RENAMED Requirements\n\n"
            "- FROM: `### Requirement: Existing requirement`\n"
            "  TO: `### Requirement: Renamed requirement`\n"
        )
        self._active(root, "renamed", renamed)
        self._write_canonical(root, CANONICAL_SPEC)

        result = self.mod.check_prearchive(root, "renamed", tier="standard")

        self.assertFalse(result.ok)
        self.assertTrue(any("RENAMED" in b for b in result.blockers), result.blockers)

    def test_light_change_without_specs_is_unaffected(self):
        root = self._repo()
        folder = root / "openspec" / "changes" / "light-change"
        folder.mkdir(parents=True)
        (folder / "tasks.md").write_text("Depth: light\n", encoding="utf-8")
        (folder / "proposal.md").write_text("# proposal\n", encoding="utf-8")

        result = self.mod.check_prearchive(root, "light-change", tier="light")

        self.assertTrue(result.ok, result.blockers)
        archived = root / "openspec" / "changes" / "archive" / "light-change"
        archived.parent.mkdir(parents=True, exist_ok=True)
        folder.rename(archived)
        self.assertTrue(
            self.mod.check_premerge(root, "light-change", tier="light").ok
        )
        self.assertEqual(list((root / "openspec" / "specs").iterdir()), [])

    def test_guardian_never_writes_canonical_specs_or_temp_files(self):
        root = self._repo()
        self._active(root, "read-only", PROMOTED_DELTA)
        canonical_dir = root / "openspec" / "specs" / "capability"

        result = self.mod.check_prearchive(root, "read-only", tier="standard")

        self.assertFalse(result.ok)
        self.assertFalse(canonical_dir.exists())
        self.assertEqual(list((root / "openspec" / "specs").iterdir()), [])

    def _promote(self, root: Path, slug: str, delta: str) -> None:
        """Compose the delta with the promoter, exactly as the skill instructs."""
        spec_path = Path(self.mod.__file__).with_name("spec_promotion.py")
        promotion = load_module(spec_path, f"spec_promotion_for_{slug}")
        folder = root / "openspec" / "changes" / slug
        (folder / "specs" / "capability").mkdir(parents=True, exist_ok=True)
        (folder / "specs" / "capability" / "spec.md").write_text(
            delta, encoding="utf-8"
        )
        (folder / "tasks.md").write_text("Depth: standard\n", encoding="utf-8")
        (folder / "proposal.md").write_text("# proposal\n", encoding="utf-8")
        report = promotion.promote_change(root, slug)
        assert not report.blockers, report.blockers


PLAN_BUILD_SPEC = ROOT / "openspec" / "specs" / "plan-build-flow" / "spec.md"
TRACKER_LEDGER_SPEC = ROOT / "openspec" / "specs" / "tracker-ledger" / "spec.md"
VCS_PR_SPEC = ROOT / "openspec" / "specs" / "vcs-pr-flow" / "spec.md"
TRACKER_GATE_HOOK = (
    ROOT / "catalog" / "recipes" / "trello-mcp-workflow" / "hooks" / "tracker-card-gate.sh"
)
PLAN_BUILD_GATE_HOOK = (
    ROOT / "catalog" / "recipes" / "plan-build-flow" / "hooks" / "plan-build-gate.sh"
)


class CanonicalOwnershipContractTests(unittest.TestCase):
    """W6: the canonical specs pin the post-split ownership, not the fused one.

    The three canonical specs are the durable contract. These regressions keep
    Plan Build owning verify -> promotion -> read-only guardian -> archive, the
    Tracker domain host owning `pre-merge`/`archive-close` independently of
    Plan Build/OpenSpec archive, and VCS owning transport/review/merge/cleanup
    only -- while no stale text still says the artifact guardian hosts tracker
    checkpoints.
    """

    @staticmethod
    def _norm(text: str) -> str:
        """Collapse wrapping so an assertion pins the clause, not the line breaks."""
        return " ".join(text.split())

    # --- plan-build-flow: verify -> promote -> guardian -> archive ---

    def test_plan_build_owns_the_ordered_review_branch_tail(self):
        text = self._norm(PLAN_BUILD_SPEC.read_text(encoding="utf-8"))
        self.assertIn(
            "verify evidence \u2192 promote canonical delta specs \u2192 run the "
            "read-only artifact guardian \u2192 archive the change folder",
            text,
        )

    def test_plan_build_guardian_validates_promotion_parity_read_only(self):
        text = self._norm(PLAN_BUILD_SPEC.read_text(encoding="utf-8"))
        self.assertIn("promotion parity", text)
        self.assertIn("read-only", text)
        self.assertIn("MUST NOT write, rewrite, promote, or repair canonical specs", text)
        # The pre-merge guardian hard-blocker list carries the parity blocker.
        self.assertIn("promotion parity check", text)

    def test_plan_build_keeps_light_and_odd_no_spec_behavior_explicit(self):
        text = self._norm(PLAN_BUILD_SPEC.read_text(encoding="utf-8"))
        self.assertIn("Light", text)
        self.assertIn("ODD task-only", text)
        self.assertIn("no spec deltas", text)

    def test_plan_build_guardian_is_not_offered_to_vcs_merge_skills(self):
        text = self._norm(PLAN_BUILD_SPEC.read_text(encoding="utf-8"))
        self.assertNotIn("or a VCS merge skill", text)

    # --- tracker-ledger: autonomous core + Tracker domain port ---

    def test_tracker_gate_direct_mode_hosts_the_pre_merge_and_archive_close_checkpoints(self):
        text = self._norm(TRACKER_LEDGER_SPEC.read_text(encoding="utf-8"))
        # The canonical contract names the live shell bridge in direct host mode.
        self.assertIn(
            "`pre-merge` and `archive-close` → the `tracker-card-gate.sh` shell "
            "bridge in direct host mode "
            "(`--root <root> --checkpoint pre-merge|archive-close`)",
            text,
        )
        # ... and no longer names the retired Python host anywhere.
        self.assertNotIn("tracker_ledger_host.py", text)

    def test_tracker_ledger_spec_never_pins_checkpoints_to_the_guardian(self):
        text = TRACKER_LEDGER_SPEC.read_text(encoding="utf-8")
        self.assertNotIn("premerge_guardian", text)
        self.assertNotIn("pre-merge guardian", text)

    def test_tracker_closure_is_independent_of_the_openspec_archive(self):
        text = self._norm(TRACKER_LEDGER_SPEC.read_text(encoding="utf-8"))
        self.assertIn("independent of OpenSpec archive", text)
        self.assertIn("no `openspec/` tree", text)

    def test_tracker_domain_port_and_declarative_adapters_are_specified(self):
        text = self._norm(TRACKER_LEDGER_SPEC.read_text(encoding="utf-8"))
        self.assertIn("### Requirement: Tracker domain port with declarative provider adapters", text)
        self.assertIn("[config.reconcile]", text)
        self.assertIn("MUST NOT enable, disable, or change ledger behavior", text)
        self.assertIn("no provider may be guessed", text)

    def test_no_host_resolves_config_from_a_hardcoded_literal(self):
        text = self._norm(TRACKER_LEDGER_SPEC.read_text(encoding="utf-8"))
        self.assertIn("No host MAY resolve its config section from a hardcoded", text)
        self.assertIn(
            "the plan-build work-start gate, the tracker gate (including its direct "
            "`--root <root> --checkpoint pre-merge|archive-close` host mode), and doctor",
            text,
        )

    # --- vcs-pr-flow: transport/review/merge/cleanup only ---

    def test_vcs_owns_transport_without_sdd_ceremony(self):
        text = self._norm(VCS_PR_SPEC.read_text(encoding="utf-8"))
        self.assertIn("### Requirement: VCS transport owns PR/MR, review, and cleanup only", text)
        self.assertIn("MUST NOT require, produce, schedule, or validate SDD/OpenSpec planning or archive artifacts", text)
        self.assertIn("MUST NOT call the artifact guardian itself", text)

    def test_vcs_spec_keeps_provider_neutral_safety(self):
        text = self._norm(VCS_PR_SPEC.read_text(encoding="utf-8"))
        for phrase in ("no auto-merge", "protected heads", "post-merge cleanup"):
            self.assertIn(phrase, text)
        self.assertNotIn("premerge_guardian", text)

    # --- no stale host reference anywhere in the shipped surface ---

    def test_tracker_gate_comments_name_the_direct_checkpoint_host(self):
        text = TRACKER_GATE_HOOK.read_text(encoding="utf-8")
        self.assertNotIn("pre-merge guardian", text)
        self.assertIn("--checkpoint pre-merge|archive-close", text)
        # The retired Python host may survive only as a historical removal note.
        if "tracker_ledger_host.py" in text:
            self.assertIn("retired", text)

    def test_unreleased_changelog_records_the_python_host_removal(self):
        changelog = (ROOT / "CHANGELOG.md").read_text(encoding="utf-8")
        unreleased = changelog.split("## [0.22.0]", 1)[0]
        # Historical removal note only: the entry names the retired module and the
        # shell bridge that now hosts the checkpoints, never a live Python host.
        self.assertIn(
            "`lib/_internal/tracker_ledger_host.py` Python host is removed", unreleased
        )
        self.assertIn("--checkpoint pre-merge|archive-close", unreleased)
        self.assertNotIn("pre-merge guardian", unreleased)

    def test_no_shipped_recipe_claims_the_guardian_hosts_tracker_checkpoints(self):
        offenders = []
        for path in sorted((ROOT / "catalog" / "recipes").rglob("*.sh")):
            text = path.read_text(encoding="utf-8", errors="ignore")
            if "pre-merge guardian" in text or "premerge_guardian" in text:
                offenders.append(str(path.relative_to(ROOT)))
        self.assertEqual(
            offenders, [],
            "no tracker host may claim the Plan Build artifact guardian grades it",
        )


if __name__ == "__main__":
    unittest.main()
