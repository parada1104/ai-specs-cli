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
	explain := fs.Bool("explain", false, "emit a JSON diagnostic on stdout (still exits 0/2)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain]\n")
	}

	if err := fs.Parse(args); err != nil {
		// A usage error MUST NOT abort: warn on stderr and fail open (exit 0).
		// A gate that refuses to run because of a launcher-flag mismatch after
		// a partial upgrade would wedge every edit.
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
		if IsInternalURI(candidate, event.Mode) {
			continue
		}
		if IsClaudeException(candidate, RealPath(filepath.Join(event.Cwd, candidate))) {
			continue
		}
		d := Decide(candidate, event.Cwd, scope, topology, protectedBranches)
		if !d.Allow {
			fmt.Fprintln(stderr, BlockMessage(event.Mode == "shell", event.Tool, candidate, d.Branch))
			if mode == "ask" {
				fmt.Fprintln(stderr, AskHint())
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
	fmt.Fprintln(stdout, "ok")
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
	Decision     string   `json:"decision"`
	Branch       string   `json:"branch"`
	Reason       string   `json:"reason"`
}

func explainRun(gateMode, gateScope, repoTopology, protected string, stdin io.Reader, stdout, stderr io.Writer) int {
	event := ParseEvent(stdin, processCwd())
	diag := explainOutput{Mode: event.Mode, Tool: event.Tool, Cwd: event.Cwd,
		GateMode: gateMode, GateScope: gateScope, RepoTopology: repoTopology,
		Candidates: event.Candidates, Decision: "allow", Reason: "no-blocking-candidate"}
	for _, candidate := range event.Candidates {
		d := Decide(candidate, event.Cwd, ResolveGateScope(os.Getenv("WORKTREE_GATE_SCOPE"), gateScope, stderr), ResolveRepoTopology(repoTopology, stderr), strings.Fields(protected))
		if !d.Allow {
			diag.Decision = "block"
			diag.Branch = d.Branch
			diag.Reason = "protected-branch"
			break
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
