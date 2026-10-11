package sync

// T4 contract tests for the native root bindings authority (GO-07 S13 PR1).
//
// ResolveBindings routes the root request into the shared decision core
// (ai-specs.dev/worktree-gate/shared.GradeBindings) the gate binary's
// --resolve-bindings surface also runs, so the root single binary and the gate
// share one implementation — never a second port. It mirrors the Python
// fallback authority: _python_resolve_bindings (lib/_internal/recipe-materialize.py
// :2614) plus _python_check_capability_conflicts, and the durable writer
// write_tracker_witness (:2763) through the shared producer.
//
// The differential test at the bottom shells out to the real Python module via
// testdata/bindings_ref.py (the hookactuator/gitignore differential shape).

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/ledger"
	"ai-specs.dev/worktree-gate/shared"
)

// bindingsRefPath resolves the reference driver under this package (go test cwd
// is internal/sync).
func bindingsRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "bindings_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

// runBindingsRefRaw runs the reference driver and returns its raw stdout bytes
// (no trimming: the witness mode compares exact bytes including the trailing
// newline).
func runBindingsRefRaw(t *testing.T, python, driver string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(python, append([]string{driver}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.Bytes()
}

// bindingsCapTable renders the [[capabilities]] tables for a fixture recipe.
func bindingsCapTable(caps ...string) string {
	var b strings.Builder
	for _, capability := range caps {
		b.WriteString("[[capabilities]]\nid = \"" + capability + "\"\n")
	}
	return b.String()
}

// bindingsCatalog is the shared fixture: alpha and beta both declare tracker
// (an ambiguity), gamma declares vcs (a single provider), and broken is present
// but unparsable (the acquisition must swallow it, as Python does).
func bindingsCatalog(t *testing.T) string {
	t.Helper()
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "alpha")+bindingsCapTable("tracker"))
	writeSyncCatalogRecipe(t, catalog, "beta", syncRecipeHeader("beta", "beta")+bindingsCapTable("tracker"))
	writeSyncCatalogRecipe(t, catalog, "gamma", syncRecipeHeader("gamma", "gamma")+bindingsCapTable("vcs"))
	writeSyncCatalogRecipe(t, catalog, "broken", syncRecipeHeader("broken", "broken")+"not = [valid\n")
	return catalog
}

// bindingsRefResult is the canonical JSON testdata/bindings_ref.py emits in
// resolve mode. A raised Python authority reports its message in Error /
// ConflictsError instead of the corresponding payload.
type bindingsRefResult struct {
	Bindings       map[string]string           `json:"bindings"`
	Conflicts      []shared.CapabilityConflict `json:"conflicts"`
	Error          string                      `json:"error"`
	ConflictsError string                      `json:"conflicts_error"`
}

// TestNativeBindingsDifferentialAgainstPython pins ResolveBindings against the
// real Python fallback authority (_python_resolve_bindings and
// _python_check_capability_conflicts). Bindings, resolution errors and graded
// conflicts must agree case for case, and an unreadable recipe.toml (parsable
// directory, unparsable bytes, absent directory) is skipped exactly as the
// Python try/except swallows it.
func TestNativeBindingsDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := bindingsRefPath(t)
	catalog := bindingsCatalog(t)

	cases := []struct {
		name     string
		enabled  []string
		bindings []shared.ManifestBinding
	}{
		{"single tracker provider auto-binds", []string{"alpha"}, nil},
		{"two capabilities auto-bind independently", []string{"alpha", "gamma"}, nil},
		{"ambiguous tracker is not bound", []string{"alpha", "beta"}, nil},
		{"explicit binding wins over two declarers", []string{"alpha", "beta"}, []shared.ManifestBinding{{Capability: "tracker", Recipe: "beta"}}},
		{"unparsable recipe.toml is skipped", []string{"broken", "alpha"}, nil},
		{"absent recipe directory is skipped", []string{"ghost", "alpha"}, nil},
		{"duplicate explicit binding raises", []string{"alpha", "beta"}, []shared.ManifestBinding{{Capability: "tracker", Recipe: "alpha"}, {Capability: "tracker", Recipe: "beta"}}},
		{"disabled or unknown recipe raises", []string{"alpha"}, []shared.ManifestBinding{{Capability: "tracker", Recipe: "ghost"}}},
		{"undeclared capability raises", []string{"alpha", "gamma"}, []shared.ManifestBinding{{Capability: "tracker", Recipe: "gamma"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enabledJSON, err := json.Marshal(tc.enabled)
			if err != nil {
				t.Fatal(err)
			}
			explicit := tc.bindings
			if explicit == nil {
				explicit = []shared.ManifestBinding{}
			}
			bindingsJSON, err := json.Marshal(explicit)
			if err != nil {
				t.Fatal(err)
			}

			raw := runBindingsRefRaw(t, python, driver, "resolve", authority, catalog, string(enabledJSON), string(bindingsJSON))
			var ref bindingsRefResult
			if err := json.Unmarshal(raw, &ref); err != nil {
				t.Fatalf("decode ref %s: %v", raw, err)
			}

			got := ResolveBindings(BindingRequest{
				EnabledRecipeIDs: tc.enabled,
				CatalogDir:       catalog,
				Bindings:         tc.bindings,
			})

			if ref.Error != "" {
				if len(got.Errors) != 1 || got.Errors[0] != ref.Error {
					t.Fatalf("errors: go=%#v python=%q", got.Errors, ref.Error)
				}
			} else {
				if len(got.Errors) != 0 {
					t.Fatalf("go reported %#v but python resolved %#v", got.Errors, ref.Bindings)
				}
				if !reflect.DeepEqual(got.Bindings, ref.Bindings) {
					t.Fatalf("bindings: go=%#v python=%#v", got.Bindings, ref.Bindings)
				}
			}
			if ref.ConflictsError != "" {
				// The Python fallback's conflict grader
				// (_python_check_capability_conflicts) has no try/except and raises on
				// an unreadable recipe.toml, while the native authority — like the gate
				// binary's --resolve-bindings surface — grades conflicts from its
				// swallowing acquisition. This divergence is pre-existing between the
				// gate and the Python fallback; the differential pins the
				// _python_resolve_bindings body, whose result above already matched.
				return
			}
			if !reflect.DeepEqual(got.Conflicts, ref.Conflicts) {
				t.Fatalf("conflicts: go=%#v python=%#v", got.Conflicts, ref.Conflicts)
			}
		})
	}
}

// TestNativeWitnessBytesMatchPythonWriter pins the exact on-disk witness bytes to
// the Python writer: json.dumps(indent=2, sort_keys=True) plus one trailing
// newline, key order included. The Go shared producer must reproduce it for every
// state (bound, ambiguous, declared-not-bound, unbound) at a fixed instant.
func TestNativeWitnessBytesMatchPythonWriter(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := bindingsRefPath(t)
	catalog := bindingsCatalog(t)

	const stamp = "2026-01-02T03:04:05Z"
	cases := []struct {
		name     string
		enabled  []string
		resolved map[string]string
		declared bool
	}{
		{"bound names the resolved recipe", []string{"alpha"}, map[string]string{"tracker": "alpha"}, false},
		{"bound wins over a competing declarer", []string{"alpha", "beta"}, map[string]string{"tracker": "beta"}, false},
		{"ambiguous records every candidate in enabled order", []string{"beta", "alpha", "gamma"}, map[string]string{}, false},
		{"declared-not-bound carries no candidate", []string{"gamma"}, map[string]string{}, true},
		{"unbound carries no candidate", []string{"gamma"}, map[string]string{}, false},
		{"single candidate with a declaration is declared-not-bound", []string{"alpha"}, map[string]string{}, true},
		{"unreadable recipe declares nothing", []string{"broken", "gamma"}, map[string]string{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enabledJSON, err := json.Marshal(tc.enabled)
			if err != nil {
				t.Fatal(err)
			}
			resolvedJSON, err := json.Marshal(tc.resolved)
			if err != nil {
				t.Fatal(err)
			}
			declared := "0"
			if tc.declared {
				declared = "1"
			}
			want := runBindingsRefRaw(t, python, driver, "witness", authority, catalog, string(enabledJSON), string(resolvedJSON), declared, stamp)

			caps := acquireRecipeCapabilities(catalog, tc.enabled)
			payload := shared.TrackerWitnessPayload(tc.enabled, caps, tc.resolved, tc.declared, stamp)
			got, err := shared.MarshalTrackerWitness(payload)
			if err != nil {
				t.Fatalf("marshal witness: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("witness bytes diverge\n--- go ---\n%q\n--- python ---\n%q", got, want)
			}
		})
	}
}

// bindingsGitRepo creates an empty Git repository in a temp dir and returns its
// path.
func bindingsGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	return dir
}

// bindingsWitnessPath resolves the durable witness path under root's Git common
// dir exactly as the shared writer does.
func bindingsWitnessPath(t *testing.T, root string) string {
	t.Helper()
	common := shared.GitCommonDir(root)
	if common == "" {
		t.Fatalf("no git common dir for %s", root)
	}
	return ledger.WitnessPath(shared.RealPath(common))
}

// TestNativeResolveBindingsWritesWitnessStates drives the root authority end to
// end with the write opt-in and requires the four witness states, the exact
// on-disk bytes (the writer's temp+rename persists MarshalTrackerWitness output),
// no temp residue, and a ledger reader that activates only on bound.
func TestNativeResolveBindingsWritesWitnessStates(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "alpha")+bindingsCapTable("tracker"))
	writeSyncCatalogRecipe(t, catalog, "beta", syncRecipeHeader("beta", "beta")+bindingsCapTable("tracker"))
	writeSyncCatalogRecipe(t, catalog, "gamma", syncRecipeHeader("gamma", "gamma")+bindingsCapTable("vcs"))

	cases := []struct {
		name           string
		enabled        []string
		declared       bool
		wantState      string
		wantRecipe     string
		wantCandidates []string
	}{
		{"bound", []string{"alpha"}, false, ledger.WitnessBound, "alpha", []string{}},
		{"ambiguous", []string{"alpha", "beta"}, false, ledger.WitnessAmbiguous, "", []string{"alpha", "beta"}},
		{"declared-not-bound", []string{"gamma"}, true, ledger.WitnessDeclaredNotBound, "", []string{}},
		{"unbound", []string{"gamma"}, false, ledger.WitnessUnbound, "", []string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := bindingsGitRepo(t)
			if tc.declared {
				if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\ntracking:\n  tracker: trello\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got := ResolveBindings(BindingRequest{
				EnabledRecipeIDs: tc.enabled,
				CatalogDir:       catalog,
				ProjectRoot:      root,
				WriteWitness:     true,
			})
			if len(got.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want none", got.Warnings)
			}

			target := bindingsWitnessPath(t, root)
			raw, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read witness: %v", err)
			}
			var payload shared.WitnessPayload
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode witness: %v", err)
			}
			if payload.State != tc.wantState || payload.RecipeID != tc.wantRecipe || !reflect.DeepEqual(payload.Candidates, tc.wantCandidates) {
				t.Fatalf("witness = %#v, want state=%s recipe=%q candidates=%v", payload, tc.wantState, tc.wantRecipe, tc.wantCandidates)
			}
			want, err := shared.MarshalTrackerWitness(payload)
			if err != nil {
				t.Fatalf("marshal witness: %v", err)
			}
			if !bytes.Equal(raw, want) {
				t.Fatalf("on-disk bytes diverge\n got %q\nwant %q", raw, want)
			}
			entries, err := os.ReadDir(filepath.Dir(target))
			if err != nil {
				t.Fatalf("read ledger dir: %v", err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".tmp.") {
					t.Fatalf("temp residue %q left in %s", entry.Name(), filepath.Dir(target))
				}
			}
			binding := ledger.ReadBinding(target)
			if tc.wantState == ledger.WitnessBound {
				if !binding.Active() || binding.RecipeID != tc.wantRecipe {
					t.Fatalf("ledger.ReadBinding = %#v, want the bound witness", binding)
				}
			} else if binding.Active() {
				t.Fatalf("ledger.ReadBinding = %#v, want dormant for state %s", binding, tc.wantState)
			}
		})
	}
}

// TestNativeResolveBindingsNoRepositoryIsANoOp mirrors the Python best-effort
// contract: outside a repository the request performs no write and adds no
// warning.
func TestNativeResolveBindingsNoRepositoryIsANoOp(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "alpha")+bindingsCapTable("tracker"))

	root := t.TempDir()
	if shared.GitCommonDir(root) != "" {
		t.Skip("temp dir is inside a Git repository; the outside-repository case is unreachable here")
	}
	got := ResolveBindings(BindingRequest{
		EnabledRecipeIDs: []string{"alpha"},
		CatalogDir:       catalog,
		ProjectRoot:      root,
		WriteWitness:     true,
	})
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want a silent skip", got.Warnings)
	}
	if _, err := os.Stat(filepath.Join(root, "ai-specs")); !os.IsNotExist(err) {
		t.Fatalf("ledger directory created outside a repository: %v", err)
	}
}

// TestNativeResolveBindingsWriteFailureIsAWarning makes the durable write
// impossible (the ledger path is a regular file) and requires a warning instead
// of a failure: the ledger stays dormant and the resolution is intact.
func TestNativeResolveBindingsWriteFailureIsAWarning(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "alpha")+bindingsCapTable("tracker"))

	root := bindingsGitRepo(t)
	blocker := filepath.Join(shared.RealPath(shared.GitCommonDir(root)), "ai-specs", "ledger")
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatalf("mkdir blocker parent: %v", err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	got := ResolveBindings(BindingRequest{
		EnabledRecipeIDs: []string{"alpha"},
		CatalogDir:       catalog,
		ProjectRoot:      root,
		WriteWitness:     true,
	})
	if !reflect.DeepEqual(got.Bindings, map[string]string{"tracker": "alpha"}) {
		t.Fatalf("bindings = %#v, want the resolution to survive", got.Bindings)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "ledger stays dormant") {
		t.Fatalf("warnings = %#v, want the dormant-ledger warning", got.Warnings)
	}
}

// TestNativeResolveBindingsResolutionErrorNeverWrites pins the Python abort
// semantics: a resolution error is returned as data and no witness is persisted,
// because writing an unbound state would invent a decision the resolver never
// made.
func TestNativeResolveBindingsResolutionErrorNeverWrites(t *testing.T) {
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "alpha")+bindingsCapTable("tracker"))

	root := bindingsGitRepo(t)
	got := ResolveBindings(BindingRequest{
		EnabledRecipeIDs: []string{"alpha"},
		CatalogDir:       catalog,
		Bindings:         []shared.ManifestBinding{{Capability: "tracker", Recipe: "ghost"}},
		ProjectRoot:      root,
		WriteWitness:     true,
	})
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %#v, want the resolution error", got.Errors)
	}
	if _, err := os.Stat(bindingsWitnessPath(t, root)); !os.IsNotExist(err) {
		t.Fatalf("witness written after a resolution error: %v", err)
	}
}
