package skills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// vendoredOracle runs the REAL skill_contract.from_dep + render_skill_markdown
// over deps[0] of a TOML file (tomllib, as vendor-skills.py's load_deps does)
// and an upstream SKILL.md text.
const vendoredOracle = `
import importlib.util, json, sys, tomllib
spec = importlib.util.spec_from_file_location("skill_contract_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
with open(sys.argv[2], "rb") as f:
    dep = tomllib.load(f)["deps"][0]
upstream = open(sys.argv[3], encoding="utf-8").read()
try:
    out = {"out": mod.render_skill_markdown(mod.from_dep(dep, upstream))}
except mod.SkillContractError as exc:
    out = {"error": str(exc)}
print(json.dumps(out))
`

func TestVendoredSkillMarkdownDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	script := legacyModule(t, "skill_contract.py")

	full := "---\nname: up\ndescription: Does things. Trigger: when asked.\n---\n\n\n# Body\ntext\n"
	cases := []struct{ name, dep, upstream string }{
		{"minimal", `id = "my-dep"` + "\n" + `source = "https://x/y.git"`, full},
		{"full-metadata", `id = "my-dep"
source = "https://x/y.git"
vendor_attribution = " Acme \"Tools\" \\ "
version = "2.1.0-rc.1"
license = "MIT"
scope = ["root", "ui"]
auto_invoke = ["When a \"quote\" appears", "Second"]`, full},
		{"scalar-scope-and-auto", `id = "d"
source = "s"
scope = "  root  "
auto_invoke = "Phrase"`, full},
		{"empty-lists-fall-back", `id = "d"
source = "s"
scope = []
auto_invoke = []`, full},
		{"no-frontmatter-upstream", `id = "d"` + "\n" + `source = "s"`, "just a body\n"},
		{"empty-upstream", `id = "d"` + "\n" + `source = "s"`, ""},
		{"list-description", `id = "d"` + "\n" + `source = "s"`, "---\ndescription: [a, 'b']\n---\nbody\n"},
		{"attribution-only", `id = "d"
source = "s"
vendor_attribution = "Org"`, "---\nname: x\n---\n"},
		{"description-trailing-dots", `id = "d"
source = "s"
vendor_attribution = "Org"`, "---\ndescription: Ends here... \n---\nb\n"},
		{"numeric-version", `id = "d"` + "\n" + `source = "s"` + "\n" + `version = 2.5`, full},
		{"int-version-invalid", `id = "d"` + "\n" + `source = "s"` + "\n" + `version = 3`, full},
		{"numeric-license", `id = "d"` + "\n" + `source = "s"` + "\n" + `license = 7`, full},
		{"bool-false-license", `id = "d"` + "\n" + `source = "s"` + "\n" + `license = false`, full},
		{"bad-id", `id = "Bad_Id"` + "\n" + `source = "s"`, full},
		{"missing-id", `source = "s"`, full},
		{"blank-source", `id = "d"` + "\n" + `source = "   "`, full},
		{"non-string-source", `id = "d"` + "\n" + `source = 5`, full},
		{"bad-version", `id = "d"` + "\n" + `source = "s"` + "\n" + `version = "v1"`, full},
		{"scope-non-list", `id = "d"` + "\n" + `source = "s"` + "\n" + `scope = 4`, full},
		{"scope-non-string-item", `id = "d"` + "\n" + `source = "s"` + "\n" + `scope = ["a", 1]`, full},
		{"scope-empty-item", `id = "d"` + "\n" + `source = "s"` + "\n" + `scope = ["a", " "]`, full},
		{"bad-upstream-frontmatter", `id = "d"` + "\n" + `source = "s"`, "---\n  indented: 1\n---\nb\n"},
		{"description-dot-space-tail", `id = "d"` + "\n" + `source = "s"`, "---\ndescription: Ends . .\n---\nb\n"},
		{"python-only-space", `id = "d"
source = "s"
vendor_attribution = "\u001fOrg\u001c"`, "---\nname: x\n---\n"},
		{"unicode-space-strip", `id = "d"
source = "s"
vendor_attribution = "\u00a0Org\u2003"`, "---\ndescription: \u3000Hi\u00a0\n---\nb\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			depPath := filepath.Join(dir, "dep.toml")
			upPath := filepath.Join(dir, "SKILL.md")
			writeFileT(t, depPath, "[[deps]]\n"+tc.dep+"\n")
			writeFileT(t, upPath, tc.upstream)

			raw := runPythonStdout(t, "-c", vendoredOracle, script, depPath, upPath)
			var legacy struct {
				Out   *string `json:"out"`
				Error *string `json:"error"`
			}
			if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
				t.Fatalf("decode oracle: %v\n%s", err, raw)
			}

			data, err := os.ReadFile(depPath)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := toml.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			deps, _ := doc.Tables("deps")
			got, gotErr := VendoredSkillMarkdown(deps[0], tc.upstream)

			switch {
			case legacy.Error != nil:
				if gotErr == nil || gotErr.Error() != *legacy.Error {
					t.Fatalf("error: port %v (out %q), legacy %q", gotErr, got, *legacy.Error)
				}
			case gotErr != nil:
				t.Fatalf("port error %v, legacy out %q", gotErr, *legacy.Out)
			case got != *legacy.Out:
				t.Fatalf("markdown:\n port   %q\n legacy %q", got, *legacy.Out)
			}
		})
	}
}
