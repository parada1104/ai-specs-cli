package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the repository root (the go test process runs with cwd =
// the package directory internal/config).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s does not contain go.mod: %v", root, err)
	}
	return root
}

func runPython(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("python3", args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("python3 %v: %v", args, err)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

// TestDifferentialReadSections compares Go ReadSectionJSON output byte-for-
// byte against `python3 lib/_internal/toml-read.py <path> <section>` for
// every fixture manifest, the repo's own manifest (copied to a temp dir —
// never touched in place), and the synthetic testdata manifests.
func TestDifferentialReadSections(t *testing.T) {
	root := repoRoot(t)
	sections := []string{"project", "agents", "deps", "mcp", "recipes", "bindings"}

	// Assemble the manifest corpus.
	type corpusEntry struct {
		name string
		data []byte
	}
	var corpus []corpusEntry

	add := func(name string, data []byte) {
		corpus = append(corpus, corpusEntry{name: name, data: data})
	}

	// tests/fixtures/**/ai-specs.toml
	err := filepath.WalkDir(filepath.Join(root, "tests", "fixtures"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "ai-specs.toml" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			add(filepath.Base(filepath.Dir(path))+"_ai-specs.toml", data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tests/fixtures: %v", err)
	}

	// catalog/recipes/*/recipe.toml
	matches, _ := filepath.Glob(filepath.Join(root, "catalog", "recipes", "*", "recipe.toml"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		add(filepath.Base(filepath.Dir(m))+"_recipe.toml", data)
	}

	// The repo's own ai-specs/ai-specs.toml, copied to a temp dir.
	repoManifest, err := os.ReadFile(filepath.Join(root, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatalf("read repo manifest: %v", err)
	}
	add("_repo_ai-specs.toml", repoManifest)

	// Synthetic testdata manifests.
	synthetic, _ := filepath.Glob("testdata/*.toml")
	for _, s := range synthetic {
		data, err := os.ReadFile(s)
		if err != nil {
			t.Fatalf("read %s: %v", s, err)
		}
		add(filepath.Base(s), data)
	}

	tmp := t.TempDir()
	compared := 0
	whitelisted := 0
	for i, entry := range corpus {
		copyPath := filepath.Join(tmp, fmt.Sprintf("%03d_%s", i, entry.name))
		if err := os.WriteFile(copyPath, entry.data, 0o644); err != nil {
			t.Fatalf("write copy: %v", err)
		}
		for _, section := range sections {
			pyOut, _, code := runPython(t, root, "lib/_internal/toml-read.py", copyPath, section)
			if code != 0 {
				// Only deliberately invalid fixtures may be skipped: python
				// refuses them, so Go must refuse them too (both-fail parity).
				// Any other non-zero python exit is a differential failure and
				// must fail loudly, never silently shrink the corpus.
				if strings.Contains(entry.name, "invalid_base") {
					whitelisted++
					if _, err := LoadManifest(copyPath); err == nil {
						t.Errorf("%s: python rejected the fixture but Go LoadManifest accepted it", entry.name)
					}
				} else {
					t.Errorf("%s [%s]: python toml-read.py exited %d — unexpected refusal, not a whitelisted invalid fixture", entry.name, section, code)
				}
				continue
			}
			data, err := LoadManifest(copyPath)
			if err != nil {
				t.Errorf("%s: Go LoadManifest failed where python succeeded: %v", entry.name, err)
				continue
			}
			got, err := ReadSectionJSON(data, section)
			if err != nil {
				t.Errorf("%s [%s]: Go ReadSectionJSON failed: %v", entry.name, section, err)
				continue
			}
			want := strings.TrimSuffix(pyOut, "\n")
			if got != want {
				t.Errorf("byte mismatch %s [%s]:\n  go:     %s\n  python: %s", entry.name, section, got, want)
			}
			compared++
		}
	}

	t.Logf("differential read: compared %d (file, section) pairs; %d whitelisted invalid-base entries asserted both-fail", compared, whitelisted)
	if compared < 100 {
		t.Fatalf("only %d (file, section) pairs compared — corpus unexpectedly small", compared)
	}
}
