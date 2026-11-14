package doctor

// Native port of doctor._check_gate_provenance (check 16 of the roster).

import (
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/schema"
)

// checkGateProvenance is doctor._check_gate_provenance.
//
// Warns when a materialized gate's current bytes differ from its recorded
// baseline (user-modified) or when no baseline exists (unknown provenance);
// stays quiet for gates whose baseline matches. Mirrors the sync-side
// classifier exactly; never rewrites anything.
func (d *Doctor) checkGateProvenance() {
	catalog := filepath.Join(d.Home, "catalog", "recipes")
	if !isDir(catalog) {
		return
	}
	if !isFile(d.manifestPath()) {
		return
	}
	data := d.manifestData()
	if data == nil {
		return
	}
	managed := d.loadManagedOverrides()

	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return
	}
	for _, rid := range recipes.Keys() {
		entry, ok := recipes.Table(rid)
		if !ok || entry == nil {
			continue
		}
		enabled, isBool := entry.Bool("enabled")
		if !isBool || !enabled {
			continue
		}
		recipeDir := filepath.Join(catalog, rid)
		recipeToml := filepath.Join(recipeDir, "recipe.toml")
		if !isFile(recipeToml) {
			continue
		}
		recipe, err := schema.LoadRecipeToml(recipeToml)
		if err != nil {
			continue
		}
		for _, hook := range recipe.RuntimeHooks {
			rel := "ai-specs/recipes/" + rid + "/hooks/" + filepath.Base(hook.Script)
			dest := filepath.Join(d.Root, filepath.FromSlash(rel))
			if !isFile(dest) {
				continue
			}
			state := classifyManagedOverride(dest, managed[rel], nil, false)
			var message string
			switch state {
			case "user_modified":
				message = rel + " is user-modified; sync will preserve it"
			case "untracked":
				message = rel + " has no recorded provenance; sync will preserve it " +
					"and record a baseline only when the CLI renders the gate"
			default:
				continue
			}
			d.add(WARN, "gate-provenance", message,
				"rm "+rel+" && ai-specs sync (or ai-specs sync --refresh-gates)")
		}
	}
}
