package config

// GO-20 U6d-1 (T4): the recipe-config write authority, promoted into
// internal/config per S6 ("one grader per behaviour"; a core living outside
// internal/ is PROMOTED, never copied).
//
// Provenance and the promotion decision (recorded in odd/tasks/go-20-u6-recipe.md
// L6): the algorithm is the 1:1 port of the gate's
// catalog/recipes/worktree-flow/gate/shared/recipeconfigwrite.go, itself
// ported 1:1 from lib/_internal/recipe-config-write.py — line surgery,
// comment preservation, the whole-table/dotted conflict refusal, the frozen
// refusal strings and the unconditional valid-TOML guard. A move of the gate
// copy into this package is blocked for BOTH candidate directions: the gate
// module (ai-specs.dev/worktree-gate) can never import
// ai-specs.dev/ai-specs/internal/config — the go tool rejects an import of
// another module's internal/ tree ("use of internal package not allowed"),
// which a replace directive does not change — and the gate would additionally
// need a require of the root module while the root already requires the gate
// (replace => ./catalog/recipes/worktree-flow/gate). This file therefore hosts
// the authoritative copy the native CLI routes through, and
// internal/config/recipeconfig_test.go proves the two implementations agree
// byte for byte over shared fixtures (shared.ApplyRecipeConfigWrite runs the
// same cases through the gate copy). The gate's copy stays a temporary
// duplicate pinned by its own recipeconfigwrite_test.go until the promotion
// (gate writer deleted, its --write-recipe-config surface re-homed on this
// package) lands as follow-up outside this slice's edit surfaces.
//
// Deliberate deviations from the gate copy, both recorded in L6:
//   - TOML parsing goes through internal/toml (the parser every writer in
//     this package already uses for its guard) instead of the gate's bounded
//     embedded-Python tomllib seam — the gate cannot import internal/toml for
//     the same internal/ rule, this package can, and a native route that
//     spawns python3 would break S1 (zero Python under the product tree).
//     Parse-error DETAIL text therefore differs from tomllib's (ADR 0002
//     class, normalized only inside the messages that embed it); the frozen
//     framings are byte-identical.
//   - A refusal does not rewrite the file. Both this port and the gate
//     validate BEFORE any write, so a refusal leaves the manifest untouched;
//     the Python reference additionally rewrites the (universal-newline
//     normalized) original on refusal, a byte-level no-op for LF manifests
//     and an unexercised CRLF corner.
// Non-bare inline-table keys render quoted (toTomlKey), the gate copy's
// documented divergence from toml_write.py's raw keys, so the two Go
// implementations stay identical.

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	shared "ai-specs.dev/worktree-gate/shared"
)

// ConfigValue is one KEY=VALUE assignment in the domain the writer and the
// validator share: Key is the config key exactly as given (inline or dotted),
// Value is a TOML-domain value (string, bool, int64, float64, []any,
// *toml.Table, …). A slice of ConfigValue preserves Python dict insertion
// order (the --set argument order, after first-position/last-value dedupe in
// the verb).
type ConfigValue struct {
	Key   string
	Value any
}

// recipeConfigSecretKeyRE is recipe-configure.py's SECRET_KEY_RE: a literal
// whose KEY looks secret-shaped is rejected before it can be written in the
// clear (the ${env:…} interpolation form is the allowed one).
var recipeConfigSecretKeyRE = regexp.MustCompile(`(?i)(?:token|password|secret|api[_-]?key)`)

// ── reads the verb shares (S5: the manifest stays the only source of truth) ──

// RecipeConfigState is recipe-configure.py's _recipe_state: whether a
// [recipes.<id>] TABLE exists, whether it is enabled == true (Python's
// `is True`: a non-boolean enabled counts as disabled), and the recipe's
// [recipes.<id>.config] table (nil when absent or not a table — Python's
// `{}`).
func RecipeConfigState(manifest *toml.Table, recipeID string) (present, enabled bool, config *toml.Table) {
	if manifest == nil {
		return false, false, nil
	}
	recipesRaw, ok := manifest.Get("recipes")
	if !ok || recipesRaw == nil {
		return false, false, nil
	}
	recipes, isTable := recipesRaw.(*toml.Table)
	if !isTable {
		return false, false, nil
	}
	entryRaw, has := recipes.Get(recipeID)
	if !has {
		return false, false, nil
	}
	entry, entryIsTable := entryRaw.(*toml.Table)
	if !entryIsTable || entry == nil {
		return false, false, nil
	}
	on, _ := entry.Bool("enabled")
	cfgRaw, hasCfg := entry.Get("config")
	if !hasCfg {
		return true, on, nil
	}
	cfg, cfgIsTable := cfgRaw.(*toml.Table)
	if !cfgIsTable {
		return true, on, nil
	}
	return true, on, cfg
}

// LookupNestedConfig is the ONE dotted-key reader: recipe-configure.py has
// two identically-behaving functions (_lookup_nested for the report,
// recipe-config-write.py's _nested_get for the writer's pending decision),
// ported once so the report and the write decision can never drift (S6). It
// resolves "a.b.c" against the nested config table; a missing segment or a
// non-table segment reports absent.
func LookupNestedConfig(current *toml.Table, key string) (any, bool) {
	if current == nil {
		return nil, false
	}
	node := any(current)
	for _, part := range strings.Split(key, ".") {
		tbl, ok := node.(*toml.Table)
		if !ok {
			return nil, false
		}
		v, exists := tbl.Get(part)
		if !exists {
			return nil, false
		}
		node = v
	}
	return node, true
}

// EqualPythonValue mirrors Python's `==` over the TOML value domain — the
// equality BOTH the report's changed/unchanged decision and the writer's
// pending decision run (recipe-configure.py's `==`, the gate copy's
// pyValueEqual): bool is an int (True == 1), int/float compare numerically,
// tables compare by key/value with order irrelevant (dict ==), lists
// element-wise, datetimes by value, and NaN never equals anything — exactly
// like Python.
func EqualPythonValue(a, b any) bool {
	na, aIsNum := pyConfigNumeric(a)
	nb, bIsNum := pyConfigNumeric(b)
	if aIsNum || bIsNum {
		if !aIsNum || !bIsNum {
			return false
		}
		return pyConfigNumEqual(na, nb)
	}
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !EqualPythonValue(av[i], bv[i]) {
				return false
			}
		}
		return true
	case *toml.Table:
		bv, ok := b.(*toml.Table)
		if !ok {
			return false
		}
		if av == nil || bv == nil {
			return av == nil && bv == nil
		}
		if len(av.Keys()) != len(bv.Keys()) {
			return false
		}
		for _, k := range av.Keys() {
			ov, exists := bv.Get(k)
			if !exists {
				return false
			}
			avv, _ := av.Get(k)
			if !EqualPythonValue(avv, ov) {
				return false
			}
		}
		return true
	case toml.DateTime:
		bv, ok := b.(toml.DateTime)
		return ok && av == bv
	}
	return false
}

// pyConfigNumeric reports the Python-numeric projection of v (bool as its int
// value, int kinds as digits, floats by shortest repr) — the same projection
// the gate copy uses, so int-vs-float and bool-vs-int equality agree with the
// Python reference and with the gate copy.
func pyConfigNumeric(v any) (string, bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return "1", true
		}
		return "0", true
	case int:
		return strconv.FormatInt(int64(t), 10), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case toml.BigInt:
		return string(t), true
	case float64:
		return formatPyFloat(t), true
	}
	return "", false
}

func pyConfigNumEqual(a, b string) bool {
	if pyConfigIsIntSyntax(a) && pyConfigIsIntSyntax(b) {
		ai, errA := strconv.ParseInt(a, 10, 64)
		bi, errB := strconv.ParseInt(b, 10, 64)
		if errA == nil && errB == nil {
			return ai == bi
		}
		// Arbitrary precision: compare digit strings (the gate's bigIntEqual).
		na := strings.TrimLeft(strings.TrimPrefix(a, "-"), "0")
		nb := strings.TrimLeft(strings.TrimPrefix(b, "-"), "0")
		if len(na) != len(nb) {
			return false
		}
		if na != nb {
			return false
		}
		if na == "" {
			return true
		}
		return strings.HasPrefix(a, "-") == strings.HasPrefix(b, "-")
	}
	af, errA := strconv.ParseFloat(strings.ToLower(a), 64)
	bf, errB := strconv.ParseFloat(strings.ToLower(b), 64)
	if errA != nil || errB != nil {
		return a == b
	}
	// NaN never equals (Python semantics); Go reports NaN == NaN as true.
	if af != af || bf != bf {
		return false
	}
	return af == bf
}

func pyConfigIsIntSyntax(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// RecipeConfigFieldType is recipe-configure.py's _field_type: the lowercased
// declared type, then the Python short names (string→str, integer→int,
// boolean→bool). One mapping for the validate path and the --inspect schema
// document, so the two can never spell a type differently.
func RecipeConfigFieldType(f *schema.ConfigField) string {
	if f == nil {
		return ""
	}
	declared := strings.ToLower(f.Type)
	switch declared {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "boolean":
		return "bool"
	}
	return declared
}

// FormatPythonJSONFloat renders f like Python json.dumps(float) — the verb's
// --inspect/--set JSON emission needs the same float repr as this package's
// encoder (one formatter, S6).
func FormatPythonJSONFloat(f float64) string { return jsonFloat(f) }

// ── recipe-configure.py's value surface (parse + validate) ──
//
// These two live beside the writer rather than in internal/cli because the
// value domain they operate on is this package's: internal/cli cannot reach
// encodeJSON without a second Python-exact encoder existing (S6), and the
// structured check routes to shared.ValidateStructured — the shipped shape
// grader — never to a copy of it.

// ParseConfigAssignment is recipe-configure.py's _parse_assignment: split on
// the first '=', strip both sides, then resolve the KEY against the recipe
// schema before parsing the value — dotted and whole-table keys parse their
// value as TOML (error detail carried in the message), a standard field
// falls back to the raw string when the value is not TOML but the field is a
// str, and a non-str field whose value is not TOML is refused WITHOUT detail
// (the exact reference messages).
func ParseConfigAssignment(r *schema.Recipe, assignment string) (ConfigValue, error) {
	if !strings.Contains(assignment, "=") {
		return ConfigValue{}, errors.New("--set requires KEY=VALUE")
	}
	parts := strings.SplitN(assignment, "=", 2)
	key := strings.TrimSpace(parts[0])
	raw := strings.TrimSpace(parts[1])
	if key == "" {
		return ConfigValue{}, errors.New("--set requires a non-empty key")
	}
	fields := map[string]*schema.ConfigField{}
	tables := map[string]*schema.ConfigTable{}
	if r != nil && r.ConfigSchema != nil {
		fields = r.ConfigSchema.Fields
		tables = r.ConfigSchema.Tables
	}
	if strings.Contains(key, ".") {
		// Dotted assignment: one nested sub-key of a declared structured table.
		dotted := strings.SplitN(key, ".", 2)
		root, rest := dotted[0], dotted[1]
		if root == "" || rest == "" {
			return ConfigValue{}, fmt.Errorf("invalid dotted config key: %s", key)
		}
		if _, ok := tables[root]; !ok {
			return ConfigValue{}, fmt.Errorf("unknown config key: %s", root)
		}
		v, err := parseConfigInlineValue(raw)
		if err != nil {
			return ConfigValue{}, fmt.Errorf("invalid TOML value for %s: %s", key, err)
		}
		return ConfigValue{Key: key, Value: v}, nil
	}
	if _, ok := tables[key]; ok {
		// Structured table value: a TOML inline table, e.g.
		// --set 'reconcile={scope_field="board_id",max_age_seconds=900}'
		v, err := parseConfigInlineValue(raw)
		if err != nil {
			return ConfigValue{}, fmt.Errorf("invalid TOML value for %s: %s", key, err)
		}
		return ConfigValue{Key: key, Value: v}, nil
	}
	field := fields[key]
	if field == nil {
		return ConfigValue{}, fmt.Errorf("unknown config key: %s", key)
	}
	v, err := parseConfigInlineValue(raw)
	if err != nil {
		if RecipeConfigFieldType(field) == "str" {
			// The str fallback: an unparsable raw value is the literal string.
			return ConfigValue{Key: key, Value: raw}, nil
		}
		return ConfigValue{}, fmt.Errorf("invalid TOML value for %s", key)
	}
	return ConfigValue{Key: key, Value: v}, nil
}

// parseConfigInlineValue is `tomllib.loads(f"value = {raw}\n")["value"]`
// through internal/toml. The returned error is the parser's detail text —
// diagnostic (ADR 0002), embedded by the caller's frozen framing.
func parseConfigInlineValue(raw string) (any, error) {
	tbl, err := toml.Parse([]byte("value = " + raw + "\n"))
	if err != nil {
		return nil, err
	}
	v, _ := tbl.Get("value")
	return v, nil
}

// ValidateConfigValues is recipe-configure.py's _validate_values: for every
// value in order — a dotted key must root in a declared structured table and
// its nested value validates against that table's shape; a whole-table key
// validates the same way; a standard field checks the secret-shaped literal
// refusal, the type, the enum and the validation regex, in the reference
// order and with its exact messages. The structured check goes through
// shared.ValidateStructured (the ONE shape grader, S6) over
// schema.ConfigTable.ShapeJSON — Python's shape rendering, declaration order
// included.
func ValidateConfigValues(r *schema.Recipe, values []ConfigValue) error {
	fields := map[string]*schema.ConfigField{}
	tables := map[string]*schema.ConfigTable{}
	if r != nil && r.ConfigSchema != nil {
		fields = r.ConfigSchema.Fields
		tables = r.ConfigSchema.Tables
	}
	for _, cv := range values {
		key := cv.Key
		if strings.Contains(key, ".") {
			dotted := strings.SplitN(key, ".", 2)
			root, rest := dotted[0], dotted[1]
			if _, ok := tables[root]; !ok {
				return fmt.Errorf("unknown config key: %s", root)
			}
			nested := newObject()
			assignConfigNested(nested, strings.Split(rest, "."), encodeConfigDomain(cv.Value))
			if err := validateStructuredConfigValue(root, nested); err != nil {
				return err
			}
			continue
		}
		if _, ok := tables[key]; ok {
			if err := validateStructuredConfigValue(key, encodeConfigDomain(cv.Value)); err != nil {
				return err
			}
			continue
		}
		field := fields[key]
		if field == nil {
			return fmt.Errorf("unknown config key: %s", key)
		}
		if isSecretConfigLiteral(key, cv.Value) {
			return fmt.Errorf("secret-shaped literal rejected for config key: %s", key)
		}
		kind := RecipeConfigFieldType(field)
		switch kind {
		case "str":
			if _, ok := cv.Value.(string); !ok {
				return fmt.Errorf("%s must be a string", key)
			}
		case "bool":
			if _, ok := cv.Value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", key)
			}
		case "int":
			_, isInt := cv.Value.(int64)
			_, isBig := cv.Value.(toml.BigInt)
			_, isBool := cv.Value.(bool)
			if (!isInt && !isBig) || isBool {
				return fmt.Errorf("%s must be an integer", key)
			}
		}
		if field.Enum != nil {
			found := false
			if s, ok := cv.Value.(string); ok {
				for _, e := range field.Enum {
					if e == s {
						found = true
						break
					}
				}
			}
			if !found {
				return fmt.Errorf("%s must be one of: %s", key, strings.Join(field.Enum, ", "))
			}
		}
		if kind == "str" {
			if pattern, ok := field.Validation["regex"].(string); ok && pattern != "" {
				if s, isStr := cv.Value.(string); isStr {
					re, cerr := regexp.Compile("^(?:" + pattern + ")$")
					if cerr != nil {
						// Python raises re.error here (an uncaught traceback,
						// exit 1); Go has no stable equivalent of that text,
						// so the compile failure surfaces as this validation
						// error instead (recorded deviation, L6).
						return fmt.Errorf("%s: invalid validation pattern: %s", key, cerr)
					}
					if !re.MatchString(s) {
						return fmt.Errorf("%s does not match its validation pattern", key)
					}
				}
			}
		}
	}
	return nil
}

// isSecretConfigLiteral is _is_secret_literal: a secret-shaped KEY carrying a
// literal string (the ${env:…} interpolation form is allowed).
func isSecretConfigLiteral(key string, value any) bool {
	if !recipeConfigSecretKeyRE.MatchString(key) {
		return false
	}
	s, ok := value.(string)
	return ok && !strings.HasPrefix(s, "${env:")
}

// assignConfigNested is _assign_nested: set a dotted path inside an ordered
// object, creating intermediate tables (Python dicts, insertion-ordered).
func assignConfigNested(node *Obj, path []string, value any) {
	if len(path) == 0 {
		return
	}
	cur := node
	for _, part := range path[:len(path)-1] {
		child, exists := cur.vals[part]
		childObj, isObj := child.(*Obj)
		if !exists || !isObj {
			childObj = newObject()
			cur.set(part, childObj)
		}
		cur = childObj
	}
	cur.set(path[len(path)-1], value)
}

// validateStructuredConfigValue is recipe_schema.validate_structured_config
// through the shipped grader: encode the value into the shared envelope
// domain (this package's Python-exact encoder), decode the declared shape
// from ShapeJSON, and run shared.ValidateStructured — byte-identical messages
// and check order, no second shape checker (S6). A section without a
// declared shape is a no-op (Python returns None).
func validateStructuredConfigValue(section string, value any) error {
	if _, declared := schema.StructuredConfigShapes[section]; !declared {
		return nil
	}
	shapeTable := &schema.ConfigTable{Shape: schema.StructuredConfigShapes[section]}
	shapeRaw, err := shapeTable.ShapeJSON()
	if err != nil {
		return err
	}
	encoded, err := encodeJSON(value)
	if err != nil {
		return err
	}
	sharedValue, err := shared.DecodeOrdered([]byte(encoded))
	if err != nil {
		return err
	}
	sharedShape, err := shared.DecodeOrdered(shapeRaw)
	if err != nil {
		return err
	}
	return shared.ValidateStructured(section, sharedValue, sharedShape)
}

// encodeConfigDomain normalizes TOML-domain interiors into the package's
// ordered JSON domain (this package's encoder walks *Obj, not *toml.Table
// interiors — and *toml.Table is immutable from outside its package), so
// parsed tables and validation-built nested paths serialize identically.
func encodeConfigDomain(v any) any {
	switch t := v.(type) {
	case *toml.Table:
		out := newObject()
		if t != nil {
			for _, k := range t.Keys() {
				value, _ := t.Get(k)
				out.set(k, encodeConfigDomain(value))
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = encodeConfigDomain(e)
		}
		return out
	}
	return v
}

// cfgOrdered is the writer's mutable insertion-ordered table — the local
// counterpart of the gate copy's *OrderedMap (this package cannot construct
// or mutate *toml.Table: its constructor and setter are package-private, and
// importing the gate's type here would invert the promotion direction).
type cfgOrdered struct {
	keys []string
	vals map[string]any
}

func newCfgOrdered() *cfgOrdered {
	return &cfgOrdered{vals: map[string]any{}}
}

func (o *cfgOrdered) set(key string, value any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

func (o *cfgOrdered) get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// ── recipe-config-write.py's line surgery (the gate copy's algorithm) ──

// toTomlKey is _toml_key: bare keys stay bare, everything else is JSON-quoted
// (the gate copy's rendering — see the file header).
func toTomlKey(key string) string {
	if isBareTomlConfigKey(key) {
		return key
	}
	return jsonString(key)
}

func isBareTomlConfigKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// toTomlValue is toml_write.toml_value over the writer's value domain,
// rendered the way the GATE copy renders it (quoted non-bare table keys) so
// the two implementations agree byte for byte: bool first, numbers as their
// Python text, JSON-quoted strings, lists, ordered tables, and the exact
// serialization refusals (None → NoneType, unknown → the Go type name, the
// Python type name for date/time kinds).
func toTomlValue(v any) (string, error) {
	switch t := v.(type) {
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case int:
		return strconv.FormatInt(int64(t), 10), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case toml.BigInt:
		return string(t), nil
	case float64:
		return formatPyFloat(t), nil
	case string:
		return jsonString(t), nil
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			encoded, err := toTomlValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, encoded)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *cfgOrdered:
		parts := make([]string, 0, len(t.keys))
		for _, k := range t.keys {
			encoded, err := toTomlValue(t.vals[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, toTomlKey(k)+" = "+encoded)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	case *toml.Table:
		parts := make([]string, 0, len(t.Keys()))
		for _, k := range t.Keys() {
			value, _ := t.Get(k)
			encoded, err := toTomlValue(value)
			if err != nil {
				return "", err
			}
			parts = append(parts, toTomlKey(k)+" = "+encoded)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	case nil:
		return "", errors.New("cannot serialize NoneType to TOML")
	case toml.DateTime:
		return "", fmt.Errorf("cannot serialize %s to TOML", t.PyTypeName())
	default:
		return "", fmt.Errorf("cannot serialize %T to TOML", v)
	}
}

// splitConfigInlineComment is _split_inline_comment: TOML string-aware inline
// comment splitting, byte-exact against the reference (the comment keeps the
// whitespace that separated it from the value).
func splitConfigInlineComment(line string) (string, string) {
	quote := byte(0)
	triple := false
	escaped := false
	for idx := 0; idx < len(line); idx++ {
		c := line[idx]
		if triple {
			if configTripleMarker(line, idx, quote) {
				triple = false
				idx += 2
				continue
			}
			continue
		}
		if quote == '"' {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			}
			continue
		}
		if configTripleMarker(line, idx, '"') || configTripleMarker(line, idx, '\'') {
			quote = c
			triple = true
			idx += 2
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '#' {
			start := idx
			for start > 0 && (line[start-1] == ' ' || line[start-1] == '\t') {
				start--
			}
			return line[:start], line[start:]
		}
	}
	return line, ""
}

func configTripleMarker(line string, idx int, quote byte) bool {
	if idx+3 > len(line) {
		return false
	}
	return line[idx] == quote && line[idx+1] == quote && line[idx+2] == quote
}

// configValueIsMultiline is _value_is_multiline: an unfinished array, table
// or string value on one source line — the refusal that keeps a multi-line
// value from being cut open by a one-line rewrite.
func configValueIsMultiline(line string) bool {
	value, _ := splitConfigInlineComment(line)
	equals := strings.Index(value, "=")
	if equals < 0 {
		return false
	}
	text := value[equals+1:]
	quote := byte(0)
	triple := false
	escaped := false
	depth := 0
	for idx := 0; idx < len(text); idx++ {
		c := text[idx]
		if triple {
			if configTripleMarker(text, idx, quote) {
				triple = false
				idx += 2
				continue
			}
			continue
		}
		if quote != 0 {
			if quote == '"' && escaped {
				escaped = false
			} else if quote == '"' && c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if configTripleMarker(text, idx, '"') || configTripleMarker(text, idx, '\'') {
			quote = c
			triple = true
			idx += 2
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return quote != 0 || triple || depth > 0
}

// matchConfigKeyLine is _key_line_re(key).match(line): leading whitespace,
// the bare or JSON-quoted key, optional horizontal whitespace, then '='. It
// returns the captured indent. Bare-first alternation, like the reference.
func matchConfigKeyLine(line, key string) (string, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	indent := line[:i]
	rest := line[i:]
	keyText := ""
	if strings.HasPrefix(rest, key) {
		keyText = key
	} else if quoted := jsonString(key); strings.HasPrefix(rest, quoted) {
		keyText = quoted
	} else {
		return "", false
	}
	rest = rest[len(keyText):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "=") {
		return indent, true
	}
	return "", false
}

// configAssignmentParts is _assignment_parts: (indent, value text, comment)
// for one `key = value` line, with the reference refusal when no '=' exists.
func configAssignmentParts(line string) (string, string, string, error) {
	prefix, comment := splitConfigInlineComment(line)
	equals := strings.Index(prefix, "=")
	if equals < 0 {
		return "", "", "", errors.New("cannot rewrite a line without a key assignment")
	}
	head := prefix[:equals]
	indent := head[:len(head)-len(strings.TrimLeftFunc(head, unicode.IsSpace))]
	return indent, strings.TrimSpace(prefix[equals+1:]), strings.TrimRight(comment, "\r\n"), nil
}

func configSubtableHeader(recipeKey string, tablePath []string) string {
	parts := make([]string, 0, len(tablePath))
	for _, part := range tablePath {
		parts = append(parts, toTomlKey(part))
	}
	return "[recipes." + recipeKey + ".config." + strings.Join(parts, ".") + "]"
}

func configIsHeaderLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "[")
}

func configFindHeader(lines []string, header string) int {
	for idx, line := range lines {
		if strings.TrimFunc(line, unicode.IsSpace) == header {
			return idx
		}
	}
	return -1
}

// configRecipeRegionEnd is _recipe_region_end: the region ends at the next
// header line that does not carry the recipe's [recipes.<key> prefix (the
// unclosed prefix is reference behavior, kept byte-exact).
func configRecipeRegionEnd(lines []string, start int, recipeKey string) int {
	prefix := "[recipes." + recipeKey
	for idx := start + 1; idx < len(lines); idx++ {
		stripped := strings.TrimFunc(lines[idx], unicode.IsSpace)
		if strings.HasPrefix(stripped, "[") && !strings.HasPrefix(stripped, prefix) {
			return idx
		}
	}
	return len(lines)
}

func configFindConfigHeader(lines []string, start, end int, configHeader string) int {
	for idx := start; idx < end; idx++ {
		if strings.TrimFunc(lines[idx], unicode.IsSpace) == configHeader {
			return idx
		}
	}
	return -1
}

func configBlockEnd(lines []string, start, regionEnd int) int {
	for idx := start + 1; idx < regionEnd; idx++ {
		if configIsHeaderLine(lines[idx]) {
			return idx
		}
	}
	return regionEnd
}

func configInsertLines(lines []string, at int, inserted []string) []string {
	out := make([]string, 0, len(lines)+len(inserted))
	out = append(out, lines[:at]...)
	out = append(out, inserted...)
	out = append(out, lines[at:]...)
	return out
}

// configComposeKeyLine rebuilds one `key = value` assignment in place: the
// encoded value, the line's original indent, its TOML-aware inline comment
// and its trailing newline all preserved.
func configComposeKeyLine(indent, key string, value any, comment string, hadNewline bool) (string, error) {
	encoded, err := toTomlValue(value)
	if err != nil {
		return "", err
	}
	newline := ""
	if hadNewline {
		newline = "\n"
	}
	return indent + toTomlKey(key) + " = " + encoded + comment + newline, nil
}

type recipeConfigDottedPair struct {
	path  []string
	value any
}

type recipeConfigPair struct {
	key   string
	value any
}

// recipeConfigWriteState carries the mutable line slice and region bounds
// through the surgery, mirroring the reference's in-place list mutation and
// stale-then-recomputed region indices.
type recipeConfigWriteState struct {
	lines     []string
	recipeIdx int
	regionEnd int
	configIdx int
	blockEnd  int
	recipeKey string
}

// readRecipeConfigTable is _existing_config: the recipe's config table from
// the manifest text. A manifest that fails TOML parsing yields nil (Python's
// {}); a config value that is not a table also yields nil.
func readRecipeConfigTable(originalText, recipeID string) *toml.Table {
	parsed, err := toml.Parse([]byte(originalText))
	if err != nil {
		return nil
	}
	_, _, config := RecipeConfigState(parsed, recipeID)
	return config
}

// configSetInlinePaths is _set_inline_paths: update dotted sub-keys inside
// one inline-table value line in place.
func configSetInlinePaths(state *recipeConfigWriteState, idx int, rootKey string, pairs []recipeConfigDottedPair) error {
	if configValueIsMultiline(state.lines[idx]) {
		return fmt.Errorf("cannot replace multiline value for key '%s'", rootKey)
	}
	indent, valueText, comment, err := configAssignmentParts(state.lines[idx])
	if err != nil {
		return err
	}
	parsed, err := parseConfigInlineValue(valueText)
	if err != nil {
		return fmt.Errorf("cannot read the existing value for '%s': %s", rootKey, err)
	}
	table, ok := parsed.(*toml.Table)
	if !ok {
		return fmt.Errorf("cannot update '%s': its value is not an inline table", rootKey)
	}
	ordered := tableToOrdered(table)
	for _, pair := range pairs {
		configNestedSet(ordered, pair.path[1:], pair.value)
	}
	newLine, err := configComposeKeyLine(indent, rootKey, ordered, comment, strings.HasSuffix(state.lines[idx], "\n"))
	if err != nil {
		return err
	}
	state.lines[idx] = newLine
	return nil
}

// tableToOrdered converts a parsed TOML table into the writer's mutable
// ordered domain (interiors recursed), for the nested updates and, when a
// value must be rebuilt, for toTomlValue's ordered-table branch.
func tableToOrdered(t *toml.Table) *cfgOrdered {
	out := newCfgOrdered()
	if t == nil {
		return out
	}
	for _, k := range t.Keys() {
		v, _ := t.Get(k)
		switch tv := v.(type) {
		case *toml.Table:
			out.set(k, tableToOrdered(tv))
		case []any:
			out.set(k, v)
		default:
			out.set(k, tv)
		}
	}
	return out
}

// configNestedSet is _nested_set over the writer's ordered table: set a
// dotted path, replacing non-table intermediates with fresh tables (existing
// keys keep their position).
func configNestedSet(m *cfgOrdered, path []string, value any) {
	if len(path) == 0 {
		return
	}
	node := m
	for _, part := range path[:len(path)-1] {
		child, _ := node.get(part)
		childTbl, isTbl := child.(*cfgOrdered)
		if !isTbl {
			childTbl = newCfgOrdered()
			node.set(part, childTbl)
		}
		node = childTbl
	}
	node.set(path[len(path)-1], value)
}

// configSetHeaderPath is _set_header_path: replace or append one leaf line
// inside a declared [config.<table>] header table, recomputing the region
// end fresh on every call.
func configSetHeaderPath(state *recipeConfigWriteState, path []string, value any) error {
	regionEnd := configRecipeRegionEnd(state.lines, state.recipeIdx, state.recipeKey)
	header := configSubtableHeader(state.recipeKey, path[:len(path)-1])
	headerIdx := -1
	for idx := state.recipeIdx; idx < regionEnd; idx++ {
		if strings.TrimFunc(state.lines[idx], unicode.IsSpace) == header {
			headerIdx = idx
			break
		}
	}
	if headerIdx < 0 {
		return fmt.Errorf("cannot update '%s': table %s not found", strings.Join(path, "."), header)
	}
	blockEnd := configBlockEnd(state.lines, headerIdx, regionEnd)
	leaf := path[len(path)-1]
	for idx := headerIdx + 1; idx < blockEnd; idx++ {
		indent, ok := matchConfigKeyLine(state.lines[idx], leaf)
		if !ok {
			continue
		}
		if configValueIsMultiline(state.lines[idx]) {
			return fmt.Errorf("cannot replace multiline value for key '%s'", leaf)
		}
		_, _, comment, err := configAssignmentParts(state.lines[idx])
		if err != nil {
			return err
		}
		newLine, err := configComposeKeyLine(indent, leaf, value, comment, strings.HasSuffix(state.lines[idx], "\n"))
		if err != nil {
			return err
		}
		state.lines[idx] = newLine
		return nil
	}
	composed, err := configComposeKeyLine("", leaf, value, "", true)
	if err != nil {
		return err
	}
	state.lines = configInsertLines(state.lines, blockEnd, []string{composed})
	return nil
}

// configDottedTable builds the inline table one root's dotted updates
// serialize to (dottedPairsToInlineTable).
func configDottedTable(pairs []recipeConfigDottedPair) *cfgOrdered {
	table := newCfgOrdered()
	for _, pair := range pairs {
		configNestedSet(table, pair.path[1:], pair.value)
	}
	return table
}

// configApplyDottedUpdates is _apply_dotted_updates: an inline-table root
// updates in place, a header-table root updates leaf lines, an absent root is
// appended as a new inline table. Refusals are the exact reference strings.
func configApplyDottedUpdates(state *recipeConfigWriteState, grouped map[string][]recipeConfigDottedPair, order []string) error {
	inlineLines := map[string]int{}
	var headerRoots, newRoots []string
	for _, rootKey := range order {
		idx := -1
		for candidate := state.configIdx + 1; candidate < state.blockEnd; candidate++ {
			if _, ok := matchConfigKeyLine(state.lines[candidate], rootKey); ok {
				idx = candidate
				break
			}
		}
		if idx >= 0 {
			inlineLines[rootKey] = idx
			continue
		}
		header := configSubtableHeader(state.recipeKey, []string{rootKey})
		found := false
		for candidate := state.recipeIdx; candidate < state.regionEnd; candidate++ {
			if strings.TrimFunc(state.lines[candidate], unicode.IsSpace) == header {
				found = true
				break
			}
		}
		if found {
			headerRoots = append(headerRoots, rootKey)
		} else {
			newRoots = append(newRoots, rootKey)
		}
	}

	for _, rootKey := range order {
		idx, ok := inlineLines[rootKey]
		if !ok {
			continue
		}
		if err := configSetInlinePaths(state, idx, rootKey, grouped[rootKey]); err != nil {
			return err
		}
	}

	for _, rootKey := range headerRoots {
		for _, pair := range grouped[rootKey] {
			if err := configSetHeaderPath(state, pair.path, pair.value); err != nil {
				return err
			}
		}
	}

	if len(newRoots) > 0 {
		blockEnd := configBlockEnd(state.lines, state.configIdx, state.regionEnd)
		var newLines []string
		for _, rootKey := range newRoots {
			encoded, err := toTomlValue(configDottedTable(grouped[rootKey]))
			if err != nil {
				return err
			}
			newLines = append(newLines, toTomlKey(rootKey)+" = "+encoded+"\n")
		}
		state.lines = configInsertLines(state.lines, blockEnd, newLines)
	}
	return nil
}

// sortedConfigPairs returns the pairs in Python's sorted() order over the
// keys (the reference iterates sorted(pending_plain) at every plain-key
// site), without mutating the caller's insertion order elsewhere.
func sortedConfigPairs(pairs []recipeConfigPair) []recipeConfigPair {
	out := append([]recipeConfigPair(nil), pairs...)
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// UpdateRecipeConfig is update_recipe_config (recipe-config-write.py): write
// values into [recipes.<id>.config], preserving comments and bytes.
//
// Keys may be flat (the whole value replaces the key, including a whole
// structured table) or dotted (`reconcile.max_age_seconds`), which updates
// one nested sub-key of an existing structured table in place. A dotted path
// and a whole-table value for the same root are refused rather than merged.
//
// Returns whether the manifest changed; a refusal (a value the surgery
// cannot apply, or a result that would not parse as TOML) comes back as the
// exact reference string with changed=false and no error; an I/O failure
// comes back as err. An empty values slice is a local no-op: no file is read
// and nothing is written.
func UpdateRecipeConfig(manifestPath, recipeID string, values []ConfigValue) (bool, string, error) {
	if len(values) == 0 {
		return false, "", nil
	}
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, "", fmt.Errorf("read manifest: %w", err)
	}
	originalText := string(original)

	current := readRecipeConfigTable(originalText, recipeID)
	if current == nil {
		// Python's _existing_config yields {} for a missing recipe or a
		// non-table/absent config; the pending decisions below read through
		// this table, so an absent one is the empty table, not a nil deref.
		current = &toml.Table{}
	}

	var plain, dotted []ConfigValue
	for _, cv := range values {
		if strings.Contains(cv.Key, ".") {
			dotted = append(dotted, cv)
		} else {
			plain = append(plain, cv)
		}
	}

	var pendingPlain []recipeConfigPair
	for _, cv := range plain {
		cur, ok := current.Get(cv.Key)
		if !ok || !EqualPythonValue(cur, cv.Value) {
			pendingPlain = append(pendingPlain, recipeConfigPair{key: cv.Key, value: cv.Value})
		}
	}
	var pendingDotted []recipeConfigPair
	for _, cv := range dotted {
		cur, ok := LookupNestedConfig(current, cv.Key)
		if !ok || !EqualPythonValue(cur, cv.Value) {
			pendingDotted = append(pendingDotted, recipeConfigPair{key: cv.Key, value: cv.Value})
		}
	}
	if len(pendingPlain) == 0 && len(pendingDotted) == 0 {
		return false, "", nil
	}

	conflictSet := map[string]bool{}
	for _, p := range pendingDotted {
		conflictSet[strings.Split(p.key, ".")[0]] = true
	}
	var conflicts []string
	for _, p := range pendingPlain {
		if conflictSet[p.key] {
			conflicts = append(conflicts, p.key)
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return false, "cannot combine a whole-table and a dotted update for key(s): " + strings.Join(conflicts, ", "), nil
	}

	grouped := map[string][]recipeConfigDottedPair{}
	var groupedOrder []string
	for _, p := range pendingDotted {
		path := strings.Split(p.key, ".")
		rootKey := path[0]
		if _, ok := grouped[rootKey]; !ok {
			groupedOrder = append(groupedOrder, rootKey)
		}
		grouped[rootKey] = append(grouped[rootKey], recipeConfigDottedPair{path: path, value: p.value})
	}
	sort.Strings(groupedOrder)

	recipeKey := toTomlKey(recipeID)
	lines := splitLinesKeepEnds(originalText)
	newText := ""

	if idx := configFindHeader(lines, "[recipes."+recipeKey+"]"); idx < 0 {
		blockLines := []string{
			"\n",
			"[recipes." + recipeKey + "]\n",
			"enabled = true\n",
			"\n",
			"[recipes." + recipeKey + ".config]\n",
		}
		for _, p := range sortedConfigPairs(pendingPlain) {
			encoded, err := toTomlValue(p.value)
			if err != nil {
				return false, err.Error(), nil
			}
			blockLines = append(blockLines, toTomlKey(p.key)+" = "+encoded+"\n")
		}
		for _, rootKey := range groupedOrder {
			encoded, err := toTomlValue(configDottedTable(grouped[rootKey]))
			if err != nil {
				return false, err.Error(), nil
			}
			blockLines = append(blockLines, toTomlKey(rootKey)+" = "+encoded+"\n")
		}
		newText = originalText
		if !strings.HasSuffix(newText, "\n") && newText != "" {
			newText += "\n"
		}
		newText += strings.Join(blockLines, "")
	} else {
		state := &recipeConfigWriteState{
			lines:     lines,
			recipeIdx: idx,
			recipeKey: recipeKey,
		}
		state.regionEnd = configRecipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
		state.configIdx = configFindConfigHeader(state.lines, state.recipeIdx, state.regionEnd, "[recipes."+recipeKey+".config]")
		if state.configIdx < 0 {
			state.lines = configInsertLines(state.lines, state.regionEnd, []string{"\n", "[recipes." + recipeKey + ".config]\n"})
			state.configIdx = state.regionEnd + 1
			state.regionEnd = configRecipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
		}
		state.blockEnd = configBlockEnd(state.lines, state.configIdx, state.regionEnd)

		var pending []recipeConfigPair
		for _, p := range sortedConfigPairs(pendingPlain) {
			replaced := false
			for candidate := state.configIdx + 1; candidate < state.blockEnd; candidate++ {
				indent, ok := matchConfigKeyLine(state.lines[candidate], p.key)
				if !ok {
					continue
				}
				if configValueIsMultiline(state.lines[candidate]) {
					return false, fmt.Sprintf("cannot replace multiline value for key '%s'", p.key), nil
				}
				_, comment := splitConfigInlineComment(state.lines[candidate])
				newLine, cErr := configComposeKeyLine(indent, p.key, p.value, strings.TrimRight(comment, "\r\n"), strings.HasSuffix(state.lines[candidate], "\n"))
				if cErr != nil {
					return false, cErr.Error(), nil
				}
				state.lines[candidate] = newLine
				replaced = true
				break
			}
			if !replaced {
				pending = append(pending, p)
			}
		}

		if len(pending) > 0 {
			newLines := make([]string, 0, len(pending))
			for _, p := range pending {
				encoded, err := toTomlValue(p.value)
				if err != nil {
					return false, err.Error(), nil
				}
				newLines = append(newLines, toTomlKey(p.key)+" = "+encoded+"\n")
			}
			state.lines = configInsertLines(state.lines, state.blockEnd, newLines)
			state.regionEnd = configRecipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
			state.blockEnd = configBlockEnd(state.lines, state.configIdx, state.regionEnd)
		}

		if len(groupedOrder) > 0 {
			if err := configApplyDottedUpdates(state, grouped, groupedOrder); err != nil {
				return false, err.Error(), nil
			}
		}
		newText = strings.Join(state.lines, "")
	}

	// The unconditional valid-TOML guard: every result must parse, whatever
	// the base looked like (recipe-config-write.py's tomllib.loads gate —
	// like AppendRecipeSegments' guard, NOT the remove heredocs'
	// valid→invalid-only guard, so the shared guard is deliberately not
	// reused: one grader per behaviour, S6).
	if _, perr := toml.Parse([]byte(newText)); perr != nil {
		return false, "invalid TOML after config write: " + perr.Error(), nil
	}
	if newText != originalText {
		if werr := atomicWrite(manifestPath, newText); werr != nil {
			return false, "", fmt.Errorf("write manifest: %w", werr)
		}
		return true, "", nil
	}
	return false, "", nil
}

// WriteProjectKey is recipe-configure.py's _write_project_key: set one scalar
// in [project], preserving the rest of the file's bytes — replace the first
// matching assignment inside the [project] region (comment preserved), else
// insert the line before the region's end (the next header), else append a
// fresh [project] table at EOF. The candidate text is parsed before the write
// (frozen `invalid TOML after project write:` refusal) and the write is
// atomic through the shared writer (S5).
func WriteProjectKey(manifestPath, key string, value any) error {
	original, err := readManifestText(manifestPath)
	if err != nil {
		return err
	}
	lines := splitLinesKeepEnds(original)
	header := -1
	for idx, line := range lines {
		if strings.TrimFunc(line, unicode.IsSpace) == "[project]" {
			header = idx
			break
		}
	}
	var newText string
	if header < 0 {
		text := original
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		encoded, err := toTomlValue(value)
		if err != nil {
			return err
		}
		newText = text + "\n[project]\n" + toTomlKey(key) + " = " + encoded + "\n"
	} else {
		end := len(lines)
		for idx := header + 1; idx < len(lines); idx++ {
			if strings.HasPrefix(strings.TrimLeftFunc(lines[idx], unicode.IsSpace), "[") {
				end = idx
				break
			}
		}
		replaced := false
		for idx := header + 1; idx < end; idx++ {
			indent, ok := matchConfigKeyLine(lines[idx], key)
			if !ok {
				continue
			}
			_, _, comment, aErr := configAssignmentParts(lines[idx])
			if aErr != nil {
				return aErr
			}
			encoded, err := toTomlValue(value)
			if err != nil {
				return err
			}
			newline := ""
			if strings.HasSuffix(lines[idx], "\n") {
				newline = "\n"
			}
			lines[idx] = indent + toTomlKey(key) + " = " + encoded + strings.TrimRight(comment, "\r\n") + newline
			replaced = true
			break
		}
		if !replaced {
			encoded, err := toTomlValue(value)
			if err != nil {
				return err
			}
			insert := []string{toTomlKey(key) + " = " + encoded + "\n"}
			region := make([]string, 0, len(lines)+1)
			region = append(region, lines[:end]...)
			region = append(region, insert...)
			region = append(region, lines[end:]...)
			lines = region
		}
		newText = strings.Join(lines, "")
	}
	if _, perr := toml.Parse([]byte(newText)); perr != nil {
		return fmt.Errorf("invalid TOML after project write: %s", perr)
	}
	if newText != original {
		return atomicWrite(manifestPath, newText)
	}
	return nil
}
