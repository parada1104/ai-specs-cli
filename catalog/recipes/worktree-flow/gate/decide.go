package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Decision is the verdict for one candidate write target. A block carries the
// protected branch name that triggered it (mirrors "block:<branch>").
type Decision struct {
	Allow  bool
	Branch string
}

// allow is the fail-open verdict: any lookup or classification failure allows
// the action (a buggy guard must never wedge editing).
var allow = Decision{Allow: true}

// Decide mirrors the embedded Python resolver in resolve_and_check
// (worktree-gate-legacy.sh:365-522): probe the existing ancestor, require the
// canonical target to live inside the repository's primary checkout, require a
// protected branch, then apply the owner × scope decision with the
// openspec/changes central-path exception. Every internal error fails open.
func Decide(candidate string, cwd string, scope, topology string, protected []string) Decision {
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	canonical := RealPath(candidate)
	ancestor := ExistingAncestor(candidate)
	if ancestor == "" {
		return allow
	}
	repoRoot := gitMemo(ancestor, "rev-parse", "--show-toplevel")
	if repoRoot == "" {
		return allow
	}
	repoRoot = RealPath(repoRoot)
	if !Inside(canonical, repoRoot) {
		return allow
	}
	gitDir := gitMemo(ancestor, "rev-parse", "--absolute-git-dir")
	common := gitCommon(ancestor)
	if gitDir == "" || common == "" {
		return allow
	}
	if RealPath(gitDir) != common {
		// Linked worktree: writes there are always allowed.
		return allow
	}
	branch := gitMemo(ancestor, "symbolic-ref", "--short", "HEAD")
	if branch == "" || !containsString(protected, branch) {
		return allow
	}
	owner := classify(repoRoot, common, topology)
	central := RealPath(filepath.Join(repoRoot, "openspec", "changes"))
	switch owner {
	case ownerSuper:
		// Explicit subrepo scope intentionally leaves superrepo writes to the
		// caller (Melón central-planning workflow); central paths remain an
		// explicit exception for the enforcing scopes.
		if scope == "subrepo" || Inside(canonical, central) {
			return allow
		}
		return Decision{Allow: false, Branch: branch}
	case ownerSub:
		if scope == "superrepo" {
			return allow
		}
		return Decision{Allow: false, Branch: branch}
	default:
		return Decision{Allow: false, Branch: branch}
	}
}

// IsClaudeException mirrors the two case statements in resolve_and_check
// (worktree-gate-legacy.sh:357-363): local gitignored agent config is always
// allowed, matched against the raw candidate and the absolutized path.
func IsClaudeException(rawCandidate, abs string) bool {
	if matchesClaude(abs) || matchesClaude(rawCandidate) {
		return true
	}
	return false
}

func matchesClaude(p string) bool {
	return strings.HasSuffix(p, "/.claude/settings.json") ||
		strings.HasSuffix(p, "/.claude/settings.local.json") ||
		strings.HasSuffix(p, "/.claude/settings") ||
		strings.HasSuffix(p, "/.claude/hooks/") ||
		strings.Contains(p, "/.claude/hooks/")
}

// classifyRepoOwner is a convenience wrapper over classify for callers that
// only need the owner verdict (unit tests).
func classifyRepoOwner(repoRoot, common, topology string) ownerKind {
	return classify(repoRoot, common, topology)
}

func containsString(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// IsExistingDirectory mirrors the event-cwd validation (worktree-gate-legacy.sh:303):
// only an absolute, existing directory is a usable resolution base.
func IsExistingDirectory(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
