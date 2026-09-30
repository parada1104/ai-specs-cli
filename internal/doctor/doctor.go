// Package doctor is the native Go port of `ai-specs doctor`
// (lib/_internal/doctor.py + lib/doctor.sh) for the Go single-binary migration
// epic, card [Go 08].
//
// Porting contract (docs/go-migration-parity-contract.md): the check roster, its
// ORDER, every severity, every message and guidance string, the report
// formatting and the exit-code rule are FROZEN and reproduced byte-identically.
// `doctor` writes nothing.
//
// Strangler seam: the command, the roster and every decision live here. The
// dependency stacks that are not ported yet (recipe-materialize/agents-render
// for the brief provenance chain) are reached through one explicit Go->Python
// bridge in bridge.go. Every check that only needs already-ported layers is
// evaluated natively.
package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Severity mirrors doctor.py's Severity enum, including declaration order
// (which the summary counts rely on).
type Severity int

// Severities in doctor.py enum declaration order.
const (
	OK Severity = iota
	INFO
	WARN
	ERROR
)

// String returns the enum value name ("OK", "INFO", "WARN", "ERROR").
func (s Severity) String() string {
	switch s {
	case OK:
		return "OK"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	}
	return "ERROR"
}

// Check mirrors doctor.py's Check dataclass.
type Check struct {
	Severity Severity
	Name     string
	Message  string
	Guidance string
}

// Render mirrors Check.render(): severity left-justified in 5 columns, name in
// 15, then the message; non-empty guidance is appended in parentheses.
//
// Python pads by characters; Go's %-15s pads by bytes. Every check name in
// doctor.py is ASCII, so the two agree.
func (c Check) Render() string {
	base := fmt.Sprintf("%-5s  %-15s  %s", c.Severity, c.Name, c.Message)
	if c.Guidance != "" {
		base += "  (" + c.Guidance + ")"
	}
	return base
}

// Doctor mirrors doctor.py's Doctor class: one target root and the ordered
// checks accumulated by Run.
type Doctor struct {
	// Root is the resolved target project root (Path(root).resolve()).
	Root string
	// Home is the CLI install root ($AI_SPECS_HOME, resolved).
	Home string
	// Stderr receives the diagnostics the legacy implementation writes out of
	// band (the Python bridge fallback announcements).
	Stderr io.Writer

	// Checks holds the emitted checks in run order.
	Checks []Check

	manifest     manifestCache
	projectCache projectCache

	bridge    *bridgeResponse
	bridgeErr error
}

// New returns a Doctor for the given project root and CLI install root. Both
// are resolved exactly like the Python Path(...).resolve() calls they mirror.
func New(root, home string) *Doctor {
	return &Doctor{Root: resolvePy(root), Home: resolvePy(home), Stderr: os.Stderr}
}

// Run executes every check in doctor.py's roster order and returns the process
// exit code. WARN and INFO never affect the exit code.
func (d *Doctor) Run() int {
	d.bridge, d.bridgeErr = d.runBridge()

	d.checkManifest()
	d.checkCLIVersion()
	d.checkLegacyRecipeVersions()
	d.checkAgentsMD()
	d.checkBriefRenderPolicy()
	d.checkBriefProvenance()
	d.checkBundledAssets()
	d.checkTrackedBundledLeftovers()
	d.checkEnabledAgents()
	d.checkRecipeCLIDeps()
	d.checkTrackerLedger()
	d.checkHarnessEnvLayout()
	d.checkWorktreeGate()
	d.checkRepoTopology()
	d.checkStaleTemplateOverrides()
	d.checkGateProvenance()

	// Fail closed: when the bridge cannot run at all, the checks that depend on
	// it would silently degrade to "not performed" -- the invisible fail-open
	// posture the worktree-gate check exists to expose elsewhere. One ERROR
	// makes the degraded run visible instead.
	if d.bridgeErr != nil {
		d.add(ERROR, "doctor-bridge",
			fmt.Sprintf("the legacy analysis bridge could not run: %v", d.bridgeErr),
			"run ai-specs doctor with python3 on PATH, or fix the ai-specs installation")
	}

	return d.exitCode()
}

// exitCode is the frozen rule: 1 when at least one ERROR check was emitted.
func (d *Doctor) exitCode() int {
	for _, c := range d.Checks {
		if c.Severity == ERROR {
			return 1
		}
	}
	return 0
}

// Report writes the frozen report skeleton to w.
func (d *Doctor) Report(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ai-specs doctor")
	fmt.Fprintf(w, "  target: %s\n", d.Root)
	fmt.Fprintln(w)
	for _, c := range d.Checks {
		fmt.Fprintf(w, "  %s\n", c.Render())
	}
	fmt.Fprintln(w)
	ok, info, warn, errCount := 0, 0, 0, 0
	for _, c := range d.Checks {
		switch c.Severity {
		case OK:
			ok++
		case INFO:
			info++
		case WARN:
			warn++
		case ERROR:
			errCount++
		}
	}
	fmt.Fprintf(w, "Summary: %d OK, %d INFO, %d WARN, %d ERROR\n", ok, info, warn, errCount)
}

// add appends one check.
func (d *Doctor) add(sev Severity, name, message string, guidance ...string) {
	c := Check{Severity: sev, Name: name, Message: message}
	if len(guidance) > 0 {
		c.Guidance = guidance[0]
	}
	d.Checks = append(d.Checks, c)
}

// --- path helpers mirroring Python semantics -------------------------------

// resolvePy mirrors Path(p).resolve() with strict=False: absolute, symlinks
// resolved as far as possible, and a dangling symlink resolving to its target
// path (Python does not require the target to exist).
func resolvePy(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	// Fast path: nothing dangling, so the stdlib resolves it.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return resolveNonStrict(abs, 0)
}

// resolveNonStrict resolves every component of an absolute POSIX path,
// following symlinks even when their target does not exist. linkBudget mirrors
// the kernel's 40-link bound so a symlink loop terminates.
func resolveNonStrict(path string, links int) string {
	if links > 40 {
		return filepath.Clean(path)
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, "/") {
		return clean
	}
	comps := strings.Split(clean[1:], "/")
	current := "/"
	for i, comp := range comps {
		if comp == "" {
			continue
		}
		if comp == ".." {
			current = filepath.Dir(current)
			continue
		}
		candidate := filepath.Join(current, comp)
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			current = candidate
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(current, target)
		}
		remainder := append([]string{target}, comps[i+1:]...)
		return resolveNonStrict(filepath.Join(remainder...), links+1)
	}
	return current
}

// isFile mirrors Path.is_file(): a regular file, following symlinks.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// isDir mirrors Path.is_dir().
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// exists mirrors Path.exists(): false only on a "not found" stat error.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// relTo mirrors Path.relative_to(root) for a path known to be under root.
func relTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
