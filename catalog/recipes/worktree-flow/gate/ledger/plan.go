package ledger

import (
	"encoding/json"
	"fmt"
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

// PlanSection is the plan section: one row per work, in stored order. A row is keyed
// by the work identity rather than by the change slug alone, because one ledger file
// is shared by every worktree of the clone while a declaration belongs to exactly one
// of them (L6).
type PlanSection struct {
	Items []PlanItem `json:"items"`
}

// PlanItem is one work's plan decisions.
type PlanItem struct {
	Identity  ItemIdentity   `json:"identity"`
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
func (s *PlanSection) appendDecision(ident ItemIdentity, decision PlanDecision) {
	if at, ok := s.row(ident.Key()); ok {
		s.Items[at].Decisions = append(s.Items[at].Decisions, decision)
		return
	}
	s.Items = append(s.Items, PlanItem{Identity: ident, Decisions: []PlanDecision{decision}})
}

// RecordPlanDecision appends one plan-flow decision to the plan section under the
// ledger lock and writes the file once. Every failure persists nothing: an
// unresolved identity, a corrupt ledger and a predecessor that cannot be backed up
// all fail closed, so the verb that records a declaration can exit non-zero instead
// of pretending it was recorded.
func RecordPlanDecision(loc StoreLocation, ident ItemIdentity, decision PlanDecision) (StoreReport, error) {
	if ident.CommonDir == "" || ident.Branch == "" {
		return StoreReport{}, fmt.Errorf("%w: identity unavailable", ErrInvalidWrite)
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
// build wrote before this section existed, plus the work identity of the checkout
// that owns it, so its events seed exactly that row and no sibling inherits them.
type PlanSource struct {
	Path     string
	Identity ItemIdentity
}

// absorbPlanSource seeds the plan section from the checkout's JSONL predecessor: one
// PlanDecision per event, in file order, keyed by the identity of the checkout that
// owns the file. The file answers when it already holds a row for that identity, so
// an event an older binary appended after the first v2 write never resurrects a
// decision the file superseded (L7). A source that cannot be read is reported and the
// ledger still answers (L17 MEDIO-3).
// WithPlanSource attaches the checkout's plan-ledger predecessor: the file whose
// events seed the plan section of this identity on load.
func (l StoreLocation) WithPlanSource(path string, ident ItemIdentity) StoreLocation {
	l.Plan = PlanSource{Path: path, Identity: ident}
	return l
}

func (l StoreLocation) absorbPlanSource(file *LedgerFile, report *StoreReport) {
	if l.Plan.Path == "" || l.Plan.Identity.Key() == "" {
		return
	}
	switch err := statExact(l.Plan.Path); {
	case err != nil:
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	}
	data, err := os.ReadFile(l.Plan.Path)
	if err != nil {
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	}
	if at, ok := file.Plan.row(l.Plan.Identity.Key()); ok && len(file.Plan.Items[at].Decisions) > 0 {
		return
	}
	seeded := PlanItem{Identity: l.Plan.Identity, Decisions: []PlanDecision{}}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event planEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			// A line no reader can parse is skipped, exactly as the predecessor's own
			// reader skipped it: it never was an event.
			continue
		}
		if event.Event == "" || event.At == "" {
			continue
		}
		seeded.Decisions = append(seeded.Decisions, PlanDecision{
			At: event.At, Kind: event.Event, Reason: event.Reason,
		})
	}
	if len(seeded.Decisions) == 0 {
		return
	}
	file.Plan.Items = append(file.Plan.Items, seeded)
	report.Absorbed += len(seeded.Decisions)
	report.LegacyRows = l.Plan.Path
}

// planEvent is one line of the JSONL predecessor.
type planEvent struct {
	Event  string `json:"event"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}
