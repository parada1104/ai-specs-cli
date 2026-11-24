package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/specs"
)

// tierOpt mirrors the CLI's optional --tier: nil means "infer from tasks.md".
func tierOpt(name string) *string { return &name }

// fixtureConfig is a valid project config: `go` is on PATH and matches the
// fixture verification rows, so a test about another rule stays about that rule.
const fixtureConfig = "schema: spec-driven\n\nstrict_tdd: true\n\ntesting:\n  runner: go test ./internal/...\n"

// newBareRepo is newRepo without the project config, for the config rules.
func newBareRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "openspec", "changes", "archive"))
	return root
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := newBareRepo(t)
	writeFile(t, filepath.Join(root, "openspec", "config.yaml"), fixtureConfig)
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
	writeFile(t, filepath.Join(folder, "tasks.md"), planOrFixture(tasks))
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
	writeFile(t, filepath.Join(folder, "tasks.md"), planOrFixture(tasks))
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

func assertBlockerLacks(t *testing.T, result Result, needles ...string) {
	t.Helper()
	text := joined(result)
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			t.Fatalf("blockers %q unexpectedly contain %q", result.Blockers, needle)
		}
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

func TestLightTierPlanPasses(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "ok-light", "Depth: light\n", archiveOpts{})
	result := CheckPremerge(root, "ok-light", tierOpt("light"))
	if !result.OK {
		t.Fatalf("light archive = %+v, want OK", result)
	}
}

func TestLightMinimumRequiresOnlyThePlan(t *testing.T) {
	root := newRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "archive", "light-no-plan")
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	// The archive holds a delta, so the folder itself proves the change existed
	// and the blocker below is the tier minimum, not the empty archive (R2).
	writeFile(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")

	result := CheckPremerge(root, "light-no-plan", tierOpt("light"))
	if result.OK {
		t.Fatal("light without tasks.md must block")
	}
	assertBlockerContains(t, result, "/tasks.md is required for all tiers")
	assertBlockerLacks(t, result, "proposal.md", "verify")
}

func TestFullMinimumBlocksWithoutADelta(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "full-no-delta", "Depth: full\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/`, exit 0\n"), archiveOpts{})

	result := CheckPremerge(root, "full-no-delta", tierOpt("full"))
	if result.OK {
		t.Fatal("full without a delta must block")
	}
	assertBlockerContains(t, result, "/specs/ must contain at least one *.md for tier full")
	assertBlockerLacks(t, result, "proposal.md", "design.md")
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

// planFixtureBody is a minimal valid plan for the fixtures that only exercise
// another rule. The `## Log` entry keeps it valid from standard up (P6).
const planFixtureBody = "## Specs\n\n- **S1.** Fixture requirement\n\n## Tasks\n\n" +
	"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
	"| T1 | S1 | Fixture task | inline | pendiente |\n\n" +
	"## Log\n\n- **L1** — fixture request\n"

// planOrFixture completes a fixture's tasks text with a conforming plan, so a
// test about another rule does not have to spell the grammar out.
func planOrFixture(tasks string) string {
	if strings.Contains(tasks, "## Specs") {
		return tasks
	}
	return tasks + "\n" + planFixtureBody
}

func TestInspectFolderBlocksAFreeFormTasksFile(t *testing.T) {
	root := newRepo(t)
	folder := archiveWith(t, root, "free-form", "Depth: standard\n\n"+planFixtureBody, archiveOpts{
		proposal: true, spec: true, report: repoString(standardReport),
		archiveName: "2026-01-02-free-form",
	})
	writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: standard\n- [x] una tarea suelta\n")
	result := CheckPremerge(root, "free-form", tierOpt("standard"))
	if result.OK {
		t.Fatal("a free-form tasks.md must block for every tier")
	}
	assertBlockerContains(t, result, "archive/2026-01-02-free-form/tasks.md: `## Specs` section is missing",
		"archive/2026-01-02-free-form/tasks.md: `## Tasks` section is missing")

	writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: standard\n\n"+planFixtureBody)
	if result := CheckPremerge(root, "free-form", tierOpt("standard")); !result.OK {
		t.Fatalf("conforming plan = %+v, want OK", result)
	}
}

// Every archive is graded by the plan grammar, whatever its folder name: the
// undated form is not an exemption (R1).
func TestEveryArchiveIsGradedByThePlanGrammar(t *testing.T) {
	for _, name := range []string{"2026-01-02-undated-free-form", "undated-free-form"} {
		t.Run(name, func(t *testing.T) {
			root := newRepo(t)
			folder := archiveWith(t, root, "undated-free-form", "Depth: standard\n", archiveOpts{
				spec: true, report: repoString(standardReport), archiveName: name,
			})
			writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: standard\n- [x] una tarea suelta\n")

			result := CheckPremerge(root, "undated-free-form", tierOpt("standard"))
			if result.OK {
				t.Fatal("a free-form tasks.md must block whatever the archive folder is called")
			}
			assertBlockerContains(t, result, "archive/"+name+"/tasks.md: `## Specs` section is missing",
				"archive/"+name+"/tasks.md: `## Tasks` section is missing")
		})
	}
}

// A conforming plan in the undated form still passes: the grammar is what is
// graded, not the folder name.
func TestUndatedArchiveWithAConformingPlanPasses(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "undated-conforming", "Depth: standard\n\n"+planFixtureBody, archiveOpts{
		spec: true, report: repoString(standardReport),
	})
	if result := CheckPremerge(root, "undated-conforming", tierOpt("standard")); !result.OK {
		t.Fatalf("undated conforming archive = %+v, want OK", result)
	}
}

// planWithoutLog is a conforming plan shape with no `## Log` section.
const planWithoutLog = "## Specs\n\n- **S1.** Fixture requirement\n\n## Tasks\n\n" +
	"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
	"| T1 | S1 | Fixture task | inline | pendiente |\n"

// Standard and Full require `## Log` with L1; Light keeps it optional (R7/P6).
func TestStandardAndFullRequireTheLogSection(t *testing.T) {
	for _, tier := range []string{TierStandard, TierFull} {
		t.Run(tier, func(t *testing.T) {
			root := newRepo(t)
			archiveWith(t, root, "log-"+tier, "Depth: "+tier+"\n\n"+planWithoutLog, archiveOpts{
				spec: true, report: repoString(standardReport),
			})
			result := CheckPremerge(root, "log-"+tier, tierOpt(tier))
			assertBlockerContains(t, result, "`## Log` section is missing")
		})
	}

	root := newRepo(t)
	archiveWith(t, root, "log-light", "Depth: light\n\n"+planWithoutLog, archiveOpts{})
	if result := CheckPremerge(root, "log-light", tierOpt(TierLight)); !result.OK {
		t.Fatalf("light plan without Log = %+v, want OK", result)
	}
}

func TestStandardMinimumNeedsThePlanAndOneDelta(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-plan-delta", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-plan-delta", tierOpt(TierStandard))
	if !result.OK {
		t.Fatalf("standard plan plus delta = %+v, want OK", result)
	}
	assertBlockerLacks(t, result, "proposal.md", "verify-report")
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

func TestLightMinimumNeedsOnlyThePlan(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "light-plan-only", "Depth: light\n", archiveOpts{})
	if result := CheckPremerge(root, "light-plan-only", tierOpt(TierLight)); !result.OK {
		t.Fatalf("light plan-only = %+v, want OK", result)
	}
}

func TestFullMinimumNeedsThePlanAndOneDelta(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "full-plan-delta", "Depth: full\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "full-plan-delta", tierOpt(TierFull))
	if !result.OK {
		t.Fatalf("full plan plus delta = %+v, want OK", result)
	}
	assertBlockerLacks(t, result, "proposal.md", "design.md")
}

// --- ODD archive condition (plan-build-openspec-odd-align I5/P4, S10) ---

// oddPromotedDelta writes and promotes a delta under the change folder.
func oddPromotedDelta(t *testing.T, root, change string) string {
	t.Helper()
	folder := filepath.Join(root, "odd", "changes", change)
	dir := filepath.Join(folder, "specs", "capability")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(oddDelta), 0o644); err != nil {
		t.Fatal(err)
	}
	if report := specs.PromoteChangeForMode(root, change, planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	return folder
}

// An active delta folder blocks even when its delta is promoted: it must be
// archived before merge, and the blocker names the verb that does it.
func TestCheckPremergeOddBlocksAnActiveDeltaFolder(t *testing.T) {
	root := oddRepo(t)
	oddPromotedDelta(t, root, "thing")

	got := CheckPremergeOdd(root, "thing")
	wantBlocked(t, got, "ai-specs changes archive")
	wantBlocked(t, got, "before merge")
}

// Once the promoted delta is archived, the active folder is gone and the guardian
// passes: the archive is read for parity, and it is already promoted (R3).
func TestCheckPremergeOddPassesAnArchivedPromotedDelta(t *testing.T) {
	root := oddRepo(t)
	folder := oddPromotedDelta(t, root, "thing")
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(folder, archive); err != nil {
		t.Fatal(err)
	}

	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK after archiving a promoted delta", got)
	}
}
