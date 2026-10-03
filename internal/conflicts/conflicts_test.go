package conflicts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// oracle runs the REAL recipe-conflicts.py functions and prints the same
// projection the Go side builds: conflicts in list order with sorted recipes,
// or the RecipeValidationError text.
const oracle = `
import importlib.util, json, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("recipe_conflicts_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
catalog = Path(sys.argv[2]); ids = json.loads(sys.argv[3]); bindings = json.loads(sys.argv[4])
def proj(cs):
    return [{"type": getattr(c, "primitive_type", "tag_conflict"),
             "id": getattr(c, "primitive_id", getattr(c, "tag", "")),
             "recipes": sorted(c.recipes), "severity": c.severity} for c in cs]
out = {}
try:
    out["primitive"] = proj(mod.check_recipe_conflicts(catalog, ids))
except mod.RecipeValidationError as exc:
    out["primitive_error"] = str(exc)
out["capability"] = proj(mod.check_capability_conflicts(catalog, ids, bindings))
recipes = []
for rid in ids:
    try:
        recipes.append(mod.load_recipe_toml(catalog / rid / "recipe.toml"))
    except Exception:
        pass
out["tag"] = proj(mod.check_tag_conflicts(recipes))
print(json.dumps(out))
`

type projection struct {
	Primitive      []Conflict `json:"primitive"`
	PrimitiveError string     `json:"primitive_error,omitempty"`
	Capability     []Conflict `json:"capability"`
	Tag            []Conflict `json:"tag"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod")
		}
		dir = parent
	}
}

func recipeToml(id, name, extra string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"" + name + "\"\ndescription = \"d\"\nversion = \"1.0.0\"\n" + extra
}

func TestConflictsDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	script := filepath.Join(repoRoot(t), "lib", "_internal", "recipe-conflicts.py")

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

	cases := []struct {
		name     string
		ids      []string
		bindings []map[string]string
	}{
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

	dir := t.TempDir()
	for id, body := range catalog {
		if err := os.MkdirAll(filepath.Join(dir, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id, "recipe.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids, _ := json.Marshal(tc.ids)
			if tc.bindings == nil {
				tc.bindings = []map[string]string{}
			}
			bind, _ := json.Marshal(tc.bindings)
			cmd := exec.Command("python3", "-c", oracle, script, dir, string(ids), string(bind))
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("oracle: %v\n%s", err, out)
			}
			var legacy projection
			if err := json.Unmarshal(out, &legacy); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
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
			}
			if legacy.PrimitiveError != "" {
				legacy.Primitive = []Conflict{}
			}
			if !reflect.DeepEqual(port, legacy) {
				pj, _ := json.Marshal(port)
				t.Fatalf("\n port   %s\n legacy %s", pj, out)
			}
		})
	}
}
