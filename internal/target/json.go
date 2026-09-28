package target

// JSON emission mirroring Python json.dumps for the plan surface:
//   - compact mode (default separators ", " / ": ") for the error envelope
//   - indent=2 mode for the plan (json.dumps(plan, indent=2))
// with ensure_ascii=True escaping throughout. Ordered objects use the local
// obj type (internal/config's Obj is intentionally unexported and outside
// this package's allowed edit surface).

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// obj is an insertion-ordered object mirroring Python dict insertion order.
type obj struct {
	keys []string
	vals map[string]any
}

func newObj() *obj {
	return &obj{vals: make(map[string]any)}
}

func (o *obj) set(key string, v any) {
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

func (o *obj) empty() bool { return len(o.keys) == 0 }

// encodeCompact renders v like Python json.dumps(v) with default separators.
func encodeCompact(v any) string {
	var sb strings.Builder
	writeCompact(&sb, v)
	return sb.String()
}

func writeCompact(sb *strings.Builder, v any) {
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
	case float64:
		sb.WriteString(formatPyFloat(x))
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			writeCompact(sb, e)
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
			writeCompact(sb, e)
		}
		sb.WriteByte(']')
	case *obj:
		if x == nil || x.empty() {
			sb.WriteString("{}")
			return
		}
		sb.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(jsonString(k))
			sb.WriteString(": ")
			writeCompact(sb, x.vals[k])
		}
		sb.WriteByte('}')
	default:
		// Unreachable: plan values are strings, bools, string lists, and obj.
		sb.WriteString("null")
	}
}

// encodeIndent renders v like Python json.dumps(v, indent=2).
func encodeIndent(v any) string {
	var sb strings.Builder
	writeIndent(&sb, v, 0)
	return sb.String()
}

func writeIndent(sb *strings.Builder, v any, depth int) {
	switch x := v.(type) {
	case *obj:
		if x == nil || x.empty() {
			sb.WriteString("{}")
			return
		}
		writeBlock(sb, "{", "}", len(x.keys), depth, func(i int) {
			sb.WriteString(jsonString(x.keys[i]))
			sb.WriteString(": ")
			writeIndent(sb, x.vals[x.keys[i]], depth+1)
		})
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		writeBlock(sb, "[", "]", len(x), depth, func(i int) {
			writeIndent(sb, x[i], depth+1)
		})
	case []*obj:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		writeBlock(sb, "[", "]", len(x), depth, func(i int) {
			writeIndent(sb, x[i], depth+1)
		})
	case []string:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		writeBlock(sb, "[", "]", len(x), depth, func(i int) {
			sb.WriteString(jsonString(x[i]))
		})
	default:
		writeCompact(sb, v)
	}
}

// writeBlock emits a non-empty JSON array/object in indent=2 style:
// entries at depth+1 separated by ",\n", closed at depth.
func writeBlock(sb *strings.Builder, open, closeCh string, n int, depth int, entry func(i int)) {
	pad := strings.Repeat("  ", depth)
	padIn := strings.Repeat("  ", depth+1)
	sb.WriteString(open)
	sb.WriteString("\n")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(padIn)
		entry(i)
	}
	sb.WriteString("\n")
	sb.WriteString(pad)
	sb.WriteString(closeCh)
}

// jsonString renders s like Python json.dumps(s) with ensure_ascii=True.
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

// --- Python str()/repr() mirrors for TOML scalar values ----------------------

// pyStr mirrors Python str() for the value types internal/toml produces.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return formatPyFloat(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *obj:
		return pyRepr(v)
	default:
		return pyRepr(v)
	}
}

// pyRepr mirrors Python repr() for TOML value types.
func pyRepr(v any) string {
	switch x := v.(type) {
	case string:
		return pyReprString(x)
	case *obj:
		if x == nil {
			return "{}"
		}
		parts := make([]string, 0, len(x.keys))
		for _, k := range x.keys {
			parts = append(parts, pyRepr(k)+": "+pyRepr(x.vals[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return pyStr(v)
}

func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if rune(r) == rune(quote) {
				sb.WriteByte('\\')
			}
			sb.WriteRune(r)
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}

// formatPyFloat renders f like Python str(float) (shortest round-trip
// repr). Mirrors internal/config's unexported formatPyFloat.
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
	e := strconv.FormatFloat(a, 'e', -1, 64)
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1

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
