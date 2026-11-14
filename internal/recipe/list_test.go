package recipe

// GO-20 U3b-2 (T4a): the recipe feed the hub's Recipes submenu needs, and the
// function U6's `recipe list` will consume. Every case mirrors one branch of
// lib/_internal/recipe-list.py's list_recipes, so the picker's candidate sets
// (status == "available" for add, installed/disabled for remove) keep meaning what
// they meant in Python.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func recipeToml(id, name, version string) string {
	return "[recipe]\n" +
		"id = \"" + id + "\"\n" +
		"name = \"" + name + "\"\n" +
		"description = \"test recipe " + id + "\"\n" +
		"version = \"" + version + "\"\n"
}

// catalog writes a catalogue recipe into home/catalog/recipes/<id>.
func catalog(t *testing.T, home, id, name, version string) {
	t.Helper()
	write(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"), recipeToml(id, name, version))
}

func manifest(t *testing.T, root, body string) {
	t.Helper()
	write(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n\n"+body)
}

func entriesByID(entries []Entry) map[string]Entry {
	out := make(map[string]Entry, len(entries))
	for _, e := range entries {
		out[e.ID] = e
	}
	return out
}

// TestListReportsStatusPerRecipe pins the four statuses: absent from the manifest
// is "available", a truthy [recipes.<id>].enabled is "installed", a falsy one is
// "disabled", and a recipe that does not parse carries its error instead of
// disappearing (the hub would otherwise show a picker that silently lost an item).
func TestListReportsStatusPerRecipe(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	catalog(t, home, "alpha", "Alpha", "1.2.3")
	catalog(t, home, "beta", "Beta", "2.0.0")
	catalog(t, home, "gamma", "Gamma", "0.1.0")
	catalog(t, home, "delta", "Delta", "9.9.9")
	write(t, filepath.Join(home, "catalog", "recipes", "delta", "recipe.toml"), "not toml at all = = =\n")
	manifest(t, root, "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = false\n")

	entries, err := List(root, home)
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	got := entriesByID(entries)
	if len(entries) != 4 {
		t.Fatalf("List returned %d entries (%v), want 4", len(entries), entries)
	}
	if e := got["alpha"]; e.Status != "installed" || e.Name != "Alpha" || e.Version != "1.2.3" {
		t.Errorf("alpha = %+v, want installed/Alpha/1.2.3", e)
	}
	if e := got["beta"]; e.Status != "disabled" {
		t.Errorf("beta = %+v, want disabled", e)
	}
	if e := got["gamma"]; e.Status != "available" || e.Name != "Gamma" || e.Version != "0.1.0" {
		t.Errorf("gamma = %+v, want available/Gamma/0.1.0", e)
	}
	if e := got["delta"]; !strings.HasPrefix(e.Status, "error (") || e.Name != "" || e.Version != "" {
		t.Errorf("delta = %+v, want an error status with no name/version", e)
	}
}

// TestListSkipsHiddenAndInternalTestRecipes pins the two exclusions: dot
// directories and the `test-` convention (util.is_internal_test_recipe), which is
// what keeps the shipped fixtures out of a user's picker.
func TestListSkipsHiddenAndInternalTestRecipes(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	catalog(t, home, "alpha", "Alpha", "1.0.0")
	catalog(t, home, "test-fixture", "Fixture", "1.0.0")
	catalog(t, home, ".hidden", "Hidden", "1.0.0")
	write(t, filepath.Join(home, "catalog", "recipes", "loose-file.toml"), recipeToml("loose", "Loose", "1.0.0"))

	entries, err := List(root, home)
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "alpha" {
		t.Fatalf("List = %v, want only alpha", entries)
	}
}

// TestListIsSortedByID pins the picker order: Python sorts the catalogue listing,
// so a port that relied on map iteration order would reshuffle a user's menu.
func TestListIsSortedByID(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	catalog(t, home, "zeta", "Zeta", "1.0.0")
	catalog(t, home, "alpha", "Alpha", "1.0.0")
	catalog(t, home, "mid", "Mid", "1.0.0")

	entries, err := List(root, home)
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(entries) != len(want) {
		t.Fatalf("List = %v, want %v", entries, want)
	}
	for i, id := range want {
		if entries[i].ID != id {
			t.Fatalf("List order = %v, want %v", entries, want)
		}
	}
}

// TestCatalogHomeIsHomeFirst pins recipe-list.py:30-41's FIRST candidate on the
// shared resolver: AI_SPECS_HOME's home wins whenever its catalog/recipes
// exists (symlinked or real), and an unset/empty home skips the candidate
// entirely — Python's `if ai_specs_home:` — falling to the install root.
func TestCatalogHomeIsHomeFirst(t *testing.T) {
	installRoot := t.TempDir()
	catalog(t, installRoot, "root-recipe", "Root", "1.0.0")

	homeWith := t.TempDir()
	catalog(t, homeWith, "home-recipe", "Home", "2.0.0")
	if got := CatalogHome(homeWith, installRoot); got != homeWith {
		t.Errorf("CatalogHome(home with a catalogue) = %q, want the home %q", got, homeWith)
	}
	if got := CatalogHome("", installRoot); got != installRoot {
		t.Errorf(`CatalogHome("") = %q, want the install root %q`, got, installRoot)
	}
}

// TestCatalogHomeFallsBackToInstallRoot pins the ANCHOR case of the second
// candidate: a home WITHOUT catalog/recipes (an AI_SPECS_HOME that carries
// lib/ but no catalogue) falls to the install root the CLI derives from the
// RUNNING BINARY, and the listing then comes from THERE. Python anchors the
// same candidate on Path(recipe-list.py).resolve() — the physical tree that
// holds lib/; in every production layout (bin/ai-specs and lib/ side by side)
// and in this repository the two anchors are the same directory, and they
// diverge only when the binary is built outside <root>/bin (the test
// harness's smoke binary) — the behaviour recipe_test.go's
// TestRecipeListFallsBackToBinaryInstallRoot pins at the verb.
func TestCatalogHomeFallsBackToInstallRoot(t *testing.T) {
	installRoot := t.TempDir()
	catalog(t, installRoot, "root-recipe", "Root", "1.0.0")
	homeWithout := t.TempDir()

	got := CatalogHome(homeWithout, installRoot)
	if got != installRoot {
		t.Fatalf("CatalogHome(home without a catalogue) = %q, want the install root %q", got, installRoot)
	}
	entries, err := List(t.TempDir(), got)
	if err != nil {
		t.Fatalf("List through the install root: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "root-recipe" {
		t.Errorf("List = %v, want the install root's root-recipe", entries)
	}
}

// TestListWithoutCatalogOrManifest pins the two empties: no catalogue directory and
// no manifest both yield an empty list rather than an error or a panic — the state
// every project starts in.
func TestListWithoutCatalogOrManifest(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	if entries, err := List(root, home); err != nil || len(entries) != 0 {
		t.Fatalf("List(no catalog) = (%v, %v), want empty and no error", entries, err)
	}

	catalog(t, home, "alpha", "Alpha", "1.0.0")
	if entries, err := List(root, home); err != nil || len(entries) != 1 {
		t.Fatalf("List(no manifest) = (%v, %v), want one available entry", entries, err)
	} else if entries[0].Status != "available" {
		t.Errorf("status without a manifest = %q, want available", entries[0].Status)
	}
}
