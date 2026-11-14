package target

import (
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestEncodeCompactDateTimeReturnsError pins the Python json.dumps failure for
// a TOML date/time: the encoder mirrors json.dumps, which raises for these
// types, so it returns the TypeError text instead of panicking or rendering
// null.
func TestEncodeCompactDateTimeReturnsError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("encodeCompact panicked on a date/time: %v", r)
		}
	}()
	dt := toml.DateTime{Kind: toml.DateTimeLocal, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32}

	out, err := encodeCompact(dt)
	if err == nil {
		t.Fatalf("encodeCompact(%v) = %q, want error", dt, out)
	}
	if out != "" {
		t.Errorf("encodeCompact returned %q alongside the error, want empty string", out)
	}
	if got, want := err.Error(), "Object of type datetime is not JSON serializable"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestEncodeIndentPropagatesDateTimeError exercises the recursive indent
// encoder: a date/time nested in an ordered object must propagate through
// writeBlock as an error, not panic.
func TestEncodeIndentPropagatesDateTimeError(t *testing.T) {
	dt := toml.DateTime{Kind: toml.DateLocal, Year: 1979, Month: 5, Day: 27}
	root := newObj()
	root.set("meta", dt)

	out, err := encodeIndent(root)
	if err == nil {
		t.Fatalf("encodeIndent = %q, want error", out)
	}
	if got, want := err.Error(), "Object of type date is not JSON serializable"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestEncodeIndentBigIntExactDigits guards the other widened branch: an
// arbitrary-precision int still renders as its exact decimal digits inside an
// indented object.
func TestEncodeIndentBigIntExactDigits(t *testing.T) {
	const big = toml.BigInt("123456789012345678901234567890123456789")
	root := newObj()
	root.set("n", big)

	got, err := encodeIndent(root)
	if err != nil {
		t.Fatalf("encodeIndent: %v", err)
	}
	want := "{\n  \"n\": " + string(big) + "\n}"
	if got != want {
		t.Errorf("encodeIndent = %q, want %q", got, want)
	}
}
