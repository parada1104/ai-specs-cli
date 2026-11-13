package sync

// GO-07 S16: the recipe names the spine prints come from the materializer's
// return value, not from grepping the captured stdout. The semantic that must
// hold is Python's grep over the PARTIAL stdout: only the recipes that actually
// printed their `▸ recipe <id>` line may be named. A recipe that fails before
// the loop prints anything is enabled but not printed, so the spine must still
// report the bare `  syncing recipes` line.

import (
	"bytes"
	"testing"
)

func TestMaterializePrintedNamesAreEmptyBeforeAnyPrint(t *testing.T) {
	t.Setenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES", "")
	fixture := writeMaterializeFixture(t, map[string]string{}, "[recipes.test-ghost]\nenabled = true\n")

	var stdout, stderr bytes.Buffer
	result, err := MaterializeRecipes(MaterializeRequest{
		ProjectRoot: fixture.project,
		AISpecsHome: fixture.home,
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	if err == nil {
		t.Fatal("the internal-test guard must fail this fixture")
	}
	if len(result.EnabledRecipeIDs) != 1 {
		t.Fatalf("enabled = %v, want the manifest's one recipe", result.EnabledRecipeIDs)
	}
	if len(result.PrintedRecipeIDs) != 0 {
		t.Fatalf("printed = %v, want none: the guard fails before the loop prints", result.PrintedRecipeIDs)
	}
	if bytes.Contains(stdout.Bytes(), []byte("▸ recipe")) {
		t.Fatalf("stdout printed a recipe line: %q", stdout.String())
	}
}
