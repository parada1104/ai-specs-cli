package cli

// The two internal verbs lib/init.sh reaches through AI_SPECS_GO_BIN (GO-07
// S19b).
//
// They are seams for the legacy scripts, not product surface: they are routed
// natively and deliberately absent from help.txt, and each mirrors ONE call
// site of the shell block it replaces, so the block keeps its own control flow,
// its own messages, its own fallback and its own temp-file handling. Without
// the seam the script never calls them and runs its Python route instead.

import (
	"fmt"
	"io"

	"ai-specs.dev/ai-specs/internal/sync"
)

// runBriefRenderPolicy is `brief-render-policy.py <toml_path>` in its default
// (non-validate) mode: the decision on stdout, exit 0, and `error: <…>` with
// exit 1 when the manifest cannot be read or parsed. A non-boolean
// [brief].render stays the fail-safe enabled default, never an error.
func runBriefRenderPolicy(args []string, stdout, stderr io.Writer) int {
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "Usage: ai-specs brief-render-policy <ai-specs.toml>")
		return 2
	}
	enabled, err := sync.EvaluateBriefRender(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if enabled {
		fmt.Fprintln(stdout, "true")
	} else {
		fmt.Fprintln(stdout, "false")
	}
	return 0
}

// runBaselineBrief is the chain init.sh runs, in one call:
//
//	materialize --resolved-config-out <path> && render AGENTS.md
//	    --preserve-if-runtime-brief [--adopt-brief] --resolved-config <path>
//
// The target is the positional project root; the CLI home is the resolved
// AI_SPECS_HOME the dispatcher already pinned for its children.
func runBaselineBrief(args []string, home string, stdout, stderr io.Writer) int {
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	var projectRoot, resolvedOut string
	adoptBrief := false
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--resolved-config-out":
			if i+1 >= len(rest) {
				fmt.Fprintln(stderr, "Usage: ai-specs baseline-brief <project-root> --resolved-config-out <path> [--adopt-brief]")
				return 2
			}
			i++
			resolvedOut = rest[i]
		case "--adopt-brief":
			adoptBrief = true
		default:
			if projectRoot != "" || rest[i] == "" {
				fmt.Fprintln(stderr, "Usage: ai-specs baseline-brief <project-root> --resolved-config-out <path> [--adopt-brief]")
				return 2
			}
			projectRoot = rest[i]
		}
	}
	if projectRoot == "" || resolvedOut == "" {
		fmt.Fprintln(stderr, "Usage: ai-specs baseline-brief <project-root> --resolved-config-out <path> [--adopt-brief]")
		return 2
	}
	return sync.RenderBaselineBrief(sync.BaselineBriefRequest{
		ProjectRoot:       projectRoot,
		AISpecsHome:       home,
		ResolvedConfigOut: resolvedOut,
		AdoptBrief:        adoptBrief,
		Stdout:            stdout,
		Stderr:            stderr,
	})
}
