package sync

// Go port of lib/_internal/brief-render-policy.py: the managed AGENTS.md
// opt-out policy. `[brief].render = false` disables the brief; any other value
// is the fail-safe enabled default so a typo never silently drops AGENTS.md
// for bash callers. The strict gate is --validate (and doctor).

import (
	"fmt"
	"os"

	"ai-specs.dev/ai-specs/internal/toml"
)

// loadBriefRender reads [brief].render from the manifest. present is false
// when the key (or the whole [brief] table) is absent; err covers an unreadable
// or unparseable manifest, which brief-render-policy.py's main() reports as
// `error: <exc>` with exit 1.
func loadBriefRender(tomlPath string) (value any, present bool, err error) {
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, false, err
	}
	root, err := toml.Parse(data)
	if err != nil {
		return nil, false, err
	}
	brief, ok := root.Table("brief")
	if !ok || brief == nil {
		return nil, false, nil
	}
	v, ok := brief.Get("render")
	return v, ok, nil
}

// EvaluateBriefRender is brief-render-policy.py in its default (non-validate)
// mode: [brief].render = false disables the brief; a non-boolean value falls
// back to the fail-safe enabled default. A missing or unparseable manifest is
// an error; callers print `error: ...` and skip the brief exactly as the Python
// `$(...)`-ignored exit status does.
func EvaluateBriefRender(tomlPath string) (bool, error) {
	v, present, err := loadBriefRender(tomlPath)
	if err != nil {
		return false, err
	}
	if !present {
		return true, nil
	}
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return true, nil
}

// ValidateBriefRender is the --validate mode: a non-boolean [brief].render is an
// error whose message is byte-identical to brief_render_enabled's ValueError.
func ValidateBriefRender(tomlPath string) error {
	v, present, err := loadBriefRender(tomlPath)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if _, ok := v.(bool); ok {
		return nil
	}
	return fmt.Errorf("[brief].render must be a boolean (true or false); got %s", pyTypeName(v))
}

// pyTypeName mirrors Python type(value).__name__ for the manifest value kinds
// reachable from a TOML document.
func pyTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case int, int64:
		return "int"
	case float64:
		return "float"
	case []any:
		return "list"
	case *toml.Table, map[string]any:
		return "dict"
	default:
		return "object"
	}
}
