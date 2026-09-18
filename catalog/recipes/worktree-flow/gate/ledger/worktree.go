package ledger

// The Worktree domain port.
//
// Worktree is the second domain port over the autonomous ledger core, alongside
// Tracker. It is deliberately not a second framework: it owns one deterministic
// predicate over already-acquired evidence, exactly as Grade owns the tracker
// predicate. There is no store, no witness, no checkpoint set, and no provider
// vocabulary here.
//
// Everything that needs git, the filesystem, or a provider CLI happens during
// acquisition and arrives as a normalized WorktreeObservation. This file never
// shells out, never reads the clock, and never guesses: a merge must be proven
// by a local proof or by a provider merge commit that acquisition already
// resolved to one of the base candidates.

// Worktree outcomes. The reason vocabulary is the cleanup contract's own
// output: these exact strings are printed in `skipped <name> (<reason>)`, so a
// caller reporting them does not translate.
const (
	// WorktreeReasonDetached: no branch is checked out, so there is no branch
	// to prove merged or to delete.
	WorktreeReasonDetached = "detached"

	// WorktreeReasonDirty: the worktree has uncommitted changes. Uncommitted
	// work is never destroyed by a merge proof.
	WorktreeReasonDirty = "dirty"

	// WorktreeReasonMerged: the branch's work is provably in the base, either by
	// a local proof or by a provider merge commit reachable from a base candidate.
	WorktreeReasonMerged = "merged"

	// WorktreeReasonUnmerged: no proof succeeded. Missing, malformed, or
	// out-of-base evidence all land here, because all of them mean the same
	// thing to a destructive caller: preserve the candidate.
	WorktreeReasonUnmerged = "unmerged"
)

// WorktreeObservation is the normalized, provider-neutral snapshot the port
// evaluates. Acquisition fills it; nothing here is inferred.
//
// Detached and Dirty are the immediate safety facts and short-circuit first.
// LocalMerged is the result of the local graph, patch, and tree proofs.
// PRMergeCommit is the provider-reported merge commit, empty when the provider
// reported none or was unavailable; MergeCommitInBase records whether
// acquisition proved that commit reachable from one of the base candidates.
type WorktreeObservation struct {
	Detached          bool
	Dirty             bool
	LocalMerged       bool
	PRMergeCommit     string
	MergeCommitInBase bool
}

// WorktreeOutcome is the deterministic verdict for one candidate. Merged is the
// only field a destructive caller acts on; Reason is the reporting contract and
// is set on every outcome, including the merged one.
type WorktreeOutcome struct {
	Merged bool
	Reason string
}

// EvaluateWorktree classifies one normalized observation. It is pure: the same
// observation always yields the same outcome.
//
// The check order is the contract, and it is safety order. Detached precedes
// dirty, and both precede any merge evidence, so an unsafe candidate is
// reported for what it is and never looks cleanable on the strength of a merge
// proof. A merge then requires a positive proof: a local proof, or a recorded
// provider merge commit that acquisition proved in base. An in-base flag
// without a recorded commit is not proof, and absent evidence is unmerged.
func EvaluateWorktree(obs WorktreeObservation) WorktreeOutcome {
	switch {
	case obs.Detached:
		return WorktreeOutcome{Reason: WorktreeReasonDetached}
	case obs.Dirty:
		return WorktreeOutcome{Reason: WorktreeReasonDirty}
	case obs.LocalMerged || (obs.PRMergeCommit != "" && obs.MergeCommitInBase):
		return WorktreeOutcome{Merged: true, Reason: WorktreeReasonMerged}
	default:
		return WorktreeOutcome{Reason: WorktreeReasonUnmerged}
	}
}
