// Package config reads and surgically edits ai-specs manifests.
//
// It is the Go port of lib/_internal/toml-read.py (section readers with
// byte-identical JSON output) and of the inline python3 heredocs in
// lib/skills-add.sh, lib/recipe-remove.sh, and lib/skills-remove.sh
// (surgical manifest writes over raw bytes, never re-serialized).
//
// Byte-identity of the observable surface is the contract
// (docs/go-migration-parity-contract.md §3): the JSON emitted by
// ReadSectionJSON must be byte-for-byte what `python3
// lib/_internal/toml-read.py <path> <section>` prints, and every write
// operation must leave the file bytes, stdout message, and error strings
// identical to the Python heredocs. Parse-error detail inside the
// valid→invalid guard messages is diagnostic only (ADR 0002).
package config

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/toml"
)

// ManifestNotFoundError mirrors the Python FileNotFoundError("<path> not
// found") raised by toml-read.py's load_toml.
type ManifestNotFoundError struct {
	Path string
}

func (e *ManifestNotFoundError) Error() string { return e.Path + " not found" }

// UnknownSectionError mirrors the KeyError(section) raised by toml-read.py's
// read_section.
type UnknownSectionError struct {
	Section string
}

func (e *UnknownSectionError) Error() string {
	return fmt.Sprintf("unknown section '%s'", e.Section)
}

// LoadManifest parses the manifest at path with internal/toml, mirroring
// toml-read.py's load_toml: a missing (or non-file) path yields the Python
// FileNotFoundError text "<path> not found".
func LoadManifest(path string) (*toml.Table, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, &ManifestNotFoundError{Path: path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return toml.Parse(data)
}

// ReadSection reads one manifest section and returns it as ordered values:
// *Obj for normalized objects (Python dict insertion order), *toml.Table
// for raw tables (TOML document order), []any for arrays, scalars verbatim.
func ReadSection(data *toml.Table, section string) (any, error) {
	switch section {
	case "project":
		return readProject(tableOr(data, "project")), nil
	case "agents":
		return readAgents(tableOr(data, "agents")), nil
	case "deps":
		return readDeps(data), nil
	case "mcp":
		return readMcp(data), nil
	case "recipes":
		return readRecipes(data), nil
	case "bindings":
		return readBindings(data), nil
	default:
		return nil, &UnknownSectionError{Section: section}
	}
}

// ReadSectionJSON renders the section exactly as
// `python3 lib/_internal/toml-read.py <path> <section>` prints it.
func ReadSectionJSON(data *toml.Table, section string) (string, error) {
	payload, err := ReadSection(data, section)
	if err != nil {
		return "", err
	}
	return encodeJSON(payload), nil
}

// --- helpers -------------------------------------------------------------

// tableOr returns the sub-table under key, or nil when absent.
func tableOr(t *toml.Table, key string) *toml.Table {
	if t == nil {
		return nil
	}
	v, ok := t.Table(key)
	if !ok {
		return nil
	}
	return v
}

// getKey is the nil-safe Get.
func getKey(t *toml.Table, key string) (any, bool) {
	if t == nil {
		return nil, false
	}
	return t.Get(key)
}

// truthy mirrors Python truthiness for the value types internal/toml
// produces.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case []*toml.Table:
		return len(x) > 0
	case *toml.Table:
		return x != nil && len(x.Keys()) > 0
	default:
		return true
	}
}

// pyStr mirrors Python str() for tomllib value types: str passes through,
// bool becomes "True"/"False", int/float use Python str formatting, and
// containers use their repr (Python str of a list/dict is its repr).
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int64:
		return formatPyInt(x)
	case float64:
		return formatPyFloat(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []*toml.Table:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *toml.Table:
		return pyReprTable(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// pyRepr mirrors Python repr() for tomllib value types.
func pyRepr(v any) string {
	if s, ok := v.(string); ok {
		return pyReprString(s)
	}
	return pyStr(v)
}

func pyReprTable(t *toml.Table) string {
	if t == nil {
		return "None"
	}
	parts := make([]string, 0, len(t.Keys()))
	for _, k := range t.Keys() {
		v, _ := t.Get(k)
		parts = append(parts, pyRepr(k)+": "+pyRepr(v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// pyReprString mirrors Python repr() for strings: quote switches to '"' when
// the value contains ' but no "; \\ \n \r \t and the active quote are escaped;
// every non-printable character becomes \\xNN / \\uNNNN / \\UNNNNNNNN (lowercase
// hex). Probe-pinned against Python 3.14 repr(): chars < 0x20 and 0x7f-0x9f
// emit \\xNN (e.g. '\\x85'); printable non-ASCII like é and non-BMP emoji stay
// raw; NBSP and U+2028 are non-printable and are escaped.
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

// --- section readers -------------------------------------------------------

// readProject: {"name": str(project.name or ""), "subrepos": [...]}.
func readProject(project *toml.Table) *Obj {
	out := newObject()
	name := ""
	if v, ok := getKey(project, "name"); ok && truthy(v) {
		name = pyStr(v)
	}
	out.set("name", name)
	out.set("subrepos", normalizedStringList(project, "subrepos"))
	return out
}

// readAgents: {"enabled": [...]}.
func readAgents(agents *toml.Table) *Obj {
	out := newObject()
	out.set("enabled", normalizedStringList(agents, "enabled"))
	return out
}

// normalizedStringList mirrors _normalize_string_list: non-list → [], keep
// only stripped non-empty strings.
func normalizedStringList(t *toml.Table, key string) []string {
	out := []string{}
	v, ok := getKey(t, key)
	if !ok {
		return out
	}
	switch items := v.(type) {
	case []any:
		for _, item := range items {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
	case []*toml.Table:
		// Array of tables: entries are dicts, filtered out by Python.
	}
	return out
}

// readDeps: raw list of dep dicts as-is (document key order preserved).
func readDeps(data *toml.Table) any {
	out := []any{}
	v, ok := getKey(data, "deps")
	if !ok {
		return out
	}
	var items []any
	switch x := v.(type) {
	case []any:
		items = x
	case []*toml.Table:
		for _, t := range x {
			items = append(items, t)
		}
	default:
		// Not a list (e.g. [[deps.cli]] makes deps a table) → Python's
		// `if not isinstance(raw_deps, list): return []`.
		return out
	}
	for _, item := range items {
		if t, ok := item.(*toml.Table); ok {
			out = append(out, t)
		}
	}
	return out
}

// readMcp: {name: normalized} in document order.
func readMcp(data *toml.Table) *Obj {
	out := newObject()
	mcp := tableOr(data, "mcp")
	if mcp == nil {
		return out
	}
	for _, name := range mcp.Keys() {
		out.set(name, normalizeMcpServer(tableOr(mcp, name)))
	}
	return out
}

// normalizeMcpServer mirrors _normalize_mcp_server: fixed key order
// (command, args, env, timeout, [enabled]) then every other key verbatim.
func normalizeMcpServer(raw *toml.Table) *Obj {
	out := newObject()

	// command: str/list verbatim, everything else (including absent) → null.
	if cmd, ok := getKey(raw, "command"); ok {
		switch cmd.(type) {
		case string, []any, []*toml.Table:
			out.set("command", cmd)
		default:
			out.set("command", nil)
		}
	} else {
		out.set("command", nil)
	}

	// args: list verbatim, else [].
	if a, ok := getKey(raw, "args"); ok {
		switch x := a.(type) {
		case []any, []*toml.Table:
			out.set("args", x)
		default:
			out.set("args", []any{})
		}
	} else {
		out.set("args", []any{})
	}

	// env: dict verbatim; list → {name: "$name"}; missing/wrong type →
	// fall back to "environment", else {}.
	envVal, hasEnv := getKey(raw, "env")
	if !hasEnv || !isMappingOrList(envVal) {
		if alt, ok := getKey(raw, "environment"); ok && isMappingOrList(alt) {
			envVal, hasEnv = alt, true
		} else {
			envVal, hasEnv = nil, false
		}
	}
	out.set("env", normalizeEnv(envVal, hasEnv))

	// timeout: int (bool is NOT int in Python) or null.
	if tv, ok := getKey(raw, "timeout"); ok {
		if n, isInt := tv.(int64); isInt {
			out.set("timeout", n)
		} else {
			out.set("timeout", nil)
		}
	} else {
		out.set("timeout", nil)
	}

	// enabled: only present when a bool.
	if ev, ok := getKey(raw, "enabled"); ok {
		if b, isBool := ev.(bool); isBool {
			out.set("enabled", b)
		}
	}

	// Every other key verbatim in original order.
	if raw != nil {
		for _, k := range raw.Keys() {
			switch k {
			case "command", "args", "env", "environment", "timeout", "enabled":
				continue
			}
			v, _ := raw.Get(k)
			out.set(k, v)
		}
	}
	return out
}

// isMappingOrList reports whether v is a TOML dict or list (Python dict/list).
func isMappingOrList(v any) bool {
	switch v.(type) {
	case *toml.Table, []any, []*toml.Table:
		return true
	}
	return false
}

// normalizeEnv mirrors _normalize_env: dict verbatim; list of strings →
// {stripped_name: "$stripped_name"}; else {}.
func normalizeEnv(v any, has bool) any {
	out := newObject()
	if !has {
		return out
	}
	switch x := v.(type) {
	case *toml.Table:
		return x // dict verbatim; document order preserved by *toml.Table
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok {
				if name := strings.TrimSpace(s); name != "" {
					out.set(name, "$"+name)
				}
			}
		}
		return out
	case []*toml.Table:
		// List of dicts: items are not strings, filtered out by Python.
		return out
	}
	return out
}

// readRecipes: {id: {enabled, version, config}}.
func readRecipes(data *toml.Table) *Obj {
	out := newObject()
	recipes := tableOr(data, "recipes")
	if recipes == nil {
		return out
	}
	for _, id := range recipes.Keys() {
		v, _ := recipes.Get(id)
		value, ok := v.(*toml.Table)
		if !ok {
			continue
		}
		entry := newObject()
		if b, ok := value.Bool("enabled"); ok {
			entry.set("enabled", b)
		} else {
			entry.set("enabled", false)
		}
		if v, ok := getKey(value, "version"); ok && v != nil {
			entry.set("version", pyStr(v))
		} else {
			entry.set("version", "")
		}
		if cfg := tableOr(value, "config"); cfg != nil {
			entry.set("config", cfg)
		} else {
			entry.set("config", newObject())
		}
		out.set(id, entry)
	}
	return out
}

// readBindings: [{capability, recipe}] where both are strings.
func readBindings(data *toml.Table) any {
	out := []any{}
	v, ok := getKey(data, "bindings")
	if !ok {
		return out
	}
	var items []any
	switch x := v.(type) {
	case []any:
		items = x
	case []*toml.Table:
		for _, t := range x {
			items = append(items, t)
		}
	default:
		return out
	}
	for _, item := range items {
		t, ok := item.(*toml.Table)
		if !ok {
			continue
		}
		capability, okCap := t.String("capability")
		recipe, okRecipe := t.String("recipe")
		if okCap && okRecipe {
			b := newObject()
			b.set("capability", capability)
			b.set("recipe", recipe)
			out = append(out, b)
		}
	}
	return out
}
