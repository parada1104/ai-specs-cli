package sync

// Go port of the PURE rendering core of lib/_internal/agents-render.py
// (card [Go 07.S3a]). Scope: everything from the recipe-fragment helpers
// (substitute_config, collect_recipe_brief_fragments, _validate_brief_modes,
// _redact_env_value) through every _section_* renderer, _render_lines, and the
// byte assembly render() performs before its governance decision
// ("\n".join(_render_lines(...)).encode()).
//
// NOT ported here (slice S3b): _load_util/_load_lock, classify_brief,
// brief_ownership_state/brief_effective_state, _brief_decision,
// _brief_is_our_output, the lock interplay, brief-render-policy.py, and the
// native sync.go/sync-agent wiring. The unknown-VCS stdin warning in
// _section_runtime_flow is a writer side effect deferred to that wiring; the
// returned bytes are unaffected.
//
// The manifest is the repo's parsed-manifest type (*toml.Table), not a plain
// map: TOML document order is observable output here ([mcp.*] server order and
// the order of [brief].mcp_descriptions), and a Go map cannot preserve it.
// internal/config and internal/target already carry parsed manifests as
// *toml.Table, so this also matches the established convention.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/target"
	"ai-specs.dev/ai-specs/internal/toml"
)

// _VALID_MODES of agents-render.py.
const (
	agentsBriefModeAppend  = "append"
	agentsBriefModeReplace = "replace"
)

// HARNESS_CLI_LITERACY_POINTER — the always-on Useful Commands pointer.
const harnessCLILiteracyPointer = "For ai-specs harness operations (init, sync, recipes, skills/deps, doctor), " +
	"load the `harness-lifecycle`, `harness-recipes`, or `harness-skills-deps` skills."

// vcsLabel is one _VCS_RECIPE_LABELS entry: (display name, CLI slug).
type vcsLabel struct {
	name string
	cli  string
}

// _VCS_RECIPE_LABELS — bound vcs-pr-flow recipe id → (display name, CLI slug).
var vcsRecipeLabels = map[string]vcsLabel{
	"git-pr-flow":       {name: "GitHub", cli: "gh"},
	"gitlab-mr-flow":    {name: "GitLab", cli: "glab"},
	"bitbucket-pr-flow": {name: "Bitbucket", cli: "bb"},
}

// _CONFIG_PLACEHOLDER_RE.
var agentsConfigPlaceholderRe = regexp.MustCompile(`\{\{|\}\}|\{config\.([A-Za-z0-9_]+)\}`)

// ---------------------------------------------------------------------------
// Recipe brief fragment helpers
// ---------------------------------------------------------------------------

// substituteConfig is substitute_config(): {config.KEY} substitution with
// {{ }} escapes, unknown keys re-emitted verbatim, and bare {KEY} untouched.
func substituteConfig(text string, cfgNS map[string]any) string {
	return agentsConfigPlaceholderRe.ReplaceAllStringFunc(text, func(token string) string {
		switch token {
		case "{{":
			return "{"
		case "}}":
			return "}"
		}
		m := agentsConfigPlaceholderRe.FindStringSubmatch(token)
		if m == nil {
			return token
		}
		if v, ok := cfgNS["config."+m[1]]; ok {
			return pyStr(v)
		}
		return token
	})
}

// recipeFragment is one collected {"key": ..., "text": ...} entry.
type recipeFragment struct {
	key  *string
	text string
}

// collectRecipeBriefFragments is collect_recipe_brief_fragments(): iterate
// resolved["enabled"] in order, substitute each fragment with its recipe's own
// config namespace, and deduplicate (key first-wins, then exact text).
func collectRecipeBriefFragments(resolved map[string]any, section string, recipeIDs map[string]bool) []recipeFragment {
	var out []recipeFragment
	seenKeys := map[string]bool{}
	seenText := map[string]bool{}
	recipes := resolvedMap(resolved, "recipes")

	for _, ridVal := range resolvedList(resolved, "enabled") {
		rid, ok := ridVal.(string)
		if !ok {
			continue
		}
		if recipeIDs != nil && !recipeIDs[rid] {
			continue
		}
		rcfg := mapValue(recipes, rid)
		cfgNS := map[string]any{}
		for k, v := range rcfg {
			if k != "brief_fragments" {
				cfgNS["config."+k] = v
			}
		}
		for _, fragVal := range listValue(mapValue(rcfg, "brief_fragments"), section) {
			fm, ok := fragVal.(map[string]any)
			if !ok {
				continue
			}
			key, hasKey := "", false
			if kv, present := fm["key"]; present && kv != nil {
				if s, isStr := kv.(string); isStr {
					key, hasKey = s, true
				}
			}
			raw, _ := fm["text"].(string)
			if hasKey && seenKeys[key] {
				continue
			}
			text := substituteConfig(raw, cfgNS)
			if seenText[text] {
				continue
			}
			if hasKey {
				seenKeys[key] = true
			}
			seenText[text] = true
			frag := recipeFragment{text: text}
			if hasKey {
				k := key
				frag.key = &k
			}
			out = append(out, frag)
		}
	}
	return out
}

// ValidateBriefModes is _validate_brief_modes(): every <section>_mode key must
// be "append" or "replace". The message is byte-identical to the Python
// ValueError so the differential can pin it.
func ValidateBriefModes(brief *toml.Table) error {
	if brief == nil {
		return nil
	}
	for _, key := range brief.Keys() {
		if !strings.HasSuffix(key, "_mode") {
			continue
		}
		val, _ := brief.Get(key)
		if s, ok := val.(string); ok && (s == agentsBriefModeAppend || s == agentsBriefModeReplace) {
			continue
		}
		return fmt.Errorf("[brief].%s: invalid mode %s; valid values are ['append', 'replace']", key, pyRepr(val))
	}
	return nil
}

// redactEnvValue is _redact_env_value(): $VAR / ${VAR} → ${VAR}, anything else
// → ***REDACTED***.
func redactEnvValue(value string) string {
	stripped := pyStrip(value)
	if strings.HasPrefix(stripped, "$") {
		return "${" + strings.Trim(stripped[1:], "{}") + "}"
	}
	return "***REDACTED***"
}

// ---------------------------------------------------------------------------
// Per-section helpers
// ---------------------------------------------------------------------------

// sectionIntro is _section_intro(): a "> " blockquote per intro line.
func sectionIntro(brief *toml.Table) []string {
	introVal := tableGetAny(brief, "intro")
	if !pyTruthy(introVal) {
		return nil
	}
	intro := pyStr(introVal)
	if pyStrip(intro) == "" {
		return nil
	}
	var lines []string
	for _, raw := range pySplitLines(intro) {
		if pyStrip(raw) == "" {
			lines = append(lines, ">")
		} else {
			lines = append(lines, "> "+raw)
		}
	}
	return append(lines, "")
}

// sectionProject is _section_project(): name, purpose, runtimes, integration
// branch, repo topology and vault scope.
func sectionProject(manifest *toml.Table, resolved map[string]any) []string {
	project := manifestTable(manifest, "project")
	agents := manifestTable(manifest, "agents")
	brief := manifestTable(manifest, "brief")
	recipes := resolvedMap(resolved, "recipes")
	bindings := resolvedMap(resolved, "bindings")

	lines := []string{"## Project", ""}

	if name := tableGetAny(project, "name"); pyTruthy(name) {
		lines = append(lines, "- **Project**: `"+pyStr(name)+"`")
	}
	if purpose := tableGetAny(brief, "purpose"); pyTruthy(purpose) {
		lines = append(lines, "- **Purpose**: "+pyStr(purpose))
	}
	if runtimes := tableList(agents, "enabled"); pyTruthy(runtimes) {
		parts := make([]string, 0, len(runtimes))
		for _, r := range runtimes {
			parts = append(parts, "`"+pyStr(r)+"`")
		}
		lines = append(lines, "- **Enabled runtimes**: "+strings.Join(parts, ", "))
	}

	worktreeRaw := mapGetAny(bindings, "worktree-isolation")
	worktreeRecipeID, _ := worktreeRaw.(string)
	var integrationBranch any
	if pyTruthy(worktreeRaw) {
		integrationBranch = cfgField(recipes, worktreeRecipeID, "integration_branch")
	}
	if !pyTruthy(integrationBranch) {
		integrationBranch = cfgField(recipes, "worktree-flow", "integration_branch")
	}
	if !pyTruthy(integrationBranch) {
		integrationBranch = cfgField(recipes, "git-pr-flow", "base_branch")
	}
	if pyTruthy(integrationBranch) {
		lines = append(lines, "- **Integration branch**: `"+pyStr(integrationBranch)+"`")
	}

	enabledRecipes := resolvedList(resolved, "enabled")
	wfEnabled := listContains(enabledRecipes, "worktree-flow") ||
		(pyTruthy(worktreeRaw) && listContains(enabledRecipes, worktreeRecipeID))
	projectRoot, err := os.Getwd()
	if err != nil {
		projectRoot = "."
	}
	if pr, ok := resolved["project_root"].(string); ok && pr != "" {
		projectRoot = pr
	}
	topo := target.ProjectRepoTopology(projectRoot, manifest)
	if topo.Source != "default" || wfEnabled {
		lines = append(lines, fmt.Sprintf("- **Repo topology**: `%s` (via %s)", topo.Resolved, topo.Via))
	}

	if canonicalRaw := mapGetAny(bindings, "canonical-store"); pyTruthy(canonicalRaw) {
		canonicalID, _ := canonicalRaw.(string)
		if vaultScope := cfgField(recipes, canonicalID, "vault_scope"); pyTruthy(vaultScope) {
			lines = append(lines, "- **Vault scope**: `"+pyStr(vaultScope)+"`")
		}
	}

	return append(lines, "")
}

// orderedDescs is a tiny insertion-ordered string map, mirroring the Python
// dict that _render_lines builds for effective mcp_descriptions (recipe
// fragments first, then [brief].mcp_descriptions overriding in place).
type orderedDescs struct {
	keys []string
	vals map[string]string
}

func newOrderedDescs() *orderedDescs {
	return &orderedDescs{vals: map[string]string{}}
}

func (o *orderedDescs) set(k, v string) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedDescs) get(k string) string { return o.vals[k] }

// effectiveMCPDescriptions reproduces the eff_mcp build in _render_lines.
func effectiveMCPDescriptions(resolved map[string]any, brief *toml.Table) *orderedDescs {
	eff := newOrderedDescs()
	for _, frag := range collectRecipeBriefFragments(resolved, "mcp_descriptions", nil) {
		if frag.key != nil {
			eff.set(*frag.key, frag.text)
		}
	}
	if md := manifestTable(brief, "mcp_descriptions"); md != nil {
		for _, k := range md.Keys() {
			v, _ := md.Get(k)
			eff.set(k, pyStr(v))
		}
	}
	return eff
}

// sectionMCP is _section_mcp(): table of servers plus description-only global
// entries and the secrets rule.
func sectionMCP(manifest *toml.Table, mcpDescriptions *orderedDescs) []string {
	mcp := manifestTable(manifest, "mcp")
	mcpEmpty := mcp == nil || len(mcp.Keys()) == 0
	if mcpEmpty && len(mcpDescriptions.keys) == 0 {
		return nil
	}

	lines := []string{"## Runtime MCPs", ""}
	if mcp != nil {
		for _, serverName := range mcp.Keys() {
			cfgVal, _ := mcp.Get(serverName)
			cfg, _ := cfgVal.(*toml.Table)
			lines = append(lines, "**"+serverName+"**")
			if cmd, ok := tableGet(cfg, "command"); ok {
				lines = append(lines, "- command: "+pyStr(cmd))
			}
			if args := tableList(cfg, "args"); pyTruthy(args) {
				parts := make([]string, 0, len(args))
				for _, a := range args {
					parts = append(parts, pyStr(a))
				}
				lines = append(lines, "- args: "+strings.Join(parts, " "))
			}
			env := tableGetAny(cfg, "env")
			if !pyTruthy(env) {
				env = tableGetAny(cfg, "environment")
			}
			if !pyTruthy(env) {
				env = nil
			}
			if pyTruthy(env) {
				lines = append(lines, "- env:")
				switch e := env.(type) {
				case []any:
					for _, v := range e {
						lines = append(lines, "  - "+pyStr(v)+": ${"+pyStr(v)+"}")
					}
				case *toml.Table:
					for _, k := range e.Keys() {
						v, _ := e.Get(k)
						lines = append(lines, "  - "+k+": "+redactEnvValue(pyStr(v)))
					}
				}
			}
			if desc := mcpDescriptions.get(serverName); pyTruthy(desc) {
				lines = append(lines, "- description: "+desc)
			}
			lines = append(lines, "")
		}
	}

	for _, serverName := range mcpDescriptions.keys {
		if mcp != nil {
			if _, ok := mcp.Get(serverName); ok {
				continue
			}
		}
		desc := mcpDescriptions.get(serverName)
		if !pyTruthy(desc) {
			continue
		}
		lines = append(lines, "**"+serverName+"** *(global)*")
		lines = append(lines, "- description: "+desc)
		lines = append(lines, "")
	}

	lines = append(lines, "Never expose env-backed secrets from MCP config in generated docs or comments.")
	return append(lines, "")
}

// sectionRuntimeFlow is _section_runtime_flow(): recipe + manifest bullets plus
// the VCS provider bullet.
func sectionRuntimeFlow(brief *toml.Table, resolved map[string]any) []string {
	mode := tableStringDefault(brief, "runtime_flow_mode", agentsBriefModeAppend)
	var recipeItems []recipeFragment
	if mode != agentsBriefModeReplace {
		recipeItems = collectRecipeBriefFragments(resolved, "runtime_flow", nil)
	}
	manifestItems := tableList(brief, "runtime_flow")
	recipes := resolvedMap(resolved, "recipes")
	bindings := resolvedMap(resolved, "bindings")

	vcsRaw := mapGetAny(bindings, "vcs-pr-flow")
	vcsRecipeID, _ := vcsRaw.(string)
	var baseBranch any
	var label *vcsLabel
	if pyTruthy(vcsRaw) {
		baseBranch = cfgField(recipes, vcsRecipeID, "base_branch")
		if l, ok := vcsRecipeLabels[vcsRecipeID]; ok {
			label = &l
		} else {
			// The unknown-id path also prints a warning to stderr in Python;
			// the pure byte core has no writer, so wiring that warning belongs
			// to S3b. Rendering falls back to the generic label here.
			label = &vcsLabel{name: "VCS PR (custom)", cli: "VCS PR (custom)"}
		}
	}

	bullets := make([]string, 0, len(recipeItems))
	for _, f := range recipeItems {
		bullets = append(bullets, f.text)
	}
	for _, m := range manifestItems {
		if pyTruthy(m) {
			if s := pyStr(m); !stringInSlice(bullets, s) {
				bullets = append(bullets, s)
			}
		}
	}

	if len(bullets) == 0 && label == nil && !pyTruthy(baseBranch) {
		return nil
	}

	lines := []string{"## Runtime Flow", ""}
	for _, item := range bullets {
		lines = append(lines, "- "+item)
	}
	if label != nil {
		var note string
		if label.cli == label.name {
			note = "VCS/PR provider: " + label.name
		} else {
			note = "VCS/PR provider: " + label.name + " (`" + label.cli + "` CLI)"
		}
		if pyTruthy(baseBranch) {
			note += "; base branch: `" + pyStr(baseBranch) + "`"
		}
		lines = append(lines, "- "+note)
	}
	return append(lines, "")
}

// sectionTrello is _section_trello().
func sectionTrello(resolved map[string]any) []string {
	recipes := resolvedMap(resolved, "recipes")
	bindings := resolvedMap(resolved, "bindings")

	trackerRaw := mapGetAny(bindings, "tracker")
	if !pyTruthy(trackerRaw) {
		return nil
	}
	trackerRecipeID, _ := trackerRaw.(string)
	boardID := cfgField(recipes, trackerRecipeID, "board_id")
	if !pyTruthy(boardID) {
		return nil
	}
	return []string{"## Trello Tracking", "", "- **Board**: `" + pyStr(boardID) + "`", ""}
}

// mergeSectionBullets mirrors the shared "recipe items then manifest items
// (exact-string dedup)" loop used by the bullet sections.
func mergeSectionBullets(recipeItems []recipeFragment, manifestItems []any) []string {
	bullets := make([]string, 0, len(recipeItems))
	for _, f := range recipeItems {
		bullets = append(bullets, f.text)
	}
	for _, m := range manifestItems {
		if pyTruthy(m) {
			if s := pyStr(m); !stringInSlice(bullets, s) {
				bullets = append(bullets, s)
			}
		}
	}
	return bullets
}

// bulletLines renders a "## title" section from a bullet list, or nil when empty.
func bulletLines(title string, bullets []string) []string {
	if len(bullets) == 0 {
		return nil
	}
	lines := []string{"## " + title, ""}
	for _, b := range bullets {
		lines = append(lines, "- "+b)
	}
	return append(lines, "")
}

// sectionContextSources is _section_context_sources().
func sectionContextSources(brief *toml.Table, resolved map[string]any) []string {
	mode := tableStringDefault(brief, "context_sources_mode", agentsBriefModeAppend)
	var recipeItems []recipeFragment
	if mode != agentsBriefModeReplace {
		recipeItems = collectRecipeBriefFragments(resolved, "context_sources", nil)
	}
	return bulletLines("Context Sources", mergeSectionBullets(recipeItems, tableList(brief, "context_sources")))
}

// sectionConflictPolicy is _section_conflict_policy().
func sectionConflictPolicy(brief *toml.Table, resolved map[string]any) []string {
	mode := tableStringDefault(brief, "conflict_policy_mode", agentsBriefModeAppend)
	var recipeItems []recipeFragment
	if mode != agentsBriefModeReplace {
		recipeItems = collectRecipeBriefFragments(resolved, "conflict_policy", nil)
	}
	return bulletLines("Conflict Policy", mergeSectionBullets(recipeItems, tableList(brief, "conflict_policy")))
}

// sectionWorkflowRules is _section_workflow_rules(), including the VCS-sibling
// fragment filter.
func sectionWorkflowRules(brief *toml.Table, resolved map[string]any) []string {
	mode := tableStringDefault(brief, "workflow_rules_mode", agentsBriefModeAppend)
	bindings := resolvedMap(resolved, "bindings")
	boundVCSRaw := mapGetAny(bindings, "vcs-pr-flow")
	boundVCS, _ := boundVCSRaw.(string)

	enabled := resolvedEnabledSet(resolved)
	vcsSiblings := map[string]bool{
		"git-pr-flow":       true,
		"gitlab-mr-flow":    true,
		"bitbucket-pr-flow": true,
	}
	var recipeIDs map[string]bool
	if pyTruthy(boundVCSRaw) {
		excluded := map[string]bool{}
		for id := range vcsSiblings {
			if id != boundVCS {
				excluded[id] = true
			}
		}
		recipeIDs = setDifference(enabled, excluded)
	} else {
		recipeIDs = setDifference(enabled, vcsSiblings)
	}

	var recipeItems []recipeFragment
	if mode != agentsBriefModeReplace {
		recipeItems = collectRecipeBriefFragments(resolved, "workflow_rules", recipeIDs)
	}
	return bulletLines("Workflow Rules", mergeSectionBullets(recipeItems, tableList(brief, "workflow_rules")))
}

// sectionUsefulCommands is _section_useful_commands().
func sectionUsefulCommands(brief *toml.Table, resolved map[string]any) []string {
	recipes := resolvedMap(resolved, "recipes")
	bindings := resolvedMap(resolved, "bindings")

	var testCommand any
	if runnerRaw := mapGetAny(bindings, "test-runner"); pyTruthy(runnerRaw) {
		runnerID, _ := runnerRaw.(string)
		testCommand = cfgField(recipes, runnerID, "test_command")
	}
	if !pyTruthy(testCommand) {
		testCommand = cfgField(recipes, "tdd-flow", "test_command")
	}

	mode := tableStringDefault(brief, "useful_commands_mode", agentsBriefModeAppend)
	var recipeItems []recipeFragment
	if mode != agentsBriefModeReplace {
		recipeItems = collectRecipeBriefFragments(resolved, "useful_commands", nil)
	}

	recipeBullets := make([]string, 0, len(recipeItems))
	for _, f := range recipeItems {
		recipeBullets = append(recipeBullets, f.text)
	}
	var manifestBullets []string
	for _, cmd := range tableList(brief, "useful_commands") {
		if pyTruthy(cmd) {
			if s := pyStr(cmd); !stringInSlice(recipeBullets, s) {
				manifestBullets = append(manifestBullets, s)
			}
		}
	}

	lines := []string{"## Useful Commands", ""}
	if pyTruthy(testCommand) {
		cmd := pyStr(testCommand)
		if strings.Contains(cmd, "validate.sh") {
			lines = append(lines, "- Full validation: `"+cmd+"`")
		} else {
			lines = append(lines, "- Focused tests: `"+cmd+"`")
		}
	}
	for _, b := range recipeBullets {
		lines = append(lines, "- "+b)
	}
	for _, b := range manifestBullets {
		lines = append(lines, "- "+b)
	}
	emitted := map[string]bool{}
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			emitted[line[2:]] = true
		}
	}
	if !emitted[harnessCLILiteracyPointer] {
		lines = append(lines, "- "+harnessCLILiteracyPointer)
	}
	return append(lines, "")
}

// ---------------------------------------------------------------------------
// Main render (pure byte assembly)
// ---------------------------------------------------------------------------

// renderLines is _render_lines(): the fixed section order.
func renderLines(manifest *toml.Table, resolved map[string]any) []string {
	project := manifestTable(manifest, "project")
	brief := manifestTable(manifest, "brief")

	var lines []string
	if name := tableGetAny(project, "name"); pyTruthy(name) {
		lines = append(lines, "# "+pyStr(name)+" Runtime Brief", "")
	} else {
		lines = append(lines, "# AGENTS.md - Runtime context", "")
	}
	lines = append(lines, sectionIntro(brief)...)
	lines = append(lines, sectionProject(manifest, resolved)...)
	lines = append(lines, sectionMCP(manifest, effectiveMCPDescriptions(resolved, brief))...)
	lines = append(lines, sectionRuntimeFlow(brief, resolved)...)
	lines = append(lines, sectionTrello(resolved)...)
	lines = append(lines, sectionContextSources(brief, resolved)...)
	lines = append(lines, sectionConflictPolicy(brief, resolved)...)
	lines = append(lines, sectionWorkflowRules(brief, resolved)...)
	lines = append(lines, sectionUsefulCommands(brief, resolved)...)
	return lines
}

// RenderAgentsMarkdown returns exactly the bytes render() would pass to
// output_path.write_bytes(...) when its governance decision is "write":
// "\n".join(_render_lines(manifest, resolved)).encode().
//
// Invalid [brief] <section>_mode values make the Python authority raise, so
// this returns nil; callers validate first with ValidateBriefModes (which
// carries the byte-exact ValueError message) and must not write on nil.
func RenderAgentsMarkdown(manifest *toml.Table, resolved map[string]any) []byte {
	if err := ValidateBriefModes(manifestTable(manifest, "brief")); err != nil {
		return nil
	}
	return []byte(strings.Join(renderLines(manifest, resolved), "\n"))
}

// ---------------------------------------------------------------------------
// Manifest / resolved accessors
// ---------------------------------------------------------------------------

func manifestTable(t *toml.Table, key string) *toml.Table {
	if t == nil {
		return nil
	}
	sub, _ := t.Table(key)
	return sub
}

func tableGet(t *toml.Table, key string) (any, bool) {
	if t == nil {
		return nil, false
	}
	return t.Get(key)
}

func tableGetAny(t *toml.Table, key string) any {
	v, _ := tableGet(t, key)
	return v
}

func tableList(t *toml.Table, key string) []any {
	if l, ok := tableGetAny(t, key).([]any); ok {
		return l
	}
	return nil
}

func tableStringDefault(t *toml.Table, key, def string) string {
	if s, ok := tableGetAny(t, key).(string); ok {
		return s
	}
	return def
}

func resolvedMap(resolved map[string]any, key string) map[string]any {
	if m, ok := resolved[key].(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func resolvedList(resolved map[string]any, key string) []any {
	switch l := resolved[key].(type) {
	case []any:
		return l
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	}
	return nil
}

func mapValue(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if sub, ok := m[key].(map[string]any); ok && sub != nil {
		return sub
	}
	return map[string]any{}
}

func mapGetAny(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

func listValue(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if l, ok := m[key].([]any); ok {
		return l
	}
	return nil
}

// cfgField reads recipes[rid][key]; a missing recipe or key yields nil (falsy).
func cfgField(recipes map[string]any, rid, key string) any {
	return mapGetAny(mapValue(recipes, rid), key)
}

func resolvedEnabledSet(resolved map[string]any) map[string]bool {
	set := map[string]bool{}
	for _, v := range resolvedList(resolved, "enabled") {
		if s, ok := v.(string); ok {
			set[s] = true
		}
	}
	return set
}

func setDifference(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		if !b[k] {
			out[k] = true
		}
	}
	return out
}

func listContains(list []any, want string) bool {
	for _, v := range list {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

func stringInSlice(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Python value formatting mirrors
// ---------------------------------------------------------------------------

// pyTruthy mirrors Python truthiness for the value types a manifest table or a
// JSON-decoded resolved-config produces.
func pyTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case []string:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case *toml.Table:
		return x != nil && len(x.Keys()) > 0
	default:
		return true
	}
}

// pyStr mirrors Python str(): strings pass through, everything else is repr().
func pyStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return pyRepr(v)
}

// pyRepr mirrors Python repr() for manifest (TOML) and resolved (JSON) values.
// A JSON-decoded map[string]any has already lost document order, so its keys
// are sorted for determinism; *toml.Table keeps it.
func pyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return pyReprString(x)
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return formatPyFloat(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyReprString(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, pyReprString(k)+": "+pyRepr(x[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *toml.Table:
		if x == nil {
			return "{}"
		}
		parts := make([]string, 0, len(x.Keys()))
		for _, k := range x.Keys() {
			vv, _ := x.Get(k)
			parts = append(parts, pyReprString(k)+": "+pyRepr(vv))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// pyReprString mirrors Python repr() for strings. Copied from
// internal/target/json.go's unexported helper (same package boundary).
func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case rune(quote):
			sb.WriteByte('\\')
			sb.WriteByte(quote)
		default:
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || !unicode.IsPrint(r) {
				sb.WriteString(pyUnicodeEscape(r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}

// pyUnicodeEscape renders one non-printable rune in Python repr escape form.
func pyUnicodeEscape(r rune) string {
	const hex = "0123456789abcdef"
	digits := func(n int) string {
		out := make([]byte, n)
		for i := n - 1; i >= 0; i-- {
			out[i] = hex[r&0xf]
			r >>= 4
		}
		return string(out)
	}
	switch {
	case r < 0x100:
		return `\x` + digits(2)
	case r < 0x10000:
		return `\u` + digits(4)
	default:
		return `\U` + digits(8)
	}
}

// formatPyFloat renders f like Python str(float) (shortest round-trip repr).
// Copied from internal/target/json.go's unexported helper.
func formatPyFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	neg := math.Signbit(f)
	a := math.Abs(f)
	if a == 0 {
		if neg {
			return "-0.0"
		}
		return "0.0"
	}
	e := strconv.FormatFloat(a, 'e', -1, 64)
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1

	var s string
	if decpt <= -4 || decpt > 16 {
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		expStr := strconv.Itoa(absInt(exp10))
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
	return s
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// pyIsSpace matches Python str.isspace() for the characters str.strip() trims.
func pyIsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		0x1c, 0x1d, 0x1e, 0x1f, 0x85, 0xa0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007,
		0x2008, 0x2009, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return false
}

func pyStrip(s string) string {
	return strings.TrimFunc(s, pyIsSpace)
}

// pyIsLineBreak matches Python str.splitlines() boundaries.
func pyIsLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// pySplitLines mirrors Python str.splitlines(): split on every line boundary,
// with no trailing empty element for a trailing terminator, and "" → [].
func pySplitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if pyIsLineBreak(r) {
			out = append(out, s[start:i])
			if r == '\r' && i+size < len(s) && s[i+size] == '\n' {
				size++
			}
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// ---------------------------------------------------------------------------
// S3b: write governance (classify / adopt / preserve) + render entry
// ---------------------------------------------------------------------------

// runtimeBriefMarker is RUNTIME_BRIEF_MARKER. A file containing it is always
// preserved (design D5 makes the marker unconditional).
const runtimeBriefMarker = "<!-- ai-specs:runtime-brief -->"

// briefPreserveMessage is BRIEF_PRESERVE_MESSAGE: the three-line stderr notice
// _brief_preserve prints, with the final newline print() adds.
const briefPreserveMessage = "  \u2139 AGENTS.md left unchanged (%s; preserving existing file)\n" +
	"    to let ai-specs manage it:  ai-specs sync --adopt-brief\n" +
	"    to keep it yours forever:   add <!-- ai-specs:runtime-brief --> at the top\n"

// RenderAgentsOptions configures RenderAgentsFile.
type RenderAgentsOptions struct {
	// PreserveIfRuntimeBrief is --preserve-if-runtime-brief. It is deliberately
	// inert (design D5): the marker in _brief_decision preserves unconditionally,
	// so the flag can never turn preservation off.
	PreserveIfRuntimeBrief bool
	// AdoptBrief is --adopt-brief: a one-time handoff that records an existing
	// user file as the managed baseline without overwriting its bytes.
	AdoptBrief bool
	// ResolvedConfigPath is --resolved-config; "" renders without structured
	// fields, exactly like render() with resolved_config_path=None.
	ResolvedConfigPath string
}

// briefLockPath is _brief_lock_path.
func briefLockPath(tomlPath string) string {
	return filepath.Join(filepath.Dir(tomlPath), ".ai-specs.lock")
}

// briefLockKey is _brief_lock_key: the project-relative lock key for a rendered
// brief, resolved the same non-strict way Python's Path.resolve() does. An
// output outside the manifest's project is refused (ValueError parity).
func briefLockKey(tomlPath, outputPath string) (string, error) {
	parent := filepath.Dir(tomlPath)
	projectRoot := filepath.Dir(parent)
	if filepath.Base(parent) != "ai-specs" {
		projectRoot = parent
	}
	projectRoot = resolveNonStrict(projectRoot)
	out := resolveNonStrict(outputPath)
	rel, err := filepath.Rel(projectRoot, out)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("brief target is outside project root: %s", outputPath)
	}
	return filepath.ToSlash(rel), nil
}

// resolveNonStrict mirrors Path.resolve(): it canonicalizes symlinks for the
// longest existing prefix and appends the non-existent tail unchanged.
func resolveNonStrict(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	dir := filepath.Dir(abs)
	if dir == abs {
		return abs
	}
	return filepath.Join(resolveNonStrict(dir), filepath.Base(abs))
}

// classifyManagedOverride is the decision half of
// _python_classify_managed_override (the historical Python authority): disk vs
// lock baseline vs the exact would-write bytes.
func classifyManagedOverride(outputPath string, entry map[string]any, wouldWrite []byte) string {
	fi, err := os.Stat(outputPath)
	if err != nil || !fi.Mode().IsRegular() {
		return "missing"
	}
	disk, err := os.ReadFile(outputPath)
	if err != nil {
		return "missing"
	}
	diskSHA := lock.Sha256Bytes(disk)
	sha, _ := entry["sha256"].(string)
	if entry == nil || sha == "" {
		return "untracked"
	}
	if diskSHA != sha {
		return "user_modified"
	}
	if wouldWrite == nil {
		return "managed_current"
	}
	if diskSHA == lock.Sha256Bytes(wouldWrite) {
		return "managed_current"
	}
	return "managed_stale"
}

// classifyBriefState is classify_brief(): the marker check first, then the
// lock-backed managed-override decision. Every failure mode collapses to
// "undetermined" (the Python `except Exception`).
func classifyBriefState(tomlPath, outputPath string, wouldWrite []byte, lockPath string, lk *lock.Lock) string {
	if _, err := os.Stat(outputPath); err == nil {
		data, rerr := os.ReadFile(outputPath)
		if rerr != nil || !utf8.Valid(data) {
			return "undetermined"
		}
		if strings.Contains(string(data), runtimeBriefMarker) {
			return "marker"
		}
	}
	if lk == nil {
		loaded, err := lock.LoadLock(lockPath)
		if err != nil {
			return "undetermined"
		}
		lk = loaded
	}
	key, err := briefLockKey(tomlPath, outputPath)
	if err != nil {
		return "undetermined"
	}
	return classifyManagedOverride(outputPath, lk.Managed[key], wouldWrite)
}

// briefIsOurOutput is _brief_is_our_output: disk bytes byte-identical to what
// we would write, compared through the same normalization the classifier uses.
func briefIsOurOutput(outputPath string, content []byte) bool {
	var disk []byte
	if fi, err := os.Stat(outputPath); err == nil && fi.Mode().IsRegular() {
		if b, rerr := os.ReadFile(outputPath); rerr == nil {
			disk = b
		}
	}
	return lock.Sha256Bytes(disk) == lock.Sha256Bytes(content)
}

// briefEffectiveState is brief_effective_state(): the state sync would ACT on.
// A brief with no baseline whose bytes are already ours is silently adopted by
// sync, so it reports as managed_current rather than untracked/user_modified.
func briefEffectiveState(tomlPath, outputPath string, wouldWrite []byte, lockPath string, lk *lock.Lock) string {
	state := classifyBriefState(tomlPath, outputPath, wouldWrite, lockPath, lk)
	if state == "untracked" || state == "user_modified" {
		if briefIsOurOutput(outputPath, wouldWrite) {
			return "managed_current"
		}
	}
	return state
}

// briefOwnershipState is the deprecated brief_ownership_state alias; it must
// stay one decision with briefEffectiveState (design D1), never a second
// inline marker check.
func briefOwnershipState(tomlPath, outputPath string, wouldWrite []byte, lockPath string, lk *lock.Lock) string {
	return briefEffectiveState(tomlPath, outputPath, wouldWrite, lockPath, lk)
}

// briefDecision is _brief_decision(): classify one brief and return the
// internal action plus the loaded lock. Classification errors fail closed as
// preservation with an empty lock.
func briefDecision(tomlPath, outputPath, lockPath string, content []byte, adoptBrief bool) (string, *lock.Lock) {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return "preserve-undetermined", nil
	}
	state := classifyBriefState(tomlPath, outputPath, content, lockPath, lk)
	if state == "marker" {
		return "preserved", lk
	}
	if state == "missing" {
		return "write", lk
	}
	isOurOutput := briefIsOurOutput(outputPath, content)
	switch state {
	case "untracked":
		if isOurOutput || adoptBrief {
			return "adopt", lk
		}
		return "preserve-untracked", lk
	case "user_modified":
		if isOurOutput || adoptBrief {
			return "adopt", lk
		}
		return "preserve-user_modified", lk
	case "managed_current":
		return "current", lk
	case "managed_stale":
		return "write", lk
	default:
		return "preserve-" + state, lk
	}
}

// loadResolvedConfig is render()'s resolved-config read: a missing, non-file,
// unreadable, non-dict or malformed JSON path degrades to the empty dict, then
// the three inner fields are coerced to their expected types.
func loadResolvedConfig(path string) map[string]any {
	resolved := map[string]any{}
	if path == "" {
		return resolved
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return resolved
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return resolved
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return resolved
	}
	m, ok := parsed.(map[string]any)
	if !ok {
		return resolved
	}
	if _, ok := m["bindings"].(map[string]any); !ok {
		m["bindings"] = map[string]any{}
	}
	if _, ok := m["recipes"].(map[string]any); !ok {
		m["recipes"] = map[string]any{}
	}
	if _, ok := m["enabled"].([]any); !ok {
		m["enabled"] = []any{}
	}
	return m
}

// loadAgentsRenderInputs is render()'s prelude: parse the manifest, load and
// coerce the resolved-config JSON, then setdefault project_root to the
// manifest's grandparent (ai-specs.toml lives under <root>/ai-specs/).
func loadAgentsRenderInputs(tomlPath, resolvedConfigPath string) (*toml.Table, map[string]any, error) {
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := toml.Parse(data)
	if err != nil {
		return nil, nil, err
	}
	resolved := loadResolvedConfig(resolvedConfigPath)
	if _, ok := resolved["project_root"]; !ok {
		resolved["project_root"] = filepath.Dir(filepath.Dir(resolveNonStrict(tomlPath)))
	}
	return manifest, resolved, nil
}

// warnUnknownVCS reproduces the one stderr warning _section_runtime_flow emits
// for a bound VCS recipe outside _VCS_RECIPE_LABELS. The Python de-dupe is a
// per-call local set, so at most one line is emitted per render.
func warnUnknownVCS(resolved map[string]any, stderr io.Writer) {
	raw := mapGetAny(resolvedMap(resolved, "bindings"), "vcs-pr-flow")
	id, ok := raw.(string)
	if !ok || id == "" {
		return
	}
	if _, known := vcsRecipeLabels[id]; known {
		return
	}
	fmt.Fprintf(stderr, "\u26a0 ai-specs: VCS recipe '%s' is not in the known label set; using generic label 'VCS PR (custom)'\n", id)
}

// writeBriefPreserve is _brief_preserve: print the state notice to stderr.
func writeBriefPreserve(stderr io.Writer, state string) {
	fmt.Fprintf(stderr, briefPreserveMessage, state)
}

// RenderAgentsFile is render(): classify the AGENTS.md target and act on it.
// It returns the action render() returned ("written"/"adopted"/"preserved"/
// "current"), whether write_bytes ran, and the process exit status main()
// would produce. stdout carries nothing (main() prints no stdout), so the only
// writer side effects are the preserve notice and the unknown-VCS warning on
// stderr plus the AGENTS.md and lock bytes.
func RenderAgentsFile(tomlPath, outputPath string, opts RenderAgentsOptions, stdout, stderr io.Writer) (action string, wrote bool, rc int) {
	manifest, resolved, err := loadAgentsRenderInputs(tomlPath, opts.ResolvedConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", false, 1
	}
	warnUnknownVCS(resolved, stderr)
	content := RenderAgentsMarkdown(manifest, resolved)
	if content == nil {
		brief, _ := manifest.Table("brief")
		if verr := ValidateBriefModes(brief); verr != nil {
			fmt.Fprintln(stderr, verr.Error())
			return "", false, 1
		}
		content = []byte{}
	}

	lockPath := briefLockPath(tomlPath)
	decision, lk := briefDecision(tomlPath, outputPath, lockPath, content, opts.AdoptBrief)
	switch {
	case decision == "preserved":
		writeBriefPreserve(stderr, "marker")
		return "preserved", false, 0
	case strings.HasPrefix(decision, "preserve-"):
		writeBriefPreserve(stderr, strings.TrimPrefix(decision, "preserve-"))
		return "preserved", false, 0
	case decision == "current":
		return "current", false, 0
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", false, 1
	}
	if decision == "write" {
		if err := os.WriteFile(outputPath, content, 0o644); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return "", false, 1
		}
		wrote = true
	}
	var baseline []byte
	if decision == "write" {
		baseline = content
	} else {
		baseline, _ = os.ReadFile(outputPath)
	}
	key, err := briefLockKey(tomlPath, outputPath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", wrote, 1
	}
	lock.SetBriefBaseline(lk, key, lock.Sha256Bytes(baseline))
	if err := lock.WriteLock(lockPath, lk); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return "", wrote, 1
	}
	if decision == "adopt" {
		return "adopted", false, 0
	}
	return "written", true, 0
}

// renderAgentsStep is the native path of the sync AGENTS.md step: the
// brief-render-policy.py gate first, then the renderer. A missing or
// unparseable manifest prints `error: ...` and skips, matching the shell's
// `$(...)`-ignored policy exit status.
func renderAgentsStep(tomlPath, outputPath, resolvedConfigPath string, adoptBrief bool, out, errW io.Writer) int {
	enabled, err := EvaluateBriefRender(tomlPath)
	if err != nil {
		fmt.Fprintf(errW, "error: %v\n", err)
		enabled = false
	}
	if !enabled {
		fmt.Fprintln(out, "  \u2139 skipped AGENTS.md (brief.render = false)")
		return 0
	}
	_, _, rc := RenderAgentsFile(tomlPath, outputPath, RenderAgentsOptions{
		AdoptBrief:         adoptBrief,
		ResolvedConfigPath: resolvedConfigPath,
	}, out, errW)
	return rc
}
