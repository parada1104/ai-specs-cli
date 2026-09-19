package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestResolveLedgerModeTable pins the effective tracker ledger-mode resolution
// contract that today lives in the shell `_ledger_mode` helper
// (catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh). Precedence,
// strongest first:
//
//  1. a valid TRACKER_LEDGER_MODE env value (always|ask|warn);
//  2. a valid bound-recipe configured ledger_mode (always|ask|warn);
//  3. the bound-recipe configured gate_mode, which maps off->off, always->always,
//     and warn or any invalid/absent value ->warn (an invalid configured gate
//     still shadows the raw legacy env);
//  4. otherwise the raw legacy TRACKER_CARD_GATE_MODE env, validated against
//     off|warn|always and falling back to the stamped hint;
//  5. otherwise the stamped legacy gate hint (off|warn|always);
//  6. otherwise the warn-first default.
//
// Worktree gate mode is a separate, independent resolution (ResolveGateMode); its
// always-on default never leaks into the ledger mode.
func TestResolveLedgerModeTable(t *testing.T) {
	cases := []struct {
		name          string
		envLedger     string
		envLegacyGate string
		configured    string
		configuredGat string
		stampedGate   string
		want          string
		wantWarnSub   string // non-empty: expected substring on warn
		wantNoWarn    bool   // true: warn must stay empty
	}{
		// 1. Valid env ledger wins over everything.
		{"env ledger always wins over configured ledger and gates", "always", "off", "warn", "off", "off", "always", "", true},
		{"env ledger ask wins over configured ledger always", "ask", "always", "always", "always", "always", "ask", "", true},
		{"env ledger off wins over all", "off", "always", "always", "always", "always", "off", "", true},
		{"invalid env ledger falls through to configured ledger", "bogus", "", "always", "", "", "always", "", false},

		// 2. Configured ledger wins over configured gate and raw legacy env.
		{"configured ledger ask wins over configured gate and legacy", "", "always", "ask", "off", "off", "ask", "", true},
		{"configured ledger always wins over legacy off", "", "off", "always", "off", "off", "always", "", true},
		{"configured ledger warn wins over legacy off", "", "off", "warn", "off", "off", "warn", "", true},
		{"invalid configured ledger falls through to configured gate", "", "", "bogus", "off", "always", "off", "", false},

		// 3. Configured gate maps off/always/warn and shadows the raw legacy env.
		{"configured gate off maps to off", "", "always", "", "off", "always", "off", "", true},
		{"configured gate always maps to always", "", "off", "", "always", "off", "always", "", true},
		{"configured gate warn maps to warn", "", "off", "", "warn", "always", "warn", "", true},
		{"invalid configured gate maps to warn and shadows legacy env", "", "off", "", "bogus", "always", "warn", "", false},

		// 4. Raw legacy env beats the stamped hint when no configured gate.
		{"raw legacy env off beats stamped always", "", "off", "", "", "always", "off", "", true},
		{"raw legacy env always beats stamped off", "", "always", "", "", "off", "always", "", true},
		{"raw legacy env warn beats stamped always", "", "warn", "", "", "always", "warn", "", true},

		// 5. Stamped hint is used when the raw legacy env is unset.
		{"stamped off used when no env", "", "", "", "", "off", "off", "", true},
		{"stamped always used when no env", "", "", "", "", "always", "always", "", true},
		{"stamped warn used when no env", "", "", "", "", "warn", "warn", "", true},

		// 6. Invalid values and the fully-unset default fall back to warn.
		{"invalid raw legacy env falls back to valid stamped", "", "bogus", "", "", "always", "always", "bogus", false},
		{"invalid raw legacy env falls back to default warn when stamped empty", "", "bogus", "", "", "", "warn", "bogus", false},
		{"invalid stamped falls back to default warn", "", "", "", "", "bogus", "warn", "bogus", false},
		{"all unset defaults to warn", "", "", "", "", "", "warn", "", true},
		{"invalid raw legacy env and invalid stamped default to warn", "", "bogus", "", "", "nope", "warn", "bogus", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var warn bytes.Buffer
			got := ResolveLedgerMode(tc.envLedger, tc.envLegacyGate, tc.configured, tc.configuredGat, tc.stampedGate, &warn)
			if got != tc.want {
				t.Fatalf(
					"ResolveLedgerMode(envLedger=%q, envLegacyGate=%q, configuredLedger=%q, configuredGate=%q, stampedGate=%q) = %q, want %q",
					tc.envLedger, tc.envLegacyGate, tc.configured, tc.configuredGat, tc.stampedGate, got, tc.want,
				)
			}
			if tc.wantWarnSub != "" && !strings.Contains(warn.String(), tc.wantWarnSub) {
				t.Fatalf("warning = %q, want substring %q", warn.String(), tc.wantWarnSub)
			}
			if tc.wantNoWarn && warn.Len() != 0 {
				t.Fatalf("unexpected warning %q", warn.String())
			}
		})
	}
}

// TestResolveLedgerModeIndependentOfWorktreeMode pins that ledger mode resolution
// is not the worktree gate resolution: the same invalid stamped value yields the
// ledger warn default, while the worktree resolver falls back to always. If the
// ledger resolver ever delegated to the worktree default, the ledger mode would
// silently become always and this test would fail.
func TestResolveLedgerModeIndependentOfWorktreeMode(t *testing.T) {
	var ledgerWarn bytes.Buffer
	if got := ResolveLedgerMode("", "", "", "", "bogus", &ledgerWarn); got != "warn" {
		t.Fatalf("ResolveLedgerMode with invalid stamped gate = %q, want the ledger warn default", got)
	}

	var worktreeWarn bytes.Buffer
	if got := ResolveGateMode("", "bogus", &worktreeWarn); got != "always" {
		t.Fatalf("ResolveGateMode with invalid stamped gate = %q, want the worktree always default", got)
	}
}
