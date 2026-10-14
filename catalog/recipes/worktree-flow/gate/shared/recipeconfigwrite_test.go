package shared

import (
	"encoding/json"
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
