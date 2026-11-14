package shared

// Recipe.toml acquisition tests, moved here by GO-20 U2e T5 PR4b from the gate
// main package (tagconflicts_test.go / primitiveconflicts_test.go) when the
// --resolve-tag-conflicts and --resolve-primitive-conflicts bridge flags were
// deleted: the loaders stay because these two seams are LIVE production code
// (internal/sync/conflicts_core.go and internal/sync/materialize.go call them).
//
// The loaders used to hand each recipe.toml to an ambient `python3 -c` tomllib
// helper. PR4b parses in process, so the acquisition tests no longer need an
// interpreter at all and TestRecipeTomlLoadersNeedNoInterpreter proves it by
// putting a python3 shim FIRST on PATH that fails loudly.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeRecipeToml(t *testing.T, catalogDir, recipeID, body string) {
	t.Helper()
	dir := filepath.Join(catalogDir, recipeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write recipe.toml: %v", err)
	}
}

// failingPython3 puts a `python3` shim first on PATH that writes a marker and
// exits nonzero, so any surviving interpreter invocation fails loudly instead of
// being papered over. It returns the marker path.
func failingPython3(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(dir, "python3-called")
	shim := fmt.Sprintf("#!/bin/sh\necho called >> %q\necho 'python3 must not be invoked by TOML acquisition' >&2\nexit 127\n", marker)
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte(shim), 0o755); err != nil {
		t.Fatalf("write python3 shim: %v", err)
	}
	t.Setenv("PATH", dir)
	return marker
}

// TestRecipeTomlLoadersNeedNoInterpreter is the PR4b RED/GREEN pin for the last
// two production python3 seams: with a failing python3 shim as the ONLY PATH
// entry, both readers must still acquire the same answers and must never have
// run the shim.
func TestRecipeTomlLoadersNeedNoInterpreter(t *testing.T) {
	marker := failingPython3(t)

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", `
[recipe]
id = "alpha"
name = "Alpha"
tags = ["vcs"]
conflicts_with = ["beta"]

[[provides.skills]]
id = "z-skill"
`)

	tags, err := LoadRecipeTagMetadata(catalogDir, []string{"dir-a"})
	if err != nil {
		t.Fatalf("LoadRecipeTagMetadata: %v", err)
	}
	wantTags := []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}, ConflictsWith: []string{"beta"}}}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tag metadata = %#v, want %#v", tags, wantTags)
	}

	primitives, err := LoadRecipePrimitives(catalogDir, []string{"dir-a"})
	if err != nil {
		t.Fatalf("LoadRecipePrimitives: %v", err)
	}
	wantPrimitives := []RecipePrimitives{{ID: "alpha", Name: "Alpha", Skills: []string{"z-skill"}, Commands: []string{}, MCP: []string{}}}
	if !reflect.DeepEqual(primitives, wantPrimitives) {
		t.Fatalf("primitives = %#v, want %#v", primitives, wantPrimitives)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the TOML loaders invoked python3: they must parse in process")
	}
}

// TestLoadRecipeTagMetadata acquires the top-level [recipe] id, tags and
// conflicts_with. The id is the TOML recipe id, never the catalog directory
// name, and a recipe whose [recipe] table is missing or malformed is unreadable
// and contributes nothing.
func TestLoadRecipeTagMetadata(t *testing.T) {
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
	writeRecipeToml(t, catalogDir, "blank-tag", `
[recipe]
id = "blank-tag"
tags = ["  "]
`)
	writeRecipeToml(t, catalogDir, "bad-toml", "not = [valid\n")

	got, err := LoadRecipeTagMetadata(catalogDir, []string{"dir-a", "dir-b", "no-recipe-table", "no-id", "bad-tags", "blank-tag", "bad-toml", "missing"})
	if err != nil {
		t.Fatalf("LoadRecipeTagMetadata: %v", err)
	}
	want := []RecipeTagMetadata{
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
	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\ntags = [\"vcs\"]\n")
	writeRecipeToml(t, catalogDir, "dir-b", "[recipe]\nid = \"beta\"\ntags = [\"vcs\"]\n")

	got, err := LoadRecipeTagMetadata(catalogDir, []string{"dir-b", "dir-a"})
	if err != nil {
		t.Fatalf("LoadRecipeTagMetadata: %v", err)
	}
	if len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("metadata = %#v, want beta then alpha", got)
	}
}

// TestLoadRecipePrimitives acquires the [recipe] id and name plus the ordered
// provides primitive ids. The conflict registry owns names, so the TOML id and
// the [recipe].name both travel separately, and array order is preserved
// exactly as declared.
func TestLoadRecipePrimitives(t *testing.T) {
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

	got, err := LoadRecipePrimitives(catalogDir, []string{"dir-a", "dir-b"})
	if err != nil {
		t.Fatalf("LoadRecipePrimitives: %v", err)
	}
	want := []RecipePrimitives{
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
	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n")
	writeRecipeToml(t, catalogDir, "dir-b", "[recipe]\nid = \"beta\"\nname = \"Beta\"\n")

	got, err := LoadRecipePrimitives(catalogDir, []string{"dir-b", "dir-a"})
	if err != nil {
		t.Fatalf("LoadRecipePrimitives: %v", err)
	}
	if len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("primitives = %#v, want beta then alpha", got)
	}
}

// TestLoadRecipePrimitivesDeduplicatesRecipeIDs keeps the sibling readers' seen
// dedup: callers pass unique ids, and a repeated id is acquired once.
func TestLoadRecipePrimitivesDeduplicatesRecipeIDs(t *testing.T) {
	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "dir-a", "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n")

	got, err := LoadRecipePrimitives(catalogDir, []string{"dir-a", "dir-a"})
	if err != nil {
		t.Fatalf("LoadRecipePrimitives: %v", err)
	}
	if len(got) != 1 || got[0].ID != "alpha" {
		t.Fatalf("primitives = %#v, want exactly alpha once", got)
	}
}

// TestLoadRecipePrimitivesRejectsUnreadableRecipes pins the parser-failure
// contract: a missing, unparseable or schema-invalid recipe.toml is an error,
// never a silent omission, because the Python authority raises
// RecipeValidationError for the same shapes. Only the diagnostic TEXT diverges
// (recorded TOLERANT in docs/go-migration-parity-contract.md).
func TestLoadRecipePrimitivesRejectsUnreadableRecipes(t *testing.T) {
	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "bad-toml", "not = [valid\n")
	writeRecipeToml(t, catalogDir, "no-name", "[recipe]\nid = \"x\"\n")
	writeRecipeToml(t, catalogDir, "no-recipe-table", "tags = []\n")
	writeRecipeToml(t, catalogDir, "bad-skills", `
[recipe]
id = "x"
name = "X"

[[provides.skills]]
source = "local"
`)
	writeRecipeToml(t, catalogDir, "blank-name", "[recipe]\nid = \"x\"\nname = \"  \"\n")

	cases := []struct {
		name string
		ids  []string
	}{
		{"missing recipe directory", []string{"ghost"}},
		{"unparseable toml", []string{"bad-toml"}},
		{"missing recipe table", []string{"no-recipe-table"}},
		{"missing recipe name", []string{"no-name"}},
		{"blank recipe name", []string{"blank-name"}},
		{"provides entry without id", []string{"bad-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadRecipePrimitives(catalogDir, tc.ids); err == nil {
				t.Fatalf("LoadRecipePrimitives(%v) = nil error, want a parser failure", tc.ids)
			}
		})
	}
}

// TestLoadRecipePrimitivesNonTableProvidesClaimsNothing pins the shapes the
// Python reader swallows: a non-table [provides] and a non-list skills/commands/
// mcp value contribute no claims instead of failing.
func TestLoadRecipePrimitivesNonTableProvidesClaimsNothing(t *testing.T) {
	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "flat-provides", "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\nprovides = 5\n")
	writeRecipeToml(t, catalogDir, "str-skills", "[recipe]\nid = \"beta\"\nname = \"Beta\"\n\n[provides]\nskills = \"nope\"\n")

	got, err := LoadRecipePrimitives(catalogDir, []string{"flat-provides", "str-skills"})
	if err != nil {
		t.Fatalf("LoadRecipePrimitives: %v", err)
	}
	want := []RecipePrimitives{
		{ID: "alpha", Name: "Alpha", Skills: []string{}, Commands: []string{}, MCP: []string{}},
		{ID: "beta", Name: "Beta", Skills: []string{}, Commands: []string{}, MCP: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("primitives = %#v, want %#v", got, want)
	}
}
