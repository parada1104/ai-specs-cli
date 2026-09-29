// Package upgrade ports lib/upgrade.sh: flag parsing, install detection, the
// dirty-tree guard, the --dry-run preview, and the legacy git fast-forward
// upgrade with its changelog release report.
//
// The single-binary self-replacement engine is separate (card [Go 12]); this
// file is the byte-faithful legacy surface. Exit codes and abort messages
// reproduce lib/upgrade.sh (FROZEN).
//
// Deliberate deviation from recorded defect D9: the mode-only-dirt
// remediation is skipped under --dry-run, so --dry-run writes nothing
// (card acceptance). D9 is a defect, not FROZEN.
package upgrade

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/changelog"
)

// Exit codes (FROZEN, parity contract section 2).
const (
	ExitOK          = 0
	ExitBroken      = 1
	ExitNotStandard = 2
	ExitPreflight   = 3
	ExitFetch       = 4
	ExitPostVerify  = 5
)

// Usage is the byte-exact lib/upgrade.sh usage heredoc.
const Usage = `Usage: ai-specs upgrade [--dry-run] [--force] [-v|--verbose]

Safely upgrade the global ai-specs installation to the latest origin/main.

Flags:
  --dry-run       Show what would change without modifying the repository.
  --force         Proceed even if the working tree has uncommitted changes.
  -v, --verbose   Show the full git output instead of one line per step.
  -h, --help      Show this help.

Exit codes:
  0   Success or dry-run completed.
  1   Broken or missing installation.
  2   Dev / non-standard checkout, or invalid usage (unknown argument).
  3   Pre-flight check failed (dirty tree, non-fast-forward, etc.).
  4   Git fetch or merge failed.
  5   Post-upgrade verification failed (symlink broken).
`

// Options holds the parsed upgrade flags.
type Options struct {
	DryRun  bool
	Force   bool
	Verbose bool
}

// Git runs a git command in dir and returns its stdout.
type Git func(dir string, args ...string) (string, error)

// GitCommand is the default runner.
func GitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Env is the resolved process environment the upgrade runs against.
type Env struct {
	Home       string // AI_SPECS_HOME
	UserHome   string // $HOME
	Executable string // the running binary path
	Git        Git
}

func (e Env) git() Git {
	if e.Git != nil {
		return e.Git
	}
	return GitCommand
}

// ParseArgs mirrors the lib/upgrade.sh parse loop. done=true means the caller
// must exit with code (help or usage error already printed).
func ParseArgs(args []string, stdout, stderr io.Writer) (Options, int, bool) {
	var o Options
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			o.DryRun = true
		case "--force":
			o.Force = true
		case "-v", "--verbose":
			o.Verbose = true
		case "-h", "--help":
			fmt.Fprint(stdout, Usage)
			return o, ExitOK, true
		default:
			fmt.Fprintf(stderr, "Unknown argument: %s\n", arg)
			fmt.Fprint(stderr, Usage)
			return o, ExitNotStandard, true
		}
	}
	return o, 0, false
}

// abort mirrors lib/upgrade.sh abort: "ai-specs upgrade: <msg>" on stderr and
// the given exit code.
func abort(stderr io.Writer, msg string, code int) int {
	fmt.Fprintf(stderr, "ai-specs upgrade: %s\n", msg)
	return code
}

// ResolveBinary walks symlinks from start and returns the absolute real path.
func ResolveBinary(start string) string {
	source := start
	for {
		fi, err := os.Lstat(source)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		target, err := os.Readlink(source)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(source), target)
		}
		source = target
	}
	dir, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		dir = filepath.Dir(source)
	}
	return filepath.Join(dir, filepath.Base(source))
}

// Install is a detected global installation.
type Install struct {
	Home         string
	ExpectedHome string
	LocalBin     string
	BinaryPath   string
}

// Detect ports the five lib/upgrade.sh install checks.
func Detect(env Env, stderr io.Writer) (Install, int, bool) {
	inst := Install{
		Home:         env.Home,
		ExpectedHome: filepath.Join(env.UserHome, ".ai-specs"),
		LocalBin:     filepath.Join(env.UserHome, ".local", "bin", "ai-specs"),
		BinaryPath:   ResolveBinary(env.Executable),
	}
	if env.Home == "" {
		return inst, abort(stderr, "AI_SPECS_HOME is not set. The installation appears broken. Re-run install.sh to repair.", ExitBroken), false
	}
	if !strings.HasPrefix(inst.BinaryPath, inst.ExpectedHome+string(os.PathSeparator)) {
		return inst, abort(stderr, fmt.Sprintf("This checkout is not the standard global installation (resolved path: %s). Use 'git pull' manually in the correct directory.", inst.BinaryPath), ExitNotStandard), false
	}
	if env.Home != inst.ExpectedHome {
		return inst, abort(stderr, fmt.Sprintf("AI_SPECS_HOME (%s) does not match the expected global path (%s). Re-run install.sh to repair.", env.Home, inst.ExpectedHome), ExitBroken), false
	}
	if fi, err := os.Stat(filepath.Join(env.Home, ".git")); err != nil || !fi.IsDir() {
		return inst, abort(stderr, fmt.Sprintf("The installation at %s is missing its .git directory. Re-run install.sh to repair.", env.Home), ExitBroken), false
	}
	if fi, err := os.Lstat(inst.LocalBin); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return inst, abort(stderr, "~/.local/bin/ai-specs is missing or not a symlink. Re-run install.sh to repair.", ExitBroken), false
	}
	target, err := filepath.EvalSymlinks(inst.LocalBin)
	if err != nil || target == "" {
		return inst, abort(stderr, "~/.local/bin/ai-specs symlink appears broken. Re-run install.sh to repair.", ExitBroken), false
	}
	if !strings.HasPrefix(filepath.Dir(target), inst.ExpectedHome+string(os.PathSeparator)) {
		return inst, abort(stderr, "~/.local/bin/ai-specs resolves outside ~/.ai-specs. Re-run install.sh to repair.", ExitBroken), false
	}
	return inst, 0, true
}

// checkTree ports the working-tree cleanliness check. It returns (code, ok);
// when ok is false the caller must return code.
func checkTree(inst Install, o Options, stderr io.Writer, git Git) (int, bool) {
	dirty, _ := git(inst.Home, "status", "--porcelain")
	if strings.TrimSpace(dirty) == "" {
		return 0, true
	}
	modeOnly, err := git(inst.Home, "-c", "core.fileMode=false", "status", "--porcelain")
	if err == nil && strings.TrimSpace(modeOnly) == "" {
		if o.DryRun {
			return 0, true // D9 corrected: dry-run writes nothing
		}
		fmt.Fprintln(stderr, "Restoring file modes altered by a previous installer...")
		_, _ = git(inst.Home, "checkout", "--", ".")
		return 0, true
	}
	if !o.Force {
		return abort(stderr, "Working tree is dirty. Stash changes, clean the tree, or use --force.\n"+strings.TrimRight(dirty, "\n"), ExitPreflight), false
	}
	fmt.Fprintln(stderr, "Warning: working tree is dirty. Proceeding because --force was given.")
	return 0, true
}

func readVersion(home string) string {
	data, err := os.ReadFile(filepath.Join(home, "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func gitShowVersion(git Git, home, ref string) string {
	out, err := git(home, "show", ref+":VERSION")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// dryRunReport ports the --dry-run branch.
func dryRunReport(inst Install, o Options, git Git, stdout, stderr io.Writer) int {
	current := readVersion(inst.Home)
	target := "unknown"
	hasOrigin := false
	if _, err := git(inst.Home, "rev-parse", "--verify", "origin/main"); err == nil {
		hasOrigin = true
		if v := gitShowVersion(git, inst.Home, "origin/main"); v != "" {
			target = v
		}
	}
	if !hasOrigin {
		return abort(stderr, "origin/main is not available locally. A real upgrade would fetch it first.", ExitPreflight)
	}
	upToDate := false
	head, e1 := git(inst.Home, "rev-parse", "HEAD")
	origin, e2 := git(inst.Home, "rev-parse", "origin/main")
	if e1 == nil && e2 == nil && strings.TrimSpace(head) == strings.TrimSpace(origin) {
		upToDate = true
	}
	fmt.Fprintln(stdout, "Dry-run: no changes will be made.")
	fmt.Fprintf(stdout, "Current version: %s\n", current)
	fmt.Fprintf(stdout, "Target version:  %s\n", target)
	if upToDate {
		fmt.Fprintln(stdout, "Already up to date.")
	}
	return ExitOK
}

// runStep prints one progress line and runs the git command, surfacing its
// output when it fails or when --verbose is set.
func runStep(o Options, git Git, dir string, stdout io.Writer, label string, args ...string) int {
	fmt.Fprintf(stdout, "  %s\n", label)
	out, err := git(dir, args...)
	if err != nil || o.Verbose {
		if out != "" {
			fmt.Fprint(stdout, out)
		}
	}
	if err != nil {
		return 1
	}
	return 0
}

// printReleaseReport replays the crossed versions and their upgrade notices.
// Best-effort: a missing or malformed changelog degrades to the plain upgrade
// line (the fast-forward has already landed).
func printReleaseReport(home, current, new string, stdout io.Writer) {
	data, err := os.ReadFile(filepath.Join(home, "CHANGELOG.md"))
	if err != nil {
		return
	}
	text := string(data)
	selected := changelog.SelectRange(changelog.ParseSections(text), current, new)
	had := false
	if len(selected) > 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "What changed")
		changelog.EmitSummary(stdout, selected, 3)
		had = true
	}
	notices := changelog.CrossedNotices(text, current, new)
	if len(notices) > 0 {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Action required")
		changelog.EmitNotices(stdout, notices)
		had = true
	}
	if had {
		fmt.Fprintln(stdout, "")
	}
}

// realUpgrade ports the fetch + fast-forward + release-report path.
func realUpgrade(inst Install, o Options, git Git, stdout, stderr io.Writer) int {
	if code := runStep(o, git, inst.Home, stdout, "fetching origin/main", "fetch", "origin", "main"); code != 0 {
		return abort(stderr, "Failed to fetch from origin. Check your network connection.", ExitFetch)
	}
	if _, err := git(inst.Home, "merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
		return abort(stderr, "Local branch has diverged from origin/main after fetch. Resolve manually or re-run install.sh.", ExitPreflight)
	}
	head, _ := git(inst.Home, "rev-parse", "HEAD")
	origin, _ := git(inst.Home, "rev-parse", "origin/main")
	current := readVersion(inst.Home)
	if strings.TrimSpace(head) != strings.TrimSpace(origin) {
		target := gitShowVersion(git, inst.Home, "origin/main")
		label := "fast-forwarding to origin/main"
		if current != "" && target != "" {
			label = fmt.Sprintf("fast-forwarding %s -> %s", current, target)
		}
		if code := runStep(o, git, inst.Home, stdout, label, "merge", "--ff-only", "origin/main"); code != 0 {
			return abort(stderr, "Fast-forward merge failed. The local branch may have diverged. Resolve manually or re-run install.sh.", ExitFetch)
		}
	}
	newVersion := readVersion(inst.Home)
	if current == newVersion {
		fmt.Fprintf(stdout, "Already up to date (version %s).\n", current)
	} else {
		fmt.Fprintf(stdout, "Upgraded: %s -> %s\n", current, newVersion)
		printReleaseReport(inst.Home, current, newVersion, stdout)
	}
	if fi, err := os.Lstat(inst.LocalBin); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return abort(stderr, "Post-upgrade: ~/.local/bin/ai-specs is no longer a symlink. Re-run install.sh to repair.", ExitPostVerify)
	}
	target, err := filepath.EvalSymlinks(inst.LocalBin)
	if err != nil || !strings.HasPrefix(filepath.Dir(target), inst.ExpectedHome+string(os.PathSeparator)) {
		return abort(stderr, "Post-upgrade: ~/.local/bin/ai-specs symlink is broken or points outside ~/.ai-specs. Re-run install.sh to repair.", ExitPostVerify)
	}
	fmt.Fprintf(stdout, "Symlink integrity verified: %s -> %s\n", inst.LocalBin, target)
	return ExitOK
}

// Run executes the upgrade and returns the process exit code.
func Run(args []string, env Env, stdout, stderr io.Writer) int {
	o, code, done := ParseArgs(args, stdout, stderr)
	if done {
		return code
	}
	inst, code, ok := Detect(env, stderr)
	if !ok {
		return code
	}
	git := env.git()
	if _, err := git(inst.Home, "rev-parse", "--verify", "origin/main"); err == nil {
		if _, err := git(inst.Home, "merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
			return abort(stderr, "Local branch has diverged from origin/main. Resolve manually or re-run install.sh.", ExitPreflight)
		}
	} else if o.DryRun {
		return abort(stderr, "origin/main is not available locally. A real upgrade would fetch it first.", ExitPreflight)
	}
	if code, ok := checkTree(inst, o, stderr, git); !ok {
		return code
	}
	if o.DryRun {
		return dryRunReport(inst, o, git, stdout, stderr)
	}
	return realUpgrade(inst, o, git, stdout, stderr)
}
