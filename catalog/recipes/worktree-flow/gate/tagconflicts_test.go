package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"testing"
)

// tagConflictOf builds one expected graded tag conflict matching the JSON
// contract emitted on stdout.
func tagConflictOf(tag, severity string, recipes ...string) tagConflict {
	return tagConflict{Type: "tag_conflict", Tag: tag, Recipes: recipes, Severity: severity}
}

// TestCheckTagConflicts mirrors recipe-conflicts.check_tag_conflicts: group the
// enabled recipes by tag in first-seen order, skip any tag held by fewer than two
// recipes or fewer than two distinct ids, and grade the overlap fatal only when a
// sharing recipe lists another sharing recipe in conflicts_with.
func TestCheckTagConflicts(t *testing.T) {
	cases := []struct {
		name    string
		recipes []recipeTagMetadata
		want    []tagConflict
	}{
		{
			name:    "shared tag without conflicts_with warns",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []tagConflict{tagConflictOf("vcs", "warning", "alpha", "beta")},
		},
		{
			name:    "one-sided conflicts_with is fatal",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}, ConflictsWith: []string{"beta"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []tagConflict{tagConflictOf("vcs", "fatal", "alpha", "beta")},
		},
		{
			name:    "conflicts_with is symmetric across the pair",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"vcs"}, ConflictsWith: []string{"alpha"}}},
			want:    []tagConflict{tagConflictOf("vcs", "fatal", "alpha", "beta")},
		},
		{
			name:    "conflicts_with naming a recipe outside the tag group stays a warning",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}, ConflictsWith: []string{"ghost"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []tagConflict{tagConflictOf("vcs", "warning", "alpha", "beta")},
		},
		{
			name:    "recipes sharing no tag produce no conflict",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"tracker"}}},
			want:    []tagConflict{},
		},
		{
			name:    "a single recipe holding a tag is not a conflict",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}},
			want:    []tagConflict{},
		},
		{
			name:    "one recipe listing the same tag twice is not a conflict",
			recipes: []recipeTagMetadata{{ID: "alpha", Tags: []string{"vcs", "vcs"}}},
			want:    []tagConflict{},
		},
		{
			name:    "three recipes sharing a tag report every id sorted",
			recipes: []recipeTagMetadata{{ID: "gamma", Tags: []string{"infra"}}, {ID: "alpha", Tags: []string{"infra"}}, {ID: "beta", Tags: []string{"infra"}}},
			want:    []tagConflict{tagConflictOf("infra", "warning", "alpha", "beta", "gamma")},
		},
		{
			name:    "empty input is an empty conflict list",
			recipes: nil,
			want:    []tagConflict{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkTagConflicts(tc.recipes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("checkTagConflicts() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestCheckTagConflictsTagOrderIsFirstSeen checks that conflict output follows
// the first-seen tag order rather than Go map iteration order.
func TestCheckTagConflictsTagOrderIsFirstSeen(t *testing.T) {
	got := checkTagConflicts([]recipeTagMetadata{
		{ID: "alpha", Tags: []string{"z-tag", "a-tag"}},
		{ID: "beta", Tags: []string{"z-tag", "a-tag"}},
	})
	wantTags := []string{"z-tag", "a-tag"}
	if len(got) != len(wantTags) {
		t.Fatalf("conflicts = %#v, want %d entries", got, len(wantTags))
	}
	for i, tag := range wantTags {
		if got[i].Tag != tag {
			t.Fatalf("conflict[%d].Tag = %q, want %q", i, got[i].Tag, tag)
		}
	}
}

// TestLoadRecipeTagMetadata acquires the top-level [recipe] id, tags and
// conflicts_with through the bounded standard-library TOML seam. The id is the
// TOML recipe id, never the catalog directory name, and a recipe whose [recipe]
// table is missing or malformed is unreadable and contributes nothing.
func TestLoadRecipeTagMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", `
[recipe]
id = "alpha"
name = "Alpha"
description = "first"
version = "1.0.0"
tags = ["vcs", "pr-flow"]
conflicts_with = ["beta"]
`)
	writeRecipeToml(t, catalogDir, "dir-b", `
[recipe]
id = "beta"
name = "Beta"
description = "second"
version = "1.0.0"
tags = ["vcs"]
`)
	writeRecipeToml(t, catalogDir, "no-recipe-table", `
tags = ["vcs"]
`)
	writeRecipeToml(t, catalogDir, "no-id", `
[recipe]
name = "No Id"
tags = ["vcs"]
`)
	writeRecipeToml(t, catalogDir, "bad-tags", `
[recipe]
id = "bad-tags"
tags = "not a list"
`)

	got, err := loadRecipeTagMetadata(catalogDir, []string{"dir-a", "dir-b", "no-recipe-table", "no-id", "bad-tags", "missing"})
	if err != nil {
		t.Fatalf("loadRecipeTagMetadata: %v", err)
	}
	want := []recipeTagMetadata{
		{ID: "alpha", Tags: []string{"vcs", "pr-flow"}, ConflictsWith: []string{"beta"}},
		{ID: "beta", Tags: []string{"vcs"}, ConflictsWith: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
}

// TestLoadRecipeTagMetadataOrderPreserved keeps the enabled order so first-seen
// tag ordering is reproducible downstream.
func TestLoadRecipeTagMetadataOrderPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\ntags = [\"vcs\"]\n")
	writeRecipeToml(t, catalogDir, "dir-b", "[recipe]\nid = \"beta\"\ntags = [\"vcs\"]\n")

	got, err := loadRecipeTagMetadata(catalogDir, []string{"dir-b", "dir-a"})
	if err != nil {
		t.Fatalf("loadRecipeTagMetadata: %v", err)
	}
	if len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("metadata = %#v, want beta then alpha", got)
	}
}

// TestTagConflictPlanJSONShape pins the exact stdout contract the later Python
// bridge consumes: one JSON object with a conflicts list.
func TestTagConflictPlanJSONShape(t *testing.T) {
	plan := tagConflictPlan{Conflicts: checkTagConflicts([]recipeTagMetadata{
		{ID: "alpha", Tags: []string{"vcs"}},
		{ID: "beta", Tags: []string{"vcs"}},
	})}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"conflicts":[{"type":"tag_conflict","tag":"vcs","recipes":["alpha","beta"],"severity":"warning"}]}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}

// TestRunResolveTagConflictsCommand exercises the flag wiring end to end: the
// command prints one JSON object and exits 0 for advisory and fatal tag
// conflicts alike, while an unusable invocation exits 2. It runs the real TOML
// acquisition subprocess, so it is skipped in short mode and without python3.
func TestRunResolveTagConflictsCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("command test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "alpha", "[recipe]\nid = \"alpha\"\ntags = [\"vcs\"]\n")
	writeRecipeToml(t, catalogDir, "beta", "[recipe]\nid = \"beta\"\ntags = [\"vcs\"]\nconflicts_with = [\"alpha\"]\n")

	t.Run("missing catalog dir exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-tag-conflicts")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("fatal tag conflict is data and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-tag-conflicts", "--catalog-dir", catalogDir, "--recipe", "alpha", "--recipe", "beta")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		var got tagConflictPlan
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout %q is not the plan JSON: %v", stdout, err)
		}
		want := tagConflictPlan{Conflicts: []tagConflict{tagConflictOf("vcs", "fatal", "alpha", "beta")}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("plan = %#v, want %#v", got, want)
		}
	})

	t.Run("no conflict still emits an empty list and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-tag-conflicts", "--catalog-dir", catalogDir, "--recipe", "alpha")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if stdout != "{\"conflicts\":[]}\n" {
			t.Fatalf("stdout = %q, want an empty conflicts list", stdout)
		}
	})
}
