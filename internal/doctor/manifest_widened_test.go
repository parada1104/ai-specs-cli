package doctor

import (
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestManifestWidenedValueKinds pins the D6 value kinds in the manifest
// formatting helpers:
//
//   - pyTypeName renders Python type(v).__name__: "int" for an
//     arbitrary-precision int and "date"/"datetime"/"time" for a TOML
//     date/time, matching the reference helpers in internal/schema/recipe.go
//     and internal/target/json.go;
//   - pythonRepr renders Python repr(): the datetime constructor form, not a
//     Go struct dump (cli_version.py:93 uses {policy!r}).
//
// The already-working scalar kinds are asserted unchanged. truthy(BigInt) is
// also asserted to document that the recorded truthiness sub-gap is not real:
// internal/toml only produces BigInt outside int64, so it is never zero and
// the default branch already matches Python's bool(int).
func TestManifestWidenedValueKinds(t *testing.T) {
	const big = toml.BigInt("123456789012345678901234567890123456789")

	date := toml.DateTime{Kind: toml.DateLocal, Year: 1979, Month: 5, Day: 27}
	dt := toml.DateTime{Kind: toml.DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true}
	tm := toml.DateTime{Kind: toml.TimeLocal, Hour: 7, Minute: 32, Second: 1}

	cases := []struct {
		name     string
		value    any
		wantRepr string
		wantType string
	}{
		{"bigint", big, string(big), "int"},
		{"date", date, "datetime.date(1979, 5, 27)", "date"},
		{"datetime", dt, "datetime.datetime(1979, 5, 27, 7, 32, tzinfo=datetime.timezone.utc)", "datetime"},
		{"time", tm, "datetime.time(7, 32, 1)", "time"},
		{"int64-unchanged", int64(5), "5", "int"},
		{"string-unchanged", "x", "'x'", "str"},
		{"bool-unchanged", true, "True", "bool"},
	}
	for _, tc := range cases {
		if got := pythonRepr(tc.value); got != tc.wantRepr {
			t.Errorf("%s: pythonRepr = %q, want %q", tc.name, got, tc.wantRepr)
		}
		if got := pyTypeName(tc.value); got != tc.wantType {
			t.Errorf("%s: pyTypeName = %q, want %q", tc.name, got, tc.wantType)
		}
	}

	// BigInt is always outside int64 (never zero), so Python bool(int) is True
	// and the default branch already matches; no code change is needed.
	if !truthy(big) {
		t.Error("truthy(BigInt) = false, want true (bool(int) is false only for zero)")
	}
}

// TestPyStrValueWidenedValueKinds pins pyStrValue's top-level str() split:
// a date/datetime/time renders its ISO form and a wide integer its exact
// digits, while composites stay in repr form (Python str(list) == repr(list)).
// The authority is doctor.py:1339's `", ".join(str(x) for x in enabled)`;
// the already-working scalar kinds are asserted unchanged.
func TestPyStrValueWidenedValueKinds(t *testing.T) {
	big := toml.BigInt("123456789012345678901234567890")
	date := toml.DateTime{Kind: toml.DateLocal, Year: 1979, Month: 5, Day: 27}
	dt := toml.DateTime{Kind: toml.DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true}

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"date-iso", date, "1979-05-27"},
		{"datetime-iso", dt, "1979-05-27 07:32:00+00:00"},
		{"bigint-digits", big, string(big)},
		{"list-uses-repr", []any{date}, "[datetime.date(1979, 5, 27)]"},
		{"string-unchanged", "claude", "claude"},
		{"int64-unchanged", int64(5), "5"},
		{"bool-unchanged", true, "True"},
		{"none-unchanged", nil, "None"},
	}
	for _, tc := range cases {
		if got := pyStrValue(tc.value); got != tc.want {
			t.Errorf("%s: pyStrValue = %q, want %q", tc.name, got, tc.want)
		}
	}
}
