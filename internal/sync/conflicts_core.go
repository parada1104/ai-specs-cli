package sync

import (
	"ai-specs.dev/worktree-gate/shared"
)

// PlanTagConflicts acquires each enabled recipe's top-level [recipe] tag
// metadata through the shared lax reader and grades tag conflicts with the
// shared authority. The Python primary authority grades tag conflicts through
// the gate binary (--resolve-tag-conflicts), whose reader omits a recipe it
// cannot fully acquire; binding the materialize call site to the same shared
// acquisition keeps Go's advisory warning set identical to Python's. Repeated
// ids dedupe on the first occurrence so first-seen tag order is stable.
func PlanTagConflicts(catalogDir string, recipeIDs []string) ([]shared.TagConflict, error) {
	recipes, err := shared.LoadRecipeTagMetadata(catalogDir, recipeIDs)
	if err != nil {
		return nil, err
	}
	return shared.CheckTagConflicts(recipes), nil
}
