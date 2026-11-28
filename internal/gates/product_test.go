package gates

// The product predicate (change-ledger-t7 L8-1): one definition of product work,
// read by the write gate, the workstream fact, the divergence list and `changes
// status`, with the project's own exclusions and no folder list.

import (
	"path/filepath"
	"strings"
	"testing"
)

// productManifest is the manifest one project declares its exclusions in.
const productManifest = "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" +
	"product_exclude = [\"internal/gen/**\", \"docs/*.md\", \"vendor/**/testdata/**\"]\n"

// A manifest that declares nothing excludes nothing: the scope is every graded path
// outside the planning trees.
func TestProductScopeIsEveryGradedPathOutsideThePlanningTrees(t *testing.T) {
	root := writeRepo(t, nil)
	scope := LoadProductScope(root)
	for _, rel := range []string{"internal/gates/gates.go", "cmd/ai-specs/main.go",
		"docs/runtime-hooks.md", "tests/run.sh", "README.md", "src/app.go",
		"odd/specs/cap/spec.md"} {
		if !scope.Path(rel) && rel != "odd/specs/cap/spec.md" {
			t.Errorf("Path(%q) = false, want a product path", rel)
		}
	}
	for _, rel := range []string{"odd/tasks/thing.md", "odd/specs/cap/spec.md",
		"odd/changes/thing/specs/cap/spec.md", "openspec/changes/thing/tasks.md",
		"openspec/specs/cap/spec.md", ".claude/settings.json", "pkg/.claude/hooks/x.sh",
		"", "."} {
		if scope.Path(rel) {
			t.Errorf("Path(%q) = true, want it outside the product scope", rel)
		}
	}
}

// The project's own globs narrow the scope: `**` covers any number of segments, a
// single `*` stays inside one, and a path the list does not name stays in scope.
func TestProductScopeAppliesTheProjectsExclusions(t *testing.T) {
	root := writeRepo(t, map[string]string{"ai-specs/ai-specs.toml": productManifest})
	scope := LoadProductScope(root)
	if len(scope.Notes) != 0 {
		t.Fatalf("Notes = %q, want none for a well-formed list", scope.Notes)
	}
	for _, rel := range []string{"internal/gen/x.go", "internal/gen/deep/y.go",
		"docs/deploy.md", "vendor/k8s/testdata/case.yaml"} {
		if scope.Path(rel) {
			t.Errorf("Path(%q) = true, want it excluded by the project", rel)
		}
	}
	for _, rel := range []string{"internal/gates/gates.go", "docs/deep/deploy.md",
		"vendor/k8s/readme.md"} {
		if !scope.Path(rel) {
			t.Errorf("Path(%q) = false, want it in scope: the exclusion does not name it", rel)
		}
	}
}

// A malformed declaration is reported and never applied in either direction: the
// scope is unchanged and the note says why.
func TestProductScopeReportsAMalformedExclusion(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"not a list", "product_exclude = \"internal\""},
		{"an entry that is not a string", "product_exclude = [\"internal/**\", 3]"},
		{"an invalid glob", "product_exclude = [\"internal/[\"]"},
		{"an empty entry", "product_exclude = [\"\"]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{
				"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" + tc.value + "\n",
			})
			scope := LoadProductScope(root)
			if len(scope.Notes) == 0 {
				t.Fatalf("Notes = %q, want the malformed declaration reported", scope.Notes)
			}
			for _, note := range scope.Notes {
				if !strings.Contains(note, "product_exclude") {
					t.Errorf("note %q does not name the key", note)
				}
			}
			if !scope.Path("src/app.go") {
				t.Errorf("Path(src/app.go) = false, want an unrelated path still product")
			}
		})
	}
	// The same value reaches the gate's notes, so the operator sees it where the
	// decision is taken.
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\nproduct_exclude = \"internal\"\n",
	})
	found := false
	for _, note := range WriteFactsFor(root, "odd", "thing", "src/app.go").Verdict().Notes {
		if strings.Contains(note, "product_exclude") {
			found = true
		}
	}
	if !found {
		t.Error("Verdict().Notes grew no note for the malformed product_exclude")
	}
}

// The exclusion is the project's declaration, not a tool default: a manifest that
// resolves outside the root or cannot be parsed declares nothing and excludes nothing.
func TestProductScopeWithoutAReadableManifestExcludesNothing(t *testing.T) {
	root := t.TempDir()
	scope := LoadProductScope(root)
	if len(scope.Exclude) != 0 || len(scope.Notes) != 0 {
		t.Fatalf("Scope = %+v, want an empty, silent scope", scope)
	}
	if !scope.Path(filepath.ToSlash("internal/gates/gates.go")) {
		t.Error("Path(internal/gates/gates.go) = false, want the default product scope")
	}
}
