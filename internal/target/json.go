package target

// JSON emission mirroring Python json.dumps for the plan surface:
//   - compact mode (default separators ", " / ": ") for the error envelope
//   - indent=2 mode for the plan (json.dumps(plan, indent=2))
// with ensure_ascii=True escaping throughout. Ordered objects use the local
// obj type (internal/config's Obj is intentionally unexported and outside
// this package's allowed edit surface).

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"ai-specs.dev/ai-specs/internal/toml"
)

// pyReprMaxDepth pins the Python recursion boundary for repr()/str() of
// nested values. Probe (Python 3.14.7, default 8 MiB stack): repr() of a
// nested list succeeds to depth ~69709 and raises RecursionError at ~69710.
// The cap sits above that failure point so Go never errors where Python
// succeeds on the probe machine, and returns a descriptive error instead of
// crashing at extreme depths. Self-referential *toml.Table sharing (not
// producible by toml.Parse, which builds trees) renders Python's ellipsis
// form instead.
const pyReprMaxDepth = 100000

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
// A value json.dumps cannot serialize (a TOML date/time) is returned as the
// Python TypeError text.
func encodeCompact(v any) (string, error) {
	var sb strings.Builder
	if err := writeCompact(&sb, v); err != nil {
		return "", err
	}
	return sb.String(), nil
}

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
		// Python json.dumps renders an arbitrary-precision int in full.
		sb.WriteString(string(x))
	case toml.DateTime:
		return jsonUnserializableError{TypeName: x.PyTypeName()}
	case float64:
		sb.WriteString(formatPyFloat(x))
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
		// Unreachable: plan values are strings, bools, string lists, and obj.
		sb.WriteString("null")
	}
	return nil
}

// jsonUnserializableError is the Python TypeError json.dumps raises for a value
// it cannot serialize. The encoder mirrors Python's json.dumps, which raises
// for these types, so callers receive it as an ordinary error instead of a
// panic.
type jsonUnserializableError struct{ TypeName string }

func (e jsonUnserializableError) Error() string {
	return "Object of type " + e.TypeName + " is not JSON serializable"
}

// encodeIndent renders v like Python json.dumps(v, indent=2).
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
	case []*obj:
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
	default:
		return writeCompact(sb, v)
	}
}

// writeBlock emits a non-empty JSON array/object in indent=2 style:
// entries at depth+1 separated by ",\n", closed at depth. An error from an
// entry (an unserializable nested value) stops emission and is returned.
func writeBlock(sb *strings.Builder, open, closeCh string, n int, depth int, entry func(i int) error) error {
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
// Non-string composites go through repr composition (Python str(list) ==
// repr(list)); an error means the nesting exceeds pyReprMaxDepth where
// Python raises RecursionError.
func pyStr(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	if bi, ok := v.(toml.BigInt); ok {
		// Python str(int) for a wide integer is its exact decimal digits.
		return string(bi), nil
	}
	if dt, ok := v.(toml.DateTime); ok {
		// Python str(datetime/date/time) is the ISO form, not the repr.
		return dt.PyStr(), nil
	}
	return pyReprDepth(v, 0, nil)
}

// pyRepr mirrors Python repr() for TOML value types. An error means the
// nesting exceeds pyReprMaxDepth where Python raises RecursionError.
func pyRepr(v any) (string, error) {
	return pyReprDepth(v, 0, nil)
}

// pyReprDepth is the shared recursion core: depth-capped (descriptive error
// instead of a stack overflow) and cycle-aware for *toml.Table (Python's
// '{...}' ellipsis for a table re-entered on the active path).
func pyReprDepth(v any, depth int, active map[*toml.Table]bool) (string, error) {
	if depth > pyReprMaxDepth {
		return "", fmt.Errorf("value nesting exceeds the maximum recursion depth (%d)", pyReprMaxDepth)
	}
	switch x := v.(type) {
	case nil:
		return "None", nil
	case string:
		return pyReprString(x), nil
	case bool:
		if x {
			return "True", nil
		}
		return "False", nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case toml.BigInt:
		return string(x), nil
	case toml.DateTime:
		return x.PyRepr(), nil
	case float64:
		return formatPyFloat(x), nil
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			p, err := pyReprDepth(e, depth+1, active)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *toml.Table:
		if x == nil {
			return "{}", nil
		}
		if active[x] {
			return "{...}", nil
		}
		if active == nil {
			active = make(map[*toml.Table]bool)
		}
		active[x] = true
		defer delete(active, x)
		tableKeys := x.Keys()
		parts := make([]string, 0, len(tableKeys))
		for _, k := range tableKeys {
			val, _ := x.Get(k)
			p, err := pyReprDepth(val, depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case *obj:
		if x == nil {
			return "{}", nil
		}
		parts := make([]string, 0, len(x.keys))
		for _, k := range x.keys {
			p, err := pyReprDepth(x.vals[k], depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	default:
		return pyReprString(fmt.Sprintf("%v", v)), nil
	}
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
		case rune(quote):
			sb.WriteByte('\\')
			sb.WriteByte(quote)
		default:
			// Python repr() escapes every non-printable char (\xNN for
			// < 0x100, \uNNNN on the BMP, \UNNNNNNNN beyond): chars < 0x20
			// and 0x7f-0x9f always, plus non-printable Unicode (NBSP, U+2028);
			// printable non-ASCII like é and non-BMP emoji stay raw.
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || !unicode.IsPrint(r) {
				sb.WriteString(pyUnicodeEscape(r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}

// pyUnicodeEscape renders one non-printable rune in Python repr escape form
// (lowercase hex, \x for < 0x100, \u for the BMP, \U beyond).
func pyUnicodeEscape(r rune) string {
	const hex = "0123456789abcdef"
	digits := func(n int) string {
		out := make([]byte, n)
		for i := n - 1; i >= 0; i-- {
			out[i] = hex[r&0xf]
			r >>= 4
		}
		return string(out)
	}
	switch {
	case r < 0x100:
		return `\x` + digits(2)
	case r < 0x10000:
		return `\u` + digits(4)
	default:
		return `\U` + digits(8)
	}
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
