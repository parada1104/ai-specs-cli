package main

import (
	"os/exec"
	"reflect"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

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
	want := shared.ReconcileStampSource{
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
