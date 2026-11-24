package gates

import (
	"path/filepath"
	"testing"
)

// legacyReport is the legacy full report without ready_for_archive, so the
// legacy-only rule can be asserted.
const legacyReport = "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
	"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n\n" +
	"## Success-criteria mapping\n- Criterion 1: PASS — covered\n"

// legacyArchive builds an archived change in the pre-plan shape: its tasks.md
// carries no `## Specs`, so the requirement source falls back to proposal.md and
// the evidence source to verify-report.md.
func legacyArchive(t *testing.T, root, slug, tier, proposal, report string) string {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", "archive", slug)
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: "+tier+"\n")
	writeFile(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	if proposal != "" {
		writeFile(t, filepath.Join(folder, "proposal.md"), proposal)
	}
	if report != "" {
		writeFile(t, filepath.Join(folder, "verify-report.md"), report)
	}
	return folder
}

// legacyFull grades one archived legacy-shaped change at tier full.
func legacyFull(t *testing.T, slug, proposal, report string) Result {
	t.Helper()
	root := newRepo(t)
	legacyArchive(t, root, slug, TierFull, proposal, report)
	return CheckPremerge(root, slug, tierOpt(TierFull))
}

func TestFullIgnoresIndentedNestedSuccessCriteriaBullets(t *testing.T) {
	result := legacyFull(t, "full-nested-criteria",
		"# proposal\n\n## Success Criteria\n- [ ] Top-level criterion\n  - Nested implementation detail\n",
		legacyReport)
	assertBlockerLacks(t, result, "success criteria", "criterion 2")
}

func TestFullAcceptsTopLevelUnorderedAndNumberedCriteria(t *testing.T) {
	cases := map[string]string{
		"dash":   "- [ ] Unordered criterion",
		"star":   "* [ ] Unordered criterion",
		"plus":   "+ [ ] Unordered criterion",
		"period": "1. Numbered criterion",
		"paren":  "1) Numbered criterion",
	}
	for marker, criterion := range cases {
		t.Run(marker, func(t *testing.T) {
			result := legacyFull(t, "full-top-level-"+marker,
				"# proposal\n\n## Success Criteria\n"+criterion+"\n", legacyReport)
			assertBlockerLacks(t, result, "success criteria", "criterion 2")
		})
	}
}

func TestFullRequiresMappingForEverySuccessCriterion(t *testing.T) {
	root := newRepo(t)
	folder := legacyArchive(t, root, "full-mapping", TierFull,
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n- [ ] Second criterion\n",
		legacyReport)

	missing := CheckPremerge(root, "full-mapping", tierOpt("full"))
	assertBlockerContains(t, missing, "criterion 2")

	writeFile(t, filepath.Join(folder, "verify-report.md"),
		legacyReport+"- Criterion 2: PASS — second criterion is covered\n")
	complete := CheckPremerge(root, "full-mapping", tierOpt("full"))
	assertBlockerLacks(t, complete, "criterion 2")
}

func TestFullRejectsUnknownAndDuplicateMappingRows(t *testing.T) {
	for name, extra := range map[string]string{
		"unknown":   "- Criterion 2: PASS — out of range\n",
		"duplicate": "- Criterion 1: PASS — repeated\n",
	} {
		t.Run(name, func(t *testing.T) {
			result := legacyFull(t, "full-mapping-"+name,
				"# proposal\n\n## Success Criteria\n- [ ] First criterion\n", legacyReport+extra)
			assertBlockerContains(t, result, name)
		})
	}
}

func TestFullRejectsNonPassMappingStatus(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n" +
		"- ready_for_archive: true\n\n" +
		"## Success-criteria mapping\n- Criterion 1: FAIL — not covered\n"
	result := legacyFull(t, "full-mapping-fail",
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n", report)
	assertBlockerContains(t, result, "must be strict pass")
}

func TestFullRequiresCanonicalMappingBlock(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n- ready_for_archive: true\n"
	result := legacyFull(t, "full-no-mapping",
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n", report)
	assertBlockerContains(t, result, "canonical ## success-criteria mapping block")
}

func TestFullDoesNotFallbackToDesignWhenProposalCriteriaMissingOrEmpty(t *testing.T) {
	for _, proposalText := range []string{"# proposal\n", "# proposal\n\n## Success Criteria\n\n"} {
		t.Run(proposalText, func(t *testing.T) {
			root := newRepo(t)
			folder := legacyArchive(t, root, "proposal-criteria-required", TierFull, proposalText, legacyReport)
			writeFile(t, filepath.Join(folder, "design.md"),
				"# design\n\n## Success Criteria\n- [ ] Design criterion\n")

			result := CheckPremerge(root, "proposal-criteria-required", tierOpt("full"))
			if result.OK {
				t.Fatal("design.md must not supply criteria when proposal.md exists")
			}
			assertBlockerContains(t, result, "proposal.md")
		})
	}
}

func TestFullRejectsDuplicateSuccessCriteriaHeadings(t *testing.T) {
	result := legacyFull(t, "duplicate-success-criteria",
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n\n"+
			"## Success Criteria\n- [ ] Second criterion\n", legacyReport)
	assertBlockerContains(t, result, "duplicate")
}

func TestFullUsesDesignCriteriaWhenProposalIsAbsent(t *testing.T) {
	root := newRepo(t)
	folder := legacyArchive(t, root, "design-criteria", TierFull, "", legacyReport)
	writeFile(t, filepath.Join(folder, "design.md"),
		"# design\n\n## Success Criteria\n- [ ] Design criterion\n")

	result := CheckPremerge(root, "design-criteria", tierOpt("full"))
	assertBlockerLacks(t, result, "success criteria", "mapping")
}

func TestFullReportsMissingCriteriaSourceWhenNeitherArtifactExists(t *testing.T) {
	t.Run("no criteria heading", func(t *testing.T) {
		root := newRepo(t)
		folder := legacyArchive(t, root, "no-criteria-source", TierFull, "", legacyReport)
		writeFile(t, filepath.Join(folder, "design.md"), "# design\n")
		result := CheckPremerge(root, "no-criteria-source", tierOpt("full"))
		assertBlockerContains(t, result, "requires a ## success criteria section in design.md")
	})
	t.Run("no proposal or design", func(t *testing.T) {
		result := legacyFull(t, "no-criteria-artifact", "", legacyReport)
		assertBlockerContains(t, result,
			"add at least one `s#` under `## specs` in tasks.md",
			"non-empty ## success criteria section in proposal.md or design.md")
	})
}

func TestLegacyArchiveKeepsCriterionGradingAndReadyForArchive(t *testing.T) {
	root := newRepo(t)
	folder := legacyArchive(t, root, "legacy-full", TierFull,
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n- [ ] Second criterion\n", legacyReport)

	partial := CheckPremerge(root, "legacy-full", tierOpt(TierFull))
	if partial.OK {
		t.Fatal("a legacy full archive without ready_for_archive must block")
	}
	assertBlockerContains(t, partial,
		"verify-report.md requires ready_for_archive: true for tier full",
		"verify-report.md is missing success-criteria mapping for criterion 2")
	assertBlockerLacks(t, partial, "success criteria section")

	writeFile(t, filepath.Join(folder, "verify-report.md"),
		"## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"+
			"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"+
			"- ready_for_archive: true\n\n"+
			"## Success-criteria mapping\n- Criterion 1: PASS — covered\n"+
			"- Criterion 2: PASS — second criterion\n")

	complete := CheckPremerge(root, "legacy-full", tierOpt(TierFull))
	assertBlockerLacks(t, complete, "ready_for_archive", "mapping", "success criteria")
	// The pre-plan shape keeps I2's plan requirement; nothing else remains.
	assertBlockerContains(t, complete, "`## Specs` section is missing")
}
