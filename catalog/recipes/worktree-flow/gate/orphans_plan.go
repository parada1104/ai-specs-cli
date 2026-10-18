package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Orphan planning for recipe materialization, moved from the Python authority
// clean_orphans (lib/_internal/recipe-materialize.py 1558-1601). Go owns the
// decision (which materialized ids are orphaned); Python keeps the destructive
// deletes and lock pruning as a thin bridge.
//
// The command is pure set arithmetic over a JSON envelope on stdin: no
// filesystem access, no TOML parsing. One materialized name is orphaned when it
// is absent from the set of ids the manifest still expects.

// orphanPlanInput is the stdin contract. Every field is a set of names; a
// missing or null field is the empty set.
type orphanPlanInput struct {
	RecipeSkills     []string `json:"recipe_skills"`
	DepsSkills       []string `json:"deps_skills"`
	InprojectDeps    []string `json:"inproject_deps"`
	LockRecipes      []string `json:"lock_recipes"`
	EnabledRecipeIDs []string `json:"enabled_recipe_ids"`
	ExpectedDepIDs   []string `json:"expected_dep_ids"`
}

// orphanPlan is the stdout contract. Every field is always present and sorted;
// an absent orphan set is an empty list, never null.
type orphanPlan struct {
	OrphanedRecipes       []string `json:"orphaned_recipes"`
	OrphanedDeps          []string `json:"orphaned_deps"`
	OrphanedInprojectDeps []string `json:"orphaned_inproject_deps"`
	StaleLockRecipes      []string `json:"stale_lock_recipes"`
}

// planOrphans is the pure decision core. The recipe-skill and lock-recipe
// scopes are compared against the enabled recipe ids; the cached dep-skill and
// in-project dep scopes are both compared against the expected dep ids.
func planOrphans(in orphanPlanInput) orphanPlan {
	return orphanPlan{
		OrphanedRecipes:       absentFrom(in.RecipeSkills, in.EnabledRecipeIDs),
		OrphanedDeps:          absentFrom(in.DepsSkills, in.ExpectedDepIDs),
		OrphanedInprojectDeps: absentFrom(in.InprojectDeps, in.ExpectedDepIDs),
		StaleLockRecipes:      absentFrom(in.LockRecipes, in.EnabledRecipeIDs),
	}
}

// absentFrom returns the values not present in expected, sorted and
// deduplicated via the shared sortedUnique helper. A nil input yields a
// non-nil empty slice, so JSON emits [] and never null.
func absentFrom(values, expected []string) []string {
	expectedSet := make(map[string]bool, len(expected))
	for _, value := range expected {
		expectedSet[value] = true
	}
	absent := make([]string, 0, len(values))
	for _, value := range values {
		if !expectedSet[value] {
			absent = append(absent, value)
		}
	}
	return sortedUnique(absent)
}

// runPlanOrphans is the --plan-orphans command: decode the JSON envelope from
// stdin, plan the orphan sets, print one JSON object on stdout. A malformed
// envelope is a process-level failure (exit 2); an empty stdin or an absent
// field is the empty set.
func runPlanOrphans(stdin io.Reader, stdout, stderr io.Writer) int {
	var in orphanPlanInput
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-orphans: read stdin: %v\n", err)
		return 2
	}
	if text := strings.TrimSpace(string(raw)); text != "" {
		if err := json.Unmarshal([]byte(text), &in); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --plan-orphans: invalid input JSON: %v\n", err)
			return 2
		}
	}
	payload, err := json.Marshal(planOrphans(in))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-orphans: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
