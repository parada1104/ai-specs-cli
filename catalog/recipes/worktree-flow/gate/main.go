// Command worktree-gate is the autocontained Go implementation of the
// worktree-flow pre-tool-use gate (see openspec/changes/worktree-gate-go).
//
// Phase 0 (PR 1) ships the skeleton only: the full flag surface is parsed
// with the final CLI contract, and --version, --selftest and --explain are
// implemented. No gate decision is made yet — every other invocation exits 0
// with empty output, which is the fail-open default. Runtime wiring, the
// decision core and distribution land in later phases of the change.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// version is injected at build time with
// -ldflags "-X main.version=<CLI version>" (see scripts/build-gate.sh).
// It defaults to "dev" for plain `go run` / `go build` without flags.
var version = "dev"

// gateRegexps holds every regular expression the gate uses for candidate
// extraction. --selftest compiles each one at startup so a pattern that
// fails to compile under the release toolchain is caught before the binary
// is trusted. Phase 2 appends the extraction patterns here.
var gateRegexps = []string{}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code. It is a separate
// function (instead of logic inside main) so the whole flag and exit-code
// contract is unit-testable in-process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("worktree-gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	gateMode := fs.String("gate-mode", "", "stamped WORKTREE_GATE_MODE value")
	gateScope := fs.String("gate-scope", "", "stamped WORKTREE_GATE_SCOPE value")
	repoTopology := fs.String("repo-topology", "", "stamped WORKTREE_REPO_TOPOLOGY value")
	protected := fs.String("protected", "main development", "space-separated protected branch names")
	showVersion := fs.Bool("version", false, "print the version and exit 0")
	cleanup := fs.Bool("cleanup", false, "run the worktree cleanup command")
	cleanupDir := fs.String("dir", ".worktrees", "worktree directory for cleanup")
	cleanupBase := fs.String("base", "", "base branch for cleanup merge proof")
	cleanupIntegration := fs.String("integration-branch", "", "configured integration branch")
	cleanupTopology := fs.String("topology", "auto", "cleanup repository topology")
	cleanupDryRun := fs.Bool("dry-run", false, "preview cleanup without destructive operations")
	cleanupScopes := stringListFlag{}
	fs.Var(&cleanupScopes, "submodule", "limit cleanup to a submodule path (repeatable)")
	fs.Var(&cleanupScopes, "subrepo", "limit cleanup to a subrepo path (repeatable)")
	tokenize := fs.Bool("tokenize", false, "tokenize stdin as a shell command (shlex posix); JSON diagnostic on stdout, exit 0")
	selfTest := fs.Bool("selftest", false, "self-check (regex compile, git presence); exit 1 on any failure")
	resolveCentral := fs.Bool("resolve-central-root", false, "print the proven central planning root and registered submodule as JSON; exit 1 when unproven")
	explain := fs.Bool("explain", false, "emit a JSON diagnostic on stdout (still exits 0/2)")
	// The ledger is the tracker grader. Its flags are disjoint from the worktree
	// gate flags and its mode never reads the worktree gate mode (A1/A9).
	ledgerRun := fs.Bool("ledger", false, "evaluate a tracker-ledger checkpoint (JSON on stdout, exit 0/2)")
	ledgerCheckpoint := fs.String("checkpoint", "", "ledger checkpoint: work-start|apply-start|pr-review|pre-merge|archive-close")
	ledgerMode := fs.String("ledger-mode", "", "ledger mode: always|ask|warn (default: resolve from env/config/hint, then warn)")
	ledgerGateMode := fs.String("ledger-gate-mode", "", "raw stamped legacy tracker gate_mode hint (off|warn|always); used only when no --ledger-mode")
	ledgerProjectRoot := fs.String("project-root", "", "owning repository path for ledger identity and the binding witness (default cwd)")
	ledgerWitness := fs.String("witness", "", "override the ledger witness path")
	ledgerStore := fs.String("store", "", "override the ledger store path")
	ledgerEvidence := fs.String("evidence", "", "path to a JSON evidence file (remote/code/git sides)")
	ledgerDecide := fs.String("decide", "", "JSON human decision to persist, then re-grade")
	ledgerWrite := fs.String("write", "", "JSON machine write to apply (open|bind|link|close|exempt), then re-grade")
	ledgerReconcile := fs.String("reconcile", "", "path to an MCP-acquired observation JSON; adds a reconcile sidecar (exit code unchanged)")
	ledgerReconcileEvent := fs.String("reconcile-event", "", "the event the caller asks to compare (recipe-declared expectations for it)")
	// Binding resolution is a separate command surface: it reads the catalog, not
	// the worktree, and its flags never touch the gate or ledger state.
	resolveBindingsCmd := fs.Bool("resolve-bindings", false, "resolve capability-to-recipe bindings and grade capability conflicts (JSON on stdout, exit 0/2)")
	bindingsCatalogDir := fs.String("catalog-dir", "", "catalog recipes directory for --resolve-bindings and --resolve-tag-conflicts")
	bindingsRecipeIDs := stringListFlag{}
	fs.Var(&bindingsRecipeIDs, "recipe", "enabled recipe id in order (repeatable, for --resolve-bindings and --resolve-tag-conflicts)")
	bindingsJSON := fs.String("bindings", "[]", "explicit manifest [[bindings]] tables as a JSON array of {capability, recipe}")
	bindingsWriteWitness := fs.Bool("write-witness", true, "persist the durable tracker binding witness after --resolve-bindings (best-effort)")
	// Tag-conflict grading is another catalog query: it reads the enabled recipes'
	// [recipe] metadata and emits one JSON envelope. Advisory only, so it never
	// changes the caller's materialization exit behavior.
	resolveTagConflictsCmd := fs.Bool("resolve-tag-conflicts", false, "grade advisory tag conflicts across enabled recipes (JSON on stdout, exit 0/2)")
	resolvePrimitiveConflictsCmd := fs.Bool("resolve-primitive-conflicts", false, "grade recipe primitive (skill/command/mcp) conflicts across enabled recipes (JSON on stdout, exit 0/2)")
	// Orphan planning is another separate command surface: pure set arithmetic
	// over a JSON envelope on stdin; it never reads the worktree or the catalog.
	planOrphansCmd := fs.Bool("plan-orphans", false, "plan materialization orphans from a JSON envelope on stdin (JSON on stdout, exit 0/2)")
	// Resolved-config projection is a manifest query: it reads one project root's
	// ai-specs.toml, projects it in Go, and emits a single JSON envelope.
	planResolvedConfigCmd := fs.Bool("plan-resolved-config", false, "project the project manifest into the resolved-config JSON envelope (exit 0/2)")
	resolvedProjectRoot := fs.String("project", "", "project root for --plan-resolved-config (default: cwd)")
	// Managed-override classification is a read-only grader: it reads the
	// destination bytes named by the stdin envelope and emits one JSON object.
	planClassifyCmd := fs.Bool("plan-classify", false, "classify a managed-override destination from a JSON envelope on stdin (JSON on stdout, exit 0/2)")
	// Config merge is another pure command surface: the Python bridge acquires
	// the already-loaded Recipe schema and sends it with the manifest config as
	// an ordered JSON envelope; Go owns the merge decision.
	planMergeConfigCmd := fs.Bool("plan-merge-config", false, "merge recipe config defaults with manifest overrides from a JSON envelope on stdin (JSON on stdout, exit 0/2)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\n")
	}

	if err := fs.Parse(args); err != nil {
		// The gate is non-destructive, so a usage error MUST NOT abort: warn on
		// stderr and fail open (exit 0). A gate that refuses to run because of a
		// launcher-flag mismatch after a partial upgrade would wedge every edit.
		//
		// Cleanup is destructive and must NOT inherit that. A version-skewed
		// binary that does not recognize a cleanup flag has to say so: exiting 0
		// would report success while deleting nothing, which the caller cannot
		// tell apart from a legitimate no-op run.
		//
		// `fs.Parse` stops at the offending flag, so *cleanup is unreliable
		// here; scan the raw arguments instead.
		for _, arg := range args {
			if arg == "--cleanup" || arg == "-cleanup" {
				fmt.Fprintf(stderr, "worktree-cleanup: %v (refusing destructive run)\n", err)
				return 2
			}
		}
		fmt.Fprintf(stderr, "worktree-gate: warning: %v (failing open)\n", err)
		return 0
	}

	switch {
	case *showVersion:
		fmt.Fprintln(stdout, version)
		return 0
	case *cleanup:
		root := processCwd()
		if resolved := git(root, "rev-parse", "--show-toplevel"); resolved != "" {
			root = RealPath(resolved)
		}
		integration := *cleanupIntegration
		if integration == "" {
			integration = *cleanupBase
		}
		cfg := newCleanupConfig(root, *cleanupDir, *cleanupBase, integration, *cleanupTopology, *cleanupDryRun, cleanupScopes.values)
		return runCleanup(root, cfg, stdout, stderr)
	case *selfTest:
		return selftest(stdout, stderr)
	case *resolveCentral:
		return resolveCentralRootRun(stdout, stderr)
	case *ledgerRun:
		return runLedger(ledgerOptions{
			checkpoint:     *ledgerCheckpoint,
			mode:           *ledgerMode,
			gateMode:       *ledgerGateMode,
			projectRoot:    *ledgerProjectRoot,
			witness:        *ledgerWitness,
			store:          *ledgerStore,
			evidence:       *ledgerEvidence,
			decide:         *ledgerDecide,
			write:          *ledgerWrite,
			reconcile:      *ledgerReconcile,
			reconcileEvent: *ledgerReconcileEvent,
		}, stdout, stderr)
	case *resolveBindingsCmd:
		return runResolveBindings(bindingsOptions{
			catalogDir:   *bindingsCatalogDir,
			recipeIDs:    bindingsRecipeIDs.values,
			bindings:     *bindingsJSON,
			projectRoot:  *ledgerProjectRoot,
			writeWitness: *bindingsWriteWitness,
		}, stdout, stderr)
	case *resolveTagConflictsCmd:
		return runResolveTagConflicts(tagConflictOptions{
			catalogDir: *bindingsCatalogDir,
			recipeIDs:  bindingsRecipeIDs.values,
		}, stdout, stderr)
	case *resolvePrimitiveConflictsCmd:
		return runResolvePrimitiveConflicts(primitiveConflictOptions{
			catalogDir: *bindingsCatalogDir,
			recipeIDs:  bindingsRecipeIDs.values,
		}, stdout, stderr)
	case *planOrphansCmd:
		return runPlanOrphans(stdin, stdout, stderr)
	case *planResolvedConfigCmd:
		root := *resolvedProjectRoot
		if root == "" {
			root = processCwd()
		}
		return runPlanResolvedConfig(root, stdout, stderr)
	case *planClassifyCmd:
		return runPlanClassify(stdin, stdout, stderr)
	case *planMergeConfigCmd:
		return runPlanMergeConfig(stdin, stdout, stderr)
	case *explain:
		return explainRun(*gateMode, *gateScope, *repoTopology, *protected, stdin, stdout, stderr)
	case *tokenize:
		return tokenizeRun(stdin, stdout)
	}

	// Resolve the event and evaluate candidates. Any malformed or incomplete
	// input remains fail-open, matching the frozen Bash reference.
	if *gateMode == "off" {
		return 0
	}
	event := ParseEvent(stdin, processCwd())
	if len(event.Candidates) == 0 {
		return 0
	}
	mode := ResolveGateMode(os.Getenv("WORKTREE_GATE_MODE"), *gateMode, stderr)
	scope := ResolveGateScope(os.Getenv("WORKTREE_GATE_SCOPE"), *gateScope, stderr)
	topology := ResolveRepoTopology(*repoTopology, stderr)
	if mode == "off" {
		return 0
	}
	protectedBranches := strings.Fields(*protected)
	for _, candidate := range event.Candidates {
		if IsInternalURI(candidate.Path, event.Mode) {
			continue
		}
		base, degrade := effectiveBase(candidate)
		if degrade {
			fmt.Fprintln(stderr, DegradeMessage(mode))
			continue
		}
		abs := candidate.Path
		if !filepath.IsAbs(candidate.Path) {
			if base != "" {
				abs = RealPath(filepath.Join(base, candidate.Path))
			}
		} else {
			abs = RealPath(candidate.Path)
		}
		if IsClaudeException(candidate.Path, abs) {
			continue
		}
		d := Decide(candidate.Path, base, scope, topology, protectedBranches)
		if !d.Allow {
			cmdCwd := base
			if cmdCwd == "" {
				cmdCwd = event.Cwd
			}
			create := shouldCreateWorktree(cmdCwd, event.Cwd)
			if mode == "ask" {
				fmt.Fprintln(stderr, AskMessage(event.Mode == "shell", event.Tool, candidate.Path, d.Branch, cmdCwd, create))
			} else {
				fmt.Fprintln(stderr, BlockMessage(event.Mode == "shell", event.Tool, candidate.Path, d.Branch, cmdCwd, create))
			}
			return 2
		}
	}
	return 0
}

// selftest checks every extraction regexp compiles and that git is invocable
// (the binary shells out to git; see design decision D7). It prints "ok" on
// stdout and exits 0, or prints the first failure on stderr and exits 1.
func selftest(stdout, stderr io.Writer) int {
	for _, pattern := range gateRegexps {
		if _, err := regexp.Compile(pattern); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: selftest: regexp compile failed: %v\n", err)
			return 1
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: selftest: git not found on PATH: %v\n", err)
		return 1
	}
	if err := exec.Command("git", "--version").Run(); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: selftest: git not invocable: %v\n", err)
		return 1
	}
	if err := ledgerSelftest(); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: selftest: ledger invariants: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "ok")
	return 0
}

// centralRootOutput is the read-only proof emitted by --resolve-central-root:
// the absolute canonical superproject root and the registered relative
// submodule path (e.g. "apps/api"). It mirrors the shell proof it replaces
// (plan-build-gate.sh resolve_central_root).
type centralRootOutput struct {
	CentralRoot string `json:"central_root"`
	Submodule   string `json:"submodule"`
}

// resolveCentralRootRun is the CLI wrapper: it emits exactly one JSON object on
// stdout when the submodule topology is proven, otherwise nothing on stdout and
// a nonzero exit (fail closed).
func resolveCentralRootRun(stdout, stderr io.Writer) int {
	root, sub, ok := resolveCentralRoot(processCwd())
	if !ok {
		fmt.Fprintln(stderr, "worktree-gate: resolve-central-root: unproven submodule topology")
		return 1
	}
	payload, err := json.Marshal(centralRootOutput{CentralRoot: root, Submodule: sub})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: resolve-central-root: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

// explainOutput is the diagnostic shape consumed by parity and doctor tooling.
type explainOutput struct {
	Mode         string   `json:"mode"`
	Tool         string   `json:"tool"`
	Cwd          string   `json:"cwd"`
	GateMode     string   `json:"gate_mode"`
	GateScope    string   `json:"gate_scope"`
	RepoTopology string   `json:"repo_topology"`
	Candidates   []string `json:"candidates"`
	CwdSource    string   `json:"cwd_source"`
	CommandCwd   string   `json:"command_cwd,omitempty"`
	Decision     string   `json:"decision"`
	Branch       string   `json:"branch"`
	Reason       string   `json:"reason"`
}

func explainRun(gateMode, gateScope, repoTopology, protected string, stdin io.Reader, stdout, stderr io.Writer) int {
	event := ParseEvent(stdin, processCwd())
	diag := explainOutput{Mode: event.Mode, Tool: event.Tool, Cwd: event.Cwd,
		GateMode: gateMode, GateScope: gateScope, RepoTopology: repoTopology,
		Candidates: candidatePaths(event.Candidates), Decision: "allow", Reason: "no-blocking-candidate"}
	if len(event.Candidates) > 0 {
		c0 := event.Candidates[0]
		diag.CwdSource = string(c0.Source)
		if c0.Source == cwdSourceCommand {
			diag.CommandCwd = c0.Base
		}
	}
	if len(event.Candidates) > 0 {
		scope := ResolveGateScope(os.Getenv("WORKTREE_GATE_SCOPE"), gateScope, stderr)
		topology := ResolveRepoTopology(repoTopology, stderr)
		protectedBranches := strings.Fields(protected)
		for _, candidate := range event.Candidates {
			base, degrade := effectiveBase(candidate)
			if degrade {
				continue
			}
			d := Decide(candidate.Path, base, scope, topology, protectedBranches)
			if !d.Allow {
				diag.Decision = "block"
				diag.Branch = d.Branch
				diag.Reason = "protected-branch"
				break
			}
		}
	}
	payload, err := json.Marshal(diag)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: explain: %v\n", err)
		return 0
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func candidatePaths(cands []WriteCandidate) []string {
	if len(cands) == 0 {
		return nil
	}
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Path
	}
	return out
}

func shouldCreateWorktree(commandCwd, sessionCwd string) bool {
	if commandCwd == "" || sessionCwd == "" {
		return true
	}
	return RealPath(commandCwd) == RealPath(sessionCwd)
}
