package sync

// Resolved-hooks document contract (U2e T3c, equivalence in L15/L16 of
// odd/tasks/go-20-u2e-flip.md). The document that `--resolved-hooks-out`
// wrote in the retired Python process tests has no native verb; MaterializeRecipes
// is its only producer. The golden files under testdata/materialize-hooks were
// captured from the legacy recipe-materialize.py process on the same fixtures
// and are compared byte for byte, and the field assertions carried over from
// the retired tests stay explicit.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const hooksRecipeWT = `[recipe]
id = "wt-hook"
name = "WT Hook"
description = "D"
version = "1.0"
%s[[provides.hooks]]
id = "gate"
event = "pre-tool-use"
script = "hooks/gate.sh"
matcher = "Edit|Write"
blocking = true
`

const hooksRecipeWTConfig = `[config.WORKTREE_GATE_PROTECTED]
required = false
type = "string"
default = "main"
[config.worktrees_dir]
required = false
type = "string"
default = ".worktrees"

`

const hooksManifest = "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n" +
	"[recipes.wt-hook]\nenabled = true\nversion = '1.0'\n"

// hooksDocument materializes the wt-hook fixture and returns the document the
// spine produced plus the file it wrote at the requested path.
func hooksDocument(t *testing.T, recipe, manifest string) (string, string) {
	t.Helper()
	fixture := writeMaterializeFixture(t, map[string]string{"wt-hook": recipe}, manifest)
	hooksDir := filepath.Join(fixture.home, "catalog", "recipes", "wt-hook", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "gate.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	out := filepath.Join(t.TempDir(), "resolved-hooks.json")
	result, err := MaterializeRecipes(MaterializeRequest{
		ProjectRoot:      fixture.project,
		AISpecsHome:      fixture.home,
		ResolvedHooksOut: &out,
	})
	if err != nil {
		t.Fatalf("MaterializeRecipes: %v", err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read resolved-hooks file: %v", err)
	}
	return result.ResolvedHooksDocument, string(written)
}

func hooksRecipe(config string) string {
	return fmt.Sprintf(hooksRecipeWT, config)
}

func readGoldenHooks(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "materialize-hooks", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func TestResolvedHooksDocumentShapeMatchesLegacy(t *testing.T) {
	document, written := hooksDocument(t, hooksRecipe(""), hooksManifest)
	want := readGoldenHooks(t, "shape.golden.json")
	if document != want {
		t.Fatalf("resolved-hooks document differs from the legacy golden:\n--- go ---\n%s\n--- legacy ---\n%s", document, want)
	}
	if written != want {
		t.Fatalf("resolved-hooks file differs from the legacy golden:\n%s", written)
	}

	var data struct {
		EnabledAgents []string         `json:"enabled_agents"`
		Hooks         []map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(document), &data); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if !reflect.DeepEqual(data.EnabledAgents, []string{"claude"}) {
		t.Fatalf("enabled_agents = %v, want [claude]", data.EnabledAgents)
	}
	if len(data.Hooks) != 1 {
		t.Fatalf("hooks = %d entries, want 1", len(data.Hooks))
	}
	h := data.Hooks[0]
	checks := map[string]any{
		"recipe":      "wt-hook",
		"id":          "gate",
		"event":       "pre-tool-use",
		"matcher":     "Edit|Write",
		"blocking":    true,
		"script_path": "ai-specs/recipes/wt-hook/hooks/gate.sh",
	}
	for key, want := range checks {
		if !reflect.DeepEqual(h[key], want) {
			t.Fatalf("hook[%s] = %#v, want %#v", key, h[key], want)
		}
	}
}

func TestResolvedHooksEnvCarriesExportedConfigOnly(t *testing.T) {
	manifest := hooksManifest + "[recipes.wt-hook.config]\nWORKTREE_GATE_PROTECTED = 'main development'\n"
	document, written := hooksDocument(t, hooksRecipe(hooksRecipeWTConfig), manifest)
	want := readGoldenHooks(t, "env.golden.json")
	if document != want {
		t.Fatalf("resolved-hooks document differs from the legacy golden:\n--- go ---\n%s\n--- legacy ---\n%s", document, want)
	}
	if written != want {
		t.Fatalf("resolved-hooks file differs from the legacy golden:\n%s", written)
	}

	var data struct {
		Hooks []struct {
			Env map[string]string `json:"env"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(document), &data); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if len(data.Hooks) != 1 {
		t.Fatalf("hooks = %d entries, want 1", len(data.Hooks))
	}
	env := data.Hooks[0].Env
	if env["WORKTREE_GATE_PROTECTED"] != "main development" {
		t.Fatalf("env WORKTREE_GATE_PROTECTED = %q, want %q", env["WORKTREE_GATE_PROTECTED"], "main development")
	}
	if _, ok := env["worktrees_dir"]; ok {
		t.Fatalf("env exports recipe-internal key worktrees_dir: %v", env)
	}
}
