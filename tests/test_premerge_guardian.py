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
                      report: str | None = None, root: Path | None = None) -> Path:
        root = root or self._repo()
        archived = root / "openspec" / "changes" / "archive" / slug
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

    def test_cli_prearchive_stage_accepts_active_folder(self):
        root = self._repo()
        slug = "cli-prearchive"
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: light\n")
        (active / "proposal.md").write_text("# proposal\n")

        self.assertEqual(self.mod.main([slug, "--root", str(root), "--stage", "pre-archive"]), 0)
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
if __name__ == "__main__":
    unittest.main()
