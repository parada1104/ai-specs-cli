package runner

// The local merge proof, exposed read-only (change-ledger-v2 T7).
//
// A report that says "merged" must say it for exactly the reason cleanup would act
// on, or the two disagree and one of them lies: cleanup preserves a candidate no
// proof covers, and a report that called that branch merged would invite a removal
// nobody proved. So this is a thin read-only surface over the proof the cleanup
// already owns (resolveBaseCandidatesCleanup + isMergedWithCandidates) instead of a
// second implementation of it.

// MergeProof is one branch tip against one base spelling: the refs the base resolved
// to, and whether a local proof places the tip's work in one of them. Merged is false
// whenever nothing could be proven, which is the safe direction for both callers.
type MergeProof struct {
	// Candidates are the base spellings that resolved: the base itself, its upstream,
	// the configured remote's branch, and origin's. Empty means no base resolves here.
	Candidates []string
	// Merged reports the local proof (ancestry, per-commit patch equivalence, combined
	// patch equivalence, combined tree equivalence) holding against one candidate.
	Merged bool
}

// ProveMerged resolves the base spellings once and grades sha against them. It reads
// only: no worktree, branch, ref or ledger is touched.
func ProveMerged(repoRoot, sha, base string) MergeProof {
	if repoRoot == "" || sha == "" || base == "" {
		return MergeProof{}
	}
	candidates := resolveBaseCandidatesCleanup(repoRoot, base)
	return MergeProof{Candidates: candidates, Merged: isMergedWithCandidates(repoRoot, sha, candidates)}
}
