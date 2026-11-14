package config

import (
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestConfigWidenedValueKinds pins the two D6 value kinds the section readers
// must not silently drop:
//
//   - a timeout wider than int64 (toml.BigInt) is kept verbatim, exactly as
//     lib/_internal/toml-read.py:123 keeps any Python int (json.dumps renders
//     the full decimal digits);
//   - a TOML date/time renders through Python str() (toml.DateTime.PyStr) in
//     the pyStr helper instead of a Go struct dump.
//
// The int64/bool/string timeout branches are asserted unchanged alongside.
func TestConfigWidenedValueKinds(t *testing.T) {
	const big = "123456789012345678901234567890123456789"

	// timeout: arbitrary-precision int is preserved; Python authority
	// toml-read.py:123 is `timeout if isinstance(timeout, int) and not
	// isinstance(timeout, bool) else None`.
	data, err := toml.Parse([]byte("[mcp.s]\ncommand = \"x\"\ntimeout = " + big + "\n"))
	if err != nil {
		t.Fatalf("toml.Parse: %v", err)
	}
	got, err := ReadSectionJSON(data, "mcp")
	if err != nil {
		t.Fatalf("ReadSectionJSON(mcp): %v", err)
	}
	want := `{"s": {"command": "x", "args": [], "env": {}, "timeout": ` + big + `}}`
	if got != want {
		t.Errorf("BigInt timeout:\n  go:     %s\n  python: %s", got, want)
	}

	// Already-working timeout kinds must be unchanged: int64 kept, bool and
	// string nulled (Python bool is not a plain int here).
	data, err = toml.Parse([]byte("[mcp.a]\ntimeout = 30\n[mcp.b]\ntimeout = true\n[mcp.c]\ntimeout = \"30\"\n"))
	if err != nil {
		t.Fatalf("toml.Parse: %v", err)
	}
	got, err = ReadSectionJSON(data, "mcp")
	if err != nil {
		t.Fatalf("ReadSectionJSON(mcp): %v", err)
	}
	want = `{"a": {"command": null, "args": [], "env": {}, "timeout": 30}, ` +
		`"b": {"command": null, "args": [], "env": {}, "timeout": null}, ` +
		`"c": {"command": null, "args": [], "env": {}, "timeout": null}}`
	if got != want {
		t.Errorf("int64/bool/string timeout:\n  go:     %s\n  python: %s", got, want)
	}

	// pyStr: Python str(date) is the ISO form, not repr() and not a struct dump.
	date := toml.DateTime{Kind: toml.DateLocal, Year: 1979, Month: 5, Day: 27}
	if got, want := pyStr(date), "1979-05-27"; got != want {
		t.Errorf("pyStr(date) = %q, want %q", got, want)
	}
	// pyRepr: Python repr(date), used inside containers, is the constructor form.
	if got, want := pyRepr(date), "datetime.date(1979, 5, 27)"; got != want {
		t.Errorf("pyRepr(date) = %q, want %q", got, want)
	}
	// Already-working scalar kinds are unchanged.
	if got, want := pyStr(int64(5)), "5"; got != want {
		t.Errorf("pyStr(int64) = %q, want %q", got, want)
	}
	if got, want := pyStr("x"), "x"; got != want {
		t.Errorf("pyStr(string) = %q, want %q", got, want)
	}
}
