package shared

// Per-recipe reconcile stamp planning, moved from the Python authority
// (lib/_internal/recipe-materialize.py stamp_recipe_reconcile_defaults) with
// the same decision:
//
//   - ReconcileStampSource is one enabled recipe's acquired input: the raw
//     declared [config.reconcile] table and the declared defaults of its
//     recognized config fields (present default keys only).
//   - ReconcileStampEntry is one planned stamp, keyed by the enabled recipe id.
//   - ReconcileStampPlan is the single JSON object emitted on stdout. Stamping
//     is advisory planning with no write side effects: recipes the loader
//     could not read, or that declare no [config.reconcile] table, are
//     silently omitted (the Python authority's `continue`), and every planned
//     result exits 0. Only a process-level failure — unusable flags or an
//     unavailable parser — exits 2 so the calling bridge can fall back to the
//     Python authority.
//
// Moved verbatim from the gate main package (reconcilestamps.go) as part of
// slice SX0e: the pure planner is shared with the native route. Only the
// declaration names gained their exported form; the planner body, the JSON
// tags, and the planned output are unchanged. The flag surface
// (reconcileStampOptions) and the --plan-reconcile-stamps command stay in the
// gate main package.

// ReconcileStampSource is the acquired per-recipe stamp input. Reconcile holds
// the raw declared [config.reconcile] values; Fields maps each recognized
// config field name to its declared default (absent defaults are omitted by
// the acquisition seam, which is exactly Python's `field.default is not None`).
type ReconcileStampSource struct {
	Reconcile map[string]any `json:"reconcile"`
	Fields    map[string]any `json:"fields"`
}

// ReconcileStampEntry is one planned stamp in the stdout envelope.
type ReconcileStampEntry struct {
	ID    string         `json:"id"`
	Stamp map[string]any `json:"stamp"`
}

// ReconcileStampPlan is the stdout contract. Stamps is always present; an
// empty plan is an empty list, never null.
type ReconcileStampPlan struct {
	Stamps []ReconcileStampEntry `json:"stamps"`
}

// BuildReconcileStamp mirrors the per-recipe stamp decision: the stamp carries
// the raw declared reconcile table plus the declared default of every used
// field name. Used names are the non-empty scope_field and, for every
// table-shaped expectations entry, the non-empty config_field and
// config_field_when_set — exactly the names Python's used set holds after
// discarding empty strings. A used name stamps only when the acquisition seam
// produced a declared default for it (present `default` key), so TOML false
// and 0 stamp while an absent default or an unknown field name does not.
// Malformed expectations entries (non-tables) contribute nothing: the Python
// authority only consumes dict-shaped expectations.
func BuildReconcileStamp(source ReconcileStampSource) map[string]any {
	stamp := map[string]any{"reconcile": source.Reconcile}
	used := map[string]bool{}
	if scope, ok := source.Reconcile["scope_field"].(string); ok && scope != "" {
		used[scope] = true
	}
	expectations, _ := source.Reconcile["expectations"].([]any)
	for _, expectation := range expectations {
		table, ok := expectation.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"config_field", "config_field_when_set"} {
			if field, ok := table[key].(string); ok && field != "" {
				used[field] = true
			}
		}
	}
	for name := range used {
		if declared, present := source.Fields[name]; present {
			stamp[name] = declared
		}
	}
	return stamp
}
