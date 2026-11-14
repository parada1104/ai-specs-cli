package target

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestPyReprStringDifferential pins pyReprString against python3 repr()
// directly: every case must render byte-identically to the Python authority.
// Probe-pinned set: quote switching (contains ' but no "), backslash, control
// chars (< 0x20, 0x7f-0x9f), non-printable Unicode (NBSP, U+2028), and
// printable non-ASCII (raw, incl. non-BMP emoji).
func TestPyReprStringDifferential(t *testing.T) {
	cases := []string{
		"plain",
		"it's",
		`say "hi"`,
		`both '"`,
		"back\\slash",
		"\x01",
		"\x1f",
		"\x7f",
		"\u0085",
		"\u009f",
		"\t",
		"\n",
		"\r",
		"caf\u00e9",
		"\U0001f44d",
		"\u00a0",
		"\u2028",
		"a\rb",
		"tab\there",
		"",
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	for _, in := range cases {
		cmd := exec.Command(py, "-c", "import sys; print(repr(sys.argv[1]))", in)
		cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONDONTWRITEBYTECODE=1")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("python3 repr(%q): %v\nstderr: %s", in, err, errb.String())
		}
		want := strings.TrimRight(out.String(), "\n")
		if got := pyReprString(in); got != want {
			t.Errorf("pyReprString(%q) = %s, want %s", in, got, want)
		}
	}
}

// buildNested builds a nested list depth levels deep (innermost nil), the
// same shape the Python recursion probe uses.
func buildNested(depth int) []any {
	var v any
	for i := 0; i < depth; i++ {
		v = []any{v}
	}
	return v.([]any)
}

// TestPyReprNestedDepthDifferential pins nested-list repr against python3 at
// a depth where Python succeeds (probe: Python 3.14 succeeds to ~69709 and
// raises RecursionError at ~69710 on the default 8 MiB stack).
func TestPyReprNestedDepthDifferential(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	const depth = 1000
	cmd := exec.Command(py, "-c", "x=None\n"+string(bytes.Repeat([]byte("x=[x]\n"), depth))+"print(repr(x))")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("python3 nested repr: %v\nstderr: %s", err, errb.String())
	}
	want := strings.TrimRight(out.String(), "\n")
	got, err := pyRepr(buildNested(depth))
	if err != nil {
		t.Fatalf("pyRepr depth %d: unexpected error: %v", depth, err)
	}
	if got != want {
		t.Errorf("nested repr mismatch at depth %d: lengths %d vs %d", depth, len(got), len(want))
	}
}

// TestPyReprDepthCapAndTableEllipsisGoOnly pins the recursion guard: a value
// nested beyond pyReprMaxDepth yields a descriptive error (Python raises
// RecursionError there instead of crashing), and a *toml.Table re-entered on
// the active path renders Python's '{...}' ellipsis. Self-referential tables
// are not producible by toml.Parse (trees), so the ellipsis is pinned with a
// pre-seeded active set.
func TestPyReprDepthCapAndTableEllipsisGoOnly(t *testing.T) {
	if _, err := pyRepr(buildNested(pyReprMaxDepth + 1)); err == nil {
		t.Error("pyRepr beyond pyReprMaxDepth must error")
	} else if !strings.Contains(err.Error(), "recursion depth") {
		t.Errorf("depth-cap error = %v, want a recursion-depth message", err)
	}

	root, err := toml.Parse([]byte("[t]\na = 1\nb = \"two\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	tbl, _ := root.Table("t")
	if got, err := pyRepr(tbl); err != nil || got != "{'a': 1, 'b': 'two'}" {
		t.Errorf("pyRepr(table) = %q, %v; want document-order dict str", got, err)
	}
	if got, err := pyStr(tbl); err != nil || got != "{'a': 1, 'b': 'two'}" {
		t.Errorf("pyStr(table) = %q, %v; want dict str", got, err)
	}
	active := map[*toml.Table]bool{tbl: true}
	if got, err := pyReprDepth(tbl, 0, active); err != nil || got != "{...}" {
		t.Errorf("pyReprDepth(active table) = %q, %v; want '{...}'", got, err)
	}
}
