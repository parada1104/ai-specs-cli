// Package hub is the native port of the `hub` verb — the interactive front door
// that bare `ai-specs` runs — starting with its non-interactive half (GO-20 U3a).
//
// The verb has two halves and this package owns the dependency-free one:
//
//   - the mode decision (`lib/_internal/hub.py`'s `decide_mode`) and the
//     non-interactive status summary, whose bytes and exit code are frozen;
//   - the flag contract and the anti-hang guard of `lib/hub.sh`;
//   - the delegation argv shape.
//
// The INTERACTIVE half is bridged to `lib/_internal/hub.py` until U3b, which is
// where the Go TUI and its dependency land, together with the three amendments
// that dependency requires (D8, the `assets.go` zero-dependency sentence and the
// parity contract's FREE entry for rich/questionary rendering). Bridging is what
// keeps the interactive UX — and the PTY suites that drive it — unchanged while
// that happens, exactly as the doctor bridges what it still needs.
package hub

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/doctor"
	"ai-specs.dev/ai-specs/internal/target"
	"ai-specs.dev/ai-specs/internal/toml"
)

// Mode is which of the four states the hub runs in (hub.py's Mode).
type Mode int

const (
	// ModeInteractiveHub is initialized + TTY: the interactive menu.
	ModeInteractiveHub Mode = iota
	// ModeNoninteractiveStatus is initialized + no TTY: the status summary.
	ModeNoninteractiveStatus
	// ModeOfferInit is uninitialized + TTY: offer the init wizard.
	ModeOfferInit
	// ModeErrorUninitialized is uninitialized + no TTY: report and exit 2. This
	// is the state that must never launch Python: no dependency, no hang.
	ModeErrorUninitialized
)

// String returns the Python enum's value, so a diagnostic or test can name the
// state with the same vocabulary the legacy implementation used.
func (m Mode) String() string {
	switch m {
	case ModeInteractiveHub:
		return "interactive-hub"
	case ModeNoninteractiveStatus:
		return "noninteractive-status"
	case ModeOfferInit:
		return "offer-init"
	default:
		return "error-uninitialized"
	}
}

// DecideMode is hub.py's decide_mode: the four-state matrix, in the same branch
// order.
func DecideMode(initialized, tty bool) Mode {
	if initialized {
		if tty {
			return ModeInteractiveHub
		}
		return ModeNoninteractiveStatus
	}
	if tty {
		return ModeOfferInit
	}
	return ModeErrorUninitialized
}

// Action is one menu entry's action, and doubles as the verb the hub delegates.
type Action string

const (
	ActionSync             Action = "sync"
	ActionDoctor           Action = "doctor"
	ActionAgents           Action = "agents"
	ActionSkills           Action = "skills"
	ActionRecipe           Action = "recipe"
	ActionConfigureRecipes Action = "configure-recipes"
	ActionRulesAudit       Action = "rules-audit"
	ActionUpgrade          Action = "upgrade"
	ActionVersion          Action = "version"
	ActionHelp             Action = "help"
	ActionInit             Action = "init"
	ActionQuit             Action = "quit"
)

// MenuEntry is one line of the hub's menu: the action, its title and its
// description, in hub.py's `_MENU` order.
type MenuEntry struct {
	Action      Action
	Title       string
	Description string
}

// Menu is hub.py's `_MENU`, verbatim: the non-interactive summary prints it and
// the interactive menu is built from it. A port that loses an entry is a silent
// product regression, so the order and the copy are pinned by a test.
func Menu() []MenuEntry {
	return []MenuEntry{
		{ActionSync, "Sync", "Reconcile manifest → bundled + vendor + AGENTS.md + agents"},
		{ActionDoctor, "Doctor", "Full project health report (read-only)"},
		{ActionAgents, "Agents", "Select which AI agents to enable"},
		{ActionSkills, "Skills", "List / inspect project skills by origin"},
		{ActionRecipe, "Recipes", "List / add / remove catalog recipes"},
		{ActionConfigureRecipes, "Configure recipes", "Set up recipe config, CLI deps, env vars"},
		{ActionRulesAudit, "Rules audit", "Inventory legacy rules for migration"},
		{ActionUpgrade, "Upgrade", "Upgrade the global ai-specs installation"},
		{ActionVersion, "Version", "Print the CLI version"},
		{ActionHelp, "Help", "Show ai-specs command help"},
		{ActionInit, "Init wizard", "Re-run interactive onboarding"},
		{ActionQuit, "Quit", "Exit the hub"},
	}
}

// usage is the byte-exact usage heredoc of lib/hub.sh.
const usage = `Usage: ai-specs hub [path] [--help]
Open the interactive ai-specs hub: project status + command menu.
With no TTY, prints a non-interactive status summary (initialized) or errors (uninitialized).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// Options is one hub invocation.
type Options struct {
	// Args are the arguments after the verb, parsed with lib/hub.sh's contract.
	Args []string
	// Target is a pre-resolved target. Empty means "take it from Args, else the
	// working directory" — the shell's `TARGET_PATH` default.
	Target string
	// Home is the CLI install root ($AI_SPECS_HOME).
	Home string
	// TTY mirrors the shell's `[[ -t 0 && -t 1 ]]`.
	TTY bool
}

// RunHub executes the hub and returns the process exit code.
func RunHub(opts Options, stdin io.Reader, stdout, stderr io.Writer) int {
	target := opts.Target

	// lib/hub.sh's parse loop. `--` shifts and breaks, so anything after it is
	// ignored — the shell never looks at the remaining argv.
	for i := 0; i < len(opts.Args); i++ {
		arg := opts.Args[i]
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, usage)
			return 0
		case arg == "--":
			i = len(opts.Args)
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "ERROR: unknown flag: %s\n", arg)
			fmt.Fprintln(stderr, "Run 'ai-specs hub --help' for usage.")
			return 2
		default:
			if target == "" {
				target = arg
				continue
			}
			fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
			return 2
		}
	}
	if target == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		target = wd
	}

	// `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"`.
	abs, err := resolveTarget(target)
	if err != nil {
		// hub.py decides this authoritatively; the shell route dies earlier
		// inside bash's `cd`, which is a recorded deviation (the doctor port has
		// the same one).
		fmt.Fprintf(stderr, "ERROR: target is not a directory: %s\n", target)
		return 2
	}

	switch mode := DecideMode(isInitialized(abs), opts.TTY); mode {
	case ModeErrorUninitialized:
		printUninitError(stderr, abs)
		return 2
	case ModeNoninteractiveStatus:
		return RunNoninteractive(abs, opts.Home, stdout, stderr)
	default:
		return runInteractiveBridge(abs, opts.Home, stdin, stdout, stderr)
	}
}

// RunNoninteractive is hub.py's `_run_noninteractive`: the frozen summary lines,
// and the DOCTOR's own exit code rather than 0 (the D5 fix).
func RunNoninteractive(root, home string, stdout, stderr io.Writer) int {
	rc, ok, info, warn, errCount := runDoctor(root, home, stderr)

	headline := "healthy"
	switch {
	case errCount > 0:
		// Quirk preserved from hub.py: errors say `error(s)` literally...
		headline = fmt.Sprintf("%d error(s)", errCount)
	case warn > 0:
		// ...while warnings pluralize properly.
		headline = fmt.Sprintf("%d warning", warn)
		if warn != 1 {
			headline += "s"
		}
	}

	fmt.Fprintf(stdout, "ai-specs status — %s\n", headline)
	fmt.Fprintf(stdout, "  version: %s\n", readVersion(home))
	fmt.Fprintf(stdout, "  target: %s\n", root)
	if topology, via := hubTopology(root); topology != "" {
		if via == "" {
			via = "auto"
		}
		fmt.Fprintf(stdout, "  topology: %s (%s)\n", topology, via)
	}
	fmt.Fprintf(stdout, "  Summary: %d OK, %d INFO, %d WARN, %d ERROR\n", ok, info, warn, errCount)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Commands:")
	for _, entry := range Menu() {
		fmt.Fprintf(stdout, "  %-12s  %s\n", entry.Title, entry.Description)
	}
	return rc
}

// DoctorExitCode runs the same doctor the summary runs and returns its exit code;
// it exists so a caller can check the "the hub exits with the doctor's code"
// contract without parsing the summary.
func DoctorExitCode(root, home string) int {
	rc, _, _, _, _ := runDoctor(root, home, io.Discard)
	return rc
}

// runDoctor runs the native doctor in-process and counts its severities.
func runDoctor(root, home string, stderr io.Writer) (rc, ok, info, warn, errCount int) {
	d := doctor.New(root, home)
	if stderr != nil {
		d.Stderr = stderr
	}
	rc = d.Run()
	for _, check := range d.Checks {
		switch check.Severity {
		case doctor.OK:
			ok++
		case doctor.INFO:
			info++
		case doctor.WARN:
			warn++
		case doctor.ERROR:
			errCount++
		}
	}
	return rc, ok, info, warn, errCount
}

// readVersion is hub.py's `_read_version`: the CLI install root's VERSION, or
// "unknown".
func readVersion(home string) string {
	data, err := os.ReadFile(filepath.Join(home, "VERSION"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// hubTopology is hub.py's conditional topology line: it appears only when the
// manifest is not the default resolution or when worktree-flow is enabled.
func hubTopology(root string) (resolved, via string) {
	manifest, err := config.LoadManifest(filepath.Join(root, "ai-specs", "ai-specs.toml"))
	if err != nil {
		return "", ""
	}
	topo := target.ProjectRepoTopology(root, manifest)
	if topo.Source == "default" && !worktreeFlowEnabled(manifest) {
		return "", ""
	}
	return topo.Resolved, topo.Via
}

// worktreeFlowEnabled mirrors hub.py's `wf.get("enabled") is True`: the
// manifest's boolean, never a truthy string.
func worktreeFlowEnabled(manifest *toml.Table) bool {
	recipes, ok := manifest.Table("recipes")
	if !ok || recipes == nil {
		return false
	}
	wf, ok := recipes.Table("worktree-flow")
	if !ok || wf == nil {
		return false
	}
	enabled, ok := wf.Get("enabled")
	if !ok {
		return false
	}
	b, isBool := enabled.(bool)
	return isBool && b
}

// delegateArgv is hub.py's DelegateRunner argv: the verb, then any extra words,
// then the target.
func delegateArgv(cli string, action Action, extra []string, target string) []string {
	argv := append([]string{cli, string(action)}, extra...)
	if target != "" {
		argv = append(argv, target)
	}
	return argv
}

// isInitialized mirrors hub.py's `_util.is_initialized`: the manifest is there.
func isInitialized(root string) bool {
	fi, err := os.Stat(filepath.Join(root, "ai-specs", "ai-specs.toml"))
	return err == nil && fi.Mode().IsRegular()
}

// printUninitError is hub.py's two frozen stderr lines.
func printUninitError(stderr io.Writer, target string) {
	fmt.Fprintf(stderr, "ERROR: no ai-specs project at %s\n", target)
	fmt.Fprintln(stderr, "Run 'ai-specs init' to create one.")
}

// resolveTarget reproduces `cd "$TARGET_PATH" && pwd` without changing the
// process working directory.
func resolveTarget(target string) (string, error) {
	fi, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return "", syscall.ENOTDIR
	}
	return filepath.Abs(target)
}

// runInteractiveBridge hands the interactive half to hub.py until U3b. The child
// inherits the hub's streams, so the TUI keeps working unchanged.
func runInteractiveBridge(target, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	script := filepath.Join(home, "lib", "_internal", "hub.py")
	cmd := exec.Command("python3", script, target)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = childEnv(home)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				return 128 + int(ws.Signal())
			}
		}
		fmt.Fprintf(stderr, "ai-specs hub: %v\n", err)
		return 1
	}
	return 0
}

// childEnv pins AI_SPECS_HOME for the bridged child, mirroring what the shell
// launcher exported before exec'ing Python.
func childEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AI_SPECS_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "AI_SPECS_HOME="+home)
}
