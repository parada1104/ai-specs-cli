package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// closedHistory builds a v1 predecessor store shaped like the real one this repo
// carries: only closed rows with their close decisions, no opt-outs. The rows stand
// for the historical decisions S11 requires to survive the migration untouched.
func closedHistory(n int) LegacyStore {
	store := LegacyStore{V: 1}
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

// trackerSection is the tracker section of a v1 predecessor: the rows it owns, with
// no status column and no close decision, plus its opt-outs.
func trackerSection(legacy LegacyStore) Store {
	store := Store{Items: []Item{}, OptOuts: append([]ScopedOptOut{}, legacy.OptOuts...)}
	for _, row := range legacy.Items {
		store.Items = append(store.Items, trackerShare(row))
	}
	return store
}

// seedLegacy writes a v1 predecessor at the ledger path under root and returns the
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

// seedT5 writes T5's mixed tracker file and returns its bytes.
func seedT5(t *testing.T, root string, store LegacyStore) []byte {
	t.Helper()
	path := TrackerStorePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := writeAtomic(path, data); err != nil {
		t.Fatal(err)
	}
	return data
}

// seedV2 writes a v2 ledger file and returns its bytes.
func seedV2(t *testing.T, path string, file LedgerFile) []byte {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := encodeLedgerFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, data); err != nil {
		t.Fatal(err)
	}
	return data
}

// sectionItems is the raw bytes of one named section's items array, so a test can
// pin a byte-level expectation instead of a decoded one.
func sectionItems(t *testing.T, path, section string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var body struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw[section], &body); err != nil {
		t.Fatalf("decode section %q of %s: %v", section, path, err)
	}
	return body.Items
}

// TestStorePaths pins the one-file location and the read-only T5 predecessor.
func TestStorePaths(t *testing.T) {
	common := filepath.Join("/repo", ".git")
	if got, want := StorePath(common), filepath.Join(common, "ai-specs", "ledger", "state.json"); got != want {
		t.Fatalf("StorePath = %q, want %q", got, want)
	}
	if got, want := TrackerStorePath(common), filepath.Join(common, "ai-specs", "ledgers", "tracker.json"); got != want {
		t.Fatalf("TrackerStorePath = %q, want %q", got, want)
	}
	if got, want := LockPath(StorePath(common)), StorePath(common)+".lock"; got != want {
		t.Fatalf("LockPath = %q, want the T5 name %q", got, want)
	}
}

// TestV1FileIsMigratedInMemoryBeforeTheFirstWrite pins the read path: a v1 file at
// the ledger path answers from both sections, nothing is written by a read, and the
// predecessor is left exactly as found.
func TestV1FileIsMigratedInMemoryBeforeTheFirstWrite(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(14))
	t5 := TrackerStorePath(root)

	file := loadFile(t, LedgerAt(root))
	if len(file.Tracker.Items) != 14 || len(file.Worktree.Items) != 14 {
		t.Fatalf("sections = %d/%d rows, want 14/14", len(file.Tracker.Items), len(file.Worktree.Items))
	}
	if file.V != LedgerVersion {
		t.Fatalf("in-memory version = %d, want %d", file.V, LedgerVersion)
	}
	if _, err := os.Stat(t5); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading created %s", t5)
	}
	if got := rawBytes(t, legacy); string(got) != string(legacyBytes) {
		t.Fatal("a read rewrote the predecessor")
	}
	if _, err := os.Stat(legacy + ".v1-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a read wrote a backup")
	}
}

// TestFirstWriteRewritesAsV2AndKeepsTheBackup pins the write path: the predecessor
// bytes are kept as a one-shot backup and the file becomes v2 with both sections.
func TestFirstWriteRewritesAsV2AndKeepsTheBackup(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(14))
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/fresh"}.Key()

	if _, err := PersistDecision(LedgerAt(root), key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}

	if got := rawBytes(t, legacy+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatalf("the backup does not hold the predecessor bytes")
	}
	var header struct {
		V int `json:"v"`
	}
	written := rawBytes(t, legacy)
	if err := json.Unmarshal(written, &header); err != nil || header.V != LedgerVersion {
		t.Fatalf("written version = %d (%v), want %d", header.V, err, LedgerVersion)
	}
	// The tracker section carries its own share of every historical row: the fields
	// it owns, unchanged, and nothing the worktree section owns. The expectation is
	// built here, independently of the production projection (L12.6).
	var gotRows []map[string]any
	if err := json.Unmarshal(sectionItems(t, legacy, "tracker"), &gotRows); err != nil {
		t.Fatalf("decode the tracker rows: %v", err)
	}
	wantRows := legacyTrackerRowMaps(closedHistory(14))
	if len(gotRows) != len(wantRows) {
		t.Fatalf("rows = %d, want %d", len(gotRows), len(wantRows))
	}
	for i := range wantRows {
		if !reflect.DeepEqual(gotRows[i], wantRows[i]) {
			t.Fatalf("row %d is not the historical share:\n got %v\nwant %v", i, gotRows[i], wantRows[i])
		}
	}
	// Byte-level guards that need no decoder.
	if bytes.Contains(sectionItems(t, legacy, "tracker"), []byte(`"status"`)) {
		t.Fatal("the tracker section carries a status column")
	}
	if bytes.Contains(sectionItems(t, legacy, "tracker"), []byte(`"kind": "close"`)) {
		t.Fatal("the tracker section carries a close decision")
	}
	for _, row := range closedHistory(14).Items {
		for _, want := range []string{"\"id\": \"" + row.ID + "\"", "\"item_id\": \"" + row.ItemID + "\"", "\"branch\": \"" + row.Identity.Branch + "\""} {
			if !bytes.Contains(sectionItems(t, legacy, "tracker"), []byte(want)) {
				t.Fatalf("the historical row %q is not on disk verbatim: missing %s", row.ID, want)
			}
		}
	}
	file := loadFile(t, LedgerAt(root))
	if len(file.Worktree.Items) != 14 || len(file.Tracker.OptOuts) != 1 {
		t.Fatalf("worktree rows = %d, opt_outs = %d, want 14 and 1", len(file.Worktree.Items), len(file.Tracker.OptOuts))
	}
	for _, row := range file.Worktree.Items {
		if row.Status != StatusClosed {
			t.Fatalf("row %q is %q: a historical row was revived as live state", row.ID, row.Status)
		}
	}

	// A second write must not overwrite the one-shot backup, so the predecessor bytes
	// change to a different *valid* store between the attempts.
	changed, err := json.MarshalIndent(closedHistory(1), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, append(changed, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistDecision(LedgerAt(root), key+"-2", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("second PersistDecision: %v", err)
	}
	if got := rawBytes(t, legacy+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the one-shot backup was replaced on a later write")
	}
}

// legacyTrackerRowMaps is the tracker section's share of a predecessor written out as
// plain JSON maps, built from the fixture alone: it is the expected value, not the
// production projection, so a projection bug cannot pass its own test.
func legacyTrackerRowMaps(legacy LegacyStore) []map[string]any {
	rows := make([]map[string]any, 0, len(legacy.Items))
	for _, row := range legacy.Items {
		rows = append(rows, map[string]any{
			"id": row.ID,
			"identity": map[string]any{
				"common_dir": row.Identity.CommonDir,
				"branch":     row.Identity.Branch,
				"change":     row.Identity.Change,
			},
			"item_id":     row.ItemID,
			"provider_id": row.ProviderID,
			"native_type": row.NativeType,
			"url":         row.URL,
			"state":       row.State,
			"provider":    nil,
			"exemption":   row.Exemption,
			"conflict":    nil,
			"decisions":   []any{},
		})
	}
	return rows
}

// TestLedgerRowsWinAndSourceOnlyRowsAreAdded pins that the ledger file's version of a
// row answers, and the rows only a predecessor has are added.
func TestLedgerRowsWinAndSourceOnlyRowsAreAdded(t *testing.T) {
	root := t.TempDir()
	split := trackerSection(closedHistory(2))
	split.Items[0].ItemID = "from-the-ledger"
	seedV2(t, StorePath(root), LedgerFile{V: LedgerVersion, Tracker: split})
	seedT5(t, root, closedHistory(14))

	file, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The 12 source-only rows and the 2 already present whose closes the worktree
	// section lacked: every source row that contributed is part of the answer.
	if report.Absorbed != 14 {
		t.Fatalf("absorbed = %d, want the 14 source rows that contributed", report.Absorbed)
	}
	if len(file.Tracker.Items) != 14 {
		t.Fatalf("rows = %d, want the file's 2 plus the 12 source-only rows", len(file.Tracker.Items))
	}
	if file.Tracker.Items[0].ItemID != "from-the-ledger" {
		t.Fatalf("row %q carries %q, want the ledger file's version", file.Tracker.Items[0].ID, file.Tracker.Items[0].ItemID)
	}
}

// TestCorruptLedgerFileIsUnevaluable pins that a corrupt v2 file is never replaced by
// predecessor rows: it is reported and nothing is synthesized.
func TestCorruptLedgerFileIsUnevaluable(t *testing.T) {
	root := t.TempDir()
	seedLegacy(t, root, closedHistory(14))
	path := StorePath(root)
	if err := os.WriteFile(path, []byte(`{"v":2,"worktree":{"items":[`), 0o600); err != nil {
		t.Fatal(err)
	}

	file, _, err := LedgerAt(root).Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("error = %v, want a *StoreCorruptError", err)
	}
	if corrupt.Path != path {
		t.Fatalf("corrupt path = %q, want %q", corrupt.Path, path)
	}
	if len(file.Tracker.Items) != 0 || len(file.Worktree.Items) != 0 {
		t.Fatalf("a corrupt file synthesized rows: %d/%d", len(file.Tracker.Items), len(file.Worktree.Items))
	}
}

// TestCorruptV1FileIsUnevaluable pins the same rule for a corrupt v1 predecessor at
// the ledger path.
func TestCorruptV1FileIsUnevaluable(t *testing.T) {
	root := t.TempDir()
	legacy := StorePath(root)
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, _, err := LedgerAt(root).Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("error = %v, want a *StoreCorruptError, never an empty ledger", err)
	}
	if len(file.Tracker.Items) != 0 {
		t.Fatalf("rows = %d, want none from a corrupt file", len(file.Tracker.Items))
	}
}

// TestExplicitStoreHasNoMigrationSource pins that --store names THE file: a T5
// sibling is not read, nothing is copied in, and no backup of it is written.
func TestExplicitStoreHasNoMigrationSource(t *testing.T) {
	root := t.TempDir()
	t5 := seedT5(t, root, closedHistory(14))
	explicit := filepath.Join(root, "custom.json")
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/explicit"}.Key()

	if _, err := PersistDecision(StoreAt(explicit), key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	file := loadFile(t, StoreAt(explicit))
	if len(file.Tracker.Items) != 0 {
		t.Fatalf("rows = %d: a predecessor was copied into the explicit ledger", len(file.Tracker.Items))
	}
	if len(file.Tracker.OptOuts) != 1 {
		t.Fatalf("opt_outs = %d, want only the decision that landed", len(file.Tracker.OptOuts))
	}
	if got := rawBytes(t, TrackerStorePath(root)); string(got) != string(t5) {
		t.Fatal("the T5 sibling was rewritten")
	}
	if _, err := os.Stat(TrackerStorePath(root) + ".v1-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the explicit ledger backed up a file it never reads")
	}
}

// TestPredecessorIsBackedUpAndUpgradedInPlace pins the one-file pairing: a v1 file at
// the ledger path is the predecessor, so upgrading it keeps its bytes and rewrites it
// as v2 — never as a second file beside it.
func TestPredecessorIsBackedUpAndUpgradedInPlace(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(3))
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/explicit"}.Key()

	if _, err := PersistDecision(StoreAt(legacy), key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	if got := rawBytes(t, legacy+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the predecessor bytes were not kept")
	}
	file := loadFile(t, StoreAt(legacy))
	if len(file.Tracker.Items) != 3 || len(file.Tracker.OptOuts) != 1 || len(file.Worktree.Items) != 3 {
		t.Fatalf("ledger = %d/%d rows and %d opt_outs, want 3/3 and the decision",
			len(file.Tracker.Items), len(file.Worktree.Items), len(file.Tracker.OptOuts))
	}
	if file.V != LedgerVersion {
		t.Fatalf("version = %d, want %d", file.V, LedgerVersion)
	}
	for _, name := range []string{"worktree.json", "tracker.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("upgrading created a second ledger file %q", name)
		}
	}
}

// TestOlderBinaryRowsAreAbsorbedAndCarried pins that rows an older ai-specs keeps
// writing to a predecessor are neither lost nor able to overwrite the ledger's own.
func TestOlderBinaryRowsAreAbsorbedAndCarried(t *testing.T) {
	root := t.TempDir()
	key := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/fresh"}.Key()

	if _, err := PersistDecision(LedgerAt(root), key+"-1", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("first PersistDecision: %v", err)
	}

	// The older binary opens a row and records an opt-out in the T5 predecessor.
	old := closedHistory(14)
	old.Items = append(old.Items, LegacyItem{
		Status: StatusOpen,
		Item: Item{
			ID: "row-from-the-old-binary", Identity: ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/old"},
			ItemID: "card-old",
		},
	})
	old.OptOuts = append(old.OptOuts, ScopedOptOut{
		Key: key + "-old", Checkpoint: CheckpointApplyStart, Scope: ScopeLifecycle,
		Choice: "local", At: "2026-11-27T00:00:00Z",
	})
	oldBytes := seedT5(t, root, old)

	file, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The 14 historical rows, the opened row and the opt-out: 16 pieces of content.
	if report.Absorbed != 16 {
		t.Fatalf("absorbed = %d, want the 14 rows, the opened row and the opt-out", report.Absorbed)
	}
	if report.LegacyRows != TrackerStorePath(root) {
		t.Fatalf("report names %q, want %q", report.LegacyRows, TrackerStorePath(root))
	}
	if len(file.Tracker.Items) != 15 {
		t.Fatalf("rows = %d, want the 14 plus the one the older binary opened", len(file.Tracker.Items))
	}
	if len(file.Tracker.OptOuts) != 2 {
		t.Fatalf("opt_outs = %d, want this build's decision and the older binary's", len(file.Tracker.OptOuts))
	}
	open := file.Worktree.OpenItems(ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/old"}.Key())
	if len(open) != 1 || open[0].ID != "row-from-the-old-binary" {
		t.Fatalf("the older binary's row is not visible to the read: %+v", open)
	}
	if !file.Tracker.HasScopedOptOut(key+"-old", CheckpointApplyStart) {
		t.Fatal("the older binary's opt-out is not visible to the read")
	}

	// The next write carries the absorbed rows into the ledger file.
	if _, err := PersistDecision(LedgerAt(root), key+"-2", DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("second PersistDecision: %v", err)
	}
	reloaded, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if report.Absorbed != 0 {
		t.Fatalf("absorbed = %d after the write carried them over, want 0", report.Absorbed)
	}
	if len(reloaded.Tracker.Items) != 15 || len(reloaded.Tracker.OptOuts) != 3 {
		t.Fatalf("ledger = %d rows / %d opt_outs, want 15 / 3", len(reloaded.Tracker.Items), len(reloaded.Tracker.OptOuts))
	}
	if got := rawBytes(t, TrackerStorePath(root)); string(got) != string(oldBytes) {
		t.Fatal("the predecessor was rewritten; the older binary owns it")
	}
}

// TestUnreadableSourceIsReportedNotFatal pins L17 MEDIO-3, which supersedes L14's
// stricter reading for a *migration source*: an unreadable predecessor is reported and
// the ledger file answers. The ledger file itself keeps the strict rule — only
// fs.ErrNotExist means absent — which the corrupt-ledger tests pin.
func TestUnreadableSourceIsReportedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	root := t.TempDir()
	stamp := t0.Format(time.RFC3339)
	file := EmptyLedger()
	file.Worktree.Items = append(file.Worktree.Items, WorktreeItem{
		ID: "row", Identity: ident(""), Status: StatusOpen,
		Decisions: []WorktreeDecision{{At: stamp, Kind: DecisionOpen}},
	})
	file.Tracker.Items = append(file.Tracker.Items, Item{
		ID: "row", Identity: ident(""), ItemID: "card-1", ProviderID: "trello-mcp-workflow",
		Decisions: []Decision{{At: stamp, Kind: DecisionOpen}},
	})
	seedV2(t, StorePath(root), file)
	seedT5(t, root, closedHistory(2))
	t5Dir := filepath.Dir(TrackerStorePath(root))
	if err := os.Chmod(t5Dir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(t5Dir, 0o755)

	loaded, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v, want the ledger to answer despite the unreadable source", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("the unreadable migration source was not reported")
	}
	if _, err := loaded.View().Primary(ident("").Key()); err != nil {
		t.Fatalf("the ledger does not answer its own row: %v", err)
	}
}

// TestFailedBackupFailsClosedWithATruthfulMessage pins MEDIO-2: the write refuses, and
// the message says only what is true (nothing was replaced).
func TestFailedBackupFailsClosedWithATruthfulMessage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory cannot be simulated as root")
	}
	root := t.TempDir()
	seedT5(t, root, closedHistory(14))
	t5Dir := filepath.Dir(TrackerStorePath(root))
	if err := os.Chmod(t5Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(t5Dir, 0o755)

	report, err := LedgerAt(root).Save(EmptyLedger(), SectionWorktree, SectionTracker, SectionPlan)
	if err == nil {
		t.Fatal("Save wrote the ledger although the predecessor backup failed")
	}
	if report.BackupFailed == nil {
		t.Fatal("a backup that could not be written was not reported")
	}
	if _, err := os.Stat(StorePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the ledger was written without a backup: %v", err)
	}
	var note strings.Builder
	report.Note(&note, "worktree-gate")
	if strings.Contains(note.String(), "no row is lost") || strings.Contains(note.String(), "untouched") {
		t.Fatalf("the failed-backup message claims something false: %q", note.String())
	}
	if !strings.Contains(note.String(), "was not written") {
		t.Fatalf("the message does not say the ledger was not written: %q", note.String())
	}
}

// TestStoreReportNotesOnlyWhatHappened pins one visible line per problem and silence
// on a clean read or write.
func TestStoreReportNotesOnlyWhatHappened(t *testing.T) {
	var clean strings.Builder
	StoreReport{}.Note(&clean, "worktree-gate")
	if clean.String() != "" {
		t.Fatalf("a clean report wrote %q", clean.String())
	}

	var absorbed strings.Builder
	StoreReport{Absorbed: 2, LegacyRows: "/repo/.git/ai-specs/ledgers/tracker.json"}.Note(&absorbed, "worktree-gate")
	if lines := strings.Count(absorbed.String(), "\n"); lines != 1 {
		t.Fatalf("absorbed report wrote %d lines, want 1: %q", lines, absorbed.String())
	}
	for _, want := range []string{"2 ledger row(s)", "tracker.json", "next write carries them"} {
		if !strings.Contains(absorbed.String(), want) {
			t.Fatalf("absorbed report %q does not name %q", absorbed.String(), want)
		}
	}

	var unreadable strings.Builder
	StoreReport{LegacyRows: "x", LegacyUnreadable: errors.New("permission denied")}.Note(&unreadable, "worktree-gate")
	if !strings.Contains(unreadable.String(), "unreadable") || !strings.Contains(unreadable.String(), "permission denied") {
		t.Fatalf("unreadable report = %q", unreadable.String())
	}

	var backup strings.Builder
	StoreReport{BackupFailed: errors.New("disk full")}.Note(&backup, "worktree-gate")
	if !strings.Contains(backup.String(), "backup") || !strings.Contains(backup.String(), "disk full") {
		t.Fatalf("backup report = %q", backup.String())
	}

	var shadowed strings.Builder
	StoreReport{Shadowed: 1, LegacyRows: "/repo/.git/ai-specs/ledgers/tracker.json",
		Path: "/repo/.git/ai-specs/ledger.json"}.Note(&shadowed, "worktree-gate")
	if lines := strings.Count(shadowed.String(), "\n"); lines != 1 {
		t.Fatalf("shadowed report wrote %d lines, want 1: %q", lines, shadowed.String())
	}
	// T5 R1: the note names both files, so a reader knows which one wins instead of
	// being told only that they disagree.
	for _, want := range []string{"1 ledger row(s) differ", "tracker.json", "ledger.json",
		"is the file that answers"} {
		if !strings.Contains(shadowed.String(), want) {
			t.Fatalf("shadowed report %q does not name %q", shadowed.String(), want)
		}
	}
}

// TestOlderBinaryChangeInPlaceIsReportedNotAdopted pins that the ledger file's version
// of a row answers and the disagreement is reported, so nobody assumes both agree.
func TestOlderBinaryChangeInPlaceIsReportedNotAdopted(t *testing.T) {
	root := t.TempDir()
	history := closedHistory(14)
	seedV2(t, StorePath(root), LedgerFile{V: LedgerVersion, Tracker: trackerSection(history)})
	changed := closedHistory(14)
	changed.Items[0].Decisions = append(changed.Items[0].Decisions, Decision{
		At: "2026-11-27T00:00:00Z", Checkpoint: CheckpointArchiveClose, Kind: DecisionLink,
	})
	seedT5(t, root, changed)

	file, report, err := LedgerAt(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Every source row contributes its closes to the worktree section, so they are
	// all part of this answer; exactly one row disagrees in the tracker field.
	if report.Absorbed != 14 {
		t.Fatalf("absorbed = %d, want the 14 source rows whose closes the worktree section lacked", report.Absorbed)
	}
	if report.Shadowed != 1 {
		t.Fatalf("shadowed = %d, want the 1 row the older binary changed", report.Shadowed)
	}
	if len(file.Tracker.Items) != 14 {
		t.Fatalf("rows = %d, want the 14 the ledger file already had", len(file.Tracker.Items))
	}
	// The predecessor's row carries its close, so the worktree section gains it and
	// nothing is lost; the tracker share still has no decisions.
	if got := len(file.Tracker.Items[0].Decisions); got != 0 {
		t.Fatalf("row %q carries %d tracker decisions, want 0", file.Tracker.Items[0].ID, got)
	}
	closes := 0
	for _, row := range file.Worktree.Items {
		for _, d := range row.Decisions {
			if d.Kind == DecisionClose {
				closes++
			}
		}
	}
	if closes != 14 {
		t.Fatalf("worktree closes = %d, want the 14 the predecessors hold", closes)
	}
}
