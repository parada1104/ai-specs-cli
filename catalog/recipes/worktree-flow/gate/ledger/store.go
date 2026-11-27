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
	// StoreVersion is the only store schema version this build reads. An unknown
	// version is unevaluable, never reinterpreted (A5).
	StoreVersion = 1

	// ReasonStoreCorrupt is the design reason for an unreadable store: a corrupt
	// file is unevaluable and is never repaired or synthesized (A5).
	ReasonStoreCorrupt = "store-corrupt"

	// Item statuses. At most one item is open (primary) per identity; a closed
	// item is never reopened (D17). Status belongs to the worktree ledger (S3):
	// the tracker row has no status column.
	StatusOpen   = "open"
	StatusClosed = "closed"

	// Append-only decision kinds (A6). A close is the worktree ledger's decision;
	// open, link, adjudicate and opt-out are the tracker ledger's.
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
	// is deliberately NOT a sixth decisionKinds entry (DW4). `bind` is the
	// generic one-shot lifecycle verb: open-if-absent + link in one locked
	// transaction, recorded with the existing open and link decisions.
	WriteOpen   = "open"
	WriteBind   = "bind"
	WriteLink   = "link"
	WriteClose  = "close"
	WriteExempt = "exempt"

	// Idempotent no-op reasons (DW4): each signals success without a mutation.
	WriteReasonAlreadyOpen   = "already-open"
	WriteReasonUnchanged     = "unchanged"
	WriteReasonAlreadyClosed = "already-closed"

	// ScopeLifecycle is the scope of every opt-out this build writes: the human
	// declined to track the change, so the decision covers the identity/change
	// lifecycle and later checkpoints do not ask again. An empty scope marks a
	// record written before lifecycle scoping, which stays checkpoint-scoped.
	ScopeLifecycle = "lifecycle"

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
var WriteKinds = []string{WriteOpen, WriteBind, WriteLink, WriteClose, WriteExempt}

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
// opt-out carries ScopeLifecycle, so it suppresses the remaining checkpoints of
// the same identity/change; Checkpoint keeps the audit trail of where the human
// answered.
type Decision struct {
	At         string `json:"at"`
	Checkpoint string `json:"checkpoint"`
	Scope      string `json:"scope,omitempty"`
	Kind       string `json:"kind"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
}

// ScopedOptOut is a human opt-out recorded for an identity that has no open item
// yet (A5): a freshly bound project must be able to answer the ask prompt before
// any tracked item exists. It is keyed by identity so the answer stays durable
// and auditable, it never synthesizes an item, and it is lifecycle-scoped like
// every other opt-out (Checkpoint is the audit trail of where it was answered).
type ScopedOptOut struct {
	Key        string `json:"key"`
	Checkpoint string `json:"checkpoint"`
	Scope      string `json:"scope,omitempty"`
	Choice     string `json:"choice"`
	At         string `json:"at"`
}

// Item is one tracker row. Core fields are provider-neutral (A7): provider
// vocabulary lives only in the opaque Provider payload, which the predicate
// never reads. Conflict is an opaque snapshot recorded alongside decisions.
//
// The row carries no status and no close decision: the lifecycle is the worktree
// ledger's state, and the two rows are joined by ID (L8/S3).
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

// ProviderBacked reports whether the row satisfies the provider-backed item
// contract: a provider item id and the bound provider id. A local-only row —
// opened locally and never linked to a provider item — is never compliant, so
// the predicate reports needs-item for it under the usual mode posture.
func (i Item) ProviderBacked() bool { return i.ItemID != "" && i.ProviderID != "" }

// Store is the tracker ledger: one version, the tracker rows, and the
// lifecycle-scoped opt-outs recorded before an item existed (A5).
type Store struct {
	V       int            `json:"v"`
	Items   []Item         `json:"items"`
	OptOuts []ScopedOptOut `json:"opt_outs"`
}

// StorePath is the pre-split single-store location:
// <git-common-dir>/ai-specs/ledger/state.json (A3). This build only reads it, as
// the one-release fallback of every ledger (S11).
func StorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledger", "state.json")
}

// LegacyStore is the pre-split single store (v1) this build only reads: one row
// per item carrying both capabilities' fields. Its rows are handed forward to the
// ledger that owns each field; the file itself is never rewritten.
type LegacyStore struct {
	V       int            `json:"v"`
	Items   []LegacyItem   `json:"items"`
	OptOuts []ScopedOptOut `json:"opt_outs"`
}

// LegacyItem is one pre-split row: the tracker row plus the status column the
// worktree ledger now owns. Embedding keeps every shared field, tag and rule in
// one place, and the JSON stays flat.
type LegacyItem struct {
	Status string `json:"status"`
	Item
}

// LoadStore reads the tracker store at path. A missing file is an empty item set,
// not an error. A corrupt or unknown-version file is a *StoreCorruptError
// carrying no items: it is unevaluable, never a synthesized record (A5).
func LoadStore(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Store{V: StoreVersion}, nil
	}
	if err != nil {
		return Store{}, &StoreCorruptError{Path: path, Err: err}
	}
	return trackerCodec.decode(path, data)
}

// LoadLegacyStore reads the pre-split store at path. A missing file is an empty
// item set; a corrupt or unknown-version file is a *StoreCorruptError, exactly as
// LoadStore reports it.
func LoadLegacyStore(path string) (LegacyStore, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return LegacyStore{V: StoreVersion}, nil
	}
	if err != nil {
		return LegacyStore{}, &StoreCorruptError{Path: path, Err: err}
	}
	var store LegacyStore
	if err := json.Unmarshal(data, &store); err != nil {
		return LegacyStore{}, &StoreCorruptError{Path: path, Err: err}
	}
	if store.V != StoreVersion {
		return LegacyStore{}, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported store version %d", store.V)}
	}
	return store, nil
}

// SaveStore writes the tracker store to path atomically: a state.json.tmp.* file
// in the same directory, fsynced, then renamed over the destination. It never
// compacts.
func SaveStore(path string, store Store) error {
	data, err := trackerCodec.encode(store)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
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
	case WriteLink, WriteBind:
		// Both verbs record the supplied native id: link targets the existing
		// open primary, bind opens-if-absent and then links.
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
//
// Each verb writes only the ledger that owns its row (L8): a close lands in the
// worktree ledger alone, and open/bind/link/exempt land on the tracker row —
// except that an open also records the row's identity and status in the worktree
// ledger, which is where the lifecycle lives.
func ApplyWrite(locs Ledgers, req ApplyWriteRequest, now time.Time) (WriteOutcome, error) {
	write := req.Request.Normalize()
	if err := write.Validate(req.Checkpoint); err != nil {
		return WriteOutcome{}, err
	}
	ident := req.Identity
	if ident.CommonDir == "" || ident.Branch == "" {
		// An identity-less write has no durable key and is never recorded (A2/A5).
		return WriteOutcome{}, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
	}
	// A bind with no change payload is a deliberate branch-level binding: the
	// branch, not any change slug, is the identity, so external binding works with
	// no SDD/ODD/OpenSpec artifact (A11). An explicit change payload keeps the
	// slug-keyed identity, and every other verb keeps the change-ambiguous refusal.
	if write.Kind == WriteBind && write.Change == "" {
		ident.Change = ""
	} else if write.Change != "" || req.Collision == CollisionChangeAmbiguous {
		if write.Change == "" {
			return WriteOutcome{}, fmt.Errorf("%w: change-ambiguous identity needs an explicit change slug", ErrInvalidWrite)
		}
		// The explicit slug is the stored change for this identity.
		ident.Change = write.Change
	}

	out := WriteOutcome{Kind: write.Kind}
	err := withStoreLock(locs.Tracker.Path, func() error {
		view, _, err := locs.Load()
		if err != nil {
			return err
		}
		applied, reason, dirty, err := applyWriteToView(&view, ident, req, write, now)
		if err != nil {
			return err
		}
		if !applied {
			out.Reason = reason
			return nil
		}
		if err := locs.save(view, dirty); err != nil {
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
// in-memory ledgers and reports which of them the caller must persist.
func applyWriteToView(view *View, ident ItemIdentity, req ApplyWriteRequest, write WriteRequest, now time.Time) (bool, string, ledgerDirty, error) {
	stamp := rfc3339Stamp(now)
	key := ident.Key()
	providerID := req.ProviderID
	item, err := view.Primary(key)
	var dirty ledgerDirty

	switch write.Kind {
	case WriteOpen:
		if err == nil {
			return false, WriteReasonAlreadyOpen, dirty, nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", dirty, err
		}
		if _, _, err := view.OpenIfAbsent(ident, providerID, now); err != nil {
			return false, "", dirty, err
		}
		return true, "", ledgerDirty{worktree: true, tracker: true}, nil

	case WriteBind:
		if write.Change == "" {
			// A bind that omits `change` is a branch-level external bind: match the
			// single open row for this common dir and branch regardless of stored
			// slug, refuse a collision, or otherwise open a fresh branch-only item.
			return applyBranchBindToView(view, ident, req, write, now, stamp)
		}
		opened := false
		if err != nil {
			if !errors.Is(err, ErrNoPrimary) {
				// A collided identity is not something a bind may guess around.
				return false, "", dirty, err
			}
			// No open primary: open-if-absent and link in this same locked
			// transaction, so bind is one atomic open+link. OpenIfAbsent never
			// reopens a closed row; it appends a distinct new primary (D17).
			item, _, err = view.OpenIfAbsent(ident, providerID, now)
			if err != nil {
				return false, "", dirty, err
			}
			opened = true
		}
		applied, reason, err := applyLinkToView(view, item, req, write, stamp)
		if err != nil {
			return false, "", dirty, err
		}
		dirty.tracker = applied
		if opened {
			// The open itself changed both ledgers, whatever the link reported.
			return true, "", ledgerDirty{worktree: true, tracker: true}, nil
		}
		return applied, reason, dirty, nil

	case WriteLink:
		if err != nil {
			// A link needs exactly one open primary: a collision or a missing
			// item is not something a link may guess around.
			return false, "", dirty, err
		}
		applied, reason, err := applyLinkToView(view, item, req, write, stamp)
		if err != nil {
			return false, "", dirty, err
		}
		dirty.tracker = applied
		return applied, reason, dirty, nil

	case WriteClose:
		if err == nil {
			// S1: a close carrying a provider payload records the observed
			// snapshot at close time; the store must never keep a stale
			// link-time snapshot when the provider state was just read. A
			// bare close invents and clears nothing. The snapshot is updated on
			// the tracker row that already exists; it never adds a row there, so
			// the close decision itself lands in the worktree ledger only.
			dirty.tracker = view.recordSnapshot(item.ID, write.State, write.Provider)
			if err := view.Worktree.closeItem(item.ID, stamp, req.Checkpoint); err != nil {
				return false, "", ledgerDirty{}, err
			}
			dirty.worktree = true
			return true, "", dirty, nil
		}
		if !errors.Is(err, ErrNoPrimary) {
			return false, "", dirty, err
		}
		if view.Worktree.hasRow(key) {
			// A closed row for this identity: the close already happened. The row
			// stays in the ledger; there is no reopen path (D17).
			return false, WriteReasonAlreadyClosed, dirty, nil
		}
		return false, "", dirty, ErrNoPrimary

	case WriteExempt:
		if err != nil && !errors.Is(err, ErrNoPrimary) {
			return false, "", dirty, err
		}
		opened := false
		if errors.Is(err, ErrNoPrimary) {
			// Exempt with no primary uses the same locked openIfAbsent as open:
			// an explicit write, never the parse auto-opening (L2).
			item, _, err = view.OpenIfAbsent(ident, providerID, now)
			if err != nil {
				return false, "", dirty, err
			}
			opened = true
		}
		if item.Exemption == write.Reason {
			return false, WriteReasonUnchanged, ledgerDirty{worktree: opened, tracker: opened}, nil
		}
		setExemption(&view.Tracker, item.ID, write.Reason)
		return true, "", ledgerDirty{worktree: opened, tracker: true}, nil
	}
	return false, "", dirty, fmt.Errorf("%w: kind %q", ErrInvalidWrite, write.Kind)
}

// applyBranchBindToView is the branch-level external bind: it links the single
// open item whose identity shares the common dir and branch, ignoring any stored
// change slug, and opens a fresh branch-only primary only when none exists. Two
// open rows for one branch cannot be disambiguated by branch alone, so it refuses
// rather than guessing (D17). It is bind-only; open, link, and close keep their
// slug-keyed identities.
func applyBranchBindToView(view *View, ident ItemIdentity, req ApplyWriteRequest, write WriteRequest, now time.Time, stamp string) (bool, string, ledgerDirty, error) {
	matching := view.OpenItemsForBranch(ident.CommonDir, ident.Branch)
	switch len(matching) {
	case 0:
		item, _, err := view.OpenIfAbsent(ident, req.ProviderID, now)
		if err != nil {
			return false, "", ledgerDirty{}, err
		}
		if _, _, err := applyLinkToView(view, item, req, write, stamp); err != nil {
			return false, "", ledgerDirty{}, err
		}
		// The open changed both ledgers, whatever the link itself reported.
		return true, "", ledgerDirty{worktree: true, tracker: true}, nil
	case 1:
		applied, reason, err := applyLinkToView(view, matching[0], req, write, stamp)
		if err != nil {
			return false, "", ledgerDirty{}, err
		}
		return applied, reason, ledgerDirty{tracker: applied}, nil
	default:
		return false, "", ledgerDirty{}, ErrMultipleOpen
	}
}

// applyLinkToView writes the link fields onto an already-selected open item and
// appends the link decision, or reports unchanged when the payload is already the
// item's exact link state. It is shared by the link and bind verbs so both record
// provider-neutral fields identically.
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
// branch, ignoring any stored change slug. It is the flow-agnostic VCS-boundary
// close seam: cleanup knows only the repository's common dir and the branch name,
// so it cannot rebuild the exact identity key when the item carries a slug. A
// ledger with no open row for that pair is an idempotent no-op — never bound, or
// already closed — and nothing is written. More than one open row is a collision
// that fails closed rather than guessing (D17). A closed row is never reopened or
// rewritten.
//
// The close is the worktree ledger's decision, so this seam reads and writes that
// ledger alone: a corrupt tracker file cannot stop the automatic post-merge close.
func CloseBranchItem(locs Ledgers, commonDir, branch, checkpoint string, now time.Time) (WriteOutcome, LedgerReports, error) {
	var reports LedgerReports
	if !ValidCheckpoint(checkpoint) {
		return WriteOutcome{}, reports, fmt.Errorf("%w: checkpoint %q", ErrInvalidWrite, checkpoint)
	}
	commonDir = strings.TrimSpace(commonDir)
	branch = strings.TrimSpace(branch)
	if commonDir == "" || branch == "" {
		return WriteOutcome{}, reports, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
	}

	out := WriteOutcome{Kind: WriteClose}
	err := withStoreLock(locs.Tracker.Path, func() error {
		worktree, report, err := locs.Worktree.LoadWorktree()
		reports.Worktree = report
		if err != nil {
			return err
		}
		matching := worktree.OpenItemsForBranch(commonDir, branch)
		switch len(matching) {
		case 0:
			out.Reason = WriteReasonAlreadyClosed
			return nil
		case 1:
			if err := worktree.closeItem(matching[0].ID, rfc3339Stamp(now), checkpoint); err != nil {
				return err
			}
			saveReport, err := locs.Worktree.SaveWorktree(worktree)
			reports.Worktree.Absorbed = report.Absorbed
			reports.Worktree.Shadowed = report.Shadowed
			reports.Worktree.LegacyRows = report.LegacyRows
			reports.Worktree.BackupFailed = saveReport.BackupFailed
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
		return WriteOutcome{}, reports, err
	}
	return out, reports, nil
}

// RecordSnapshot updates the observed provider snapshot (state and opaque
// payload) on one tracker row without touching decisions. Used by close to
// persist what the provider actually reported at close time (S1). A worktree row
// with no tracker row has no snapshot to update, so nothing is written for it.
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
// both ledgers (L11.3).
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

// AppendDecision appends an append-only decision to the item with id. Earlier
// entries are never rewritten and no status is inferred from the decision kind.
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

// setExemption records the tracker.none reason on the item with id. It is the
// only writer of Item.Exemption; adjudication is the only clearer (decide.go).
func setExemption(s *Store, id, reason string) {
	if at, ok := s.itemAt(id); ok {
		s.Items[at].Exemption = reason
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

// HasScopedOptOut reports whether key recorded a scoped opt-out covering
// checkpoint with no open item. Like an item opt-out, a lifecycle-scoped record
// covers every checkpoint and a legacy scope-less one only its own.
func (s Store) HasScopedOptOut(key, checkpoint string) bool {
	for _, o := range s.OptOuts {
		if o.Key == key && optOutCovers(o.Scope, o.Checkpoint, checkpoint) {
			return true
		}
	}
	return false
}

// optOutCovers reports whether an opt-out record covers checkpoint. A record with
// no scope was written before lifecycle scoping and covers only the checkpoint it
// names, so every old store keeps its exact old behavior; a lifecycle-scoped
// record covers the remaining checkpoints of the identity/change.
func optOutCovers(scope, recordCheckpoint, checkpoint string) bool {
	return scope == ScopeLifecycle || recordCheckpoint == checkpoint
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
		item, err := singleFileView(store).Primary(key)
		if err != nil {
			return err
		}
		if err := store.AppendDecision(item.ID, d); err != nil {
			return err
		}
		return SaveStore(path, store)
	})
}

// withStoreLock serializes read-modify-write access to the ledger across
// processes and goroutines with a BOUNDED non-blocking acquisition: 5 attempts
// with a 20 ms backoff, then ErrLockTimeout (~100 ms cap, DW3). flock is released
// by the kernel when the file description closes, so a crashed writer cannot
// wedge the ledger. Grade paths treat ErrLockTimeout as fail-open; write and
// --decide paths fail closed.
//
// One lock covers a whole ledger directory (L12.3): a mutation that touches two
// files must exclude every other writer, and two overrides naming different files
// in one directory must not take different locks for the same lifecycle rows.
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
