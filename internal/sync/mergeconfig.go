package sync

// Native root authority for the recipe config merge (GO-07 S13 PR2b).
//
// The decision core is the shared package
// (ai-specs.dev/worktree-gate/shared.PlanMergeConfig) the gate binary's
// --plan-merge-config surface also runs, so the root single binary and the gate
// share one merge decision. This file owns only the root boundary: it acquires
// the recipe's declared config schema and the manifest's [recipes.<id>.config]
// overrides natively and maps them onto the shared request envelope. S14 owns
// the sync-pipeline wiring; this authority is added, not called from the
// pipeline.
//
// It reproduces the Python envelope builder _merge_config_request_envelope
// (lib/_internal/recipe-materialize.py:2842) and the decision _python_merge_config
// (:3006). The request the core decodes is
//
//	{"recipe_name": str,
//	 "fields":    [{"key","required","has_default","default","enum"}, ...],
//	 "tables":    [{"key","shape"}, ...],
//	 "manifest":  [{"key","value"}, ...]}
//
// with fields/tables in recipe declaration order and manifest pairs in manifest
// document order. The result is {config, warnings, error}; a semantic validation
// failure is data (Error set, nil Go error, exit 0 in the gate) and an
// acquisition or serialization failure is a Go error (the Python bridge's
// transport failure, which falls back once to the temporary Python authority).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/worktree-gate/shared"
)

// MergeConfigRequest is the root boundary input for one config merge: the
// recipe.toml to read the schema from, the manifest to read the overrides from,
// and the [recipes.<id>.config] key to read them under (the manifest id the
// Python authority takes from its enabled-recipe map).
type MergeConfigRequest struct {
	RecipePath   string
	ManifestPath string
	RecipeID     string
}

// PlanMergeConfig acquires the native inputs, builds the ordered request the
// shared core decodes, and delegates the decision. A non-nil error is an
// acquisition/serialization failure — an unreadable recipe or manifest, a TOML
// date/time the envelope's json.dumps cannot serialize, or a non-finite float
// Python's json.dumps(allow_nan=False) rejects. A semantic validation failure is
// result.Error with a nil error, exactly as the gate envelope reports it.
func PlanMergeConfig(req MergeConfigRequest) (shared.MergeConfigResult, error) {
	sreq, err := acquireMergeConfigRequest(req)
	if err != nil {
		return shared.MergeConfigResult{}, err
	}
	return shared.PlanMergeConfig(sreq)
}

// acquireMergeConfigRequest converts the native recipe/manifest inputs into the
// ordered shared request, mirroring _merge_config_request_envelope.
func acquireMergeConfigRequest(req MergeConfigRequest) (shared.MergeConfigRequest, error) {
	recipe, err := schema.LoadRecipeToml(req.RecipePath)
	if err != nil {
		return shared.MergeConfigRequest{}, err
	}
	// ConfigSchema.Fields is a map, so declaration order is lost; the Python
	// envelope preserves it. Re-parse the same file through internal/toml, whose
	// tables keep document order, to recover the exact field and table order.
	raw, err := os.ReadFile(req.RecipePath)
	if err != nil {
		return shared.MergeConfigRequest{}, err
	}
	doc, err := toml.Parse(raw)
	if err != nil {
		return shared.MergeConfigRequest{}, err
	}
	configTable, _ := doc.Table("config")

	fields := []shared.MergeConfigField{}
	tables := []shared.MergeConfigTable{}
	if recipe.ConfigSchema != nil && configTable != nil {
		for _, key := range configTable.Keys() {
			if table, isTable := recipe.ConfigSchema.Tables[key]; isTable {
				shape, err := table.ShapeJSON()
				if err != nil {
					return shared.MergeConfigRequest{}, fmt.Errorf("table %q shape: %w", key, err)
				}
				tables = append(tables, shared.MergeConfigTable{Key: key, Shape: shape})
				continue
			}
			field, isField := recipe.ConfigSchema.Fields[key]
			if !isField {
				continue
			}
			var def json.RawMessage
			if field.Default != nil {
				def, err = pyJSONValue(field.Default)
				if err != nil {
					return shared.MergeConfigRequest{}, fmt.Errorf("field %q default: %w", key, err)
				}
			}
			enum := field.Enum
			if enum == nil {
				enum = []string{}
			}
			fields = append(fields, shared.MergeConfigField{
				Key:        key,
				Required:   field.Required,
				HasDefault: field.Default != nil,
				Default:    def,
				Enum:       enum,
			})
		}
	}

	manifest, err := acquireManifestConfig(req.ManifestPath, req.RecipeID)
	if err != nil {
		return shared.MergeConfigRequest{}, err
	}

	return shared.MergeConfigRequest{
		RecipeName: recipe.Name,
		Fields:     fields,
		Tables:     tables,
		Manifest:   manifest,
	}, nil
}

// acquireManifestConfig reads [recipes.<id>.config] in document order, the way
// the Python authority reads cfg.get("config", {}) from its enabled-recipe map.
// A manifest without that table yields no pairs.
func acquireManifestConfig(manifestPath, recipeID string) ([]shared.MergeConfigPair, error) {
	data, err := config.LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	recipes, _ := data.Table("recipes")
	if recipes == nil {
		return []shared.MergeConfigPair{}, nil
	}
	entry, _ := recipes.Table(recipeID)
	if entry == nil {
		return []shared.MergeConfigPair{}, nil
	}
	cfg, _ := entry.Table("config")
	if cfg == nil {
		return []shared.MergeConfigPair{}, nil
	}
	pairs := []shared.MergeConfigPair{}
	for _, key := range cfg.Keys() {
		value, _ := cfg.Get(key)
		encoded, err := pyJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("manifest %q value: %w", key, err)
		}
		pairs = append(pairs, shared.MergeConfigPair{Key: key, Value: encoded})
	}
	return pairs, nil
}

// pyJSONValue renders one native TOML value the way Python's
// json.dumps(..., allow_nan=False) would: floats keep their float form ("1.0",
// never "1"), a wide integer keeps its exact decimal digits, tables and
// array-of-tables keep document order, and a non-finite float or a date/time
// is rejected with Python's serialization error so int/float survives the
// shared json.Number path and a datetime never silently becomes null.
func pyJSONValue(value any) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := writePyJSON(&buf, value); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

func writePyJSON(buf *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return writeJSONString(buf, v)
	case int64:
		buf.WriteString(strconv.FormatInt(v, 10))
	case toml.BigInt:
		// Python json.dumps renders an arbitrary-precision int in full.
		buf.WriteString(string(v))
	case toml.DateTime:
		return fmt.Errorf("Object of type %s is not JSON serializable", v.PyTypeName())
	case float64:
		text, err := pyJSONFloat(v)
		if err != nil {
			return err
		}
		buf.WriteString(text)
	case []any:
		buf.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writePyJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case []*toml.Table:
		buf.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writePyJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case *toml.Table:
		buf.WriteByte('{')
		for i, key := range v.Keys() {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSONString(buf, key); err != nil {
				return err
			}
			buf.WriteByte(':')
			item, _ := v.Get(key)
			if err := writePyJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("Object of type %T is not JSON serializable", value)
	}
	return nil
}

// writeJSONString writes one JSON string literal. SetEscapeHTML(false) keeps
// the encoding to the escapes Python's json.dumps needs; the decoder that
// consumes the request decodes both forms identically.
func writeJSONString(buf *bytes.Buffer, s string) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	buf.Write(bytes.TrimSuffix(tmp.Bytes(), []byte("\n")))
	return nil
}

// pyJSONFloat renders a finite float exactly as Python's json.dumps renders its
// repr(float): shortest round-trip, scientific notation when the decimal point
// position is <= -4 or > 16, and a forced ".0" on integral values. NaN and the
// infinities are rejected with Python's ValueError message, because
// allow_nan=False refuses them (a Go encoding/json-only path would instead
// render an int for 1.0 and lose the int/float distinction).
func pyJSONFloat(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("Out of range float values are not JSON compliant")
	}
	neg := math.Signbit(f)
	a := math.Abs(f)
	if a == 0 {
		if neg {
			return "-0.0", nil
		}
		return "0.0", nil
	}
	e := strconv.FormatFloat(a, 'e', -1, 64) // e.g. "1.5e+22"
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1 // position of the decimal point among the digits

	var s string
	if decpt <= -4 || decpt > 16 {
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		expStr := strconv.Itoa(exp10)
		if exp10 < 0 {
			expStr = expStr[1:]
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
	return s, nil
}
