package cli

// Native `ai-specs tracker slug|evidence` (GO-20 U2e, unit U-C): the two
// ledger_bridge.py verbs tracker-card-gate.sh calls. Acquisition only, never a
// grader; the ## Tracker validity rule stays with the gate binary's --ledger
// predicate. The root dispatcher has NO `slug`/`evidence` verb of its own: both
// are reachable only through the `tracker` family.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"ai-specs.dev/ai-specs/internal/tracker"
)

// trackerUsage is the family's own usage, printed on stdout for --help/-h/help
// and on stderr after the bare `tracker` (same shape as the specs/gates families).
const trackerUsage = `Usage: ai-specs tracker <subcommand> [args]
Subcommands:
  slug <root>                        Print the single active change slug
  evidence <root> <slug> <outfile>   Write the ledger evidence JSON to outfile
  field <path>                       Print the value at a JSON path read from stdin
Run 'ai-specs tracker <subcommand> --help' for full options.
`

const trackerSlugUsage = `Usage: ai-specs tracker slug <root>

Print the single active change slug under <root>/openspec/changes. The archive/
directory is excluded; two or more active changes are ambiguous and print an
empty line. Read-only: nothing is created or deleted.

Arguments:
  <root>              Resolved planning root

Options:
  --help, -h          Show this help
`

const trackerEvidenceUsage = `Usage: ai-specs tracker evidence <root> <slug> <outfile>

Write the local ledger evidence JSON to <outfile>: local and remote stay empty,
code is the change's ## Tracker card_id (blank under tracker.none), and git is
that same id only when a pr: is recorded. An empty <slug> means no scope. On an
acquisition error the command exits 1 and writes nothing.

Arguments:
  <root>              Resolved planning root
  <slug>              Change slug under openspec/changes/ (may be empty)
  <outfile>           Destination file for the evidence JSON

Options:
  --help, -h          Show this help
`

const trackerFieldUsage = `Usage: ai-specs tracker field <path>

Read one JSON object from stdin and print the value at <path>. Dotted paths
descend objects (evidence.local). A string prints as is, true as 1, false as 0,
null or a missing key as an empty line, a list as its items joined with ", ",
and an object as compact JSON. Invalid JSON, or a top-level value that is not an
object, exits 1 with nothing printed.

Arguments:
  <path>              Key or dotted path to print

Options:
  --help, -h          Show this help
`

// runTrackerDispatch is the family remainder: help on stdout, usage on stderr
// for the bare verb, and the unknown-subcommand pair.
func runTrackerDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, trackerUsage)
		return 0
	case "":
		fmt.Fprint(stderr, trackerUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs tracker: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs tracker --help' for usage.")
		return 2
	}
}

func runTrackerSlug(args []string, stdout, stderr io.Writer) int {
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs tracker slug --help' for usage.")
		return 2
	}
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, trackerSlugUsage)
			return 0
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usageError("ERROR: missing required argument: <root>")
	}
	if len(positional) > 1 {
		return usageError("ERROR: unexpected positional argument: " + positional[1])
	}
	fmt.Fprintln(stdout, tracker.ChangeSlug(positional[0]))
	return 0
}

func runTrackerEvidence(args []string, stdout, stderr io.Writer) int {
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs tracker evidence --help' for usage.")
		return 2
	}
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, trackerEvidenceUsage)
			return 0
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 3 {
		return usageError("ERROR: missing required arguments: <root> <slug> <outfile>")
	}
	if len(positional) > 3 {
		return usageError("ERROR: unexpected positional argument: " + positional[3])
	}
	evidence, err := tracker.EvidencePayload(positional[0], positional[1])
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if err := os.WriteFile(positional[2], evidence.JSON(), 0o644); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	return 0
}

// runTrackerFieldVerb prints the value at a dotted path of the JSON object on
// stdin. It replaces the python3 helpers of tracker-card-gate.sh; the output
// format is the one documented in trackerFieldUsage.
func runTrackerFieldVerb(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs tracker field --help' for usage.")
		return 2
	}
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, trackerFieldUsage)
			return 0
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usageError("ERROR: missing required argument: <path>")
	}
	if len(positional) > 1 {
		return usageError("ERROR: unexpected positional argument: " + positional[1])
	}
	decoder := json.NewDecoder(stdin)
	decoder.UseNumber()
	var doc any
	if err := decoder.Decode(&doc); err != nil {
		fmt.Fprintln(stderr, "ERROR: stdin is not valid JSON")
		return 1
	}
	if _, err := decoder.Token(); err != io.EOF {
		fmt.Fprintln(stderr, "ERROR: stdin is not valid JSON")
		return 1
	}
	if _, ok := doc.(map[string]any); !ok {
		fmt.Fprintln(stderr, "ERROR: stdin JSON is not an object")
		return 1
	}
	var current any = doc
	for _, key := range strings.Split(positional[0], ".") {
		object, ok := current.(map[string]any)
		if !ok {
			current = nil
			break
		}
		value, found := object[key]
		if !found {
			current = nil
			break
		}
		current = value
	}
	fmt.Fprintln(stdout, fieldText(current))
	return 0
}

// fieldText renders one decoded JSON value as the field verb prints it.
func fieldText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case bool:
		if typed {
			return "1"
		}
		return "0"
	case string:
		return typed
	case json.Number:
		return typed.String()
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fieldText(item))
		}
		return strings.Join(parts, ", ")
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}
