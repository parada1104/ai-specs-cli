package conflicts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// The legacy recipe-conflicts.py oracle (the projection below: conflicts in
// list order with sorted recipes, or the RecipeValidationError text) was
// captured once into testdata/recipe_conflicts_golden.json. The test never execs
// python3 or reads lib/. The golden is a snapshot: regenerating it needs a human
// decision.

const conflictsGoldenPath = "testdata/recipe_conflicts_golden.json"

// catalogToken replaces the fabricated catalog directory: it is the only
// host-dependent value in the payload, and it reaches the frozen snapshot only
// through the "recipe directory not found: <dir>/<id>" error text.
const catalogToken = "<CATALOG>"

type projection struct {
	Primitive      []Conflict `json:"primitive"`
	PrimitiveError string     `json:"primitive_error,omitempty"`
	Capability     []Conflict `json:"capability"`
	Tag            []Conflict `json:"tag"`
}

type frozenConflictsCase struct {
	Name     string              `json:"name"`
	IDs      []string            `json:"ids"`
	Bindings []map[string]string `json:"bindings"`
	Legacy   projection          `json:"legacy"`
}

type frozenConflicts struct {
	Oracle map[string]any        `json:"oracle"`
	Corpus []string              `json:"corpus"`
	Cases  []frozenConflictsCase `json:"cases"`
}

// expectedConflictsGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedConflictsGoldenCases = []string{
	"no-conflicts",
	"skill-command-tag-cap",
	"order-reversed",
	"dup-inside-one-recipe-stops-registration",
	"mcp-collision",
	"explicit-binding-silences-warning",
	"duplicate-explicit-binding-fatal",
	"binding-missing-keys",
	"missing-recipe-dir",
	"invalid-recipe",
}

func recipeToml(id, name, extra string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"" + name + "\"\ndescription = \"d\"\nversion = \"1.0.0\"\n" + extra
}

type conflictsCase struct {
	name     string
	ids      []string
	bindings []map[string]string
}

// differentialCorpus is the fabricated catalog plus the enabled-recipe and
// binding sets the oracle was captured over. The catalog is shared by every
// case and pinned by a sha256 manifest in the golden.
func differentialCorpus() (map[string]string, []conflictsCase) {
	skills := func(ids ...string) string {
		s := "skills = ["
		for _, id := range ids {
			s += `{ id = "` + id + `", source = "bundled" }, `
		}
		return s + "]\n"
	}
	catalog := map[string]string{
		"a": recipeToml("a", "Rec A", `tags = ["tracker", "vcs"]
conflicts_with = ["c"]
[provides]
`+skills("s1", "s2")+`commands = [{ id = "cmd1", path = "c.md" }]
capabilities = [{ id = "tracker" }]
`),
		"b": recipeToml("b", "Rec B", `tags = ["vcs", "vcs"]
[provides]
`+skills("s2", "s3")+`capabilities = [{ id = "tracker" }, { id = "vault" }]
`),
		"c": recipeToml("c", "Rec C", `tags = ["tracker"]
[provides]
commands = [{ id = "cmd1", path = "x.md" }]
capabilities = [{ id = "vault" }]
`),
		"dup": recipeToml("dup", "Rec Dup", `tags = ["solo", "solo"]
[provides]
`+skills("x", "x", "y")),
		"mcp1":   recipeToml("mcp1", "M1", "[provides]\nmcp = [{ id = \"srv\" }]\n"),
		"mcp2":   recipeToml("mcp2", "M2", "[provides]\nmcp = [{ id = \"srv\" }]\n"),
		"broken": "[recipe]\nid = \"broken\"\n",
	}
	cases := []conflictsCase{
		{"no-conflicts", []string{"a", "mcp1"}, nil},
		{"skill-command-tag-cap", []string{"a", "b", "c"}, nil},
		{"order-reversed", []string{"c", "b", "a"}, nil},
		{"dup-inside-one-recipe-stops-registration", []string{"dup", "a"}, nil},
		{"mcp-collision", []string{"mcp1", "mcp2"}, nil},
		{"explicit-binding-silences-warning", []string{"a", "b", "c"}, []map[string]string{{"capability": "tracker", "recipe": "a"}}},
		{"duplicate-explicit-binding-fatal", []string{"a", "b"}, []map[string]string{
			{"capability": "vault", "recipe": "b"}, {"capability": "tracker", "recipe": "a"}, {"capability": "vault", "recipe": "c"}}},
		{"binding-missing-keys", []string{"a", "b"}, []map[string]string{{}, {}}},
		{"missing-recipe-dir", []string{"a", "nope", "b"}, nil},
		{"invalid-recipe", []string{"a", "broken"}, nil},
	}
	return catalog, cases
}

// catalogManifest pins the catalog inputs the oracle was captured from as
// sorted "sha256  <id>/recipe.toml" lines, so corpus drift fails as input drift
// instead of an opaque payload mismatch.
func catalogManifest(t *testing.T, dir string, catalog map[string]string) []string {
	t.Helper()
	lines := make([]string, 0, len(catalog))
	for id := range catalog {
		rel := id + "/recipe.toml"
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("manifest %s: %v", rel, err)
		}
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), rel))
	}
	sort.Strings(lines)
	return lines
}

func loadConflictsGolden(t *testing.T) frozenConflicts {
	t.Helper()
	raw, err := os.ReadFile(conflictsGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", conflictsGoldenPath, err)
	}
	var golden frozenConflicts
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", conflictsGoldenPath, err)
	}
	return golden
}

// TestConflictsDifferential compares the Go port with the frozen legacy oracle
// across every fabricated case: the primitive conflicts or the
// RecipeValidationError text, the capability conflicts and the tag conflicts.
// Inputs are pinned by a catalog manifest and by the per-case ids/bindings.
func TestConflictsDifferential(t *testing.T) {
	golden := loadConflictsGolden(t)
	catalog, cases := differentialCorpus()

	if len(cases) != len(expectedConflictsGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(expectedConflictsGoldenCases))
	}
	if len(golden.Cases) != len(expectedConflictsGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedConflictsGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedConflictsGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || cases[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, cases[i].name, want)
		}
	}

	dir := t.TempDir()
	for id, body := range catalog {
		if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id, "recipe.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gotCorpus := catalogManifest(t, dir, catalog)
	if strings.Join(gotCorpus, "\n") != strings.Join(golden.Corpus, "\n") {
		t.Fatalf("frozen inputs differ from the corpus catalog:\n--- frozen ---\n%s\n--- corpus ---\n%s",
			strings.Join(golden.Corpus, "\n"), strings.Join(gotCorpus, "\n"))
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy := golden.Cases[i].Legacy
			if !reflect.DeepEqual(golden.Cases[i].IDs, tc.ids) {
				t.Fatalf("%s: frozen ids %v, corpus %v", tc.name, golden.Cases[i].IDs, tc.ids)
			}
			if tc.bindings == nil {
				tc.bindings = []map[string]string{}
			}
			if !reflect.DeepEqual(golden.Cases[i].Bindings, tc.bindings) {
				t.Fatalf("%s: frozen bindings %v, corpus %v", tc.name, golden.Cases[i].Bindings, tc.bindings)
			}

			var port projection
			if cs, err := CheckRecipeConflicts(dir, tc.ids); err != nil {
				port.PrimitiveError = err.Error()
			} else {
				port.Primitive = cs
			}
			port.Capability = CheckCapabilityConflicts(dir, tc.ids, tc.bindings)
			var recipes []*schema.Recipe
			for _, id := range tc.ids {
				if r, err := schema.LoadRecipeToml(filepath.Join(dir, id, "recipe.toml")); err == nil {
					recipes = append(recipes, r)
				}
			}
			port.Tag = CheckTagConflicts(recipes)

			norm := func(p *projection) {
				for _, s := range []*[]Conflict{&p.Primitive, &p.Capability, &p.Tag} {
					if *s == nil {
						*s = []Conflict{}
					}
				}
			}
			norm(&legacy) // port slices are never nil (asserted below)
			if port.Capability == nil || port.Tag == nil || (port.PrimitiveError == "" && port.Primitive == nil) {
				t.Fatal("port returned a nil slice; JSON would be null, not []")
			}
			if port.PrimitiveError != "" {
				port.Primitive = []Conflict{}
				port.PrimitiveError = strings.ReplaceAll(port.PrimitiveError, dir, catalogToken)
			}
			if legacy.PrimitiveError != "" {
				legacy.Primitive = []Conflict{}
			}
			if !reflect.DeepEqual(port, legacy) {
				pj, _ := json.Marshal(port)
				lj, _ := json.Marshal(legacy)
				t.Fatalf("\n port   %s\n legacy %s", pj, lj)
			}
		})
	}
}
