package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tierOpt mirrors the CLI's optional --tier: nil means "infer from tasks.md".
func tierOpt(name string) *string { return &name }

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "openspec", "changes", "archive"))
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// archiveOpts mirrors the Python suite's _archive_with helper.
type archiveOpts struct {
	proposal    bool
	design      bool
	spec        bool
	report      *string
	archiveName string
}

const standardReport = "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
	"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"

func repoString(report string) *string { return &report }

func archiveWith(t *testing.T, root, slug, tasks string, opts archiveOpts) string {
	t.Helper()
	name := opts.archiveName
	if name == "" {
		name = slug
	}
	folder := filepath.Join(root, "openspec", "changes", "archive", name)
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), tasks)
	if opts.proposal {
		writeFile(t, filepath.Join(folder, "proposal.md"),
			"# proposal\n\n## Success Criteria\n- [ ] Criterion fixture\n")
	}
	if opts.design {
		writeFile(t, filepath.Join(folder, "design.md"), "# design\n")
	}
	if opts.spec {
		writeFile(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	}
	if opts.report != nil {
		writeFile(t, filepath.Join(folder, "verify-report.md"), *opts.report)
	}
	return folder
}

func activeWith(t *testing.T, root, slug, tasks string, opts archiveOpts) string {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", slug)
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), tasks)
	if opts.proposal {
		writeFile(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	}
	if opts.spec {
		writeFile(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	}
	if opts.report != nil {
		writeFile(t, filepath.Join(folder, "verify-report.md"), *opts.report)
	}
	return folder
}

func joined(result Result) string { return strings.ToLower(strings.Join(result.Blockers, " ")) }

func assertBlockerContains(t *testing.T, result Result, needles ...string) {
	t.Helper()
	text := joined(result)
	for _, needle := range needles {
		if !strings.Contains(text, strings.ToLower(needle)) {
			t.Fatalf("blockers %q do not contain %q", result.Blockers, needle)
		}
	}
}

func assertBlockerLacks(t *testing.T, result Result, needle string) {
	t.Helper()
	if strings.Contains(joined(result), strings.ToLower(needle)) {
		t.Fatalf("blockers %q unexpectedly contain %q", result.Blockers, needle)
	}
}

func TestBlocksWhenActiveChangeFolderExists(t *testing.T) {
	root := newRepo(t)
	slug := "demo-change"
	activeWith(t, root, slug, "Depth: standard\n", archiveOpts{spec: true})

	result := CheckPremerge(root, slug, tierOpt("standard"))
	if result.OK {
		t.Fatal("expected a blocker while the active folder exists")
	}
	assertBlockerContains(t, result, "active")
}

func TestBlocksWhenArchiveMissing(t *testing.T) {
	result := CheckPremerge(newRepo(t), "missing-slug", tierOpt("standard"))
	if result.OK {
		t.Fatal("expected a blocker without an archive")
	}
	assertBlockerContains(t, result, "archive")
}

func TestStandardMinimumBlocksWithoutSpecs(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-change", "Depth: standard\n", archiveOpts{})
	result := CheckPremerge(root, "std-change", tierOpt("standard"))
	if result.OK {
		t.Fatal("expected a blocker without specs/")
	}
	assertBlockerContains(t, result, "spec")
}

func TestStandardMinimumPasses(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "ok-std", "Depth: standard\n", archiveOpts{
		proposal: true, spec: true, report: repoString(standardReport),
	})
	result := CheckPremerge(root, "ok-std", tierOpt("standard"))
	if !result.OK || len(result.Blockers) != 0 {
		t.Fatalf("conforming archive = %+v, want OK with no blockers", result)
	}
}

func TestLightTierRequiresProposal(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "ok-light", "Depth: light\n", archiveOpts{proposal: true})
	result := CheckPremerge(root, "ok-light", tierOpt("light"))
	if !result.OK {
		t.Fatalf("light archive = %+v, want OK", result)
	}
}

func TestLightMinimumRequiresProposalButNotVerifyEvidence(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "light-proposal", "Depth: light\n", archiveOpts{})
	result := CheckPremerge(root, "light-proposal", tierOpt("light"))
	if result.OK {
		t.Fatal("light without proposal must block")
	}
	assertBlockerContains(t, result, "proposal.md")
	assertBlockerLacks(t, result, "verify")
}

func TestFullTierRequiresProposalOrDesign(t *testing.T) {
	root := newRepo(t)
	slug := "full-change"
	folder := archiveWith(t, root, slug, "Depth: full\n", archiveOpts{spec: true})
	if result := CheckPremerge(root, slug, tierOpt("full")); result.OK {
		t.Fatal("full without proposal.md or design.md must block")
	}

	writeFile(t, filepath.Join(folder, "design.md"),
		"# design\n\n## Success Criteria\n- [ ] Criterion fixture\n")
	writeFile(t, filepath.Join(folder, "verify-report.md"),
		"## Verify evidence\n- Verdict: PASS\n- Command: ./tests/validate.sh\n"+
			"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"+
			"- ready_for_archive: true\n\n## Success-criteria mapping\n"+
			"- Criterion 1: PASS — covered\n")
	if result := CheckPremerge(root, slug, tierOpt("full")); !result.OK {
		t.Fatalf("full with design.md = %+v, want OK", result)
	}
}

func TestInferTierFromTasksDepthLine(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "infer", "Depth: light\n- [x] done\n", archiveOpts{proposal: true})
	result := CheckPremerge(root, "infer", nil)
	if !result.OK || result.Tier != "light" {
		t.Fatalf("inferred result = %+v, want OK light", result)
	}
}

func TestOmittedTierDefaultsToInferenceNotStandard(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "omit-tier", "Depth: light\n- [x] done\n", archiveOpts{proposal: true})
	if result := CheckPremerge(root, "omit-tier", nil); !result.OK || result.Tier != "light" {
		t.Fatalf("result = %+v, want OK light", result)
	}
}

func TestUnknownTierBlocksAndFallsBackToInference(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "bad-tier", "Depth: light\n", archiveOpts{proposal: true})
	result := CheckPremerge(root, "bad-tier", tierOpt("Deep"))
	if result.OK {
		t.Fatal("an unknown tier must block")
	}
	if result.Tier != "light" {
		t.Fatalf("tier = %q, want the inferred light", result.Tier)
	}
	assertBlockerContains(t, result, "unknown tier 'Deep' (expected light|standard|full)")
}

func TestPrearchiveChecksActiveFolderWithoutActiveBlocker(t *testing.T) {
	root := newRepo(t)
	slug := "active-prearchive"
	folder := activeWith(t, root, slug, "Depth: standard\n", archiveOpts{proposal: true, spec: true})

	blocked := CheckPrearchive(root, slug, tierOpt("standard"))
	if blocked.OK {
		t.Fatal("standard pre-archive without verify-report.md must block")
	}
	assertBlockerContains(t, blocked, "verify-report.md")
	assertBlockerLacks(t, blocked, "active")

	writeFile(t, filepath.Join(folder, "verify-report.md"), standardReport)
	if result := CheckPrearchive(root, slug, tierOpt("standard")); !result.OK {
		t.Fatalf("conforming pre-archive = %+v, want OK", result)
	}
}

func TestPrearchiveBlocksMissingActiveFolder(t *testing.T) {
	result := CheckPrearchive(newRepo(t), "absent", tierOpt("standard"))
	if result.OK {
		t.Fatal("a missing active folder must block")
	}
	assertBlockerContains(t, result, "active change folder missing at openspec/changes/absent/")
}

func TestPrearchiveKeepsTheRawTierWhenTheFolderIsMissing(t *testing.T) {
	result := CheckPrearchive(newRepo(t), "absent", tierOpt("Standard"))
	if result.Tier != "Standard" {
		t.Fatalf("tier = %q, want the raw option value", result.Tier)
	}
}
