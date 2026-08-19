"""RED/GREEN tests for pre-merge archive/artifact guardian."""

from __future__ import annotations

import importlib.util
import os
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

# TRIAGE: test_blocks_when_active_change_folder_exists asserts result.ok is False with an
# "active"/"archive" blocker from check_premerge(). Ran `grep -n guardian bin/ai-specs` (empty,
# exit 1), `bin/ai-specs help` (14-verb surface, no guardian verb), and
# `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest` on fixture /tmp/gtest holding
# openspec/changes/demo-change (ERROR lines only manifest/agents-md/bundled-skill, exit 1). None
# exposed the ok/blockers verdict; the only reachable surface is the direct
# `python3 lib/_internal/premerge_guardian.py <slug> --root <root>` call, which is the coupling
# being removed.
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

# TRIAGE: test_infer_tier_from_tasks_depth_line asserts result.ok True after tier=None infers
# "Depth: light" from tasks.md. Ran `bin/ai-specs help` (no verb computes tiering) and
# `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest` (checks are
# manifest/cli-version/agents-md/bundled-skill only, no Depth tier inference), and
# `AI_SPECS_NO_NETWORK=1 LC_ALL=C < /dev/null bin/ai-specs hub /tmp/hubtest` (exits 2 with
# "no ai-specs project", never reaches tier logic). Tier inference surfaced nowhere; only
# `python3 lib/_internal/premerge_guardian.py` exposes it.
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

# TRIAGE: test_multiple_dated_archives_block_as_ambiguous asserts result.ok False with
# "ambiguous" and both "2026-08-15-"/"2026-08-16-" archive names in the joined blockers. Ran
# `bin/ai-specs help` (no verb resolves dated archives or selects a slug) and
# `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest` (no archive-resolution or
# ambiguity check in its ERROR lines). The dated-archive ambiguity verdict surfaced in neither;
# it exists only on `python3 lib/_internal/premerge_guardian.py`.
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

# TRIAGE: test_standard_report_requires_all_auditable_fields_and_zero_exit asserts ok False
# with "exit"/"date"/"commit" blockers when verify-report.md lacks canonical fields. Ran
# `bin/ai-specs help` (no report-parsing verb) and
# `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest` (ERROR output is
# manifest/agents-md/bundled-skill only). report-field validation surfaced in neither surface;
# it is reachable only via `python3 lib/_internal/premerge_guardian.py`.
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

# TRIAGE: test_full_requires_deterministic_mapping_for_every_success_criterion asserts ok False
# with a "criterion 2" blocker when a success criterion is unmapped, then ok True once mapped.
# Ran `bin/ai-specs help` and `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest`;
# neither surfaces success-criteria mapping validation, so that full-tier gate exists only in
# `python3 lib/_internal/premerge_guardian.py`.
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

# TRIAGE: test_guardian_blocks_missing_tier_files_under_any_store asserts result.ok False with
# proposal.md/spec blockers invariant across PLAN_BUILD_ARTIFACT_STORE=openspec|engram|both via
# mock.patch.dict. Ran `bin/ai-specs help` (no verb reads that env or offers a store-blind
# verdict) and `AI_SPECS_NO_NETWORK=1 LC_ALL=C bin/ai-specs doctor /tmp/gtest` (no
# store-selection surface in its output). The store-blind invariant surfaced in neither surface;
# it is testable only by importing `python3 lib/_internal/premerge_guardian.py` directly.
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

# TRIAGE: test_cli_requires_explicit_root_and_never_falls_back_to_cwd asserts SystemExit code 2
# when --root is omitted and code 0 with --root, exercising the guardian module's own argparse
# main(). Ran `grep -rn premerge_guardian lib/*.sh ai-specs/ catalog/recipes/*/hooks/` (only
# ai-specs/.deps/BRIEF.md doc lines, no lib/*.sh caller) and `bin/ai-specs help` (no premerge
# verb). This exit-code contract surfaced in neither surface; it is reachable only by invoking
# `python3 lib/_internal/premerge_guardian.py <slug>` directly, which is the coupling removed.
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
if __name__ == "__main__":
    unittest.main()
