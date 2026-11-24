package cli

// Native `ai-specs changes archive [<change>] --root <planning-root>
// [--date YYYY-MM-DD]` (plan-build-openspec-odd-align I5/P4, S10): the
// mechanical archive step both pre-merge guardians require. It refuses while the
// promotion is pending, refuses when the change already has an archive, and
// moves the change folder with a single os.Rename. In odd mode the plan stays at
// odd/tasks/<change>.md; in openspec it lives inside the change folder and moves
// with it.
//
// The root dispatcher has NO `archive` verb: the verb is reachable only through
// the `changes` family, and the root help announces the family alone.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/specs"
)

// changesUsage is the family's own usage, printed on stdout for --help/-h/help
// and on stderr after the bare `changes` (same shape as the specs/gates
// families).
const changesUsage = `Usage: ai-specs changes <subcommand> [args]
Subcommands:
  archive [<change>] --root <planning-root> [--date YYYY-MM-DD] [--change <name>]
                             Move the change folder into the dated archive
Run 'ai-specs changes archive --help' for full options.
`

const changesArchiveUsage = `Usage: ai-specs changes archive [<change>] --root <planning-root> [--date YYYY-MM-DD] [--change <name>]

Move a change folder to <mode>/changes/archive/<date>-<change> with a single
os.Rename. The change resolves through the same ladder the pre-merge guardian
uses: the positional argument wins, then --change, then the branch pairing, then
the only active change. In odd mode only odd/changes/<change>/ moves and the plan
stays at odd/tasks/<change>.md; in openspec the plan lives inside the change
folder and moves with it.

The verb refuses while the promotion is pending (run 'ai-specs specs promote'
first), refuses when a dated archive for the change already exists, refuses a
destination that resolves through a symlink or outside the repository, and never
deletes anything. A change with no delta folder is still archived: the dated
folder is created holding a copy of the plan, and the original plan stays where
it is.

Arguments:
  <change>            Change override; wins over --change and the branch pairing
                      An empty <change> is a usage error, never "omitted".

Options:
  --root <path>       Resolved planning root (never the process cwd). Required.
  --date <date>       Archive date prefix, a real ISO calendar date
                      (YYYY-MM-DD). Defaults to today in UTC.
  --change <name>     Explicit change override, for CI and detached HEAD. A
                      value containing '/' keeps its last path segment, so
                      GITHUB_HEAD_REF can be passed as is.
  --help, -h          Show this help
`

// runChangesDispatch is the family remainder: help on stdout, usage on stderr
// for the bare verb, and the unknown-subcommand pair.
func runChangesDispatch(args []string, stdout, stderr io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}
	switch subcmd {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, changesUsage)
		return 0
	case "":
		fmt.Fprint(stderr, changesUsage)
		return 2
	default:
		fmt.Fprintf(stderr, "ai-specs changes: unknown subcommand '%s'\n", subcmd)
		fmt.Fprintln(stderr, "Run 'ai-specs changes --help' for usage.")
		return 2
	}
}

// runChangesArchive is the verb's own argument contract and its reporting.
func runChangesArchive(args []string, stdout, stderr io.Writer) int {
	change := ""
	positional := false
	override := ""
	changeFlag := false
	root := ""
	date := ""
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs changes archive --help' for usage.")
		return 2
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, changesArchiveUsage)
			return 0
		case arg == "--root":
			if i+1 >= len(args) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			root = args[i]
		case strings.HasPrefix(arg, "--root="):
			root = strings.TrimPrefix(arg, "--root=")
		case arg == "--date":
			if i+1 >= len(args) {
				return usageError("ERROR: --date requires a value")
			}
			i++
			date = args[i]
		case strings.HasPrefix(arg, "--date="):
			date = strings.TrimPrefix(arg, "--date=")
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
			// silently treating it as absent would archive the only active change.
			if arg == "" {
				return usageError("ERROR: empty change argument: name a change or omit the argument entirely")
			}
			positional = true
			change = arg
		}
	}
	if changeFlag && override == "" {
		return usageError("ERROR: --change requires a non-empty value")
	}

	if root == "" {
		return usageError("ERROR: missing required option: --root")
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	if !archiveDateRe.MatchString(date) {
		return usageError(fmt.Sprintf("ERROR: invalid --date value: %s (expected YYYY-MM-DD)", date))
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return usageError(fmt.Sprintf("ERROR: invalid --date value: %s (expected YYYY-MM-DD)", date))
	}

	mode := planbuild.ResolveRepo(root)
	// The positional change wins over --change, the general override.
	if change == "" {
		change = override
	}
	resolved, err := planbuild.ResolveChange(root, mode, planbuild.Branch(root), change)
	if err != nil {
		fmt.Fprintf(stderr, "changes archive: %s\n", err)
		return 1
	}
	change = resolved
	if !planbuild.ValidChangeName(change) {
		return usageError(fmt.Sprintf("ERROR: invalid change name: %s (expected a single path segment)", change))
	}

	archiveBase := planbuild.ArchiveBase(root, mode)
	dst := filepath.Join(archiveBase, date+"-"+change)
	// The archive write is contained (F1): a symlinked archive tree, or a
	// destination outside the repository, is refused before anything moves.
	if !specs.PathContained(root, dst) {
		fmt.Fprintf(stderr, "changes archive: refusing to archive %s: %s resolves through a symlink "+
			"or outside the repository; make it a real directory of this repository and retry\n",
			change, repoRelative(root, archiveBase))
		return 1
	}

	src := planbuild.ChangeFolder(root, mode, change)
	isActive := false
	if info, statErr := os.Stat(src); statErr == nil && info.IsDir() {
		isActive = true
	}
	if isActive {
		if blockers := specs.CheckFolderParityForMode(root, src, mode); len(blockers) > 0 {
			fmt.Fprintln(stderr, "changes archive: BLOCKED")
			for _, blocker := range blockers {
				fmt.Fprintf(stderr, "  - %s\n", blocker)
			}
			return 1
		}
	}
	if existing, found := datedArchiveFor(archiveBase, change); found {
		fmt.Fprintf(stderr, "changes archive: an archive for '%s' already exists at %s; "+
			"refusing to create a second one\n", change, existing)
		return 1
	}

	// A delta-less change is still archived (F3): the dated folder holds a copy
	// of the plan, and the original plan stays where it is.
	plan := planbuild.TasksPath(root, mode, change)
	if !isActive {
		if !planbuild.PlanIsValidForMode(root, mode, change) {
			fmt.Fprintf(stderr, "changes archive: %s has no change folder under %s and no plan at %s; "+
				"nothing to archive\n", change, planbuild.ChangesBase(root, mode),
				repoRelative(root, plan))
			return 1
		}
		content, readErr := os.ReadFile(plan)
		if readErr != nil {
			fmt.Fprintf(stderr, "changes archive: cannot read %s: %s\n", plan, readErr)
			return 1
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			fmt.Fprintf(stderr, "changes archive: cannot create %s: %s\n", dst, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dst, "tasks.md"), content, 0o644); err != nil {
			fmt.Fprintf(stderr, "changes archive: cannot write %s: %s\n",
				filepath.Join(dst, "tasks.md"), err)
			return 1
		}
		fmt.Fprintf(stdout, "changes archive: archived the plan of %s at %s (no delta folder)\n", change, dst)
		return 0
	}

	if err := os.MkdirAll(archiveBase, 0o755); err != nil {
		fmt.Fprintf(stderr, "changes archive: cannot create %s: %s\n", archiveBase, err)
		return 1
	}
	if err := os.Rename(src, dst); err != nil {
		fmt.Fprintf(stderr, "changes archive: cannot move %s to %s: %s\n", src, dst, err)
		return 1
	}
	fmt.Fprintf(stdout, "changes archive: moved %s to %s\n", src, dst)
	return 0
}

// repoRelative is the repository-relative path the blockers name.
func repoRelative(root, path string) string {
	if relative, err := filepath.Rel(root, path); err == nil {
		return relative
	}
	return path
}

// archiveDateRe is the exact YYYY-MM-DD shape the dated archive demands;
// time.Parse then rejects an impossible calendar date such as 2026-02-30.
var archiveDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// datedArchiveFor reports an existing archive of the change: any
// YYYY-MM-DD-<change> folder, or the undated legacy <change> folder. It mirrors
// the ambiguity resolveArchive reports rather than overwriting.
func datedArchiveFor(archiveBase, change string) (string, bool) {
	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		return "", false
	}
	dated := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-` + regexp.QuoteMeta(change) + `$`)
	for _, entry := range entries {
		name := entry.Name()
		if name == change || dated.MatchString(name) {
			if entry.IsDir() {
				return filepath.Join(archiveBase, name), true
			}
		}
	}
	return "", false
}
