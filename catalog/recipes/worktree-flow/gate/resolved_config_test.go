package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func entry(id string, table map[string]any) manifestEntry {
	return manifestEntry{ID: id, Table: table}
}

func TestProjectRecipesFlatAndConfigStyle(t *testing.T) {
	entries := []manifestEntry{
		entry("flat-recipe", map[string]any{"enabled": true, "version": "1.2.3", "key": "value"}),
		entry("table-recipe", map[string]any{"enabled": true, "version": "9.9.9", "config": map[string]any{"gate_mode": "ask"}}),
		entry("mixed-recipe", map[string]any{"enabled": true, "flat": "kept", "config": map[string]any{"inner": float64(1)}}),
	}
	want := map[string]map[string]any{
		"flat-recipe":  {"key": "value"},
		"table-recipe": {"gate_mode": "ask"},
		"mixed-recipe": {"flat": "kept", "inner": float64(1)},
	}
	if got := projectRecipes(entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("projectRecipes = %#v, want %#v", got, want)
	}
}

func TestProjectRecipesSkipsNonTableEntry(t *testing.T) {
	got := projectRecipes([]manifestEntry{
		entry("good", map[string]any{"enabled": true}),
		entry("bad", nil),
	})
	want := map[string]map[string]any{"good": {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectRecipes = %#v, want %#v", got, want)
	}
}

func TestProjectEnabledManifestOrder(t *testing.T) {
	got := projectEnabled([]manifestEntry{
		entry("c", map[string]any{"enabled": true}),
		entry("a", map[string]any{}),
		entry("b", map[string]any{"enabled": false}),
		entry("d", map[string]any{"enabled": "yes"}),
		entry("e", map[string]any{"enabled": true}),
	})
	want := []string{"c", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectEnabled = %#v, want %#v", got, want)
	}
}

func TestProjectBindingsExplicitOnly(t *testing.T) {
	got := projectBindings([]map[string]any{
		{"capability": "tracker", "recipe": "jinna-tracker"},
		{"capability": "", "recipe": "dropped"},
		{"capability": "vcs", "recipe": ""},
		{"recipe": "dropped"},
		{"capability": "flow", "recipe": "worktree-flow"},
	})
	want := map[string]string{"tracker": "jinna-tracker", "flow": "worktree-flow"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectBindings = %#v, want %#v", got, want)
	}
}

func TestTopologyConfiguredPrecedence(t *testing.T) {
	legacy := entry("worktree-flow", map[string]any{
		"enabled": true,
		"config":  map[string]any{"repo_topology": "monorepo-apps"},
	})

	got, source := topologyConfigured(map[string]any{"repo_topology": "monorepo-submodules"}, []manifestEntry{legacy})
	if got != "monorepo-submodules" || source != topologySourceProject {
		t.Fatalf("[project] precedence = (%q, %q), want (%q, %q)",
			got, source, "monorepo-submodules", topologySourceProject)
	}

	got, source = topologyConfigured(map[string]any{}, []manifestEntry{legacy})
	if got != "monorepo-apps" || source != topologySourceLegacyRecipe {
		t.Fatalf("legacy fallback = (%q, %q), want (%q, %q)",
			got, source, "monorepo-apps", topologySourceLegacyRecipe)
	}

	got, source = topologyConfigured(map[string]any{}, nil)
	if got != "auto" || source != topologySourceDefault {
		t.Fatalf("default = (%q, %q), want (%q, %q)", got, source, "auto", topologySourceDefault)
	}
}

func TestResolveRepoTopologyConfiguredPassthrough(t *testing.T) {
	root := t.TempDir()
	for _, configured := range []string{"standalone", "monorepo-apps"} {
		got := resolveRepoTopology(root, configured)
		if got.Resolved != configured || got.Via != topologyViaConfig {
			t.Fatalf("%q: resolved = (%q, %q), want (%q, %q)",
				configured, got.Resolved, got.Via, configured, topologyViaConfig)
		}
		if len(got.Submodules) != 0 || got.GitmodulesPresent {
			t.Fatalf("%q: unexpected detection: %#v", configured, got)
		}
	}
}

func TestResolveRepoTopologyAutoStandalone(t *testing.T) {
	got := resolveRepoTopology(t.TempDir(), "auto")
	if got.Resolved != "standalone" || got.Via != topologyViaAuto {
		t.Fatalf("resolved = (%q, %q), want (%q, %q)",
			got.Resolved, got.Via, "standalone", topologyViaAuto)
	}
	if len(got.Submodules) != 0 || got.GitmodulesPresent {
		t.Fatalf("unexpected detection: %#v", got)
	}
}

func TestResolveRepoTopologyAutoDetectsSubmodules(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	super := makeSuper(t, makeRemoteModule(t))

	got := resolveRepoTopology(super, "auto")
	if got.Resolved != "monorepo-submodules" || got.Via != topologyViaAuto {
		t.Fatalf("resolved = (%q, %q), want (%q, %q)",
			got.Resolved, got.Via, "monorepo-submodules", topologyViaAuto)
	}
	if !got.GitmodulesPresent {
		t.Fatalf("gitmodules_present = false, want true: %#v", got)
	}
	if want := []string{"apps/api"}; !reflect.DeepEqual(got.Submodules, want) {
		t.Fatalf("submodules = %#v, want %#v", got.Submodules, want)
	}
}

func TestBuildResolvedConfigEnvelope(t *testing.T) {
	payload := manifestPayload{
		Recipes: []manifestEntry{
			entry("jinna-tracker", map[string]any{"enabled": true, "version": "1.0.0"}),
			entry("worktree-flow", map[string]any{"enabled": true, "config": map[string]any{"gate_mode": "ask"}}),
		},
		Bindings: []map[string]any{{"capability": "tracker", "recipe": "jinna-tracker"}},
	}
	root := RealPath(t.TempDir())

	got := buildResolvedConfig(payload, root)
	if got.ProjectRoot != root {
		t.Fatalf("project_root = %q, want %q", got.ProjectRoot, root)
	}
	if want := []string{"jinna-tracker", "worktree-flow"}; !reflect.DeepEqual(got.Enabled, want) {
		t.Fatalf("enabled = %#v, want %#v", got.Enabled, want)
	}
	if got.Bindings["tracker"] != "jinna-tracker" {
		t.Fatalf("bindings = %#v, want tracker=jinna-tracker", got.Bindings)
	}
	if got.Recipes["worktree-flow"]["gate_mode"] != "ask" {
		t.Fatalf("worktree-flow config = %#v, want gate_mode=ask", got.Recipes["worktree-flow"])
	}
	if _, ok := got.Recipes["jinna-tracker"]["version"]; ok {
		t.Fatalf("jinna-tracker config kept version: %#v", got.Recipes["jinna-tracker"])
	}
	if got.Topology.Resolved != "standalone" {
		t.Fatalf("topology resolved = %q, want standalone", got.Topology.Resolved)
	}
}

func TestRunPlanResolvedConfigEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	writeResolvedManifest(t, root, `
[project]
repo_topology = "standalone"

[recipes.worktree-flow]
enabled = true

[recipes.worktree-flow.config]
gate_mode = "ask"

[[bindings]]
capability = "flow"
recipe = "worktree-flow"
`)

	var stdout, stderr strings.Builder
	if code := runPlanResolvedConfig(root, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	var got resolvedConfig
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("unmarshal envelope: %v (stdout=%q)", err, stdout.String())
	}
	if got.Topology.Resolved != "standalone" || got.Topology.Source != topologySourceProject {
		t.Fatalf("topology = %#v, want resolved=standalone source=project", got.Topology)
	}
	if want := []string{"worktree-flow"}; !reflect.DeepEqual(got.Enabled, want) {
		t.Fatalf("enabled = %#v, want %#v", got.Enabled, want)
	}
	if got.Bindings["flow"] != "worktree-flow" {
		t.Fatalf("bindings = %#v, want flow=worktree-flow", got.Bindings)
	}
	if got.ProjectRoot != RealPath(root) {
		t.Fatalf("project_root = %q, want %q", got.ProjectRoot, RealPath(root))
	}
}

func TestRunPlanResolvedConfigMissingManifestFailsClosed(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := runPlanResolvedConfig(t.TempDir(), &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestResolvedConfigSubmodulesSorted(t *testing.T) {
	got := sortedUnique([]string{"b", "a", "a", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedUnique = %#v, want %#v", got, want)
	}
}

func writeResolvedManifest(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
