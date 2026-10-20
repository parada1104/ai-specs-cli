// Package recipe's init brief is the port of lib/_internal/recipe-init.py
// (GO-20 U7-2): a read-only, agent-readable setup brief for one recipe. The
// wrapper grammar lives in RunInit (lib/recipe-init.sh's argument loop); the
// brief itself in BuildInitBrief. Nothing here writes to the project tree.
package recipe

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// initUsage is lib/recipe-init.sh's usage() heredoc byte for byte: a QUOTED
// heredoc, so the text is literal and the trailing newline is part of it.
const initUsage = `Usage: ai-specs recipe init <id> [path] [--help]
Print a read-only, agent-readable initialization brief for a recipe.
Arguments:
  id      Recipe identifier (directory name under catalog/recipes/)
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// InitOptions carries the two catalogue anchors: the AI_SPECS_HOME value
// (home-first candidate) and the running CLI's install root (fallback).
type InitOptions struct {
	Home        string
	InstallRoot string
}

var (
	initEnvReferenceRe = regexp.MustCompile(`^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$`)
	initSecretKeyRe    = regexp.MustCompile(`(?i)(key|token|secret|password|api|auth)`)
)

// initRecipeEntry is read_recipes reduced to the fields the brief uses.
type initRecipeEntry struct {
	enabled bool
	config  *toml.Table
}

// initManifest is the handful of manifest sections build_init_brief reads.
type initManifest struct {
	projectName   string
	enabledAgents []string
	recipes       map[string]initRecipeEntry
	mcp           map[string]any
	activeBinding int
}

// RunInit is lib/recipe-init.sh: its argument loop first, then the cd/pwd
// target resolution, then recipe-init.py's main() (the brief, or the error on
// stderr with exit 1).
func RunInit(args []string, opts InitOptions, stdout, stderr io.Writer) int {
	var recipeID, target string
	takePositional := func(arg string) int {
		if recipeID == "" {
			recipeID = arg
			return 0
		}
		if target == "" {
			target = arg
			return 0
		}
		fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
		return 2
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, initUsage)
			return 0
		case arg == "--":
			for i++; i < len(args); i++ {
				if code := takePositional(args[i]); code != 0 {
					return code
				}
			}
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs recipe init --help' for usage.")
			return 2
		default:
			if code := takePositional(arg); code != 0 {
				return code
			}
		}
	}

	if recipeID == "" {
		fmt.Fprintln(stderr, "ERROR: missing recipe id")
		fmt.Fprint(stderr, initUsage)
		return 2
	}
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "cd: .: %s\n", err)
			return 1
		}
		target = cwd
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		fmt.Fprintf(stderr, "cd: %s: %s\n", target, err)
		return 1
	}

	brief, err := BuildInitBrief(abs, recipeID, opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, brief)
	return 0
}

// BuildInitBrief mirrors build_init_brief: load the manifest, resolve the
// recipe from the catalogue, then render the ordered sections.
func BuildInitBrief(projectRoot, recipeID string, opts InitOptions) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		root = filepath.Clean(projectRoot)
	}
	manifest, err := loadInitManifest(filepath.Join(root, "ai-specs", "ai-specs.toml"))
	if err != nil {
		return "", err
	}
	loaded, recipeDir, err := loadInitRecipe(recipeID, opts)
	if err != nil {
		return "", err
	}

	promptBytes, err := os.ReadFile(filepath.Join(recipeDir, loaded.Init.Prompt))
	if err != nil {
		return "", err
	}
	promptText := strings.TrimRightFunc(string(promptBytes), unicode.IsSpace)

	state := initInstallState(recipeID, manifest.recipes)
	description := loaded.Init.Description
	if description == "" {
		description = "(none)"
	}
	needsMCP := "(none)"
	if len(loaded.Init.NeedsMCP) > 0 {
		needsMCP = strings.Join(loaded.Init.NeedsMCP, ", ")
	}
	agents := "(none)"
	if len(manifest.enabledAgents) > 0 {
		agents = strings.Join(manifest.enabledAgents, ", ")
	}
	projectName := manifest.projectName
	if projectName == "" {
		projectName = "(unnamed)"
	}

	lines := []string{
		"# Recipe Init Brief",
		"",
		"## Recipe",
		"",
		"- ID: " + loaded.ID,
		"- Name: " + loaded.Name,
		"- Version: " + loaded.Version,
		"- Description: " + loaded.Description,
		"- Install state: " + state,
		"",
		"## Init Workflow",
		"",
		"- Prompt: " + loaded.Init.Prompt,
		"- Description: " + description,
		"- Needs manifest: " + strconv.FormatBool(loaded.Init.NeedsManifest),
		"- Needs MCP: " + needsMCP,
		"",
		"## Prompt Content",
		"",
		promptText,
		"",
		"## Manifest Context",
		"",
		"- Project: " + projectName,
		"- Enabled agents: " + agents,
		"- Active bindings: " + strconv.Itoa(manifest.activeBinding),
		"",
	}
	lines = append(lines, initConfigLines(loaded, recipeID, manifest.recipes)...)
	lines = append(lines, initMCPLines(loaded, manifest.mcp)...)
	lines = append(lines, initTemplateLines(loaded, root)...)
	lines = append(lines,
		"## Reviewable Next Actions",
		"",
		"- Ask the project-specific setup questions from the prompt.",
		fmt.Sprintf("- Review any proposed `[recipes.%s.config]` changes before editing the manifest.", recipeID),
		"- Run `ai-specs sync` only after humans approve durable configuration changes.",
		"- No files were changed. Init does not run sync or materialize primitives.",
		"",
	)
	return strings.Join(lines, "\n"), nil
}

// loadInitManifest is _load_manifest plus the toml-read section reads.
func loadInitManifest(manifestPath string) (initManifest, error) {
	info, err := os.Stat(manifestPath)
	if err != nil || info.IsDir() {
		return initManifest{}, fmt.Errorf("Project not initialized. Run: ai-specs init")
	}
	data, err := config.LoadManifest(manifestPath)
	if err != nil {
		return initManifest{}, err
	}

	out := initManifest{recipes: map[string]initRecipeEntry{}, mcp: map[string]any{}}
	if project, ok := data.Table("project"); ok && project != nil {
		if v, ok := project.Get("name"); ok && config.Truthy(v) {
			out.projectName = config.PyStr(v)
		}
	}
	if agents, ok := data.Table("agents"); ok && agents != nil {
		out.enabledAgents = initStringList(agents, "enabled")
	}
	if recipes, ok := data.Table("recipes"); ok && recipes != nil {
		for _, id := range recipes.Keys() {
			v, _ := recipes.Get(id)
			table, ok := v.(*toml.Table)
			if !ok {
				continue
			}
			entry := initRecipeEntry{config: nil}
			entry.enabled, _ = table.Bool("enabled")
			if cfg, ok := table.Table("config"); ok {
				entry.config = cfg
			}
			out.recipes[id] = entry
		}
	}
	if mcp, ok := data.Table("mcp"); ok && mcp != nil {
		for _, name := range mcp.Keys() {
			server, _ := mcp.Table(name)
			out.mcp[name] = initNormalizeMCPServer(server)
		}
	}
	if bindings, ok := data.Array("bindings"); ok {
		for _, item := range bindings {
			if table, ok := item.(*toml.Table); ok {
				out.activeBinding += initBindingCount(table)
			}
		}
	} else if tables, ok := data.Tables("bindings"); ok {
		for _, table := range tables {
			out.activeBinding += initBindingCount(table)
		}
	}
	return out, nil
}

// initBindingCount is read_bindings' filter: a top-level [[bindings]] entry
// counts only when BOTH capability and recipe are strings.
func initBindingCount(table *toml.Table) int {
	_, okCap := table.String("capability")
	_, okRecipe := table.String("recipe")
	if okCap && okRecipe {
		return 1
	}
	return 0
}

// initStringList mirrors _normalize_string_list: only list-of-strings counts,
// and every item is stripped and dropped when empty.
func initStringList(t *toml.Table, key string) []string {
	v, ok := t.Get(key)
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// loadInitRecipe is _load_recipe: the internal-test guard first, then the
// catalogue resolution, recipe.toml validation and the init-workflow check.
func loadInitRecipe(recipeID string, opts InitOptions) (*schema.Recipe, string, error) {
	if strings.HasPrefix(recipeID, "test-") {
		return nil, "", fmt.Errorf("Recipe '%s' is an internal test fixture and is not part of the public catalog.", recipeID)
	}
	home := CatalogHome(opts.Home, opts.InstallRoot)
	catalog := CatalogDir(home)
	recipeDir := filepath.Join(catalog, recipeID)
	if filepath.IsAbs(recipeID) {
		recipeDir = recipeID
	}
	if info, err := os.Stat(recipeDir); err != nil || !info.IsDir() {
		return nil, "", fmt.Errorf("Recipe '%s' not found in catalog/recipes/", recipeID)
	}
	loaded, err := schema.LoadRecipeToml(filepath.Join(recipeDir, "recipe.toml"))
	if err != nil {
		return nil, "", err
	}
	if loaded.Init == nil {
		return nil, "", fmt.Errorf("Recipe '%s' has no init workflow", recipeID)
	}
	return loaded, recipeDir, nil
}

func initInstallState(recipeID string, recipes map[string]initRecipeEntry) string {
	entry, ok := recipes[recipeID]
	if !ok {
		return "available (not installed)"
	}
	if entry.enabled {
		return "installed"
	}
	return "disabled"
}

func initConfigLines(r *schema.Recipe, recipeID string, recipes map[string]initRecipeEntry) []string {
	entry := recipes[recipeID]
	cfg := entry.config
	var existing []string
	if cfg != nil {
		existing = cfg.Keys()
		sort.Strings(existing)
	}
	lines := []string{"## Config Guidance", ""}
	if len(existing) > 0 {
		lines = append(lines, "Existing config keys: "+strings.Join(existing, ", "))
	} else {
		lines = append(lines, "Existing config keys: (none)")
	}
	if _, installed := recipes[recipeID]; !installed {
		lines = append(lines, "Reviewable manifest addition:")
		lines = append(lines, fmt.Sprintf("```toml\n[recipes.%s]\nenabled = true\n```", recipeID))
	}

	fields := map[string]*schema.ConfigField{}
	if r.ConfigSchema != nil {
		fields = r.ConfigSchema.Fields
	}
	if len(fields) > 0 {
		lines = append(lines, "Schema-aligned config targets:")
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, key := range names {
			field := fields[key]
			present := false
			if cfg != nil {
				_, present = cfg.Get(key)
			}
			switch {
			case present:
				lines = append(lines, fmt.Sprintf("- Update existing key `%s` if needed; do not append a duplicate `%s` key.", key, key))
			case field.Required:
				lines = append(lines, fmt.Sprintf("- Add required `%s` under `[recipes.%s.config]`; sync still validates it later.", key, recipeID))
			case field.Default != nil:
				lines = append(lines, fmt.Sprintf("- `%s` is optional and defaults to `%s`.", key, initStr(field.Default)))
			default:
				lines = append(lines, fmt.Sprintf("- `%s` is optional.", key))
			}
		}
	} else {
		lines = append(lines, "Recipe declares no config schema.")
	}

	var unknown []string
	if cfg != nil {
		for _, key := range cfg.Keys() {
			if _, known := fields[key]; !known {
				unknown = append(unknown, key)
			}
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		lines = append(lines, "Unknown config keys: "+strings.Join(unknown, ", ")+"; sync still validates recipe config later.")
	}
	return append(lines, "")
}

func initMCPLines(r *schema.Recipe, manifestMCP map[string]any) []string {
	presets := map[string]any{}
	presetIDs := make([]string, 0, len(r.MCP))
	for _, m := range r.MCP {
		presets[m.ID] = initPlainValue(m.Config)
		presetIDs = append(presetIDs, m.ID)
	}
	needed := make([]string, 0, len(r.Init.NeedsMCP)+len(presetIDs))
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, r.Init.NeedsMCP...), presetIDs...) {
		if !seen[id] {
			seen[id] = true
			needed = append(needed, id)
		}
	}

	lines := []string{"## MCP Discovery", ""}
	lines = append(lines, "Reminder: project manifest values take precedence during sync-time MCP merge.")
	if len(needed) == 0 {
		lines = append(lines, "No MCP servers declared by this init workflow or recipe presets.")
		return append(lines, "")
	}
	for _, id := range needed {
		configuredValue, configured := manifestMCP[id]
		presetValue, preset := presets[id]
		status := "missing"
		if configured {
			status = "configured"
		}
		if preset {
			status += "; recipe preset available"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", id, status))
		if configured {
			lines = append(lines, "  project manifest:")
			lines = append(lines, initFormatMapping(initRedactValue(id, configuredValue).(map[string]any), "    ")...)
		}
		if preset {
			lines = append(lines, "  recipe preset:")
			lines = append(lines, initFormatMapping(initRedactValue(id, presetValue).(map[string]any), "    ")...)
		}
	}
	return append(lines, "")
}

func initTemplateLines(r *schema.Recipe, projectRoot string) []string {
	lines := []string{"## Template/Override Preview", ""}
	if len(r.Templates) == 0 {
		lines = append(lines, "No template/override targets declared.")
		return append(lines, "")
	}
	for _, tpl := range r.Templates {
		target := filepath.Join(projectRoot, tpl.Target)
		if filepath.IsAbs(tpl.Target) {
			target = tpl.Target
		}
		exists := false
		if _, err := os.Stat(target); err == nil {
			exists = true
		}
		state, action := "missing", "review create"
		if exists {
			state, action = "exists", "review update/skip/diff"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s; condition `%s`; action: %s.", tpl.Target, state, tpl.Condition, action))
	}
	return append(lines, "")
}

// initNormalizeMCPServer mirrors toml-read.py's _normalize_mcp_server. The
// result is a plain map because the brief only ever sorts and redacts it.
func initNormalizeMCPServer(raw *toml.Table) map[string]any {
	out := map[string]any{}
	if raw == nil {
		return out
	}
	if command, ok := raw.Get("command"); ok {
		switch command.(type) {
		case string, []any, []*toml.Table:
			out["command"] = initPlainValue(command)
		default:
			out["command"] = nil
		}
	} else {
		out["command"] = nil
	}

	if args, ok := raw.Get("args"); ok {
		switch args.(type) {
		case []any, []*toml.Table:
			out["args"] = initPlainValue(args)
		default:
			out["args"] = []any{}
		}
	} else {
		out["args"] = []any{}
	}

	env, hasEnv := raw.Get("env")
	if !hasEnv || !initMappingOrList(env) {
		if alt, ok := raw.Get("environment"); ok && initMappingOrList(alt) {
			env, hasEnv = alt, true
		} else {
			env, hasEnv = nil, false
		}
	}
	out["env"] = initNormalizeEnv(env, hasEnv)

	if timeout, ok := raw.Get("timeout"); ok {
		switch timeout.(type) {
		case int64, toml.BigInt:
			out["timeout"] = timeout
		default:
			out["timeout"] = nil
		}
	} else {
		out["timeout"] = nil
	}
	if enabled, ok := raw.Get("enabled"); ok {
		if b, isBool := enabled.(bool); isBool {
			out["enabled"] = b
		}
	}
	for _, key := range raw.Keys() {
		switch key {
		case "command", "args", "env", "environment", "timeout", "enabled":
			continue
		}
		v, _ := raw.Get(key)
		out[key] = initPlainValue(v)
	}
	return out
}

func initMappingOrList(v any) bool {
	switch v.(type) {
	case *toml.Table, []any, []*toml.Table:
		return true
	}
	return false
}

func initNormalizeEnv(v any, has bool) any {
	if !has {
		return map[string]any{}
	}
	switch x := v.(type) {
	case *toml.Table:
		return initPlainValue(x)
	case []any:
		out := map[string]any{}
		for _, item := range x {
			if s, ok := item.(string); ok {
				if name := strings.TrimSpace(s); name != "" {
					out[name] = "$" + name
				}
			}
		}
		return out
	}
	return map[string]any{}
}

// initPlainValue converts toml trees to plain nested maps/slices so redaction
// and formatting can recurse uniformly.
func initPlainValue(v any) any {
	switch x := v.(type) {
	case *toml.Table:
		if x == nil {
			return map[string]any{}
		}
		out := make(map[string]any, len(x.Keys()))
		for _, key := range x.Keys() {
			item, _ := x.Get(key)
			out[key] = initPlainValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, item := range x {
			out[key] = initPlainValue(item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = initPlainValue(item)
		}
		return out
	case []*toml.Table:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = initPlainValue(item)
		}
		return out
	default:
		return v
	}
}

func initRedactValue(key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = initRedactValue(k, item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = initRedactValue(key, item)
		}
		return out
	case string:
		if m := initEnvReferenceRe.FindStringSubmatch(strings.TrimSpace(x)); m != nil {
			return "${" + m[1] + "}"
		}
		if initSecretKeyRe.MatchString(key) {
			return "***"
		}
		return x
	default:
		return v
	}
}

func initFormatMapping(data map[string]any, indent string) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		value := data[key]
		if nested, ok := value.(map[string]any); ok {
			lines = append(lines, indent+key+":")
			lines = append(lines, initFormatMapping(nested, indent+"  ")...)
			continue
		}
		lines = append(lines, indent+key+": "+initStr(value))
	}
	return lines
}

// initStr is Python str() for the values the brief prints.
func initStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = initRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		return initRepr(x)
	default:
		return config.PyStr(v)
	}
}

// initRepr is Python repr() for the same values (strings quoted, containers
// recursive). Maps are rendered in sorted key order.
func initRepr(v any) string {
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(x, `\`, `\\`), "'", `\'`) + "'"
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = initRepr(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, initRepr(key)+": "+initRepr(x[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return initStr(v)
	}
}
