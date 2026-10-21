package cli

// The native CLI-dep gate's CLI half — odd/tasks/go-20-u7-dep-gate.md T3. It
// replaces S37's `runLegacyInitDepGate` seam: internal/deps owns the gate
// itself, and these two adapters wire it to the three callers, which differ
// only in who owns the raw-mode window.
//
//   - init's wizard already holds the terminal (OpenKeys), so the gate asks
//     through the live KeySource and suspends that window around every install
//     child (a brew/apt child may prompt for a password and needs a cooked
//     terminal).
//   - `recipe add` and `configure-recipes` hold no window when they gate (both
//     open one per prompt), so the adapter opens ONE window for the gate's own
//     questions and closes it before returning.
//
// Everything else — the probe, the plans, the panel, the offer, the recheck and
// the frozen `Configure anyway?` confirm — is internal/deps'. The outcome
// vocabulary is the callers': Proceed is "configure this recipe", Skip is the
// declined confirm (each caller prints its own frozen line), and Err is the one
// arm Python had no analogue for: a terminal that cannot be handed back.

import (
	"errors"
	"fmt"
	"io"
	"os"

	"ai-specs.dev/ai-specs/internal/deps"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/tui"
)

// gateTerminal is the raw-mode window the gate suspends around a child; every
// tui.Terminal satisfies it and a scripted KeySource need not.
type gateTerminal interface {
	Suspend()
	Resume() error
}

// gateOutcome folds deps.Gate's answer into the callers' vocabulary.
func gateOutcome(ok bool, err error) wizardDepGateOutcome {
	if err != nil {
		return wizardDepGateOutcome{Err: err}
	}
	return wizardDepGateOutcome{Proceed: ok}
}

// gateRequest is the part both adapters share: the recipe the caller already
// read, the console (stderr — every panel and offer line), the offer's TTY
// guard, and the streams an install child inherits. The probe, the package
// runner and the provider install keep their production defaults.
func gateRequest(home string, stdin io.Reader, stdout, stderr io.Writer, recipe *schema.Recipe) deps.GateRequest {
	out := stderr
	if out == nil {
		out = io.Discard
	}
	return deps.GateRequest{
		Recipe: recipe,
		Home:   home,
		Out:    out,
		TTY:    isTTY(stdin, stdout),
		Stdin:  stdin,
		Stdout: stdout,
	}
}

// initDepGate is the init wizard's adapter: the raw-mode window is the
// wizard's own, so the gate asks over its live keys and hands the terminal to
// each install child.
func initDepGate(req initWizardRequest, keys tui.KeySource, recipe *schema.Recipe) wizardDepGateOutcome {
	if keys == nil {
		return wizardDepGateOutcome{Err: errors.New("the CLI-dep gate has no terminal to prompt on")}
	}
	request := gateRequest(req.Home, req.Stdin, req.Stdout, req.Stderr, recipe)
	request.Prompt = func(message string, def bool) (bool, bool) {
		return tui.Confirm(req.Stdout, keys, message, def)
	}
	if terminal, ok := keys.(gateTerminal); ok {
		request.Suspend = terminal.Suspend
		request.Resume = terminal.Resume
	}
	return gateOutcome(deps.Gate(request))
}

// recipeDepGate is the `recipe add` and `configure-recipes` adapter: no window
// is open when the gate runs, so it opens one for its own questions and closes
// it on the way out — the same per-prompt takeover the sibling confirms use.
func recipeDepGate(home string, stdin io.Reader, stdout, stderr io.Writer, recipe *schema.Recipe) wizardDepGateOutcome {
	file, ok := stdin.(*os.File)
	if !ok {
		return wizardDepGateOutcome{Err: errors.New("the CLI-dep gate needs a terminal stdin")}
	}
	terminal, err := tui.NewTerminal(file)
	if err != nil {
		return wizardDepGateOutcome{Err: fmt.Errorf("the CLI-dep gate cannot open the terminal: %v", err)}
	}
	defer terminal.Close()
	request := gateRequest(home, stdin, stdout, stderr, recipe)
	request.Prompt = func(message string, def bool) (bool, bool) {
		return tui.Confirm(stdout, terminal, message, def)
	}
	return gateOutcome(deps.Gate(request))
}
