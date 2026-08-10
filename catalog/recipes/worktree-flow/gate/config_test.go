package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolveGateModeTable(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		stamped string
		want    string
		warn    string // substring expected on stderr; empty = no warning
	}{
		{"env overrides stamped", "off", "always", "off", ""},
		{"env ask", "ask", "always", "ask", ""},
		{"no env uses stamped", "", "ask", "ask", ""},
		{"invalid env falls back to valid stamped", "bogus", "always", "always", "ignoring invalid WORKTREE_GATE_MODE='bogus'"},
		{"invalid env falls back to invalid stamped then always", "bogus", "nope", "always", "ignoring invalid WORKTREE_GATE_MODE='bogus'; falling back to stamped mode."},
		{"invalid stamped falls back to always", "", "nope", "always", "invalid stamped gate_mode='nope'; falling back to always."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var warn bytes.Buffer
			got := ResolveGateMode(tc.env, tc.stamped, &warn)
			if got != tc.want {
				t.Fatalf("ResolveGateMode(%q, %q) = %q, want %q", tc.env, tc.stamped, got, tc.want)
			}
			if tc.warn == "" {
				if warn.Len() != 0 {
					t.Fatalf("unexpected warning %q", warn.String())
				}
			} else if !strings.Contains(warn.String(), tc.warn) {
				t.Fatalf("warning = %q, want substring %q", warn.String(), tc.warn)
			}
		})
	}
}

func TestResolveGateScopeTable(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		stamped string
		want    string
		warn    string
	}{
		{"valid env wins", "superrepo", "subrepo", "superrepo", ""},
		{"no env uses stamped", "", "subrepo", "subrepo", ""},
		{"invalid env falls back to stamped", "bogus", "auto", "auto", "invalid WORKTREE_GATE_SCOPE='bogus'; falling back to stamped scope."},
		{"invalid stamped falls back to auto", "", "bogus", "auto", "missing or invalid stamped gate_scope='bogus'; falling back to auto."},
		{"empty stamped falls back to auto", "", "", "auto", "missing or invalid stamped gate_scope=''; falling back to auto."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var warn bytes.Buffer
			got := ResolveGateScope(tc.env, tc.stamped, &warn)
			if got != tc.want {
				t.Fatalf("ResolveGateScope(%q, %q) = %q, want %q", tc.env, tc.stamped, got, tc.want)
			}
			if tc.warn == "" {
				if warn.Len() != 0 {
					t.Fatalf("unexpected warning %q", warn.String())
				}
			} else if !strings.Contains(warn.String(), tc.warn) {
				t.Fatalf("warning = %q, want substring %q", warn.String(), tc.warn)
			}
		})
	}
}

func TestResolveRepoTopologyStampedOnly(t *testing.T) {
	valid := []string{"auto", "standalone", "monorepo-apps", "monorepo-submodules"}
	for _, v := range valid {
		var warn bytes.Buffer
		if got := ResolveRepoTopology(v, &warn); got != v {
			t.Fatalf("ResolveRepoTopology(%q) = %q, want %q", v, got, v)
		}
		if warn.Len() != 0 {
			t.Fatalf("unexpected warning for valid %q: %q", v, warn.String())
		}
	}
	var warn bytes.Buffer
	if got := ResolveRepoTopology("bogus", &warn); got != "auto" {
		t.Fatalf("ResolveRepoTopology(bogus) = %q, want auto", got)
	}
	if !strings.Contains(warn.String(), "invalid stamped repo_topology='bogus'; falling back to auto") {
		t.Fatalf("warning = %q, want invalid-topology fallback warning", warn.String())
	}
}
