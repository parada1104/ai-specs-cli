package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The worktree ledger: which rows exist, which one is open, and when each one
// closed. It is the lifecycle capability's own state (S3), kept apart from the
// tracker record so a close lands in one file and nowhere else.

// WorktreeItem is one worktree lifecycle row. Identity and status are this
// ledger's own columns; the provider record lives in the tracker ledger and the
// two rows are joined by ID.
type WorktreeItem struct {
	ID        string             `json:"id"`
	Identity  ItemIdentity       `json:"identity"`
	Status    string             `json:"status"`
	Decisions []WorktreeDecision `json:"decisions"`
}

// Key is the row's identity key.
func (i WorktreeItem) Key() string { return i.Identity.Key() }

// WorktreeDecision is one lifecycle entry: only a close reaches this ledger, so
// the entry carries the audit trail and nothing else.
type WorktreeDecision struct {
	At         string `json:"at"`
	Checkpoint string `json:"checkpoint"`
	Kind       string `json:"kind"`
}

// WorktreeStore is the worktree ledger: one version and the lifecycle rows.
type WorktreeStore struct {
	V     int            `json:"v"`
	Items []WorktreeItem `json:"items"`
}

// OpenItems returns the open rows for key in stored order.
func (s WorktreeStore) OpenItems(key string) []WorktreeItem {
	var open []WorktreeItem
	for _, item := range s.Items {
		if item.Status == StatusOpen && item.Key() == key {
			open = append(open, item)
		}
	}
	return open
}

// OpenItemsForBranch returns the open rows whose identity shares commonDir and
// branch, regardless of change slug, in stored order.
func (s WorktreeStore) OpenItemsForBranch(commonDir, branch string) []WorktreeItem {
	var open []WorktreeItem
	for _, item := range s.Items {
		if item.Status != StatusOpen {
			continue
		}
		if item.Identity.CommonDir == commonDir && item.Identity.Branch == branch {
			open = append(open, item)
		}
	}
	return open
}

// LatestClosed returns the most recently stored closed row for key, if any.
// Closed rows are never primary (D17); this is the read-only report selector the
// explicit close write path uses to grade the row it just closed.
func (s WorktreeStore) LatestClosed(key string) (WorktreeItem, bool) {
	for i := len(s.Items) - 1; i >= 0; i-- {
		if s.Items[i].Status == StatusClosed && s.Items[i].Key() == key {
			return s.Items[i], true
		}
	}
	return WorktreeItem{}, false
}

// hasRow reports whether any row — open or closed — exists for key.
func (s WorktreeStore) hasRow(key string) bool {
	for _, item := range s.Items {
		if item.Key() == key {
			return true
		}
	}
	return false
}

// hasItemID reports whether any row, open or closed, already uses id.
func (s WorktreeStore) hasItemID(id string) bool {
	for _, item := range s.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// openIfAbsent appends a new open primary row only when the identity has none.
// It returns the stored row and whether it was created.
func (s *WorktreeStore) openIfAbsent(ident ItemIdentity, id, at string) (WorktreeItem, bool, error) {
	if len(s.OpenItems(ident.Key())) > 0 {
		return WorktreeItem{}, false, nil
	}
	row := WorktreeItem{ID: id, Identity: ident, Status: StatusOpen, Decisions: []WorktreeDecision{}}
	s.Items = append(s.Items, row)
	return row, true, nil
}

// closeItem appends the close decision and marks the row closed. There is no
// reopen path: a closed row is never selected again (D17).
func (s *WorktreeStore) closeItem(id, at, checkpoint string) error {
	for i := range s.Items {
		if s.Items[i].ID != id {
			continue
		}
		s.Items[i].Decisions = append(s.Items[i].Decisions,
			WorktreeDecision{At: at, Checkpoint: checkpoint, Kind: DecisionClose})
		s.Items[i].Status = StatusClosed
		return nil
	}
	return fmt.Errorf("ledger: no item %q", id)
}

// worktreeShare is this ledger's part of a pre-split row: the id, the identity,
// the lifecycle status and the close decisions. The provider record and the
// tracker decisions belong to the tracker ledger and are never copied here (S3).
func worktreeShare(row LegacyItem) WorktreeItem {
	share := WorktreeItem{
		ID:        row.ID,
		Identity:  row.Identity,
		Status:    row.Status,
		Decisions: []WorktreeDecision{},
	}
	for _, d := range row.Decisions {
		if d.Kind == DecisionClose {
			share.Decisions = append(share.Decisions, WorktreeDecision{At: d.At, Checkpoint: d.Checkpoint, Kind: d.Kind})
		}
	}
	return share
}

// absorbWorktree takes this ledger's share of a pre-split source: the rows it does
// not have at all, and the close decisions a row it already has is missing. A
// close is never dropped from its source until this file holds it (L11), so a
// close is merged rather than reported as a shadow. Only the fields this ledger
// owns are compared, so a disagreement in a tracker field is not a shadow here.
//
// A source row with no status and no close carries no lifecycle at all — that is
// what this build's own normalised tracker.json looks like — and is skipped, so it
// cannot shadow a closed worktree row.
func absorbWorktree(split *WorktreeStore, legacy LegacyStore) merge {
	seen := make(map[string]int, len(split.Items))
	for i, item := range split.Items {
		seen[item.ID] = i
	}
	var result merge
	for _, row := range legacy.Items {
		share := worktreeShare(row)
		if share.Status == "" && len(share.Decisions) == 0 {
			continue
		}
		at, ok := seen[share.ID]
		if !ok {
			seen[share.ID] = len(split.Items)
			split.Items = append(split.Items, share)
			result.absorbed++
			if len(share.Decisions) > 0 {
				result.carried = true
			}
			continue
		}
		if added := mergeCloses(&split.Items[at], share.Decisions); added > 0 {
			result.carried = true
		}
		if split.Items[at].Status != share.Status {
			result.shadowed++
		}
	}
	return result
}

// mergeCloses appends the close decisions a row does not have yet, in the source's
// order, and reports how many it appended. A close already present is never
// duplicated or rewritten.
func mergeCloses(row *WorktreeItem, closes []WorktreeDecision) int {
	added := 0
	for _, candidate := range closes {
		seen := false
		for _, existing := range row.Decisions {
			if existing == candidate {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		row.Decisions = append(row.Decisions, candidate)
		added++
	}
	return added
}

// trackerShare is the tracker ledger's part of a pre-split row: the provider
// record and the decisions the tracker owns. Status and the close decisions stay
// with the worktree ledger, so they are never copied into a tracker row (S3).
func trackerShare(row LegacyItem) Item {
	item := row.Item
	item.Provider = nullToNil(item.Provider)
	item.Conflict = nullToNil(item.Conflict)
	item.Decisions = trackerDecisions(row.Decisions)
	return item
}

// nullToNil reads a JSON null as the absent value it means, so a row decoded from
// one file compares equal to the same row built in memory.
func nullToNil(raw json.RawMessage) json.RawMessage {
	if string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	return raw
}

// trackerDecisions drops the close decisions, which the worktree ledger owns. The
// result is always a real array so a row this build hands forward decodes to the
// same bytes whoever wrote it.
func trackerDecisions(decisions []Decision) []Decision {
	out := make([]Decision, 0, len(decisions))
	for _, d := range decisions {
		if d.Kind != DecisionClose {
			out = append(out, d)
		}
	}
	return out
}

// absorbTracker takes this ledger's share of the pre-split rows the split file
// lacks, by row id, and the scoped opt-outs it lacks, and counts the rows both
// files have with different content. Only the fields this ledger owns are
// compared, so a disagreement in the lifecycle status is not a shadow here.
func absorbTracker(split *Store, legacy LegacyStore) merge {
	seen := make(map[string]int, len(split.Items))
	for i, item := range split.Items {
		seen[item.ID] = i
	}
	var result merge
	for _, row := range legacy.Items {
		share := trackerShare(row)
		if at, ok := seen[share.ID]; ok {
			// Compare the split row's share too, so the two sides are normalised
			// the same way and a formatting difference is never a shadow.
			if !sameJSON(trackerShare(LegacyItem{Item: split.Items[at]}), share) {
				result.shadowed++
			}
			continue
		}
		seen[share.ID] = len(split.Items)
		split.Items = append(split.Items, share)
		result.absorbed++
	}
	seenOptOut := make(map[string]int, len(split.OptOuts))
	for i, optOut := range split.OptOuts {
		seenOptOut[optOutKey(optOut)] = i
	}
	for _, optOut := range legacy.OptOuts {
		key := optOutKey(optOut)
		if _, ok := seenOptOut[key]; ok {
			continue
		}
		seenOptOut[key] = len(split.OptOuts)
		split.OptOuts = append(split.OptOuts, optOut)
		result.absorbed++
	}
	return result
}

// sameJSON reports whether two ledger rows carry the same content. A row the
// split ledger wrote before an older ai-specs touched it is not rewritten in
// place: the split ledger's version answers and the difference is reported.
func sameJSON(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && left != nil && string(left) == string(right)
}
