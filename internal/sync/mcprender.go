package sync

// Byte-exact Go port of lib/_internal/mcp-render.py (GO-07.S4).
//
// Scope: library only. The Bash spine keeps shelling out to mcp-render.py until
// the sync-agent fan-out moves to Go in S15 (same treatment as S2's
// gitignore-render.py); there is deliberately no GO_SYNC_STEP_* flag yet.
//
// The port preserves the script's merge rules and output, except that invalid
// JSON targets are backed up and reported before being regenerated.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/toml"
)

// ---------------------------------------------------------------------------
// Insertion-ordered data model (mirrors a Python dict, not a Go map)
// ---------------------------------------------------------------------------

// mcpObj is an insertion-ordered object mirroring a Python dict: first
// appearance wins the position, last assignment wins the value (json.loads
// duplicate-key semantics).
type mcpObj struct {
	keys []string
	vals map[string]any
}

func newMCPObj() *mcpObj { return &mcpObj{vals: map[string]any{}} }

func (o *mcpObj) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *mcpObj) get(k string) (any, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *mcpObj) len() int { return len(o.keys) }

func (o *mcpObj) has(k string) bool {
	_, ok := o.vals[k]
	return ok
}

// del removes k, keeping the remaining keys in order.
func (o *mcpObj) del(k string) bool {
	if _, ok := o.vals[k]; !ok {
		return false
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
	return true
}

func mcpShallowCopy(o *mcpObj) *mcpObj {
	out := &mcpObj{keys: append([]string(nil), o.keys...), vals: make(map[string]any, len(o.vals))}
	for k, v := range o.vals {
		out.vals[k] = v
	}
	return out
}

// mcpToOrdered converts internal/toml values (ordered *toml.Table) into the
// ordered model, recursively.
func mcpToOrdered(v any) any {
	switch x := v.(type) {
	case *toml.Table:
		if x == nil {
			return newMCPObj()
		}
		out := newMCPObj()
		for _, k := range x.Keys() {
			vv, _ := x.Get(k)
			out.set(k, mcpToOrdered(vv))
		}
		return out
	case []*toml.Table:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = mcpToOrdered(e)
		}
		return arr
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = mcpToOrdered(e)
		}
		return arr
	case toml.BigInt:
		// internal/toml.BigInt and the JSON reader's mcpBigInt represent the
		// same concept — an integer wider than int64 kept as canonical decimal
		// digits — for the two different readers; unify them at this seam so
		// the ordered model has one wide-integer type.
		return mcpBigInt(x)
	case toml.DateTime:
		// A date/time is not a JSON value; keep it typed so the JSON writer can
		// raise Python's TypeError instead of silently emitting null.
		return x
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// JSON: ordered decode + json.dumps(indent=2) encode
// ---------------------------------------------------------------------------

// mcpBigInt is an integer literal that overflows int64, kept as its canonical
// decimal digits so it round-trips exactly like a Python int (arbitrary
// precision) instead of degrading to an approximate float64.
type mcpBigInt string

// mcpJSONMaxDepth bounds the decoder's nesting depth. Python's json.loads
// raises RecursionError at a stack- and version-dependent depth (100000
// accepted, 200000 rejected on CPython 3.14); Go's unbounded recursion would
// instead die with a fatal stack overflow (rc 2), losing the port's rc 1.
const mcpJSONMaxDepth = 100000

// errMCPJSONTooDeep distinguishes depth overflow from a syntax error when the
// caller decides whether read-only recipe and hook inputs may be ignored.
var errMCPJSONTooDeep = errors.New("json: maximum recursion depth exceeded")

// mcpParseJSONOrdered mirrors json.loads with its defaults: insertion-ordered
// objects (duplicate keys keep the first position and the last value), int64 for
// integer literals (mcpBigInt when wider than int64), float64 otherwise, the
// bare NaN / Infinity / -Infinity constants, and \uXXXX lone-surrogate escapes
// kept verbatim (a valid surrogate pair still combines). Any structural error
// (including trailing data or invalid UTF-8) is returned to the caller.
//
// encoding/json cannot express the last two, so this is a hand-written
// recursive-descent decoder. Python's json scanner accepts exactly the
// whitespace set ' ', '\t', '\n', '\r'.
func mcpParseJSONOrdered(data []byte) (any, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("json: invalid UTF-8")
	}
	d := &mcpJSONDecoder{s: string(data)}
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	d.ws()
	if d.pos != len(d.s) {
		return nil, fmt.Errorf("json: trailing data")
	}
	return v, nil
}

type mcpJSONDecoder struct {
	s     string
	pos   int
	depth int
}

// nested runs one object()/array() recursion level under the depth bound.
func (d *mcpJSONDecoder) nested(parse func() (any, error)) (any, error) {
	d.depth++
	if d.depth > mcpJSONMaxDepth {
		return nil, errMCPJSONTooDeep
	}
	v, err := parse()
	d.depth--
	return v, err
}

func (d *mcpJSONDecoder) ws() {
	for d.pos < len(d.s) && strings.IndexByte(" \t\n\r", d.s[d.pos]) >= 0 {
		d.pos++
	}
}

// mcpJSONLiterals are the bare literals json.loads accepts beyond the number
// grammar: NaN / ±Infinity arrive through parse_constant.
var mcpJSONLiterals = map[byte]struct {
	lit string
	val any
}{
	't': {"true", true},
	'f': {"false", false},
	'n': {"null", nil},
	'N': {"NaN", math.NaN()},
	'I': {"Infinity", math.Inf(1)},
}

func (d *mcpJSONDecoder) value() (any, error) {
	d.ws()
	if d.pos >= len(d.s) {
		return nil, fmt.Errorf("json: unexpected end of input")
	}
	c := d.s[d.pos]
	if l, ok := mcpJSONLiterals[c]; ok {
		return d.literal(l.lit, l.val)
	}
	switch {
	case c == '{':
		return d.nested(d.object)
	case c == '[':
		return d.nested(d.array)
	case c == '"':
		s, err := d.str()
		if err != nil {
			return nil, err
		}
		return s, nil
	case c == '-' || (c >= '0' && c <= '9'):
		return d.number()
	}
	return nil, fmt.Errorf("json: unexpected character %q", c)
}

func (d *mcpJSONDecoder) literal(lit string, v any) (any, error) {
	if !strings.HasPrefix(d.s[d.pos:], lit) {
		return nil, fmt.Errorf("json: invalid literal")
	}
	d.pos += len(lit)
	return v, nil
}

func (d *mcpJSONDecoder) object() (any, error) {
	d.pos++ // '{'
	obj := newMCPObj()
	d.ws()
	if d.pos < len(d.s) && d.s[d.pos] == '}' {
		d.pos++
		return obj, nil
	}
	for {
		d.ws()
		if d.pos >= len(d.s) || d.s[d.pos] != '"' {
			return nil, fmt.Errorf("json: expected object key")
		}
		k, err := d.str()
		if err != nil {
			return nil, err
		}
		d.ws()
		if d.pos >= len(d.s) || d.s[d.pos] != ':' {
			return nil, fmt.Errorf("json: expected ':'")
		}
		d.pos++
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		obj.set(k, v)
		d.ws()
		if d.pos >= len(d.s) {
			return nil, fmt.Errorf("json: unexpected end of input")
		}
		switch d.s[d.pos] {
		case ',':
			d.pos++
		case '}':
			d.pos++
			return obj, nil
		default:
			return nil, fmt.Errorf("json: expected ',' or '}'")
		}
	}
}

func (d *mcpJSONDecoder) array() (any, error) {
	d.pos++ // '['
	arr := []any{}
	d.ws()
	if d.pos < len(d.s) && d.s[d.pos] == ']' {
		d.pos++
		return arr, nil
	}
	for {
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		d.ws()
		if d.pos >= len(d.s) {
			return nil, fmt.Errorf("json: unexpected end of input")
		}
		switch d.s[d.pos] {
		case ',':
			d.pos++
		case ']':
			d.pos++
			return arr, nil
		default:
			return nil, fmt.Errorf("json: expected ',' or ']'")
		}
	}
}

func (d *mcpJSONDecoder) str() (string, error) {
	d.pos++ // '"'
	var sb strings.Builder
	for {
		if d.pos >= len(d.s) {
			return "", fmt.Errorf("json: unterminated string")
		}
		c := d.s[d.pos]
		switch {
		case c == '"':
			d.pos++
			return sb.String(), nil
		case c == '\\':
			if err := d.escape(&sb); err != nil {
				return "", err
			}
		case c < 0x20:
			// json.loads(strict=True) rejects raw control characters.
			return "", fmt.Errorf("json: invalid control character")
		case c < 0x80:
			sb.WriteByte(c)
			d.pos++
		default:
			r, size := utf8.DecodeRuneInString(d.s[d.pos:])
			if r == utf8.RuneError && size == 1 {
				return "", fmt.Errorf("json: invalid UTF-8")
			}
			sb.WriteString(d.s[d.pos : d.pos+size])
			d.pos += size
		}
	}
}

func (d *mcpJSONDecoder) escape(sb *strings.Builder) error {
	d.pos++ // '\\'
	if d.pos >= len(d.s) {
		return fmt.Errorf("json: unterminated escape")
	}
	e := d.s[d.pos]
	d.pos++
	switch e {
	case '"', '\\', '/':
		sb.WriteByte(e)
	case 'b':
		sb.WriteByte('\b')
	case 'f':
		sb.WriteByte('\f')
	case 'n':
		sb.WriteByte('\n')
	case 'r':
		sb.WriteByte('\r')
	case 't':
		sb.WriteByte('\t')
	case 'u':
		r, err := d.hex4()
		if err != nil {
			return err
		}
		if !mcpIsSurrogate(r) {
			sb.WriteRune(r)
			return nil
		}
		// A high surrogate followed by a low-surrogate escape is one astral
		// character; any other lone surrogate stays a lone surrogate (WTF-8)
		// so mcpJSONString re-emits it as \uXXXX like json.dumps.
		if d.pos+1 < len(d.s) && d.s[d.pos] == '\\' && d.s[d.pos+1] == 'u' {
			save := d.pos
			d.pos += 2
			r2, err := d.hex4()
			if err != nil {
				return err
			}
			if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
				sb.WriteRune(dec)
				return nil
			}
			d.pos = save
		}
		sb.WriteString(mcpWTF8Surrogate(r))
	default:
		return fmt.Errorf("json: invalid escape")
	}
	return nil
}

func (d *mcpJSONDecoder) hex4() (rune, error) {
	if d.pos+4 > len(d.s) {
		return 0, fmt.Errorf("json: invalid \\u escape")
	}
	var r rune
	for i := 0; i < 4; i++ {
		v, ok := mcpHexVal(d.s[d.pos+i])
		if !ok {
			return 0, fmt.Errorf("json: invalid \\u escape")
		}
		r = r<<4 | rune(v)
	}
	d.pos += 4
	return r, nil
}

func mcpHexVal(c byte) (byte, bool) {
	if c >= '0' && c <= '9' {
		return c - '0', true
	}
	if lc := c | 0x20; lc >= 'a' && lc <= 'f' {
		return lc - 'a' + 10, true
	}
	return 0, false
}

func (d *mcpJSONDecoder) number() (any, error) {
	start := d.pos
	if d.s[d.pos] == '-' {
		d.pos++
		if strings.HasPrefix(d.s[d.pos:], "Infinity") {
			d.pos += len("Infinity")
			return math.Inf(-1), nil
		}
	}
	if d.pos >= len(d.s) || d.s[d.pos] < '0' || d.s[d.pos] > '9' {
		return nil, fmt.Errorf("json: invalid number")
	}
	if d.s[d.pos] == '0' {
		d.pos++
	} else {
		for d.pos < len(d.s) && d.s[d.pos] >= '0' && d.s[d.pos] <= '9' {
			d.pos++
		}
	}
	isFloat := false
	if d.pos < len(d.s) && d.s[d.pos] == '.' {
		isFloat = true
		d.pos++
		if err := d.digits(); err != nil {
			return nil, err
		}
	}
	if d.pos < len(d.s) && (d.s[d.pos] == 'e' || d.s[d.pos] == 'E') {
		isFloat = true
		d.pos++
		if d.pos < len(d.s) && (d.s[d.pos] == '+' || d.s[d.pos] == '-') {
			d.pos++
		}
		if err := d.digits(); err != nil {
			return nil, err
		}
	}
	text := d.s[start:d.pos]
	if !isFloat {
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return i, nil
		}
		return mcpBigInt(text), nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		// Python float() overflows an out-of-range literal to ±inf; only
		// overflow can reach here (underflow returns 0 without error).
		if math.IsInf(f, 0) {
			return f, nil
		}
		return nil, err
	}
	return f, nil
}

func (d *mcpJSONDecoder) digits() error {
	if d.pos >= len(d.s) || d.s[d.pos] < '0' || d.s[d.pos] > '9' {
		return fmt.Errorf("json: invalid number")
	}
	for d.pos < len(d.s) && d.s[d.pos] >= '0' && d.s[d.pos] <= '9' {
		d.pos++
	}
	return nil
}

func mcpIsSurrogate(r rune) bool { return r >= 0xD800 && r <= 0xDFFF }

// mcpWTF8Surrogate encodes a lone surrogate as its 3-byte WTF-8 sequence. That
// byte pattern is invalid UTF-8, so it can only come from a lone-surrogate
// escape: mcpDecodeRune recognizes it and mcpJSONString re-emits \uXXXX.
func mcpWTF8Surrogate(r rune) string {
	return string([]byte{0xE0 | byte(r>>12), 0x80 | byte(r>>6&0x3F), 0x80 | byte(r&0x3F)})
}

func mcpDecodeRune(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xED && s[1] >= 0xA0 && s[1] <= 0xBF && s[2] >= 0x80 && s[2] <= 0xBF {
		return rune(s[0]&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	}
	return utf8.DecodeRuneInString(s)
}

// mcpJSONDumpsIndent2 renders v exactly like json.dumps(v, indent=2): default
// ", "/": " separators under indent, key insertion order, empty {} / [] and
// ensure_ascii=True escaping. It returns Python's TypeError text for a value
// json.dumps cannot serialize (a TOML date/time), where the Python caller
// raises instead of writing a document; the scan runs first so the datetime
// never reaches the shared scalar writer and becomes null.
func mcpJSONDumpsIndent2(v any) (string, error) {
	if err := mcpJSONUnserializable(v); err != nil {
		return "", err
	}
	var sb strings.Builder
	mcpWriteJSONIndent(&sb, v, 0)
	return sb.String(), nil
}

// mcpJSONUnserializable returns the TypeError json.dumps raises for the first
// value it cannot serialize (a TOML date/time), or nil. It mirrors Python's
// recursive encoder, which raises at the first unserializable value in
// document order.
func mcpJSONUnserializable(v any) error {
	switch x := v.(type) {
	case *mcpObj:
		for _, k := range x.keys {
			if err := mcpJSONUnserializable(x.vals[k]); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range x {
			if err := mcpJSONUnserializable(e); err != nil {
				return err
			}
		}
	case toml.DateTime:
		return fmt.Errorf("Object of type %s is not JSON serializable", x.PyTypeName())
	}
	return nil
}

func mcpWriteJSONIndent(sb *strings.Builder, v any, depth int) {
	switch x := v.(type) {
	case *mcpObj:
		if x == nil || x.len() == 0 {
			sb.WriteString("{}")
			return
		}
		mcpWriteJSONBlock(sb, "{", "}", x.len(), depth, func(i int) {
			sb.WriteString(mcpJSONString(x.keys[i]))
			sb.WriteString(": ")
			mcpWriteJSONIndent(sb, x.vals[x.keys[i]], depth+1)
		})
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		mcpWriteJSONBlock(sb, "[", "]", len(x), depth, func(i int) {
			mcpWriteJSONIndent(sb, x[i], depth+1)
		})
	default:
		mcpWriteJSONCompact(sb, v)
	}
}

func mcpWriteJSONBlock(sb *strings.Builder, open, closeCh string, n, depth int, entry func(int)) {
	pad := strings.Repeat("  ", depth)
	padIn := strings.Repeat("  ", depth+1)
	sb.WriteString(open)
	sb.WriteByte('\n')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(padIn)
		entry(i)
	}
	sb.WriteByte('\n')
	sb.WriteString(pad)
	sb.WriteString(closeCh)
}

func mcpWriteJSONCompact(sb *strings.Builder, v any) {
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
		sb.WriteString(mcpJSONString(x))
	case int:
		sb.WriteString(strconv.Itoa(x))
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case mcpBigInt:
		sb.WriteString(string(x))
	case toml.BigInt:
		// Defensive: mcpToOrdered normalizes toml.BigInt to mcpBigInt.
		sb.WriteString(string(x))
	case float64:
		sb.WriteString(mcpJSONFloat(x))
	default:
		// Unreachable: composites are always handled by mcpWriteJSONIndent,
		// and mcpJSONDumpsIndent2 rejects a date/time before this writer runs.
		sb.WriteString("null")
	}
}

// mcpJSONFloat mirrors json.dumps(float) for finite values. An infinity can
// only come from an overflowing literal such as 1e400 (bare constants are
// rejected on read), so it is written back as a valid JSON number that
// re-parses to the same value instead of Python's bare Infinity.
func mcpJSONFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "1e999"
	case math.IsInf(f, -1):
		return "-1e999"
	}
	return formatPyFloat(f)
}

// mcpJSONString renders s exactly like json.dumps(s) with ensure_ascii=True.
// Lone surrogates (WTF-8, from a \uXXXX escape) re-emit as \uXXXX; a valid
// astral character is emitted as its surrogate pair.
func mcpJSONString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := mcpDecodeRune(s[i:])
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
			switch {
			case mcpIsSurrogate(r):
				fmt.Fprintf(&sb, `\u%04x`, r)
			case r < 0x20 || r > 0x7e:
				for _, u := range utf16.Encode([]rune{r}) {
					fmt.Fprintf(&sb, `\u%04x`, u)
				}
			default:
				sb.WriteRune(r)
			}
		}
		i += size
	}
	sb.WriteByte('"')
	return sb.String()
}

// ---------------------------------------------------------------------------
// TOML literal serialization (ordered variant of toml_write.toml_value)
// ---------------------------------------------------------------------------

// mcpTOMLValue mirrors toml_write.toml_value while preserving insertion order
// for nested tables (internal/toml.TOMLValue sorts nested maps, which would
// break byte parity for env/header tables).
func mcpTOMLValue(v any) (string, error) {
	switch x := v.(type) {
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			s, err := mcpTOMLValue(e)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *mcpObj:
		parts := make([]string, 0, x.len())
		for _, k := range x.keys {
			s, err := mcpTOMLValue(x.vals[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, k+" = "+s)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	case mcpBigInt:
		// Python str(int): exact decimal digits.
		return string(x), nil
	case string:
		// toml_write.toml_value uses json.dumps for strings; this copy is the
		// same algorithm but also re-emits lone-surrogate escapes.
		return mcpJSONString(x), nil
	default:
		// Scalars (bool/int/float/string) delegate to the existing port.
		return toml.TOMLValue(v)
	}
}

// ---------------------------------------------------------------------------
// read_mcp (manifest section, recipe-mcp merge)
// ---------------------------------------------------------------------------

func mcpIsFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// mcpReadTextFile mirrors Python's Path.read_text(): the file must be readable
// and valid UTF-8 (strict), otherwise the error is propagated so the caller
// fails like the script's uncaught PermissionError / UnicodeDecodeError instead
// of silently rewriting the user's file.
func mcpReadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s: invalid UTF-8", path)
	}
	return string(data), nil
}

// mcpReadServers mirrors toml-read.read_mcp: the [mcp.*] table, with each
// server normalized in the script's fixed key order.
func mcpReadServers(root *toml.Table) *mcpObj {
	raw, ok := root.Table("mcp")
	if !ok || raw == nil {
		return newMCPObj()
	}
	out := newMCPObj()
	for _, name := range raw.Keys() {
		cfg, _ := raw.Get(name)
		out.set(name, mcpNormalizeServer(mcpToOrdered(cfg)))
	}
	return out
}

// mcpNormalizeServer mirrors toml-read._normalize_mcp_server. The fixed key
// order is command, args, env, timeout, [enabled], then the remaining keys in
// document order.
func mcpNormalizeServer(raw any) *mcpObj {
	server, _ := raw.(*mcpObj)
	if server == nil {
		server = newMCPObj()
	}

	normalized := newMCPObj()

	command, _ := server.get("command")
	switch command.(type) {
	case string, []any, nil:
		normalized.set("command", command)
	default:
		normalized.set("command", nil)
	}

	if args, ok := server.get("args"); ok {
		if _, isList := args.([]any); isList {
			normalized.set("args", args)
		} else {
			normalized.set("args", []any{})
		}
	} else {
		normalized.set("args", []any{})
	}

	env, _ := server.get("env")
	_, envIsObj := env.(*mcpObj)
	_, envIsList := env.([]any)
	if !envIsObj && !envIsList {
		env, _ = server.get("environment")
	}
	normalized.set("env", mcpNormalizeEnv(env))

	if timeout, ok := server.get("timeout"); ok {
		switch timeout.(type) {
		case int64, mcpBigInt:
			// Python keeps any int (bool excluded); a wide integer survives.
			normalized.set("timeout", timeout)
		default:
			normalized.set("timeout", nil)
		}
	} else {
		normalized.set("timeout", nil)
	}

	if enabled, ok := server.get("enabled"); ok {
		if b, isBool := enabled.(bool); isBool {
			normalized.set("enabled", b)
		}
	}

	skip := map[string]bool{
		"command": true, "args": true, "env": true,
		"environment": true, "timeout": true, "enabled": true,
	}
	for _, k := range server.keys {
		if skip[k] {
			continue
		}
		normalized.set(k, server.vals[k])
	}
	return normalized
}

// mcpNormalizeEnv mirrors toml-read._normalize_env: dict passthrough, list →
// {NAME: "$NAME"} for non-blank strings, anything else → {}.
func mcpNormalizeEnv(raw any) *mcpObj {
	switch x := raw.(type) {
	case *mcpObj:
		return mcpShallowCopy(x)
	case []any:
		out := newMCPObj()
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				continue
			}
			name := pyStrip(s)
			if name == "" {
				continue
			}
			out.set(name, "$"+name)
		}
		return out
	default:
		return newMCPObj()
	}
}

// mcpLoadServers mirrors mcp-render.load_mcp: manifest servers, then a recipe
// MCP JSON file (when present) merged with recipe values taking precedence and
// new keys appended.
func mcpLoadServers(root *toml.Table, recipeMCPPath string) (*mcpObj, error) {
	mcp := mcpReadServers(root)
	if recipeMCPPath == "" || !mcpIsFile(recipeMCPPath) {
		return mcp, nil
	}
	text, err := mcpReadTextFile(recipeMCPPath)
	if err != nil {
		return nil, err
	}
	// A bare NaN/Infinity is not JSON. Writing it would poison the target with a
	// document the next sync has to back up and regenerate, so the recipe's MCP
	// contribution is refused and the target left untouched.
	// Checked only on input the parser accepted, so otherwise-invalid recipe
	// JSON keeps being ignored as before.
	// Invalid JSON (json.JSONDecodeError) and non-object JSON are silently
	// ignored; a read error is not. A depth overflow (RecursionError) is fatal.
	if parsed, perr := mcpParseJSONOrdered([]byte(text)); perr == nil {
		if hasBareNonFiniteConstant([]byte(text)) {
			return nil, fmt.Errorf("%s: invalid JSON: non-finite constant NaN/Infinity", recipeMCPPath)
		}
		if rm, ok := parsed.(*mcpObj); ok {
			mcp = mcpMergeObjs(mcp, rm)
		}
	} else if errors.Is(perr, errMCPJSONTooDeep) {
		return nil, perr
	}
	return mcp, nil
}

func mcpMergeObjs(a, b *mcpObj) *mcpObj {
	out := mcpShallowCopy(a)
	for _, k := range b.keys {
		out.set(k, b.vals[k])
	}
	return out
}

// ---------------------------------------------------------------------------
// Per-agent translators
// ---------------------------------------------------------------------------

var (
	mcpOpencodeBracedVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	mcpOpencodeBareVarRe   = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	mcpCursorEnvHeaderRe   = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// mcpEnvVarRefName implements _ENV_VAR_RE = ^\$\{?([A-Z_][A-Z0-9_]*)\}?$.
//
// Python's `$` also matches just before one trailing newline, so "$VAR\n"
// matches; re.sub replaces only the matched portion, so the newline survives
// (returned here as suffix). The optional braces are independent: $VAR,
// ${VAR, $VAR} and ${VAR} all match.
func mcpEnvVarRefName(s string) (name, suffix string, ok bool) {
	body := s
	if strings.HasSuffix(body, "\n") {
		body = body[:len(body)-1]
		suffix = "\n"
	}
	if !strings.HasPrefix(body, "$") {
		return "", "", false
	}
	body = body[1:]
	if strings.HasPrefix(body, "{") {
		body = body[1:]
	}
	if strings.HasSuffix(body, "}") {
		body = body[:len(body)-1]
	}
	if body == "" {
		return "", "", false
	}
	for i, r := range body {
		if i == 0 {
			if !(r >= 'A' && r <= 'Z' || r == '_') {
				return "", "", false
			}
			continue
		}
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return "", "", false
		}
	}
	return body, suffix, true
}

// mcpEnvRefsForOpencode mirrors _env_refs_for_opencode.
func mcpEnvRefsForOpencode(value string) string {
	value = mcpOpencodeBracedVarRe.ReplaceAllString(value, "{env:${1}}")
	return mcpOpencodeBareVarRe.ReplaceAllString(value, "{env:${1}}")
}

// mcpHeadersForOpencode mirrors _headers_for_opencode.
func mcpHeadersForOpencode(headers any) any {
	obj, ok := headers.(*mcpObj)
	if !ok {
		return headers
	}
	out := newMCPObj()
	for _, k := range obj.keys {
		v := obj.vals[k]
		if s, ok := v.(string); ok {
			v = mcpEnvRefsForOpencode(mcpCursorEnvHeaderRe.ReplaceAllString(s, "{env:${1}}"))
		}
		out.set(k, v)
	}
	return out
}

// mcpTranslateGeneric mirrors _translate_generic: expand $VAR to ${VAR} in env.
func mcpTranslateGeneric(servers *mcpObj) *mcpObj {
	out := newMCPObj()
	for _, name := range servers.keys {
		cfg, ok := servers.vals[name].(*mcpObj)
		if !ok {
			out.set(name, servers.vals[name])
			continue
		}
		newCfg := mcpShallowCopy(cfg)
		if env, ok := newCfg.get("env"); ok && mcpJSONTruthy(env) {
			if eo, ok := env.(*mcpObj); ok {
				newEnv := newMCPObj()
				for _, k := range eo.keys {
					v := eo.vals[k]
					if s, isStr := v.(string); isStr {
						if ref, suffix, isRef := mcpEnvVarRefName(s); isRef {
							v = "${" + ref + "}" + suffix
						}
					}
					newEnv.set(k, v)
				}
				newCfg.set("env", newEnv)
			}
		}
		out.set(name, newCfg)
	}
	return out
}

// mcpTranslateOpencode mirrors _translate_opencode: the native local/remote
// schema with {env:VAR} substitutions.
func mcpTranslateOpencode(servers *mcpObj) *mcpObj {
	out := newMCPObj()
	for _, name := range servers.keys {
		cfg, ok := servers.vals[name].(*mcpObj)
		if !ok {
			out.set(name, servers.vals[name])
			continue
		}

		mcpType, _ := cfg.get("type")
		url, _ := cfg.get("url")
		urlStr, urlIsStr := url.(string)
		if mcpIsHTTPType(mcpType) && urlIsStr && urlStr != "" {
			newCfg := newMCPObj()
			newCfg.set("type", "remote")
			newCfg.set("url", url)
			for _, p := range []string{"timeout", "enabled"} {
				if v, ok := cfg.get(p); ok {
					newCfg.set(p, v)
				}
			}
			if v, ok := cfg.get("headers"); ok {
				newCfg.set("headers", mcpHeadersForOpencode(v))
			}
			out.set(name, newCfg)
			continue
		}

		command, _ := cfg.get("command")
		args, _ := cfg.get("args")
		var full []any
		switch c := command.(type) {
		case string:
			full = append([]any{c}, mcpAsList(args)...)
		case []any:
			full = append([]any{}, c...)
		default:
			full = []any{}
		}
		for i, a := range full {
			if s, ok := a.(string); ok {
				full[i] = mcpEnvRefsForOpencode(s)
			}
		}

		newCfg := newMCPObj()
		newCfg.set("type", "local")
		newCfg.set("command", full)

		if env, ok := cfg.get("env"); ok && mcpJSONTruthy(env) {
			if eo, ok := env.(*mcpObj); ok {
				newEnv := newMCPObj()
				for _, k := range eo.keys {
					v := eo.vals[k]
					if s, isStr := v.(string); isStr {
						if ref, suffix, isRef := mcpEnvVarRefName(s); isRef {
							v = "{env:" + ref + "}" + suffix
						}
					}
					newEnv.set(k, v)
				}
				newCfg.set("environment", newEnv)
			}
		}

		for _, p := range []string{"timeout", "enabled"} {
			if v, ok := cfg.get(p); ok {
				newCfg.set(p, v)
			}
		}
		out.set(name, newCfg)
	}
	return out
}

func mcpTranslateServers(agent string, servers *mcpObj) *mcpObj {
	if agent == "opencode" {
		return mcpTranslateOpencode(servers)
	}
	return mcpTranslateGeneric(servers)
}

func mcpIsHTTPType(t any) bool {
	s, ok := t.(string)
	return ok && (s == "http" || s == "remote" || s == "sse")
}

func mcpAsList(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

// mcpJSONTruthy mirrors Python truthiness for the ordered JSON model.
func mcpJSONTruthy(v any) bool {
	switch x := v.(type) {
	case *mcpObj:
		return x != nil && x.len() > 0
	case []any:
		return len(x) > 0
	default:
		return pyTruthy(v)
	}
}

// mcpSlimConfigForWrite mirrors _slim_mcp_config_for_write.
func mcpSlimConfigForWrite(cfg *mcpObj) *mcpObj {
	mcpType, _ := cfg.get("type")
	url, _ := cfg.get("url")
	if urlStr, ok := url.(string); ok && urlStr != "" && mcpIsHTTPType(mcpType) {
		slim := newMCPObj()
		slim.set("type", mcpType)
		slim.set("url", url)
		for _, k := range []string{"timeout", "enabled", "headers"} {
			if v, ok := cfg.get(k); ok && v != nil {
				slim.set(k, v)
			}
		}
		return slim
	}
	out := newMCPObj()
	for _, k := range cfg.keys {
		if cfg.vals[k] != nil {
			out.set(k, cfg.vals[k])
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Target-file merge (JSON + TOML) and entry point
// ---------------------------------------------------------------------------

// mcpMergePlan is the per-server ownership decision for one target: the
// manifest servers to (re)write, the recorded servers to drop because the
// manifest no longer declares them, the servers kept as user-authored because a
// same-named entry was never recorded as ai-specs-managed, and the servers that
// are neither (the upgrade notice's legacy leftovers).
type mcpMergePlan struct {
	managed    []string
	removed    []string
	preserved  []string
	unrecorded []string
}

func mcpPlanMerge(desired *mcpObj, owned map[string]bool, ownedNames []string, current *mcpObj) mcpMergePlan {
	plan := mcpMergePlan{}
	for _, name := range ownedNames {
		if !desired.has(name) && current.has(name) {
			plan.removed = append(plan.removed, name)
		}
	}
	for _, name := range desired.keys {
		if current.has(name) && !owned[name] {
			plan.preserved = append(plan.preserved, name)
			continue
		}
		plan.managed = append(plan.managed, name)
	}
	for _, name := range current.keys {
		if !owned[name] && !desired.has(name) {
			plan.unrecorded = append(plan.unrecorded, name)
		}
	}
	return plan
}

// mcpMergeIntoJSON merges per server (S5): foreign top-level keys keep their
// order, a recorded server is replaced or dropped, an unrecorded one is left to
// the user, and the manifest servers are appended after the existing ones.
func mcpMergeIntoJSON(existing *mcpObj, mcpKey string, desired *mcpObj, owned map[string]bool, ownedNames []string, agent string) (string, mcpMergePlan, error) {
	if agent == "opencode" {
		var schema any = "https://opencode.ai/config.json"
		if v, ok := existing.get("$schema"); ok {
			schema = v
		}
		rebuilt := newMCPObj()
		rebuilt.set("$schema", schema)
		for _, k := range existing.keys {
			if k == "$schema" {
				continue
			}
			rebuilt.set(k, existing.vals[k])
		}
		existing = rebuilt
	}

	current, _ := existing.get(mcpKey)
	servers, ok := current.(*mcpObj)
	if !ok {
		servers = newMCPObj()
	}
	plan := mcpPlanMerge(desired, owned, ownedNames, servers)
	merged := mcpShallowCopy(servers)
	for _, name := range ownedNames {
		if !desired.has(name) {
			merged.del(name)
		}
	}
	for _, name := range plan.managed {
		merged.set(name, desired.vals[name])
	}
	existing.set(mcpKey, merged)

	document, err := mcpJSONDumpsIndent2(existing)
	if err != nil {
		return "", plan, err
	}
	return document + "\n", plan, nil
}

// ---------------------------------------------------------------------------
// Codex TOML: one managed block, user bytes never rewritten
// ---------------------------------------------------------------------------

// The Codex target is user config, so ai-specs only ever writes inside its own
// marker block (the same managed-block shape as the .envrc block). Everything
// outside the markers is preserved byte-for-byte — comments, CRLF, ordering,
// formatting, servers declared in any TOML form — which is what makes the merge
// safe on documents whose TOML dialect differs from ours.
const (
	mcpTOMLBlockBegin = "# managed-by: ai-specs (do not remove block): MCP servers"
	mcpTOMLBlockEnd   = "# end managed-by: ai-specs: MCP servers"
)

// mcpTOMLRawLine is one line of the target with its byte offsets, so the block
// can be cut and appended without touching a single byte outside it.
type mcpTOMLRawLine struct {
	start, end int // end is past the line terminator
	text       string
}

func mcpTOMLRawLines(text string) []mcpTOMLRawLine {
	var lines []mcpTOMLRawLine
	for start := 0; start < len(text); {
		nl := strings.IndexByte(text[start:], '\n')
		end := len(text)
		if nl >= 0 {
			end = start + nl + 1
		}
		content := text[start:end]
		content = strings.TrimSuffix(content, "\n")
		content = strings.TrimSuffix(content, "\r")
		lines = append(lines, mcpTOMLRawLine{start: start, end: end, text: content})
		start = end
	}
	return lines
}

// mcpTOMLBlock is the managed block found in a target: the byte span to replace
// or remove (markers plus the blank separator line on either side, so removing
// it restores the original bytes exactly) and the server names it declares.
type mcpTOMLBlock struct {
	present bool
	outside string
	names   []string
}

// mcpTOMLSplitBlock finds the managed block. Malformed markers (begin without
// end, duplicates, end before begin, an unparsable block) are an error: the
// caller refuses instead of guessing which markers are its own.
func mcpTOMLSplitBlock(text, targetPath, mcpKey string) (mcpTOMLBlock, error) {
	malformed := fmt.Errorf("%s: refusing to rewrite Codex MCP config: malformed %q / %q block markers", targetPath, mcpTOMLBlockBegin, mcpTOMLBlockEnd)
	lines := mcpTOMLRawLines(text)
	begins, ends := []int{}, []int{}
	for i, line := range lines {
		switch strings.TrimSpace(line.text) {
		case mcpTOMLBlockBegin:
			begins = append(begins, i)
		case mcpTOMLBlockEnd:
			ends = append(ends, i)
		}
	}
	if len(begins) == 0 && len(ends) == 0 {
		return mcpTOMLBlock{outside: text}, nil
	}
	if len(begins) != 1 || len(ends) != 1 || begins[0] > ends[0] {
		return mcpTOMLBlock{}, malformed
	}
	begin, endIdx := begins[0], ends[0]

	// The separator blank line belongs to the block on both sides.
	removeStart, removeEnd := lines[begin].start, lines[endIdx].end
	if begin > 0 && strings.TrimSpace(lines[begin-1].text) == "" {
		removeStart = lines[begin-1].start
	}
	if endIdx+1 < len(lines) && strings.TrimSpace(lines[endIdx+1].text) == "" {
		removeEnd = lines[endIdx+1].end
	}

	blockText := text[lines[begin].start:lines[endIdx].end]
	parsed, err := toml.Parse([]byte(blockText))
	if err != nil {
		return mcpTOMLBlock{}, fmt.Errorf("%s: refusing to rewrite Codex MCP config: the managed MCP block does not parse: %w", targetPath, err)
	}
	names := mcpTOMLServerNames(parsed, mcpKey)
	return mcpTOMLBlock{present: true, outside: text[:removeStart] + text[removeEnd:], names: names}, nil
}

// mcpTOMLServerNames lists the servers a parsed document declares under
// <mcp_key>, in document order.
func mcpTOMLServerNames(doc *toml.Table, mcpKey string) []string {
	servers, ok := doc.Table(mcpKey)
	if !ok || servers == nil {
		return nil
	}
	return servers.Keys()
}

// mcpTOMLAppendBlock appends the managed block after the preserved bytes, with
// one blank separator line when there is content before it.
func mcpTOMLAppendBlock(outside, block string) string {
	switch {
	case outside == "":
		return block + "\n"
	case strings.HasSuffix(outside, "\n"):
		return outside + "\n" + block + "\n"
	default:
		return outside + "\n\n" + block + "\n"
	}
}

// mcpMergeIntoTOML renders the managed Codex block: servers declared outside it
// are the user's (collision → preserved), and everything outside the markers is
// kept byte-for-byte. The outside content must parse with the project's TOML
// parser so the server names it declares are read semantically, not by matching
// text; when it does not, nothing is added or changed.
func mcpMergeIntoTOML(targetPath, mcpKey string, desired *mcpObj) (string, mcpMergePlan, error) {
	text := ""
	if mcpIsFile(targetPath) {
		raw, err := mcpReadTextFile(targetPath)
		if err != nil {
			return "", mcpMergePlan{}, err
		}
		text = raw
	}

	block, err := mcpTOMLSplitBlock(text, targetPath, mcpKey)
	if err != nil {
		return "", mcpMergePlan{}, err
	}
	outsideTable, err := toml.Parse([]byte(block.outside))
	if err != nil {
		return "", mcpMergePlan{}, fmt.Errorf("%s: refusing to rewrite Codex MCP config: %w", targetPath, err)
	}

	present := newMCPObj()
	for _, name := range mcpTOMLServerNames(outsideTable, mcpKey) {
		present.set(name, true)
	}
	plan := mcpMergePlan{}
	for _, name := range present.keys {
		if desired.has(name) {
			plan.preserved = append(plan.preserved, name)
		} else {
			plan.unrecorded = append(plan.unrecorded, name)
		}
	}
	written := make(map[string]bool, desired.len())
	for _, name := range desired.keys {
		if present.has(name) {
			continue
		}
		plan.managed = append(plan.managed, name)
		written[name] = true
	}
	for _, name := range block.names {
		if !written[name] {
			plan.removed = append(plan.removed, name)
		}
	}
	if !block.present && len(plan.managed) == 0 {
		// Nothing to write and nothing to prune: never touch the file.
		return text, plan, nil
	}

	if len(plan.managed) == 0 {
		// The block is removed and the outside bytes stay exactly as they were.
		return block.outside, plan, nil
	}
	rendered, err := mcpTOMLRenderBlock(mcpKey, desired, plan.managed)
	if err != nil {
		return "", plan, err
	}
	content := mcpTOMLAppendBlock(block.outside, rendered)
	if mcpHasLoneSurrogate(content) {
		// RenderMCPFile refuses this atomically with its own message.
		return content, plan, nil
	}
	if _, err := toml.Parse([]byte(content)); err != nil {
		// Self-check: never write a document our own parser rejects.
		return "", plan, fmt.Errorf("%s: refusing to rewrite Codex MCP config: merged document does not parse: %w", targetPath, err)
	}
	return content, plan, nil
}

// mcpTOMLRenderBlock renders the marker block around one table per managed
// server, in manifest order.
func mcpTOMLRenderBlock(mcpKey string, desired *mcpObj, names []string) (string, error) {
	var sb strings.Builder
	sb.WriteString(mcpTOMLBlockBegin)
	sb.WriteString("\n")
	for _, name := range names {
		cfg, ok := desired.vals[name].(*mcpObj)
		if !ok {
			cfg = newMCPObj()
		}
		sb.WriteString("[" + mcpKey + "." + name + "]\n")
		for _, k := range cfg.keys {
			val, err := mcpTOMLValue(cfg.vals[k])
			if err != nil {
				return "", err
			}
			sb.WriteString(k + " = " + val + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(mcpTOMLBlockEnd)
	return sb.String(), nil
}

// RenderMCPOptions configures RenderMCPFile. RecipeMCPPath is the
// --recipe-mcp argument ("" = flag absent); a non-empty path that is not a
// file is ignored, exactly like load_mcp's is_file guard. Owned is the lock's
// per-target server-name record (S5), Recorded says the lock has a record entry
// for the target, and Result receives the outcome.
type RenderMCPOptions struct {
	RecipeMCPPath string
	DryRun        bool
	Owned         []string
	Recorded      bool
	Result        *MCPResult
}

// MCPResult is one target render's ownership outcome: the server names
// ai-specs now manages at that target with the sha256 of their rendered entry,
// plus what it removed, what it left to the user (a name collision), and the
// servers it found but never recorded (the upgrade notice).
type MCPResult struct {
	Managed    map[string]string
	Removed    []string
	Preserved  []string
	Unrecorded []string
	Noticed    bool
	Wrote      bool
}

// mcpServerHashes is the lock witness for the servers ai-specs owns at one
// target: the sha256 of each rendered entry (canonical JSON dump).
func mcpServerHashes(servers *mcpObj, names []string) map[string]string {
	out := make(map[string]string, len(names))
	for _, name := range names {
		doc, err := mcpJSONDumpsIndent2(servers.vals[name])
		if err != nil {
			continue
		}
		out[name] = lock.Sha256Bytes([]byte(doc))
	}
	return out
}

// mcpPathSuffix mirrors pathlib.PurePath.suffix: the extension from the last
// dot of the final component, "" when there is no dot or the name only starts
// with one.
func mcpPathSuffix(p string) string {
	base := filepath.Base(p)
	i := strings.LastIndexByte(base, '.')
	if i <= 0 {
		return ""
	}
	return base[i:]
}

// mcpHasLoneSurrogate reports whether s holds a lone surrogate — the one
// character Python's UTF-8 write_text()/print() cannot encode. mcpJSONString
// escapes every surrogate it re-emits and all inputs are validated UTF-8, so
// only a raw TOML table name or inline-table key can leak a WTF-8 surrogate
// into the rendered content.
func mcpHasLoneSurrogate(s string) bool {
	for i := 0; i < len(s); {
		r, size := mcpDecodeRune(s[i:])
		if mcpIsSurrogate(r) {
			return true
		}
		i += size
	}
	return false
}

// RenderMCPFile renders one agent's MCP configuration and returns its exit code.
func RenderMCPFile(tomlPath, agent, targetPath, mcpKey string, opts RenderMCPOptions, stdout, stderr io.Writer) int {
	if !mcpIsFile(tomlPath) {
		fmt.Fprintf(stderr, "error: %s not found\n", tomlPath)
		return 1
	}
	text, err := mcpReadTextFile(tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	root, err := toml.Parse([]byte(text))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	servers, err := mcpLoadServers(root, opts.RecipeMCPPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if servers.len() == 0 && len(opts.Owned) == 0 && !mcpIsFile(targetPath) {
		fmt.Fprintf(stderr, "info: no [mcp.*] entries \u2014 skipping %s\n", agent)
		return 0
	}

	translated := mcpTranslateServers(agent, servers)
	slimmed := newMCPObj()
	for _, name := range translated.keys {
		if cfg, ok := translated.vals[name].(*mcpObj); ok {
			slimmed.set(name, mcpSlimConfigForWrite(cfg))
		} else {
			slimmed.set(name, translated.vals[name])
		}
	}

	owned := make(map[string]bool, len(opts.Owned))
	ownedNames := append([]string(nil), opts.Owned...)
	sort.Strings(ownedNames)
	for _, name := range ownedNames {
		owned[name] = true
	}

	var content string
	var invalid []byte
	var plan mcpMergePlan
	if mcpPathSuffix(targetPath) == ".toml" {
		content, plan, err = mcpMergeIntoTOML(targetPath, mcpKey, slimmed)
	} else {
		var existing *mcpObj
		existing, invalid, err = loadJSONTarget(targetPath)
		if err == nil {
			content, plan, err = mcpMergeIntoJSON(existing, mcpKey, slimmed, owned, ownedNames, agent)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if opts.Result != nil {
		opts.Result.Managed = mcpServerHashes(slimmed, plan.managed)
		opts.Result.Removed = plan.removed
		opts.Result.Preserved = plan.preserved
		opts.Result.Unrecorded = plan.unrecorded
	}
	for _, name := range plan.preserved {
		fmt.Fprintf(stderr, "    ! preserved user MCP server %q in %s (not recorded as ai-specs-managed)\n", name, targetPath)
	}
	// Upgrade notice: a target with servers ai-specs never recorded, before the
	// lock knows about the target at all. One line per target, never a delete.
	if !opts.Recorded && len(plan.unrecorded) > 0 {
		fmt.Fprintf(stderr, "    ! %s: MCP servers not recorded as ai-specs-managed: %s (remove them if ai-specs generated them)\n",
			targetPath, strings.Join(plan.unrecorded, ", "))
		if opts.Result != nil {
			opts.Result.Noticed = true
		}
	}
	// A corrupt target is regenerated only when ai-specs owns something there.
	regenerate := invalid != nil && len(ownedNames) > 0
	if len(plan.managed) == 0 && len(plan.removed) == 0 && !regenerate {
		// Nothing to add, update or drop: an untouched user file stays untouched.
		// The caller owns the "no [mcp.*] entries" line for an existing target.
		return 0
	}

	if !opts.DryRun {
		if dir := filepath.Dir(targetPath); dir != "" {
			if err := os.MkdirAll(dir, 0o777); err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err)
				return 1
			}
		}
	}

	if mcpHasLoneSurrogate(content) {
		// Python's write_text() truncates the target before it fails to encode
		// the text; S5 requires the target to stay byte-identical on error, so
		// the render refuses before touching it.
		fmt.Fprintf(stderr, "error: %s: rendered content is not UTF-8 encodable (lone surrogate)\n", targetPath)
		return 1
	}

	if opts.DryRun {
		fmt.Fprintf(stdout, "--- %s (dry-run) ---\n", targetPath)
		fmt.Fprintln(stdout, content)
		return 0
	}

	if err := backupInvalidJSON(targetPath, invalid, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := os.WriteFile(targetPath, []byte(content), 0o666); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if opts.Result != nil {
		opts.Result.Wrote = true
	}
	for _, name := range plan.removed {
		fmt.Fprintf(stdout, "    \u2713 removed managed MCP server %q from %s\n", name, targetPath)
	}
	if len(plan.managed) > 0 {
		fmt.Fprintf(stdout, "  \u2713 %s \u2192 %s (%d server(s))\n", agent, targetPath, len(plan.managed))
	}
	return 0
}
