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

// The split authority (S3): every verb writes the ledger that owns its row.

// TestCloseLandsInTheWorktreeLedgerOnly pins S3(a): a close is the lifecycle's
// decision, so it lands in ledgers/worktree.json and the tracker ledger is left
// byte-identical.
func TestCloseLandsInTheWorktreeLedgerOnly(t *testing.T) {
	path := ledgerPath(t)
	locs := openFixture("card-1").write(t, path)
	trackerBefore := rawBytes(t, locs.Tracker.Path)

	out, err := ApplyWrite(locs, ApplyWriteRequest{
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

	if got := rawBytes(t, locs.Tracker.Path); string(got) != string(trackerBefore) {
		t.Fatalf("the close rewrote the tracker ledger:\nbefore=%s\nafter=%s", trackerBefore, got)
	}
	worktree := string(rawBytes(t, locs.Worktree.Path))
	for _, want := range []string{`"status": "closed"`, `"kind": "close"`, `"checkpoint": "pre-merge"`} {
		if !strings.Contains(worktree, want) {
			t.Fatalf("the worktree ledger does not carry %s:\n%s", want, worktree)
		}
	}
	if strings.Contains(string(trackerBefore), `"kind": "close"`) {
		t.Fatalf("the tracker ledger carries a close decision: %s", trackerBefore)
	}
	if _, ok := loadView(t, locs).LatestClosed(ident("").Key()); !ok {
		t.Fatal("the closed row is not visible to the read")
	}
}

// TestDecisionsLandInTheTrackerLedgerOnly pins S3(c): a binding, an opt-out and
// an adjudication are the tracker's decisions, written to ledgers/tracker.json
// with the worktree ledger left byte-identical.
func TestDecisionsLandInTheTrackerLedgerOnly(t *testing.T) {
	path := ledgerPath(t)
	locs := openFixture("card-1").write(t, path)
	key := ident("").Key()
	worktreeBefore := rawBytes(t, locs.Worktree.Path)
	unchanged := func(step string) {
		t.Helper()
		if got := string(rawBytes(t, locs.Worktree.Path)); got != string(worktreeBefore) {
			t.Fatalf("%s rewrote the worktree ledger:\nbefore=%s\nafter=%s", step, worktreeBefore, got)
		}
	}

	if _, err := ApplyWrite(locs, ApplyWriteRequest{
		Identity: ident(""), Checkpoint: CheckpointApplyStart, ProviderID: "trello-mcp-workflow",
		Request: WriteRequest{Kind: WriteLink, ItemID: "card-1", URL: "https://example.invalid/c/1"},
	}, vtNow); err != nil {
		t.Fatalf("link: %v", err)
	}
	unchanged("link")

	if _, err := PersistDecision(locs, key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue",
	}, vtNow); err != nil {
		t.Fatalf("opt-out: %v", err)
	}
	unchanged("opt-out")

	if _, err := PersistDecision(locs, key, DecisionRequest{
		Checkpoint: CheckpointPreMerge, Kind: DecisionAdjudicate, Choice: "local",
	}, vtNow); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	unchanged("adjudication")

	item, err := loadView(t, locs).Primary(key)
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
		t.Fatalf("the tracker row carries a close decision: %+v", item.Decisions)
	}
	// The worktree row holds the lifecycle only: no binding, no opt-out, no
	// adjudication reaches it.
	for _, row := range loadView(t, locs).Worktree.Items {
		if len(row.Decisions) != 0 {
			t.Fatalf("worktree row %q carries tracker decisions: %+v", row.ID, row.Decisions)
		}
	}
}

// TestCorruptWorktreeLedgerStillLetsTheTrackerAnswer pins the isolation PR-1
// delivers: a corrupt worktree ledger is reported, the tracker ledger still
// answers its own rows and opt-outs, and the grade reports unevaluable instead
// of silently allowing with unknown lifecycle state.
func TestCorruptWorktreeLedgerStillLetsTheTrackerAnswer(t *testing.T) {
	root := t.TempDir()
	key := ident("").Key()
	tracker := Store{V: StoreVersion, OptOuts: []ScopedOptOut{{
		Key: key, Checkpoint: CheckpointApplyStart, Scope: ScopeLifecycle,
		Choice: "continue", At: t0.Format(time.RFC3339),
	}}}
	locs := LedgersAt(root)
	if _, err := locs.Tracker.Save(tracker); err != nil {
		t.Fatalf("save tracker: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(locs.Worktree.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(locs.Worktree.Path, []byte(`{"v":1,"items":[`), 0o600); err != nil {
		t.Fatal(err)
	}

	view, _, err := locs.Load()
	if err == nil {
		t.Fatal("a corrupt worktree ledger must be reported")
	}
	if !view.HasScopedOptOut(key, CheckpointApplyStart) ||
		!view.HasScopedOptOut(key, CheckpointPreMerge) {
		t.Fatal("the tracker ledger did not answer its own opt-out")
	}
	if len(view.Tracker.OptOuts) != 1 {
		t.Fatalf("tracker opt_outs = %+v, want the stored one", view.Tracker.OptOuts)
	}
	if _, perr := view.Primary(key); perr == nil {
		t.Fatal("a corrupt worktree ledger cannot select a primary")
	}

	// The corrupt file is reported at grading time as infrastructure, never as a
	// silent allow and never repaired.
	got := Grade(Input{
		Checkpoint: CheckpointApplyStart, Mode: ModeAlways, Binding: boundBinding(),
		Identity: availIdentity(), View: view, StoreErr: err, Now: vtNow,
	})
	assertVerdict(t, "corrupt worktree ledger", got, vWant{DecisionUnevaluable, ReasonStoreCorrupt, 0, SeverityError, true})
	if raw := string(rawBytes(t, locs.Worktree.Path)); raw != `{"v":1,"items":[` {
		t.Fatalf("the corrupt worktree ledger was rewritten: %s", raw)
	}
}

// TestEachLedgerAbsorbsOnlyItsOwnShare pins the migration honesty (S11/L8): a
// disagreement in a field a ledger does not own is not a shadow for it. The same
// pre-split change is a shadow for exactly the ledger that owns the field.
func TestEachLedgerAbsorbsOnlyItsOwnShare(t *testing.T) {
	root := t.TempDir()
	history := closedHistory(14)

	// Both split ledgers already hold their own share of every historical row.
	if _, err := WorktreeLocation(root).SaveWorktree(
		WorktreeStore{V: StoreVersion, Items: worktreeRows(history)}); err != nil {
		t.Fatalf("save worktree: %v", err)
	}
	if _, err := TrackerLocation(root).Save(trackerRows(history)); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	// An older binary changes a tracker-owned field on one historical row.
	trackerOnly := closedHistory(14)
	trackerOnly.Items[0].ItemID = "card-changed"
	seedLegacy(t, root, trackerOnly)

	_, worktreeReport, err := WorktreeLocation(root).LoadWorktree()
	if err != nil {
		t.Fatalf("LoadWorktree: %v", err)
	}
	if worktreeReport.Shadowed != 0 || worktreeReport.Absorbed != 0 {
		t.Fatalf("worktree report = %+v, want no shadow: item_id is not its field", worktreeReport)
	}
	_, trackerReport, err := TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if trackerReport.Shadowed != 1 {
		t.Fatalf("tracker shadowed = %d, want the 1 row it owns", trackerReport.Shadowed)
	}

	// An older binary changes a worktree-owned field instead.
	statusOnly := closedHistory(14)
	statusOnly.Items[0].Status = StatusOpen
	seedLegacy(t, root, statusOnly)

	_, worktreeReport, err = WorktreeLocation(root).LoadWorktree()
	if err != nil {
		t.Fatalf("LoadWorktree: %v", err)
	}
	if worktreeReport.Shadowed != 1 {
		t.Fatalf("worktree shadowed = %d, want the 1 row it owns", worktreeReport.Shadowed)
	}
	_, trackerReport, err = TrackerLocation(root).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if trackerReport.Shadowed != 0 {
		t.Fatalf("tracker report = %+v, want no shadow: status is not its field", trackerReport)
	}
}

// TestPreSplitLedgerStaysByteIdenticalAndBacksUpOnce pins S11 for the worktree
// ledger: the pre-split file is never rewritten, and the one-shot backup is kept.
func TestPreSplitLedgerStaysByteIdenticalAndBacksUpOnce(t *testing.T) {
	root := t.TempDir()
	legacy, legacyBytes := seedLegacy(t, root, closedHistory(3))

	worktree, report, err := WorktreeLocation(root).LoadWorktree()
	if err != nil {
		t.Fatalf("LoadWorktree: %v", err)
	}
	// With no split file yet, the pre-split file is the answer: its rows are not
	// "missing" from anything, so nothing is absorbed and nothing is reported.
	if len(worktree.Items) != 3 || report.Absorbed != 0 {
		t.Fatalf("worktree = %d rows (absorbed %d), want the 3 historical rows and no report", len(worktree.Items), report.Absorbed)
	}
	if _, err := WorktreeLocation(root).SaveWorktree(worktree); err != nil {
		t.Fatalf("SaveWorktree: %v", err)
	}
	if got := rawBytes(t, legacy); string(got) != string(legacyBytes) {
		t.Fatalf("the pre-split store was rewritten:\nbefore=%s\nafter=%s", legacyBytes, got)
	}
	backup := rawBytes(t, legacy+".v1-backup")
	if string(backup) != string(legacyBytes) {
		t.Fatal("the one-shot backup does not match the pre-split store")
	}
	// A second write keeps the original backup.
	if _, err := WorktreeLocation(root).SaveWorktree(worktree); err != nil {
		t.Fatalf("second SaveWorktree: %v", err)
	}
	if got := rawBytes(t, legacy+".v1-backup"); string(got) != string(legacyBytes) {
		t.Fatal("the one-shot backup was replaced by a later write")
	}

	// Each split row carries only its own share.
	var rows []WorktreeItem
	if err := json.Unmarshal(itemsOf(t, WorktreeStorePath(root)), &rows); err != nil {
		t.Fatalf("decode the split worktree rows: %v", err)
	}
	want := worktreeRows(closedHistory(3))
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i := range want {
		if !sameJSON(rows[i], want[i]) {
			t.Fatalf("row %d is not the worktree share:\n got %+v\nwant %+v", i, rows[i], want[i])
		}
	}
}

// worktreeRows is the worktree ledger's share of a pre-split store.
func worktreeRows(legacy LegacyStore) []WorktreeItem {
	var rows []WorktreeItem
	for _, row := range legacy.Items {
		rows = append(rows, worktreeShare(row))
	}
	return rows
}

// TestPlanLocationIsDeclaredWithoutAStore pins the third location: it is named
// beside the other two and carries no store yet, so nothing can write it.
func TestPlanLocationIsDeclaredWithoutAStore(t *testing.T) {
	root := "/repo/.git"
	want := filepath.Join(root, "ai-specs", "ledgers", "plan.json")
	if got := PlanLocation(root).Path; got != want {
		t.Fatalf("PlanLocation path = %q, want %q", got, want)
	}
	if got := PlanLocation(root).Legacy; got != StorePath(root) {
		t.Fatalf("PlanLocation legacy = %q, want the pre-split %q", got, StorePath(root))
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("naming the plan location created %s", want)
	}
}
