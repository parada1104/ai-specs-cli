package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// t0 is a fixed timestamp so open/close stamps are deterministic in assertions.
var t0 = time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)

// ident is the canonical test identity. Its key is the A5 join of common dir,
// branch, and change slug.
func ident(change string) ItemIdentity {
	return ItemIdentity{CommonDir: "/repo/.git", Branch: "change/tracker-ledger-foundation", Change: change}
}

// TestStorePathDesignAuthority pins A3: the store is a single JSON object under
// the Git common dir, shared across worktrees, never under openspec/**.
func TestStorePathDesignAuthority(t *testing.T) {
	common := filepath.Join("/repo", ".git")
	if got, want := StorePath(common), filepath.Join(common, "ai-specs", "ledger", "state.json"); got != want {
		t.Fatalf("store path = %q, want %q", got, want)
	}
}

// TestLoadStoreMissingIsEmpty pins "a missing file reads as an empty item set":
// a fresh clone or unsynced checkout is not an error and never a synthesized item.
func TestLoadStoreMissingIsEmpty(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("missing store must not error: %v", err)
	}
	if len(store.Items) != 0 {
		t.Fatalf("missing store items = %d, want 0", len(store.Items))
	}
	if store.V != StoreVersion {
		t.Fatalf("store version = %d, want %d", store.V, StoreVersion)
	}
}

// TestLoadStoreCorruptIsUnevaluable pins "corrupt JSON is unevaluable
// (reason=store-corrupt), never a synthesized item": the error is typed, the
// returned store carries no invented rows, and the reason is design-pinned.
func TestLoadStoreCorruptIsUnevaluable(t *testing.T) {
	if ReasonStoreCorrupt != "store-corrupt" {
		t.Fatalf("corrupt reason = %q, want store-corrupt", ReasonStoreCorrupt)
	}
	cases := []struct {
		name string
		body string
	}{
		{"invalid json", `{"v":1,"items":[`},
		{"not an object", `[]`},
		{"unknown version", `{"v":2,"items":[]}`},
		{"empty file", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := LoadStore(path)
			var corrupt *StoreCorruptError
			if !errors.As(err, &corrupt) {
				t.Fatalf("error = %v, want *StoreCorruptError", err)
			}
			if corrupt.Path != path {
				t.Fatalf("corrupt path = %q, want %q", corrupt.Path, path)
			}
			if len(store.Items) != 0 {
				t.Fatalf("corrupt store must not synthesize items: %+v", store.Items)
			}
			// The corrupt file is left exactly as found: never repaired silently.
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != tc.body {
				t.Fatalf("corrupt store was rewritten: %q", after)
			}
		})
	}
}

// TestSaveStoreAtomicRenameLeavesNoTempResidue pins the atomic-write contract:
// the destination exists and parses, no state.json.tmp.* file survives, and a
// first save creates the nested common-dir ledger directory.
func TestSaveStoreAtomicRenameLeavesNoTempResidue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".git", "ai-specs", "ledger")
	path := filepath.Join(dir, "state.json")

	var store Store
	store.OpenItem(ident("tracker-ledger-foundation"), "trello-mcp-workflow", t0)
	if err := SaveStore(path, store); err != nil {
		t.Fatalf("save: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "state.json.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp residue after save: %v", matches)
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("round-trip items = %d, want 1", len(loaded.Items))
	}
	if loaded.Items[0].ProviderID != "trello-mcp-workflow" {
		t.Fatalf("round-trip provider id = %q", loaded.Items[0].ProviderID)
	}

	// A later save replaces the file rather than appending to it.
	if err := SaveStore(path, Store{}); err != nil {
		t.Fatal(err)
	}
	replaced, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Items) != 0 {
		t.Fatalf("second save must replace, got %d items", len(replaced.Items))
	}

	// A store with no items still writes a JSON array, not null.
	empty := filepath.Join(t.TempDir(), ".git", "ai-specs", "ledger", "state.json")
	if err := SaveStore(empty, Store{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"items": []`) {
		t.Fatalf("empty store bytes = %s, want an items array", raw)
	}
}

// TestStoreIdentityKeyExactJoin pins the A5 key format
// common_dir+"\x1f"+branch[+"\x1f"+change] as the store's matching key: a
// change-enriched item is not matched by the branch-only or another-change key.
func TestStoreIdentityKeyExactJoin(t *testing.T) {
	id := ident("tracker-ledger-foundation")
	if got, want := id.Key(), IdentityKey("/repo/.git", "change/tracker-ledger-foundation", "tracker-ledger-foundation"); got != want {
		t.Fatalf("item key = %q, want %q", got, want)
	}

	var store Store
	item := store.OpenItem(id, "trello-mcp-workflow", t0)
	if item.Key() != id.Key() {
		t.Fatalf("item key = %q, want %q", item.Key(), id.Key())
	}

	if _, err := store.Primary(id.Key()); err != nil {
		t.Fatalf("full key must select the item: %v", err)
	}
	for _, miss := range []string{
		IdentityKey("/repo/.git", "change/tracker-ledger-foundation", ""),
		IdentityKey("/repo/.git", "change/tracker-ledger-foundation", "other-change"),
		IdentityKey("/repo/.git", "another-branch", "tracker-ledger-foundation"),
		IdentityKey("/other/.git", "change/tracker-ledger-foundation", "tracker-ledger-foundation"),
	} {
		if _, err := store.Primary(miss); !errors.Is(err, ErrNoPrimary) {
			t.Fatalf("key %q must not match (err = %v)", miss, err)
		}
	}
}

// TestNewItemIDIsStable16Hex pins the design id: 16 hex chars of
// sha256(identity key + opened-at), stable for one input and unique per input.
func TestNewItemIDIsStable16Hex(t *testing.T) {
	stamp := t0.Format(time.RFC3339)
	id := NewItemID(ident("slug").Key(), stamp)
	if len(id) != 16 {
		t.Fatalf("item id length = %d (%q), want 16", len(id), id)
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("item id %q is not lowercase hex", id)
		}
	}
	if got := NewItemID(ident("slug").Key(), stamp); got != id {
		t.Fatalf("id is not deterministic: %q vs %q", got, id)
	}
	if NewItemID(ident("slug").Key(), "2026-09-13T00:00:01Z") == id {
		t.Fatal("different opened-at must produce a different id")
	}
	if NewItemID(ident("other").Key(), stamp) == id {
		t.Fatal("different identity must produce a different id")
	}
}

// TestClosedItemIsNeverSelectedOrReopened pins D17's negative half: a closed
// item stays closed and is never the primary, and no reopen path exists.
func TestClosedItemIsNeverSelectedOrReopened(t *testing.T) {
	id := ident("old-work")
	var store Store
	item := store.OpenItem(id, "trello-mcp-workflow", t0)
	if err := store.CloseItem(item.ID, Decision{At: "2026-09-13T01:00:00Z", Checkpoint: "archive-close", Note: "merged"}); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := store.Primary(id.Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("closed item must not be primary (err = %v)", err)
	}
	if store.Items[0].Status != StatusClosed {
		t.Fatalf("status = %q, want %q", store.Items[0].Status, StatusClosed)
	}
	decisions := store.Items[0].Decisions
	if len(decisions) == 0 || decisions[len(decisions)-1].Kind != DecisionClose {
		t.Fatalf("close decision missing: %+v", decisions)
	}
	if err := store.CloseItem("does-not-exist", Decision{At: "x"}); err == nil {
		t.Fatal("closing an unknown item must error")
	}
}

// TestLatestClosedReportsLatestClosedRowWithoutPrimary pins the read-only report
// selector: LatestClosed returns the most recently stored closed row for a key,
// and that row is never primary, so new work on a reused branch still selects the
// new open row (D17).
func TestLatestClosedReportsLatestClosedRowWithoutPrimary(t *testing.T) {
	id := ident("old-work")
	var store Store
	if _, ok := store.LatestClosed(id.Key()); ok {
		t.Fatal("an empty store has no closed row")
	}
	first := store.OpenItem(id, "trello-mcp-workflow", t0)
	if err := store.CloseItem(first.ID, Decision{At: "2026-09-13T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	got, ok := store.LatestClosed(id.Key())
	if !ok || got.ID != first.ID || got.Status != StatusClosed {
		t.Fatalf("LatestClosed = %+v/%v, want the closed row %q", got, ok, first.ID)
	}
	if _, err := store.Primary(id.Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("a closed row must never be primary (err = %v)", err)
	}

	// A newer close for the same key is the row reported.
	second := store.OpenItem(id, "trello-mcp-workflow", t0.Add(time.Minute))
	if err := store.CloseItem(second.ID, Decision{At: "2026-09-13T02:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.LatestClosed(id.Key()); got.ID != second.ID {
		t.Fatalf("LatestClosed = %q, want the newer closed row %q", got.ID, second.ID)
	}

	// New work still opens its own primary and LatestClosed never returns it.
	third := store.OpenItem(id, "trello-mcp-workflow", t0.Add(2*time.Minute))
	primary, err := store.Primary(id.Key())
	if err != nil || primary.ID != third.ID {
		t.Fatalf("new work primary = %+v/%v, want the new open row %q", primary, err, third.ID)
	}
	if got, _ := store.LatestClosed(id.Key()); got.Status != StatusClosed {
		t.Fatalf("LatestClosed returned a non-closed row: %+v", got)
	}
}

// TestReusedBranchOpensNewItemD17 pins D17's positive half: new work after a
// close opens a NEW primary item, and the old closed row is left closed.
func TestReusedBranchOpensNewItemD17(t *testing.T) {
	old := ident("old-work")
	newWork := ident("new-work")

	var store Store
	closed := store.OpenItem(old, "trello-mcp-workflow", t0)
	if err := store.CloseItem(closed.ID, Decision{At: "2026-09-13T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	// Same branch, new change identity: a brand-new open item.
	reopened := store.OpenItem(newWork, "trello-mcp-workflow", t0.Add(time.Hour))
	if reopened.ID == closed.ID {
		t.Fatalf("reused branch must open a new item, reused id %q", reopened.ID)
	}
	primary, err := store.Primary(newWork.Key())
	if err != nil {
		t.Fatalf("new work has no primary: %v", err)
	}
	if primary.ID != reopened.ID {
		t.Fatalf("primary = %q, want the new item %q", primary.ID, reopened.ID)
	}
	if len(store.Items) != 2 || store.Items[0].Status != StatusClosed {
		t.Fatalf("closed row must remain: %+v", store.Items)
	}

	// Same branch and same change identity is also a new row, not a reopen.
	same := store.OpenItem(old, "trello-mcp-workflow", t0.Add(2*time.Hour))
	if same.ID == closed.ID || store.Items[0].Status != StatusClosed {
		t.Fatalf("closed item was reopened: %+v", store.Items)
	}
	if _, err := store.Primary(old.Key()); err != nil {
		t.Fatalf("the fresh same-key row must be primary: %v", err)
	}
}

// TestTwoOpenItemsAreConflictNotPick pins "two open rows are a conflict, never a
// silent pick": Primary reports the collision and returns no item.
func TestTwoOpenItemsAreConflictNotPick(t *testing.T) {
	id := ident("tracker-ledger-foundation")
	var store Store
	first := store.OpenItem(id, "trello-mcp-workflow", t0)
	second := store.OpenItem(id, "trello-mcp-workflow", t0.Add(time.Minute))

	got, err := store.Primary(id.Key())
	if !errors.Is(err, ErrMultipleOpen) {
		t.Fatalf("error = %v, want ErrMultipleOpen", err)
	}
	if got.ID != "" {
		t.Fatalf("a conflicted identity must not pick an item, got %q", got.ID)
	}
	if first.ID == second.ID {
		t.Fatal("two opens must produce two distinct rows")
	}
}

// TestItemCoreFieldsAreProviderNeutral pins A7: the item's core fields are the
// neutral set only, provider vocabulary stays opaque inside provider, and the
// opaque payload round-trips untouched.
func TestItemCoreFieldsAreProviderNeutral(t *testing.T) {
	provider := json.RawMessage(`{"board_id":"b","default_list":"Doing","labels":["red"],"card_id":"c"}`)
	item := Item{
		ID:         "0123456789abcdef",
		Identity:   ident("tracker-ledger-foundation"),
		Status:     StatusOpen,
		ItemID:     "card-1",
		ProviderID: "trello-mcp-workflow",
		NativeType: "card",
		URL:        "https://example.invalid/c/1",
		State:      "open",
		Provider:   provider,
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	want := []string{"id", "identity", "status", "item_id", "provider_id", "native_type", "url", "state", "provider", "exemption", "conflict", "decisions"}
	if len(keys) != len(want) {
		t.Fatalf("core field count = %d (%v), want %d", len(keys), raw, len(want))
	}
	for _, name := range want {
		if _, ok := keys[name]; !ok {
			t.Fatalf("core field %q missing from %s", name, raw)
		}
	}
	for _, banned := range []string{"board_id", "default_list", "epic_list", "labels", "card_id", "list"} {
		if _, ok := keys[banned]; ok {
			t.Fatalf("provider field %q was promoted into ledger core: %s", banned, raw)
		}
	}

	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	if err := SaveStore(path, Store{Items: []Item{item}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var got, wantProvider map[string]any
	if err := json.Unmarshal(loaded.Items[0].Provider, &got); err != nil {
		t.Fatalf("opaque provider did not round-trip: %v", err)
	}
	if err := json.Unmarshal(provider, &wantProvider); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantProvider) {
		t.Fatalf("opaque provider changed: got %v want %v", got, wantProvider)
	}
}

// TestDecisionsAreAppendOnly pins A6: decisions append in order, earlier entries
// are untouched, and an unknown kind is rejected without mutating the item.
func TestDecisionsAreAppendOnly(t *testing.T) {
	var store Store
	item := store.OpenItem(ident("slug"), "trello-mcp-workflow", t0)
	before := append([]Decision(nil), item.Decisions...)
	if len(before) != 1 || before[0].Kind != DecisionOpen {
		t.Fatalf("open decision missing: %+v", before)
	}

	kinds := []string{DecisionAdjudicate, DecisionOptOut, DecisionLink, DecisionClose}
	for i, kind := range kinds {
		d := Decision{At: fmt.Sprintf("2026-09-13T0%d:00:00Z", i+1), Checkpoint: "pr-review", Kind: kind}
		if err := store.AppendDecision(item.ID, d); err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
	}
	got := store.Items[0].Decisions
	if len(got) != len(before)+len(kinds) {
		t.Fatalf("decisions = %d, want %d", len(got), len(before)+len(kinds))
	}
	for i := range before {
		if got[i] != before[i] {
			t.Fatalf("earlier decision changed at %d: %+v vs %+v", i, got[i], before[i])
		}
	}
	for i, kind := range kinds {
		if got[len(before)+i].Kind != kind {
			t.Fatalf("decision order: got %q at %d, want %q", got[len(before)+i].Kind, len(before)+i, kind)
		}
	}

	if err := store.AppendDecision(item.ID, Decision{At: "2026-09-13T05:00:00Z", Kind: "invented"}); !errors.Is(err, ErrUnknownDecisionKind) {
		t.Fatalf("unknown kind error = %v, want ErrUnknownDecisionKind", err)
	}
	if len(store.Items[0].Decisions) != len(before)+len(kinds) {
		t.Fatal("a rejected decision must not mutate the item")
	}
	if err := store.AppendDecision("missing-id", Decision{At: "x", Kind: DecisionLink}); err == nil {
		t.Fatal("appending to an unknown item must error")
	}
}

// TestOptOutIsCheckpointScopedD19 pins D19: an opt-out covers only the
// checkpoint that was answered; the next checkpoint is not covered.
func TestOptOutIsCheckpointScopedD19(t *testing.T) {
	id := ident("slug")
	var store Store
	item := store.OpenItem(id, "trello-mcp-workflow", t0)

	if store.HasOptOut(id.Key(), "apply-start") {
		t.Fatal("no opt-out recorded yet, want false")
	}
	if err := store.AppendDecision(item.ID, Decision{At: "2026-09-13T01:00:00Z", Checkpoint: "apply-start", Kind: DecisionOptOut, Choice: "continue"}); err != nil {
		t.Fatal(err)
	}
	if !store.HasOptOut(id.Key(), "apply-start") {
		t.Fatal("apply-start opt-out not detected")
	}
	if store.HasOptOut(id.Key(), "pre-merge") {
		t.Fatal("opt-out must not carry to pre-merge (D19)")
	}

	if err := store.AppendDecision(item.ID, Decision{At: "2026-09-13T02:00:00Z", Checkpoint: "pre-merge", Kind: DecisionOptOut, Choice: "continue"}); err != nil {
		t.Fatal(err)
	}
	if !store.HasOptOut(id.Key(), "pre-merge") {
		t.Fatal("pre-merge opt-out not detected after being recorded")
	}

	// A non-opt-out decision at another checkpoint is not an opt-out.
	if err := store.AppendDecision(item.ID, Decision{At: "2026-09-13T03:00:00Z", Checkpoint: "pr-review", Kind: DecisionAdjudicate, Choice: "local"}); err != nil {
		t.Fatal(err)
	}
	if store.HasOptOut(id.Key(), "pr-review") {
		t.Fatal("an adjudicate decision must not read as an opt-out")
	}

	// A deleted identity has no opt-out (no primary).
	if store.HasOptOut(ident("other").Key(), "apply-start") {
		t.Fatal("an unrelated identity must not inherit an opt-out")
	}
}

// TestAdvisoryCeilingDoesNotCompact pins A5's advisory ceiling: it is a warning
// threshold (32 items or 64 KiB), and saving never drops rows.
func TestAdvisoryCeilingDoesNotCompact(t *testing.T) {
	if MaxItems != 32 {
		t.Fatalf("MaxItems = %d, want 32", MaxItems)
	}
	if MaxStoreBytes != 64*1024 {
		t.Fatalf("MaxStoreBytes = %d, want 65536", MaxStoreBytes)
	}

	var atCeiling Store
	for i := 0; i < MaxItems; i++ {
		atCeiling.OpenItem(ItemIdentity{CommonDir: "/repo/.git", Branch: fmt.Sprintf("branch-%d", i)}, "p", t0)
	}
	if atCeiling.OverCeiling() {
		t.Fatalf("exactly %d items is the ceiling, not over it", MaxItems)
	}
	atCeiling.OpenItem(ItemIdentity{CommonDir: "/repo/.git", Branch: "one-more"}, "p", t0)
	if !atCeiling.OverCeiling() {
		t.Fatalf("%d items must be over the advisory ceiling", MaxItems+1)
	}

	// The byte branch: a single oversized item is over the ceiling.
	big := Store{Items: []Item{{ID: "big", Identity: ident("slug"), Status: StatusOpen, URL: strings.Repeat("u", MaxStoreBytes)}}}
	if !big.OverCeiling() {
		t.Fatal("an oversized store must be over the advisory ceiling")
	}
	if (Store{Items: []Item{{ID: "small"}}}).OverCeiling() {
		t.Fatal("a tiny store must not be over the ceiling")
	}

	// No compaction: 40 saved rows load back as 40 rows.
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	var store Store
	for i := 0; i < MaxItems+8; i++ {
		store.OpenItem(ItemIdentity{CommonDir: "/repo/.git", Branch: fmt.Sprintf("branch-%d", i)}, "p", t0)
	}
	if len(store.Items) != MaxItems+8 {
		t.Fatalf("prepared items = %d", len(store.Items))
	}
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Items) != MaxItems+8 {
		t.Fatalf("saved rows = %d, want %d (no compaction)", len(loaded.Items), MaxItems+8)
	}
	if !loaded.OverCeiling() {
		t.Fatal("loaded over-ceiling store must report over the ceiling")
	}
}

// TestAppendDecisionToPrimaryFailsClosed pins the locked append's fail-closed
// behavior: a missing or collided primary (and an unknown decision kind) writes
// nothing to the store.
func TestAppendDecisionToPrimaryFailsClosed(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	id := ident("slug")
	d := Decision{At: "2026-09-13T01:00:00Z", Checkpoint: "apply-start", Kind: DecisionOptOut, Choice: "continue"}

	// No primary: a missing store is not created by a failed persist.
	if err := AppendDecisionToPrimary(path, id.Key(), d); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("missing primary error = %v, want ErrNoPrimary", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a failed persist must not create a store (stat err = %v)", err)
	}

	// Two open rows: a collision is not a pick, and the store is untouched.
	var collided Store
	collided.OpenItem(id, "p", t0)
	collided.OpenItem(id, "p", t0)
	if err := SaveStore(path, collided); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendDecisionToPrimary(path, id.Key(), d); !errors.Is(err, ErrMultipleOpen) {
		t.Fatalf("collision error = %v, want ErrMultipleOpen", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a failed persist must not modify the store")
	}

	// An unknown kind is rejected before any write.
	var single Store
	single.OpenItem(id, "p", t0)
	if err := SaveStore(path, single); err != nil {
		t.Fatal(err)
	}
	if err := AppendDecisionToPrimary(path, id.Key(), Decision{At: "x", Kind: "invented"}); !errors.Is(err, ErrUnknownDecisionKind) {
		t.Fatalf("unknown kind error = %v, want ErrUnknownDecisionKind", err)
	}
}

// TestConcurrentAppendDoesNotLoseDecisions pins the atomic replace + lock
// contract: two concurrent appenders both land their decision, and the file is
// always a complete, parseable store with no temp residue.
func TestConcurrentAppendDoesNotLoseDecisions(t *testing.T) {
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	id := ident("slug")

	var store Store
	store.OpenItem(id, "trello-mcp-workflow", t0)
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	checkpoints := []string{"apply-start", "pre-merge"}
	var wg sync.WaitGroup
	for i, checkpoint := range checkpoints {
		wg.Add(1)
		go func(i int, checkpoint string) {
			defer wg.Done()
			d := Decision{
				At:         t0.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
				Checkpoint: checkpoint,
				Kind:       DecisionOptOut,
				Choice:     "continue",
			}
			if err := AppendDecisionToPrimary(path, id.Key(), d); err != nil {
				t.Errorf("append %s: %v", checkpoint, err)
			}
		}(i, checkpoint)
	}
	wg.Wait()

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatalf("store must stay parseable under concurrency: %v", err)
	}
	primary, err := loaded.Primary(id.Key())
	if err != nil {
		t.Fatalf("primary after concurrent appends: %v", err)
	}
	// One open decision + two concurrent opt-outs, none lost.
	if len(primary.Decisions) != len(checkpoints)+1 {
		t.Fatalf("decisions = %d, want %d: %+v", len(primary.Decisions), len(checkpoints)+1, primary.Decisions)
	}
	for _, checkpoint := range checkpoints {
		if !loaded.HasOptOut(id.Key(), checkpoint) {
			t.Fatalf("lost decision for checkpoint %s: %+v", checkpoint, primary.Decisions)
		}
	}

	residue, err := filepath.Glob(filepath.Join(filepath.Dir(path), "state.json.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(residue) != 0 {
		t.Fatalf("temp residue after concurrent appends: %v", residue)
	}
}
