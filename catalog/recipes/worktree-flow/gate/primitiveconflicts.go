package main

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-specs.dev/worktree-gate/shared"
)

// Primitive-conflict grading. The pure grader and its JSON types live in the
// shared package (slice SX0d.1) so the root single binary can reach the same
// decision in process; this file keeps the flag surface and the command
// wrapper. The Python authority is
// lib/_internal/recipe-conflicts.py check_recipe_conflicts.

// primitiveConflictOptions is the parsed --resolve-primitive-conflicts flag
// surface.
type primitiveConflictOptions struct {
	catalogDir string
	recipeIDs  []string
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
	payload, err := json.Marshal(shared.PrimitiveConflictPlan{Conflicts: shared.CheckPrimitiveConflicts(recipes)})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-primitive-conflicts: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
