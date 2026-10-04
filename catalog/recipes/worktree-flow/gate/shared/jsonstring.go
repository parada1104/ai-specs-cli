package shared

import (
	"fmt"
	"strings"
)

// PyJSONString reproduces json.dumps(s) with ensure_ascii=True: named escapes
// for the CPython escape set, lowercase 4-digit hex for remaining control
// characters and everything >= 0x7f, surrogate pairs above the BMP.
//
// Moved verbatim from the gate main package (recipeconfigwrite.go) as part of
// slice SX0b: the lock writer shares this helper with the recipe-config
// writer and the copy-apply actuator. Single authority; the main-package copy
// was deleted with this move.
func PyJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || r >= 0x7f:
			if r > 0xFFFF {
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
