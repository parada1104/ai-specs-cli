package config

// GO-20 U6d-1 (T4): unit tests for the promoted recipe-config write authority
// and its value surface. These are the flag-independent parts of
// lib/_internal/recipe-configure.py + recipe-config-write.py: assignment
// parsing (_parse_assignment), validation (_validate_values), the write with
// its valid-TOML guard (update_recipe_config) and the project-key write
// (_write_project_key).
//
// The file header of recipeconfig.go claims byte-parity with the gate's
// shared/recipeconfigwrite.go over shared fixtures; TestUpdateRecipeConfig
// MatchesGateCopy is that claim's evidence — it runs the SAME fixtures and
// requires identical resulting bytes, identical changed flags and identical
// refusal strings against the gate outputs frozen in the recipeconfig_golden
// section of testdata/config_recipeconfig.json. The gate copy parses TOML
// through its embedded-Python seam, so its results were captured once; this
// file never executes an interpreter.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// ── fixtures ───────────────────────────────────────────────────────────────

// recipeConfigUnitRecipe is the schema the parse/validate tests address:
// standard fields of several kinds (with an enum, a validation regex and a
// secret-shaped key) plus the one declared structured table.
func recipeConfigUnitRecipe() *schema.Recipe {
	return &schema.Recipe{
		ConfigSchema: &schema.ConfigSchema{
			Fields: map[string]*schema.ConfigField{
				"retries": {Type: "integer"},
				"note":    {Type: "string"},
				"mode":    {Type: "string", Enum: []string{"fast", "slow"}},
				"auto":    {Type: "boolean"},
				"api_key": {Type: "string"},
				"slug":    {Type: "string", Validation: map[string]any{"regex": "^[a-z][a-z0-9-]*$"}},
				"legacy":  {Type: "boolean"}, // "boolean" is the catalog spelling
				// A-001/B-005 (dual-blind review): fields whose declared type maps
				// to NEITHER str/int/bool — a custom name and a missing `type` —
				// carrying a validation.regex. Python's _validate_values gates the
				// regex on isinstance(value, str), never on the field kind, so a
				// non-matching string must be refused on these too.
				"floaty":  {Type: "float", Validation: map[string]any{"regex": "^[0-9]+(\\.[0-9]+)?$"}},
				"untyped": {Validation: map[string]any{"regex": "^#[0-9]+$"}},
			},
			Tables: map[string]*schema.ConfigTable{
				"reconcile": {Shape: schema.StructuredConfigShapes["reconcile"]},
			},
		},
	}
}

func writeUnitManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── ParseConfigAssignment (recipe-configure.py's _parse_assignment) ────────

func TestParseConfigAssignment(t *testing.T) {
	r := recipeConfigUnitRecipe()
	cases := []struct {
		name       string
		assignment string
		wantKey    string
		wantValue  any
		wantErr    string // exact message, or "" for success
		wantErrPfx string // message PREFIX when the embedded parser detail is diagnostic
	}{
		{name: "int field parses as TOML int", assignment: "retries=5",
			wantKey: "retries", wantValue: int64(5)},
		{name: "whitespace is stripped on both sides", assignment: "  retries = 7  ",
			wantKey: "retries", wantValue: int64(7)},
		{name: "bool field parses", assignment: "auto=false",
			wantKey: "auto", wantValue: false},
		{name: "quoted string parses", assignment: `note="hello world"`,
			wantKey: "note", wantValue: "hello world"},
		{name: "str field falls back to the raw value when not TOML", assignment: "note=In Progress",
			wantKey: "note", wantValue: "In Progress"},
		{name: "str fallback keeps the trimmed raw value", assignment: "note = In Progress ",
			wantKey: "note", wantValue: "In Progress"},
		{name: "non-str field whose value is not TOML is refused without detail",
			assignment: "retries=abc", wantErr: "invalid TOML value for retries"},
		{name: "dotted key parses its value as TOML",
			assignment: "reconcile.max_age_seconds=900",
			wantKey:    "reconcile.max_age_seconds", wantValue: int64(900)},
		{name: "dotted key with a structured value",
			assignment: `reconcile.expectations=[{event="sync",property="a"}]`,
			wantKey:    "reconcile.expectations", wantValue: nil /* asserted loosely below */, wantErr: ""},
		{name: "dotted key whose root is not a table is unknown",
			assignment: "nope.deep=1", wantErr: "unknown config key: nope"},
		{name: "whole structured table key parses an inline table",
			assignment: `reconcile={scope_field="board_id",max_age_seconds=60}`,
			wantKey:    "reconcile", wantValue: nil},
		{name: "structured table value with bad TOML carries the detail",
			assignment: "reconcile={scope_field", wantErrPfx: "invalid TOML value for reconcile: "},
		{name: "unknown field", assignment: "nope=1", wantErr: "unknown config key: nope"},
		{name: "missing equals", assignment: "retries", wantErr: "--set requires KEY=VALUE"},
		{name: "empty key", assignment: " =5", wantErr: "--set requires a non-empty key"},
		{name: "empty assignment", assignment: "", wantErr: "--set requires KEY=VALUE"},
		{name: "value keeps interior equals", assignment: "note=a=b",
			wantKey: "note", wantValue: nil}, // TOML string "a=b"? no — raw TOML fails, str fallback
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseConfigAssignment(r, tc.assignment)
			switch {
			case tc.wantErr != "":
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
			case tc.wantErrPfx != "":
				if err == nil || !strings.HasPrefix(err.Error(), tc.wantErrPfx) {
					t.Fatalf("err = %v, want prefix %q", err, tc.wantErrPfx)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Key != tc.wantKey {
					t.Errorf("key = %q, want %q", got.Key, tc.wantKey)
				}
				if tc.wantValue == nil && strings.Contains(tc.assignment, "expectations") {
					// list/inline-table values: type-check only, exact equality
					// is the writer's business (tested below).
					if _, ok := got.Value.([]any); !ok {
						if _, isTbl := got.Value.(*toml.Table); !isTbl {
							t.Errorf("value type = %T, want a TOML list or table", got.Value)
						}
					}
					return
				}
				if tc.wantValue == nil && strings.HasPrefix(tc.wantKey, "reconcile") {
					if _, isTbl := got.Value.(*toml.Table); !isTbl {
						if _, isArr := got.Value.([]any); !isArr {
							t.Errorf("value type = %T, want a TOML table or list", got.Value)
						}
					}
					return
				}
				if tc.wantValue == nil {
					// str-fallback cases spelled with a raw value: assert the
					// value equals the assignment's raw text.
					raw := tc.assignment
					if i := strings.Index(raw, "="); i >= 0 {
						raw = strings.TrimSpace(raw[i+1:])
					}
					if s, ok := got.Value.(string); !ok || s != raw {
						t.Errorf("value = %#v, want the raw string %q", got.Value, raw)
					}
					return
				}
				if !EqualPythonValue(got.Value, tc.wantValue) {
					t.Errorf("value = %#v (%T), want %#v", got.Value, got.Value, tc.wantValue)
				}
			}
		})
	}
}

// TestParseConfigAssignmentValuesOnlySplitsOnFirstEquals pins the SplitN(…, 2)
// contract: only the FIRST '=' separates key from value.
func TestParseConfigAssignmentValuesOnlySplitsOnFirstEquals(t *testing.T) {
	got, err := ParseConfigAssignment(recipeConfigUnitRecipe(), "note=a=b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := got.Value.(string); !ok || s != "a=b" {
		// raw "a=b" is not valid TOML → str fallback → exactly the raw text.
		t.Errorf("value = %#v, want the raw string \"a=b\"", got.Value)
	}
}

// ── ValidateConfigValues (recipe-configure.py's _validate_values) ──────────

func TestValidateConfigValues(t *testing.T) {
	r := recipeConfigUnitRecipe()
	cases := []struct {
		name    string
		values  []ConfigValue
		wantErr string
	}{
		{"valid ints/bools/strings pass",
			[]ConfigValue{{"retries", int64(5)}, {"auto", true}, {"note", "x"}}, ""},
		{"str field rejects a non-string",
			[]ConfigValue{{"note", int64(3)}}, "note must be a string"},
		{"bool field rejects a non-boolean",
			[]ConfigValue{{"auto", "yes"}}, "auto must be a boolean"},
		{"int field rejects a non-integer",
			[]ConfigValue{{"retries", int64(0)}, {"retries", float64(1.5)}}, "retries must be an integer"},
		{"int field rejects a boolean (Python: bool is not int)",
			[]ConfigValue{{"retries", true}}, "retries must be an integer"},
		{"enum member passes", []ConfigValue{{"mode", "fast"}}, ""},
		{"enum non-member fails with the joined list",
			[]ConfigValue{{"mode", "turbo"}}, "mode must be one of: fast, slow"},
		{"regex mismatch fails",
			[]ConfigValue{{"slug", "Not-Valid"}}, "slug does not match its validation pattern"},
		{"regex match passes", []ConfigValue{{"slug", "ok-1"}}, ""},
		{"secret-shaped literal refused",
			[]ConfigValue{{"api_key", "sk-literal"}}, "secret-shaped literal rejected for config key: api_key"},
		{"secret-shaped key accepts the interpolation form",
			[]ConfigValue{{"api_key", "${env:API_KEY}"}}, ""},
		// A-001/B-005: the regex gate follows isinstance(value, str), not kind.
		{"a non-str/int/bool field still enforces its regex on a string value",
			[]ConfigValue{{"floaty", "twelve"}}, "floaty does not match its validation pattern"},
		{"a field with no declared type still enforces its regex",
			[]ConfigValue{{"untyped", "x1"}}, "untyped does not match its validation pattern"},
		{"a matching string passes a non-str/int/bool field's regex",
			[]ConfigValue{{"floaty", "12.5"}}, ""},
		{"the regex is not applied to a non-string value (isinstance gate)",
			[]ConfigValue{{"floaty", float64(12.5)}}, ""},
		{"unknown key", []ConfigValue{{"nope", "x"}}, "unknown config key: nope"},
		{"dotted key with unknown root", []ConfigValue{{"nope.deep", int64(1)}}, "unknown config key: nope"},
		{"dotted valid sub-key passes",
			[]ConfigValue{{"reconcile.max_age_seconds", int64(900)}}, ""},
		{"dotted wrong type carries the shared grader's exact message",
			[]ConfigValue{{"reconcile.max_age_seconds", "fast"}},
			"[config.reconcile].max_age_seconds: expected integer, got str"},
		{"dotted unknown sub-key carries the shared grader's exact message",
			[]ConfigValue{{"reconcile.bogus", int64(1)}},
			"[config.reconcile]: unknown key 'bogus'"},
		{"whole structured table passes when the shape holds",
			[]ConfigValue{{"reconcile", mustInlineTable(t, `{scope_field="board_id",max_age_seconds=60}`)}}, ""},
		{"whole structured table is refused when the shape breaks",
			[]ConfigValue{{"reconcile", mustInlineTable(t, `{scope_field=1}`)}},
			"[config.reconcile].scope_field: expected string, got int"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfigValues(r, tc.values)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func mustInlineTable(t *testing.T, raw string) *toml.Table {
	t.Helper()
	v, err := parseConfigInlineValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := v.(*toml.Table)
	if !ok {
		t.Fatalf("value is %T, want a table", v)
	}
	return tbl
}

// ── UpdateRecipeConfig (recipe-config-write.py's update_recipe_config) ─────

const recipeConfigUnitManifest = `[project]
name = "unit"

[recipes.demo]
enabled = true

[recipes.demo.config]
retries = 3  # keep this comment
note = "old"
reconcile = { scope_field = "board_id" }
`

const recipeConfigUnitManifestHeaderTable = `[project]
name = "unit"

[recipes.demo]
enabled = true

[recipes.demo.config]
note = "old"

[recipes.demo.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900  # pinned
`

func TestUpdateRecipeConfig(t *testing.T) {
	t.Run("plain write replaces the value and keeps its comment", func(t *testing.T) {
		path := writeUnitManifest(t, recipeConfigUnitManifest)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"retries", int64(9)}})
		if err != nil || refusal != "" {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a clean write", changed, refusal, err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}
		got := readFile(t, path)
		if !strings.Contains(got, "retries = 9  # keep this comment\n") {
			t.Errorf("manifest = %q, want the rewritten line with its comment", got)
		}
		if !strings.Contains(got, "note = \"old\"\n") {
			t.Errorf("untouched sibling line lost: %q", got)
		}
	})

	t.Run("equal value is a no-op: changed=false, bytes untouched", func(t *testing.T) {
		path := writeUnitManifest(t, recipeConfigUnitManifest)
		before := readFile(t, path)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"retries", int64(3)}})
		if err != nil || refusal != "" || changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want an unchanged no-op", changed, refusal, err)
		}
		if after := readFile(t, path); after != before {
			t.Errorf("no-op rewrote the manifest:\nbefore %q\nafter  %q", before, after)
		}
	})

	t.Run("empty values never touch the file (even a missing one)", func(t *testing.T) {
		changed, refusal, err := UpdateRecipeConfig(filepath.Join(t.TempDir(), "absent.toml"), "demo", nil)
		if err != nil || refusal != "" || changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a local no-op", changed, refusal, err)
		}
	})

	t.Run("dotted update rewrites the inline table in place", func(t *testing.T) {
		path := writeUnitManifest(t, recipeConfigUnitManifest)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"reconcile.max_age_seconds", int64(60)}})
		if err != nil || refusal != "" || !changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a clean write", changed, refusal, err)
		}
		got := readFile(t, path)
		want := `reconcile = { scope_field = "board_id", max_age_seconds = 60 }` + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("manifest = %q, want the inline table %q", got, want)
		}
	})

	t.Run("dotted update replaces the leaf inside a header table", func(t *testing.T) {
		path := writeUnitManifest(t, recipeConfigUnitManifestHeaderTable)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"reconcile.max_age_seconds", int64(60)}})
		if err != nil || refusal != "" || !changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a clean write", changed, refusal, err)
		}
		got := readFile(t, path)
		if !strings.Contains(got, "max_age_seconds = 60  # pinned\n") {
			t.Errorf("manifest = %q, want the replaced leaf with its comment", got)
		}
		if !strings.Contains(got, "scope_field = \"board_id\"\n") {
			t.Errorf("sibling leaf lost: %q", got)
		}
	})

	t.Run("a missing recipe block is appended with enabled=true", func(t *testing.T) {
		path := writeUnitManifest(t, "[project]\nname = \"unit\"\n")
		changed, refusal, err := UpdateRecipeConfig(path, "fresh",
			[]ConfigValue{{"retries", int64(1)}})
		if err != nil || refusal != "" || !changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a clean append", changed, refusal, err)
		}
		got := readFile(t, path)
		want := "\n[recipes.fresh]\nenabled = true\n\n[recipes.fresh.config]\nretries = 1\n"
		if !strings.HasSuffix(got, want) {
			t.Errorf("manifest = %q, want the appended block %q", got, want)
		}
	})

	t.Run("whole-table and dotted update for one root are refused", func(t *testing.T) {
		path := writeUnitManifest(t, recipeConfigUnitManifest)
		before := readFile(t, path)
		changed, refusal, err := UpdateRecipeConfig(path, "demo", []ConfigValue{
			{"reconcile", mustInlineTable(t, `{scope_field="list_id"}`)},
			{"reconcile.max_age_seconds", int64(60)},
		})
		if err != nil || changed {
			t.Fatalf("changed/err = %v/%v, want a refusal", changed, err)
		}
		want := "cannot combine a whole-table and a dotted update for key(s): reconcile"
		if refusal != want {
			t.Errorf("refusal = %q, want %q", refusal, want)
		}
		if after := readFile(t, path); after != before {
			t.Errorf("refusal rewrote the manifest:\nbefore %q\nafter  %q", before, after)
		}
	})

	t.Run("replacing a multiline value is refused byte-exactly", func(t *testing.T) {
		path := writeUnitManifest(t, `[project]
name = "unit"

[recipes.demo]
enabled = true

[recipes.demo.config]
tags = """
one
two
"""
`)
		before := readFile(t, path)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"tags", []any{"x"}}})
		if err != nil || changed {
			t.Fatalf("changed/err = %v/%v, want a refusal", changed, err)
		}
		if want := "cannot replace multiline value for key 'tags'"; refusal != want {
			t.Errorf("refusal = %q, want %q", refusal, want)
		}
		if after := readFile(t, path); after != before {
			t.Errorf("refusal rewrote the manifest")
		}
	})

	t.Run("a result that would not parse is refused, bytes untouched", func(t *testing.T) {
		// An ALREADY-invalid base (`True`): nothing parses, the surgery still
		// runs over the lines and the unconditional guard must refuse with the
		// frozen framing (the embedded parser detail is diagnostic, ADR 0002).
		path := writeUnitManifest(t, "[project]\nname = \"x\"\n\n[recipes.demo.config]\nflag = True\n")
		before := readFile(t, path)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"retries", int64(5)}})
		if err != nil || changed {
			t.Fatalf("changed/err = %v/%v, want a refusal", changed, err)
		}
		if !strings.HasPrefix(refusal, "invalid TOML after config write: ") {
			t.Errorf("refusal = %q, want the frozen guard framing", refusal)
		}
		if after := readFile(t, path); after != before {
			t.Errorf("guard left modified bytes:\nbefore %q\nafter  %q", before, after)
		}
	})
}

// TestUpdateRecipeConfigNormalizesCRLFToLF pins recipe-config-write.py's
// universal-newline read (Path.read_text): the reference reads the manifest
// through text mode, so a CRLF manifest is LF-normalized in memory BEFORE the
// surgery and the whole rewritten file comes back LF. Reading the raw bytes
// instead leaves every untouched line CRLF while the rewritten line gets LF —
// mixed endings (B-002, dual-blind review).
func TestUpdateRecipeConfigNormalizesCRLFToLF(t *testing.T) {
	crlf := strings.ReplaceAll(recipeConfigUnitManifest, "\n", "\r\n")

	t.Run("a write normalizes the whole file to LF", func(t *testing.T) {
		path := writeUnitManifest(t, crlf)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"retries", int64(9)}})
		if err != nil || refusal != "" || !changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want a clean write", changed, refusal, err)
		}
		got := readFile(t, path)
		if strings.Contains(got, "\r") {
			t.Errorf("manifest carries CR after the write (mixed endings): %q", got)
		}
		want := strings.ReplaceAll(recipeConfigUnitManifest,
			"retries = 3  #", "retries = 9  #")
		if got != want {
			t.Errorf("manifest = %q, want the LF-normalized reference bytes %q", got, want)
		}
	})

	t.Run("a no-op leaves the CRLF bytes untouched", func(t *testing.T) {
		// Python returns before any write when nothing is pending, so the CRLF
		// file must survive a no-op byte for byte — normalization happens on
		// the WRITE, not on every call.
		path := writeUnitManifest(t, crlf)
		before := readFile(t, path)
		changed, refusal, err := UpdateRecipeConfig(path, "demo",
			[]ConfigValue{{"retries", int64(3)}})
		if err != nil || refusal != "" || changed {
			t.Fatalf("changed/refusal/err = %v/%q/%v, want an unchanged no-op", changed, refusal, err)
		}
		if after := readFile(t, path); after != before {
			t.Errorf("no-op rewrote the CRLF manifest:\nbefore %q\nafter  %q", before, after)
		}
	})
}

// TestWriteProjectKey covers recipe-configure.py's _write_project_key: replace
// the first matching assignment in [project] (comment preserved), insert before
// the region's end when the key is absent, and append a fresh [project] table
// when the section does not exist.
func TestWriteProjectKey(t *testing.T) {
	t.Run("replaces an existing key and keeps its comment", func(t *testing.T) {
		path := writeUnitManifest(t, "[project]\nname = \"cfg\"  # keep\nrepo_topology = \"auto\"\n\n[recipes.x]\nenabled = true\n")
		if err := WriteProjectKey(path, "repo_topology", "standalone"); err != nil {
			t.Fatal(err)
		}
		got := readFile(t, path)
		want := "[project]\nname = \"cfg\"  # keep\nrepo_topology = \"standalone\"\n"
		if !strings.HasPrefix(got, want) {
			t.Errorf("manifest = %q, want prefix %q", got, want)
		}
		if !strings.Contains(got, "[recipes.x]\nenabled = true\n") {
			t.Errorf("following section lost: %q", got)
		}
	})

	t.Run("inserts an absent key before the next header", func(t *testing.T) {
		path := writeUnitManifest(t, "[project]\nname = \"cfg\"\n\n[recipes.x]\nenabled = true\n")
		if err := WriteProjectKey(path, "repo_topology", "standalone"); err != nil {
			t.Fatal(err)
		}
		got := readFile(t, path)
		want := "[project]\nname = \"cfg\"\n\nrepo_topology = \"standalone\"\n[recipes.x]\n"
		if !strings.HasPrefix(got, want) {
			t.Errorf("manifest = %q, want prefix %q", got, want)
		}
	})

	t.Run("appends a fresh [project] table at EOF", func(t *testing.T) {
		path := writeUnitManifest(t, "[recipes.x]\nenabled = true\n")
		if err := WriteProjectKey(path, "repo_topology", "standalone"); err != nil {
			t.Fatal(err)
		}
		got := readFile(t, path)
		want := "[recipes.x]\nenabled = true\n\n[project]\nrepo_topology = \"standalone\"\n"
		if got != want {
			t.Errorf("manifest = %q, want %q", got, want)
		}
	})

	t.Run("an unserializable value is refused with the file untouched", func(t *testing.T) {
		path := writeUnitManifest(t, "[project]\nname = \"cfg\"\n")
		before := readFile(t, path)
		if err := WriteProjectKey(path, "name", nil); err == nil {
			t.Fatal("err = nil, want the serialization refusal")
		}
		if after := readFile(t, path); after != before {
			t.Errorf("refused write modified the manifest")
		}
	})
}

// TestUpdateRecipeConfigMatchesGateCopy is the L6 one-grader evidence: the
// promoted writer and the gate's shared copy (S6: promoted, not copied — the
// gate module cannot import internal/, and the root module's require/replace
// cycle forbids the reverse) produce the SAME bytes for the same fixtures and
// the SAME refusal strings. The gate copy's results were captured once through
// its embedded-Python TOML seam and are frozen in the recipeconfig_golden
// section of testdata/config_recipeconfig.json; this test needs no interpreter.
func TestUpdateRecipeConfigMatchesGateCopy(t *testing.T) {
	fixtures := []struct {
		slug     string
		base     string
		recipeID string
		values   []ConfigValue
	}{
		{
			slug: "plain-scalar-write", base: recipeConfigUnitManifest, recipeID: "demo",
			values: []ConfigValue{{"retries", int64(9)}},
		},
		{
			slug: "dotted-inline-table", base: recipeConfigUnitManifest, recipeID: "demo",
			values: []ConfigValue{{"reconcile.max_age_seconds", int64(60)}},
		},
		{
			slug: "dotted-header-table", base: recipeConfigUnitManifestHeaderTable, recipeID: "demo",
			values: []ConfigValue{{"reconcile.max_age_seconds", int64(60)}},
		},
		{
			slug: "whole-table-dotted-conflict", base: recipeConfigUnitManifest, recipeID: "demo",
			values: []ConfigValue{
				{"reconcile", mustInlineTable(t, `{scope_field="list_id"}`)},
				{"reconcile.max_age_seconds", int64(60)},
			},
		},
		{
			slug: "new-recipe-block", base: "[project]\nname = \"unit\"\n", recipeID: "fresh",
			values: []ConfigValue{{"retries", int64(1)}},
		},
	}
	for _, tc := range fixtures {
		t.Run(tc.slug, func(t *testing.T) {
			gate := loadRecipeConfigGateOracle(t, tc.slug)
			if gate.Error != "" {
				t.Fatalf("frozen gate oracle recorded an error: %s", gate.Error)
			}
			path := writeUnitManifest(t, tc.base)
			changed, refusal, err := UpdateRecipeConfig(path, tc.recipeID, tc.values)
			if err != nil {
				t.Fatalf("internal/config error: %v", err)
			}
			if changed != gate.Changed {
				t.Errorf("changed: internal/config = %v, gate = %v", changed, gate.Changed)
			}
			if refusal != gate.Refusal {
				t.Errorf("refusal mismatch:\n internal/config = %q\n gate           = %q", refusal, gate.Refusal)
			}
			if got := readFile(t, path); got != gate.Manifest {
				t.Errorf("bytes diverge:\n internal/config = %q\n gate           = %q", got, gate.Manifest)
			}
		})
	}
}

// TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts is the honest
// companion of TestUpdateRecipeConfigMatchesGateCopy (B-003, dual-blind
// review): the byte-identity pin holds over the value domain BOTH copies
// serialize, and a TOML datetime inside a re-serialized inline table is an
// explicit, documented DIVERGENCE, not shared behavior:
//
//   - the GATE copy corrupts: its bounded embedded-Python seam lowers
//     tomllib's datetime through json.dumps(default=str), so the round-tripped
//     table carries `stamped = "1979-05-27 07:32:00+00:00"` — a quoted string
//     where the manifest held an offset date-time literal;
//   - the PROMOTED copy (this package) refuses `cannot serialize datetime to
//     TOML` — byte-for-byte the Python reference's own answer
//     (toml_write.py's toml_value raises TypeError with that text) — and
//     leaves the manifest untouched.
//
// Do NOT repair the promoted copy to match the gate: the corruption is the
// gate's defect and is recorded in odd/tasks/go-20-u6-recipe.md (L6's
// one-grader paragraph and L8). The gate leg asserts the frozen gate oracle
// in the recipeconfig_golden section of testdata/config_recipeconfig.json, so
// neither subtest runs an interpreter.
func TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts(t *testing.T) {
	base := `[project]
name = "unit"

[recipes.demo]
enabled = true

[recipes.demo.config]
reconcile = { scope_field = "board_id", stamped = 1979-05-27T07:32:00Z }
`
	values := []ConfigValue{{"reconcile.max_age_seconds", int64(60)}}

	t.Run("promoted copy refuses before any write", func(t *testing.T) {
		path := writeUnitManifest(t, base)
		changed, refusal, err := UpdateRecipeConfig(path, "demo", values)
		if err != nil {
			t.Fatalf("err = %v, want the refusal as a message, not an I/O error", err)
		}
		if changed {
			t.Error("changed = true, want false (the datetime refusal fires before the write)")
		}
		if want := "cannot serialize datetime to TOML"; refusal != want {
			t.Errorf("refusal = %q, want %q (the Python reference's TypeError text)", refusal, want)
		}
		if got := readFile(t, path); got != base {
			t.Errorf("refusal modified the manifest:\n got %q\nwant %q", got, base)
		}
	})

	t.Run("gate copy corrupts the datetime into a quoted string", func(t *testing.T) {
		gate := loadRecipeConfigGateOracle(t, "datetime-gate-corruption")
		if gate.Error != "" || gate.Refusal != "" {
			t.Fatalf("frozen gate refusal/error = %q/%q, want it to accept the write", gate.Refusal, gate.Error)
		}
		if !gate.Changed {
			t.Fatal("frozen gate changed = false, want true (it silently wrote the corruption)")
		}
		const corrupt = `stamped = "1979-05-27 07:32:00+00:00"`
		if !strings.Contains(gate.Manifest, corrupt) {
			t.Errorf("frozen gate manifest = %q, want the corrupted line %q", gate.Manifest, corrupt)
		}
		if strings.Contains(gate.Manifest, "1979-05-27T07:32:00Z") {
			t.Errorf("frozen gate manifest = %q, want the date-time literal replaced by the corruption", gate.Manifest)
		}
	})
}

// recipeConfigGateOracle is one frozen gate-copy result in this file's
// recipeconfig_golden section. The gate copy parses TOML through its bounded
// embedded-Python (tomllib) seam, so its results were captured once — with a
// Python ≥3.11 interpreter present on PATH, tomllib importable — in two
// identical runs; the tests assert them without executing an interpreter.
type recipeConfigGateOracle struct {
	Changed  bool   `json:"changed"`
	Refusal  string `json:"refusal"`
	Error    string `json:"error"`
	Manifest string `json:"manifest"`
}

// configRecipeConfigPath is the frozen oracle file for the recipe-config gate
// differential; this file never reads another differential's data.
const configRecipeConfigPath = "testdata/config_recipeconfig.json"

func loadRecipeConfigGateOracle(t *testing.T, slug string) recipeConfigGateOracle {
	t.Helper()
	raw, err := os.ReadFile(configRecipeConfigPath)
	if err != nil {
		t.Fatalf("read frozen oracle file %s: %v", configRecipeConfigPath, err)
	}
	var sections map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatalf("decode %s: %v", configRecipeConfigPath, err)
	}
	records, ok := sections["recipeconfig_golden"]
	if !ok {
		t.Fatalf("frozen oracle file %s has no recipeconfig_golden section", configRecipeConfigPath)
	}
	record, ok := records[slug]
	if !ok {
		t.Fatalf("frozen oracle file %s has no recipeconfig_golden/%s record", configRecipeConfigPath, slug)
	}
	var rec recipeConfigGateOracle
	if err := json.Unmarshal(record, &rec); err != nil {
		t.Fatalf("decode frozen recipe-config record recipeconfig_golden/%s: %v", slug, err)
	}
	return rec
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
