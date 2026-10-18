package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestEvidenceConflictFourSidesNoDefaultWinner pins the conflict predicate: any
// available side that disagrees with the ledger snapshot is a conflict, while an
// empty side is unavailable and never a winner.
func TestEvidenceConflictFourSidesNoDefaultWinner(t *testing.T) {
	cases := []struct {
		name string
		ev   Evidence
		want bool
	}{
		{"four distinct sides disagree", Evidence{Local: "local", Remote: "remote", Code: "code", Git: "git"}, true},
		{"all four sides agree", Evidence{Local: "x", Remote: "x", Code: "x", Git: "x"}, false},
		{"local with unavailable others is consistent", Evidence{Local: "x"}, false},
		{"local present disagrees with remote", Evidence{Local: "x", Remote: "y"}, true},
		{"missing local disagrees with a present side", Evidence{Remote: "remote"}, true},
		{"two non-local sides disagree without local", Evidence{Remote: "a", Code: "b"}, true},
		{"all sides unavailable", Evidence{}, false},
	}
	for _, tc := range cases {
		if got := tc.ev.Conflict(); got != tc.want {
			t.Fatalf("%s: Conflict() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDecisionChoiceEnumPinned pins the design human-choice vocabulary.
func TestDecisionChoiceEnumPinned(t *testing.T) {
	want := []string{"local", "remote", "code", "git", "exempt", "continue"}
	if !reflect.DeepEqual(DecisionChoices, want) {
		t.Fatalf("DecisionChoices = %v, want %v", DecisionChoices, want)
	}
}

// TestPersistDecisionThenRegradeAllows pins the adjudication flow: the human
// choice is appended, the conflict is no longer current, and the next grade of
// the same input allows.
func TestPersistDecisionThenRegradeAllows(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, storeWithOpenItem("local")); err != nil {
		t.Fatal(err)
	}
	key := ident("").Key()
	in := Input{Checkpoint: CheckpointPreMerge, Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(),
		Store: mustLoad(t, path), Evidence: Evidence{Local: "local", Remote: "remote"}, Now: vtNow}
	if got := Grade(in); got.Decision != DecisionAsk {
		t.Fatalf("pre-decision grade = %q, want ask", got.Decision)
	}

	if _, err := PersistDecision(path, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Kind: DecisionAdjudicate, Choice: "local"}, vtNow); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}

	in.Store = mustLoad(t, path)
	got := Grade(in)
	assertVerdict(t, "post-decision", got, vWant{DecisionAllow, ReasonAdjudicated, 0, SeverityOK, true})

	item, err := in.Store.Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Decisions) != 2 {
		t.Fatalf("decisions = %+v, want the open decision plus the adjudicate", item.Decisions)
	}
	last := item.Decisions[len(item.Decisions)-1]
	if last.Kind != DecisionAdjudicate || last.Choice != "local" || last.Checkpoint != CheckpointPreMerge {
		t.Fatalf("persisted decision = %+v, want adjudicate/local/pre-merge", last)
	}
	if last.At != vtNow.UTC().Format(time.RFC3339) {
		t.Fatalf("decision at = %q, want the injected clock", last.At)
	}
}

// TestPersistDecisionClearsCurrentConflictSnapshot pins that an adjudication
// clears the item's current conflict snapshot (A6), so doctor stops warning.
func TestPersistDecisionClearsCurrentConflictSnapshot(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, storeWithOpenItem("local")); err != nil {
		t.Fatal(err)
	}
	key := ident("").Key()
	snapshot := Conflict{Sides: Evidence{Local: "local", Remote: "remote"}, RecordedAt: vtNow.Format(time.RFC3339)}
	if err := PersistConflict(path, key, snapshot); err != nil {
		t.Fatalf("PersistConflict: %v", err)
	}
	item, err := mustLoad(t, path).Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	var recorded Conflict
	if err := json.Unmarshal(item.Conflict, &recorded); err != nil {
		t.Fatalf("stored conflict is not a Conflict snapshot: %v (%s)", err, item.Conflict)
	}
	if recorded.Sides != snapshot.Sides {
		t.Fatalf("stored conflict sides = %+v, want %+v", recorded.Sides, snapshot.Sides)
	}

	if _, err := PersistDecision(path, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "remote"}, vtNow); err != nil {
		t.Fatalf("PersistDecision: %v", err)
	}
	item, err = mustLoad(t, path).Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	var after *Conflict
	if err := json.Unmarshal(item.Conflict, &after); err != nil {
		t.Fatalf("cleared conflict is not valid JSON: %v (%s)", err, item.Conflict)
	}
	if after != nil {
		t.Fatalf("conflict snapshot after adjudication = %+v, want cleared", after)
	}
}

// TestPersistDecisionFailsClosed pins every persist failure path: the caller
// must exit 2 rather than pretend the human answer was recorded.
func TestPersistDecisionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	good := StorePath(filepath.Join(dir, ".git"))
	if err := SaveStore(good, storeWithOpenItem("local")); err != nil {
		t.Fatal(err)
	}
	key := ident("").Key()

	corrupt := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	twoOpen := StorePath(filepath.Join(dir, "two", ".git"))
	s := Store{V: StoreVersion}
	s.OpenItem(ident(""), "p", t0)
	s.OpenItem(ident(""), "p", t0)
	if err := SaveStore(twoOpen, s); err != nil {
		t.Fatal(err)
	}

	empty := StorePath(filepath.Join(dir, "empty", ".git"))
	if err := SaveStore(empty, Store{V: StoreVersion}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		key  string
		req  DecisionRequest
	}{
		{"no primary item", empty, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "local"}},
		{"missing checkpoint", good, key, DecisionRequest{Choice: "local"}},
		{"unknown choice", good, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "bogus"}},
		{"unknown kind", good, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Kind: "bogus", Choice: "local"}},
		{"two open items", twoOpen, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "local"}},
		{"corrupt store", corrupt, key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "local"}},
		{"path under a file", filepath.Join(blocker, "state.json"), key, DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "local"}},
	}
	for _, tc := range cases {
		if _, err := PersistDecision(tc.path, tc.key, tc.req, vtNow); err == nil {
			t.Fatalf("%s: PersistDecision succeeded, want a closed failure", tc.name)
		}
	}
}

// TestPersistScopedOptOutWithoutPrimaryIsLifecycleScoped pins the fresh-binding
// ask path (A5): with no open item yet, an explicit opt-out is recorded on its
// own, allows its own checkpoint, suppresses the later checkpoints of the same
// identity/change, and never synthesizes a tracked item.
func TestPersistScopedOptOutWithoutPrimaryIsLifecycleScoped(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, Store{V: StoreVersion}); err != nil {
		t.Fatal(err)
	}
	key := ident("").Key()
	if _, err := PersistDecision(path, key, DecisionRequest{Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue"}, vtNow); err != nil {
		t.Fatalf("PersistDecision opt-out without a primary: %v", err)
	}

	store := mustLoad(t, path)
	if len(store.Items) != 0 {
		t.Fatalf("items = %+v, want no synthesized item", store.Items)
	}
	if len(store.OptOuts) != 1 {
		t.Fatalf("opt_outs = %+v, want exactly one scoped opt-out", store.OptOuts)
	}
	got := store.OptOuts[0]
	if got.Key != key || got.Checkpoint != CheckpointApplyStart || got.Scope != ScopeLifecycle ||
		got.Choice != "continue" || got.At != vtNow.UTC().Format(time.RFC3339) {
		t.Fatalf("scoped opt-out = %+v, want the human answer keyed, stamped, lifecycle-scoped and audited at its checkpoint", got)
	}

	base := Input{Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(), Store: store, Now: vtNow}
	apply := base
	apply.Checkpoint = CheckpointApplyStart
	assertVerdict(t, "apply-start scoped opt-out", Grade(apply), vWant{DecisionAllow, ReasonOptOut, 0, SeverityOK, true})

	pre := base
	pre.Checkpoint = CheckpointPreMerge
	assertVerdict(t, "pre-merge after scoped opt-out", Grade(pre), vWant{DecisionAllow, ReasonOptOut, 0, SeverityOK, true})
}

// TestPersistWithoutPrimaryStaysClosedForUnrecordableAnswers pins the boundary of
// the scoped opt-out: an adjudication has no item, no conflict snapshot and no
// sides, and an identity-unavailable key is not durable, so both stay closed.
func TestPersistWithoutPrimaryStaysClosedForUnrecordableAnswers(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, Store{V: StoreVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistDecision(path, ident("").Key(), DecisionRequest{Checkpoint: CheckpointPreMerge, Choice: "local"}, vtNow); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("adjudication without a primary error = %v, want ErrNoPrimary", err)
	}
	if _, err := PersistDecision(path, "", DecisionRequest{Checkpoint: CheckpointPreMerge, Kind: DecisionOptOut, Choice: "continue"}, vtNow); err == nil {
		t.Fatalf("keyless opt-out succeeded, want a closed failure")
	}
	if store := mustLoad(t, path); len(store.OptOuts) != 0 {
		t.Fatalf("opt_outs = %+v, want nothing recorded for an unrecordable answer", store.OptOuts)
	}
}

// TestPersistConflictFailsClosedForCollision pins that a collided identity cannot
// silently attach a conflict to one of the open items.
func TestPersistConflictFailsClosedForCollision(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	s := Store{V: StoreVersion}
	s.OpenItem(ident(""), "p", t0)
	s.OpenItem(ident(""), "p", t0)
	if err := SaveStore(path, s); err != nil {
		t.Fatal(err)
	}
	if err := PersistConflict(path, ident("").Key(), Conflict{Sides: Evidence{Local: "a", Remote: "b"}}); !errors.Is(err, ErrMultipleOpen) {
		t.Fatalf("PersistConflict error = %v, want ErrMultipleOpen", err)
	}
}

// TestPersistOptOutIsLifecycleScoped pins the revised opt-out lifecycle: the
// persisted decline is remembered for the current change, so the answered
// checkpoint allows and a later checkpoint no longer asks.
func TestPersistOptOutIsLifecycleScoped(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, storeWithOpenItem("local")); err != nil {
		t.Fatal(err)
	}
	key := ident("").Key()
	if _, err := PersistDecision(path, key, DecisionRequest{Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue"}, vtNow); err != nil {
		t.Fatalf("PersistDecision opt-out: %v", err)
	}

	store := mustLoad(t, path)
	item, err := store.Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	last := item.Decisions[len(item.Decisions)-1]
	if last.Kind != DecisionOptOut || last.Scope != ScopeLifecycle || last.Checkpoint != CheckpointApplyStart {
		t.Fatalf("persisted opt-out = %+v, want a lifecycle-scoped decision audited at apply-start", last)
	}

	base := Input{Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(), Store: store,
		Evidence: Evidence{Local: "local", Remote: "remote"}, Now: vtNow}
	apply := base
	apply.Checkpoint = CheckpointApplyStart
	assertVerdict(t, "apply-start opt-out", Grade(apply), vWant{DecisionAllow, ReasonOptOut, 0, SeverityOK, true})

	pre := base
	pre.Checkpoint = CheckpointPreMerge
	assertVerdict(t, "pre-merge after opt-out", Grade(pre), vWant{DecisionAllow, ReasonOptOut, 0, SeverityOK, true})
}

// TestLegacyOptOutRecordsStayCheckpointScopedOnDisk pins on-disk backward
// compatibility: a store written before lifecycle scoping carries no scope field,
// loads unchanged, and keeps covering only its own checkpoint.
func TestLegacyOptOutRecordsStayCheckpointScopedOnDisk(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	id := ident("")
	legacy := Store{V: StoreVersion}
	legacy.OpenItem(id, "trello-mcp-workflow", t0)
	legacy.Items[0].ItemID = "local"
	legacy.Items[0].Decisions = append(legacy.Items[0].Decisions,
		Decision{At: t0.Format(time.RFC3339), Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue"})
	legacy.OptOuts = append(legacy.OptOuts,
		ScopedOptOut{Key: id.Key(), Checkpoint: CheckpointApplyStart, Choice: "continue", At: t0.Format(time.RFC3339)})
	if err := SaveStore(path, legacy); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"scope"`)) {
		t.Fatalf("a legacy record must not carry a scope field: %s", raw)
	}

	loaded := mustLoad(t, path)
	if got := loaded.Items[0].Decisions[len(loaded.Items[0].Decisions)-1].Scope; got != "" {
		t.Fatalf("legacy decision scope = %q, want empty", got)
	}
	if loaded.OptOuts[0].Scope != "" {
		t.Fatalf("legacy scoped opt-out = %+v, want no scope", loaded.OptOuts[0])
	}
	if !loaded.HasOptOut(id.Key(), CheckpointApplyStart) || loaded.HasOptOut(id.Key(), CheckpointPreMerge) {
		t.Fatal("a legacy item opt-out must stay checkpoint-scoped")
	}
	if !loaded.HasScopedOptOut(id.Key(), CheckpointApplyStart) || loaded.HasScopedOptOut(id.Key(), CheckpointPreMerge) {
		t.Fatal("a legacy scoped opt-out must stay checkpoint-scoped")
	}
}

// mustLoad loads a store and fails on error.
func mustLoad(t *testing.T, path string) Store {
	t.Helper()
	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore(%s): %v", path, err)
	}
	return store
}
