package gates

import (
	"path/filepath"
	"testing"
)

// evidenceArchive builds one archived change at tier standard with a report, so
// the evidence rules are asserted through the public entry point.
func evidenceArchive(t *testing.T, slug, report string, tasks string) string {
	t.Helper()
	root := newRepo(t)
	archiveWith(t, root, slug, tasks, archiveOpts{
		proposal: true, spec: true, report: repoString(report),
	})
	return root
}

func TestPassesWithTheCanonicalEvidenceBlock(t *testing.T) {
	root := evidenceArchive(t, "canonical", standardReport, "Depth: standard\n")
	if result := CheckPremerge(root, "canonical", tierOpt("standard")); !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
}

func TestStandardReportAcceptsAliasLabelAndBacktickedValues(t *testing.T) {
	report := "## Verify evidence\n\n- Status: PASS (all focused checks)\n" +
		"- Command: `./tests/run.sh`\n- Exit status: 0\n- Date: 2026-08-07\n" +
		"- SHA: 1234567\n"
	root := evidenceArchive(t, "aliases", report, "Depth: standard\n")
	if result := CheckPremerge(root, "aliases", tierOpt("standard")); !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
}

func TestReportIgnoresLabelsOutsideCanonicalEvidenceBlock(t *testing.T) {
	report := "- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 0\n" +
		"- Date: 2026-08-07\n- Commit: 1234567\n" +
		"## Notes\n" +
		"Example labels: Verdict: PASS; Exit: 0; Date: 2026-08-07; Commit: 1234567\n" +
		"## Verify evidence\n" +
		"Evidence is pending.\n"
	root := evidenceArchive(t, "outside-labels", report, "Depth: standard\n")
	result := CheckPremerge(root, "outside-labels", tierOpt("standard"))
	if result.OK {
		t.Fatal("evidence outside the canonical block must not count")
	}
	assertBlockerContains(t, result, "verdict", "command", "exit", "date", "commit")
}

func TestReportRequiresCanonicalEvidenceHeading(t *testing.T) {
	report := "- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 0\n" +
		"- Date: 2026-08-07\n- Commit: 1234567\n"
	root := evidenceArchive(t, "missing-canonical-heading", report, "Depth: standard\n")
	result := CheckPremerge(root, "missing-canonical-heading", tierOpt("standard"))
	if result.OK {
		t.Fatal("a report without the canonical heading must block")
	}
	assertBlockerContains(t, result, "canonical")
}

func TestReportRejectsDuplicateCanonicalEvidenceBlocks(t *testing.T) {
	report := standardReport + "\n## Verify evidence\n- Verdict: PASS\n"
	root := evidenceArchive(t, "duplicate-blocks", report, "Depth: standard\n")
	result := CheckPremerge(root, "duplicate-blocks", tierOpt("standard"))
	if result.OK {
		t.Fatal("duplicate canonical blocks must block")
	}
	assertBlockerContains(t, result, "duplicate canonical ## verify evidence blocks")
}

func TestReportIgnoresFencedCodeBlockEvidence(t *testing.T) {
	report := "```markdown\n## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n```\n"
	root := evidenceArchive(t, "fenced-evidence", report, "Depth: standard\n")
	result := CheckPremerge(root, "fenced-evidence", tierOpt("standard"))
	if result.OK {
		t.Fatal("fenced evidence must not count")
	}
	assertBlockerContains(t, result, "canonical")
}

func TestReportIgnoresIndentedCodeBlockEvidence(t *testing.T) {
	report := "    ## Verify evidence\n    - Verdict: PASS\n    - Command: ./tests/run.sh\n" +
		"    - Exit: 0\n    - Date: 2026-08-07\n    - Commit: 1234567\n"
	root := evidenceArchive(t, "indented-evidence", report, "Depth: standard\n")
	result := CheckPremerge(root, "indented-evidence", tierOpt("standard"))
	if result.OK {
		t.Fatal("indented evidence must not count")
	}
	assertBlockerContains(t, result, "canonical")
}

func TestReportRejectsDuplicateCanonicalLabels(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Status: PASS\n- Command: ./tests/run.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
	root := evidenceArchive(t, "duplicate-labels", report, "Depth: standard\n")
	result := CheckPremerge(root, "duplicate-labels", tierOpt("standard"))
	if result.OK {
		t.Fatal("duplicate labels must block")
	}
	assertBlockerContains(t, result, "duplicate")
}

func TestReportRejectsImpossibleCalendarDate(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
		"- Exit: 0\n- Date: 2026-99-99\n- Commit: 1234567\n"
	root := evidenceArchive(t, "impossible-date", report, "Depth: standard\n")
	result := CheckPremerge(root, "impossible-date", tierOpt("standard"))
	if result.OK {
		t.Fatal("an impossible date must block")
	}
	assertBlockerContains(t, result, "date")
}

func TestNestedHeadingCannotSupplyEvidenceFields(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
		"### Notes\n- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
	root := evidenceArchive(t, "nested-evidence", report, "Depth: standard\n")
	result := CheckPremerge(root, "nested-evidence", tierOpt("standard"))
	if result.OK {
		t.Fatal("a nested heading must end the evidence section")
	}
	assertBlockerContains(t, result, "exit", "date", "commit")
}

func TestStandardReportRequiresAllAuditableFieldsAndZeroExit(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n- Exit: 1\n"
	root := evidenceArchive(t, "bad-report", report, "Depth: standard\n")
	result := CheckPremerge(root, "bad-report", tierOpt("standard"))
	if result.OK {
		t.Fatal("an incomplete report must block")
	}
	assertBlockerContains(t, result, "exit", "date", "commit")
}

func TestStandardEvidenceInsideTasksDoesNotCount(t *testing.T) {
	root := newRepo(t)
	slug := "tasks-evidence"
	archiveWith(t, root, slug,
		"Depth: standard\n\n## Verify evidence\n- Verdict: PASS\n"+
			"- Command: ./tests/run.sh\n- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n",
		archiveOpts{proposal: true, spec: true})
	result := CheckPremerge(root, slug, tierOpt("standard"))
	if result.OK {
		t.Fatal("evidence in tasks.md must not count")
	}
	assertBlockerContains(t, result, "verify-report.md")
	if _, err := filepath.Glob(filepath.Join(root, "openspec", "changes", "archive", slug, "verify-report.md")); err != nil {
		t.Fatal(err)
	}
}

func TestStandardRejectsFailureVerdicts(t *testing.T) {
	for _, verdict := range []string{"FAIL", "BLOCKED", "failure"} {
		t.Run(verdict, func(t *testing.T) {
			report := "## Verify evidence\n- Verdict: " + verdict + "\n- Command: ./tests/run.sh\n" +
				"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"
			root := evidenceArchive(t, "failed-verdict", report, "Depth: standard\n")
			result := CheckPremerge(root, "failed-verdict", tierOpt("standard"))
			if result.OK {
				t.Fatalf("verdict %s must block at standard", verdict)
			}
			assertBlockerContains(t, result, "must not be fail or blocked")
		})
	}
}

func TestFullRequiresStrictPassAndReadyForArchive(t *testing.T) {
	report := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n" +
		"\n## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
	root := evidenceArchive(t, "full-report", report, "Depth: full\n")

	missingReady := CheckPremerge(root, "full-report", tierOpt("full"))
	if missingReady.OK {
		t.Fatal("full without ready_for_archive must block")
	}
	assertBlockerContains(t, missingReady, "ready_for_archive")

	writeFile(t, filepath.Join(root, "openspec", "changes", "archive", "full-report", "verify-report.md"),
		"## Verify evidence\n- Verdict: WARN\n- Command: ./tests/validate.sh\n"+
			"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"+
			"- ready_for_archive: true\n\n## Success-criteria mapping\n- Criterion 1: PASS — covered\n")
	failed := CheckPremerge(root, "full-report", tierOpt("full"))
	if failed.OK {
		t.Fatal("a non-PASS verdict must block at full")
	}
	assertBlockerContains(t, failed, "pass")
}

func TestFullAcceptsAliasLabelsAndReadyForArchive(t *testing.T) {
	report := "## Verify evidence\n- Status: PASS\n- Command: ./tests/validate.sh\n" +
		"- Exit status: 0\n- Date: 2026-08-07\n- SHA: 1234567\n" +
		"- ready_for_archive: true\n\n## Success-criteria mapping\n- Criterion 1: PASS — covered\n"
	root := evidenceArchive(t, "full-no-explore", report, "Depth: full\n")
	if result := CheckPremerge(root, "full-no-explore", tierOpt("full")); !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
}

func TestLightIsAdvisoryAboutEvidence(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "light-no-report", "Depth: light\n", archiveOpts{proposal: true})
	if result := CheckPremerge(root, "light-no-report", tierOpt("light")); !result.OK {
		t.Fatalf("light = %+v, want OK without a report", result)
	}
}
