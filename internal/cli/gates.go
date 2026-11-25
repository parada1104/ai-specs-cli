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
	"ai-specs.dev/ai-specs/internal/planbuild"
)

// gatesUsage is the family's own usage, printed on stdout for --help/-h/help and
// on stderr after the bare `gates` (same shape as the specs/skills families).
const gatesUsage = `Usage: ai-specs gates <subcommand> [args]
Subcommands:
  premerge <slug> --root <planning-root> [--tier light|standard] [--change <name>]
                             Check the archived artifacts and staged evidence
Run 'ai-specs gates premerge --help' for full options.
`

const gatesPremergeUsage = `Usage: ai-specs gates premerge <slug> --root <planning-root> [--tier light|standard] [--stage pre-merge|pre-archive] [--change <name>]

In ` + "odd" + ` mode the change comes from the current branch (odd/tasks/<change>.md):
<slug> is accepted and ignored, the plan and the promotion parity of its deltas
are validated, and no OpenSpec archive or tier minimum is required. The mode is
resolved from the project manifest, exactly as the pre-tool-use gate resolves it.

Check the pre-merge archive/artifact gates for one change: tier minimum files,
staged verification evidence, and (Standard) canonical spec promotion parity.
Read-only: nothing is written, and canonical specs are never composed.

Arguments:
  <slug>              Change slug under openspec/changes/. Optional when
                      --change is given or the branch resolves the change.

Options:
  --root <path>       Resolved planning root (never the process cwd; a subrepo
                      request passes the proven superproject root). Required.
  --change <name>     Explicit change override, for CI and detached HEAD. A
                      positional <slug> wins over it. Without either, the change
                      is the folder paired by the branch, or the only active one.
  --tier <tier>       Planning depth: light or standard (default: infer from
                      the tasks.md Depth: line). The former full tier was
                      removed: declaring it blocks with a migration message
                      pointing at standard.
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
	positional := false
	override := ""
	changeFlag := false
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
		case arg == "--change":
			if i+1 >= len(args) {
				return usageError("ERROR: --change requires a value")
			}
			changeFlag = true
			i++
			override = args[i]
		case strings.HasPrefix(arg, "--change="):
			changeFlag = true
			override = strings.TrimPrefix(arg, "--change=")
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			if positional {
				return usageError("ERROR: unexpected positional argument: " + arg)
			}
			// An empty argument is an explicit value, not an omitted one (F2):
			// silently treating it as absent would grade the only active change.
			if arg == "" {
				return usageError("ERROR: empty change argument: name a change or omit the argument entirely")
			}
			positional = true
			slug = arg
		}
	}
	if changeFlag && override == "" {
		return usageError("ERROR: --change requires a non-empty value")
	}

	// D': the positional slug wins silently over --change, which is the general
	// override; with neither, the mode's change ladder resolves the change. The
	// missing-argument error stays exactly as it was when resolution fails.
	change := slug
	source := "positional"
	if change == "" && override != "" {
		change = override
		source = "override"
	}
	if change == "" && root != "" {
		if resolved, err := planbuild.ResolveChange(root, planbuild.ResolveRepo(root), planbuild.Branch(root), ""); err == nil {
			change = resolved
			source = "resolved"
		}
	}
	if change == "" {
		return usageError("ERROR: missing required argument: <slug>")
	}
	// The verdict prints, and the guardian grades, the normalized change: a
	// branch-shaped override (CI passes GITHUB_HEAD_REF) keeps its last segment.
	change = planbuild.NormalizeChange(change)
	if root == "" {
		return usageError("ERROR: missing required option: --root")
	}
	if tier != nil && *tier != gates.TierLight && *tier != gates.TierStandard && *tier != gates.TierFull {
		return choiceError("--tier", *tier, gates.TierLight, gates.TierStandard)
	}
	if stage != "pre-merge" && stage != "pre-archive" {
		return choiceError("--stage", stage, "pre-merge", "pre-archive")
	}

	mode := planbuild.ResolveRepo(root)
	// In odd mode a positional slug is accepted and ignored: the change comes from
	// the branch unless an override named one.
	if stage == "pre-merge" && mode == planbuild.ModeOdd && source == "positional" {
		change, _ = planbuild.ChangeFromBranch(planbuild.Branch(root))
	}
	// The graded change becomes a path inside the planning tree, so a name that is
	// not a single segment is a usage error, never a folder to grade (R5).
	if change != "" && !planbuild.ValidChangeName(change) {
		return usageError(fmt.Sprintf(
			"ERROR: invalid change name: %s (expected a single path segment)", change))
	}

	var result gates.Result
	switch {
	case stage == "pre-archive":
		result = gates.CheckPrearchive(root, change, tier)
	case mode == planbuild.ModeOdd:
		result = gates.CheckPremergeOdd(root, change)
	default:
		result = gates.CheckPremerge(root, change, tier)
	}
	if result.OK {
		if source == "positional" {
			fmt.Fprintf(stdout, "premerge-guardian: OK (%s)\n", result.Tier)
		} else {
			fmt.Fprintf(stdout, "premerge-guardian: OK (%s, change %s)\n", result.Tier, change)
		}
		return 0
	}
	fmt.Fprintln(stderr, "premerge-guardian: BLOCKED")
	for _, blocker := range result.Blockers {
		fmt.Fprintf(stderr, "  - %s\n", blocker)
	}
	return 1
}
