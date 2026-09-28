package toml

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// TOMLValue serializes a Go value to a valid TOML inline literal, the exact
// port of lib/_internal/toml_write.py:toml_value.
//
//	bool            -> true / false (checked before numbers, mirroring the
//	                   Python bool-is-int subtlety)
//	int / int64 ... -> decimal via strconv (Python str(int))
//	float64         -> Python str(float) formatting (repr): "0.5", "100.0",
//	                   "5e+22", "1e-05", "inf", "nan"
//	string          -> JSON-style basic string, identical to Python
//	                   json.dumps(v) defaults (ensure_ascii=True: non-ASCII
//	                   becomes \uXXXX, lowercase hex, surrogate pairs)
//	[]any / []string -> "[" + ", "-joined elements + "]"
//	map[string]any  -> "{ k = v, ... }" with keys in sorted order
//	                   (deterministic; Python dicts preserve insertion
//	                   order, so order-sensitive writers use
//	                   TOMLValueOrdered)
//
// Anything else raises an error naming the Go type, mirroring the Python
// TypeError("cannot serialize ... to TOML") so malformed defaults fail
// loudly at the write site.
func TOMLValue(v any) (string, error) {
	switch x := v.(type) {
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float64:
		return formatPyFloat(x), nil
	case string:
		return quoteJSONString(x), nil
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			s, err := TOMLValue(e)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case []string:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = quoteJSONString(e)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		return dictLiteral(sortedKeys(x), x)
	default:
		return "", fmt.Errorf("cannot serialize %T to TOML", v)
	}
}

// TOMLValueOrdered serializes a map as an inline table with the given key
// order, for call sites that mirror Python dict insertion order (e.g.
// recipe-config-write's inline table blocks). Every key in keys must exist
// in m; keys of m not listed are ignored. Nested maps still serialize in
// sorted order.
func TOMLValueOrdered(keys []string, m map[string]any) (string, error) {
	return dictLiteral(keys, m)
}

func dictLiteral(keys []string, m map[string]any) (string, error) {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		val, ok := m[k]
		if !ok {
			return "", fmt.Errorf("cannot serialize to TOML: missing key %q", k)
		}
		s, err := TOMLValue(val)
		if err != nil {
			return "", err
		}
		parts = append(parts, k+" = "+s)
	}
	return "{ " + strings.Join(parts, ", ") + " }", nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	// Small insertion sort keeps the zero-dependency surface tiny; stdlib
	// sort is also fine, but this avoids importing it for one call site.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// quoteJSONString renders s exactly like Python json.dumps(s) with defaults:
// ensure_ascii=True escaping (\uXXXX, lowercase hex, surrogate pairs for
// astral characters), \b \t \n \f \r short escapes, and \" \\ escaping.
func quoteJSONString(s string) string {
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
					fmt.Fprintf(&sb, `\u%04x`, u)
				}
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
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
