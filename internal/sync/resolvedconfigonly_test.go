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
	Name             string            `json:"name"`
	Recipes          map[string]string `json:"recipes"`
	Manifest         string            `json:"manifest"`
	RC               int               `json:"rc"`
	Stdout           string            `json:"stdout"`
	Stderr           string            `json:"stderr"`
	Document         string            `json:"document"`
	ProjectPathCount map[string]int    `json:"project_path_count"`
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

// expandResolvedConfigOnlyRoot restores the tokenized host path in the three
// captured surfaces. The recorded per-stream occurrence inventory must hold
// exactly (no token may appear where none was captured and none may be
// missing).
func expandResolvedConfigOnlyRoot(t *testing.T, tc resolvedConfigOnlyGoldenCase, root string) (stdout, stderr, document string) {
	t.Helper()
	resolved := shared.RealPath(root)
	surfaces := map[string]*string{
		"stdout":   &tc.Stdout,
		"stderr":   &tc.Stderr,
		"document": &tc.Document,
	}
	for label, surface := range surfaces {
		if got := strings.Count(*surface, "<PROJECT_ROOT>"); got != tc.ProjectPathCount[label] {
			t.Fatalf("scenario %q: %d <PROJECT_ROOT> tokens in %s, inventory says %d", tc.Name, got, label, tc.ProjectPathCount[label])
		}
		*surface = strings.ReplaceAll(*surface, "<PROJECT_ROOT>", resolved)
	}
	return tc.Stdout, tc.Stderr, tc.Document
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

			wantStdout, wantStderr, wantDocument := expandResolvedConfigOnlyRoot(t, scenario, fixture.project)

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
