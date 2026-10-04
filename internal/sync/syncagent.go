// syncagent.go — native port of lib/sync-agent.sh (epic [Go 07.S15]).
//
// The port mirrors the shell script's observable contract line by line:
// flag parsing and the exit-2 contract, the standalone target resolution and
// multi-target fan-out (children run nested), and the single-target body
// (materialize fallback with the RECIPE_MCP_TEMP: seam, cache derivation,
// flatten/merge, ensure_target_workspace, per-agent fan-out) with the
// recorded D3/D22/D24 semantics. Steps that already have byte-exact Go ports
// (gitignore render, brief policy, agents/mcp/hooks render, flatten,
// merge-commands, cache paths) call them directly; recipe-materialize.py and
// target-resolve.py are still exec'd (S14 owns the materialize port), and
// every python3 child runs with the same argv/cwd/env as the shell did.
//
// Shell facts reproduced deliberately (each pinned by a test or verified
// against bash under set -e): a failing command substitution inside [[ ]]
// falls through to the false branch (the brief-policy gate), while a plain
// assignment's substitution failure kills the script with the child's
// status; cp keeps an existing destination's mode and takes the source's
// mode for a new one; cp -R preserves modes and copies symlinks as links.

package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/toml"
)

// syncAgentCDLineTarget / syncAgentCDLineSource are the lines of the two
// `cd` command substitutions in lib/sync-agent.sh (108 and 110). bash reports
// those numbers in its failing-cd diagnostic; the native spine emits them
// too. Pinned by TestSyncAgentCDLineMatchesShellSource.
const (
	syncAgentCDLineTarget = 108
	syncAgentCDLineSource = 110
)

// syncAgentUsage is the byte-exact body of usage() in lib/sync-agent.sh (a
// quoted heredoc, so nothing is substituted). Pinned by
// TestSyncAgentUsageMatchesShellHeredoc.
const syncAgentUsage = `Usage: ai-specs sync-agent [path] [--all | --<agent>...] [--adopt-brief] [-v|--verbose]
       ai-specs sync-agent --source-root <root> --target <path> [--all | --<agent>...] [--adopt-brief] [-v|--verbose]

Render per-agent configs from the root manifest.

Arguments:
  path             Target path when using the legacy single-target form

Flags:
  --source-root    Root project that owns ai-specs/ai-specs.toml (default: target)
  --target         Target directory receiving derived local artifacts
  --resolved-hooks Pre-resolved runtime-hooks JSON (from recipe-materialize)
  --adopt-brief   Explicitly adopt the current AGENTS.md as the managed runtime-brief baseline
  --all            All agents listed under [agents].enabled in ai-specs.toml
  --claude         Claude Code  (CLAUDE.md, .claude/skills, .mcp.json)
  --cursor         Cursor       (.cursor/mcp.json)
  --opencode       OpenCode     (opencode.json, .opencode/skills, .opencode/commands)
  --codex          Codex        (.codex/config.toml)
  --copilot        GitHub Copilot (.github/copilot-instructions.md)
  --gemini         Gemini CLI   (GEMINI.md, .gemini/skills, .gemini/settings.json)
  --pi             Pi (pi.dev)  (.pi/skills, .mcp.json)
  --omp            Oh My Pi     (.omp/skills, .omp/mcp.json, .omp/commands)
  -v, --verbose    Print full per-step detail instead of compact summaries

If no selector is given, defaults to --all.
`

// agentOptions is the parsed sync-agent command line.
type agentOptions struct {
	target             string
	sourceRoot         string
	recipeMCP          string
	resolvedConfig     string
	resolvedHooks      string
	adoptBrief         bool
	selectAll          bool
	selected           []string
	verbose            bool
	explicitSourceRoot bool
	explicitTarget     bool
}

// RunAgent is the in-process entry point for the `sync-agent` verb. args[0]
// is the verb itself and is dropped, exactly like sync.Run.
func RunAgent(args []string, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	opts, code, done := parseAgentFlags(rest, stdout, stderr)
	if done {
		return code
	}
	r := &runner{
		home:    home,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		verbose: opts.verbose,
		env:     childEnv(home),
	}
	s := agentStreams{out: stdout, errW: stderr}
	script := filepath.Join(home, "lib", "sync-agent.sh")

	// lib/sync-agent.sh:107-108 — TARGET_PATH defaults to pwd, then is
	// resolved through `cd ... && pwd`.
	target := opts.target
	if target == "" {
		wd, err := os.Getwd()
		if err != nil {
			return 1
		}
		target = wd
	}
	absTarget, err := resolveTarget(target)
	if err != nil {
		fmt.Fprintf(stderr, "%s: line %d: cd: %s: %s\n", script, syncAgentCDLineTarget, target, strerrorText(err))
		return 1
	}
	// lib/sync-agent.sh:109-110 — SOURCE_ROOT defaults to the RESOLVED
	// target path, then is resolved too.
	sourceRoot := opts.sourceRoot
	if sourceRoot == "" {
		sourceRoot = absTarget
	}
	absSource, err := resolveTarget(sourceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "%s: line %d: cd: %s: %s\n", script, syncAgentCDLineSource, sourceRoot, strerrorText(err))
		return 1
	}

	// Standalone mode (lib/sync-agent.sh:112-172): neither flag was given
	// explicitly. With more than one resolved target the root manifest fans
	// out to per-target nested children; with 0 or 1 the shell falls through
	// keeping the ORIGINAL paths (it never adopts the resolved root for the
	// single-target body).
	if !opts.explicitSourceRoot && !opts.explicitTarget {
		planJSON, ok := r.resolvePlanStandalone(absTarget, stderr)
		if !ok {
			fmt.Fprintln(stderr, "ERROR: target resolution failed before any writes.")
			return 1
		}
		// The shell's ROOT_PATH/RESOLVED_TARGETS extraction dies on a
		// malformed plan (json.JSONDecodeError traceback, measured bytes
		// pinned below) and on a plan without "root" (KeyError: 'root').
		// Both deaths happen BEFORE any write; stderr is FROZEN, so the port
		// reproduces the measured diagnostic bytes instead of a generic line.
		var rawPlan map[string]json.RawMessage
		if err := json.Unmarshal(planJSON, &rawPlan); err != nil {
			fmt.Fprint(stderr, pyJSONDecodeErrPlanBytes)
			return 1
		}
		rawRoot, ok := rawPlan["root"]
		if !ok {
			fmt.Fprint(stderr, pyKeyErrRootBytes)
			return 1
		}
		var p plan
		if err := json.Unmarshal(planJSON, &p); err != nil {
			fmt.Fprint(stderr, pyJSONDecodeErrPlanBytes)
			return 1
		}
		if p.Root == "" && string(bytes.TrimSpace(rawRoot)) == "null" {
			// json["root"] → None prints as `None`, which the shell then uses
			// verbatim as ROOT_PATH (no KeyError). Reproduced faithfully.
			p.Root = "None"
		}
		if len(p.Targets) > 1 {
			return r.runAgentStandaloneFanout(p.Root, opts, p, s)
		}
	}

	child := agentOptions{
		sourceRoot:     absSource,
		target:         absTarget,
		recipeMCP:      opts.recipeMCP,
		resolvedConfig: opts.resolvedConfig,
		resolvedHooks:  opts.resolvedHooks,
		adoptBrief:     opts.adoptBrief,
		selectAll:      opts.selectAll,
		selected:       opts.selected,
		verbose:        opts.verbose,
	}
	return runAgentSingle(r, child, s, nestedFromEnv())
}

// nestedFromEnv mirrors the `[[ "${AI_SPECS_SYNC_NESTED:-0}" != "1" ]]`
// framing guards: exactly the value "1" suppresses banner and footer.
func nestedFromEnv() bool {
	return os.Getenv("AI_SPECS_SYNC_NESTED") == "1"
}

// pyJSONDecodeErrPlanBytes / pyKeyErrRootBytes are the STABLE PORTABLE
// diagnostics for the standalone plan extraction (human decision
// 2026-10-04: pinned local Python traceback constants were REJECTED — the
// interpreter/install-anchored bytes under /usr/bin/python3 3.9.6 vs
// 3.13/3.14 proved them non-portable). They preserve the oracle's FROZEN
// diagnostic meaning — exit 1, the exception-class distinction
// (JSONDecodeError vs KeyError: 'root'), no banner and no writes before the
// failure — while the traceback formatting, interpreter paths and frames
// are TOLERANT by contract (docs/go-migration-parity-contract.md).
const pyJSONDecodeErrPlanBytes = "ERROR: resolver plan is not valid JSON (json.JSONDecodeError)\n"

const pyKeyErrRootBytes = "ERROR: resolver plan is missing the \"root\" key (KeyError: 'root')\n"

// resolvePlanStandalone runs target-resolve.py with stderr inherited and
// stdout captured, mirroring PLAN_JSON="$(python3 ...)".
func (r *runner) resolvePlanStandalone(target string, stderr io.Writer) ([]byte, bool) {
	argv := []string{"python3", filepath.Join(r.home, "lib", "_internal", "target-resolve.py"), target}
	var buf bytes.Buffer
	if rc := r.exec(argv, &buf, stderr); rc != 0 {
		return nil, false
	}
	return buf.Bytes(), true
}

// runAgentStandaloneFanout is lib/sync-agent.sh's multi-target branch: the
// public-root banner, the --resolved-config-only seam, then one nested child
// per resolved target, stopping on the first failure.
func (r *runner) runAgentStandaloneFanout(root string, opts agentOptions, p plan, s agentStreams) int {
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "ai-specs sync-agent")
	fmt.Fprintf(s.out, "  source root: %s\n", root)
	fmt.Fprintf(s.out, "  planning:    %s\n", root)
	paths := make([]string, len(p.Targets))
	for i, t := range p.Targets {
		paths[i] = t.Path
	}
	fmt.Fprintf(s.out, "  targets:     %s\n", strings.Join(paths, " "))
	fmt.Fprintln(s.out, "  mode:        public root fan-out (declared-only)")
	fmt.Fprintln(s.out, "")

	// Resolved-config for subrepo AGENTS.md enrichment: deliberately
	// copy-free/hook-free/lock-free (--resolved-config-only), merged onto the
	// script's stdout by the shell's `2>&1` (one fd, kernel write order).
	resolvedConfigTemp, mkErrText, ok := r.mktempCapture("-t", "ai-specs-resolved-config-XXXXXX.json")
	if !ok {
		fmt.Fprint(s.errW, mkErrText)
		return 1
	}
	defer removeIfSet(resolvedConfigTemp)
	matArgs := []string{"python3", filepath.Join(r.home, "lib", "_internal", "recipe-materialize.py"), root, r.home,
		"--resolved-config-out", resolvedConfigTemp,
		"--resolved-config-only"}
	if rc := r.execMerged(matArgs, s.out); rc != 0 {
		fmt.Fprintln(s.errW, "WARNING: resolved-config generation failed; subrepo AGENTS.md will be rendered without structured fields.")
	}

	forward := agentOptions{
		sourceRoot:     root,
		recipeMCP:      "", // every standalone child materializes itself
		resolvedConfig: resolvedConfigTemp,
		adoptBrief:     opts.adoptBrief,
		selectAll:      opts.selectAll,
		selected:       opts.selected,
		verbose:        opts.verbose,
	}
	// lib/sync-agent.sh:156-161 — --resolved-config is forwarded only when
	// the temp exists; mktemp already created the file, so a failed
	// materialize still forwards the empty file, exactly like the shell.
	if !isRegularFilePath(resolvedConfigTemp) {
		forward.resolvedConfig = ""
	}
	for _, resolvedTarget := range paths {
		child := forward
		child.target = resolvedTarget
		fmt.Fprintf(s.out, "  syncing %s\n", resolvedTarget)
		if rc := runAgentSingle(r, child, s, true); rc != 0 {
			fmt.Fprintf(s.errW, "ERROR: sync-agent failed for target: %s\n", resolvedTarget)
			fmt.Fprintln(s.errW, "       Stopped on first failure; no overall success reported.")
			return 1
		}
	}
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "✓ sync-agent complete")
	return 0
}

// agentStreams are the output streams of one agent-runner invocation. They
// differ from r.stdout/r.stderr when the fan-out runs as a captured parent
// step (the sync spine) rather than at the top level.
type agentStreams struct {
	out  io.Writer
	errW io.Writer
}

// agentRun is one resolved single-target run: the shell's script-level
// variables between the flag loop and the final footer.
type agentRun struct {
	r      *runner
	out    io.Writer
	errW   io.Writer
	nested bool // AI_SPECS_SYNC_NESTED=1: banner/footer suppressed

	sourceRoot string
	targetPath string
	tomlPath   string

	recipeMCP      string
	resolvedConfig string
	resolvedHooks  string
	adoptBrief     bool
	verbose        bool
	selectAll      bool
	selected       []string

	enabledAgents     []string
	mcpCount          int
	skillsSource      string
	commandsSource    string
	resolvedSkillsDir string
	mergedCommandsDir string
}

// runAgentSingle executes one single-target run: the shell's body from the
// manifest guard to the footer. nested suppresses the framing.
func runAgentSingle(r *runner, opts agentOptions, s agentStreams, nested bool) int {
	a, rc := buildAgentRun(r, opts, s, nested)
	if rc != 0 {
		return rc
	}
	return a.runBody()
}

// buildAgentRun performs everything the shell does before the D24 gate:
// manifest guard, materialize fallback, cache derivation with the flatten and
// merge steps, and enabled-agent resolution.
func buildAgentRun(r *runner, opts agentOptions, s agentStreams, nested bool) (*agentRun, int) {
	a := &agentRun{
		r:              r,
		out:            s.out,
		errW:           s.errW,
		nested:         nested,
		sourceRoot:     opts.sourceRoot,
		targetPath:     opts.target,
		tomlPath:       filepath.Join(opts.sourceRoot, "ai-specs", "ai-specs.toml"),
		recipeMCP:      opts.recipeMCP,
		resolvedConfig: opts.resolvedConfig,
		resolvedHooks:  opts.resolvedHooks,
		adoptBrief:     opts.adoptBrief,
		verbose:        opts.verbose,
		selectAll:      opts.selectAll,
		selected:       opts.selected,
	}

	// Guard before any recipe/flatten work (lib/sync-agent.sh:178-186): an
	// uninitialized project gets the init hint, not a materialize error.
	if !isRegularFilePath(a.tomlPath) {
		fmt.Fprintf(s.errW, "ERROR: %s not found. Run 'ai-specs init %s' first.\n", a.tomlPath, a.sourceRoot)
		return nil, 1
	}

	// Materialize fallback (lib/sync-agent.sh:191-202): when --recipe-mcp is
	// absent, run materialize now with both streams captured to one temp file
	// and parse RECIPE_MCP_TEMP: out of it. Failure prints the full output
	// minus the marker line and exits 1. A mktemp failure replays mktemp's
	// OWN stderr (the oracle's `$(mktemp ...)` lets the child's stderr pass
	// through before the set -e death) and exits 1.
	if a.recipeMCP == "" {
		outPath, mkErrText, ok := r.mktempCapture("-t", "ai-specs-materialize-XXXXXX")
		if !ok {
			fmt.Fprint(s.errW, mkErrText)
			return nil, 1
		}
		defer removeIfSet(outPath)
		matArgs := []string{"python3", filepath.Join(r.home, "lib", "_internal", "recipe-materialize.py"), a.sourceRoot, r.home}
		if rc := r.execToFileMerged(matArgs, outPath); rc != 0 {
			fmt.Fprintln(s.errW, "ERROR: recipe materialize failed")
			replayExceptMarker(s.errW, outPath)
			return nil, 1
		}
		a.recipeMCP = markerTempPath(outPath)
	}

	// Cache derivation (lib/sync-agent.sh:291-296). The shell's command
	// substitution dies with the child's rc on failure; ensure_cache errors
	// propagate the same way.
	resolvedSkills, rc := deriveCachePath(r, s.errW, a.sourceRoot, projectcache.ResolvedSkillsDir)
	if rc != 0 {
		return nil, rc
	}
	a.resolvedSkillsDir = resolvedSkills
	if rc := r.runStepW(s.out, s.errW, "flatten resolved skills", func(out, errW io.Writer) int {
		return skills.Flatten(a.sourceRoot, resolvedSkills, r.home, out, errW)
	}); rc != 0 {
		return nil, rc
	}
	cacheRoot, rc := deriveCachePath(r, s.errW, a.sourceRoot, projectcache.CacheRoot)
	if rc != 0 {
		return nil, rc
	}
	merged := filepath.Join(cacheRoot, "merged-commands")
	a.mergedCommandsDir = merged
	if rc := r.runStepW(s.out, s.errW, "merge commands", func(out, errW io.Writer) int {
		n, err := projectcache.MergeCommands(a.sourceRoot, merged, r.home, out, errW)
		if err != nil {
			fmt.Fprintf(errW, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(out, "  ✓ merged %d command file(s) → %s\n", n, merged)
		return 0
	}); rc != 0 {
		return nil, rc
	}

	enabled, rc := readEnabledAgents(a.tomlPath, s.errW)
	if rc != 0 {
		return nil, rc
	}
	a.enabledAgents = enabled
	return a, 0
}

// deriveCachePath mirrors `$(python3 project-cache.py ROOT path KIND)`:
// ensure_cache first (its failure is Python's uncaught RuntimeError, rc 1),
// then the derived path.
func deriveCachePath(r *runner, errW io.Writer, projectRoot string, fn func(root, cliHome string) string) (string, int) {
	if _, err := projectcache.EnsureCache(projectRoot, r.home); err != nil {
		fmt.Fprintf(errW, "error: %s\n", err)
		return "", 1
	}
	return fn(projectRoot, r.home), 0
}

// readEnabledAgents mirrors ENABLED_JSON + the json → print loop: every
// element of [agents].enabled in manifest order, empty strings dropped.
func readEnabledAgents(tomlPath string, errW io.Writer) ([]string, int) {
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		fmt.Fprintf(errW, "error: %s\n", err)
		return nil, 1
	}
	manifest, err := toml.Parse(data)
	if err != nil {
		// The shell's ENABLED_JSON substitution dies rc 1 on a toml-read
		// failure; the port keeps the status and prints one error line.
		fmt.Fprintf(errW, "error: %s\n", err)
		return nil, 1
	}
	agents, _ := manifest.Table("agents")
	var raw any
	if agents != nil {
		raw, _ = agents.Get("enabled")
	}
	return normalizedStringList(raw), 0
}

// normalizedStringList mirrors toml-read.py's _normalize_string_list
// (lib/_internal/toml-read.py:47-58), which read_agents applies to
// [agents].enabled: keep only STRIPPED non-empty strings; non-string items
// are dropped, never repr'd. The canonical Go port lives in
// internal/config (normalizedStringList); it is unexported, so the exact
// semantics are reproduced here rather than widening that package's
// surface.
//
// The value is then printed by the shell (`print(a)` per element) and read
// back through `while IFS= read -r` (lib/sync-agent.sh:391-394), so an
// element's EMBEDDED newlines split it into several agent entries; only
// non-empty lines survive the `[[ -n "$agent" ]]` guard, and \r is retained
// inside a piece (read splits on \n only). Measured RED probes:
// "\u001cclaude" strips to claude (Python isspace includes the information
// separators U+001C–U+001F; Go unicode.IsSpace does not), and "a\nb" yields
// agents a and b. Internal blank lines, a trailing newline and a CR are all
// verified against the while-read oracle (TestAgentsReadLoopSplitting).
func normalizedStringList(raw any) []string {
	out := []string{}
	for _, item := range plainList(raw) {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = pyStrStrip(s)
		for _, piece := range strings.Split(s, "\n") {
			if piece != "" {
				out = append(out, piece)
			}
		}
	}
	return out
}

// pyStrStrip mirrors Python str.strip() with no arguments: every character
// where str.isspace() is true. Python's isspace INCLUDES U+001C–U+001F
// (information separators); Go's unicode.IsSpace does not, so the predicate
// adds them explicitly. Measured oracle: "\u001cclaude" → "claude".
func pyStrStrip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || (r >= 0x001C && r <= 0x001F)
	})
}

// plainList mirrors doctor's asPlainList: the TOML array forms a plain list.
func plainList(raw any) []any {
	switch x := raw.(type) {
	case []any:
		return x
	}
	return nil
}

// runBody is the shell body from the agent-selection guard to the footer
// (lib/sync-agent.sh:394-551).
func (a *agentRun) runBody() int {
	// Agent selection: --all or no selector → [agents].enabled; otherwise the
	// explicit selectors.
	targets := a.selected
	if a.selectAll || len(a.selected) == 0 {
		targets = a.enabledAgents
	}

	// D24 (lib/sync-agent.sh:403-410): ensure the target workspace BEFORE
	// reporting success, so a subrepo still receives its artifacts and a root
	// workspace missing AGENTS.md fails instead of exiting 0.
	if len(targets) == 0 {
		if rc := a.ensureTargetWorkspace(a.out, a.errW); rc != 0 {
			return rc
		}
		fmt.Fprintln(a.errW, "WARNING: no agents to sync. Set [agents].enabled in ai-specs.toml.")
		return 0
	}

	// MCP_COUNT (lib/sync-agent.sh:412-425): manifest [mcp.*] keys plus the
	// recipe-mcp JSON top-level entries, purely for the banner and the mcp
	// gate. The shell's inline python dies rc 1 on a malformed manifest.
	count, err := a.mcpServerCount()
	if err != nil {
		// The oracle's inline python dies with an uncaught traceback (rc 1);
		// the port prints the stable portable class-correct diagnostic
		// (contract: recipe-mcp fatal-error framing — formatting/frames
		// TOLERANT, class distinction and exit 1 FROZEN).
		fmt.Fprintln(a.errW, err.Error())
		return 1
	}
	a.mcpCount = count

	if !a.nested {
		fmt.Fprintln(a.out, "")
		fmt.Fprintln(a.out, "ai-specs sync-agent")
		fmt.Fprintf(a.out, "  source root: %s\n", a.sourceRoot)
		fmt.Fprintf(a.out, "  target:      %s\n", a.targetPath)
		fmt.Fprintf(a.out, "  agents:      %s\n", strings.Join(targets, " "))
		enabled := "(none)"
		if len(a.enabledAgents) > 0 {
			enabled = strings.Join(a.enabledAgents, " ")
		}
		fmt.Fprintf(a.out, "  enabled:     %s\n", enabled)
		fmt.Fprintf(a.out, "  mcp:         %d server(s)\n", a.mcpCount)
		fmt.Fprintln(a.out, "")
		fmt.Fprintln(a.out, "  derived artifacts: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs")
	}

	if rc := a.ensureTargetWorkspace(a.out, a.errW); rc != 0 {
		return rc
	}

	// For the root workspace agents consume from the cache flatten + merged
	// commands; other targets consume from their mirrored artifacts.
	if a.targetPath == a.sourceRoot {
		a.skillsSource = a.resolvedSkillsDir
		a.commandsSource = a.mergedCommandsDir
	} else {
		a.skillsSource = filepath.Join(a.targetPath, "ai-specs", "skills")
		a.commandsSource = filepath.Join(a.targetPath, "ai-specs", "commands")
	}

	for _, agent := range targets {
		agentName := agent
		if rc := a.r.runStepW(a.out, a.errW, agentName, func(out, errW io.Writer) int {
			return a.syncOneAgent(agentName, out, errW)
		}); rc != 0 {
			return rc
		}
	}

	if !a.nested {
		fmt.Fprintln(a.out, "")
		fmt.Fprintln(a.out, "✓ sync-agent complete")
	}
	return 0
}

// mcpServerCount is the inline python heredoc of lib/sync-agent.sh:412-425:
// len([mcp.*] keys) + len(recipe-mcp JSON top level). A missing or malformed
// recipe-mcp JSON counts as empty (FileNotFoundError / JSONDecodeError).
func (a *agentRun) mcpServerCount() (int, error) {
	data, err := os.ReadFile(a.tomlPath)
	if err != nil {
		return 0, err
	}
	manifest, err := toml.Parse(data)
	if err != nil {
		return 0, err
	}
	count := 0
	if mcp, ok := manifest.Table("mcp"); ok {
		count += len(mcp.Keys())
	}
	if a.recipeMCP != "" {
		raw, rerr := os.ReadFile(a.recipeMCP)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				return count, nil // FileNotFoundError → pass
			}
			// Other read errors are UNCAUGHT in the oracle (script dies rc 1).
			// Portable class-correct diagnostic (final bounded round F3): the
			// Python exception class is named, no interpreter paths/frames.
			return 0, fmt.Errorf("ERROR: recipe-mcp read failed (%s): %s", mcpReadClass(rerr), a.recipeMCP)
		}
		var v any
		if jerr := json.Unmarshal(raw, &v); jerr != nil {
			return count, nil // JSONDecodeError (malformed/empty) → pass
		}
		switch x := v.(type) {
		case map[string]any:
			count += len(x)
		case []any:
			count += len(x) // measured RED: "[1,2]" → 2 server(s)
		case string:
			count += utf8.RuneCountInString(x) // Python len(str): code points; "abc" → 3
		default:
			// number/bool/null: len() raises TypeError, uncaught → death rc 1
			// (measured RED: "123" → traceback, rc 1). Class-correct portable
			// diagnostic (F3) — the previous text ("object of unsized type")
			// was invented without meaning.
			return 0, fmt.Errorf("ERROR: recipe-mcp JSON has no len (TypeError)")
		}
	}
	return count, nil
}

// mcpReadClass maps a recipe-mcp read error to the Python exception class the
// oracle would raise for it (open() semantics): EISDIR → IsADirectoryError,
// EACCES → PermissionError, otherwise OSError.
func mcpReadClass(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EISDIR:
			return "IsADirectoryError"
		case syscall.EACCES:
			return "PermissionError"
		}
	}
	return "OSError"
}

// ensureTargetWorkspace is lib/sync-agent.sh:357-386.
func (a *agentRun) ensureTargetWorkspace(out, errW io.Writer) int {
	targetAgentsMD := filepath.Join(a.targetPath, "AGENTS.md")
	if a.targetPath == a.sourceRoot {
		if !isRegularFilePath(targetAgentsMD) {
			fmt.Fprintf(errW, "ERROR: %s not found. Run 'ai-specs init %s' first.\n", targetAgentsMD, a.targetPath)
			return 1
		}
		return 0
	}

	targetAiSpecs := filepath.Join(a.targetPath, "ai-specs")
	// `mkdir -p "$TARGET_AI_SPECS"` runs bare at top level (set -e): the
	// mkdir binary prints its coreutils shape — an occupied leaf is File
	// exists — and the script dies rc 1 (measured RED probe P4).
	if err := mkdirPMirror(targetAiSpecs); err != nil {
		printChildErr(errW, "mkdir", err)
		return 1
	}
	// Subrepo/fan-out side-output: the gitignore render goes through run_step
	// like every other step so compact mode filters its ✓ line.
	if rc := a.r.runStepW(out, errW, "ai-specs/.gitignore", func(stepOut, stepErr io.Writer) int {
		return RenderAiSpecsGitignore(a.tomlPath, filepath.Join(targetAiSpecs, ".gitignore"), stepOut)
	}); rc != 0 {
		return rc
	}
	if rc := mirrorDirectory(a.resolvedSkillsDir, filepath.Join(targetAiSpecs, "skills"), errW); rc != 0 {
		return rc
	}
	if rc := mirrorDirectory(a.mergedCommandsDir, filepath.Join(targetAiSpecs, "commands"), errW); rc != 0 {
		return rc
	}

	// Brief policy gate (lib/sync-agent.sh:371). A failing command
	// substitution inside [[ ]] falls through to the false branch under set
	// -e (verified against bash), so a policy error lands in the else path.
	enabled, err := EvaluateBriefRender(a.tomlPath)
	if err != nil {
		fmt.Fprintf(errW, "error: %v\n", err)
		enabled = false
	}
	if enabled {
		opts := RenderAgentsOptions{PreserveIfRuntimeBrief: true, AdoptBrief: a.adoptBrief}
		if a.resolvedConfig != "" && isRegularFilePath(a.resolvedConfig) {
			opts.ResolvedConfigPath = a.resolvedConfig
		}
		// The shell calls agents-render.py directly (not through run_step):
		// its output is unfiltered in both compact and verbose mode.
		_, _, rc := RenderAgentsFile(a.tomlPath, targetAgentsMD, opts, out, errW)
		return rc
	}
	if !isRegularFilePath(targetAgentsMD) {
		fmt.Fprintf(errW, "ERROR: %s not found and brief.render = false.\n", targetAgentsMD)
		fmt.Fprintln(errW, "       Create AGENTS.md manually or set [brief].render = true.")
		return 1
	}
	fmt.Fprintln(out, "    ℹ skipped AGENTS.md (brief.render = false)")
	return 0
}

// syncOneAgent is lib/sync-agent.sh:448-542.
func (a *agentRun) syncOneAgent(agent string, out, errW io.Writer) int {
	if _, ok := platformGet(agent, "native"); !ok {
		fmt.Fprintf(errW, "  ✗ unknown agent: %s\n", agent)
		return 0
	}

	isEnabled := false
	for _, e := range a.enabledAgents {
		if e == agent {
			isEnabled = true
			break
		}
	}
	if !isEnabled {
		fmt.Fprintf(out, "  ! %s not in [agents].enabled — syncing anyway\n", agent)
	}

	// Instructions: a RELATIVE symlink to the target AGENTS.md.
	if instr, _ := platformGet(agent, "instructions_path"); instr != "" {
		if rc := makeRelativeSymlink(filepath.Join(a.targetPath, "AGENTS.md"), filepath.Join(a.targetPath, instr), out, errW); rc != 0 {
			return rc
		}
	}

	// Skills: an ABSOLUTE symlink to the cache-backed resolved skills; a real
	// directory from a legacy install is replaced (lib/sync-agent.sh:476-481).
	if skillsDir, _ := platformGet(agent, "skills_dir"); skillsDir != "" {
		skillsLink := filepath.Join(a.targetPath, skillsDir)
		if _, err := os.Stat(skillsLink); err == nil {
			if info, lerr := os.Lstat(skillsLink); lerr == nil && info.Mode()&os.ModeSymlink == 0 {
				if rc := removeTree(skillsLink, errW); rc != 0 {
					return rc
				}
			}
		}
		if rc := makeSkillsSymlink(a.skillsSource, skillsLink, out, errW); rc != 0 {
			return rc
		}
	}

	// MCP: rendered only when the manifest declares servers; otherwise a
	// compact-surviving notice.
	mcpPath, _ := platformGet(agent, "mcp_config_path")
	mcpKey, _ := platformGet(agent, "mcp_key")
	if mcpPath != "" && mcpKey != "" {
		if a.mcpCount > 0 {
			opts := RenderMCPOptions{RecipeMCPPath: a.recipeMCP}
			if rc := RenderMCPFile(a.tomlPath, agent, filepath.Join(a.targetPath, mcpPath), mcpKey, opts, out, errW); rc != 0 {
				return rc
			}
		} else {
			fmt.Fprintln(out, "    ℹ mcp skipped (no [mcp.*] in manifest)")
		}
	}

	// Commands: the D3' fix — never rm -rf the agent commands dir.
	if cmdDir, _ := platformGet(agent, "commands_dir"); cmdDir != "" && isDirPath(a.commandsSource) {
		if rc := a.syncCommands(cmdDir, out, errW); rc != 0 {
			return rc
		}
	}

	// Runtime hooks: only when --resolved-hooks was passed AND the file
	// exists (D22: standalone runs render no hooks). A render failure is
	// swallowed by the shell's `if python3 ...; then echo ✓; fi`: no notice,
	// no failure.
	if hooksTarget, _ := platformGet(agent, "runtime_hooks_target"); hooksTarget != "" && a.resolvedHooks != "" && isRegularFilePath(a.resolvedHooks) {
		if RenderHooks(a.resolvedHooks, agent, a.targetPath, out, errW) == 0 {
			fmt.Fprintf(out, "    ✓ runtime hooks %s\n", hooksTarget)
		}
	}
	return 0
}

// syncCommands is the commands fan-out with the D3' semantics: managed names
// are overwritten in place (unlinking any symlink or non-regular occupant
// first so cp cannot write through), everything else is preserved with a
// warning (lib/sync-agent.sh:497-531).
func (a *agentRun) syncCommands(cmdDir string, out, errW io.Writer) int {
	dest := filepath.Join(a.targetPath, cmdDir)
	// `mkdir -p "$dest" || return $?`: the mkdir binary prints the
	// coreutils-shaped error (an intermediate occupied component is Not a
	// directory) and the failure DOES abort the agent step with mkdir's rc.
	if err := mkdirPMirror(dest); err != nil {
		printChildErr(errW, "mkdir", err)
		return 1
	}
	copied := 0
	managed := map[string]bool{}
	for _, src := range globMDFilesSh(a.commandsSource) {
		name := filepath.Base(src)
		targetFile := filepath.Join(dest, name)
		if info, err := os.Lstat(targetFile); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				// Never write through a symlink occupying a managed name;
				// `rm -f ... || return $?` aborts with rm's own shape.
				if err := os.Remove(targetFile); err != nil {
					printChildErr(errW, "rm", err)
					return 1
				}
			} else if !info.Mode().IsRegular() {
				if rc := removeTree(targetFile, errW); rc != 0 {
					return rc
				}
			}
		}
		// `cp "$src" "$target_file" || return $?`: the cp binary prints its
		// own shape — `cp: <destpath>: <strerror>` (measured RED probe P5:
		// a 0555 commands dir yields Permission denied naming the DEST) —
		// and the step aborts with cp's rc.
		if err := copyFileCp(src, targetFile); err != nil {
			printChildErr(errW, "cp", err)
			return 1
		}
		managed[name] = true
		copied++
	}
	for _, name := range globStarNames(dest) {
		full := filepath.Join(dest, name)
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if !managed[name] {
			fmt.Fprintf(errW, "    ! preserved non-managed file %s/%s (move it to ai-specs/commands/ to manage it)\n", cmdDir, name)
		}
	}
	if copied > 0 {
		fmt.Fprintf(out, "    ✓ commands     %s/ (%d file(s))\n", cmdDir, copied)
	}
	return 0
}

// platformInfo is one platform.sh row.
type platformInfo struct {
	instructionsPath   string
	skillsDir          string
	mcpConfigPath      string
	mcpKey             string
	native             string
	commandsDir        string
	runtimeHooksTarget string
}

// agentPlatformMatrix is lib/_internal/platform.sh's agent→paths/keys table.
// Pinned field-by-field against the shell source by
// TestAgentPlatformMatchesPlatformSh; do not mutate without the shell file.
var agentPlatformMatrix = map[string]platformInfo{
	"claude": {
		instructionsPath:   "CLAUDE.md",
		skillsDir:          ".claude/skills",
		mcpConfigPath:      ".mcp.json",
		mcpKey:             "mcpServers",
		native:             "false",
		commandsDir:        ".claude/commands",
		runtimeHooksTarget: ".claude/settings.json",
	},
	"cursor": {
		instructionsPath:   "",
		skillsDir:          ".cursor/skills",
		mcpConfigPath:      ".cursor/mcp.json",
		mcpKey:             "mcpServers",
		native:             "true",
		commandsDir:        ".cursor/commands",
		runtimeHooksTarget: ".cursor/hooks.json",
	},
	"opencode": {
		instructionsPath:   "",
		skillsDir:          ".opencode/skills",
		mcpConfigPath:      "opencode.json",
		mcpKey:             "mcp",
		native:             "true",
		commandsDir:        ".opencode/commands",
		runtimeHooksTarget: ".opencode/plugin",
	},
	"codex": {
		instructionsPath:   "",
		skillsDir:          "",
		mcpConfigPath:      ".codex/config.toml",
		mcpKey:             "mcp_servers",
		native:             "true",
		commandsDir:        "",
		runtimeHooksTarget: "",
	},
	"copilot": {
		instructionsPath:   ".github/copilot-instructions.md",
		skillsDir:          "",
		mcpConfigPath:      "",
		mcpKey:             "",
		native:             "true",
		commandsDir:        "",
		runtimeHooksTarget: "",
	},
	"gemini": {
		instructionsPath:   "GEMINI.md",
		skillsDir:          ".gemini/skills",
		mcpConfigPath:      ".gemini/settings.json",
		mcpKey:             "mcpServers",
		native:             "false",
		commandsDir:        "",
		runtimeHooksTarget: "",
	},
	"pi": {
		instructionsPath:   "",
		skillsDir:          ".pi/skills",
		mcpConfigPath:      ".mcp.json",
		mcpKey:             "mcpServers",
		native:             "true",
		commandsDir:        "",
		runtimeHooksTarget: ".pi/extensions",
	},
	"omp": {
		instructionsPath:   ".omp/AGENTS.md",
		skillsDir:          ".omp/skills",
		mcpConfigPath:      ".omp/mcp.json",
		mcpKey:             "mcpServers",
		native:             "true",
		commandsDir:        ".omp/commands",
		runtimeHooksTarget: ".omp/extensions",
	},
}

// platformGet mirrors platform_get: the field value for a known agent, false
// for an unknown agent or field.
func platformGet(agent, field string) (string, bool) {
	p, ok := agentPlatformMatrix[agent]
	if !ok {
		return "", false
	}
	switch field {
	case "instructions_path":
		return p.instructionsPath, true
	case "skills_dir":
		return p.skillsDir, true
	case "mcp_config_path":
		return p.mcpConfigPath, true
	case "mcp_key":
		return p.mcpKey, true
	case "native":
		return p.native, true
	case "commands_dir":
		return p.commandsDir, true
	case "runtime_hooks_target":
		return p.runtimeHooksTarget, true
	}
	return "", false
}

// parseAgentFlags translates the lib/sync-agent.sh parse loop (L64-106). It
// returns done=true when the loop terminated the process (help, usage error).
func parseAgentFlags(args []string, stdout, stderr io.Writer) (agentOptions, int, bool) {
	var opts agentOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--source-root":
			v, ok := argValue(args, &i)
			if !ok {
				return opts, 1, true
			}
			opts.sourceRoot = v
			opts.explicitSourceRoot = true
		case arg == "--target":
			v, ok := argValue(args, &i)
			if !ok {
				return opts, 1, true
			}
			opts.target = v
			opts.explicitTarget = true
		case arg == "--recipe-mcp":
			v, ok := argValue(args, &i)
			if !ok {
				return opts, 1, true
			}
			opts.recipeMCP = v
		case arg == "--resolved-config":
			v, ok := argValue(args, &i)
			if !ok {
				return opts, 1, true
			}
			opts.resolvedConfig = v
		case arg == "--resolved-hooks":
			v, ok := argValue(args, &i)
			if !ok {
				return opts, 1, true
			}
			opts.resolvedHooks = v
		case arg == "--adopt-brief":
			opts.adoptBrief = true
		case arg == "--all":
			opts.selectAll = true
		case len(arg) > 2 && strings.HasPrefix(arg, "--") && agentSelectorKnown(arg[2:]):
			opts.selected = append(opts.selected, arg[2:])
		case arg == "-v" || arg == "--verbose":
			opts.verbose = true
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, syncAgentUsage)
			return opts, 0, true
		case arg == "--":
			// `shift; break`: parsing stops, the remainder is discarded.
			return opts, 0, false
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs sync-agent --help' for usage.")
			return opts, 2, true
		default:
			if opts.target == "" {
				opts.target = arg
			} else {
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
				return opts, 2, true
			}
		}
	}
	return opts, 0, false
}

// agentSelectorKnown reports whether name is one of the eight selector flags
// the shell's case list accepts (--claude ... --omp).
func agentSelectorKnown(name string) bool {
	_, ok := platformGet(name, "native")
	return ok
}

// argValue consumes the flag's operand ("${2:-}"; shift 2). A value-taking
// flag as the LAST argument makes `shift 2` fail under set -e: the script
// exits 1 with NO output and no writes (measured rc=1, empty stderr) — ok
// false signals that death to the caller (D20 defect class).
func argValue(args []string, i *int) (string, bool) {
	if *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	return "", false
}

// ── Symlink helpers (lib/sync-agent.sh:318-355) ───────────────────────────

// makeSkillsSymlink links with an ABSOLUTE realpath target: cache-backed
// resolved skills live out of tree.
func makeSkillsSymlink(targetAbs, linkPath string, out, errW io.Writer) int {
	linkDir := filepath.Dir(linkPath)
	if err := mkdirPMirror(linkDir); err != nil {
		// `mkdir -p "$link_dir"` has no `|| return` in the oracle: the failure
		// prints coreutils-shaped stderr and the helper CONTINUES (errexit is
		// off inside run_step; the ✓ echo and rc 0 still happen).
		printChildErr(errW, "mkdir", err)
	}
	abs := projectcache.ResolvePath(targetAbs) // os.path.realpath
	return writeSymlink(linkPath, abs, out, errW)
}

// makeRelativeSymlink links with a target relative to the link's directory:
// instructions files point at the target's own AGENTS.md.
func makeRelativeSymlink(targetAbs, linkPath string, out, errW io.Writer) int {
	linkDir := filepath.Dir(linkPath)
	if err := mkdirPMirror(linkDir); err != nil {
		printChildErr(errW, "mkdir", err)
	}
	rel, err := filepath.Rel(linkDir, targetAbs)
	if err != nil {
		fmt.Fprintf(errW, "error: %s\n", err)
		return 1
	}
	return writeSymlink(linkPath, rel, out, errW)
}

// writeSymlink is the shared body of the two shell helpers: idempotent when
// the existing link already points at the target, relink when it points
// elsewhere, hard refusal on non-symlink occupancy. mkdir/rm/ln failures
// print the child's stderr shape and CONTINUE — the oracle runs these bare
// inside run_step's set +e, so the ✓ echo and rc 0 still happen; only the
// explicit refusal returns 1.
func writeSymlink(linkPath, target string, out, errW io.Writer) int {
	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if existing, rerr := os.Readlink(linkPath); rerr == nil && existing == target {
				// Noise (keep ·): idempotent success detail; filtered in
				// compact mode.
				fmt.Fprintf(out, "    · symlink ok      %s → %s\n", linkPath, target)
				return 0
			}
			if err := os.Remove(linkPath); err != nil {
				printChildErr(errW, "rm", err)
			}
		} else if _, serr := os.Stat(linkPath); serr == nil {
			fmt.Fprintf(errW, "    ✗ refuse to overwrite non-symlink: %s\n", linkPath)
			return 1
		}
	}
	if err := os.Symlink(target, linkPath); err != nil {
		printChildErr(errW, "ln", err)
	}
	fmt.Fprintf(out, "    ✓ symlink created %s → %s\n", linkPath, target)
	return 0
}

// printChildErr renders a failed child-tool error the way the named coreutils
// binary does on this platform (measured against the real mkdir/rm/ln):
// `<tool>: <path>: <strerror>`, where path is the failing component for
// mkdir -p and the destination path for rm/ln (ln prints only the linkpath).
func printChildErr(errW io.Writer, tool string, err error) {
	var le *os.LinkError
	if errors.As(err, &le) {
		fmt.Fprintf(errW, "%s: %s: %s\n", tool, le.New, strerrorText(le.Err))
		return
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		fmt.Fprintf(errW, "%s: %s: %s\n", tool, pe.Path, strerrorText(pe.Err))
		return
	}
	fmt.Fprintf(errW, "%s: %s\n", tool, strerrorText(err))
}

// mkdirPMirror reproduces `mkdir -p` (BSD, measured on this platform):
// components are processed shallow→deep; an existing non-directory is
// reported as File exists when it is the argument itself and Not a directory
// when it is an intermediate component. Symlinks to directories count as
// directories, exactly like mkdir -p.
func mkdirPMirror(path string) error {
	if path == "" {
		return nil
	}
	prefix := ""
	if filepath.IsAbs(path) {
		prefix = "/"
	}
	for _, comp := range strings.Split(path, string(filepath.Separator)) {
		if comp == "" || comp == "." {
			continue
		}
		prefix = filepath.Join(prefix, comp)
		info, err := os.Stat(prefix)
		switch {
		case err == nil:
			if info.IsDir() {
				continue
			}
			// Existing non-directory: the argument itself → File exists,
			// an intermediate component → Not a directory.
			if prefix == path {
				return &os.PathError{Op: "mkdir", Path: prefix, Err: syscall.EEXIST}
			}
			return &os.PathError{Op: "mkdir", Path: prefix, Err: syscall.ENOTDIR}
		case os.IsNotExist(err):
			if mkErr := os.Mkdir(prefix, 0o777); mkErr != nil && !os.IsExist(mkErr) {
				return &os.PathError{Op: "mkdir", Path: prefix, Err: mkErr}
			}
		default:
			return &os.PathError{Op: "mkdir", Path: prefix, Err: err}
		}
	}
	return nil
}

// ── Filesystem helpers mirroring the shell's cp/rm semantics ──────────────

// mirrorDirectory is mirror_directory (lib/sync-agent.sh:298-308). Its bare
// `mkdir -p`/`rm -rf`/`cp -R` run at top level (set -e): failures print the
// coreutils shape and die rc 1.
func mirrorDirectory(src, dest string, errW io.Writer) int {
	if rc := removeTree(dest, errW); rc != 0 {
		return rc
	}
	if err := mkdirPMirror(filepath.Dir(dest)); err != nil {
		printChildErr(errW, "mkdir", err)
		return 1
	}
	if isDirPath(src) {
		return copyTreeCp(src, dest, errW)
	}
	if err := mkdirPMirror(dest); err != nil {
		printChildErr(errW, "mkdir", err)
		return 1
	}
	return 0
}

// copyTreeCp mirrors `cp -R src dest`: modes preserved, symlinks copied as
// symlinks. Directories are created writable and the source mode is applied
// AFTER the contents are written — cp's measured order (a 0555 source
// mirrors successfully and the destination ends 0555). A failing final
// chmod is an error (rc 1), matching cp's failure.
func copyTreeCp(src, dest string, errW io.Writer) int {
	info, err := os.Lstat(src)
	if err != nil {
		printChildErr(errW, "cp", err)
		return 1
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			printChildErr(errW, "cp", err)
			return 1
		}
		if err := os.Symlink(link, dest); err != nil {
			printChildErr(errW, "cp", err)
			return 1
		}
		return 0
	}
	if !info.IsDir() {
		if err := copyFileCp(src, dest); err != nil {
			printChildErr(errW, "cp", err)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(dest, 0o777); err != nil {
		printChildErr(errW, "cp", err)
		return 1
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		printChildErr(errW, "cp", err)
		return 1
	}
	for _, entry := range entries {
		if rc := copyTreeCp(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name()), errW); rc != 0 {
			return rc
		}
	}
	if err := os.Chmod(dest, info.Mode().Perm()); err != nil {
		printChildErr(errW, "cp", err)
		return 1
	}
	return 0
}

// copyFileCp mirrors `cp src dest`: content replaced, an existing regular
// destination keeps its mode, a new one takes the source's mode (measured on
// this platform's cp).
func copyFileCp(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if fi, err := os.Lstat(dest); err == nil && fi.Mode().IsRegular() {
		mode = fi.Mode().Perm()
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// removeTree mirrors `rm -rf` with rm's OWN traversal and reporting
// semantics (measured RED probe P6 + fixture delta): depth-first removal;
// every failed unlink/rmdir prints `rm: <path>: <strerror>` and removal
// CONTINUES (a child that cannot be removed still yields the parent's
// `Directory not empty` line afterwards); a missing path is ignored; the rc
// is 1 when anything failed. Call sites keep their oracle semantics (bare
// call at top level → set -e death; `|| return $?` sites → step abort).
func removeTree(path string, errW io.Writer) int {
	return removeRf(path, errW)
}

// removeTree mirrors `rm -rf` with the MEASURED BSD semantics (both shapes
// sanctioned-measured): (1) 0555 readable dir with child — the child rmdir
// fails EACCES AND the parent rmdir fails ENOTEMPTY: BOTH lines print, rc 1
// (child failures propagate but never stop the parent removal attempt);
// (2) 0333 unreadable EMPTY dir — rm does NOT report the read failure:
// it goes straight to the removal, which succeeds → SILENT rc 0. Reported
// errors are removal failures only; rc is 1 exactly when one was printed.
func removeRf(path string, errW io.Writer) int {
	rc := 0
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0 // rm -rf ignores missing operands
		}
		printChildErr(errW, "rm", err)
		return 1
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		// A read failure is NOT reported (measured: 0333 empty dir → silent
		// success); rm falls through to the removal attempt either way. A child
		// removal failure is printed by the child call and propagates (P6:
		// the child's EACCES line AND the parent's `Directory not empty` line
		// both print — rm continues to the parent attempt after a failure),
		// but it does NOT skip the parent's own removal attempt.
		childFailed := false
		if entries, rdErr := os.ReadDir(path); rdErr == nil {
			for _, entry := range entries {
				if rc := removeRf(filepath.Join(path, entry.Name()), errW); rc != 0 {
					childFailed = true
				}
			}
		}
		if childFailed {
			rc = 1
		}
	}
	if err := os.Remove(path); err != nil {
		printChildErr(errW, "rm", err)
		return 1
	}
	return rc
}

func isRegularFilePath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDirPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// globMDFilesSh mirrors the `for src in "$COMMANDS_SOURCE"/*.md` glob: byte-
// sorted (LC_ALL=C) names ending in .md, dotfiles excluded (bash globs never
// match a leading dot), symlinks to regular files included (`[[ -f ]]`).
func globMDFilesSh(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || filepath.Ext(name) != ".md" {
			continue
		}
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, full)
	}
	sort.Strings(out)
	return out
}

// globStarNames mirrors the `for extra in "$dest"/*` glob: byte-sorted
// non-hidden names.
func globStarNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		out = append(out, entry.Name())
	}
	sort.Strings(out)
	return out
}

// ── Child execution with the shell's stream shapes ────────────────────────

// mktempCapture runs the real mktemp binary and additionally captures its
// stderr, so a failure can replay mktemp's OWN diagnostic — the oracle's
// `$(mktemp ...)` lets the child's stderr pass through before the set -e
// death (rc 1, no other output). macOS `mktemp -t` ignores TMPDIR, so the
// live trigger is unreachable on this platform; the behavior is pinned by a
// PATH-stubbed mktemp in the unit tests (the same seam
// test_sync_run_step_errexit.py established for the S1 spine).
func (r *runner) mktempCapture(args ...string) (string, string, bool) {
	cmd := exec.Command("mktemp", args...)
	cmd.Env = r.env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", errBuf.String(), false
	}
	return strings.TrimRight(outBuf.String(), "\n"), "", true
}

// execMerged runs argv with stdout and stderr sharing one descriptor (the
// shell's `2>&1`), copying the merged bytes to w in kernel write order.
func (r *runner) execMerged(argv []string, w io.Writer) int {
	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(r.stderr, "%s: %s\n", argv[0], strerrorText(err))
		return 126
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = r.stdin
	cmd.Env = r.env
	cmd.Stdout = pw
	cmd.Stderr = pw
	startErr := cmd.Start()
	pw.Close()
	if startErr == nil {
		io.Copy(w, pr)
	}
	pr.Close()
	if startErr != nil {
		return mapExecErr(startErr, r.stderr, argv[0])
	}
	return mapExecErr(cmd.Wait(), r.stderr, argv[0])
}

// execToFileMerged runs argv with both streams redirected into one file
// (`>"$out" 2>&1`), preserving kernel write order.
func (r *runner) execToFileMerged(argv []string, path string) int {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(r.stderr, "%s: %s\n", argv[0], strerrorText(err))
		return 126
	}
	defer f.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = r.stdin
	cmd.Env = r.env
	cmd.Stdout = f
	cmd.Stderr = f
	return runExitCode(cmd, r.stderr, argv[0])
}

// mapExecErr translates an exec result into a process exit code the way
// step.go's runExitCode does (same table; duplicated because the merged-fd
// child started manually and this slice must not widen step.go).
func mapExecErr(err error, errW io.Writer, name string) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return 1
	}
	if isNotFoundExec(err) {
		fmt.Fprintf(errW, "%s: command not found\n", name)
		return 127
	}
	fmt.Fprintf(errW, "%s: %s\n", name, strerrorText(err))
	return 126
}

// isNotFoundExec mirrors step.go's isNotFound.
func isNotFoundExec(err error) bool {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.ENOENT
}

// markerTempPath is `grep '^RECIPE_MCP_TEMP:' file | cut -d: -f2-` joined by
// the shell's command substitution: the marker prefixes stripped, multiple
// matches newline-joined, empty when nothing matched.
func markerTempPath(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var matches []string
	for _, line := range lines {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			matches = append(matches, strings.TrimPrefix(line, "RECIPE_MCP_TEMP:"))
		}
	}
	return strings.Join(matches, "\n")
}

// replayExceptMarker is `grep -v '^RECIPE_MCP_TEMP:' file >&2 || true`.
func replayExceptMarker(w io.Writer, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			continue
		}
		fmt.Fprintln(w, line)
	}
}

// runStepW is run_step parameterized by streams: the fan-out runs as a
// captured parent step, so the agent body's labels and captures must land on
// the parent's writers instead of the runner's process-level ones.
func (r *runner) runStepW(out, errW io.Writer, label string, step stepFunc) int {
	fmt.Fprintf(out, "  syncing %s\n", label)

	outPath, ok := r.mktemp()
	if ok {
		errPath, ok2 := r.mktemp()
		if ok2 {
			return r.runStepCapturedW(out, errW, step, outPath, errPath)
		}
		os.Remove(outPath)
	}
	fmt.Fprintln(errW, "  ! cannot create temporary files (check TMPDIR); running this step with unfiltered output")
	return step(out, errW)
}

// runStepCapturedW is runStepCaptured with explicit writers.
func (r *runner) runStepCapturedW(out, errW io.Writer, step stepFunc, outPath, errPath string) int {
	outF, err := os.Create(outPath)
	if err != nil {
		os.Remove(outPath)
		os.Remove(errPath)
		return 1
	}
	errF, err := os.Create(errPath)
	if err != nil {
		outF.Close()
		os.Remove(outPath)
		os.Remove(errPath)
		return 1
	}

	rc := step(outF, errF)
	outF.Close()
	errF.Close()

	if rc != 0 {
		replayRaw(out, outPath)
		replayRaw(errW, errPath)
		os.Remove(outPath)
		os.Remove(errPath)
		return rc
	}
	printStepOutput(out, outPath, r.verbose)
	printStepOutput(errW, errPath, r.verbose)
	os.Remove(outPath)
	os.Remove(errPath)
	return 0
}
