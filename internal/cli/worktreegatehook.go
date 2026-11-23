package cli

// `ai-specs worktree-gate --hook <id>` — the CLI-served worktree gate
// (cli-served-recipes T2, S2/S6).
//
// The agent hook renderers wire this verb instead of a materialized launcher.
// The verb reads ONLY the per-project sync snapshot (projectcache
// .RuntimeSnapshot), never ai-specs.toml, then hands the same stamped flags the
// retired launcher produced to the gate runner, so the decision, its messages
// and its fail-open posture are unchanged.
//
// Without a snapshot the gate allows the edit (exit 0) and prints one message
// addressed to the AGENT: the project has not been synced, protected-branch
// writes are unguarded, and the agent must ask the user — through its runtime's
// question tool — whether to run `ai-specs sync` (S6).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/worktree-gate/runner"
)

// worktreeGateHookFlag selects the CLI-served hook mode.
const worktreeGateHookFlag = "--hook"

// worktreeGateAgentEnv names the runtime the hook serves, so the fail-open
// message can ride that runtime's allow-with-message channel (S6). The
// renderers export it beside the project root; unknown or absent means silent.
const worktreeGateAgentEnv = "AI_SPECS_HOOK_AGENT"

// worktreeGateProjectRootEnv is the project root every renderer exports from its
// runtime: the hook enforces the policy of the project that INSTALLED it.
const worktreeGateProjectRootEnv = "AI_SPECS_PROJECT_ROOT"

// worktreeGateNoSnapshotMessage is the S6 fail-open message. One paragraph, it
// names the project, the consequence and the exact next action, and it asks the
// agent to put the question to the user instead of guessing.
const worktreeGateNoSnapshotMessage = "worktree-gate: %s has no ai-specs sync snapshot, " +
	"so the worktree gate is not enforced and protected-branch writes are unguarded until the project is synced. " +
	"Ask the user now, with this runtime's question tool (ask_user_choice or the equivalent), " +
	"whether to run `ai-specs sync` in that project."

// worktreeGateHookRequest reports whether args select the hook mode and returns
// the hook id it names. Only the leading `--hook <id>` pair is the hook mode;
// every other argument shape stays a plain gate invocation.
func worktreeGateHookRequest(args []string) (string, bool) {
	if len(args) < 2 || args[0] != worktreeGateHookFlag || args[1] == "" {
		return "", false
	}
	return args[1], true
}

// worktreeGateAgentMessage renders the fail-open message on the channel the
// named runtime reads while allowing the call. Claude Code injects
// hookSpecificOutput.additionalContext from a PreToolUse hook (exit 0,
// "PreToolUse decision control"; docs.claude.com/en/docs/claude-code/hooks); the
// generated Cursor wrapper and pi/omp extension forward the plain stdout line.
// Runtimes with no such channel (opencode) simply ignore it and stay silent.
// shortcut: the event name is fixed to PreToolUse because the only CLI-served
// hooks are worktree-gate's pre-tool-use ones; derive it if that widens.
func worktreeGateAgentMessage(stdout io.Writer, agent, message string) {
	switch agent {
	case "":
		return
	case "claude":
		encoded, err := json.Marshal(message)
		if err != nil {
			return
		}
		fmt.Fprintf(stdout, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":%s}}`, encoded)
	default:
		fmt.Fprintf(stdout, "%s\n", message)
	}
}

// runWorktreeGateHook answers one agent hook invocation from the snapshot.
func runWorktreeGateHook(home string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	// The payload is read once: the runner judges the event cwd it names.
	input, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: cannot read the hook payload: %v\n", err)
		return 0
	}
	roots := worktreeGateRoots()
	snapshot, found := worktreeGateSnapshot(home, roots)
	if !found {
		message := fmt.Sprintf(worktreeGateNoSnapshotMessage, roots[0])
		fmt.Fprintf(stderr, "%s\n", message)
		worktreeGateAgentMessage(stdout, os.Getenv(worktreeGateAgentEnv), message)
		return 0
	}
	// The launcher let WORKTREE_GATE_PROTECTED override its stamped value; the
	// scope and mode overrides stay the runner's (it reads its own env).
	if protected := os.Getenv("WORKTREE_GATE_PROTECTED"); protected != "" {
		snapshot.Protected = protected
	}
	return runner.Run([]string{
		"--gate-mode", snapshot.GateMode,
		"--gate-scope", snapshot.GateScope,
		"--repo-topology", snapshot.RepoTopology,
		"--protected", snapshot.Protected,
	}, bytes.NewReader(input), stdout, stderr, version)
}

// worktreeGateSnapshot reads the first snapshot present in the given roots: the
// project its runtime exported, then the main worktree. found is false when the
// project has never been synced.
func worktreeGateSnapshot(home string, roots []string) (projectcache.RuntimeSnapshot, bool) {
	for _, root := range roots {
		candidate, present, err := projectcache.ReadRuntimeSnapshot(root, home)
		if err != nil || !present {
			continue
		}
		return candidate, true
	}
	return projectcache.RuntimeSnapshot{}, false
}

// worktreeGateRoots lists the snapshot locations of the project that installed
// this hook: the root its runtime exported (or, for an older hook, the process
// cwd's git toplevel), then the main worktree of that repository, so a linked
// worktree finds what sync wrote. No ancestor walk: a nested repo keeps its own.
func worktreeGateRoots() []string {
	root := os.Getenv(worktreeGateProjectRootEnv)
	if root == "" {
		if root = worktreeGateGit(".", "rev-parse", "--show-toplevel"); root == "" {
			root, _ = os.Getwd()
		}
	}
	// The cache key resolves the project root, and the S6 message names it.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if out := worktreeGateGit(root, "worktree", "list", "--porcelain"); strings.HasPrefix(out, "worktree ") {
		main, _, _ := strings.Cut(strings.TrimPrefix(out, "worktree "), "\n")
		if main != root {
			return []string{root, main}
		}
	}
	return []string{root}
}

// worktreeGateGit runs one read-only git query and returns its trimmed output,
// or "" when git is missing or the query fails.
func worktreeGateGit(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
