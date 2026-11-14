// Package hub is the native port of the `hub` verb — the interactive front door
// that bare `ai-specs` runs. Both halves are native (GO-20 U3a for the
// non-interactive one, U3b for the interactive one) and the package carries no
// third-party dependency:
//
//   - the mode decision (`lib/_internal/hub.py`'s `decide_mode`) and the
//     non-interactive status summary, whose bytes and exit code are frozen;
//   - the flag contract and the anti-hang guard of `lib/hub.sh`;
//   - the interactive menu, panel, submenus, agents write and delegation
//     (interactive.go) on top of internal/tui, with `delegateArgv` keeping the
//     argv shape hub.py's DelegateRunner used.
//
// The interactive half no longer bridges to `lib/_internal/hub.py`: that bridge was
// the last Python dependency of the default verb, and the pty and dependency-gate
// concerns it carried went with it.
package hub

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/doctor"
	"ai-specs.dev/ai-specs/internal/target"
	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/ai-specs/internal/tui"
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
	// Target is a target the CALLER already resolved. Empty means "take it from
	// Args, else the working directory" — the shell's `TARGET_PATH` default. The
	// dispatcher leaves it empty; a test sets it to skip the flag contract.
	Target string
	// Home is the CLI install root ($AI_SPECS_HOME).
	Home string
	// TTY mirrors the shell's `[[ -t 0 && -t 1 ]]`.
	TTY bool
	// Keys overrides the terminal as the keyboard source. Production leaves it
	// nil, so the hub reads a raw-mode tui.Terminal; a test injects scripted
	// keys to drive the same loop without a pty.
	Keys tui.KeySource
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
		// No TTY and no manifest: report and exit 2 WITHOUT launching Python, and
		// therefore without requiring its dependencies.
		printUninitError(stderr, abs)
		return 2
	case ModeNoninteractiveStatus:
		return RunNoninteractive(abs, opts.Home, stdout, stderr)
	case ModeInteractiveHub, ModeOfferInit:
		// Both interactive states draw their prompts natively; the offer opens
		// with the init wizard, the hub goes straight to the menu.
		return runInteractiveSession(mode, abs, opts, stdin, stdout, stderr)
	}
	return 1
}

// severityCounts is the doctor's tally. Named fields rather than five positional
// return values, so the summary reads as data and a caller cannot swap two of them
// without the compiler noticing (U3a advisory R2-1).
type severityCounts struct {
	OK    int
	Info  int
	Warn  int
	Error int
}

// RunNoninteractive is hub.py's `_run_noninteractive`: the frozen summary lines,
// and the DOCTOR's own exit code rather than 0 (the D5 fix).
func RunNoninteractive(root, home string, stdout, stderr io.Writer) int {
	run := doctorRunFor(root, home, stderr)
	c := run.counts

	fmt.Fprintf(stdout, "ai-specs status — %s\n", headlineForCounts(c))
	fmt.Fprintf(stdout, "  version: %s\n", readVersion(home))
	fmt.Fprintf(stdout, "  target: %s\n", root)
	if topology, via := hubTopology(root); topology != "" {
		if via == "" {
			via = "auto"
		}
		fmt.Fprintf(stdout, "  topology: %s (%s)\n", topology, via)
	}
	fmt.Fprintf(stdout, "  Summary: %d OK, %d INFO, %d WARN, %d ERROR\n", c.OK, c.Info, c.Warn, c.Error)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Commands:")
	for _, entry := range Menu() {
		fmt.Fprintf(stdout, "  %-12s  %s\n", entry.Title, entry.Description)
	}
	return run.exitCode
}

// headlineForCounts is the frozen headline vocabulary, quirks included: errors say
// `error(s)` LITERALLY and win over warnings, while warnings pluralize properly
// (`1 warning` vs `2 warnings`). Both spellings are user-visible bytes of the
// default verb, so they are pinned by TestHeadlineForCountsPinsTheFrozenWording.
func headlineForCounts(c severityCounts) string {
	switch {
	case c.Error > 0:
		return fmt.Sprintf("%d error(s)", c.Error)
	case c.Warn == 1:
		return "1 warning"
	case c.Warn > 1:
		return fmt.Sprintf("%d warnings", c.Warn)
	default:
		return "healthy"
	}
}

// DoctorExitCode runs the same doctor the summary runs and returns its exit code;
// it exists so a caller can check the "the hub exits with the doctor's code"
// contract without parsing the summary.
func DoctorExitCode(root, home string) int {
	return doctorRunFor(root, home, io.Discard).exitCode
}

// doctorRun is one doctor run's observable outcome: the exit code, the severity
// tally and the non-OK checks the panel shows. The interactive panel and the
// non-interactive summary both consume this one run, which is what keeps them
// from disagreeing about the project's health.
type doctorRun struct {
	exitCode int
	counts   severityCounts
	checks   []tui.PanelCheck
}

// doctorRunFor runs the native doctor in-process and maps its checks onto the
// summary tally and the panel rows in one pass.
func doctorRunFor(root, home string, stderr io.Writer) doctorRun {
	d := doctor.New(root, home)
	if stderr != nil {
		d.Stderr = stderr
	}
	run := doctorRun{exitCode: d.Run()}
	for _, check := range d.Checks {
		switch check.Severity {
		case doctor.OK:
			run.counts.OK++
		case doctor.INFO:
			run.counts.Info++
		case doctor.WARN:
			run.counts.Warn++
		case doctor.ERROR:
			run.counts.Error++
		}
		if check.Severity != doctor.OK {
			run.checks = append(run.checks, tui.PanelCheck{
				Label: check.Severity.String(),
				Text:  check.Name + ": " + check.Message,
			})
		}
	}
	return run
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
// then the target. The interactive hub is its caller; the argv shape is also
// pinned directly by TestDelegateArgvShape so it cannot drift.
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

// childEnv pins AI_SPECS_HOME for a delegated child, mirroring what the shell
// launcher exported before exec'ing the child.
func childEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AI_SPECS_HOME=") || strings.HasPrefix(kv, "AI_SPECS_GO_BIN=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "AI_SPECS_HOME="+home)
	// The launcher execs AI_SPECS_GO_BIN: pin the running binary so the child
	// never depends on a dist/ build inside the installed home (GO-20 U2e T2).
	if self, err := os.Executable(); err == nil {
		env = append(env, "AI_SPECS_GO_BIN="+self)
	}
	return env
}
