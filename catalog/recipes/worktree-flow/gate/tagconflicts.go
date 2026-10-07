package main

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-specs.dev/worktree-gate/shared"
)

// Advisory tag-conflict grading. The pure grader and its JSON types live in the
// shared package (slice SX0d.1) so the root single binary can reach the same
// decision in process; this file keeps the flag surface and the command
// wrapper. The Python authority is
// lib/_internal/recipe-conflicts.py check_tag_conflicts.

// tagConflictOptions is the parsed --resolve-tag-conflicts flag surface.
type tagConflictOptions struct {
	catalogDir string
	recipeIDs  []string
}

// runResolveTagConflicts is the --resolve-tag-conflicts command: acquire the
// enabled recipes' [recipe] metadata through the TOML seam, grade tag conflicts,
// and print one JSON object on stdout. Tag conflicts are advisory, so any graded
// result exits 0; only a process-level failure — unusable flags or an unavailable
// parser — exits 2.
func runResolveTagConflicts(opts tagConflictOptions, stdout, stderr io.Writer) int {
	if opts.catalogDir == "" {
		fmt.Fprintln(stderr, "worktree-gate: --resolve-tag-conflicts requires --catalog-dir")
		return 2
	}
	recipes, err := loadRecipeTagMetadata(opts.catalogDir, opts.recipeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-tag-conflicts: %v\n", err)
		return 2
	}
	payload, err := json.Marshal(shared.TagConflictPlan{Conflicts: shared.CheckTagConflicts(recipes)})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-tag-conflicts: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
