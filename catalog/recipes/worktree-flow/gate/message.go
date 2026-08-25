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

// AskHint mirrors the legacy shell hook byte for byte: in ask mode the
// block message is followed by a hint on how to bypass the gate.
//
// The hint names WHERE to set the variable on purpose. main.go reads it with
// os.Getenv, i.e. from this hook's own environment — and a PreToolUse hook
// runs BEFORE the command it is inspecting, receiving that command as a
// string. So an inline `WORKTREE_GATE_MODE=off <cmd>` prefix can never reach
// it: the assignment only exists for the process <cmd> would spawn, and <cmd>
// never runs. The earlier wording ("re-run with WORKTREE_GATE_MODE=off") read
// as an inline prefix and sent agents in circles.
func AskHint() string {
	return "worktree-gate: to bypass, set WORKTREE_GATE_MODE=off in the environment that launches the agent, then retry. An inline `WORKTREE_GATE_MODE=off <command>` prefix does NOT work: this hook runs before <command> and reads its own environment."
}
