package config

import (
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestEncodeJSONDateTimeReturnsError pins the Python json.dumps failure for a
// TOML date/time: the encoder mirrors json.dumps, which raises for these
// types, so it returns the TypeError text instead of panicking or rendering
// null.
func TestEncodeJSONDateTimeReturnsError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("encodeJSON panicked on a date/time: %v", r)
		}
	}()
	dt := toml.DateTime{Kind: toml.DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32}

	out, err := encodeJSON(dt)
	if err == nil {
		t.Fatalf("encodeJSON(%v) = %q, want error", dt, out)
	}
	if out != "" {
		t.Errorf("encodeJSON returned %q alongside the error, want empty string", out)
	}
	if got, want := err.Error(), "Object of type datetime is not JSON serializable"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestReadSectionJSONPropagatesEncoderError exercises the caller chain: a
// date/time kept verbatim under an mcp server entry must surface as the
// encoder error from ReadSectionJSON, not a panic.
func TestReadSectionJSONPropagatesEncoderError(t *testing.T) {
	data, err := toml.Parse([]byte("[mcp.foo]\ncommand = \"x\"\nwhen = 1979-05-27T07:32:00Z\n"))
	if err != nil {
		t.Fatalf("toml.Parse: %v", err)
	}
	out, err := ReadSectionJSON(data, "mcp")
	if err == nil {
		t.Fatalf("ReadSectionJSON = %q, want error", out)
	}
	if got, want := err.Error(), "Object of type datetime is not JSON serializable"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestEncodeJSONBigIntExactDigits guards the other widened branch: an
// arbitrary-precision int still renders as its exact decimal digits.
func TestEncodeJSONBigIntExactDigits(t *testing.T) {
	const big = toml.BigInt("123456789012345678901234567890123456789")
	got, err := encodeJSON(big)
	if err != nil {
		t.Fatalf("encodeJSON(BigInt): %v", err)
	}
	if want := string(big); got != want {
		t.Errorf("encodeJSON(BigInt) = %q, want %q", got, want)
	}
}
