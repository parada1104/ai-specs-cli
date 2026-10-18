package sync

// Native port of env_scaffold.py's NON-interactive half (Go 20 / U4d-1,
// S23-S27 of odd/tasks/go-20-u4-init.md): the step `sync` runs — generate
// <root>/ai-specs.env.example, ensure the managed block in <root>/.envrc,
// warn once per required variable with no value, print "Wrote <path>".
//
// Ported module surface: main() (env_scaffold.py:644), generate_env_example
// (:300), ensure_root_envrc (:355), collect_env_vars (:90),
// _mcp_env_declarations (:118), _format_dotenv (:247), _quote_dotenv (:217),
// missing_required_values (:266), load_harness_env (:258) and the constants
// at :12-75.
//
// NOT ported, deferred to U4d-2 and named here so the seam is explicit (S27):
// prompt_env_vars, offer_harness_env, write_env (the merge that never wipes a
// non-empty secret with a blank answer), the three legacy migrations
// (migrate_nested_harness_env, migrate_legacy_envrc, migrate_legacy_harness_env)
// and direnv allow — this step never runs `direnv allow` (S26), the Python
// main() does not either.
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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
// (:118): (recipe_id, preset_id, declared) where declared maps each env
// declaration key to the $VAR it references (only $VAR references, in
// declaration order). env_allowed is not collected — U4d-1 never consumes it.
type mcpEnvDeclaration struct {
	recipeID     string
	presetID     string
	declaredKeys []string
	declared     map[string]string
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
			declarations = append(declarations, mcpEnvDeclaration{
				recipeID:     recipe.ID,
				presetID:     preset.id,
				declaredKeys: declaredKeys,
				declared:     declared,
			})
		}
	}
	return declarations
}

// collectEnvVars mirrors env_scaffold.collect_env_vars (:90): {VAR_NAME:
// purpose}, first declaration wins for the purpose text.
func collectEnvVars(projectRoot, home string) map[string]string {
	collected := map[string]string{}
	for _, decl := range mcpEnvDeclarations(projectRoot, home) {
		for _, key := range decl.declaredKeys {
			variable := decl.declared[key]
			purpose := "required by " + decl.presetID + " (" + decl.recipeID + ")"
			existing, ok := collected[variable]
			if !ok {
				collected[variable] = purpose
				continue
			}
			if !strings.Contains(existing, decl.recipeID) {
				collected[variable] = existing + "; also " + decl.presetID + " (" + decl.recipeID + ")"
			}
		}
	}
	return collected
}

// missingRequiredValues mirrors env_scaffold.missing_required_values (:266):
// the required MCP env vars with no non-empty value in ai-specs.env, sorted.
func missingRequiredValues(projectRoot, home string) []string {
	required := collectEnvVars(projectRoot, home)
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

	varsMap := collectEnvVars(projectRoot, home)
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
		return writeEnvrcFile(target, envrcHeader+"\n"+block+"\n")
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
		return writeEnvrcFile(target, newText)
	}

	sep := "\n"
	if strings.HasSuffix(text, "\n") {
		sep = ""
	}
	return writeEnvrcFile(target, text+sep+"\n"+block+"\n")
}

// writeEnvrcFile is target.write_text(..., encoding="utf-8") with the Python
// open() mode: 0666 & ^umask for a new file, the existing file's mode kept
// by the truncating open for a rewrite.
func writeEnvrcFile(path, text string) error {
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
