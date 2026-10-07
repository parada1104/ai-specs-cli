package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// primitiveConflictOf builds one expected conflict matching the JSON contract
// emitted on stdout; recipes are stored as given (the grader sorts them).
func primitiveConflictOf(primitiveType, id string, recipes ...string) shared.PrimitiveConflict {
	return shared.PrimitiveConflict{Type: primitiveType, ID: id, Recipes: recipes, Severity: "fatal"}
}

// TestLoadRecipePrimitives acquires the [recipe] id and name plus the ordered
// provides primitive ids through the bounded standard-library TOML seam. The
// conflict registry owns names, so the TOML id and the [recipe].name both travel
// separately, and array order is preserved exactly as declared.
func TestLoadRecipePrimitives(t *testing.T) {
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

[[provides.skills]]
id = "z-skill"
source = "local"

[[provides.skills]]
id = "a-skill"
source = "local"

[[provides.commands]]
id = "alpha-cmd"
path = "bin/alpha"

[[provides.mcp]]
id = "alpha-mcp"
`)
	writeRecipeToml(t, catalogDir, "dir-b", `
[recipe]
id = "beta"
name = "Beta"
description = "second"
version = "1.0.0"
`)

	got, err := loadRecipePrimitives(catalogDir, []string{"dir-a", "dir-b"})
	if err != nil {
		t.Fatalf("loadRecipePrimitives: %v", err)
	}
	want := []shared.RecipePrimitives{
		{ID: "alpha", Name: "Alpha", Skills: []string{"z-skill", "a-skill"}, Commands: []string{"alpha-cmd"}, MCP: []string{"alpha-mcp"}},
		{ID: "beta", Name: "Beta", Skills: []string{}, Commands: []string{}, MCP: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("primitives = %#v, want %#v", got, want)
	}
}

// TestLoadRecipePrimitivesOrderPreserved keeps the enabled order so conflict
// ordering is reproducible downstream.
func TestLoadRecipePrimitivesOrderPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n")
	writeRecipeToml(t, catalogDir, "dir-b", "[recipe]\nid = \"beta\"\nname = \"Beta\"\n")

	got, err := loadRecipePrimitives(catalogDir, []string{"dir-b", "dir-a"})
	if err != nil {
		t.Fatalf("loadRecipePrimitives: %v", err)
	}
	if len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("primitives = %#v, want beta then alpha", got)
	}
}

// TestLoadRecipePrimitivesDeduplicatesRecipeIDs keeps the sibling readers' seen
// dedup: callers pass unique ids, and a repeated id is acquired once.
func TestLoadRecipePrimitivesDeduplicatesRecipeIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n")

	got, err := loadRecipePrimitives(catalogDir, []string{"dir-a", "dir-a"})
	if err != nil {
		t.Fatalf("loadRecipePrimitives: %v", err)
	}
	if len(got) != 1 || got[0].ID != "alpha" {
		t.Fatalf("primitives = %#v, want exactly alpha once", got)
	}
}

// TestLoadRecipePrimitivesRejectsUnreadableRecipes pins the parser-failure
// contract the Python bridge relies on: a missing, unparseable or schema-invalid
// recipe.toml is an error (exit 2 upstream), never a silent omission, because
// the Python authority raises RecipeValidationError for the same shapes.
func TestLoadRecipePrimitivesRejectsUnreadableRecipes(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "bad-toml", "[recipe]\n")
	writeRecipeToml(t, catalogDir, "no-name", "[recipe]\nid = \"x\"\n")
	writeRecipeToml(t, catalogDir, "bad-skills", `
[recipe]
id = "x"
name = "X"

[[provides.skills]]
source = "local"
`)

	cases := []struct {
		name string
		ids  []string
	}{
		{"missing recipe directory", []string{"ghost"}},
		{"unparseable toml", []string{"bad-toml"}},
		{"missing recipe name", []string{"no-name"}},
		{"provides entry without id", []string{"bad-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadRecipePrimitives(catalogDir, tc.ids); err == nil {
				t.Fatalf("loadRecipePrimitives(%v) = nil error, want a parser failure", tc.ids)
			}
		})
	}
}

// TestRunResolvePrimitiveConflictsCommand exercises the flag wiring end to end:
// the command prints one JSON object and exits 0 for any graded result — even
// with conflicts — while unusable flags, an unreadable recipe or an invalid
// recipe.toml exit 2 so the calling bridge can fall back to the Python
// authority. It runs the real TOML acquisition subprocess, so it is skipped in
// short mode and without python3.
func TestRunResolvePrimitiveConflictsCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("command test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", `
[recipe]
id = "alpha"
name = "Alpha"

[[provides.skills]]
id = "shared"
source = "local"
`)
	writeRecipeToml(t, catalogDir, "dir-b", `
[recipe]
id = "beta"
name = "Beta"

[[provides.skills]]
id = "shared"
source = "local"
`)
	writeRecipeToml(t, catalogDir, "dir-broken", "not = [valid\n")

	t.Run("missing catalog dir exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-primitive-conflicts")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("skill conflict is data and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-primitive-conflicts", "--catalog-dir", catalogDir, "--recipe", "dir-a", "--recipe", "dir-b")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		var got shared.PrimitiveConflictPlan
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout %q is not the plan JSON: %v", stdout, err)
		}
		want := shared.PrimitiveConflictPlan{Conflicts: []shared.PrimitiveConflict{primitiveConflictOf("skill", "shared", "Alpha", "Beta")}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("plan = %#v, want %#v", got, want)
		}
	})

	t.Run("no conflict still emits an empty list and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-primitive-conflicts", "--catalog-dir", catalogDir, "--recipe", "dir-a")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if stdout != "{\"conflicts\":[]}\n" {
			t.Fatalf("stdout = %q, want an empty conflicts list", stdout)
		}
	})

	t.Run("invalid recipe.toml exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-primitive-conflicts", "--catalog-dir", catalogDir, "--recipe", "dir-broken")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("missing recipe directory exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-primitive-conflicts", "--catalog-dir", catalogDir, "--recipe", "ghost")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})
}
