package cli

// Native `ai-specs specs promote <slug> --root <planning-root>` (GO-20 U2e,
// unit U-A): the port of lib/_internal/spec_promotion.py's main(). The verb
// composes the change's delta specs into the canonical openspec/specs/ specs;
// the composition itself lives in internal/specs. Output and exit codes are the
// Python contract's, byte for byte, because the Plan Build skill and recipe
// README tell agents to run exactly this command.
//
// The root dispatcher has NO `promote` verb: the verb is reachable only through
// the `specs` family, and the root help announces the family alone.

import (
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/ai-specs/internal/specs"
)

// specsUsage is the family's own usage, printed on stdout for --help/-h/help and
// on stderr after the bare `specs` (same shape as the skills/recipe families).
const specsUsage = `Usage: ai-specs specs <subcommand> [args]
Subcommands:
  promote <slug> --root <planning-root> [--allow-removed]
                             Compose change delta specs into canonical specs
Run 'ai-specs specs promote --help' for full options.
`

const specsPromoteUsage = `Usage: ai-specs specs promote <slug> --root <planning-root> [--allow-removed]

Compose openspec/changes/<slug>/specs/<domain>/spec.md deltas into
openspec/specs/<domain>/spec.md: ADDED requirements are appended, MODIFIED
requirements replace the canonical block with the exact same name, and unrelated
canonical requirements and sections survive. A rerun after an interruption is a
no-op.

Arguments:
  <slug>              Change slug under openspec/changes/

Options:
  --root <path>       Resolved planning root (never the process cwd; a subrepo
                      request passes the proven superproject root). Required.
  --allow-removed     Execute destructive REMOVED requirements (only after the
                      removal was explicitly approved)
  --help, -h          Show this help
`

// runSpecsDispatch is the family remainder: help on stdout, usage on stderr for
// the bare verb, and the unknown-subcommand pair.
func runSpecsDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, specsUsage)
		return 0
	case "":
		fmt.Fprint(stderr, specsUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs specs: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs specs --help' for usage.")
		return 2
	}
}

// runSpecsPromote is the verb's own argument contract (slug, --root,
// --allow-removed) and the Python main()'s reporting.
func runSpecsPromote(args []string, stdout, stderr io.Writer) int {
	slug := ""
	root := ""
	allowRemoved := false
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs specs promote --help' for usage.")
		return 2
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, specsPromoteUsage)
			return 0
		case arg == "--allow-removed":
			allowRemoved = true
		case arg == "--root":
			if i+1 >= len(args) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			root = args[i]
		case strings.HasPrefix(arg, "--root="):
			root = strings.TrimPrefix(arg, "--root=")
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		case slug == "":
			slug = arg
		default:
			return usageError("ERROR: unexpected positional argument: " + arg)
		}
	}

	if slug == "" {
		return usageError("ERROR: missing required argument: <slug>")
	}
	if root == "" {
		return usageError("ERROR: missing required option: --root")
	}

	report := specs.PromoteChange(root, slug, allowRemoved)
	if len(report.Blockers) > 0 {
		fmt.Fprintln(stderr, "spec-promotion: BLOCKED")
		for _, blocker := range report.Blockers {
			fmt.Fprintf(stderr, "  - %s\n", blocker)
		}
		return 1
	}
	if pending := report.Pending(); len(pending) > 0 {
		fmt.Fprintf(stdout, "spec-promotion: OK (%d domain spec(s) composed)\n", len(report.Plans))
		for _, item := range pending {
			fmt.Fprintf(stdout, "  - %s\n", item)
		}
		return 0
	}
	if applied := report.Applied(); len(applied) > 0 {
		fmt.Fprintf(stdout, "spec-promotion: OK (already promoted: %s)\n", strings.Join(applied, ", "))
		return 0
	}
	fmt.Fprintln(stdout, "spec-promotion: OK (nothing to promote)")
	return 0
}
