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

// The legacy live leg drove the real Python materialize tag authority
// (recipe-materialize.py's check_tag_conflicts, which grades through the gate
// binary pinned by WORKTREE_GATE_BIN) and compared its conflict list with the
// native PlanTagConflicts. It was captured once into
// testdata/conflictscore_golden.json: the fixture catalog the leg built (file
// bytes plus the one directory that carries no recipe.toml), and per case the
// enabled ids and the graded conflicts, or the raised exception type.
//
// The payload is host-independent — the driver printed only the conflict records
// and never a path, a mode, a clock or a cache key — so no case needed
// normalization. File and directory modes are deliberately not frozen: every
// fixture mode follows the process umask, and this package also runs
// t.Parallel() tests that must not have the process umask pinned under them. A
// run of this test under `umask 077` proves it.
//
// The test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const conflictsCoreGoldenFile = "conflictscore_golden.json"

// conflictsCoreGoldenCases is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle of
// coverage — kept in the test, never derived from the bundle — so a bundle that
// is missing, renamed, duplicated or reordered cannot validate silently.
var conflictsCoreGoldenCases = []string{
	"shared tags with an absent directory skipped",
	"absent recipe.toml is skipped",
	"unparsable recipe.toml is omitted",
	"recipe.toml without [recipe] is omitted",
}

// conflictsCoreNode is one frozen catalog node: a directory by path only, or a
// regular file by path and bytes.
type conflictsCoreNode struct {
	Path    string `json:"path"`
	Dir     bool   `json:"dir,omitempty"`
	Content string `json:"content,omitempty"`
}

// conflictsCoreFrozenCase is one frozen case: the enabled ids and the legacy
// grade (the graded conflicts, or the raised exception type).
type conflictsCoreFrozenCase struct {
	Name      string               `json:"name"`
	IDs       []string             `json:"ids"`
	Conflicts []shared.TagConflict `json:"conflicts"`
	Error     string               `json:"error,omitempty"`
}

// conflictsCoreGolden is the frozen bundle: oracle provenance, the catalog the
// cases run over, and the per-case grades.
type conflictsCoreGolden struct {
	Oracle  map[string]string         `json:"oracle"`
	Catalog []conflictsCoreNode       `json:"catalog"`
	Cases   []conflictsCoreFrozenCase `json:"cases"`
}

// conflictsCoreCase is one case of the in-test corpus.
type conflictsCoreCase struct {
	name          string
	ids           []string
	wantConflicts bool
}

// conflictsCoreCorpus is the in-test case corpus: the shared catalog with the
// ids, the vacuity expectation and the case order the live leg graded.
func conflictsCoreCorpus() []conflictsCoreCase {
	return []conflictsCoreCase{
		{"shared tags with an absent directory skipped", []string{"ghost", "a", "b", "c"}, true},
		{"absent recipe.toml is skipped", []string{"empty", "a", "b", "c"}, true},
		{"unparsable recipe.toml is omitted", []string{"a", "dir-broken"}, false},
		{"recipe.toml without [recipe] is omitted", []string{"a", "dir-norecipe"}, false},
	}
}

// conflictsCoreCatalog writes the fixture catalog the live leg built and returns
// it: three recipes that share tags (one declaring conflicts_with), one
// unparsable recipe.toml, one recipe.toml without a [recipe] table, and one
// enabled directory whose recipe.toml is absent.
func conflictsCoreCatalog(t *testing.T) string {
	t.Helper()
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
	return catalog
}

// conflictsCoreTree lists the catalog as sorted root-relative nodes: a directory
// by path and a regular file by path and bytes. Modes are omitted because they
// follow the process umask.
func conflictsCoreTree(t *testing.T, root string) []conflictsCoreNode {
	t.Helper()
	out := []conflictsCoreNode{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &os.PathError{Op: "snapshot", Path: path, Err: os.ErrInvalid}
		}
		if info.IsDir() {
			out = append(out, conflictsCoreNode{Path: filepath.ToSlash(rel), Dir: true})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, conflictsCoreNode{Path: filepath.ToSlash(rel), Content: string(data)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// compareConflictsCoreCatalog requires identical node inventories: the same
// paths in the same order, the same kind and the same bytes.
func compareConflictsCoreCatalog(t *testing.T, label string, got, want []conflictsCoreNode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d nodes, want %d\n native = %v\n frozen = %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Path != want[i].Path || got[i].Dir != want[i].Dir {
			t.Fatalf("%s: node %d is %+v, want %+v", label, i, got[i], want[i])
		}
		if got[i].Content != want[i].Content {
			t.Fatalf("%s: %s bytes differ:\n native = %q\n frozen = %q", label, want[i].Path, got[i].Content, want[i].Content)
		}
	}
}

// loadConflictsCoreGolden reads the frozen bundle and validates its identity
// inventory, its oracle metadata and the root-relative shape of every frozen
// path before any comparison runs.
func loadConflictsCoreGolden(t *testing.T) conflictsCoreGolden {
	t.Helper()
	var golden conflictsCoreGolden
	if err := json.Unmarshal(readGoldenBundle(t, conflictsCoreGoldenFile), &golden); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", conflictsCoreGoldenFile, err)
	}
	for _, key := range []string{"python", "script", "driver"} {
		if golden.Oracle[key] == "" {
			t.Fatalf("frozen oracle %s carries no %s metadata", conflictsCoreGoldenFile, key)
		}
	}
	names := make([]string, len(golden.Cases))
	for i, rec := range golden.Cases {
		names[i] = rec.Name
		if rec.Conflicts == nil {
			t.Fatalf("frozen record %q grades no conflict list", rec.Name)
		}
	}
	for _, node := range golden.Catalog {
		if filepath.IsAbs(node.Path) || strings.HasPrefix(node.Path, "..") {
			t.Fatalf("frozen catalog path %q must be root-relative", node.Path)
		}
	}
	validateGoldenIdentity(t, "conflictscore", names, conflictsCoreGoldenCases)
	return golden
}

// TestNativeConflictTagDifferentialAgainstPython pins PlanTagConflicts against
// the frozen grade of the real Python materialize tag authority
// (check_tag_conflicts, graded through the gate binary). The valid case carries
// an absent ghost directory and must grade the same conflicts in the same
// first-seen tag order. The unparsable and the [recipe]-less files are omitted
// by the gate's reader on both sides, because the materialize call site uses
// that same lax acquisition.
func TestNativeConflictTagDifferentialAgainstPython(t *testing.T) {
	golden := loadConflictsCoreGolden(t)
	cases := conflictsCoreCorpus()
	if len(cases) != len(conflictsCoreGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(conflictsCoreGoldenCases))
	}

	catalog := conflictsCoreCatalog(t)
	// The fixture the freeze was captured from is an input of the payload: a
	// drifted catalog fails here, not as an opaque payload mismatch.
	compareConflictsCoreCatalog(t, "catalog input", conflictsCoreTree(t, catalog), golden.Catalog)

	for i, tc := range cases {
		if tc.name != conflictsCoreGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, conflictsCoreGoldenCases[i])
		}
		rec := golden.Cases[i]

		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(rec.IDs, tc.ids) {
				t.Fatalf("%s: frozen ids %v, corpus %v", tc.name, rec.IDs, tc.ids)
			}

			got, gotErr := PlanTagConflicts(catalog, tc.ids)
			if rec.Error != "" {
				if gotErr == nil {
					t.Fatalf("the frozen authority raised %s but native returned %#v", rec.Error, got)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("native error %v but the frozen authority graded %#v", gotErr, rec.Conflicts)
			}
			if tc.wantConflicts && len(rec.Conflicts) == 0 {
				t.Fatal("the frozen authority graded no conflicts; the shared-tag fixture is vacuous")
			}
			if !reflect.DeepEqual(got, rec.Conflicts) {
				t.Fatalf("tag conflicts diverge\n--- go ---\n%#v\n--- frozen ---\n%#v", got, rec.Conflicts)
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
