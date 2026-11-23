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
	got := CheckPremergeOdd(root, "thing")
	if !got.OK || len(got.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK after promotion", got)
	}
	if got.ArchivePath == "" || got.Tier != TierOdd {
		t.Fatalf("ArchivePath = %q, Tier = %q, want the plan and %q", got.ArchivePath, got.Tier, TierOdd)
	}
}
