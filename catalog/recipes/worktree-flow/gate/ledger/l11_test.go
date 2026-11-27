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

// L11: T5's mixed ledgers/tracker.json is the split's immediate predecessor. It
// still carries each row's status and its close decisions, so it is a migration
// source for the worktree ledger alongside the pre-split state.json.

// writeT5Tracker writes a store exactly as T5 left it: the mixed
// ledgers/tracker.json with status and close decisions on its rows.
func writeT5Tracker(t *testing.T, path string, store LegacyStore) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

// t5MixedStore is a T5 tracker.json: an open provider-backed row for the
// canonical identity, and a closed row whose close decision only exists there.
func t5MixedStore() LegacyStore {
	stamp := t0.Format(time.RFC3339)
	return LegacyStore{V: StoreVersion, Items: []LegacyItem{
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

// TestUpgradeFromT5MixedTrackerKeepsTheLifecycle is blockers 1 and 2: a repository
// upgraded straight from T5 has state.json plus a mixed tracker.json and no
// worktree.json. Its open row must grade, the explicit close must succeed, and
// both closes must end up in worktree.json.
func TestUpgradeFromT5MixedTrackerKeepsTheLifecycle(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(2))
	writeT5Tracker(t, TrackerStorePath(root), t5MixedStore())

	locs := LedgersAt(root)
	view, _, err := locs.Load()
	if err != nil {
		t.Fatalf("Load: %v (the T5 tracker row's status was not read)", err)
	}
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

	out, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("explicit close over a T5 store: %v", err)
	}
	if !out.Applied {
		t.Fatalf("close = %+v, want applied", out)
	}

	worktree := string(rawBytes(t, WorktreeStorePath(root)))
	if !strings.Contains(worktree, `"id": "t5-open"`) || !strings.Contains(worktree, `"status": "closed"`) {
		t.Fatalf("worktree.json does not hold the closed T5 row:\n%s", worktree)
	}
	if !strings.Contains(worktree, `"id": "t5-closed"`) {
		t.Fatalf("worktree.json does not hold the historical T5 row:\n%s", worktree)
	}
	// Four closes: the two state.json history rows, the T5 row that only carried
	// its close in the mixed tracker file, and the new explicit close.
	if got := strings.Count(worktree, `"kind": "close"`); got != 4 {
		t.Fatalf("worktree.json carries %d close decisions, want the 3 inherited and the new one:\n%s", got, worktree)
	}
	reloaded := loadView(t, locs)
	if len(reloaded.Worktree.Items) != 4 {
		t.Fatalf("worktree rows = %d, want the 2 T5 rows plus the 2 historical ones", len(reloaded.Worktree.Items))
	}
}

// TestTrackerWriteRefusesToDropACloseItCannotPlace is blocker 2's refusal half:
// the T5 tracker.json still carries a close, and the worktree ledger cannot be
// written, so the tracker write fails closed instead of persisting a normalized
// tracker file that loses the close.
func TestTrackerWriteRefusesToDropACloseItCannotPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	base := t.TempDir()
	wtDir := filepath.Join(base, "wt")
	trDir := filepath.Join(base, "tr")
	for _, dir := range []string{wtDir, trDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The tracker file carries the close; the worktree file has the row without it.
	writeT5Tracker(t, filepath.Join(trDir, trackerFileName), t5MixedStore())
	stale := `{"v":1,"items":[{"id":"t5-closed","identity":{"common_dir":"/repo/.git","branch":"feat/old","change":""},"status":"closed","decisions":[]}]}` + "\n"
	if err := os.WriteFile(filepath.Join(wtDir, worktreeFileName), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	locs := Ledgers{
		Worktree: StoreLocation{Path: filepath.Join(wtDir, worktreeFileName), Inherit: filepath.Join(trDir, trackerFileName)},
		Tracker:  StoreLocation{Path: filepath.Join(trDir, trackerFileName)},
	}
	before := rawBytes(t, locs.Tracker.Path)

	// The worktree ledger reads fine but cannot be written; the tracker can.
	if err := os.Chmod(wtDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(wtDir, 0o755)

	if _, err := PersistDecision(locs, ident("").Key(), DecisionRequest{
		Checkpoint: CheckpointPreMerge, Kind: DecisionOptOut, Choice: "continue",
	}, vtNow); err == nil {
		t.Fatal("the tracker write succeeded and dropped the close it could not place")
	}
	if got := rawBytes(t, locs.Tracker.Path); string(got) != string(before) {
		t.Fatalf("the refused write rewrote tracker.json:\nbefore=%s\nafter=%s", before, got)
	}
}

// TestSnapshotCloseDoesNotSurviveAFailedTrackerSave is L12.1's second rule: when a
// close carries a snapshot, the tracker save is the second file, so a failure there
// must undo the close in the worktree file rather than leave the close persisted
// without its snapshot.
func TestSnapshotCloseDoesNotSurviveAFailedTrackerSave(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	base := t.TempDir()
	wtDir := filepath.Join(base, "wt")
	trDir := filepath.Join(base, "tr")
	for _, dir := range []string{wtDir, trDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	locs := Ledgers{
		Worktree: StoreLocation{Path: filepath.Join(wtDir, worktreeFileName)},
		Tracker:  StoreLocation{Path: filepath.Join(trDir, trackerFileName)},
	}
	var seeded View
	if _, _, err := seeded.OpenIfAbsent(ident(""), "trello-mcp-workflow", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := locs.Worktree.SaveWorktree(seeded.Worktree); err != nil {
		t.Fatal(err)
	}
	if _, err := locs.Tracker.Save(seeded.Tracker); err != nil {
		t.Fatal(err)
	}
	before := rawBytes(t, locs.Worktree.Path)

	if err := os.Chmod(trDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(trDir, 0o755)

	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose,
		Request: WriteRequest{Kind: WriteClose, State: "done", Provider: []byte(`{"list":"Done"}`)},
	}, vtNow); err == nil {
		t.Fatal("the snapshot close succeeded despite the failed tracker save")
	}
	if after := rawBytes(t, locs.Worktree.Path); string(after) != string(before) {
		t.Fatalf("the close survived without its snapshot:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestT5TrackerFileStillAnswersItsOpenBinding is the supplied regression for
// blocker 1: T5's mixed tracker.json carries the row's status, so an upgrade whose
// worktree file does not exist yet must still select and allow the open binding.
func TestT5TrackerFileStillAnswersItsOpenBinding(t *testing.T) {
	root := t.TempDir()
	writeT5Tracker(t, TrackerStorePath(root), t5MixedStore())

	view, _, err := LedgersAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	item, err := view.Primary(ident("").Key())
	if err != nil {
		t.Fatalf("the T5 open binding does not answer: %v", err)
	}
	if item.ItemID != "card-1" || !item.ProviderBacked() {
		t.Fatalf("primary = %+v, want the T5 provider-backed binding", item)
	}
	got := Grade(Input{
		Checkpoint: CheckpointApplyStart, Mode: ModeAlways, Binding: boundBinding(),
		Identity: availIdentity(), View: view, Evidence: consistentEvidence("card-1"), Now: vtNow,
	})
	assertVerdict(t, "T5 open binding", got, vWant{DecisionAllow, "", 0, SeverityOK, true})
}

// TestFailedBindDoesNotLeaveAnOrphan is the supplied regression for L12.1: when the
// second file of a two-file write cannot be written, the first is restored, so no
// half mutation survives and the retry is not blocked by an orphan row. The tracker
// directory is read-only and has no tracker file, so the read succeeds and only the
// save fails: a directory at the path would make the load fail and prove nothing.
func TestFailedBindDoesNotLeaveAnOrphan(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	base := t.TempDir()
	wtDir := filepath.Join(base, "wt")
	trDir := filepath.Join(base, "tr")
	for _, dir := range []string{wtDir, trDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	trackerPath := filepath.Join(trDir, trackerFileName)
	locs := Ledgers{
		Worktree: StoreLocation{Path: filepath.Join(wtDir, worktreeFileName)},
		Tracker:  StoreLocation{Path: trackerPath},
	}
	req := ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart,
		Request: WriteRequest{Kind: WriteBind, ItemID: "card-1"},
	}
	// The lock file exists, so acquiring it needs no directory write: only the
	// tracker save itself fails.
	if err := os.WriteFile(LockPath(trackerPath), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(trDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := ApplyWrite(locs, req, vtNow); err == nil {
		os.Chmod(trDir, 0o755)
		t.Fatal("the bind succeeded despite the failed tracker save")
	}
	if err := os.Chmod(trDir, 0o755); err != nil {
		t.Fatalf("chmod back: %v", err)
	}
	if _, err := os.Stat(locs.Worktree.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the failed bind left an orphan worktree ledger: %v", err)
	}

	// The retry after the failure must find no orphan and succeed.
	if _, err := ApplyWrite(locs, req, vtNow); err != nil {
		t.Fatalf("the retry after a restored failure: %v", err)
	}
	if _, err := loadView(t, locs).Primary(ident("").Key()); err != nil {
		t.Fatalf("the retry did not leave a usable primary: %v", err)
	}
}

// TestAliasIndependenceOfTwoOverrides is L12.4: two overrides naming different
// canonical files in one directory resolve to the same ledger pair and the same
// lock, so they serialize and never alias.
func TestAliasIndependenceOfTwoOverrides(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ai-specs", "ledgers")
	fromWorktree := LedgersAtStore(filepath.Join(dir, worktreeFileName))
	fromTracker := LedgersAtStore(filepath.Join(dir, trackerFileName))
	if fromWorktree.Worktree.Path != fromTracker.Worktree.Path ||
		fromWorktree.Tracker.Path != fromTracker.Tracker.Path {
		t.Fatalf("the two overrides resolve to different pairs: %+v vs %+v", fromWorktree, fromTracker)
	}
	if fromWorktree.Worktree.Path == fromWorktree.Tracker.Path {
		t.Fatalf("the pair aliases one file: %q", fromWorktree.Worktree.Path)
	}
	if LockPath(fromWorktree.Tracker.Path) != LockPath(fromTracker.Worktree.Path) {
		t.Fatal("two files of one ledger directory take different locks")
	}
}

// TestItemIDsAreUniqueAcrossBothLedgers is blocker 3: T5 bound a row in the same
// second, in the tracker file, while the worktree file was still empty, so an
// immediate candidate open must not reuse that id.
func TestItemIDsAreUniqueAcrossBothLedgers(t *testing.T) {
	root := t.TempDir()
	id := ident("")
	stamp := t0.Format(time.RFC3339)
	taken := NewItemID(id.Key(), stamp)
	writeT5Tracker(t, TrackerStorePath(root), LegacyStore{V: StoreVersion, Items: []LegacyItem{{
		Status: StatusClosed,
		Item:   Item{ID: taken, Identity: id, ItemID: "card-old", ProviderID: "trello-mcp-workflow"},
	}}})

	locs := LedgersAt(root)
	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: id, Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
	}, t0); err != nil {
		t.Fatalf("open: %v", err)
	}

	view := loadView(t, locs)
	// One row per ledger shares its id by design (the join key), so a row may
	// appear at most once in each ledger.
	counts := map[string]int{}
	for _, row := range view.Worktree.Items {
		counts[row.ID]++
		if counts[row.ID] > 1 {
			t.Fatalf("duplicate item id %q in the worktree ledger", row.ID)
		}
	}
	for _, item := range view.Tracker.Items {
		if counts[item.ID] > 1 {
			t.Fatalf("duplicate item id %q across the ledgers", item.ID)
		}
		counts[item.ID]++
	}
	primary, err := view.Primary(id.Key())
	if err != nil {
		t.Fatalf("the fresh open must be the primary: %v", err)
	}
	if primary.ID == taken {
		t.Fatalf("the fresh open reused the id T5 already took in the tracker file: %q", taken)
	}
	if len(view.Worktree.Items) != 2 || len(view.Tracker.Items) != 2 {
		t.Fatalf("rows = %d worktree / %d tracker, want the historical pair plus the fresh pair",
			len(view.Worktree.Items), len(view.Tracker.Items))
	}
}

// TestBareCloseDoesNotCreateTheTrackerLedger pins L12.2's other half: a close with
// no provider payload changes no tracker snapshot, so the tracker ledger is not
// created at all and the close lands only in the worktree ledger.
func TestBareCloseDoesNotCreateTheTrackerLedger(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, LegacyStore{V: StoreVersion, Items: []LegacyItem{{
		Status: StatusOpen,
		Item: Item{
			ID: "legacy-open", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: t0.Format(time.RFC3339), Kind: DecisionOpen}},
		},
	}}})

	locs := LedgersAt(root)
	out, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("bare close: %v", err)
	}
	if !out.Applied {
		t.Fatalf("bare close = %+v, want applied", out)
	}
	if _, err := os.Stat(TrackerStorePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a bare close created the tracker ledger: %v", err)
	}
	worktree := string(rawBytes(t, WorktreeStorePath(root)))
	if !strings.Contains(worktree, `"kind": "close"`) || !strings.Contains(worktree, `"status": "closed"`) {
		t.Fatalf("the close is not in the worktree ledger:\n%s", worktree)
	}
}

// TestExplicitStoreNamedWorktreeDoesNotAliasTheTracker is blocker 4: an explicit
// --store naming the worktree ledger must select the pair in that directory, not
// map both ledgers onto one file.
func TestExplicitStoreNamedWorktreeDoesNotAliasTheTracker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-specs", "ledgers", "worktree.json")
	locs := LedgersAtStore(path)
	if locs.Worktree.Path != path {
		t.Fatalf("worktree path = %q, want %q", locs.Worktree.Path, path)
	}
	if locs.Tracker.Path == path {
		t.Fatalf("the tracker ledger aliases %q", path)
	}

	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart,
		Request: WriteRequest{Kind: WriteBind, ItemID: "card-1"},
	}, vtNow); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := loadView(t, locs).Primary(ident("").Key()); err != nil {
		t.Fatalf("the bind lost the lifecycle: %v", err)
	}
	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
	}, vtNow); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !strings.Contains(string(rawBytes(t, path)), `"kind": "close"`) {
		t.Fatalf("the close is not in the named worktree ledger:\n%s", rawBytes(t, path))
	}
	if strings.Contains(string(rawBytes(t, locs.Tracker.Path)), `"kind": "close"`) {
		t.Fatalf("the close landed in the tracker ledger too:\n%s", rawBytes(t, locs.Tracker.Path))
	}
}

// TestCorruptPreSplitStoreStaysUnevaluableAndBindsNothing is blocker 5: a corrupt
// v1-only store is unevaluable and a bind fails closed, never a synthetic success.
func TestCorruptPreSplitStoreStaysUnevaluableAndBindsNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(StorePath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StorePath(root), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	locs := LedgersAt(root)
	_, _, err := locs.Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("Load error = %v, want a *StoreCorruptError (unevaluable)", err)
	}
	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
	}, vtNow); err == nil {
		t.Fatal("a bind over a corrupt pre-split store succeeded instead of failing closed")
	}
	for _, p := range []string{WorktreeStorePath(root), TrackerStorePath(root)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was written over a corrupt store", p)
		}
	}
}
