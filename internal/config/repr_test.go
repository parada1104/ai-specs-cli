package config

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// configReprPath is the frozen oracle file for the repr differential; its
// repr_golden section holds one record per case, so this file never reads
// another differential's data.
const configReprPath = "testdata/config_repr.json"

// loadPyRepr returns the frozen Python repr() oracle output for one record in
// this file's repr_golden section. The oracle was captured once from Python
// (two identical runs); tests never execute an interpreter.
func loadPyRepr(t *testing.T, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(configReprPath)
	if err != nil {
		t.Fatalf("read frozen oracle file %s: %v", configReprPath, err)
	}
	var sections map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatalf("decode %s: %v", configReprPath, err)
	}
	records, ok := sections["repr_golden"]
	if !ok {
		t.Fatalf("frozen oracle file %s has no repr_golden section", configReprPath)
	}
	record, ok := records[slug]
	if !ok {
		t.Fatalf("frozen oracle file %s has no repr_golden/%s record", configReprPath, slug)
	}
	var rec struct {
		Repr string `json:"repr"`
	}
	if err := json.Unmarshal(record, &rec); err != nil {
		t.Fatalf("decode frozen repr record repr_golden/%s: %v", slug, err)
	}
	return rec.Repr
}

// TestPyReprStringDifferential pins pyReprString against the frozen Python
// repr() oracle: every case must render byte-identically to the Python
// authority.
// Probe-pinned set: quote switching (contains ' but no "), backslash, control
// chars (< 0x20, 0x7f-0x9f), non-printable Unicode (NBSP, U+2028), and
// printable non-ASCII (raw, incl. non-BMP emoji), plus U+2028 and the empty
// string. Reachable surface: the recipes-section str(version) coercion, so a
// manifest version = "\u0001" must render '\x01', not a raw control byte.
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
	for i, in := range cases {
		want := loadPyRepr(t, fmt.Sprintf("repr-string-%02d", i))
		if got := pyReprString(in); got != want {
			t.Errorf("pyReprString(%q) = %s, want %s", in, got, want)
		}
	}
}
