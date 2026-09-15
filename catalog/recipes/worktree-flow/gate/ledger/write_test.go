package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writePath is a fresh store path under a temp git common dir.
func writePath(t *testing.T) string {
	t.Helper()
	return StorePath(filepath.Join(t.TempDir(), ".git"))
}

// writeRepos writes a store and returns its path.
func saveWriteStore(t *testing.T, path string, store Store) {
	t.Helper()
	if err := SaveStore(path, store); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
}

// applyWrite is the terse call helper: one write against the canonical identity.
func applyWrite(t *testing.T, path, providerID string, req WriteRequest) (WriteOutcome, error) {
	t.Helper()
	return ApplyWrite(path, ApplyWriteRequest{
		Identity:   ident(""),
		Checkpoint: CheckpointApplyStart,
		ProviderID: providerID,
		Request:    req,
	}, vtNow)
}

// loadWriteStore loads a store and fails the test on error.
func loadWriteStore(t *testing.T, path string) Store {
	t.Helper()
	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore(%s): %v", path, err)
	}
	return store
}

// rawBytes reads a file and fails the test on error.
func rawBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestWriteKindEnumPinned pins the closed machine-write verb set (DW2).
func TestWriteKindEnumPinned(t *testing.T) {
	want := []string{WriteOpen, WriteLink, WriteClose, WriteExempt}
	if len(WriteKinds) != len(want) {
		t.Fatalf("WriteKinds = %v, want %v", WriteKinds, want)
	}
	for i, kind := range want {
		if WriteKinds[i] != kind {
			t.Fatalf("WriteKinds[%d] = %q, want %q", i, WriteKinds[i], kind)
		}
	}
	// No sixth kind leaks into the closed append-only decision set (DW4).
	if decisionKinds[WriteExempt] {
		t.Fatal("exempt must not be a decisionKinds entry (DW4)")
	}
	for _, kind := range want {
		if !decisionKinds[kind] && kind != WriteExempt {
			t.Fatalf("%q has no decision kind mapping", kind)
		}
	}
}

// TestWriteRequestValidate pins the payload rules: kind enum, link needs item_id,
// exempt needs a trimmed reason, checkpoint must be real, and a bad payload fails
// closed after flag parsing (L6).
func TestWriteRequestValidate(t *testing.T) {
	ok := []struct {
		name string
		req  WriteRequest
		cp   string
	}{
		{"open", WriteRequest{Kind: WriteOpen}, CheckpointApplyStart},
		{"link with item id", WriteRequest{Kind: WriteLink, ItemID: "card-1"}, CheckpointPRReview},
		{"close", WriteRequest{Kind: WriteClose}, CheckpointPreMerge},
		{"exempt with reason", WriteRequest{Kind: WriteExempt, Reason: "no tracker for this change"}, CheckpointArchiveClose},
	}
	for _, tc := range ok {
		if err := tc.req.Normalize().Validate(tc.cp); err != nil {
			t.Fatalf("%s: Validate = %v, want nil", tc.name, err)
		}
	}

	bad := []struct {
		name string
		req  WriteRequest
		cp   string
	}{
		{"empty kind", WriteRequest{}, CheckpointApplyStart},
		{"unknown kind", WriteRequest{Kind: "unexempt"}, CheckpointApplyStart},
		{"uppercase kind", WriteRequest{Kind: "OPEN"}, CheckpointApplyStart},
		{"link without item id", WriteRequest{Kind: WriteLink}, CheckpointApplyStart},
		{"exempt with blank reason", WriteRequest{Kind: WriteExempt, Reason: "   "}, CheckpointApplyStart},
		{"unknown checkpoint", WriteRequest{Kind: WriteOpen}, "bogus"},
		{"missing checkpoint", WriteRequest{Kind: WriteOpen}, ""},
	}
	for _, tc := range bad {
		if err := tc.req.Normalize().Validate(tc.cp); err == nil {
			t.Fatalf("%s: Validate succeeded, want a closed failure", tc.name)
		}
	}
}

// TestWriteRequestNormalizeDefaultsProvider pins the payload defaults: provider is
// an opaque object defaulting to {}, and the scalar fields are trimmed.
func TestWriteRequestNormalizeDefaultsProvider(t *testing.T) {
	got := WriteRequest{Kind: "  link  ", ItemID: " card-1 ", URL: " u ", NativeType: " card ",
		State: " open ", Reason: " why ", Change: " slug "}.Normalize()
	if got.Kind != WriteLink || got.ItemID != "card-1" || got.URL != "u" ||
		got.NativeType != "card" || got.State != "open" || got.Reason != "why" || got.Change != "slug" {
		t.Fatalf("Normalize trimmed nothing: %+v", got)
	}
	if string(canonicalProvider(got.Provider)) != "{}" {
		t.Fatalf("provider default = %q, want {}", got.Provider)
	}
}

// TestApplyWriteOpenIsOpenIfAbsent pins the retried-open invariant: the second
// open reports already-open and the store still holds exactly one open item.
func TestApplyWriteOpenIsOpenIfAbsent(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})

	first, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{Kind: WriteOpen})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if !first.Applied || first.Kind != WriteOpen || first.Reason != "" {
		t.Fatalf("first open = %+v, want applied open with no reason", first)
	}

	before := rawBytes(t, path)
	second, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{Kind: WriteOpen})
	if err != nil {
		t.Fatalf("retried open: %v", err)
	}
	if second.Applied || second.Reason != WriteReasonAlreadyOpen {
		t.Fatalf("retried open = %+v, want applied=false/already-open", second)
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("an idempotent open must not rewrite the store")
	}

	open := loadWriteStore(t, path).OpenItems(ident("").Key())
	if len(open) != 1 {
		t.Fatalf("open items = %d, want exactly 1", len(open))
	}
	if len(open[0].Decisions) != 1 || open[0].Decisions[0].Kind != DecisionOpen {
		t.Fatalf("decisions = %+v, want a single open decision", open[0].Decisions)
	}
	if open[0].ProviderID != "trello-mcp-workflow" {
		t.Fatalf("provider id = %q, want the bound recipe", open[0].ProviderID)
	}
}

// TestApplyWriteSameSecondIdsStayUnique pins the second-precision guard: an open in
// the same second as an existing row must not reuse that row's id.
func TestApplyWriteSameSecondIdsStayUnique(t *testing.T) {
	path := writePath(t)
	store := Store{V: StoreVersion}
	// A closed row and a fresh open share one second (vtNow) and one identity.
	closed := store.OpenItem(ident(""), "p", vtNow)
	if err := store.CloseItem(closed.ID, Decision{At: t0.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	saveWriteStore(t, path, store)

	out, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteOpen})
	if err != nil {
		t.Fatalf("open after close: %v", err)
	}
	if !out.Applied {
		t.Fatalf("open after close = %+v, want applied", out)
	}

	loaded := loadWriteStore(t, path)
	if len(loaded.Items) != 2 {
		t.Fatalf("items = %d, want the closed row plus the new open row", len(loaded.Items))
	}
	ids := map[string]bool{}
	for _, item := range loaded.Items {
		if item.ID == "" || ids[item.ID] {
			t.Fatalf("duplicate or empty item id: %+v", loaded.Items)
		}
		ids[item.ID] = true
	}
	if _, err := loaded.Primary(ident("").Key()); err != nil {
		t.Fatalf("the new row must be the single primary: %v", err)
	}
	if loaded.Items[0].Status != StatusClosed {
		t.Fatalf("the closed row was reopened: %+v", loaded.Items[0])
	}
}

// TestApplyWriteLinkIsIdempotentForIdenticalState pins DW4 for link: identical core
// fields plus canonically-equal provider bytes append no duplicate decision.
func TestApplyWriteLinkIsIdempotentForIdenticalState(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, storeWithOpenItem("card-1"))

	link := WriteRequest{
		Kind: WriteLink, ItemID: "card-1", URL: "https://example.invalid/c/1",
		NativeType: "card", State: "open",
		Provider: json.RawMessage(`{"board_id":"b","card_id":"card-1"}`),
	}
	first, err := applyWrite(t, path, "trello-mcp-workflow", link)
	if err != nil {
		t.Fatalf("first link: %v", err)
	}
	if !first.Applied {
		t.Fatalf("first link = %+v, want applied", first)
	}

	// Same values, different JSON formatting and key order: canonically identical.
	same := link
	same.Provider = json.RawMessage("{\n  \"card_id\": \"card-1\",\n  \"board_id\": \"b\"\n}")
	before := rawBytes(t, path)
	second, err := applyWrite(t, path, "trello-mcp-workflow", same)
	if err != nil {
		t.Fatalf("second link: %v", err)
	}
	if second.Applied || second.Reason != WriteReasonUnchanged {
		t.Fatalf("repeated link = %+v, want applied=false/unchanged", second)
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("an unchanged link must not rewrite the store")
	}

	item, err := loadWriteStore(t, path).Primary(ident("").Key())
	if err != nil {
		t.Fatal(err)
	}
	links := 0
	for _, d := range item.Decisions {
		if d.Kind == DecisionLink {
			links++
		}
	}
	if links != 1 {
		t.Fatalf("link decisions = %d, want exactly 1 (%+v)", links, item.Decisions)
	}
}

// TestApplyWriteLinkPopulatesCoreFieldsAndAppendsDecision pins the link contract:
// the provider's native fields land on the provider-neutral core, the provider
// payload stays opaque, and a link decision is appended.
func TestApplyWriteLinkPopulatesCoreFieldsAndAppendsDecision(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, storeWithOpenItem(""))

	out, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{
		Kind: WriteLink, ItemID: "card-9", URL: "https://example.invalid/c/9",
		NativeType: "card", State: "in-progress",
		Provider: json.RawMessage(`{"list":"Doing"}`),
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if !out.Applied {
		t.Fatalf("link = %+v, want applied", out)
	}

	item, err := loadWriteStore(t, path).Primary(ident("").Key())
	if err != nil {
		t.Fatal(err)
	}
	if item.ItemID != "card-9" || item.URL != "https://example.invalid/c/9" ||
		item.NativeType != "card" || item.State != "in-progress" {
		t.Fatalf("core fields not populated: %+v", item)
	}
	var provider map[string]string
	if err := json.Unmarshal(item.Provider, &provider); err != nil {
		t.Fatalf("opaque provider did not round-trip: %v (%s)", err, item.Provider)
	}
	if provider["list"] != "Doing" {
		t.Fatalf("provider payload = %v, want list=Doing", provider)
	}
	last := item.Decisions[len(item.Decisions)-1]
	if last.Kind != DecisionLink || last.At != vtNow.UTC().Format(time.RFC3339) {
		t.Fatalf("last decision = %+v, want a link decision at the injected clock", last)
	}
}

// TestApplyWriteCloseThenCloseIsExplicit pins DW4 for close: the second close is
// signalled as already-closed and the item stays in the store, closed.
func TestApplyWriteCloseThenCloseIsExplicit(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, storeWithOpenItem("card-1"))

	first, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteClose})
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	if !first.Applied {
		t.Fatalf("first close = %+v, want applied", first)
	}

	before := rawBytes(t, path)
	second, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteClose})
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if second.Applied || second.Reason != WriteReasonAlreadyClosed {
		t.Fatalf("second close = %+v, want applied=false/already-closed", second)
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("an already-closed item must not be rewritten")
	}

	loaded := loadWriteStore(t, path)
	if len(loaded.Items) != 1 || loaded.Items[0].Status != StatusClosed {
		t.Fatalf("items = %+v, want one closed row that stays in the store", loaded.Items)
	}
	if _, err := loaded.Primary(ident("").Key()); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("a closed row must not be primary (err = %v)", err)
	}
}

// TestApplyWriteCloseWithoutRowFailsClosed pins that closing an identity with no
// row persists nothing.
func TestApplyWriteCloseWithoutRowFailsClosed(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})
	before := rawBytes(t, path)

	if _, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteClose}); !errors.Is(err, ErrNoPrimary) {
		t.Fatalf("close without a row error = %v, want ErrNoPrimary", err)
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("a failed close must leave the store byte-identical")
	}
}

// TestApplyWriteExemptPersistsReasonAndIsIdempotent pins the tracker.none mapping:
// exempt opens if absent, stores the reason on Item.Exemption without a new
// decision kind, reports unchanged for the same text and applies a new text.
func TestApplyWriteExemptPersistsReasonAndIsIdempotent(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})

	first, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{
		Kind: WriteExempt, Reason: "no tracker for this exploit spike",
	})
	if err != nil {
		t.Fatalf("exempt: %v", err)
	}
	if !first.Applied {
		t.Fatalf("exempt = %+v, want applied", first)
	}

	item, err := loadWriteStore(t, path).Primary(ident("").Key())
	if err != nil {
		t.Fatalf("exempt must open-if-absent: %v", err)
	}
	if item.Exemption != "no tracker for this exploit spike" {
		t.Fatalf("exemption = %q, want the persisted reason", item.Exemption)
	}
	for _, d := range item.Decisions {
		if d.Kind == WriteExempt {
			t.Fatalf("exempt added a sixth decision kind: %+v", item.Decisions)
		}
	}

	repeat, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{
		Kind: WriteExempt, Reason: "no tracker for this exploit spike",
	})
	if err != nil {
		t.Fatalf("repeated exempt: %v", err)
	}
	if repeat.Applied || repeat.Reason != WriteReasonUnchanged {
		t.Fatalf("repeated exempt = %+v, want applied=false/unchanged", repeat)
	}

	changed, err := applyWrite(t, path, "trello-mcp-workflow", WriteRequest{
		Kind: WriteExempt, Reason: "tracker now exists; see card",
	})
	if err != nil {
		t.Fatalf("changed exempt: %v", err)
	}
	if !changed.Applied {
		t.Fatalf("changed exempt = %+v, want applied", changed)
	}
	item, err = loadWriteStore(t, path).Primary(ident("").Key())
	if err != nil {
		t.Fatal(err)
	}
	if item.Exemption != "tracker now exists; see card" {
		t.Fatalf("exemption = %q, want the overwritten reason", item.Exemption)
	}
}

// TestApplyWriteChangeAmbiguousRefusesWithoutSlug pins the collision refusal: no
// explicit change slug means nothing is persisted, while an explicit slug is used
// as the stored change.
func TestApplyWriteChangeAmbiguousRefusesWithoutSlug(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})
	before := rawBytes(t, path)

	_, err := ApplyWrite(path, ApplyWriteRequest{
		Identity:   ident(""),
		Checkpoint: CheckpointApplyStart,
		Collision:  CollisionChangeAmbiguous,
		Request:    WriteRequest{Kind: WriteOpen},
	}, vtNow)
	if err == nil {
		t.Fatal("a change-ambiguous write without a slug must be refused")
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("a refused write must leave the store byte-identical")
	}

	if _, err := ApplyWrite(path, ApplyWriteRequest{
		Identity:   ident(""),
		Checkpoint: CheckpointApplyStart,
		Collision:  CollisionChangeAmbiguous,
		Request:    WriteRequest{Kind: WriteOpen, Change: "the-one-change"},
	}, vtNow); err != nil {
		t.Fatalf("explicit slug must be accepted: %v", err)
	}
	stored := loadWriteStore(t, path)
	if len(stored.Items) != 1 || stored.Items[0].Identity.Change != "the-one-change" {
		t.Fatalf("items = %+v, want one item under the explicit slug", stored.Items)
	}
}

// TestApplyWriteRefusesUnavailableIdentity pins that a write cannot invent a
// durable key for an identity Git could not name.
func TestApplyWriteRefusesUnavailableIdentity(t *testing.T) {
	path := writePath(t)
	if _, err := ApplyWrite(path, ApplyWriteRequest{
		Identity:   ItemIdentity{},
		Checkpoint: CheckpointApplyStart,
		Request:    WriteRequest{Kind: WriteOpen},
	}, vtNow); err == nil {
		t.Fatal("an identity-less write must be refused")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a refused write must not create a store (stat err = %v)", err)
	}
}

// TestApplyWriteHeldLockTimesOutFailsClosed pins the bounded lock: a held store
// lock yields ErrLockTimeout in roughly 100 ms and the store is untouched.
func TestApplyWriteHeldLockTimesOutFailsClosed(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, storeWithOpenItem("card-1"))
	before := rawBytes(t, path)

	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("test could not hold the store lock: %v", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	started := time.Now()
	_, err = applyWrite(t, path, "p", WriteRequest{Kind: WriteLink, ItemID: "card-2"})
	elapsed := time.Since(started)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("held-lock error = %v, want ErrLockTimeout", err)
	}
	if elapsed < 20*time.Millisecond || elapsed > time.Second {
		t.Fatalf("lock timeout took %v, want a bounded ~100 ms wait", elapsed)
	}
	if string(before) != string(rawBytes(t, path)) {
		t.Fatal("a lock-timeout write must leave the store byte-identical")
	}
}

// TestApplyWriteFailuresLeaveStoreByteIdentical pins L6 for the whole failure
// family: validation, lock timeout, refused collision and IO each persist nothing.
func TestApplyWriteFailuresLeaveStoreByteIdentical(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	twoOpen := writePath(t)
	two := Store{V: StoreVersion}
	two.OpenItem(ident(""), "p", t0)
	two.OpenItem(ident(""), "p", t0)
	saveWriteStore(t, twoOpen, two)

	cases := []struct {
		name string
		path string
		req  WriteRequest
	}{
		{"invalid payload", writePath(t), WriteRequest{Kind: "invented"}},
		{"link without item id", writePath(t), WriteRequest{Kind: WriteLink}},
		{"exempt without reason", writePath(t), WriteRequest{Kind: WriteExempt}},
		{"close without a row", writePath(t), WriteRequest{Kind: WriteClose}},
		{"link on a collided identity", twoOpen, WriteRequest{Kind: WriteLink, ItemID: "card-1"}},
		{"path under a file", filepath.Join(blocker, "state.json"), WriteRequest{Kind: WriteOpen}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "invalid payload" || tc.name == "link without item id" ||
				tc.name == "exempt without reason" || tc.name == "close without a row" {
				saveWriteStore(t, tc.path, Store{V: StoreVersion})
			}
			before, beforeErr := os.ReadFile(tc.path)
			if _, err := applyWrite(t, tc.path, "p", tc.req); err == nil {
				t.Fatalf("%s: write succeeded, want a closed failure", tc.name)
			}
			after, afterErr := os.ReadFile(tc.path)
			switch {
			case beforeErr != nil || afterErr != nil:
				// An unwritable path (a store under a regular file) must stay just
				// as absent as it already was.
				if beforeErr == nil || afterErr == nil {
					t.Fatalf("%s: store presence changed on a failed write (%v -> %v)",
						tc.name, beforeErr, afterErr)
				}
			case string(before) != string(after):
				t.Fatalf("%s: store changed on a failed write:\nbefore=%s\nafter=%s", tc.name, before, after)
			}
		})
	}
}

// TestApplyWriteLeavesNoTempResidue pins that a successful write uses the atomic
// temp-file + rename path and leaves no state.json.tmp.* behind.
func TestApplyWriteLeavesNoTempResidue(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})
	if _, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteOpen}); err != nil {
		t.Fatal(err)
	}
	residue, err := filepath.Glob(filepath.Join(filepath.Dir(path), "state.json.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(residue) != 0 {
		t.Fatalf("temp residue after a write: %v", residue)
	}
}

// TestApplyWriteConcurrentOpensKeepOnePrimary drives the writer under -race: two
// concurrent opens for one identity must end with exactly one open item and no
// duplicate ids.
func TestApplyWriteConcurrentOpensKeepOnePrimary(t *testing.T) {
	path := writePath(t)
	saveWriteStore(t, path, Store{V: StoreVersion})

	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := applyWrite(t, path, "p", WriteRequest{Kind: WriteOpen})
			done <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent open: %v", err)
		}
	}

	loaded := loadWriteStore(t, path)
	open := loaded.OpenItems(ident("").Key())
	if len(open) != 1 {
		t.Fatalf("open items = %d, want exactly 1 under concurrency", len(open))
	}
	ids := map[string]bool{}
	for _, item := range loaded.Items {
		if ids[item.ID] {
			t.Fatalf("duplicate item id %q", item.ID)
		}
		ids[item.ID] = true
	}
	residue, err := filepath.Glob(filepath.Join(filepath.Dir(path), "state.json.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(residue) != 0 {
		t.Fatalf("temp residue under concurrency: %v", residue)
	}
}

// TestAdjudicateClearsExemption pins the revoke path: Item.Exemption is cleared
// only by a persisted human adjudication, never by a grade or by file removal.
func TestAdjudicateClearsExemption(t *testing.T) {
	path := writePath(t)
	store := storeWithOpenItem("card-1")
	store.Items[0].Exemption = "no tracker for this change"
	saveWriteStore(t, path, store)
	key := ident("").Key()

	// The exemption alone allows every checkpoint with allow/exempt.
	base := Input{Mode: ModeAlways, Binding: boundBinding(), Identity: availIdentity(),
		Store: loadWriteStore(t, path), Now: vtNow}
	for _, cp := range Checkpoints {
		in := base
		in.Checkpoint = cp
		assertVerdict(t, "exempt "+cp, Grade(in), vWant{DecisionAllow, ReasonExempt, 0, SeverityOK, true})
	}

	if _, err := PersistDecision(path, key, DecisionRequest{
		Checkpoint: CheckpointApplyStart, Kind: DecisionAdjudicate, Choice: "code",
	}, vtNow); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	item, err := loadWriteStore(t, path).Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	if item.Exemption != "" {
		t.Fatalf("exemption = %q, want cleared by adjudication", item.Exemption)
	}
	if _, err := PersistDecision(path, key, DecisionRequest{
		Checkpoint: CheckpointPreMerge, Kind: DecisionOptOut, Choice: "continue",
	}, vtNow); err != nil {
		t.Fatalf("opt-out: %v", err)
	}
	item, err = loadWriteStore(t, path).Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	if item.Exemption != "" {
		t.Fatalf("exemption = %q, want an opt-out to leave it unlocked", item.Exemption)
	}
}

// TestCanonicalProviderIsOrderAndWhitespaceInsensitive pins the equality basis for
// link idempotency.
func TestCanonicalProviderIsOrderAndWhitespaceInsensitive(t *testing.T) {
	a := canonicalProvider(json.RawMessage(`{"b":1,"a":[2,3]}`))
	b := canonicalProvider(json.RawMessage("{\n  \"a\": [2, 3],\n  \"b\": 1\n}"))
	if string(a) != string(b) {
		t.Fatalf("canonical provider mismatch: %s vs %s", a, b)
	}
	if got := string(canonicalProvider(nil)); got != "{}" {
		t.Fatalf("nil provider canonicalised to %q, want {}", got)
	}
	if got := string(canonicalProvider(json.RawMessage(`{"not":"json"`))); !strings.Contains(got, "not") {
		t.Fatalf("invalid provider was replaced with %q, want the raw bytes preserved", got)
	}
}
