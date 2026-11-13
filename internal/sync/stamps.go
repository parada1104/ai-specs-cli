package sync

import (
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/worktree-gate/shared"
)

// PlanReconcileStamps acquires each enabled recipe's declared
// [config.reconcile] table and its declared config-field defaults, then plans
// one stamp per recipe with the shared planner. It mirrors the Python stamp
// authority (_stamp_recipe_reconcile_defaults_python at
// lib/_internal/recipe-materialize.py:407-415), which catches every read
// failure and continues: a missing, unreadable, invalid, or structurally
// invalid recipe.toml, and a recipe without a [config.reconcile] table, all
// produce no stamp. A field default is acquired only when the declared default
// is present (Python's `field.default is not None`), so TOML false and 0
// survive while an absent default does not. Repeated ids dedupe on the first
// occurrence so first-seen enabled order is stable.
func PlanReconcileStamps(catalogDir string, recipeIDs []string) []shared.ReconcileStampEntry {
	out := []shared.ReconcileStampEntry{}
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		recipe, err := schema.LoadRecipeToml(filepath.Join(catalogDir, rid, "recipe.toml"))
		if err != nil {
			continue
		}
		declared, ok := recipe.ConfigSchema.Tables["reconcile"]
		if !ok || declared == nil {
			continue
		}
		source := shared.ReconcileStampSource{
			Reconcile: plainReconcileValues(declared.Values),
			Fields:    declaredFieldDefaults(recipe),
		}
		out = append(out, shared.ReconcileStampEntry{ID: rid, Stamp: shared.BuildReconcileStamp(source)})
	}
	return out
}

// plainReconcileValues normalizes the declared [config.reconcile] values one
// field at a time. The raw values arrive as *toml.Table and []*toml.Table, and
// toml.PlainValue never descends into an arbitrary map: normalizing the
// enclosing Values map would hand the shared planner raw tables, whose
// expectations type assertion fails and silently drops every used-field
// default. The same per-field rule applies to each declared field default
// below (a table or array-of-tables default must reach the planner plain).
func plainReconcileValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for name, value := range values {
		out[name] = toml.PlainValue(value)
	}
	return out
}

// declaredFieldDefaults collects every recognized config field whose declared
// default is present. The shared planner narrows this map to the field names
// the declared reconcile mapping actually uses.
func declaredFieldDefaults(recipe *schema.Recipe) map[string]any {
	fields := map[string]any{}
	for name, field := range recipe.ConfigSchema.Fields {
		if field != nil && field.Default != nil {
			fields[name] = toml.PlainValue(field.Default)
		}
	}
	return fields
}
