package rulesaudit

// JSON emission mirroring Python json.dumps(payload, indent=2) with the
// default ensure_ascii=True escaping. encoding/json sorts map keys and emits
// raw UTF-8, so the ordered obj type and a hand-rolled encoder are required
// for byte identity with the legacy rules-inventory.py output.

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"ai-specs.dev/ai-specs/internal/toml"
)

// obj is an insertion-ordered object mirroring Python dict insertion order.
type obj struct {
	keys []string
	vals map[string]any
}

func newObj() *obj { return &obj{vals: map[string]any{}} }

func (o *obj) set(key string, v any) {
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

func (o *obj) empty() bool { return len(o.keys) == 0 }

// encodeIndent renders v like Python json.dumps(v, indent=2). A value
// json.dumps cannot serialize (a TOML date/time) is returned as the Python
// TypeError text rather than the pre-existing "null", so the caller fails
// the way the Python authority fails.
func encodeIndent(v any) (string, error) {
	var sb strings.Builder
	if err := writeIndent(&sb, v, 0); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func writeIndent(sb *strings.Builder, v any, depth int) error {
	switch x := v.(type) {
	case *obj:
		if x == nil || x.empty() {
			sb.WriteString("{}")
			return nil
		}
		return writeBlock(sb, "{", "}", len(x.keys), depth, func(i int) error {
			sb.WriteString(jsonString(x.keys[i]))
			sb.WriteString(": ")
			return writeIndent(sb, x.vals[x.keys[i]], depth+1)
		})
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return nil
		}
		return writeBlock(sb, "[", "]", len(x), depth, func(i int) error {
			return writeIndent(sb, x[i], depth+1)
		})
	case []string:
		if len(x) == 0 {
			sb.WriteString("[]")
			return nil
		}
		return writeBlock(sb, "[", "]", len(x), depth, func(i int) error {
			sb.WriteString(jsonString(x[i]))
			return nil
		})
	case []*obj:
		if len(x) == 0 {
			sb.WriteString("[]")
			return nil
		}
		return writeBlock(sb, "[", "]", len(x), depth, func(i int) error {
			return writeIndent(sb, x[i], depth+1)
		})
	default:
		return writeCompact(sb, v)
	}
}

// writeBlock emits a non-empty JSON array/object in indent=2 style: entries at
// depth+1 separated by ",\n", closed at depth. An entry error (an
// unserializable nested value) stops emission and is returned.
func writeBlock(sb *strings.Builder, open, closeCh string, n, depth int, entry func(i int) error) error {
	pad := strings.Repeat("  ", depth)
	padIn := strings.Repeat("  ", depth+1)
	sb.WriteString(open)
	sb.WriteString("\n")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(padIn)
		if err := entry(i); err != nil {
			return err
		}
	}
	sb.WriteString("\n")
	sb.WriteString(pad)
	sb.WriteString(closeCh)
	return nil
}

// writeCompact renders a scalar (or a *obj reached through an []any slot) the
// way Python json.dumps would inline it.
func writeCompact(sb *strings.Builder, v any) error {
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		if x {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case string:
		sb.WriteString(jsonString(x))
	case int:
		sb.WriteString(strconv.Itoa(x))
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case toml.BigInt:
		// Python json.dumps renders an arbitrary-precision int in full; int64
		// is just the narrower fast path above.
		sb.WriteString(string(x))
	case float64:
		sb.WriteString(jsonFloat(x))
	case toml.DateTime:
		// Python json.dumps raises TypeError("Object of type datetime is not
		// JSON serializable") for a raw date/time in a mirrored raw section
		// (e.g. [agents].enabled). Return the same text as an ordinary error
		// so Run can fail the way the Python authority fails.
		return jsonUnserializableError{TypeName: x.PyTypeName()}
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := writeCompact(sb, e); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
	case []string:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(jsonString(e))
		}
		sb.WriteByte(']')
	case []*obj:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := writeCompact(sb, e); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
	case *obj:
		if x == nil || x.empty() {
			sb.WriteString("{}")
			return nil
		}
		sb.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(jsonString(k))
			sb.WriteString(": ")
			if err := writeCompact(sb, x.vals[k]); err != nil {
				return err
			}
		}
		sb.WriteByte('}')
	default:
		// Any remaining type (an unexpected Go numeric width, say) renders
		// "null" with no error, matching the prior behavior. float64 is
		// handled above.
		sb.WriteString("null")
	}
	return nil
}

// jsonFloat renders f like Python json.dumps(f) under its default
// allow_nan=True: the shortest round-trip float.__repr__ for finite values
// and the literal NaN / Infinity / -Infinity spellings otherwise. It is a
// copy of mcpJSONFloat in internal/sync/mcprender.go:605 (the allow_nan=True
// JSON companion of mergeconfig.pyJSONFloat, which uses the same finite-value
// rules but rejects non-finite floats with allow_nan=False). It is copied
// inline rather than imported because that helper is unexported and pulling
// internal/sync into this package would add a dependency it does not have.
// The finite rules are: decimal notation unless the decimal point position
// is <= -4 or > 16, a forced ".0" on integral values, and "-0.0" for negative
// zero.
func jsonFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	neg := math.Signbit(f)
	a := math.Abs(f)
	if a == 0 {
		if neg {
			return "-0.0"
		}
		return "0.0"
	}
	e := strconv.FormatFloat(a, 'e', -1, 64) // e.g. "1.5e+22"
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1 // position of the decimal point among the digits

	var s string
	if decpt <= -4 || decpt > 16 {
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		expStr := strconv.Itoa(exp10)
		if exp10 < 0 {
			expStr = expStr[1:]
		}
		if len(expStr) < 2 {
			expStr = "0" + expStr
		}
		signChar := byte('+')
		if exp10 < 0 {
			signChar = '-'
		}
		s = mant + "e" + string(signChar) + expStr
	} else {
		switch {
		case decpt <= 0:
			s = "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= len(digits):
			s = digits + strings.Repeat("0", decpt-len(digits)) + ".0"
		default:
			s = digits[:decpt] + "." + digits[decpt:]
		}
	}
	if neg {
		s = "-" + s
	}
	return s
}

// jsonUnserializableError is the Python TypeError json.dumps raises for a value
// it cannot serialize. The encoder mirrors Python's json.dumps, which raises
// for these types, so callers receive it as an ordinary error instead of a
// panic or a silent null.
type jsonUnserializableError struct{ TypeName string }

func (e jsonUnserializableError) Error() string {
	return "Object of type " + e.TypeName + " is not JSON serializable"
}

// jsonString renders s like Python json.dumps(s) with ensure_ascii=True:
// \b \t \n \f \r short escapes, \" \\ escaping, and \uXXXX for everything
// outside printable ASCII (astral runes as UTF-16 surrogate pairs, lowercase
// hex).
func jsonString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 || r > 0x7e {
				for _, u := range utf16.Encode([]rune{r}) {
					const hex = "0123456789abcdef"
					sb.WriteString(`\u`)
					for shift := 12; shift >= 0; shift -= 4 {
						sb.WriteByte(hex[(u>>uint(shift))&0xf])
					}
				}
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
