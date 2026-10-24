package cli

// Native `ai-specs hook field|normalize-path` (GO-20 U2e, unit U-F2a): the JSON
// helpers plan-build-gate.sh calls instead of python3. Fail-open by contract:
// normalize-path prints nothing and exits 0 whenever the python original exited
// 0 without output; only usage errors exit 2.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ai-specs.dev/ai-specs/internal/hook"
)

const hookUsage = `Usage: ai-specs hook <subcommand> [args]

Subcommands:
  field [--string] <path>   Print the value at <path> of the JSON object on stdin
  normalize-path            Print the canonical target of a hook event on stdin

Run 'ai-specs hook <subcommand> --help' for full options.
`

const hookFieldUsage = `Usage: ai-specs hook field [--string] <path>

Read one JSON object from stdin and print the value at <path>, with the same
text rules as 'ai-specs tracker field'. With --string the value must be a JSON
string: any other value, or a missing key, exits 1 with nothing on stdout.

Arguments:
  <path>              Key or dotted path to print

Options:
  --string            Refuse any value that is not a JSON string
  --help, -h          Show this help
`

const hookNormalizePathUsage = `Usage: ai-specs hook normalize-path

Read one hook event (JSON) on stdin and print "<tool>\t<target>\t<probe>":
the canonical write target (symlinks resolved, missing tail kept) and the
canonical nearest existing directory. Prints nothing and exits 0 when the event
has no string file path or no existing ancestor, or is not valid JSON.

Options:
  --help, -h          Show this help
`

func runHookDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, hookUsage)
		return 0
	case "":
		fmt.Fprint(stderr, hookUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs hook: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs hook --help' for usage.")
		return 2
	}
}

func runHookField(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runJSONField(args, stdin, stdout, stderr, "ai-specs hook field", hookFieldUsage, true)
}

func runHookNormalizePath(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			fmt.Fprint(stdout, hookNormalizePathUsage)
			return 0
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintln(stderr, "ERROR: unknown flag: "+arg)
			fmt.Fprintln(stderr, "Run 'ai-specs hook normalize-path --help' for usage.")
			return 2
		}
		fmt.Fprintln(stderr, "ERROR: unexpected positional argument: "+arg)
		fmt.Fprintln(stderr, "Run 'ai-specs hook normalize-path --help' for usage.")
		return 2
	}
	event, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	target, ok := hook.NormalizeEvent(event, cwd)
	if ok {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", target.Tool, target.Path, target.Probe)
	}
	return 0
}
