package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeRecipeToml lays down one recipe directory with a recipe.toml body.
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

// TestLoadRecipeCapabilities acquires capability declarations through the
// bounded standard-library TOML seam. A recipe with an invalid capability block
// is unreadable and declares nothing, exactly as the Python loader that
// swallows RecipeValidationError treats it.
func TestLoadRecipeCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("acquisition test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "with-caps", `
[[capabilities]]
id = "tracker"

[[capabilities]]
id = "vcs"
`)
	writeRecipeToml(t, catalogDir, "no-caps", `
name = "no caps"
`)
	writeRecipeToml(t, catalogDir, "scalar-caps", `
capabilities = "not a list"
`)
	writeRecipeToml(t, catalogDir, "bad-caps", `
[[capabilities]]
id = "tracker"

[[capabilities]]
id = "tracker"
`)

	got, err := loadRecipeCapabilities(catalogDir, []string{"with-caps", "no-caps", "scalar-caps", "bad-caps", "missing"})
	if err != nil {
		t.Fatalf("loadRecipeCapabilities: %v", err)
	}

	wantCaps := []string{"tracker", "vcs"}
	if !equalStrings(got["with-caps"], wantCaps) {
		t.Fatalf("with-caps capabilities = %#v, want %#v", got["with-caps"], wantCaps)
	}
	if caps, ok := got["no-caps"]; !ok || len(caps) != 0 {
		t.Fatalf("no-caps capabilities = %#v (present %v), want an empty list", caps, ok)
	}
	if caps, ok := got["scalar-caps"]; !ok || len(caps) != 0 {
		t.Fatalf("scalar-caps capabilities = %#v (present %v), want an empty list", caps, ok)
	}
	if caps, ok := got["bad-caps"]; ok {
		t.Fatalf("bad-caps capabilities = %#v, want the recipe to be unreadable", caps)
	}
	if caps, ok := got["missing"]; ok {
		t.Fatalf("missing capabilities = %#v, want the recipe to be unreadable", caps)
	}
}

// TestLoadRecipeCapabilitiesEmptySelection skips the parser entirely when no
// enabled recipe exposes a readable recipe.toml.
func TestLoadRecipeCapabilitiesEmptySelection(t *testing.T) {
	got, err := loadRecipeCapabilities(t.TempDir(), []string{"missing"})
	if err != nil {
		t.Fatalf("loadRecipeCapabilities: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("capabilities = %#v, want none", got)
	}
}
