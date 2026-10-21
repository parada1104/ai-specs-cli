package deps

// Tests for Gate (T3): config_wizard._dep_gate — the panel, the TTY-gated
// install offer, the recheck and the "Configure anyway?" confirm — with every
// side effect scripted: the probe's answers, the prompt's answers, the
// package-manager commands and the provider install. The interactive loop is
// pinned in-process like the sister slices: no PTY in the table (L4's
// constraint unchanged).
//
// The bytes asserted here are the legacy script's bytes: the panel through
// _render_dep_panel, every offer line through offer_and_install, the two
// frozen confirm messages, and the `install offer skipped:` arm.

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
)

// gateScript is one scripted Gate run: the probe's answers in order, the
// prompt's answers in order, and recorders for what the gate did.
type gateScript struct {
	probes   [][]Result
	probed   int
	answers  []bool
	asked    int
	prompts  []string
	commands [][]string
	code     int
	runErr   error
	provide  provider.Resolution
	provErr  error
	installs []Plan
}

func (s *gateScript) probe() []Result {
	if s.probed >= len(s.probes) {
		return s.probes[len(s.probes)-1]
	}
	out := s.probes[s.probed]
	s.probed++
	return out
}

// prompt is the gate's one question type. An exhausted script answers
// "not answered", which the gate reads as a decline.
func (s *gateScript) prompt(message string, def bool) (bool, bool) {
	s.prompts = append(s.prompts, message)
	if s.asked >= len(s.answers) {
		return false, false
	}
	answer := s.answers[s.asked]
	s.asked++
	return answer, true
}

func (s *gateScript) run(command []string) (int, error) {
	s.commands = append(s.commands, command)
	return s.code, s.runErr
}

func (s *gateScript) install(plan Plan) (provider.Resolution, error) {
	s.installs = append(s.installs, plan)
	return s.provide, s.provErr
}

// gate runs the scripted request and returns the verdict plus the console.
func (s *gateScript) gate(t *testing.T, recipe *schema.Recipe, tty bool) (bool, string, error) {
	t.Helper()
	var console strings.Builder
	ok, err := Gate(GateRequest{
		Recipe:          recipe,
		Home:            "/home/test",
		Out:             &console,
		TTY:             tty,
		Prompt:          s.prompt,
		Probe:           s.probe,
		Run:             s.run,
		InstallProvider: s.install,
	})
	return ok, console.String(), err
}

func gateRecipe(deps ...*schema.CliDep) *schema.Recipe {
	return &schema.Recipe{ID: "demo", CliDeps: deps}
}

func gateDep(binary, purpose, url string, required bool) *schema.CliDep {
	return &schema.CliDep{Binary: binary, Purpose: purpose, InstallURL: url, Required: required}
}

// gateResult builds one scripted probe answer. The panel reads Purpose,
// Detail, InstallURL, Required and OK; nothing else.
func gateResult(binary, purpose, detail, url string, required, ok bool) Result {
	return Result{
		Binary: binary, Purpose: purpose, Detail: detail,
		InstallURL: url, Required: required, OK: ok, Found: ok,
	}
}

// TestGatePanelWithNothingMissing pins the quiet path: the panel prints, no
// offer is attempted and no confirm is asked (_dep_gate's `if not
// missing_required: return True`).
func TestGatePanelWithNothingMissing(t *testing.T) {
	withPath(t, "jq")
	s := &gateScript{probes: [][]Result{{
		gateResult("jq", "JSON processor", "", "https://example.com/jq", true, true),
	}}}
	recipe := gateRecipe(gateDep("jq", "JSON processor", "https://example.com/jq", true))
	ok, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !ok {
		t.Fatalf("Gate() = false, want true")
	}
	want := "OK    jq            JSON processor\n"
	if console != want {
		t.Fatalf("console = %q, want %q", console, want)
	}
	if len(s.prompts) != 0 || len(s.commands) != 0 {
		t.Fatalf("prompts = %v, commands = %v; want neither", s.prompts, s.commands)
	}
}

// TestGateOffersEveryMissingPlanAndRechecks pins offer_and_install's loop: the
// package plan asks with its display line and runs nothing when declined, the
// guidance plan prints `  → <binary>: <display>` and never prompts, and the
// recheck re-renders the panel before the frozen confirm.
func TestGateOffersEveryMissingPlanAndRechecks(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	missing := []Result{
		gateResult("jq", "JSON processor", "not found on PATH", "https://example.com/jq", true, false),
		gateResult("npx", "Node runner", "not found on PATH", "", true, false),
	}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{false, true}, // decline jq's install, configure anyway
	}
	recipe := gateRecipe(
		gateDep("jq", "JSON processor", "https://example.com/jq", true),
		gateDep("npx", "Node runner", "", true),
	)
	ok, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !ok {
		t.Fatalf("Gate() = false, want true")
	}
	wantConsole := "WARN  jq            JSON processor (not found on PATH) → https://example.com/jq\n" +
		"WARN  npx           Node runner (not found on PATH)\n" +
		"  → npx: install 'npx' manually\n" +
		"WARN  jq            JSON processor (not found on PATH) → https://example.com/jq\n" +
		"WARN  npx           Node runner (not found on PATH)\n"
	if console != wantConsole {
		t.Fatalf("console = %q, want %q", console, wantConsole)
	}
	wantPrompts := []string{
		"Install jq now? (brew install jq)",
		"2 required CLI tool(s) missing. Configure anyway?",
	}
	if strings.Join(s.prompts, "\n") != strings.Join(wantPrompts, "\n") {
		t.Fatalf("prompts = %q, want %q", s.prompts, wantPrompts)
	}
	if len(s.commands) != 0 {
		t.Fatalf("commands = %v, want none (the install was declined)", s.commands)
	}
}

// TestGateInstallsPackagePlan pins the consenting arm's three lines: the
// command runs with brew's own argv, and `  ✓ <binary> installed` prints only
// when the binary is on PATH afterwards.
func TestGateInstallsPackagePlan(t *testing.T) {
	withPath(t, "brew", "gh")
	withSystem(t, "darwin")
	s := &gateScript{
		probes: [][]Result{
			{gateResult("gh", "GitHub CLI", "not found on PATH", "https://cli.github.com", true, false)},
			{gateResult("gh", "GitHub CLI", "", "https://cli.github.com", true, true)},
		},
		answers: []bool{true},
	}
	recipe := gateRecipe(gateDep("gh", "GitHub CLI", "https://cli.github.com", true))
	ok, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !ok {
		t.Fatalf("Gate() = false, want true")
	}
	if len(s.commands) != 1 || strings.Join(s.commands[0], " ") != "brew install gh" {
		t.Fatalf("commands = %v, want [brew install gh]", s.commands)
	}
	wantConsole := "WARN  gh            GitHub CLI (not found on PATH) → https://cli.github.com\n" +
		"  ✓ gh installed\n" +
		"OK    gh            GitHub CLI\n"
	if console != wantConsole {
		t.Fatalf("console = %q, want %q", console, wantConsole)
	}
	// The recheck cleared the dep, so the confirm never fired.
	if len(s.prompts) != 1 {
		t.Fatalf("prompts = %q, want only the install offer", s.prompts)
	}
}

// TestGatePackageInstallExitCode pins the rc!=0 arm: the exit code line, the
// guidance line, and the confirm still firing because the dep is still
// missing.
func TestGatePackageInstallExitCode(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	missing := []Result{gateResult("gh", "GitHub CLI", "not found on PATH", "https://cli.github.com", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
		code:    1,
	}
	recipe := gateRecipe(gateDep("gh", "GitHub CLI", "https://cli.github.com", true))
	ok, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if ok {
		t.Fatalf("Gate() = true, want false (the confirm was declined)")
	}
	want := "WARN  gh            GitHub CLI (not found on PATH) → https://cli.github.com\n" +
		"  ! install gh exited 1\n" +
		"    see https://cli.github.com\n" +
		"WARN  gh            GitHub CLI (not found on PATH) → https://cli.github.com\n"
	if console != want {
		t.Fatalf("console = %q, want %q", console, want)
	}
	if len(s.prompts) != 2 {
		t.Fatalf("prompts = %q, want the offer and the confirm", s.prompts)
	}
}

// TestGatePackageInstallErrorHasNoGuidanceLine pins the asymmetry the source
// carries: the OSError/TimeoutExpired arm prints `  ! install … failed: <exc>`
// and NO `see` line, unlike the exit-code and provider arms.
func TestGatePackageInstallErrorHasNoGuidanceLine(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	missing := []Result{gateResult("gh", "GitHub CLI", "not found on PATH", "https://cli.github.com", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
		runErr:  errors.New("Command '['brew', 'install', 'gh']' timed out after 300 seconds"),
	}
	recipe := gateRecipe(gateDep("gh", "GitHub CLI", "https://cli.github.com", true))
	_, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if strings.Contains(console, "see https://cli.github.com") {
		t.Fatalf("console = %q, want no guidance line on the exception arm", console)
	}
	if !strings.Contains(console, "  ! install gh failed: Command '['brew', 'install', 'gh']' timed out after 300 seconds\n") {
		t.Fatalf("console = %q, want the failed line with the error's own text", console)
	}
}

// TestGatePackageInstalledButNotOnPath pins the arm after a successful exit:
// `  ! <binary> still not on PATH after install` plus the guidance line.
func TestGatePackageInstalledButNotOnPath(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	missing := []Result{gateResult("gh", "GitHub CLI", "not found on PATH", "https://cli.github.com", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
	}
	recipe := gateRecipe(gateDep("gh", "GitHub CLI", "https://cli.github.com", true))
	_, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !strings.Contains(console, "  ! gh still not on PATH after install\n    see https://cli.github.com\n") {
		t.Fatalf("console = %q, want the still-not-on-PATH pair", console)
	}
}

// TestGateProviderPlanDescribesThenInstalls pins the provider arm: the consent
// preview comes first (describe_install_plan), then the install, then the
// verified line with the version.
func TestGateProviderPlanDescribesThenInstalls(t *testing.T) {
	s := &gateScript{
		probes: [][]Result{
			{gateResult("jinna", "Jinna MCP", "no managed version", "https://example.com/jinna", true, false)},
			{gateResult("jinna", "Jinna MCP", "", "https://example.com/jinna", true, true)},
		},
		answers: []bool{true},
		provide: provider.Resolution{Binary: "jinna", Version: "1.4.2", Verified: true},
	}
	recipe := gateRecipe(&schema.CliDep{
		Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		InstallURL:    "https://example.com/jinna",
		Installer:     "github-release",
		Repository:    provider.Repository,
		ReleasePolicy: "latest-stable",
	})
	ok, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !ok {
		t.Fatalf("Gate() = false, want true")
	}
	if len(s.installs) != 1 || s.installs[0].Kind != KindGitHubRelease {
		t.Fatalf("installs = %v, want one github-release plan", s.installs)
	}
	for _, want := range []string{
		"  GitHub Release installation plan for 'jinna':\n",
		"    repository:      " + provider.Repository + "\n",
		"    release policy:  latest-stable (latest non-draft, non-prerelease tag)\n",
		"    checksum source: SHA256SUMS from the same release\n",
		"  ✓ jinna installed (1.4.2)\n",
		"OK    jinna         Jinna MCP\n",
	} {
		if !strings.Contains(console, want) {
			t.Fatalf("console = %q, want it to contain %q", console, want)
		}
	}
	if wok := strings.Contains(s.prompts[0], "Install jinna now? (GitHub Release "+provider.Repository+
		" (latest-stable) for "); !wok {
		t.Fatalf("prompt = %q, want the provider display line", s.prompts[0])
	}
}

// TestGateProviderInstallFailureSeesGuidance pins the provider failure arm: the
// failed line AND the `see` line, which the package exception arm omits.
func TestGateProviderInstallFailureSeesGuidance(t *testing.T) {
	missing := []Result{gateResult("jinna", "Jinna MCP", "no managed version", "https://example.com/jinna", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
		provErr: errors.New("provider download URL is not an allowlisted HTTPS URL"),
	}
	recipe := gateRecipe(&schema.CliDep{
		Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		InstallURL:    "https://example.com/jinna",
		Installer:     "github-release",
		Repository:    provider.Repository,
		ReleasePolicy: "latest-stable",
	})
	_, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !strings.Contains(console, "  ! install jinna failed: provider download URL is not an allowlisted HTTPS URL\n"+
		"    see https://example.com/jinna\n") {
		t.Fatalf("console = %q, want the failed+see pair", console)
	}
}

// TestGatePlanResolutionErrorSkipsTheWholeOffer pins the outer catch: one
// rejected plan aborts the WHOLE batch (`install offer skipped: <exc>`), the
// recheck never runs, and the confirm uses the pre-offer count.
func TestGatePlanResolutionErrorSkipsTheWholeOffer(t *testing.T) {
	missing := []Result{
		gateResult("gh", "GitHub CLI", "not found on PATH", "https://cli.github.com", true, false),
		gateResult("jinna", "Jinna MCP", "no managed version", "", true, false),
	}
	s := &gateScript{
		probes:  [][]Result{missing},
		answers: []bool{false},
	}
	recipe := gateRecipe(
		gateDep("gh", "GitHub CLI", "https://cli.github.com", true),
		&schema.CliDep{
			Binary: "jinna", Purpose: "Jinna MCP", Required: true,
			Installer: "github-release", Repository: "evil/repo", ReleasePolicy: "latest-stable",
		},
	)
	_, console, err := s.gate(t, recipe, true)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if s.probed != 1 {
		t.Fatalf("probe ran %d times, want exactly 1 (no recheck after the abort)", s.probed)
	}
	if !strings.Contains(console, "install offer skipped: provider dependency is not the allowlisted GitHub Release installer\n") {
		t.Fatalf("console = %q, want the frozen offer-skipped line", console)
	}
	if len(s.prompts) != 1 || s.prompts[0] != "2 required CLI tool(s) missing. Configure anyway?" {
		t.Fatalf("prompts = %q, want only the confirm at the pre-offer count", s.prompts)
	}
}

// TestGateOffTTYSkipsTheOffer pins the isatty guard: without a terminal the
// plans are never resolved and the gate goes straight to the confirm — a
// non-TTY run therefore never surfaces a plan-resolution error either.
func TestGateOffTTYSkipsTheOffer(t *testing.T) {
	missing := []Result{gateResult("jinna", "Jinna MCP", "no managed version", "", true, false)}
	s := &gateScript{probes: [][]Result{missing}, answers: []bool{false}}
	recipe := gateRecipe(&schema.CliDep{
		Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		Installer: "github-release", Repository: "evil/repo", ReleasePolicy: "latest-stable",
	})
	if _, console, err := s.gate(t, recipe, false); err != nil {
		t.Fatalf("Gate() error = %v", err)
	} else if strings.Contains(console, "install offer skipped") {
		t.Fatalf("console = %q, want no offer off the TTY", console)
	}
	if len(s.installs) != 0 || len(s.commands) != 0 {
		t.Fatalf("installs = %v, commands = %v; want none off the TTY", s.installs, s.commands)
	}
	if len(s.prompts) != 1 || s.prompts[0] != "1 required CLI tool(s) missing. Configure anyway?" {
		t.Fatalf("prompts = %q, want the confirm alone", s.prompts)
	}
}

// TestGateUnansweredPromptDeclines pins the port's precedent for a prompt that
// cannot be answered (EOF, interrupt): a decline, never an error, exactly as
// questionary's None reads through bool().
func TestGateUnansweredPromptDeclines(t *testing.T) {
	missing := []Result{gateResult("jq", "JSON processor", "not found on PATH", "", true, false)}
	s := &gateScript{probes: [][]Result{missing}} // no answers: the script runs dry
	recipe := gateRecipe(gateDep("jq", "JSON processor", "", true))
	ok, _, err := s.gate(t, recipe, false)
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if ok {
		t.Fatalf("Gate() = true, want false for an unanswered confirm")
	}
}

// TestGateResumeErrorIsAnError pins the one arm the port adds: a suspended raw
// mode that cannot resume is an error the caller words — Python had no such
// seam because the raw-mode window belonged to the parent process.
func TestGateResumeErrorIsAnError(t *testing.T) {
	withPath(t, "brew", "gh")
	withSystem(t, "darwin")
	missing := []Result{gateResult("gh", "GitHub CLI", "not found on PATH", "", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
	}
	resumeErr := errors.New("resume failed")
	var console strings.Builder
	_, err := Gate(GateRequest{
		Recipe:  gateRecipe(gateDep("gh", "GitHub CLI", "", true)),
		Home:    "/home/test",
		Out:     &console,
		TTY:     true,
		Prompt:  s.prompt,
		Probe:   s.probe,
		Run:     s.run,
		Suspend: func() {},
		Resume:  func() error { return resumeErr },
	})
	if !errors.Is(err, resumeErr) {
		t.Fatalf("Gate() error = %v, want %v", err, resumeErr)
	}
}

// TestGateSuspendsAroundEveryChild pins the terminal handoff: raw mode is off
// while a child process holds the terminal, and back on before the next
// prompt.
func TestGateSuspendsAroundEveryChild(t *testing.T) {
	withPath(t, "brew", "gh")
	withSystem(t, "darwin")
	var trace []string
	missing := []Result{gateResult("gh", "GitHub CLI", "not found on PATH", "", true, false)}
	s := &gateScript{
		probes:  [][]Result{missing, missing},
		answers: []bool{true, false},
	}
	runner := func(command []string) (int, error) {
		trace = append(trace, "child")
		return 0, nil
	}
	var console strings.Builder
	_, err := Gate(GateRequest{
		Recipe:  gateRecipe(gateDep("gh", "GitHub CLI", "", true)),
		Home:    "/home/test",
		Out:     &console,
		TTY:     true,
		Prompt:  func(string, bool) (bool, bool) { trace = append(trace, "prompt"); return s.prompt("", false) },
		Probe:   s.probe,
		Run:     runner,
		Suspend: func() { trace = append(trace, "suspend") },
		Resume:  func() error { trace = append(trace, "resume"); return nil },
	})
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	want := []string{"prompt", "suspend", "child", "resume", "prompt"}
	if strings.Join(trace, ",") != strings.Join(want, ",") {
		t.Fatalf("trace = %v, want %v", trace, want)
	}
}

// TestGateDefaultsToTheRealProbe pins the injected-seam default: with no Probe
// the gate reads CheckRecipe, so a recipe whose dep really exists passes with
// the panel drawn and nothing asked.
func TestGateDefaultsToTheRealProbe(t *testing.T) {
	withPath(t, "sh")
	withSystem(t, "darwin")
	recipe := gateRecipe(gateDep("sh", "POSIX shell", "", true))
	var console strings.Builder
	ok, err := Gate(GateRequest{Recipe: recipe, Home: t.TempDir(), Out: &console, TTY: false,
		Prompt: func(string, bool) (bool, bool) { return false, true }})
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	if !ok {
		t.Fatalf("Gate() = false, want true")
	}
	if console.String() != "OK    sh            POSIX shell\n" {
		t.Fatalf("console = %q, want the OK panel line", console.String())
	}
}

// TestGateProductionRunTimesOut pins the package runner's 300-second bound and
// its message: subprocess.run's TimeoutExpired text is the shape the `failed:`
// line carries, and a command that has already exited reports its code.
func TestGateProductionRunTimesOut(t *testing.T) {
	code, err := runPackageCommand([]string{"sh", "-c", "exit 0"}, nil, nil, nil, time.Minute)
	if err != nil || code != 0 {
		t.Fatalf("runPackageCommand(exit 0) = %d, %v; want 0, nil", code, err)
	}
	code, err = runPackageCommand([]string{"sh", "-c", "exit 3"}, nil, nil, nil, time.Minute)
	if err != nil || code != 3 {
		t.Fatalf("runPackageCommand(exit 3) = %d, %v; want 3, nil", code, err)
	}
	_, err = runPackageCommand([]string{"sh", "-c", "sleep 5"}, nil, nil, nil, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out after 0 seconds") {
		t.Fatalf("runPackageCommand(timeout) error = %v, want the TimeoutExpired shape", err)
	}
	_, err = runPackageCommand([]string{"definitely-not-a-binary-xyz"}, nil, nil, nil, time.Second)
	if err == nil {
		t.Fatalf("runPackageCommand(missing binary) error = nil, want an OSError analogue")
	}
	if !errors.Is(err, exec.ErrNotFound) && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("runPackageCommand(missing binary) error = %v, want a not-found wording", err)
	}
}
