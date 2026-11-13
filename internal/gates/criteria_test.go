package gates

import (
	"path/filepath"
	"testing"
)

const fullReport = "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
	"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n" +
	"- ready_for_archive: true\n\n" +
	"## Success-criteria mapping\n- Criterion 1: PASS — covered\n"

func fullArchive(t *testing.T, root, slug, report, proposal string) string {
	t.Helper()
	folder := archiveWith(t, root, slug, "Depth: full\n", archiveOpts{
		proposal: true, spec: true, report: repoString(report),
	})
	if proposal != "" {
		writeFile(t, filepath.Join(folder, "proposal.md"), proposal)
	}
	return folder
}

func TestFullIgnoresIndentedNestedSuccessCriteriaBullets(t *testing.T) {
	root := newRepo(t)
	fullArchive(t, root, "full-nested-criteria", fullReport, "")
	writeFile(t, filepath.Join(root, "openspec", "changes", "archive", "full-nested-criteria", "proposal.md"),
		"# proposal\n\n## Success Criteria\n- [ ] Top-level criterion\n  - Nested implementation detail\n")
	if result := CheckPremerge(root, "full-nested-criteria", tierOpt("full")); !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
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
			root := newRepo(t)
			slug := "full-top-level-" + marker
			fullArchive(t, root, slug, fullReport, "# proposal\n\n## Success Criteria\n"+criterion+"\n")
			if result := CheckPremerge(root, slug, tierOpt("full")); !result.OK {
				t.Fatalf("result = %+v, want OK", result)
			}
		})
	}
}

func TestFullRequiresMappingForEverySuccessCriterion(t *testing.T) {
	root := newRepo(t)
	folder := fullArchive(t, root, "full-mapping", fullReport, "")
	writeFile(t, filepath.Join(folder, "proposal.md"),
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n- [ ] Second criterion\n")

	missing := CheckPremerge(root, "full-mapping", tierOpt("full"))
	if missing.OK {
		t.Fatal("a criterion without a mapping row must block")
	}
	assertBlockerContains(t, missing, "criterion 2")

	writeFile(t, filepath.Join(folder, "verify-report.md"),
		fullReport+"- Criterion 2: PASS — second criterion is covered\n")
	if result := CheckPremerge(root, "full-mapping", tierOpt("full")); !result.OK {
		t.Fatalf("complete mapping = %+v, want OK", result)
	}
}

func TestFullRejectsUnknownAndDuplicateMappingRows(t *testing.T) {
	for name, extra := range map[string]string{
		"unknown":   "- Criterion 2: PASS — out of range\n",
		"duplicate": "- Criterion 1: PASS — repeated\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := newRepo(t)
			slug := "full-mapping-" + name
			fullArchive(t, root, slug, fullReport+extra, "")
			result := CheckPremerge(root, slug, tierOpt("full"))
			if result.OK {
				t.Fatalf("%s mapping row must block", name)
			}
			assertBlockerContains(t, result, name)
		})
	}
}

func TestFullRejectsNonPassMappingStatus(t *testing.T) {
	root := newRepo(t)
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n" +
		"- ready_for_archive: true\n\n" +
		"## Success-criteria mapping\n- Criterion 1: FAIL — not covered\n"
	fullArchive(t, root, "full-mapping-fail", report, "")
	result := CheckPremerge(root, "full-mapping-fail", tierOpt("full"))
	if result.OK {
		t.Fatal("a non-PASS mapping must block at full")
	}
	assertBlockerContains(t, result, "must be strict pass")
}

func TestFullRequiresCanonicalMappingBlock(t *testing.T) {
	root := newRepo(t)
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n- ready_for_archive: true\n"
	fullArchive(t, root, "full-no-mapping", report, "")
	result := CheckPremerge(root, "full-no-mapping", tierOpt("full"))
	if result.OK {
		t.Fatal("a missing mapping block must block at full")
	}
	assertBlockerContains(t, result, "canonical ## success-criteria mapping block")
}

func TestFullDoesNotFallbackToDesignWhenProposalCriteriaMissingOrEmpty(t *testing.T) {
	for _, proposalText := range []string{"# proposal\n", "# proposal\n\n## Success Criteria\n\n"} {
		t.Run(proposalText, func(t *testing.T) {
			root := newRepo(t)
			folder := fullArchive(t, root, "proposal-criteria-required", fullReport, proposalText)
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
	root := newRepo(t)
	fullArchive(t, root, "duplicate-success-criteria", fullReport,
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n\n"+
			"## Success Criteria\n- [ ] Second criterion\n")
	result := CheckPremerge(root, "duplicate-success-criteria", tierOpt("full"))
	if result.OK {
		t.Fatal("duplicate Success Criteria headings must block")
	}
	assertBlockerContains(t, result, "duplicate")
}

func TestFullUsesDesignCriteriaWhenProposalIsAbsent(t *testing.T) {
	root := newRepo(t)
	folder := archiveWith(t, root, "design-criteria", "Depth: full\n", archiveOpts{
		design: true, spec: true, report: repoString(fullReport),
	})
	writeFile(t, filepath.Join(folder, "design.md"),
		"# design\n\n## Success Criteria\n- [ ] Design criterion\n")
	if result := CheckPremerge(root, "design-criteria", tierOpt("full")); !result.OK {
		t.Fatalf("result = %+v, want OK with design criteria", result)
	}
}

func TestFullReportsMissingCriteriaSourceWhenNeitherArtifactExists(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "no-criteria-source", "Depth: full\n", archiveOpts{
		design: true, spec: true, report: repoString(fullReport),
	})
	result := CheckPremerge(root, "no-criteria-source", tierOpt("full"))
	if result.OK {
		t.Fatalf("result = %+v, want a missing-criteria blocker", result)
	}
	assertBlockerContains(t, result, "requires a ## success criteria section")
}
