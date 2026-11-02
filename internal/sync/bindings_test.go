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
// The differential tests at the bottom compare the native authority against
// testdata/bindings_golden.json, the frozen golden bundle captured once from the
// live Python oracle (PR2-B S5); no test execs Python anymore.

import (
	"bytes"
	"encoding/base64"
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

// bindingsResolveCase is one resolve-mode differential scenario: the enabled
// recipe ids and the explicit manifest bindings handed to both legs.
type bindingsResolveCase struct {
	name     string
	enabled  []string
	bindings []shared.ManifestBinding
}

// bindingsResolveCases is the shared resolve fixture (the live differential and
// the PR2-B S5 capture test both consume it, so the inputs stay byte-identical).
func bindingsResolveCases() []bindingsResolveCase {
	return []bindingsResolveCase{
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
}

// bindingsWitnessCase is one witness-mode differential scenario: the resolved
// bindings and declaration flag fed to the writer at a fixed instant.
type bindingsWitnessCase struct {
	name     string
	enabled  []string
	resolved map[string]string
	declared bool
}

// bindingsWitnessCases is the shared witness fixture (see bindingsResolveCases).
func bindingsWitnessCases() []bindingsWitnessCase {
	return []bindingsWitnessCase{
		{"bound names the resolved recipe", []string{"alpha"}, map[string]string{"tracker": "alpha"}, false},
		{"bound wins over a competing declarer", []string{"alpha", "beta"}, map[string]string{"tracker": "beta"}, false},
		{"ambiguous records every candidate in enabled order", []string{"beta", "alpha", "gamma"}, map[string]string{}, false},
		{"declared-not-bound carries no candidate", []string{"gamma"}, map[string]string{}, true},
		{"unbound carries no candidate", []string{"gamma"}, map[string]string{}, false},
		{"single candidate with a declaration is declared-not-bound", []string{"alpha"}, map[string]string{}, true},
		{"unreadable recipe declares nothing", []string{"broken", "gamma"}, map[string]string{}, false},
	}
}

// bindingsWitnessGoldenCase is one frozen record of the witness half of
// testdata/bindings_golden.json: the raw writer bytes plus their sha256.
type bindingsWitnessGoldenCase struct {
	BytesB64 string `json:"bytes_b64"`
	SHA256   string `json:"sha256"`
}

// bindingsResolveGoldenNames is the hard-coded identity inventory of the frozen
// resolve half: exact case names in capture order (9 = 6 resolved + 3 errors).
// Kept in the test, never derived from the bundle.
var bindingsResolveGoldenNames = []string{
	"single tracker provider auto-binds",
	"two capabilities auto-bind independently",
	"ambiguous tracker is not bound",
	"explicit binding wins over two declarers",
	"unparsable recipe.toml is skipped",
	"absent recipe directory is skipped",
	"duplicate explicit binding raises",
	"disabled or unknown recipe raises",
	"undeclared capability raises",
}

// bindingsWitnessGoldenNames is the hard-coded identity inventory of the frozen
// witness half (7 records, capture order).
var bindingsWitnessGoldenNames = []string{
	"bound names the resolved recipe",
	"bound wins over a competing declarer",
	"ambiguous records every candidate in enabled order",
	"declared-not-bound carries no candidate",
	"unbound carries no candidate",
	"single candidate with a declaration is declared-not-bound",
	"unreadable recipe declares nothing",
}

// loadBindingsGolden validates and indexes testdata/bindings_golden.json. The
// identity inventories are asserted before any case loop runs.
func loadBindingsGolden(t *testing.T) (map[string]bindingsRefResult, map[string]bindingsWitnessGoldenCase) {
	t.Helper()
	var bundle struct {
		Resolve struct {
			Names []string            `json:"names"`
			Cases []bindingsRefResult `json:"cases"`
		} `json:"resolve"`
		Witness struct {
			Names []string                    `json:"names"`
			Cases []bindingsWitnessGoldenCase `json:"cases"`
		} `json:"witness"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "bindings_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}

	validateGoldenIdentity(t, "bindings resolve", bundle.Resolve.Names, bindingsResolveGoldenNames)
	if len(bundle.Resolve.Cases) != len(bundle.Resolve.Names) {
		t.Fatalf("bindings resolve: %d records for %d names", len(bundle.Resolve.Cases), len(bundle.Resolve.Names))
	}
	resolve := make(map[string]bindingsRefResult, len(bundle.Resolve.Cases))
	errors := 0
	for i, rec := range bundle.Resolve.Cases {
		if rec.Error != "" {
			errors++
		}
		resolve[bundle.Resolve.Names[i]] = rec
	}
	if errors != 3 {
		t.Fatalf("bindings resolve bundle has %d error records, want 3", errors)
	}

	validateGoldenIdentity(t, "bindings witness", bundle.Witness.Names, bindingsWitnessGoldenNames)
	if len(bundle.Witness.Cases) != len(bundle.Witness.Names) {
		t.Fatalf("bindings witness: %d records for %d names", len(bundle.Witness.Cases), len(bundle.Witness.Names))
	}
	witness := make(map[string]bindingsWitnessGoldenCase, len(bundle.Witness.Cases))
	for i, rec := range bundle.Witness.Cases {
		if rec.BytesB64 == "" || rec.SHA256 == "" {
			t.Fatalf("frozen witness record %q has no bytes", bundle.Witness.Names[i])
		}
		witness[bundle.Witness.Names[i]] = rec
	}
	return resolve, witness
}

// TestNativeBindingsDifferentialAgainstFrozenOracle pins ResolveBindings against
// the frozen record of the real Python fallback authority
// (_python_resolve_bindings and _python_check_capability_conflicts, captured in
// testdata/bindings_golden.json). Bindings, resolution errors and graded
// conflicts must agree case for case, and an unreadable recipe.toml (parsable
// directory, unparsable bytes, absent directory) is skipped exactly as the
// Python try/except swallows it.
func TestNativeBindingsDifferentialAgainstFrozenOracle(t *testing.T) {
	golden, _ := loadBindingsGolden(t)
	catalog := bindingsCatalog(t)

	for _, tc := range bindingsResolveCases() {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := golden[tc.name]
			if !ok {
				t.Fatalf("frozen record %q missing", tc.name)
			}

			got := ResolveBindings(BindingRequest{
				EnabledRecipeIDs: tc.enabled,
				CatalogDir:       catalog,
				Bindings:         tc.bindings,
			})

			if ref.Error != "" {
				if len(got.Errors) != 1 || got.Errors[0] != ref.Error {
					t.Fatalf("errors: go=%#v frozen=%q", got.Errors, ref.Error)
				}
			} else {
				if len(got.Errors) != 0 {
					t.Fatalf("go reported %#v but the frozen authority resolved %#v", got.Errors, ref.Bindings)
				}
				if !reflect.DeepEqual(got.Bindings, ref.Bindings) {
					t.Fatalf("bindings: go=%#v frozen=%#v", got.Bindings, ref.Bindings)
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
				t.Fatalf("conflicts: go=%#v frozen=%#v", got.Conflicts, ref.Conflicts)
			}
		})
	}
}

// TestNativeWitnessBytesMatchFrozenWriter pins the exact on-disk witness bytes
// to the frozen record of the Python writer: json.dumps(indent=2,
// sort_keys=True) plus one trailing newline, key order included. The Go shared
// producer must reproduce it for every state (bound, ambiguous,
// declared-not-bound, unbound) at a fixed instant.
func TestNativeWitnessBytesMatchFrozenWriter(t *testing.T) {
	_, witness := loadBindingsGolden(t)
	catalog := bindingsCatalog(t)

	const stamp = "2026-01-02T03:04:05Z"

	for _, tc := range bindingsWitnessCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := witness[tc.name]
			if !ok {
				t.Fatalf("frozen record %q missing", tc.name)
			}
			want, err := base64.StdEncoding.DecodeString(rec.BytesB64)
			if err != nil {
				t.Fatalf("decode frozen witness bytes: %v", err)
			}
			if gotSHA := sha256Hex(want); gotSHA != rec.SHA256 {
				t.Fatalf("frozen witness sha256 = %s, recorded %s", gotSHA, rec.SHA256)
			}

			caps := acquireRecipeCapabilities(catalog, tc.enabled)
			payload := shared.TrackerWitnessPayload(tc.enabled, caps, tc.resolved, tc.declared, stamp)
			got, err := shared.MarshalTrackerWitness(payload)
			if err != nil {
				t.Fatalf("marshal witness: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("witness bytes diverge\n--- go ---\n%q\n--- frozen ---\n%q", got, want)
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
