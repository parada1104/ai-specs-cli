package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DecisionChoices is the closed human-answer vocabulary (design decisions[]).
var DecisionChoices = []string{"local", "remote", "code", "git", "exempt", "continue"}

// Persist errors. Persisting a human answer fails closed: the caller must exit 2
// rather than pretend the answer was recorded.
var (
	ErrInvalidDecision = errors.New("ledger: invalid human decision")
	ErrUnknownChoice   = errors.New("ledger: unknown human choice")
)

// DecisionRequest is the --decide payload. Only a human answer is accepted here:
// the machine-written kinds open/close/link go through the store API directly.
type DecisionRequest struct {
	Checkpoint string `json:"checkpoint"`
	Kind       string `json:"kind"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
}

// Normalize fills the defaults a host is allowed to omit: an unnamed kind is an
// adjudication, because --decide is the human conflict answer.
func (r DecisionRequest) Normalize() DecisionRequest {
	if r.Kind == "" {
		r.Kind = DecisionAdjudicate
	}
	return r
}

// Validate rejects anything that is not a well-formed, legal human decision.
func (r DecisionRequest) Validate() error {
	if !ValidCheckpoint(r.Checkpoint) {
		return fmt.Errorf("%w: checkpoint %q", ErrInvalidDecision, r.Checkpoint)
	}
	if r.Kind != DecisionAdjudicate && r.Kind != DecisionOptOut {
		return fmt.Errorf("%w: kind %q", ErrInvalidDecision, r.Kind)
	}
	if !contains(DecisionChoices, r.Choice) {
		return fmt.Errorf("%w: %q", ErrUnknownChoice, r.Choice)
	}
	return nil
}

// PersistDecision appends the human answer to the single open item for key and
// clears its current conflict snapshot, all under the store lock. A checkpoint-
// scoped opt-out is the one answer that does not need an existing item (D19): a
// fresh binding records it scoped to its own checkpoint instead of inventing a
// tracked item. It otherwise fails closed on a missing key, a collided identity,
// a corrupt store, or an invalid request, so the CLI can exit 2.
func PersistDecision(storePath, key string, req DecisionRequest, now time.Time) (Decision, error) {
	req = req.Normalize()
	if err := req.Validate(); err != nil {
		return Decision{}, err
	}
	// An identity-less answer has no durable key and is never recorded: the host
	// reports identity_unavailable instead of asking (A2/A5).
	if key == "" {
		return Decision{}, fmt.Errorf("%w: empty identity key", ErrInvalidDecision)
	}
	stamp := rfc3339Stamp(now)
	var persisted Decision
	err := withStoreLock(storePath, func() error {
		store, err := LoadStore(storePath)
		if err != nil {
			return err
		}
		item, err := store.Primary(key)
		if errors.Is(err, ErrNoPrimary) && req.Kind == DecisionOptOut {
			// A fresh binding has no item yet (A5/D19): record the explicit human
			// opt-out checkpoint-scoped, without inventing an item that `always`
			// would then treat as satisfied.
			store.OptOuts = append(store.OptOuts, ScopedOptOut{
				Key: key, Checkpoint: req.Checkpoint, Choice: req.Choice, At: stamp,
			})
			persisted = Decision{At: stamp, Checkpoint: req.Checkpoint, Kind: req.Kind, Choice: req.Choice, Note: req.Note}
			return SaveStore(storePath, store)
		}
		if err != nil {
			return err
		}
		persisted = Decision{At: stamp, Checkpoint: req.Checkpoint, Kind: req.Kind, Choice: req.Choice, Note: req.Note}
		if err := store.AppendDecision(item.ID, persisted); err != nil {
			return err
		}
		clearConflict(&store, item.ID)
		return SaveStore(storePath, store)
	})
	if err != nil {
		return Decision{}, err
	}
	return persisted, nil
}

// PersistConflict records the latest disagreement as the item's current conflict
// snapshot. It never resolves anything and never picks a side.
func PersistConflict(storePath, key string, c Conflict) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return withStoreLock(storePath, func() error {
		store, err := LoadStore(storePath)
		if err != nil {
			return err
		}
		item, err := store.Primary(key)
		if err != nil {
			return err
		}
		setConflict(&store, item.ID, raw)
		return SaveStore(storePath, store)
	})
}

// clearConflict drops the current conflict snapshot: once a human adjudicates,
// the conflict is no longer current.
func clearConflict(s *Store, id string) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			s.Items[i].Conflict = nil
		}
	}
}

// setConflict stores the snapshot on the item with id.
func setConflict(s *Store, id string, raw json.RawMessage) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			s.Items[i].Conflict = raw
		}
	}
}
