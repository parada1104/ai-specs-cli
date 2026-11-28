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

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/gates"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/planledger"
	"ai-specs.dev/ai-specs/internal/receipts"
	"ai-specs.dev/worktree-gate/ledger"
	"ai-specs.dev/worktree-gate/runner"
)

// The phase vocabulary of L5 item 1, plus the integration line (L8-2). A phase is
// derived, never stored: nothing in a ledger carries a writable status field that
// could disagree with the facts.
const (
	statusWorkstream  = "workstream"
	statusOpen        = "open"
	statusPlanned     = "planned"
	statusMerged      = "merged"
	statusCleaned     = "cleaned"
	statusIntegration = "integration"
	statusUnknown     = "unknown"
)

// The posture a phase's checkpoint is graded under (T8, L5 item 6): advise is the
// default, require is the project's opt-in per checkpoint, and a phase with no
// checkpoint has no posture at all — a workstream owes nothing (S16).
const (
	statusPostureAdvise  = "advise"
	statusPostureRequire = "require"
	statusPostureNone    = "none"
)

// statusCheckpoint is the checkpoint a phase is heading to, "" when it has none.
func statusCheckpoint(phase string) string {
	switch phase {
	case statusOpen, statusPlanned:
		// Both are on their way to a merge; the phases that follow it (merged,
		// integration, cleaned) and the ones no posture governs have no checkpoint.
		return gates.CheckpointMerge
	}
	return ""
}

// statusPosture is the posture of one checkpoint: `none` when there is no checkpoint
// to govern, otherwise the project's declaration or the default `advise`.
func statusPosture(posture gates.Posture, checkpoint string) string {
	switch {
	case checkpoint == "":
		return statusPostureNone
	case posture.Requires(checkpoint):
		return statusPostureRequire
	}
	return statusPostureAdvise
}

// changesStatusUsage is the verb's own usage.
const changesStatusUsage = `Usage: ai-specs changes status --root <repo-root> [--json] [--explain] [--ledger worktree|plan|tracker]

Report what this worktree is, what it owes and what it can do next, composed from the
three capability ledgers and the review receipt. It is read-only: it writes no ledger
file, no backup, no creation token and no declaration.

The phase is derived from the facts, first match wins: unknown (git cannot name the
checkout) -> integration (this checkout is on the integration branch, so it is the
line changes merge into, not a change worktree) -> cleaned (the lifecycle row is
closed and none is open) -> merged (the local merge proof the cleanup owns holds and
the work is real) -> planned (a plan or a delta) -> open (a change with neither) ->
workstream (no artifact and no product-path divergence, so it owes nothing). A squash
merge the local proof cannot see stays planned or open, and the next line says so.

Exit: 0 a resolved phase; 2 the facts refuse the pending write (the verdict carries
blockers, or the project requires this checkpoint's posture and a merge-time rule
fires); 3 a fact the answer needs is unknown, because git or a ledger could not
answer it. Usage errors keep the family's 2. The review receipt is reported, never
graded, so it never changes the exit.

The posture is the T8 half: the checkpoint the phase heads to (open and planned head
to merge; every other phase has none), the posture it is graded under there, and the
advice it carries. Advise is the default, so the merge-time rules R3 (the size and
module threshold), R4 (a removed high-risk path) and R5 (a plan whose S# rows carry
no evidence) are printed as advice lines and never change the exit. A project that
declares require = ["merge"] in [recipes.plan-build-flow.config] of
ai-specs/ai-specs.toml turns them into blockers and exit 2. A workstream has no
checkpoint, so it is never subject to a posture.

Options:
  --root <path>       Resolved repository root. Required.
  --json              Print one JSON object: the phase, the posture and the per-ledger facts.
  --explain           Print every fact with its source on stderr.
  --ledger <name>     Narrow the report to one capability: worktree, plan or tracker.
                      The posture and the exit stay the composite's.
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
// capability, the blocker lines and the unknowns the answer still needs. The posture
// fields are the T8 half: the checkpoint the phase heads to, the posture it is graded
// under there, and the rules that only advise under the default posture.
type statusReport struct {
	Phase      string       `json:"phase"`
	Why        string       `json:"why"`
	Next       string       `json:"next"`
	Checkpoint string       `json:"checkpoint"`
	Posture    string       `json:"posture"`
	Worktree   []statusFact `json:"worktree,omitempty"`
	Plan       []statusFact `json:"plan,omitempty"`
	Tracker    []statusFact `json:"tracker,omitempty"`
	Review     []statusFact `json:"review,omitempty"`
	Blockers   []string     `json:"blockers"`
	Advice     []string     `json:"advice"`
	Unknown    []string     `json:"unknown"`
	// needed are the unknown facts the phase depends on: they decide exit 3 and are
	// reported in Unknown, never hidden behind a resolved-looking phase.
	needed []statusFact
	// root is the resolved root the report was read from, for the next line.
	root string // report is the ledger's own account of what reading it took. It is printed once on
	// stderr and never persisted (T7.4).
	report *ledger.StoreReport
	// ledgerErr is a ledger section that could not be read at all.
	ledgerErr error
	// scopeNotes names a product-scope declaration the predicate could not read. It is
	// printed once on stderr and never persisted: a scope that changed because its
	// declaration could not be read must never be silent (L8-1).
	scopeNotes []string
	// postureNotes names a posture or merge-time declaration that could not be read, in
	// the same spirit: a rule that could not run is never silent, and never a pass.
	postureNotes []string
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
	// Integration names the integration branch this checkout sits on, "" when it is
	// not one: the integration line is never a workstream (L8-2).
	Integration string
}

// resolveStatusPhase is L5 item 1: first match wins, and nothing is guessed. A phase
// whose fact is missing reports `unknown` instead of falling through to a phase the
// facts do not support.
func resolveStatusPhase(evidence statusEvidence) (phase, why string) {
	artifacts := statusArtifacts(evidence)
	switch {
	case !evidence.Checkout:
		return statusUnknown, "git cannot name this checkout, so no fact of the worktree can be read"
	case evidence.Integration != "":
		return statusIntegration, "this checkout is on the integration branch " + evidence.Integration +
			", the line changes merge into, so it is not a change worktree"
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
	case statusIntegration:
		// The cleanup runs from the main checkout; from the integration line itself it
		// is the one step left.
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

	relayStatusNotes(report, stderr, "changes status")
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
	// statusSourceTrackerRecipe names the two declarations the tracker capability's
	// provider comes from: the witness sync wrote and the manifest's own enable bit.
	statusSourceTrackerRecipe = "the tracker witness and [recipes.<id>].enabled in ai-specs/ai-specs.toml"
)

// collectStatus collects every capability's facts once. It is the read form end to end:
// the ledger is loaded without minting the creation token, no section is written and no
// backup is kept. narrow drops the sections `--ledger` did not ask for; the phase and the
// exit stay the composite's.
func collectStatus(root, narrow string) statusReport {
	// The blocker and unknown lists are always arrays, never null: a host reads them as
	// lists, and an empty list is an answer ("nothing blocks", "nothing is unknown").
	report := statusReport{root: root, Blockers: []string{}, Advice: []string{}, Unknown: []string{}}
	mode := planbuild.ResolveRepo(root)
	branch := planbuild.Branch(root)
	change, _ := planbuild.ChangeFromBranch(branch)
	facts := gates.WriteFactsFor(root, mode, change, "")
	report.scopeNotes = facts.Scope.Notes
	// The untracked listing is collected here, once, and written back into the collected
	// facts, so the report and the workstream predicate cannot disagree about whether git
	// answered it (L5 item 8).
	untracked, untrackedOK := planbuild.UntrackedPaths(root)
	facts.Untracked, facts.UntrackedKnown = statusProductPaths(untracked, facts.Scope), untrackedOK

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
		Integration:     statusIntegrationBranch(root, branch, facts.Base),
	}
	report.Phase, report.Why = resolveStatusPhase(evidence)

	verdict := facts.Verdict()
	for _, blocker := range verdict.Blockers {
		report.Blockers = append(report.Blockers, blocker.String())
	}
	// The posture half (L5 items 6-7): one collection, so `status` and `next` cannot
	// disagree about which checkpoint the phase heads to or which rules advise. R1/R2
	// above keep blocking in both postures; R3-R5 advise unless the project requires
	// this checkpoint.
	posture := gates.LoadPosture(root)
	merge := gates.MergeFactsFor(root, facts)
	report.Checkpoint = statusCheckpoint(report.Phase)
	report.Posture = statusPosture(posture, report.Checkpoint)
	report.postureNotes = append(append([]string{}, posture.Notes...), facts.MergeNotes(merge)...)
	for _, refutation := range facts.MergeRefutations(merge, posture.Thresholds) {
		if report.Posture == statusPostureRequire {
			report.Blockers = append(report.Blockers, refutation.String())
			continue
		}
		report.Advice = append(report.Advice, refutation.String())
	}
	report.Worktree, report.needed = statusWorktreeFacts(facts, loaded.Ident, branch, proof, openRow, closedRow,
		ledgerErr, report.needed)
	report.Plan = statusPlanFacts(facts, verdict, loaded, ledgerErr, report.needed)
	report.Tracker = statusTrackerFacts(root, loaded.Ident.CommonDir, view, openRow, closedRow)
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

// statusLegacyTrackerRecipe mirrors ledger_bridge.LEGACY_RECIPE_ID (and doctor's
// legacyTrackerRecipeID): the provider a witness-less project is checked against, so a
// fresh clone with no sync still reports the tracker it enables.
const statusLegacyTrackerRecipe = "trello-mcp-workflow"

// statusIntegrationBranch names the integration line this checkout sits on, "" when it
// is not one. The two names are the repo's own: the resolved base (BaseRef's answer, so
// the configured base_branch with its main/master fallbacks) and the worktree-flow
// integration branch declaration.
func statusIntegrationBranch(root, branch, base string) string {
	if branch == "" {
		return ""
	}
	names := []string{gates.IntegrationBranch(root)}
	if base != "" {
		names = append(names, strings.TrimPrefix(base, "origin/"))
	}
	for _, name := range names {
		if name != "" && name == branch {
			return branch
		}
	}
	return ""
}

// statusProductPaths keeps the listing's product paths: divergence is product work,
// and an untracked note or a scratch file is not a change. It reads the same predicate
// the gate grades, so the report and the gate cannot disagree.
func statusProductPaths(paths []string, scope gates.ProductScope) []string {
	var product []string
	for _, path := range paths {
		if scope.Path(path) {
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

// statusTrackerFacts is the tracker capability's fact set: the provider recipe the
// project enables, the row, the card it is bound to, its exemption, its conflict and
// the archive-close opt-out. A branch with no row is `none`, which is an answer and not
// an unknown, and whether that is even applicable is the project's own declaration
// (L8-3): a tracker recipe enabled with no row names the real state, never
// `not-applicable`.
func statusTrackerFacts(root, commonDir string, view ledger.View, openRow, closedRow *ledger.Item) []statusFact {
	recipe := knownFact("recipe", "none", statusSourceTrackerRecipe)
	if enabled := statusTrackerRecipe(root, commonDir); enabled != "" {
		recipe = knownFact("recipe", enabled, statusSourceTrackerRecipe)
	}
	row, card := knownFact("row", "none", statusSourceTracker), knownFact("card", "none", statusSourceTracker)
	exemption := knownFact("exemption", "none", statusSourceTracker)
	conflict := knownFact("conflict", "none", statusSourceTracker)
	optOut := knownFact("opt-out", "none", statusSourceTracker)
	item := openRow
	if item == nil {
		item = closedRow
	}
	if item == nil {
		return []statusFact{recipe, row, card, exemption, conflict, optOut}
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
	return []statusFact{recipe, row, card, exemption, conflict, optOut}
}

// statusTrackerRecipe names the tracker provider this project enables, "" when it
// enables none. It reuses the detection doctor's ledger check reads: the recipe the
// witness bound the tracker capability to (the legacy provider literal when the witness
// names none), kept only when [recipes.<id>].enabled is a real true in the manifest.
func statusTrackerRecipe(root, commonDir string) string {
	recipe := statusLegacyTrackerRecipe
	if commonDir != "" {
		if bound := ledger.ReadBinding(ledger.WitnessPath(commonDir)).RecipeID; bound != "" {
			recipe = bound
		}
	}
	manifest, err := config.LoadManifest(planbuild.ManifestPath(root))
	if err != nil {
		return ""
	}
	recipes, ok := manifest.Table("recipes")
	if !ok || recipes == nil {
		return ""
	}
	entry, ok := recipes.Table(recipe)
	if !ok || entry == nil {
		return ""
	}
	if enabled, isBool := entry.Bool("enabled"); isBool && enabled {
		return recipe
	}
	return ""
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
	if report.Checkpoint != "" {
		// The human report keeps its ten-column labels; `changes next` prints the same
		// facts with its own short labels.
		fmt.Fprintf(&out, "posture:  %s at the %s checkpoint\n", report.Posture, report.Checkpoint)
	}
	for _, blocker := range report.Blockers {
		fmt.Fprintf(&out, "blocked:  %s\n", blocker)
	}
	for _, advice := range report.Advice {
		fmt.Fprintf(&out, "advice:   %s\n", advice)
	}
	return out.String()
}

// relayStatusNotes prints what reading the facts cost, once, under the verb's own
// prefix: the ledger itself, the store's own report, and the declarations that could
// not be read. The posture notes go the same way as the scope notes.
func relayStatusNotes(report statusReport, stderr io.Writer, prefix string) {
	if report.ledgerErr != nil {
		where := planledger.Store(report.root)
		if where == "" {
			where = "a repository Git cannot name"
		}
		fmt.Fprintf(stderr, "%s: cannot read the ledger for %s: %s\n", prefix, where, report.ledgerErr)
	}
	if report.report != nil {
		// The store notes surface here, once (T7.4): a predecessor absorbed, a row that
		// differs in place, a source nobody could read, a backup that could not be kept.
		report.report.Note(stderr, prefix)
	}
	for _, note := range append(append([]string{}, report.scopeNotes...), report.postureNotes...) {
		fmt.Fprintf(stderr, "%s: note: %s\n", prefix, note)
	}
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

// statusTrackerLine renders the tracker capability as one line: `not-applicable` only
// when the project enables no tracker recipe, otherwise the real state and the fields
// that carry a value. What to do about a missing binding is T9's, so this line never
// adds a step of its own.
func statusTrackerLine(facts []statusFact) string {
	row := statusFactValue(facts, "row")
	if row == "none" {
		if statusFactValue(facts, "recipe") == "none" {
			return "not-applicable (no tracker recipe enabled)"
		}
		return "no item bound, card: none"
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
