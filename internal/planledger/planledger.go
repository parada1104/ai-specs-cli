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

// work is the checkout's ledger and the plan identity its rows are keyed by, in the
// WRITE form: it mints the checkout's creation token so a recorded decision has a
// durable identity. Every read path uses readWork instead. The key is the common dir
// plus the worktree root: a declaration belongs to the worktree, so a renamed branch
// neither loses it nor brings it back, and the branch travels as data (R1-001).
func work(root string) (ledger.StoreLocation, ledger.PlanIdentity, error) {
	return workAt(root, true)
}

// readWork is the READ form of the same handle: the creation token is read and never
// written, so a report leaves the git admin directory byte-identical (T7/L5-6).
func readWork(root string) (ledger.StoreLocation, ledger.PlanIdentity, error) {
	return workAt(root, false)
}

func workAt(root string, mint bool) (ledger.StoreLocation, ledger.PlanIdentity, error) {
	identFn := ledger.PlanIdentityReadAt
	if mint {
		identFn = ledger.PlanIdentityAt
	}
	ident := identFn(root, nil)
	if ident.CommonDir == "" || ident.Root == "" {
		return ledger.StoreLocation{}, ledger.PlanIdentity{},
			errors.New("plan ledger: cannot name this checkout (no Git common dir)")
	}
	return ledger.LedgerAt(ident.CommonDir).WithPlanSource(File(root), ident), ident, nil
}

// Store is the ledger file the plan section lives in, for the messages a verb prints.
// It needs no branch, so a repository without a commit still names its ledger. It is a
// read-form identity too: naming the ledger never mints a token.
func Store(root string) string {
	ident := ledger.PlanIdentityReadAt(root, nil)
	if ident.CommonDir == "" {
		return ""
	}
	return ledger.StorePath(ident.CommonDir)
}

// Ledger is the CLI's read handle on this checkout's ledger: the file every section
// answers from, the report of what reaching it took, and the plan identity its rows are
// keyed by. One load answers all three capabilities, so a composite report can never
// disagree with itself about what the ledger holds.
type Ledger struct {
	File   ledger.LedgerFile
	Report ledger.StoreReport
	Ident  ledger.PlanIdentity
	Path   string
}

// Load reads the whole ledger once, in the read form: it never mints the creation token,
// never writes and never takes the lock.
func Load(root string) (Ledger, error) {
	loc, ident, err := readWork(root)
	if err != nil {
		return Ledger{}, err
	}
	file, report, err := loc.Load()
	if err != nil {
		return Ledger{}, err
	}
	return Ledger{File: file, Report: report, Ident: ident, Path: loc.Path}, nil
}

// Append records one decision in the plan section, and reports what reaching the section
// had to do beyond the write — a predecessor absorbed, one that could not be read, a copy
// that could not be kept. Every failure persists nothing, so the verb that records a
// declaration can exit non-zero instead of claiming success.
func Append(root string, event Event) (ledger.StoreReport, error) {
	if event.Event == "" {
		return ledger.StoreReport{}, errors.New("plan ledger: the event name is empty")
	}
	if event.At == "" {
		return ledger.StoreReport{}, errors.New("plan ledger: the event time is empty")
	}
	loc, ident, err := work(root)
	if err != nil {
		return ledger.StoreReport{}, err
	}
	return ledger.RecordPlanDecision(loc, ident, ledger.PlanDecision{
		At: event.At, Kind: event.Event, Reason: event.Reason,
	})
}

// Declaration is what the plan section says about this checkout, with the report of what
// reaching it took: rows and opt-outs absorbed from a predecessor, and a source that could
// not be read. The report is the verb's to print, so an unreadable predecessor is never
// silent to the human running the command.
type Declaration struct {
	Declared bool
	Reason   string
	Report   ledger.StoreReport
}

// Read is the verb's contract. The error means the section could not be read at all,
// which the verb prints instead of reporting "no declaration" out of a file nobody
// opened.
func Read(root string) (Declaration, error) {
	loaded, err := Load(root)
	if err != nil {
		return Declaration{}, err
	}
	declared, reason := loaded.File.Plan.Workstream(loaded.Ident.Key())
	return Declaration{Declared: declared, Reason: reason, Report: loaded.Report}, nil
}

// Workstream is the facts contract the write gate reads: the declaration and why the
// ledger could not answer it. The error is never swallowed there either — a corrupt shared
// ledger would otherwise look like "this worktree is a change" in every worktree of the
// clone — and the report is left to the verb, which is where a human reads it.
func Workstream(root string) (declared bool, reason string, err error) {
	declaration, err := Read(root)
	if err != nil {
		return false, "", err
	}
	return declaration.Declared, declaration.Reason, nil
}
