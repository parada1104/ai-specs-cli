package sync

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/worktree-gate/shared"
)

// writeSyncCatalogRecipe writes <catalogDir>/<id>/recipe.toml. A broken body
// pins each seam's boundary: the stamp planner silently skips every load
// failure, while the tag planner skips only a path that is not a regular file
// and propagates every other loader error.
func writeSyncCatalogRecipe(t *testing.T, catalogDir, id, body string) {
	t.Helper()
	dir := filepath.Join(catalogDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// syncRecipeHeader is the minimal valid [recipe] table every fixture needs.
func syncRecipeHeader(id, name string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"" + name + "\"\ndescription = \"d\"\nversion = \"1.0.0\"\n"
}

// TestReconcileStampsDifferentialAgainstPython pins PlanReconcileStamps against
// the frozen output of the Python stamp authority for the same fixture catalog:
// dir-a carries a false default and a zero default selected by one
// table-shaped expectation each, dir-b carries a single scope default, and the
// ghost id and the unparsable dir-broken must be skipped by both authorities.
// Byte equality of the compact JSON pins the recipe ids, the order, the
// reconcile values and every selected default — including TOML false and 0.
func TestReconcileStampsDifferentialAgainstPython(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "dir-a", syncRecipeHeader("dir-a", "A")+`
[config.reconcile]
scope_field = "workflow"
max_age_seconds = 3600

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"

[[config.reconcile.expectations]]
event = "card_moved"
config_field_when_set = "max_retries"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = false

[config.max_retries]
required = true
default = 0

[config.unused]
required = true
default = "never"
`)
	writeSyncCatalogRecipe(t, catalog, "dir-b", syncRecipeHeader("dir-b", "B")+`
[config.reconcile]
scope_field = "phase"

[config.phase]
required = true
default = "build"
`)
	writeSyncCatalogRecipe(t, catalog, "dir-broken", "not = [valid\n")

	ids := []string{"dir-a", "dir-b", "ghost", "dir-broken"}
	got, err := json.Marshal(PlanReconcileStamps(catalog, ids))
	if err != nil {
		t.Fatalf("marshal native plan: %v", err)
	}
	frozen, err := os.ReadFile(filepath.Join("testdata", "stamps_golden.json"))
	if err != nil {
		t.Fatalf("read frozen oracle: %v", err)
	}
	want := strings.TrimSpace(string(frozen))
	// Non-vacuity: the oracle really stamped both readable recipes and kept the
	// TOML false and 0 defaults, so the comparison below cannot pass on an empty
	// pair of plans.
	for _, marker := range []string{`"id":"dir-a"`, `"id":"dir-b"`, `"base_branch":false`, `"max_retries":0`} {
		if !strings.Contains(want, marker) {
			t.Fatalf("oracle plan is missing %s: %s", marker, want)
		}
	}
	if string(got) != want {
		t.Fatalf("stamp plan diverges\n--- go ---\n%s\n--- python ---\n%s", got, want)
	}
}

// TestPlanReconcileStampsAcquisition pins the native reconcile-stamp seam.
// Every used config field keeps its declared default — including TOML false
// and 0 — while unused fields and absent defaults stay out of the stamp. The
// expectations entry arrives as a raw TOML array-of-tables, so a seam that
// feeds the shared planner without per-field normalization collects no used
// field names and drops every used default: this is the call-site regression
// SX0e requested.
func TestPlanReconcileStampsAcquisition(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "dir-a", syncRecipeHeader("dir-a", "A")+`
[config.reconcile]
scope_field = "workflow"
max_age_seconds = 3600

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"

[[config.reconcile.expectations]]
event = "card_moved"
config_field_when_set = "max_retries"

[[config.reconcile.expectations]]
event = "card_closed"
config_field = "no_default_field"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = false

[config.max_retries]
required = true
default = 0

[config.no_default_field]
required = true

[config.unused]
required = true
default = "never"
`)
	writeSyncCatalogRecipe(t, catalog, "dir-b", syncRecipeHeader("dir-b", "B")+`
[config.reconcile]
scope_field = "phase"

[config.phase]
required = true
default = "build"
`)
	writeSyncCatalogRecipe(t, catalog, "dir-no-config", syncRecipeHeader("dir-no-config", "No Config"))
	writeSyncCatalogRecipe(t, catalog, "dir-broken", "not = [valid\n")

	dirA := shared.ReconcileStampEntry{
		ID: "dir-a",
		Stamp: map[string]any{
			"reconcile": map[string]any{
				"scope_field":     "workflow",
				"max_age_seconds": int64(3600),
				"expectations": []any{
					map[string]any{"event": "pr_opened", "config_field": "base_branch"},
					map[string]any{"event": "card_moved", "config_field_when_set": "max_retries"},
					map[string]any{"event": "card_closed", "config_field": "no_default_field"},
				},
			},
			"workflow":    "feature",
			"base_branch": false,
			"max_retries": int64(0),
		},
	}
	dirB := shared.ReconcileStampEntry{
		ID: "dir-b",
		Stamp: map[string]any{
			"reconcile": map[string]any{"scope_field": "phase"},
			"phase":     "build",
		},
	}

	t.Run("used defaults survive and false zero absent and unused stay correct", func(t *testing.T) {
		got := PlanReconcileStamps(catalog, []string{"dir-a"})
		if !reflect.DeepEqual(got, []shared.ReconcileStampEntry{dirA}) {
			t.Fatalf("PlanReconcileStamps = %#v, want %#v", got, []shared.ReconcileStampEntry{dirA})
		}
	})

	t.Run("enabled order is preserved and repeated ids acquire once", func(t *testing.T) {
		got := PlanReconcileStamps(catalog, []string{"dir-b", "dir-a", "dir-b"})
		if !reflect.DeepEqual(got, []shared.ReconcileStampEntry{dirB, dirA}) {
			t.Fatalf("PlanReconcileStamps = %#v, want [dir-b dir-a]", got)
		}
	})

	t.Run("skipped recipes leave an empty JSON list", func(t *testing.T) {
		got := PlanReconcileStamps(catalog, []string{"ghost", "dir-no-config", "dir-broken"})
		if got == nil {
			t.Fatal("PlanReconcileStamps returned nil; the plan list must never be null")
		}
		if b, err := json.Marshal(got); err != nil || string(b) != "[]" {
			t.Fatalf("empty plan JSON = %s (%v), want []", b, err)
		}
	})
}

// TestStampReconcileDefaultsWarnsPerRecipeAndContinues pins the native writer
// failure contract: a manifest write that refuses for ONE recipe warns once,
// verbatim, naming the recipe by its [recipe].name, and the remaining planned
// stamps still land. The stamp step never fails the sync. This is the native
// twin of the retired Python bridge's per-recipe writer-failure case (an
// injected OSError there; here the writer's own multiline refusal for beta).
func TestStampReconcileDefaultsWarnsPerRecipeAndContinues(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", materializeReconcileRecipe("alpha", "Alpha"))
	writeSyncCatalogRecipe(t, catalog, "beta", materializeReconcileRecipe("beta", "Beta"))

	manifestPath := filepath.Join(t.TempDir(), "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// beta's stamped `reconcile` key already holds a multiline value, which the
	// shared writer refuses to replace; alpha is a plain first stamp.
	seed := "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n\n" +
		"[recipes.beta.config]\nreconcile = \"\"\"stale\nmultiline\n\"\"\"\n"
	if err := os.WriteFile(manifestPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := stampReconcileDefaults(manifestPath, catalog, []string{"alpha", "beta"}, &stderr); err != nil {
		t.Fatalf("a per-recipe writer failure must never fail the stamp step: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("the stamp step prints nothing on stdout, got %q", stdout.String())
	}
	wantWarning := "  ! recipe 'Beta': reconcile defaults not stamped (RuntimeError: cannot replace multiline value for key 'reconcile')\n"
	if got := stderr.String(); got != wantWarning {
		t.Errorf("stderr = %q, want %q", got, wantWarning)
	}

	final, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), "reconcile = \"\"\"stale\nmultiline\n\"\"\"") {
		t.Errorf("the refused recipe's own bytes must survive untouched:\n%s", final)
	}
	manifest, err := toml.Parse(final)
	if err != nil {
		t.Fatalf("parse stamped manifest: %v", err)
	}
	recipes, _ := manifest.Table("recipes")
	alpha, _ := recipes.Table("alpha")
	config, _ := alpha.Table("config")
	if got, _ := config.Get("workflow"); got != "feature" {
		t.Errorf("alpha workflow = %v, want the surviving stamp's feature", got)
	}
	if got, _ := config.Get("base_branch"); got != "main" {
		t.Errorf("alpha base_branch = %v, want the surviving stamp's main", got)
	}
}
