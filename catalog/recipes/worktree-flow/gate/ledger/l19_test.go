package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// L19 review regressions: the v1 opt-outs must survive the upgrade, the report must
// name the file it could not read, and a close 0.28.0 wrote is still terminal.

// v1WithOptOut seeds a 0.28.0-era store holding only a human opt-out for ident.
func v1WithOptOut(t *testing.T, root string) ScopedOptOut {
	t.Helper()
	stamp := t0.Format(time.RFC3339)
	optOut := ScopedOptOut{
		Key: ident("").Key(), Checkpoint: CheckpointApplyStart, Scope: ScopeLifecycle,
		Choice: "continue", At: stamp,
	}
	seedLegacy(t, root, LegacyStore{V: 1, OptOuts: []ScopedOptOut{optOut}})
	return optOut
}

// TestStaleV1OptOutSurvivesTheUpgrade is the blocking regression: the most common
// upgrade is v1 with no T5 file at all, and a human opt-out must still answer.
func TestStaleV1OptOutSurvivesTheUpgrade(t *testing.T) {
	root := t.TempDir()
	optOut := v1WithOptOut(t, root)

	file, _, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(file.Tracker.OptOuts) != 1 || file.Tracker.OptOuts[0].Key != optOut.Key {
		t.Fatalf("opt_outs = %+v, want the v1 opt-out", file.Tracker.OptOuts)
	}
	if !file.View().HasScopedOptOut(optOut.Key, CheckpointApplyStart) {
		t.Fatal("the view does not answer the v1 opt-out")
	}
	got := Grade(Input{
		Checkpoint: CheckpointApplyStart, Mode: ModeAlways, Binding: boundBinding(),
		Identity: availIdentity(), View: file.View(), Now: vtNow,
	})
	if got.Decision != DecisionAllow || got.Reason != ReasonOptOut {
		t.Fatalf("grade = %q/%q, want allow/opt-out (0.28.0 answered opt-out here)", got.Decision, got.Reason)
	}

	// The first write must carry the opt-out into the v2 file, not lose it.
	if _, err := PersistDecision(LedgerAt(root), ident("other").Key(), DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	reloaded, _, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Tracker.OptOuts) != 2 {
		t.Fatalf("opt_outs after the write = %+v, want the v1 decision and the new one", reloaded.Tracker.OptOuts)
	}
	if !reloaded.View().HasScopedOptOut(optOut.Key, CheckpointApplyStart) {
		t.Fatal("the v1 opt-out was lost by the first write")
	}
}

// TestUnreadableMigrationSourceIsNamedCorrectly is the cheap low: the note must name
// the file that is actually unreadable, not the ledger the reader fell back to.
func TestUnreadableMigrationSourceIsNamedCorrectly(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, LegacyStore{V: 1, Items: closedHistory(2).Items})
	if err := os.MkdirAll(filepath.Dir(TrackerStorePath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(TrackerStorePath(root), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("the unreadable source was not reported")
	}
	if report.LegacyRows != TrackerStorePath(root) {
		t.Fatalf("report names %q, want the unreadable %q", report.LegacyRows, TrackerStorePath(root))
	}
}

// TestCloseWrittenAfterTheT5CopyIsStillTerminal is the other cheap low: T5 copied the
// row while it was open, then 0.28.0 closed it in v1. The newer file wins for the
// record, but a close is terminal, so the row must be closed.
func TestCloseWrittenAfterTheT5CopyIsStillTerminal(t *testing.T) {
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	seedLegacy(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusClosed,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-A", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: stamp, Kind: DecisionOpen},
				{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose},
			},
		},
	}}})
	seedT5(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusOpen,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-B", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
		},
	}}})

	view := loadView(t, LedgerAt(root))
	if _, err := view.Primary(ident("").Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("the row came back selectable although 0.28.0 closed it (err = %v)", err)
	}
	closed, ok := view.LatestClosed(ident("").Key())
	if !ok {
		t.Fatal("the row is not closed although a close exists in the v1 predecessor")
	}
	if closed.ItemID != "card-B" {
		t.Fatalf("closed row carries %q, want the newer T5 record card-B", closed.ItemID)
	}
}
