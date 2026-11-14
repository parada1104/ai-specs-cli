package shared

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Config merge decision for recipe materialization, moved out of the gate main
// package by GO-07 S13 PR2. The gate binary's --plan-merge-config surface and
// the root native sync authority share this one decision core; acquisition
// (reading recipe.toml and the manifest) stays per-side.
//
// The decision is pure over a JSON envelope: no filesystem access, no TOML
// re-read. It merges the recipe's declared config defaults with the manifest's
// [*.config] overrides, validates structured (table) sections against the
// recipe-declared shape, and returns the merged config plus warnings.
//
// It was moved from the Python authority merge_config
// (lib/_internal/recipe-materialize.py) with the structured-shape grammar of
// lib/_internal/recipe_schema.py. Go owns the decision; Python keeps schema
// acquisition (the already-loaded Recipe) as a thin bridge.
//
// Ordering parity: Python dicts are insertion-ordered and the reference relies
// on it (defaults in field order, manifest keys in manifest order, structured
// unknown-key checks in value order, sub-checks in shape order). Go maps
// iterate randomly, so every ordered surface crosses the JSON boundary as an
// array of pairs or an ordered decode — never a Go map.

// MergeConfigField is one recipe schema field, ordered by declaration.
// HasDefault transports Python's "default is not None" so defaults of false,
// 0 and "" still apply while an absent default does not.
type MergeConfigField struct {
	Key        string          `json:"key"`
	Required   bool            `json:"required"`
	HasDefault bool            `json:"has_default"`
	Default    json.RawMessage `json:"default"`
	Enum       []string        `json:"enum"`
}

// MergeConfigTable is one structured (table) config section and its declared
// shape (e.g. reconcile).
type MergeConfigTable struct {
	Key   string          `json:"key"`
	Shape json.RawMessage `json:"shape"`
}

// MergeConfigPair is one manifest config entry, ordered by manifest appearance.
type MergeConfigPair struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// MergeConfigRequest is the stdin contract. recipe_name is required (it heads
// every error and warning message).
type MergeConfigRequest struct {
	RecipeName string             `json:"recipe_name"`
	Fields     []MergeConfigField `json:"fields"`
	Tables     []MergeConfigTable `json:"tables"`
	Manifest   []MergeConfigPair  `json:"manifest"`
}

// MergeConfigResult is the stdout contract. Config is always an object (never
// null), Warnings always a list (never null), Error empty when the merge
// succeeded. A semantic validation error (required/enum/structured) fills
// Error and still exits 0 so the bridge fails the materialization without
// triggering its binary fallback; a malformed request exits 2.
type MergeConfigResult struct {
	Config   *OrderedMap `json:"config"`
	Warnings []string    `json:"warnings"`
	Error    string      `json:"error"`
}

// PyType names the value the way Python's type(value).__name__ would after the
// bridge decoded its TOML into Python objects and re-encoded with json.dumps.
func PyType(value any) string {
	switch t := value.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case json.Number:
		if strings.ContainsAny(t.String(), ".eE") {
			return "float"
		}
		return "int"
	case []any:
		return "list"
	case *OrderedMap:
		return "dict"
	}
	return "unknown"
}

// pyTruthy mirrors Python truthiness (used by the gate_scope fallback, where
// `result.get("gate_scope") or ""` swallows None/False/0/""/empty containers).
func pyTruthy(value any) bool {
	switch t := value.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case json.Number:
		// Python ints are arbitrary precision; a zero literal is the only falsy
		// integer (Python never emits "0.0" for an int, so dot/exponent means float).
		if !strings.ContainsAny(t.String(), ".eE") {
			return t.String() != "0"
		}
		f, err := t.Float64()
		return err == nil && f != 0
	case []any:
		return len(t) > 0
	case *OrderedMap:
		return len(t.keys) > 0
	}
	return true
}

// pyString mirrors Python str() for the values the envelope can carry:
// numbers arrive as canonical literals from Python's json.dumps, bools render
// as True/False, and containers use Python's repr-based rendering.
func pyString(value any) string {
	switch t := value.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return t
	case json.Number:
		return t.String()
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			parts[i] = PyRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *OrderedMap:
		parts := make([]string, 0, len(t.keys))
		for _, key := range t.keys {
			parts = append(parts, PyRepr(key)+": "+PyRepr(t.values[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprintf("%v", value)
}

// PyRepr renders one element the way repr() would inside a Python container:
// CPython wraps the string in double quotes when it contains an apostrophe and
// no double quote, keeps the single-quote default otherwise, names \n \r \t,
// and escapes every other non-printable rune as \xNN (up to 0xFF), \uXXXX
// (up to 0xFFFF), or \UXXXXXXXX.
func PyRepr(value any) string {
	if s, ok := value.(string); ok {
		return PyReprString(s)
	}
	return pyString(value)
}

// PyReprString renders a string the way Python repr() would.
func PyReprString(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case rune(quote):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if unicode.IsPrint(r) {
				b.WriteRune(r)
				continue
			}
			switch {
			case r <= 0xFF:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r <= 0xFFFF:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// structuredListMax is the Python STRUCTURED_LIST_MAX bound on a structured
// array such as the reconcile expectations list.
const structuredListMax = 32

// ValidateStructured validates a manifest config table against its declared
// shape, mirroring recipe_schema.validate_structured_config and
// _check_shape_value with byte-identical messages and check order.
func ValidateStructured(section string, value, shape any) error {
	return checkShape(fmt.Sprintf("[config.%s]", section), value, shape)
}

func checkShape(context string, value, shape any) error {
	switch s := shape.(type) {
	case string:
		return checkShapeScalar(context, value, s)
	case []any:
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %s", context, PyType(value))
		}
		if len(arr) > structuredListMax {
			return fmt.Errorf("%s: expected at most %d entries, got %d", context, structuredListMax, len(arr))
		}
		if len(s) == 0 {
			return fmt.Errorf("%s: invalid shape declaration", context)
		}
		for i, item := range arr {
			if err := checkShape(fmt.Sprintf("%s[%d]", context, i), item, s[0]); err != nil {
				return err
			}
		}
		return nil
	case *OrderedMap:
		m, ok := value.(*OrderedMap)
		if !ok {
			return fmt.Errorf("%s: expected table, got %s", context, PyType(value))
		}
		// Unknown keys are reported in the value's own key order first.
		for _, key := range m.keys {
			if _, declared := s.Get(key); !declared {
				return fmt.Errorf("%s: unknown key '%s'", context, key)
			}
		}
		// Then declared sub-shapes are checked in shape declaration order.
		for _, key := range s.keys {
			if value, present := m.Get(key); present {
				if err := checkShape(context+"."+key, value, s.values[key]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("%s: invalid shape declaration", context)
}

func checkShapeScalar(context string, value any, kind string) error {
	switch kind {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: expected string, got %s", context, PyType(value))
		}
	case "integer":
		// Python: isinstance(value, int) and not isinstance(value, bool).
		if PyType(value) != "int" {
			return fmt.Errorf("%s: expected integer, got %s", context, PyType(value))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %s", context, PyType(value))
		}
	default:
		return fmt.Errorf("%s: invalid scalar type '%s'", context, kind)
	}
	return nil
}

// PlanMergeConfig mirrors merge_config: defaults in field order (has_default
// transports "default is not None"), manifest overlay in manifest order with
// structured validation and unknown-key warnings, the gate_scope blank
// fallback, then required and enum loops in field order. The first problem in
// that sequence is the reported error, matching Python's exception flow. A
// non-nil error return is a process-level (malformed request) failure; a
// semantic failure is result.Error with nil error.
func PlanMergeConfig(req MergeConfigRequest) (MergeConfigResult, error) {
	result := MergeConfigResult{Config: NewOrderedMap(), Warnings: []string{}}

	// Envelope entries are not required to be keyed: _python_merge_config
	// treats the empty string as an ordinary key (an empty field keeps its
	// default, an empty manifest key is an unknown key that only warns), so a
	// keyless entry flows through the same lookup as any other.
	fields := map[string]MergeConfigField{}
	for _, f := range req.Fields {
		fields[f.Key] = f
	}
	shapes := map[string]json.RawMessage{}
	for _, tb := range req.Tables {
		shapes[tb.Key] = tb.Shape
	}

	for _, f := range req.Fields {
		if !f.HasDefault {
			continue
		}
		def, err := DecodeOrdered(f.Default)
		if err != nil {
			return result, fmt.Errorf("field %q default: %w", f.Key, err)
		}
		result.Config.Set(f.Key, def)
	}

	for _, pair := range req.Manifest {
		if _, isField := fields[pair.Key]; isField {
			value, err := DecodeOrdered(pair.Value)
			if err != nil {
				return result, fmt.Errorf("manifest %q value: %w", pair.Key, err)
			}
			result.Config.Set(pair.Key, value)
			continue
		}
		if shapeRaw, isTable := shapes[pair.Key]; isTable {
			shape, err := DecodeOrdered(shapeRaw)
			if err != nil {
				return result, fmt.Errorf("table %q shape: %w", pair.Key, err)
			}
			value, err := DecodeOrdered(pair.Value)
			if err != nil {
				return result, fmt.Errorf("manifest %q value: %w", pair.Key, err)
			}
			if err := ValidateStructured(pair.Key, value, shape); err != nil {
				result.Error = fmt.Sprintf(
					"recipe '%s': invalid config field '%s': %v", req.RecipeName, pair.Key, err)
				return result, nil
			}
			result.Config.Set(pair.Key, value)
			continue
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"recipe '%s': unknown config key '%s' in manifest (ignored)",
			req.RecipeName, pair.Key))
	}

	// Blank/whitespace/falsy gate_scope resolves to auto — but only when the
	// recipe actually declares the field (Python: "gate_scope" in schema_fields).
	if _, declared := fields["gate_scope"]; declared {
		value, present := result.Config.Get("gate_scope")
		if !present || !pyTruthy(value) || strings.TrimSpace(pyString(value)) == "" {
			result.Config.Set("gate_scope", "auto")
		}
	}

	for _, f := range req.Fields {
		if !f.Required {
			continue
		}
		if _, present := result.Config.Get(f.Key); !present {
			result.Error = fmt.Sprintf(
				"recipe '%s': missing required config field '%s'", req.RecipeName, f.Key)
			return result, nil
		}
	}

	for _, f := range req.Fields {
		if len(f.Enum) == 0 {
			continue
		}
		value, present := result.Config.Get(f.Key)
		if !present {
			continue
		}
		valueStr := pyString(value)
		allowed := false
		for _, e := range f.Enum {
			if e == valueStr {
				allowed = true
				break
			}
		}
		if allowed {
			continue
		}
		if f.Key == "gate_impl" {
			result.Error = fmt.Sprintf(
				"invalid gate_impl '%s'; bash has been removed; allowed: auto | go", valueStr)
		} else {
			result.Error = fmt.Sprintf(
				"recipe '%s': config field '%s' value '%s' is invalid; allowed: %s",
				req.RecipeName, f.Key, valueStr, strings.Join(f.Enum, " | "))
		}
		return result, nil
	}

	return result, nil
}
