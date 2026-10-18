package sync

// Native port of env_scaffold.py (Go 20 / U4d-1 + U4d-2,
// S23-S33 of odd/tasks/go-20-u4-init.md).
//
// U4d-1, the NON-interactive half `sync` runs — generate
// <root>/ai-specs.env.example, ensure the managed block in <root>/.envrc,
// warn once per required variable with no value, print "Wrote <path>".
//
// U4d-2, the interactive half the init wizard offers (S28-S33): the prompts
// behind an injected EnvOfferPrompts interface (prompt_env_vars :496), the
// orchestrator OfferHarnessEnv (:572) with the Python's soft-fail lines, the
// WriteHarnessEnv merge that never wipes a secret (:277), the three legacy
// migrations (:386-473) and DirenvAllow (:549). The non-interactive step
// still never runs `direnv allow` (S26); only the offer does (S29).
//
// Ported module surface: main() (env_scaffold.py:644), generate_env_example
// (:300), ensure_root_envrc (:355), collect_env_vars (:90),
// collect_env_allowed (:198), _mcp_env_declarations (:118), _format_dotenv
// (:247), _quote_dotenv (:217), missing_required_values (:266),
// load_harness_env (:258), write_env (:277), prompt_env_vars (:496),
// offer_harness_env (:572), the migrations (:386-473), direnv_allow (:549),
// _is_secret_var/_select_default (:474-495) and the constants at :12-75.
//
// The dotenv subset below (UnquoteEnvValue, ParseDotenv, LoadHarnessEnv,
// HarnessEnvPath, HasManagedBlock and the managed-block constants) is the
// promotion of internal/doctor/checks_env.go's port, following the
// ProjectOwnedRecipeConfig precedent (internal/sync/projectowned.go): the
// decision lives once here so sync and doctor share ONE authority, and
// doctor's local wrappers delegate. The recipe-MCP read side is not
// duplicated: _mcp_env_declarations is composed on this package's existing
// readRecipeMCP (buildrecipemcp.go), the authority the unit's reuse note
// names.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ManagedStart / ManagedEnd / ManagedBody are env_scaffold's
	// MANAGED_START / MANAGED_END / MANAGED_BODY (env_scaffold.py:12-14).
	// The exported names are the shared authority; doctor's private aliases
	// keep its callers and tests unchanged.
	ManagedStart = "# managed-by: ai-specs (do not remove block)"
	ManagedEnd   = "# end managed-by: ai-specs"
	ManagedBody  = "dotenv_if_exists .env\ndotenv_if_exists ai-specs.env"

	// harnessEnvName / harnessEnvExampleName are HARNESS_ENV_NAME and
	// HARNESS_ENV_EXAMPLE_NAME (:16-17).
	harnessEnvName        = "ai-specs.env"
	harnessEnvExampleName = "ai-specs.env.example"

	// envExampleHeader is _ENV_EXAMPLE_HEADER verbatim (:43), trailing
	// newline included; generation rstrips it exactly like the Python
	// `_ENV_EXAMPLE_HEADER.rstrip()`.
	envExampleHeader = `# ai-specs.env.example — committed template (safe to regenerate).
# Copy values into ai-specs.env (gitignored). Root .envrc loads it via direnv.
# Generated from enabled recipes' [[provides.mcp]] env references.
`

	// envrcHeader is the two-line header ensure_root_envrc writes only when
	// the file does not exist yet.
	envrcHeader = "# .envrc — direnv entry for this project\n" +
		"# Harness block managed by ai-specs; safe to add custom lines outside it.\n"
)

var (
	// envDotenvRe mirrors env_scaffold._DOTENV_RE (:27).
	envDotenvRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	// envExportRe mirrors env_scaffold._EXPORT_RE (:23); ParseExports carries
	// it for the U4d-2 migration ports (write_env's siblings), so the parser
	// stays one authority across the seam.
	envExportRe = regexp.MustCompile(`^\s*export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	// EnvReferenceRe mirrors recipe-init.ENV_REFERENCE_RE: only a whole-value
	// `$VAR` / `${env:VAR}` (with an optional stray closing brace) is a
	// reference. Exported because internal/doctor's declarations walk shares
	// the same regex (its private alias keeps existing call sites unchanged).
	EnvReferenceRe = regexp.MustCompile(`^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$`)
)

// envVarHelp is ENV_VAR_HELP (:49-69), verbatim.
var envVarHelp = map[string]string{
	"TRELLO_API_KEY": "Trello API key — create at https://trello.com/power-ups/admin",
	"TRELLO_TOKEN": "Trello token — generate from https://trello.com/power-ups/admin " +
		"(Power-Up → API key page → Token)",
	"CANONICAL_VAULT_PATH": "Absolute path to the project-scoped vault folder (the only env the " +
		"vault MCP reads). Example: /Users/you/.../vault/nnodes/proyectos/app. " +
		"Must be fully resolved — do not leave nested $OTHER_VAR unexpanded.",
	"OPENPROJECT_AUTH": "Optional — `basic` is the provider's effective default when unset; " +
		"use `bearer` for Bearer tokens",
}

// envExampleDefaults is ENV_EXAMPLE_DEFAULTS (:71-75): values pre-filled in
// ai-specs.env.example only, never written to ai-specs.env and never
// overriding an explicit runtime value.
var envExampleDefaults = map[string]string{
	"OPENPROJECT_AUTH": "basic",
}

// HarnessEnvPath mirrors env_scaffold.harness_env_path (:79).
func HarnessEnvPath(projectRoot string) string {
	return filepath.Join(projectRoot, harnessEnvName)
}

// harnessEnvExamplePath mirrors env_scaffold.harness_env_example_path (:83).
func harnessEnvExamplePath(projectRoot string) string {
	return filepath.Join(projectRoot, harnessEnvExampleName)
}

// UnquoteEnvValue mirrors env_scaffold._unquote (:200). Promoted from
// internal/doctor/checks_env.go, whose unquoteEnvValue delegates here.
func UnquoteEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == raw[len(raw)-1] && (raw[0] == '"' || raw[0] == '\'') {
		inner := raw[1 : len(raw)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	// strip inline comments for unquoted values
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return raw
}

// ParseDotenv mirrors env_scaffold._parse_dotenv (:217). Promoted from
// internal/doctor/checks_env.go, whose parseDotenv delegates here.
func ParseDotenv(text string) map[string]string {
	out := map[string]string{}
	for _, line := range pySplitLines(text) {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		match := envDotenvRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		out[match[1]] = UnquoteEnvValue(match[2])
	}
	return out
}

// ParseExports mirrors env_scaffold._parse_exports (:230).
func ParseExports(text string) map[string]string {
	out := map[string]string{}
	for _, line := range pySplitLines(text) {
		match := envExportRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		out[match[1]] = UnquoteEnvValue(match[2])
	}
	return out
}

// LoadHarnessEnv mirrors env_scaffold.load_harness_env (:258): KEY→value from
// ai-specs.env, empty when the file is absent, not a regular file (the legacy
// path.is_file() guard — a FIFO is never opened), unreadable or invalid UTF-8
// (the legacy read_text(encoding="utf-8") raises into the caller's empty-dict
// branch).
func LoadHarnessEnv(projectRoot string) map[string]string {
	path := HarnessEnvPath(projectRoot)
	if !isRegularFilePath(path) {
		return map[string]string{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	if !utf8.Valid(raw) {
		return map[string]string{}
	}
	return ParseDotenv(string(raw))
}

// HasManagedBlock mirrors env_scaffold.has_managed_block (:345).
func HasManagedBlock(text string) bool {
	return strings.Contains(text, ManagedStart) && strings.Contains(text, ManagedEnd)
}

// mcpEnvDeclaration mirrors one tuple from env_scaffold._mcp_env_declarations
// (:118): (recipe_id, preset_id, declared, allowed) where declared maps each
// env declaration key to the $VAR it references (only $VAR references, in
// declaration order) and allowed maps each referenced variable to the preset's
// env_allowed values (U4d-2 — the offer's select reads it, S30).
type mcpEnvDeclaration struct {
	recipeID     string
	presetID     string
	declaredKeys []string
	declared     map[string]string
	allowedKeys  []string
	allowed      map[string][]string
}

// mcpEnvDeclarations mirrors env_scaffold._mcp_env_declarations over the
// package's existing recipe-MCP reader: one entry per [[provides.mcp]]
// preset of an enabled, in-scope recipe whose env table references $VAR
// values. An unreadable manifest is the legacy is_file()/except → [] guard;
// a recipe that cannot be read from the catalog is the legacy
// `except Exception: continue`.
func mcpEnvDeclarations(projectRoot, home string) []mcpEnvDeclaration {
	manifest, err := loadMaterializeManifest(materializeManifestPath(projectRoot))
	if err != nil {
		return nil
	}
	catalogDir := filepath.Join(home, "catalog", "recipes")
	var declarations []mcpEnvDeclaration
	for _, recipe := range loadMaterializeRecipes(manifest) {
		if !recipe.Enabled {
			continue
		}
		loaded, err := readRecipeMCP(catalogDir, recipe.ID)
		if err != nil {
			continue
		}
		for _, preset := range loaded.presets {
			envRaw, ok := preset.config.get("env")
			if !ok {
				continue
			}
			envObj, isObj := envRaw.(*mcpObj)
			if !isObj {
				continue
			}
			declared := map[string]string{}
			var declaredKeys []string
			for _, key := range envObj.keys {
				value, _ := envObj.get(key)
				s, isStr := value.(string)
				if !isStr {
					continue
				}
				match := EnvReferenceRe.FindStringSubmatch(strings.TrimSpace(s))
				if match == nil {
					continue
				}
				declared[key] = match[1]
				declaredKeys = append(declaredKeys, key)
			}
			if len(declared) == 0 {
				continue
			}
			allowed := map[string][]string{}
			var allowedKeys []string
			if rawAllowed, ok := preset.config.get("env_allowed"); ok {
				if allowedObj, isObj := rawAllowed.(*mcpObj); isObj {
					for _, akey := range allowedObj.keys {
						variable, referenced := declared[akey]
						if !referenced {
							continue
						}
						avalue, _ := allowedObj.get(akey)
						list, isList := avalue.([]any)
						if !isList {
							continue
						}
						valid := []string{}
						for _, item := range list {
							if s, isStr := item.(string); isStr && strings.TrimSpace(s) != "" {
								valid = append(valid, s)
							}
						}
						if len(valid) == 0 {
							continue
						}
						allowed[variable] = valid
						allowedKeys = append(allowedKeys, variable)
					}
				}
			}
			declarations = append(declarations, mcpEnvDeclaration{
				recipeID:     recipe.ID,
				presetID:     preset.id,
				declaredKeys: declaredKeys,
				declared:     declared,
				allowedKeys:  allowedKeys,
				allowed:      allowed,
			})
		}
	}
	return declarations
}

// collectEnvVarsInOrder mirrors collect_env_vars' dict semantics: position
// from the FIRST declaration, purpose text updated in place by later ones
// (U4d-2 — prompt_env_vars' banner iterates this order, S30).
type envVarEntry struct {
	name    string
	purpose string
}

func collectEnvVarsInOrder(projectRoot, home string) []envVarEntry {
	var ordered []envVarEntry
	index := map[string]int{}
	for _, decl := range mcpEnvDeclarations(projectRoot, home) {
		for _, key := range decl.declaredKeys {
			variable := decl.declared[key]
			purpose := "required by " + decl.presetID + " (" + decl.recipeID + ")"
			i, seen := index[variable]
			if !seen {
				index[variable] = len(ordered)
				ordered = append(ordered, envVarEntry{name: variable, purpose: purpose})
				continue
			}
			if !strings.Contains(ordered[i].purpose, decl.recipeID) {
				ordered[i].purpose += "; also " + decl.presetID + " (" + decl.recipeID + ")"
			}
		}
	}
	return ordered
}

// CollectEnvVars mirrors env_scaffold.collect_env_vars (:90): {VAR_NAME:
// purpose}, first declaration wins for the purpose text. The export is the
// offer's S28 gate — init_tui.py:390 collects before it offers (U4d-2).
func CollectEnvVars(projectRoot, home string) map[string]string {
	collected := map[string]string{}
	for _, entry := range collectEnvVarsInOrder(projectRoot, home) {
		collected[entry.name] = entry.purpose
	}
	return collected
}

// collectEnvAllowed mirrors env_scaffold.collect_env_allowed (:198):
// {VAR_NAME: [allowed values]} for vars whose recipe preset declares
// env_allowed beside the env reference, first declaration wins (S30's select).
func collectEnvAllowed(projectRoot, home string) map[string][]string {
	collected := map[string][]string{}
	for _, decl := range mcpEnvDeclarations(projectRoot, home) {
		for _, variable := range decl.allowedKeys {
			if _, ok := collected[variable]; !ok {
				collected[variable] = decl.allowed[variable]
			}
		}
	}
	return collected
}

// missingRequiredValues mirrors env_scaffold.missing_required_values (:266):
// the required MCP env vars with no non-empty value in ai-specs.env, sorted.
func missingRequiredValues(projectRoot, home string) []string {
	required := CollectEnvVars(projectRoot, home)
	configured := LoadHarnessEnv(projectRoot)
	var missing []string
	for variable := range required {
		if strings.TrimSpace(configured[variable]) == "" {
			missing = append(missing, variable)
		}
	}
	sort.Strings(missing)
	return missing
}

// quoteDotenv mirrors env_scaffold._quote_dotenv (:217).
func quoteDotenv(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\n\"'#$&*|\\") {
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return value
}

// formatDotenv mirrors env_scaffold._format_dotenv (:247): header lines,
// one blank line, sorted VAR=quoted lines (or the literal no-variable line),
// one trailing newline. Ported with the U4d-1 module surface; its caller
// write_env lands with U4d-2 (S27).
func formatDotenv(values map[string]string, headerLines []string) string {
	lines := make([]string, 0, len(headerLines)+len(values)+3)
	lines = append(lines, headerLines...)
	lines = append(lines, "")
	if len(values) > 0 {
		variables := make([]string, 0, len(values))
		for variable := range values {
			variables = append(variables, variable)
		}
		sort.Strings(variables)
		for _, variable := range variables {
			lines = append(lines, variable+"="+quoteDotenv(values[variable]))
		}
	} else {
		lines = append(lines, "# (no env vars required by enabled recipes)")
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

// generateEnvExample mirrors env_scaffold.generate_env_example (:300) — S24:
// exact bytes, the ENV_VAR_HELP sentence appended when known, the
// ENV_EXAMPLE_DEFAULTS value pre-filled, .bak only when the content changed,
// no rewrite at all when identical. It returns the target path for main's
// `Wrote <path>` line.
func generateEnvExample(projectRoot, home string) (string, error) {
	target := harnessEnvExamplePath(projectRoot)

	varsMap := CollectEnvVars(projectRoot, home)
	var b strings.Builder
	b.WriteString(strings.TrimRight(envExampleHeader, "\n"))
	b.WriteString("\n\n")
	if len(varsMap) > 0 {
		variables := make([]string, 0, len(varsMap))
		for variable := range varsMap {
			variables = append(variables, variable)
		}
		sort.Strings(variables)
		for _, variable := range variables {
			helpBits := varsMap[variable]
			if help, ok := envVarHelp[variable]; ok {
				helpBits += " " + help
			}
			b.WriteString(variable + "=" + envExampleDefaults[variable] + "  # " + helpBits + "\n")
		}
	} else {
		b.WriteString("# (no env vars required by enabled recipes)\n")
	}
	newText := b.String()

	if isRegularFilePath(target) {
		oldRaw, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", target, err)
		}
		// Python read_text(encoding="utf-8") raises before any write on
		// invalid UTF-8; the observable result (fail, no .bak, no rewrite)
		// is kept without porting the traceback.
		if !utf8.Valid(oldRaw) {
			return "", fmt.Errorf("cannot read %s: invalid UTF-8", target)
		}
		if string(oldRaw) == newText {
			return target, nil
		}
		backup := target + ".bak"
		if err := os.WriteFile(backup, oldRaw, 0o666); err != nil {
			return "", fmt.Errorf("cannot write %s: %w", backup, err)
		}
	}
	if err := os.WriteFile(target, []byte(newText), 0o666); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", target, err)
	}
	return target, nil
}

// ensureRootEnvrc mirrors env_scaffold.ensure_root_envrc (:355) — S25: one
// managed block, created with the two-line header when the file is absent,
// replaced in place when the block is present, appended after a blank line
// when it is not; lines outside the block survive byte for byte.
func ensureRootEnvrc(projectRoot string) error {
	target := filepath.Join(projectRoot, ".envrc")
	block := ManagedStart + "\n" + ManagedBody + "\n" + ManagedEnd

	if !isRegularFilePath(target) {
		return writeTextFile(target, envrcHeader+"\n"+block+"\n")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", target, err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("cannot read %s: invalid UTF-8", target)
	}
	text := string(raw)

	if HasManagedBlock(text) {
		// Python: re.sub(escape(START) + ".*?" + escape(END), block, text,
		// count=1) — the first START, lazily to the first END after it. With
		// no END after the START the pattern does not match and text is left
		// untouched (no write), which the end<0 branch reproduces.
		start := strings.Index(text, ManagedStart)
		rest := text[start+len(ManagedStart):]
		end := strings.Index(rest, ManagedEnd)
		if end < 0 {
			return nil
		}
		regionEnd := start + len(ManagedStart) + end + len(ManagedEnd)
		newText := text[:start] + block + text[regionEnd:]
		if newText == text {
			return nil
		}
		if !strings.HasSuffix(newText, "\n") {
			newText += "\n"
		}
		return writeTextFile(target, newText)
	}

	sep := "\n"
	if strings.HasSuffix(text, "\n") {
		sep = ""
	}
	return writeTextFile(target, text+sep+"\n"+block+"\n")
}

// writeTextFile is target.write_text(..., encoding="utf-8") with the Python
// open() mode: 0666 & ^umask for a new file, the existing file's mode kept
// by the truncating open for a rewrite. It is the shared writer for the
// Python's text writes — the managed .envrc and, since U4d-2, ai-specs.env.
func writeTextFile(path, text string) error {
	if err := os.WriteFile(path, []byte(text), 0o666); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

// RunEnvScaffold is env_scaffold.main() (:644) — S23: the non-interactive
// env step `sync` runs. It refuses a missing manifest (stderr + exit 1),
// writes the example, ensures the managed .envrc block, warns per required
// variable with no value, and prints `Wrote <path>` last. It never prompts
// and never runs direnv allow (S26); the interactive half stays on the
// Python seam until U4d-2 (S27).
func RunEnvScaffold(projectRoot, home string, stdout, stderr io.Writer) int {
	// Python: root = Path(args.path).resolve() — the same resolve the sync
	// spine's target resolver already applies, so the step's diagnostics and
	// paths render the resolved absolute root.
	root := resolveNonStrict(projectRoot)
	if !isRegularFilePath(filepath.Join(root, "ai-specs", "ai-specs.toml")) {
		fmt.Fprintf(stderr, "Project not initialized: missing ai-specs/ai-specs.toml under %s\n", root)
		return 1
	}
	path, err := generateEnvExample(root, home)
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	if err := ensureRootEnvrc(root); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	for _, variable := range missingRequiredValues(root, home) {
		fmt.Fprintf(stderr, "! %s has no value in ai-specs.env — run ai-specs configure-recipes\n", variable)
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)
	return 0
}

// ---------------------------------------------------------------------------
// U4d-2 (S28-S33): the interactive half — the wizard's env offer.
// ---------------------------------------------------------------------------

// ErrEnvOfferCancelled is questionary's None: a prompt that answered nothing,
// or — per S30's amendment — the DECLINED offer question. env_scaffold.py
// collapses decline and cancel alike into `return None` (:522) and
// offer_harness_env returns on None (:631), so this error stops the whole
// offer before anything is written.
var ErrEnvOfferCancelled = errors.New("env offer cancelled")

// EnvOfferPrompts is the injected prompt seam for the offer (S29):
// internal/cli satisfies it with internal/tui over the wizard's own key
// source, a test satisfies it with scripted answers. Every method answers
// ErrEnvOfferCancelled for "questionary returned None" (Ctrl-C, EOF, decline)
// and any other error for "questionary raised", which offer_harness_env
// reports with the Python's `Could not configure environment variables: …`.
type EnvOfferPrompts interface {
	// Confirm asks the offer question (questionary.confirm).
	Confirm(message string, def bool) (bool, error)
	// Select asks a constrained choice with initial preselected on the cursor
	// (questionary.select's default).
	Select(message string, choices []string, initial string) (string, error)
	// Password asks a hidden prompt (questionary.password's instruction token).
	Password(message, instruction string) (string, error)
	// Text asks a free-text prompt returning the raw buffer (questionary.text).
	Text(message, defaultValue string) (string, error)
}

// promptEnvVars mirrors env_scaffold.prompt_env_vars (:496) — S30: the banner
// in first-declaration order with each purpose and its ENV_VAR_HELP sentence,
// the offer question (decline == cancel), then the prompts SORTED — a select
// over the recipe's declared env_allowed values with _select_default's value
// preselected, a hidden prompt when the name looks like a secret, a raw text
// prompt otherwise — with the help sentence printed before each prompt it
// belongs to. Answers come back verbatim; the never-wipe rule lives in
// write_env's blank check, not in the prompt.
func promptEnvVars(projectRoot, home string, prompts EnvOfferPrompts, stdout io.Writer) (map[string]string, error) {
	varsMap := collectEnvVarsInOrder(projectRoot, home)
	if len(varsMap) == 0 {
		return map[string]string{}, nil
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Required environment variables")
	for _, entry := range varsMap {
		fmt.Fprintf(stdout, "  %s — %s\n", entry.name, entry.purpose)
		if help, ok := envVarHelp[entry.name]; ok {
			fmt.Fprintf(stdout, "    ℹ️  %s\n", help)
		}
	}
	fmt.Fprintln(stdout)

	confirmed, err := prompts.Confirm("Configure these values now?", true)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		// S30, amended: declining is NOT "continue without values" — the
		// Python's `if not … : return None` (:522) returns None for a decline
		// and for a cancel alike, and the offer returns on None.
		return nil, ErrEnvOfferCancelled
	}

	existing := LoadHarnessEnv(projectRoot)
	allowedMap := collectEnvAllowed(projectRoot, home)

	variables := make([]string, 0, len(varsMap))
	for _, entry := range varsMap {
		variables = append(variables, entry.name)
	}
	sort.Strings(variables)

	result := map[string]string{}
	for _, variable := range variables {
		if help, ok := envVarHelp[variable]; ok {
			fmt.Fprintf(stdout, "ℹ️  %s\n", help)
		}
		var value string
		choices := allowedMap[variable]
		switch {
		case len(choices) > 0:
			value, err = prompts.Select(variable, choices, selectDefault(variable, choices, existing))
		case isSecretVar(variable):
			value, err = prompts.Password(variable, "(hidden input)")
		default:
			value, err = prompts.Text(variable, "")
		}
		if err != nil {
			return nil, err
		}
		result[variable] = value
	}
	return result, nil
}

// selectDefault mirrors _select_default (:480) — S30's three steps: a matching
// existing value (strip()ed, case-insensitive), else the matching example
// default, else the first declared choice. A value that matches nothing (a
// stale existing or example value) falls through instead of being answered,
// so the select never offers a default outside its choices.
func selectDefault(variable string, choices []string, existing map[string]string) string {
	if len(choices) == 0 {
		// questionary refuses an empty choice list outright; the prompt seam
		// only reaches this helper with one, and a Go panic here would abort
		// the offer instead of soft-failing.
		return ""
	}
	canonical := map[string]string{}
	for _, choice := range choices {
		canonical[strings.ToLower(choice)] = choice // last wins, like the dict comprehension
	}
	if current := strings.TrimSpace(existing[variable]); current != "" {
		if match, ok := canonical[strings.ToLower(current)]; ok {
			return match
		}
	}
	if example, ok := envExampleDefaults[variable]; ok {
		if match, ok := canonical[strings.ToLower(example)]; ok {
			return match
		}
	}
	return choices[0]
}

// isSecretVar mirrors _is_secret_var (:474): the five keywords, uppercase,
// substring match.
func isSecretVar(variable string) bool {
	upper := strings.ToUpper(variable)
	for _, keyword := range []string{"API_KEY", "TOKEN", "SECRET", "PASSWORD", "APIKEY"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// WriteHarnessEnv mirrors env_scaffold.write_env (:277) — S31: merge into the
// existing ai-specs.env and write it with the Python's header and formatDotenv's
// sorted, quoted lines. The merge never destroys: a blank/whitespace answer is
// skipped, so a re-prompt with Enter cannot wipe a non-empty secret (the file's
// own docstring rule, pinned by tests/test_env_scaffold.py); a non-blank answer
// replaces the previous value — that is the re-prompt that UPDATES a secret;
// every other existing key keeps its own value. The project-root .env is never
// touched: only ai-specs.env is written.
func WriteHarnessEnv(projectRoot string, varValues map[string]string) (string, error) {
	target := HarnessEnvPath(projectRoot)
	existing := map[string]string{}
	if isRegularFilePath(target) {
		raw, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", target, err)
		}
		// Python read_text(encoding="utf-8") raises before any merge on
		// invalid UTF-8; the observable result (fail, no write) is kept
		// without porting the traceback.
		if !utf8.Valid(raw) {
			return "", fmt.Errorf("cannot read %s: invalid UTF-8", target)
		}
		existing = ParseDotenv(string(raw))
	}
	merged := make(map[string]string, len(existing)+len(varValues))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range varValues {
		if strings.TrimSpace(value) == "" {
			continue
		}
		merged[key] = value
	}
	header := []string{
		"# ai-specs.env — generated by ai-specs configure-recipes",
		"# gitignored. Regenerate values via: ai-specs configure-recipes",
	}
	if err := writeTextFile(target, formatDotenv(merged, header)); err != nil {
		return "", err
	}
	return target, nil
}

// mergeIntoHarnessEnv mirrors _merge_into_harness_env (:386): non-empty
// EXISTING values win and incoming only fills keys that are missing or empty —
// the migrations' never-wipe rule (S32), which is the one place the incoming
// map cannot replace a configured secret.
func mergeIntoHarnessEnv(incoming, existing map[string]string) map[string]string {
	writeValues := make(map[string]string, len(existing)+len(incoming))
	for key, value := range existing {
		writeValues[key] = value
	}
	for key, value := range incoming {
		if current, ok := writeValues[key]; !ok || current == "" {
			writeValues[key] = value
		}
	}
	return writeValues
}

// parseNestedHarnessEnv mirrors _parse_nested_harness_file (:410): dotenv
// first, then the export lines fill the gaps only (S32 — the export migration
// that follows must never override a dotenv value).
func parseNestedHarnessEnv(text string) map[string]string {
	parsed := ParseDotenv(text)
	for key, value := range ParseExports(text) {
		if current, ok := parsed[key]; !ok || strings.TrimSpace(current) == "" {
			parsed[key] = value
		}
	}
	return parsed
}

// migrateEnvFile is the shared body of migrate_nested_harness_env (:414) and
// migrate_legacy_envrc (:441): parse, refuse when nothing parsed (JD-6 — no
// rename without a key to merge), merge under the never-wipe rule, write, and
// only after a SUCCESSFUL write rename the source to .bak (S32). parse is the
// per-file parser: nested parses dotenv+export, legacy parses export only.
func migrateEnvFile(projectRoot, source, bak string, parse func(string) map[string]string) (bool, error) {
	if !isRegularFilePath(source) {
		return false, nil
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", source, err)
	}
	if !utf8.Valid(raw) {
		return false, fmt.Errorf("cannot read %s: invalid UTF-8", source)
	}
	parsed := parse(string(raw))
	if len(parsed) == 0 {
		return false, nil
	}
	existing := LoadHarnessEnv(projectRoot)
	writeValues := mergeIntoHarnessEnv(parsed, existing)
	// Persist the merged map; write_env's blank omit keeps non-empty secrets
	// from being wiped by an empty existing or incoming value.
	if _, err := WriteHarnessEnv(projectRoot, writeValues); err != nil {
		return false, err
	}
	if err := os.Remove(bak); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("cannot remove %s: %w", bak, err)
	}
	if err := os.Rename(source, bak); err != nil {
		return false, fmt.Errorf("cannot rename %s: %w", source, err)
	}
	if err := ensureRootEnvrc(projectRoot); err != nil {
		return false, err
	}
	return true, nil
}

// migrateNestedHarnessEnv mirrors migrate_nested_harness_env (:414): the
// nested ai-specs/.env into the root ai-specs.env, renamed to .env.bak only
// after a successful write and only when at least one key parsed (S32/JD-6).
func migrateNestedHarnessEnv(projectRoot string) (bool, error) {
	source := filepath.Join(projectRoot, "ai-specs", ".env")
	return migrateEnvFile(projectRoot, source, source+".bak", parseNestedHarnessEnv)
}

// migrateLegacyEnvrc mirrors migrate_legacy_envrc (:441): the legacy
// ai-specs/.envrc exports, renamed to .envrc.bak on the same two conditions.
func migrateLegacyEnvrc(projectRoot string) (bool, error) {
	source := filepath.Join(projectRoot, "ai-specs", ".envrc")
	return migrateEnvFile(projectRoot, source, source+".bak", ParseExports)
}

// migrateLegacyHarnessEnv mirrors migrate_legacy_harness_env (:464): nested
// dotenv FIRST so the export migration that follows fills gaps only (S32).
func migrateLegacyHarnessEnv(projectRoot string) (bool, error) {
	ran := false
	if nestedRan, err := migrateNestedHarnessEnv(projectRoot); err != nil {
		return ran, err
	} else if nestedRan {
		ran = true
	}
	if legacyRan, err := migrateLegacyEnvrc(projectRoot); err != nil {
		return ran, err
	} else if legacyRan {
		ran = true
	}
	return ran, nil
}

// DirenvAllow mirrors env_scaffold.direnv_allow (:549): `direnv allow <root>`
// with its output captured (never inherited), a 10s timeout, and Python's
// error split: a non-zero exit, a missing binary (FileNotFoundError) and a
// timeout (TimeoutExpired) answer (false, nil) — the caller prints the
// three-line message — while any other start failure propagates, which is the
// Python exception offer_harness_env reports as `direnv allow failed: …`.
func DirenvAllow(projectRoot string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "direnv", "allow", projectRoot)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, context.DeadlineExceeded) {
		return false, nil
	}
	return false, err
}

// harnessEnvWritePhase is offer_harness_env's single try block (:631-639): the
// values write (skipped when there are none), the example regeneration and the
// managed .envrc block. Any error stops the phase — the caller prints
// `Could not write harness env: …` and skips the direnv step, exactly like the
// Python's except → return.
func harnessEnvWritePhase(root, home string, values map[string]string, stdout io.Writer) error {
	if len(values) > 0 {
		path, err := WriteHarnessEnv(root, values)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ wrote %s\n", path)
	}
	if _, err := generateEnvExample(root, home); err != nil {
		return err
	}
	if err := ensureRootEnvrc(root); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ root .envrc managed block → %s\n", filepath.Join(root, ".envrc"))
	return nil
}

// OfferHarnessEnv mirrors env_scaffold.offer_harness_env (:572) — S29: in
// order, migrate the legacy files, collect the variables, prompt, write
// ai-specs.env, regenerate the example and the managed .envrc block, run
// direnv allow. Every branch soft-fails with the Python's own line on the
// Python's stream instead of aborting the verb: migration and write failures
// and prompt exceptions go to stdout (rich Console()'s default), the direnv
// three-liner goes to stderr (print file=sys.stderr). A cancelled or declined
// prompt returns silently, before anything is written (S30).
//
// S33, deferred and named here: when direnv is missing the Python offers to
// INSTALL it through the deps layer (dep_install.offer_and_install). That
// consent offer belongs to the deps surface, not to init, so this port skips
// it and lets the failure below print the Python's three-line message.
func OfferHarnessEnv(projectRoot, home string, prompts EnvOfferPrompts, stdout, stderr io.Writer) {
	// Python: init_tui's main() resolves the target before run_wizard, so the
	// offer's paths and ✓ lines carry Path.resolve()'s root — the same resolve
	// the sync spine's env step applies.
	root := resolveNonStrict(projectRoot)

	if _, err := migrateLegacyHarnessEnv(root); err != nil {
		fmt.Fprintf(stdout, "Legacy harness env migration skipped: %v\n", err)
	}

	if len(CollectEnvVars(root, home)) == 0 {
		return
	}

	values, err := promptEnvVars(root, home, prompts, stdout)
	if err != nil {
		if errors.Is(err, ErrEnvOfferCancelled) {
			return
		}
		fmt.Fprintf(stdout, "Could not configure environment variables: %v\n", err)
		return
	}

	if err := harnessEnvWritePhase(root, home, values, stdout); err != nil {
		fmt.Fprintf(stdout, "Could not write harness env: %v\n", err)
		return
	}

	if ok, err := DirenvAllow(root); err != nil {
		fmt.Fprintf(stdout, "direnv allow failed: %v\n", err)
	} else if !ok {
		fmt.Fprint(stderr, "  ! direnv is not installed or could not be run.\n")
		fmt.Fprint(stderr, "    Install it with: brew install direnv\n")
		fmt.Fprint(stderr, "    Then run: direnv allow\n")
	} else {
		fmt.Fprint(stdout, "  ✓ direnv allow — variables are now active in this terminal\n")
	}
}
