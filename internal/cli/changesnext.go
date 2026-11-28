package cli

// `ai-specs changes next --root <repo-root> [--json]` (change-ledger-v2 T8.1,
// S5/S16).
//
// The verb is the one-step projection of `changes status`: it collects the same facts
// through the same `collectStatus`, derives the same phase and the same posture, and
// prints only what a session needs to act — the next step, the checkpoint it is
// heading to with its posture, then every blocker, every advice line and every unknown
// fact. The exit is `status`'s (0/2/3), so a caller can gate on either verb.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// changesNextUsage is the verb's own usage.
const changesNextUsage = `Usage: ai-specs changes next --root <repo-root> [--json]

Print the one next step this worktree has, the checkpoint it is heading to with its
posture, and every blocker, advice line and unknown fact behind that answer. It
composes on 'ai-specs changes status' and is read-only: it writes no ledger file, no
backup, no creation token and no declaration.

Advise is the default posture: the merge-time rules R3 (the size and module
threshold), R4 (a removed high-risk path) and R5 (a plan whose S# rows carry no
evidence) are printed as advice lines and never change the exit. A project that
declares require = ["merge"] in [recipes.plan-build-flow.config] of
ai-specs/ai-specs.toml turns them into blockers. A phase with no checkpoint (a
workstream, an integration checkout, a merge already cleaned) is subject to no
posture at all.

Exit: 0 the next step is available; 2 the facts refuse it (R1, R2, or an R3-R5 the
project required); 3 a fact the answer needs is unknown. Usage errors keep the
family's 2.

Options:
  --root <path>       Resolved repository root. Required.
  --json              Print one JSON object: phase, checkpoint, posture, next, blockers,
                      advice and unknown.
  --help, -h          Show this help
`

// changesNextArgs is the verb's parsed request.
type changesNextArgs struct {
	root   string
	asJSON bool
}

// parseChangesNext is the verb's argument contract, in the family's shape.
func parseChangesNext(args []string, stdout, stderr io.Writer) (changesNextArgs, int, bool) {
	request := changesNextArgs{}
	usageError := func(message string) (changesNextArgs, int, bool) {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs changes next --help' for usage.")
		return request, 2, true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, changesNextUsage)
			return request, 0, true
		case arg == "--root":
			if i+1 >= len(args) || optionShaped(args[i+1]) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			request.root = args[i]
		case strings.HasPrefix(arg, "--root="):
			request.root = strings.TrimPrefix(arg, "--root=")
		case arg == "--json":
			request.asJSON = true
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			return usageError("ERROR: unexpected positional argument: " + arg)
		}
	}
	if request.root == "" {
		return usageError("ERROR: missing required option: --root")
	}
	return request, 0, false
}

// changesNextReport is the `--json` shape: the phase, the posture that graded it and
// the three lists, always arrays so a host never has to tell null from empty.
type changesNextReport struct {
	Phase      string   `json:"phase"`
	Checkpoint string   `json:"checkpoint"`
	Posture    string   `json:"posture"`
	Next       string   `json:"next"`
	Blockers   []string `json:"blockers"`
	Advice     []string `json:"advice"`
	Unknown    []string `json:"unknown"`
}

// runChangesNext collects the facts once, prints the next step and returns its exit.
func runChangesNext(args []string, stdout, stderr io.Writer) int {
	request, status, stop := parseChangesNext(args, stdout, stderr)
	if stop {
		return status
	}
	report := collectStatus(request.root, "")
	report.Next = statusNext(report)
	relayStatusNotes(report, stderr, "changes next")
	if request.asJSON {
		encoded, err := json.Marshal(changesNextReport{
			Phase:      report.Phase,
			Checkpoint: report.Checkpoint,
			Posture:    report.Posture,
			Next:       report.Next,
			Blockers:   report.Blockers,
			Advice:     report.Advice,
			Unknown:    report.Unknown,
		})
		if err != nil {
			fmt.Fprintf(stderr, "changes next: cannot encode the report: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", encoded)
		return statusExitCode(report)
	}
	fmt.Fprint(stdout, nextLines(report))
	return statusExitCode(report)
}

// nextLines is the human answer: the step, the checkpoint and posture it is graded
// under, then every refusal, advice and unknown fact behind it.
func nextLines(report statusReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "next: %s\n", report.Next)
	if report.Checkpoint == "" {
		// No checkpoint means no posture governs this worktree (S16): say so instead
		// of leaving a reader to assume the default posture applied.
		fmt.Fprintf(&out, "posture: %s\n", statusPostureNone)
	} else {
		fmt.Fprintf(&out, "checkpoint: %s (posture %s)\n", report.Checkpoint, report.Posture)
	}
	for _, blocker := range report.Blockers {
		fmt.Fprintf(&out, "blocked: %s\n", blocker)
	}
	for _, advice := range report.Advice {
		fmt.Fprintf(&out, "advice: %s\n", advice)
	}
	for _, unknown := range report.Unknown {
		fmt.Fprintf(&out, "unknown: %s\n", unknown)
	}
	return out.String()
}
