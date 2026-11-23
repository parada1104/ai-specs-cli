package cli

// `ai-specs worktree-cleanup` — CLI-served post-merge cleanup (cli-served-recipes
// T3, S1/S4/S6).
//
// The retired launcher (ai-specs/recipes/worktree-flow/overrides/bin/
// worktree-cleanup.sh) and its `.git/hooks/post-merge` wrapper are replaced by
// this verb. It reads ONLY the per-project sync snapshot, then hands the pass to
// the gate runner's existing cleanup implementation, so merge proof, the
// protected-branch refusals, ledger close and remote verification are unchanged.
//
// Without a snapshot cleanup deletes NOTHING and exits 2 (fail-closed, S6): the
// project has never been synced, so no cleanup policy was ever applied.

import (
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/worktree-gate/runner"
)

// worktreeCleanupNoSnapshotMessage is the S6 fail-closed message: it names the
// project, says nothing was removed, and names the required next action.
const worktreeCleanupNoSnapshotMessage = "worktree-cleanup: %s has no ai-specs sync snapshot, " +
	"so there is no cleanup policy to apply and nothing was removed. Run `ai-specs sync` in that project first."

const worktreeCleanupUsage = `Usage: ai-specs worktree-cleanup [--dry-run] [--submodule <path> | --subrepo <path>]

Remove merged worktrees and their branches. Every cleanup input (worktrees dir,
integration branch, topology, protected branches) comes from the project's last
ai-specs sync snapshot; without one nothing is removed and the exit code is 2.

Options:
  --dry-run           Preview the removals without changing anything
  --submodule <path>  Limit the pass to one initialized submodule (repeatable)
  --subrepo <path>    Alias of --submodule
  --help, -h          Show this help
`

// runWorktreeCleanup parses the verb's flags, loads the snapshot and runs the
// cleanup pass. An unknown flag is a usage error, never a partial pass.
func runWorktreeCleanup(home string, args []string, stdout, stderr io.Writer) int {
	dryRun := false
	var scopes []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, worktreeCleanupUsage)
			return 0
		case arg == "--dry-run":
			dryRun = true
		case arg == "--submodule" || arg == "--subrepo":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "ERROR: %s requires a value\n", arg)
				fmt.Fprintln(stderr, "Run 'ai-specs worktree-cleanup --help' for usage.")
				return 2
			}
			i++
			// An option is never a scope value: `--submodule --dry-run` must not
			// clean up for real while the user asked for a preview.
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "ERROR: %s requires a value, got the option %s\n", arg, args[i])
				fmt.Fprintln(stderr, "Run 'ai-specs worktree-cleanup --help' for usage.")
				return 2
			}
			scopes = append(scopes, args[i])
		case strings.HasPrefix(arg, "--submodule=") || strings.HasPrefix(arg, "--subrepo="):
			_, value, _ := strings.Cut(arg, "=")
			if value == "" {
				fmt.Fprintf(stderr, "ERROR: %s requires a value\n", arg)
				fmt.Fprintln(stderr, "Run 'ai-specs worktree-cleanup --help' for usage.")
				return 2
			}
			scopes = append(scopes, value)
		default:
			fmt.Fprintf(stderr, "ERROR: unknown argument: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs worktree-cleanup --help' for usage.")
			return 2
		}
	}

	roots := worktreeGateRoots()
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
		fmt.Fprintf(stderr, worktreeCleanupNoSnapshotMessage+"\n", roots[0])
		return 2
	}
	return runner.RunCleanupFromPolicy(roots[0], runner.CleanupPolicy{
		WorktreesDir:      snapshot.WorktreesDir,
		IntegrationBranch: snapshot.IntegrationBranch,
		Topology:          snapshot.RepoTopology,
		Protected:         snapshot.Protected,
	}, dryRun, scopes, stdout, stderr)
}
