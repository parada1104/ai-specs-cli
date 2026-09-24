package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"testing"
)

// reconcileStampOf builds one acquired stamp source for the decision tests;
// values mirror what the JSON acquisition seam decodes (UseNumber keeps TOML
// integers as json.Number on the real path).
func reconcileStampOf(reconcile, fields map[string]any) reconcileStampSource {
	return reconcileStampSource{Reconcile: reconcile, Fields: fields}
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
		source reconcileStampSource
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
			got := buildReconcileStamp(tc.source)
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
	plan := reconcileStampPlan{Stamps: []reconcileStampEntry{
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

// TestLoadReconcileStampSources acquires the reconcile stamp inputs through the
// bounded standard-library TOML seam. A recipe whose recipe.toml is missing,
// unparseable, has no [config] table or no [config.reconcile] table is omitted
// — the Python authority silently continues for the same shapes — while the
// remaining recipes keep their enabled order. Repeated ids acquire once.
func TestLoadReconcileStampSources(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", `
[config]

[config.reconcile]
scope_field = "workflow"

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = false
`)
	writeRecipeToml(t, catalogDir, "dir-b", `
[config]

[config.other]
required = true
default = "no reconcile table"
`)
	writeRecipeToml(t, catalogDir, "dir-broken", "not = [valid\n")
	writeRecipeToml(t, catalogDir, "dir-noconfig", "[recipe]\nid = \"x\"\n")

	acquired, err := loadReconcileStampSources(catalogDir, []string{"dir-b", "dir-a", "dir-broken", "ghost", "dir-a"})
	if err != nil {
		t.Fatalf("loadReconcileStampSources: %v", err)
	}
	if len(acquired) != 1 || acquired[0].RecipeID != "dir-a" {
		t.Fatalf("acquired = %#v, want only dir-a in enabled order", acquired)
	}
	want := reconcileStampSource{
		Reconcile: map[string]any{
			"scope_field": "workflow",
			"expectations": []any{
				map[string]any{"event": "pr_opened", "config_field": "base_branch"},
			},
		},
		Fields: map[string]any{"workflow": "feature", "base_branch": false},
	}
	if !reflect.DeepEqual(acquired[0].Source, want) {
		t.Fatalf("source = %#v, want %#v", acquired[0].Source, want)
	}
}

// TestRunPlanReconcileStampsCommand exercises the flag wiring end to end: the
// command prints one ordered JSON envelope and exits 0 even when recipes are
// skipped (no reconcile table, unreadable, missing) — the Python authority's
// silent-continue semantics — while unusable flags or a broken parser exit 2
// so the calling bridge can fall back to the Python authority. It runs the
// real TOML acquisition subprocess, so it is skipped in short mode and
// without python3.
func TestRunPlanReconcileStampsCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("command test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", `
[config]

[config.reconcile]
scope_field = "workflow"
max_age_seconds = 3600

[[config.reconcile.expectations]]
event = "pr_opened"
property = "branch"
config_field = "base_branch"
config_field_when_set = "feature_branch"

[[config.reconcile.expectations]]
event = "card_moved"
config_field = "no_default_field"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = "development"

[config.feature_branch]
required = true
default = false

[config.no_default_field]
required = true

[config.unused]
required = true
default = "never-referenced"
`)
	writeRecipeToml(t, catalogDir, "dir-b", `
[config]

[config.other]
required = true
default = "no reconcile table"
`)
	writeRecipeToml(t, catalogDir, "dir-c", `
[config]

[config.reconcile]
scope_field = "alpha"

[config.alpha]
required = true
default = 0
`)
	writeRecipeToml(t, catalogDir, "dir-broken", "not = [valid\n")

	t.Run("missing catalog dir exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--plan-reconcile-stamps")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("envelope is ordered and skips silently", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--plan-reconcile-stamps",
			"--catalog-dir", catalogDir,
			"--recipe", "dir-c", "--recipe", "dir-b", "--recipe", "dir-broken",
			"--recipe", "ghost", "--recipe", "dir-a")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		// Stamp map keys marshal in sorted order; the stamps list follows the
		// enabled-recipe order (dir-c before dir-a), not catalog or skip order.
		want := `{"stamps":[` +
			`{"id":"dir-c","stamp":{"alpha":0,"reconcile":{"scope_field":"alpha"}}},` +
			`{"id":"dir-a","stamp":{"base_branch":"development","feature_branch":false,"reconcile":{"expectations":[{"config_field":"base_branch","config_field_when_set":"feature_branch","event":"pr_opened","property":"branch"},{"config_field":"no_default_field","event":"card_moved"}],"max_age_seconds":3600,"scope_field":"workflow"},"workflow":"feature"}}` +
			`]}`
		if stdout != want+"\n" {
			t.Fatalf("stdout = %q, want %q", stdout, want+"\n")
		}
	})

	t.Run("every recipe skipped still emits an empty list and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--plan-reconcile-stamps",
			"--catalog-dir", catalogDir, "--recipe", "dir-b", "--recipe", "dir-broken", "--recipe", "ghost")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if stdout != "{\"stamps\":[]}\n" {
			t.Fatalf("stdout = %q, want an empty stamps list", stdout)
		}
	})
}
