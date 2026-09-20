package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// runPlanCLI invokes run() with the given stdin and returns exit code, stdout
// and stderr, so the whole --plan-orphans flag/exit contract is testable
// in-process without touching the filesystem.
func runPlanCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// emptyOrphanPlan is the all-empty result every scope must carry when nothing
// is orphaned. It is a helper so the "not orphaned" cases stay readable.
func emptyOrphanPlan() orphanPlan {
	return orphanPlan{
		OrphanedRecipes:       []string{},
		OrphanedDeps:          []string{},
		OrphanedInprojectDeps: []string{},
		StaleLockRecipes:      []string{},
	}
}

func TestPlanOrphansSemantics(t *testing.T) {
	tests := []struct {
		name string
		in   orphanPlanInput
		want orphanPlan
	}{
		{
			name: "orphan in every scope",
			in: orphanPlanInput{
				RecipeSkills:     []string{"a", "b"},
				DepsSkills:       []string{"x", "y"},
				InprojectDeps:    []string{"p", "q"},
				LockRecipes:      []string{"a", "z"},
				EnabledRecipeIDs: []string{"a"},
				ExpectedDepIDs:   []string{"x", "p"},
			},
			want: orphanPlan{
				OrphanedRecipes:       []string{"b"},
				OrphanedDeps:          []string{"y"},
				OrphanedInprojectDeps: []string{"q"},
				StaleLockRecipes:      []string{"z"},
			},
		},
		{
			name: "nothing orphaned when every id is expected",
			in: orphanPlanInput{
				RecipeSkills:     []string{"a"},
				DepsSkills:       []string{"x"},
				InprojectDeps:    []string{"y"},
				LockRecipes:      []string{"a"},
				EnabledRecipeIDs: []string{"a"},
				ExpectedDepIDs:   []string{"x", "y"},
			},
			want: emptyOrphanPlan(),
		},
		{
			name: "empty inputs yield an empty plan",
			in:   orphanPlanInput{},
			want: emptyOrphanPlan(),
		},
		{
			name: "stale lock recipe not enabled",
			in: orphanPlanInput{
				LockRecipes:      []string{"keep", "stale"},
				EnabledRecipeIDs: []string{"keep"},
			},
			want: orphanPlan{
				OrphanedRecipes:       []string{},
				OrphanedDeps:          []string{},
				OrphanedInprojectDeps: []string{},
				StaleLockRecipes:      []string{"stale"},
			},
		},
		{
			name: "output is sorted and deduplicated",
			in: orphanPlanInput{
				RecipeSkills:     []string{"c", "a", "b", "a"},
				DepsSkills:       []string{"z", "m", "m"},
				InprojectDeps:    []string{"q", "n"},
				LockRecipes:      []string{"r", "d"},
				EnabledRecipeIDs: nil,
				ExpectedDepIDs:   nil,
			},
			want: orphanPlan{
				OrphanedRecipes:       []string{"a", "b", "c"},
				OrphanedDeps:          []string{"m", "z"},
				OrphanedInprojectDeps: []string{"n", "q"},
				StaleLockRecipes:      []string{"d", "r"},
			},
		},
		{
			name: "in-project dep orphans independent of cached dep skills",
			in: orphanPlanInput{
				DepsSkills:     []string{"cached"},
				InprojectDeps:  []string{"inproject"},
				ExpectedDepIDs: []string{"cached"},
			},
			want: orphanPlan{
				OrphanedRecipes:       []string{},
				OrphanedDeps:          []string{},
				OrphanedInprojectDeps: []string{"inproject"},
				StaleLockRecipes:      []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planOrphans(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planOrphans() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPlanOrphansCLIEmitsEnvelope(t *testing.T) {
	stdin := `{"recipe_skills":["a","b"],"deps_skills":["x"],"inproject_deps":["y"],` +
		`"lock_recipes":["a","z"],"enabled_recipe_ids":["a"],"expected_dep_ids":["x"]}`
	code, stdout, stderr := runPlanCLI(t, stdin, "--plan-orphans")
	if code != 0 {
		t.Fatalf("--plan-orphans exit = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("--plan-orphans stderr = %q, want empty", stderr)
	}
	var got orphanPlan
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--plan-orphans stdout is not JSON: %v (%q)", err, stdout)
	}
	want := orphanPlan{
		OrphanedRecipes:       []string{"b"},
		OrphanedDeps:          []string{},
		OrphanedInprojectDeps: []string{"y"},
		StaleLockRecipes:      []string{"z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("--plan-orphans = %#v, want %#v", got, want)
	}
	// Every field must be emitted as a list, never as null.
	if strings.Contains(stdout, "null") {
		t.Fatalf("--plan-orphans emitted a null list: %q", stdout)
	}
}

func TestPlanOrphansCLIEmptyStdin(t *testing.T) {
	code, stdout, stderr := runPlanCLI(t, "", "--plan-orphans")
	if code != 0 {
		t.Fatalf("--plan-orphans empty stdin exit = %d, want 0; stderr: %s", code, stderr)
	}
	for _, field := range []string{
		"orphaned_recipes", "orphaned_deps",
		"orphaned_inproject_deps", "stale_lock_recipes",
	} {
		if !strings.Contains(stdout, `"`+field+`":[]`) {
			t.Fatalf("--plan-orphans empty stdin stdout = %q, want %q empty", stdout, field)
		}
	}
}

func TestPlanOrphansCLIMalformedJSON(t *testing.T) {
	code, stdout, stderr := runPlanCLI(t, "{not json", "--plan-orphans")
	if code != 2 {
		t.Fatalf("--plan-orphans malformed input exit = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("--plan-orphans malformed input stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "plan-orphans") {
		t.Fatalf("--plan-orphans malformed input stderr = %q, want a plan-orphans diagnostic", stderr)
	}
}
