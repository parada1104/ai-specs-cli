package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// manifestCache holds one run's view of ai-specs/ai-specs.toml. It mirrors
// doctor._load_manifest: a missing, unreadable or unparseable manifest is
// reported as an empty manifest, never as an error; the dedicated `manifest`
// check reports those states separately.
type manifestCache struct {
	loaded   bool
	exists   bool
	data     *toml.Table
	parseErr error
}

// manifestPath mirrors `self.root / "ai-specs" / "ai-specs.toml"`.
func (d *Doctor) manifestPath() string {
	return filepath.Join(d.Root, "ai-specs", "ai-specs.toml")
}

func (d *Doctor) loadManifest() {
	if d.manifest.loaded {
		return
	}
	d.manifest.loaded = true
	path := d.manifestPath()
	if !isFile(path) {
		return
	}
	d.manifest.exists = true
	data, err := os.ReadFile(path)
	if err != nil {
		d.manifest.parseErr = err
		return
	}
	table, err := toml.Parse(data)
	if err != nil {
		d.manifest.parseErr = err
		return
	}
	d.manifest.data = table
}

// manifestData is doctor._load_manifest(): the parsed table, or nil for the
// `{}` fallback.
func (d *Doctor) manifestData() *toml.Table {
	d.loadManifest()
	return d.manifest.data
}

// manifestHasContent mirrors `if not manifest` for the brief-render check: an
// absent, empty or unparseable manifest is falsy.
func (d *Doctor) manifestHasContent() bool {
	data := d.manifestData()
	return data != nil && len(data.Keys()) > 0
}

// truthy mirrors Python truthiness for the value types internal/toml produces.
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

// briefRenderError is the ValueError brief_render_enabled raises for a
// non-boolean [brief].render.
type briefRenderError struct{ typeName string }

func (e *briefRenderError) Error() string {
	return "[brief].render must be a boolean (true or false); got " + e.typeName
}

// briefRenderEnabled mirrors brief-render-policy.brief_render_enabled: false
// only when [brief].render is exactly `false` (identity, not truthiness).
func briefRenderEnabled(manifest *toml.Table) (bool, error) {
	var brief *toml.Table
	if manifest != nil {
		brief, _ = manifest.Table("brief")
	}
	if brief == nil {
		return true, nil
	}
	raw, present := brief.Get("render")
	if !present {
		return true, nil
	}
	if b, isBool := raw.(bool); isBool {
		return b, nil
	}
	return false, &briefRenderError{typeName: pyTypeName(raw)}
}

// briefRenderDisabled mirrors doctor._brief_render_disabled: an invalid render
// type counts as render enabled.
func (d *Doctor) briefRenderDisabled() bool {
	enabled, err := briefRenderEnabled(d.manifestData())
	if err != nil {
		return false
	}
	return !enabled
}

// legacyRecipeVersions mirrors the list comprehension in
// _check_legacy_recipe_versions: recipe entries carrying a `version` key whose
// value is neither None nor the empty string. Returned sorted.
func legacyRecipeVersions(data *toml.Table) []string {
	if data == nil {
		return nil
	}
	recipes, ok := data.Table("recipes")
	if !ok {
		return nil
	}
	var legacy []string
	for _, rid := range recipes.Keys() {
		cfg, ok := recipes.Table(rid)
		if !ok {
			continue
		}
		value, present := cfg.Get("version")
		if !present || value == nil {
			continue
		}
		if s, isStr := value.(string); isStr && s == "" {
			continue
		}
		legacy = append(legacy, rid)
	}
	sort.Strings(legacy)
	return legacy
}

// pyTypeName mirrors Python's type(v).__name__ for the value types
// internal/toml produces.
func pyTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case int64:
		return "int"
	case float64:
		return "float"
	case string:
		return "str"
	case []any, []*toml.Table:
		return "list"
	case *toml.Table:
		return "dict"
	}
	return "object"
}

// pythonRepr mirrors Python's repr() for the TOML value types doctor
// interpolates with {value!r} (the [tool].policy diagnostics).
func pythonRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return pyFloatRepr(x)
	case string:
		return pyReprString(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, pythonRepr(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []*toml.Table:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, pythonRepr(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *toml.Table:
		parts := make([]string, 0, len(x.Keys()))
		for _, key := range x.Keys() {
			value, _ := x.Get(key)
			parts = append(parts, pyReprString(key)+": "+pythonRepr(value))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprintf("%v", v)
}

// pyFloatRepr mirrors Python's repr() for floats closely enough for a
// diagnostic: integral values keep a trailing .0.
func pyFloatRepr(f float64) string {
	if f == float64(int64(f)) && f < 1e16 && f > -1e16 {
		return strconv.FormatInt(int64(f), 10) + ".0"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// pyReprString mirrors Python's repr() for a str.
func pyReprString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}
