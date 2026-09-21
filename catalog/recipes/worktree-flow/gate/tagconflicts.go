package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// Advisory tag-conflict grading, moved from the Python authority
// (lib/_internal/recipe-conflicts.py check_tag_conflicts) with the same grouping
// and the same outcomes:
//
//   - recipeTagMetadata is one enabled recipe's top-level [recipe] declaration:
//     the TOML recipe id, its tags, and its conflicts_with ids.
//   - tagConflict is one graded tag conflict, recipes sorted and deduplicated and
//     a severity of "warning" or "fatal".
//   - tagConflictPlan is the single JSON object emitted on stdout. Tag conflicts
//     are advisory: every graded result exits 0, and the calling bridge owns the
//     warning text and never changes the materialization exit code.

// recipeTagMetadata is the acquired top-level [recipe] declaration the grader
// reads. ID is the TOML recipe id, never the catalog directory name.
type recipeTagMetadata struct {
	ID            string   `json:"id"`
	Tags          []string `json:"tags"`
	ConflictsWith []string `json:"conflicts_with"`
}

// tagConflict is one graded tag conflict. Recipes is always sorted and
// deduplicated so the JSON is deterministic. Severity is the extra field the
// Python bridge needs to reproduce the warning/fatal message contract; the Python
// authority carries it on the TagConflict object rather than in to_dict().
type tagConflict struct {
	Type     string   `json:"type"`
	Tag      string   `json:"tag"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// tagConflictPlan is the stdout contract. Conflicts is always present; an absent
// conflict set is an empty list, never null.
type tagConflictPlan struct {
	Conflicts []tagConflict `json:"conflicts"`
}

// tagConflictOptions is the parsed --resolve-tag-conflicts flag surface.
type tagConflictOptions struct {
	catalogDir string
	recipeIDs  []string
}

// checkTagConflicts mirrors recipe-conflicts.check_tag_conflicts: group the
// enabled recipes by tag in first-seen order, skip every tag held by fewer than
// two recipes or fewer than two distinct ids, and grade the overlap fatal when a
// sharing recipe lists another sharing recipe in conflicts_with. The relationship
// is symmetric — one side declaring it is enough.
func checkTagConflicts(recipes []recipeTagMetadata) []tagConflict {
	order := []string{}
	groups := map[string][]recipeTagMetadata{}
	for _, recipe := range recipes {
		for _, tag := range recipe.Tags {
			if _, seen := groups[tag]; !seen {
				order = append(order, tag)
			}
			groups[tag] = append(groups[tag], recipe)
		}
	}

	conflicts := []tagConflict{}
	for _, tag := range order {
		group := groups[tag]
		if len(group) < 2 {
			continue
		}
		ids := make([]string, 0, len(group))
		seen := make(map[string]bool, len(group))
		for _, recipe := range group {
			if !seen[recipe.ID] {
				seen[recipe.ID] = true
				ids = append(ids, recipe.ID)
			}
		}
		// A single recipe that lists the same tag twice is not a conflict.
		if len(ids) < 2 {
			continue
		}
		fatal := false
		for _, recipe := range group {
			for _, other := range group {
				if recipe.ID != other.ID && slices.Contains(recipe.ConflictsWith, other.ID) {
					fatal = true
				}
			}
		}
		severity := "warning"
		if fatal {
			severity = "fatal"
		}
		conflicts = append(conflicts, tagConflict{
			Type:     "tag_conflict",
			Tag:      tag,
			Recipes:  sortedUnique(ids),
			Severity: severity,
		})
	}
	return conflicts
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
	payload, err := json.Marshal(tagConflictPlan{Conflicts: checkTagConflicts(recipes)})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-tag-conflicts: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
