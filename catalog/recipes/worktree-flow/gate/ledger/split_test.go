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

// The section authority (L14): each capability writes only its own section, and one
// atomic rename makes the write all-or-nothing.

// TestCloseLandsInTheWorktreeSectionOnly pins that a close is the lifecycle's
// decision: the worktree section gains it, the tracker section is unchanged.
func TestCloseLandsInTheWorktreeSectionOnly(t *testing.T) {
	loc := openFixture("card-1").write(t, ledgerPath(t))
	trackerBefore := mustMarshal(t, loadFile(t, loc).Tracker)

	out, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity:   ident(""),
		Checkpoint: CheckpointPreMerge,
		Request:    WriteRequest{Kind: WriteClose},
	}, vtNow)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if !out.Applied {
		t.Fatalf("close = %+v, want applied", out)
	}

	file := loadFile(t, loc)
	if got := mustMarshal(t, file.Tracker); got != trackerBefore {
		t.Fatalf("the close changed the tracker section:\nbefore=%s\nafter=%s", trackerBefore, got)
	}
	row := file.Worktree.Items[0]
	if row.Status != StatusClosed {
		t.Fatalf("status = %q, want closed", row.Status)
	}
	last := row.Decisions[len(row.Decisions)-1]
	if last.Kind != DecisionClose || last.Checkpoint != CheckpointPreMerge {
		t.Fatalf("last worktree decision = %+v, want close at pre-merge", last)
	}
}

// TestDecisionsLandInTheTrackerSectionOnly pins that a binding, an opt-out and an
// adjudication are the tracker's decisions: the worktree section is unchanged.
func TestDecisionsLandInTheTrackerSectionOnly(t *testing.T) {
	loc := openFixture("card-1").write(t, ledgerPath(t))
	key := ident("").Key()
	worktreeBefore := mustMarshal(t, loadFile(t, loc).Worktree)
	unchanged := func(step string) {
		t.Helper()
		if got := mustMarshal(t, loadFile(t, loc).Worktree); got != worktreeBefore {
			t.Fatalf("%s changed the worktree section:\nbefore=%s\nafter=%s", step, worktreeBefore, got)
		}
	}

	if _, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart, ProviderID: "trello-mcp-workflow",
		Request: WriteRequest{Kind: WriteLink, ItemID: "card-1", URL: "https://example.invalid/c/1"},
	}, vtNow); err != nil {
		t.Fatalf("link: %v", err)
	}
	unchanged("link")

	if _, err := PersistDecision(loc, key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue",
	}, vtNow); err != nil {
		t.Fatalf("opt-out: %v", err)
	}
	unchanged("opt-out")

	if _, err := PersistDecision(loc, key, DecisionRequest{
		Checkpoint: CheckpointPreMerge, Kind: DecisionAdjudicate, Choice: "local",
	}, vtNow); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	unchanged("adjudication")

	item, err := loadView(t, loc).Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, d := range item.Decisions {
		kinds[d.Kind]++
	}
	for _, want := range []string{DecisionOpen, DecisionLink, DecisionOptOut, DecisionAdjudicate} {
		if kinds[want] != 1 {
			t.Fatalf("%s decisions = %d, want 1 (%+v)", want, kinds[want], item.Decisions)
		}
	}
	if kinds[DecisionClose] != 0 {
		t.Fatalf("the tracker section carries a close decision: %+v", item.Decisions)
	}
}

// TestCorruptLedgerIsUnevaluableAndWritesNothing pins the corruption contract on one
// file: a corrupt ledger is unevaluable, and a write fails closed leaving the bytes
// exactly as found (never "empty and writable").
func TestCorruptLedgerIsUnevaluableAndWritesNothing(t *testing.T) {
	loc := StoreAt(ledgerPath(t))
	if err := os.MkdirAll(filepath.Dir(loc.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loc.Path, []byte(`{"v":2,"worktree":{"items":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := rawBytes(t, loc.Path)

	_, _, err := loc.Load()
	var corrupt *StoreCorruptError
	if !errors.As(err, &corrupt) {
		t.Fatalf("Load error = %v, want a *StoreCorruptError", err)
	}
	if _, err := ApplyWrite(loc, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
	}, vtNow); err == nil {
		t.Fatal("a write over a corrupt ledger succeeded")
	}
	if got := rawBytes(t, loc.Path); string(got) != string(before) {
		t.Fatalf("the corrupt ledger was rewritten:\nbefore=%s\nafter=%s", before, got)
	}

	// A read error that is not "absent" is corruption, never an empty ledger.
	blocked := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := StoreAt(filepath.Join(blocked, "state.json")).Load(); !errors.As(err, &corrupt) {
		t.Fatalf("an unreadable path read as an empty ledger: %v", err)
	}
}

// TestMigrationNormalisesEachSectionFromItsOwnFields pins the migration shape: a v1
// row gives the worktree section its lifecycle and the tracker section its record,
// and a disagreement in a tracker field never changes the worktree share.
func TestMigrationNormalisesEachSectionFromItsOwnFields(t *testing.T) {
	loc := StoreAt(ledgerPath(t))
	history := closedHistory(3)
	writeLegacyFixture(t, loc.Path, history)

	file := legacyAsLedger(history)
	if len(file.Worktree.Items) != 3 || len(file.Tracker.Items) != 3 {
		t.Fatalf("sections = %d/%d rows, want 3/3", len(file.Worktree.Items), len(file.Tracker.Items))
	}
	for i, row := range history.Items {
		if file.Worktree.Items[i].ID != row.ID || file.Worktree.Items[i].Status != row.Status {
			t.Fatalf("worktree row %d = %+v, want the v1 row's id and status", i, file.Worktree.Items[i])
		}
		if len(file.Worktree.Items[i].Decisions) != 1 || file.Worktree.Items[i].Decisions[0].Kind != DecisionClose {
			t.Fatalf("worktree row %d decisions = %+v, want only the close", i, file.Worktree.Items[i].Decisions)
		}
		for _, d := range file.Tracker.Items[i].Decisions {
			if d.Kind == DecisionClose {
				t.Fatalf("the tracker row carries a close: %+v", file.Tracker.Items[i].Decisions)
			}
		}
	}
}

// TestPreSplitLedgerIsBackedUpOnceAndRewrittenAsV2 pins the migration write: the v1
// bytes are kept as a one-shot backup, and the file becomes v2 exactly once.
func TestPreSplitLedgerIsBackedUpOnceAndRewrittenAsV2(t *testing.T) {
	loc := StoreAt(ledgerPath(t))
	legacyBytes := writeLegacyFixture(t, loc.Path, closedHistory(3))

	file := loadFile(t, loc)
	if len(file.Worktree.Items) != 3 {
		t.Fatalf("worktree rows = %d, want the 3 migrated rows", len(file.Worktree.Items))
	}
	if file.V != LedgerVersion {
		t.Fatalf("loaded version = %d, want %d in memory", file.V, LedgerVersion)
	}
	if _, err := loc.Save(file); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := rawBytes(t, loc.Path+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the one-shot backup does not hold the predecessor bytes")
	}
	// The file is v2 now and keeps both sections.
	var onDisk struct {
		V        int `json:"v"`
		Worktree struct {
			Items []WorktreeItem `json:"items"`
		} `json:"worktree"`
		Tracker struct {
			Items []Item `json:"items"`
		} `json:"tracker"`
	}
	if err := json.Unmarshal(rawBytes(t, loc.Path), &onDisk); err != nil {
		t.Fatalf("decode the written ledger: %v", err)
	}
	if onDisk.V != LedgerVersion || len(onDisk.Worktree.Items) != 3 || len(onDisk.Tracker.Items) != 3 {
		t.Fatalf("written ledger = v%d %d/%d rows, want v2 with 3/3", onDisk.V, len(onDisk.Worktree.Items), len(onDisk.Tracker.Items))
	}

	// Changing the source bytes between attempts must not replace the backup.
	writeLegacyFixture(t, loc.Path, closedHistory(1))
	if _, err := loc.Save(file); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got := rawBytes(t, loc.Path+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the one-shot backup was replaced on a later write")
	}
}

// TestNextWriteCarriesMigratedRows pins that the absorbed rows are part of the file
// after the first write, so the divergence heals instead of recurring.
func TestNextWriteCarriesMigratedRows(t *testing.T) {
	root := t.TempDir()
	writeLegacyFixture(t, TrackerStorePath(root), closedHistory(2))
	loc := LedgerAt(root)

	file, report, err := loc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Absorbed != 2 || len(file.Worktree.Items) != 2 {
		t.Fatalf("absorbed = %d, rows = %d, want 2 and 2", report.Absorbed, len(file.Worktree.Items))
	}
	if _, err := PersistDecision(loc, ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/fresh"}.Key(), DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "local",
	}, time.Now()); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	reloaded, report, err := loc.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if report.Absorbed != 0 || len(reloaded.Worktree.Items) != 2 || len(reloaded.Tracker.OptOuts) != 1 {
		t.Fatalf("after the write: absorbed = %d, rows = %d, opt_outs = %d, want 0/2/1",
			report.Absorbed, len(reloaded.Worktree.Items), len(reloaded.Tracker.OptOuts))
	}
	if file := fileBytes(t, TrackerStorePath(root)); !strings.Contains(file, `"v": 1`) {
		t.Fatalf("the T5 predecessor was rewritten: %s", file)
	}
}

// TestPlanSectionIsDeclaredWithoutARow pins the third section: the shape exists and
// stays empty, and no separate plan file is created.
func TestPlanSectionIsDeclaredWithoutARow(t *testing.T) {
	loc := StoreAt(ledgerPath(t))
	if _, err := loc.Save(EmptyLedger()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var onDisk struct {
		Plan struct {
			Items []json.RawMessage `json:"items"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(rawBytes(t, loc.Path), &onDisk); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if onDisk.Plan.Items == nil || len(onDisk.Plan.Items) != 0 {
		t.Fatalf("plan section = %+v, want an empty items list", onDisk.Plan)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(loc.Path), "..", "ledgers", "plan.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a plan ledger file was created")
	}
}

// mustMarshal renders one section for a before/after comparison.
func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
