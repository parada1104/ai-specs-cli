package shared

import "slices"

// Pure recipe-conflict grading, moved verbatim from the gate main package
// (tagconflicts.go and primitiveconflicts.go) as slice SX0d.1 so the root
// single binary can reach the same decisions in process. The JSON shapes,
// grouping order, claim order and outcomes are unchanged; the acquisition and
// command wrappers stay in the gate main package.
//
// Two authorities are mirrored:
//
//   - Advisory tag-conflict grading, moved from the Python authority
//     (lib/_internal/recipe-conflicts.py check_tag_conflicts) with the same
//     grouping and the same outcomes:
//
//   - RecipeTagMetadata is one enabled recipe's top-level [recipe] declaration:
//     the TOML recipe id, its tags, and its conflicts_with ids.
//   - TagConflict is one graded tag conflict, recipes sorted and deduplicated
//     and a severity of "warning" or "fatal".
//   - TagConflictPlan is the single JSON object emitted on stdout. Tag
//     conflicts are advisory: every graded result exits 0, and the calling
//     bridge owns the warning text and never changes the materialization exit
//     code.
//
//   - Primitive-conflict grading, moved from the Python authority
//     (lib/_internal/recipe-conflicts.py check_recipe_conflicts) with the same
//     claim order and the same outcomes:
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

// RecipeTagMetadata is the acquired top-level [recipe] declaration the tag
// grader reads. ID is the TOML recipe id, never the catalog directory name.
type RecipeTagMetadata struct {
	ID            string   `json:"id"`
	Tags          []string `json:"tags"`
	ConflictsWith []string `json:"conflicts_with"`
}

// TagConflict is one graded tag conflict. Recipes is always sorted and
// deduplicated so the JSON is deterministic. Severity is the extra field the
// Python bridge needs to reproduce the warning/fatal message contract; the
// Python authority carries it on the TagConflict object rather than in
// to_dict().
type TagConflict struct {
	Type     string   `json:"type"`
	Tag      string   `json:"tag"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// TagConflictPlan is the stdout contract. Conflicts is always present; an
// absent conflict set is an empty list, never null.
type TagConflictPlan struct {
	Conflicts []TagConflict `json:"conflicts"`
}

// CheckTagConflicts mirrors recipe-conflicts.check_tag_conflicts: group the
// enabled recipes by tag in first-seen order, skip every tag held by fewer than
// two recipes or fewer than two distinct ids, and grade the overlap fatal when a
// sharing recipe lists another sharing recipe in conflicts_with. The relationship
// is symmetric — one side declaring it is enough.
func CheckTagConflicts(recipes []RecipeTagMetadata) []TagConflict {
	order := []string{}
	groups := map[string][]RecipeTagMetadata{}
	for _, recipe := range recipes {
		for _, tag := range recipe.Tags {
			if _, seen := groups[tag]; !seen {
				order = append(order, tag)
			}
			groups[tag] = append(groups[tag], recipe)
		}
	}

	conflicts := []TagConflict{}
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
		conflicts = append(conflicts, TagConflict{
			Type:     "tag_conflict",
			Tag:      tag,
			Recipes:  SortedUnique(ids),
			Severity: severity,
		})
	}
	return conflicts
}

// RecipePrimitives is the acquired primitive declaration one enabled recipe
// makes: the TOML recipe id, the [recipe].name that owns claims in the conflict
// registry, and the ordered skill, command and mcp ids under [provides].
type RecipePrimitives struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Skills   []string `json:"skills"`
	Commands []string `json:"commands"`
	MCP      []string `json:"mcp"`
}

// PrimitiveConflict is one graded primitive conflict. Recipes is always sorted
// and deduplicated so the JSON is deterministic; severity is always "fatal".
type PrimitiveConflict struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

// PrimitiveConflictPlan is the stdout contract. Conflicts is always present; an
// absent conflict set is an empty list, never null.
type PrimitiveConflictPlan struct {
	Conflicts []PrimitiveConflict `json:"conflicts"`
}

// primitiveClaim is one primitive id a recipe lays claim to, tagged with the
// primitive type the Python authority reports on the Conflict.
type primitiveClaim struct {
	primitiveType string
	id            string
}

// claims returns the recipe's claims in the Python authority's registration
// order: skills, then commands, then mcp, array order within each list.
func (r RecipePrimitives) claims() []primitiveClaim {
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

// CheckPrimitiveConflicts mirrors ConflictRegistry.register_recipe over the
// enabled recipes in order, collecting the conflicts Python raises per recipe.
func CheckPrimitiveConflicts(recipes []RecipePrimitives) []PrimitiveConflict {
	owners := map[string]map[string]string{}
	conflicts := []PrimitiveConflict{}
	for _, recipe := range recipes {
		for _, claim := range recipe.claims() {
			byID, ok := owners[claim.primitiveType]
			if !ok {
				byID = map[string]string{}
				owners[claim.primitiveType] = byID
			}
			if owner, claimed := byID[claim.id]; claimed {
				conflicts = append(conflicts, PrimitiveConflict{
					Type:     claim.primitiveType,
					ID:       claim.id,
					Recipes:  SortedUnique([]string{owner, recipe.Name}),
					Severity: "fatal",
				})
				break
			}
			byID[claim.id] = recipe.Name
		}
	}
	return conflicts
}
