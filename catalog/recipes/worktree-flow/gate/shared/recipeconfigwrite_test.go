package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unit tests for the promoted recipe-config writer core. These moved out of the
// gate package main test file with GO-07 S14a: they assert the pure
// serialization and line-surgery helpers directly, with no subprocess seam. The
// envelope/CLI tests remain in the gate package.

func TestTomlKeyEncoding(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mode", "mode"},
		{"max_age_seconds", "max_age_seconds"},
		{"A-b_C9", "A-b_C9"},
		{"my.recipe", `"my.recipe"`},
		{"a b", `"a b"`},
		{"", `""`},
		{"ключ", `"\u043a\u043b\u044e\u0447"`},
		{"with#hash", `"with#hash"`},
	}
	for _, tc := range cases {
		if got := tomlKey(tc.in); got != tc.want {
			t.Errorf("tomlKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPyJSONStringEscaping pins the json.dumps(ensure_ascii=True) contract the
// Python writer relies on: named escapes, lowercase 4-digit hex for control
// and non-ASCII characters, 0x7f escaped, and surrogate pairs above the BMP.
func TestPyJSONStringEscaping(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"tab\tx", `"tab\tx"`},
		{"nl\nx", `"nl\nx"`},
		{"cr\rx", `"cr\rx"`},
		{"bs\bx", `"bs\bx"`},
		{"vt\x0bx", `"vt\u000bx"`},
		{"ff\x0cx", `"ff\fx"`},
		{"ctl\x01x", `"ctl\u0001x"`},
		{"del\x7fx", `"del\u007fx"`},
		{"café", `"caf\u00e9"`},
		{"emoji \U0001F600", `"emoji \ud83d\ude00"`},
		{"uni\u2028x", `"uni\u2028x"`},
		{"plain", `"plain"`},
	}
	for _, tc := range cases {
		if got := PyJSONString(tc.in); got != tc.want {
			t.Errorf("PyJSONString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTomlValueSerialization(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"true", true, "true"},
		{"false", false, "false"},
		{"int", json.Number("5"), "5"},
		{"float", json.Number("5.0"), "5.0"},
		{"exponent", json.Number("1e-06"), "1e-06"},
		{"negative", json.Number("-0.5"), "-0.5"},
		{"string", "hello", `"hello"`},
		{"escaped string", "a \"quoted\" # value", `"a \"quoted\" # value"`},
		{"empty list", []any{}, "[]"},
		{"list", []any{json.Number("1"), "a", true}, `[1, "a", true]`},
		{"empty dict", NewOrderedMap(), "{  }"},
		{"dict", func() any {
			m := NewOrderedMap()
			m.Set("a", json.Number("1"))
			m.Set("b", "x")
			return m
		}(), `{ a = 1, b = "x" }`},
		{"nested", func() any {
			inner := NewOrderedMap()
			inner.Set("k", []any{json.Number("1")})
			outer := []any{inner}
			return outer
		}(), `[{ k = [1] }]`},
	}
	for _, tc := range cases {
		got, err := tomlValue(tc.in)
		if err != nil {
			t.Fatalf("%s: tomlValue: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: tomlValue = %q, want %q", tc.name, got, tc.want)
		}
	}
	if _, err := tomlValue(nil); err == nil || err.Error() != "cannot serialize NoneType to TOML" {
		t.Errorf("tomlValue(nil) error = %v, want the NoneType refusal", err)
	}
}

// TestPyValueEqual pins the Python == parity of the pending check, including
// the numeric edge cases Python's equality accepts (True == 1, 5 == 5.0).
func TestPyValueEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		{"int equal", json.Number("5"), json.Number("5"), true},
		{"int vs float", json.Number("5"), json.Number("5.0"), true},
		{"int differs", json.Number("5"), json.Number("6"), false},
		{"float equal", json.Number("1.5"), json.Number("1.5"), true},
		{"bool cross int", true, json.Number("1"), true},
		{"bool cross int false", false, json.Number("0"), true},
		{"bool cross float", true, json.Number("1.0"), true},
		{"bool vs two", true, json.Number("2"), false},
		{"bool equal", true, true, true},
		{"bool differs", true, false, false},
		{"string equal", "fast", "fast", true},
		{"string differs", "fast", "slow", false},
		{"string vs number", "1", json.Number("1"), false},
		{"bool vs string", true, "1", false},
		{"nil vs nil", nil, nil, true},
		{"nil vs number", nil, json.Number("0"), false},
		{"list equal", []any{json.Number("1"), json.Number("2")}, []any{json.Number("1"), json.Number("2")}, true},
		{"list vs bool element", []any{json.Number("1")}, []any{true}, true},
		{"list length", []any{json.Number("1")}, []any{}, false},
		{"dict equal", func() any {
			m := NewOrderedMap()
			m.Set("a", json.Number("1"))
			m.Set("b", "x")
			return m
		}(), func() any {
			m := NewOrderedMap()
			m.Set("b", "x")
			m.Set("a", json.Number("1"))
			return m
		}(), true},
		{"dict differs", func() any {
			m := NewOrderedMap()
			m.Set("a", json.Number("1"))
			return m
		}(), NewOrderedMap(), false},
	}
	for _, tc := range cases {
		if got := pyValueEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: pyValueEqual(%#v, %#v) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPySplitLines(t *testing.T) {
	got := pySplitLines("a\nb\r\nc\rd")
	want := []string{"a\n", "b\r\n", "c\r", "d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("pySplitLines = %#v, want %#v", got, want)
	}
	if lines := pySplitLines("a\n"); len(lines) != 1 || lines[0] != "a\n" {
		t.Fatalf("trailing newline keeps one line, got %#v", lines)
	}
}

// TestPySplitLinesExoticSeparators pins the remaining Python
// str.splitlines(keepends=True) separators: \v \f \x1c \x1d \x1e \u0085,
// U+2028 and U+2029, each kept as its own line ending.
func TestPySplitLinesExoticSeparators(t *testing.T) {
	got := pySplitLines("a\x0bb\x0cc\x1cd\x1de\x1ef\u0085g\u2028h\u2029")
	want := []string{"a\x0b", "b\x0c", "c\x1c", "d\x1d", "e\x1e", "f\u0085", "g\u2028", "h\u2029"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("pySplitLines = %#v, want %#v", got, want)
	}
}

// TestPyNumEqualNumericEdges covers the numeric branches the parity tests
// never reach: integers beyond int64 (bigIntEqual), the float64 comparison
// path, the +Inf overflow edge, and the documented accepted corner
// (int > 2^53 against a float approximates Python ==).
func TestPyNumEqualNumericEdges(t *testing.T) {
	cases := []struct {
		name       string
		a, b       string
		want       bool
		wantPython bool // what CPython == says; false means the accepted corner
	}{
		{"int64 edge equal", "9223372036854775807", "9223372036854775807", true, true},
		{"beyond int64 equal", "90071992547409930000", "90071992547409930000", true, true},
		{"beyond int64 differs", "90071992547409930000", "90071992547409930001", false, false},
		{"leading zeros", "0007", "7", true, true},
		{"negative zero", "-0", "0", true, true},
		{"sign differs", "-9223372036854775808", "9223372036854775808", false, false},
		{"float overflow equal", "1e999", "1e999", true, true},
		{"float overflow differs", "1e999", "-1e999", false, false},
		{"accepted corner: huge int vs float", "9007199254740993", "9007199254740992.0", true, false},
	}
	for _, tc := range cases {
		if got := pyNumEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: pyNumEqual(%q, %q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
		if tc.wantPython != tc.want {
			t.Logf("%s: accepted corner (Python == would say %v)", tc.name, tc.wantPython)
		}
	}
}

// TestMatchKeyLine covers the bare/quoted alternation and indent capture the
// end-to-end tests only reach through whole manifests.
func TestMatchKeyLine(t *testing.T) {
	cases := []struct {
		line, key string
		indent    string
		want      bool
	}{
		{"mode = 1", "mode", "", true},
		{"  mode = 1", "mode", "  ", true},
		{"\tmode = 1", "mode", "\t", true},
		{"mode=true", "mode", "", true},
		{`"my.key" = 1`, "my.key", "", true},
		{"mode = 1", "ode", "", false},
		{"mode_x = 1", "mode", "", false},
		{"mode: 1", "mode", "", false},
		{"mode = 1", "my.key", "", false},
		{"", "mode", "", false},
	}
	for _, tc := range cases {
		indent, ok := matchKeyLine(tc.line, tc.key)
		if ok != tc.want || (ok && indent != tc.indent) {
			t.Errorf("matchKeyLine(%q, %q) = (%q, %v), want (%q, %v)", tc.line, tc.key, indent, ok, tc.indent, tc.want)
		}
	}
}

// TestNestedGetSet covers the dotted-path helpers: absent segments, non-table
// intermediates, and the reference's replace-non-table-with-table semantics.
func TestNestedGetSet(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", json.Number("1"))
	if v, ok := nestedGet(m, []string{"a"}); !ok || v != json.Number("1") {
		t.Errorf("nestedGet existing = (%v, %v)", v, ok)
	}
	if _, ok := nestedGet(m, []string{"a", "b"}); ok {
		t.Errorf("nestedGet through a scalar must be absent")
	}
	if _, ok := nestedGet(m, []string{"missing"}); ok {
		t.Errorf("nestedGet missing must be absent")
	}

	nestedSet(m, []string{"a", "b"}, "x") // scalar 'a' is replaced by a table
	table, ok := m.Get("a")
	if !ok {
		t.Fatalf("nestedSet: key 'a' missing")
	}
	asMap, isMap := table.(*OrderedMap)
	if !isMap {
		t.Fatalf("nestedSet: scalar intermediate not replaced by a table: %T", table)
	}
	if v, _ := asMap.Get("b"); v != "x" {
		t.Errorf("nestedSet leaf = %v, want x", v)
	}
	nestedSet(m, nil, "ignored") // empty path is a no-op
}

// TestSubtableHeader covers the quoted-header composition for non-bare
// table path segments. The recipe key arrives pre-encoded (callers pass
// tomlKey(recipeID)); path segments are encoded here.
func TestSubtableHeader(t *testing.T) {
	cases := []struct {
		recipeKey string
		path      []string
		want      string
	}{
		{"reconcile", []string{"opts"}, "[recipes.reconcile.config.opts]"},
		{`"my.recipe"`, []string{"opts"}, `[recipes."my.recipe".config.opts]`},
		{"reconcile", []string{"opts", "deep.key"}, `[recipes.reconcile.config.opts."deep.key"]`},
	}
	for _, tc := range cases {
		if got := subtableHeader(tc.recipeKey, tc.path); got != tc.want {
			t.Errorf("subtableHeader(%q, %v) = %q, want %q", tc.recipeKey, tc.path, got, tc.want)
		}
	}
}

func TestSplitInlineComment(t *testing.T) {
	cases := []struct{ line, value, comment string }{
		{`mode = "fast"  # tail`, `mode = "fast"`, `  # tail`},
		{`mode = "fast"`, `mode = "fast"`, ``},
		{`mode = 'x # y'`, `mode = 'x # y'`, ``},
		{`mode = "x # y"`, `mode = "x # y"`, ``},
		{`a = "esc\"" # tail`, `a = "esc\""`, ` # tail`},
		{`a = """x # y"""`, `a = """x # y"""`, ``},
		{`a = 1  `, `a = 1  `, ``},
		{`a = 1 # c`, `a = 1`, ` # c`},
	}
	for _, tc := range cases {
		value, comment := splitInlineComment(tc.line)
		if value != tc.value || comment != tc.comment {
			t.Errorf("splitInlineComment(%q) = (%q, %q), want (%q, %q)", tc.line, value, comment, tc.value, tc.comment)
		}
	}
}

func TestValueIsMultiline(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{`a = { x = 1 }`, false},
		{`a = { x = [1, 2] }`, false},
		{`a = [1,`, true},
		{`a = "open`, true},
		{`a = """x"""`, true},
		{`a = "x" # y`, false},
		{`a = 'x'`, false},
		{`a = 1`, false},
		{`a = [ # comment`, true},
		{`a = { x = "}" }`, false},
		{`a = { x = "}`, true},
		{`a = ]`, false},
		{`a = { x = 1 }}`, false},
	}
	for _, tc := range cases {
		if got := valueIsMultiline(tc.line); got != tc.want {
			t.Errorf("valueIsMultiline(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// TestTomlValueInlineNonBareKeysQuoted pins the Go round-trip correctness
// divergence from the Python reference: inline-table keys that are not bare
// TOML keys must be emitted quoted, including nested dicts.
func TestTomlValueInlineNonBareKeysQuoted(t *testing.T) {
	inner := NewOrderedMap()
	inner.Set("my key", json.Number("1"))
	inner.Set("a.b", "x")
	got, err := tomlValue(inner)
	if err != nil {
		t.Fatalf("tomlValue: %v", err)
	}
	want := `{ "my key" = 1, "a.b" = "x" }`
	if got != want {
		t.Errorf("tomlValue(inline) = %q, want %q", got, want)
	}

	deep := NewOrderedMap()
	deep.Set("opts", []any{func() any {
		m := NewOrderedMap()
		m.Set("a b", true)
		return m
	}()})
	got, err = tomlValue(deep)
	if err != nil {
		t.Fatalf("tomlValue(deep): %v", err)
	}
	want = `{ opts = [{ "a b" = true }] }`
	if got != want {
		t.Errorf("tomlValue(deep) = %q, want %q", got, want)
	}
}

// --- GO-20 U2e T5: the TOML acquisition seam runs in process ---

// recipeConfigPythonShim puts a python3 shim first on PATH that records every
// invocation and fails loudly, and returns the record file.
func recipeConfigPythonShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(t.TempDir(), "python3-calls")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + record + "\n" +
		"echo 'python3 must not be invoked' >&2\n" +
		"exit 97\n"
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

func recipeConfigPythonCalls(t *testing.T, record string) string {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

// TestRecipeConfigSeamNeverInvokesAnInterpreter pins that reading the recipe
// config table, parsing an inline value and validating the candidate text all
// run in process: no ambient python3 is executed by the writer.
func TestRecipeConfigSeamNeverInvokesAnInterpreter(t *testing.T) {
	record := recipeConfigPythonShim(t)
	manifest := []byte("[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n")
	config, err := readRecipeConfigTable(manifest, "reconcile")
	if err != nil {
		t.Fatalf("readRecipeConfigTable: %v", err)
	}
	if got := config.Values()["mode"]; got != "fast" {
		t.Fatalf("mode = %v, want fast", got)
	}
	if _, err := parseInlineTomlValue(`{ mode = "fast" }`); err != nil {
		t.Fatalf("parseInlineTomlValue: %v", err)
	}
	if err := validateTomlText(string(manifest)); err != nil {
		t.Fatalf("validateTomlText: %v", err)
	}
	if calls := recipeConfigPythonCalls(t, record); calls != "" {
		t.Fatalf("python3 was invoked: %q, want none", calls)
	}
}

// TestReadRecipeConfigTableKeepsTheReferenceDomain pins the values the retired
// reader produced: ordered tables, json.Number for numbers, and the reference's
// two swallows (an unparseable manifest and a non-table config yield {}).
func TestReadRecipeConfigTableKeepsTheReferenceDomain(t *testing.T) {
	manifest := []byte("[recipes.reconcile.config]\n" +
		"zeta = \"last\"\n" +
		"alpha = 0x1F\n" +
		"ratio = 1.5\n" +
		"flag = true\n" +
		"items = [1, \"two\"]\n" +
		"nested = { mode = \"fast\" }\n")
	config, err := readRecipeConfigTable(manifest, "reconcile")
	if err != nil {
		t.Fatalf("readRecipeConfigTable: %v", err)
	}
	if keys := config.Keys(); len(keys) != 6 || keys[0] != "zeta" || keys[5] != "nested" {
		t.Fatalf("keys = %v, want declaration order preserved", keys)
	}
	if got := config.Values()["alpha"]; !pyValueEqual(got, json.Number("31")) {
		t.Fatalf("alpha = %#v, want json.Number(\"31\")", got)
	}
	if got := config.Values()["ratio"]; !pyValueEqual(got, json.Number("1.5")) {
		t.Fatalf("ratio = %#v, want json.Number(\"1.5\")", got)
	}
	if got, _ := config.Values()["nested"].(*OrderedMap); got == nil {
		t.Fatalf("nested = %#v, want an ordered table", config.Values()["nested"])
	}

	for _, tc := range []struct {
		name string
		text string
	}{
		{"unparseable manifest", "[recipes.reconcile.config\nmode = \"fast\"\n"},
		{"config is not a table", "[recipes.reconcile]\nconfig = 5\n"},
		{"recipe absent", "[recipes.other]\nenabled = true\n"},
		{"recipes not a table", "recipes = 5\n"},
	} {
		config, err := readRecipeConfigTable([]byte(tc.text), "reconcile")
		if err != nil {
			t.Fatalf("%s: readRecipeConfigTable: %v", tc.name, err)
		}
		if len(config.Keys()) != 0 {
			t.Fatalf("%s: config = %v, want the empty-table fallback", tc.name, config.Values())
		}
	}

	if _, err := readRecipeConfigTable([]byte("mode = \"\xff\"\n"), "reconcile"); err == nil {
		t.Fatal("error = nil, want invalid UTF-8 reported")
	}
}

// TestParseInlineTomlValueKeepsTheReferenceDomain pins the inline-value reader:
// scalars, arrays, inline tables and the reference's default=str lowering of a
// TOML date/time to a Python str.
func TestParseInlineTomlValueKeepsTheReferenceDomain(t *testing.T) {
	cases := []struct {
		text string
		want any
	}{
		{`"fast"`, "fast"},
		{`31`, json.Number("31")},
		{`true`, true},
		{`[1, "two"]`, []any{json.Number("1"), "two"}},
	}
	for _, tc := range cases {
		got, err := parseInlineTomlValue(tc.text)
		if err != nil {
			t.Fatalf("parseInlineTomlValue(%s): %v", tc.text, err)
		}
		if !pyValueEqual(got, tc.want) {
			t.Fatalf("parseInlineTomlValue(%s) = %#v, want %#v", tc.text, got, tc.want)
		}
	}
	table, err := parseInlineTomlValue(`{ mode = "fast" }`)
	if err != nil {
		t.Fatalf("parseInlineTomlValue(inline table): %v", err)
	}
	if _, ok := table.(*OrderedMap); !ok {
		t.Fatalf("parseInlineTomlValue(inline table) = %#v, want an ordered table", table)
	}
	stamp, err := parseInlineTomlValue("1979-05-27T07:32:00Z")
	if err != nil {
		t.Fatalf("parseInlineTomlValue(datetime): %v", err)
	}
	text, ok := stamp.(string)
	if !ok || !strings.HasPrefix(text, "1979-05-27 07:32:00") {
		t.Fatalf("parseInlineTomlValue(datetime) = %#v, want the reference's str() lowering", stamp)
	}
	if _, err := parseInlineTomlValue(`"unterminated`); err == nil {
		t.Fatal("error = nil, want the unparseable inline value reported")
	}
}

// TestValidateTomlTextIsThePostWriteGate pins the final validation: valid text
// passes, invalid text is refused with the frozen framing upstream.
func TestValidateTomlTextIsThePostWriteGate(t *testing.T) {
	if err := validateTomlText("[recipes.x.config]\nmode = \"fast\"\n"); err != nil {
		t.Fatalf("validateTomlText(valid): %v", err)
	}
	for _, bad := range []string{"[recipes.x.config\nmode = 1\n", "[recipes.x]\nmode = \"unterminated\n", "mode = "} {
		if err := validateTomlText(bad); err == nil {
			t.Fatalf("validateTomlText(%q) = nil, want a refusal", bad)
		}
	}
}
