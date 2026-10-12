package sync

// T4 differential for the native root merge-config authority (GO-07 S13 PR2b).
//
// PlanMergeConfig acquires the recipe schema and the manifest overrides natively
// and routes them into the shared decision core
// (ai-specs.dev/worktree-gate/shared.PlanMergeConfig) the gate binary's
// --plan-merge-config surface also runs, so the root single binary and the gate
// share one merge decision — never a second port. This file pins the adapter it
// adds against the real Python bodies in lib/_internal/recipe-materialize.py via
// testdata/mergeconfig_ref.py:
//
//   - the request envelope matches _merge_config_request_envelope (:2842),
//     key order and shape order included;
//   - the merged config, warnings and error match _python_merge_config (:3006),
//     the temporary authority the bridge falls back to;
//   - a semantic error is data (error string, no Go error), exactly as the gate
//     envelope reports it, while a non-JSON value (TOML datetime, non-finite
//     float) is a transport failure on both sides.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// mergeConfigRefPath resolves the reference driver under this package (go test
// cwd is internal/sync).
func mergeConfigRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "mergeconfig_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

// runMergeConfigRef runs the driver in one mode (reusing the bindings harness's
// subprocess runner) and returns its raw stdout bytes.
func runMergeConfigRef(t *testing.T, python, driver, mode, authority, catalog, rid, projectRoot string) []byte {
	t.Helper()
	return runBindingsRefRaw(t, python, driver, mode, authority, catalog, rid, projectRoot)
}

// mergeConfigRefResult is the canonical JSON testdata/mergeconfig_ref.py emits
// in plan mode. Error is nil for a successful merge; Config is null when a
// semantic failure raised.
type mergeConfigRefResult struct {
	Config   json.RawMessage `json:"config"`
	Warnings []string        `json:"warnings"`
	Error    *string         `json:"error"`
}

// mergeConfigEnvelopeRef is the canonical JSON emitted in envelope mode.
type mergeConfigEnvelopeRef struct {
	OK      bool   `json:"ok"`
	Request string `json:"request"`
	ExcType string `json:"exc_type"`
	Message string `json:"message"`
}

// mergeConfigFixture writes <catalog>/<rid>/recipe.toml and the project manifest
// at <projectRoot>/ai-specs/ai-specs.toml, returning the resolved paths.
func mergeConfigFixture(t *testing.T, rid, recipeBody, manifestBody string) (catalog, projectRoot, recipePath, manifestPath string) {
	t.Helper()
	catalog = t.TempDir()
	writeSyncCatalogRecipe(t, catalog, rid, recipeBody)
	recipePath = filepath.Join(catalog, rid, "recipe.toml")

	projectRoot = t.TempDir()
	manifestPath = filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifestBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return catalog, projectRoot, recipePath, manifestPath
}

// assertMergeConfigParity requires the Go result to equal the Python authority
// on warnings, the error string, and (when there is no error) the merged config
// including key order and int/float identity.
func assertMergeConfigParity(t *testing.T, goResult shared.MergeConfigResult, ref mergeConfigRefResult) {
	t.Helper()
	if !reflect.DeepEqual(goResult.Warnings, ref.Warnings) {
		t.Fatalf("warnings: go=%#v python=%#v", goResult.Warnings, ref.Warnings)
	}
	wantErr := ""
	if ref.Error != nil {
		wantErr = *ref.Error
	}
	if goResult.Error != wantErr {
		t.Fatalf("error: go=%q python=%q", goResult.Error, wantErr)
	}
	if wantErr != "" {
		return
	}
	pyConfig, err := shared.DecodeOrdered(ref.Config)
	if err != nil {
		t.Fatalf("decode python config %s: %v", ref.Config, err)
	}
	if !orderedEqual(goResult.Config, pyConfig) {
		goRaw, _ := json.Marshal(goResult.Config)
		t.Fatalf("config mismatch:\n go: %s\n py: %s", goRaw, ref.Config)
	}
}

// assertMergeConfigRequestParity requires the Go-built request to equal the real
// Python envelope, order included.
func assertMergeConfigRequestParity(t *testing.T, goReq shared.MergeConfigRequest, ref mergeConfigEnvelopeRef) {
	t.Helper()
	if !ref.OK {
		t.Fatalf("python envelope failed: %s: %s", ref.ExcType, ref.Message)
	}
	goRaw, err := json.Marshal(goReq)
	if err != nil {
		t.Fatalf("marshal go request: %v", err)
	}
	goVal, err := shared.DecodeOrdered(goRaw)
	if err != nil {
		t.Fatalf("decode go request %s: %v", goRaw, err)
	}
	pyVal, err := shared.DecodeOrdered([]byte(ref.Request))
	if err != nil {
		t.Fatalf("decode python envelope %s: %v", ref.Request, err)
	}
	if !orderedEqual(goVal, pyVal) {
		t.Fatalf("request mismatch:\n go: %s\n py: %s", goRaw, ref.Request)
	}
}

// orderedEqual compares two ordered JSON values by key order and json.Number
// literal, so {"a":1.0} never equals {"a":1}.
func orderedEqual(a, b any) bool {
	switch av := a.(type) {
	case *shared.OrderedMap:
		bv, ok := b.(*shared.OrderedMap)
		if !ok {
			return false
		}
		aKeys, bKeys := av.Keys(), bv.Keys()
		if len(aKeys) != len(bKeys) {
			return false
		}
		for i, key := range aKeys {
			if bKeys[i] != key {
				return false
			}
			if !orderedEqual(av.Values()[key], bv.Values()[bKeys[i]]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !orderedEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case json.Number:
		bv, ok := b.(json.Number)
		return ok && av.String() == bv.String()
	default:
		return reflect.DeepEqual(a, b)
	}
}

// assertMergeConfigCase runs one fixture end to end: the Go authority's result
// and request must both match the real Python bodies.
func assertMergeConfigCase(t *testing.T, python, driver, authority, catalog, rid, projectRoot, recipePath, manifestPath string) shared.MergeConfigResult {
	t.Helper()
	goResult, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: rid})
	if err != nil {
		t.Fatalf("PlanMergeConfig: %v", err)
	}

	planRaw := runMergeConfigRef(t, python, driver, "plan", authority, catalog, rid, projectRoot)
	var planRef mergeConfigRefResult
	if err := json.Unmarshal(planRaw, &planRef); err != nil {
		t.Fatalf("decode plan ref %s: %v", planRaw, err)
	}
	assertMergeConfigParity(t, goResult, planRef)

	envRaw := runMergeConfigRef(t, python, driver, "envelope", authority, catalog, rid, projectRoot)
	var envRef mergeConfigEnvelopeRef
	if err := json.Unmarshal(envRaw, &envRef); err != nil {
		t.Fatalf("decode envelope ref %s: %v", envRaw, err)
	}
	goReq, err := acquireMergeConfigRequest(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: rid})
	if err != nil {
		t.Fatalf("acquireMergeConfigRequest: %v", err)
	}
	assertMergeConfigRequestParity(t, goReq, envRef)
	return goResult
}

// mergeRecipeBody declares a deliberate field order (ratio, count, mode, label,
// gate_scope, board_id) plus one structured section. The int/float pair pins the
// 1/1.0 boundary and the structured section pins shape order.
const mergeRecipeBody = `[recipe]
id = "alpha"
name = "Alpha"
description = "d"
version = "1.0.0"

[config.ratio]
required = false
type = "float"
default = 1.0

[config.count]
required = false
type = "integer"
default = 1

[config.mode]
required = false
type = "string"
enum = ["a", "b"]
default = "a"

[config.label]
required = false
type = "string"
default = "café"

[config.gate_scope]
required = false
type = "string"
default = "auto"

[config.board_id]
required = true
type = "string"

[config.reconcile]
scope_field = "x"
max_age_seconds = 5
`

// mergeConfigDivergenceEmptyKey records the one scenario the parity differential
// cannot assert equal: a config key that is the empty string. The shared gate
// core rejects a keyless envelope entry as a malformed request, while
// _python_merge_config tolerates it — it warns and continues for a keyless
// manifest pair and silently keeps a keyless recipe field. Both envelope
// builders carry the empty key verbatim, so this is a pre-existing
// gate-core-versus-Python-fallback divergence, NOT something this delivery
// introduced and NOT a parity claim. The differential excludes the case from
// its equality assertion and pins both observed behaviors instead
// (assertMergeConfigEmptyKeyDivergence), so a future fix must update the test
// deliberately. Follow-up: decide whether the empty key is rejected at the
// bridge envelope boundary or tolerated by the shared core.
const mergeConfigDivergenceEmptyKey = "empty config key: root reports a process failure while Python warns and continues (recorded divergence, follow-up pending)"

// assertMergeConfigEmptyKeyDivergence pins the recorded empty-key divergence in
// place of the parity assertion. The adapter request must still equal the real
// Python envelope (the adapter is faithful: it sends exactly what the Python
// envelope builder sends), and then each authority's own behavior is pinned:
// the shared core returns the process error naming the keyless entry, while
// _python_merge_config continues — warning for a keyless manifest pair, and
// silently returning the keyless field. The side is derived from the request,
// so one helper covers the manifest-side and field-side inversion.
func assertMergeConfigEmptyKeyDivergence(t *testing.T, python, driver, authority, catalog, projectRoot, recipePath, manifestPath string) {
	t.Helper()
	request := MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"}

	goReq, err := acquireMergeConfigRequest(request)
	if err != nil {
		t.Fatalf("acquireMergeConfigRequest: %v", err)
	}
	envRaw := runMergeConfigRef(t, python, driver, "envelope", authority, catalog, "alpha", projectRoot)
	var envRef mergeConfigEnvelopeRef
	if err := json.Unmarshal(envRaw, &envRef); err != nil {
		t.Fatalf("decode envelope ref %s: %v", envRaw, err)
	}
	assertMergeConfigRequestParity(t, goReq, envRef)

	planRaw := runMergeConfigRef(t, python, driver, "plan", authority, catalog, "alpha", projectRoot)
	var planRef mergeConfigRefResult
	if err := json.Unmarshal(planRaw, &planRef); err != nil {
		t.Fatalf("decode plan ref %s: %v", planRaw, err)
	}
	pyConfig, err := shared.DecodeOrdered(planRef.Config)
	if err != nil {
		t.Fatalf("decode python config %s: %v", planRef.Config, err)
	}
	pyMap, ok := pyConfig.(*shared.OrderedMap)
	if !ok {
		t.Fatalf("python config is %T, want an object: %s", pyConfig, planRef.Config)
	}

	side, wantNative := "field", "field entry missing key"
	for _, pair := range goReq.Manifest {
		if pair.Key == "" {
			side, wantNative = "manifest", "manifest entry missing key"
		}
	}

	if side == "manifest" {
		wantWarnings := []string{"recipe 'Alpha': unknown config key '' in manifest (ignored)"}
		if !reflect.DeepEqual(planRef.Warnings, wantWarnings) {
			t.Fatalf("python warnings = %#v, want %#v", planRef.Warnings, wantWarnings)
		}
		if planRef.Error != nil {
			t.Fatalf("python error = %q, want the warning-only continuation", *planRef.Error)
		}
		if _, present := pyMap.Get(""); present {
			t.Fatalf("python config kept the ignored empty manifest key: %s", planRef.Config)
		}
	} else {
		if len(planRef.Warnings) != 0 || planRef.Error != nil {
			t.Fatalf("python warnings=%#v error=%v, want a clean continuation", planRef.Warnings, planRef.Error)
		}
		if got, present := pyMap.Get(""); !present {
			t.Fatalf("python config dropped the empty field: %s", planRef.Config)
		} else if got != "empty-key-default" {
			t.Fatalf("python config[\"\"] = %#v, want the declared default", got)
		}
	}

	if _, err := PlanMergeConfig(request); err == nil || err.Error() != wantNative {
		t.Fatalf("native error = %v, want the shared core's %q process failure", err, wantNative)
	}
}

// TestNativeMergeConfigDifferentialAgainstPython pins the adapter and the shared
// core against the real Python envelope builder and the temporary Python
// authority for the ordering, float, semantic-error and gate_scope boundaries.
// The two empty-key cases at the end are explicitly excluded from the equality
// assertion and pin the recorded divergence (mergeConfigDivergenceEmptyKey)
// instead of claiming parity.
func TestNativeMergeConfigDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := mergeConfigRefPath(t)

	cases := []struct {
		name       string
		manifest   string
		body       string // empty means mergeRecipeBody
		divergence string // non-empty excludes the case from the parity assertion
	}{
		{
			"defaults in field order with a manifest override",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\n",
			"", "",
		},
		{
			"override keeps position, unknown warns, table appends",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nmode = \"b\"\nmystery = 1\nreconcile = { scope_field = \"y\", max_age_seconds = 9 }\n",
			"", "",
		},
		{
			"manifest int stays int and float stays float",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nratio = 2\ncount = 2.0\n",
			"", "",
		},
		{
			"blank gate_scope resolves to auto",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\ngate_scope = \"   \"\n",
			"", "",
		},
		{
			"enum violation is a semantic error",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nmode = \"z\"\n",
			"", "",
		},
		{
			"missing required field is a semantic error",
			"[recipes.alpha]\nenabled = true\n",
			"", "",
		},
		{
			"nested structured shape order drives the error",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nreconcile = { expectations = [{ property = 1, event = 2 }] }\n",
			"", "",
		},
		{
			"structured scalar type error after unknown-key check",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nreconcile = { max_age_seconds = 1.5 }\n",
			"", "",
		},
		{
			"recorded divergence: empty manifest key warns in Python but fails the core",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\n\"\" = \"x\"\nboard_id = \"b1\"\n",
			mergeRecipeBody,
			mergeConfigDivergenceEmptyKey,
		},
		{
			"recorded divergence: empty recipe field key continues in Python but fails the core",
			"[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\n",
			mergeRecipeBody + "\n[config.\"\"]\nrequired = false\ntype = \"string\"\ndefault = \"empty-key-default\"\n",
			mergeConfigDivergenceEmptyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if body == "" {
				body = mergeRecipeBody
			}
			catalog, projectRoot, recipePath, manifestPath := mergeConfigFixture(t, "alpha", body, tc.manifest)
			if tc.divergence != "" {
				assertMergeConfigEmptyKeyDivergence(t, python, driver, authority, catalog, projectRoot, recipePath, manifestPath)
				return
			}
			assertMergeConfigCase(t, python, driver, authority, catalog, "alpha", projectRoot, recipePath, manifestPath)
		})
	}
}

// TestNativeMergeConfigFieldOrderIsRecipeDeclarationOrder proves the map-order
// blocker is solved by re-parsing: reversing the declaration order reverses the
// Go result and the Python authority in lockstep, and the request envelope
// matches Python's preserved order.
func TestNativeMergeConfigFieldOrderIsRecipeDeclarationOrder(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := mergeConfigRefPath(t)

	body := `[recipe]
id = "alpha"
name = "Alpha"
description = "d"
version = "1.0.0"

[config.zeta]
required = false
type = "string"
default = "z"

[config.alpha]
required = false
type = "string"
default = "a"

[config.mu]
required = false
type = "string"
default = "m"
`
	manifest := "[recipes.alpha]\nenabled = true\n"
	catalog, projectRoot, recipePath, manifestPath := mergeConfigFixture(t, "alpha", body, manifest)
	got := assertMergeConfigCase(t, python, driver, authority, catalog, "alpha", projectRoot, recipePath, manifestPath)

	want := []string{"zeta", "alpha", "mu"}
	if !reflect.DeepEqual(got.Config.Keys(), want) {
		t.Fatalf("config order = %v, want %v", got.Config.Keys(), want)
	}
}

// TestNativeMergeConfigNonFiniteFloatIsRejected pins the allow_nan=False
// boundary: Python's envelope serialization raises ValueError, and the native
// adapter rejects the same value instead of emitting a bare NaN token that
// encoding/json would otherwise refuse (or worse, emit differently).
func TestNativeMergeConfigNonFiniteFloatIsRejected(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := mergeConfigRefPath(t)

	for _, literal := range []string{"nan", "inf", "-inf"} {
		t.Run(literal, func(t *testing.T) {
			manifest := "[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nboard_id = \"b1\"\nratio = " + literal + "\n"
			catalog, projectRoot, recipePath, manifestPath := mergeConfigFixture(t, "alpha", mergeRecipeBody, manifest)

			envRaw := runMergeConfigRef(t, python, driver, "envelope", authority, catalog, "alpha", projectRoot)
			var envRef mergeConfigEnvelopeRef
			if err := json.Unmarshal(envRaw, &envRef); err != nil {
				t.Fatalf("decode envelope ref %s: %v", envRaw, err)
			}
			if envRef.OK || envRef.ExcType != "ValueError" {
				t.Fatalf("python envelope = %+v, want a ValueError for %s", envRef, literal)
			}
			// The two non-finite texts are NOT byte-comparable, so this pins
			// BOTH live texts instead of matching one against a hardcoded copy.
			// Python's is the live json.dumps(..., allow_nan=False) transport
			// failure, which never crosses the wire (the envelope cannot be
			// serialized, so no process runs) and CPython suffixes the offending
			// value; the native text is the adapter's own acquisition-time
			// rejection, which wraps Python's prefix in the manifest-key
			// context and has no value suffix.
			if want := "Out of range float values are not JSON compliant: " + literal; envRef.Message != want {
				t.Fatalf("python message = %q, want the live ValueError text %q", envRef.Message, want)
			}

			_, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
			if err == nil {
				t.Fatalf("PlanMergeConfig accepted %s", literal)
			}
			const nativeText = `manifest "ratio" value: Out of range float values are not JSON compliant`
			if err.Error() != nativeText {
				t.Fatalf("native error = %q, want the live text %q", err.Error(), nativeText)
			}
			if err.Error() == envRef.Message {
				t.Fatalf("the non-finite texts became byte-equal (%q); this divergence is pinned deliberately, update the assertion", envRef.Message)
			}
		})
	}
}

// TestNativeMergeConfigDatetimeIsABoundary pins the datetime boundary the Go
// toml reader makes explicit: tomllib carries a datetime into the envelope and
// json.dumps(allow_nan=False) raises TypeError (the bridge falls back), while
// internal/toml rejects the value at acquisition so the native authority never
// silently learns a datetime's text.
func TestNativeMergeConfigDatetimeIsABoundary(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := mergeConfigRefPath(t)

	for _, literal := range []string{"2024-01-02T03:04:05", "2024-01-02"} {
		t.Run(literal, func(t *testing.T) {
			manifest := "[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nstarts = " + literal + "\n"
			catalog, projectRoot, recipePath, manifestPath := mergeConfigFixture(t, "alpha", mergeRecipeBody, manifest)

			envRaw := runMergeConfigRef(t, python, driver, "envelope", authority, catalog, "alpha", projectRoot)
			var envRef mergeConfigEnvelopeRef
			if err := json.Unmarshal(envRaw, &envRef); err != nil {
				t.Fatalf("decode envelope ref %s: %v", envRaw, err)
			}
			if envRef.OK || envRef.ExcType != "TypeError" {
				t.Fatalf("python envelope = %+v, want a TypeError for %s", envRef, literal)
			}

			if _, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"}); err == nil {
				t.Fatalf("PlanMergeConfig accepted the datetime %s", literal)
			}
		})
	}
}
