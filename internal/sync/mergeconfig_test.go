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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// mergeConfigExpectedInventory is the hard-coded identity inventory of the
// frozen bundle, derived from the base test matrix — never generated from the
// golden at runtime. The loader must find exactly these cases, in this order,
// with no duplicates, so deleting one record (which would silently drop a
// subtest) or removing all records of one Kind (which would let its loop pass
// with zero cases) fails before any loop runs.
var mergeConfigExpectedInventory = []struct{ Kind, Name string }{
	{"differential", "defaults in field order with a manifest override"},
	{"differential", "override keeps position, unknown warns, table appends"},
	{"differential", "manifest int stays int and float stays float"},
	{"differential", "blank gate_scope resolves to auto"},
	{"differential", "enum violation is a semantic error"},
	{"differential", "missing required field is a semantic error"},
	{"differential", "nested structured shape order drives the error"},
	{"differential", "structured scalar type error after unknown-key check"},
	{"differential", "empty manifest key warns and is ignored"},
	{"differential", "empty recipe field key keeps its default"},
	{"order", "field order is recipe declaration order"},
	{"nonfinite", "nonfinite nan"},
	{"nonfinite", "nonfinite inf"},
	{"nonfinite", "nonfinite -inf"},
	{"datetime", "datetime 2024-01-02T03:04:05"},
	{"datetime", "datetime 2024-01-02"},
}

// checkMergeConfigInventory validates one loaded bundle against the hard-coded
// inventory. It is pure so the focused control can run it against a mutated
// in-memory copy.
func checkMergeConfigInventory(cases []mergeConfigGoldenCase) error {
	if len(cases) != len(mergeConfigExpectedInventory) {
		return fmt.Errorf("frozen oracle has %d cases, want %d", len(cases), len(mergeConfigExpectedInventory))
	}
	seen := map[string]bool{}
	for i, tc := range cases {
		if seen[tc.Name] {
			return fmt.Errorf("frozen oracle duplicates case %q", tc.Name)
		}
		seen[tc.Name] = true
		want := mergeConfigExpectedInventory[i]
		if tc.Kind != want.Kind || tc.Name != want.Name {
			return fmt.Errorf("frozen oracle case %d = (%s, %q), want (%s, %q)", i, tc.Kind, tc.Name, want.Kind, want.Name)
		}
	}
	return nil
}

// loadMergeConfigGolden reads the frozen authority bundle. A missing bundle is
// a failure, never a skip: the bundle IS the oracle, and its case identities
// must match the hard-coded inventory before any loop runs.
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
	if err := checkMergeConfigInventory(golden.Cases); err != nil {
		t.Fatalf("frozen merge-config oracle %s: %v", path, err)
	}
	return golden.Cases
}

// TestMergeConfigFrozenInventoryControl proves the inventory validation bites:
// the intact bundle validates, while removing one record or duplicating one
// (both in memory, never on disk) fails loudly.
func TestMergeConfigFrozenInventoryControl(t *testing.T) {
	cases := loadMergeConfigGolden(t)
	if err := checkMergeConfigInventory(cases); err != nil {
		t.Fatalf("intact inventory must validate: %v", err)
	}
	missing := append(append([]mergeConfigGoldenCase{}, cases[:1]...), cases[2:]...)
	if err := checkMergeConfigInventory(missing); err == nil {
		t.Fatal("a removed record must fail the inventory validation")
	}
	duplicated := append(append([]mergeConfigGoldenCase{}, cases...), cases[0])
	if err := checkMergeConfigInventory(duplicated); err == nil {
		t.Fatal("a duplicated record must fail the inventory validation")
	}
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

// ── Retired Python bridge coverage (GO-20 U2e T5 PR4a S5) ───────────────────
//
// tests/test_merge_config_bridge.py pinned the --plan-merge-config transport and
// the temporary Python authority. Both die with lib/ and the bridge flag (PR4b),
// so the surviving rules it also asserted are pinned here, against the native
// authority, with the expectations taken verbatim from the Python assertions.
// The frozen differential above cannot carry them: it only records the case
// matrix captured once from Python.

// mergeConfigPlanRecipe renders a fixture recipe around the shared header.
func mergeConfigPlanRecipe(sections string) string {
	return syncRecipeHeader("alpha", "Alpha") + "\n" + sections
}

// mergeConfigPlanManifest renders a fixture manifest whose [recipes.alpha.config]
// carries the given pairs (empty for no config table).
func mergeConfigPlanManifest(pairs string) string {
	body := "[recipes.alpha]\nenabled = true\n"
	if pairs != "" {
		body += "\n[recipes.alpha.config]\n" + pairs
	}
	return body
}

// mergeConfigPlanned runs the native authority over one fixture recipe/manifest
// pair and returns the shared result.
func mergeConfigPlanned(t *testing.T, recipe, manifest string) shared.MergeConfigResult {
	t.Helper()
	_, _, recipePath, manifestPath := mergeConfigFixture(t, "alpha", recipe, manifest)
	got, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
	if err != nil {
		t.Fatalf("PlanMergeConfig: %v", err)
	}
	return got
}

// overCapReconcileManifest declares the reconcile table with 33 expectation
// entries (the retired Python test's list-over-cap input).
func overCapReconcileManifest() string {
	var b strings.Builder
	b.WriteString(mergeConfigPlanManifest("board_id = \"abc\"\n"))
	b.WriteString("\n[recipes.alpha.config.reconcile]\n")
	for i := 0; i < 33; i++ {
		fmt.Fprintf(&b, "\n[[recipes.alpha.config.reconcile.expectations]]\nevent = \"e%d\"\n", i)
	}
	return b.String()
}

// TestNativeMergeConfigPlanMatrix pins the decision matrix the retired Python
// bridge test asserted beyond the frozen differential: falsy defaults, the
// required-before-enum order, the gate_impl special message, the gate_scope
// fallbacks, unknown-key warning order, warnings before a semantic error, the
// structured table rules, and Python's value rendering in enum errors.
func TestNativeMergeConfigPlanMatrix(t *testing.T) {
	const falsyDefaults = "" +
		"[config.flag]\nrequired = false\ntype = \"bool\"\ndefault = false\n\n" +
		"[config.count]\nrequired = false\ntype = \"integer\"\ndefault = 0\n\n" +
		"[config.label]\nrequired = false\ntype = \"string\"\ndefault = \"\"\n\n" +
		"[config.size]\nrequired = false\ntype = \"integer\"\ndefault = 30\n\n" +
		"[config.board_id]\nrequired = true\ntype = \"string\"\n"
	const requiredAndEnum = "" +
		"[config.board_id]\nrequired = true\ntype = \"string\"\n\n" +
		"[config.gate_mode]\nrequired = false\ntype = \"string\"\nenum = [\"always\", \"ask\", \"off\"]\n"
	const gateImpl = "[config.gate_impl]\nrequired = false\ntype = \"string\"\ndefault = \"auto\"\nenum = [\"auto\", \"go\"]\n"
	const gateScope = "[config.gate_scope]\nrequired = false\ntype = \"string\"\ndefault = \"auto\"\n"
	const otherField = "[config.other]\nrequired = false\ntype = \"string\"\n"
	const reconcile = "" +
		"[config.board_id]\nrequired = true\ntype = \"string\"\n\n" +
		"[config.reconcile]\nscope_field = \"board_id\"\nmax_age_seconds = 900\n"
	const shape = "[config.shape]\nrequired = false\ntype = \"string\"\nenum = [\"ok\"]\n"
	const ratio = "[config.ratio]\nrequired = false\ntype = \"float\"\nenum = [\"0.5\"]\n"

	cases := []struct {
		name         string
		recipe       string
		manifest     string
		wantConfig   string
		wantWarnings []string
		wantError    string
	}{
		{
			name:         "falsy defaults are selected and overridden in declaration order",
			recipe:       mergeConfigPlanRecipe(falsyDefaults),
			manifest:     mergeConfigPlanManifest("flag = true\ncount = 7\nlabel = \"x\"\nboard_id = \"abc\"\n"),
			wantConfig:   `{"flag": true, "count": 7, "label": "x", "size": 30, "board_id": "abc"}`,
			wantWarnings: []string{},
		},
		{
			name:         "false zero and empty defaults apply when the manifest omits them",
			recipe:       mergeConfigPlanRecipe(falsyDefaults),
			manifest:     mergeConfigPlanManifest("board_id = \"abc\"\n"),
			wantConfig:   `{"flag": false, "count": 0, "label": "", "size": 30, "board_id": "abc"}`,
			wantWarnings: []string{},
		},
		{
			name:         "the required error precedes an enum error",
			recipe:       mergeConfigPlanRecipe(requiredAndEnum),
			manifest:     mergeConfigPlanManifest("gate_mode = \"rust\"\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': missing required config field 'board_id'",
		},
		{
			name:         "gate_impl rejection keeps its special message",
			recipe:       mergeConfigPlanRecipe(gateImpl),
			manifest:     mergeConfigPlanManifest("gate_impl = \"bash\"\n"),
			wantWarnings: []string{},
			wantError:    "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go",
		},
		{
			name:         "empty gate_scope resolves to auto",
			recipe:       mergeConfigPlanRecipe(gateScope),
			manifest:     mergeConfigPlanManifest("gate_scope = \"\"\n"),
			wantConfig:   `{"gate_scope": "auto"}`,
			wantWarnings: []string{},
		},
		{
			name:         "whitespace gate_scope resolves to auto",
			recipe:       mergeConfigPlanRecipe(gateScope),
			manifest:     mergeConfigPlanManifest("gate_scope = \"   \"\n"),
			wantConfig:   `{"gate_scope": "auto"}`,
			wantWarnings: []string{},
		},
		{
			name:         "false gate_scope resolves to auto",
			recipe:       mergeConfigPlanRecipe(gateScope),
			manifest:     mergeConfigPlanManifest("gate_scope = false\n"),
			wantConfig:   `{"gate_scope": "auto"}`,
			wantWarnings: []string{},
		},
		{
			name:         "zero gate_scope resolves to auto",
			recipe:       mergeConfigPlanRecipe(gateScope),
			manifest:     mergeConfigPlanManifest("gate_scope = 0\n"),
			wantConfig:   `{"gate_scope": "auto"}`,
			wantWarnings: []string{},
		},
		{
			name:         "a truthy gate_scope override wins",
			recipe:       mergeConfigPlanRecipe(gateScope),
			manifest:     mergeConfigPlanManifest("gate_scope = \"manifest\"\n"),
			wantConfig:   `{"gate_scope": "manifest"}`,
			wantWarnings: []string{},
		},
		{
			name:         "an undeclared gate_scope is only an unknown key",
			recipe:       mergeConfigPlanRecipe(otherField),
			manifest:     mergeConfigPlanManifest("gate_scope = \"\"\n"),
			wantConfig:   `{}`,
			wantWarnings: []string{"recipe 'Alpha': unknown config key 'gate_scope' in manifest (ignored)"},
		},
		{
			name:       "unknown keys warn in manifest order",
			recipe:     mergeConfigPlanRecipe("[config.board_id]\nrequired = true\ntype = \"string\"\n"),
			manifest:   mergeConfigPlanManifest("zz_last = 1\nboard_id = \"abc\"\naa_first = 2\n"),
			wantConfig: `{"board_id": "abc"}`,
			wantWarnings: []string{
				"recipe 'Alpha': unknown config key 'zz_last' in manifest (ignored)",
				"recipe 'Alpha': unknown config key 'aa_first' in manifest (ignored)",
			},
		},
		{
			name:         "the unknown-key warning precedes a semantic error",
			recipe:       mergeConfigPlanRecipe("[config.board_id]\nrequired = true\ntype = \"string\"\n"),
			manifest:     mergeConfigPlanManifest("unknown = 1\n"),
			wantWarnings: []string{"recipe 'Alpha': unknown config key 'unknown' in manifest (ignored)"},
			wantError:    "recipe 'Alpha': missing required config field 'board_id'",
		},
		{
			name:     "a declared structured table is carried through verbatim",
			recipe:   mergeConfigPlanRecipe(reconcile),
			manifest: mergeConfigPlanManifest("board_id = \"abc\"\n") + "\n[recipes.alpha.config.reconcile]\nscope_field = \"board_id\"\nmax_age_seconds = 900\n\n[[recipes.alpha.config.reconcile.expectations]]\nevent = \"delivery\"\nproperty = \"list\"\nconfig_field = \"default_list\"\n",
			wantConfig: `{"board_id": "abc", "reconcile": {"scope_field": "board_id", "max_age_seconds": 900, ` +
				`"expectations": [{"event": "delivery", "property": "list", "config_field": "default_list"}]}}`,
			wantWarnings: []string{},
		},
		{
			name:         "a structured unknown sub-key is rejected",
			recipe:       mergeConfigPlanRecipe(reconcile),
			manifest:     mergeConfigPlanManifest("board_id = \"abc\"\nreconcile = { scope_field = \"a\", bogus = 1 }\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': invalid config field 'reconcile': [config.reconcile]: unknown key 'bogus'",
		},
		{
			name:         "a nested structured type error names the index",
			recipe:       mergeConfigPlanRecipe(reconcile),
			manifest:     mergeConfigPlanManifest("board_id = \"abc\"\nreconcile = { expectations = [{ event = 5 }] }\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': invalid config field 'reconcile': [config.reconcile].expectations[0].event: expected string, got int",
		},
		{
			name:         "a structured bool is not an integer",
			recipe:       mergeConfigPlanRecipe(reconcile),
			manifest:     mergeConfigPlanManifest("board_id = \"abc\"\nreconcile = { max_age_seconds = true }\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': invalid config field 'reconcile': [config.reconcile].max_age_seconds: expected integer, got bool",
		},
		{
			name:         "a structured list over the cap is rejected",
			recipe:       mergeConfigPlanRecipe(reconcile),
			manifest:     overCapReconcileManifest(),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': invalid config field 'reconcile': [config.reconcile].expectations: expected at most 32 entries, got 33",
		},
		{
			name:         "a structured non-table value is rejected",
			recipe:       mergeConfigPlanRecipe(reconcile),
			manifest:     mergeConfigPlanManifest("board_id = \"abc\"\nreconcile = \"nope\"\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': invalid config field 'reconcile': [config.reconcile]: expected table, got str",
		},
		{
			name:         "floats render as Python repr in the enum error",
			recipe:       mergeConfigPlanRecipe(ratio),
			manifest:     mergeConfigPlanManifest("ratio = 0.1\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'ratio' value '0.1' is invalid; allowed: 0.5",
		},
		{
			name:         "negative zero renders as Python repr in the enum error",
			recipe:       mergeConfigPlanRecipe(ratio),
			manifest:     mergeConfigPlanManifest("ratio = -0.0\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'ratio' value '-0.0' is invalid; allowed: 0.5",
		},
		{
			name:         "a large float renders in scientific form in the enum error",
			recipe:       mergeConfigPlanRecipe(ratio),
			manifest:     mergeConfigPlanManifest("ratio = 1e30\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'ratio' value '1e+30' is invalid; allowed: 0.5",
		},
		{
			name:         "a container renders as Python repr in the enum error",
			recipe:       mergeConfigPlanRecipe(shape),
			manifest:     mergeConfigPlanManifest("shape = { x = [1, \"two\"] }\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'shape' value '{'x': [1, 'two']}' is invalid; allowed: ok",
		},
		{
			name:         "an apostrophe switches the repr to double quotes",
			recipe:       mergeConfigPlanRecipe(shape),
			manifest:     mergeConfigPlanManifest("shape = [\"a'b\"]\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'shape' value '[\"a'b\"]' is invalid; allowed: ok",
		},
		{
			name:         "an apostrophe inside a table value switches to double quotes",
			recipe:       mergeConfigPlanRecipe(shape),
			manifest:     mergeConfigPlanManifest("shape = { k = \"o'clock\" }\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'shape' value '{'k': \"o'clock\"}' is invalid; allowed: ok",
		},
		{
			name:         "a control character escapes as xNN in the repr",
			recipe:       mergeConfigPlanRecipe(shape),
			manifest:     mergeConfigPlanManifest("shape = [\"a\\u0001b\"]\n"),
			wantWarnings: []string{},
			wantError:    "recipe 'Alpha': config field 'shape' value '['a\\x01b']' is invalid; allowed: ok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeConfigPlanned(t, tc.recipe, tc.manifest)
			if !reflect.DeepEqual(got.Warnings, tc.wantWarnings) {
				t.Fatalf("warnings = %#v, want %#v", got.Warnings, tc.wantWarnings)
			}
			if got.Error != tc.wantError {
				t.Fatalf("error = %q, want %q", got.Error, tc.wantError)
			}
			if tc.wantConfig == "" {
				return
			}
			want, err := shared.DecodeOrdered([]byte(tc.wantConfig))
			if err != nil {
				t.Fatalf("decode want config %s: %v", tc.wantConfig, err)
			}
			if !orderedEqual(got.Config, want) {
				raw, _ := json.Marshal(got.Config)
				t.Fatalf("config mismatch:\n got: %s\nwant: %s", raw, tc.wantConfig)
			}
		})
	}
}

// TestNativeMergeConfigRealCatalogSchema pins the shipped catalog schemas through
// the native authority: the retired Python bridge test asserted that the real
// worktree-flow and plan-build recipes merge to their declared defaults, accept
// overrides, and reject an out-of-enum value.
func TestNativeMergeConfigRealCatalogSchema(t *testing.T) {
	realRecipe := func(id string) string {
		return filepath.Join("..", "..", "catalog", "recipes", id, "recipe.toml")
	}
	realManifest := func(t *testing.T, id, pairs string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "ai-specs", "ai-specs.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(mergeConfigPlanManifestFor(id, pairs)), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	plan := func(t *testing.T, id, pairs string) shared.MergeConfigResult {
		t.Helper()
		got, err := PlanMergeConfig(MergeConfigRequest{
			RecipePath:   realRecipe(id),
			ManifestPath: realManifest(t, id, pairs),
			RecipeID:     id,
		})
		if err != nil {
			t.Fatalf("PlanMergeConfig(%s): %v", id, err)
		}
		return got
	}

	t.Run("worktree-flow declares its shipped defaults", func(t *testing.T) {
		got := plan(t, "worktree-flow", "")
		if got.Error != "" {
			t.Fatalf("error = %q, want none", got.Error)
		}
		for key, want := range map[string]string{
			"worktrees_dir":      `".worktrees"`,
			"gate_impl":          `"auto"`,
			"auto_remove_merged": "true",
		} {
			value, present := got.Config.Get(key)
			if !present {
				t.Fatalf("config has no %q: %s", key, mergeConfigRaw(t, got))
			}
			raw, _ := json.Marshal(value)
			if string(raw) != want {
				t.Fatalf("config[%q] = %s, want %s", key, raw, want)
			}
		}
	})

	t.Run("worktree-flow applies overrides and warns once", func(t *testing.T) {
		got := plan(t, "worktree-flow", "gate_mode = \"ask\"\nworktrees_dir = \"custom\"\ngate_scope = \"   \"\nmystery = 1\n")
		for key, want := range map[string]string{
			"gate_mode":     `"ask"`,
			"worktrees_dir": `"custom"`,
			"gate_scope":    `"auto"`,
		} {
			value, present := got.Config.Get(key)
			if !present {
				t.Fatalf("config has no %q: %s", key, mergeConfigRaw(t, got))
			}
			raw, _ := json.Marshal(value)
			if string(raw) != want {
				t.Fatalf("config[%q] = %s, want %s", key, raw, want)
			}
		}
		if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "mystery") {
			t.Fatalf("warnings = %#v, want one warning naming mystery", got.Warnings)
		}
	})

	t.Run("worktree-flow rejects the removed bash gate_impl", func(t *testing.T) {
		got := plan(t, "worktree-flow", "gate_impl = \"bash\"\n")
		if want := "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go"; got.Error != want {
			t.Fatalf("error = %q, want %q", got.Error, want)
		}
	})

	t.Run("plan-build-flow carries its enum default and rejects a stranger", func(t *testing.T) {
		got := plan(t, "plan-build-flow", "")
		if want := `{"artifact_store_default":"openspec"}`; mergeConfigRaw(t, got) != want {
			t.Fatalf("config = %s, want %s", mergeConfigRaw(t, got), want)
		}
		got = plan(t, "plan-build-flow", "artifact_store_default = \"both\"\n")
		if want := `{"artifact_store_default":"both"}`; mergeConfigRaw(t, got) != want {
			t.Fatalf("config = %s, want %s", mergeConfigRaw(t, got), want)
		}
		got = plan(t, "plan-build-flow", "artifact_store_default = \"vault\"\n")
		if !strings.Contains(got.Error, "allowed:") {
			t.Fatalf("error = %q, want the allowed-values message", got.Error)
		}
	})
}

// mergeConfigPlanManifestFor renders a manifest for the given recipe id.
func mergeConfigPlanManifestFor(id, pairs string) string {
	body := "[recipes." + id + "]\nenabled = true\n"
	if pairs != "" {
		body += "\n[recipes." + id + ".config]\n" + pairs
	}
	return body
}

// mergeConfigRaw renders a result's config as compact JSON.
func mergeConfigRaw(t *testing.T, got shared.MergeConfigResult) string {
	t.Helper()
	raw, err := json.Marshal(got.Config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return string(raw)
}

// TestNativeMergeConfigTimeAndNestedDatetimeAreBoundaries pins the two
// non-JSON value shapes the retired Python bridge test reached through its
// fallback (which kept the typed value): a local TOML time, and a datetime
// nested inside a table. The native authority rejects both at the envelope
// boundary with Python's serialization message, so no unserializable value can
// silently survive the merge.
func TestNativeMergeConfigTimeAndNestedDatetimeAreBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		recipe   string
		manifest string
		wantErr  string
	}{
		{
			name:     "local time",
			recipe:   mergeConfigPlanRecipe("[config.starts]\nrequired = false\ntype = \"string\"\n"),
			manifest: mergeConfigPlanManifest("starts = 03:04:05\n"),
			wantErr:  `manifest "starts" value: Object of type time is not JSON serializable`,
		},
		{
			name:     "datetime nested in a table",
			recipe:   mergeConfigPlanRecipe("[config.blob]\nrequired = false\ntype = \"string\"\n"),
			manifest: mergeConfigPlanManifest("blob = { when = [2024-01-02T03:04:05] }\n"),
			wantErr:  `manifest "blob" value: Object of type datetime is not JSON serializable`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, recipePath, manifestPath := mergeConfigFixture(t, "alpha", tc.recipe, tc.manifest)
			_, err := PlanMergeConfig(MergeConfigRequest{RecipePath: recipePath, ManifestPath: manifestPath, RecipeID: "alpha"})
			if err == nil {
				t.Fatalf("PlanMergeConfig accepted the non-JSON value")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("native error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}
