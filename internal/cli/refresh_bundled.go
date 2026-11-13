package cli

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"ai-specs.dev/ai-specs"
)

// refreshBundledUsage is printed for -h/--help (exit 0).
const refreshBundledUsage = `Usage: ai-specs refresh-bundled [path] [--init]

Compatibility stub: catalog, templates, bundled-skills and bundled-commands
are embedded in the ai-specs binary itself. There is no bundled-asset cache
to flatten or repair; this verb is a no-op that reports where assets now
come from.

Arguments:
  path      Accepted for backward compatibility; ignored.

Flags:
  --init    Accepted for backward compatibility (init.sh invoked the legacy
            verb with it); no-op.
  -h, --help
`

// runRefreshBundled is the native refresh-bundled compatibility stub
// (card [Go 06]). The legacy flatten path (lib/_internal/refresh-bundled.py,
// still called internally by sync) is untouched; only the dispatcher-level
// verb is reduced to a no-op. Exit codes follow the frozen cross-cutting
// contract: 0 for ok/help, 2 for unknown flag or unexpected positional.
func runRefreshBundled(args []string, stdout, stderr io.Writer) int {
	rest := args[1:] // drop the verb
	positionals := 0
	for i := 0; i < len(rest); i++ {
		switch arg := rest[i]; arg {
		case "--init":
			// Accepted for backward compatibility; no-op.
		case "-h", "--help":
			fmt.Fprint(stdout, refreshBundledUsage)
			return 0
		case "--":
			positionals += len(rest) - i - 1
			i = len(rest)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
				fmt.Fprintln(stderr, "Run 'ai-specs refresh-bundled --help' for usage.")
				return 2
			}
			positionals++
			if positionals > 1 {
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
				return 2
			}
		}
	}
	fmt.Fprintln(stdout, "ai-specs refresh-bundled")
	fmt.Fprintln(stdout, "  Bundled assets are embedded in this binary; there is no cache to")
	fmt.Fprintln(stdout, "  flatten or repair. Embedded provenance:")
	for _, root := range assets.Roots() {
		n := 0
		if err := assets.Walk(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				n++
			}
			return nil
		}); err != nil {
			fmt.Fprintf(stdout, "    %-20s unavailable (%v)\n", root, err)
			continue
		}
		fmt.Fprintf(stdout, "    %-20s %d files\n", root, n)
	}
	fmt.Fprintln(stdout, "  This verb is a compatibility no-op.")
	return 0
}
