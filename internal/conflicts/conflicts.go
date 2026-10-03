// Package conflicts is the native port of lib/_internal/recipe-conflicts.py:
// primitive ID collisions (skill/command/mcp), tag conflicts and capability
// ambiguity across enabled recipes.
//
// The worktree-gate keeps its own package-main copies (tagconflicts.go,
// primitiveconflicts.go) behind the Python bridges until the materialize
// slices retire them (plan Q2 = keep delegating).
package conflicts

import (
	"os"
	"path/filepath"
	"sort"

	"ai-specs.dev/ai-specs/internal/schema"
)

// Conflict mirrors both Conflict (Type = primitive type) and TagConflict
// (Type = "tag_conflict", ID = tag). Recipes is the Python set, sorted.
type Conflict struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Recipes  []string `json:"recipes"`
	Severity string   `json:"severity"`
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// CheckRecipeConflicts mirrors check_recipe_conflicts. A missing recipe dir
// or an invalid recipe.toml aborts with the RecipeValidationError text. As in
// ConflictRegistry.register_recipe, a recipe's first collision stops the
// registration of its remaining primitives.
func CheckRecipeConflicts(catalogDir string, recipeIDs []string) ([]Conflict, error) {
	claimed := map[string]map[string]string{}
	var out []Conflict
	for _, rid := range recipeIDs {
		recipeDir := filepath.Join(catalogDir, rid)
		if !isDir(recipeDir) {
			return nil, schema.NewValidationError("recipe directory not found: " + recipeDir)
		}
		recipe, err := schema.LoadRecipeToml(filepath.Join(recipeDir, "recipe.toml"))
		if err != nil {
			return nil, err
		}
		type claim struct{ kind, id string }
		var claims []claim
		for _, s := range recipe.Skills {
			claims = append(claims, claim{"skill", s.ID})
		}
		for _, c := range recipe.Commands {
			claims = append(claims, claim{"command", c.ID})
		}
		for _, m := range recipe.MCP {
			claims = append(claims, claim{"mcp", m.ID})
		}
		for _, c := range claims {
			pt := claimed[c.kind]
			if pt == nil {
				pt = map[string]string{}
				claimed[c.kind] = pt
			}
			if owner, ok := pt[c.id]; ok {
				out = append(out, Conflict{c.kind, c.id, sortedSet(map[string]bool{owner: true, recipe.Name: true}), "fatal"})
				break
			}
			pt[c.id] = recipe.Name
		}
	}
	return out, nil
}

// CheckTagConflicts mirrors check_tag_conflicts: one conflict per tag shared
// by 2+ distinct recipes (first-seen tag order); fatal when any sharing recipe
// lists another in conflicts_with.
func CheckTagConflicts(recipes []*schema.Recipe) []Conflict {
	var order []string
	groups := map[string][]*schema.Recipe{}
	for _, r := range recipes {
		for _, tag := range r.Tags {
			if _, ok := groups[tag]; !ok {
				order = append(order, tag)
			}
			groups[tag] = append(groups[tag], r)
		}
	}
	var out []Conflict
	for _, tag := range order {
		group := groups[tag]
		ids := map[string]bool{}
		for _, r := range group {
			ids[r.ID] = true
		}
		if len(group) < 2 || len(ids) < 2 {
			continue
		}
		severity := "warning"
	scan:
		for _, r := range group {
			for _, other := range group {
				if r.ID == other.ID {
					continue
				}
				for _, c := range r.ConflictsWith {
					if c == other.ID {
						severity = "fatal"
						break scan
					}
				}
			}
		}
		out = append(out, Conflict{"tag_conflict", tag, sortedSet(ids), severity})
	}
	return out
}

// CheckCapabilityConflicts mirrors check_capability_conflicts: unreadable
// recipes are skipped; the first duplicate explicit binding is the only
// (fatal) result; otherwise every unbound capability with 2+ providers is a
// warning, in first-seen capability order.
func CheckCapabilityConflicts(catalogDir string, recipeIDs []string, bindings []map[string]string) []Conflict {
	var order []string
	providers := map[string]map[string]bool{}
	for _, rid := range recipeIDs {
		recipeDir := filepath.Join(catalogDir, rid)
		if !isDir(recipeDir) {
			continue
		}
		recipe, err := schema.LoadRecipeToml(filepath.Join(recipeDir, "recipe.toml"))
		if err != nil {
			continue
		}
		for _, c := range recipe.Capabilities {
			if providers[c.ID] == nil {
				providers[c.ID] = map[string]bool{}
				order = append(order, c.ID)
			}
			providers[c.ID][rid] = true
		}
	}

	bound := map[string]string{}
	for _, b := range bindings {
		cap, rec := b["capability"], b["recipe"]
		if prev, ok := bound[cap]; ok {
			return []Conflict{{"capability", cap, sortedSet(map[string]bool{prev: true, rec: true}), "fatal"}}
		}
		bound[cap] = rec
	}

	var out []Conflict
	for _, cap := range order {
		if _, ok := bound[cap]; ok {
			continue
		}
		if len(providers[cap]) > 1 {
			out = append(out, Conflict{"capability", cap, sortedSet(providers[cap]), "warning"})
		}
	}
	return out
}
