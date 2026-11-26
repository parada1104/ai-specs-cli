package gates

// ODD pre-merge gate (plan-build-odd-companion, open decision B): the mode is
// resolved by the caller, the change comes from the branch pairing, and the
// verdict is the artifacts the change carries (S15) plus the parity of its
// deltas. The archive is a duty only of a change that carries a folder (S21), so
// a plan-only or delta-only change has no OpenSpec-style archive requirement.

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

// A change with no plan, no change folder and nothing archived carries no
// artifact: it is the neutral `none` verdict and the guardian passes (S20/S21).
func TestCheckPremergeOddPassesASmallChangeWithNoArtifacts(t *testing.T) {
	root := oddRepo(t)
	if err := os.Remove(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Fatal(err)
	}
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", got)
	}
	if got.Tier != TierNone {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierNone)
	}
	if got.ArchivePath != "" {
		t.Fatalf("ArchivePath = %q, want empty", got.ArchivePath)
	}
}

// S21: the plan of an ODD change stays in odd/tasks/ as the permanent record, so
// a plan-only change owes no archive.
func TestCheckPremergeOddDoesNotRequireAnArchiveForAPlanOnlyChange(t *testing.T) {
	root := oddRepo(t)
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", got)
	}
	if got.Tier != TierOdd {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierOdd)
	}
}

// A directory at the plan path is not a plan: with no artifact at all the change
// is `none`, never a missing-plan blocker.
func TestCheckPremergeOddTreatsADirectoryPlanAsNoPlan(t *testing.T) {
	root := oddRepo(t)
	plan := filepath.Join(root, "odd", "tasks", "thing.md")
	if err := os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plan, 0o755); err != nil {
		t.Fatal(err)
	}
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", got)
	}
	if got.Tier != TierNone {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierNone)
	}
}

// A change that still carries its folder owes an archive before merge.
func TestCheckPremergeOddStillBlocksAnActiveFolder(t *testing.T) {
	root := oddRepo(t)
	mkdirAll(t, filepath.Join(root, "odd", "changes", "thing"))
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "run `ai-specs changes archive`")
}

// S20: a delta is a complete artifact on its own. Once promoted and archived it
// passes with no odd/tasks/<change>.md at all.
func TestCheckPremergeOddAllowsADeltaWithoutAPlan(t *testing.T) {
	root := oddRepo(t)
	if err := os.Remove(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(root, "odd", "changes", "thing")
	writeFile(t, filepath.Join(folder, "specs", "capability", "spec.md"), oddDelta)
	if report := specs.PromoteChangeForMode(root, "thing", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")
	mkdirAll(t, filepath.Dir(archive))
	if err := os.Rename(folder, archive); err != nil {
		t.Fatal(err)
	}

	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK after promotion and archiving", got)
	}
	if got.Tier != TierOdd {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierOdd)
	}
}

// The same delta with no plan blocks while it is unpromoted: the plan is
// optional, the promotion is not.
func TestCheckPremergeOddBlocksADeltaWithoutAPlanWhenUnpromoted(t *testing.T) {
	root := oddRepo(t)
	if err := os.Remove(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "odd", "changes", "thing", "specs", "capability", "spec.md"), oddDelta)
	wantBlocked(t, CheckPremergeOdd(root, "thing"), "is not promoted into odd/specs/capability/spec.md")
}

// An unresolvable branch blocks instead of guessing a change.
func TestCheckPremergeOddBlocksOnDetachedHead(t *testing.T) {
	root := oddRepo(t)
	wantBlocked(t, CheckPremergeOdd(root, ""), "cannot be resolved (detached HEAD)")
	if got := CheckPremergeOdd(root, ""); got.Tier != TierOdd {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierOdd)
	}
}

// A change with no folder but a dated archive is still graded: the archive entry
// is what the change claims to have delivered.
func TestCheckPremergeOddGradesAPlanOnlyChangeThatCarriesAnArchive(t *testing.T) {
	root := oddRepo(t)
	oddWriteArchive(t, root, "thing")
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK once archived", got)
	}
	if got.Tier != TierOdd {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierOdd)
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

// A symlinked tasks directory is not a plan: the plan must be a regular file of
// the repository. Since S21 no plan is owed, so the verdict is the neutral `none`
// instead of a blocker; the tree is still never read through the symlink.
func TestCheckPremergeOddTreatsASymlinkedTasksDirAsNoPlan(t *testing.T) {
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
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", got)
	}
	if got.Tier != TierNone {
		t.Fatalf("Tier = %q, want %q", got.Tier, TierNone)
	}
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

// blocks. Every plan pays the same contract, whatever its `Depth:` line says.
func TestCheckPremergeOddIgnoresAnOddConfig(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	oddWriteArchive(t, root, "thing")

	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK without a config.yaml", got)
	}

	// An odd config that would block if it were read at all, naming a runner no
	// row runs, is dead weight: odd has no config.yaml.
	if err := os.WriteFile(filepath.Join(root, "odd", "config.yaml"),
		[]byte("schema: \ntesting:\n  runner: go vet ./internal/...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK with an odd config.yaml ignored", got)
	}
}

// The odd path grades no verification rows: a non-zero exit is not a blocker.
func TestCheckPremergeOddDoesNotGradeTheVerificationRows(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	plan := strings.Replace(readText(filepath.Join(root, "odd", "tasks", "thing.md")),
		"exit 0", "exit 1", 1)
	writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), plan)
	oddWriteArchive(t, root, "thing")

	if got := CheckPremergeOdd(root, "thing"); !got.OK {
		t.Fatalf("Result = %+v, want OK: the odd contract grades no verification rows", got)
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

// oddWriteArchive creates the dated archive folder the contract requires, holding
// the archived.md marker `changes archive` writes for a delta-less change.
func oddWriteArchive(t *testing.T, root, change string) {
	t.Helper()
	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-"+change)
	mkdirAll(t, archive)
	writeFile(t, filepath.Join(archive, "archived.md"),
		change+"\n2026-01-02\nodd/tasks/"+change+".md\n")
}

// An undated archive is not an ODD archive: the contract requires the dated form
// (C5). The blocker names that form instead of silently accepting the fallback.
func TestCheckPremergeOddRejectsAnUndatedArchive(t *testing.T) {
	root := oddDeclaredTierRepo(t)
	archive := filepath.Join(root, "odd", "changes", "archive", "thing")
	mkdirAll(t, archive)
	writeFile(t, filepath.Join(archive, "archived.md"), "thing\nodd/tasks/thing.md\n")

	wantBlocked(t, CheckPremergeOdd(root, "thing"), "YYYY-MM-DD-thing")
}

// An empty archive folder never satisfies the requirement, dated or not: it holds
// nothing of the change (R2).
func TestCheckPremergeOddRejectsAnEmptyArchive(t *testing.T) {
	root := oddDeclaredTierRepo(t)
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
