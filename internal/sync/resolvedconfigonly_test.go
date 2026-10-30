package sync

// Differential contract tests for the native resolved-config-only mode
// (GO-07 S14c), FROZEN (PR2-B S2): the Python leg is no longer executed. Its
// behavior is captured once in testdata/resolvedconfigonly_golden.json — the
// exact exit codes, both streams and the written document of
// testdata/resolvedconfigonly_ref.py (which drove the real
// build_resolved_config_only in lib/_internal/recipe-materialize.py with the
// built worktree-gate binary) over the unchanged scenario matrix — and
// sync.BuildResolvedConfigOnly must still agree on the exit code, both streams
// and the written document byte for byte after the single tokenized host path
// (<PROJECT_ROOT>) is expanded with the shared RealPath rule.
//
// Host-path inventory: the resolved project root appears exactly once in the
// document of every success scenario and never in the streams; the fatal
// scenario writes no document and prints only the binding-validation error.
// A missing or mutated bundle is a failure, never a skip. The .py driver file
// itself stays on disk until T5 PR4b.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// resolvedConfigOnlyGoldenCase is one frozen scenario: the exact inputs that
// produced it and the captured {rc, stdout, stderr, document}, with the
// per-stream <PROJECT_ROOT> occurrence inventory.
type resolvedConfigOnlyGoldenCase struct {
	Name              string            `json:"name"`
	Recipes           map[string]string `json:"recipes"`
	Manifest          string            `json:"manifest"`
	RC                int               `json:"rc"`
	Stdout            string            `json:"stdout"`
	Stderr            string            `json:"stderr"`
	Document          string            `json:"document"`
	ProjectPathFields map[string]int    `json:"project_path_fields"`
}

type resolvedConfigOnlyGolden struct {
	Comment string                         `json:"comment"`
	Cases   []resolvedConfigOnlyGoldenCase `json:"cases"`
}

// loadResolvedConfigOnlyGolden reads the frozen authority bundle. A missing
// bundle is a failure, never a skip: the bundle IS the oracle.
func loadResolvedConfigOnlyGolden(t *testing.T) []resolvedConfigOnlyGoldenCase {
	t.Helper()
	path := filepath.Join("testdata", "resolvedconfigonly_golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen resolved-config-only oracle: %v", err)
	}
	var golden resolvedConfigOnlyGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatalf("parse frozen resolved-config-only oracle %s: %v", path, err)
	}
	if len(golden.Cases) == 0 {
		t.Fatalf("frozen resolved-config-only oracle %s carries no cases", path)
	}
	return golden.Cases
}

const resolvedConfigOnlyRootToken = "<PROJECT_ROOT>"

const resolvedConfigOnlyRootSlot = "\"project_root\": \"" + resolvedConfigOnlyRootToken + "\""

// expandResolvedConfigOnlyRoot restores the tokenized host path in the captured
// surfaces, but only where the inventory records it: the streams must carry the
// token exactly per inventory (normally none), and in the document it may occur
// only inside the exact project_root slot. No global replace.
func expandResolvedConfigOnlyRoot(tc resolvedConfigOnlyGoldenCase, root string) (stdout, stderr, document string, err error) {
	resolved := shared.RealPath(root)
	for field := range tc.ProjectPathFields {
		if field != "document.project_root" && field != "stdout" && field != "stderr" {
			return "", "", "", fmt.Errorf("scenario %q: inventory names non-owner field %q", tc.Name, field)
		}
	}
	for _, surface := range []struct{ label, text string }{{"stdout", tc.Stdout}, {"stderr", tc.Stderr}} {
		if got := strings.Count(surface.text, resolvedConfigOnlyRootToken); got != tc.ProjectPathFields[surface.label] {
			return "", "", "", fmt.Errorf("scenario %q: %d %s tokens in %s, inventory says %d", tc.Name, got, resolvedConfigOnlyRootToken, surface.label, tc.ProjectPathFields[surface.label])
		}
	}
	stdout, stderr, document = tc.Stdout, tc.Stderr, tc.Document

	slots := strings.Count(document, resolvedConfigOnlyRootSlot)
	tokens := strings.Count(document, resolvedConfigOnlyRootToken)
	if slots != tokens {
		return "", "", "", fmt.Errorf("scenario %q: %s occurs outside the exact document slot %s", tc.Name, resolvedConfigOnlyRootToken, resolvedConfigOnlyRootSlot)
	}
	if want := tc.ProjectPathFields["document.project_root"]; tokens != want {
		return "", "", "", fmt.Errorf("scenario %q: %d %s tokens in document, inventory says %d", tc.Name, tokens, resolvedConfigOnlyRootToken, want)
	}
	if tokens > 0 {
		document = strings.Replace(document, resolvedConfigOnlyRootSlot, "\"project_root\": \""+resolved+"\"", slots)
	}
	return stdout, stderr, document, nil
}

func readIfExists(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func TestResolvedConfigOnlyDifferential(t *testing.T) {
	for _, scenario := range loadResolvedConfigOnlyGolden(t) {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			fixture := writeMaterializeFixture(t, scenario.Recipes, scenario.Manifest)

			goOut := filepath.Join(t.TempDir(), "resolved-config.json")
			var stdout, stderr strings.Builder
			goErr := BuildResolvedConfigOnly(ResolvedConfigOnlyRequest{
				ProjectRoot: fixture.project,
				AISpecsHome: fixture.home,
				Out:         goOut,
				Stdout:      &stdout,
				Stderr:      &stderr,
			})
			goRC := 0
			if goErr != nil {
				goRC = 1
			}

			wantStdout, wantStderr, wantDocument, err := expandResolvedConfigOnlyRoot(scenario, fixture.project)
			if err != nil {
				t.Fatalf("expand frozen scenario %q: %v", scenario.Name, err)
			}

			if goRC != scenario.RC {
				t.Fatalf("rc: go=%d frozen python=%d\n--- go stderr ---\n%s\n--- frozen python stderr ---\n%s", goRC, scenario.RC, stderr.String(), scenario.Stderr)
			}
			if stdout.String() != wantStdout {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- frozen python ---\n%q", stdout.String(), wantStdout)
			}
			if stderr.String() != wantStderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- frozen python ---\n%q", stderr.String(), wantStderr)
			}

			goDoc := readIfExists(t, goOut)
			assertDocumentEqual(t, "resolved-config", goDoc, wantDocument)
		})
	}
}

// TestResolvedConfigOnlyFrozenExpansionIsSlotScoped is a negative control: the
// token is restored only in the exact project_root slot, so a literal token
// injected into a non-path value must fail instead of being normalized away.
func TestResolvedConfigOnlyFrozenExpansionIsSlotScoped(t *testing.T) {
	scenario := loadResolvedConfigOnlyGolden(t)[0]
	fixture := writeMaterializeFixture(t, scenario.Recipes, scenario.Manifest)

	mutated := scenario
	mutated.Document = strings.Replace(scenario.Document, "\"Rule one\"", "\"<PROJECT_ROOT>\"", 1)
	if mutated.Document == scenario.Document {
		t.Fatalf("injection did not land in the frozen document")
	}
	if _, _, _, err := expandResolvedConfigOnlyRoot(mutated, fixture.project); err == nil {
		t.Fatalf("expanded a <PROJECT_ROOT> token injected outside the project_root slot")
	} else if !strings.Contains(err.Error(), "exact document slot") {
		t.Fatalf("error does not name the non-slot position: %v", err)
	}

	_, _, document, err := expandResolvedConfigOnlyRoot(scenario, fixture.project)
	if err != nil {
		t.Fatalf("expand the unmodified frozen scenario: %v", err)
	}
	want := strings.Replace(scenario.Document, resolvedConfigOnlyRootSlot, "\"project_root\": \""+shared.RealPath(fixture.project)+"\"", 1)
	if document != want {
		t.Fatalf("unmodified document expanded beyond its project_root slot:\n got: %q\nwant: %q", document, want)
	}
}
