package ledger

import (
	"errors"
	"time"
)

// View is the composed, read-only ledger: the worktree lifecycle joined with the
// tracker record by the item id both rows carry. Every read the gate used to
// answer from one store is answered here from the ledger that owns the field, so
// no caller needs to know which file a row lives in (L8/S3).
type View struct {
	Worktree WorktreeStore
	Tracker  Store
}

// Primary returns the single open item for key, joined with its tracker record.
// ErrNoPrimary means no open row; ErrMultipleOpen means two open rows collide and
// a human must adjudicate (A5).
func (v View) Primary(key string) (Item, error) {
	open := v.OpenItems(key)
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
func (v View) OpenItems(key string) []Item {
	var open []Item
	for _, row := range v.Worktree.OpenItems(key) {
		open = append(open, v.item(row))
	}
	return open
}

// OpenItemsForBranch returns the open items whose identity shares commonDir and
// branch, regardless of change slug, in stored order.
func (v View) OpenItemsForBranch(commonDir, branch string) []Item {
	var open []Item
	for _, row := range v.Worktree.OpenItemsForBranch(commonDir, branch) {
		open = append(open, v.item(row))
	}
	return open
}

// LatestClosed returns the most recently stored closed item for key, if any.
// Closed rows are never primary (D17); this is the read-only report selector the
// explicit close write path uses to grade the row it just closed.
func (v View) LatestClosed(key string) (Item, bool) {
	row, ok := v.Worktree.LatestClosed(key)
	if !ok {
		return Item{}, false
	}
	return v.item(row), true
}

// ClosedItems returns every closed row in stored order, joined with its tracker
// record. It is the read-only lookup the reconciliation sidecar uses, and it never
// makes a closed row primary.
func (v View) ClosedItems() []Item {
	var closed []Item
	for _, row := range v.Worktree.Items {
		if row.Status == StatusClosed {
			closed = append(closed, v.item(row))
		}
	}
	return closed
}

// Status reports the worktree lifecycle status of the row with id, or "" when the
// worktree ledger has no such row. It answers the host-visible item contract; it
// is never a reason to store a status column in the tracker ledger.
func (v View) Status(id string) string {
	for _, row := range v.Worktree.Items {
		if row.ID == id {
			return row.Status
		}
	}
	return ""
}

// HasOptOut reports whether the primary item for key recorded an opt-out covering
// checkpoint. A lifecycle-scoped opt-out covers every checkpoint of the
// identity/change; a legacy scope-less record covers only its own checkpoint.
func (v View) HasOptOut(key, checkpoint string) bool {
	item, err := v.Primary(key)
	if err != nil {
		return false
	}
	return hasOptOut(item, checkpoint)
}

// HasScopedOptOut reports whether key recorded a scoped opt-out covering
// checkpoint with no open item. Scoped opt-outs fill a row's absence, so this is
// the tracker ledger's answer and stays readable when the worktree ledger is not.
func (v View) HasScopedOptOut(key, checkpoint string) bool {
	return v.Tracker.HasScopedOptOut(key, checkpoint)
}

// item joins one worktree row with its tracker record. The worktree ledger owns
// the identity and the status; the tracker record owns the provider fields, the
// exemption, the conflict and the tracker decisions. A worktree row with no
// tracker record still answers — it is a row whose provider fields were never
// recorded, which grades as needs-item, never as a missing row.
func (v View) item(row WorktreeItem) Item {
	if at, ok := v.Tracker.itemAt(row.ID); ok {
		item := v.Tracker.Items[at]
		item.Identity = row.Identity
		return item
	}
	return Item{ID: row.ID, Identity: row.Identity}
}

// OpenIfAbsent appends a new open primary row to both ledgers only when the
// identity has none. The open is one row per ledger, each carrying only its own
// share (L8): the lifecycle and identity in the worktree ledger, the provider
// record and the open decision in the tracker. A collided identity is refused
// instead of guessing (DW4/A5).
func (v *View) OpenIfAbsent(ident ItemIdentity, providerID string, at time.Time) (Item, bool, error) {
	existing, err := v.Primary(ident.Key())
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, ErrNoPrimary):
		return Item{}, false, err
	}
	stamp := at.UTC().Format(time.RFC3339)
	id := v.Worktree.uniqueItemID(ident, stamp)
	if _, created, err := v.Worktree.openIfAbsent(ident, id, stamp); err != nil {
		return Item{}, false, err
	} else if !created {
		return Item{}, false, ErrMultipleOpen
	}
	item := Item{
		ID:         id,
		Identity:   ident,
		ProviderID: providerID,
		Decisions:  []Decision{{At: stamp, Kind: DecisionOpen}},
	}
	v.Tracker.Items = append(v.Tracker.Items, item)
	return item, true, nil
}

// recordSnapshot updates the observed provider snapshot on the tracker row that
// already exists and reports whether it had one. It never adds a row, so a close
// carrying a provider payload never lands a second close row anywhere: the close
// belongs to the worktree ledger and the snapshot to the row the tracker already
// has (L8).
func (v *View) recordSnapshot(id, state string, provider []byte) bool {
	if _, ok := v.Tracker.itemAt(id); !ok {
		return false
	}
	if err := v.Tracker.RecordSnapshot(id, state, provider); err != nil {
		return false
	}
	return true
}

// singleFileView reads one pre-split store as a view. It exists only for the two
// dead writers PR-2 deletes with the monolithic single store: that file carries
// its own status column, and this type no longer has one, so every row it holds is
// taken as selectable.
func singleFileView(store Store) View {
	worktree := WorktreeStore{V: StoreVersion}
	for _, item := range store.Items {
		worktree.Items = append(worktree.Items, WorktreeItem{ID: item.ID, Identity: item.Identity, Status: StatusOpen})
	}
	return View{Worktree: worktree, Tracker: store}
}
