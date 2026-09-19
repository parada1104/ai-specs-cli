package main

import (
	"fmt"
	"io"
	"os"

	"ai-specs.dev/worktree-gate/ledger"
)

// ledgerModeOff is the sentinel effective mode that disables the tracker ledger
// at a checkpoint. `off` is deliberately not one of ledger.LedgerModes: it never
// reaches the grader, it short-circuits before grading (the shell behavior this
// resolver takes ownership of).
const ledgerModeOff = "off"

// ResolveLedgerMode resolves the effective tracker ledger mode from already
// acquired inputs, in the exact precedence the shell `_ledger_mode` helper used:
//
//  1. a valid TRACKER_LEDGER_MODE env value (always|ask|warn);
//  2. a valid bound-recipe configured ledger_mode (always|ask|warn);
//  3. the bound-recipe configured gate_mode, which maps off->off, always->always,
//     and warn or any invalid value ->warn. A present configured gate shadows the
//     raw legacy env even when its value is invalid;
//  4. a valid raw legacy TRACKER_CARD_GATE_MODE env value (off|warn|always);
//  5. a valid stamped legacy gate hint (off|warn|always);
//  6. otherwise the warn-first default.
//
// Invalid legacy inputs (the env value and the stamped hint) warn and fall
// through; invalid ledger inputs fall through silently, which is the pinned
// legacy behavior. Worktree gate mode is a separate resolution (ResolveGateMode)
// and is never consulted here.
func ResolveLedgerMode(envLedger, envLegacyGate, configuredLedger, configuredGate, stampedGate string, warn io.Writer) string {
	if validLedgerMode(envLedger) {
		return envLedger
	}
	if validLedgerMode(configuredLedger) {
		return configuredLedger
	}
	if configuredGate != "" {
		switch configuredGate {
		case ledgerModeOff:
			return ledgerModeOff
		case ledger.ModeAlways:
			return ledger.ModeAlways
		default:
			// warn and any invalid value map to warn. The configured key is
			// authoritative: its presence shadows the raw legacy env and hint.
			return ledger.ModeWarn
		}
	}
	if validLegacyGateMode(envLegacyGate) {
		return envLegacyGate
	}
	if envLegacyGate != "" {
		fmt.Fprintf(warn, "worktree-gate: ledger: ignoring invalid TRACKER_CARD_GATE_MODE='%s'; falling back.\n", envLegacyGate)
	}
	if validLegacyGateMode(stampedGate) {
		return stampedGate
	}
	if stampedGate != "" {
		fmt.Fprintf(warn, "worktree-gate: ledger: invalid stamped ledger gate_mode='%s'; falling back to warn.\n", stampedGate)
	}
	return ledger.ModeWarn
}

// validLedgerMode reports whether m is a valid ledger-mode input. Only the three
// grader modes qualify: `off` is not a ledger mode, it is a mapping of the
// configured or legacy gate_mode (precedence 3-5), so an `off` ledger env or
// configured value is invalid input that falls through.
func validLedgerMode(m string) bool {
	switch m {
	case ledger.ModeAlways, ledger.ModeAsk, ledger.ModeWarn:
		return true
	}
	return false
}

// validLegacyGateMode reports whether m is one of the legacy gate vocabulary
// values. `ask` is a ledger mode, never a legacy gate value.
func validLegacyGateMode(m string) bool {
	switch m {
	case ledgerModeOff, ledger.ModeWarn, ledger.ModeAlways:
		return true
	}
	return false
}

// resolveLedgerMode acquires the configured and legacy inputs for a project root
// and resolves the effective mode. Configuration comes through the existing
// manifest acquisition seam (readManifestRecipeConfig + configValue); no TOML is
// parsed here. A missing recipe, manifest or config value simply leaves the
// configured inputs empty and the resolver falls through to env, hint and warn.
func resolveLedgerMode(dir string, binding ledger.Binding, stampedGate string, warn io.Writer) string {
	config, err := readManifestRecipeConfig(dir, binding.RecipeID)
	if err != nil {
		config = nil
	}
	configuredLedger, _ := configValue(config, "ledger_mode")
	// Presence of the gate_mode key matters even when its value is not a string:
	// the resolver must map a present-but-invalid value to warn and shadow the
	// raw legacy env. Pass the raw JSON token for a non-string so it resolves as
	// an invalid value instead of an absent key.
	configuredGate, gateOK := configValue(config, "gate_mode")
	if !gateOK {
		if raw, present := config["gate_mode"]; present {
			configuredGate = string(raw)
		}
	}
	return ResolveLedgerMode(
		os.Getenv("TRACKER_LEDGER_MODE"),
		os.Getenv("TRACKER_CARD_GATE_MODE"),
		configuredLedger,
		configuredGate,
		stampedGate,
		warn,
	)
}
