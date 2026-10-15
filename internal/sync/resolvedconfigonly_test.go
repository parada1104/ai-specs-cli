package sync

// Differential contract tests for the native resolved-config-only mode
// (GO-07 S14c). The Python authority is driven through
// testdata/resolvedconfigonly_ref.py and the two legs must agree on the exit
// code, both streams, and the written document byte for byte.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func resolvedConfigOnlyRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "resolvedconfigonly_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

type resolvedConfigOnlyRefOut struct {
	RC       int    `json:"rc"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Document string `json:"document"`
}

func runResolvedConfigOnlyRef(t *testing.T, gate, driver, authority, project, home, out string) resolvedConfigOnlyRefOut {
	t.Helper()
	cmd := exec.Command("python3", driver, authority, project, home, out)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"WORKTREE_GATE_BIN="+gate,
		"AI_SPECS_GATE_OFFLINE=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	var out2 resolvedConfigOnlyRefOut
	if err := json.Unmarshal(stdout.Bytes(), &out2); err != nil {
		t.Fatalf("decode ref %s: %v", stdout.String(), err)
	}
	return out2
}

func TestResolvedConfigOnlyDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	authority, err := filepath.Abs(filepath.Join("..", "..", "lib", "_internal", "recipe-materialize.py"))
	if err != nil {
		t.Fatalf("resolve authority: %v", err)
	}
	if _, err := os.Stat(authority); err != nil {
		t.Skipf("recipe-materialize.py unavailable: %v", err)
	}
	driver := resolvedConfigOnlyRefPath(t)
	gate := materializeGateBinary(t)

	enriched := syncRecipeHeader("alpha", "Alpha") +
		"\n[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"development\"\n" +
		"\n[provides.brief]\nworkflow_rules = [\"Rule one\"]\n"

	scenarios := []struct {
		name     string
		recipes  map[string]string
		manifest string
	}{
		{
			name: "auto-bound capability with catalog defaults and brief fragments",
			recipes: map[string]string{
				"alpha": enriched + "\n[[capabilities]]\nid = \"tracker\"\n",
			},
			manifest: "[recipes.alpha]\nenabled = true\n",
		},
		{
			name: "malformed explicit binding is fatal",
			recipes: map[string]string{
				"alpha": enriched,
			},
			manifest: "[recipes.alpha]\nenabled = true\n\n[[bindings]]\ncapability = \"tracker\"\nrecipe = \"ghost\"\n",
		},
		{
			name: "no enabled recipes keeps the explicit bindings",
			recipes: map[string]string{
				"alpha": enriched + "\n[[capabilities]]\nid = \"tracker\"\n",
			},
			manifest: "[recipes.alpha]\nenabled = false\n\n[[bindings]]\ncapability = \"tracker\"\nrecipe = \"alpha\"\n",
		},
		{
			name:     "absent catalog degrades to the explicit bindings",
			recipes:  map[string]string{},
			manifest: "[recipes.alpha]\nenabled = true\n",
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			fixture := writeMaterializeFixture(t, scenario.recipes, scenario.manifest)

			pyOut := filepath.Join(t.TempDir(), "resolved-config.json")
			ref := runResolvedConfigOnlyRef(t, gate, driver, authority, fixture.project, fixture.home, pyOut)

			goOut := filepath.Join(t.TempDir(), "resolved-config.json")
			var stdout, stderr bytes.Buffer
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

			if goRC != ref.RC {
				t.Fatalf("rc: go=%d python=%d\n--- go stderr ---\n%s\n--- python stderr ---\n%s", goRC, ref.RC, stderr.String(), ref.Stderr)
			}
			if stdout.String() != ref.Stdout {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- python ---\n%q", stdout.String(), ref.Stdout)
			}
			if stderr.String() != ref.Stderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- python ---\n%q", stderr.String(), ref.Stderr)
			}

			goDoc := readIfExists(t, goOut)
			assertDocumentEqual(t, "resolved-config", goDoc, ref.Document)
		})
	}
}

func readIfExists(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
