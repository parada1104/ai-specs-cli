package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// StoreVersion is the only store schema version this build reads. An unknown
	// version is unevaluable, never reinterpreted (A5).
	StoreVersion = 1

	// ReasonStoreCorrupt is the design reason for an unreadable store: a corrupt
	// file is unevaluable and is never repaired or synthesized (A5).
	ReasonStoreCorrupt = "store-corrupt"

	// Item statuses. At most one item is open (primary) per identity; a closed
	// item is never reopened (D17).
	StatusOpen   = "open"
	StatusClosed = "closed"

	// Append-only decision kinds (A6).
	DecisionAdjudicate = "adjudicate"
	DecisionOptOut     = "opt-out"
	DecisionOpen       = "open"
	DecisionClose      = "close"
	DecisionLink       = "link"

	// Advisory ceilings (A5): exceeding either only warrants a warning. There is
	// no compaction in this slice, so saving never drops rows.
	MaxItems      = 32
	MaxStoreBytes = 64 * 1024

	// Machine write verbs (DW2). This is the closed write vocabulary; `exempt`
	// is deliberately NOT a sixth decisionKinds entry (DW4).
	WriteOpen   = "open"
	WriteLink   = "link"
	WriteClose  = "close"
	WriteExempt = "exempt"

	// Idempotent no-op reasons (DW4): each signals success without a mutation.
	WriteReasonAlreadyOpen   = "already-open"
	WriteReasonUnchanged     = "unchanged"
	WriteReasonAlreadyClosed = "already-closed"

	// lockAttempts/lockBackoff bound every store-lock acquisition (DW3): one
	// budget for every checkpoint and caller, ~100 ms total. The kernel still
	// releases the flock on close, so a crashed writer cannot wedge the ledger.
	lockAttempts = 5
	lockBackoff  = 20 * time.Millisecond
)

// Store errors. ErrNoPrimary and ErrMultipleOpen are the two ways "one primary
// item per identity" fails; the latter is a collision for a human to adjudicate,
// never a silent pick.
var (
	ErrNoPrimary           = errors.New("ledger: no open item for identity")
	ErrMultipleOpen        = errors.New("ledger: multiple open items for identity")
	ErrUnknownDecisionKind = errors.New("ledger: unknown decision kind")
	// ErrLockTimeout is the bounded-lock outcome. Grade paths fail open on it;
	// write and --decide paths fail closed with exit 2 (DW3/L6).
	ErrLockTimeout  = errors.New("ledger: store lock timeout")
	ErrInvalidWrite = errors.New("ledger: invalid write request")
)

// WriteKinds is the closed machine-write verb set (DW2).
var WriteKinds = []string{WriteOpen, WriteLink, WriteClose, WriteExempt}

// decisionKinds is the closed set of append-only decision kinds (A6).
var decisionKinds = map[string]bool{
	DecisionAdjudicate: true,
	DecisionOptOut:     true,
	DecisionOpen:       true,
	DecisionClose:      true,
	DecisionLink:       true,
}

// StoreCorruptError marks a store that cannot be read as unevaluable. Callers
// map it to ReasonStoreCorrupt and must not synthesize items.
type StoreCorruptError struct {
	Path string
	Err  error
}

func (e *StoreCorruptError) Error() string {
	return "ledger store corrupt at " + e.Path + ": " + e.Err.Error()
}

func (e *StoreCorruptError) Unwrap() error { return e.Err }

// ItemIdentity is the identity snapshot stored on an item. Its fields are the
// A2/A11 identity, not a filename; Key is the store matching key.
type ItemIdentity struct {
	CommonDir string `json:"common_dir"`
	Branch    string `json:"branch"`
	Change    string `json:"change"`
}

// Key is the A5 identity key: common_dir + "\x1f" + branch [+ "\x1f" + change].
func (i ItemIdentity) Key() string { return IdentityKey(i.CommonDir, i.Branch, i.Change) }

// Decision is one append-only entry in an item's decisions[] (A6). A kind
// opt-out applies only to its own Checkpoint (D19).
type Decision struct {
	At         string `json:"at"`
	Checkpoint string `json:"checkpoint"`
	Kind       string `json:"kind"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
}

// ScopedOptOut is a checkpoint-scoped human opt-out recorded for an identity that
// has no open item yet (D19): a freshly bound project must be able to answer the
// ask prompt before any tracked item exists. It is keyed by identity so the answer
// stays durable and auditable, and it never synthesizes an item.
type ScopedOptOut struct {
	Key        string `json:"key"`
	Checkpoint string `json:"checkpoint"`
	Choice     string `json:"choice"`
	At         string `json:"at"`
}

// Item is one tracked item. Core fields are provider-neutral (A7): provider
// vocabulary lives only in the opaque Provider payload, which the predicate
// never reads. Conflict is an opaque snapshot recorded alongside decisions.
type Item struct {
	ID         string          `json:"id"`
	Identity   ItemIdentity    `json:"identity"`
	Status     string          `json:"status"`
	ItemID     string          `json:"item_id"`
	ProviderID string          `json:"provider_id"`
	NativeType string          `json:"native_type"`
	URL        string          `json:"url"`
	State      string          `json:"state"`
	Provider   json.RawMessage `json:"provider"`
	Exemption  string          `json:"exemption"`
	Conflict   json.RawMessage `json:"conflict"`
	Decisions  []Decision      `json:"decisions"`
}

// Key is the item's identity key.
func (i Item) Key() string { return i.Identity.Key() }

// Store is the on-disk ledger: one version, the item list, and the
// checkpoint-scoped opt-outs recorded before an item existed (A5).
type Store struct {
	V       int            `json:"v"`
	Items   []Item         `json:"items"`
	OptOuts []ScopedOptOut `json:"opt_outs"`
}

// StorePath is the durable store location: <git-common-dir>/ai-specs/ledger/state.json (A3).
func StorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledger", "state.json")
}

// LoadStore reads the store at path. A missing file is an empty item set, not an
// error. A corrupt or unknown-version file is a *StoreCorruptError carrying no
// items: it is unevaluable, never a synthesized record (A5).
func LoadStore(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Store{V: StoreVersion}, nil
	}
	if err != nil {
		return Store{}, &StoreCorruptError{Path: path, Err: err}
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, &StoreCorruptError{Path: path, Err: err}
	}
	if store.V != StoreVersion {
		return Store{}, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported store version %d", store.V)}
	}
	return store, nil
}

// SaveStore writes the store to path atomically: a state.json.tmp.* file in the
// same directory, fsynced, then renamed over the destination. It never compacts.
func SaveStore(path string, store Store) error {
	if store.V == 0 {
		store.V = StoreVersion
	}
	if store.Items == nil {
		store.Items = []Item{}
	}
	if store.OptOuts == nil {
		store.OptOuts = []ScopedOptOut{}
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

// writeAtomic is the shared temp-file + rename writer.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, writeErr := tmp.Write(data)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	if closeErr := tmp.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = os.Rename(name, path)
	}
	if writeErr != nil {
		os.Remove(name)
		return writeErr
	}
	return nil
}

// WriteRequest is the --write payload (DW2). It is a sibling of DecisionRequest:
// the machine-written kinds open/link/close, plus the tracker.none exemption.
type WriteRequest struct {
	Kind       string          `json:"kind"`
	ItemID     string          `json:"item_id"`
	URL        string          `json:"url"`
	NativeType string          `json:"native_type"`
	State      string          `json:"state"`
	Provider   json.RawMessage `json:"provider"`
	Reason     string          `json:"reason"`
	Change     string          `json:"change"`
}

// Normalize trims the scalar fields a host may pad and defaults the opaque
// provider payload to {} so it always stores as an object.
func (r WriteRequest) Normalize() WriteRequest {
	r.Kind = strings.TrimSpace(r.Kind)
	r.ItemID = strings.TrimSpace(r.ItemID)
	r.URL = strings.TrimSpace(r.URL)
	r.NativeType = strings.TrimSpace(r.NativeType)
	r.State = strings.TrimSpace(r.State)
	r.Reason = strings.TrimSpace(r.Reason)
	r.Change = strings.TrimSpace(r.Change)
	if len(bytes.TrimSpace(r.Provider)) == 0 {
		r.Provider = json.RawMessage("{}")
	}
	return r
}

// Validate rejects anything that is not a well-formed, legal write. Hosts parse
// the flag first; after a successful parse a validation failure fails closed
// (L6), so the caller must exit 2 without persisting anything.
func (r WriteRequest) Validate(checkpoint string) error {
	if !ValidCheckpoint(checkpoint) {
		return fmt.Errorf("%w: checkpoint %q", ErrInvalidWrite, checkpoint)
	}
	if !contains(WriteKinds, r.Kind) {
		return fmt.Errorf("%w: kind %q", ErrInvalidWrite, r.Kind)
	}
	switch r.Kind {
	case WriteLink:
		if r.ItemID == "" {
			return fmt.Errorf("%w: link requires item_id", ErrInvalidWrite)
		}
	case WriteExempt:
		if r.Reason == "" {
			return fmt.Errorf("%w: exempt requires a reason", ErrInvalidWrite)
		}
	}
	return nil
}

// WriteOutcome is the explicit result of a write: whether it mutated the store,
// and the DW4 reason when it legitimately did not.
type WriteOutcome struct {
	Kind    string `json:"kind"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"`
}

// ApplyWriteRequest is one write against one identity at one checkpoint.
// ProviderID is the bound recipe id recorded when the write has to open the
// item; Collision is the identity's Collision field, because change-ambiguous
// refuses an open unless the payload names the change explicitly (DW2).
type ApplyWriteRequest struct {
	Identity   ItemIdentity
	Checkpoint string
	ProviderID string
	Collision  string
	Request    WriteRequest
}

// ApplyWrite performs one machine write under the bounded store lock and reports
// whether the store changed. Every failure persists nothing (L6): the caller maps
// any error to stderr plus exit 2.
func ApplyWrite(storePath string, req ApplyWriteRequest, now time.Time) (WriteOutcome, error) {
	write := req.Request.Normalize()
	if err := write.Validate(req.Checkpoint); err != nil {
		return WriteOutcome{}, err
	}
	ident := req.Identity
	if ident.CommonDir == "" || ident.Branch == "" {
		// An identity-less write has no durable key and is never recorded (A2/A5).
		return WriteOutcome{}, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
	}
	if write.Change != "" || req.Collision == CollisionChangeAmbiguous {
		if write.Change == "" {
			return WriteOutcome{}, fmt.Errorf("%w: change-ambiguous identity needs an explicit change slug", ErrInvalidWrite)
		}
		// The explicit slug is the stored change for this identity.
		ident.Change = write.Change
	}

	out := WriteOutcome{Kind: write.Kind}
	err := withStoreLock(storePath, func() error {
		store, err := LoadStore(storePath)
		if err != nil {
			return err
		}
		applied, reason, err := applyWriteToStore(&store, ident, req, write, now)
		if err != nil {
			return err
		}
		if !applied {
			out.Reason = reason
			return nil
		}
		if err := SaveStore(storePath, store); err != nil {
			return err
		}
		out.Applied = true
		return nil
	})
	if err != nil {
		return WriteOutcome{}, err
	}
	return out, nil
}

// applyWriteToStore is the design lifecycle: refuse → verb → save. It mutates the
// in-memory store and reports whether the caller must persist it.
func applyWriteToStore(store *Store, ident ItemIdentity, req ApplyWriteRequest, write WriteRequest, now time.Time) (bool, string, error) {
	stamp := rfc3339Stamp(now)
	key := ident.Key()
	providerID := req.ProviderID
	item, err := store.Primary(key)

	switch write.Kind {
	case WriteOpen:
		if err == nil {
			return false, WriteReasonAlreadyOpen, nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if _, _, err := store.OpenIfAbsent(ident, providerID, now); err != nil {
			return false, "", err
		}
		return true, "", nil

	case WriteLink:
		if err != nil {
			// A link needs exactly one open primary: a collision or a missing
			// item is not something a link may guess around.
			return false, "", err
		}
		if sameLink(item, write) {
			return false, WriteReasonUnchanged, nil
		}
		for i := range store.Items {
			if store.Items[i].ID != item.ID {
				continue
			}
			store.Items[i].ItemID = write.ItemID
			store.Items[i].URL = write.URL
			store.Items[i].NativeType = write.NativeType
			store.Items[i].State = write.State
			store.Items[i].Provider = json.RawMessage(canonicalProvider(write.Provider))
			store.Items[i].Decisions = append(store.Items[i].Decisions,
				Decision{At: stamp, Checkpoint: req.Checkpoint, Kind: DecisionLink})
			return true, "", nil
		}
		return false, "", ErrNoPrimary

	case WriteClose:
		if err == nil {
			// S1: a close carrying a provider payload records the observed
			// snapshot at close time; the store must never keep a stale
			// link-time snapshot when the provider state was just read. A
			// bare close invents and clears nothing.
			if write.State != "" || len(write.Provider) > 0 {
				if err := store.RecordSnapshot(item.ID, write.State, write.Provider); err != nil {
					return false, "", err
				}
			}
			if err := store.CloseItem(item.ID, Decision{At: stamp, Checkpoint: req.Checkpoint}); err != nil {
				return false, "", err
			}
			return true, "", nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if store.hasRow(key) {
			// A closed row for this identity: the close already happened. The row
			// stays in the store; there is no reopen path (D17).
			return false, WriteReasonAlreadyClosed, nil
		}
		return false, "", ErrNoPrimary

	case WriteExempt:
		if err != nil && !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if errors.Is(err, ErrNoPrimary) {
			// Exempt with no primary uses the same locked OpenIfAbsent as open:
			// an explicit write, never the parse auto-opening (L2).
			if _, _, err := store.OpenIfAbsent(ident, providerID, now); err != nil {
				return false, "", err
			}
			item, err = store.Primary(key)
			if err != nil {
				return false, "", err
			}
		}
		if item.Exemption == write.Reason {
			return false, WriteReasonUnchanged, nil
		}
		setExemption(store, item.ID, write.Reason)
		return true, "", nil
	}
	return false, "", fmt.Errorf("%w: kind %q", ErrInvalidWrite, write.Kind)
}

// OpenIfAbsent appends a new open primary item only when the identity has none.
// A repeated open is a no-op rather than a self-inflicted collision, and a
// collided identity is refused instead of guessing (DW4/A5).
func (s *Store) OpenIfAbsent(ident ItemIdentity, providerID string, at time.Time) (Item, bool, error) {
	existing, err := s.Primary(ident.Key())
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, ErrNoPrimary):
		return Item{}, false, err
	}
	stamp := at.UTC().Format(time.RFC3339)
	item := Item{
		ID:         s.uniqueItemID(ident, stamp),
		Identity:   ident,
		Status:     StatusOpen,
		ProviderID: providerID,
		Decisions:  []Decision{{At: stamp, Kind: DecisionOpen}},
	}
	s.Items = append(s.Items, item)
	return item, true, nil
}

// uniqueItemID returns NewItemID unless that id is already taken by any stored
// row — open or closed — in which case it suffixes the stamp input with \x1fN
// until unique. Two opens for one identity in the same second therefore cannot
// share an id. The unlocked OpenItem stays deterministic for existing tests.
func (s *Store) uniqueItemID(ident ItemIdentity, stamp string) string {
	id := NewItemID(ident.Key(), stamp)
	for n := 1; s.hasItemID(id); n++ {
		id = NewItemID(ident.Key(), stamp+"\x1f"+strconv.Itoa(n))
	}
	return id
}

// hasItemID reports whether any row, open or closed, already uses id.
func (s Store) hasItemID(id string) bool {
	for _, item := range s.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// hasRow reports whether any row — open or closed — exists for key.
func (s Store) hasRow(key string) bool {
	for _, item := range s.Items {
		if item.Key() == key {
			return true
		}
	}
	return false
}

// setExemption records the tracker.none reason on the item with id. It is the
// only writer of Item.Exemption; adjudication is the only clearer (decide.go).
func setExemption(s *Store, id, reason string) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			s.Items[i].Exemption = reason
		}
	}
}

// sameLink reports whether the payload is already exactly the item's link state.
// Core fields compare directly; the opaque provider payload compares on canonical
// bytes so JSON formatting is not a spurious change.
func sameLink(item Item, req WriteRequest) bool {
	return item.ItemID == req.ItemID &&
		item.URL == req.URL &&
		item.NativeType == req.NativeType &&
		item.State == req.State &&
		bytes.Equal(canonicalProvider(item.Provider), canonicalProvider(req.Provider))
}

// canonicalProvider renders an opaque provider payload order- and
// whitespace-insensitively. An unparsable payload keeps its raw bytes rather than
// being silently dropped.
func canonicalProvider(raw json.RawMessage) []byte {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return []byte("{}")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	out, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return out
}

// NewItemID is the design id: 16 lowercase hex chars of
// sha256(identity key + "\x1f" + opened-at).
func NewItemID(identityKey, openedAt string) string {
	sum := sha256.Sum256([]byte(identityKey + "\x1f" + openedAt))
	return hex.EncodeToString(sum[:])[:16]
}

// OpenItem appends a NEW open primary item for ident and records its open
// decision. A closed item for the same identity is left closed: new work on a
// reused branch opens a new item (D17). It does not check for an existing open
// item; Primary reports that collision.
func (s *Store) OpenItem(ident ItemIdentity, providerID string, at time.Time) Item {
	stamp := at.UTC().Format(time.RFC3339)
	item := Item{
		ID:         s.uniqueItemID(ident, stamp),
		Identity:   ident,
		Status:     StatusOpen,
		ProviderID: providerID,
		Decisions:  []Decision{{At: stamp, Kind: DecisionOpen}},
	}
	s.Items = append(s.Items, item)
	return item
}

// RecordSnapshot updates the observed provider snapshot (state and opaque
// payload) on one item without touching status or decisions. Used by close to
// persist what the provider actually reported at close time (S1).
func (s *Store) RecordSnapshot(id, state string, provider json.RawMessage) error {
	for i := range s.Items {
		if s.Items[i].ID != id {
			continue
		}
		canonical := canonicalProvider(provider)
		if state != "" {
			s.Items[i].State = state
		}
		if len(canonical) > 0 && string(canonical) != "{}" {
			s.Items[i].Provider = json.RawMessage(canonical)
		}
		return nil
	}
	return fmt.Errorf("ledger: no item %q", id)
}

// CloseItem appends a close decision and marks the item closed. There is no
// reopen path: a closed item is never selected again (D17).
func (s *Store) CloseItem(id string, d Decision) error {
	for i := range s.Items {
		if s.Items[i].ID != id {
			continue
		}
		d.Kind = DecisionClose
		if err := s.AppendDecision(id, d); err != nil {
			return err
		}
		s.Items[i].Status = StatusClosed
		return nil
	}
	return fmt.Errorf("ledger: no item %q", id)
}

// AppendDecision appends an append-only decision to the item with id. Earlier
// entries are never rewritten and the item's status is not inferred from the
// decision kind.
func (s *Store) AppendDecision(id string, d Decision) error {
	if !decisionKinds[d.Kind] {
		return fmt.Errorf("%w: %q", ErrUnknownDecisionKind, d.Kind)
	}
	for i := range s.Items {
		if s.Items[i].ID == id {
			s.Items[i].Decisions = append(s.Items[i].Decisions, d)
			return nil
		}
	}
	return fmt.Errorf("ledger: no item %q", id)
}

// Primary returns the single open item for key. ErrNoPrimary means no open item;
// ErrMultipleOpen means two open rows collide and a human must adjudicate (A5).
func (s Store) Primary(key string) (Item, error) {
	open := s.OpenItems(key)
	switch len(open) {
	case 0:
		return Item{}, ErrNoPrimary
	case 1:
		return open[0], nil
	default:
		return Item{}, ErrMultipleOpen
	}
}

// OpenItems returns the open items for key in stored order.
func (s Store) OpenItems(key string) []Item {
	var open []Item
	for _, item := range s.Items {
		if item.Status == StatusOpen && item.Key() == key {
			open = append(open, item)
		}
	}
	return open
}

// LatestClosed returns the most recently stored closed item for key, if any.
// Closed rows are never primary (D17); this is the read-only report selector the
// explicit close write path uses to grade the row it just closed, and it never
// makes a closed row selectable for new work.
func (s Store) LatestClosed(key string) (Item, bool) {
	for i := len(s.Items) - 1; i >= 0; i-- {
		if s.Items[i].Status == StatusClosed && s.Items[i].Key() == key {
			return s.Items[i], true
		}
	}
	return Item{}, false
}

// HasOptOut reports whether the primary item for key recorded an opt-out at
// checkpoint. An opt-out covers only the checkpoint it was answered for (D19).
func (s Store) HasOptOut(key, checkpoint string) bool {
	item, err := s.Primary(key)
	if err != nil {
		return false
	}
	for _, d := range item.Decisions {
		if d.Kind == DecisionOptOut && d.Checkpoint == checkpoint {
			return true
		}
	}
	return false
}

// HasScopedOptOut reports whether key recorded a checkpoint-scoped opt-out with
// no open item. Like an item opt-out it covers only the checkpoint it answered.
func (s Store) HasScopedOptOut(key, checkpoint string) bool {
	for _, o := range s.OptOuts {
		if o.Key == key && o.Checkpoint == checkpoint {
			return true
		}
	}
	return false
}

// OverCeiling reports whether the store exceeds the advisory item or byte
// ceiling (A5). Nothing is compacted; the caller only warns.
func (s Store) OverCeiling() bool {
	if len(s.Items) > MaxItems {
		return true
	}
	data, err := json.Marshal(s)
	if err != nil {
		return false
	}
	return len(data) > MaxStoreBytes
}

// AppendDecisionToPrimary is the locked read-modify-write append: it takes an
// exclusive lock on the store, reloads it, appends d to the single open item for
// key, and saves atomically, so concurrent appenders do not lose each other's
// decisions. It fails closed on a missing or collided primary.
func AppendDecisionToPrimary(path, key string, d Decision) error {
	return withStoreLock(path, func() error {
		store, err := LoadStore(path)
		if err != nil {
			return err
		}
		item, err := store.Primary(key)
		if err != nil {
			return err
		}
		if err := store.AppendDecision(item.ID, d); err != nil {
			return err
		}
		return SaveStore(path, store)
	})
}

// withStoreLock serializes read-modify-write access to the store across
// processes and goroutines with a BOUNDED non-blocking acquisition: 5 attempts
// with a 20 ms backoff, then ErrLockTimeout (~100 ms cap, DW3). flock is released
// by the kernel when the file description closes, so a crashed writer cannot
// wedge the ledger. Grade paths treat ErrLockTimeout as fail-open; write and
// --decide paths fail closed.
func withStoreLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()

	acquired := false
	for attempt := 0; attempt < lockAttempts; attempt++ {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			acquired = true
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		time.Sleep(lockBackoff)
	}
	if !acquired {
		return ErrLockTimeout
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
