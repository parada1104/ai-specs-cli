package shared

import (
	"encoding/json"
	"reflect"
	"testing"
)

// reconcileStampOf builds one acquired stamp source for the decision tests;
// values mirror what the JSON acquisition seam decodes (UseNumber keeps TOML
// integers as json.Number on the real path).
func reconcileStampOf(reconcile, fields map[string]any) ReconcileStampSource {
	return ReconcileStampSource{Reconcile: reconcile, Fields: fields}
}

// TestBuildReconcileStamp mirrors the per-recipe decision of Python's
// stamp_recipe_reconcile_defaults: the stamp carries the raw declared
// [config.reconcile] table plus the declared default of every used field name
// — the scope field and each table-shaped expectation's config_field and
// config_field_when_set, empty strings discarded — and only when that default
// is present. TOML false and 0 are present values and MUST stamp (Python
// checks `default is not None`, not truthiness); an absent default or an
// unknown field name is not stamped.
func TestBuildReconcileStamp(t *testing.T) {
	cases := []struct {
		name   string
		source ReconcileStampSource
		want   map[string]any
	}{
		{
			name: "scope_field and expectations select declared defaults",
			source: reconcileStampOf(
				map[string]any{
					"scope_field": "workflow",
					"expectations": []any{
						map[string]any{"config_field": "base_branch", "config_field_when_set": "feature_branch"},
					},
				},
				map[string]any{
					"workflow":       "feature",
					"base_branch":    "development",
					"feature_branch": false,
					"unrelated":      "never-referenced",
				},
			),
			want: map[string]any{
				"reconcile": map[string]any{
					"scope_field": "workflow",
					"expectations": []any{
						map[string]any{"config_field": "base_branch", "config_field_when_set": "feature_branch"},
					},
				},
				"workflow":       "feature",
				"base_branch":    "development",
				"feature_branch": false,
			},
		},
		{
			name: "boolean false and zero defaults stamp",
			source: reconcileStampOf(
				map[string]any{
					"scope_field": "enabled_field",
					"expectations": []any{
						map[string]any{"config_field": "count_field"},
					},
				},
				map[string]any{"enabled_field": false, "count_field": 0},
			),
			want: map[string]any{
				"reconcile": map[string]any{
					"scope_field": "enabled_field",
					"expectations": []any{
						map[string]any{"config_field": "count_field"},
					},
				},
				"enabled_field": false,
				"count_field":   0,
			},
		},
		{
			name: "absent default and unknown field are not stamped",
			source: reconcileStampOf(
				map[string]any{
					"scope_field": "declared_without_default",
					"expectations": []any{
						map[string]any{"config_field": "ghost_field"},
					},
				},
				map[string]any{"declared_with_default": "unused"},
			),
			want: map[string]any{
				"reconcile": map[string]any{
					"scope_field": "declared_without_default",
					"expectations": []any{
						map[string]any{"config_field": "ghost_field"},
					},
				},
			},
		},
		{
			name: "non-table expectation entries are ignored",
			source: reconcileStampOf(
				map[string]any{
					"expectations": []any{
						"not-a-table",
						5,
						map[string]any{"config_field": "real_field"},
					},
				},
				map[string]any{"real_field": "yes", "not-a-table": "no", "5": "no"},
			),
			want: map[string]any{
				"reconcile": map[string]any{
					"expectations": []any{
						"not-a-table",
						5,
						map[string]any{"config_field": "real_field"},
					},
				},
				"real_field": "yes",
			},
		},
		{
			name: "empty used set stamps only the reconcile table",
			source: reconcileStampOf(
				map[string]any{"max_age_seconds": 3600},
				map[string]any{"workflow": "feature"},
			),
			want: map[string]any{
				"reconcile": map[string]any{"max_age_seconds": 3600},
			},
		},
		{
			name: "empty-string field names are discarded",
			source: reconcileStampOf(
				map[string]any{"scope_field": "", "expectations": []any{map[string]any{"config_field": "", "config_field_when_set": ""}}},
				map[string]any{"": "would-be-stamped"},
			),
			want: map[string]any{
				"reconcile": map[string]any{"scope_field": "", "expectations": []any{map[string]any{"config_field": "", "config_field_when_set": ""}}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildReconcileStamp(tc.source)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("stamp = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestReconcileStampPlanJSONShape pins the exact stdout contract the later
// Python bridge consumes: one JSON object with an ordered stamps list whose
// entries carry the recipe id and the stamp dict.
func TestReconcileStampPlanJSONShape(t *testing.T) {
	plan := ReconcileStampPlan{Stamps: []ReconcileStampEntry{
		{ID: "dir-a", Stamp: map[string]any{
			"reconcile":    map[string]any{"scope_field": "workflow"},
			"workflow":     "feature",
			"feature_flag": false,
		}},
	}}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"stamps":[{"id":"dir-a","stamp":{"feature_flag":false,"reconcile":{"scope_field":"workflow"},"workflow":"feature"}}]}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}
