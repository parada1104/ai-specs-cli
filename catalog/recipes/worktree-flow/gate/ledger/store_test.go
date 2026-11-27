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

// fixture is the pair of ledgers a test seeds the way the pre-split store was
// seeded before the split: one open row per ledger, a close that only the
// worktree ledger records, and the two read back together as the composed View.
type fixture struct {
	Tracker  Store
	Worktree WorktreeStore
}

func newFixture() *fixture { return &fixture{} }

// openItem appends one open row to each ledger, exactly as ApplyWrite's open
// verb does: identity and status in the worktree ledger, the provider record and
// the open decision in the tracker.
func (f *fixture) openItem(id ItemIdentity, providerID string, at time.Time) Item {
	stamp := at.UTC().Format(time.RFC3339)
	rowID := f.view().uniqueItemID(id, stamp)
	f.Worktree.Items = append(f.Worktree.Items,
		WorktreeItem{ID: rowID, Identity: id, Status: StatusOpen, Decisions: []WorktreeDecision{}})
	item := Item{ID: rowID, Identity: id, ProviderID: providerID,
		Decisions: []Decision{{At: stamp, Kind: DecisionOpen}}}
	f.Tracker.Items = append(f.Tracker.Items, item)
	return item
}

// closeItem closes the row in the worktree ledger: that is where the lifecycle
// lives, so the close decision never reaches the tracker row.
func (f *fixture) closeItem(id string, d Decision) error {
	at := d.At
	if at == "" {
		at = t0.UTC().Format(time.RFC3339)
	}
	return f.Worktree.closeItem(id, at, d.Checkpoint)
}

// view composes the two ledgers into the read-only contract.
func (f *fixture) view() View { return View{Worktree: f.Worktree, Tracker: f.Tracker} }

// write stores both ledgers at the explicit locations a test names: the tracker
// file path and its worktree sibling.
func (f *fixture) write(t *testing.T, path string) Ledgers {
	t.Helper()
	locs := LedgersAtStore(path)
	if _, err := locs.Worktree.SaveWorktree(f.Worktree); err != nil {
		t.Fatalf("save worktree ledger: %v", err)
	}
	if _, err := locs.Tracker.Save(f.Tracker); err != nil {
		t.Fatalf("save tracker ledger: %v", err)
	}
	return locs
}

// ledgerPath is a fresh tracker ledger path under a temp git common dir; its
// worktree sibling lives beside it.
func ledgerPath(t *testing.T) string {
	t.Helper()
	return TrackerStorePath(filepath.Join(t.TempDir(), ".git"))
}

// loadView reads a ledger pair back as the composed View.
func loadView(t *testing.T, locs Ledgers) View {
	t.Helper()
	view, _, err := locs.Load()
	if err != nil {
		t.Fatalf("load ledgers: %v", err)
	}
	return view
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

	f := newFixture()
	f.openItem(ident("tracker-ledger-foundation"), "trello-mcp-workflow", t0)
	if err := SaveStore(path, f.Tracker); err != nil {
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

	f := newFixture()
	item := f.openItem(id, "trello-mcp-workflow", t0)
	if item.Key() != id.Key() {
		t.Fatalf("item key = %q, want %q", item.Key(), id.Key())
	}

	if _, err := f.view().Primary(id.Key()); err != nil {
		t.Fatalf("full key must select the item: %v", err)
	}
	for _, miss := range []string{
		IdentityKey("/repo/.git", "change/tracker-ledger-foundation", ""),
		IdentityKey("/repo/.git", "change/tracker-ledger-foundation", "other-change"),
		IdentityKey("/repo/.git", "another-branch", "tracker-ledger-foundation"),
		IdentityKey("/other/.git", "change/tracker-ledger-foundation", "tracker-ledger-foundation"),
	} {
		if _, err := f.view().Primary(miss); !errors.Is(err, ErrNoPrimary) {
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
	f := newFixture()
	item := f.openItem(id, "trello-mcp-workflow", t0)
	if err := f.closeItem(item.ID, Decision{At: "2026-09-13T01:00:00Z", Checkpoint: "archive-close", Note: "merged"}); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := f.view().Primary(id.Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("closed item must not be primary (err = %v)", err)
	}
	if f.Worktree.Items[0].Status != StatusClosed {
		t.Fatalf("status = %q, want %q", f.Worktree.Items[0].Status, StatusClosed)
	}
	decisions := f.Worktree.Items[0].Decisions
	if len(decisions) == 0 || decisions[len(decisions)-1].Kind != DecisionClose {
		t.Fatalf("close decision missing: %+v", decisions)
	}
	if err := f.closeItem("does-not-exist", Decision{At: "x"}); err == nil {
		t.Fatal("closing an unknown item must error")
	}
}

// TestLatestClosedReportsLatestClosedRowWithoutPrimary pins the read-only report
// selector: LatestClosed returns the most recently stored closed row for a key,
// and that row is never primary, so new work on a reused branch still selects the
// new open row (D17).
func TestLatestClosedReportsLatestClosedRowWithoutPrimary(t *testing.T) {
	id := ident("old-work")
	f := newFixture()
	if _, ok := f.view().LatestClosed(id.Key()); ok {
		t.Fatal("an empty store has no closed row")
	}
	first := f.openItem(id, "trello-mcp-workflow", t0)
	if err := f.closeItem(first.ID, Decision{At: "2026-09-13T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	got, ok := f.view().LatestClosed(id.Key())
	if !ok || got.ID != first.ID || f.Worktree.Items[0].Status != StatusClosed {
		t.Fatalf("LatestClosed = %+v/%v, want the closed row %q", got, ok, first.ID)
	}
	if _, err := f.view().Primary(id.Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("a closed row must never be primary (err = %v)", err)
	}

	// A newer close for the same key is the row reported.
	second := f.openItem(id, "trello-mcp-workflow", t0.Add(time.Minute))
	if err := f.closeItem(second.ID, Decision{At: "2026-09-13T02:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.view().LatestClosed(id.Key()); got.ID != second.ID {
		t.Fatalf("LatestClosed = %q, want the newer closed row %q", got.ID, second.ID)
	}

	// New work still opens its own primary and LatestClosed never returns it.
	third := f.openItem(id, "trello-mcp-workflow", t0.Add(2*time.Minute))
	primary, err := f.view().Primary(id.Key())
	if err != nil || primary.ID != third.ID {
		t.Fatalf("new work primary = %+v/%v, want the new open row %q", primary, err, third.ID)
	}
	if got, _ := f.view().LatestClosed(id.Key()); got.ID == third.ID {
		t.Fatalf("LatestClosed returned the open row: %+v", got)
	}
}

// TestReusedBranchOpensNewItemD17 pins D17's positive half: new work after a
// close opens a NEW primary item, and the old closed row is left closed.
func TestReusedBranchOpensNewItemD17(t *testing.T) {
	old := ident("old-work")
	newWork := ident("new-work")

	f := newFixture()
	closed := f.openItem(old, "trello-mcp-workflow", t0)
	if err := f.closeItem(closed.ID, Decision{At: "2026-09-13T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	// Same branch, new change identity: a brand-new open item.
	reopened := f.openItem(newWork, "trello-mcp-workflow", t0.Add(time.Hour))
	if reopened.ID == closed.ID {
		t.Fatalf("reused branch must open a new item, reused id %q", reopened.ID)
	}
	primary, err := f.view().Primary(newWork.Key())
	if err != nil {
		t.Fatalf("new work has no primary: %v", err)
	}
	if primary.ID != reopened.ID {
		t.Fatalf("primary = %q, want the new item %q", primary.ID, reopened.ID)
	}
	if len(f.Tracker.Items) != 2 || f.Worktree.Items[0].Status != StatusClosed {
		t.Fatalf("closed row must remain: %+v", f.Tracker.Items)
	}

	// Same branch and same change identity is also a new row, not a reopen.
	same := f.openItem(old, "trello-mcp-workflow", t0.Add(2*time.Hour))
	if same.ID == closed.ID || f.Worktree.Items[0].Status != StatusClosed {
		t.Fatalf("closed item was reopened: %+v", f.Tracker.Items)
	}
	if _, err := f.view().Primary(old.Key()); err != nil {
		t.Fatalf("the fresh same-key row must be primary: %v", err)
	}
}

// TestSameSecondOpenItemIDsStayUnique pins the persisted-row identity guard:
// a close followed by a new open in the same second must not reuse the ID.
func TestSameSecondOpenItemIDsStayUnique(t *testing.T) {
	id := ident("same-second")
	f := newFixture()
	closed := f.openItem(id, "trello-mcp-workflow", vtNow)
	if err := f.closeItem(closed.ID, Decision{At: t0.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	fresh := f.openItem(id, "trello-mcp-workflow", vtNow)
	if fresh.ID == closed.ID {
		t.Fatalf("same-second open reused ID %q", fresh.ID)
	}
	if len(f.Tracker.Items) != 2 || f.Worktree.Items[0].Status != StatusClosed || f.Worktree.Items[1].Status != StatusOpen {
		t.Fatalf("store rows = %+v, want closed then open", f.Tracker.Items)
	}
}

// TestTwoOpenItemsAreConflictNotPick pins "two open rows are a conflict, never a
// silent pick": Primary reports the collision and returns no item.
func TestTwoOpenItemsAreConflictNotPick(t *testing.T) {
	id := ident("tracker-ledger-foundation")
	f := newFixture()
	first := f.openItem(id, "trello-mcp-workflow", t0)
	second := f.openItem(id, "trello-mcp-workflow", t0.Add(time.Minute))

	got, err := f.view().Primary(id.Key())
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
	want := []string{"id", "identity", "item_id", "provider_id", "native_type", "url", "state", "provider", "exemption", "conflict", "decisions"}
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
	f := newFixture()
	item := f.openItem(ident("slug"), "trello-mcp-workflow", t0)
	before := append([]Decision(nil), item.Decisions...)
	if len(before) != 1 || before[0].Kind != DecisionOpen {
		t.Fatalf("open decision missing: %+v", before)
	}

	// The tracker's append-only set: a close is the worktree ledger's decision
	// and is covered by the close tests above, never appended here (S3).
	kinds := []string{DecisionAdjudicate, DecisionOptOut, DecisionLink}
	for i, kind := range kinds {
		d := Decision{At: fmt.Sprintf("2026-09-13T0%d:00:00Z", i+1), Checkpoint: "pr-review", Kind: kind}
		if err := f.Tracker.AppendDecision(item.ID, d); err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
	}
	got := f.Tracker.Items[0].Decisions
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

	if err := f.Tracker.AppendDecision(item.ID, Decision{At: "2026-09-13T05:00:00Z", Kind: "invented"}); !errors.Is(err, ErrUnknownDecisionKind) {
		t.Fatalf("unknown kind error = %v, want ErrUnknownDecisionKind", err)
	}
	if len(f.Tracker.Items[0].Decisions) != len(before)+len(kinds) {
		t.Fatal("a rejected decision must not mutate the item")
	}
	if err := f.Tracker.AppendDecision("missing-id", Decision{At: "x", Kind: DecisionLink}); err == nil {
		t.Fatal("appending to an unknown item must error")
	}
}

// TestOptOutIsCheckpointScopedD19 pins D19: an opt-out covers only the
// checkpoint that was answered; the next checkpoint is not covered.
func TestOptOutIsCheckpointScopedD19(t *testing.T) {
	id := ident("slug")
	f := newFixture()
	item := f.openItem(id, "trello-mcp-workflow", t0)

	if f.view().HasOptOut(id.Key(), "apply-start") {
		t.Fatal("no opt-out recorded yet, want false")
	}
	if err := f.Tracker.AppendDecision(item.ID, Decision{At: "2026-09-13T01:00:00Z", Checkpoint: "apply-start", Kind: DecisionOptOut, Choice: "continue"}); err != nil {
		t.Fatal(err)
	}
	if !f.view().HasOptOut(id.Key(), "apply-start") {
		t.Fatal("apply-start opt-out not detected")
	}
	if f.view().HasOptOut(id.Key(), "pre-merge") {
		t.Fatal("opt-out must not carry to pre-merge (D19)")
	}

	if err := f.Tracker.AppendDecision(item.ID, Decision{At: "2026-09-13T02:00:00Z", Checkpoint: "pre-merge", Kind: DecisionOptOut, Choice: "continue"}); err != nil {
		t.Fatal(err)
	}
	if !f.view().HasOptOut(id.Key(), "pre-merge") {
		t.Fatal("pre-merge opt-out not detected after being recorded")
	}

	// A non-opt-out decision at another checkpoint is not an opt-out.
	if err := f.Tracker.AppendDecision(item.ID, Decision{At: "2026-09-13T03:00:00Z", Checkpoint: "pr-review", Kind: DecisionAdjudicate, Choice: "local"}); err != nil {
		t.Fatal(err)
	}
	if f.view().HasOptOut(id.Key(), "pr-review") {
		t.Fatal("an adjudicate decision must not read as an opt-out")
	}

	// A deleted identity has no opt-out (no primary).
	if f.view().HasOptOut(ident("other").Key(), "apply-start") {
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

	atCeiling := newFixture()
	for i := 0; i < MaxItems; i++ {
		atCeiling.openItem(ItemIdentity{CommonDir: "/repo/.git", Branch: fmt.Sprintf("branch-%d", i)}, "p", t0)
	}
	if atCeiling.Tracker.OverCeiling() {
		t.Fatalf("exactly %d items is the ceiling, not over it", MaxItems)
	}
	atCeiling.openItem(ItemIdentity{CommonDir: "/repo/.git", Branch: "one-more"}, "p", t0)
	if !atCeiling.Tracker.OverCeiling() {
		t.Fatalf("%d items must be over the advisory ceiling", MaxItems+1)
	}

	// The byte branch: a single oversized item is over the ceiling.
	big := Store{Items: []Item{{ID: "big", Identity: ident("slug"), URL: strings.Repeat("u", MaxStoreBytes)}}}
	if !big.OverCeiling() {
		t.Fatal("an oversized store must be over the advisory ceiling")
	}
	if (Store{Items: []Item{{ID: "small"}}}).OverCeiling() {
		t.Fatal("a tiny store must not be over the ceiling")
	}

	// No compaction: 40 saved rows load back as 40 rows.
	path := StorePath(filepath.Join(t.TempDir(), ".git"))
	f := newFixture()
	for i := 0; i < MaxItems+8; i++ {
		f.openItem(ItemIdentity{CommonDir: "/repo/.git", Branch: fmt.Sprintf("branch-%d", i)}, "p", t0)
	}
	if len(f.Tracker.Items) != MaxItems+8 {
		t.Fatalf("prepared items = %d", len(f.Tracker.Items))
	}
	if err := SaveStore(path, f.Tracker); err != nil {
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
	collided := newFixture()
	collided.openItem(id, "p", t0)
	collided.openItem(id, "p", t0)
	if err := SaveStore(path, collided.Tracker); err != nil {
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
	single := newFixture()
	single.openItem(id, "p", t0)
	if err := SaveStore(path, single.Tracker); err != nil {
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

	f := newFixture()
	f.openItem(id, "trello-mcp-workflow", t0)
	if err := SaveStore(path, f.Tracker); err != nil {
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
	primary, err := singleFileView(loaded).Primary(id.Key())
	if err != nil {
		t.Fatalf("primary after concurrent appends: %v", err)
	}
	// One open decision + two concurrent opt-outs, none lost.
	if len(primary.Decisions) != len(checkpoints)+1 {
		t.Fatalf("decisions = %d, want %d: %+v", len(primary.Decisions), len(checkpoints)+1, primary.Decisions)
	}
	for _, checkpoint := range checkpoints {
		if !singleFileView(loaded).HasOptOut(id.Key(), checkpoint) {
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

// --- Flow-agnostic VCS close (T3) -------------------------------------------

// branchCloseIdent is the cleanup-side identity: common dir + branch, no slug.
func branchCloseIdent() ItemIdentity {
	return ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close"}
}

// TestCloseBranchItemClosesSingleMatchingOpenItemAcrossChangeSlug pins the
// cleanup seam: cleanup knows only the common dir and the branch, but the stored
// item may carry a change slug. The close must still find and close it at the
// injected checkpoint (archive-close).
func TestCloseBranchItemClosesSingleMatchingOpenItemAcrossChangeSlug(t *testing.T) {
	path := writePath(t)
	stored := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "some-change"}
	f := newFixture()
	opened := f.openItem(stored, "test-recipe", t0)
	saveWriteStore(t, path, f)

	out, _, err := CloseBranchItem(LedgersAtStore(path), "/repo/.git", "feat/vcs-close", CheckpointArchiveClose, vtNow)
	if err != nil {
		t.Fatalf("CloseBranchItem: %v", err)
	}
	if !out.Applied || out.Kind != WriteClose {
		t.Fatalf("outcome = %+v, want an applied close", out)
	}

	view := loadWriteView(t, path)
	if len(view.Worktree.Items) != 1 {
		t.Fatalf("rows = %+v, want the single stored row", view.Worktree.Items)
	}
	row := view.Worktree.Items[0]
	if row.ID != opened.ID || row.Status != StatusClosed {
		t.Fatalf("row = %+v, want the matching row closed", row)
	}
	last := row.Decisions[len(row.Decisions)-1]
	if last.Kind != DecisionClose || last.Checkpoint != CheckpointArchiveClose {
		t.Fatalf("last decision = %+v, want close at archive-close", last)
	}
	if _, err := view.Primary(row.Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("closed row must not be primary (err = %v)", err)
	}
}

// TestCloseBranchItemAbsentOrClosedIsNoOp pins the idempotent half: an identity
// with no open row is a successful no-op and the store is left byte-identical.
func TestCloseBranchItemAbsentOrClosedIsNoOp(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		path := writePath(t)
		saveWriteStore(t, path, newFixture())
		before := pairBytes(t, path)

		out, _, err := CloseBranchItem(LedgersAtStore(path), "/repo/.git", "feat/vcs-close", CheckpointArchiveClose, vtNow)
		if err != nil {
			t.Fatalf("absent close: %v", err)
		}
		if out.Applied || out.Kind != WriteClose || out.Reason != WriteReasonAlreadyClosed {
			t.Fatalf("absent close = %+v, want applied=false/already-closed", out)
		}
		if before != pairBytes(t, path) {
			t.Fatal("an absent no-op must not rewrite the store")
		}
	})

	t.Run("already closed", func(t *testing.T) {
		path := writePath(t)
		ident := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "some-change"}
		f := newFixture()
		item := f.openItem(ident, "test-recipe", t0)
		if err := f.closeItem(item.ID, Decision{At: t0.Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		saveWriteStore(t, path, f)
		before := pairBytes(t, path)

		out, _, err := CloseBranchItem(LedgersAtStore(path), "/repo/.git", "feat/vcs-close", CheckpointArchiveClose, vtNow)
		if err != nil {
			t.Fatalf("closed close: %v", err)
		}
		if out.Applied || out.Reason != WriteReasonAlreadyClosed {
			t.Fatalf("closed close = %+v, want applied=false/already-closed", out)
		}
		if before != pairBytes(t, path) {
			t.Fatal("a closed row must not be rewritten or reopened (D17)")
		}
		view := loadWriteView(t, path)
		if view.Worktree.Items[0].Status != StatusClosed {
			t.Fatalf("status = %q, want the closed row preserved", view.Worktree.Items[0].Status)
		}
	})
}

// TestCloseBranchItemMultipleOpenIsCollisionRefused pins the refusal: two open
// rows for one common dir + branch cannot be disambiguated by branch alone, so
// the close fails closed and persists nothing.
func TestCloseBranchItemMultipleOpenIsCollisionRefused(t *testing.T) {
	path := writePath(t)
	f := newFixture()
	f.openItem(ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "one"}, "test-recipe", t0)
	f.openItem(ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "two"}, "test-recipe", t0)
	saveWriteStore(t, path, f)
	before := pairBytes(t, path)

	_, _, err := CloseBranchItem(LedgersAtStore(path), "/repo/.git", "feat/vcs-close", CheckpointArchiveClose, vtNow)
	if !errors.Is(err, ErrMultipleOpen) {
		t.Fatalf("error = %v, want ErrMultipleOpen", err)
	}
	if before != pairBytes(t, path) {
		t.Fatal("a refused collision must persist nothing")
	}
}

// TestCloseBranchItemTargetsOnlyOpenRowOnReusedBranch triangulates D17: a
// reused branch can hold a closed old row and a new open row; the close selects
// the open one and leaves the closed one closed.
func TestCloseBranchItemTargetsOnlyOpenRowOnReusedBranch(t *testing.T) {
	path := writePath(t)
	oldIdent := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "old"}
	newIdent := ItemIdentity{CommonDir: "/repo/.git", Branch: "feat/vcs-close", Change: "new"}
	f := newFixture()
	old := f.openItem(oldIdent, "test-recipe", t0)
	if err := f.closeItem(old.ID, Decision{At: t0.Format(time.RFC3339), Checkpoint: CheckpointArchiveClose}); err != nil {
		t.Fatal(err)
	}
	fresh := f.openItem(newIdent, "test-recipe", t0.Add(time.Hour))
	saveWriteStore(t, path, f)

	out, _, err := CloseBranchItem(LedgersAtStore(path), "/repo/.git", "feat/vcs-close", CheckpointArchiveClose, vtNow)
	if err != nil {
		t.Fatalf("CloseBranchItem: %v", err)
	}
	if !out.Applied {
		t.Fatalf("outcome = %+v, want the new open row closed", out)
	}
	view := loadWriteView(t, path)
	if len(view.Worktree.Items) != 2 {
		t.Fatalf("rows = %d, want the old closed + new closed row", len(view.Worktree.Items))
	}
	for _, row := range view.Worktree.Items {
		if row.Status != StatusClosed {
			t.Fatalf("row %s status = %q, want closed", row.ID, row.Status)
		}
	}
	// The fresh row was the one closed now; the old row keeps its original close.
	if last := view.Worktree.Items[1].Decisions[len(view.Worktree.Items[1].Decisions)-1]; view.Worktree.Items[1].ID != fresh.ID || last.Checkpoint != CheckpointArchiveClose {
		t.Fatalf("fresh row = %+v, want it closed at archive-close", view.Worktree.Items[1])
	}
}

// TestCloseBranchItemRejectsMissingIdentityOrCheckpoint pins the fail-closed
// input validation: cleanup must never guess a key or accept a fake checkpoint.
func TestCloseBranchItemRejectsMissingIdentityOrCheckpoint(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, newFixture())
	before := pairBytes(t, path)

	cases := []struct {
		name    string
		common  string
		branch  string
		checkpt string
	}{
		{"missing common dir", "", "feat/vcs-close", CheckpointArchiveClose},
		{"missing branch", "/repo/.git", "", CheckpointArchiveClose},
		{"unknown checkpoint", "/repo/.git", "feat/vcs-close", "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := CloseBranchItem(LedgersAtStore(path), tc.common, tc.branch, tc.checkpt, vtNow); err == nil {
				t.Fatal("invalid close input was accepted")
			}
			if before != pairBytes(t, path) {
				t.Fatal("a rejected close must persist nothing")
			}
		})
	}
}
