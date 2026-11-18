package doctor

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRootWithWitness returns a git project whose ledger witness records the
// given state, the way `ai-specs sync` writes it (even with no tracker at all).
func gitRootWithWitness(t *testing.T, state string) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v: %s", err, out)
	}
	writeFile(t, filepath.Join(root, ".git", "ai-specs", "ledger", "witness.json"),
		`{"candidates":[],"capability":"tracker","recipe_id":"","state":"`+state+`","v":1,"written_at":"2026-11-14T07:01:50Z"}`)
	t.Setenv("WORKTREE_GATE_BIN", "")
	return root
}

// An unbound witness records that no tracker is configured: there is nothing to
// grade, so a project without a gate must not be reported as failing open.
func TestTrackerLedgerUnboundWitnessIsNotInPlay(t *testing.T) {
	d := New(gitRootWithWitness(t, "unbound"), t.TempDir())
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{})
}

// A bound witness keeps the ledger in play: a missing installed CLI is still an ERROR.
func TestTrackerLedgerBoundWitnessStaysInPlay(t *testing.T) {
	d := New(gitRootWithWitness(t, "bound"), t.TempDir())
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{
		{INFO, "tracker-ledger",
			"work-start is unhosted: enable plan-build-flow to grade that checkpoint",
			"enable the plan-build-flow recipe, or grade work-start with an explicit --write"},
		{ERROR, "tracker-ledger",
			"ai-specs CLI is missing or not executable; the tracker ledger is failing open",
			"run ai-specs sync"},
	})
}

// An ambiguous witness (several tracker candidates) also keeps it in play.
func TestTrackerLedgerAmbiguousWitnessStaysInPlay(t *testing.T) {
	d := New(gitRootWithWitness(t, "ambiguous"), t.TempDir())
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{
		{ERROR, "tracker-ledger",
			"ai-specs CLI is missing or not executable; the tracker ledger is failing open",
			"run ai-specs sync"},
	})
}
