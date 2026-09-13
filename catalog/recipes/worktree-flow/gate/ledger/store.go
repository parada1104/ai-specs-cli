package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
)

// Store errors. ErrNoPrimary and ErrMultipleOpen are the two ways "one primary
// item per identity" fails; the latter is a collision for a human to adjudicate,
// never a silent pick.
var (
	ErrNoPrimary           = errors.New("ledger: no open item for identity")
	ErrMultipleOpen        = errors.New("ledger: multiple open items for identity")
	ErrUnknownDecisionKind = errors.New("ledger: unknown decision kind")
)

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

// Store is the on-disk ledger: one version plus the item list (A5).
type Store struct {
	V     int    `json:"v"`
	Items []Item `json:"items"`
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
		ID:         NewItemID(ident.Key(), stamp),
		Identity:   ident,
		Status:     StatusOpen,
		ProviderID: providerID,
		Decisions:  []Decision{{At: stamp, Kind: DecisionOpen}},
	}
	s.Items = append(s.Items, item)
	return item
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
// processes and goroutines. flock is released by the kernel when the file
// description closes, so a crashed writer cannot wedge the ledger.
func withStoreLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
