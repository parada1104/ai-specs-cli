package sync

import (
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/worktree-gate/shared"
)

// PlanTagConflicts acquires each enabled recipe's top-level [recipe] tag
// metadata and grades tag conflicts with the shared authority. It mirrors the
// Python tag acquisition (_python_check_tag_conflicts at
// lib/_internal/recipe-materialize.py:635) for the materialize call site at
// :3960: a recipe.toml that is missing or is not a regular file is skipped,
// while a present file the loader rejects propagates its error because the
// Python authority calls load_recipe_toml without a try/except. Repeated ids
// dedupe on the first occurrence so first-seen tag order is stable.
func PlanTagConflicts(catalogDir string, recipeIDs []string) ([]shared.TagConflict, error) {
	recipes := []shared.RecipeTagMetadata{}
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		path := filepath.Join(catalogDir, rid, "recipe.toml")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		recipe, err := schema.LoadRecipeToml(path)
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, shared.RecipeTagMetadata{
			ID:            recipe.ID,
			Tags:          recipe.Tags,
			ConflictsWith: recipe.ConflictsWith,
		})
	}
	return shared.CheckTagConflicts(recipes), nil
}
