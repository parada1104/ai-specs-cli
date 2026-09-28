package config

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"ai-specs.dev/ai-specs/internal/toml"
)

// Obj is an insertion-ordered object mirroring Python dict insertion order.
// encoding/json sorts map keys, which would break the byte-identity
// contract, so serialization goes through this type (and *toml.Table, whose
// Keys() are in document order) instead.
type Obj struct {
	keys []string
	vals map[string]any
}

func newObject() *Obj {
	return &Obj{vals: make(map[string]any)}
}

func (o *Obj) set(key string, v any) {
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

// encodeJSON renders v exactly like Python json.dumps(payload) with default
// separators (", " / ": ") and ensure_ascii=True.
func encodeJSON(v any) string {
	var sb strings.Builder
	encodeJSONInto(&sb, v)
	return sb.String()
}

func encodeJSONInto(sb *strings.Builder, v any) {
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
		sb.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case float64:
		sb.WriteString(jsonFloat(x))
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			encodeJSONInto(sb, e)
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
	case []*toml.Table:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			encodeTableInto(sb, e)
		}
		sb.WriteByte(']')
	case *toml.Table:
		encodeTableInto(sb, x)
	case *Obj:
		sb.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(jsonString(k))
			sb.WriteString(": ")
			encodeJSONInto(sb, x.vals[k])
		}
		sb.WriteByte('}')
	default:
		// Unreachable: every value entering the serializer is produced by
		// the reader in this package or parsed by internal/toml.
		sb.WriteString("null")
	}
}

func encodeTableInto(sb *strings.Builder, t *toml.Table) {
	sb.WriteByte('{')
	for i, k := range t.Keys() {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(jsonString(k))
		sb.WriteString(": ")
		v, _ := t.Get(k)
		encodeJSONInto(sb, v)
	}
	sb.WriteByte('}')
}

// jsonString renders s exactly like Python json.dumps(s) with defaults:
// ensure_ascii=True escaping (\uXXXX, lowercase hex, surrogate pairs for
// astral characters), \b \t \n \f \r short escapes, and \" \\ escaping.
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
			// ensure_ascii: everything outside printable ASCII (0x20-0x7e)
			// becomes \uXXXX, astral characters as UTF-16 surrogate pairs.
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

// jsonFloat renders f like Python json.dumps(float): float.__repr__ for
// finite values ("NaN", "Infinity", "-Infinity" otherwise).
func jsonFloat(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	return formatPyFloat(f)
}

// formatPyFloat renders f like Python str(float) (shortest round-trip
// repr): scientific notation when the decimal point position is <= -4 or
// > 16, otherwise decimal notation with a forced ".0" on integral values.
func formatPyFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	neg := math.Signbit(f)
	a := math.Abs(f)
	if a == 0 {
		if neg {
			return "-0.0"
		}
		return "0.0"
	}
	// Shortest digits and decimal exponent via the 'e' format.
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
		expStr := strconv.Itoa(absInt(exp10))
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

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// formatPyInt mirrors Python str(int) for tomllib integers.
func formatPyInt(n int64) string {
	return strconv.FormatInt(n, 10)
}
