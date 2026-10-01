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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

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

// mcpParseJSONOrdered mirrors json.loads: ordered objects (duplicate keys keep
// the first position and the last value), int64 for integer literals, float64
// otherwise. Any structural error (including trailing data) is returned so the
// caller can degrade to {}.
func mcpParseJSONOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := mcpDecodeJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("json: trailing data")
		}
		return nil, err
	}
	return v, nil
}

func mcpDecodeJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := newMCPObj()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("json: object key is not a string")
				}
				val, err := mcpDecodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj.set(key, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			arr := []any{}
			for dec.More() {
				val, err := mcpDecodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("json: unexpected delimiter %v", t)
	case string:
		return t, nil
	case json.Number:
		return mcpJSONNumber(t)
	case bool:
		return t, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("json: unexpected token")
}

func mcpJSONNumber(n json.Number) (any, error) {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i, nil
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return f, nil
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
func mcpJSONString(s string) string {
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
func mcpLoadServers(root *toml.Table, recipeMCPPath string) *mcpObj {
	mcp := mcpReadServers(root)
	if recipeMCPPath != "" && mcpIsFile(recipeMCPPath) {
		if data, err := os.ReadFile(recipeMCPPath); err == nil {
			if parsed, perr := mcpParseJSONOrdered(data); perr == nil {
				if rm, ok := parsed.(*mcpObj); ok {
					mcp = mcpMergeObjs(mcp, rm)
				}
			}
		}
	}
	return mcp
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
		if data, err := os.ReadFile(targetPath); err == nil {
			if parsed, perr := mcpParseJSONOrdered(data); perr == nil {
				if obj, ok := parsed.(*mcpObj); ok {
					existing = obj
				}
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
		if data, err := os.ReadFile(targetPath); err == nil {
			existingLines = pySplitLines(string(data))
		}
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

// RenderMCPFile is the byte-exact port of mcp-render.py's main() body: it
// returns the process exit code and writes the same stdout/stderr the script
// would. It is a library entry point only; the Bash spine still execs the
// script until S15.
func RenderMCPFile(tomlPath, agent, targetPath, mcpKey string, opts RenderMCPOptions, stdout, stderr io.Writer) int {
	if !mcpIsFile(tomlPath) {
		fmt.Fprintf(stderr, "error: %s not found\n", tomlPath)
		return 1
	}
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s not found\n", tomlPath)
		return 1
	}
	root, err := toml.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	servers := mcpLoadServers(root, opts.RecipeMCPPath)
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

	if opts.DryRun {
		fmt.Fprintf(stdout, "--- %s (dry-run) ---\n", targetPath)
		fmt.Fprintln(stdout, content)
		return 0
	}

	if dir := filepath.Dir(targetPath); dir != "" {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}
	if err := os.WriteFile(targetPath, []byte(content), 0o666); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "  \u2713 %s \u2192 %s (%d server(s))\n", agent, targetPath, slimmed.len())
	return 0
}
