package gates

// One product predicate (change-ledger-t7 L7-1/L8-1): product work is a graded path
// outside the planning trees, minus the project's own exclusions. It is loaded once
// per root and every consumer reads it — the write gate's production check, the
// workstream fact, the divergence list and `changes status` — so no folder list can
// decide that a change touching only internal/ or cmd/ owes nothing.
//
// The exclusions are the project's declaration, not the tool's: a tree that is
// generated, vendored or otherwise not the project's own work is named by the
// project in [recipes.plan-build-flow.config] product_exclude of
// ai-specs/ai-specs.toml, as path globs where `**` covers any number of segments.
// The key is deliberately not declared in the recipe's own schema (that schema only
// describes scalar fields, which is why high_risk_paths is not declared either), so
// it is written by hand like the rest of the project's scope.
//
// A malformed value is reported and never silently applied: a scope that changes
// because a declaration could not be read is exactly the lie this predicate exists
// to prevent.

import (
	"fmt"
	"path"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/planbuild"
)

// ProductExcludeKey is the manifest key that narrows the product scope.
const ProductExcludeKey = "product_exclude"

// ProductScope is the project's product predicate: the exclusions it declared and
// the notes naming a declaration that could not be read.
type ProductScope struct {
	// Exclude is the applied list of path globs, in declaration order.
	Exclude []string
	// Notes names every declaration that was not applied, so a malformed value is
	// never indistinguishable from an absent one.
	Notes []string
}

// LoadProductScope reads the project's exclusions from the working tree. An absent
// manifest or key declares nothing, which is an answer: the product scope is then
// every graded path outside the planning trees.
func LoadProductScope(root string) ProductScope {
	manifest, err := config.LoadManifest(planbuild.ManifestPath(root))
	if err != nil {
		return ProductScope{}
	}
	_, _, cfg := config.RecipeConfigState(manifest, planbuild.RecipeID)
	value, ok := config.LookupNestedConfig(cfg, ProductExcludeKey)
	if !ok {
		return ProductScope{}
	}
	raw, isArray := value.([]any)
	if !isArray {
		return ProductScope{Notes: []string{fmt.Sprintf(
			"`%s` in %s is not an array of path globs (got %T), so none of it was applied; "+
				"the product scope is unchanged and this note is the only trace of the declaration",
			ProductExcludeKey, planbuild.ManifestRelPath, value)}}
	}
	scope := ProductScope{Exclude: make([]string, 0, len(raw))}
	for index, item := range raw {
		pattern, isString := item.(string)
		switch {
		case !isString:
			scope.Notes = append(scope.Notes, fmt.Sprintf(
				"`%s` entry %d in %s is not a string, so it was not applied; the product "+
					"scope is unchanged by it", ProductExcludeKey, index, planbuild.ManifestRelPath))
		case !productGlobValid(pattern):
			scope.Notes = append(scope.Notes, fmt.Sprintf(
				"`%s` entry %d in %s (%q) is not a valid path glob, so it was not applied; "+
					"the product scope is unchanged by it", ProductExcludeKey, index,
				planbuild.ManifestRelPath, pattern))
		default:
			scope.Exclude = append(scope.Exclude, pattern)
		}
	}
	return scope
}

// Path reports whether a repository-relative path is product work here: graded, so
// agent configuration and the planning artifacts stay out, outside the planning
// trees odd/ and openspec/, and matched by none of the project's exclusions.
func (s ProductScope) Path(rel string) bool {
	if rel == "" || rel == "." || !GradedPath(rel) {
		return false
	}
	for _, tree := range []string{"odd", "openspec"} {
		if rel == tree || strings.HasPrefix(rel, tree+"/") {
			return false
		}
	}
	for _, pattern := range s.Exclude {
		if productGlobMatches(pattern, rel) {
			return false
		}
	}
	return true
}

// productGlobValid reports whether one exclusion is a usable path glob: `**` is any
// number of segments, everything else is a shell-style match of one segment.
func productGlobValid(pattern string) bool {
	if pattern == "" {
		return false
	}
	for _, segment := range strings.Split(pattern, "/") {
		switch segment {
		case "**":
			continue
		case "":
			return false
		}
		if _, err := path.Match(segment, "x"); err != nil {
			return false
		}
	}
	return true
}

// productGlobMatches matches one repository-relative path against one exclusion.
func productGlobMatches(pattern, rel string) bool {
	return productSegmentsMatch(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

// productSegmentsMatch walks the two segment lists, letting `**` consume any number
// of them.
func productSegmentsMatch(pattern, rel []string) bool {
	if len(pattern) == 0 {
		return len(rel) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(rel); i++ {
			if productSegmentsMatch(pattern[1:], rel[i:]) {
				return true
			}
		}
		return false
	}
	if len(rel) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], rel[0])
	return err == nil && matched && productSegmentsMatch(pattern[1:], rel[1:])
}
