// Package sync is the forward-strangler port of the lib/sync.sh pipeline
// spine. It reimplements only the orchestration — flag parsing, the
// before-any-writes target resolution, the header/footer, run_step stream
// capture and the errectx discipline — while every step still execs the
// existing Python module with the same argv, cwd and environment as the shell
// implementation. Nothing in the Python layer is ported here.
package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

// syncUsage is the byte-exact body of usage() in lib/sync.sh (a quoted
// heredoc, so nothing is substituted).
const syncUsage = `Usage: ai-specs sync [path] [--ignore-cli-version] [-v|--verbose]

Reconcile a project's ai-specs/ with its root manifest:
  - resolve [root, ...project.subrepos]
  - vendor [[deps]] once in the root workspace
  - regenerate AGENTS.md auto-invoke table
  - fan out local derived artifacts to every resolved target

Arguments:
  path      Project root (default: current directory)

Flags:
  --ignore-cli-version  Skip [tool] CLI version policy check (warns on stderr)
  --refresh-gates       Explicitly refresh customized gate hooks: save exact
                        pre-refresh bytes to a cache-only immutable backup,
                        then replace (never set by an ordinary sync)
  --adopt-brief         Explicitly adopt the current AGENTS.md as the managed
                        runtime-brief baseline
  -v, --verbose         Print full per-step detail instead of compact summaries
`

// syncCDLine is the line of `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"` in
// lib/sync.sh (lib/sync.sh:76). bash reports a failing `cd` with that line
// number, so the Go spine emits it too. args_test.go pins this constant to the
// shell source: drift in lib/sync.sh fails the Go test instead of silently
// breaking the byte-for-byte differential smoke test.
const syncCDLine = 76

// options is the parsed command line.
type options struct {
	target           string
	ignoreCliVersion bool
	refreshGates     bool
	adoptBrief       bool
	verbose          bool
}

// Run is the in-process entry point for the `sync` verb. args[0] is the verb
// itself and is dropped, exactly like runRefreshBundled.
func Run(args []string, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	opts, code, done := parseFlags(rest, stdout, stderr)
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
	return r.run(opts)
}

// parseFlags translates the lib/sync.sh parse loop. It returns done=true when
// the loop terminated the process (help, usage error); the caller returns code
// immediately.
func parseFlags(args []string, stdout, stderr io.Writer) (options, int, bool) {
	var opts options
	for _, arg := range args {
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, syncUsage)
			return opts, 0, true
		case arg == "--ignore-cli-version":
			opts.ignoreCliVersion = true
		case arg == "--refresh-gates":
			opts.refreshGates = true
		case arg == "--adopt-brief":
			opts.adoptBrief = true
		case arg == "-v" || arg == "--verbose":
			opts.verbose = true
		case arg == "--":
			// `shift; break` (lib/sync.sh:57): stop parsing and discard the
			// remainder. This is frozen parity, not a gap — legacy also leaves
			// TARGET_PATH empty for `sync -- <path>` and falls back to pwd
			// (lib/sync.sh:75). Pinned by TestParseFlagsDashDashParity.
			return opts, 0, false
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs sync --help' for usage.")
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

// stepMode resolves the per-step strangler flag GO_SYNC_STEP_<NAME>, defaulting
// to "python". NAME is the step's canonical name uppercased with every
// non-alphanumeric byte replaced by "_": "gitignore" → GO_SYNC_STEP_GITIGNORE,
// "gitignore-root" → GO_SYNC_STEP_GITIGNORE_ROOT. S1 has no Go step, so any
// value other than "python" is refused loudly rather than silently ignored.
func stepMode(name string) string {
	if v, ok := os.LookupEnv(stepKey(name)); ok && v != "" {
		return v
	}
	return "python"
}

// stepKey is the environment variable name for a step's strangler flag.
func stepKey(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return "GO_SYNC_STEP_" + strings.ToUpper(b.String())
}

// plan is the subset of target-resolve.py's JSON the spine consumes. Target
// resolution itself is never reimplemented.
type plan struct {
	Root         string `json:"root"`
	PlanningRoot string `json:"planning_root"`
	Topology     struct {
		Resolved string `json:"resolved"`
		Via      string `json:"via"`
		Source   string `json:"source"`
	} `json:"topology"`
	Targets []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
		Rel  string `json:"rel"`
	} `json:"targets"`
}

// recipeNameRe is the `▸ recipe ([^ \n]+)` extraction RECIPE_NAMES depends on
// (lib/sync.sh's grep|sed|paste pipeline).
var recipeNameRe = regexp.MustCompile(`▸ recipe ([^ \n]+)`)

// run executes the pipeline after flags were parsed.
func (r *runner) run(opts options) int {
	target := opts.target
	if target == "" {
		wd, err := os.Getwd()
		if err != nil {
			return 1
		}
		target = wd
	}
	// `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"` (lib/sync.sh:76). The shell
	// chdirs in a subshell only; the process cwd is left untouched.
	absTarget, err := resolveTarget(target)
	if err != nil {
		fmt.Fprintf(r.stderr, "%s: line %d: cd: %s: %s\n", filepath.Join(r.home, "lib", "sync.sh"), syncCDLine, target, strerrorText(err))
		return 1
	}

	// Target resolution before any write is the whole failure contract.
	planJSON, ok := r.resolvePlan(absTarget)
	if !ok {
		fmt.Fprintln(r.stderr, "ERROR: target resolution failed before any writes.")
		return 1
	}
	var p plan
	if err := json.Unmarshal(planJSON, &p); err != nil {
		return 1
	}
	root := p.Root
	planningRoot := p.PlanningRoot
	tomlPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	aiGitignore := filepath.Join(root, "ai-specs", ".gitignore")
	if fi, statErr := os.Stat(tomlPath); statErr != nil || !fi.Mode().IsRegular() {
		fmt.Fprintf(r.stderr, "ERROR: %s not found.\n", tomlPath)
		fmt.Fprintf(r.stderr, "       Run 'ai-specs init %s' first.\n", root)
		return 1
	}

	// cli_version.py check-sync runs directly (streams inherited); any failure
	// collapses to exit 1.
	checkArgs := []string{"python3", filepath.Join(r.home, "lib", "_internal", "cli_version.py"), "check-sync", root, r.home}
	if opts.ignoreCliVersion {
		checkArgs = append(checkArgs, "--ignore-cli-version")
	}
	if rc := r.exec(checkArgs, r.stdout, r.stderr); rc != 0 {
		return 1
	}

	paths := make([]string, len(p.Targets))
	labels := make([]string, len(p.Targets))
	for i, t := range p.Targets {
		paths[i] = t.Path
		labels[i] = t.Kind + ":" + t.Rel
	}

	fmt.Fprintln(r.stdout, "")
	fmt.Fprintln(r.stdout, "ai-specs sync")
	fmt.Fprintf(r.stdout, "  root:    %s\n", root)
	fmt.Fprintf(r.stdout, "  planning: %s\n", planningRoot)
	source := p.Topology.Source
	if source == "" {
		source = "default"
	}
	fmt.Fprintf(r.stdout, "  topology: %s (via %s; source: %s)\n", p.Topology.Resolved, p.Topology.Via, source)
	if p.Topology.Source == "legacy-recipe" {
		fmt.Fprintln(r.stderr, "  WARN: recipes.worktree-flow.config.repo_topology is deprecated; set [project].repo_topology")
	}
	fmt.Fprintf(r.stdout, "  targets: %s\n", strings.Join(labels, " "))
	fmt.Fprintln(r.stdout, "  fan-out: declared-only (project.subrepos; .gitmodules is advisory-only)")
	fmt.Fprintln(r.stdout, "  derived: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs")
	fmt.Fprintln(r.stdout, "")

	internal := func(name string) string {
		return filepath.Join(r.home, "lib", "_internal", name)
	}

	// Strangler flags a-c: GO_SYNC_STEP_GITIGNORE and GO_SYNC_STEP_GITIGNORE_ROOT
	// select the native Go gitignore renderers; GO_SYNC_STEP_AGENTS_RENDER
	// selects the native brief-render-policy + agents-render pair. "python" (the
	// default) still execs the modules; any other value is refused loudly rather
	// than silently falling back.
	for _, name := range [...]string{"gitignore", "gitignore-root", "agents-render"} {
		if mode := stepMode(name); mode != "python" && mode != "go" {
			fmt.Fprintf(r.stderr, "ERROR: %s=%s is not implemented in this slice\n", stepKey(name), mode)
			return 1
		}
	}
	rootTemplate := filepath.Join(r.home, "templates", "gitignore-root.tmpl")
	if rc := r.runStep("ai-specs/.gitignore", func(out, errW io.Writer) int {
		if stepMode("gitignore") == "go" {
			return RenderAiSpecsGitignore(tomlPath, aiGitignore, out)
		}
		return r.exec([]string{"python3", internal("gitignore-render.py"), tomlPath, aiGitignore}, out, errW)
	}); rc != 0 {
		return rc
	}
	if rc := r.runStep("root .gitignore (agent block)", func(out, errW io.Writer) int {
		if stepMode("gitignore-root") == "go" {
			return RefreshRootGitignore(root, rootTemplate, out, errW)
		}
		return r.exec([]string{"python3", internal("gitignore-root-refresh.py"), root, rootTemplate}, out, errW)
	}); rc != 0 {
		return rc
	}
	if rc := r.runStep("bundled skills + commands", func(out, errW io.Writer) int {
		return r.exec([]string{"python3", internal("refresh-bundled.py"), root, r.home}, out, errW)
	}); rc != 0 {
		return rc
	}
	if rc := r.runStep("vendored skills", func(out, errW io.Writer) int {
		return r.exec([]string{"python3", internal("vendor-skills.py"), root}, out, errW)
	}); rc != 0 {
		return rc
	}
	if rc := r.runStep("harness env (.envrc + ai-specs.env.example)", func(out, errW io.Writer) int {
		return r.exec([]string{"python3", internal("env_scaffold.py"), root}, out, errW)
	}); rc != 0 {
		return rc
	}

	// Recipe capture block (lib/sync.sh L226-272). The three -t temps are
	// created BEFORE the cleanup is registered, so an early failure strands
	// them exactly like the shell (reproduce, do not improve).
	recipeMcpTemp, ok := r.mktemp("-t", "ai-specs-recipe-mcp-XXXXXX.json")
	if !ok {
		return 1
	}
	resolvedConfigTemp, ok := r.mktemp("-t", "ai-specs-resolved-config-XXXXXX.json")
	if !ok {
		return 1
	}
	resolvedHooksTemp, ok := r.mktemp("-t", "ai-specs-resolved-hooks-XXXXXX.json")
	if !ok {
		return 1
	}
	// The shell's EXIT trap registers here, covering all five temps; the two
	// capture files are created after. defer runs when run returns — the whole
	// remainder of the pipeline — so the recipe temps survive the AGENTS step
	// and the fan-out, exactly like the trap.
	var recipeOutFile, recipeErrFile string
	defer func() {
		removeIfSet(recipeMcpTemp)
		removeIfSet(resolvedConfigTemp)
		removeIfSet(resolvedHooksTemp)
		removeIfSet(recipeOutFile)
		removeIfSet(recipeErrFile)
	}()

	recipeOutFile, ok = r.mktemp()
	if !ok {
		return 1
	}
	recipeErrFile, ok = r.mktemp()
	if !ok {
		return 1
	}

	matArgs := []string{"python3", internal("recipe-materialize.py"), root, r.home,
		"--recipe-mcp-out", recipeMcpTemp,
		"--resolved-config-out", resolvedConfigTemp,
		"--resolved-hooks-out", resolvedHooksTemp}
	if opts.refreshGates {
		matArgs = append(matArgs, "--refresh-gates")
	}
	recipeRC := 0
	if outF, err := os.Create(recipeOutFile); err != nil {
		recipeRC = 1
	} else if errF, err := os.Create(recipeErrFile); err != nil {
		outF.Close()
		recipeRC = 1
	} else {
		recipeRC = r.exec(matArgs, outF, errF)
		outF.Close()
		errF.Close()
	}

	// RECIPE_NAMES: `grep -oE '▸ recipe [^ ]+' | sed | paste -sd, -` (|| true).
	names := []string{}
	if data, err := os.ReadFile(recipeOutFile); err == nil {
		for _, m := range recipeNameRe.FindAllStringSubmatch(string(data), -1) {
			names = append(names, m[1])
		}
	}
	if recipeNames := strings.Join(names, ","); recipeNames != "" {
		fmt.Fprintf(r.stdout, "  syncing recipes → %s\n", strings.ReplaceAll(recipeNames, ",", ", "))
	} else {
		fmt.Fprintln(r.stdout, "  syncing recipes")
	}

	if recipeRC != 0 {
		replayRaw(r.stdout, recipeOutFile)
		replayRaw(r.stderr, recipeErrFile)
		removeIfSet(recipeOutFile)
		removeIfSet(recipeErrFile)
		return recipeRC
	}
	printStepOutput(r.stdout, recipeOutFile, opts.verbose)
	printStepOutput(r.stderr, recipeErrFile, opts.verbose)
	removeIfSet(recipeOutFile)
	removeIfSet(recipeErrFile)

	// AGENTS.md: policy gate then render. The policy child's exit code is
	// ignored, matching `$(...)` inside `[[ ]]`.
	if rc := r.runStep("AGENTS.md", func(out, errW io.Writer) int {
		if stepMode("agents-render") == "go" {
			return renderAgentsStep(tomlPath, filepath.Join(root, "AGENTS.md"), resolvedConfigTemp, opts.adoptBrief, out, errW)
		}
		var policyOut bytes.Buffer
		r.exec([]string{"python3", internal("brief-render-policy.py"), tomlPath}, &policyOut, errW)
		if strings.TrimRight(policyOut.String(), "\n") == "true" {
			argv := []string{"python3", internal("agents-render.py"), tomlPath, filepath.Join(root, "AGENTS.md"), "--preserve-if-runtime-brief"}
			if opts.adoptBrief {
				argv = append(argv, "--adopt-brief")
			}
			argv = append(argv, "--resolved-config", resolvedConfigTemp)
			return r.exec(argv, out, errW)
		}
		fmt.Fprintln(out, "  ℹ skipped AGENTS.md (brief.render = false)")
		return 0
	}); rc != 0 {
		return rc
	}

	// Fan-out: AI_SPECS_SYNC_NESTED=1 is exported from here on (never for
	// steps a-g). Any target failure is exit 1, not the step's rc.
	for i := range paths {
		target := paths[i]
		label := labels[i]
		argv := []string{"bash", filepath.Join(r.home, "lib", "sync-agent.sh"),
			"--source-root", root,
			"--target", target,
			"--all",
			"--recipe-mcp", recipeMcpTemp,
			"--resolved-config", resolvedConfigTemp,
			"--resolved-hooks", resolvedHooksTemp}
		if opts.adoptBrief {
			argv = append(argv, "--adopt-brief")
		}
		if opts.verbose {
			argv = append(argv, "--verbose")
		}
		rc := r.runStep(label+" → "+target, func(out, errW io.Writer) int {
			return r.execNested(argv, out, errW)
		})
		if rc != 0 {
			fmt.Fprintf(r.stderr, "ERROR: sync failed for target %s (%s). Stopped on first failure; previous writes are not rolled back.\n", target, label)
			return 1
		}
	}

	if rc := r.exec([]string{"python3", internal("cli_version.py"), "stamp-meta", root, r.home}, r.stdout, r.stderr); rc != 0 {
		return rc
	}

	fmt.Fprintln(r.stdout, "")
	fmt.Fprintln(r.stdout, "✓ ai-specs sync complete")
	return 0
}

// resolvePlan runs target-resolve.py with its stderr inherited and stdout
// captured, mirroring PLAN_JSON="$(python3 ...)".
func (r *runner) resolvePlan(target string) ([]byte, bool) {
	argv := []string{"python3", filepath.Join(r.home, "lib", "_internal", "target-resolve.py"), target}
	var buf bytes.Buffer
	if rc := r.exec(argv, &buf, r.stderr); rc != 0 {
		return nil, false
	}
	return buf.Bytes(), true
}

// resolveTarget reproduces `cd "$TARGET_PATH" && pwd` without changing the
// process cwd: it verifies the operand is a directory and returns its absolute
// cleaned path. On failure it returns the underlying stat error so the caller
// can print bash's `cd:` diagnostic.
func resolveTarget(target string) (string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", syscall.ENOTDIR
	}
	return filepath.Abs(target)
}

// strerrorText renders a filesystem error the way bash's `cd` does: strerror(3)
// wording with its first letter capitalized. Go's syscall.Errno.String() is the
// same wording in lowercase, so the explicit table exists only to pin the
// common cases and to capitalize everything else.
func strerrorText(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "No such file or directory"
		case syscall.ENOTDIR:
			return "Not a directory"
		case syscall.EACCES:
			return "Permission denied"
		case syscall.ELOOP:
			return "Too many levels of symbolic links"
		case syscall.ENAMETOOLONG:
			return "File name too long"
		}
		return capitalizeFirst(errno.Error())
	}
	return capitalizeFirst(err.Error())
}

// capitalizeFirst uppercases the first rune of s, leaving the rest untouched.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
