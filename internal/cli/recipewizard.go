package cli

// GO-20 U6d-2 (T4b): the config wizard core — lib/_internal/config_wizard.py's
// run_config_wizard ported onto internal/tui behind an injected prompt seam,
// and the recipe add verb's narrowed TTY wiring (T3b, first half).
//
// The contract this file ports, from config_wizard.py:
//
//   - iterate sorted(fields) over recipe.config_schema.fields — never .extra
//     and never the structured tables;
//   - print the field's help_text (when present) immediately before its prompt;
//   - the message is `<recipe_id>.<key> (<required|optional>, <kind>)`, kind
//     being the declared type, or "enum" only when no type is declared, or
//     "string" when neither is (Python: `field.type or ("enum" if field.enum
//     else "string")` — a declared TYPE wins even on an enum field, while the
//     enum still drives the BRANCH, `if field.enum:` first);
//   - the default resolves in _field_default's order: existing_config[key] if
//     the key is present, else field.default if set, else False for a bool,
//     else "" — rendered through Python's str() for the text prefill (list and
//     table defaults keep their repr) and Python's bool() for the confirm;
//   - enum → a select over the declared choices with that default when it IS
//     one of them and the FIRST choice when it is not; bool → a confirm with
//     bool(default); everything else → a TEXT prompt;
//   - the text validator is one of three shapes: required-only ("This field is
//     required." on a blank line), regex-only (empty ALWAYS passes, otherwise
//     `Must match <pattern>` on a failed prefix match — re.match semantics),
//     or composed (required checked FIRST, then the regex), and the prompt
//     RE-PROMPTS on failure showing the validator's message (questionary's
//     validate= — internal/tui's TextValidated);
//   - a blank answer OMITS the key so write-back keeps the existing/default
//     value; a cancelled prompt returns "nothing" (Python {}) and the caller
//     skips the write.
//
// The wizard NEVER writes: it only collects answers. The caller owns the
// write-back through config.UpdateRecipeConfig (U6d-1's promoted writer), and
// an unparseable validation.regex surfaces as an error here so the caller can
// word it like the Python's `except Exception` arm.
//
// U4b-2 IS UNBLOCKED BY THIS FILE (S7): init's recipe-configuration step will
// consume exactly these pieces — runConfigWizard over internal/schema's
// ConfigSchema (the same *schema.ConfigField metadata recipe.toml parsing
// already produces) with its own prompt seam backed by internal/tui — plus the
// caller-side write-back helper sortedWizardConfigValues feeding
// config.UpdateRecipeConfig. Nothing else of recipe add's verb is needed.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/ai-specs/internal/tui"
)

// wizardPrompts is the injected prompt seam: production wires internal/tui
// (tuiWizardPrompts below), a test scripts answers and reads one ordered
// transcript of every call. Help is the rich console line config_wizard prints
// before each prompt; Select/Confirm/Text are the three questionary prompts
// the core branches to, Text carrying the validate= handler that re-prompts
// inside the one prompt (questionary's contract).
type wizardPrompts interface {
	// Help prints the field's help_text before its prompt.
	Help(text string)
	// Select asks for one of choices, starting on initial.
	Select(message string, choices []string, initial string) (string, bool)
	// Confirm asks a yes/no question with def as the default.
	Confirm(message string, def bool) (bool, bool)
	// Text asks for a line with defaultValue as the prefill, running
	// validate over each submitted line until it passes (nil = accept).
	Text(message, defaultValue string, validate func(string) error) (string, bool)
}

// runConfigWizard is config_wizard.run_config_wizard. It returns the value
// map with ok=true on completion (possibly empty: every answer blank), or
// nil/ok=false when a prompt was cancelled (Python's `{}` — the caller skips
// the write), or an error when a declared validation.regex does not compile
// (Python's re.error escaping to the caller's `except Exception`).
func runConfigWizard(recipeID string, cs *schema.ConfigSchema, existing map[string]any, p wizardPrompts) (map[string]any, bool, error) {
	result := map[string]any{}
	if cs == nil || len(cs.Fields) == 0 {
		return result, true, nil
	}
	keys := make([]string, 0, len(cs.Fields))
	for key := range cs.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys) // Python's sorted(fields)
	for _, key := range keys {
		field := cs.Fields[key]
		if field.HelpText != "" {
			p.Help(field.HelpText)
		}
		message := wizardPromptMessage(recipeID, key, field)
		def := wizardFieldDefault(field, existing, key)

		if len(field.Enum) > 0 {
			choices := field.Enum
			initial := choices[0]
			if s, isStr := def.(string); isStr {
				for _, c := range choices {
					if c == s {
						initial = s
						break
					}
				}
			}
			answer, ok := p.Select(message, choices, initial)
			if !ok {
				return nil, false, nil
			}
			result[key] = answer
			continue
		}

		if field.Type == "bool" {
			answer, ok := p.Confirm(message, wizardTruthy(def))
			if !ok {
				return nil, false, nil
			}
			result[key] = answer
			continue
		}

		validate, err := wizardTextValidator(field)
		if err != nil {
			return nil, false, err
		}
		answer, ok := p.Text(message, wizardTextDefault(def), validate)
		if !ok {
			return nil, false, nil
		}
		if strings.TrimSpace(answer) == "" {
			// Blank → omit (keep existing/default).
			continue
		}
		result[key] = answer
	}
	return result, true, nil
}

// wizardPromptMessage is _prompt_message: the kind is the declared type, or
// "enum" only when no type is declared, else "string".
func wizardPromptMessage(recipeID, key string, field *schema.ConfigField) string {
	req := "optional"
	if field.Required {
		req = "required"
	}
	kind := field.Type
	if kind == "" {
		if len(field.Enum) > 0 {
			kind = "enum"
		} else {
			kind = "string"
		}
	}
	return fmt.Sprintf("%s.%s (%s, %s)", recipeID, key, req, kind)
}

// wizardFieldDefault is _field_default's four steps verbatim.
func wizardFieldDefault(field *schema.ConfigField, existing map[string]any, key string) any {
	if v, ok := existing[key]; ok {
		return v
	}
	if field.Default != nil {
		return field.Default
	}
	if field.Type == "bool" {
		return false
	}
	return ""
}

// wizardTruthy is Python's bool() over a TOML-domain default: non-empty
// strings and collections are truthy (bool("no") is True), 0 and "" are not,
// any other object is.
func wizardTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case []any:
		return len(t) > 0
	case *toml.Table:
		return len(t.Keys()) > 0
	default:
		return true
	}
}

// wizardTextDefault is Python's str() as the questionary text prefill: the
// string as-is, True/False, decimal ints, repr-shaped floats, and repr of
// lists/tables (`['alpha', 'beta']`, `{'k': 'v'}`) so an untouched Enter
// hands the write-back the same string the Python would.
func wizardTextDefault(v any) string {
	switch t := v.(type) {
	case nil:
		return "" // `"" if default is None else str(default)`
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return wizardPyFloat(t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = wizardPyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *toml.Table:
		keys := t.Keys()
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			val, _ := t.Get(k)
			parts = append(parts, wizardPyRepr(k)+": "+wizardPyRepr(val))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// wizardPyRepr is repr() for elements inside a collection: strings are
// single-quoted, everything else renders through wizardTextDefault.
func wizardPyRepr(v any) string {
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	}
	return wizardTextDefault(v)
}

// wizardPyFloat is repr() for a float: fixed notation with a trailing .0 for
// magnitudes Python keeps fixed (1e-4 .. 1e16), exponential otherwise, and
// the three special values spelled Python's way.
func wizardPyFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	expPart := strconv.FormatFloat(f, 'e', -1, 64)
	if i := strings.LastIndexByte(expPart, 'e'); i >= 0 {
		if exp, err := strconv.Atoi(expPart[i+1:]); err == nil && exp >= -4 && exp < 16 {
			s := strconv.FormatFloat(f, 'f', -1, 64)
			if !strings.Contains(s, ".") {
				s += ".0"
			}
			return s
		}
	}
	return expPart
}

// wizardRequiredValidator is _required_validator: blank after strip() fails
// with the frozen message.
func wizardRequiredValidator(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("This field is required.")
	}
	return nil
}

// wizardTextValidator builds the validate= handler run_config_wizard builds:
// the composed shape (required FIRST) when both are declared, the regex alone,
// required alone, or no validator at all.
//
// The pattern compiles as `^(?:<pattern>)`: Python's re.match anchors at the
// start of the value, which Go's MatchString does not do by itself (the
// grouped wrap also keeps a leading `|` from escaping the anchor).
func wizardTextValidator(field *schema.ConfigField) (func(string) error, error) {
	pattern := wizardRegexPattern(field.Validation)
	switch {
	case pattern != "" && field.Required:
		rx, err := regexp.Compile("^(?:" + pattern + ")")
		if err != nil {
			return nil, err
		}
		return func(value string) error {
			if err := wizardRequiredValidator(value); err != nil {
				return err
			}
			return wizardRegexCheck(rx, pattern, value)
		}, nil
	case pattern != "":
		rx, err := regexp.Compile("^(?:" + pattern + ")")
		if err != nil {
			return nil, err
		}
		return func(value string) error {
			return wizardRegexCheck(rx, pattern, value)
		}, nil
	case field.Required:
		return wizardRequiredValidator, nil
	}
	return nil, nil
}

// wizardRegexCheck is _regex_validator: an empty value always passes.
func wizardRegexCheck(rx *regexp.Regexp, pattern, value string) error {
	if value == "" {
		return nil
	}
	if rx.MatchString(value) {
		return nil
	}
	return errors.New("Must match " + pattern)
}

// wizardRegexPattern is `str(field.validation.get("regex") or "")`.
func wizardRegexPattern(validation map[string]any) string {
	if validation == nil {
		return ""
	}
	raw, present := validation["regex"]
	if !present || raw == nil || !wizardTruthy(raw) {
		return ""
	}
	return wizardTextDefault(raw)
}

// sortedWizardConfigValues turns the wizard's map into the writer's ordered
// assignments: run_config_wizard inserts keys in sorted(fields) order, which
// is Python's dict order and therefore the order UpdateRecipeConfig appends
// new keys in — Go's map order must not leak into the manifest bytes.
func sortedWizardConfigValues(values map[string]any) []config.ConfigValue {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]config.ConfigValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, config.ConfigValue{Key: k, Value: values[k]})
	}
	return out
}

// wizardExistingConfig is configure_selected_recipes' existing_by_id read for
// one recipe: [recipes.<id>.config] as a plain map, empty when the recipe
// block or its config table is absent (Python's `dict(entry.get("config") or
// {})` through recipe-config-write's _existing_config shape).
func wizardExistingConfig(manifest *toml.Table, recipeID string) map[string]any {
	existing := map[string]any{}
	if manifest == nil {
		return existing
	}
	_, _, cfg := config.RecipeConfigState(manifest, recipeID)
	if cfg == nil {
		return existing
	}
	for _, key := range cfg.Keys() {
		if v, ok := cfg.Get(key); ok {
			existing[key] = v
		}
	}
	return existing
}

// recipeAddWizardOpener produces the TTY configure leg's prompt seam over the
// given streams, plus a closer that restores the terminal, or a non-zero rc
// meaning the TUI is unavailable before any prompt is drawn (the
// openInitWizardOpener pattern, split so a test scripts the answers).
type recipeAddWizardOpener func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int)

// tuiWizardPrompts wires the core onto internal/tui's three primitives: Help
// prints config_wizard's help line, Select routes to SelectDefault (the
// choices are their own titles — questionary renders bare strings), Confirm
// to Confirm, and Text to TextValidated so the validator re-prompts inside
// the one prompt.
type tuiWizardPrompts struct {
	out  io.Writer
	keys tui.KeySource
}

func (p tuiWizardPrompts) Help(text string) {
	// config_wizard prints the help through a rich console before prompting.
	fmt.Fprintf(p.out, "ℹ️  %s\n", text)
}

func (p tuiWizardPrompts) Select(message string, choices []string, initial string) (string, bool) {
	tuiChoices := make([]tui.Choice, len(choices))
	for i, c := range choices {
		tuiChoices[i] = tui.Choice{Title: c, Value: c}
	}
	return tui.SelectDefault(p.out, p.keys, message, tuiChoices, initial)
}

func (p tuiWizardPrompts) Confirm(message string, def bool) (bool, bool) {
	return tui.Confirm(p.out, p.keys, message, def)
}

func (p tuiWizardPrompts) Text(message, defaultValue string, validate func(string) error) (string, bool) {
	return tui.TextValidated(p.out, p.keys, message, defaultValue, validate)
}

// openRecipeWizardPrompts is the production opener: the same raw-mode
// takeover openInitWizardKeys uses — a non-terminal, a non-file stdin or a
// failed takeover refuses with rc 3 before anything is drawn.
func openRecipeWizardPrompts(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
	if !isTTY(stdin, stdout) {
		return nil, nil, 3
	}
	file, ok := stdin.(*os.File)
	if !ok {
		return nil, nil, 3
	}
	term, err := tui.NewTerminal(file)
	if err != nil {
		return nil, nil, 3
	}
	return tuiWizardPrompts{out: stdout, keys: term}, term.Close, 0
}
