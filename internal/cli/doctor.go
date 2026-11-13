package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ai-specs.dev/ai-specs/internal/doctor"
)

// doctorUsage is the byte-exact usage block of lib/doctor.sh (the heredoc body,
// trailing newline included, no trailing blank line).
const doctorUsage = `Usage: ai-specs doctor [path] [--help]
Diagnose whether an ai-specs project is correctly initialized and in a
consistent state. Never modifies project files; runs external version
checks (recipe deps, gate binary selftest).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// runDoctor is the native `ai-specs doctor` dispatcher (card [Go 08]). The
// command, its roster and every decision live in internal/doctor; this file
// only reproduces lib/doctor.sh's argument handling and process contract.
//
// Argument handling mirrors lib/doctor.sh exactly:
//   - --help/-h (before a `--`) prints usage to stdout and exits 0 at once;
//   - `--` stops parsing: every remaining argument is ignored;
//   - any other -flag is an unknown flag (two stderr lines, exit 2);
//   - the first positional is the target, a second positional exits 2;
//   - no positional means the current directory.
//
// Documented deviation (deliberate; the frozen exit code is preserved): the
// legacy launcher resolved the target with `cd "$TARGET_PATH" && pwd`, so a
// nonexistent path died inside bash, with bash's own diagnostic
// (`lib/doctor.sh: line 46: cd: ...`, exit 1) and doctor.py's own
// "is not a directory" guard left as dead code. This port skips the bash
// layer and reports the message the Python guard intended, keeping the frozen
// exit code 1 instead of fabricating a bash line number.
func runDoctor(args []string, home string, stdout, stderr io.Writer) int {
	rest := args[1:] // drop the verb
	target := ""
	for i := 0; i < len(rest); i++ {
		switch arg := rest[i]; arg {
		case "--help", "-h":
			fmt.Fprint(stdout, doctorUsage)
			return 0
		case "--":
			// Stop parsing; every remaining argument is ignored.
			i = len(rest)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
				fmt.Fprintln(stderr, "Run 'ai-specs doctor --help' for usage.")
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

	d := doctor.New(target, home)
	d.Stderr = stderr
	if !isDirectory(d.Root) {
		fmt.Fprintf(stderr, "ERROR: %s is not a directory.\n", d.Root)
		return 1
	}
	code := d.Run()
	d.Report(stdout)
	return code
}

// isDirectory mirrors doctor.py's `root.is_dir()` guard for the native port.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
