// Package toml parses and serializes the TOML 1.0 subset used by ai-specs
// manifests, recipe manifests, and lock files.
//
// The parser mirrors python3 stdlib tomllib's observable value types for the
// subset documented in docs/go-adr/0002-toml-line-editor.md: tables and
// array-of-tables, dotted and quoted keys, basic / literal / multi-line
// strings, integers (dec/hex/oct/bin), floats, booleans, arrays, and inline
// tables. Datetime values are outside the subset and produce a descriptive
// error, matching the Python pipeline where json.dumps cannot serialize the
// datetime objects tomllib returns.
//
// This is a reader for parsed values only; manifest writes are surgical text
// operations owned by the line/segment editor (internal/config). Error text
// is diagnostic: it is deliberately precise but not byte-parity with
// tomllib's interpreter-specific messages.
package toml

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Table is an ordered TOML table. Keys iterate in document order (order of
// first appearance), mirroring tomllib's dict insertion order.
//
// Values are one of: string, int64, float64, bool, []any (array),
// *Table (table or inline table), or []*Table (array of tables).
type Table struct {
	order   []string
	entries map[string]any
	// inline marks tables created via dotted keys inside a key/value or by
	// an inline table literal; TOML forbids extending them via [header].
	inline bool
}

func newTable(inline bool) *Table {
	return &Table{entries: make(map[string]any), inline: inline}
}

// Keys returns the table's keys in document order.
func (t *Table) Keys() []string {
	keys := make([]string, len(t.order))
	copy(keys, t.order)
	return keys
}

// Get returns the raw value stored under key, if present.
func (t *Table) Get(key string) (any, bool) {
	v, ok := t.entries[key]
	return v, ok
}

// set stores a value, recording document order on first appearance.
func (t *Table) set(key string, v any) {
	if _, exists := t.entries[key]; !exists {
		t.order = append(t.order, key)
	}
	t.entries[key] = v
}

// String returns the string value under key.
func (t *Table) String(key string) (string, bool) {
	v, ok := t.entries[key]
	s, ok2 := v.(string)
	return s, ok && ok2
}

// Int64 returns the integer value under key.
func (t *Table) Int64(key string) (int64, bool) {
	v, ok := t.entries[key]
	n, ok2 := v.(int64)
	return n, ok && ok2
}

// Float64 returns the float value under key.
func (t *Table) Float64(key string) (float64, bool) {
	v, ok := t.entries[key]
	f, ok2 := v.(float64)
	return f, ok && ok2
}

// Bool returns the boolean value under key.
func (t *Table) Bool(key string) (bool, bool) {
	v, ok := t.entries[key]
	b, ok2 := v.(bool)
	return b, ok && ok2
}

// Array returns the array value under key (not array-of-tables; use Tables).
func (t *Table) Array(key string) ([]any, bool) {
	v, ok := t.entries[key]
	a, ok2 := v.([]any)
	return a, ok && ok2
}

// Table returns the sub-table under key.
func (t *Table) Table(key string) (*Table, bool) {
	v, ok := t.entries[key]
	sub, ok2 := v.(*Table)
	return sub, ok && ok2
}

// Tables returns the array-of-tables under key.
func (t *Table) Tables(key string) ([]*Table, bool) {
	v, ok := t.entries[key]
	a, ok2 := v.([]*Table)
	return a, ok && ok2
}

// Any returns the table as plain nested Go values (map[string]any, []any,
// and scalars), suitable for JSON marshaling and differential comparison.
func (t *Table) Any() map[string]any {
	out := make(map[string]any, len(t.entries))
	for k, v := range t.entries {
		out[k] = plainValue(v)
	}
	return out
}

func plainValue(v any) any {
	switch x := v.(type) {
	case *Table:
		return x.Any()
	case []*Table:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = e.Any()
		}
		return arr
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = plainValue(e)
		}
		return arr
	default:
		return v
	}
}

// Parse parses TOML data restricted to the manifest subset and returns the
// root table. Input must be valid UTF-8; a trailing newline is optional and
// CRLF line endings are tolerated.
func Parse(data []byte) (*Table, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("toml: input is not valid UTF-8")
	}
	// Tolerate CRLF: every bare \r must be part of a \r\n pair.
	for i, b := range data {
		if b == '\r' && (i+1 >= len(data) || data[i+1] != '\n') {
			return nil, fmt.Errorf("toml: line %d: bare CR is not a line ending", countLines(data, i))
		}
	}
	p := &parser{
		data:     data,
		line:     1,
		root:     newTable(false),
		explicit: make(map[string]bool),
	}
	p.cur = p.root
	if err := p.parseDocument(); err != nil {
		return nil, err
	}
	return p.root, nil
}

func countLines(data []byte, offset int) int {
	return strings.Count(string(data[:offset]), "\n") + 1
}

type parser struct {
	data []byte
	pos  int
	line int
	root *Table
	cur  *Table
	// explicit records full paths declared via [table] headers, for
	// redefinition detection ("\x00"-joined segments to avoid ambiguity).
	explicit map[string]bool
}

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("toml: line %d: %s", p.line, fmt.Sprintf(format, args...))
}

func (p *parser) peek() byte {
	if p.pos >= len(p.data) {
		return 0
	}
	return p.data[p.pos]
}

func (p *parser) hasPrefix(s string) bool {
	return p.pos+len(s) <= len(p.data) && string(p.data[p.pos:p.pos+len(s)]) == s
}

// skipSpaces consumes spaces and tabs.
func (p *parser) skipSpaces() {
	for p.pos < len(p.data) && (p.data[p.pos] == ' ' || p.data[p.pos] == '\t') {
		p.pos++
	}
}

// skipComment consumes a '#' comment up to (not including) the newline.
func (p *parser) skipComment() {
	if p.peek() == '#' {
		for p.pos < len(p.data) && p.data[p.pos] != '\n' {
			p.pos++
		}
	}
}

// skipBlank consumes whitespace, comments, and newlines between statements.
func (p *parser) skipBlank() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		switch {
		case c == ' ' || c == '\t':
			p.pos++
		case c == '\n':
			p.line++
			p.pos++
		case c == '#':
			p.skipComment()
		default:
			return
		}
	}
}

// expectLineEnd consumes optional trailing whitespace/comment and the line
// ending (or EOF).
func (p *parser) expectLineEnd() error {
	p.skipSpaces()
	p.skipComment()
	if p.pos >= len(p.data) {
		return nil
	}
	if p.data[p.pos] == '\n' {
		p.line++
		p.pos++
		return nil
	}
	return p.errf("expected end of line, found %q", string(p.data[p.pos]))
}

func (p *parser) parseDocument() error {
	for {
		p.skipBlank()
		if p.pos >= len(p.data) {
			return nil
		}
		if p.data[p.pos] == '[' {
			if err := p.parseTableHeader(); err != nil {
				return err
			}
			continue
		}
		if err := p.parseKeyValue(p.cur); err != nil {
			return err
		}
		if err := p.expectLineEnd(); err != nil {
			return err
		}
	}
}

// isBareKeyChar reports whether b may appear in a bare key.
func isBareKeyChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

// parseKeyPath parses a (possibly dotted) key: bare, basic-quoted, or
// literal-quoted segments separated by '.'.
func (p *parser) parseKeyPath() ([]string, error) {
	var keys []string
	for {
		p.skipSpaces()
		switch {
		case p.peek() == '"':
			s, err := p.parseBasicString()
			if err != nil {
				return nil, err
			}
			keys = append(keys, s)
		case p.peek() == '\'':
			s, err := p.parseLiteralString()
			if err != nil {
				return nil, err
			}
			keys = append(keys, s)
		default:
			start := p.pos
			for p.pos < len(p.data) && isBareKeyChar(p.data[p.pos]) {
				p.pos++
			}
			if p.pos == start {
				return nil, p.errf("expected key, found %q", string(p.peek()))
			}
			keys = append(keys, string(p.data[start:p.pos]))
		}
		p.skipSpaces()
		if p.peek() == '.' {
			p.pos++
			continue
		}
		return keys, nil
	}
}

// parseKeyValue parses `key[.subkey...] = value` into tbl.
func (p *parser) parseKeyValue(tbl *Table) error {
	keys, err := p.parseKeyPath()
	if err != nil {
		return err
	}
	p.skipSpaces()
	if p.peek() != '=' {
		return p.errf("expected '=' after key %q", keys[len(keys)-1])
	}
	p.pos++
	p.skipSpaces()
	v, err := p.parseValue()
	if err != nil {
		return err
	}

	cur := tbl
	for i := 0; i < len(keys)-1; i++ {
		e, ok := cur.entries[keys[i]]
		if !ok {
			nt := newTable(true)
			cur.set(keys[i], nt)
			cur = nt
			continue
		}
		switch tv := e.(type) {
		case *Table:
			cur = tv
		case []*Table:
			cur = tv[len(tv)-1]
		default:
			return p.errf("cannot redefine key %q as a table", keys[i])
		}
	}
	last := keys[len(keys)-1]
	if _, exists := cur.entries[last]; exists {
		return p.errf("duplicate key %q", last)
	}
	cur.set(last, v)
	return nil
}

// parseTableHeader parses [table] and [[array-of-tables]] headers.
func (p *parser) parseTableHeader() error {
	p.pos++ // '['
	aot := p.peek() == '['
	if aot {
		p.pos++
	}
	keys, err := p.parseKeyPath()
	if err != nil {
		return err
	}
	p.skipSpaces()
	if p.peek() != ']' {
		return p.errf("expected ']' to close table header")
	}
	p.pos++
	if aot {
		if p.peek() != ']' {
			return p.errf("expected ']]' to close array-of-tables header")
		}
		p.pos++
	}
	if err := p.expectLineEnd(); err != nil {
		return err
	}

	pathKey := strings.Join(keys, "\x00")
	cur := p.root
	for i := 0; i < len(keys)-1; i++ {
		if cur, err = p.descendHeader(cur, keys[i]); err != nil {
			return err
		}
	}
	last := keys[len(keys)-1]
	e, exists := cur.entries[last]

	if aot {
		if !exists {
			t := newTable(false)
			cur.set(last, []*Table{t})
			p.cur = t
			return nil
		}
		arr, ok := e.([]*Table)
		if !ok {
			return p.errf("cannot redefine %q as an array of tables", last)
		}
		t := newTable(false)
		cur.entries[last] = append(arr, t)
		p.cur = t
		return nil
	}

	if !exists {
		t := newTable(false)
		cur.set(last, t)
		p.explicit[pathKey] = true
		p.cur = t
		return nil
	}
	t, ok := e.(*Table)
	if !ok {
		return p.errf("cannot redefine non-table key %q", last)
	}
	if t.inline {
		return p.errf("cannot redefine table %q defined via dotted keys", last)
	}
	if p.explicit[pathKey] {
		return p.errf("table %q redefined", last)
	}
	p.explicit[pathKey] = true
	p.cur = t
	return nil
}

// descendHeader resolves one mid-path header segment, creating implicit
// tables as needed. References to an array-of-tables resolve to its last
// element.
func (p *parser) descendHeader(cur *Table, key string) (*Table, error) {
	e, ok := cur.entries[key]
	if !ok {
		t := newTable(false)
		cur.set(key, t)
		return t, nil
	}
	switch tv := e.(type) {
	case *Table:
		if tv.inline {
			return nil, p.errf("cannot extend table %q defined via dotted keys with a header", key)
		}
		return tv, nil
	case []*Table:
		return tv[len(tv)-1], nil
	default:
		return nil, p.errf("cannot redefine non-table key %q", key)
	}
}
