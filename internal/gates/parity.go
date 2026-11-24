package gates

import "ai-specs.dev/ai-specs/internal/specs"

// promotionBlockers blocks any change that carries a delta but leaves it
// unpromoted or uncomposable. Read-only and scoped by the delta itself, not by a
// tier label: a delta is exactly the artifact that must reach odd/specs or
// openspec/specs, and no canonical spec is written here.
//
// Python additionally guarded a missing/unloadable spec_promotion.py sibling and
// wrapped parser exceptions. Both are unreachable here: the parity checker is a
// compiled-in package and has no runtime load step.
func promotionBlockers(repoRoot, folder, location string) []string {
	if !hasSpecDelta(folder) {
		return nil
	}
	var blockers []string
	for _, blocker := range specs.CheckFolderParity(repoRoot, folder) {
		blockers = append(blockers, location+": "+blocker)
	}
	return blockers
}
