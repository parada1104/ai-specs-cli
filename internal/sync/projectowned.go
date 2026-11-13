package sync

// Promoted project-owned recipe-config merge (GO-07 S14a WU2).
//
// util.project_owned_recipe_config (lib/_internal/util.py:334) copies a merged
// recipe config and overlays the project-owned keys, so a project value always
// wins over the recipe's legacy alias. Today the only project-owned key is
// worktree-flow's repo_topology. The logic lived in
// internal/doctor/checks_overrides.go; it is promoted here so sync and doctor
// share ONE grader, and doctor's local wrapper delegates.

import (
	"ai-specs.dev/ai-specs/internal/target"
	"ai-specs.dev/ai-specs/internal/toml"
)

// legacyTopologyRecipeID is util.LEGACY_TOPOLOGY_RECIPE_ID: the only recipe
// with a project-owned config key.
const legacyTopologyRecipeID = "worktree-flow"

// ProjectOwnedRecipeConfig mirrors util.project_owned_recipe_config: a copy of
// config with the project-owned keys applied. For worktree-flow the
// configured repo_topology (nil data falls back to a best-effort manifest
// read) replaces any recipe-declared value. Every other recipe is returned
// unchanged.
func ProjectOwnedRecipeConfig(projectRoot string, data *toml.Table, recipeID string, config map[string]any) map[string]any {
	merged := make(map[string]any, len(config)+1)
	for key, value := range config {
		merged[key] = value
	}
	if recipeID == legacyTopologyRecipeID {
		merged["repo_topology"] = target.ProjectRepoTopology(projectRoot, data).Configured
	}
	return merged
}
