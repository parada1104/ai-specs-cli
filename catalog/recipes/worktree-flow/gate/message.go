package main

import "fmt"

// BlockMessage builds the verbatim stderr message for a blocked candidate.
// The two variants mirror worktree-gate-legacy.sh:527-530 byte for byte
// (including the U+2014 em dash). The block path writes to the primary
// checkout of the main worktree, so the guidance is to move the write into a
// dedicated worktree.
func BlockMessage(shell bool, toolName, candidate, branch string) string {
	if shell {
		return fmt.Sprintf("worktree-gate: refusing shell command that writes '%s' on protected branch '%s' in the main worktree — using bash/shell to write here bypasses the worktree gate. Create a dedicated worktree first (e.g. /worktree-new) and run there — exploration ends at the first write.", candidate, branch)
	}
	if toolName == "" {
		toolName = "edit"
	}
	return fmt.Sprintf("worktree-gate: refusing to %s '%s' on protected branch '%s' in the main worktree. Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write.", toolName, candidate, branch)
}

// AskHint mirrors worktree-gate-legacy.sh:531-533: in ask mode the block
// message is followed by a hint that the invocation can be bypassed.
func AskHint() string {
	return "worktree-gate: to bypass for this invocation, re-run with WORKTREE_GATE_MODE=off"
}
