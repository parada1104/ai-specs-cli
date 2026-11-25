package cli

// Native `ai-specs changes archive [<change>] --root <planning-root>
// [--date YYYY-MM-DD]` (plan-build-openspec-odd-align I5/P4, S10): the
// mechanical archive step both pre-merge guardians require. It refuses while the
// promotion is pending, refuses when the change already has an archive, and
// moves the change folder with a single os.Rename. In odd mode the plan stays at
// odd/tasks/<change>.md; in openspec it lives inside the change folder and moves
// with it.
//
// Native `ai-specs changes start --none --root <planning-root>`
// (plan-build-openspec-odd-align T14/S19): the local, per-branch declaration that
// the change needs no plan, written as one git config entry. Only the plan
// requirement of the openspec hook is relaxed; it never becomes an artifact.
//
// The root dispatcher has NO `archive` verb: the verb is reachable only through
// the `changes` family, and the root help announces the family alone.

import (
	"fmt"
	"io"
	"io/fs"
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
  start --none --root <planning-root>
                             Declare that this branch needs no plan
Run 'ai-specs changes archive --help' for full options.
Run 'ai-specs changes start --help' for full options.
`

const changesStartUsage = `Usage: ai-specs changes start --none --root <planning-root>

Declare that the change on the current branch needs no plan. The declaration is
one entry of this clone's local git config,
branch.<branch>.aiSpecsCeremony=none: it is never an artifact, never travels
with the branch and is never committed. The pre-tool-use hook then accepts a
production write on that branch without a plan.

Only the plan requirement of "openspec" mode is relaxed. The odd mode is not
touched, the ledger work-start checkpoint still runs, and a high-risk path on a
declared branch is still refused at pre-merge by the objective refutation.

Options:
  --none              Declare this branch ceremony-free. Required.
  --root <path>       Resolved repository root. Required.
  --help, -h          Show this help
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
folder is created holding a two-line archived.md marker (change name, ISO date),
and the original plan stays where it is.

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

// runChangesStart is the bash-free declaration verb: it resolves the branch of
// the repository at --root and writes the local no-plan entry. Re-running it is
// idempotent and prints the same confirmation.
func runChangesStart(args []string, stdout, stderr io.Writer) int {
	none := false
	root := ""
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs changes start --help' for usage.")
		return 2
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, changesStartUsage)
			return 0
		case arg == "--none":
			none = true
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
		default:
			return usageError("ERROR: unexpected positional argument: " + arg)
		}
	}

	if root == "" {
		return usageError("ERROR: missing required option: --root")
	}
	if !none {
		fmt.Fprint(stderr, changesStartUsage)
		return 2
	}
	branch := planbuild.Branch(root)
	if branch == "" {
		return usageError(fmt.Sprintf("ERROR: cannot resolve the current branch at %s "+
			"(detached HEAD or not a Git repository); run this from the branch that carries the change", root))
	}
	if err := planbuild.DeclareCeremonyNone(root, branch); err != nil {
		fmt.Fprintf(stderr, "changes start: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "changes start: declared no-plan ceremony for branch '%s' "+
		"(git config branch.%s.%s none); the declaration is local and never committed\n",
		branch, branch, planbuild.CeremonyKey)
	return 0
}

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
	dateSet := false
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
			dateSet = true
			i++
			date = args[i]
		case strings.HasPrefix(arg, "--date="):
			dateSet = true
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
	// A supplied empty date is a usage error, never "omitted" (C2): defaulting
	// it would archive to today instead of rejecting the value.
	if dateSet && date == "" {
		return usageError("ERROR: --date requires a non-empty value")
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
	// A symlink at the change path is not the change: os.Stat would call it
	// active and os.Rename would move the link (C1).
	if info, linkErr := os.Lstat(src); linkErr == nil && info.Mode()&os.ModeSymlink != 0 {
		fmt.Fprintf(stderr, "changes archive: refusing to archive %s: %s is a symlink, not a change directory; "+
			"replace it with the real directory and retry\n", change, repoRelative(root, src))
		return 1
	}
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

	// A delta-less change is still archived (F3): the dated folder holds an
	// archived.md marker naming the change and its date.
	plan := planbuild.TasksPath(root, mode, change)
	if !isActive {
		if !planbuild.PlanIsValidForMode(root, mode, change) {
			fmt.Fprintf(stderr, "changes archive: %s has no change folder under %s and no plan at %s; "+
				"nothing to archive\n", change, planbuild.ChangesBase(root, mode),
				repoRelative(root, plan))
			return 1
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			fmt.Fprintf(stderr, "changes archive: cannot create %s: %s\n", dst, err)
			return 1
		}
		marker := "# archived: " + change + "\ndate: " + date + "\n"
		if err := writeArchiveMarker(dst, marker); err != nil {
			fmt.Fprintf(stderr, "changes archive: cannot write %s: %s\n",
				filepath.Join(dst, "archived.md"), err)
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
	// A folder that held only notes renames into an archive the guardian rejects
	// as empty (C1): give it the same marker a delta-less change gets.
	if !archiveHoldsContent(dst) {
		marker := "# archived: " + change + "\ndate: " + date + "\n"
		if err := writeArchiveMarker(dst, marker); err != nil {
			fmt.Fprintf(stderr, "changes archive: cannot write %s: %s\n",
				filepath.Join(dst, "archived.md"), err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "changes archive: moved %s to %s\n", src, dst)
	return 0
}

// writeArchiveMarker writes the three-line marker a folder without a plan or a
// delta carries into its archive, so the guardian never grades it as empty.
func writeArchiveMarker(folder, marker string) error {
	return os.WriteFile(filepath.Join(folder, "archived.md"), []byte(marker), 0o644)
}

// archiveHoldsContent mirrors the guardian's emptiness rule: the plan or any
// spec delta makes the archive content-provable.
func archiveHoldsContent(folder string) bool {
	if _, err := os.Stat(filepath.Join(folder, "tasks.md")); err == nil {
		return true
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(folder, "specs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
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
