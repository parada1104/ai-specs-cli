// Package rulesaudit is the native port of `ai-specs rules-audit`, replacing
// lib/rules-audit.sh + lib/_internal/rules-inventory.py. It scans a project for
// legacy Cursor rules, AGENTS.md sections, the manifest, resolved skills, and
// the .atl registry, then emits the same JSON payload, byte for byte, as the
// legacy Python implementation.
//
// This package owns the scanner only; argument parsing, help/usage, and
// dispatcher routing live in internal/cli (routed in a later slice).
package rulesaudit

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/toml"
)

const (
	schemaVersion  = 1
	bodyExcerptLen = 400
)

// recipeCatalog mirrors RECIPE_CATALOG (emitted verbatim in sources).
var recipeCatalog = []string{
	"worktree-flow",
	"git-pr-flow",
	"session-context",
	"tdd-flow",
	"trello-mcp-workflow",
	"vault-canonical-store",
}

// recipeKeywordOrder reproduces RECIPE_KEYWORDS insertion order, which decides
// the order of candidate_recipes.
var recipeKeywordOrder = []string{
	"worktree-flow",
	"git-pr-flow",
	"tdd-flow",
	"trello-mcp-workflow",
	"vault-canonical-store",
	"session-context",
}

var recipeKeywords = map[string][]string{
	"worktree-flow":         {"worktree"},
	"git-pr-flow":           {"pull request", "git pr", "open pr"},
	"tdd-flow":              {"tdd", "test-driven", "red-green", "run tests first"},
	"trello-mcp-workflow":   {"trello", "trello board", "trello card"},
	"vault-canonical-store": {"vault", "obsidian", "canonical vault"},
	"session-context":       {"session context", "session bootstrap", "bootstrap session"},
}

// Classification buckets (CLASSIFICATION_BUCKETS) are documentation only: the
// scan output never names them, it only emits the chosen classification. The
// set is: keep_in_brief, enable_recipe, use_catalog_dep, create_local_skill,
// merge_into_skill, already_in_atl, deprecate_rule_file.

// lockfileHints mirrors LOCKFILE_HINTS insertion order (hint order and
// default_recipes depend on it).
var lockfileHints = []struct{ name, stack string }{
	{"package-lock.json", "node"},
	{"yarn.lock", "node"},
	{"pnpm-lock.yaml", "node"},
	{"pyproject.toml", "python"},
	{"requirements.txt", "python"},
	{"Pipfile", "python"},
	{"go.mod", "go"},
	{"Cargo.toml", "rust"},
}

// pySpace is Python's Unicode \s set for str patterns (ASCII whitespace plus
// \x1c-\x1f, \x85, the Zs category, and the line/paragraph separators).
const pySpace = `[\t\n\v\f\r \x1c-\x1f\x{85}\p{Zs}\x{2028}\x{2029}]`

// headingRE mirrors HEADING_RE: ^#{1,3}\s+(.+)$ with re.MULTILINE.
var headingRE = regexp.MustCompile(`(?m)^#{1,3}` + pySpace + `+(.+)$`)

// skillIDRE mirrors SKILL_ID_RE: `([a-z0-9][a-z0-9-]*)`.
var skillIDRE = regexp.MustCompile("`([a-z0-9][a-z0-9-]*)`")

// mdcFallbackRE mirrors _MDC_META_FALLBACK_RE.items() in insertion order.
var mdcFallbackRE = []struct {
	key string
	re  *regexp.Regexp
}{
	{"description", regexp.MustCompile(`(?im)^description:` + pySpace + `*(.+)$`)},
	{"globs", regexp.MustCompile(`(?im)^globs:` + pySpace + `*(\[[^\]]*\]|.+)$`)},
	{"alwaysApply", regexp.MustCompile(`(?im)^alwaysApply:` + pySpace + `*(.+)$`)},
	{"always_apply", regexp.MustCompile(`(?im)^always_apply:` + pySpace + `*(.+)$`)},
}

// Run scans projectRoot and writes the JSON payload to stdout. It mirrors the
// body of rules-inventory.main (the launcher's flag parsing is the caller's
// concern): non-directories exit 2, scan failures exit 1. home is the CLI home
// (AI_SPECS_HOME) used for recipe/dep/bundled skill resolution.
func Run(projectRoot, home string, stdout, stderr io.Writer) int {
	root := projectcache.ResolvePath(projectRoot)
	if !isDir(root) {
		fmt.Fprintf(stderr, "ERROR: not a directory: %s\n", root)
		return 2
	}

	payload, err := scan(root, home, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}

	fmt.Fprint(stdout, encodeIndent(payload))
	fmt.Fprint(stdout, "\n")
	return 0
}

type inventoryItem struct {
	path             string
	bodyExcerpt      string
	description      string
	globs            []string
	alwaysApply      bool
	heading          string
	candidateRecipes []string
	alreadyResolved  bool
	classification   string
}

// cursorrulesResult carries both the JSON value and the absent check that
// _detect_mode / the summary need.
type cursorrulesResult struct {
	absent bool
	items  []*obj
}

func (r cursorrulesResult) jsonValue() any {
	if r.absent {
		o := newObj()
		o.set("status", "absent")
		return o
	}
	return r.items
}

type atlResult struct {
	present  bool
	skillIDs []string
}

func (a atlResult) jsonValue() *obj {
	o := newObj()
	o.set("present", a.present)
	ids := a.skillIDs
	if ids == nil {
		ids = []string{}
	}
	o.set("skill_ids", ids)
	return o
}

// scan mirrors RulesInventory.scan.
func scan(root, home string, stderr io.Writer) (*obj, error) {
	resolvedMap := skills.CollectSkillsTo(root, home, stderr)
	resolvedIDs := make(map[string]bool, len(resolvedMap))
	ids := make([]string, 0, len(resolvedMap))
	for id := range resolvedMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	resolvedList := make([]*obj, 0, len(ids))
	for _, id := range ids {
		resolvedIDs[id] = true
		o := newObj()
		o.set("id", id)
		o.set("source", resolvedMap[id].Source)
		resolvedList = append(resolvedList, o)
	}

	atl, err := scanATL(root)
	if err != nil {
		return nil, err
	}
	atlIDs := make(map[string]bool, len(atl.skillIDs))
	for _, id := range atl.skillIDs {
		atlIDs[id] = true
	}

	cursorRules, err := scanCursorRules(root, resolvedIDs, atlIDs)
	if err != nil {
		return nil, err
	}
	cursorrules, err := scanCursorrules(root, resolvedIDs, atlIDs)
	if err != nil {
		return nil, err
	}
	agentsSections, err := scanAgentsMDSections(root, resolvedIDs, atlIDs)
	if err != nil {
		return nil, err
	}
	manifest, err := scanManifest(root)
	if err != nil {
		return nil, err
	}
	mode := detectMode(cursorRules, cursorrules, root)

	sources := newObj()
	sources.set("cursor_rules", itemDicts(cursorRules))
	sources.set("cursorrules", cursorrules.jsonValue())
	sources.set("agents_md_sections", itemDicts(agentsSections))
	sources.set("agents_md_present", isFile(filepath.Join(root, "AGENTS.md")))
	sources.set("manifest", manifest)
	sources.set("resolved_skills", resolvedList)
	sources.set("recipe_catalog", append([]string(nil), recipeCatalog...))
	sources.set("atl_registry", atl.jsonValue())

	payload := newObj()
	payload.set("schema_version", schemaVersion)
	payload.set("mode", mode)
	payload.set("target", root)
	payload.set("classification_is_suggestion", true)
	payload.set("sources", sources)

	summary := newObj()
	summary.set("cursor_rules", len(cursorRules))
	if cursorrules.absent {
		summary.set("cursorrules", 0)
	} else {
		summary.set("cursorrules", 1)
	}
	summary.set("agents_md_sections", len(agentsSections))
	payload.set("summary", summary)

	if mode == "B" {
		hints := stackHints(root)
		payload.set("stack_hints", hints)
		rec := newObj()
		rec.set("init", "ai-specs init")
		rec.set("default_recipes", defaultRecipesForStack(hints))
		rec.set("brief_hint", "Draft a runtime-brief [brief] section in AGENTS.md after init.")
		payload.set("recommendations", rec)
	}

	return payload, nil
}

func itemDicts(items []*inventoryItem) []*obj {
	out := make([]*obj, 0, len(items))
	for _, item := range items {
		out = append(out, itemDict(item))
	}
	return out
}

// itemDict mirrors RulesInventory._item_dict, preserving the conditional key
// order.
func itemDict(item *inventoryItem) *obj {
	o := newObj()
	o.set("path", item.path)
	o.set("body_excerpt", item.bodyExcerpt)
	recipes := item.candidateRecipes
	if recipes == nil {
		recipes = []string{}
	}
	o.set("candidate_recipes", recipes)
	o.set("already_resolved", item.alreadyResolved)
	o.set("classification", item.classification)
	if item.description != "" {
		o.set("description", item.description)
	}
	if len(item.globs) > 0 {
		o.set("globs", item.globs)
	}
	if item.heading != "" {
		o.set("heading", item.heading)
	}
	if strings.HasSuffix(item.path, ".mdc") {
		o.set("always_apply", item.alwaysApply)
	}
	return o
}

// detectMode mirrors RulesInventory._detect_mode.
func detectMode(cursorRules []*inventoryItem, cursorrules cursorrulesResult, root string) string {
	if len(cursorRules) > 0 || !cursorrules.absent {
		return "A"
	}
	if isFile(filepath.Join(root, "AGENTS.md")) {
		return "A"
	}
	return "B"
}

// stackHints mirrors RulesInventory._stack_hints.
func stackHints(root string) []string {
	var hints []string
	for _, hint := range lockfileHints {
		if isFile(filepath.Join(root, hint.name)) && !containsString(hints, hint.stack) {
			hints = append(hints, hint.stack)
		}
	}
	return hints
}

// defaultRecipesForStack mirrors RulesInventory._default_recipes_for_stack.
func defaultRecipesForStack(stackHints []string) []string {
	recipes := []string{"session-context"}
	if containsString(stackHints, "python") || containsString(stackHints, "node") {
		recipes = append(recipes, "worktree-flow", "tdd-flow")
	}
	if containsString(stackHints, "node") {
		recipes = append(recipes, "git-pr-flow")
	}
	seen := map[string]bool{}
	var ordered []string
	for _, recipe := range recipes {
		if containsString(recipeCatalog, recipe) && !seen[recipe] {
			seen[recipe] = true
			ordered = append(ordered, recipe)
		}
	}
	return ordered
}

// applyClassification mirrors RulesInventory._apply_classification.
func applyClassification(item *inventoryItem, resolvedIDs, atlIDs map[string]bool) {
	item.candidateRecipes = matchRecipes(item.description, item.heading, item.bodyExcerpt)
	already := false
	for _, recipe := range item.candidateRecipes {
		if resolvedIDs[strings.ReplaceAll(recipe, "-", "_")] || resolvedIDs[recipe] {
			already = true
			break
		}
	}
	if !already {
		for _, recipe := range item.candidateRecipes {
			if atlIDs[recipe] {
				already = true
				break
			}
		}
	}
	item.alreadyResolved = already
	item.classification = classify(item.alreadyResolved, item.heading, item.bodyExcerpt, item.candidateRecipes)
}

// matchRecipes mirrors RulesInventory._match_recipes.
func matchRecipes(parts ...string) []string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	haystack := strings.ToLower(strings.Join(kept, " "))
	var matches []string
	for _, id := range recipeKeywordOrder {
		for _, keyword := range recipeKeywords[id] {
			if keywordInHaystack(keyword, haystack) {
				matches = append(matches, id)
				break
			}
		}
	}
	return matches
}

// classify mirrors RulesInventory._classify.
func classify(alreadyResolved bool, heading, body string, candidateRecipes []string) string {
	if alreadyResolved {
		return "already_in_atl"
	}
	headingLower := strings.ToLower(heading)
	bodyLower := strings.ToLower(body)
	if headingLower != "" {
		for _, token := range []string{"workflow", "project", "conflict", "runtime"} {
			if keywordInHaystack(token, headingLower) {
				return "keep_in_brief"
			}
		}
	}
	if len(candidateRecipes) > 0 {
		return "enable_recipe"
	}
	if strings.Contains(bodyLower, "deprecate") || strings.Contains(bodyLower, "obsolete") {
		return "deprecate_rule_file"
	}
	if strings.Contains(bodyLower, "merge into") {
		return "merge_into_skill"
	}
	if strings.Contains(bodyLower, "catalog dep") || strings.Contains(bodyLower, "vendored skill") {
		return "use_catalog_dep"
	}
	return "create_local_skill"
}

// excerpt mirrors RulesInventory._excerpt; the 400-unit cap counts Unicode
// code points, not bytes.
func excerpt(text string) string {
	compact := strings.TrimSpace(text)
	runes := []rune(compact)
	if len(runes) <= bodyExcerptLen {
		return compact
	}
	return string(runes[:bodyExcerptLen])
}

// keywordInHaystack mirrors _keyword_in_haystack: multi-word keywords use a
// plain substring test, single words use Python's Unicode \b...\b boundary.
func keywordInHaystack(keyword, haystack string) bool {
	if strings.Contains(keyword, " ") {
		return strings.Contains(haystack, keyword)
	}
	from := 0
	for {
		j := strings.Index(haystack[from:], keyword)
		if j < 0 {
			return false
		}
		start := from + j
		end := start + len(keyword)
		beforeOK := start == 0
		if !beforeOK {
			r, _ := utf8.DecodeLastRuneInString(haystack[:start])
			beforeOK = !isPyWord(r)
		}
		afterOK := end == len(haystack)
		if !afterOK {
			r, _ := utf8.DecodeRuneInString(haystack[end:])
			afterOK = !isPyWord(r)
		}
		if beforeOK && afterOK {
			return true
		}
		from = start + 1
	}
}

// isPyWord mirrors Python's \w for str patterns (letter, number, or underscore).
func isPyWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// --- source scanners --------------------------------------------------------

// scanCursorRules mirrors RulesInventory._scan_cursor_rules.
func scanCursorRules(root string, resolvedIDs, atlIDs map[string]bool) ([]*inventoryItem, error) {
	rulesDir := filepath.Join(root, ".cursor", "rules")
	if !isDir(rulesDir) {
		return nil, nil
	}
	paths, err := rglobMdc(rulesDir)
	if err != nil {
		return nil, err
	}
	var items []*inventoryItem
	for _, path := range paths {
		text, err := readText(path)
		if err != nil {
			return nil, err
		}
		frontmatter, body := skills.SplitFrontmatter(text)
		meta := parseMDCMeta(frontmatter)
		alwaysVal := meta["always_apply"]
		if v, ok := meta["alwaysApply"]; ok {
			alwaysVal = v
		}
		item := &inventoryItem{
			path:        relPath(root, path),
			description: pyStrValue(meta["description"]),
			globs:       coerceGlobs(meta["globs"]),
			alwaysApply: coerceBool(alwaysVal),
			bodyExcerpt: excerpt(body),
		}
		applyClassification(item, resolvedIDs, atlIDs)
		items = append(items, item)
	}
	return items, nil
}

// scanCursorrules mirrors RulesInventory._scan_cursorrules.
func scanCursorrules(root string, resolvedIDs, atlIDs map[string]bool) (cursorrulesResult, error) {
	path := filepath.Join(root, ".cursorrules")
	if !isFile(path) {
		return cursorrulesResult{absent: true}, nil
	}
	body, err := readText(path)
	if err != nil {
		return cursorrulesResult{}, err
	}
	item := &inventoryItem{path: ".cursorrules", bodyExcerpt: excerpt(body)}
	applyClassification(item, resolvedIDs, atlIDs)
	return cursorrulesResult{items: []*obj{itemDict(item)}}, nil
}

// scanAgentsMDSections mirrors RulesInventory._scan_agents_md_sections.
func scanAgentsMDSections(root string, resolvedIDs, atlIDs map[string]bool) ([]*inventoryItem, error) {
	path := filepath.Join(root, "AGENTS.md")
	if !isFile(path) {
		return nil, nil
	}
	text, err := readText(path)
	if err != nil {
		return nil, err
	}

	textNoFences := blankFences(text)
	matches := headingRE.FindAllStringSubmatchIndex(textNoFences, -1)
	if len(matches) == 0 {
		// N1: monolithic AGENTS.md with no headings — a single section.
		body := strings.TrimSpace(text)
		if body == "" {
			return nil, nil
		}
		item := &inventoryItem{path: "AGENTS.md", bodyExcerpt: excerpt(body)}
		applyClassification(item, resolvedIDs, atlIDs)
		return []*inventoryItem{item}, nil
	}

	var items []*inventoryItem
	preamble := strings.TrimSpace(text[:matches[0][0]])
	if preamble != "" {
		item := &inventoryItem{path: "AGENTS.md", bodyExcerpt: excerpt(preamble)}
		applyClassification(item, resolvedIDs, atlIDs)
		items = append(items, item)
	}
	for index, match := range matches {
		heading := strings.TrimSpace(textNoFences[match[2]:match[3]])
		start := match[1]
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		body := strings.TrimSpace(text[start:end])
		item := &inventoryItem{path: "AGENTS.md", heading: heading, bodyExcerpt: excerpt(body)}
		applyClassification(item, resolvedIDs, atlIDs)
		items = append(items, item)
	}
	return items, nil
}

// scanManifest mirrors RulesInventory._scan_manifest.
func scanManifest(root string) (*obj, error) {
	tomlPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if !isFile(tomlPath) {
		o := newObj()
		o.set("present", false)
		return o, nil
	}
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, err
	}
	table, err := toml.Parse(data)
	if err != nil {
		o := newObj()
		o.set("present", true)
		o.set("parse_error", true)
		return o, nil
	}

	var enabled []any
	if agents, ok := table.Table("agents"); ok {
		if arr, ok := agents.Array("enabled"); ok {
			enabled = append(enabled, arr...)
		}
	}

	var recipes []any
	recipesRaw, _ := table.Get("recipes")
	switch raw := recipesRaw.(type) {
	case *toml.Table:
		for _, id := range raw.Keys() {
			cfg, _ := raw.Get(id)
			if ct, ok := cfg.(*toml.Table); ok {
				v, _ := ct.Get("enabled")
				if coerceBool(v) {
					recipes = append(recipes, id)
				}
			}
		}
	case []*toml.Table:
		for _, block := range raw {
			if id, ok := recipeBlockID(block); ok {
				recipes = append(recipes, id)
			}
		}
	case []any:
		for _, block := range raw {
			bt, ok := block.(*toml.Table)
			if !ok {
				continue
			}
			if id, ok := recipeBlockID(bt); ok {
				recipes = append(recipes, id)
			}
		}
	}

	hasRuntimeBrief := false
	agentsMD := filepath.Join(root, "AGENTS.md")
	if isFile(agentsMD) {
		content, err := readText(agentsMD)
		if err != nil {
			return nil, err
		}
		lower := strings.ToLower(content)
		hasRuntimeBrief = strings.Contains(lower, "runtime brief") || strings.Contains(lower, "director de orquesta")
	}

	o := newObj()
	o.set("present", true)
	if enabled == nil {
		enabled = []any{}
	}
	o.set("enabled_agents", enabled)
	if recipes == nil {
		recipes = []any{}
	}
	o.set("recipes", recipes)
	o.set("has_runtime_brief", hasRuntimeBrief)
	return o, nil
}

// recipeBlockID mirrors the [[recipes]] branch: a block contributes its id
// when the id is truthy and enabled is absent or truthy.
func recipeBlockID(block *toml.Table) (string, bool) {
	idv, ok := block.Get("id")
	if !ok {
		return "", false
	}
	id, ok := idv.(string)
	if !ok || id == "" {
		return "", false
	}
	enabledVal := any(true)
	if v, ok := block.Get("enabled"); ok {
		enabledVal = v
	}
	return id, coerceBool(enabledVal)
}

// scanATL mirrors RulesInventory._scan_atl_registry.
func scanATL(root string) (atlResult, error) {
	registryPath := filepath.Join(root, ".atl", "skill-registry.md")
	if !isFile(registryPath) {
		return atlResult{present: false, skillIDs: []string{}}, nil
	}
	text, err := readText(registryPath)
	if err != nil {
		return atlResult{}, err
	}
	seen := map[string]bool{}
	for _, m := range skillIDRE.FindAllStringSubmatch(text, -1) {
		seen[m[1]] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return atlResult{present: true, skillIDs: ids}, nil
}

// --- .mdc metadata ----------------------------------------------------------

// parseMDCMeta mirrors _parse_mdc_meta: try the full YAML parser first, then
// fall back to a tolerant line/regex reader.
func parseMDCMeta(frontmatter string) map[string]any {
	if frontmatter == "" {
		return map[string]any{}
	}
	if parsed, err := skills.ParseFrontmatter(frontmatter); err == nil {
		return parsed
	}

	meta := map[string]any{}
	for _, line := range skills.SplitLines(frontmatter) {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") {
			continue
		}
		if !strings.Contains(stripped, ":") {
			continue
		}
		key, rest, _ := strings.Cut(stripped, ":")
		key = strings.TrimSpace(key)
		rest = strings.TrimSpace(rest)
		if key != "description" && key != "globs" && key != "alwaysApply" && key != "always_apply" {
			continue
		}
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			meta[key] = skills.SplitInlineList(rest)
		} else {
			meta[key] = skills.StripQuotes(rest)
		}
	}

	for _, fb := range mdcFallbackRE {
		if _, ok := meta[fb.key]; ok {
			continue
		}
		match := fb.re.FindStringSubmatch(frontmatter)
		if match == nil {
			continue
		}
		value := strings.TrimSpace(match[1])
		if fb.key == "globs" && strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			meta[fb.key] = skills.SplitInlineList(value)
		} else {
			meta[fb.key] = skills.StripQuotes(value)
		}
	}
	return meta
}

// coerceBool mirrors _coerce_bool.
func coerceBool(value any) bool {
	switch x := value.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes":
			return true
		}
		return false
	case []string:
		return len(x) > 0
	case []any:
		return len(x) > 0
	case *toml.Table:
		return x != nil && len(x.Keys()) > 0
	default:
		return true
	}
}

// coerceGlobs mirrors the str/list/else coercion in _scan_cursor_rules.
func coerceGlobs(value any) []string {
	switch x := value.(type) {
	case string:
		return []string{x}
	case []string:
		return append([]string(nil), x...)
	default:
		return nil
	}
}

// pyStrValue mirrors str(meta.get("description", "")) for the value shapes
// parse_frontmatter can produce.
func pyStrValue(value any) string {
	switch x := value.(type) {
	case nil:
		return ""
	case string:
		return x
	case []string:
		parts := make([]string, len(x))
		for i, s := range x {
			parts[i] = pyReprStr(s)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", x)
	}
}

// pyReprStr renders a string the way Python repr() would (single quotes unless
// the value contains one and no double quote).
func pyReprStr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for _, r := range s {
		if rune(r) == rune(quote) || r == '\\' {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	sb.WriteByte(quote)
	return sb.String()
}

// --- AGENTS.md fence blanking ----------------------------------------------

// blankFences mirrors the FENCE_RE.sub + unterminated-fence pass in
// _scan_agents_md_sections. It returns a copy of text with fenced regions
// blanked to spaces (newlines preserved), so heading offsets still align with
// the original text.
func blankFences(text string) string {
	b := []byte(text)
	pos := 0
	for pos < len(text) {
		start, fenceChar, fenceLen, bodyStart, ok := findOpenFence(text, pos)
		if !ok {
			break
		}
		closeEnd, ok := findCloseFence(text, bodyStart, fenceChar, fenceLen)
		if !ok {
			pos = bodyStart
			continue
		}
		blankRange(b, start, closeEnd)
		pos = closeEnd
	}

	noFences := string(b)
	for _, m := range openFenceRE.FindAllStringIndex(noFences, -1) {
		if noFences[m[0]] == '`' || noFences[m[0]] == '~' {
			noFences = noFences[:m[0]] + blankNonNewlines(noFences[m[0]:])
			break
		}
	}
	return noFences
}

var openFenceRE = regexp.MustCompile(`(?m)^(?:` + "`{3,}" + `|~{3,})[^\n]*\n`)

// findOpenFence returns the leftmost opening fence at or after pos: a line
// starting with a run of 3+ backticks or tildes and ending in a newline.
func findOpenFence(text string, pos int) (start int, fenceChar byte, fenceLen, bodyStart int, ok bool) {
	for p := pos; p < len(text); p++ {
		if p != 0 && text[p-1] != '\n' {
			continue
		}
		c := text[p]
		if c != '`' && c != '~' {
			continue
		}
		run := p
		for run < len(text) && text[run] == c {
			run++
		}
		if run-p < 3 {
			continue
		}
		nl := strings.IndexByte(text[run:], '\n')
		if nl < 0 {
			continue
		}
		return p, c, run - p, run + nl + 1, true
	}
	return 0, 0, 0, 0, false
}

// FROZEN PARITY (oracle-wins): a closing fence line ending in \r (CRLF) or a
// closing run longer than the opening run is rejected here, exactly like the
// frozen oracle regex `^(?P=fence)[ \t]*$` in
// lib/_internal/rules-inventory.py:417-420 ( \r is neither space nor tab, and
// the backreference pins the exact run length). Both implementations then
// treat the fence as unterminated and blank to EOF. Pinned by
// TestBlankFencesFrozenFenceParity; revisit only in the post-cutover parity
// amendment (Trello card [Go 08][frozen quirk]).
// findCloseFence returns the end offset (index of the terminating newline or
// len(text)) of the first closing fence line for the given run.
func findCloseFence(text string, bodyStart int, fenceChar byte, fenceLen int) (int, bool) {
	for q := bodyStart; q <= len(text); {
		if q != 0 && text[q-1] != '\n' {
			q++
			continue
		}
		if q+fenceLen <= len(text) && onlyByte(text[q:q+fenceLen], fenceChar) {
			rest := q + fenceLen
			for rest < len(text) && (text[rest] == ' ' || text[rest] == '\t') {
				rest++
			}
			if rest == len(text) {
				return rest, true
			}
			if text[rest] == '\n' {
				return rest, true
			}
		}
		nl := strings.IndexByte(text[q:], '\n')
		if nl < 0 {
			return 0, false
		}
		q += nl + 1
	}
	return 0, false
}

func onlyByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// blankRange blanks every non-newline byte in b[start:end).
func blankRange(b []byte, start, end int) {
	if end > len(b) {
		end = len(b)
	}
	for i := start; i < end; i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

// blankNonNewlines replaces every non-newline byte with a space.
func blankNonNewlines(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// --- filesystem helpers -----------------------------------------------------

// rglobMdc mirrors sorted(Path(dir).rglob("*.mdc")): every entry (file or
// directory) whose name ends in .mdc, recursively, sorted by path string.
func rglobMdc(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(d.Name(), ".mdc") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// relPath mirrors str(path.relative_to(root)).
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// readText mirrors Path.read_text(encoding="utf-8", errors="replace").
func readText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ToValidUTF8(string(data), "\uFFFD"), nil
}

// isFile mirrors Path.is_file(): a regular file, following symlinks.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// isDir mirrors Path.is_dir().
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
