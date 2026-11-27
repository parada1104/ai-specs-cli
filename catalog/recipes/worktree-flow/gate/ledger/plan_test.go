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
// losing an event, the row is keyed by the checkout so a branch rename cannot lose its
// end, and a predecessor that cannot be read is reported rather than fatal.

// planFixture is a ledger file plus the checkout's JSONL predecessor beside it.
type planFixture struct {
	location ledger.StoreLocation
	ident    ledger.PlanIdentity
	source   string
	root     string
}

func newPlanFixture(t *testing.T) planFixture {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "checkout")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(dir, ".git")
	ident := ledger.PlanIdentity{CommonDir: common, Root: root, Branch: "feat-plan"}
	source := filepath.Join(dir, ".ai-specs", "plan-ledger.jsonl")
	return planFixture{
		location: ledger.LedgerAt(common).WithPlanSource(source, ident),
		ident:    ident,
		source:   source,
		root:     root,
	}
}

func (f planFixture) withIdent(t *testing.T, ident ledger.PlanIdentity) ledger.StoreLocation {
	t.Helper()
	return ledger.LedgerAt(ident.CommonDir).WithPlanSource(f.source, ident)
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

// recordPlan is the fixture's shorthand for one plan write.
func (f planFixture) recordPlan(t *testing.T, kind, at string) {
	t.Helper()
	if _, err := ledger.RecordPlanDecision(f.location, f.ident, ledger.PlanDecision{At: at, Kind: kind}); err != nil {
		t.Fatalf("RecordPlanDecision(%s): %v", kind, err)
	}
}

// TestPlanDecisionLandsOnlyInThePlanSection is S3's evidence for the plan
// capability: the worktree and tracker sections answer byte for byte what they
// answered before, and no row is written twice.
func TestPlanDecisionLandsOnlyInThePlanSection(t *testing.T) {
	fixture := newPlanFixture(t)
	seed := ledger.EmptyLedger()
	seed.Worktree.Items = append(seed.Worktree.Items, ledger.WorktreeItem{
		ID: "item-1", Identity: ledger.ItemIdentity{CommonDir: fixture.ident.CommonDir, Branch: fixture.ident.Branch},
		Status: ledger.StatusOpen, Decisions: []ledger.WorktreeDecision{},
	})
	seed.Tracker.Items = append(seed.Tracker.Items, ledger.Item{
		ID: "item-1", ItemID: "card-1",
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
	if row.Identity.Root != fixture.root || row.Identity.Branch != "feat-plan" {
		t.Fatalf("plan row identity = %+v, want the checkout root and the branch as data", row.Identity)
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
	want := []string{ledger.PlanKindWorkstream, ledger.PlanKindWorkstreamEnd, ledger.PlanKindWorkstream}
	if len(decisions) != len(want) {
		t.Fatalf("decisions = %+v, want the three JSONL events", decisions)
	}
	for i, kind := range want {
		if decisions[i].Kind != kind {
			t.Fatalf("decision %d kind = %q, want %q", i, decisions[i].Kind, kind)
		}
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

// TestPlanSectionFileAnswersOverThePredecessor pins the newest-form-wins rule: a
// decision the ledger file already holds is not resurrected from the JSONL an older
// binary appended to (L7).
func TestPlanSectionFileAnswersOverThePredecessor(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.recordPlan(t, ledger.PlanKindWorkstreamEnd, "2026-11-27T11:00:00Z")
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

	fixture.recordPlan(t, ledger.PlanKindWorkstreamEnd, "2026-11-27T11:00:00Z")

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

// TestPlanSectionSeedsOnlyTheOwningCheckout: two checkouts share one ledger file, and
// the predecessor of one never speaks for the other.
func TestPlanSectionSeedsOnlyTheOwningCheckout(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))

	file, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sibling := ledger.PlanIdentity{CommonDir: fixture.ident.CommonDir, Root: t.TempDir(), Branch: "feat-sibling"}
	if declared, _ := file.Plan.Workstream(sibling.Key()); declared {
		t.Fatal("a sibling checkout inherited the declaration")
	}
}

// TestBranchRenameKeepsTheDeclarationEnded is R1-001: the row is keyed by the
// checkout, so a renamed branch neither loses the end nor resurrects the declaration,
// and the branch travels as data so a report can still name it.
func TestBranchRenameKeepsTheDeclarationEnded(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))
	fixture.recordPlan(t, ledger.PlanKindWorkstreamEnd, "2026-11-27T11:00:00Z")

	renamed := fixture.ident
	renamed.Branch = "feat-renamed"
	file, report, err := fixture.withIdent(t, renamed).Load()
	if err != nil {
		t.Fatalf("Load after rename: %v", err)
	}
	if declared, reason := file.Plan.Workstream(renamed.Key()); declared || reason != "" {
		t.Fatalf("Workstream() = %v, %q; a rename must not resurrect the declaration", declared, reason)
	}
	if len(file.Plan.Items) != 1 {
		t.Fatalf("plan rows = %d, want the one row", len(file.Plan.Items))
	}
	if file.Plan.Items[0].Identity.Branch != "feat-renamed" {
		t.Fatalf("stored branch = %q, want the branch as data refreshed to the new name",
			file.Plan.Items[0].Identity.Branch)
	}
	if report.Absorbed != 0 {
		t.Fatalf("Absorbed = %d, want nothing absorbed after the rename", report.Absorbed)
	}
}

// TestMovedCheckoutCarriesItsDeclaration is the other half of R1-001: a checkout whose
// folder moved carries its end, because the row whose root no longer exists cannot
// answer for anything, while a live sibling checkout is never adopted.
func TestMovedCheckoutCarriesItsDeclaration(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.recordPlan(t, ledger.PlanKindWorkstream, "2026-11-27T11:00:00Z")
	fixture.recordPlan(t, ledger.PlanKindWorkstreamEnd, "2026-11-27T12:00:00Z")

	moved := filepath.Join(t.TempDir(), "checkout-moved")
	if err := os.MkdirAll(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fixture.root); err != nil {
		t.Fatal(err)
	}
	relocated := ledger.PlanIdentity{CommonDir: fixture.ident.CommonDir, Root: moved, Branch: fixture.ident.Branch}
	file, _, err := fixture.withIdent(t, relocated).Load()
	if err != nil {
		t.Fatalf("Load after moving the checkout: %v", err)
	}
	if declared, reason := file.Plan.Workstream(relocated.Key()); declared || reason != "" {
		t.Fatalf("Workstream() = %v, %q; a moved checkout must carry its end", declared, reason)
	}

	// A sibling checkout whose root still exists is not adopted, even on the same
	// branch: it has its own declaration to answer for.
	live := filepath.Join(t.TempDir(), "checkout-live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := ledger.PlanIdentity{CommonDir: fixture.ident.CommonDir, Root: live, Branch: fixture.ident.Branch}
	file, _, err = fixture.withIdent(t, sibling).Load()
	if err != nil {
		t.Fatalf("Load for the live sibling: %v", err)
	}
	if declared, _ := file.Plan.Workstream(sibling.Key()); declared {
		t.Fatal("a live sibling checkout took a row that belongs to another checkout")
	}
}

// TestEmptyPlanRowIsFilledNotDuplicated is V-3: a row that exists with no decisions is
// still the answer, so the predecessor fills it instead of adding a second row whose
// stale reason would win the fold.
func TestEmptyPlanRowIsFilledNotDuplicated(t *testing.T) {
	fixture := newPlanFixture(t)
	seed := ledger.EmptyLedger()
	seed.Plan.Items = append(seed.Plan.Items, ledger.PlanItem{Identity: fixture.ident, Decisions: []ledger.PlanDecision{}})
	if _, err := fixture.location.Save(seed, ledger.SectionWorktree, ledger.SectionTracker, ledger.SectionPlan); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstreamEnd, "2026-11-20T09:00:00Z", ""))

	file, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(file.Plan.Items) != 1 {
		t.Fatalf("plan rows = %d, want the empty row to be filled", len(file.Plan.Items))
	}
	if declared, reason := file.Plan.Workstream(fixture.ident.Key()); declared || reason != "" {
		t.Fatalf("Workstream() = %v, %q; want the predecessor's end to answer", declared, reason)
	}

	fixture.recordPlan(t, ledger.PlanKindWorkstream, "2026-11-27T11:00:00Z")
	if declared, _ := fixture.readFile(t).Plan.Workstream(fixture.ident.Key()); !declared {
		t.Fatal("the new decision did not land in the only row")
	}
}

// TestAbsentPredecessorIsNotReported is R3-001: a checkout that never had a
// predecessor is ordinary absence, not an unreadable source, and it never overwrites
// another source's diagnostic.
func TestAbsentPredecessorIsNotReported(t *testing.T) {
	fixture := newPlanFixture(t)
	file, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.LegacyUnreadable != nil {
		t.Fatalf("LegacyUnreadable = %v, want nil for an absent predecessor", report.LegacyUnreadable)
	}
	if report.LegacyRows != "" {
		t.Fatalf("LegacyRows = %q, want nothing named", report.LegacyRows)
	}
	if declared, _ := file.Plan.Workstream(fixture.ident.Key()); declared {
		t.Fatal("an absent predecessor declared a workstream")
	}
}

// TestUnreadablePlanSourceIsReportedNotFatal pins MEDIO-3 for the third source: the
// ledger still answers, and the failure is said out loud instead of swallowed.
func TestUnreadablePlanSourceIsReportedNotFatal(t *testing.T) {
	fixture := newPlanFixture(t)
	// A directory where the predecessor file belongs: present, never a line file, and
	// never read (which is also what keeps a FIFO from blocking the invocation).
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

// TestPlanDecisionReportsUnreadableLines is R1-002: the read stays lenient, so an
// interrupted run never blocks the ledger, but a line nobody could read is reported.
// An event without a timestamp is still an event: the predecessor's own reader always
// honoured it, so dropping it would lose a real end.
func TestPlanDecisionReportsUnreadableLines(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t,
		"{not json",
		`{"at":"2026-11-19T09:00:00Z"}`,
		planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"),
		`{"event":"workstream-end"}`,
	)

	file, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("LegacyUnreadable is nil; the two unreadable lines must be reported")
	}
	decisions := file.Plan.Items[0].Decisions
	if len(decisions) != 2 {
		t.Fatalf("decisions = %+v, want the dated declaration and the undated end", decisions)
	}
	if decisions[1].Kind != ledger.PlanKindWorkstreamEnd {
		t.Fatalf("second decision = %+v, want the undated end kept", decisions[1])
	}
	if declared, _ := file.Plan.Workstream(fixture.ident.Key()); declared {
		t.Fatal("the undated end did not end the declaration")
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

// TestRecordPlanDecisionNeedsACheckout refuses to invent a row with no worktree to
// attach it to.
func TestRecordPlanDecisionNeedsACheckout(t *testing.T) {
	fixture := newPlanFixture(t)
	if _, err := ledger.RecordPlanDecision(fixture.location, ledger.PlanIdentity{}, ledger.PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: ledger.PlanKindWorkstream,
	}); err == nil {
		t.Fatal("RecordPlanDecision with no checkout returned no error")
	}
	if _, err := os.Stat(fixture.location.Path); !os.IsNotExist(err) {
		t.Fatalf("a refused plan decision wrote the ledger file: %v", err)
	}
}

// TestSaveCannotWriteASectionItDoesNotOwn is T6b.2's evidence: the whole-store write
// that could land a row in the wrong ledger is gone.
func TestSaveCannotWriteASectionItDoesNotOwn(t *testing.T) {
	fixture := newPlanFixture(t)
	seed := ledger.EmptyLedger()
	seed.Worktree.Items = append(seed.Worktree.Items, ledger.WorktreeItem{
		ID: "item-1", Identity: ledger.ItemIdentity{CommonDir: fixture.ident.CommonDir, Branch: fixture.ident.Branch},
		Status: ledger.StatusOpen, Decisions: []ledger.WorktreeDecision{},
	})
	seed.Tracker.Items = append(seed.Tracker.Items, ledger.Item{
		ID: "item-1", ItemID: "card-1",
		Decisions: []ledger.Decision{{At: "2026-11-27T10:00:00Z", Kind: ledger.DecisionOpen}},
	})
	if _, err := fixture.location.Save(seed, ledger.SectionWorktree, ledger.SectionTracker); err != nil {
		t.Fatalf("seed: %v", err)
	}

	loaded, _, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded.Worktree.Items = append(loaded.Worktree.Items, ledger.WorktreeItem{
		ID: "item-2", Identity: ledger.ItemIdentity{CommonDir: fixture.ident.CommonDir, Branch: fixture.ident.Branch},
		Status: ledger.StatusClosed, Decisions: []ledger.WorktreeDecision{},
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

// TestTrackerWriteKeepsTheAbsorbedPlanRow is V-1: Save reports the rows it absorbed on
// read as part of the answer, so a write by another capability must persist them
// instead of dropping what the report promised to carry.
func TestTrackerWriteKeepsTheAbsorbedPlanRow(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.writeSource(t, planEvent(t, ledger.PlanKindWorkstream, "2026-11-20T09:00:00Z", "spike"))

	loaded, report, err := fixture.location.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if report.Absorbed == 0 {
		t.Fatal("nothing was absorbed, so the promise under test is not exercised")
	}
	loaded.Tracker.Items = append(loaded.Tracker.Items, ledger.Item{
		ID: "item-1", ItemID: "card-1",
		Decisions: []ledger.Decision{{At: "2026-11-27T10:00:00Z", Kind: ledger.DecisionOpen}},
	})
	if _, err := fixture.location.Save(loaded, ledger.SectionTracker); err != nil {
		t.Fatalf("tracker write: %v", err)
	}

	after := fixture.readFile(t)
	if declared, reason := after.Plan.Workstream(fixture.ident.Key()); !declared || reason != "spike" {
		t.Fatalf("plan row after a tracker write = %+v, want the absorbed declaration kept", after.Plan.Items)
	}
}

// TestAllSectionSaveStillFailsClosedOnACorruptLedger is V-2: naming every section does
// not buy a bypass, so the corruption contract holds for every caller and not only for
// the production writers, which happen to load first.
func TestAllSectionSaveStillFailsClosedOnACorruptLedger(t *testing.T) {
	fixture := newPlanFixture(t)
	if err := os.MkdirAll(filepath.Dir(fixture.location.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.location.Path, []byte("{ corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(fixture.location.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.location.Save(ledger.EmptyLedger(),
		ledger.SectionWorktree, ledger.SectionTracker, ledger.SectionPlan); err == nil {
		t.Fatal("an all-section save over a corrupt ledger was accepted")
	}
	after, err := os.ReadFile(fixture.location.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("the corrupt ledger was rewritten: %q", after)
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
