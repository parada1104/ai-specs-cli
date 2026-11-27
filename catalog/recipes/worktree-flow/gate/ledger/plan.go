package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// The plan ledger: the change's own flow (S3). It owns whether this worktree is a
// workstream at all, and later its exemption and archive decisions. Nothing here
// writes a lifecycle row or a tracker record, and nothing outside this file writes
// the plan section.

const (
	// PlanKindWorkstream and PlanKindWorkstreamEnd are the declaration and its end.
	// A later unit adds the plan exemption and the archive decision; a reader skips
	// a kind it does not know.
	PlanKindWorkstream    = "workstream"
	PlanKindWorkstreamEnd = "workstream-end"
)

// PlanIdentity is a plan row's key: the checkout, not the branch. A workstream
// declaration belongs to the worktree it was made in, so renaming the branch must not
// lose its end and must not resurrect it; the branch travels as data so a report can
// still name it (Cody's R1-001 decision, option A).
type PlanIdentity struct {
	CommonDir string `json:"common_dir"`
	Root      string `json:"root"`
	Branch    string `json:"branch,omitempty"`
}

// Key is the row's matching key: common dir + worktree root, never the branch.
func (i PlanIdentity) Key() string { return i.CommonDir + "\x1f" + i.Root }

// PlanIdentityAt derives the plan key for the checkout containing dir. Unlike the item
// identity it needs no commit, so a declaration can be recorded before the branch has
// one; the branch is recorded when git can name it and left empty otherwise.
func PlanIdentityAt(dir string, facts Facts) PlanIdentity {
	if facts == nil {
		facts = localFacts
	}
	root := facts(dir, "rev-parse", "--show-toplevel")
	if root == "" {
		root = dir
	}
	return PlanIdentity{
		CommonDir: realPath(commonDir(dir, facts)),
		Root:      realPath(root),
		Branch:    facts(dir, "symbolic-ref", "--quiet", "--short", "HEAD"),
	}
}

// PlanSection is the plan section: one row per work, in stored order.
type PlanSection struct {
	Items []PlanItem `json:"items"`
}

// PlanItem is one work's plan decisions.
type PlanItem struct {
	Identity  PlanIdentity   `json:"identity"`
	Decisions []PlanDecision `json:"decisions"`
}

// PlanDecision is one plan-flow entry. `kind` names the decision; a workstream
// declaration carries the reason a human gave, the other kinds their choice and note.
type PlanDecision struct {
	At         string `json:"at"`
	Kind       string `json:"kind"`
	Checkpoint string `json:"checkpoint,omitempty"`
	Choice     string `json:"choice,omitempty"`
	Note       string `json:"note,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Workstream is the standing declaration for key: the last workstream or
// workstream-end decision wins, and a kind this build does not know is skipped. A
// missing row is no declaration.
func (s PlanSection) Workstream(key string) (declared bool, reason string) {
	if key == "" {
		return false, ""
	}
	for _, item := range s.Items {
		if item.Identity.Key() != key {
			continue
		}
		for _, decision := range item.Decisions {
			switch decision.Kind {
			case PlanKindWorkstream:
				declared, reason = true, decision.Reason
			case PlanKindWorkstreamEnd:
				declared, reason = false, ""
			}
		}
	}
	return declared, reason
}

// row returns the index of key's row and whether it exists.
func (s PlanSection) row(key string) (int, bool) {
	for i, item := range s.Items {
		if item.Identity.Key() == key {
			return i, true
		}
	}
	return -1, false
}

// appendDecision appends one decision to key's row, creating the row on first use.
func (s *PlanSection) appendDecision(ident PlanIdentity, decision PlanDecision) {
	if at, ok := s.row(ident.Key()); ok {
		s.Items[at].Decisions = append(s.Items[at].Decisions, decision)
		return
	}
	s.Items = append(s.Items, PlanItem{Identity: ident, Decisions: []PlanDecision{decision}})
}

// RecordPlanDecision appends one plan-flow decision to the plan section under the
// ledger lock and writes the file once. Every failure persists nothing: a checkout
// git cannot name, a corrupt ledger and a predecessor that cannot be backed up all
// fail closed, so the verb that records a declaration can exit non-zero instead of
// pretending it was recorded.
func RecordPlanDecision(loc StoreLocation, ident PlanIdentity, decision PlanDecision) (StoreReport, error) {
	if ident.CommonDir == "" || ident.Root == "" {
		return StoreReport{}, fmt.Errorf("%w: the checkout cannot be named", ErrInvalidWrite)
	}
	if decision.At == "" || decision.Kind == "" {
		return StoreReport{}, fmt.Errorf("%w: a plan decision needs an at and a kind", ErrInvalidWrite)
	}
	var report StoreReport
	err := withStoreLock(loc.Path, func() error {
		file, loadReport, err := loc.Load()
		report = loadReport
		if err != nil {
			return err
		}
		file.Plan.appendDecision(ident, decision)
		saveReport, err := loc.Save(file, SectionPlan)
		report.BackupFailed = saveReport.BackupFailed
		return err
	})
	return report, err
}

// PlanSource is a checkout's plan-ledger predecessor: the append-only line file one
// build wrote before this section existed, plus the identity of the checkout that owns
// it, so its events seed exactly that row and no sibling inherits them.
type PlanSource struct {
	Path     string
	Identity PlanIdentity
}

// WithPlanSource attaches the checkout's plan-ledger predecessor: the file whose
// events seed the plan section of this checkout on load.
func (l StoreLocation) WithPlanSource(path string, ident PlanIdentity) StoreLocation {
	l.Plan = PlanSource{Path: path, Identity: ident}
	return l
}

// absorbPlanSource seeds the plan section from the checkout's JSONL predecessor: one
// PlanDecision per event, in file order, into the row of the checkout that owns the
// file. The row answers when it already holds decisions, and is filled when it exists
// but is empty, so no event is lost and no superseded decision comes back (L7). An
// absent predecessor is ordinary absence; a source that exists but cannot be read, or
// that is not a regular file, is reported and the ledger still answers (L17 MEDIO-3).
func (l StoreLocation) absorbPlanSource(file *LedgerFile, report *StoreReport) {
	ident := l.Plan.Identity
	l.rekeyPlanRows(file, ident)
	if l.Plan.Path == "" || ident.CommonDir == "" || ident.Root == "" {
		return
	}
	// A row that already holds decisions is the answer; only a missing or empty one
	// takes the predecessor's events. The predecessor is still inspected below, so an
	// unreadable or damaged one is reported either way.
	at, hasRow := file.Plan.row(ident.Key())
	info, err := os.Stat(l.Plan.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	case !info.Mode().IsRegular():
		// Reported without being read: a directory or a FIFO holds no events, and
		// reading one could block the whole invocation.
		report.LegacyUnreadable = fmt.Errorf("%s is not a regular file", l.Plan.Path)
		report.LegacyRows = l.Plan.Path
		return
	}
	data, err := os.ReadFile(l.Plan.Path)
	if err != nil {
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	}
	seeded, unreadable := decodePlanEvents(data)
	if unreadable > 0 {
		// The read stays lenient — an interrupted run must not block the ledger — but
		// a line nobody could read is said out loud instead of swallowed (R1-002).
		report.LegacyUnreadable = fmt.Errorf("%s: %d line(s) could not be read", l.Plan.Path, unreadable)
		report.LegacyRows = l.Plan.Path
	}
	if hasRow && len(file.Plan.Items[at].Decisions) > 0 {
		return
	}
	if len(seeded) == 0 {
		return
	}
	if hasRow {
		file.Plan.Items[at].Decisions = append(file.Plan.Items[at].Decisions, seeded...)
	} else {
		file.Plan.Items = append(file.Plan.Items, PlanItem{Identity: ident, Decisions: seeded})
	}
	report.Absorbed += len(seeded)
	if report.LegacyRows == "" {
		report.LegacyRows = l.Plan.Path
	}
}

// rekeyPlanRows adopts a row this checkout inherited, and refreshes the branch it
// carries as data. Three cases, and a row of another live checkout is never touched:
//   - the same checkout under another branch name: the key already matches, so only
//     the branch data is refreshed (R1-001);
//   - the checkout moved: its old root no longer exists, so the row is rekeyed to the
//     root it answers for now;
//   - a row written before the key moved to the checkout carries no root, and is
//     adopted by the checkout whose common dir and branch it names.
func (l StoreLocation) rekeyPlanRows(file *LedgerFile, ident PlanIdentity) {
	if ident.CommonDir == "" || ident.Root == "" {
		return
	}
	for i := range file.Plan.Items {
		row := &file.Plan.Items[i]
		if row.Identity.CommonDir != ident.CommonDir {
			continue
		}
		if row.Identity.Key() == ident.Key() {
			row.Identity.Branch = ident.Branch
			continue
		}
		if row.Identity.Root == "" {
			if row.Identity.Branch == ident.Branch {
				row.Identity = ident
			}
			continue
		}
		if info, err := os.Stat(row.Identity.Root); err == nil && info.IsDir() {
			continue
		}
		if row.Identity.Branch == ident.Branch {
			row.Identity = ident
		}
	}
}

// decodePlanEvents reads the predecessor's lines. A line that does not parse, or that
// names no event, is not an event: the count of those is returned so the caller can
// report it. Everything else is kept in file order, including an event without a
// timestamp, which the predecessor's own reader always honoured.
func decodePlanEvents(data []byte) (seeded []PlanDecision, unreadable int) {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event planEvent
		if json.Unmarshal([]byte(line), &event) != nil || event.Event == "" {
			unreadable++
			continue
		}
		seeded = append(seeded, PlanDecision{At: event.At, Kind: event.Event, Reason: event.Reason})
	}
	return seeded, unreadable
}

// planEvent is one line of the JSONL predecessor.
type planEvent struct {
	Event  string `json:"event"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}
