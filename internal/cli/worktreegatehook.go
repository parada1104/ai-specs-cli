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

// runWorktreeGateHook answers one agent hook invocation from the snapshot.
func runWorktreeGateHook(home string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	// The payload is read once: the event's cwd selects the project (the hook
	// may be launched from anywhere) and the runner gets the same bytes.
	input, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: cannot read the hook payload: %v\n", err)
		return 0
	}
	roots := worktreeGateRoots(worktreeGateEventCwd(input))
	var snapshot projectcache.RuntimeSnapshot
	found := false
	for _, root := range roots {
		candidate, present, err := projectcache.ReadRuntimeSnapshot(root, home)
		if err != nil || !present {
			continue
		}
		snapshot, found = candidate, true
		break
	}
	if !found {
		fmt.Fprintf(stderr, worktreeGateNoSnapshotMessage+"\n", roots[0])
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

// worktreeGateEventCwd is the cwd the hook payload names — the directory the
// gate runner judges — or the process cwd when the payload carries none.
func worktreeGateEventCwd(input []byte) string {
	var event struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal(input, &event) == nil && event.Cwd != "" {
		return event.Cwd
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		return cwd
	}
	return "."
}

// worktreeGateRoots lists the candidate project roots for the snapshot lookup,
// most specific first: the event cwd and every one of its ancestors up to the
// filesystem root (ai-specs.toml may sit below the git toplevel, and a submodule
// resolves to the superproject), then the git toplevel (a linked worktree
// included) and the repository's main worktree, where sync usually ran. The
// first root carrying a snapshot wins, so working inside a worktree never
// reports a missing snapshot.
func worktreeGateRoots(cwd string) []string {
	// The cache key resolves the project root, and the S6 message names it, so
	// start from the real path.
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	var roots []string
	for dir := cwd; ; {
		roots = append(roots, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if top := worktreeGateGit(cwd, "rev-parse", "--show-toplevel"); top != "" {
		roots = append(roots, top)
	}
	if main := worktreeGateGit(cwd, "worktree", "list", "--porcelain"); main != "" {
		for _, line := range strings.Split(main, "\n") {
			if path, ok := strings.CutPrefix(line, "worktree "); ok && path != "" {
				roots = append(roots, path)
				break
			}
		}
	}
	return roots
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
