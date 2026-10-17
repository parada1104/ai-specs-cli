// Package recipe owns the recipe catalogue as the CLI sees it. This file is the
// GO-20 U3b-2 slice the hub's Recipes submenu needs (list_recipes in
// lib/_internal/recipe-list.py); U6's `recipe list` command consumes the same
// function, so the two never drift.
package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/schema"
)

// Statuses a catalogue recipe can carry. They are the strings hub.py's pickers
// branch on, so they are part of the contract rather than a display detail.
const (
	// StatusAvailable is a catalogue recipe the project has not installed.
	StatusAvailable = "available"
	// StatusInstalled is a recipe the manifest enables.
	StatusInstalled = "installed"
	// StatusDisabled is a recipe the manifest carries with enabled = false.
	StatusDisabled = "disabled"
)

// Entry is one catalogue recipe: what the picker labels and what it filters on.
type Entry struct {
	ID      string
	Name    string
	Version string
	Status  string
}

// CatalogDir is the single catalogue location the CLI resolves: the install
// root's catalog/recipes. recipe-list.py resolves the same path twice — from
// AI_SPECS_HOME, then from its own module path — and both reduce to this one.
func CatalogDir(cliHome string) string {
	return filepath.Join(cliHome, "catalog", "recipes")
}

// List scans the catalogue and compares it against the manifest's [recipes.*]
// tables, mirroring list_recipes: hidden directories and the internal `test-`
// fixtures are skipped, the order is the sorted directory listing, and a recipe
// that does not parse keeps its id with an `error (...)` status instead of
// disappearing. An unreadable manifest is not an error (Python swallows it, and
// every recipe then reads as available); a missing catalogue is an empty list.
// The returned error is for a catalogue directory that exists but cannot be
// read, which is what keeps the hub's "Recipes unavailable" path reachable.
func List(projectRoot, cliHome string) ([]Entry, error) {
	enabled := enabledRecipes(filepath.Join(projectRoot, "ai-specs", "ai-specs.toml"))

	dir := CatalogDir(cliHome)
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Entry
	for _, de := range names {
		id := de.Name()
		if strings.HasPrefix(id, ".") || strings.HasPrefix(id, "test-") {
			continue
		}
		// is_dir(), not the directory entry's own type: Python follows a
		// symlinked recipe directory and a port that did not would drop it.
		info, err := os.Stat(filepath.Join(dir, id))
		if err != nil || !info.IsDir() {
			continue
		}
		loaded, err := schema.LoadRecipeToml(filepath.Join(dir, id, "recipe.toml"))
		if err != nil {
			out = append(out, Entry{ID: id, Status: fmt.Sprintf("error (%v)", err)})
			continue
		}
		out = append(out, Entry{
			ID:      id,
			Name:    loaded.Name,
			Version: loaded.Version,
			Status:  enabledStatus(id, enabled),
		})
	}
	return out, nil
}

// enabledStatus is list_recipes' status decision: the manifest decides installed
// vs disabled, and a recipe the manifest does not mention is available.
func enabledStatus(id string, enabled map[string]bool) string {
	if on, ok := enabled[id]; ok {
		if on {
			return StatusInstalled
		}
		return StatusDisabled
	}
	return StatusAvailable
}

// enabledRecipes is toml-read.read_recipes reduced to the one field this package
// needs: only a real boolean counts as enabled, so `enabled = "true"` or `enabled
// = 1` stay disabled, exactly as Python's isinstance check decides.
func enabledRecipes(manifestPath string) map[string]bool {
	out := map[string]bool{}
	data, err := config.LoadManifest(manifestPath)
	if err != nil {
		return out
	}
	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return out
	}
	for _, id := range recipes.Keys() {
		table, ok := recipes.Table(id)
		if !ok || table == nil {
			continue
		}
		on, _ := table.Bool("enabled")
		out[id] = on
	}
	return out
}
