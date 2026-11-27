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
const (
	EventWorkstream    = "workstream"
	EventWorkstreamEnd = "workstream-end"
)

// Event is one plan-flow decision as the verb's caller names it.
type Event struct {
	Event  string `json:"event"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}

// File is the predecessor's absolute path for a repository root.
func File(root string) string { return filepath.Join(root, filepath.FromSlash(Path)) }

// work is the checkout's ledger and the identity its rows are keyed by. The key is
// the common dir plus the branch and never the change slug: a workstream has no
// change, and the ledger file is shared by every worktree while a declaration
// belongs to exactly one of them.
func work(root string) (ledger.StoreLocation, ledger.ItemIdentity, error) {
	ident := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: root})
	if ident.CommonDir == "" || ident.Branch == "" {
		return ledger.StoreLocation{}, ledger.ItemIdentity{},
			errors.New("plan ledger: cannot name this work (detached HEAD, unborn branch, or no Git common dir)")
	}
	row := ledger.ItemIdentity{CommonDir: ident.CommonDir, Branch: ident.Branch}
	return ledger.LedgerAt(ident.CommonDir).WithPlanSource(File(root), row), row, nil
}

// Store is the ledger file the plan section lives in, for the messages a verb
// prints. It is "" when Git cannot name the work.
func Store(root string) string {
	ident := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: root})
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

// Declared is the standing declaration, plus why the ledger could not answer: a verb
// that records a decision must be able to refuse instead of claiming there is
// nothing to end.
func Declared(root string) (declared bool, reason string, err error) {
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

// Workstream is the facts contract the write gate reads: a ledger that cannot be read
// declares nothing, exactly as a missing line file did, so the gate keeps grading the
// facts instead of failing open on a corrupt file it would have to guess about.
func Workstream(root string) (declared bool, reason string) {
	declared, reason, err := Declared(root)
	if err != nil {
		return false, ""
	}
	return declared, reason
}
