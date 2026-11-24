package gates

// ODD pre-merge gate (plan-build-odd-companion, open decision B): the mode is
// resolved by the caller, the change comes from the branch pairing, and the
// verdict is the paired plan plus the promotion parity of its deltas. No
// OpenSpec archive and no tier minimum apply.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/specs"
)

const oddDelta = "## ADDED Requirements\n\n### Requirement: ODD rule\n\nBody.\n\n" +
	"#### Scenario: odd\n\n- **WHEN** odd\n- **THEN** odd\n"

// oddRepo builds an odd planning root with the paired plan for change "thing".
func oddRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"odd/tasks", "odd/changes", "odd/specs"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "odd", "tasks", "thing.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func wantBlocked(t *testing.T, got Result, needle string) {
	t.Helper()
	if got.OK || len(got.Blockers) == 0 {
		t.Fatalf("Result = %+v, want a blocker mentioning %q", got, needle)
	}
	if joined := strings.Join(got.Blockers, "\n"); !strings.Contains(joined, needle) {
		t.Fatalf("Blockers = %q, want one mentioning %q", got.Blockers, needle)
	}
}

// A missing paired plan is a blocker naming the path the branch expects.
func TestCheckPremergeOddRequiresThePairedPlan(t *testing.T) {
	root := oddRepo(t)
	if err := os.Remove(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Fatal(err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "ODD plan 'odd/tasks/thing.md' is missing")
}

// A directory at the plan path is not a plan.
func TestCheckPremergeOddRejectsADirectoryPlan(t *testing.T) {
	root := oddRepo(t)
	plan := filepath.Join(root, "odd", "tasks", "thing.md")
	if err := os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plan, 0o755); err != nil {
		t.Fatal(err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "is not a regular file")
}

// An unresolvable branch blocks instead of guessing a change.
func TestCheckPremergeOddBlocksOnDetachedHead(t *testing.T) {
	root := oddRepo(t)
	wantBlocked(t, CheckPremergeOdd(root, ""), "cannot be resolved (detached HEAD)")
	if got := CheckPremergeOdd(root, ""); got.Tier != TierOdd {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierOdd)
	}
}

// A change with no delta passes with its plan: no archive, no tier minimum.
func TestCheckPremergeOddPassesATaskOnlyChange(t *testing.T) {
	root := oddRepo(t)
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", got)
	}
}

// An unpromoted delta blocks, naming the odd canonical tree.
func TestCheckPremergeOddBlocksAnUnpromotedDelta(t *testing.T) {
	root := oddRepo(t)
	dir := filepath.Join(root, "odd", "changes", "thing", "specs", "capability")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(oddDelta), 0o644); err != nil {
		t.Fatal(err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "is not promoted into odd/specs/capability/spec.md")
}

// Once the delta is promoted, the same change passes.
func TestCheckPremergeOddPassesAPromotedDelta(t *testing.T) {
	root := oddRepo(t)
	dir := filepath.Join(root, "odd", "changes", "thing", "specs", "capability")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(oddDelta), 0o644); err != nil {
		t.Fatal(err)
	}
	if report := specs.PromoteChangeForMode(root, "thing", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	// Archiving is mandatory before merge: move the promoted delta folder out of
	// the active tree, then the guardian passes. The plan stays at
	// odd/tasks/thing.md (plan-build-openspec-odd-align I5/P4).
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "odd", "changes", "thing"), archive); err != nil {
		t.Fatal(err)
	}
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK after promotion and archiving", got)
	}
	if got.ArchivePath == "" || got.Tier != TierOdd {
		t.Fatalf("ArchivePath = %q, Tier = %q, want the plan and %q", got.ArchivePath, got.Tier, TierOdd)
	}
}

// A symlinked tasks directory does not satisfy the paired-plan rule: the plan
// must be a regular file of the repository, as the production gate requires.
func TestCheckPremergeOddRefusesASymlinkedTasksDir(t *testing.T) {
	root := oddRepo(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "thing.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "odd", "tasks")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "odd", "tasks")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "resolves outside the repository")
}

// A symlinked odd/changes cannot become the parity source: the OpenSpec tree
// must never be read as odd input.
func TestCheckPremergeOddRefusesASymlinkedChangeTree(t *testing.T) {
	root := oddRepo(t)
	dir := filepath.Join(root, "openspec", "changes", "thing", "specs", "capability")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(oddDelta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "odd", "changes")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "openspec", "changes"), filepath.Join(root, "odd", "changes")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "symlinked odd change tree")
}

// A plan that declares its tier is graded like an OpenSpec change: the project
// config must exist and the runner it declares must be the one that ran. A plan
// with no Depth: line keeps the narrower #589 contract.
func TestCheckPremergeOddGradesTheConfigForADeclaredTier(t *testing.T) {
	root := oddDeclaredTierRepo(t)

	wantBlocked(t, CheckPremergeOdd(root, "thing"), "config.yaml")

	if err := os.WriteFile(filepath.Join(root, "odd", "config.yaml"),
		[]byte("schema: spec-driven\ntesting:\n  runner: go vet ./internal/...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "never runs the configured testing.runner")

	if err := os.WriteFile(filepath.Join(root, "odd", "config.yaml"),
		[]byte("schema: spec-driven\ntesting:\n  runner: go test ./internal/...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oddWriteArchive(t, root, "thing")
	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK with a matching runner", got)
	}
}

// A declared-tier plan reaches the verification grader: a row with the matching
// runner and a recorded non-zero exit blocks (C3).
func TestCheckPremergeOddGradesTheVerificationRows(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	plan := strings.Replace(readText(filepath.Join(root, "odd", "tasks", "thing.md")),
		"exit 0", "exit 1", 1)
	writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), plan)
	writeFile(t, filepath.Join(root, "odd", "config.yaml"),
		"schema: spec-driven\ntesting:\n  runner: go test ./internal/...\n")
	oddWriteArchive(t, root, "thing")

	wantBlocked(t, CheckPremergeOdd(root, "thing"), "records a failing run (exit 1)")
}

// The declared-tier archive is required: a deleted or never-created archive
// blocks, because archiving always leaves a dated folder holding the plan (F3).
func TestCheckPremergeOddRequiresTheArchiveOfADeclaredTierPlan(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	if err := os.WriteFile(filepath.Join(root, "odd", "config.yaml"),
		[]byte("schema: spec-driven\ntesting:\n  runner: go test ./internal/...\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wantBlocked(t, CheckPremergeOdd(root, "thing"),
		"missing archive at odd/changes/archive/thing/ or a valid dated archive named YYYY-MM-DD-thing/")

	oddWriteArchive(t, root, "thing")
	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK once the dated archive exists", got)
	}
}

// oddDeclaredTierRepo is oddRepo with a valid plan declaring Depth: standard
// and the verification row that names the configured runner.
func oddDeclaredTierRepo(t *testing.T) string {
	t.Helper()
	root := oddRepo(t)
	plan := "# plan\n\nDepth: standard\n\n## Specs\n- **S1.** x — *\"x\"*\n\n" +
		"## Tasks\n| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1 | x | inline | pendiente |\n\n" +
		"## Verification\n- **S1: PASS** — `go test ./internal/...`, exit 0\n\n" +
		"## Log\n- **L1** — x\n"
	if err := os.WriteFile(filepath.Join(root, "odd", "tasks", "thing.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// oddWriteArchive creates the dated archive folder the declared tier requires,
// holding a copy of the plan exactly as `changes archive` does for a delta-less
// change: an empty folder proves nothing (R2).
func oddWriteArchive(t *testing.T, root, change string) {
	t.Helper()
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-"+change)
	mkdirAll(t, archive)
	writeFile(t, filepath.Join(archive, "tasks.md"),
		readText(filepath.Join(root, "odd", "tasks", change+".md")))
}

// An empty archive folder never satisfies the requirement, dated or not: it holds
// nothing of the change (R2).
func TestCheckPremergeOddRejectsAnEmptyArchive(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	writeFile(t, filepath.Join(root, "odd", "config.yaml"),
		"schema: spec-driven\ntesting:\n  runner: go test ./internal/...\n")
	mkdirAll(t, filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing"))

	wantBlocked(t, CheckPremergeOdd(root, "thing"), "is empty: an archive must hold")
}

// After archiving, the archived delta is the only remaining source: an unpromoted
// delta in the archive blocks with the parity wording (R3).
func TestCheckPremergeOddBlocksAnArchivedUnpromotedDelta(t *testing.T) {
	root := oddRepo(t)
	folder := filepath.Join(root, "odd", "changes", "thing")
	writeFile(t, filepath.Join(folder, "specs", "capability", "spec.md"), oddDelta)
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")
	mkdirAll(t, filepath.Dir(archive))
	if err := os.Rename(folder, archive); err != nil {
		t.Fatal(err)
	}

	wantBlocked(t, CheckPremergeOdd(root, "thing"),
		"specs/capability/spec.md is not promoted into odd/specs/capability/spec.md")
}

// A plan without a Depth: line is legacy: no config duty, so existing ODD
// repositories do not retro-block.
func TestCheckPremergeOddLeavesALegacyPlanWithoutADepthLine(t *testing.T) {
	root := oddRepo(t)
	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK for a legacy plan with no Depth: line", got)
	}
}
