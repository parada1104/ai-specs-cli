package shared

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
//
// This package is the SX0a in-process extraction: the gate main package
// dispatches to it and the root ai-specs binary imports it directly.

// OrphanPlanInput is the stdin contract. Every field is a set of names; a
// missing or null field is the empty set.
type OrphanPlanInput struct {
	RecipeSkills     []string `json:"recipe_skills"`
	DepsSkills       []string `json:"deps_skills"`
	InprojectDeps    []string `json:"inproject_deps"`
	LockRecipes      []string `json:"lock_recipes"`
	EnabledRecipeIDs []string `json:"enabled_recipe_ids"`
	ExpectedDepIDs   []string `json:"expected_dep_ids"`
}

// OrphanPlan is the stdout contract. Every field is always present and sorted;
// an absent orphan set is an empty list, never null.
type OrphanPlan struct {
	OrphanedRecipes       []string `json:"orphaned_recipes"`
	OrphanedDeps          []string `json:"orphaned_deps"`
	OrphanedInprojectDeps []string `json:"orphaned_inproject_deps"`
	StaleLockRecipes      []string `json:"stale_lock_recipes"`
}

// PlanOrphans is the pure decision core. The recipe-skill and lock-recipe
// scopes are compared against the enabled recipe ids; the cached dep-skill and
// in-project dep scopes are both compared against the expected dep ids.
func PlanOrphans(in OrphanPlanInput) OrphanPlan {
	return OrphanPlan{
		OrphanedRecipes:       absentFrom(in.RecipeSkills, in.EnabledRecipeIDs),
		OrphanedDeps:          absentFrom(in.DepsSkills, in.ExpectedDepIDs),
		OrphanedInprojectDeps: absentFrom(in.InprojectDeps, in.ExpectedDepIDs),
		StaleLockRecipes:      absentFrom(in.LockRecipes, in.EnabledRecipeIDs),
	}
}

// absentFrom returns the values not present in expected, sorted and
// deduplicated via the shared SortedUnique helper. A nil input yields a
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
	return SortedUnique(absent)
}

// RunPlanOrphans is the --plan-orphans command: decode the JSON envelope from
// stdin, plan the orphan sets, print one JSON object on stdout. A malformed
// envelope is a process-level failure (exit 2); an empty stdin or an absent
// field is the empty set.
func RunPlanOrphans(stdin io.Reader, stdout, stderr io.Writer) int {
	var in OrphanPlanInput
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
	payload, err := json.Marshal(PlanOrphans(in))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-orphans: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
