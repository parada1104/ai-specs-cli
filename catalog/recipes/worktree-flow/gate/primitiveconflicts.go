package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Primitive-conflict grading, moved from the Python authority
// (lib/_internal/recipe-conflicts.py check_recipe_conflicts) with the same
// claim order and the same outcomes:
//
//   - Each recipe claims its provides primitives in registration order: skills,
//     then commands, then mcp, TOML array order within each list. Recipes are
//     processed in the given --recipe order.
//   - The registry is shared across recipes and keyed
//     [primitive_type][primitive_id] -> owning recipe [recipe].name. Names, not
//     TOML ids, own claims and appear in conflicts.
//   - On a collision the grader appends one fatal Conflict carrying the owner
//     and the colliding recipe's names and abandons the remaining claims of
//     that recipe (the Python ConflictError propagates out of
//     register_recipe). The colliding recipe is never registered as owner, so
//     a third colliding recipe pairs against the first owner.
//   - The Python claim raises unconditionally when the id is already claimed —
//     including by the same recipe name — so a same-name re-claim conflicts
//     with itself and the recipe pair collapses to one name.
//
// Every graded result exits 0; only a process-level failure — unusable flags or
// a parser failure — exits 2, which the calling Python bridge relies on to fall
// back to the Python authority on invalid recipe.toml.

// primitiveConflict is one graded primitive conflict. Recipes is always sorted
// and deduplicated so the JSON is deterministic; severity is always "fatal".
type primitiveConflict struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// primitiveConflictPlan is the stdout contract. Conflicts is always present; an
// absent conflict set is an empty list, never null.
type primitiveConflictPlan struct {
	Conflicts []primitiveConflict `json:"conflicts"`
}

// primitiveClaim is one primitive id a recipe lays claim to, tagged with the
// primitive type the Python authority reports on the Conflict.
type primitiveClaim struct {
	primitiveType string
	id            string
}

// claims returns the recipe's claims in the Python authority's registration
// order: skills, then commands, then mcp, array order within each list.
func (r recipePrimitives) claims() []primitiveClaim {
	claims := make([]primitiveClaim, 0, len(r.Skills)+len(r.Commands)+len(r.MCP))
	for _, id := range r.Skills {
		claims = append(claims, primitiveClaim{primitiveType: "skill", id: id})
	}
	for _, id := range r.Commands {
		claims = append(claims, primitiveClaim{primitiveType: "command", id: id})
	}
	for _, id := range r.MCP {
		claims = append(claims, primitiveClaim{primitiveType: "mcp", id: id})
	}
	return claims
}

// primitiveConflictOptions is the parsed --resolve-primitive-conflicts flag
// surface.
type primitiveConflictOptions struct {
	catalogDir string
	recipeIDs  []string
}

// checkPrimitiveConflicts mirrors ConflictRegistry.register_recipe over the
// enabled recipes in order, collecting the conflicts Python raises per recipe.
func checkPrimitiveConflicts(recipes []recipePrimitives) []primitiveConflict {
	owners := map[string]map[string]string{}
	conflicts := []primitiveConflict{}
	for _, recipe := range recipes {
		for _, claim := range recipe.claims() {
			byID, ok := owners[claim.primitiveType]
			if !ok {
				byID = map[string]string{}
				owners[claim.primitiveType] = byID
			}
			if owner, claimed := byID[claim.id]; claimed {
				conflicts = append(conflicts, primitiveConflict{
					Type:     claim.primitiveType,
					ID:       claim.id,
					Recipes:  sortedUnique([]string{owner, recipe.Name}),
					Severity: "fatal",
				})
				break
			}
			byID[claim.id] = recipe.Name
		}
	}
	return conflicts
}

// runResolvePrimitiveConflicts is the --resolve-primitive-conflicts command:
// acquire the enabled recipes' primitive claims through the TOML seam, grade
// primitive conflicts, and print one JSON object on stdout. Any graded result —
// including conflicts — exits 0; only unusable flags or a parser failure exit 2.
func runResolvePrimitiveConflicts(opts primitiveConflictOptions, stdout, stderr io.Writer) int {
	if opts.catalogDir == "" {
		fmt.Fprintln(stderr, "worktree-gate: --resolve-primitive-conflicts requires --catalog-dir")
		return 2
	}
	recipes, err := loadRecipePrimitives(opts.catalogDir, opts.recipeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-primitive-conflicts: %v\n", err)
		return 2
	}
	payload, err := json.Marshal(primitiveConflictPlan{Conflicts: checkPrimitiveConflicts(recipes)})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-primitive-conflicts: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
