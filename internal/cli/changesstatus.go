package cli

// `ai-specs changes status --root <repo-root> [--json] [--explain]
// [--ledger worktree|plan|tracker]` (change-ledger-v2 T7, S3/S4/S12/S14/S16).
//
// The verb is the READ-ONLY composite the `changes` family was missing: it collects
// one fact set per capability — the worktree (identity, base, divergence, merge proof,
// the lifecycle row), the plan (tier, artifacts, the standing declaration) and the
// tracker (row, card, exemption, conflict, opt-outs) — consumes the review receipt
// gentle-ai recorded for the candidate, and derives the phase from those facts alone.
//
// Two invariants shape every line below. A ledger is not mixed with another (S3): each
// capability's facts come from its own section and none is written. And no fact is
// defaulted (T7.1): what git or a ledger cannot answer is `unknown` with the reason,
// never false, because a status verb that lies is worse than no status verb at all.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/ai-specs/internal/gates"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/planledger"
	"ai-specs.dev/ai-specs/internal/receipts"
	"ai-specs.dev/worktree-gate/ledger"
	"ai-specs.dev/worktree-gate/runner"
)

// The phase vocabulary of L5 item 1. A phase is derived, never stored: nothing in a
// ledger carries a writable status field that could disagree with the facts.
const (
	statusWorkstream = "workstream"
	statusOpen       = "open"
	statusPlanned    = "planned"
	statusMerged     = "merged"
	statusCleaned    = "cleaned"
	statusUnknown    = "unknown"
)

// changesStatusUsage is the verb's own usage.
const changesStatusUsage = `Usage: ai-specs changes status --root <repo-root> [--json] [--explain] [--ledger worktree|plan|tracker]

Report what this worktree is, what it owes and what it can do next, composed from the
three capability ledgers and the review receipt. It is read-only: it writes no ledger
file, no backup, no creation token and no declaration.

The phase is derived from the facts, first match wins: cleaned (the lifecycle row is
closed and none is open) -> merged (the local merge proof the cleanup owns holds and the
work is real) -> planned (a plan or a delta) -> open (a change with neither) -> workstream
(no artifact and no product-path divergence, so it owes nothing). A squash merge the
local proof cannot see stays planned or open, and the next line says so.

Exit: 0 a resolved phase; 2 the facts refuse the pending write (the verdict carries
blockers); 3 a fact the answer needs is unknown, because git or a ledger could not
answer it. Usage errors keep the family's 2. The review receipt is reported, never
graded, so it never changes the exit.

Options:
  --root <path>       Resolved repository root. Required.
  --json              Print one JSON object: the phase and the per-ledger facts.
  --explain           Print every fact with its source on stderr.
  --ledger <name>     Narrow the report to one capability: worktree, plan or tracker.
  --help, -h          Show this help
`

// changesStatusArgs is the verb's parsed request.
type changesStatusArgs struct {
	root    string
	asJSON  bool
	explain bool
	ledger  string
}

// parseChangesStatus is the verb's argument contract, in the family's shape: the
// request, the exit status to stop at, and whether it stopped there.
func parseChangesStatus(args []string, stdout, stderr io.Writer) (changesStatusArgs, int, bool) {
	request := changesStatusArgs{}
	usageError := func(message string) (changesStatusArgs, int, bool) {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs changes status --help' for usage.")
		return request, 2, true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, changesStatusUsage)
			return request, 0, true
		case arg == "--root":
			if i+1 >= len(args) || optionShaped(args[i+1]) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			request.root = args[i]
		case strings.HasPrefix(arg, "--root="):
			request.root = strings.TrimPrefix(arg, "--root=")
		case arg == "--json":
			request.asJSON = true
		case arg == "--explain":
			request.explain = true
		case arg == "--ledger":
			if i+1 >= len(args) || optionShaped(args[i+1]) {
				return usageError("ERROR: --ledger requires a value")
			}
			i++
			request.ledger = args[i]
		case strings.HasPrefix(arg, "--ledger="):
			request.ledger = strings.TrimPrefix(arg, "--ledger=")
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			return usageError("ERROR: unexpected positional argument: " + arg)
		}
	}
	if request.root == "" {
		return usageError("ERROR: missing required option: --root")
	}
	switch request.ledger {
	case "", string(ledger.SectionWorktree), string(ledger.SectionPlan), string(ledger.SectionTracker):
	default:
		return usageError("ERROR: --ledger must be one of worktree, plan, tracker: " + request.ledger)
	}
	return request, 0, false
}

// statusFact is one derived fact: the value a source answered, or `unknown` with the
// reason nothing could answer it. Nothing is defaulted (T7.1) — a fact no source can
// answer is unknown, never false — and every fact names its source so `--explain` can
// show where the answer came from.
type statusFact struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Source string `json:"source"`
	Known  bool   `json:"known"`
	Reason string `json:"reason,omitempty"`
}

// knownFact is an answered fact.
func knownFact(name, value, source string) statusFact {
	return statusFact{Name: name, Value: value, Source: source, Known: true}
}

// unknownFact is an unanswered fact: the reason is why, and it is never blank.
func unknownFact(name, source, reason string) statusFact {
	return statusFact{Name: name, Source: source, Reason: reason}
}

// statusReport is the composite answer: the phase and why it won, one fact list per
// capability, the blocker lines and the unknowns the answer still needs.
type statusReport struct {
	Phase    string       `json:"phase"`
	Why      string       `json:"why"`
	Next     string       `json:"next"`
	Worktree []statusFact `json:"worktree,omitempty"`
	Plan     []statusFact `json:"plan,omitempty"`
	Tracker  []statusFact `json:"tracker,omitempty"`
	Review   []statusFact `json:"review,omitempty"`
	Blockers []string     `json:"blockers"`
	Unknown  []string     `json:"unknown"`
	// needed are the unknown facts the phase depends on: they decide exit 3 and are
	// reported in Unknown, never hidden behind a resolved-looking phase.
	needed []statusFact
	// root is the resolved root the report was read from, for the next line.
	root string
	// report is the ledger's own account of what reading it took. It is printed once on
	// stderr and never persisted (T7.4).
	report *ledger.StoreReport
	// ledgerErr is a ledger section that could not be read at all.
	ledgerErr error
}

// statusEvidence is the resolved fact set the phase is derived from: one field per
// fact of L5 item 1, so the precedence is testable without a repository.
type statusEvidence struct {
	Checkout   bool
	Base       string
	Plan       bool
	Delta      bool
	PlanPath   string
	DeltaPath  string
	Divergence []string
	// DivergenceKnown reports whether git answered the divergence. The last two phases
	// are told apart by it alone, so an unanswered divergence cannot resolve either.
	DivergenceKnown bool
	OpenRow         bool
	ClosedRow       bool
	Merged          bool
	Workstream      bool
}

// resolveStatusPhase is L5 item 1: first match wins, and nothing is guessed. A phase
// whose fact is missing reports `unknown` instead of falling through to a phase the
// facts do not support.
func resolveStatusPhase(evidence statusEvidence) (phase, why string) {
	artifacts := statusArtifacts(evidence)
	switch {
	case !evidence.Checkout:
		return statusUnknown, "git cannot name this checkout, so no fact of the worktree can be read"
	case !evidence.OpenRow && evidence.ClosedRow:
		return statusCleaned, "the lifecycle row for this branch is closed and none is open, so the work is cleaned"
	case evidence.Merged && (evidence.Plan || evidence.Delta || evidence.OpenRow):
		return statusMerged, "the local merge proof holds against " + evidence.Base +
			": the branch tip's work is in the base, and the work is real (a plan, a delta or an open row)"
	case evidence.Plan || evidence.Delta:
		return statusPlanned, "the worktree carries " + artifacts
	case !evidence.DivergenceKnown:
		return statusUnknown, "git could not answer the divergence of this branch, so a change cannot " +
			"be told apart from a workstream"
	case !evidence.Workstream:
		return statusOpen, "product paths diverge from " + evidence.Base +
			" and the worktree carries no plan and no delta yet"
	}
	return statusWorkstream, "no plan, no delta and no product-path divergence from " + evidence.Base
}

// statusArtifacts names the artifacts a change carries, so a phase line always says
// what decided it.
func statusArtifacts(evidence statusEvidence) string {
	switch {
	case evidence.Plan && evidence.Delta:
		return "a plan at " + evidence.PlanPath + " and a delta at " + evidence.DeltaPath
	case evidence.Plan:
		return "a plan at " + evidence.PlanPath
	}
	return "a delta at " + evidence.DeltaPath
}

// statusNext is the next step the phase implies, in the verb a human can run.
func statusNext(report statusReport) string {
	switch report.Phase {
	case statusCleaned:
		return "nothing — the work is closed"
	case statusMerged:
		return "ai-specs worktree-cleanup"
	case statusPlanned, statusOpen:
		if mergeProof := statusFind(report.Worktree, "merge-proof"); mergeProof != nil &&
			mergeProof.Known && mergeProof.Value == "not merged" {
			// L5 item 1: a squash or rebase merge the local proof cannot see stays here,
			// and the line says so instead of guessing it merged.
			return "ai-specs gates premerge --root " + report.root +
				" (no local merge proof: a squash or rebase merge the proof cannot see also stays here)"
		}
		return "ai-specs gates premerge --root " + report.root
	case statusWorkstream:
		return "nothing owed — when product paths change, this becomes a change"
	}
	return "resolve the unknown facts this report names"
}

// statusExitCode is L5 item 2: 0 a resolved phase, 3 an unknown fact the answer needs,
// 2 the facts refusing the requested step. The review receipt is reported, never
// graded, so it is not part of this decision.
func statusExitCode(report statusReport) int {
	if len(report.needed) > 0 {
		return 3
	}
	if len(report.Blockers) > 0 {
		return 2
	}
	return 0
}

// runChangesStatus collects the facts once, prints the report and returns its exit.
func runChangesStatus(args []string, stdout, stderr io.Writer) int {
	request, status, stop := parseChangesStatus(args, stdout, stderr)
	if stop {
		return status
	}
	report := collectStatus(request.root, request.ledger)
	report.Next = statusNext(report)

	if report.ledgerErr != nil {
		where := planledger.Store(request.root)
		if where == "" {
			where = "a repository Git cannot name"
		}
		fmt.Fprintf(stderr, "changes status: cannot read the ledger for %s: %s\n", where, report.ledgerErr)
	}
	if report.report != nil {
		// The store notes surface here, once (T7.4): a predecessor absorbed, a row that
		// differs in place, a source nobody could read, a backup that could not be kept.
		report.report.Note(stderr, "changes status")
	}
	if request.explain {
		explainStatusFacts(report, stderr)
	}
	if request.asJSON {
		encoded, err := json.Marshal(report)
		if err != nil {
			fmt.Fprintf(stderr, "changes status: cannot encode the report: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", encoded)
		return statusExitCode(report)
	}
	fmt.Fprint(stdout, statusLines(report))
	return statusExitCode(report)
}

// The fact sources. Each one names the command or the section that answered, so
// `--explain` shows provenance instead of asking for trust.
const (
	statusSourceCommonDir = "git rev-parse --path-format=absolute --git-common-dir"
	statusSourceBranch    = "git symbolic-ref --quiet --short HEAD"
	statusSourceRoot      = "git rev-parse --show-toplevel"
	statusSourceBase      = "ai-specs base resolution (manifest base_branch, then development, main, master)"
	statusSourceFacts     = "gates.WriteFactsFor, the collection the write gate grades"
	statusSourceVerdict   = "gates.WriteFacts.Verdict()"
	statusSourceReceipt   = "gentle-ai review assess --committed-only --json"
	statusSourceReceipts  = "the ledger worktree section"
	statusSourceTracker   = "the ledger tracker section"
	statusSourcePlan      = "the ledger plan section"
)

// collectStatus collects every capability's facts once. It is the read form end to end:
// the ledger is loaded without minting the creation token, no section is written and no
// backup is kept. narrow drops the sections `--ledger` did not ask for; the phase and the
// exit stay the composite's.
func collectStatus(root, narrow string) statusReport {
	// The blocker and unknown lists are always arrays, never null: a host reads them as
	// lists, and an empty list is an answer ("nothing blocks", "nothing is unknown").
	report := statusReport{root: root, Blockers: []string{}, Unknown: []string{}}

	mode := planbuild.ResolveRepo(root)
	branch := planbuild.Branch(root)
	change, _ := planbuild.ChangeFromBranch(branch)
	facts := gates.WriteFactsFor(root, mode, change, "")
	// The untracked listing is collected here, once, and written back into the collected
	// facts, so the report and the workstream predicate cannot disagree about whether git
	// answered it (L5 item 8).
	untracked, untrackedOK := planbuild.UntrackedPaths(root)
	facts.Untracked, facts.UntrackedKnown = statusProductPaths(untracked), untrackedOK

	loaded, ledgerErr := planledger.Load(root)
	if ledgerErr != nil {
		report.ledgerErr = ledgerErr
	} else {
		note := loaded.Report
		report.report = &note
	}

	view := loaded.File.View()
	openRow, closedRow := statusRows(view, loaded.Ident.CommonDir, branch)
	proof := runner.ProveMerged(root, worktreeGateGit(root, "rev-parse", "--verify", "HEAD"), facts.Base)

	evidence := statusEvidence{
		Checkout:        ledgerErr == nil,
		Base:            facts.Base,
		Plan:            facts.HasPlan,
		Delta:           facts.HasDelta,
		PlanPath:        facts.PlanPath,
		DeltaPath:       facts.DeltaPath,
		Divergence:      workstreamDivergence(facts),
		DivergenceKnown: facts.Base != "" && facts.DiffKnown && facts.UntrackedKnown,
		OpenRow:         openRow != nil,
		ClosedRow:       closedRow != nil,
		Merged:          proof.Merged,
		Workstream:      facts.Workstream,
	}
	report.Phase, report.Why = resolveStatusPhase(evidence)

	verdict := facts.Verdict()
	for _, blocker := range verdict.Blockers {
		report.Blockers = append(report.Blockers, blocker.String())
	}
	report.Worktree, report.needed = statusWorktreeFacts(facts, loaded.Ident, branch, proof, openRow, closedRow,
		ledgerErr, report.needed)
	report.Plan = statusPlanFacts(facts, verdict, loaded, ledgerErr, report.needed)
	report.Tracker = statusTrackerFacts(view, openRow, closedRow)
	for _, needed := range report.needed {
		report.Unknown = append(report.Unknown, needed.Name+": "+needed.Reason)
	}
	// The receipt is a report, not a grade (S12/L5 item 2): it is read only when it is
	// shown, and it never changes the exit code.
	if narrow == "" {
		report.Review = statusReviewFacts(root, worktreeGateGit(root, "rev-parse", "--verify", facts.Base))
	}
	if narrow != "" {
		report.Worktree, report.Plan, report.Tracker = statusNarrow(narrow, report)
	}
	return report
}

// statusNarrow keeps the one capability `--ledger` named: the phase and the exit are the
// composite's, the facts are the narrowed capability's.
func statusNarrow(narrow string, report statusReport) (worktree, plan, tracker []statusFact) {
	switch narrow {
	case string(ledger.SectionWorktree):
		return report.Worktree, nil, nil
	case string(ledger.SectionPlan):
		return nil, report.Plan, nil
	case string(ledger.SectionTracker):
		return nil, nil, report.Tracker
	}
	return report.Worktree, report.Plan, report.Tracker
}

// statusProductPaths keeps the listing's production paths: divergence is product work,
// and an untracked note or a scratch file is not a change.
func statusProductPaths(paths []string) []string {
	var product []string
	for _, path := range paths {
		if gates.ProductPath(path) {
			product = append(product, path)
		}
	}
	return product
}

// statusRows resolves this branch's lifecycle rows from the worktree section. The match is
// common dir + branch — the pair cleanup closes on — so the report and the close can never
// select different rows.
func statusRows(view ledger.View, commonDir, branch string) (open, closed *ledger.Item) {
	if commonDir == "" || branch == "" {
		return nil, nil
	}
	if items := view.OpenItemsForBranch(commonDir, branch); len(items) > 0 {
		first := items[0]
		open = &first
	}
	closedItems := view.ClosedItems()
	for i := len(closedItems) - 1; i >= 0; i-- {
		candidate := closedItems[i]
		if candidate.Identity.CommonDir == commonDir && candidate.Identity.Branch == branch {
			closed = &candidate
			break
		}
	}
	return open, closed
}

// statusWorktreeFacts is the worktree capability's fact set, plus the unknowns the phase
// needs. The identity is answered by the plan port's read-form identity: the same checkout
// git names, with no token minted to name it.
func statusWorktreeFacts(facts gates.WriteFacts, ident ledger.PlanIdentity, branch string,
	proof runner.MergeProof, openRow, closedRow *ledger.Item, ledgerErr error,
	needed []statusFact) ([]statusFact, []statusFact) {
	commonDir := statusIdentityFact("common-dir", ident.CommonDir, statusSourceCommonDir,
		"git cannot name this checkout: the root is not a Git repository or a worktree of one")
	rootFact := statusIdentityFact("root", ident.Root, statusSourceRoot,
		"git cannot name the worktree root of this checkout")
	branchFact := unknownFact("branch", statusSourceBranch,
		"HEAD is detached, so this checkout pairs no branch")
	if branch != "" {
		branchFact = knownFact("branch", branch, statusSourceBranch)
	} else if ledgerErr != nil {
		branchFact = unknownFact("branch", statusSourceBranch,
			"git cannot name a branch of this root: it is not a Git repository or a worktree of one")
	}
	if ledgerErr != nil {
		// One unknown names the checkout itself; the identity facts above carry the same
		// reason, and the phase cannot resolve without it.
		needed = append(needed, unknownFact("checkout", statusSourceCommonDir,
			"git cannot name this checkout: "+ledgerErr.Error()))
	}

	baseFact := knownFact("base", facts.Base, statusSourceBase)
	if facts.Base == "" {
		reason := "no base ref resolves in this root: fetch the integration branch or declare base_branch"
		switch {
		case ledgerErr != nil:
			reason = "no base ref resolves: this root is not a Git repository or a worktree of one"
		case facts.Unborn:
			reason = "the branch has no commit yet, so there is no base to compare against"
		}
		baseFact = unknownFact("base", statusSourceBase, reason)
		needed = append(needed, baseFact)
	}

	divergence := knownFact("divergence", statusJoin(workstreamDivergence(facts)), statusSourceFacts)
	switch {
	case !facts.DiffKnown && facts.Base != "":
		divergence = unknownFact("divergence", statusSourceFacts,
			"git could not answer the diff against "+facts.Base)
		needed = append(needed, divergence)
	case facts.Base != "" && !facts.UntrackedKnown:
		divergence = unknownFact("divergence", statusSourceFacts,
			"git could not answer the untracked listing, so uncommitted product work cannot be ruled out")
		needed = append(needed, divergence)
	}

	mergeFact := unknownFact("merge-proof", statusSourceFacts,
		"no base candidate resolves, so no merge proof can be attempted")
	if len(proof.Candidates) > 0 {
		source := "the worktree-cleanup local merge proof against " + strings.Join(proof.Candidates, ", ")
		mergeFact = knownFact("merge-proof", "not merged", source)
		if proof.Merged {
			mergeFact = knownFact("merge-proof", "merged", source)
		}
	}

	rowFact := unknownFact("ledger-row", statusSourceReceipts, "the ledger could not be read")
	if ledgerErr != nil {
		needed = append(needed, rowFact)
	} else {
		// Absent is an answer, not an unknown: this branch never opened a row.
		rowFact = knownFact("ledger-row", "none", statusSourceReceipts)
		switch {
		case openRow != nil:
			rowFact = knownFact("ledger-row", "open "+openRow.ID, statusSourceReceipts)
		case closedRow != nil:
			rowFact = knownFact("ledger-row", "closed "+closedRow.ID, statusSourceReceipts)
		}
	}

	worktree := []statusFact{commonDir, branchFact, rootFact, baseFact, divergence, mergeFact, rowFact}
	return worktree, needed
}

// statusIdentityFact answers one git identity fact, naming why it could not when the
// checkout itself is what git could not name.
func statusIdentityFact(name, value, source, reason string) statusFact {
	if value == "" {
		return unknownFact(name, source, reason)
	}
	return knownFact(name, value, source)
}

// statusPlanFacts is the plan capability's fact set: the tier the artifacts infer, the
// artifacts themselves, the standing declaration and the refutations the verdict proves.
func statusPlanFacts(facts gates.WriteFacts, verdict gates.Verdict, loaded planledger.Ledger,
	ledgerErr error, needed []statusFact) []statusFact {
	tierFact := unknownFact("tier", statusSourceVerdict,
		"the branch pairs no change, so no tier can be inferred here")
	if facts.Change != "" {
		tierFact = knownFact("tier", verdict.Tier, statusSourceVerdict)
	}
	artifacts := knownFact("artifacts", "none", statusSourceFacts)
	switch {
	case facts.HasPlan && facts.HasDelta:
		artifacts = knownFact("artifacts", "plan and delta", statusSourceFacts)
	case facts.HasPlan:
		artifacts = knownFact("artifacts", "plan only", statusSourceFacts)
	case facts.HasDelta:
		artifacts = knownFact("artifacts", "delta only", statusSourceFacts)
	}
	planPath := knownFact("plan", "none", statusSourceFacts)
	if facts.HasPlan {
		planPath = knownFact("plan", facts.PlanPath, statusSourceFacts)
	}
	deltaPath := knownFact("delta", "none", statusSourceFacts)
	if facts.HasDelta {
		deltaPath = knownFact("delta", facts.DeltaPath, statusSourceFacts)
	}
	declaration := knownFact("declaration", "none", statusSourcePlan)
	if ledgerErr != nil {
		declaration = unknownFact("declaration", statusSourcePlan, "the ledger could not be read")
		needed = append(needed, declaration)
	} else if declared, reason := loaded.File.Plan.Workstream(loaded.Ident.Key()); declared {
		declaration = knownFact("declaration", "declared: "+reason, statusSourcePlan)
	}
	refutations := knownFact("refutations", "none", statusSourceVerdict)
	if rules := verdict.Rules(); rules != "none" {
		refutations = knownFact("refutations", rules, statusSourceVerdict)
	}
	return []statusFact{tierFact, artifacts, planPath, deltaPath, declaration, refutations}
}

// statusTrackerFacts is the tracker capability's fact set: the row, the card it is bound
// to, its exemption, its conflict and the archive-close opt-out. A branch with no row is
// `none`, which is an answer and not an unknown.
func statusTrackerFacts(view ledger.View, openRow, closedRow *ledger.Item) []statusFact {
	row, card := knownFact("row", "none", statusSourceTracker), knownFact("card", "none", statusSourceTracker)
	exemption := knownFact("exemption", "none", statusSourceTracker)
	conflict := knownFact("conflict", "none", statusSourceTracker)
	optOut := knownFact("opt-out", "none", statusSourceTracker)
	item := openRow
	if item == nil {
		item = closedRow
	}
	if item == nil {
		return []statusFact{row, card, exemption, conflict, optOut}
	}
	state := "closed"
	if openRow != nil {
		state = "open"
	}
	row = knownFact("row", state+" "+item.ID, statusSourceTracker)
	if item.ItemID != "" {
		card = knownFact("card", strings.TrimSpace(item.ItemID+" "+item.URL), statusSourceTracker)
	}
	if item.Exemption != "" {
		exemption = knownFact("exemption", item.Exemption, statusSourceTracker)
	}
	if trimmed := strings.TrimSpace(string(item.Conflict)); trimmed != "" && trimmed != "null" {
		conflict = knownFact("conflict", trimmed, statusSourceTracker)
	}
	covered := "no"
	if view.HasScopedOptOut(item.Key(), ledger.CheckpointArchiveClose) {
		covered = "yes"
	}
	optOut = knownFact("opt-out", covered, statusSourceTracker)
	return []statusFact{row, card, exemption, conflict, optOut}
}

// statusReviewFacts consumes the review receipt for the candidate the base SHA names
// (S12). It is reported and never graded: an unknown receipt is a fact about the
// environment, not a refusal.
func statusReviewFacts(root, baseSHA string) []statusFact {
	if baseSHA == "" {
		return []statusFact{unknownFact("receipt", statusSourceReceipt,
			"no base ref resolves in this root, so no candidate SHA can be asked for")}
	}
	assessment := receipts.Assess(root, baseSHA)
	if !assessment.Known {
		return []statusFact{unknownFact("receipt", statusSourceReceipt, assessment.Reason)}
	}
	value := fmt.Sprintf("consumed=%t", *assessment.Consumed)
	if assessment.ReviewDue != nil {
		value += fmt.Sprintf(" review_due=%t", *assessment.ReviewDue)
	}
	if assessment.ReviewDueReason != "" {
		value += " (" + assessment.ReviewDueReason + ")"
	}
	if assessment.Risk != "" {
		value += " risk=" + assessment.Risk
	}
	return []statusFact{knownFact("receipt", value, statusSourceReceipt)}
}

// statusFind returns the named fact, nil when the section was not reported.
func statusFind(facts []statusFact, name string) *statusFact {
	for i := range facts {
		if facts[i].Name == name {
			return &facts[i]
		}
	}
	return nil
}

// statusFactValue renders one fact: its value when it is known, and `unknown (reason)`
// when it is not, so no line can print a plausible value for a fact nobody answered.
func statusFactValue(facts []statusFact, name string) string {
	fact := statusFind(facts, name)
	switch {
	case fact == nil:
		return "not reported"
	case !fact.Known:
		return "unknown (" + fact.Reason + ")"
	}
	return fact.Value
}

// statusJoin renders a list fact, bounded so one large branch cannot print a page.
func statusJoin(values []string) string {
	const shown = 5
	switch {
	case len(values) == 0:
		return "none"
	case len(values) <= shown:
		return strings.Join(values, ", ")
	}
	return fmt.Sprintf("%s, +%d more", strings.Join(values[:shown], ", "), len(values)-shown)
}

// statusLines is the human report: one line per capability plus the phase, the next step
// and every blocker the facts prove.
func statusLines(report statusReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "worktree: %s, base %s, %s\n",
		statusFactValue(report.Worktree, "branch"),
		statusFactValue(report.Worktree, "base"),
		statusDivergenceLine(report.Worktree),
	)
	fmt.Fprintf(&out, "phase:    %s — %s\n", report.Phase, report.Why)
	if len(report.Plan) > 0 {
		tier := statusFactValue(report.Plan, "tier")
		if tierFact := statusFind(report.Plan, "tier"); tierFact != nil && tierFact.Known {
			artifacts := statusFactValue(report.Plan, "artifacts")
			if artifacts == "none" {
				artifacts = "no artifacts"
			}
			tier += " (" + artifacts + ")"
		}
		fmt.Fprintf(&out, "plan:     %s, refutations: %s\n", tier,
			statusFactValue(report.Plan, "refutations"))
	}
	if len(report.Tracker) > 0 {
		fmt.Fprintf(&out, "tracker:  %s\n", statusTrackerLine(report.Tracker))
	}
	if len(report.Review) > 0 {
		fmt.Fprintf(&out, "review:   %s\n", statusFactValue(report.Review, "receipt"))
	}
	fmt.Fprintf(&out, "next:     %s\n", report.Next)
	for _, blocker := range report.Blockers {
		fmt.Fprintf(&out, "blocked:  %s\n", blocker)
	}
	return out.String()
}

// statusDivergenceLine renders the divergence fact as the sentence the report shows: what
// diverges, or that nothing does.
func statusDivergenceLine(facts []statusFact) string {
	divergence := statusFactValue(facts, "divergence")
	switch divergence {
	case "none":
		return "no product path diverging"
	case "not reported":
		return divergence
	}
	return "product paths diverging: " + divergence
}

// statusTrackerLine renders the tracker capability as one line, naming only the fields
// that carry a value: a branch with no row is not-applicable, never a blocker.
func statusTrackerLine(facts []statusFact) string {
	row := statusFactValue(facts, "row")
	if row == "none" {
		return "not-applicable (no lifecycle row for this branch)"
	}
	parts := []string{row, "card: " + statusFactValue(facts, "card")}
	if exemption := statusFactValue(facts, "exemption"); exemption != "none" {
		parts = append(parts, "exempt: "+exemption)
	}
	if conflict := statusFactValue(facts, "conflict"); conflict != "none" {
		parts = append(parts, "conflict: "+conflict)
	}
	if optOut := statusFactValue(facts, "opt-out"); optOut == "yes" {
		parts = append(parts, "archive-close opt-out")
	}
	return strings.Join(parts, ", ")
}

// explainStatusFacts prints every fact with its source on stderr (L5 item 4).
func explainStatusFacts(report statusReport, stderr io.Writer) {
	sections := []struct {
		name  string
		facts []statusFact
	}{
		{string(ledger.SectionWorktree), report.Worktree},
		{string(ledger.SectionPlan), report.Plan},
		{string(ledger.SectionTracker), report.Tracker},
		{"review", report.Review},
	}
	for _, section := range sections {
		for _, fact := range section.facts {
			value := fact.Value
			if !fact.Known {
				value = "unknown — " + fact.Reason
			}
			fmt.Fprintf(stderr, "changes status: %s.%s = %s (source: %s)\n",
				section.name, fact.Name, value, fact.Source)
		}
	}
}
