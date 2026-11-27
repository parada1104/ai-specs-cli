package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corruption-containment contract (change-ledger-t6b T6b.3, S11): a corrupt
// ledger file is unevaluable and every writer fails closed with the file
// byte-identical, an unreadable predecessor is reported without disabling the
// ledger, and corruption is never "repaired" by seeding from a predecessor.

// corruptionCase is one writer under test: the call the CLI entrypoint makes for one
// capability's write.
type corruptionCase struct {
	name  string
	write func(t *testing.T, loc StoreLocation, id ItemIdentity) error
}

func corruptionWriters() []corruptionCase {
	return []corruptionCase{
		{
			name: "ApplyWrite",
			write: func(t *testing.T, loc StoreLocation, id ItemIdentity) error {
				t.Helper()
				_, err := ApplyWrite(loc, ApplyWriteRequest{
					Identity: id, Checkpoint: CheckpointApplyStart, ProviderID: "trello-mcp-workflow",
					Request: WriteRequest{Kind: WriteOpen},
				}, vtNow)
				return err
			},
		},
		{
			name: "CloseBranchItem",
			write: func(t *testing.T, loc StoreLocation, id ItemIdentity) error {
				t.Helper()
				_, _, err := CloseBranchItem(loc, id.CommonDir, id.Branch, CheckpointArchiveClose, vtNow)
				return err
			},
		},
		{
			name: "PersistDecision",
			write: func(t *testing.T, loc StoreLocation, id ItemIdentity) error {
				t.Helper()
				_, err := PersistDecision(loc, id.Key(), DecisionRequest{
					Checkpoint: CheckpointPreMerge, Kind: DecisionAdjudicate, Choice: "local",
				}, vtNow)
				return err
			},
		},
		{
			name: "RecordPlanDecision",
			write: func(t *testing.T, loc StoreLocation, id ItemIdentity) error {
				t.Helper()
				_, err := RecordPlanDecision(loc, id, PlanDecision{
					At: "2026-11-27T11:00:00Z", Kind: PlanKindWorkstream, Reason: "corrupt ledger",
				})
				return err
			},
		},
	}
}

// TestEveryWriterFailsClosedOnACorruptLedger is T6b.3's first half: each capability's
// writer returns an error, writes nothing, leaves the corrupt bytes exactly as found
// and never leaves a temporary file behind.
func TestEveryWriterFailsClosedOnACorruptLedger(t *testing.T) {
	for _, tc := range corruptionWriters() {
		t.Run(tc.name, func(t *testing.T) {
			common := filepath.Join(t.TempDir(), ".git")
			loc := LedgerAt(common)
			if err := os.MkdirAll(filepath.Dir(loc.Path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(loc.Path, []byte(`{"v":2,"worktree":{"items":[`), 0o600); err != nil {
				t.Fatal(err)
			}
			before := rawBytes(t, loc.Path)

			err := tc.write(t, loc, ItemIdentity{CommonDir: common, Branch: "change/corruption"})
			if err == nil {
				t.Fatal("a write over a corrupt ledger succeeded")
			}
			var corrupt *StoreCorruptError
			if !errors.As(err, &corrupt) {
				t.Fatalf("write error = %v, want a *StoreCorruptError", err)
			}
			if got := rawBytes(t, loc.Path); string(got) != string(before) {
				t.Fatalf("the corrupt ledger was rewritten:\nbefore=%s\nafter=%s", before, got)
			}
			assertNoTempResidue(t, filepath.Dir(loc.Path), filepath.Base(loc.Path))
		})
	}
}

// TestUnreadableMigrationSourceDoesNotDisableWrites is T6b.3's second half for T5's
// tracker file: the source is reported, the v2 file's rows still answer, and a write
// still lands.
func TestUnreadableMigrationSourceDoesNotDisableWrites(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, ".git")
	id := ItemIdentity{CommonDir: common, Branch: "change/unreadable-migration"}
	seed := newFixture()
	seed.openItem(id, "trello-mcp-workflow", t0)
	if _, err := StoreAt(StorePath(common)).Save(seed.File, SectionWorktree, SectionTracker, SectionPlan); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A directory where the migration source belongs: present, never readable.
	source := TrackerStorePath(common)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	file, report, err := LedgerAt(common).Load()
	if err != nil {
		t.Fatalf("Load: %v (an unreadable predecessor must not invalidate the ledger)", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("LegacyUnreadable is nil; an unreadable migration source must be reported")
	}
	if report.LegacyRows != source {
		t.Fatalf("LegacyRows = %q, want the migration source %q", report.LegacyRows, source)
	}
	if _, err := file.View().Primary(id.Key()); err != nil {
		t.Fatalf("the ledger does not answer its own row: %v", err)
	}

	out, _, err := CloseBranchItem(LedgerAt(common), id.CommonDir, id.Branch, CheckpointArchiveClose, vtNow)
	if err != nil {
		t.Fatalf("close over an unreadable migration source: %v", err)
	}
	if !out.Applied {
		t.Fatalf("close = %+v, want applied", out)
	}
	if status := loadFile(t, LedgerAt(common)).Worktree.Items[0].Status; status != StatusClosed {
		t.Fatalf("row status = %q, want the close to have landed", status)
	}
}

// TestUnreadablePlanSourceDoesNotDisableWrites is the same contract for the plan
// predecessor: the source is reported, the ledger's own plan row answers, and a plan
// write still lands. A source that cannot be read is never read as an empty one.
func TestUnreadablePlanSourceDoesNotDisableWrites(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, ".git")
	id := ItemIdentity{CommonDir: common, Branch: "change/unreadable-plan"}
	source := filepath.Join(root, ".ai-specs", "plan-ledger.jsonl")
	seed := newFixture()
	seed.File.Plan.Items = append(seed.File.Plan.Items, PlanItem{
		Identity:  id,
		Decisions: []PlanDecision{{At: "2026-11-20T09:00:00Z", Kind: PlanKindWorkstream, Reason: "spike"}},
	})
	if _, err := StoreAt(StorePath(common)).Save(seed.File, SectionWorktree, SectionTracker, SectionPlan); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A directory where the predecessor belongs: present, never readable.
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	loc := LedgerAt(common).WithPlanSource(source, id)

	file, report, err := loc.Load()
	if err != nil {
		t.Fatalf("Load: %v (an unreadable predecessor must not invalidate the ledger)", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("LegacyUnreadable is nil; an unreadable predecessor must be reported")
	}
	if report.LegacyRows != source {
		t.Fatalf("LegacyRows = %q, want the predecessor %q", report.LegacyRows, source)
	}
	if report.Absorbed != 0 {
		t.Fatalf("Absorbed = %d, want nothing: an unreadable predecessor is reported, not read as empty", report.Absorbed)
	}
	if declared, reason := file.Plan.Workstream(id.Key()); !declared || reason != "spike" {
		t.Fatalf("Workstream() = %v, %q; want the ledger file's own row to answer", declared, reason)
	}

	if _, err := RecordPlanDecision(loc, id, PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: PlanKindWorkstreamEnd,
	}); err != nil {
		t.Fatalf("RecordPlanDecision over an unreadable predecessor: %v", err)
	}

	after, report, err := loc.Load()
	if err != nil {
		t.Fatalf("Load after the write: %v", err)
	}
	if report.LegacyUnreadable == nil {
		t.Fatal("the unreadable predecessor stopped being reported once the ledger held a row")
	}
	decisions := after.Plan.Items[0].Decisions
	if len(decisions) != 2 || decisions[1].Kind != PlanKindWorkstreamEnd {
		t.Fatalf("decisions = %+v, want the seeded row and the new end", decisions)
	}
	if declared, _ := after.Plan.Workstream(id.Key()); declared {
		t.Fatal("the workstream is still declared after the end was recorded")
	}
}

// TestCorruptLedgerIsNeverSeededFromThePredecessor pins that corruption is not a
// repair trigger: with a corrupt v2 file and a valid plan predecessor, a write fails
// and neither file is touched.
func TestCorruptLedgerIsNeverSeededFromThePredecessor(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, ".git")
	id := ItemIdentity{CommonDir: common, Branch: "change/corrupt-plan"}
	source := filepath.Join(root, ".ai-specs", "plan-ledger.jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]string{
		"event": PlanKindWorkstream, "at": "2026-11-20T09:00:00Z", "reason": "spike",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, append(event, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	loc := LedgerAt(common).WithPlanSource(source, id)
	if err := os.MkdirAll(filepath.Dir(loc.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loc.Path, []byte(`{"v":2,"plan":{"items":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	fileBefore := rawBytes(t, loc.Path)
	sourceBefore := rawBytes(t, source)

	if _, err := RecordPlanDecision(loc, id, PlanDecision{
		At: "2026-11-27T11:00:00Z", Kind: PlanKindWorkstreamEnd,
	}); err == nil {
		t.Fatal("a plan write over a corrupt ledger succeeded")
	}
	if got := rawBytes(t, loc.Path); string(got) != string(fileBefore) {
		t.Fatalf("the corrupt ledger was repaired from the predecessor:\nbefore=%s\nafter=%s", fileBefore, got)
	}
	if got := rawBytes(t, source); string(got) != string(sourceBefore) {
		t.Fatalf("the predecessor was rewritten while the ledger was corrupt: %q", got)
	}
	// Still unevaluable, still the same bytes: nothing was healed behind the caller.
	if _, _, err := loc.Load(); err == nil {
		t.Fatal("the corrupt ledger became readable after a failed write")
	}
	if got := rawBytes(t, loc.Path); string(got) != string(fileBefore) {
		t.Fatalf("the corrupt ledger changed after a read:\nbefore=%s\nafter=%s", fileBefore, got)
	}
}

// assertNoTempResidue requires dir to hold no writeAtomic scratch file for name.
func assertNoTempResidue(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), name+".tmp.") {
			t.Fatalf("a temp file survived the failed write: %s", filepath.Join(dir, entry.Name()))
		}
	}
}
