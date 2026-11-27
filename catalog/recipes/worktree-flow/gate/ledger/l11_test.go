package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The T5 migration and the invariants it established, on the one-file design (L14):
// T5's mixed tracker file is absorbed unconditionally, no row is lost, no source is
// rewritten, and ids stay unique across sections.

// t5MixedStore is a T5 tracker.json: an open provider-backed row for the canonical
// identity, and a closed row whose close decision only exists there.
func t5MixedStore() LegacyStore {
	stamp := t0.Format(time.RFC3339)
	return LegacyStore{V: 1, Items: []LegacyItem{
		{Status: StatusOpen, Item: Item{
			ID: "t5-open", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
		}},
		{Status: StatusClosed, Item: Item{
			ID: "t5-closed", Identity: ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/old"},
			ItemID: "card-0", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: stamp, Kind: DecisionOpen},
				{At: stamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose},
			},
		}},
	}}
}

// TestUpgradeFromT5MixedTrackerKeepsTheLifecycle is the T5 upgrade regression: a v2
// file absent, a v1 file and T5's tracker file present, and no row lost.
func TestUpgradeFromT5MixedTrackerKeepsTheLifecycle(t *testing.T) {
	root := t.TempDir()
	legacyBytes := writeLegacyFixture(t, StorePath(root), closedHistory(2))
	t5Bytes := writeLegacyFixture(t, TrackerStorePath(root), t5MixedStore())
	loc := LedgerAt(root)

	view := loadView(t, loc)
	item, err := view.Primary(ident("").Key())
	if err != nil {
		t.Fatalf("the T5 open row is not primary: %v", err)
	}
	if item.ItemID != "card-1" || !item.ProviderBacked() {
		t.Fatalf("primary = %+v, want the T5 provider-backed row", item)
	}
	grade := Grade(Input{
		Checkpoint: CheckpointPreMerge, Mode: ModeAlways, Binding: boundBinding(),
		Identity: availIdentity(), View: view, Evidence: consistentEvidence("card-1"), Now: vtNow,
	})
	if grade.Decision != DecisionAllow {
		t.Fatalf("pre-close grade = %q/%q, want allow", grade.Decision, grade.Reason)
	}

	out, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("explicit close over a T5 store: %v", err)
	}
	if !out.Applied {
		t.Fatalf("close = %+v, want applied", out)
	}

	file := loadFile(t, loc)
	if len(file.Worktree.Items) != 4 {
		t.Fatalf("worktree rows = %d, want the 2 v1 rows plus the 2 T5 rows", len(file.Worktree.Items))
	}
	closes := 0
	for _, row := range file.Worktree.Items {
		for _, d := range row.Decisions {
			if d.Kind == DecisionClose {
				closes++
			}
		}
	}
	// The two v1 history closes, the T5 close, and the new close.
	if closes != 4 {
		t.Fatalf("close decisions = %d, want the 3 inherited and the new one", closes)
	}
	for _, item := range file.Tracker.Items {
		for _, d := range item.Decisions {
			if d.Kind == DecisionClose {
				t.Fatalf("the tracker section carries a close decision: %+v", item.Decisions)
			}
		}
	}
	if got := rawBytes(t, StorePath(root)+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the v1 predecessor was not backed up")
	}
	if got := rawBytes(t, TrackerStorePath(root)); string(got) != string(t5Bytes) {
		t.Fatal("the T5 predecessor was rewritten")
	}
}

// TestT5TrackerFileStillAnswersItsOpenBinding is the supplied regression: a T5 file
// holding only an open row must still answer, before any v2 file exists.
func TestT5TrackerFileStillAnswersItsOpenBinding(t *testing.T) {
	root := t.TempDir()
	writeLegacyFixture(t, TrackerStorePath(root), LegacyStore{V: 1, Items: []LegacyItem{
		{Status: StatusOpen, Item: Item{
			ID: "t5-open", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: matrixStamp, Kind: DecisionOpen}},
		}},
	}})

	view := loadView(t, LedgerAt(root))
	item, err := view.Primary(ident("").Key())
	if err != nil {
		t.Fatalf("the T5 open binding does not answer: %v", err)
	}
	if item.ItemID != "card-1" {
		t.Fatalf("primary = %+v, want the T5 binding", item)
	}
	// A tracker-only mutation materialises the lifecycle into the v2 file.
	if _, err := PersistDecision(LedgerAt(root), ident("").Key(), DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue",
	}, vtNow); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	if _, err := loadView(t, LedgerAt(root)).Primary(ident("").Key()); err != nil {
		t.Fatalf("the migrated binding is not primary after the write: %v", err)
	}
}

// TestFailedBindDoesNotLeaveAnOrphan keeps the supplied regression's assertion on one
// file: a bind that cannot write persists nothing, and the retry is not blocked.
func TestFailedBindDoesNotLeaveAnOrphan(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	loc := StoreAt(filepath.Join(blocker, storeFileName))
	req := ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart,
		Request: WriteRequest{Kind: WriteBind, ItemID: "card-1"},
	}
	if _, err := ApplyWrite(loc, req, vtNow); err == nil {
		t.Fatal("the bind succeeded despite the unwritable path")
	}
	if _, err := os.ReadFile(loc.Path); err == nil {
		t.Fatal("the failed bind created a ledger")
	}

	// The retry on a usable path finds no orphan and succeeds.
	loc = StoreAt(ledgerPath(t))
	if _, err := ApplyWrite(loc, req, vtNow); err != nil {
		t.Fatalf("the retry after a restored failure: %v", err)
	}
	if _, err := loadView(t, loc).Primary(ident("").Key()); err != nil {
		t.Fatalf("the retry did not leave a usable primary: %v", err)
	}
}

// TestItemIDsAreUniqueAcrossSections pins that an id taken in the tracker section is
// never reused by a fresh worktree row.
func TestItemIDsAreUniqueAcrossSections(t *testing.T) {
	root := t.TempDir()
	id := ident("")
	stamp := t0.Format(time.RFC3339)
	taken := NewItemID(id.Key(), stamp)
	writeLegacyFixture(t, TrackerStorePath(root), LegacyStore{V: 1, Items: []LegacyItem{{
		Status: StatusClosed,
		Item:   Item{ID: taken, Identity: id, ItemID: "card-old", ProviderID: "trello-mcp-workflow"},
	}}})

	loc := LedgerAt(root)
	if _, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: id, Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
	}, t0); err != nil {
		t.Fatalf("open: %v", err)
	}

	file := loadFile(t, loc)
	// One row per section shares its id by design (the join key), so a row may appear
	// at most once in each section.
	inWorktree := map[string]bool{}
	for _, row := range file.Worktree.Items {
		if inWorktree[row.ID] {
			t.Fatalf("duplicate item id %q in the worktree section", row.ID)
		}
		inWorktree[row.ID] = true
	}
	seenTracker := map[string]bool{}
	for _, item := range file.Tracker.Items {
		if seenTracker[item.ID] {
			t.Fatalf("duplicate item id %q in the tracker section", item.ID)
		}
		seenTracker[item.ID] = true
	}
	primary, err := file.View().Primary(id.Key())
	if err != nil {
		t.Fatalf("the fresh open must be the primary: %v", err)
	}
	if primary.ID == taken {
		t.Fatalf("the fresh open reused the id the predecessor already took: %q", taken)
	}
}

// TestBareCloseDoesNotAddATrackerRow pins that a close with no provider payload
// leaves the tracker section untouched, so the close decision lives only in the
// worktree section.
func TestBareCloseDoesNotAddATrackerRow(t *testing.T) {
	loc := openFixture("card-1").write(t, ledgerPath(t))
	trackerBefore := mustMarshal(t, loadFile(t, loc).Tracker)

	out, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("bare close: %v", err)
	}
	if !out.Applied {
		t.Fatalf("bare close = %+v, want applied", out)
	}
	file := loadFile(t, loc)
	if got := mustMarshal(t, file.Tracker); got != trackerBefore {
		t.Fatalf("a bare close changed the tracker section:\nbefore=%s\nafter=%s", trackerBefore, got)
	}
	if row := file.Worktree.Items[0]; row.Status != StatusClosed {
		t.Fatalf("status = %q, want closed", row.Status)
	}
}

// TestCorruptPreSplitStoreStaysUnevaluableAndBindsNothing pins that a corrupt v1
// file is never read as empty and a bind fails closed.
func TestCorruptPreSplitStoreStaysUnevaluableAndBindsNothing(t *testing.T) {
	loc := StoreAt(ledgerPath(t))
	if err := os.MkdirAll(filepath.Dir(loc.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loc.Path, []byte(`{"v":1,"items":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := loc.Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("Load error = %v, want a *StoreCorruptError (unevaluable)", err)
	}
	if _, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
	}, vtNow); err == nil {
		t.Fatal("a bind over a corrupt predecessor succeeded instead of failing closed")
	}
	if got := string(rawBytes(t, loc.Path)); !strings.Contains(got, `"items":[`) {
		t.Fatalf("the corrupt file was rewritten: %s", got)
	}
}

// TestSnapshotCloseRecordsTheObservedProviderState pins S1 on one file: a close with
// a provider payload updates the tracker row's snapshot in the same atomic write.
func TestSnapshotCloseRecordsTheObservedProviderState(t *testing.T) {
	loc := openFixture("card-1").write(t, ledgerPath(t))
	if _, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose,
		Request: WriteRequest{Kind: WriteClose, State: "done", Provider: []byte(`{"list":"Done"}`)},
	}, vtNow); err != nil {
		t.Fatalf("close with payload: %v", err)
	}
	file := loadFile(t, loc)
	if file.Tracker.Items[0].State != "done" {
		t.Fatalf("state = %q, want the observed done state", file.Tracker.Items[0].State)
	}
	var provider map[string]any
	if err := json.Unmarshal(file.Tracker.Items[0].Provider, &provider); err != nil {
		t.Fatal(err)
	}
	if provider["list"] != "Done" {
		t.Fatalf("provider = %v, want list=Done", provider)
	}
}
