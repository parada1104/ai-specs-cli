package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

// writeCatalogRecipe materializes <home>/catalog/recipes/<id>/recipe.toml, the
// on-disk install catalog recipe-read.read_recipe reads (never the embedded
// asset tree).
func writeCatalogRecipe(t *testing.T, home, id, body string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"), body)
}

// catalogRecipe is the minimal valid recipe.toml (id/name/description/version
// are required by recipe_schema); extra is appended verbatim.
func catalogRecipe(id, extra string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"N\"\ndescription = \"D\"\nversion = \"1.0\"\n" + extra
}

// TestEmitRecipeDepChecks pins the DepResult -> Check mapping: the probe's
// answers are internal/deps'; the frozen check text and its guidance are
// doctor's.
func TestEmitRecipeDepChecks(t *testing.T) {
	d := &Doctor{}
	d.emitRecipeDepChecks([]depResult{
		{Binary: "git", OK: true, Required: true, RecipeID: "worktree-flow"},
		{Binary: "npx", OK: false, Required: true, RecipeID: "trello-mcp-workflow",
			Purpose: "Run the MCP server", InstallURL: "https://example.com/install"},
		{Binary: "missing", OK: false, Required: true, RecipeID: "r", Purpose: "do things"},
		{Binary: "opt", OK: false, Required: false, RecipeID: "r2", Purpose: "optional thing"},
		{Binary: "opt2", OK: false, Required: false, RecipeID: "r3", Purpose: "optional thing",
			InstallURL: "https://example.com/opt"},
	})
	assertChecks(t, d, []Check{
		{OK, "recipe-dep", "git available for worktree-flow", ""},
		{WARN, "recipe-dep", "npx missing/unusable for trello-mcp-workflow: Run the MCP server",
			"https://example.com/install"},
		{WARN, "recipe-dep", "missing missing/unusable for r: do things", "install the required CLI"},
		{INFO, "recipe-dep", "optional opt not found for r2: optional thing", ""},
		{INFO, "recipe-dep", "optional opt2 not found for r3: optional thing", "https://example.com/opt"},
	})
}

// TestCheckRecipeCLIDeps exercises the doctor-facing wiring end to end against
// an on-disk install catalog: the probe is internal/deps', the mapping is
// doctor's.
func TestCheckRecipeCLIDeps(t *testing.T) {
	t.Run("no-recipes-table", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"x\"\n")
		d := New(root, t.TempDir())
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 0 {
			t.Errorf("checks = %+v, want none", d.Checks)
		}
	})
	t.Run("recipe-without-cli-deps", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeManifest(t, root, "[recipes.session-context]\nenabled = true\n")
		writeCatalogRecipe(t, home, "session-context", catalogRecipe("session-context", ""))
		d := New(root, home)
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 0 {
			t.Errorf("checks = %+v, want none", d.Checks)
		}
	})
	t.Run("git-recipe", func(t *testing.T) {
		if !whichBinary("git") {
			t.Skip("git not on PATH")
		}
		root := t.TempDir()
		home := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
		writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
			"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))
		d := New(root, home)
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 1 {
			t.Fatalf("checks = %+v, want 1", d.Checks)
		}
		c := d.Checks[0]
		if c.Name != "recipe-dep" || !strings.HasSuffix(c.Message, "for worktree-flow") {
			t.Errorf("check = %+v", c)
		}
	})
}
