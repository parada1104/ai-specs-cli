package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// The legacy vendored-skill oracle (lib/_internal/skill_contract.py from_dep +
// render_skill_markdown over deps[0] of a TOML file, loaded with tomllib as
// vendor-skills.py does) was captured once into
// testdata/vendored_skill_golden.json. The test never execs python3 or reads
// lib/. The golden is a snapshot: regenerating it needs a human decision.

const vendoredSkillGoldenPath = "testdata/vendored_skill_golden.json"

type frozenVendoredCase struct {
	Name   string   `json:"name"`
	Inputs []string `json:"inputs"`
	Out    string   `json:"out"`
	Error  string   `json:"error"`
}

type frozenVendored struct {
	Oracle map[string]any       `json:"oracle"`
	Cases  []frozenVendoredCase `json:"cases"`
}

// expectedVendoredGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedVendoredGoldenCases = []string{
	"minimal",
	"full-metadata",
	"scalar-scope-and-auto",
	"empty-lists-fall-back",
	"no-frontmatter-upstream",
	"empty-upstream",
	"list-description",
	"attribution-only",
	"description-trailing-dots",
	"numeric-version",
	"int-version-invalid",
	"numeric-license",
	"bool-false-license",
	"bad-id",
	"missing-id",
	"blank-source",
	"non-string-source",
	"bad-version",
	"scope-non-list",
	"scope-non-string-item",
	"scope-empty-item",
	"bad-upstream-frontmatter",
	"description-dot-space-tail",
	"python-only-space",
	"multiline-attribution-frozen",
	"unicode-space-strip",
}

type vendoredCase struct{ name, dep, upstream string }

func vendoredCases() []vendoredCase {
	full := "---\nname: up\ndescription: Does things. Trigger: when asked.\n---\n\n\n# Body\ntext\n"
	return []vendoredCase{
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
		{"multiline-attribution-frozen", `id = "d"
source = "s"
vendor_attribution = "Line one\nline two"`, full},
		{"unicode-space-strip", `id = "d"
source = "s"
vendor_attribution = "\u00a0Org\u2003"`, "---\ndescription: \u3000Hi\u00a0\n---\nb\n"},
	}
}

// corpusInputs pins the exact bytes a case feeds the oracle: the dep.toml the
// test writes and the upstream SKILL.md text.
func corpusInputs(t *testing.T, dir string) []string {
	t.Helper()
	lines := make([]string, 0, 2)
	for _, name := range []string{"dep.toml", "SKILL.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read corpus input %s: %v", name, err)
		}
		sum := sha256.Sum256(data)
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+name)
	}
	return lines
}

func loadVendoredGolden(t *testing.T) frozenVendored {
	t.Helper()
	raw, err := os.ReadFile(vendoredSkillGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", vendoredSkillGoldenPath, err)
	}
	var golden frozenVendored
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", vendoredSkillGoldenPath, err)
	}
	return golden
}

// TestVendoredSkillMarkdownDifferential compares the Go port with the frozen
// legacy oracle over every dep/upstream pair: rendered markdown, or the exact
// SkillContractError message.
func TestVendoredSkillMarkdownDifferential(t *testing.T) {
	golden := loadVendoredGolden(t)
	corpus := vendoredCases()

	if len(corpus) != len(expectedVendoredGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(corpus), len(expectedVendoredGoldenCases))
	}
	if len(golden.Cases) != len(expectedVendoredGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedVendoredGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedVendoredGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || corpus[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, corpus[i].name, want)
		}
	}

	for i, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			frozen := golden.Cases[i]
			dir := t.TempDir()
			depPath := filepath.Join(dir, "dep.toml")
			writeFileT(t, depPath, "[[deps]]\n"+tc.dep+"\n")
			writeFileT(t, filepath.Join(dir, "SKILL.md"), tc.upstream)

			gotInputs := corpusInputs(t, dir)
			if len(gotInputs) != len(frozen.Inputs) || gotInputs[0] != frozen.Inputs[0] || gotInputs[1] != frozen.Inputs[1] {
				t.Fatalf("%s: frozen inputs differ from the corpus: frozen %v, corpus %v", tc.name, frozen.Inputs, gotInputs)
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

			if frozen.Error != "" {
				if gotErr == nil || gotErr.Error() != frozen.Error {
					t.Fatalf("error: port %v (out %q), legacy %q", gotErr, got, frozen.Error)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("port error %v, legacy out %q", gotErr, frozen.Out)
			}
			if got != frozen.Out {
				t.Fatalf("markdown:\n port   %q\n legacy %q", got, frozen.Out)
			}
		})
	}
}
