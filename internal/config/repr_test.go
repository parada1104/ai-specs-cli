package config

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestPyReprStringDifferential pins pyReprString against python3 repr()
// directly: every case must render byte-identically to the Python authority.
// Mirrors internal/target/repr_test.go's case list: quote switching (contains
// ' but no "), backslash, control chars (< 0x20, 0x7f-0x9f), non-printable
// Unicode (NBSP, U+2028), and printable non-ASCII (raw, incl. non-BMP emoji).
// Reachable surface: the recipes-section str(version) coercion, so a manifest
// version = "\u0001" must render '\x01', not a raw control byte.
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
