package sync

// Native root authority for the recipe hooks runner (GO-07 S13 PR4a).
//
// It mirrors the Python authority execute_hooks
// (lib/_internal/recipe-materialize.py:3069): iterate recipe.Hooks in
// declaration order and dispatch, raising on the first validate-config failure
// (which the caller turns into a failed sync). bootstrap-board writes a real
// marker file, so — unlike the read-only resolved-config authority — this one
// owns a durable write. S14 owns the sync-pipeline wiring; this authority is
// added, not called from the pipeline.
//
// Native building blocks are reused, never re-invented: internal/schema for the
// recipe and its config schema, internal/projectcache.RecipeSkillsRoot for the
// marker directory and internal/config.PyStr for Python str() parity.
//
// The error, info and warning strings are byte-for-byte the Python f-strings,
// including the two-space "  \u2139 " / "  ! " prefixes info()/warn() add.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// HooksRequest is the root boundary input for one recipe-hook run: the
// recipe.toml whose [[hooks]] to execute, the already-merged recipe config, and
// the roots the bootstrap-board marker resolves against. It mirrors the Python
// authority's execute_hooks(recipe, merged_config, project_root, cli_home).
type HooksRequest struct {
	RecipePath   string
	MergedConfig map[string]any
	ProjectRoot  string
	CLIHome      string
}

// RunRecipeHooks executes the recipe's hooks in declaration order. A validation
// failure is returned as an error carrying the exact Python RuntimeError
// message (an exception fails sync); the deferred actions print their info line
// to stdout and the unknown action prints its warning to stderr, both then
// continuing.
func RunRecipeHooks(req HooksRequest, stdout, stderr io.Writer) error {
	recipe, err := schema.LoadRecipeToml(req.RecipePath)
	if err != nil {
		return err
	}
	// ConfigSchema.Fields is a map, so declaration order is lost; the Python
	// authority iterates schema_fields.items() in recipe declaration order, so
	// re-parse the recipe through internal/toml, whose tables keep document
	// order, to recover it (the same technique mergeconfig.go uses).
	order, err := hooksConfigFieldOrder(req.RecipePath)
	if err != nil {
		return err
	}

	for _, hook := range recipe.Hooks {
		switch hook.Action {
		case "validate-config":
			if err := hooksValidateConfig(recipe, order, req.MergedConfig); err != nil {
				return err
			}
		case "bootstrap-board":
			if err := hooksBootstrapBoard(recipe, req.MergedConfig, req.ProjectRoot, req.CLIHome); err != nil {
				return err
			}
		case "link-trello-card", "sync-card-state", "comment-verification":
			fmt.Fprintf(stdout, "  \u2139 recipe '%s': hook '%s' deferred to agent runtime\n", recipe.Name, hook.Action)
		default:
			fmt.Fprintf(stderr, "  ! recipe '%s': unknown hook action '%s' (skipped)\n", recipe.Name, hook.Action)
		}
	}
	return nil
}

// hooksConfigFieldOrder returns the [config] keys in recipe declaration order.
func hooksConfigFieldOrder(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := toml.Parse(raw)
	if err != nil {
		return nil, err
	}
	configTable, _ := doc.Table("config")
	if configTable == nil {
		return nil, nil
	}
	return configTable.Keys(), nil
}

// hooksValidateConfig mirrors the validate-config branch, field by field in
// declaration order: a required field missing from the merged config raises; an
// absent field is skipped; a present value is stringified with Python str()
// parity; an 8-character alphanumeric board_id raises the shortLink error; and
// a configured field.validation.regex must re.fullmatch the value.
func hooksValidateConfig(recipe *schema.Recipe, order []string, merged map[string]any) error {
	if recipe.ConfigSchema == nil {
		return nil
	}
	for _, key := range order {
		field, isField := recipe.ConfigSchema.Fields[key]
		if !isField {
			continue
		}
		if field.Required {
			if _, ok := merged[key]; !ok {
				return fmt.Errorf(
					"recipe '%s': hook 'validate-config' failed: missing required config field '%s'",
					recipe.Name, key,
				)
			}
		}
		value, ok := merged[key]
		if !ok {
			continue
		}
		valueStr := hooksPyStr(value)
		if key == "board_id" && hooksIsShortLink(valueStr) {
			return fmt.Errorf(
				"recipe '%s': hook 'validate-config' failed: field 'board_id' value '%s' looks like a Trello shortLink; the real board ID is 24 hex characters (e.g., '69ec0a2099ea20956e371d62')",
				recipe.Name, valueStr,
			)
		}
		pattern, err := hooksRegexPattern(field.Validation)
		if err != nil {
			return err
		}
		if pattern == "" {
			continue
		}
		matched, err := hooksFullMatch(pattern, valueStr)
		if err != nil {
			return err
		}
		if !matched {
			return fmt.Errorf(
				"recipe '%s': hook 'validate-config' failed: field '%s' value '%s' does not match required pattern '%s'",
				recipe.Name, key, valueStr, pattern,
			)
		}
	}
	return nil
}

// hooksRegexPattern mirrors `pattern = validation.get("regex", "")` plus the
// `if pattern:` truthiness guard. A truthy non-string pattern would make
// Python's re.fullmatch raise TypeError; the same message is returned here.
func hooksRegexPattern(validation map[string]any) (string, error) {
	raw, present := validation["regex"]
	if !present || !pyTruthy(raw) {
		return "", nil
	}
	pattern, isStr := raw.(string)
	if !isStr {
		return "", errors.New("first argument must be string or compiled pattern")
	}
	return pattern, nil
}

// hooksFullMatch applies a Python re.fullmatch: the whole value must match.
// Go regexp is RE2 (no backreferences or lookarounds) and its \d\w\s classes
// are ASCII-only, so a Python-valid pattern can fail to compile or match a
// different set; those divergences are recorded by
// TestRecipeHooksRegexEngineDivergence rather than silently accepted.
func hooksFullMatch(pattern, value string) (bool, error) {
	re, err := regexp.Compile(`\A(?:` + pattern + `)\z`)
	if err != nil {
		return false, err
	}
	return re.MatchString(value), nil
}

// hooksBootstrapBoard mirrors the bootstrap-board branch: create the recipe's
// skills root with parents and write the three-line bootstrap-ready marker.
func hooksBootstrapBoard(recipe *schema.Recipe, merged map[string]any, projectRoot, cliHome string) error {
	markerDir := filepath.Join(projectcache.RecipeSkillsRoot(projectRoot, cliHome), recipe.ID)
	if err := os.MkdirAll(markerDir, 0o755); err != nil {
		return err
	}
	content := "board_id=" + hooksFString(merged, "board_id", "") + "\n" +
		"default_list=" + hooksFString(merged, "default_list", "In Progress") + "\n" +
		"epic_list=" + hooksFString(merged, "epic_list", "Epic") + "\n"
	return os.WriteFile(filepath.Join(markerDir, "bootstrap-ready"), []byte(content), 0o644)
}

// hooksFString mirrors an f-string interpolation of `merged.get(key, default)`:
// the default only applies when the key is absent, and a present value is
// stringified with str().
func hooksFString(merged map[string]any, key, def string) string {
	if value, ok := merged[key]; ok {
		return hooksPyStr(value)
	}
	return def
}

// hooksPyStr reuses internal/config.PyStr for Python str() parity over TOML
// values, with one gap closed: PyStr(nil) is "" while Python str(None) is
// "None". No repr port is added.
func hooksPyStr(value any) string {
	if value == nil {
		return "None"
	}
	return config.PyStr(value)
}

// hooksIsShortLink mirrors `len(value_str) == 8 and value_str.isalnum()`:
// Python len() counts code points, not bytes.
func hooksIsShortLink(value string) bool {
	return utf8.RuneCountInString(value) == 8 && hooksIsAlnum(value)
}

// hooksIsAlnum mirrors str.isalnum(): at least one character and every
// character a Unicode letter or number (Python alpha|decimal|digit|numeric).
func hooksIsAlnum(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
