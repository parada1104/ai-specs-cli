package rulesaudit

// JSON emission mirroring Python json.dumps(payload, indent=2) with the
// default ensure_ascii=True escaping. encoding/json sorts map keys and emits
// raw UTF-8, so the ordered obj type and a hand-rolled encoder are required
// for byte identity with the legacy rules-inventory.py output.

import (
	"strconv"
	"strings"
	"unicode/utf16"
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
		writeBlock(sb, "[", "]", len(x), depth, func(i int) { writeIndent(sb, x[i], depth+1) })
	case []string:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		writeBlock(sb, "[", "]", len(x), depth, func(i int) { sb.WriteString(jsonString(x[i])) })
	case []*obj:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		writeBlock(sb, "[", "]", len(x), depth, func(i int) { writeIndent(sb, x[i], depth+1) })
	default:
		writeCompact(sb, v)
	}
}

// writeBlock emits a non-empty JSON array/object in indent=2 style: entries at
// depth+1 separated by ",\n", closed at depth.
func writeBlock(sb *strings.Builder, open, closeCh string, n, depth int, entry func(i int)) {
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

// writeCompact renders a scalar (or a *obj reached through an []any slot) the
// way Python json.dumps would inline it.
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
		sb.WriteString("null")
	}
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
