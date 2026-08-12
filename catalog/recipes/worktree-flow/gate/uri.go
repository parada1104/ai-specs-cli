package main

import "strings"

// uriSchemes is the twelve-scheme internal harness allowlist
// (worktree-gate-legacy.sh:340): known internal tool interfaces are not Git
// destinations, so they bypass filesystem classification in PATH mode.
var uriSchemes = []string{
	"xd://", "skill://", "rule://", "agent://", "history://",
	"artifact://", "local://", "vault://", "mcp://", "issue://",
	"pr://", "omp://",
}

// IsInternalURI mirrors the URI case statement in resolve_and_check
// (worktree-gate-legacy.sh:339-352): a known scheme bypasses classification
// only in PATH mode, and only when it cannot resolve into the repository —
// candidates masked by ../ traversal or by an absolute path after the scheme
// are filesystem paths wearing a URI prefix and must be classified normally.
// In SHELL mode every candidate is a literal write target, so a URI prefix
// never bypasses classification (task 1.12, spec line 36).
func IsInternalURI(candidate, mode string) bool {
	if mode != "path" {
		return false
	}
	known := false
	for _, scheme := range uriSchemes {
		if strings.HasPrefix(candidate, scheme) {
			known = true
			break
		}
	}
	if !known {
		return false
	}
	if strings.Contains(candidate, "/../") || strings.HasSuffix(candidate, "/..") {
		return false // traversal-masked path: classify normally
	}
	rest := candidate[strings.Index(candidate, "://")+3:]
	if strings.HasPrefix(rest, "/") {
		return false // absolute-path-masked: classify normally
	}
	return true // genuine internal URI: bypass classification
}
