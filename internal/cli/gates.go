package cli

// Native `ai-specs gates premerge <slug> --root <planning-root>` (GO-20 U2e,
// unit U-B): the port of lib/_internal/premerge_guardian.py's main(). The verb
// validates tier minima, staged verification evidence and canonical/delta spec
// promotion parity; the checks themselves live in internal/gates. The guardian
// is read-only and never writes canonical specs or tracker state.
//
// The root dispatcher has NO `premerge` verb: the verb is reachable only
// through the `gates` family, and the root help announces the family alone.

import (
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/ai-specs/internal/gates"
)

// gatesUsage is the family's own usage, printed on stdout for --help/-h/help and
// on stderr after the bare `gates` (same shape as the specs/skills families).
const gatesUsage = `Usage: ai-specs gates <subcommand> [args]
Subcommands:
  premerge <slug> --root <planning-root> [--tier light|standard|full]
                             Check the archived artifacts and staged evidence
Run 'ai-specs gates premerge --help' for full options.
`

const gatesPremergeUsage = `Usage: ai-specs gates premerge <slug> --root <planning-root> [--tier light|standard|full] [--stage pre-merge|pre-archive]

Check the pre-merge archive/artifact gates for one change: tier minimum files,
staged verification evidence, and (Standard/Full) canonical spec promotion
parity. Read-only: nothing is written, and canonical specs are never composed.

Arguments:
  <slug>              Change slug under openspec/changes/

Options:
  --root <path>       Resolved planning root (never the process cwd; a subrepo
                      request passes the proven superproject root). Required.
  --tier <tier>       Planning depth: light, standard or full (default: infer
                      from the tasks.md Depth: line)
  --stage <stage>     Enforcement stage: pre-merge or pre-archive
                      (default: pre-merge)
  --help, -h          Show this help
`

// runGatesDispatch is the family remainder: help on stdout, usage on stderr for
// the bare verb, and the unknown-subcommand pair.
func runGatesDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, gatesUsage)
		return 0
	case "":
		fmt.Fprint(stderr, gatesUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs gates: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs gates --help' for usage.")
		return 2
	}
}

// runGatesPremerge is the verb's own argument contract (slug, --root, --tier,
// --stage) and the Python main()'s reporting.
func runGatesPremerge(args []string, stdout, stderr io.Writer) int {
	slug := ""
	root := ""
	var tier *string
	stage := "pre-merge"
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs gates premerge --help' for usage.")
		return 2
	}
	choiceError := func(flag, value string, choices ...string) int {
		return usageError(fmt.Sprintf("ERROR: invalid %s value: %s (expected %s)",
			flag, value, strings.Join(choices, "|")))
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, gatesPremergeUsage)
			return 0
		case arg == "--root":
			if i+1 >= len(args) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			root = args[i]
		case strings.HasPrefix(arg, "--root="):
			root = strings.TrimPrefix(arg, "--root=")
		case arg == "--tier":
			if i+1 >= len(args) {
				return usageError("ERROR: --tier requires a value")
			}
			i++
			value := args[i]
			tier = &value
		case strings.HasPrefix(arg, "--tier="):
			value := strings.TrimPrefix(arg, "--tier=")
			tier = &value
		case arg == "--stage":
			if i+1 >= len(args) {
				return usageError("ERROR: --stage requires a value")
			}
			i++
			stage = args[i]
		case strings.HasPrefix(arg, "--stage="):
			stage = strings.TrimPrefix(arg, "--stage=")
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
	if tier != nil && *tier != gates.TierLight && *tier != gates.TierStandard && *tier != gates.TierFull {
		return choiceError("--tier", *tier, gates.TierLight, gates.TierStandard, gates.TierFull)
	}
	if stage != "pre-merge" && stage != "pre-archive" {
		return choiceError("--stage", stage, "pre-merge", "pre-archive")
	}

	var result gates.Result
	if stage == "pre-archive" {
		result = gates.CheckPrearchive(root, slug, tier)
	} else {
		result = gates.CheckPremerge(root, slug, tier)
	}
	if result.OK {
		fmt.Fprintf(stdout, "premerge-guardian: OK (%s)\n", result.Tier)
		return 0
	}
	fmt.Fprintln(stderr, "premerge-guardian: BLOCKED")
	for _, blocker := range result.Blockers {
		fmt.Fprintf(stderr, "  - %s\n", blocker)
	}
	return 1
}
