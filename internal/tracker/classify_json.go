package tracker

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"
)

type pyNumber struct{ truthy bool } // only truthiness is ever asked of a number

type pyParser struct {
	data []byte
	pos  int
}

var pyLiterals = []struct {
	lit string
	val any
}{
	{"null", nil}, {"true", true}, {"false", false},
	{"NaN", pyNumber{true}}, {"Infinity", pyNumber{true}}, {"-Infinity", pyNumber{true}},
}

// pyNumberRE is CPython's number grammar, anchored at the current position.
var pyNumberRE = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][-+]?[0-9]+)?`)

// decodePyJSON parses data the way CPython's json.loads(data, strict=True) does.
func decodePyJSON(data []byte) (any, error) {
	p := &pyParser{data: data}
	p.skipWS()
	if p.pos >= len(p.data) {
		return nil, fmt.Errorf("json: empty input")
	}
	value, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.pos < len(p.data) {
		return nil, fmt.Errorf("json: extra data at offset %d", p.pos)
	}
	return value, nil
}

func (p *pyParser) skipWS() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *pyParser) hasPrefix(s string) bool {
	return bytes.HasPrefix(p.data[p.pos:], []byte(s))
}

func (p *pyParser) consume(c byte) bool {
	if p.pos < len(p.data) && p.data[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

func (p *pyParser) value() (any, error) {
	p.skipWS()
	if p.pos >= len(p.data) {
		return nil, fmt.Errorf("json: unexpected end of input")
	}
	for _, l := range pyLiterals {
		if p.hasPrefix(l.lit) {
			p.pos += len(l.lit)
			return l.val, nil
		}
	}
	c := p.data[p.pos]
	switch {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		return p.str()
	case c == '-' || isDigit(c):
		return p.number()
	}
	return nil, fmt.Errorf("json: unexpected %q at offset %d", c, p.pos)
}

func (p *pyParser) object() (any, error) {
	p.pos++ // '{'
	object := make(map[string]any)
	p.skipWS()
	if p.consume('}') {
		return object, nil
	}
	for {
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, fmt.Errorf("json: object key must be a string at offset %d", p.pos)
		}
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if !p.consume(':') {
			return nil, fmt.Errorf("json: expected ':' at offset %d", p.pos)
		}
		item, err := p.value()
		if err != nil {
			return nil, err
		}
		object[key] = item // duplicate keys keep the last value, as in Python
		p.skipWS()
		if p.consume('}') {
			return object, nil
		}
		if !p.consume(',') {
			return nil, fmt.Errorf("json: expected ',' or '}' at offset %d", p.pos)
		}
	}
}

func (p *pyParser) array() (any, error) {
	p.pos++ // '['
	array := make([]any, 0)
	p.skipWS()
	if p.consume(']') {
		return array, nil
	}
	for {
		item, err := p.value()
		if err != nil {
			return nil, err
		}
		array = append(array, item)
		p.skipWS()
		if p.consume(']') {
			return array, nil
		}
		if !p.consume(',') {
			return nil, fmt.Errorf("json: expected ',' or ']' at offset %d", p.pos)
		}
	}
}

var pyEscapes = map[byte]byte{
	'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t',
}

func (p *pyParser) str() (string, error) {
	p.pos++ // opening '"'
	out := make([]byte, 0, 16)
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(out), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", fmt.Errorf("json: unterminated escape")
			}
			e := p.data[p.pos]
			p.pos++
			if e == 'u' {
				code, err := p.hex4()
				if err != nil {
					return "", err
				}
				out, err = p.appendUnicodeEscape(out, code)
				if err != nil {
					return "", err
				}
			} else if b, ok := pyEscapes[e]; ok {
				out = append(out, b)
			} else {
				return "", fmt.Errorf("json: invalid escape \\%c", e)
			}
		case c < 0x20:
			return "", fmt.Errorf("json: control character in string")
		default:
			out = append(out, c) // raw bytes kept verbatim, even invalid UTF-8
			p.pos++
		}
	}
	return "", fmt.Errorf("json: unterminated string")
}

// appendUnicodeEscape joins a valid surrogate pair and keeps any unpaired
// surrogate as WTF-8; a non-low second escape is then re-processed normally.
func (p *pyParser) appendUnicodeEscape(out []byte, code uint32) ([]byte, error) {
	for {
		if code >= 0xd800 && code <= 0xdbff && p.pos+1 < len(p.data) && p.data[p.pos] == '\\' && p.data[p.pos+1] == 'u' {
			p.pos += 2
			next, err := p.hex4()
			if err != nil {
				return nil, err
			}
			if next >= 0xdc00 && next <= 0xdfff {
				return utf8.AppendRune(out, rune(0x10000+((code-0xd800)<<10)+(next-0xdc00))), nil
			}
			out = appendLoneSurrogate(out, code)
			code = next
			continue
		}
		if code >= 0xd800 && code <= 0xdfff {
			return appendLoneSurrogate(out, code), nil
		}
		return utf8.AppendRune(out, rune(code)), nil
	}
}

func appendLoneSurrogate(out []byte, code uint32) []byte {
	return append(out, 0xe0|byte(code>>12), 0x80|byte((code>>6)&0x3f), 0x80|byte(code&0x3f))
}

func (p *pyParser) hex4() (uint32, error) {
	if p.pos+4 > len(p.data) {
		return 0, fmt.Errorf("json: truncated \\u escape")
	}
	n, err := strconv.ParseUint(string(p.data[p.pos:p.pos+4]), 16, 32)
	if err != nil {
		return 0, fmt.Errorf("json: invalid \\u hex digit")
	}
	p.pos += 4
	return uint32(n), nil
}

func (p *pyParser) number() (any, error) {
	literal := pyNumberRE.Find(p.data[p.pos:])
	if literal == nil {
		return nil, fmt.Errorf("json: invalid number at offset %d", p.pos)
	}
	p.pos += len(literal)
	if bytes.ContainsAny(literal, ".eE") {
		f, _ := strconv.ParseFloat(string(literal), 64) // range errors ignored on purpose
		return pyNumber{truthy: f != 0}, nil
	}
	if len(bytes.TrimPrefix(literal, []byte("-"))) > 4300 { // sign not counted
		return nil, fmt.Errorf("json: integer literal exceeds 4300 digits")
	}
	return pyNumber{truthy: bytes.IndexAny(literal, "123456789") >= 0}, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func pyTruthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return len(v) > 0
	case pyNumber:
		return v.truthy
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return false
}

func pyPrintable(s string) bool {
	return utf8.ValidString(s)
}
