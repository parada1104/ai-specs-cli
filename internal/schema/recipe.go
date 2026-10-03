// Package schema ports lib/_internal/recipe_schema.py (recipe.toml
// dataclasses + validation) to Go with character-exact validation error
// strings (FROZEN surface per docs/go-migration-parity-contract.md).
//
// Every helper mirrors its Python counterpart function by function; Python
// type names in messages (str/int/float/bool/list/dict/NoneType) are mapped
// from Go TOML value kinds, and Python str()/repr() coercions are reproduced.
package schema

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/toml"
)

// ValidationError carries an exact Python-side validation message.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

// NewValidationError builds a RecipeValidationError with a literal message.
func NewValidationError(msg string) *ValidationError { return &ValidationError{msg} }

func ve(format string, args ...any) *ValidationError {
	return &ValidationError{fmt.Sprintf(format, args...)}
}

// ContributableSections are the [provides.brief] sections a recipe may contribute.
var ContributableSections = []string{
	"runtime_flow",
	"context_sources",
	"conflict_policy",
	"workflow_rules",
	"useful_commands",
	"mcp_descriptions",
}

// ProjectOnlySections are brief sections recipes MUST NOT contribute.
var ProjectOnlySections = []string{"intro", "purpose"}

// KnownRuntimeHookEvents are the abstract runtime-hook events the product owns
// (mapped per-harness downstream).
var KnownRuntimeHookEvents = []string{"pre-tool-use", "post-tool-use", "session-start", "stop"}

// StructuredListMax is the upper bound on a structured array such as the
// reconcile expectations list.
const StructuredListMax = 32

// shapeTable is an order-preserving table shape declaration (Python relies on
// dict insertion order when checking shape keys).
type shapeTable struct {
	order   []string
	entries map[string]any
}

var reconcileExpectationShape = &shapeTable{
	order: []string{"event", "property", "config_field", "config_field_when_set"},
	entries: map[string]any{
		"event":                 "string",
		"property":              "string",
		"config_field":          "string",
		"config_field_when_set": "string",
	},
}

// StructuredConfigShapes declares the shape of structured (table) config
// sections; a section without a declared shape is a no-op.
var StructuredConfigShapes = map[string]any{
	"reconcile": &shapeTable{
		order: []string{"scope_field", "max_age_seconds", "expectations"},
		entries: map[string]any{
			"scope_field":     "string",
			"max_age_seconds": "integer",
			"expectations":    []any{reconcileExpectationShape},
		},
	},
}

// --- mirrored dataclasses ---

type BriefFragment struct {
	Text any // stored verbatim; may be any TOML value
	Key  any // nil (None) for the string-array form
}

type BriefFragments struct {
	RuntimeFlow     []BriefFragment
	ContextSources  []BriefFragment
	ConflictPolicy  []BriefFragment
	WorkflowRules   []BriefFragment
	UsefulCommands  []BriefFragment
	McpDescriptions []BriefFragment
}

type SkillRef struct {
	ID     string
	Source string // "bundled" or "dep"
	URL    string
	Path   string
}

type CommandRef struct {
	ID   string
	Path string
}

type McpPreset struct {
	ID     string
	Config map[string]any
}

type TemplateRef struct {
	Source       string
	Target       string
	Condition    string
	UpdatePolicy string
}

type DocRef struct {
	Source string
	Target string
}

type Capability struct{ ID string }

type Hook struct {
	Event  string
	Action string
}

type RuntimeHook struct {
	ID          string
	Event       string
	Script      string
	Matcher     string
	Blocking    bool
	Description string
}

type ConfigField struct {
	Required   bool
	Type       string
	Default    any
	Validation map[string]any
	Enum       []string
	HelpText   string
}

type ConfigTable struct {
	Shape  any // declarative authority (mirrors STRUCTURED_CONFIG_SHAPES)
	Values map[string]any
}

type ConfigSchema struct {
	Fields map[string]*ConfigField
	Extra  map[string]any
	Tables map[string]*ConfigTable
}

type CliDep struct {
	Binary        string
	Purpose       string
	Required      bool
	InstallURL    string
	VersionCheck  string
	MinVersion    string
	Installer     string
	Repository    string
	ReleasePolicy string
}

type InitWorkflow struct {
	Prompt        string
	Description   string
	NeedsManifest bool
	NeedsMCP      []string
}

type Recipe struct {
	ID             string
	Name           string
	Description    string
	Version        string
	Author         string
	License        string
	Tags           []string
	ConflictsWith  []string
	Skills         []*SkillRef
	Commands       []*CommandRef
	MCP            []*McpPreset
	Templates      []*TemplateRef
	Docs           []*DocRef
	Capabilities   []*Capability
	Hooks          []*Hook
	RuntimeHooks   []*RuntimeHook
	ConfigSchema   *ConfigSchema
	CliDeps        []*CliDep
	Init           *InitWorkflow
	BriefFragments *BriefFragments
}

// --- table helpers (nil-safe; a nil *Table is an absent table) ---

func get(t *toml.Table, key string) (any, bool) {
	if t == nil {
		return nil, false
	}
	return t.Get(key)
}

func getOrNil(t *toml.Table, key string) any {
	v, _ := get(t, key)
	return v
}

func getOr(t *toml.Table, key string, def any) any {
	if v, ok := get(t, key); ok {
		return v
	}
	return def
}

func keys(t *toml.Table) []string {
	if t == nil {
		return nil
	}
	return t.Keys()
}

// asList mirrors Python's isinstance(x, list): both []any (array) and
// []*toml.Table (array of tables) are lists in tomllib terms.
func asList(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []*toml.Table:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = e
		}
		return out, true
	}
	return nil, false
}

// rawTableMap copies a table's entries as raw TOML values (mirrors dict(t)).
func rawTableMap(t *toml.Table) map[string]any {
	m := make(map[string]any, len(keys(t)))
	for _, k := range keys(t) {
		v, _ := get(t, k)
		m[k] = v
	}
	return m
}

// --- Python str()/repr()/type-name emulation ---

// pyTypeName maps a Go TOML value kind to the Python type name tomllib
// values would report (FROZEN message surface).
func pyTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case string:
		return "str"
	case int64:
		return "int"
	case float64:
		return "float"
	case bool:
		return "bool"
	case []any, []*toml.Table:
		return "list"
	case *toml.Table:
		return "dict"
	default:
		return "str"
	}
}

// pyReprMaxDepth pins the Python recursion boundary for repr()/str() of
// nested values. Probe (Python 3.14.7, default 8 MiB stack): repr() of a
// nested list succeeds to depth ~69709 and raises RecursionError at ~69710.
// The cap sits above that failure point so Go never errors where Python
// succeeds on the probe machine, and returns a descriptive error instead of
// crashing at extreme depths. Self-referential *toml.Table sharing (not
// producible by toml.Parse, which builds trees) renders Python's ellipsis
// form instead.
const pyReprMaxDepth = 100000

// pyRepr mirrors Python repr() for the value kinds reachable from TOML.
// An error means the nesting exceeds pyReprMaxDepth where Python raises
// RecursionError.
func pyRepr(v any) (string, error) {
	return pyReprDepth(v, 0, nil)
}

// pyReprDepth is the shared recursion core: depth-capped (descriptive error
// instead of a stack overflow) and cycle-aware for *toml.Table (Python's
// '{...}' ellipsis for a table re-entered on the active path).
func pyReprDepth(v any, depth int, active map[*toml.Table]bool) (string, error) {
	if depth > pyReprMaxDepth {
		return "", fmt.Errorf("value nesting exceeds the maximum recursion depth (%d)", pyReprMaxDepth)
	}
	switch x := v.(type) {
	case nil:
		return "None", nil
	case string:
		return pyReprString(x), nil
	case bool:
		if x {
			return "True", nil
		}
		return "False", nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		return pyFloatStr(x), nil
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			p, err := pyReprDepth(e, depth+1, active)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case []*toml.Table:
		parts := make([]string, len(x))
		for i, e := range x {
			p, err := pyReprDepth(e, depth+1, active)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *toml.Table:
		if x == nil {
			return "{}", nil
		}
		if active[x] {
			return "{...}", nil
		}
		if active == nil {
			active = make(map[*toml.Table]bool)
		}
		active[x] = true
		defer delete(active, x)
		tableKeys := x.Keys()
		parts := make([]string, 0, len(tableKeys))
		for _, k := range tableKeys {
			val, _ := x.Get(k)
			p, err := pyReprDepth(val, depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case map[string]any:
		// ponytail: maps normalized by rawTableMap lose document order; emit
		// in sorted key order — byte-identical to Python's insertion order
		// whenever the producer wrote keys in sorted order.
		mapKeys := make([]string, 0, len(x))
		for k := range x {
			mapKeys = append(mapKeys, k)
		}
		sort.Strings(mapKeys)
		parts := make([]string, 0, len(mapKeys))
		for _, k := range mapKeys {
			p, err := pyReprDepth(x[k], depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	default:
		return pyReprString(fmt.Sprintf("%v", v)), nil
	}
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

// pyStr mirrors Python str(): strings pass through, everything else takes its
// repr form. An error means the nesting exceeds pyReprMaxDepth where Python
// raises RecursionError inside str().
func pyStr(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return pyReprDepth(v, 0, nil)
}

// pyFloatStr renders f like Python str(float) (shortest round-trip repr):
// scientific notation when the decimal point position is <= -4 or > 16,
// otherwise decimal notation with a forced ".0" on integral values.
// Replicated from internal/config/json.go (formatPyFloat), which is outside
// this package's allowed edit surface.
func pyFloatStr(f float64) string {
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
	e := strconv.FormatFloat(a, 'e', -1, 64) // e.g. "1.5e+22"
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1

	var s string
	if decpt <= -4 || decpt > 16 {
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		expStr := strconv.Itoa(exp10)
		if exp10 < 0 {
			expStr = strconv.Itoa(-exp10)
		}
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

// --- parse/validate helpers (ports of the Python _-prefixed functions) ---

func requireString(data *toml.Table, key, context string) (string, error) {
	value, _ := get(data, key)
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", ve("%s: missing or invalid required field '%s'", context, key)
	}
	return strings.TrimSpace(s), nil
}

func optStr(data *toml.Table, key, context string) (string, error) {
	value, ok := get(data, key)
	if !ok {
		return "", nil
	}
	s, isStr := value.(string)
	if !isStr {
		return "", ve("%s.%s: expected string, got %s", context, key, pyTypeName(value))
	}
	return strings.TrimSpace(s), nil
}

func parseTags(raw any, context string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	rawList, ok := asList(raw)
	if !ok {
		return nil, ve("%s: 'tags' must be an array of strings, got %s", context, pyTypeName(raw))
	}
	out := []string{}
	for idx, item := range rawList {
		s, isStr := item.(string)
		if !isStr {
			return nil, ve("%s: 'tags'[%d] must be a string, got %s", context, idx, pyTypeName(item))
		}
		if strings.TrimSpace(s) == "" {
			return nil, ve("%s: 'tags'[%d] must be a non-empty string", context, idx)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseConflictsWith(raw any, recipeID, context string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	rawList, ok := asList(raw)
	if !ok {
		return nil, ve("%s: 'conflicts_with' must be an array of strings, got %s", context, pyTypeName(raw))
	}
	out := []string{}
	for idx, item := range rawList {
		s, isStr := item.(string)
		if !isStr {
			return nil, ve("%s: 'conflicts_with'[%d] must be a string, got %s", context, idx, pyTypeName(item))
		}
		if strings.TrimSpace(s) == "" {
			return nil, ve("%s: 'conflicts_with'[%d] must be a non-empty string", context, idx)
		}
		if s == recipeID {
			return nil, ve("%s: 'conflicts_with' must not reference the recipe itself ('%s')", context, recipeID)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseSkills(raw any, context string) ([]*SkillRef, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*SkillRef{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.skills[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		skillID, err := requireString(tbl, "id", itemCtx)
		if err != nil {
			return nil, err
		}
		source, err := requireString(tbl, "source", itemCtx)
		if err != nil {
			return nil, err
		}
		url, err := pyStr(getOr(tbl, "url", ""))
		if err != nil {
			return nil, err
		}
		path, err := pyStr(getOr(tbl, "path", ""))
		if err != nil {
			return nil, err
		}
		if source == "dep" && url == "" {
			return nil, ve("%s: source='dep' requires 'url'", itemCtx)
		}
		out = append(out, &SkillRef{ID: skillID, Source: source, URL: url, Path: path})
	}
	return out, nil
}

func parseCommands(raw any, context string) ([]*CommandRef, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*CommandRef{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.commands[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		cmdID, err := requireString(tbl, "id", itemCtx)
		if err != nil {
			return nil, err
		}
		path, err := requireString(tbl, "path", itemCtx)
		if err != nil {
			return nil, err
		}
		out = append(out, &CommandRef{ID: cmdID, Path: path})
	}
	return out, nil
}

func parseMcp(raw any, context string) ([]*McpPreset, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*McpPreset{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.mcp[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		mcpID, err := requireString(tbl, "id", itemCtx)
		if err != nil {
			return nil, err
		}
		config := map[string]any{}
		for _, k := range keys(tbl) {
			if k != "id" {
				v, _ := get(tbl, k)
				config[k] = v
			}
		}
		out = append(out, &McpPreset{ID: mcpID, Config: config})
	}
	return out, nil
}

func parseTemplates(raw any, context string) ([]*TemplateRef, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	allowedPolicies := map[string]bool{"auto": true, "confirm": true, "never-force": true}
	out := []*TemplateRef{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.templates[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		source, err := requireString(tbl, "source", itemCtx)
		if err != nil {
			return nil, err
		}
		target, err := requireString(tbl, "target", itemCtx)
		if err != nil {
			return nil, err
		}
		condition, err := pyStr(getOr(tbl, "condition", "not_exists"))
		if err != nil {
			return nil, err
		}
		policyRaw := getOr(tbl, "update_policy", "auto")
		policy, isStr := policyRaw.(string)
		if !isStr || !allowedPolicies[policy] {
			policyRepr, err := pyRepr(policyRaw)
			if err != nil {
				// Python raises RecursionError inside repr(); surface the
				// same condition as a plain descriptive error.
				return nil, err
			}
			return nil, ve("%s.update_policy: invalid value %s; expected auto | confirm | never-force",
				itemCtx, policyRepr)
		}
		out = append(out, &TemplateRef{Source: source, Target: target, Condition: condition, UpdatePolicy: policy})
	}
	return out, nil
}

func parseDocs(raw any, context string) ([]*DocRef, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*DocRef{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.docs[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		source, err := requireString(tbl, "source", itemCtx)
		if err != nil {
			return nil, err
		}
		target, err := requireString(tbl, "target", itemCtx)
		if err != nil {
			return nil, err
		}
		out = append(out, &DocRef{Source: source, Target: target})
	}
	return out, nil
}

// resolvePy mirrors Path.resolve(strict=False) (Python 3.14): follow
// symlinks in every existing component, keep the non-existent tail.
// filepath.EvalSymlinks alone fails when the final component is missing or a
// dangling symlink, so resolve the deepest existing ancestor (following a
// dangling final symlink to its target) and re-join the remainder — the same
// algorithm internal/target uses for its resolve() surface.
func resolvePy(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	// ponytail: depth cap 40 mirrors kernel MAXSYMLINKS; deeper chains
	// degrade to the un-resolved tail instead of looping.
	return resolvePyDepth(abs, 40)
}

func resolvePyDepth(abs string, depth int) string {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	if depth > 0 {
		if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if tgt, err := os.Readlink(abs); err == nil {
				if !filepath.IsAbs(tgt) {
					tgt = filepath.Join(filepath.Dir(abs), tgt)
				}
				return resolvePyDepth(tgt, depth-1)
			}
		}
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs
	}
	return filepath.Join(resolvePyDepth(parent, depth-1), filepath.Base(abs))
}

func withinDir(root, target string) bool {
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

func parseRuntimeHooks(raw any, context, recipeDir string) ([]*RuntimeHook, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*RuntimeHook{}
	seen := map[string]bool{}
	for idx, item := range rawList {
		ctx := fmt.Sprintf("%s.hooks[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", ctx, pyTypeName(item))
		}
		hookID, err := requireString(tbl, "id", ctx)
		if err != nil {
			return nil, err
		}
		if seen[hookID] {
			return nil, ve("%s: duplicate hook id '%s'", ctx, hookID)
		}
		seen[hookID] = true
		event, err := requireString(tbl, "event", ctx)
		if err != nil {
			return nil, err
		}
		known := false
		for _, e := range KnownRuntimeHookEvents {
			if event == e {
				known = true
				break
			}
		}
		if !known {
			return nil, ve("%s: unknown event '%s'; known events: %s",
				ctx, event, strings.Join(KnownRuntimeHookEvents, ", "))
		}
		script, err := requireString(tbl, "script", ctx)
		if err != nil {
			return nil, err
		}

		if filepath.IsAbs(script) {
			return nil, ve("%s.script: hook script paths must be relative to the recipe directory (got absolute path '%s')",
				ctx, script)
		}
		if recipeDir != "" {
			// Python parity: root and target both go through Path.resolve(),
			// which follows symlinks in INTERMEDIATE components too; the fully
			// resolved target is what the containment compares.
			root := resolvePy(recipeDir)
			target := resolvePy(filepath.Join(recipeDir, filepath.Clean(script)))
			if !withinDir(root, target) {
				return nil, ve("%s.script: hook script paths must stay inside the recipe directory (got '%s')",
					ctx, script)
			}
		} else {
			// No recipe_dir to resolve against; still reject obvious ../ escapes.
			for _, part := range strings.Split(script, "/") {
				if part == ".." {
					return nil, ve("%s.script: hook script paths must stay inside the recipe directory (got '%s')",
						ctx, script)
				}
			}
		}

		matcher, err := pyStr(getOr(tbl, "matcher", ""))
		if err != nil {
			return nil, err
		}
		blockingRaw := getOr(tbl, "blocking", false)
		blocking, isBool := blockingRaw.(bool)
		if !isBool {
			return nil, ve("%s.blocking: expected boolean, got %s", ctx, pyTypeName(blockingRaw))
		}
		description, err := pyStr(getOr(tbl, "description", ""))
		if err != nil {
			return nil, err
		}
		out = append(out, &RuntimeHook{
			ID:          hookID,
			Event:       event,
			Script:      script,
			Matcher:     matcher,
			Blocking:    blocking,
			Description: description,
		})
	}
	return out, nil
}

func parseCapabilities(raw any, context string) ([]*Capability, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*Capability{}
	seen := map[string]bool{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.capabilities[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		capID, err := requireString(tbl, "id", itemCtx)
		if err != nil {
			return nil, err
		}
		if seen[capID] {
			return nil, ve("%s: duplicate capability id '%s'", itemCtx, capID)
		}
		seen[capID] = true
		out = append(out, &Capability{ID: capID})
	}
	return out, nil
}

func parseHooks(raw any, context string) ([]*Hook, error) {
	rawList, ok := asList(raw)
	if !ok {
		return nil, nil
	}
	out := []*Hook{}
	for idx, item := range rawList {
		itemCtx := fmt.Sprintf("%s.hooks[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected object, got %s", itemCtx, pyTypeName(item))
		}
		event, err := requireString(tbl, "event", itemCtx)
		if err != nil {
			return nil, err
		}
		action, err := requireString(tbl, "action", itemCtx)
		if err != nil {
			return nil, err
		}
		out = append(out, &Hook{Event: event, Action: action})
	}
	return out, nil
}

func parseCliDeps(raw any, context string) ([]*CliDep, error) {
	if raw == nil {
		return nil, nil
	}
	rawList, ok := asList(raw)
	if !ok {
		return nil, ve("%s: expected array of tables, got %s", context, pyTypeName(raw))
	}
	allowed := map[string]bool{
		"binary": true, "purpose": true, "required": true, "install_url": true,
		"version_check": true, "min_version": true, "installer": true,
		"repository": true, "release_policy": true,
	}
	out := []*CliDep{}
	for idx, item := range rawList {
		ctx := fmt.Sprintf("%s[%d]", context, idx)
		tbl, isTbl := item.(*toml.Table)
		if !isTbl {
			return nil, ve("%s: expected table, got %s", ctx, pyTypeName(item))
		}
		for _, k := range keys(tbl) {
			if !allowed[k] {
				return nil, ve("%s: unknown key '%s'", ctx, k)
			}
		}
		binary, err := requireString(tbl, "binary", ctx)
		if err != nil {
			return nil, err
		}
		purpose, err := requireString(tbl, "purpose", ctx)
		if err != nil {
			return nil, err
		}
		requiredRaw := getOr(tbl, "required", true)
		required, isBool := requiredRaw.(bool)
		if !isBool {
			return nil, ve("%s.required: expected boolean, got %s", ctx, pyTypeName(requiredRaw))
		}
		installURL, err := optStr(tbl, "install_url", ctx)
		if err != nil {
			return nil, err
		}
		versionCheck, err := optStr(tbl, "version_check", ctx)
		if err != nil {
			return nil, err
		}
		minVersion, err := optStr(tbl, "min_version", ctx)
		if err != nil {
			return nil, err
		}
		installer, err := optStr(tbl, "installer", ctx)
		if err != nil {
			return nil, err
		}
		repository, err := optStr(tbl, "repository", ctx)
		if err != nil {
			return nil, err
		}
		releasePolicy, err := optStr(tbl, "release_policy", ctx)
		if err != nil {
			return nil, err
		}
		if installer != "" && installer != "github-release" {
			return nil, ve("%s.installer: unsupported installer '%s'", ctx, installer)
		}
		if installer == "github-release" {
			if repository != "parada1104/jinna-provider" {
				return nil, ve("%s.repository: only 'parada1104/jinna-provider' is allowed", ctx)
			}
			if releasePolicy != "latest-stable" {
				return nil, ve("%s.release_policy: expected 'latest-stable'", ctx)
			}
		} else if repository != "" || releasePolicy != "" {
			return nil, ve("%s: repository/release_policy require installer 'github-release'", ctx)
		}
		out = append(out, &CliDep{
			Binary:        binary,
			Purpose:       purpose,
			Required:      required,
			InstallURL:    installURL,
			VersionCheck:  versionCheck,
			MinVersion:    minVersion,
			Installer:     installer,
			Repository:    repository,
			ReleasePolicy: releasePolicy,
		})
	}
	return out, nil
}

func parseConfig(raw any, context string) (*ConfigSchema, error) {
	tbl, ok := raw.(*toml.Table)
	if !ok {
		return &ConfigSchema{
			Fields: map[string]*ConfigField{},
			Extra:  map[string]any{},
			Tables: map[string]*ConfigTable{},
		}, nil
	}
	fields := map[string]*ConfigField{}
	extra := map[string]any{}
	tables := map[string]*ConfigTable{}
	for _, key := range keys(tbl) {
		value, _ := get(tbl, key)
		sub, isTbl := value.(*toml.Table)
		if !isTbl {
			return nil, ve("%s.config.%s: expected table, got %s", context, key, pyTypeName(value))
		}
		// Structured sections are validated against their declarative shape and
		// exposed as first-class table values.
		if shape, structured := StructuredConfigShapes[key]; structured {
			if err := validateStructuredConfig(key, sub); err != nil {
				return nil, err
			}
			tables[key] = &ConfigTable{Shape: shape, Values: rawTableMap(sub)}
			continue
		}
		// Detect standard ConfigField entries by the presence of 'required'.
		// Non-standard config sections (e.g., board_isolation) go to extra.
		if _, hasRequired := get(sub, "required"); !hasRequired {
			extra[key] = rawTableMap(sub)
			continue
		}
		requiredRaw, _ := get(sub, "required")
		required, isBool := requiredRaw.(bool)
		if !isBool {
			return nil, ve("%s.config.%s: missing or invalid 'required' (must be boolean)", context, key)
		}
		fieldType, err := pyStr(getOr(sub, "type", ""))
		if err != nil {
			return nil, err
		}
		// Catalog recipes historically use type = "boolean"; wizard expects "bool".
		if fieldType == "boolean" {
			fieldType = "bool"
		}
		defVal := getOrNil(sub, "default")

		allowedFieldKeys := map[string]bool{
			"required": true, "type": true, "default": true,
			"validation": true, "enum": true, "help_text": true,
		}
		for _, fk := range keys(sub) {
			if !allowedFieldKeys[fk] {
				return nil, ve("%s.config.%s: unknown key '%s'", context, key, fk)
			}
		}

		var enumValues []string
		enumRaw := getOrNil(sub, "enum")
		if enumRaw != nil {
			enumList, isList := enumRaw.([]any)
			if !isList {
				return nil, ve("%s.config.%s.enum: expected array of strings, got %s", context, key, pyTypeName(enumRaw))
			}
			enumValues = []string{}
			for idx, item := range enumList {
				s, isStr := item.(string)
				if !isStr || strings.TrimSpace(s) == "" {
					return nil, ve("%s.config.%s.enum[%d]: expected non-empty string", context, key, idx)
				}
				enumValues = append(enumValues, s)
			}
		}

		validation := map[string]any{}
		validationRaw := getOrNil(sub, "validation")
		if validationRaw != nil {
			valTbl, isTbl2 := validationRaw.(*toml.Table)
			if isTbl2 {
				for _, vk := range keys(valTbl) {
					if vk != "regex" {
						return nil, ve("%s.config.%s.validation: unknown key '%s'", context, key, vk)
					}
				}
				validation = rawTableMap(valTbl)
			} else {
				return nil, ve("%s.config.%s.validation: expected table, got %s", context, key, pyTypeName(validationRaw))
			}
		}
		helpText, err := optStr(sub, "help_text", fmt.Sprintf("%s.config.%s", context, key))
		if err != nil {
			return nil, err
		}

		fields[key] = &ConfigField{
			Required:   required,
			Type:       fieldType,
			Default:    defVal,
			Validation: validation,
			Enum:       enumValues,
			HelpText:   helpText,
		}
	}
	return &ConfigSchema{Fields: fields, Extra: extra, Tables: tables}, nil
}

func checkShapeScalar(context string, value any, kind string) error {
	switch kind {
	case "string":
		if _, ok := value.(string); !ok {
			return ve("%s: expected string, got %s", context, pyTypeName(value))
		}
	case "integer":
		_, isInt := value.(int64)
		_, isBool := value.(bool)
		if !isInt || isBool {
			return ve("%s: expected integer, got %s", context, pyTypeName(value))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return ve("%s: expected boolean, got %s", context, pyTypeName(value))
		}
	default:
		return ve("%s: invalid scalar type '%s'", context, kind)
	}
	return nil
}

func checkShapeValue(context string, value, shape any) error {
	switch sh := shape.(type) {
	case string:
		return checkShapeScalar(context, value, sh)
	case []any:
		rawList, ok := asList(value)
		if !ok {
			return ve("%s: expected array, got %s", context, pyTypeName(value))
		}
		if len(rawList) > StructuredListMax {
			return ve("%s: expected at most %d entries, got %d", context, StructuredListMax, len(rawList))
		}
		for idx, item := range rawList {
			if err := checkShapeValue(fmt.Sprintf("%s[%d]", context, idx), item, sh[0]); err != nil {
				return err
			}
		}
		return nil
	case *shapeTable:
		tbl, ok := value.(*toml.Table)
		if !ok {
			return ve("%s: expected table, got %s", context, pyTypeName(value))
		}
		for _, k := range keys(tbl) {
			if _, known := sh.entries[k]; !known {
				return ve("%s: unknown key '%s'", context, k)
			}
		}
		for _, k := range sh.order {
			if v, has := get(tbl, k); has {
				if err := checkShapeValue(fmt.Sprintf("%s.%s", context, k), v, sh.entries[k]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ve("%s: invalid shape declaration", context)
}

// validateStructuredConfig validates a [config.<section>] table against its
// declared shape; a section without a declared shape is a no-op.
func validateStructuredConfig(section string, value any) error {
	shape, ok := StructuredConfigShapes[section]
	if !ok {
		return nil
	}
	return checkShapeValue(fmt.Sprintf("[config.%s]", section), value, shape)
}

func parseInit(raw any, context, recipeDir string) (*InitWorkflow, error) {
	if raw == nil {
		return nil, nil
	}
	tbl, ok := raw.(*toml.Table)
	if !ok {
		return nil, ve("%s: expected table, got %s", context, pyTypeName(raw))
	}

	allowed := map[string]bool{"prompt": true, "description": true, "needs_manifest": true, "needs_mcp": true}
	for _, k := range keys(tbl) {
		if !allowed[k] {
			return nil, ve("%s: unsupported init field '%s'", context, k)
		}
	}

	prompt, err := requireString(tbl, "prompt", context+".prompt")
	if err != nil {
		return nil, err
	}

	descriptionRaw := getOr(tbl, "description", "")
	description, isStr := descriptionRaw.(string)
	if !isStr {
		return nil, ve("%s.description: expected string, got %s", context, pyTypeName(descriptionRaw))
	}
	description = strings.TrimSpace(description)

	needsManifestRaw := getOr(tbl, "needs_manifest", false)
	needsManifest, isBool := needsManifestRaw.(bool)
	if !isBool {
		return nil, ve("%s.needs_manifest: expected boolean, got %s", context, pyTypeName(needsManifestRaw))
	}

	needsMcpRaw := getOr(tbl, "needs_mcp", []any{})
	rawList, isList := asList(needsMcpRaw)
	if !isList {
		return nil, ve("%s.needs_mcp: expected array of strings", context)
	}
	needsMcp := []string{}
	for idx, item := range rawList {
		s, isStr := item.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			return nil, ve("%s.needs_mcp[%d]: expected non-empty string", context, idx)
		}
		needsMcp = append(needsMcp, strings.TrimSpace(s))
	}

	if recipeDir != "" {
		if filepath.IsAbs(prompt) {
			return nil, ve("%s.prompt: init prompt paths must be relative to the recipe directory", context)
		}
		root := resolvePy(recipeDir)
		target := resolvePy(filepath.Join(recipeDir, filepath.Clean(prompt)))
		if !withinDir(root, target) {
			return nil, ve("%s.prompt: init prompt paths must stay inside the recipe directory", context)
		}
		// Python stats the RESOLVED target (target.exists()/target.is_file()).
		info, statErr := os.Stat(target)
		if statErr != nil {
			return nil, ve("%s.prompt: init prompt file not found: %s", context, prompt)
		}
		if info.IsDir() {
			return nil, ve("%s.prompt: init prompt path must be a file: %s", context, prompt)
		}
	}

	return &InitWorkflow{
		Prompt:        prompt,
		Description:   description,
		NeedsManifest: needsManifest,
		NeedsMCP:      needsMcp,
	}, nil
}

func parseBriefFragments(raw any, context string) (*BriefFragments, error) {
	if raw == nil {
		return nil, nil
	}
	tbl, ok := raw.(*toml.Table)
	if !ok {
		return nil, nil
	}

	projectOnly := map[string]bool{}
	for _, s := range ProjectOnlySections {
		projectOnly[s] = true
	}
	contributable := map[string]bool{}
	for _, s := range ContributableSections {
		contributable[s] = true
	}

	result := map[string][]BriefFragment{}
	for _, name := range keys(tbl) {
		value, _ := get(tbl, name)
		ctxSec := context + "." + name

		if projectOnly[name] {
			return nil, ve("%s: section is project-only; recipes MUST NOT contribute it", ctxSec)
		}
		if !contributable[name] {
			return nil, ve("%s: unknown section '%s'; valid: %s",
				ctxSec, name, strings.Join(ContributableSections, ", "))
		}

		rawList, isList := asList(value)
		if !isList {
			return nil, ve("%s: expected array, got %s", ctxSec, pyTypeName(value))
		}

		// Mixed-form detection: list must be all-strings OR all-dicts.
		hasStrings, hasDicts := false, false
		for _, item := range rawList {
			switch item.(type) {
			case string:
				hasStrings = true
			case *toml.Table:
				hasDicts = true
			}
		}
		if hasStrings && hasDicts {
			return nil, ve("%s: section '%s' mixes string-array and inline-table forms", context, name)
		}

		fragments := []BriefFragment{}
		for idx, item := range rawList {
			ctxItem := fmt.Sprintf("%s[%d]", ctxSec, idx)
			switch x := item.(type) {
			case string:
				fragments = append(fragments, BriefFragment{Text: x})
			case *toml.Table:
				if _, has := get(x, "text"); !has {
					return nil, ve("%s: missing required field 'text'", ctxItem)
				}
				if _, has := get(x, "key"); !has {
					return nil, ve("%s: missing required field 'key'", ctxItem)
				}
				textVal, _ := get(x, "text")
				keyVal, _ := get(x, "key")
				fragments = append(fragments, BriefFragment{Text: textVal, Key: keyVal})
			default:
				return nil, ve("%s: expected string or inline-table, got %s", ctxItem, pyTypeName(item))
			}
		}
		result[name] = fragments
	}

	// Build BriefFragments with only the sections that were declared.
	bf := &BriefFragments{}
	if fs, ok := result["runtime_flow"]; ok {
		bf.RuntimeFlow = fs
	}
	if fs, ok := result["context_sources"]; ok {
		bf.ContextSources = fs
	}
	if fs, ok := result["conflict_policy"]; ok {
		bf.ConflictPolicy = fs
	}
	if fs, ok := result["workflow_rules"]; ok {
		bf.WorkflowRules = fs
	}
	if fs, ok := result["useful_commands"]; ok {
		bf.UsefulCommands = fs
	}
	if fs, ok := result["mcp_descriptions"]; ok {
		bf.McpDescriptions = fs
	}
	return bf, nil
}

// ValidateRecipeToml validates a raw parsed recipe.toml and returns a Recipe.
// An empty recipeDir means recipe_dir=None (no filesystem resolution; ".."
// escapes rejected lexically).
func ValidateRecipeToml(data *toml.Table, recipeDir string) (*Recipe, error) {
	var recipeTable *toml.Table
	if recipeRaw, has := get(data, "recipe"); has {
		rt, ok := recipeRaw.(*toml.Table)
		if !ok {
			return nil, ve("[recipe] must be a table")
		}
		recipeTable = rt
	}

	ctx := "[recipe]"
	recipeID, err := requireString(recipeTable, "id", ctx)
	if err != nil {
		return nil, err
	}
	name, err := requireString(recipeTable, "name", ctx)
	if err != nil {
		return nil, err
	}
	description, err := requireString(recipeTable, "description", ctx)
	if err != nil {
		return nil, err
	}
	version, err := requireString(recipeTable, "version", ctx)
	if err != nil {
		return nil, err
	}
	author, err := pyStr(getOr(recipeTable, "author", ""))
	if err != nil {
		return nil, err
	}
	license, err := pyStr(getOr(recipeTable, "license", ""))
	if err != nil {
		return nil, err
	}
	tags, err := parseTags(getOrNil(recipeTable, "tags"), ctx)
	if err != nil {
		return nil, err
	}
	conflictsWith, err := parseConflictsWith(getOrNil(recipeTable, "conflicts_with"), recipeID, ctx)
	if err != nil {
		return nil, err
	}

	var provides, deps *toml.Table
	if p, has := get(data, "provides"); has {
		if pt, isTbl := p.(*toml.Table); isTbl {
			provides = pt
		}
	}
	if d, has := get(data, "deps"); has {
		if dt, isTbl := d.(*toml.Table); isTbl {
			deps = dt
		}
	}

	skills, err := parseSkills(getOrNil(provides, "skills"), "[provides]")
	if err != nil {
		return nil, err
	}
	commands, err := parseCommands(getOrNil(provides, "commands"), "[provides]")
	if err != nil {
		return nil, err
	}
	mcp, err := parseMcp(getOrNil(provides, "mcp"), "[provides]")
	if err != nil {
		return nil, err
	}
	templates, err := parseTemplates(getOrNil(provides, "templates"), "[provides]")
	if err != nil {
		return nil, err
	}
	docs, err := parseDocs(getOrNil(provides, "docs"), "[provides]")
	if err != nil {
		return nil, err
	}
	runtimeHooks, err := parseRuntimeHooks(getOrNil(provides, "hooks"), "[provides]", recipeDir)
	if err != nil {
		return nil, err
	}
	capabilities, err := parseCapabilities(getOrNil(data, "capabilities"), "")
	if err != nil {
		return nil, err
	}
	hooks, err := parseHooks(getOrNil(data, "hooks"), "")
	if err != nil {
		return nil, err
	}
	configSchema, err := parseConfig(getOrNil(data, "config"), "")
	if err != nil {
		return nil, err
	}
	cliDeps, err := parseCliDeps(getOrNil(deps, "cli"), "[deps.cli]")
	if err != nil {
		return nil, err
	}
	init, err := parseInit(getOrNil(data, "init"), "[init]", recipeDir)
	if err != nil {
		return nil, err
	}
	briefFragments, err := parseBriefFragments(getOrNil(provides, "brief"), "[provides.brief]")
	if err != nil {
		return nil, err
	}

	return &Recipe{
		ID:             recipeID,
		Name:           name,
		Description:    description,
		Version:        version,
		Author:         author,
		License:        license,
		Tags:           tags,
		ConflictsWith:  conflictsWith,
		Skills:         skills,
		Commands:       commands,
		MCP:            mcp,
		Templates:      templates,
		Docs:           docs,
		RuntimeHooks:   runtimeHooks,
		Capabilities:   capabilities,
		Hooks:          hooks,
		ConfigSchema:   configSchema,
		CliDeps:        cliDeps,
		Init:           init,
		BriefFragments: briefFragments,
	}, nil
}

// LoadRecipeToml loads and fully validates the recipe.toml at path, resolving
// init/hook paths against the recipe's directory (mirrors load_recipe_toml).
func LoadRecipeToml(path string) (*Recipe, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, ve("recipe.toml not found: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tbl, err := toml.Parse(raw)
	if err != nil {
		return nil, err
	}
	return ValidateRecipeToml(tbl, filepath.Dir(path))
}
