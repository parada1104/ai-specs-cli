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
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/gates"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/planledger"
	"ai-specs.dev/ai-specs/internal/specs"
)

// changesUsage is the family's own usage, printed on stdout for --help/-h/help
// and on stderr after the bare `changes` (same shape as the specs/gates
// families).
const changesUsage = `Usage: ai-specs changes <subcommand> [args]
Subcommands:
  archive [<change>] --root <planning-root> [--date YYYY-MM-DD] [--change <name>]
                             Move the change folder into the dated archive
  workstream --root <repo-root> [--reason <text>] [--end] [--json]
                             Report whether this worktree is a change, or declare it a workstream
  start --none --root <planning-root>
                             Deprecated: the tier is inferred from the artifacts
Run 'ai-specs changes archive --help' for full options.
Run 'ai-specs changes workstream --help' for full options.
Run 'ai-specs changes start --help' for full options.
`

const changesStartUsage = `Usage: ai-specs changes start --none --root <planning-root>

Deprecated for one version. The change tier is inferred from the artifacts the
change already carries (no plan and no delta = none, either one = light, both =
standard), so a production write no longer needs a plan, a declaration or an
authorization. This verb records nothing the gate reads and has no effect.

A worktree that is not a change (an audit, an exploration, a spike) owes
nothing: the CLI tells the two apart from facts — product-path divergence,
committed or not yet committed, or an artifact naming a change. Run
'ai-specs changes workstream' to report it, and declare it with
'ai-specs changes workstream --reason <text>' only when the facts cannot
separate an audit from unfinished product work. The gate ignores any
pre-existing branch.<branch>.aiSpecsCeremony value.

Options:
  --none              Kept for compatibility. Required.
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

// runChangesStart is the deprecated declaration verb: it keeps the argument
// contract for one version, records nothing the gate reads, and explains the
// inferred tier and the workstream classification instead.
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
	fmt.Fprintln(stderr, changesStartDeprecation)
	return 0
}

// changesStartDeprecation is the one-line notice the retired verb prints.
const changesStartDeprecation = "changes start: deprecated — the tier is inferred from the change's " +
	"artifacts, so this verb no longer affects any gate, and it records nothing. A worktree that is " +
	"not a change is classified from facts; run 'ai-specs changes workstream --help'."

// changesWorkstreamUsage is the verb's own usage.
const changesWorkstreamUsage = `Usage: ai-specs changes workstream --root <repo-root> [--reason <text>] [--end] [--json]

Report whether this worktree is a change or a workstream, or record the explicit
decision for the worktree the facts cannot separate.

A worktree is a change when it diverges from the integration base on a product
path — committed or not yet committed — or when it carries an artifact that names
a change (a plan or a delta). Otherwise it is a workstream: an audit, an
exploration, a spike, a review. A workstream owes no plan, no tier, no tracker
item and no archive, and no refutation whose evidence is a change's artifact.

The classification is read from the same facts the write gate reads, so the two
can never disagree about what this worktree is.

Options:
  --root <path>       Resolved repository root. Required.
  --reason <text>     Declare this worktree a workstream, recording the decision in
                      the plan section of the CLI's ledger. Refused when the worktree
                      carries a plan or a delta: that is a change (S16). A text
                      starting with '-' needs the --reason=<text> form.
  --end               Close a standing declaration; the worktree returns to the
                      facts. Refused when no declaration is open.
  --json              Print one JSON object with the classification and the facts.
  --help, -h          Show this help
`

// optionShaped reports a token that is an option rather than a value, so a
// misplaced separator (`--reason --end`) is never read as the reason it names.
func optionShaped(arg string) bool { return strings.HasPrefix(arg, "-") && arg != "-" }

// changesWorkstreamArgs is the verb's parsed request.
type changesWorkstreamArgs struct {
	root      string
	reason    string
	reasonSet bool
	end       bool
	asJSON    bool
}

// parseChangesWorkstream is the verb's argument contract: the request, the exit
// status to stop at, and whether it stopped there.
func parseChangesWorkstream(args []string, stdout, stderr io.Writer) (changesWorkstreamArgs, int, bool) {
	request := changesWorkstreamArgs{}
	usageError := func(message string) (changesWorkstreamArgs, int, bool) {
		fmt.Fprintln(stderr, message)
		fmt.Fprintln(stderr, "Run 'ai-specs changes workstream --help' for usage.")
		return request, 2, true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, changesWorkstreamUsage)
			return request, 0, true
		case arg == "--root":
			if i+1 >= len(args) || optionShaped(args[i+1]) {
				return usageError("ERROR: --root requires a value")
			}
			i++
			request.root = args[i]
		case strings.HasPrefix(arg, "--root="):
			request.root = strings.TrimPrefix(arg, "--root=")
		case arg == "--reason":
			if i+1 >= len(args) || optionShaped(args[i+1]) {
				return usageError("ERROR: --reason requires a value")
			}
			request.reasonSet = true
			i++
			request.reason = args[i]
		case strings.HasPrefix(arg, "--reason="):
			request.reasonSet = true
			request.reason = strings.TrimPrefix(arg, "--reason=")
		case arg == "--end":
			request.end = true
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
	// An empty value is a supplied value, never an omitted one: defaulting it would
	// record a decision nobody wrote.
	if request.reasonSet && request.reason == "" {
		return usageError("ERROR: --reason requires a non-empty value")
	}
	if request.reasonSet && request.end {
		return usageError("ERROR: --reason and --end are opposite decisions: pass one of them")
	}
	return request, 0, false
}

// runChangesWorkstream reports the two classifications of the worktree, or records
// the append-only decision for the case the facts cannot separate (T3, S16).
func runChangesWorkstream(args []string, stdout, stderr io.Writer) int {
	request, status, stop := parseChangesWorkstream(args, stdout, stderr)
	if stop {
		return status
	}
	root := request.root

	mode := planbuild.ResolveRepo(root)
	branch := planbuild.Branch(root)
	// The pairing is the one the write gate uses, so both read the same change.
	change, _ := planbuild.ChangeFromBranch(branch)
	collect := func() gates.WriteFacts { return gates.WriteFactsFor(root, mode, change, "") }
	facts := collect()

	if request.end || request.reasonSet {
		if request.end {
			declared, _, err := planledger.Declared(root)
			if err != nil {
				where := planledger.Store(root)
				if where == "" {
					where = "a repository Git cannot name"
				}
				fmt.Fprintf(stderr, "changes workstream: cannot read the plan ledger in %s: %s\n",
					where, err)
				return 1
			}
			if !declared {
				fmt.Fprintf(stderr, "changes workstream: no workstream declaration is open for %s; "+
					"nothing to end\n", workstreamBranch(branch))
				return 1
			}
		}
		if request.reasonSet {
			if refusal := workstreamRefusal(facts, change); refusal != "" {
				fmt.Fprint(stderr, refusal)
				return 1
			}
		}
		event := planledger.EventWorkstream
		if request.end {
			event = planledger.EventWorkstreamEnd
		}
		if err := planledger.Append(root, planledger.Event{
			Event: event, At: time.Now().UTC().Format(time.RFC3339), Reason: request.reason,
		}); err != nil {
			fmt.Fprintf(stderr, "changes workstream: cannot record the decision in %s: %s\n",
				planledger.Store(root), err)
			return 1
		}
		facts = collect()
		if !request.asJSON && !request.end {
			fmt.Fprintf(stdout, "changes workstream: declared %s a workstream — recorded in %s "+
				"(reason: %s). Nothing is owed until 'changes workstream --end'.\n",
				workstreamBranch(branch), planledger.Store(root), request.reason)
		}
		if !request.asJSON && request.end {
			fmt.Fprintf(stdout, "changes workstream: ended the declaration for %s — the worktree returns "+
				"to the facts.\n", workstreamBranch(branch))
		}
	}

	view := workstreamViewOf(facts, branch, planledger.Store(root))
	if request.asJSON {
		encoded, err := json.Marshal(view)
		if err != nil {
			fmt.Fprintf(stderr, "changes workstream: cannot encode the report: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", encoded)
		return 0
	}
	fmt.Fprint(stdout, workstreamLine(view))
	return 0
}

// workstreamView is the verb's report: the classification and every fact it read.
// A workstream is reported from the same collection the gate grades, so a
// '--json' consumer sees exactly what the gate saw.
type workstreamView struct {
	Classification string   `json:"classification"`
	Mode           string   `json:"mode"`
	Branch         string   `json:"branch,omitempty"`
	Change         string   `json:"change,omitempty"`
	Base           string   `json:"base,omitempty"`
	Plan           string   `json:"plan,omitempty"`
	Delta          bool     `json:"delta"`
	Declared       bool     `json:"declared"`
	Reason         string   `json:"reason,omitempty"`
	Divergence     []string `json:"divergence"`
	Note           string   `json:"note,omitempty"`
	Ledger         string   `json:"ledger"`
}

func workstreamViewOf(facts gates.WriteFacts, branch, ledgerPath string) workstreamView {
	view := workstreamView{
		Classification: "change",
		Mode:           facts.Mode,
		Branch:         branch,
		Change:         facts.Change,
		Base:           facts.Base,
		Delta:          facts.HasDelta,
		Declared:       facts.Declared,
		Reason:         facts.DeclaredReason,
		Divergence:     workstreamDivergence(facts),
		Note:           workstreamNote(facts),
		Ledger:         ledgerPath,
	}
	if facts.HasPlan {
		view.Plan = facts.PlanPath
	}
	if facts.Workstream {
		view.Classification = "workstream"
	}
	return view
}

// workstreamDivergence is the product-path divergence actually observed: the diff
// against the base plus the untracked files, deduplicated.
func workstreamDivergence(facts gates.WriteFacts) []string {
	paths := append([]string(nil), facts.Untracked...)
	for _, entry := range facts.Diff {
		if gates.ProductPath(entry.Path) {
			paths = append(paths, entry.Path)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// workstreamNote names the fact the collection could not read, so a worktree that
// stayed graded for want of an answer is never reported as if it had none.
func workstreamNote(facts gates.WriteFacts) string {
	switch {
	case facts.Base == "" && facts.Unborn:
		return "the branch has no commit yet, so there is no diff to read and the worktree stays graded"
	case facts.Base == "":
		return "no base ref resolves in this root, so no diff could be graded and the worktree stays graded"
	case !facts.DiffKnown:
		return fmt.Sprintf("git could not answer the diff against %s, so the worktree stays graded", facts.Base)
	}
	return ""
}

// workstreamLine is the one human line of the report.
func workstreamLine(view workstreamView) string {
	switch {
	case view.Classification == "workstream" && view.Declared:
		return fmt.Sprintf("changes workstream: workstream — declared (%q): this worktree owes no plan, no "+
			"tier, no item and no archive until 'changes workstream --end'.\n", view.Reason)
	case view.Classification == "workstream":
		return "changes workstream: workstream — no plan, no delta and no product-path divergence: this " +
			"worktree owes no plan, no tier, no item and no archive (S16). Product paths changing here make " +
			"it a change.\n"
	case view.Plan != "":
		return fmt.Sprintf("changes workstream: change — the worktree carries a plan at %s.\n", view.Plan)
	case view.Delta:
		return "changes workstream: change — the worktree carries a delta, which is a change's artifact.\n"
	case len(view.Divergence) > 0:
		return fmt.Sprintf("changes workstream: change — product paths diverge from %s%s.\n",
			view.Base, workstreamPaths(view.Divergence))
	case view.Note != "":
		return fmt.Sprintf("changes workstream: change — %s.\n", view.Note)
	}
	return "changes workstream: change — HEAD is detached, so no branch pairs this worktree and every " +
		"write stays graded.\n"
}

// workstreamPaths names the observed paths, bounded so one large branch cannot
// print a page of them.
func workstreamPaths(paths []string) string {
	const shown = 5
	if len(paths) <= shown {
		return " (" + strings.Join(paths, ", ") + ")"
	}
	return fmt.Sprintf(" (%s +%d more)", strings.Join(paths[:shown], ", "), len(paths)-shown)
}

// workstreamRefusal is the reason a declaration is refused, "" when it is not:
// a plan or a delta is a change, and a worktree with no branch cannot be keyed.
func workstreamRefusal(facts gates.WriteFacts, change string) string {
	switch {
	case facts.HasPlan:
		return fmt.Sprintf("changes workstream: refusing to declare a workstream: the worktree carries a "+
			"plan at %s — a worktree with a plan or a delta is a change, and the declaration never waives its "+
			"tier, its refutations or its archive (S16).\n", facts.PlanPath)
	case facts.HasDelta:
		return fmt.Sprintf("changes workstream: refusing to declare a workstream: the worktree carries a "+
			"delta at %s — a worktree with a plan or a delta is a change, and the declaration never waives "+
			"its tier, its refutations or its archive (S16).\n", facts.DeltaPath)
	case change == "":
		return "changes workstream: refusing to declare a workstream: HEAD is detached, so no branch " +
			"pairs this worktree and no declaration can key it; every write stays graded until the worktree " +
			"is on a branch (S16).\n"
	}
	return ""
}

// workstreamBranch names the branch in the verb's own messages.
func workstreamBranch(branch string) string {
	if branch == "" {
		return "HEAD"
	}
	return branch
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
