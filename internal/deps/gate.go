package deps

// The native CLI-dep gate — odd/tasks/go-20-u7-dep-gate.md T3, the port of
// config_wizard._dep_gate plus dep_install.offer_and_install's TTY half. It
// retires S37's `runLegacyInitDepGate` seam (internal/cli): the last
// interactive surface whose only Go side was a `python3 -c` driver.
//
// The shape is the legacy function's, arm by arm:
//
//	probe → panel → (TTY only) resolve a plan per missing required dep →
//	offer each plan → recheck → panel → `N required CLI tool(s) missing.
//	Configure anyway?`
//
// Two things are deliberately NOT here: nothing reads a recipe from disk (the
// caller already holds the parsed *schema.Recipe — the Python re-read existed
// only because the seam was a separate process) and nothing bootstraps a
// vendor tree (the port has no Python dependencies to install). Everything
// that touches the outside world — the probe, the yes/no question, the
// package-manager process, the provider download, the raw-mode handoff — is an
// injected seam, so the sequencing and the bytes are pinned in-process.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
)

// packageInstallTimeout is subprocess.run(command, timeout=300): the bound the
// legacy offer puts on brew/apt, which can wait on a password prompt of their
// own.
const packageInstallTimeout = 300 * time.Second

// GatePrompts asks one gate question the way questionary.confirm does: the
// message goes to the prompt stream (stdout), and answered is false when the
// terminal could not answer (EOF, interrupt) — which reads as a decline, the
// port's precedent for questionary's None.
type GatePrompts func(message string, def bool) (answer bool, answered bool)

// InstallRunner runs ONE package-manager command, returning the exit status,
// or an error for subprocess.run's OSError/TimeoutExpired arm. The error's text
// is what the frozen `  ! install <binary> failed: <exc>` line carries.
type InstallRunner func(command []string) (int, error)

// ProviderInstaller is provider_install.install_github_release for one plan;
// nil selects the package's own production install.
type ProviderInstaller func(plan Plan) (provider.Resolution, error)

// GateRequest is _dep_gate's whole input with its seams named.
type GateRequest struct {
	// Recipe is the parsed recipe the caller already read: the gate never
	// touches the catalog.
	Recipe *schema.Recipe
	// Home is the ai-specs install root the plans resolve against
	// (_util.ai_specs_home()).
	Home string
	// Out is the console: every panel line and every offer line lands here —
	// rich Console(stderr=True) plus offer_and_install's `file=sys.stderr`.
	Out io.Writer
	// TTY is `sys.stdin.isatty() and sys.stdout.isatty()`: without it the
	// install offer never runs and the gate goes straight to the confirm.
	TTY bool
	// Prompt asks the questions. A nil Prompt cannot answer for the user, so
	// the gate treats every question as unanswered.
	Prompt GatePrompts
	// Probe reads the dep results; nil means CheckRecipe(Recipe, Home). The
	// recheck after the offer runs the SAME closure — one probe, two reads.
	Probe func() []Result
	// Run installs one package-manager plan; nil means the production runner
	// over Stdin/Stdout/Out.
	Run InstallRunner
	// InstallProvider installs one GitHub Release plan; nil means
	// provider.InstallRelease with the production fetcher and version probe.
	InstallProvider ProviderInstaller
	// Stdin and Stdout are the streams a package-manager child inherits
	// (subprocess.run inherits the process's own); Out doubles as the child's
	// stderr.
	Stdin  io.Reader
	Stdout io.Writer
	// Suspend/Resume bracket every child process, so a child that prompts
	// (sudo's password) reaches a cooked terminal and the next question gets
	// raw mode back. Errors from Resume are fatal: nothing after it can prompt.
	Suspend func()
	Resume  func() error
}

// Gate is config_wizard._dep_gate. True means "configure this recipe": either
// nothing required is missing, or the human answered the frozen confirm with
// yes. The error arm exists only for a terminal that cannot be handed back —
// every other failure is a printed line, exactly where the legacy prints it.
func Gate(req GateRequest) (bool, error) {
	out := req.Out
	if out == nil {
		out = io.Discard
	}
	probe := req.Probe
	if probe == nil {
		probe = func() []Result { return CheckRecipe(req.Recipe, req.Home) }
	}

	results := probe()
	writeGatePanel(out, results)
	missing := MissingRequired(results)
	if len(missing) > 0 && req.TTY {
		plans, err := resolveGatePlans(req, missing)
		switch {
		case err != nil:
			// The legacy outer `except Exception`: one line, and the pre-offer
			// missing list survives — the recheck never ran.
			fmt.Fprintf(out, "install offer skipped: %v\n", err)
		default:
			if fatal := runGatePlans(req, plans); fatal != nil {
				return false, fatal
			}
			results = probe()
			writeGatePanel(out, results)
			missing = MissingRequired(results)
		}
	}
	if len(missing) == 0 {
		return true, nil
	}
	answer, _ := askGate(req, fmt.Sprintf("%d required CLI tool(s) missing. Configure anyway?", len(missing)), false)
	return answer, nil
}

// writeGatePanel is `_render_dep_panel(results, console)`; an empty result set
// prints nothing at all.
func writeGatePanel(out io.Writer, results []Result) {
	lines := PanelLines(results)
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(out, strings.Join(lines, "\n"))
}

// askGate is one question through the injected prompt, with the "cannot answer"
// arm folded into the decline.
func askGate(req GateRequest, message string, def bool) (bool, bool) {
	if req.Prompt == nil {
		return false, false
	}
	return req.Prompt(message, def)
}

// resolveGatePlans is _dep_gate's plan loop: one resolve_install_plan per
// missing required dep, carrying the declared dep's installer hints when the
// recipe declares that binary at all. The FIRST resolution error aborts the
// whole batch — the legacy loop builds every plan before offering any, and its
// caller turns the exception into `install offer skipped:`.
func resolveGatePlans(req GateRequest, missing []Result) ([]Plan, error) {
	depByBinary := make(map[string]*schema.CliDep, len(req.Recipe.CliDeps))
	for _, dep := range req.Recipe.CliDeps {
		if dep == nil {
			continue
		}
		depByBinary[dep.Binary] = dep
	}
	plans := make([]Plan, 0, len(missing))
	for _, result := range missing {
		request := PlanRequest{Binary: result.Binary, InstallURL: result.InstallURL}
		if dep, ok := depByBinary[result.Binary]; ok {
			request.Installer = dep.Installer
			request.Repository = dep.Repository
			request.ReleasePolicy = dep.ReleasePolicy
			request.MinVersion = dep.MinVersion
		}
		plan, err := ResolvePlan(request, req.Home)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// runGatePlans is offer_and_install(plans, tty=True): a guidance plan (or a
// plan with no command, which the legacy refuses to hand to a subprocess)
// prints its arrow line and asks nothing; every other plan describes itself,
// asks `Install <binary> now? (<display>)` and runs only on a yes.
func runGatePlans(req GateRequest, plans []Plan) error {
	out := req.Out
	if out == nil {
		out = io.Discard
	}
	runner := req.Run
	if runner == nil {
		runner = func(command []string) (int, error) {
			return runPackageCommand(command, req.Stdin, req.Stdout, out, packageInstallTimeout)
		}
	}
	installer := req.InstallProvider
	if installer == nil {
		installer = func(plan Plan) (provider.Resolution, error) {
			if plan.ProviderPlan == nil {
				return provider.Resolution{}, errors.New("provider plan is missing its release plan")
			}
			return provider.InstallRelease(*plan.ProviderPlan, nil, nil)
		}
	}

	for _, plan := range plans {
		if plan.Kind == KindGuidance || (plan.Kind != KindGitHubRelease && len(plan.Command) == 0) {
			fmt.Fprintf(out, "  → %s: %s\n", plan.Binary, plan.Display)
			continue
		}
		if plan.Kind == KindGitHubRelease {
			fmt.Fprintln(out, Describe(plan))
		}
		answer, answered := askGate(req, "Install "+plan.Binary+" now? ("+plan.Display+")", false)
		if !answered || !answer {
			continue
		}
		if req.Suspend != nil {
			req.Suspend()
		}
		if plan.Kind == KindGitHubRelease {
			installProviderPlan(out, installer, plan)
		} else {
			installPackagePlan(out, runner, plan)
		}
		if req.Resume != nil {
			if resumeErr := req.Resume(); resumeErr != nil {
				return resumeErr
			}
		}
	}
	return nil
}

// installProviderPlan is the github-release arm: the provider's own error text
// through the frozen `failed:` line plus its guidance line, and the verified
// line only when the install reports one.
func installProviderPlan(out io.Writer, installer ProviderInstaller, plan Plan) {
	result, err := installer(plan)
	if err != nil {
		fmt.Fprintf(out, "  ! install %s failed: %v\n", plan.Binary, err)
		if plan.GuidanceURL != "" {
			fmt.Fprintf(out, "    see %s\n", plan.GuidanceURL)
		}
		return
	}
	if result.Verified {
		fmt.Fprintf(out, "  ✓ %s installed (%s)\n", plan.Binary, result.Version)
	}
}

// installPackagePlan is the brew/apt arm: the exit status, the still-not-on-
// PATH check, and the guidance lines. The OSError/TimeoutExpired arm prints NO
// guidance line — the source's asymmetry, pinned by a test.
func installPackagePlan(out io.Writer, runner InstallRunner, plan Plan) {
	code, err := runner(plan.Command)
	if err != nil {
		fmt.Fprintf(out, "  ! install %s failed: %v\n", plan.Binary, err)
		return
	}
	if code != 0 {
		fmt.Fprintf(out, "  ! install %s exited %d\n", plan.Binary, code)
		if plan.GuidanceURL != "" {
			fmt.Fprintf(out, "    see %s\n", plan.GuidanceURL)
		}
		return
	}
	if Which(plan.Binary) {
		fmt.Fprintf(out, "  ✓ %s installed\n", plan.Binary)
		return
	}
	fmt.Fprintf(out, "  ! %s still not on PATH after install\n", plan.Binary)
	if plan.GuidanceURL != "" {
		fmt.Fprintf(out, "    see %s\n", plan.GuidanceURL)
	}
}

// runPackageCommand is the production InstallRunner: one package-manager
// command with subprocess.run's own stream inheritance and its 300-second
// bound. The timeout error carries TimeoutExpired's text, because the frozen
// `failed:` line embeds it; an exec error surfaces Go's own wording (the
// ADR 0002 deviation class, pinned by a test).
func runPackageCommand(command []string, stdin io.Reader, stdout, stderr io.Writer, timeout time.Duration) (int, error) {
	if len(command) == 0 {
		return 0, errors.New("empty install command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	child := exec.CommandContext(ctx, command[0], command[1:]...)
	child.Stdin, child.Stdout, child.Stderr = stdin, stdout, stderr
	runErr := child.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("Command '%s' timed out after %d seconds", pyArgvRepr(command), int(timeout.Seconds()))
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, runErr
	}
	return 0, nil
}

// pyArgvRepr is Python's repr of the argv list, which TimeoutExpired's message
// embeds: `['brew', 'install', 'gh']`. Package-manager argv carries no quoting
// characters, so the simple form is exact where it is reachable.
func pyArgvRepr(command []string) string {
	parts := make([]string, len(command))
	for i, arg := range command {
		parts[i] = "'" + arg + "'"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
