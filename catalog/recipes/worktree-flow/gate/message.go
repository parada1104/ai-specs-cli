package main

import "fmt"

func locationPhrase(commandCwd string, createWorktree bool) string {
	if commandCwd == "" {
		return "the main worktree"
	}
	if createWorktree {
		return fmt.Sprintf("the main worktree (%s)", commandCwd)
	}
	return commandCwd
}

// BlockMessage builds the stderr message for a blocked candidate.
// createWorktree keeps the legacy /worktree-new sentence and names commandCwd.
// When createWorktree is false, stderr names the absolute command cwd and MUST
// NOT instruct creating another worktree.
func BlockMessage(shell bool, toolName, candidate, branch, commandCwd string, createWorktree bool) string {
	loc := locationPhrase(commandCwd, createWorktree)
	if shell {
		msg := fmt.Sprintf("worktree-gate: refusing shell command that writes '%s' on protected branch '%s' in %s — using bash/shell to write here bypasses the worktree gate.", candidate, branch, loc)
		if createWorktree {
			msg += " Create a dedicated worktree first (e.g. /worktree-new) and run there — exploration ends at the first write."
		}
		return msg
	}
	if toolName == "" {
		toolName = "edit"
	}
	msg := fmt.Sprintf("worktree-gate: refusing to %s '%s' on protected branch '%s' in %s.", toolName, candidate, branch, loc)
	if createWorktree {
		msg += " Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write."
	}
	return msg
}

// AskMessage builds the stderr guidance for gate_mode=ask. The write is still
// refused (exit 2) but the agent is directed to ask the user which destination
// to use, never to self-bypass. It does NOT advertise WORKTREE_GATE_MODE=off.
func AskMessage(shell bool, toolName, candidate, branch, commandCwd string, createWorktree bool) string {
	loc := locationPhrase(commandCwd, createWorktree)
	prefix := fmt.Sprintf("refusing to %s '%s' on protected branch '%s' in %s", defaultTool(toolName), candidate, branch, loc)
	if shell {
		prefix = fmt.Sprintf("refusing shell command that writes '%s' on protected branch '%s' in %s", candidate, branch, loc)
	}
	opt1 := "(1) create a dedicated worktree (recommended)"
	if !createWorktree {
		opt1 = fmt.Sprintf("(1) continue in %s", commandCwd)
	}
	return fmt.Sprintf(
		"worktree-gate: %s. Ask the user where to put this work: %s, (2) create a feature branch here, or (3) write on the protected branch with the user's explicit override.",
		prefix, opt1,
	)
}

// DegradeMessage is fail-open guidance when a relative write has no recoverable
// cwd. It MUST NOT advertise WORKTREE_GATE_MODE=off, "to bypass", or /worktree-new.
func DegradeMessage(mode string) string {
	core := "command cwd could not be recovered for a relative write; it was not classified against the host process cwd."
	if mode == "ask" {
		return "worktree-gate: " + core + " Ask the user if this write is intentional."
	}
	return "worktree-gate: warn: " + core
}

func defaultTool(toolName string) string {
	if toolName == "" {
		return "edit"
	}
	return toolName
}
