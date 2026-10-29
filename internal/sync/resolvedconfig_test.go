package sync

// T4 differential for the native root resolved-config authority (GO-07 S13 PR3),
// FROZEN (PR2-B S2): the Python leg is no longer executed. Its behavior is
// captured once in testdata/resolvedconfig_golden.json — the exact project-mode
// envelopes of testdata/resolvedconfig_ref.py (which drives the real fail-open
// body _python_build_resolved_config in lib/_internal/recipe-materialize.py)
// over the unchanged case matrix — and BuildResolvedConfig is required to equal
// them. Bindings, per-recipe config, enabled order, project root and the
// resolved topology (including the source label and the sorted submodule list)
// must agree case for case, and the acquisition errors must agree on failure.
//
// Host-path inventory: the resolved project root is the only host-derived byte
// sequence in any record. On success it appears exactly once (the envelope's
// project_root); the missing-manifest FileNotFoundError message also quotes it.
// It is tokenized as <PROJECT_ROOT> in the bundle and expanded with the shared
// RealPath rule (Python Path.resolve() semantics) on replay; every other byte
// is compared verbatim (no global normalizer). The Go-side git fixtures for the
// submodule cases stay live. A missing or mutated bundle is a failure, never a
// skip. The .py driver file itself stays on disk until T5 PR4b.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// resolvedConfigGoldenCase is one frozen record: the exact fixture inputs that
// produced it, the inventory of host-path occurrences, and the oracle's
// project-mode envelope with <PROJECT_ROOT> tokens.
type resolvedConfigGoldenCase struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	WriteManifest   bool   `json:"write_manifest"`
	Manifest        string `json:"manifest"`
	Gitmodules      string `json:"gitmodules,omitempty"`
	BuildSubmodules bool   `json:"build_submodules"`
	PathCount       int    `json:"path_count"`
	Python          string `json:"python"`
}

type resolvedConfigGolden struct {
	Comment string                     `json:"comment"`
	Cases   []resolvedConfigGoldenCase `json:"cases"`
}

// loadResolvedConfigGolden reads the frozen authority bundle. A missing bundle
// is a failure, never a skip: the bundle IS the oracle.
func loadResolvedConfigGolden(t *testing.T) []resolvedConfigGoldenCase {
	t.Helper()
	path := filepath.Join("testdata", "resolvedconfig_golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen resolved-config oracle: %v", err)
	}
	var golden resolvedConfigGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatalf("parse frozen resolved-config oracle %s: %v", path, err)
	}
	if len(golden.Cases) == 0 {
		t.Fatalf("frozen resolved-config oracle %s carries no cases", path)
	}
	return golden.Cases
}

// findResolvedConfigCase returns the named frozen record, failing loudly when
// the bundle no longer carries it.
func findResolvedConfigCase(t *testing.T, name string) resolvedConfigGoldenCase {
	t.Helper()
	for _, tc := range loadResolvedConfigGolden(t) {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("frozen resolved-config oracle is missing case %q", name)
	return resolvedConfigGoldenCase{}
}

// resolvedConfigRefResult is the canonical JSON the oracle emits in project
// mode. Result is the Python envelope; ExcType/Message carry a raised failure.
type resolvedConfigRefResult struct {
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
	ExcType string          `json:"exc_type"`
	Message string          `json:"message"`
}

// expandResolvedConfigRoot restores the single tokenized host path with the
// fixture's real root. The recorded occurrence inventory must hold exactly
// (no token may appear where none was captured and none may be missing).
func expandResolvedConfigRoot(t *testing.T, tc resolvedConfigGoldenCase, root string) []byte {
	t.Helper()
	if got := strings.Count(tc.Python, "<PROJECT_ROOT>"); got != tc.PathCount {
		t.Fatalf("case %q: %d <PROJECT_ROOT> tokens in the frozen record, inventory says %d", tc.Name, got, tc.PathCount)
	}
	if tc.PathCount == 0 {
		return []byte(tc.Python)
	}
	return []byte(strings.ReplaceAll(tc.Python, "<PROJECT_ROOT>", shared.RealPath(root)))
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

// buildResolvedConfigFrozenFixture materializes the frozen record's inputs on
// disk (an empty root, .gitmodules and/or the manifest, and — for the submodule
// case — a real superproject with two submodules added in reverse order).
func buildResolvedConfigFrozenFixture(t *testing.T, tc resolvedConfigGoldenCase) string {
	t.Helper()
	root := t.TempDir()
	if tc.BuildSubmodules {
		root = resolvedConfigMakeSuper(t)
	}
	if tc.Gitmodules != "" {
		if err := os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(tc.Gitmodules), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if tc.WriteManifest {
		writeResolvedConfigManifest(t, root, tc.Manifest)
	}
	return root
}

// assertResolvedConfigFrozenParity runs the Go authority on the same root
// (read-only) and requires equality with the frozen Python envelope. When the
// frozen authority raised, the Go authority must return a Go error.
func assertResolvedConfigFrozenParity(t *testing.T, tc resolvedConfigGoldenCase, root string) shared.ResolvedConfig {
	t.Helper()
	raw := expandResolvedConfigRoot(t, tc, root)
	var ref resolvedConfigRefResult
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("decode frozen ref for %q: %v", tc.Name, err)
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
// and the shared projection against the FROZEN _python_build_resolved_config
// for the manifest shapes the Python authority distinguishes: flat and
// config-style recipes, explicit bindings, and each topology source/label
// branch.
func TestNativeResolvedConfigDifferentialAgainstPython(t *testing.T) {
	for _, tc := range loadResolvedConfigGolden(t) {
		if tc.Kind != "differential" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			root := buildResolvedConfigFrozenFixture(t, tc)
			assertResolvedConfigFrozenParity(t, tc, root)
		})
	}
}

// TestNativeResolvedConfigAcquisitionErrors pins the transport failures: a
// missing or unparsable manifest raised in the Python authority (frozen, the
// missing-manifest message quoting the path) and is a Go error natively.
func TestNativeResolvedConfigAcquisitionErrors(t *testing.T) {
	t.Run("missing manifest", func(t *testing.T) {
		tc := findResolvedConfigCase(t, "missing manifest")
		root := buildResolvedConfigFrozenFixture(t, tc)
		assertResolvedConfigFrozenParity(t, tc, root)
	})
	t.Run("unparsable manifest", func(t *testing.T) {
		tc := findResolvedConfigCase(t, "unparsable manifest")
		root := buildResolvedConfigFrozenFixture(t, tc)
		assertResolvedConfigFrozenParity(t, tc, root)
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
// the gitmodules_present flag against the frozen Python detection on a live
// real superproject: two submodules added in reverse order resolve to the
// sorted list under the default auto source.
func TestNativeResolvedConfigSubmoduleTopology(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	tc := findResolvedConfigCase(t, "submodule topology sorted under auto")
	root := buildResolvedConfigFrozenFixture(t, tc)

	got := assertResolvedConfigFrozenParity(t, tc, root)
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
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	tc := findResolvedConfigCase(t, "gitmodules present uninitialized")
	root := buildResolvedConfigFrozenFixture(t, tc)

	got := assertResolvedConfigFrozenParity(t, tc, root)
	if !got.Topology.GitmodulesPresent {
		t.Fatalf("gitmodules_present = false, want true: %#v", got.Topology)
	}
	if len(got.Topology.Submodules) != 0 {
		t.Fatalf("submodules = %#v, want the empty set", got.Topology.Submodules)
	}
}

// TestNativeResolvedConfigNonStringBindingCoercion pins the scalar coercion the
// Python authority applies (frozen): _python_build_resolved_config keys
// bindings_out[capability] with any truthy value and json.dumps renders the
// object key, so an integer capability 5 keys as "5" and a boolean true as
// "true". The native projection coerces the same way instead of skipping the
// entry. A truthy integer recipe value is kept as well; the native envelope
// contract is {capability: recipe} with string values (the gate bridge's
// _is_str_keyed_mapping(bindings, str) requires it), so the recipe is rendered
// with Python str() where the raw Python fallback would keep the number.
func TestNativeResolvedConfigNonStringBindingCoercion(t *testing.T) {
	t.Run("integer capability keys as json.dumps renders 5", func(t *testing.T) {
		tc := findResolvedConfigCase(t, "integer capability keys as json.dumps renders 5")
		root := buildResolvedConfigFrozenFixture(t, tc)
		got := assertResolvedConfigFrozenParity(t, tc, root)
		if got.Bindings["5"] != "a" {
			t.Fatalf("native bindings = %#v, want the coerced key \"5\"", got.Bindings)
		}
	})

	t.Run("boolean capability keys as json.dumps renders true", func(t *testing.T) {
		tc := findResolvedConfigCase(t, "boolean capability keys as json.dumps renders true")
		root := buildResolvedConfigFrozenFixture(t, tc)
		got := assertResolvedConfigFrozenParity(t, tc, root)
		if got.Bindings["true"] != "a" {
			t.Fatalf("native bindings = %#v, want the coerced key \"true\"", got.Bindings)
		}
	})

	t.Run("integer recipe is kept and str-coerced", func(t *testing.T) {
		tc := findResolvedConfigCase(t, "integer recipe is kept and str-coerced")
		root := buildResolvedConfigFrozenFixture(t, tc)
		raw := expandResolvedConfigRoot(t, tc, root)
		var ref resolvedConfigRefResult
		if err := json.Unmarshal(raw, &ref); err != nil {
			t.Fatalf("decode frozen ref: %v", err)
		}
		if !ref.OK {
			t.Fatalf("frozen python authority raised: %s: %s", ref.ExcType, ref.Message)
		}
		pyBindings, _ := decodeGeneric(t, ref.Result)["bindings"].(map[string]any)
		if pyBindings["tracker"] != float64(5) {
			t.Fatalf("frozen python bindings = %#v, want the recorded raw integer recipe 5", pyBindings)
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
// native projection and the frozen Python authority, exactly as
// _python_build_resolved_config assigns recipes_out[rid] = config
// unconditionally.
func TestNativeResolvedConfigEmptyRecipeTableParity(t *testing.T) {
	tc := findResolvedConfigCase(t, "empty recipe table parity")
	root := buildResolvedConfigFrozenFixture(t, tc)

	got := assertResolvedConfigFrozenParity(t, tc, root)
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
	tc := findResolvedConfigCase(t, "flat config collision is deterministic")
	root := buildResolvedConfigFrozenFixture(t, tc)

	got := assertResolvedConfigFrozenParity(t, tc, root)
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
	tc := findResolvedConfigCase(t, "inline config before flat key divergence")
	root := buildResolvedConfigFrozenFixture(t, tc)

	// Frozen Python authority: document order, so the later flat foo = 1 wins.
	raw := expandResolvedConfigRoot(t, tc, root)
	var ref resolvedConfigRefResult
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("decode frozen ref: %v", err)
	}
	if !ref.OK {
		t.Fatalf("frozen python authority raised: %s: %s", ref.ExcType, ref.Message)
	}
	pyRecipes, _ := decodeGeneric(t, ref.Result)["recipes"].(map[string]any)
	pyRecipe, _ := pyRecipes["a"].(map[string]any)
	if pyRecipe["foo"] != float64(1) {
		t.Fatalf("frozen python recipes.a.foo = %#v, want the recorded document-ordered 1; the mixed inline-config-before-flat-key divergence is deliberate and pinned, update this assertion only if the Python authority changes", pyRecipe["foo"])
	}
	t.Logf("python authority recipes.a.foo = %#v (document-ordered, frozen)", pyRecipe["foo"])

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
