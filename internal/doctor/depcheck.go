package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"ai-specs.dev/ai-specs/internal/deps"
	"ai-specs.dev/ai-specs/internal/schema"
)

// doctor's dep_check.py surface. The PROBE itself is internal/deps' — doctor
// and the CLI-dep gate ask the same questions ("is this binary reachable, at
// which version"), so there is exactly one authority for the answers and this
// file keeps only what is doctor's own: the mapping from a result onto its
// frozen check, and the two helpers other doctor checks share.

// depResult is the probe's result (dep_check.DepResult field for field).
type depResult = deps.Result

// checkRecipeCLIDeps is doctor._check_recipe_cli_deps.
func (d *Doctor) checkRecipeCLIDeps() {
	d.loadManifest()
	if !d.hasRecipeTables() {
		return
	}
	d.emitRecipeDepChecks(d.collectRecipeDepResults())
}

// collectRecipeDepResults is doctor._collect_recipe_dep_results: the aggregate
// of dep_check.check_project_deps for the enabled recipes.
func (d *Doctor) collectRecipeDepResults() []depResult {
	return deps.CheckProject(d.Root, d.Home)
}

// emitRecipeDepChecks maps each DepResult onto its frozen check. Extracted so
// the mapping is testable without a real PATH or catalog.
func (d *Doctor) emitRecipeDepChecks(results []depResult) {
	for _, r := range results {
		switch {
		case r.OK:
			d.add(OK, "recipe-dep", r.Binary+" available for "+r.RecipeID)
		case r.Required:
			guidance := r.InstallURL
			if guidance == "" {
				guidance = "install the required CLI"
			}
			d.add(WARN, "recipe-dep",
				r.Binary+" missing/unusable for "+r.RecipeID+": "+r.Purpose, guidance)
		default:
			d.add(INFO, "recipe-dep",
				"optional "+r.Binary+" not found for "+r.RecipeID+": "+r.Purpose, r.InstallURL)
		}
	}
}

// readCatalogRecipe is recipe-read.read_recipe, through the one authority: the
// env check reads a catalog recipe with the probe's own reader.
func readCatalogRecipe(home, recipeID string) (*schema.Recipe, error) {
	return deps.ReadCatalogRecipe(home, recipeID)
}

// whichBinary is shutil.which(binary) is not None, through the one authority.
func whichBinary(binary string) bool {
	return deps.Which(binary)
}

// fileSHA256 is provider_install._sha256, kept here because the gate binary
// verification (gatebinary.go) also hashes an on-disk candidate. The provider
// resolver hashes through internal/provider; this stays the doctor-local helper
// for the gate path.
func fileSHA256(filePath string) (string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
