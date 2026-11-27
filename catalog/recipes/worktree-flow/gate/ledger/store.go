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
	"strings"
	"syscall"
	"time"
)

const (
	// ReasonStoreCorrupt is the design reason for an unreadable ledger: corruption
	// is unevaluable and is never repaired or synthesized (A5).
	ReasonStoreCorrupt = "store-corrupt"

	// Item statuses. At most one item is open (primary) per identity; a closed item
	// is never reopened (D17). Status belongs to the worktree section (L14).
	StatusOpen   = "open"
	StatusClosed = "closed"

	// Append-only decision kinds (A6). A close is the worktree section's decision;
	// open, link, adjudicate and opt-out are the tracker section's.
	DecisionAdjudicate = "adjudicate"
	DecisionOptOut     = "opt-out"
	DecisionOpen       = "open"
	DecisionClose      = "close"
	DecisionLink       = "link"

	// Advisory ceilings (A5): exceeding either only warrants a warning. There is no
	// compaction in this slice, so saving never drops rows.
	MaxItems      = 32
	MaxStoreBytes = 64 * 1024

	// Machine write verbs (DW2). `exempt` is deliberately NOT a sixth decisionKinds
	// entry (DW4). `bind` is the generic one-shot lifecycle verb: open-if-absent +
	// link in one locked transaction, recorded with the open and link decisions.
	WriteOpen   = "open"
	WriteBind   = "bind"
	WriteLink   = "link"
	WriteClose  = "close"
	WriteExempt = "exempt"

	// Idempotent no-op reasons (DW4): each signals success without a mutation.
	WriteReasonAlreadyOpen   = "already-open"
	WriteReasonUnchanged     = "unchanged"
	WriteReasonAlreadyClosed = "already-closed"

	// ScopeLifecycle is the scope of every opt-out this build writes.
	ScopeLifecycle = "lifecycle"

	// lockAttempts/lockBackoff bound every lock acquisition (DW3): ~100 ms total.
	lockAttempts = 5
	lockBackoff  = 20 * time.Millisecond
)

// Ledger errors.
var (
	ErrNoPrimary           = errors.New("ledger: no open item for identity")
	ErrMultipleOpen        = errors.New("ledger: multiple open items for identity")
	ErrUnknownDecisionKind = errors.New("ledger: unknown decision kind")
	ErrLockTimeout         = errors.New("ledger: store lock timeout")
	ErrInvalidWrite        = errors.New("ledger: invalid write request")
)

// WriteKinds is the closed machine-write verb set (DW2).
var WriteKinds = []string{WriteOpen, WriteBind, WriteLink, WriteClose, WriteExempt}

// decisionKinds is the closed set of append-only decision kinds (A6).
var decisionKinds = map[string]bool{
	DecisionAdjudicate: true,
	DecisionOptOut:     true,
	DecisionOpen:       true,
	DecisionClose:      true,
	DecisionLink:       true,
}

// StoreCorruptError marks a ledger that cannot be read as unevaluable. Callers map
// it to ReasonStoreCorrupt and must not synthesize items.
type StoreCorruptError struct {
	Path string
	Err  error
}

func (e *StoreCorruptError) Error() string {
	return "ledger store corrupt at " + e.Path + ": " + e.Err.Error()
}

func (e *StoreCorruptError) Unwrap() error { return e.Err }

// ItemIdentity is the identity snapshot stored on an item.
type ItemIdentity struct {
	CommonDir string `json:"common_dir"`
	Branch    string `json:"branch"`
	Change    string `json:"change"`
}

// Key is the A5 identity key: common_dir + "\x1f" + branch [+ "\x1f" + change].
func (i ItemIdentity) Key() string { return IdentityKey(i.CommonDir, i.Branch, i.Change) }

// Decision is one append-only entry in an item's decisions[] (A6).
type Decision struct {
	At         string `json:"at"`
	Checkpoint string `json:"checkpoint"`
	Scope      string `json:"scope,omitempty"`
	Kind       string `json:"kind"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
}

// ScopedOptOut is a human opt-out recorded for an identity that has no open item yet.
type ScopedOptOut struct {
	Key        string `json:"key"`
	Checkpoint string `json:"checkpoint"`
	Scope      string `json:"scope,omitempty"`
	Choice     string `json:"choice"`
	At         string `json:"at"`
}

// Item is one tracker row. It carries no status and no close decision: the lifecycle
// is the worktree section's state, and the two are joined by ID (L8/L14).
type Item struct {
	ID         string          `json:"id"`
	Identity   ItemIdentity    `json:"identity"`
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

// ProviderBacked reports whether the row satisfies the provider-backed contract.
func (i Item) ProviderBacked() bool { return i.ItemID != "" && i.ProviderID != "" }

// Store is the tracker section: the tracker rows and the lifecycle-scoped opt-outs
// recorded before an item existed (A5).
type Store struct {
	Items   []Item         `json:"items"`
	OptOuts []ScopedOptOut `json:"opt_outs"`
}

// writeAtomic is the shared temp-file + rename writer: one rename per write.
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

// WriteRequest is the --write payload (DW2).
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

// Normalize trims the scalar fields a host may pad and defaults the opaque provider
// payload to {} so it always stores as an object.
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

// Validate rejects anything that is not a well-formed, legal write. A failure fails
// closed (L6), so the caller must exit 2 without persisting anything.
func (r WriteRequest) Validate(checkpoint string) error {
	if !ValidCheckpoint(checkpoint) {
		return fmt.Errorf("%w: checkpoint %q", ErrInvalidWrite, checkpoint)
	}
	if !contains(WriteKinds, r.Kind) {
		return fmt.Errorf("%w: kind %q", ErrInvalidWrite, r.Kind)
	}
	switch r.Kind {
	case WriteLink, WriteBind:
		if r.ItemID == "" {
			return fmt.Errorf("%w: %s requires item_id", ErrInvalidWrite, r.Kind)
		}
	case WriteExempt:
		if r.Reason == "" {
			return fmt.Errorf("%w: exempt requires a reason", ErrInvalidWrite)
		}
	}
	return nil
}

// WriteOutcome is the explicit result of a write.
type WriteOutcome struct {
	Kind    string `json:"kind"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"`
}

// ApplyWriteRequest is one write against one identity at one checkpoint.
type ApplyWriteRequest struct {
	Identity   ItemIdentity
	Checkpoint string
	ProviderID string
	Collision  string
	Request    WriteRequest
}

// ApplyWrite performs one machine write under the bounded lock and reports whether
// the ledger changed. Every failure persists nothing (L6): the caller maps any error
// to stderr plus exit 2.
func ApplyWrite(loc StoreLocation, req ApplyWriteRequest, now time.Time) (WriteOutcome, error) {
	write := req.Request.Normalize()
	if err := write.Validate(req.Checkpoint); err != nil {
		return WriteOutcome{}, err
	}
	ident := req.Identity
	if ident.CommonDir == "" || ident.Branch == "" {
		return WriteOutcome{}, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
	}
	// A bind with no change payload is a deliberate branch-level binding (A11). An
	// explicit change payload keeps the slug-keyed identity, and every other verb
	// keeps the change-ambiguous refusal.
	if write.Kind == WriteBind && write.Change == "" {
		ident.Change = ""
	} else if write.Change != "" || req.Collision == CollisionChangeAmbiguous {
		if write.Change == "" {
			return WriteOutcome{}, fmt.Errorf("%w: change-ambiguous identity needs an explicit change slug", ErrInvalidWrite)
		}
		ident.Change = write.Change
	}

	out := WriteOutcome{Kind: write.Kind}
	err := withStoreLock(loc.Path, func() error {
		file, _, err := loc.Load()
		if err != nil {
			return err
		}
		view := file.View()
		applied, reason, err := applyWriteToView(&view, ident, req, write, now)
		if err != nil {
			return err
		}
		if !applied {
			out.Reason = reason
			return nil
		}
		file.Worktree, file.Tracker = view.Worktree, view.Tracker
		if _, err := loc.Save(file); err != nil {
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

// applyWriteToView is the design lifecycle: refuse → verb → save. It mutates the
// in-memory sections and reports whether the caller must persist them.
func applyWriteToView(view *View, ident ItemIdentity, req ApplyWriteRequest, write WriteRequest, now time.Time) (bool, string, error) {
	stamp := rfc3339Stamp(now)
	key := ident.Key()
	providerID := req.ProviderID
	item, err := view.Primary(key)

	switch write.Kind {
	case WriteOpen:
		if err == nil {
			return false, WriteReasonAlreadyOpen, nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if _, _, err := view.OpenIfAbsent(ident, providerID, now); err != nil {
			return false, "", err
		}
		return true, "", nil

	case WriteBind:
		if write.Change == "" {
			return applyBranchBindToView(view, ident, req, write, now, stamp)
		}
		if err != nil {
			if !errors.Is(err, ErrNoPrimary) {
				return false, "", err
			}
			// Open-if-absent and link in this same locked transaction. OpenIfAbsent
			// never reopens a closed row; it appends a distinct new primary (D17).
			item, _, err = view.OpenIfAbsent(ident, providerID, now)
			if err != nil {
				return false, "", err
			}
			if _, _, err := applyLinkToView(view, item, req, write, stamp); err != nil {
				return false, "", err
			}
			return true, "", nil
		}
		return applyLinkToView(view, item, req, write, stamp)

	case WriteLink:
		if err != nil {
			return false, "", err
		}
		return applyLinkToView(view, item, req, write, stamp)

	case WriteClose:
		if err == nil {
			// S1: a close carrying a provider payload records the observed snapshot.
			// It updates the tracker row that exists; it never adds one (L12.2).
			view.recordSnapshot(item.ID, write.State, write.Provider)
			if err := view.Worktree.closeItem(item.ID, stamp, req.Checkpoint); err != nil {
				return false, "", err
			}
			return true, "", nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if view.Worktree.hasRow(key) {
			return false, WriteReasonAlreadyClosed, nil
		}
		return false, "", ErrNoPrimary

	case WriteExempt:
		if err != nil && !errors.Is(err, ErrNoPrimary) {
			return false, "", err
		}
		if errors.Is(err, ErrNoPrimary) {
			item, _, err = view.OpenIfAbsent(ident, providerID, now)
			if err != nil {
				return false, "", err
			}
		}
		if item.Exemption == write.Reason {
			return false, WriteReasonUnchanged, nil
		}
		setExemption(&view.Tracker, item.ID, write.Reason)
		return true, "", nil
	}
	return false, "", fmt.Errorf("%w: kind %q", ErrInvalidWrite, write.Kind)
}

// applyBranchBindToView is the branch-level external bind: it links the single open
// item whose identity shares the common dir and branch, or opens a fresh branch-only
// primary when none exists. Two open rows cannot be disambiguated by branch alone, so
// it refuses rather than guessing (D17).
func applyBranchBindToView(view *View, ident ItemIdentity, req ApplyWriteRequest, write WriteRequest, now time.Time, stamp string) (bool, string, error) {
	matching := view.OpenItemsForBranch(ident.CommonDir, ident.Branch)
	switch len(matching) {
	case 0:
		item, _, err := view.OpenIfAbsent(ident, req.ProviderID, now)
		if err != nil {
			return false, "", err
		}
		if _, _, err := applyLinkToView(view, item, req, write, stamp); err != nil {
			return false, "", err
		}
		return true, "", nil
	case 1:
		return applyLinkToView(view, matching[0], req, write, stamp)
	default:
		return false, "", ErrMultipleOpen
	}
}

// applyLinkToView writes the link fields onto the selected open item and appends the
// link decision, or reports unchanged when the payload is already its link state.
func applyLinkToView(view *View, item Item, req ApplyWriteRequest, write WriteRequest, stamp string) (bool, string, error) {
	if sameLink(item, write) {
		return false, WriteReasonUnchanged, nil
	}
	at, ok := view.Tracker.itemAt(item.ID)
	if !ok {
		return false, "", ErrNoPrimary
	}
	row := &view.Tracker.Items[at]
	row.ItemID = write.ItemID
	row.URL = write.URL
	row.NativeType = write.NativeType
	row.State = write.State
	row.Provider = json.RawMessage(canonicalProvider(write.Provider))
	row.Decisions = append(row.Decisions, Decision{At: stamp, Checkpoint: req.Checkpoint, Kind: DecisionLink})
	return true, "", nil
}

// CloseBranchItem closes the single open item whose identity shares commonDir and
// branch, ignoring any stored change slug: cleanup knows only those two fields. No
// open row is an idempotent no-op; more than one is a collision that fails closed.
func CloseBranchItem(loc StoreLocation, commonDir, branch, checkpoint string, now time.Time) (WriteOutcome, StoreReport, error) {
	var report StoreReport
	if !ValidCheckpoint(checkpoint) {
		return WriteOutcome{}, report, fmt.Errorf("%w: checkpoint %q", ErrInvalidWrite, checkpoint)
	}
	commonDir = strings.TrimSpace(commonDir)
	branch = strings.TrimSpace(branch)
	if commonDir == "" || branch == "" {
		return WriteOutcome{}, report, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
	}

	out := WriteOutcome{Kind: WriteClose}
	err := withStoreLock(loc.Path, func() error {
		file, loadReport, err := loc.Load()
		report = loadReport
		if err != nil {
			return err
		}
		matching := file.Worktree.OpenItemsForBranch(commonDir, branch)
		switch len(matching) {
		case 0:
			out.Reason = WriteReasonAlreadyClosed
			return nil
		case 1:
			if err := file.Worktree.closeItem(matching[0].ID, rfc3339Stamp(now), checkpoint); err != nil {
				return err
			}
			saveReport, err := loc.Save(file)
			report.BackupFailed = saveReport.BackupFailed
			if err != nil {
				return err
			}
			out.Applied = true
			return nil
		default:
			return ErrMultipleOpen
		}
	})
	if err != nil {
		return WriteOutcome{}, report, err
	}
	return out, report, nil
}

// RecordSnapshot updates the observed provider snapshot on one tracker row without
// touching decisions.
func (s *Store) RecordSnapshot(id, state string, provider json.RawMessage) error {
	at, ok := s.itemAt(id)
	if !ok {
		return fmt.Errorf("ledger: no item %q", id)
	}
	canonical := canonicalProvider(provider)
	if state != "" {
		s.Items[at].State = state
	}
	if len(canonical) > 0 && string(canonical) != "{}" {
		s.Items[at].Provider = json.RawMessage(canonical)
	}
	return nil
}

// hasItemID reports whether any tracker row already uses id. Ids are unique across
// both sections (L14).
func (s Store) hasItemID(id string) bool {
	_, ok := s.itemAt(id)
	return ok
}

// itemAt returns the tracked index of the row with id.
func (s Store) itemAt(id string) (int, bool) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

// AppendDecision appends an append-only decision to the item with id.
func (s *Store) AppendDecision(id string, d Decision) error {
	if !decisionKinds[d.Kind] {
		return fmt.Errorf("%w: %q", ErrUnknownDecisionKind, d.Kind)
	}
	at, ok := s.itemAt(id)
	if !ok {
		return fmt.Errorf("ledger: no item %q", id)
	}
	s.Items[at].Decisions = append(s.Items[at].Decisions, d)
	return nil
}

// setExemption records the tracker.none reason on the item with id.
func setExemption(s *Store, id, reason string) {
	if at, ok := s.itemAt(id); ok {
		s.Items[at].Exemption = reason
	}
}

// sameLink reports whether the payload is already exactly the item's link state.
func sameLink(item Item, req WriteRequest) bool {
	return item.ItemID == req.ItemID &&
		item.URL == req.URL &&
		item.NativeType == req.NativeType &&
		item.State == req.State &&
		bytes.Equal(canonicalProvider(item.Provider), canonicalProvider(req.Provider))
}

// canonicalProvider renders an opaque provider payload order- and
// whitespace-insensitively.
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

// HasScopedOptOut reports whether key recorded a scoped opt-out covering checkpoint.
func (s Store) HasScopedOptOut(key, checkpoint string) bool {
	for _, o := range s.OptOuts {
		if o.Key == key && optOutCovers(o.Scope, o.Checkpoint, checkpoint) {
			return true
		}
	}
	return false
}

// optOutCovers reports whether an opt-out record covers checkpoint.
func optOutCovers(scope, recordCheckpoint, checkpoint string) bool {
	return scope == ScopeLifecycle || recordCheckpoint == checkpoint
}

// OverCeiling reports whether the ledger section exceeds the advisory ceiling (A5).
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

// AppendDecisionToPrimary is the locked read-modify-write append on one ledger file.
// It is a dead writer kept until the monolithic-writer removal lands.
func AppendDecisionToPrimary(path, key string, d Decision) error {
	return withStoreLock(path, func() error {
		loc := StoreAt(path)
		file, _, err := loc.Load()
		if err != nil {
			return err
		}
		item, err := file.View().Primary(key)
		if err != nil {
			return err
		}
		if err := file.Tracker.AppendDecision(item.ID, d); err != nil {
			return err
		}
		_, err = loc.Save(file)
		return err
	})
}

// withStoreLock serializes read-modify-write access to the ledger file with a
// BOUNDED non-blocking acquisition: 5 attempts with a 20 ms backoff, then
// ErrLockTimeout (~100 ms cap, DW3). The lock is "<store>.lock", the name 0.28.0
// already takes for the same file, so an installed 0.28.0 and this build exclude each
// other. T5 needs no exclusion: it writes ledgers/tracker.json, never this file.
func withStoreLock(path string, fn func() error) error {
	lockPath := LockPath(path)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
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

// _ keeps io/fs referenced by the corruption contract above.
var _ = fs.ErrNotExist
