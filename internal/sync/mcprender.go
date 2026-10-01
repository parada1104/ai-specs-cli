package sync

// Byte-exact Go port of lib/_internal/mcp-render.py (GO-07.S4).
//
// Scope: library only. The Bash spine keeps shelling out to mcp-render.py until
// the sync-agent fan-out moves to Go in S15 (same treatment as S2's
// gitignore-render.py); there is deliberately no GO_SYNC_STEP_* flag yet.
//
// The port mirrors the script's observable surface exactly: the merged
// per-agent config bytes, stdout, stderr and exit code. Every Python quirk that
// affects those bytes is reproduced rather than "fixed" (see the inline notes
// on Python's `$`-anchor regex, dict first-position/last-value duplicate keys,
// and str.splitlines() boundaries).

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

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

// mcpParseJSONOrdered mirrors json.loads with its defaults: insertion-ordered
// objects (duplicate keys keep the first position and the last value), int64 for
// integer literals (mcpBigInt when wider than int64), float64 otherwise, the
// bare NaN / Infinity / -Infinity constants, and \uXXXX lone-surrogate escapes
// kept verbatim (a valid surrogate pair still combines). Any structural error
// (including trailing data or invalid UTF-8) is returned so the caller can
// degrade to {}.
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
	s   string
	pos int
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
		return d.object()
	case c == '[':
		return d.array()
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
// ensure_ascii=True escaping.
func mcpJSONDumpsIndent2(v any) string {
	var sb strings.Builder
	mcpWriteJSONIndent(&sb, v, 0)
	return sb.String()
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
	case float64:
		sb.WriteString(mcpJSONFloat(x))
	default:
		// Unreachable: composites are always handled by mcpWriteJSONIndent.
		sb.WriteString("null")
	}
}

// mcpJSONFloat mirrors json.dumps(float): float.__repr__ for finite values,
// NaN / Infinity / -Infinity otherwise.
func mcpJSONFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
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
		if t, isInt := timeout.(int64); isInt {
			normalized.set("timeout", t)
		} else {
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
	// Invalid JSON (json.JSONDecodeError) and non-object JSON are silently
	// ignored; a read error is not.
	if parsed, perr := mcpParseJSONOrdered([]byte(text)); perr == nil {
		if rm, ok := parsed.(*mcpObj); ok {
			mcp = mcpMergeObjs(mcp, rm)
		}
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

// mcpMergeIntoJSON mirrors merge_into_json: preserve other top-level keys (and
// their order) from the existing file, force $schema first for opencode, then
// replace our mcp key.
func mcpMergeIntoJSON(targetPath, mcpKey string, servers *mcpObj, agent string) (string, error) {
	existing := newMCPObj()
	if mcpIsFile(targetPath) {
		text, err := mcpReadTextFile(targetPath)
		if err != nil {
			return "", err
		}
		if parsed, perr := mcpParseJSONOrdered([]byte(text)); perr == nil {
			if obj, ok := parsed.(*mcpObj); ok {
				existing = obj
			}
		}
	}

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

	existing.set(mcpKey, servers)
	return mcpJSONDumpsIndent2(existing) + "\n", nil
}

// mcpMergeIntoTOML mirrors merge_into_toml: strip prior [<mcp_key>.*] blocks,
// trim trailing blanks, append one table per server.
func mcpMergeIntoTOML(targetPath, mcpKey string, servers *mcpObj) (string, error) {
	var existingLines []string
	if mcpIsFile(targetPath) {
		text, err := mcpReadTextFile(targetPath)
		if err != nil {
			return "", err
		}
		existingLines = pySplitLines(text)
	}

	outLines := []string{}
	skip := false
	sectionPrefix := "[" + mcpKey + "."
	for _, line := range existingLines {
		stripped := pyStrip(line)
		if strings.HasPrefix(stripped, sectionPrefix) {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(stripped, "[") && !strings.HasPrefix(stripped, sectionPrefix) {
			skip = false
		}
		if skip {
			continue
		}
		outLines = append(outLines, line)
	}

	for len(outLines) > 0 && pyStrip(outLines[len(outLines)-1]) == "" {
		outLines = outLines[:len(outLines)-1]
	}
	if len(outLines) > 0 {
		outLines = append(outLines, "")
	}

	for _, name := range servers.keys {
		cfg, ok := servers.vals[name].(*mcpObj)
		if !ok {
			cfg = newMCPObj()
		}
		outLines = append(outLines, "["+mcpKey+"."+name+"]")
		for _, k := range cfg.keys {
			val, err := mcpTOMLValue(cfg.vals[k])
			if err != nil {
				return "", err
			}
			outLines = append(outLines, k+" = "+val)
		}
		outLines = append(outLines, "")
	}

	return strings.TrimRightFunc(strings.Join(outLines, "\n"), pyIsSpace) + "\n", nil
}

// RenderMCPOptions configures RenderMCPFile. RecipeMCPPath is the
// --recipe-mcp argument ("" = flag absent); a non-empty path that is not a
// file is ignored, exactly like load_mcp's is_file guard.
type RenderMCPOptions struct {
	RecipeMCPPath string
	DryRun        bool
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

// RenderMCPFile is the byte-exact port of mcp-render.py's main() body: it
// returns the process exit code and writes the same stdout/stderr the script
// would. It is a library entry point only; the Bash spine still execs the
// script until S15.
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
	if servers.len() == 0 {
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

	var content string
	if mcpPathSuffix(targetPath) == ".toml" {
		content, err = mcpMergeIntoTOML(targetPath, mcpKey, slimmed)
	} else {
		content, err = mcpMergeIntoJSON(targetPath, mcpKey, slimmed, agent)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
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
		// mcp-render.py lets the rendered text reach write_text()/print(), which
		// re-encode it and raise UnicodeEncodeError on a lone surrogate. Only a
		// raw TOML key leaks one here (JSON and TOML *values* go through
		// json.dumps, which escapes it). write_text() opens the target before it
		// encodes, so the file is left created/truncated empty; reproduce that
		// side effect and fail like the uncaught exception (rc 1).
		if !opts.DryRun {
			_ = os.WriteFile(targetPath, nil, 0o666)
		}
		fmt.Fprintf(stderr, "error: %s: rendered content is not UTF-8 encodable (lone surrogate)\n", targetPath)
		return 1
	}

	if opts.DryRun {
		fmt.Fprintf(stdout, "--- %s (dry-run) ---\n", targetPath)
		fmt.Fprintln(stdout, content)
		return 0
	}

	if err := os.WriteFile(targetPath, []byte(content), 0o666); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "  \u2713 %s \u2192 %s (%d server(s))\n", agent, targetPath, slimmed.len())
	return 0
}
