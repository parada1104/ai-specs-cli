package gates

import (
	"fmt"
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

func TestLightIsAdvisoryAboutEvidence(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "light-no-report", "Depth: light\n", archiveOpts{proposal: true})
	if result := CheckPremerge(root, "light-no-report", tierOpt("light")); !result.OK {
		t.Fatalf("light = %+v, want OK without a report", result)
	}
}

// verificationPlan builds a valid plan whose `## Verification` rows are given,
// so the evidence chain (plan section first, verify-report.md second) can be
// asserted through the public premerge entry point.
// verificationPlan is a standard/full plan whose `## Verification` holds the
// given rows; `## Log` is present because those tiers require it (P6).
func verificationPlan(specs int, rows string) string {
	specLines := ""
	for number := 1; number <= specs; number++ {
		specLines += fmt.Sprintf("- **S%d.** Fixture requirement %d\n", number, number)
	}
	return "## Specs\n\n" + specLines + "\n## Tasks\n\n" +
		"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1 | Fixture task | inline | pendiente |\n\n" +
		"## Verification\n\n" + rows + "\n## Log\n\n- **L1** — fixture request\n"
}

// planEvidenceArchive archives a change whose evidence lives in the plan.
func planEvidenceArchive(t *testing.T, slug, tier string, specs int, rows string, opts archiveOpts) string {
	t.Helper()
	root := newRepo(t)
	archiveWith(t, root, slug, "Depth: "+tier+"\n\n"+verificationPlan(specs, rows), opts)
	return root
}

func TestStandardVerificationRequiresARowPerSpec(t *testing.T) {
	root := planEvidenceArchive(t, "verify-coverage", TierStandard, 2,
		"- **S1: PASS** — `go test ./internal/gates/`, exit 0\n", archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-coverage", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a spec without a verification row must block at standard")
	}
	assertBlockerContains(t, result,
		`verification is missing a row for S2: add "- **S2: PASS** — <evidence>"`)
}

func TestStandardVerificationFailBlocks(t *testing.T) {
	root := planEvidenceArchive(t, "verify-fail", TierStandard, 2,
		"- **S1: PASS** — covered\n- **S2: FAIL** — broken\n", archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-fail", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a FAIL verdict must block at standard")
	}
	assertBlockerContains(t, result,
		`verification for S2 is FAIL: fix it or record "- **S2: N/A** — <why>"`)
}

func TestVerificationAcceptsCriterionLabelsForCompat(t *testing.T) {
	root := planEvidenceArchive(t, "verify-criterion-labels", TierStandard, 2,
		"- **Criterion 1: PASS** — `go test ./internal/gates/`, exit 0\n"+
			"- **Criterion 2: N/A** — `go test ./internal/gates/`, exit 0\n", archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-criterion-labels", tierOpt(TierStandard))
	if !result.OK {
		t.Fatalf("Criterion N rows = %+v, want OK for compatibility", result)
	}
}

// A verdict without a backticked command is prose, not evidence: every graded
// row must name exactly one command, which closes the "PASS — nothing executed"
// bypass (F4, R6).
func TestVerificationRowWithoutABacktickedCommandBlocks(t *testing.T) {
	root := planEvidenceArchive(t, "verify-no-command", TierStandard, 1,
		"- **S1: PASS** — nothing executed\n", archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-no-command", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a row without a backticked command must block")
	}
	assertBlockerContains(t, result, "verification for S1"+
		` must name the command it ran in backticks: record "- **S1: PASS** — `+"`go test ./...`\"")
}

// Prose around the command, a blank span, a bare second backticked value and a
// recorded non-zero exit are all refused (R6).
func TestMalformedVerificationCommandsBlock(t *testing.T) {
	cases := map[string]string{
		"prose before":    "nothing executed (`see notes`)",
		"blank span":      "` `",
		"bare second":     "`go test ./not-a-package`, `no-such-executable`",
		"non-zero exit":   "`go test ./...`, exit 1",
		"non-zero status": "`go test ./...`, exit status: 2",
		"failed word":     "`go test ./...`, failed",
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			root := planEvidenceArchive(t, "verify-malformed", TierStandard, 1,
				"- **S1: PASS** — "+evidence+"\n", archiveOpts{spec: true})

			result := CheckPremerge(root, "verify-malformed", tierOpt(TierStandard))
			if result.OK {
				t.Fatalf("evidence %q must block", evidence)
			}
			assertBlockerContains(t, result, "verification for S1")
		})
	}
}

// A conforming row with trailing notes after the command still passes: notes are
// allowed, a second command is not (R6).
func TestVerificationRowWithTrailingNotesPasses(t *testing.T) {
	root := planEvidenceArchive(t, "verify-notes", TierStandard, 1,
		"- **S1: PASS** — `go test ./...`, exit 0, commit abc1234, 2026-01-01\n",
		archiveOpts{spec: true})

	if result := CheckPremerge(root, "verify-notes", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("row with trailing notes = %+v, want OK", result)
	}
}

// An N/A row carries its reason and records no run, so it is exempt from the
// command requirement (C', R6).
func TestVerificationRowNAWithAReasonPasses(t *testing.T) {
	root := planEvidenceArchive(t, "verify-na", TierStandard, 1,
		"- **S1: N/A** — manual review, no runnable command\n", archiveOpts{spec: true})

	if result := CheckPremerge(root, "verify-na", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("N/A row with a reason = %+v, want OK", result)
	}
}

// Two rows for the same requirement block, naming the id (R6).
func TestDuplicateVerificationRowsBlockByName(t *testing.T) {
	root := planEvidenceArchive(t, "verify-duplicate", TierStandard, 1,
		"- **S1: PASS** — `go test ./...`, exit 0\n- **S1: PASS** — `go test ./...`, exit 0\n",
		archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-duplicate", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("duplicate rows for one requirement must block")
	}
	assertBlockerContains(t, result, "verification has more than one row for S1")
}

func TestVerificationRowForUndeclaredSpecBlocks(t *testing.T) {
	root := planEvidenceArchive(t, "verify-undeclared", TierStandard, 2,
		"- **S1: PASS** — covered\n- **S2: PASS** — covered\n- **S9: PASS** — stale row\n",
		archiveOpts{spec: true})

	result := CheckPremerge(root, "verify-undeclared", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a row for an undeclared spec must block")
	}
	assertBlockerContains(t, result, "S9", "S1-S2")
}
