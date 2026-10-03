package sync

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// This file is the SX0a in-process import evidence: the root single binary
// reaches the gate's authoritative orphans decision through the importable
// shared package (ai-specs.dev/worktree-gate/shared) instead of shelling out
// to the gate binary. The go.mod require/replace wiring makes the nested
// gate module resolvable without a published release.

// TestSharedOrphansPlanContract pins the plan envelope contract from the
// root side: same decision, same JSON field names, lists never null.
func TestSharedOrphansPlanContract(t *testing.T) {
	in := shared.OrphanPlanInput{
		RecipeSkills:     []string{"c", "a", "b", "a"},
		DepsSkills:       []string{"z", "m", "m"},
		InprojectDeps:    []string{"q", "n"},
		LockRecipes:      []string{"r", "d"},
		EnabledRecipeIDs: nil,
		ExpectedDepIDs:   nil,
	}
	got := shared.PlanOrphans(in)
	want := shared.OrphanPlan{
		OrphanedRecipes:       []string{"a", "b", "c"},
		OrphanedDeps:          []string{"m", "z"},
		OrphanedInprojectDeps: []string{"n", "q"},
		StaleLockRecipes:      []string{"d", "r"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanOrphans() = %#v, want %#v", got, want)
	}
}

// TestSharedRunPlanOrphansEnvelope drives the full command surface
// in-process: stdin envelope in, one JSON object out, exit 0/2 semantics.
func TestSharedRunPlanOrphansEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := `{"recipe_skills":["a","b"],"enabled_recipe_ids":["a"]}`
	code := shared.RunPlanOrphans(strings.NewReader(stdin), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunPlanOrphans exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("RunPlanOrphans stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	for _, field := range []string{
		"orphaned_recipes", "orphaned_deps",
		"orphaned_inproject_deps", "stale_lock_recipes",
	} {
		if !strings.Contains(out, `"`+field+`":`) {
			t.Fatalf("RunPlanOrphans stdout = %q, want field %q", out, field)
		}
	}
	if strings.Contains(out, "null") {
		t.Fatalf("RunPlanOrphans emitted a null list: %q", out)
	}
}

// TestSharedRunPlanOrphansMalformedInput pins the process-level failure
// contract: malformed JSON is exit 2 with a diagnostic on stderr and an
// empty stdout.
func TestSharedRunPlanOrphansMalformedInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := shared.RunPlanOrphans(strings.NewReader("{not json"), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("RunPlanOrphans malformed input exit = %d, want 2", code)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty on malformed input", stdout.String())
	}
	if !strings.Contains(stderr.String(), "plan-orphans") {
		t.Fatalf("stderr = %q, want a plan-orphans diagnostic", stderr.String())
	}
}

// TestSharedSortedUniqueSemantics pins the shared helper the root side now
// consumes: sorted, deduplicated, stable order.
func TestSharedSortedUniqueSemantics(t *testing.T) {
	got := shared.SortedUnique([]string{"b", "a", "a", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedUnique = %#v, want %#v", got, want)
	}
	if got := shared.SortedUnique(nil); got == nil || len(got) != 0 {
		t.Fatalf("SortedUnique(nil) = %#v, want non-nil empty slice", got)
	}
}
