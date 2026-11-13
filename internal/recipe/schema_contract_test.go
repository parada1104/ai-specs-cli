package recipe

// The `[[hooks]]` v2 table lives in internal/schema. The frozen schema corpus
// pins the missing-`action` arm (err_hooks_missing_action.toml) but carries no
// recipe with `action` alone, so the missing-`event` arm had no Go assertion
// (legacy tests/test_recipe_read.py::test_v2_missing_hook_event_fails). Both
// arms reject before any other field is read, naming the absent key.

import (
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

func writeHookRecipe(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "recipe.toml")
	src := "[recipe]\nid = \"x\"\nname = \"N\"\ndescription = \"D\"\nversion = \"1.0\"\n\n" + body
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHookEventIsRequired(t *testing.T) {
	_, err := schema.LoadRecipeToml(writeHookRecipe(t, "[[hooks]]\naction = \"validate-config\"\n"))
	// The legacy assertion names `event`: the missing-event arm rejects before
	// action is read.
	want := ".hooks[0]: missing or invalid required field 'event'"
	if err == nil {
		t.Fatalf("a [[hooks]] entry without event must be rejected")
	}
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

// PRESERVE: the sibling arm (event present, action absent) is pinned by the
// frozen corpus fixture err_hooks_missing_action.toml; this keeps the pair in
// one place for the reader.
func TestHookActionIsRequired(t *testing.T) {
	_, err := schema.LoadRecipeToml(writeHookRecipe(t, "[[hooks]]\nevent = \"on-sync\"\n"))
	want := ".hooks[0]: missing or invalid required field 'action'"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}
