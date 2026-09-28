package toml

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseValue parses a single TOML value.
func (p *parser) parseValue() (any, error) {
	if p.pos >= len(p.data) {
		return nil, p.errf("expected a value")
	}
	switch c := p.peek(); {
	case c == '"':
		if p.hasPrefix(`"""`) {
			return p.parseMultilineBasic()
		}
		return p.parseBasicString()
	case c == '\'':
		if p.hasPrefix("'''") {
			return p.parseMultilineLiteral()
		}
		return p.parseLiteralString()
	case c == '[':
		return p.parseArray()
	case c == '{':
		return p.parseInlineTable()
	case c == 't':
		if p.hasPrefix("true") && p.valueDelimited(4) {
			p.pos += 4
			return true, nil
		}
	case c == 'f':
		if p.hasPrefix("false") && p.valueDelimited(5) {
			p.pos += 5
			return false, nil
		}
	}
	return p.parseNumberOrToken()
}

// valueDelimited reports whether the byte offset n past the current position
// ends the token (whitespace, delimiter, comment, line end, or EOF).
func (p *parser) valueDelimited(n int) bool {
	j := p.pos + n
	if j >= len(p.data) {
		return true
	}
	c := p.data[j]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == ']' || c == '}' || c == '#'
}

// parseNumberOrToken scans a bare token and classifies it as integer, float,
// or — outside the supported subset — datetime.
func (p *parser) parseNumberOrToken() (any, error) {
	start := p.pos
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c == '_' || c == '+' || c == '-' || c == '.' || c == ':' {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return nil, p.errf("expected a value, found %q", string(p.peek()))
	}
	tok := string(p.data[start:p.pos])

	// Datetimes are out of the subset: date, time, and offset-date-time.
	if len(tok) >= 7 && isDigits(tok[0:4]) && tok[4] == '-' ||
		len(tok) >= 3 && isDigits(tok[0:2]) && tok[2] == ':' {
		return nil, p.errf("datetime values (%q) are not supported", tok)
	}

	neg := false
	body := tok
	switch body[0] {
	case '+':
		body = body[1:]
	case '-':
		neg = true
		body = body[1:]
	}
	if body == "" {
		return nil, p.errf("invalid value %q", tok)
	}
	switch body {
	case "inf":
		if neg {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	case "nan":
		return math.NaN(), nil
	}

	switch {
	case strings.HasPrefix(body, "0x"):
		return p.parseIntBase(tok, neg, body[2:], 16)
	case strings.HasPrefix(body, "0o"):
		return p.parseIntBase(tok, neg, body[2:], 8)
	case strings.HasPrefix(body, "0b"):
		return p.parseIntBase(tok, neg, body[2:], 2)
	}

	if strings.ContainsAny(body, ".eE") {
		return p.parseFloat(tok, neg, body)
	}
	return p.parseIntDec(tok, neg, body)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// validateUnderscores enforces TOML's rule: underscores appear only between
// alphanumeric characters.
func validateUnderscores(s string) error {
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			continue
		}
		if i == 0 || i == len(s)-1 ||
			!isAlnum(s[i-1]) || !isAlnum(s[i+1]) {
			return fmt.Errorf("misplaced underscore")
		}
	}
	return nil
}

func isAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func (p *parser) parseIntBase(tok string, neg bool, body string, base int) (any, error) {
	if err := validateUnderscores(body); err != nil {
		return nil, p.errf("invalid integer %q: %v", tok, err)
	}
	digits := strings.ReplaceAll(body, "_", "")
	if digits == "" {
		return nil, p.errf("invalid integer %q", tok)
	}
	n, err := strconv.ParseInt(digits, base, 64)
	if err != nil {
		return nil, p.errf("invalid integer %q", tok)
	}
	if neg {
		n = -n
	}
	return n, nil
}

func (p *parser) parseIntDec(tok string, neg bool, body string) (any, error) {
	if err := validateUnderscores(body); err != nil {
		return nil, p.errf("invalid integer %q: %v", tok, err)
	}
	digits := strings.ReplaceAll(body, "_", "")
	if !isDigits(digits) {
		return nil, p.errf("invalid integer %q", tok)
	}
	if len(digits) > 1 && digits[0] == '0' {
		return nil, p.errf("invalid integer %q: leading zeros are not allowed", tok)
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return nil, p.errf("invalid integer %q", tok)
	}
	if neg {
		n = -n
	}
	return n, nil
}

func (p *parser) parseFloat(tok string, neg bool, body string) (any, error) {
	if err := validateUnderscores(body); err != nil {
		return nil, p.errf("invalid float %q: %v", tok, err)
	}
	s := strings.ReplaceAll(body, "_", "")
	if !validFloatShape(s) {
		return nil, p.errf("invalid float %q", tok)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, p.errf("invalid float %q", tok)
	}
	if neg {
		f = -f
	}
	return f, nil
}

// validFloatShape enforces TOML's float grammar:
// int-part [ frac [ exp ] | exp ], with digits required on both sides of '.'
// and after the exponent marker.
func validFloatShape(s string) bool {
	i := 0
	if !consumeDigits(s, &i, 1) {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !consumeDigits(s, &i, 1) {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if !consumeDigits(s, &i, 1) {
			return false
		}
	}
	return i == len(s)
}

func consumeDigits(s string, i *int, min int) bool {
	start := *i
	for *i < len(s) && s[*i] >= '0' && s[*i] <= '9' {
		*i++
	}
	return *i-start >= min
}

// parseBasicString parses "..." with escapes; no raw newlines allowed.
func (p *parser) parseBasicString() (string, error) {
	p.pos++ // opening quote
	var sb strings.Builder
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated string")
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return sb.String(), nil
		case c == '\n':
			return "", p.errf("unterminated string (newline in basic string)")
		case c == '\\':
			if err := p.parseEscape(&sb); err != nil {
				return "", err
			}
		default:
			sb.WriteByte(c)
			p.pos++
		}
	}
}

// parseEscape handles one backslash escape, writing to sb.
func (p *parser) parseEscape(sb *strings.Builder) error {
	p.pos++ // backslash
	if p.pos >= len(p.data) {
		return p.errf("unterminated string (dangling escape)")
	}
	c := p.data[p.pos]
	p.pos++
	switch c {
	case 'b':
		sb.WriteByte('\b')
	case 't':
		sb.WriteByte('\t')
	case 'n':
		sb.WriteByte('\n')
	case 'f':
		sb.WriteByte('\f')
	case 'r':
		sb.WriteByte('\r')
	case '"':
		sb.WriteByte('"')
	case '\\':
		sb.WriteByte('\\')
	case 'u':
		return p.parseHexRune(sb, 4)
	case 'U':
		return p.parseHexRune(sb, 8)
	default:
		return p.errf("invalid escape sequence \\%s", string(c))
	}
	return nil
}

func (p *parser) parseHexRune(sb *strings.Builder, n int) error {
	if p.pos+n > len(p.data) {
		return p.errf("invalid escape: truncated \\%c escape", map[int]byte{4: 'u', 8: 'U'}[n])
	}
	hex := string(p.data[p.pos : p.pos+n])
	code, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return p.errf("invalid escape: \\%c%s is not valid hex", map[int]byte{4: 'u', 8: 'U'}[n], hex)
	}
	p.pos += n
	if code > 0x10FFFF || code >= 0xD800 && code <= 0xDFFF {
		return p.errf("invalid escape: \\%c%s is not a Unicode scalar value", map[int]byte{4: 'u', 8: 'U'}[n], hex)
	}
	sb.WriteRune(rune(code))
	return nil
}

// parseMultilineBasic parses """...""" with leading-newline trim and
// line-ending backslash continuation.
func (p *parser) parseMultilineBasic() (string, error) {
	p.pos += 3
	p.trimLeadingNewline()
	var sb strings.Builder
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated multi-line string")
		}
		if p.hasPrefix(`"""`) {
			run := 0
			for p.pos+run < len(p.data) && p.data[p.pos+run] == '"' {
				run++
			}
			if run > 3 {
				sb.WriteString(strings.Repeat(`"`, run-3))
			}
			p.pos += run
			return sb.String(), nil
		}
		if p.data[p.pos] == '\\' && p.isLineEndingBackslash() {
			p.pos++ // the backslash itself
			// Trim all whitespace (including newlines) after the backslash.
			for p.pos < len(p.data) {
				c := p.data[p.pos]
				if c == ' ' || c == '\t' || c == '\r' {
					p.pos++
					continue
				}
				if c == '\n' {
					p.line++
					p.pos++
					continue
				}
				break
			}
			continue
		}
		if p.data[p.pos] == '\\' {
			if err := p.parseEscape(&sb); err != nil {
				return "", err
			}
			continue
		}
		if p.data[p.pos] == '\n' {
			p.line++
		}
		sb.WriteByte(p.data[p.pos])
		p.pos++
	}
}

// isLineEndingBackslash reports whether the backslash at p.pos is followed
// by only whitespace until a newline (TOML line-continuation).
func (p *parser) isLineEndingBackslash() bool {
	j := p.pos + 1
	for j < len(p.data) && (p.data[j] == ' ' || p.data[j] == '\t') {
		j++
	}
	if j >= len(p.data) {
		return false
	}
	return p.data[j] == '\n' || p.data[j] == '\r'
}

func (p *parser) trimLeadingNewline() {
	if p.hasPrefix("\r\n") {
		p.pos += 2
		p.line++
		return
	}
	if p.peek() == '\n' {
		p.pos++
		p.line++
	}
}

// parseLiteralString parses '...' with no escapes; no raw newlines allowed.
func (p *parser) parseLiteralString() (string, error) {
	p.pos++ // opening quote
	start := p.pos
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated string")
		}
		c := p.data[p.pos]
		if c == '\'' {
			s := string(p.data[start:p.pos])
			p.pos++
			return s, nil
		}
		if c == '\n' {
			return "", p.errf("unterminated string (newline in literal string)")
		}
		p.pos++
	}
}

// parseMultilineLiteral parses '''...''' with no escapes.
func (p *parser) parseMultilineLiteral() (string, error) {
	p.pos += 3
	p.trimLeadingNewline()
	var sb strings.Builder
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated multi-line string")
		}
		if p.hasPrefix("'''") {
			run := 0
			for p.pos+run < len(p.data) && p.data[p.pos+run] == '\'' {
				run++
			}
			if run > 3 {
				sb.WriteString(strings.Repeat("'", run-3))
			}
			p.pos += run
			return sb.String(), nil
		}
		if p.data[p.pos] == '\n' {
			p.line++
		}
		sb.WriteByte(p.data[p.pos])
		p.pos++
	}
}

// parseArray parses [ ... ] with multi-line content, comments, and a
// trailing comma allowed.
func (p *parser) parseArray() (any, error) {
	p.pos++ // '['
	arr := []any{}
	for {
		p.skipBlank()
		if p.pos >= len(p.data) {
			return nil, p.errf("unterminated array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		p.skipBlank()
		if p.pos >= len(p.data) {
			return nil, p.errf("unterminated array")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return arr, nil
		default:
			return nil, p.errf("expected ',' or ']' in array, found %q", string(p.data[p.pos]))
		}
	}
}

// parseInlineTable parses { k = v, ... } (single line, TOML 1.0).
func (p *parser) parseInlineTable() (any, error) {
	p.pos++ // '{'
	tbl := newTable(true)
	p.skipSpaces()
	if p.peek() == '}' {
		p.pos++
		return tbl, nil
	}
	for {
		if err := p.parseKeyValue(tbl); err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.data) {
			return nil, p.errf("unterminated inline table")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return tbl, nil
		default:
			return nil, p.errf("expected ',' or '}' in inline table, found %q", string(p.data[p.pos]))
		}
	}
}
