package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// closedHistory builds a pre-split store shaped like the real one this repo
// carries: only closed rows, no opt-outs. The rows stand for the historical
// decisions S11 requires to survive the migration untouched.
func closedHistory(n int) Store {
	store := Store{V: StoreVersion}
	for i := 0; i < n; i++ {
		branch := "feat/history-" + string(rune('a'+i))
		store.Items = append(store.Items, Item{
			ID:       "item-" + string(rune('a'+i)),
			Identity: ItemIdentity{CommonDir: "/repo/.git", Branch: branch},
			Status:   StatusClosed,
			ItemID:   "card-" + string(rune('a'+i)),
			Decisions: []Decision{{
				At: "2026-10-08T14:40:02Z", Checkpoint: CheckpointArchiveClose,
				Kind: DecisionClose, Choice: "local", Note: "merged",
			}},
		})
	}
	return store
}

// seedLegacy writes store at the pre-split location under root and returns the
// path and the exact bytes on disk.
func seedLegacy(t *testing.T, root string, store Store) (string, []byte) {
	t.Helper()
	path := StorePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := SaveStore(path, store); err != nil {
		t.Fatalf("seed legacy store: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy store: %v", err)
	}
	return path, data
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

	store, err := TrackerLocation(root).Load()
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

	_, err := PersistDecision(TrackerLocation(root), key, DecisionRequest{
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
	if got, want := string(itemsOf(t, split)), string(itemsOf(t, legacy)); got != want {
		t.Fatalf("historical rows are not byte-identical in the split store:\n got %s\nwant %s", got, want)
	}

	store, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.Items) != 14 {
		t.Fatalf("items = %d, want the 14 historical rows", len(store.Items))
	}
	for _, item := range store.Items {
		if item.Status != StatusClosed {
			t.Fatalf("row %q is %q: a historical row was revived as live state", item.ID, item.Status)
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
	if _, err := PersistDecision(TrackerLocation(root), key+"-2", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("second PersistDecision: %v", err)
	}
	backup, err = os.ReadFile(legacy + ".v1-backup")
	if err != nil || string(backup) != string(legacyBytes) {
		t.Fatalf("the backup was replaced on a later write (%v); it is one-shot", err)
	}
}

func TestTrackerStorePrefersTheSplitFileOverTheLegacyOne(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(14))
	split := TrackerStorePath(root)
	if err := TrackerLocation(root).Save(closedHistory(2)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(split); err != nil {
		t.Fatalf("split store missing: %v", err)
	}

	store, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(store.Items) != 2 {
		t.Fatalf("items = %d, want the 2 of the split store", len(store.Items))
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

	store, err := TrackerLocation(root).Load()
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

	store, err := TrackerLocation(root).Load()
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

	if _, err := PersistDecision(StoreAt(explicit), key, DecisionRequest{
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

			if _, err := PersistDecision(StoreAt(explicit), key, DecisionRequest{
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
