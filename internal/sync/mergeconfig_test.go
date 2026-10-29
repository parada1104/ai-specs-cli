package sync

// T4 differential for the native root merge-config authority (GO-07 S13 PR2b),
// FROZEN (PR2-B S2): the Python leg is no longer executed. Its behavior is
// captured once in testdata/mergeconfig_golden.json — the exact plan and
// envelope outputs of testdata/mergeconfig_ref.py (which drives the real bodies
// in lib/_internal/recipe-materialize.py) over the unchanged case matrix — and
// the Go authority is required to equal it:
//
//   - the request envelope matches _merge_config_request_envelope (:2842),
//     key order and shape order included;
//   - the merged config, warnings and error match _python_merge_config (:3006),
//     the temporary authority the bridge falls back to;
//   - a semantic error is data (error string, no Go error), exactly as the gate
//     envelope reports it, while a non-JSON value (TOML datetime, non-finite
//     float) is a transport failure on both sides.
//
// The oracle outputs are path-free (no host path crosses this surface), so the
// bundle needs no tokenization. A missing or mutated bundle is a failure, never
// a skip. The .py driver file itself stays on disk until T5 PR4b (no Python is
// retired in this feature).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// mergeConfigGoldenCase is one frozen record: the exact fixture inputs that
// produced it and the canonical JSON the Python oracle emitted (plan mode
// and/or envelope mode, exactly what the live differential ran).
type mergeConfigGoldenCase struct {
	Name     string          `json:"name"`
	Kind     string          `json:"kind"` // differential | order | nonfinite | datetime
	Literal  string          `json:"literal,omitempty"`
	TypeName string          `json:"type_name,omitempty"`
	Recipe   string          `json:"recipe"`
	Manifest string          `json:"manifest"`
	Plan     json.RawMessage `json:"plan"`
	Envelope json.RawMessage `json:"envelope"`
}

type mergeConfigGolden struct {
	Comment string                  `json:"comment"`
	Cases   []mergeConfigGoldenCase `json:"cases"`
}

// loadMergeConfigGolden reads the frozen authority bundle. A missing bundle is
// a failure, never a skip: the bundle IS the oracle.
func loadMergeConfigGolden(t *testing.T) []mergeConfigGoldenCase {
	t.Helper()
	path := filepath.Join("testdata", "mergeconfig_golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen merge-config oracle: %v", err)
	}
	var golden mergeConfigGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatalf("parse frozen merge-config oracle %s: %v", path, err)
	}
	if len(golden.Cases) == 0 {
		t.Fatalf("frozen merge-config oracle %s carries no cases", path)
	}
	return golden.Cases
}

// mergeConfigRefResult is the canonical JSON the oracle emits in plan mode.
// Error is nil for a successful merge; Config is null when a semantic failure
// raised.
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

// assertMergeConfigParity requires the Go result to equal the frozen Python
// authority on warnings, the error string, and (when there is no error) the
// merged config including key order and int/float identity.
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

// assertMergeConfigRequestParity requires the Go-built request to equal the
// frozen Python envelope, order included.
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

// assertMergeConfigFrozenCase builds the fixture from the frozen inputs and
// requires the Go authority's result and request to both match the frozen
// Python records.
func assertMergeConfigFrozenCase(t *testing.T, tc mergeConfigGoldenCase) shared.MergeConfigResult {
	t.Helper()
	_, _, recipePath, manifestPath := mergeConfigFixture(t, "alpha", tc.Recipe, tc.Manifest)

	goResult, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
	if err != nil {
		t.Fatalf("PlanMergeConfig: %v", err)
	}

	var planRef mergeConfigRefResult
	if err := json.Unmarshal(tc.Plan, &planRef); err != nil {
		t.Fatalf("decode frozen plan record for %q: %v", tc.Name, err)
	}
	assertMergeConfigParity(t, goResult, planRef)

	var envRef mergeConfigEnvelopeRef
	if err := json.Unmarshal(tc.Envelope, &envRef); err != nil {
		t.Fatalf("decode frozen envelope record for %q: %v", tc.Name, err)
	}
	goReq, err := acquireMergeConfigRequest(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
	if err != nil {
		t.Fatalf("acquireMergeConfigRequest: %v", err)
	}
	assertMergeConfigRequestParity(t, goReq, envRef)
	return goResult
}

// TestNativeMergeConfigDifferentialAgainstPython pins the adapter and the shared
// core against the FROZEN Python envelope builder and temporary Python
// authority for the ordering, float, semantic-error and gate_scope boundaries,
// including the empty-key cases the core used to reject as malformed: an empty
// manifest key is an ordinary unknown key (Python warns and ignores it) and an
// empty recipe field key is an ordinary field (Python keeps its default).
func TestNativeMergeConfigDifferentialAgainstPython(t *testing.T) {
	for _, tc := range loadMergeConfigGolden(t) {
		if tc.Kind != "differential" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			assertMergeConfigFrozenCase(t, tc)
		})
	}
}

// TestNativeMergeConfigFieldOrderIsRecipeDeclarationOrder proves the map-order
// blocker is solved by re-parsing: reversing the declaration order reverses the
// Go result exactly as the frozen Python authority recorded, and the request
// envelope matches Python's preserved order.
func TestNativeMergeConfigFieldOrderIsRecipeDeclarationOrder(t *testing.T) {
	cases := loadMergeConfigGolden(t)
	var orderCase *mergeConfigGoldenCase
	for i, tc := range cases {
		if tc.Kind == "order" {
			orderCase = &cases[i]
		}
	}
	if orderCase == nil {
		t.Fatal("frozen oracle has no field-order case")
	}
	got := assertMergeConfigFrozenCase(t, *orderCase)

	want := []string{"zeta", "alpha", "mu"}
	if !reflect.DeepEqual(got.Config.Keys(), want) {
		t.Fatalf("config order = %v, want %v", got.Config.Keys(), want)
	}
}

// TestNativeMergeConfigNonFiniteFloatIsRejected pins the allow_nan=False
// boundary: Python's envelope serialization raises ValueError (frozen), and the
// native adapter rejects the same value instead of emitting a bare NaN token
// that encoding/json would otherwise refuse (or worse, emit differently).
func TestNativeMergeConfigNonFiniteFloatIsRejected(t *testing.T) {
	for _, tc := range loadMergeConfigGolden(t) {
		if tc.Kind != "nonfinite" {
			continue
		}
		t.Run(tc.Literal, func(t *testing.T) {
			var envRef mergeConfigEnvelopeRef
			if err := json.Unmarshal(tc.Envelope, &envRef); err != nil {
				t.Fatalf("decode frozen envelope record: %v", err)
			}
			if envRef.OK || envRef.ExcType != "ValueError" {
				t.Fatalf("frozen python envelope = %+v, want a ValueError for %s", envRef, tc.Literal)
			}
			// The two non-finite texts are NOT byte-comparable, so this pins
			// BOTH live texts instead of matching one against a hardcoded copy.
			// Python's is the live json.dumps(..., allow_nan=False) transport
			// failure, which never crosses the wire (the envelope cannot be
			// serialized, so no process runs) and CPython suffixes the offending
			// value; the native text is the adapter's own acquisition-time
			// rejection, which wraps Python's prefix in the manifest-key
			// context and has no value suffix.
			if want := "Out of range float values are not JSON compliant: " + tc.Literal; envRef.Message != want {
				t.Fatalf("frozen python message = %q, want the recorded ValueError text %q", envRef.Message, want)
			}

			_, _, recipePath, manifestPath := mergeConfigFixture(t, "alpha", tc.Recipe, tc.Manifest)
			_, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
			if err == nil {
				t.Fatalf("PlanMergeConfig accepted %s", tc.Literal)
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

// TestNativeMergeConfigDatetimeIsABoundary pins the widened internal/toml value
// subset at the envelope boundary: tomllib and internal/toml both read the
// datetime/date, and the envelope serialization then fails on both sides with
// Python's TypeError (frozen). Python's json.dumps(allow_nan=False) raises it;
// the Go acquisition rejects the same value with the same message, wrapped in
// the manifest-key context (the same wrapper shape the non-finite float
// boundary uses).
func TestNativeMergeConfigDatetimeIsABoundary(t *testing.T) {
	for _, tc := range loadMergeConfigGolden(t) {
		if tc.Kind != "datetime" {
			continue
		}
		t.Run(tc.Literal, func(t *testing.T) {
			var envRef mergeConfigEnvelopeRef
			if err := json.Unmarshal(tc.Envelope, &envRef); err != nil {
				t.Fatalf("decode frozen envelope record: %v", err)
			}
			wantMsg := "Object of type " + tc.TypeName + " is not JSON serializable"
			if envRef.OK || envRef.ExcType != "TypeError" || envRef.Message != wantMsg {
				t.Fatalf("frozen python envelope = %+v, want a TypeError %q", envRef, wantMsg)
			}

			_, _, recipePath, manifestPath := mergeConfigFixture(t, "alpha", tc.Recipe, tc.Manifest)
			_, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
			if err == nil {
				t.Fatalf("PlanMergeConfig accepted the datetime %s", tc.Literal)
			}
			wantErr := `manifest "starts" value: ` + wantMsg
			if err.Error() != wantErr {
				t.Fatalf("native error = %q, want %q", err.Error(), wantErr)
			}
		})
	}
}
