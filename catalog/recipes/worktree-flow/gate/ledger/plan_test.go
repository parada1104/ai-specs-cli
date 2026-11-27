package ledger_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/ledger"
)

// The plan ledger tests (change-ledger-v2 T6b, S3): one capability's decisions land
// in its own section and nowhere else, the JSONL predecessor is absorbed without
// losing an event, and a predecessor that cannot be read is reported rather than
// fatal.

// planFixture is a ledger file plus the checkout's JSONL predecessor beside it.
type planFixture struct {
	location ledger.StoreLocation
	ident    ledger.ItemIdentity
	source   string
}

func newPlanFixture(t *testing.T) planFixture {
	t.Helper()
	dir := t.TempDir()
	ident := ledger.ItemIdentity{CommonDir: filepath.Join(dir, ".git"), Branch: "feat-plan"}
	source := filepath.Join(dir, ".ai-specs", "plan-ledger.jsonl")
	return planFixture{
		location: ledger.LedgerAt(ident.CommonDir).WithPlanSource(source, ident),
		ident:    ident,
		source:   source,
	}
}

func (f planFixture) writeSource(t *testing.T, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.source), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(f.source, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f planFixture) readSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.source)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f planFixture) readFile(t *testing.T) ledger.LedgerFile {
	t.Helper()
	file, _, err := f.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return file
}

func planEvent(t *testing.T, kind, at, reason string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"event": kind, "at": at, "reason": reason})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestPlanDecisionLandsOnlyInThePlanSection is S3's evidence for the plan
// capability: the worktree and tracker sections answer byte for byte what they
// answered before, and no row is written twice.
func TestPlanDecisionLandsOnlyInThePlanSection(t *testing.T) {
	fixture := newPlanFixture(t)
	seed := ledger.EmptyLedger()
	seed.Worktree.Items = append(seed.Worktree.Items, ledger.WorktreeItem{
		ID: "item-1", Identity: fixture.ident, Status: ledger.StatusOpen,
		Decisions: []ledger.WorktreeDecision{},
	})
	seed.Tracker.Items = append(seed.Tracker.Items, ledger.Item{
		ID: "item-1", Identity: fixture.ident, ItemID: "card-1",
		Decisions: []ledger.Decision{{At: "2026-11-27T10:00:00Z", Kind: ledger.DecisionOpen}},
	})
	if _, err := fixture.location.Save(seed, ledger.SectionWorktree, ledger.SectionTracker); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := fixture.readFile(t)

	if _, err := ledger.RecordPlanDecision(fixture.location, fixture.ident, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstream, Reason: "audit of the release",
	}); err != nil {
		t.Fatalf("RecordPlanDecision: %v", err)
	}

	after := fixture.readFile(t)
	if got := mustJSON(t, after.Worktree); got != mustJSON(t, before.Worktree) {
		t.Fatalf("worktree section changed by a plan decision:\nbefore %s\nafter  %s", mustJSON(t, before.Worktree), got)
	}
	if got := mustJSON(t, after.Tracker); got != mustJSON(t, before.Tracker) {
		t.Fatalf("tracker section changed by a plan decision:\nbefore %s\nafter  %s", mustJSON(t, before.Tracker), got)
	}
	if len(after.Plan.Items) != 1 {
		t.Fatalf("plan rows = %d, want 1", len(after.Plan.Items))
	}
	row := after.Plan.Items[0]
	if row.Identity.Key() != fixture.ident.Key() {
		t.Fatalf("plan row key = %q, want %q", row.Identity.Key(), fixture.ident.Key())
	}
	if len(row.Decisions) != 1 || row.Decisions[0].Kind != ledger.PlanKindWorkstream ||
		row.Decisions[0].Reason != "audit of the release" {
		t.Fatalf("plan decisions = %+v, want one workstream decision with its reason", row.Decisions)
	}
	if declared, reason := after.Plan.Workstream(fixture.ident.Key()); !declared || reason != "audit of the release" {
		t.Fatalf("Workstream() = %v, %q; want true and the declared reason", declared, reason)
	}
}

// TestPlanSectionSeedsEveryJSONLEvent is S11's evidence for the plan half: the
// predecessor is absorbed in file order, no event is lost, and the predecessor's
// bytes are untouched by the read.
func TestPlanSectionSeedsEveryJSONLEvent(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t,
		planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"),
		planEvent(t, ledger.PlanKindWorkstreamEnd, "2026-11-21T09:00:00Z", ""),
		planEvent(t, ledger.PlanKindWorkstream, "2026-11-22T09:00:00Z", "review of T6a"),
	)
	before := fixture.readSource(t)

	file, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := fixture.readSource(t); got != before {
		t.Fatalf("reading the predecessor rewrote it:\nbefore %q\nafter  %q", before, got)
	}
	if len(file.Plan.Items) != 1 {
		t.Fatalf("plan rows = %d, want the one predecessor row", len(file.Plan.Items))
	}
	decisions := file.Plan.Items[0].Decisions
	if len(decisions) != 3 {
		t.Fatalf("decisions = %+v, want the three JSONL events", decisions)
	}
	want := []string{ledger.PlanKindWorkstream, ledger.PlanKindWorkstreamEnd, ledger.PlanKindWorkstream}
	for i, kind := range want {
		if decisions[i].Kind != kind {
			t.Fatalf("decision %d kind = %q, want %q", i, decisions[i].Kind, kind)
		}
	}
	if decisions[2].Reason != "review of T6a" {
		t.Fatalf("last decision reason = %q, want the JSONL's own text", decisions[2].Reason)
	}
	if declared, reason := file.Plan.Workstream(fixture.ident.Key()); !declared || reason != "review of T6a" {
		t.Fatalf("Workstream() = %v, %q; want the last event to win", declared, reason)
	}
	if report.Absorbed != 3 {
		t.Fatalf("Absorbed = %d, want one per JSONL event", report.Absorbed)
	}
	if report.LegacyRows != fixture.source {
		t.Fatalf("LegacyRows = %q, want the predecessor path %q", report.LegacyRows, fixture.source)
	}
}

// TestPlanSectionFileAnswersOverThePredecessor pins the newest-form-wins rule for
// the third source: a decision the ledger file already holds is not resurrected from
// the JSONL an older binary appended to (L7).
func TestPlanSectionFileAnswersOverThePredecessor(t *testing.T) {
	fixture := newPlanFixture(t)
	if _, err := ledger.RecordPlanDecision(fixture.location, fixture.ident, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstreamEnd,
	}); err != nil {
		t.Fatalf("RecordPlanDecision: %v", err)
	}
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-28T09:00:00Z", "stale append"))

	file, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if declared, reason := file.Plan.Workstream(fixture.ident.Key()); declared || reason != "" {
		t.Fatalf("Workstream() = %v, %q; want the file's current end to answer", declared, reason)
	}
	if len(file.Plan.Items[0].Decisions) != 1 {
		t.Fatalf("decisions = %+v, want only the file's own decision", file.Plan.Items[0].Decisions)
	}
	if report.Absorbed != 0 {
		t.Fatalf("Absorbed = %d, want nothing absorbed while the file answers", report.Absorbed)
	}
}

// TestPlanSectionAbsorbsThePredecessorOnTheFirstWrite pins the one-shot backup
// discipline (L7): the seeded events reach the file alongside the new decision and
// the predecessor keeps its original bytes in the backup.
func TestPlanSectionAbsorbsThePredecessorOnTheFirstWrite(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))
	before := fixture.readSource(t)

	if _, err := ledger.RecordPlanDecision(fixture.location, fixture.ident, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstreamEnd,
	}); err != nil {
		t.Fatalf("RecordPlanDecision: %v", err)
	}

	decisions := fixture.readFile(t).Plan.Items[0].Decisions
	if len(decisions) != 2 {
		t.Fatalf("decisions = %+v, want the absorbed event and the new one", decisions)
	}
	if decisions[0].Kind != ledger.PlanKindWorkstream || decisions[1].Kind != ledger.PlanKindWorkstreamEnd {
		t.Fatalf("decision order = %+v, want the predecessor's event first", decisions)
	}
	backup, err := os.ReadFile(fixture.source + ".v1-backup")
	if err != nil {
		t.Fatalf("read the predecessor backup: %v", err)
	}
	if string(backup) != before {
		t.Fatalf("backup = %q, want the predecessor's original bytes %q", backup, before)
	}
}

// TestPlanSectionSeedsOnlyTheOwningCheckout pins the per-checkout property the JSONL
// had: the shared ledger file never hands one worktree's declaration to a sibling.
func TestPlanSectionSeedsOnlyTheOwningCheckout(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))

	file, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sibling := ledger.ItemIdentity{CommonDir: fixture.ident.CommonDir, Branch: "feat-sibling"}
	if declared, _ := file.Plan.Workstream(sibling.Key()); declared {
		t.Fatal("a sibling worktree inherited the declaration")
	}
}

// TestUnreadablePlanSourceIsReportedNotFatal pins MEDIO-3 for the third source: the
// ledger still answers, and the failure is said out loud instead of swallowed.
func TestUnreadablePlanSourceIsReportedNotFatal(t *testing.T) {
	fixture := newPlanFixture(t)
	// A directory where the predecessor file belongs: readable as a path, never as a
	// ledger line file.
	if err := os.MkdirAll(fixture.source, 0o755); err != nil {
		t.Fatal(err)
	}

	file, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v (an unreadable predecessor must not invalidate the ledger)", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("LegacyUnreadable is nil; an unreadable predecessor must be reported")
	}
	if report.LegacyRows != fixture.source {
		t.Fatalf("LegacyRows = %q, want the predecessor path", report.LegacyRows)
	}
	if declared, _ := file.Plan.Workstream(fixture.ident.Key()); declared {
		t.Fatal("an unreadable predecessor declared a workstream")
	}
}

// TestPlanDecisionSkipsUnparseableLines keeps the predecessor's leniency: a line no
// reader can parse is skipped, and every line after it still counts.
func TestPlanDecisionSkipsUnparseableLines(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t,
		"{not json",
		`{"event":"unknown-kind","at":"2026-11-19T09:00:00Z"}`,
		planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"),
	)

	file, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	decisions := file.Plan.Items[0].Decisions
	if len(decisions) != 2 {
		t.Fatalf("decisions = %+v, want the unknown kind kept and the garbage line dropped", decisions)
	}
	if declared, reason := file.Plan.Workstream(fixture.ident.Key()); !declared || reason != "spike" {
		t.Fatalf("Workstream() = %v, %q; want the parseable event to answer", declared, reason)
	}
}

// TestRecordPlanDecisionFailsClosedOnACorruptLedger is T6b.3's contract for the plan
// writer: nothing is persisted and the file is byte-identical.
func TestRecordPlanDecisionFailsClosedOnACorruptLedger(t *testing.T) {
	fixture := newPlanFixture(t)
	if err := os.MkdirAll(filepath.Dir(fixture.location.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.location.Path, []byte("{ corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))
	fileBefore, err := os.ReadFile(fixture.location.Path)
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore := fixture.readSource(t)

	if _, err := ledger.RecordPlanDecision(fixture.location, fixture.ident, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstream,
	}); err == nil {
		t.Fatal("RecordPlanDecision over a corrupt ledger returned no error")
	}
	fileAfter, err := os.ReadFile(fixture.location.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(fileAfter) != string(fileBefore) {
		t.Fatalf("the corrupt ledger was rewritten: %q", fileAfter)
	}
	if got := fixture.readSource(t); got != sourceBefore {
		t.Fatalf("the predecessor was rewritten: %q", got)
	}
}

// TestRecordPlanDecisionNeedsAnIdentity refuses to invent a row with no work to
// attach it to.
func TestRecordPlanDecisionNeedsAnIdentity(t *testing.T) {
	fixture := newPlanFixture(t)
	if _, err := ledger.RecordPlanDecision(fixture.location, ledger.ItemIdentity{}, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstream,
	}); err == nil {
		t.Fatal("RecordPlanDecision with no identity returned no error")
	}
	if _, err := os.Stat(fixture.location.Path); !os.IsNotExist(err) {
		t.Fatalf("a refused plan decision wrote the ledger file: %v", err)
	}
}

// TestSaveCannotWriteASectionItDoesNotOwn is T6b.2's evidence: the whole-store write
// that could land a row in the wrong ledger is gone, because Save restores every
// section the caller did not name from disk, and a write that names none is refused.
func TestSaveCannotWriteASectionItDoesNotOwn(t *testing.T) {
	fixture := newPlanFixture(t)
	seed := ledger.EmptyLedger()
	seed.Worktree.Items = append(seed.Worktree.Items, ledger.WorktreeItem{
		ID: "item-1", Identity: fixture.ident, Status: ledger.StatusOpen,
		Decisions: []ledger.WorktreeDecision{},
	})
	seed.Tracker.Items = append(seed.Tracker.Items, ledger.Item{
		ID: "item-1", Identity: fixture.ident, ItemID: "card-1",
		Decisions: []ledger.Decision{{At: "2026-11-27T10:00:00Z", Kind: ledger.DecisionOpen}},
	})
	if _, err := fixture.location.Save(seed, ledger.SectionWorktree, ledger.SectionTracker); err != nil {
		t.Fatalf("seed: %v", err)
	}

	loaded, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A tracker-only writer hands over a file that also carries a stolen lifecycle row
	// and a stolen plan row, exactly the shape the old whole-store write accepted.
	loaded.Worktree.Items = append(loaded.Worktree.Items, ledger.WorktreeItem{
		ID: "item-2", Identity: fixture.ident, Status: ledger.StatusClosed,
		Decisions: []ledger.WorktreeDecision{},
	})
	loaded.Plan.Items = append(loaded.Plan.Items, ledger.PlanItem{
		Identity:  fixture.ident,
		Decisions: []ledger.PlanDecision{{At: "2026-11-27T12:00:00Z", Kind: ledger.PlanKindWorkstream}},
	})
	if _, err := fixture.location.Save(loaded, ledger.SectionTracker); err != nil {
		t.Fatalf("tracker write: %v", err)
	}

	after := fixture.readFile(t)
	if len(after.Worktree.Items) != 1 || after.Worktree.Items[0].ID != "item-1" {
		t.Fatalf("worktree rows = %+v, want only the row the seed wrote", after.Worktree.Items)
	}
	if len(after.Plan.Items) != 0 {
		t.Fatalf("plan rows = %+v, want none: the caller did not own the plan section", after.Plan.Items)
	}

	if _, err := fixture.location.Save(after); err == nil {
		t.Fatal("a write that names no section was accepted")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
