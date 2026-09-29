package doctor

// Native port of the env_scaffold.py subset doctor._check_harness_env_layout
// needs: the harness env path/parsers, the managed .envrc block predicates, and
// collect_env_vars / collect_env_allowed over the enabled recipes' MCP env
// declarations. Byte-identical message surface; the parser semantics are the
// legacy ones (quoted values keep their body, inline comments are stripped only
// for unquoted values, `$VAR` references only, declaration order preserved).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/toml"
)

const (
	managedStart = "# managed-by: ai-specs (do not remove block)"
	managedEnd   = "# end managed-by: ai-specs"
	managedBody  = "dotenv_if_exists .env\ndotenv_if_exists ai-specs.env"

	harnessEnvName = "ai-specs.env"
)

var (
	// envExportRe mirrors env_scaffold._EXPORT_RE (leading whitespace, `export `,
	// then a shell-style assignment).
	envExportRe = regexp.MustCompile(`^\s*export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	// envDotenvRe mirrors env_scaffold._DOTENV_RE.
	envDotenvRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	// envReferenceRe mirrors recipe-init.ENV_REFERENCE_RE: only a whole-value
	// `$VAR` / `${env:VAR}` (with an optional stray closing brace) is a reference.
	envReferenceRe = regexp.MustCompile(`^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$`)
)

// harnessEnvPath mirrors env_scaffold.harness_env_path.
func harnessEnvPath(projectRoot string) string {
	return filepath.Join(projectRoot, harnessEnvName)
}

// unquoteEnvValue mirrors env_scaffold._unquote.
func unquoteEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == raw[len(raw)-1] && (raw[0] == '"' || raw[0] == '\'') {
		inner := raw[1 : len(raw)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	// strip inline comments for unquoted values
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = strings.TrimRightFunc(raw[:idx], unicode.IsSpace)
	}
	return raw
}

// pySplitLines mirrors Python str.splitlines() for the line boundaries that can
// appear in a dotenv / export / SHA256SUMS file. A trailing break yields no
// extra empty element, exactly as Python's splitlines does.
func pySplitLines(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			out = append(out, string(runes[start:i]))
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			start = i + 1
			continue
		}
		if pyLineBreak(r) {
			out = append(out, string(runes[start:i]))
			start = i + 1
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

// pyLineBreak reports the non-CR line boundaries Python's splitlines uses.
func pyLineBreak(r rune) bool {
	switch r {
	case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\x85', '\u2028', '\u2029':
		return true
	}
	return false
}

// parseDotenv mirrors env_scaffold._parse_dotenv.
func parseDotenv(text string) map[string]string {
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
		out[match[1]] = unquoteEnvValue(match[2])
	}
	return out
}

// parseExports mirrors env_scaffold._parse_exports (used by the legacy
// migration paths; kept here as part of the ported env_scaffold subset).
func parseExports(text string) map[string]string {
	out := map[string]string{}
	for _, line := range pySplitLines(text) {
		match := envExportRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		out[match[1]] = unquoteEnvValue(match[2])
	}
	return out
}

// loadHarnessEnv mirrors env_scaffold.load_harness_env. An unreadable or
// undecodable file is the caller's legacy exception branch ({}).
func loadHarnessEnv(projectRoot string) map[string]string {
	path := harnessEnvPath(projectRoot)
	if !isFile(path) {
		return map[string]string{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	// The legacy read_text(encoding="utf-8") raises on invalid UTF-8; the check
	// catches it and falls back to {}. Go strings tolerate arbitrary bytes, so
	// the equivalent empty result is returned here explicitly.
	if !utf8.Valid(raw) {
		return map[string]string{}
	}
	return parseDotenv(string(raw))
}

// hasManagedBlock mirrors env_scaffold.has_managed_block.
func hasManagedBlock(text string) bool {
	return strings.Contains(text, managedStart) && strings.Contains(text, managedEnd)
}

// managedBlockIsCurrent mirrors env_scaffold.managed_block_is_current: the
// markers exist and the FIRST body between them matches the canonical body.
func managedBlockIsCurrent(text string) bool {
	if !hasManagedBlock(text) {
		return false
	}
	start := strings.Index(text, managedStart)
	rest := text[start+len(managedStart):]
	end := strings.Index(rest, managedEnd)
	if end < 0 {
		return false
	}
	return strings.TrimSpace(rest[:end]) == managedBody
}

// mcpEnvDeclaration mirrors one tuple from env_scaffold._mcp_env_declarations.
// declared/allowed keep declaration order so first-declaration-wins and the
// purpose text match the legacy dict insertion order.
type mcpEnvDeclaration struct {
	recipeID     string
	presetID     string
	declaredKeys []string
	declared     map[string]string
	allowedKeys  []string
	allowed      map[string][]string
}

// mcpEnvDeclarations mirrors env_scaffold._mcp_env_declarations: one entry per
// [[provides.mcp]] preset of an enabled recipe whose env table references
// `$VAR` values. The catalog root is the CLI install root (AI_SPECS_HOME).
func mcpEnvDeclarations(projectRoot, home string) []mcpEnvDeclaration {
	manifest := filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
	if !isFile(manifest) {
		return nil
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return nil
	}
	data, err := toml.Parse(raw)
	if err != nil {
		return nil
	}
	recipes, _ := data.Table("recipes")
	if recipes == nil {
		return nil
	}
	var declarations []mcpEnvDeclaration
	for _, recipeID := range recipes.Keys() {
		entry, ok := recipes.Table(recipeID)
		if !ok {
			continue
		}
		if enabled, isBool := entry.Bool("enabled"); !isBool || !enabled {
			continue
		}
		recipe, err := readCatalogRecipe(home, recipeID)
		if err != nil {
			continue
		}
		for _, preset := range recipe.MCP {
			envRaw, ok := preset.Config["env"]
			if !ok {
				continue
			}
			envTbl, isTbl := envRaw.(*toml.Table)
			if !isTbl {
				continue
			}
			declared := map[string]string{}
			var declaredKeys []string
			for _, key := range envTbl.Keys() {
				value, _ := envTbl.Get(key)
				s, isStr := value.(string)
				if !isStr {
					continue
				}
				match := envReferenceRe.FindStringSubmatch(strings.TrimSpace(s))
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
			if allowedRaw, ok := preset.Config["env_allowed"]; ok {
				if allowedTbl, isTbl := allowedRaw.(*toml.Table); isTbl {
					for _, akey := range allowedTbl.Keys() {
						variable, ok := declared[akey]
						if !ok {
							continue
						}
						avalue, _ := allowedTbl.Get(akey)
						list, isList := avalue.([]any)
						if !isList {
							continue
						}
						valid := []string{}
						for _, item := range list {
							s, isStr := item.(string)
							if isStr && strings.TrimSpace(s) != "" {
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
				recipeID:     recipeID,
				presetID:     preset.ID,
				declaredKeys: declaredKeys,
				declared:     declared,
				allowedKeys:  allowedKeys,
				allowed:      allowed,
			})
		}
	}
	return declarations
}

// collectEnvVars mirrors env_scaffold.collect_env_vars: {VAR_NAME: purpose},
// first declaration wins for the purpose text.
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

// collectEnvAllowed mirrors env_scaffold.collect_env_allowed: {VAR_NAME:
// [allowed values]}, first declaration wins.
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

// checkHarnessEnvLayout is doctor._check_harness_env_layout. The legacy
// `_load_env_scaffold()` None guard is unreachable for a compiled binary (the
// module is not loadable), so only the empty-declaration early return remains.
func (d *Doctor) checkHarnessEnvLayout() {
	varsMap := collectEnvVars(d.Root, d.Home)
	if len(varsMap) == 0 {
		return
	}

	if whichBinary("direnv") {
		d.add(OK, "direnv", "direnv available on PATH")
	} else {
		d.add(WARN, "direnv",
			"direnv not on PATH (needed to load harness MCP env for shells)",
			"brew install direnv && direnv allow  # or see https://direnv.net")
	}

	envrc := filepath.Join(d.Root, ".envrc")
	envrcText := ""
	isCurrent := false
	if isFile(envrc) {
		if raw, err := os.ReadFile(envrc); err == nil {
			envrcText = string(raw)
		}
		isCurrent = managedBlockIsCurrent(envrcText)
	}
	if !isCurrent {
		stale := envrcText != "" && hasManagedBlock(envrcText)
		message := "project-root .envrc missing ai-specs managed block"
		if stale {
			message = "project-root .envrc has stale ai-specs managed block"
		}
		d.add(WARN, "envrc-managed", message,
			"run ai-specs configure-recipes to ensure root .envrc")
	} else {
		d.add(OK, "envrc-managed", "project-root .envrc has ai-specs managed block")
	}

	present := loadHarnessEnv(d.Root)
	var missing []string
	for variable := range varsMap {
		if strings.TrimSpace(present[variable]) == "" {
			missing = append(missing, variable)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		d.add(WARN, "harness-env",
			"missing/empty in ai-specs.env: "+strings.Join(missing, ", "),
			"run ai-specs configure-recipes to set harness env values")
	} else {
		d.add(OK, "harness-env",
			fmt.Sprintf("ai-specs.env has %d required MCP env key(s)", len(varsMap)))
	}

	allowedMap := collectEnvAllowed(d.Root, d.Home)
	allowedVars := make([]string, 0, len(allowedMap))
	for variable := range allowedMap {
		allowedVars = append(allowedVars, variable)
	}
	sort.Strings(allowedVars)
	for _, variable := range allowedVars {
		choices := allowedMap[variable]
		configured := strings.TrimSpace(present[variable])
		if configured == "" {
			continue
		}
		valid := false
		for _, choice := range choices {
			if strings.EqualFold(choice, configured) {
				valid = true
				break
			}
		}
		if !valid {
			d.add(WARN, "harness-env-value",
				"invalid value for "+variable+" in ai-specs.env (allowed: "+strings.Join(choices, ", ")+")",
				"run ai-specs configure-recipes to pick a valid value")
		}
	}
}
