package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ai-specs.dev/ai-specs/internal/rulesaudit"
)

// rulesAuditUsage is the byte-exact usage block of lib/rules-audit.sh (the
// heredoc body, trailing newline included, no trailing blank line).
const rulesAuditUsage = `Usage: ai-specs rules-audit [path] [--help]
Inventory legacy Cursor rules and emit a JSON migration inventory.
This command is read-only and never modifies any files.
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// runRulesAudit is the native `ai-specs rules-audit` dispatcher (card [Go 08]).
// The scanner lives in internal/rulesaudit; this file reproduces
// lib/rules-audit.sh's argument handling and process contract.
//
// Argument handling mirrors lib/rules-audit.sh exactly:
//   - --help/-h (before a `--`) prints usage to stdout and exits 0 at once;
//   - `--` stops parsing: every remaining argument is ignored;
//   - any other -flag is an unknown flag (two stderr lines, exit 2);
//   - the first positional is the target, a second positional exits 2;
//   - no positional means the current directory.
//
// The bash script's `-d` guard fires BEFORE any path resolution and reports the
// path exactly as given (the legacy `cd ... && pwd` resolution happens only
// after the guard, for the Python child). This port keeps that raw-path
// message byte-identical: the resolved-path guard is internal/rulesaudit's own
// (a defensive backstop that the launcher's guard already shadows).
func runRulesAudit(args []string, home string, stdout, stderr io.Writer) int {
	rest := args[1:] // drop the verb
	target := ""
	for i := 0; i < len(rest); i++ {
		switch arg := rest[i]; arg {
		case "--help", "-h":
			fmt.Fprint(stdout, rulesAuditUsage)
			return 0
		case "--":
			// Stop parsing; every remaining argument is ignored.
			i = len(rest)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
				fmt.Fprintln(stderr, "Run 'ai-specs rules-audit --help' for usage.")
				return 2
			}
			if target == "" {
				target = arg
			} else {
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
				return 2
			}
		}
	}
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: cannot determine the current directory: %v\n", err)
			return 2
		}
		target = cwd
	}

	// The -d guard precedes every resolution: report the path exactly as the
	// user gave it (the differential smoke test compares this against bash).
	if !isDirectory(target) {
		fmt.Fprintf(stderr, "ERROR: not a directory: %s\n", target)
		return 2
	}

	return rulesaudit.Run(target, home, stdout, stderr)
}
