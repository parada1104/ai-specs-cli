package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/conflicts"
	"ai-specs.dev/worktree-gate/shared"
)

// tagConflictOracle imports the real Python materialize authority and runs the
// PRIMARY tag authority (check_tag_conflicts, which grades through the gate
// binary pinned by WORKTREE_GATE_BIN). The gate's reader omits a recipe it
// cannot fully acquire, so this is the exact authority the materialize call site
// uses; it reports the graded conflicts, or the raised exception type when the
// primary authority raises.
const tagConflictOracle = `
import importlib.util, json, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("materialize_tag_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
catalog = Path(sys.argv[2]); ids = json.loads(sys.argv[3])
out = {}
try:
    out["conflicts"] = [
        {"type": "tag_conflict", "tag": c.tag, "recipes": sorted(c.recipes), "severity": c.severity}
        for c in mod.check_tag_conflicts(catalog, ids)
    ]
except Exception as exc:
    out["error"] = type(exc).__name__
print(json.dumps(out, sort_keys=True, separators=(",", ":")))
`

// TestNativeConflictTagDifferentialAgainstPython pins PlanTagConflicts against the
// real Python materialize tag authority (check_tag_conflicts, graded through the
// gate binary). The valid case carries an absent ghost directory and must grade
// the same conflicts in the same first-seen tag order. The unparsable and the
// [recipe]-less files are omitted by the gate's reader on both sides, because the
// materialize call site uses that same lax acquisition.
func TestNativeConflictTagDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	gate := materializeGateBinary(t)
	t.Setenv("WORKTREE_GATE_BIN", gate)
	driver := filepath.Join(t.TempDir(), "tag_conflict_oracle.py")
	if err := os.WriteFile(driver, []byte(tagConflictOracle), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "a", syncRecipeHeader("zeta", "Zeta")+"tags = [\"tracker\", \"vcs\"]\n")
	writeSyncCatalogRecipe(t, catalog, "b", syncRecipeHeader("alpha", "Alpha")+"tags = [\"vcs\", \"vault\"]\nconflicts_with = [\"gamma\"]\n")
	writeSyncCatalogRecipe(t, catalog, "c", syncRecipeHeader("gamma", "Gamma")+"tags = [\"vault\"]\n")
	writeSyncCatalogRecipe(t, catalog, "dir-broken", "not = [valid\n")
	writeSyncCatalogRecipe(t, catalog, "dir-norecipe", "tags = [\"x\"]\n")
	// An enabled recipe directory whose recipe.toml is absent is skipped, like
	// the entirely absent ghost directory.
	if err := os.MkdirAll(filepath.Join(catalog, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		ids           []string
		wantConflicts bool
	}{
		{"shared tags with an absent directory skipped", []string{"ghost", "a", "b", "c"}, true},
		{"absent recipe.toml is skipped", []string{"empty", "a", "b", "c"}, true},
		{"unparsable recipe.toml is omitted", []string{"a", "dir-broken"}, false},
		{"recipe.toml without [recipe] is omitted", []string{"a", "dir-norecipe"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idsJSON, err := json.Marshal(tc.ids)
			if err != nil {
				t.Fatal(err)
			}
			raw := runSyncPythonOracle(t, python, driver, authority, catalog, string(idsJSON))
			var oracle struct {
				Conflicts []shared.TagConflict `json:"conflicts"`
				Error     string               `json:"error"`
			}
			if err := json.Unmarshal([]byte(raw), &oracle); err != nil {
				t.Fatalf("decode oracle %s: %v", raw, err)
			}

			got, gotErr := PlanTagConflicts(catalog, tc.ids)
			if oracle.Error != "" {
				if gotErr == nil {
					t.Fatalf("python raised %s but native returned %#v", oracle.Error, got)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("native error %v but python graded %#v", gotErr, oracle.Conflicts)
			}
			if tc.wantConflicts && len(oracle.Conflicts) == 0 {
				t.Fatal("oracle graded no conflicts; the shared-tag fixture is vacuous")
			}
			if !reflect.DeepEqual(got, oracle.Conflicts) {
				t.Fatalf("tag conflicts diverge\n--- go ---\n%#v\n--- python ---\n%#v", got, oracle.Conflicts)
			}
		})
	}
}

// TestNativeConflictPrimitiveBoundary reuses internal/conflicts directly. The
// native primitive path must keep the existing error boundary: a missing recipe
// directory and an invalid recipe.toml propagate the loader error, while a
// clean pair grades fatal with sorted recipe names and a non-nil empty list.
func TestNativeConflictPrimitiveBoundary(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "a", syncRecipeHeader("a", "Rec A")+`
[provides]
skills = [{ id = "s1", source = "bundled" }]
`)
	writeSyncCatalogRecipe(t, catalog, "b", syncRecipeHeader("b", "Rec B")+`
[provides]
skills = [{ id = "s1", source = "bundled" }]
`)
	writeSyncCatalogRecipe(t, catalog, "broken", "[recipe]\nid = \"broken\"\n")

	got, err := conflicts.CheckRecipeConflicts(catalog, []string{"a", "b"})
	if err != nil {
		t.Fatalf("CheckRecipeConflicts: %v", err)
	}
	want := []conflicts.Conflict{{Type: "skill", ID: "s1", Recipes: []string{"Rec A", "Rec B"}, Severity: "fatal"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CheckRecipeConflicts = %#v, want %#v", got, want)
	}

	if _, err := conflicts.CheckRecipeConflicts(catalog, []string{"a", "ghost"}); err == nil || !strings.Contains(err.Error(), "recipe directory not found") {
		t.Fatalf("missing directory error = %v, want the existing directory error", err)
	}
	if _, err := conflicts.CheckRecipeConflicts(catalog, []string{"a", "broken"}); err == nil {
		t.Fatal("an invalid recipe.toml must propagate the loader error")
	}

	clean, err := conflicts.CheckRecipeConflicts(catalog, []string{"a"})
	if err != nil {
		t.Fatalf("CheckRecipeConflicts(clean): %v", err)
	}
	if clean == nil {
		t.Fatal("a clean primitive grade must be a non-nil list")
	}
	if b, err := json.Marshal(clean); err != nil || string(b) != "[]" {
		t.Fatalf("clean primitive JSON = %s (%v), want []", b, err)
	}
}

// TestNativeConflictTagBoundary pins the native tag acquisition boundary as the
// gate's lax reader defines it: a recipe.toml that is missing, is not a regular
// file, is unparsable, or lacks a valid [recipe] table is omitted, while shared
// tags still grade with first-seen order and severity. The Python materialize
// call site reaches this same shared acquisition through the gate's
// --resolve-tag-conflicts command, so an invalid or structurally invalid
// recipe.toml never aborts the tag phase.
func TestNativeConflictTagBoundary(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "a", syncRecipeHeader("zeta", "Zeta")+"tags = [\"tracker\", \"vcs\"]\n")
	writeSyncCatalogRecipe(t, catalog, "b", syncRecipeHeader("alpha", "Alpha")+"tags = [\"vcs\", \"vault\"]\nconflicts_with = [\"gamma\"]\n")
	writeSyncCatalogRecipe(t, catalog, "c", syncRecipeHeader("gamma", "Gamma")+"tags = [\"vault\"]\n")
	writeSyncCatalogRecipe(t, catalog, "broken", "not = [valid\n")
	writeSyncCatalogRecipe(t, catalog, "norecipe", "tags = [\"x\"]\n")
	// A recipe.toml that exists but is not a regular file is skipped like a
	// missing one, matching Path.is_file().
	if err := os.MkdirAll(filepath.Join(catalog, "dir-recipe", "recipe.toml"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := PlanTagConflicts(catalog, []string{"ghost", "dir-recipe", "a", "b", "c"})
	if err != nil {
		t.Fatalf("PlanTagConflicts: %v", err)
	}
	want := []shared.TagConflict{
		{Type: "tag_conflict", Tag: "vcs", Recipes: []string{"alpha", "zeta"}, Severity: "warning"},
		{Type: "tag_conflict", Tag: "vault", Recipes: []string{"alpha", "gamma"}, Severity: "fatal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanTagConflicts = %#v, want %#v", got, want)
	}

	if _, err := PlanTagConflicts(catalog, []string{"a", "broken"}); err != nil {
		t.Fatalf("an unparseable recipe.toml must be omitted, got err=%v", err)
	}
	if _, err := PlanTagConflicts(catalog, []string{"a", "norecipe"}); err != nil {
		t.Fatalf("a recipe.toml without [recipe] must be omitted, got err=%v", err)
	}

	empty, err := PlanTagConflicts(catalog, []string{"ghost", "dir-recipe"})
	if err != nil {
		t.Fatalf("PlanTagConflicts(skipped): %v", err)
	}
	if empty == nil {
		t.Fatal("no tag conflicts must be a non-nil list")
	}
	if b, err := json.Marshal(empty); err != nil || string(b) != "[]" {
		t.Fatalf("empty tag JSON = %s (%v), want []", b, err)
	}
}
