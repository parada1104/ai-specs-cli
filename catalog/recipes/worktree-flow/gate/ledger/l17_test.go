package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// L17 review regressions: the newest form wins, a close is terminal, backups hold the
// bytes they replace, a corrupt migration source never invalidates the ledger.

// TestT5TrackerFileWinsOverStaleV1 is Cody's exact ALTO-1 scenario: 0.28.0 bound the
// row with card-A, then T5 re-linked it to card-B and closed it. The stale v1 file must
// not answer for that row.
func TestT5TrackerFileWinsOverStaleV1(t *testing.T) {
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	seedLegacy(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusOpen,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-A", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
		},
	}}})
	seedT5(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusClosed,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-B", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: stamp, Kind: DecisionOpen},
				{At: stamp, Checkpoint: CheckpointApplyStart, Kind: DecisionLink},
				{At: stamp, Checkpoint: CheckpointApplyStart, Kind: DecisionLink},
				{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose},
			},
		},
	}}})

	view := loadView(t, LedgerAt(root))
	if _, err := view.Primary(ident("").Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("the T5-closed row came back selectable (err = %v)", err)
	}
	closed, ok := view.LatestClosed(ident("").Key())
	if !ok {
		t.Fatal("the T5-closed row is not the latest closed row")
	}
	if closed.ItemID != "card-B" {
		t.Fatalf("closed row carries %q, want the T5 link card-B", closed.ItemID)
	}
	kinds := make([]string, 0, len(closed.Decisions))
	for _, d := range closed.Decisions {
		kinds = append(kinds, d.Kind)
	}
	if got, want := strings.Join(kinds, ","), "open,link,link,close"; got != want {
		t.Fatalf("decision history = %q, want the T5 history %q", got, want)
	}
	grade := Grade(Input{
		Checkpoint: CheckpointApplyStart, Mode: ModeWarn, Binding: boundBinding(),
		Identity: availIdentity(), View: view, Now: vtNow,
	})
	if grade.Decision == DecisionAllow && grade.Item != nil && grade.Item.ID == "row" {
		t.Fatalf("grade allowed the closed T5 row as if open: %+v", grade)
	}

	// A further close must not duplicate the close decision.
	out, err := ApplyWrite(LedgerAt(root), ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("close after the T5 close: %v", err)
	}
	if out.Applied || out.Reason != WriteReasonAlreadyClosed {
		t.Fatalf("close = %+v, want applied=false/already-closed", out)
	}
	after := loadFile(t, LedgerAt(root))
	closes := 0
	for _, row := range after.Worktree.Items {
		for _, d := range row.Decisions {
			if d.Kind == DecisionClose {
				closes++
			}
		}
	}
	if closes != 1 {
		t.Fatalf("close decisions = %d, want the single T5 close (no duplicate)", closes)
	}
}

// TestAbsorbedCloseClosesTheRow is ALTO-2: a close is terminal, so any row carrying
// one is closed whatever the source order.
func TestAbsorbedCloseClosesTheRow(t *testing.T) {
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	seedLegacy(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusOpen,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: stamp, Kind: DecisionOpen},
				{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose},
			},
		},
	}}})

	row := loadFile(t, LedgerAt(root)).Worktree.Items[0]
	if row.Status != StatusClosed {
		t.Fatalf("status = %q, want closed: a carried close is terminal (D17)", row.Status)
	}
}

// TestBackupKeepsTheBytesItReplaces is MEDIO-2: a stale backup from an earlier attempt
// must not stand for bytes that have since grown.
func TestBackupKeepsTheBytesItReplaces(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(15))
	if err := os.WriteFile(legacy+".v1-backup", []byte("stale 14-row backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := StoreAt(legacy).Save(EmptyLedger(), SectionWorktree, SectionTracker, SectionPlan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fresh := false
	entries, err := os.ReadDir(filepath.Dir(legacy))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), "v1-backup") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(legacy), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == string(legacyBytes) {
			fresh = true
		}
	}
	if !fresh {
		t.Fatal("no backup holds the bytes actually replaced (the stale one was reused)")
	}
	if data, err := os.ReadFile(legacy + ".v1-backup"); err != nil || string(data) != "stale 14-row backup\n" {
		t.Fatalf("the earlier backup was destroyed (%v)", err)
	}
}

// TestWriteFailsClosedWhenTheBackupCannotBeWritten is MEDIO-2's second half: the write
// must not proceed while claiming the predecessor is safe.
func TestWriteFailsClosedWhenTheBackupCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	root := t.TempDir()
	seedT5(t, root, closedHistory(3))
	t5Dir := filepath.Dir(TrackerStorePath(root))
	if err := os.Chmod(t5Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(t5Dir, 0o755)

	if _, err := LedgerAt(root).Save(EmptyLedger(), SectionWorktree, SectionTracker, SectionPlan); err == nil {
		t.Fatal("Save wrote the ledger although the predecessor backup could not be written")
	}
	if _, err := os.Stat(StorePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the ledger was written without a backup: %v", err)
	}
}

// TestCorruptMigrationSourceStillLetsTheLedgerAnswer is MEDIO-3: a corrupt T5 file is
// reported, and the v2 file answers (cleanup must stay able to close).
func TestCorruptMigrationSourceStillLetsTheLedgerAnswer(t *testing.T) {
	root := t.TempDir()
	file := EmptyLedger()
	stamp := t0.Format(time.RFC3339)
	file.Worktree.Items = append(file.Worktree.Items, WorktreeItem{
		ID: "row", Identity: ident(""), Status: StatusOpen,
		Decisions: []WorktreeDecision{{At: stamp, Kind: DecisionOpen}},
	})
	file.Tracker.Items = append(file.Tracker.Items, Item{
		ID: "row", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
		Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
	})
	seedV2(t, StorePath(root), file)
	if err := os.MkdirAll(filepath.Dir(TrackerStorePath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(TrackerStorePath(root), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v, want the ledger to answer despite the corrupt source", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("the unreadable migration source was not reported")
	}
	if _, err := loaded.View().Primary(ident("").Key()); err != nil {
		t.Fatalf("the ledger does not answer its own row: %v", err)
	}
	out, _, err := CloseBranchItem(LedgerAt(root), ident("").CommonDir, ident("").Branch, CheckpointArchiveClose, vtNow)
	if err != nil {
		t.Fatalf("cleanup close over a corrupt source: %v", err)
	}
	if !out.Applied {
		t.Fatalf("close = %+v, want applied", out)
	}
}

// TestRowDifferingInBothSectionsIsOneShadow is the LOW finding: one row is one row.
func TestRowDifferingInBothSectionsIsOneShadow(t *testing.T) {
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	file := EmptyLedger()
	file.Worktree.Items = append(file.Worktree.Items, WorktreeItem{
		ID: "row", Identity: ident(""), Status: StatusClosed,
		Decisions: []WorktreeDecision{{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose}},
	})
	file.Tracker.Items = append(file.Tracker.Items, Item{
		ID: "row", Identity: ident(""), ItemID: "card-ledger", ProviderID: "trello-mcp-workflow",
		Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
	})
	seedV2(t, StorePath(root), file)
	seedT5(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusOpen,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-source", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
		},
	}}})

	_, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Shadowed != 1 {
		t.Fatalf("shadowed = %d, want 1: one row differing in two sections is one row", report.Shadowed)
	}
}

// TestCloseKeepsItsChoiceNoteAndScope is the LOW finding about WorktreeDecision: a
// carried close must not silently lose the fields the source recorded.
func TestCloseKeepsItsChoiceNoteAndScope(t *testing.T) {
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	seedLegacy(t, root, LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusClosed,
		Item: Item{
			ID: "row", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: stamp, Kind: DecisionOpen},
				{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose, Choice: "local", Note: "merged", Scope: ScopeLifecycle},
			},
		},
	}}})

	row := loadFile(t, LedgerAt(root)).Worktree.Items[0]
	if len(row.Decisions) != 1 {
		t.Fatalf("decisions = %+v, want the single close", row.Decisions)
	}
	closeDecision := row.Decisions[0]
	if closeDecision.Choice != "local" || closeDecision.Note != "merged" || closeDecision.Scope != ScopeLifecycle {
		t.Fatalf("carried close = %+v, want choice/note/scope preserved", closeDecision)
	}
	// The host contract's joined history keeps them too.
	joined := loadView(t, LedgerAt(root)).LatestClosed
	item, ok := joined(ident("").Key())
	if !ok {
		t.Fatal("no closed row")
	}
	if item.Decisions[len(item.Decisions)-1].Note != "merged" {
		t.Fatalf("joined close = %+v, want the note preserved", item.Decisions)
	}
}
