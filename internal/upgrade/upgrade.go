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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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
		// The error text is git's own stderr, byte for byte (minus the final
		// newline), so runStep can replay it exactly as lib/upgrade.sh did.
		if msg := strings.TrimRight(errb.String(), "\n"); msg != "" {
			return out.String(), errors.New(msg)
		}
		return out.String(), err
	}
	return out.String(), nil
}

// Env is the resolved process environment the upgrade runs against.
type Env struct {
	Home       string // AI_SPECS_HOME
	UserHome   string // $HOME
	Executable string // the running binary path
	Git        Git
	// ReleaseBase is where the platform assets live, without a trailing slash.
	// Empty means the repository's own release downloads for the checked-out
	// version; tests and mirrors set it.
	ReleaseBase string
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
//
// FROZEN PARITY (oracle-wins): this intentionally mirrors resolve_binary()
// in lib/upgrade.sh:149-160 byte-for-byte, including the unbounded
// `while [[ -L "$source" ]]` walk. A symlink cycle hangs the legacy shell
// implementation identically; that behavior is preserved until the
// post-cutover parity amendment (Trello card [Go 12][frozen quirk]).
// Tests below pin the non-cyclic chain semantics only.
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

// Install is a detected global installation, classified by Kind.
//
// InstallKind classifies the installation layout (GO-20 U1b-2a).
type InstallKind int

const (
	// KindLegacy is the historical install: the entry is a launcher symlinked
	// into the checkout, and the running executable lives inside that home.
	KindLegacy InstallKind = iota
	// KindBinary is the post-flip install: the entry is a REAL compiled file in
	// $INSTALL_BIN, and the checkout is the data home and the trust anchor.
	KindBinary
)

// String names the kind for diagnostics and tests.
func (k InstallKind) String() string {
	if k == KindBinary {
		return "binary"
	}
	return "legacy"
}

type Install struct {
	Kind         InstallKind
	Home         string
	ExpectedHome string
	LocalBin     string
	BinaryPath   string
}

// binaryEntry reports whether the running binary IS the installed entry: a real
// executable file at LocalBin — not a symlink — that resolves to the same path
// this process runs from. That is what separates a binary install from a legacy
// one, whose entry is a symlink into the checkout.
func binaryEntry(inst Install) bool {
	fi, err := os.Lstat(inst.LocalBin)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return false
	}
	if fi.Mode().Perm()&0o111 == 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(inst.LocalBin)
	if err != nil {
		return false
	}
	// T9: the running executable may be named through a symlinked directory
	// component (a symlinked home); ResolveBinary keeps that spelling — frozen
	// parity with lib/upgrade.sh resolve_binary, — so resolve this side too, or a
	// binary install under a symlinked home never classifies as binary.
	binaryPath, err := filepath.EvalSymlinks(inst.BinaryPath)
	if err != nil {
		binaryPath = inst.BinaryPath
	}
	return filepath.Clean(resolved) == filepath.Clean(binaryPath)
}

// Detect ports the five lib/upgrade.sh install checks and classifies the
// layout. A binary install is recognised FIRST, because every one of those five
// checks assumes the legacy shape (the entry resolving inside the home as a
// symlink) and would refuse the post-flip install the installer is about to
// produce. Every legacy failure keeps its exact message and exit code.
func Detect(env Env, stderr io.Writer) (Install, int, bool) {
	inst := Install{
		Kind:         KindLegacy,
		Home:         env.Home,
		ExpectedHome: filepath.Join(env.UserHome, ".ai-specs"),
		LocalBin:     filepath.Join(env.UserHome, ".local", "bin", "ai-specs"),
		BinaryPath:   ResolveBinary(env.Executable),
	}
	if env.Home == "" {
		return inst, abort(stderr, "AI_SPECS_HOME is not set. The installation appears broken. Re-run install.sh to repair.", ExitBroken), false
	}
	if binaryEntry(inst) {
		if env.Home != inst.ExpectedHome {
			return inst, abort(stderr, fmt.Sprintf("AI_SPECS_HOME (%s) does not match the expected global path (%s). Re-run install.sh to repair.", env.Home, inst.ExpectedHome), ExitBroken), false
		}
		inst.Kind = KindBinary
		return inst, 0, true
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
	if !strings.HasPrefix(filepath.Dir(target), canonicalHome(inst.ExpectedHome)+string(os.PathSeparator)) {
		return inst, abort(stderr, "~/.local/bin/ai-specs resolves outside ~/.ai-specs. Re-run install.sh to repair.", ExitBroken), false
	}
	return inst, 0, true
}

// canonicalHome resolves only the container of ~/.ai-specs (macOS /var ->
// /private/var, D-U2e-1), so it matches the fully resolved link target. The
// final component is kept as-is: resolving it would accept a ~/.ai-specs that
// itself points outside the home.
func canonicalHome(home string) string {
	if parent, err := filepath.EvalSymlinks(filepath.Dir(home)); err == nil {
		return filepath.Join(parent, filepath.Base(home))
	}
	return home
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

// fetchRefspec names the destination explicitly so a clone without a configured
// remote.origin.fetch still advances origin/main (parity D11).
const fetchRefspec = "+refs/heads/main:refs/remotes/origin/main"

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
func runStep(o Options, git Git, dir string, stdout, stderr io.Writer, label string, args ...string) int {
	fmt.Fprintf(stdout, "  %s\n", label)
	out, err := git(dir, args...)
	if err != nil || o.Verbose {
		if out != "" {
			fmt.Fprint(stdout, out)
		}
	}
	if err != nil {
		// Git's own stderr goes to stderr, as the legacy run_step did.
		fmt.Fprintln(stderr, err)
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
//
// Two steps of lib/upgrade.sh are DELIBERATELY not reproduced here (GO-20 U1a),
// because both belong to the shell layer this program retires and neither is a
// behaviour the end state wants:
//
//   - the TUI-dependency refresh (`pip install --target lib/_vendor rich
//     questionary`, lib/upgrade.sh:330). It exists only for the still-Python
//     wizard, which the Go port replaces; the shell itself treats it as
//     best-effort ("init will prompt on first use"), so a fresh machine now
//     gets that prompt instead of a provisioned vendor directory.
//   - `lib/_internal/narrow-checkout.sh` (lib/upgrade.sh:343), the best-effort
//     pruning of subtrees the CLI never reads. It is a size cost, never a
//     behaviour cost, and it becomes meaningless once the binary — not the
//     checkout — is the installed artifact.
//
// Pinned both directions by TestUpgradeRunsNativelyAndSkipsTheShellPostSteps,
// so an accidental reintroduction fails a test instead of passing silently.
func realUpgrade(inst Install, o Options, env Env, git Git, stdout, stderr io.Writer) int {
	if code := runStep(o, git, inst.Home, stdout, stderr, "fetching origin/main", "fetch", "origin", fetchRefspec); code != 0 {
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
		if code := runStep(o, git, inst.Home, stdout, stderr, label, "merge", "--ff-only", "origin/main"); code != 0 {
			return abort(stderr, "Fast-forward merge failed. The local branch may have diverged. Resolve manually or re-run install.sh.", ExitFetch)
		}
	}
	newVersion := readVersion(inst.Home)
	if current == newVersion {
		fmt.Fprintf(stdout, "Already up to date (version %s).\n", current)
	} else {
		// JD-A-005: this route runs inside the OLD binary, so the launcher copy
		// it leaves behind is what keeps serving commands. Refresh it FIRST and
		// only then claim completion; a failed refresh reports the checkout
		// advance and the failure separately, never a success.
		if err := refreshLegacyDistCopy(inst, env, newVersion, stdout); err != nil {
			fmt.Fprintf(stdout, "Fast-forwarded the checkout: %s -> %s\n", current, newVersion)
			return abort(stderr, fmt.Sprintf("could not refresh the launcher copy %s: %v. The checkout advanced; re-run install.sh or run 'ai-specs upgrade' again once it can be replaced.", distCopyPath(inst), err), ExitPostVerify)
		}
		fmt.Fprintf(stdout, "Upgraded: %s -> %s\n", current, newVersion)
		printReleaseReport(inst.Home, current, newVersion, stdout)
	}
	if fi, err := os.Lstat(inst.LocalBin); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return abort(stderr, "Post-upgrade: ~/.local/bin/ai-specs is no longer a symlink. Re-run install.sh to repair.", ExitPostVerify)
	}
	target, err := filepath.EvalSymlinks(inst.LocalBin)
	if err != nil || !strings.HasPrefix(filepath.Dir(target), canonicalHome(inst.ExpectedHome)+string(os.PathSeparator)) {
		return abort(stderr, "Post-upgrade: ~/.local/bin/ai-specs symlink is broken or points outside ~/.ai-specs. Re-run install.sh to repair.", ExitPostVerify)
	}
	fmt.Fprintf(stdout, "Symlink integrity verified: %s -> %s\n", inst.LocalBin, target)
	return ExitOK
}

// canonicalDistHome resolves the HOME DIRECTORY a launcher copy lives under
// (D-U2e-1): a symlinked home must receive dist/ under its real directory.
func canonicalDistHome(inst Install) string {
	home := inst.ExpectedHome
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	return home
}

// distCopyPath is the launcher copy bin/ai-specs execs after the flip.
func distCopyPath(inst Install) string {
	return filepath.Join(canonicalDistHome(inst), "dist", "ai-specs")
}

// distStampPath holds the version a launcher copy was acquired for. It is the
// record the launcher compares against: mtimes are never used (JD-A-005),
// because touching VERSION must not trigger a download.
func distStampPath(inst Install) string {
	return filepath.Join(canonicalDistHome(inst), "dist", "VERSION")
}

// distCacheKey is the cheap "<size>:<mtime>:<ctime>:<inode>" key the launcher
// and install.sh also compute. It only decides whether a stamp's digest must be
// recomputed; it is never version evidence (JD-A-005). The ctime and the inode
// are what a swap cannot restore: size and mtime survive os.Chtimes, but any
// write, rename or chmod moves the ctime.
// shortcut: whole-second ctime (matches the launcher); a same-size in-place
// rewrite with mtime restored within that second is not detected. The key is a
// cache, not a security boundary (whoever can do that can rewrite the stamp).
func distCacheKey(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	ctime, ino, ok := cacheKeyIdentity(fi)
	if !ok {
		return "", fmt.Errorf("could not read the file identity of %s", path)
	}
	return fmt.Sprintf("%d:%d:%d:%d", fi.Size(), fi.ModTime().Unix(), ctime, ino), nil
}

// cacheKeyIdentity reads the change time (whole seconds) and the inode out of
// the platform's Stat_t. The field names differ (Ctimespec on Darwin, Ctim on
// Linux, Ino on both), so they are resolved by name rather than with one
// build-tagged file per platform. Either way the value matches stat(1), which
// is what install.sh and bin/ai-specs compute for the same file: keeping the
// three implementations on one format is what keeps the cache key a cache hit.
func cacheKeyIdentity(fi os.FileInfo) (ctime int64, ino uint64, ok bool) {
	v := reflect.ValueOf(fi.Sys())
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, 0, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, 0, false
	}
	// The first field that has an integer Sec, as int64.
	seconds := func(structNames ...string) (int64, bool) {
		for _, name := range structNames {
			ts := v.FieldByName(name)
			if !ts.IsValid() || ts.Kind() != reflect.Struct {
				continue
			}
			sec := ts.FieldByName("Sec")
			if !sec.IsValid() {
				continue
			}
			switch sec.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				return sec.Int(), true
			}
		}
		return 0, false
	}
	ctime, ok = seconds("Ctimespec", "Ctim")
	if !ok {
		return 0, 0, false
	}
	inode := v.FieldByName("Ino")
	switch inode.Kind() {
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		return ctime, inode.Uint(), true
	case reflect.Int, reflect.Int32, reflect.Int64:
		return ctime, uint64(inode.Int()), true
	}
	return 0, 0, false
}

// stampDistCopy records the version dist/ai-specs now holds AND the digest of
// the bytes it holds: "<version> <sha256> <size>:<mtime>:<ctime>:<inode>". The
// digest is what makes the stamp proof of the binary rather than of a string,
// so a copy replaced later is treated as absent (JD-A-005).
func stampDistCopy(inst Install, version string) error {
	dist := distCopyPath(inst)
	digest, err := Sha256File(dist)
	if err != nil {
		return fmt.Errorf("could not hash the launcher copy: %w", err)
	}
	key, err := distCacheKey(dist)
	if err != nil {
		return fmt.Errorf("could not read the launcher copy: %w", err)
	}
	if err := os.WriteFile(distStampPath(inst), []byte(version+" "+digest+" "+key+"\n"), 0o644); err != nil {
		return fmt.Errorf("could not record the launcher copy version: %w", err)
	}
	return nil
}

// distStampProves reports whether the stamp is bound to the copy on disk and
// names `version`. A stamp without a digest, or one whose digest no longer
// matches the file, proves nothing (JD-A-005).
func distStampProves(inst Install, version string) bool {
	data, err := os.ReadFile(distStampPath(inst))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	if len(fields) != 3 || fields[0] != version {
		return false
	}
	dist := distCopyPath(inst)
	if key, err := distCacheKey(dist); err == nil && key == fields[2] {
		return true
	}
	digest, err := Sha256File(dist)
	if err != nil || digest != fields[1] {
		return false
	}
	// Same bytes, new cache key: refresh it so the next run is cheap.
	return stampDistCopy(inst, version) == nil
}

// refreshLegacyDistCopy replaces a stale launcher copy with the verified
// release asset of the version the checkout now declares (JD-A-005). The
// launcher only acquires a missing copy or one whose reported version differs,
// so a stale copy would keep serving the old executable after a "successful"
// upgrade. A copy that is already stamped with the target version is left
// alone, and an absent one is left to the launcher's own self-heal.
func refreshLegacyDistCopy(inst Install, env Env, version string, stdout io.Writer) error {
	dist := distCopyPath(inst)
	if _, err := os.Lstat(dist); err != nil {
		return nil
	}
	if distStampProves(inst, version) {
		return nil
	}
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	root, err := LoadTrustRoot(filepath.Join(inst.Home, "bin", "SHA256SUMS"))
	if err != nil {
		return fmt.Errorf("could not read the committed digests: %w", err)
	}
	if _, ok := root[asset]; !ok {
		return fmt.Errorf("no committed digest for %s, so the stale copy cannot be replaced", asset)
	}
	if _, err := Acquire(AcquireOptions{
		TrustRoot: root,
		BaseURL:   releaseBaseURL(env, version),
		Asset:     asset,
		Target:    dist,
	}); err != nil {
		return err
	}
	if err := stampDistCopy(inst, version); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Refreshed launcher copy: %s\n", dist)
	return nil
}

// refreshDistCopy writes the just-installed LocalBin bytes to
// $AI_SPECS_HOME/dist/ai-specs, the path the repo launcher bin/ai-specs execs
// after the flip (GO-20 U2e). It reuses the installed file — never a rebuild —
// reports a failed write instead of swallowing it, and stamps the version it
// copied so the launcher's version check needs no download (JD-A-005).
func refreshDistCopy(inst Install) error {
	data, err := os.ReadFile(inst.LocalBin)
	if err != nil {
		return fmt.Errorf("could not read the installed binary at %s: %w", inst.LocalBin, err)
	}
	target := distCopyPath(inst)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return fmt.Errorf("could not write %s: %w", target, err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return fmt.Errorf("could not make %s executable: %w", target, err)
	}
	return stampDistCopy(inst, readVersion(inst.Home))
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
	if inst.Kind == KindBinary {
		code := binaryUpgrade(inst, o, env, stdout, stderr)
		// GO-20 U2e: a real (non-dry-run) upgrade that reached this point has
		// replaced LocalBin, so the launcher copy must be refreshed too. Every
		// non-installing outcome (dry-run, refusal) returned earlier, and a
		// symlink install never takes this branch.
		if code != ExitOK || o.DryRun {
			return code
		}
		if err := refreshDistCopy(inst); err != nil {
			return abort(stderr, fmt.Sprintf("could not refresh the launcher copy: %v", err), ExitPostVerify)
		}
		return code
	}
	git := env.git()
	if o.DryRun {
		// Fetch so the preview reports the real target (parity D10). It only
		// moves refs/remotes/origin/main; the working tree is untouched.
		if _, err := git(inst.Home, "fetch", "origin", fetchRefspec); err != nil {
			return abort(stderr, "Failed to fetch from origin. Check your network connection.", ExitFetch)
		}
	}
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
	return realUpgrade(inst, o, env, git, stdout, stderr)
}
