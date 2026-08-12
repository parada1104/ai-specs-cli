package main

import (
	"fmt"
	"io"
)

// ResolveGateMode mirrors _resolve_gate_mode (worktree-gate-legacy.sh:35-47):
// the env override beats the stamped sync value; an invalid env value warns
// and falls back to the stamped value; an invalid stamped value warns and
// falls back to "always". Warnings go to the provided writer verbatim.
func ResolveGateMode(envOverride, stamped string, warn io.Writer) string {
	candidate := envOverride
	if candidate == "" {
		candidate = stamped
	}
	switch candidate {
	case "always", "ask", "off":
		return candidate
	}
	if envOverride != "" {
		fmt.Fprintf(warn, "worktree-gate: ignoring invalid WORKTREE_GATE_MODE='%s'; falling back to stamped mode.\n", envOverride)
	} else if stamped != "always" && stamped != "ask" && stamped != "off" {
		fmt.Fprintf(warn, "worktree-gate: invalid stamped gate_mode='%s'; falling back to always.\n", stamped)
	}
	switch stamped {
	case "always", "ask", "off":
		return stamped
	}
	return "always"
}

// ResolveGateScope mirrors _resolve_gate_scope (worktree-gate-legacy.sh:50-62):
// a valid env override wins; an invalid env override warns and falls back to
// the stamped value; an invalid stamped value warns and falls back to "auto".
func ResolveGateScope(envOverride, stamped string, warn io.Writer) string {
	if envOverride != "" {
		switch envOverride {
		case "auto", "superrepo", "subrepo":
			return envOverride
		}
		fmt.Fprintf(warn, "worktree-gate: invalid WORKTREE_GATE_SCOPE='%s'; falling back to stamped scope.\n", envOverride)
	}
	switch stamped {
	case "auto", "superrepo", "subrepo":
		return stamped
	}
	fmt.Fprintf(warn, "worktree-gate: missing or invalid stamped gate_scope='%s'; falling back to auto.\n", stamped)
	return "auto"
}

// ResolveRepoTopology mirrors _resolve_repo_topology (worktree-gate-legacy.sh:67-72).
// Topology is stamped-only by design (decision D17): there is no env override,
// and an invalid stamped value warns and falls back to "auto".
func ResolveRepoTopology(stamped string, warn io.Writer) string {
	switch stamped {
	case "auto", "standalone", "monorepo-apps", "monorepo-submodules":
		return stamped
	}
	fmt.Fprintf(warn, "worktree-gate: missing or invalid stamped repo_topology='%s'; falling back to auto.\n", stamped)
	return "auto"
}
