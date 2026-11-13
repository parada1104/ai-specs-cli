package toml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// --- Differential harness -------------------------------------------------
//
// Byte-identity of parsed values is checked against python3 stdlib tomllib:
// both sides are marshaled to JSON and compared after a JSON round-trip.

const pyDiffScript = `import sys,json,tomllib; d=tomllib.loads(sys.stdin.read()); print(json.dumps(d, sort_keys=True))`

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("json encode: %v", err)
	}
	return buf.Bytes()
}

func pyParse(t *testing.T, data []byte) (stdout string, err error) {
	t.Helper()
	cmd := exec.Command("python3", "-c", pyDiffScript)
	cmd.Stdin = bytes.NewReader(data)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if cmdErr := cmd.Run(); cmdErr != nil {
		return "", fmt.Errorf("python3 tomllib: %v: %s", cmdErr, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func diffCases(t *testing.T) []struct{ name, path string } {
	t.Helper()
	var cases []struct{ name, path string }
	add := func(pattern string) {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		for _, m := range matches {
			cases = append(cases, struct{ name, path string }{filepath.Base(m), m})
		}
	}
	add("testdata/*.toml")
	add("../../tests/fixtures/recipes/*/recipe.toml")
	add("../../catalog/recipes/*/recipe.toml")
	// ai-specs.toml manifests live at arbitrary depth: walk for them.
	err := filepath.WalkDir("../../tests/fixtures", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "ai-specs.toml" {
			cases = append(cases, struct{ name, path string }{path, path})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
	return cases
}

func TestParseDifferentialCorpus(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	cases := diffCases(t)
	if len(cases) < 3 {
		t.Fatalf("expected at least the 3 testdata corpus files, got %d", len(cases))
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			data, err := os.ReadFile(c.path)
			if err != nil {
				t.Fatalf("read %s: %v", c.path, err)
			}
			pyOut, pyErr := pyParse(t, data)
			table, goErr := Parse(data)

			if pyErr != nil {
				// Invalid TOML: the Go parser must refuse it too.
				if goErr == nil {
					t.Fatalf("python3 tomllib rejected %s (%v) but Go Parse accepted it", c.path, pyErr)
				}
				return
			}
			if goErr != nil {
				t.Fatalf("Go Parse failed on %s: %v", c.path, goErr)
			}

			var want, got any
			if err := json.Unmarshal([]byte(pyOut), &want); err != nil {
				t.Fatalf("unmarshal python side: %v", err)
			}
			if err := json.Unmarshal(mustJSON(t, table.Any()), &got); err != nil {
				t.Fatalf("unmarshal go side: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				gj, _ := json.Marshal(got)
				t.Fatalf("value mismatch for %s\ngo:     %s\npython: %s", c.path, gj, pyOut)
			}
		})
	}
}

// --- Parser API and semantics ---------------------------------------------

func TestParseSectionsAndOrder(t *testing.T) {
	root, err := Parse([]byte("z = 1\n[aaa]\nx = 1\n[mmm]\ny = 2\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	keys := root.Keys()
	want := []string{"z", "aaa", "mmm"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("Keys() = %v, want %v", keys, want)
	}
	if v, ok := root.Get("z"); !ok || v != int64(1) {
		t.Fatalf("Get(z) = %v, %v; want int64(1), true", v, ok)
	}
	sub, ok := root.Table("aaa")
	if !ok {
		t.Fatalf("Table(aaa) missing")
	}
	if v, ok := sub.Get("x"); !ok || v != int64(1) {
		t.Fatalf("Get(aaa.x) = %v, %v", v, ok)
	}
}

func TestParseStrings(t *testing.T) {
	root, err := Parse([]byte("a = \"x\\ty\\u00e9\"\nb = 'raw\\n'\nc = \"\"\"\nmulti\nline\"\"\"\nd = '''\nlit\\n'''\ne = \"\"\"cont \\\n   joined\"\"\""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v, _ := root.String("a"); v != "x\ty\u00e9" {
		t.Fatalf("a = %q", v)
	}
	if v, _ := root.String("b"); v != `raw\n` {
		t.Fatalf("b = %q", v)
	}
	if v, _ := root.String("c"); v != "multi\nline" {
		t.Fatalf("c = %q", v)
	}
	if v, _ := root.String("d"); v != "lit\\n" {
		t.Fatalf("d = %q", v)
	}
	if v, _ := root.String("e"); v != "cont joined" {
		t.Fatalf("e = %q", v)
	}
}

func TestParseNumbers(t *testing.T) {
	root, err := Parse([]byte("a = 0xDEADBEEF\nb = 0o755\nc = 0b1010\nd = 1_000\ne = -17\nf = 5e22\ng = -2E-2\nh = inf\ni = nan"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checks := []struct {
		key  string
		want any
	}{
		{"a", int64(0xDEADBEEF)},
		{"b", int64(0o755)},
		{"c", int64(0b1010)},
		{"d", int64(1000)},
		{"e", int64(-17)},
		{"f", float64(5e22)},
		{"g", float64(-2e-2)},
		{"h", math.Inf(1)},
	}
	for _, c := range checks {
		if v, ok := root.Get(c.key); !ok || v != c.want {
			t.Fatalf("Get(%s) = %v, %v; want %v", c.key, v, ok, c.want)
		}
	}
	if v, _ := root.Float64("i"); v == v { // NaN must be NaN
		t.Fatalf("nan parsed as %v", v)
	}
}

// TestParseDateTimes pins all four TOML date/time forms. tomllib returns
// datetime.datetime for the two datetime kinds and datetime.date / datetime.time
// for the other two, and internal/toml keeps the same distinction.
func TestParseDateTimes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want DateTime
	}{
		{"offset zulu", "a = 1979-05-27T07:32:00Z\n",
			DateTime{Kind: DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true}},
		{"offset lower z", "a = 1979-05-27t07:32:00z\n",
			DateTime{Kind: DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true}},
		{"offset seconds", "a = 1979-05-27T07:32:00+05:30\n",
			DateTime{Kind: DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true, OffsetSeconds: 19800}},
		{"offset negative", "a = 1979-05-27T07:32:00-00:30\n",
			DateTime{Kind: DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true, OffsetSeconds: -1800}},
		{"local datetime", "a = 1979-05-27T07:32:00\n",
			DateTime{Kind: DateTimeLocal, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32}},
		{"local datetime space", "a = 1979-05-27 07:32:00\n",
			DateTime{Kind: DateTimeLocal, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32}},
		{"fractional truncation", "a = 07:32:00.123456789\n",
			DateTime{Kind: TimeLocal, Hour: 7, Minute: 32, Micros: 123456}},
		{"local date", "a = 1979-05-27\n",
			DateTime{Kind: DateLocal, Year: 1979, Month: 5, Day: 27}},
		{"local time", "a = 07:32:00\n",
			DateTime{Kind: TimeLocal, Hour: 7, Minute: 32}},
	}
	for _, c := range cases {
		root, err := Parse([]byte(c.src))
		if err != nil {
			t.Fatalf("%s: Parse(%q): %v", c.name, c.src, err)
		}
		got, ok := root.Get("a")
		if !ok {
			t.Fatalf("%s: key a missing", c.name)
		}
		dt, isDT := got.(DateTime)
		if !isDT {
			t.Fatalf("%s: a = %T, want DateTime", c.name, got)
		}
		if dt != c.want {
			t.Fatalf("%s: a = %+v, want %+v", c.name, dt, c.want)
		}
		if dt.PyTypeName() != c.want.PyTypeName() {
			t.Fatalf("%s: PyTypeName = %q, want %q", c.name, dt.PyTypeName(), c.want.PyTypeName())
		}
	}

	// An impossible calendar date matches the grammar but must be rejected.
	for _, bad := range []string{"1979-02-30", "1900-02-29", "0000-01-01"} {
		if _, err := Parse([]byte("a = " + bad + "\n")); err == nil {
			t.Fatalf("Parse accepted the invalid date %q", bad)
		}
	}
}

const pyDateTimeScript = `import sys,json,tomllib; v=tomllib.loads(sys.stdin.read())["a"]; print(json.dumps([repr(v), str(v)]))`

// TestDateTimePythonParity pins PyRepr/PyStr against the real tomllib objects:
// every datetime/date/time literal is read by both readers and the Go repr/str
// match CPython's exactly.
func TestDateTimePythonParity(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	literals := []string{
		"1979-05-27T07:32:00Z",
		"1979-05-27T07:32:00",
		"1979-05-27 07:32:00",
		"1979-05-27T00:32:00.999999-07:00",
		"1979-05-27T07:32:00+05:30",
		"1979-05-27T07:32:00-00:30",
		"1979-05-27",
		"07:32:00",
		"07:32:00.123456789",
	}
	for _, lit := range literals {
		t.Run(lit, func(t *testing.T) {
			doc := "a = " + lit + "\n"
			root, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, _ := root.Get("a")
			dt, isDT := got.(DateTime)
			if !isDT {
				t.Fatalf("a = %T, want DateTime", got)
			}
			cmd := exec.Command("python3", "-c", pyDateTimeScript)
			cmd.Stdin = strings.NewReader(doc)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("python3: %v", err)
			}
			var want []string
			if err := json.Unmarshal(bytes.TrimSpace(out), &want); err != nil {
				t.Fatalf("decode python %q: %v", out, err)
			}
			if got := dt.PyRepr(); got != want[0] {
				t.Fatalf("PyRepr = %q, python repr = %q", got, want[0])
			}
			if got := dt.PyStr(); got != want[1] {
				t.Fatalf("PyStr = %q, python str = %q", got, want[1])
			}
		})
	}
}

// TestParseBigIntegers pins that an integer that fits int64 stays int64 and a
// wider one becomes a canonical-decimal BigInt, in every base tomllib accepts.
func TestParseBigIntegers(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"a = 9223372036854775807\n", int64(math.MaxInt64)},
		{"a = -9223372036854775808\n", int64(math.MinInt64)},
		{"a = 9223372036854775808\n", BigInt("9223372036854775808")},
		{"a = -9223372036854775809\n", BigInt("-9223372036854775809")},
		{"a = 123456789012345678901234567890\n", BigInt("123456789012345678901234567890")},
		{"a = -123456789012345678901234567890\n", BigInt("-123456789012345678901234567890")},
		{"a = 0xFFFFFFFFFFFFFFFFFFFFFFFF\n", BigInt("79228162514264337593543950335")},
		{"a = -0xFFFFFFFFFFFFFFFFFFFFFFFF\n", BigInt("-79228162514264337593543950335")},
	}
	for _, c := range cases {
		root, err := Parse([]byte(c.src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.src, err)
		}
		got, _ := root.Get("a")
		if got != c.want {
			t.Fatalf("Parse(%q) a = %#v, want %#v", c.src, got, c.want)
		}
	}
}

const pyBigIntScript = `import sys,json,tomllib; print(json.dumps(tomllib.loads(sys.stdin.read())["a"]))`

// TestBigIntPythonParity pins that a wide integer reads and renders exactly like
// Python's arbitrary-precision int, in every base. json.dumps emits the exact
// decimal digits, which is what internal/toml's BigInt carries and what the
// TOML literal writer emits.
func TestBigIntPythonParity(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	literals := []string{
		"9223372036854775807",
		"9223372036854775808",
		"-9223372036854775809",
		"123456789012345678901234567890",
		"-123456789012345678901234567890",
		"0xFFFFFFFFFFFFFFFFFFFFFFFF",
		"0o1777777777777777777777",
		"0b1000000000000000000000000000000000000000000000000000000000000000",
	}
	for _, lit := range literals {
		t.Run(lit, func(t *testing.T) {
			doc := "a = " + lit + "\n"
			root, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			v, _ := root.Get("a")
			var goDec string
			switch x := v.(type) {
			case int64:
				goDec = strconv.FormatInt(x, 10)
			case BigInt:
				goDec = string(x)
			default:
				t.Fatalf("a = %T, want int64 or BigInt", v)
			}
			cmd := exec.Command("python3", "-c", pyBigIntScript)
			cmd.Stdin = strings.NewReader(doc)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("python3: %v", err)
			}
			if got := strings.TrimSpace(string(out)); got != goDec {
				t.Fatalf("decimal: go=%q python=%q", goDec, got)
			}
		})
	}
}

// TestBigIntAnyJSON pins that Any()/PlainValue keep their "suitable for JSON
// marshaling" contract for a wide integer: the value reaches encoding/json as
// a decimal number, never a quoted string.
func TestBigIntAnyJSON(t *testing.T) {
	root, err := Parse([]byte("a = 123456789012345678901234567890\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.TrimSpace(string(mustJSON(t, root.Any()))); got != `{"a":123456789012345678901234567890}` {
		t.Fatalf("Any() JSON = %s", got)
	}
	if _, ok := PlainValue(BigInt("1")).(json.Number); !ok {
		t.Fatalf("PlainValue(BigInt) = %T, want json.Number", PlainValue(BigInt("1")))
	}
}

func TestParseArrayOfTablesNested(t *testing.T) {
	src := `[[a]]
x = 1

[[a.b]]
y = "first"

[[a]]
x = 2

[[a.b]]
y = "second"

[a.c]
z = true
`
	root, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a, ok := root.Tables("a")
	if !ok || len(a) != 2 {
		t.Fatalf("Tables(a) = %v, %v", a, ok)
	}
	// [a.c] applies to the LAST [[a]] element.
	c, ok := a[1].Table("c")
	if !ok {
		t.Fatalf("a[1].c missing")
	}
	if v, _ := c.Bool("z"); !v {
		t.Fatalf("a[1].c.z = %v", v)
	}
	// Each [[a]] element has its own [[a.b]] array.
	b0, ok := a[0].Tables("b")
	if !ok || len(b0) != 1 || mustGetString(t, b0[0], "y") != "first" {
		t.Fatalf("a[0].b = %v, %v", b0, ok)
	}
	b1, ok := a[1].Tables("b")
	if !ok || len(b1) != 1 || mustGetString(t, b1[0], "y") != "second" {
		t.Fatalf("a[1].b = %v, %v", b1, ok)
	}
	// Document-order iteration sees a's keys in insertion order.
	if keys := a[1].Keys(); !reflect.DeepEqual(keys, []string{"x", "b", "c"}) {
		t.Fatalf("a[1].Keys() = %v", keys)
	}
}

func mustGetString(t *testing.T, tbl *Table, key string) string {
	t.Helper()
	v, ok := tbl.String(key)
	if !ok {
		t.Fatalf("key %q missing", key)
	}
	return v
}

// TestPlainValue pins the public normalizer the native planner seam uses: a
// raw parsed value becomes the JSON-marshalable shape shared's planner
// consumes, with scalars unchanged. Raw array-of-tables entries (the
// [[reconcile.expectations]] shape) convert to []any of map[string]any.
// Arbitrary Go maps are not recursively normalized.
func TestPlainValue(t *testing.T) {
	src := `[[reconcile.expectations]]
config_field = "base_branch"
config_field_when_set = "feature_branch"
required = true
max_age_seconds = 3600
aliases = ["dev", "trunk"]

[reconcile.expectations.defaults]
base_branch = "development"
`
	root, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	reconcile, ok := root.Table("reconcile")
	if !ok {
		t.Fatalf("reconcile table missing")
	}
	raw, ok := reconcile.Get("expectations")
	if !ok {
		t.Fatalf("expectations missing")
	}
	if _, isRaw := raw.([]*Table); !isRaw {
		t.Fatalf("expectations raw type = %T, want []*toml.Table", raw)
	}

	arr, ok := PlainValue(raw).([]any)
	if !ok {
		t.Fatalf("PlainValue(%T) = %T, want []any", raw, PlainValue(raw))
	}
	if len(arr) != 1 {
		t.Fatalf("PlainValue len = %d, want 1", len(arr))
	}
	entry, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("entry = %T, want map[string]any", arr[0])
	}
	want := map[string]any{
		"config_field":          "base_branch",
		"config_field_when_set": "feature_branch",
		"required":              true,
		"max_age_seconds":       int64(3600),
		"aliases":               []any{"dev", "trunk"},
		"defaults":              map[string]any{"base_branch": "development"},
	}
	if !reflect.DeepEqual(entry, want) {
		t.Fatalf("entry = %#v, want %#v", entry, want)
	}

	// A parsed table normalizes to map[string]any.
	tbl, ok := PlainValue(root).(map[string]any)
	if !ok || tbl["reconcile"] == nil {
		t.Fatalf("PlainValue(root) = %#v, want map with reconcile", PlainValue(root))
	}

	// Arbitrary Go maps pass through unchanged: no recursive normalization.
	pass := map[string]any{"nested": map[string]any{"k": int64(1)}}
	if out := PlainValue(pass); !reflect.DeepEqual(out, pass) {
		t.Fatalf("PlainValue(map) = %#v, want pass-through %#v", out, pass)
	}
}

// --- Serializer ------------------------------------------------------------

func TestTOMLValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"true", true, "true"},
		{"false", false, "false"},
		{"int", 42, "42"},
		{"int64 negative", int64(-7), "-7"},
		{"float", 0.5, "0.5"},
		{"float integral", float64(100), "100.0"},
		{"float exp", 5e22, "5e+22"},
		{"float small exp", 1e-5, "1e-05"},
		{"string escapes", "a\"b\\c", `"a\"b\\c"`},
		{"string non-ascii", "é", `"\u00e9"`},
		{"string newline", "a\nb", `"a\nb"`},
		{"string astral", "\U0001F389", `"\ud83c\udf89"`},
		{"string list", []string{"a", "b"}, `["a", "b"]`},
		{"any list", []any{int64(1), "x", true}, `[1, "x", true]`},
		{"nested list", []any{[]any{int64(1)}, "y"}, `[[1], "y"]`},
		{"dict sorted", map[string]any{"b": int64(1), "a": "x"}, `{ a = "x", b = 1 }`},
		{"dict nested", map[string]any{"k": []string{"v"}}, `{ k = ["v"] }`},
		{"bigint digits", BigInt("123456789012345678901234567890"), "123456789012345678901234567890"},
	}
	for _, c := range cases {
		got, err := TOMLValue(c.in)
		if err != nil {
			t.Fatalf("%s: TOMLValue(%v) error: %v", c.name, c.in, err)
		}
		if got != c.want {
			t.Fatalf("%s: TOMLValue = %q, want %q", c.name, got, c.want)
		}
	}

	// bool-before-int: in Go the type switch is static, but pin the contract
	// that true serializes as "true" and not as a number.
	if got, _ := TOMLValue(true); got != "true" {
		t.Fatalf("bool-before-int: got %q", got)
	}

	// Ordered keys variant mirrors Python dict insertion order.
	ordered, err := TOMLValueOrdered([]string{"z", "a"}, map[string]any{"a": int64(1), "z": "last"})
	if err != nil {
		t.Fatalf("TOMLValueOrdered: %v", err)
	}
	if ordered != `{ z = "last", a = 1 }` {
		t.Fatalf("TOMLValueOrdered = %q", ordered)
	}

	// Error case names the Go type.
	if _, err := TOMLValue(struct{}{}); err == nil || !strings.Contains(err.Error(), "cannot serialize") {
		t.Fatalf("TOMLValue(struct{}{}) error = %v, want 'cannot serialize ...'", err)
	}
}

// TestCRLFLineEndings pins tomllib's observed CRLF behavior: \r\n is a line
// ending everywhere, lone \r is an error everywhere, and CRLF inside
// multi-line string values is normalized to \n.
func TestCRLFLineEndings(t *testing.T) {
	CRLF := "\r\n"

	// CRLF after key/value pairs, table headers, comments, blank lines,
	// multi-line arrays, and array-of-tables headers.
	src := strings.Join([]string{
		"# comment" + CRLF,
		CRLF,
		"title = \"crlf\" # inline" + CRLF,
		CRLF,
		"[project]" + CRLF,
		"name = \"x\"" + CRLF,
		"items = [" + CRLF,
		"  \"a\"," + CRLF,
		"  \"b\", # trailing" + CRLF,
		"]" + CRLF,
		CRLF,
		"[[deps.servers]]" + CRLF,
		"args = [\"-y\"]" + CRLF,
	}, "")
	root, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("CRLF document: Parse: %v", err)
	}
	if v, _ := root.String("title"); v != "crlf" {
		t.Fatalf("title = %q", v)
	}
	proj, _ := root.Table("project")
	if v, _ := proj.String("name"); v != "x" {
		t.Fatalf("project.name = %q", v)
	}
	if items, _ := proj.Array("items"); len(items) != 2 {
		t.Fatalf("project.items = %v", items)
	}
	deps, _ := root.Table("deps")
	if servers, _ := deps.Tables("servers"); len(servers) != 1 {
		t.Fatalf("deps.servers = %v", servers)
	}

	// Multi-line basic and literal strings normalize CRLF content to \n;
	// line-ending backslash continuation works with CRLF too.
	mlSrc := "a = \"\"\"" + CRLF +
		"line one" + CRLF +
		"line two" + CRLF +
		"\"\"\"" + CRLF +
		"b = '''" + CRLF +
		"raw" + CRLF +
		"text'''" + CRLF +
		"c = \"\"\"cont \\" + CRLF +
		"   joined\"\"\"" + CRLF
	mlRoot, err := Parse([]byte(mlSrc))
	if err != nil {
		t.Fatalf("CRLF multiline: Parse: %v", err)
	}
	if v, _ := mlRoot.String("a"); v != "line one\nline two\n" {
		t.Fatalf("a = %q, want %q", v, "line one\nline two\n")
	}
	if v, _ := mlRoot.String("b"); v != "raw\ntext" {
		t.Fatalf("b = %q, want %q", v, "raw\ntext")
	}
	if v, _ := mlRoot.String("c"); v != "cont joined" {
		t.Fatalf("c = %q, want %q", v, "cont joined")
	}

	// Lone \r is rejected everywhere.
	for _, c := range []struct{ name, src string }{
		{"lone CR after value", "a = 1\r"},
		{"lone CR alone", "\r"},
		{"raw CR in basic string", "a = \"x\ry\"\n"},
		{"CRLF in basic string", "a = \"x\r\ny\"\n"},
		{"raw CR in literal string", "a = 'x\ry'\n"},
	} {
		if _, err := Parse([]byte(c.src)); err == nil {
			t.Fatalf("%s: Parse accepted invalid input %q", c.name, c.src)
		}
	}
}

// --- Error cases ------------------------------------------------------------

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // substring of the error message
	}{
		{"duplicate key", "a = 1\na = 2\n", "duplicate"},
		{"table redefinition", "[a]\nb = 1\n[a]\n", "redefin"},
		{"bad escape", "a = \"b\\qc\"\n", "escape"},
		{"unterminated string", "a = \"abc\n", "unterminated"},
		{"unterminated array", "a = [1, 2\n", "unterminated"},
		{"non-table redefinition", "a = 1\n[a.b]\n", "redefin"},
		{"aot over table", "[a]\n[[a]]\n", "redefin"},
		{"bad int", "a = 01\n", "invalid"},
		{"bad underscore", "a = 1__0\n", "invalid"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src))
		if err == nil {
			t.Fatalf("%s: Parse accepted invalid input %q", c.name, c.src)
		}
		if !strings.Contains(strings.ToLower(err.Error()), c.want) {
			t.Fatalf("%s: error %q does not mention %q", c.name, err, c.want)
		}
	}
}

// TestTOMLValueRefusesDateTime pins the TOML-literal writer's refusal: the
// Python toml_write.toml_value chain has no datetime branch, so it raises
// TypeError("cannot serialize <name> to TOML") and internal/toml matches it.
func TestTOMLValueRefusesDateTime(t *testing.T) {
	cases := []struct {
		kind DateTimeKind
		want string
	}{
		{DateTimeOffset, "cannot serialize datetime to TOML"},
		{DateTimeLocal, "cannot serialize datetime to TOML"},
		{DateLocal, "cannot serialize date to TOML"},
		{TimeLocal, "cannot serialize time to TOML"},
	}
	for _, c := range cases {
		if _, err := TOMLValue(DateTime{Kind: c.kind}); err == nil || err.Error() != c.want {
			t.Fatalf("TOMLValue(kind %d) error = %v, want %q", c.kind, err, c.want)
		}
	}
}

func TestSortedKeysHelperUsed(t *testing.T) {
	// Pins that the "sort" import is exercised via the serializer's map path.
	got, err := TOMLValue(map[string]any{"c": int64(3), "a": int64(1), "b": int64(2)})
	if err != nil {
		t.Fatalf("TOMLValue: %v", err)
	}
	keys := strings.Split(strings.Trim(got, "{ }"), ", ")
	sort.Strings(keys)
	if strings.Join(keys, ",") != "a = 1,b = 2,c = 3" {
		t.Fatalf("dict order = %q", got)
	}
}
