package gates

import "ai-specs.dev/ai-specs/internal/specs"

// promotionBlockers blocks Standard/Full when a delta is unpromoted or cannot be
// composed. Read-only and tier-scoped: Light and changes without specs/ deltas
// are never affected, and no canonical spec is written here.
//
// Python additionally guarded a missing/unloadable spec_promotion.py sibling and
// wrapped parser exceptions. Both are unreachable here: the parity checker is a
// compiled-in package and has no runtime load step.
func promotionBlockers(repoRoot, folder, tier, location string) []string {
	if tier != TierStandard && tier != TierFull {
		return nil
	}
	var blockers []string
	for _, blocker := range specs.CheckFolderParity(repoRoot, folder) {
		blockers = append(blockers, location+": "+blocker)
	}
	return blockers
}
