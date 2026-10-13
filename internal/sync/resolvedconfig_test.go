package sync

// T4 differential for the native root resolved-config authority (GO-07 S13 PR3).
//
// BuildResolvedConfig acquires the project manifest natively and routes it into
// the shared decision core (ai-specs.dev/worktree-gate/shared.BuildResolvedConfig)
// the gate binary's --plan-resolved-config surface also runs, so the root single
// binary and the gate share one projection — never a second port. This file pins
// the adapter it adds against the real Python fail-open body in
// lib/_internal/recipe-materialize.py via testdata/resolvedconfig_ref.py:
// _python_build_resolved_config (:3763), the temporary authority the bridge
// falls back to. Bindings, per-recipe config, enabled order, project root and
// the resolved topology (including the source label and the sorted submodule
// list) must agree case for case, and the acquisition errors must agree on
// failure.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// resolvedConfigRefPath resolves the reference driver under this package (go
// test cwd is internal/sync).
func resolvedConfigRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "resolvedconfig_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

// resolvedConfigRefResult is the canonical JSON testdata/resolvedconfig_ref.py
// emits. Result is the Python envelope; ExcType/Message carry a raised failure.
type resolvedConfigRefResult struct {
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
	ExcType string          `json:"exc_type"`
	Message string          `json:"message"`
}

// writeResolvedConfigManifest writes <root>/ai-specs/ai-specs.toml.
func writeResolvedConfigManifest(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// decodeGeneric decodes one JSON document into untyped Go values, so the Go and
// Python envelopes are compared after an identical JSON round-trip (numbers
// become float64 on both sides, so 7 and 7.0 compare equal as Python's dict
// equality does).
func decodeGeneric(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return value
}

// assertResolvedConfigParity runs the Go authority and the real Python body on
// the same root (both read-only) and requires equality of the whole envelope.
// When the Python body raises, the Go authority must return a Go error.
func assertResolvedConfigParity(t *testing.T, python, driver, authority, root string) shared.ResolvedConfig {
	t.Helper()
	raw := runBindingsRefRaw(t, python, driver, "project", authority, root)
	var ref resolvedConfigRefResult
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("decode ref %s: %v", raw, err)
	}

	got, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: root})
	if !ref.OK {
		if err == nil {
			t.Fatalf("go accepted the manifest python rejected (%s: %s)", ref.ExcType, ref.Message)
		}
		return shared.ResolvedConfig{}
	}
	if err != nil {
		t.Fatalf("BuildResolvedConfig: %v (python authority succeeded)", err)
	}
	goRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal go result: %v", err)
	}
	if !reflect.DeepEqual(decodeGeneric(t, goRaw), decodeGeneric(t, ref.Result)) {
		t.Fatalf("resolved config mismatch:\n go: %s\n py: %s", goRaw, ref.Result)
	}
	return got
}

// TestNativeResolvedConfigDifferentialAgainstPython pins the native acquisition
// and the shared projection against _python_build_resolved_config for the
// manifest shapes the Python authority distinguishes: flat and config-style
// recipes, explicit bindings, and each topology source/label branch.
func TestNativeResolvedConfigDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)

	cases := []struct {
		name     string
		manifest string
	}{
		{
			"flat and config recipes with enabled order and bindings",
			"[project]\nname = \"x\"\nrepo_topology = \"standalone\"\n\n" +
				"[recipes.flat-recipe]\nenabled = true\nversion = \"1.0.0\"\nflat_setting = \"on\"\nanother = 7\n\n" +
				"[recipes.config-recipe]\nenabled = false\nversion = \"2.0.0\"\n\n" +
				"[recipes.config-recipe.config]\nnested_setting = \"yes\"\ncount = 3\n\n" +
				"[[bindings]]\ncapability = \"tracker\"\nrecipe = \"flat-recipe\"\n",
		},
		{
			"explicit bindings with an incomplete pair dropped",
			"[[bindings]]\ncapability = \"tracker\"\nrecipe = \"recipe-a\"\n\n" +
				"[[bindings]]\ncapability = \"vcs\"\nrecipe = \"recipe-b\"\n\n" +
				"[[bindings]]\ncapability = \"\"\nrecipe = \"recipe-c\"\n",
		},
		{"project topology is the project source label", "[project]\nrepo_topology = \"standalone\"\n\n[recipes.a]\nenabled = true\n"},
		{"legacy recipe config topology is the legacy-recipe label", "[recipes.worktree-flow]\nenabled = true\n\n[recipes.worktree-flow.config]\nrepo_topology = \"monorepo-apps\"\n"},
		{"legacy recipe flat topology is the legacy-recipe label", "[recipes.worktree-flow]\nenabled = true\nrepo_topology = \"monorepo-apps\"\n"},
		{"blank project topology falls back to the default label", "[project]\nrepo_topology = \"   \"\n\n[recipes.worktree-flow]\nenabled = true\n\n[recipes.worktree-flow.config]\nrepo_topology = \"monorepo-apps\"\n"},
		{"non-string project topology falls back to the default label", "[project]\nrepo_topology = 5\n"},
		{"unknown configured topology takes the auto branch", "[project]\nrepo_topology = \"weird\"\n"},
		{"empty manifest is auto standalone at the default label", "# no recipes table\n"},
		{
			"inline array of inline-table bindings is acquired in document order",
			"bindings = [{capability = \"tracker\", recipe = \"a\"}, {capability = \"vcs\", recipe = \"b\"}]\n\n" +
				"[recipes.a]\nenabled = true\n",
		},
		{
			"inline array bindings skipping non-table items keep only the tables",
			"bindings = [{capability = \"tracker\", recipe = \"a\"}, 5, \"x\", {capability = \"vcs\", recipe = \"b\"}]\n\n" +
				"[recipes.a]\nenabled = true\n",
		},
		{
			"inline array binding missing the capability string is dropped",
			"bindings = [{recipe = \"a\"}]\n\n[recipes.a]\nenabled = true\n",
		},
		{
			"inline array binding missing the recipe string is dropped",
			"bindings = [{capability = \"tracker\"}]\n\n[recipes.a]\nenabled = true\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeResolvedConfigManifest(t, root, tc.manifest)
			assertResolvedConfigParity(t, python, driver, authority, root)
		})
	}
}

// TestNativeResolvedConfigAcquisitionErrors pins the transport failures: a
// missing or unparsable manifest raises in the Python authority and is a Go
// error natively.
func TestNativeResolvedConfigAcquisitionErrors(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)

	t.Run("missing manifest", func(t *testing.T) {
		assertResolvedConfigParity(t, python, driver, authority, t.TempDir())
	})
	t.Run("unparsable manifest", func(t *testing.T) {
		root := t.TempDir()
		writeResolvedConfigManifest(t, root, "not = [valid\n")
		assertResolvedConfigParity(t, python, driver, authority, root)
	})
}

// resolvedConfigGit runs one git command in dir and fails the test on error.
func resolvedConfigGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// resolvedConfigMakeRemote creates a local bare remote for one submodule.
func resolvedConfigMakeRemote(t *testing.T, name string) string {
	t.Helper()
	src := t.TempDir()
	resolvedConfigGit(t, src, "init", "-q")
	resolvedConfigGit(t, src, "config", "user.email", "t@t.t")
	resolvedConfigGit(t, src, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte(name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedConfigGit(t, src, "add", "-A")
	resolvedConfigGit(t, src, "commit", "-qm", "init")
	resolvedConfigGit(t, src, "checkout", "-q", "-B", "main")
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "clone", "--bare", "-q", src, remote).CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %v: %s", err, out)
	}
	return "file://" + remote
}

// resolvedConfigMakeSuper creates a superproject with two initialized submodules
// added in non-alphabetical order, so the sorted contract is observable.
func resolvedConfigMakeSuper(t *testing.T) string {
	t.Helper()
	super := t.TempDir()
	resolvedConfigGit(t, super, "init", "-q")
	resolvedConfigGit(t, super, "config", "user.email", "t@t.t")
	resolvedConfigGit(t, super, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(super, "ROOT"), []byte("super\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedConfigGit(t, super, "add", "-A")
	resolvedConfigGit(t, super, "commit", "-qm", "root")
	for _, rel := range []string{"zeta", "alpha"} {
		cmd := exec.Command("git", "-C", super, "-c", "protocol.file.allow=always",
			"submodule", "add", "-q", resolvedConfigMakeRemote(t, rel), rel)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("submodule add %s: %v: %s", rel, err, out)
		}
	}
	resolvedConfigGit(t, super, "commit", "-qam", "add modules")
	return super
}

// TestNativeResolvedConfigSubmoduleTopology pins the sorted submodule list and
// the gitmodules_present flag against the real Python detection on a real
// superproject: two submodules added in reverse order resolve to the sorted
// list under the default auto source.
func TestNativeResolvedConfigSubmoduleTopology(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := resolvedConfigMakeSuper(t)
	writeResolvedConfigManifest(t, root, "")

	got := assertResolvedConfigParity(t, python, driver, authority, root)
	if got.Topology.Resolved != "monorepo-submodules" || got.Topology.Via != shared.TopologyViaAuto {
		t.Fatalf("topology = %#v, want auto monorepo-submodules", got.Topology)
	}
	if !got.Topology.GitmodulesPresent {
		t.Fatalf("gitmodules_present = false, want true: %#v", got.Topology)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(got.Topology.Submodules, want) {
		t.Fatalf("submodules = %#v, want the sorted %#v", got.Topology.Submodules, want)
	}
}

// TestNativeResolvedConfigGitmodulesPresentUninitialized pins the flag when
// .gitmodules exists but no submodule is initialized: present is true and the
// submodule list is empty, under an explicitly configured monorepo-submodules.
func TestNativeResolvedConfigGitmodulesPresentUninitialized(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitmodules"),
		[]byte("[submodule \"apps/api\"]\n\tpath = apps/api\n\turl = https://example.invalid/api.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeResolvedConfigManifest(t, root, "[project]\nrepo_topology = \"monorepo-submodules\"\n")

	got := assertResolvedConfigParity(t, python, driver, authority, root)
	if !got.Topology.GitmodulesPresent {
		t.Fatalf("gitmodules_present = false, want true: %#v", got.Topology)
	}
	if len(got.Topology.Submodules) != 0 {
		t.Fatalf("submodules = %#v, want the empty set", got.Topology.Submodules)
	}
}

// TestNativeResolvedConfigNonStringBindingCoercion pins the scalar coercion the
// Python authority applies: _python_build_resolved_config keys
// bindings_out[capability] with any truthy value and json.dumps renders the
// object key, so an integer capability 5 keys as "5" and a boolean true as
// "true". The native projection coerces the same way instead of skipping the
// entry. A truthy integer recipe value is kept as well; the native envelope
// contract is {capability: recipe} with string values (the gate bridge's
// _is_str_keyed_mapping(bindings, str) requires it), so the recipe is rendered
// with Python str() where the raw Python fallback would keep the number.
func TestNativeResolvedConfigNonStringBindingCoercion(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)

	t.Run("integer capability keys as json.dumps renders 5", func(t *testing.T) {
		root := t.TempDir()
		writeResolvedConfigManifest(t, root, "[recipes.a]\nenabled = true\n\n[[bindings]]\ncapability = 5\nrecipe = \"a\"\n")
		got := assertResolvedConfigParity(t, python, driver, authority, root)
		if got.Bindings["5"] != "a" {
			t.Fatalf("native bindings = %#v, want the coerced key \"5\"", got.Bindings)
		}
	})

	t.Run("boolean capability keys as json.dumps renders true", func(t *testing.T) {
		root := t.TempDir()
		writeResolvedConfigManifest(t, root, "[recipes.a]\nenabled = true\n\n[[bindings]]\ncapability = true\nrecipe = \"a\"\n")
		got := assertResolvedConfigParity(t, python, driver, authority, root)
		if got.Bindings["true"] != "a" {
			t.Fatalf("native bindings = %#v, want the coerced key \"true\"", got.Bindings)
		}
	})

	t.Run("integer recipe is kept and str-coerced", func(t *testing.T) {
		root := t.TempDir()
		writeResolvedConfigManifest(t, root, "[recipes.a]\nenabled = true\n\n[[bindings]]\ncapability = \"tracker\"\nrecipe = 5\n")
		raw := runBindingsRefRaw(t, python, driver, "project", authority, root)
		var ref resolvedConfigRefResult
		if err := json.Unmarshal(raw, &ref); err != nil {
			t.Fatalf("decode ref %s: %v", raw, err)
		}
		if !ref.OK {
			t.Fatalf("python authority raised: %s: %s", ref.ExcType, ref.Message)
		}
		pyBindings, _ := decodeGeneric(t, ref.Result)["bindings"].(map[string]any)
		if pyBindings["tracker"] != float64(5) {
			t.Fatalf("python bindings = %#v, want the raw integer recipe 5", pyBindings)
		}
		got, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: root})
		if err != nil {
			t.Fatalf("BuildResolvedConfig: %v", err)
		}
		if got.Bindings["tracker"] != "5" {
			t.Fatalf("native bindings = %#v, want the str-coerced recipe \"5\"", got.Bindings)
		}
	})
}

// TestNativeResolvedConfigEmptyRecipeTableParity pins the empty-table shape: a
// [recipes.foo] header with no keys survives as recipes["foo"] = {} in both the
// native projection and the Python authority, exactly as
// _python_build_resolved_config assigns recipes_out[rid] = config
// unconditionally.
func TestNativeResolvedConfigEmptyRecipeTableParity(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)
	root := t.TempDir()
	writeResolvedConfigManifest(t, root, "[recipes.foo]\n")

	got := assertResolvedConfigParity(t, python, driver, authority, root)
	if goFoo, present := got.Recipes["foo"]; !present || len(goFoo) != 0 {
		t.Fatalf("native recipes = %#v, want the empty recipe table kept as foo:{}", got.Recipes)
	}
}

// TestNativeResolvedConfigFlatConfigCollisionIsDeterministic pins the
// flat-versus-[config] collision. TOML requires the flat keys of [recipes.<id>]
// to be declared before the [recipes.<id>.config] header, and Python's document
// walk therefore always applies the nested table last, so the [config] value
// wins. The native projection applies the flat keys first and the config
// sub-table last, reproducing that outcome deterministically (the winner used to
// be decided by Go map iteration order).
func TestNativeResolvedConfigFlatConfigCollisionIsDeterministic(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)
	root := t.TempDir()
	writeResolvedConfigManifest(t, root, "[recipes.a]\nfoo = 1\n\n[recipes.a.config]\nfoo = 2\n")

	got := assertResolvedConfigParity(t, python, driver, authority, root)
	if n, ok := got.Recipes["a"]["foo"].(int64); !ok || n != 2 {
		t.Fatalf("native recipes.a.foo = %#v, want the document-ordered 2", got.Recipes["a"]["foo"])
	}

	const runs = 64
	for i := 0; i < runs; i++ {
		again, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: root})
		if err != nil {
			t.Fatalf("BuildResolvedConfig: %v", err)
		}
		if n, ok := again.Recipes["a"]["foo"].(int64); !ok || n != 2 {
			t.Fatalf("run %d: native recipes.a.foo = %#v, want the deterministic 2", i, again.Recipes["a"]["foo"])
		}
	}
}

// TestNativeResolvedConfigInlineConfigBeforeFlatKeyDivergence pins the one
// document shape the native two-pass resolution cannot reproduce: an inline
// [recipes.<id>] config table declared BEFORE a colliding flat key, e.g.
//
//	[recipes.a]
//	config = { foo = 2 }
//	foo = 1
//
// Python walks the document in order, so the later flat `foo = 1` wins and the
// authority reports foo = 1. The native projection applies the flat keys first
// and the nested config sub-table last (the fix for the header-form collision
// above), so it reports foo = 2. The direction is therefore explicit: Python is
// document-ordered, Go is two-pass deterministic; the two rules agree for the
// `[recipes.<id>.config]` header form and diverge only for this inline form.
// That divergence is the pinned boundary, not parity claimed and not a missed
// branch.
//
// No shipped recipe uses this shape: a scoped grep over catalog/ for a TOML
// inline `config = {` assignment returns nothing, and every real recipe uses
// the `[recipes.<id>.config]` header. Exact parity would require document order
// to survive TOML parsing end to end, but internal/toml represents tables as Go
// maps, so the order is lost at parse time — a structural limit of the reader,
// out of scope for this unit.
func TestNativeResolvedConfigInlineConfigBeforeFlatKeyDivergence(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := resolvedConfigRefPath(t)
	root := t.TempDir()
	writeResolvedConfigManifest(t, root, "[recipes.a]\nconfig = { foo = 2 }\nfoo = 1\n")

	// Python authority: document order, so the later flat foo = 1 wins.
	raw := runBindingsRefRaw(t, python, driver, "project", authority, root)
	var ref resolvedConfigRefResult
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("decode ref %s: %v", raw, err)
	}
	if !ref.OK {
		t.Fatalf("python authority raised: %s: %s", ref.ExcType, ref.Message)
	}
	pyRecipes, _ := decodeGeneric(t, ref.Result)["recipes"].(map[string]any)
	pyRecipe, _ := pyRecipes["a"].(map[string]any)
	if pyRecipe["foo"] != float64(1) {
		t.Fatalf("python recipes.a.foo = %#v, want the document-ordered 1; the mixed inline-config-before-flat-key divergence is deliberate and pinned, update this assertion only if the Python authority changes", pyRecipe["foo"])
	}
	t.Logf("python authority recipes.a.foo = %#v (document-ordered)", pyRecipe["foo"])

	// Native authority: two-pass, the nested config table applied last wins.
	got, err := BuildResolvedConfig(ResolvedConfigRequest{ProjectRoot: root})
	if err != nil {
		t.Fatalf("BuildResolvedConfig: %v", err)
	}
	if n, ok := got.Recipes["a"]["foo"].(int64); !ok || n != 2 {
		t.Fatalf("native recipes.a.foo = %#v, want the two-pass 2; the mixed inline-config-before-flat-key divergence is deliberate and pinned, update this assertion only if the native resolution changes", got.Recipes["a"]["foo"])
	}
	t.Logf("native authority recipes.a.foo = %#v (two-pass)", got.Recipes["a"]["foo"])
}
