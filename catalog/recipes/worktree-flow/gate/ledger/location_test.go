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

// closedHistory builds a pre-split store shaped like the real one this repo
// carries: only closed rows, no opt-outs. The rows stand for the historical
// decisions S11 requires to survive the migration untouched.
func closedHistory(n int) LegacyStore {
	store := LegacyStore{V: StoreVersion}
	for i := 0; i < n; i++ {
		branch := "feat/history-" + string(rune('a'+i))
		store.Items = append(store.Items, LegacyItem{
			Status: StatusClosed,
			Item: Item{
				ID:       "item-" + string(rune('a'+i)),
				Identity: ItemIdentity{CommonDir: "/repo/.git", Branch: branch},
				ItemID:   "card-" + string(rune('a'+i)),
				Decisions: []Decision{{
					At: "2026-10-08T14:40:02Z", Checkpoint: CheckpointArchiveClose,
					Kind: DecisionClose, Choice: "local", Note: "merged",
				}},
			},
		})
	}
	return store
}

// trackerRows is the tracker ledger's share of a pre-split store: what that
// ledger writes for the rows it owns, with no status column and no close
// decision (L8).
func trackerRows(legacy LegacyStore) Store {
	store := Store{V: StoreVersion}
	for _, row := range legacy.Items {
		store.Items = append(store.Items, trackerShare(row))
	}
	store.OptOuts = legacy.OptOuts
	return store
}

// seedLegacy writes store at the pre-split location under root and returns the
// path and the exact bytes on disk.
func seedLegacy(t *testing.T, root string, store LegacyStore) (string, []byte) {
	t.Helper()
	path := StorePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	// Written exactly as an older ai-specs wrote it: this build only reads it.
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy store: %v", err)
	}
	if err := writeAtomic(path, append(data, '\n')); err != nil {
		t.Fatalf("seed legacy store: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy store: %v", err)
	}
	return path, written
}

// itemsOf returns the exact on-disk bytes of the items array, so a test can pin
// "byte-identical rows" instead of "equal after decoding".
func itemsOf(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return raw.Items
}

func TestTrackerStorePathIsTheSplitLayout(t *testing.T) {
	got := TrackerStorePath("/repo/.git")
	want := filepath.Join("/repo/.git", "ai-specs", "ledgers", "tracker.json")
	if got != want {
		t.Fatalf("TrackerStorePath = %q, want %q", got, want)
	}
}

func TestTrackerStoreReadsTheLegacyStoreBeforeTheFirstDecision(t *testing.T) {
	root := t.TempDir()
	legacy, _ := seedLegacy(t, root, closedHistory(14))
	split := TrackerStorePath(root)

	store, _, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.Items) != 14 {
		t.Fatalf("items = %d, want the 14 legacy rows", len(store.Items))
	}
	if _, err := os.Stat(split); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading the legacy store wrote %s", split)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy store went missing on a read: %v", err)
	}
}

func TestTrackerStoreHandsTheLegacyRowsForwardOnTheFirstDecision(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(14))
	split := TrackerStorePath(root)
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/fresh"}.Key()

	_, err := PersistDecision(LedgersAt(root), key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now())
	if err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}

	if got, err := os.ReadFile(legacy); err != nil || string(got) != string(legacyBytes) {
		t.Fatalf("the pre-split store was rewritten (%v); it must stay readable and unmodified", err)
	}
	splitBytes, err := os.ReadFile(split)
	if err != nil {
		t.Fatalf("the first decision did not land in the split store: %v", err)
	}
	// The tracker ledger carries its own share of every historical row: the
	// fields it owns, unchanged, and nothing the worktree ledger owns.
	var gotRows []Item
	if err := json.Unmarshal(itemsOf(t, split), &gotRows); err != nil {
		t.Fatalf("decode the split rows: %v", err)
	}
	wantRows := trackerRows(closedHistory(14)).Items
	if len(gotRows) != len(wantRows) {
		t.Fatalf("rows = %d, want %d", len(gotRows), len(wantRows))
	}
	for i := range wantRows {
		// Compared as the ledger compares rows, so a JSON null and an absent
		// value are the same row.
		if !sameJSON(trackerShare(LegacyItem{Item: gotRows[i]}), wantRows[i]) {
			t.Fatalf("row %d is not the historical share:\n got %+v\nwant %+v", i, gotRows[i], wantRows[i])
		}
	}

	store, _, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.Items) != 14 {
		t.Fatalf("items = %d, want the 14 historical rows", len(store.Items))
	}
	worktree, _, err := WorktreeLocation(root).LoadWorktree()
	if err != nil {
		t.Fatalf("LoadWorktree: %v", err)
	}
	if len(worktree.Items) != 14 {
		t.Fatalf("worktree rows = %d, want the 14 historical rows", len(worktree.Items))
	}
	for _, row := range worktree.Items {
		if row.Status != StatusClosed {
			t.Fatalf("row %q is %q: a historical row was revived as live state", row.ID, row.Status)
		}
	}
	for _, item := range store.Items {
		for _, d := range item.Decisions {
			if d.Kind == DecisionClose {
				t.Fatalf("row %q carries a close decision in the tracker ledger", item.ID)
			}
		}
	}
	if len(store.OptOuts) != 1 {
		t.Fatalf("opt_outs = %d, want the 1 decision that landed", len(store.OptOuts))
	}
	var header struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(splitBytes, &header); err != nil || header.V != StoreVersion {
		t.Fatalf("split store version = %d (%v), want %d", header.V, err, StoreVersion)
	}

	backup, err := os.ReadFile(legacy + ".v1-backup")
	if err != nil {
		t.Fatalf("the first handoff kept no backup of the legacy rows: %v", err)
	}
	if string(backup) != string(legacyBytes) {
		t.Fatalf("backup != legacy store")
	}
	// A second decision must not overwrite the one-shot backup.
	if err := os.WriteFile(legacy, []byte("\n"), 0o600); err != nil {
		t.Fatalf("rewrite legacy: %v", err)
	}
	if _, err := PersistDecision(LedgersAt(root), key+"-2", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("second PersistDecision: %v", err)
	}
	backup, err = os.ReadFile(legacy + ".v1-backup")
	if err != nil || string(backup) != string(legacyBytes) {
		t.Fatalf("the backup was replaced on a later write (%v); it is one-shot", err)
	}
}

// The split store wins for every row it already has — its version is the one that
// answers — and the rows only the pre-split store has are added, so an older
// binary's decisions are neither lost nor able to overwrite the newer file.
func TestSplitRowsWinAndLegacyOnlyRowsAreAdded(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(14))
	splitRows := trackerRows(closedHistory(2))
	splitRows.Items[0].ItemID = "from-the-split-store"
	if _, err := TrackerLocation(root).Save(splitRows); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store, report, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.Items) != 14 {
		t.Fatalf("items = %d, want the split store's 2 plus the 12 rows only the pre-split store has", len(store.Items))
	}
	if report.Absorbed != 12 {
		t.Fatalf("absorbed = %d, want the 12 rows the split store lacks", report.Absorbed)
	}
	if store.Items[0].ItemID != "from-the-split-store" {
		t.Fatalf("row %q carries %q, want the split store's version", store.Items[0].ID, store.Items[0].ItemID)
	}
}

func TestTrackerStoreReportsACorruptSplitStoreWithoutResurrectingLegacyRows(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(14))
	split := TrackerStorePath(root)
	if err := os.MkdirAll(filepath.Dir(split), 0o755); err != nil {
		t.Fatalf("mkdir split: %v", err)
	}
	if err := os.WriteFile(split, []byte(`{"v":1,"items":[`), 0o600); err != nil {
		t.Fatalf("corrupt split store: %v", err)
	}

	store, _, err := TrackerLocation(root).Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("error = %v, want a *StoreCorruptError", err)
	}
	if corrupt.Path != split {
		t.Fatalf("corrupt path = %q, want %q", corrupt.Path, split)
	}
	if len(store.Items) != 0 {
		t.Fatalf("items = %d: the legacy rows came back as live state", len(store.Items))
	}
}

func TestTrackerStoreReportsACorruptLegacyStore(t *testing.T) {
	root := t.TempDir()
	legacy := StorePath(root)
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(legacy, []byte("not json"), 0o600); err != nil {
		t.Fatalf("corrupt legacy store: %v", err)
	}

	store, _, err := TrackerLocation(root).Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("error = %v, want a *StoreCorruptError, never an empty store", err)
	}
	if corrupt.Path != legacy {
		t.Fatalf("corrupt path = %q, want the legacy %q", corrupt.Path, legacy)
	}
	if len(store.Items) != 0 {
		t.Fatalf("items = %d, want none from a corrupt store", len(store.Items))
	}
}

func TestExplicitStorePathHasNoLegacyFallback(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "state.json")
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/explicit"}.Key()

	if _, err := PersistDecision(LedgersAtStore(explicit), key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	if _, err := os.Stat(explicit); err != nil {
		t.Fatalf("explicit store was not written: %v", err)
	}
	for _, name := range []string{"state.json.v1-backup", StorePath(root) + ".v1-backup"} {
		if _, err := os.Stat(filepath.Join(root, filepath.Base(name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("explicit store path grew the backup sibling %q", name)
		}
	}
	if _, err := os.Stat(StorePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("an explicit store path wrote the pre-split store")
	}
}

// TestExplicitStoreUnderALedgersDirKeepsNoLegacyFallback pins the native finding:
// an explicitly named file is not a split store just because its directory is
// called `ledgers`. Nothing may be copied into it from a pre-split sibling, it
// must not grow the one-shot backup, and a corrupt pre-split sibling must not
// break it.
func TestExplicitStoreUnderALedgersDirKeepsNoLegacyFallback(t *testing.T) {
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/explicit"}.Key()
	cases := map[string]func(t *testing.T, root, legacy string){
		"existing legacy rows": func(t *testing.T, root, legacy string) {
			seedLegacy(t, root, closedHistory(14))
		},
		"corrupt legacy sibling": func(t *testing.T, root, legacy string) {
			if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
				t.Fatalf("mkdir legacy: %v", err)
			}
			if err := os.WriteFile(legacy, []byte("not json"), 0o600); err != nil {
				t.Fatalf("corrupt legacy: %v", err)
			}
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			legacy := StorePath(root)
			seed(t, root, legacy)
			explicit := filepath.Join(root, "ai-specs", "ledgers", "tracker.json")

			if _, err := PersistDecision(LedgersAtStore(explicit), key, DecisionRequest{
				Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
			}, time.Now()); err != nil {
				t.Fatalf("PersistDecision: %v", err)
			}

			store, err := LoadStore(explicit)
			if err != nil {
				t.Fatalf("the explicit store was not written: %v", err)
			}
			if len(store.Items) != 0 {
				t.Fatalf("items = %d: pre-split rows were copied into the explicit store", len(store.Items))
			}
			if len(store.OptOuts) != 1 {
				t.Fatalf("opt_outs = %d, want only the decision that landed", len(store.OptOuts))
			}
			if _, err := os.Stat(legacy + ".v1-backup"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the explicit store path grew the backup %q", legacy+".v1-backup")
			}
		})
	}
}

// An ai-specs older than this build — the installed CLI the hooks still run until
// the release — keeps writing the pre-split store after the handoff. Its rows are
// real decisions: dropping them would ignore a human opt-out or let a second item
// be opened for an identity that already has one.
func TestTrackerStoreAbsorbsRowsAnOlderBinaryWroteAfterTheHandoff(t *testing.T) {
	root := t.TempDir()
	legacy, _ := seedLegacy(t, root, closedHistory(14))
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/fresh"}.Key()

	if _, err := PersistDecision(LedgersAt(root), key+"-1", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("first PersistDecision: %v", err)
	}

	// The older binary opens a row and records an opt-out in the pre-split store.
	old, err := LoadLegacyStore(legacy)
	if err != nil {
		t.Fatalf("LoadLegacyStore(legacy): %v", err)
	}
	old.Items = append(old.Items, LegacyItem{
		Status: StatusOpen,
		Item: Item{
			ID:       "row-from-the-old-binary",
			Identity: ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/old"},
			ItemID:   "card-old",
		},
	})
	old.OptOuts = append(old.OptOuts, ScopedOptOut{
		Key: key + "-old", Checkpoint: CheckpointApplyStart, Scope: ScopeLifecycle,
		Choice: "local", At: "2026-11-27T00:00:00Z",
	})
	oldBytes, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := writeAtomic(legacy, append(oldBytes, '\n')); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	legacyBytes, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatalf("read legacy: %v", err)
	}

	store, report, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Absorbed != 2 {
		t.Fatalf("absorbed = %d, want the 1 row and the 1 opt-out the split store lacks", report.Absorbed)
	}
	if report.LegacyRows != legacy {
		t.Fatalf("report names %q, want %q", report.LegacyRows, legacy)
	}
	if len(store.Items) != 15 {
		t.Fatalf("items = %d, want the 14 historical rows plus the one the older binary opened", len(store.Items))
	}
	if len(store.OptOuts) != 2 {
		t.Fatalf("opt_outs = %d, want this build's decision and the older binary's", len(store.OptOuts))
	}
	worktree, _, err := WorktreeLocation(root).LoadWorktree()
	if err != nil {
		t.Fatalf("LoadWorktree: %v", err)
	}
	open := worktree.OpenItems(ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/old"}.Key())
	if len(open) != 1 || open[0].ID != "row-from-the-old-binary" {
		t.Fatalf("the older binary's row is not visible to the read: %+v", open)
	}
	found := false
	for _, item := range store.Items {
		if item.ID == "row-from-the-old-binary" && item.ItemID == "card-old" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the older binary's tracker row is not visible to the read: %+v", store.Items)
	}
	if !store.HasScopedOptOut(key+"-old", CheckpointApplyStart) {
		t.Fatalf("the older binary's opt-out is not visible to the read")
	}

	// The next write carries the absorbed rows into the split store, so the
	// divergence heals instead of recurring, and the pre-split file is untouched.
	if _, err := PersistDecision(LedgersAt(root), key+"-2", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("second PersistDecision: %v", err)
	}
	reloaded, report, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Absorbed != 0 {
		t.Fatalf("absorbed = %d after the write carried them over, want 0", report.Absorbed)
	}
	if len(reloaded.Items) != 15 || len(reloaded.OptOuts) != 3 {
		t.Fatalf("split store = %d items / %d opt_outs, want 15 / 3", len(reloaded.Items), len(reloaded.OptOuts))
	}
	if got, err := os.ReadFile(legacy); err != nil || string(got) != string(legacyBytes) {
		t.Fatalf("the pre-split store was rewritten (%v); the older binary owns it", err)
	}
}

// A corrupt pre-split store must not take the split store's rows down with it, and
// must not be silent either.
func TestTrackerStoreReportsAnUnreadableLegacyWithoutDroppingSplitRows(t *testing.T) {
	root := t.TempDir()
	if _, err := TrackerLocation(root).Save(trackerRows(closedHistory(2))); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(StorePath(root)), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(StorePath(root), []byte("not json"), 0o600); err != nil {
		t.Fatalf("corrupt legacy: %v", err)
	}

	store, report, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v, want the split store to answer despite the unreadable fallback", err)
	}
	if len(store.Items) != 2 {
		t.Fatalf("items = %d, want the split store's 2 rows", len(store.Items))
	}
	if report.LegacyUnreadable == nil {
		t.Fatalf("the unreadable pre-split store was not reported")
	}
}

// The one-shot backup is a safety net, so a copy that cannot be written must be
// reported rather than swallowed — and must not block the split write either.
func TestTrackerStoreReportsAFailedBackup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(14))
	legacyDir := filepath.Dir(StorePath(root))
	if err := os.Chmod(legacyDir, 0o500); err != nil {
		t.Fatalf("chmod legacy dir: %v", err)
	}
	defer os.Chmod(legacyDir, 0o755)

	report, err := TrackerLocation(root).Save(trackerRows(closedHistory(1)))
	if err != nil {
		t.Fatalf("Save: %v, want the split write to succeed where it lives", err)
	}
	if report.BackupFailed == nil {
		t.Fatalf("a backup that could not be written was not reported")
	}
	if _, err := os.Stat(StorePath(root) + ".v1-backup"); err == nil {
		t.Fatalf("a backup appeared in a directory that could not be written")
	}
}

// Every problem in a report is one visible line; a plain read or write says
// nothing at all, so a clean repository stays silent.
func TestStoreReportNotesOnlyWhatHappened(t *testing.T) {
	var clean strings.Builder
	StoreReport{}.Note(&clean, "worktree-gate")
	if clean.String() != "" {
		t.Fatalf("a clean report wrote %q", clean.String())
	}

	var absorbed strings.Builder
	StoreReport{Absorbed: 2, LegacyRows: "/repo/.git/ai-specs/ledger/state.json"}.Note(&absorbed, "worktree-gate")
	if lines := strings.Count(absorbed.String(), "\n"); lines != 1 {
		t.Fatalf("absorbed report wrote %d lines, want 1: %q", lines, absorbed.String())
	}
	for _, want := range []string{"2 ledger row(s)", "ledger/state.json", "next write carries them"} {
		if !strings.Contains(absorbed.String(), want) {
			t.Fatalf("absorbed report %q does not name %q", absorbed.String(), want)
		}
	}

	var unreadable strings.Builder
	StoreReport{LegacyUnreadable: errors.New("permission denied")}.Note(&unreadable, "worktree-gate")
	if !strings.Contains(unreadable.String(), "unreadable") || !strings.Contains(unreadable.String(), "permission denied") {
		t.Fatalf("unreadable report = %q", unreadable.String())
	}

	var backup strings.Builder
	StoreReport{BackupFailed: errors.New("disk full")}.Note(&backup, "worktree-gate")
	if !strings.Contains(backup.String(), "backup") || !strings.Contains(backup.String(), "disk full") {
		t.Fatalf("backup report = %q", backup.String())
	}

	var shadowed strings.Builder
	StoreReport{Shadowed: 1, LegacyRows: "/repo/.git/ai-specs/ledger/state.json"}.Note(&shadowed, "worktree-gate")
	if lines := strings.Count(shadowed.String(), "\n"); lines != 1 {
		t.Fatalf("shadowed report wrote %d lines, want 1: %q", lines, shadowed.String())
	}
	for _, want := range []string{"1 ledger row(s) differ", "split store's version answers"} {
		if !strings.Contains(shadowed.String(), want) {
			t.Fatalf("shadowed report %q does not name %q", shadowed.String(), want)
		}
	}
}

// A row the older binary changed in place cannot be absorbed without rewriting a
// row this build already wrote, so the split store's version answers and the
// disagreement is reported. Silence here would let the two files drift apart.
func TestTrackerStoreReportsRowsAnOlderBinaryChangedInPlace(t *testing.T) {
	root := t.TempDir()
	history := closedHistory(14)
	legacy, _ := seedLegacy(t, root, history)
	if _, err := TrackerLocation(root).Save(trackerRows(history)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	changed, err := LoadLegacyStore(legacy)
	if err != nil {
		t.Fatalf("LoadLegacyStore(legacy): %v", err)
	}
	changed.Items[0].Decisions = append(changed.Items[0].Decisions, Decision{
		At: "2026-11-27T00:00:00Z", Checkpoint: CheckpointArchiveClose, Kind: DecisionLink,
	})
	changedBytes, err := json.MarshalIndent(changed, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := writeAtomic(legacy, append(changedBytes, '\n')); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	store, report, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Absorbed != 0 {
		t.Fatalf("absorbed = %d, want 0: the row already exists in the split store", report.Absorbed)
	}
	if report.Shadowed != 1 {
		t.Fatalf("shadowed = %d, want the 1 row the older binary changed", report.Shadowed)
	}
	if len(store.Items) != 14 {
		t.Fatalf("items = %d, want the 14 rows the split store already had", len(store.Items))
	}
	// The tracker share of a historical row carries no close decision, so the
	// split store's version is the empty decision list, not the older binary's.
	if got := len(store.Items[0].Decisions); got != 0 {
		t.Fatalf("row %q carries %d decisions, want the split store's 0", store.Items[0].ID, got)
	}
}
