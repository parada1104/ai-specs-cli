package planledger

// The plan ledger's root-module port (change-ledger-v2 S14): the CLI's one way to the
// plan section of the versioned ledger file. This package used to own an append-only
// line file at the repository root; that file is now a read-only predecessor the
// ledger absorbs on read (T6b), so nothing here writes it.

import (
	"errors"
	"path/filepath"

	"ai-specs.dev/worktree-gate/ledger"
)

// Path is the predecessor's repository-relative location: the line file one build
// wrote per checkout, before the plan section existed. It is read on load and never
// written again.
const Path = ".ai-specs/plan-ledger.jsonl"

// The two events this port records. A reader that meets a kind it does not know
// skips it, so a later unit can add its own.
// They are the ledger's own kinds, aliased so the verb and the fold can never drift
// apart; a later unit adds its kinds there and inherits them here.
const (
	EventWorkstream    = ledger.PlanKindWorkstream
	EventWorkstreamEnd = ledger.PlanKindWorkstreamEnd
)

// Event is one plan-flow decision as this package's verbs name it.
type Event struct {
	Event  string
	At     string
	Reason string
}

// File is the predecessor's absolute path for a repository root.
func File(root string) string { return filepath.Join(root, filepath.FromSlash(Path)) }

// work is the checkout's ledger and the plan identity its rows are keyed by. The key
// is the common dir plus the worktree root: a declaration belongs to the worktree, so a
// renamed branch neither loses it nor brings it back, and the branch travels as data
// (R1-001).
func work(root string) (ledger.StoreLocation, ledger.PlanIdentity, error) {
	ident := ledger.PlanIdentityAt(root, nil)
	if ident.CommonDir == "" || ident.Root == "" {
		return ledger.StoreLocation{}, ledger.PlanIdentity{},
			errors.New("plan ledger: cannot name this checkout (no Git common dir)")
	}
	return ledger.LedgerAt(ident.CommonDir).WithPlanSource(File(root), ident), ident, nil
}

// Store is the ledger file the plan section lives in, for the messages a verb prints.
// It needs no branch, so a repository without a commit still names its ledger.
func Store(root string) string {
	ident := ledger.PlanIdentityAt(root, nil)
	if ident.CommonDir == "" {
		return ""
	}
	return ledger.StorePath(ident.CommonDir)
}

// Append records one decision in the plan section. Every failure persists nothing, so
// the verb that records a declaration can exit non-zero instead of claiming success.
func Append(root string, event Event) error {
	if event.Event == "" {
		return errors.New("plan ledger: the event name is empty")
	}
	if event.At == "" {
		return errors.New("plan ledger: the event time is empty")
	}
	loc, ident, err := work(root)
	if err != nil {
		return err
	}
	_, err = ledger.RecordPlanDecision(loc, ident, ledger.PlanDecision{
		At: event.At, Kind: event.Event, Reason: event.Reason,
	})
	return err
}

// Workstream is the standing declaration, and why the ledger could not answer it. The
// error is never swallowed: a verb that records a decision must refuse instead of
// claiming there is nothing to end, and the write gate turns it into a note naming the
// file, because a corrupt shared ledger would otherwise look like "this worktree is a
// change" in every worktree of the clone.
func Workstream(root string) (declared bool, reason string, err error) {
	loc, ident, err := work(root)
	if err != nil {
		return false, "", err
	}
	file, _, err := loc.Load()
	if err != nil {
		return false, "", err
	}
	declared, reason = file.Plan.Workstream(ident.Key())
	return declared, reason, nil
}
