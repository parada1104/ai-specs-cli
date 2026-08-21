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

// AskMessage builds the stderr guidance for gate_mode=ask. The write is still
// refused (exit 2) but the agent is directed to ask the user which destination
// to use, never to self-bypass. Unlike the legacy AskHint it does NOT advertise
// the WORKTREE_GATE_MODE=off escape hatch: option 3 (write on the protected
// branch) is regulated by the skill, not by the gate message.
func AskMessage(shell bool, toolName, candidate, branch string) string {
	prefix := fmt.Sprintf("refusing to %s '%s' on protected branch '%s' in the main worktree", defaultTool(toolName), candidate, branch)
	if shell {
		prefix = fmt.Sprintf("refusing shell command that writes '%s' on protected branch '%s' in the main worktree", candidate, branch)
	}
	return fmt.Sprintf(
		"worktree-gate: %s. Ask the user where to put this work: (1) create a dedicated worktree (recommended), (2) create a feature branch here, or (3) write on the protected branch with the user's explicit override.",
		prefix,
	)
}

func defaultTool(toolName string) string {
	if toolName == "" {
		return "edit"
	}
	return toolName
}
