package ledger

import (
	"errors"
	"time"
)

// Checkpoints are the five lifecycle points the ledger grades. The same
// predicate answers every one of them (design "Mode and checkpoint posture").
const (
	CheckpointWorkStart    = "work-start"
	CheckpointApplyStart   = "apply-start"
	CheckpointPRReview     = "pr-review"
	CheckpointPreMerge     = "pre-merge"
	CheckpointArchiveClose = "archive-close"
)

// Modes are the three project postures (A9/D18). There is no fourth mode; the
// tracker gate's off/warn/always vocabulary maps forward at the host, never here.
const (
	ModeAlways = "always"
	ModeAsk    = "ask"
	ModeWarn   = "warn"
)

// Decisions are the only outcomes a verdict may carry.
const (
	DecisionAllow       = "allow"
	DecisionBlock       = "block"
	DecisionAsk         = "ask"
	DecisionDormant     = "dormant"
	DecisionUnevaluable = "unevaluable"
)

// Reasons explain a non-allow verdict. An empty reason means "consistent".
const (
	ReasonNeedsItem         = "needs-item"
	ReasonConflict          = "conflict"
	ReasonExempt            = "exempt"
	ReasonOptOut            = "opt-out"
	ReasonAdjudicated       = "adjudicated"
	ReasonUnknownCheckpoint = "unknown-checkpoint"
)

// CheckName is the single doctor check that owns ledger visibility (A10/D15).
const CheckName = "tracker-ledger"

// Doctor severities (A10).
const (
	SeverityOK    = "OK"
	SeverityInfo  = "INFO"
	SeverityWarn  = "WARN"
	SeverityError = "ERROR"
)

// Checkpoints is the ordered checkpoint vocabulary.
var Checkpoints = []string{
	CheckpointWorkStart, CheckpointApplyStart, CheckpointPRReview, CheckpointPreMerge, CheckpointArchiveClose,
}

// LedgerModes is the ordered mode vocabulary.
var LedgerModes = []string{ModeAlways, ModeAsk, ModeWarn}

// ValidCheckpoint reports whether c is one of the five checkpoints.
func ValidCheckpoint(c string) bool { return contains(Checkpoints, c) }

// ValidMode reports whether m is one of the three ledger modes.
func ValidMode(m string) bool { return contains(LedgerModes, m) }

// NormalizeMode resolves the effective mode. An unset or unknown value is warn,
// which is the warn-first adoption posture (A9/D18): promotion is a human edit.
func NormalizeMode(m string) string {
	if ValidMode(m) {
		return m
	}
	return ModeWarn
}

// Evidence is the four-sided reconciliation input (A6/D16). An empty string
// means the side is unavailable; unavailable sides are never a winner.
type Evidence struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
	Code   string `json:"code"`
	Git    string `json:"git"`
}

// Conflict reports whether any available side disagrees with the ledger
// snapshot. Empty sides never conflict, and no side is preferred.
func (e Evidence) Conflict() bool {
	for _, side := range []string{e.Remote, e.Code, e.Git} {
		if side != "" && side != e.Local {
			return true
		}
	}
	return false
}

// Conflict is the snapshot of a current unresolved disagreement (A6). It is the
// evidence the host prints; resolution is a persisted human decision, never a
// choice made here.
type Conflict struct {
	Sides      Evidence `json:"sides"`
	RecordedAt string   `json:"recorded_at"`
}

// Prompt is the human question an ask verdict carries: the reason, the four
// sides, and the legal answers. The host prints it and persists the answer with
// --decide.
type Prompt struct {
	Reason   string   `json:"reason"`
	Evidence Evidence `json:"evidence"`
	Choices  []string `json:"choices"`
}

// DoctorFinding is the ledger's visibility record (A10). Ledger mode never
// changes it; it exists so the doctor host can render without a second grader.
type DoctorFinding struct {
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Message  string `json:"message"`
}

// Input is everything the pure predicate needs. Loaders live in the dispatcher;
// Grade itself performs no IO.
type Input struct {
	Checkpoint string
	Mode       string
	Binding    Binding
	Identity   Identity
	Store      Store
	StoreErr   error
	Evidence   Evidence
	Now        time.Time
	// ReportItem is an explicit row to evaluate when the identity has no open
	// primary. It is set only by the explicit close write path, whose own row is
	// closed and therefore never primary (D17); a plain grade leaves it nil and
	// still reports needs-item for a closed-only store.
	ReportItem *Item
}

// Verdict is the checkpoint outcome. Decision is the only field a host maps to
// an exit code; Conflict and Prompt carry the evidence for the human.
type Verdict struct {
	Checkpoint string
	Mode       string
	Decision   string
	Reason     string
	Active     bool
	Identity   Identity
	Item       *Item
	Conflict   *Conflict
	Prompt     *Prompt
	Doctor     DoctorFinding
}

// ExitCode is the host contract: 2 only when the host must stop.
func (v Verdict) ExitCode() int {
	if v.Decision == DecisionBlock {
		return 2
	}
	return 0
}

// Grade evaluates one checkpoint. It is pure: all IO happened before the call.
// Order matters — dormancy and infrastructure failures short-circuit before any
// item is selected, and no path ever guesses a provider or a winning side.
func Grade(in Input) Verdict {
	v := Verdict{
		Checkpoint: in.Checkpoint,
		Mode:       NormalizeMode(in.Mode),
		Identity:   in.Identity,
	}

	if !ValidCheckpoint(in.Checkpoint) {
		v.Decision = DecisionUnevaluable
		v.Reason = ReasonUnknownCheckpoint
		v.Doctor = DoctorFinding{Severity: SeverityError, Name: CheckName, Message: "unknown checkpoint: " + in.Checkpoint}
		return v
	}

	if !in.Binding.Active() {
		v.Decision = DecisionDormant
		v.Reason = dormantReason(in.Binding)
		v.Doctor = bindingDoctor(in.Binding)
		return v
	}
	v.Active = true

	if in.StoreErr != nil {
		// A corrupt store is infrastructure, not a merge decision: unevaluable
		// fails open at every checkpoint and is surfaced by doctor (A5 spec).
		v.Decision = DecisionUnevaluable
		v.Reason = ReasonStoreCorrupt
		v.Doctor = DoctorFinding{Severity: SeverityError, Name: CheckName, Message: "ledger store unreadable; run ai-specs sync"}
		return v
	}

	if !in.Identity.Available() {
		v.outcome(in.Evidence, ReasonIdentityUnavailable)
		v.Doctor = okayDoctor()
		return v
	}

	ev := in.Evidence
	item, err := in.Store.Primary(in.Identity.Key)
	if errors.Is(err, ErrNoPrimary) && in.ReportItem != nil {
		// An explicit close write reports the row it just closed: the row stays
		// non-primary (D17), but this one invocation must grade its own observation.
		item, err = *in.ReportItem, nil
	}
	switch {
	case errors.Is(err, ErrMultipleOpen):
		// Two open rows are a conflict for a human, never a silent pick (A5).
		v.Conflict = newConflict(ev, in.Now)
		v.outcome(ev, ReasonConflict)
	case errors.Is(err, ErrNoPrimary):
		if in.Store.HasScopedOptOut(in.Identity.Key, in.Checkpoint) {
			// A fresh binding answered the ask prompt with an explicit opt-out
			// (A5): the decline is remembered for the identity/change, so every
			// checkpoint of this lifecycle proceeds. No item exists, so nothing
			// else can be satisfied implicitly.
			v.Decision, v.Reason = DecisionAllow, ReasonOptOut
		} else {
			v.outcome(ev, ReasonNeedsItem)
		}
	case err != nil:
		v.outcome(ev, ReasonNeedsItem)
	default:
		if ev.Local == "" {
			// The local side is the ledger's own snapshot when the host did not
			// override it (design evidence table).
			ev.Local = item.ItemID
		}
		v.Item = &item
		v.resolve(item, ev, in)
	}
	v.Doctor = verdictDoctor(v)
	return v
}

// resolve grades a selected primary item. A recorded exemption, an opt-out, or
// an adjudication for this checkpoint settles the item; a row that is not
// provider-backed is a needs-item outcome; otherwise a disagreement is a
// conflict.
func (v *Verdict) resolve(item Item, ev Evidence, in Input) {
	switch {
	case item.Exemption != "":
		v.Decision, v.Reason = DecisionAllow, ReasonExempt
	case hasOptOut(item, in.Checkpoint):
		v.Decision, v.Reason = DecisionAllow, ReasonOptOut
	case hasDecision(item, DecisionAdjudicate, in.Checkpoint):
		v.Decision, v.Reason = DecisionAllow, ReasonAdjudicated
	case !item.ProviderBacked():
		// A local-only row (opened or linked without a provider item) is never
		// compliant: the lifecycle requires a real provider item, so the missing
		// link is a needs-item outcome under the usual mode posture. The
		// exemption and opt-out cases above are explicit human answers and
		// therefore still settle the row.
		v.outcome(ev, ReasonNeedsItem)
	case ev.Conflict():
		v.Conflict = newConflict(ev, in.Now)
		v.outcome(ev, ReasonConflict)
	default:
		v.Decision = DecisionAllow
	}
}

// outcome applies the mode posture to a reason: always blocks, ask prompts
// (exit 0), warn reports and proceeds.
func (v *Verdict) outcome(ev Evidence, reason string) {
	v.Reason = reason
	switch v.Mode {
	case ModeAlways:
		v.Decision = DecisionBlock
	case ModeAsk:
		v.Decision = DecisionAsk
	default:
		v.Decision = DecisionAllow
	}
	if v.Decision == DecisionAsk {
		v.Prompt = &Prompt{Reason: reason, Evidence: ev, Choices: promptChoices(reason)}
	}
}

// promptChoices lists the legal answers for an ask prompt.
func promptChoices(reason string) []string {
	switch reason {
	case ReasonConflict:
		return append([]string{}, DecisionChoices...)
	case ReasonNeedsItem:
		return []string{"exempt", "continue"}
	default:
		return []string{"continue"}
	}
}

// rfc3339Stamp formats a clock as RFC3339 UTC. A zero clock defaults to now so
// a direct caller can never emit an empty or year-zero timestamp.
func rfc3339Stamp(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC().Format(time.RFC3339)
}

// newConflict stamps a conflict snapshot, defaulting to now for direct callers.
func newConflict(ev Evidence, now time.Time) *Conflict {
	return &Conflict{Sides: ev, RecordedAt: rfc3339Stamp(now)}
}

// hasDecision reports whether the item recorded kind at checkpoint. An
// adjudication resolves the conflict of one checkpoint only.
func hasDecision(item Item, kind, checkpoint string) bool {
	for _, d := range item.Decisions {
		if d.Kind == kind && d.Checkpoint == checkpoint {
			return true
		}
	}
	return false
}

// hasOptOut reports whether the item carries an opt-out covering checkpoint. A
// current opt-out is lifecycle-scoped and suppresses the remaining checkpoints
// of the identity/change; a legacy scope-less record covers only its own
// checkpoint (A5).
func hasOptOut(item Item, checkpoint string) bool {
	for _, d := range item.Decisions {
		if d.Kind == DecisionOptOut && optOutCovers(d.Scope, d.Checkpoint, checkpoint) {
			return true
		}
	}
	return false
}

// dormantReason is the reason carried by an inactive ledger.
func dormantReason(b Binding) string {
	if b.Reason != "" {
		return b.Reason
	}
	return b.State
}

// bindingDoctor maps the witness state to the A10 severity.
func bindingDoctor(b Binding) DoctorFinding {
	switch {
	case b.Reason == ReasonWitnessMissing:
		return DoctorFinding{Severity: SeverityWarn, Name: CheckName, Message: "witness missing; run ai-specs sync"}
	case b.State == WitnessAmbiguous:
		return DoctorFinding{Severity: SeverityWarn, Name: CheckName, Message: `ambiguous tracker binding; add [[bindings]] capability="tracker"`}
	case b.State == WitnessDeclaredNotBound:
		return DoctorFinding{Severity: SeverityWarn, Name: CheckName, Message: "tracking is declared but no tracker recipe is bound"}
	default:
		return DoctorFinding{Severity: SeverityInfo, Name: CheckName, Message: "no tracker recipe bound; enable one or ignore"}
	}
}

// verdictDoctor maps the graded state to the A10 severity. Warn never blocks,
// but it must not report OK either: a missing provider-backed requirement is
// advisory-visible at the doctor surface.
func verdictDoctor(v Verdict) DoctorFinding {
	if v.Conflict != nil {
		return DoctorFinding{Severity: SeverityWarn, Name: CheckName, Message: "conflict recorded; adjudicate at the next checkpoint"}
	}
	if v.Reason == ReasonNeedsItem {
		return DoctorFinding{Severity: SeverityWarn, Name: CheckName, Message: "no provider item linked; create or bind one to satisfy tracker tracking"}
	}
	return okayDoctor()
}

// okayDoctor is the bound, conflict-free finding.
func okayDoctor() DoctorFinding {
	return DoctorFinding{Severity: SeverityOK, Name: CheckName}
}

// contains reports whether list holds want.
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
