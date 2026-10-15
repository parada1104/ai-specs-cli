package sync

// Catalog enrichment of the resolved-config document (GO-07 S14b).
//
// Ports merge_catalog_defaults_into_resolved and attach_brief_fragments_to_resolved
// (lib/_internal/recipe-materialize.py:198-291). Python applies both, in that
// order, only to the main resolved-config dump (:4123-4124) and to the
// resolved-config-only mode (:4194-4195), never to the no-enabled branch. Both
// are best-effort: a recipe whose catalog recipe.toml does not load is skipped
// silently, exactly like Python's `except Exception: pass`.

import (
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/schema"
)

// enrichResolvedConfig merges catalog [config] defaults (manifest values win)
// and then attaches [provides.brief] fragments for every enabled recipe. It
// works on the ordered document AFTER the projection's JSON round trip: the
// projection is the Go gate envelope Python decodes (so a manifest 1.0 is 1
// there too), while the catalog values come from the Go recipe loader, whose
// value types match tomllib's (a default 2.0 stays 2.0, a wide integer keeps
// its digits).
func enrichResolvedConfig(doc *mcpObj, enabled []string, catalogDir string) {
	mergeCatalogDefaultsIntoResolved(doc, enabled, catalogDir)
	attachBriefFragmentsToResolved(doc, enabled, catalogDir)
}

// mergeCatalogDefaultsIntoResolved is merge_catalog_defaults_into_resolved
// (lib/_internal/recipe-materialize.py:245): for every enabled recipe, every
// catalog [config] default the manifest does not already carry is filled in. A
// recipe whose recipe.toml does not load is skipped silently, exactly like
// Python's per-recipe `except Exception: pass`.
func mergeCatalogDefaultsIntoResolved(doc *mcpObj, enabled []string, catalogDir string) {
	recipes := resolvedRecipesDoc(doc)
	for _, rid := range enabled {
		recipe := catalogRecipe(catalogDir, rid)
		if recipe == nil || recipe.ConfigSchema == nil || len(recipe.ConfigSchema.Fields) == 0 {
			continue
		}
		existing := resolvedRecipeEntry(recipes, rid)
		for key, field := range recipe.ConfigSchema.Fields {
			if field == nil || field.Default == nil {
				continue
			}
			if _, present := existing.get(key); !present {
				existing.set(key, mcpToOrdered(field.Default))
			}
		}
	}
}

// attachBriefFragmentsToResolved is attach_brief_fragments_to_resolved
// (lib/_internal/recipe-materialize.py:224): every enabled recipe gets its
// catalog [provides.brief] fragments attached, `{}` when it declares none. A
// recipe whose recipe.toml does not load is skipped silently.
func attachBriefFragmentsToResolved(doc *mcpObj, enabled []string, catalogDir string) {
	recipes := resolvedRecipesDoc(doc)
	for _, rid := range enabled {
		recipe := catalogRecipe(catalogDir, rid)
		if recipe == nil {
			continue
		}
		resolvedRecipeEntry(recipes, rid).set("brief_fragments", resolvedFragmentsJSON(recipe.BriefFragments))
	}
}

// resolvedRecipesDoc is Python's `resolved.setdefault("recipes", {})`: the
// document's recipes object, created when the projection left it absent or
// non-object.
func resolvedRecipesDoc(doc *mcpObj) *mcpObj {
	recipesValue, _ := doc.get("recipes")
	recipes, ok := recipesValue.(*mcpObj)
	if !ok || recipes == nil {
		recipes = newMCPObj()
		doc.set("recipes", recipes)
	}
	return recipes
}

// resolvedRecipeEntry is the recipe's entry object, created when absent or
// non-object (Python's `recipes.setdefault(rid, {})`).
func resolvedRecipeEntry(recipes *mcpObj, rid string) *mcpObj {
	value, _ := recipes.get(rid)
	existing, ok := value.(*mcpObj)
	if !ok || existing == nil {
		existing = newMCPObj()
		recipes.set(rid, existing)
	}
	return existing
}

// catalogRecipe loads one catalog recipe; nil when it does not load, exactly
// like the Python callers' `except Exception: pass`.
func catalogRecipe(catalogDir, rid string) *schema.Recipe {
	recipe, err := schema.LoadRecipeToml(filepath.Join(catalogDir, rid, "recipe.toml"))
	if err != nil {
		return nil
	}
	return recipe
}

// resolvedFragmentsJSON mirrors _fragments_to_json: {} for no brief block, and
// only the declared sections, each fragment as {"key": key|null, "text": text}.
func resolvedFragmentsJSON(bf *schema.BriefFragments) *mcpObj {
	out := newMCPObj()
	if bf == nil {
		return out
	}
	sections := []struct {
		name  string
		frags []schema.BriefFragment
	}{
		{"runtime_flow", bf.RuntimeFlow},
		{"context_sources", bf.ContextSources},
		{"conflict_policy", bf.ConflictPolicy},
		{"workflow_rules", bf.WorkflowRules},
		{"useful_commands", bf.UsefulCommands},
		{"mcp_descriptions", bf.McpDescriptions},
	}
	for _, section := range sections {
		if section.frags == nil {
			continue
		}
		list := make([]any, 0, len(section.frags))
		for _, frag := range section.frags {
			item := newMCPObj()
			item.set("key", mcpToOrdered(frag.Key))
			item.set("text", mcpToOrdered(frag.Text))
			list = append(list, item)
		}
		out.set(section.name, list)
	}
	return out
}
