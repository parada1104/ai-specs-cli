package sync

// Write containment for agent destinations (T6c of sync-ownership).
//
// The agent preflight checks the LISTED destinations, but a write then follows
// the destination leaf (`.pi/mcp.json` may be a symlink) or a generated child
// (`.pi/extensions/<shim>.ts`, a file inside `.claude/commands/`). One guard
// resolves where a write would ACTUALLY land — following an existing leaf
// symlink and every ancestor — and refuses when that location is outside the
// resolved main project root, or inside the protected `ai-specs/` / `.git/`
// trees. Every agent-destination write and create in the sync package routes
// through it.

import (
	"fmt"
	"os"
	"path/filepath"

	shared "ai-specs.dev/worktree-gate/shared"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// protectedTrees are never an agent-destination write target, even inside the
// project: `ai-specs/` is the reconciled project tree and `.git/` the repository.
var protectedTrees = []string{"ai-specs", ".git"}

// agentWriteGuard refuses a write whose resolved location leaves the project.
// followLeaf follows an existing symlink at `path` itself (a real write through
// it); false checks only the parent chain, for writes that REPLACE the leaf —
// the agent skills and instruction links ai-specs owns.
func agentWriteGuard(projectRoot, path string, followLeaf bool) error {
	probe := path
	if !followLeaf {
		probe = filepath.Dir(path)
	}
	effective, err := resolveWriteTarget(probe)
	if err != nil {
		return fmt.Errorf("refuse to write %s: %v", path, err)
	}
	rootReal := projectcache.ResolvePath(projectRoot)
	if !shared.TemplatePathContained(rootReal, effective) {
		return fmt.Errorf("refuse to write %s: it resolves outside the project root (%s)", path, effective)
	}
	for _, tree := range protectedTrees {
		if shared.TemplatePathContained(filepath.Join(rootReal, tree), effective) {
			return fmt.Errorf("refuse to write %s: it resolves inside the protected %s/ tree (%s)", path, tree, effective)
		}
	}
	return nil
}

// resolveWriteTarget reports where a write to path would land: an existing
// symlink (leaf or ancestor) resolves to its target, and a not-yet-existing
// tail is re-joined onto the deepest existing ancestor. A link that exists but
// cannot be resolved — dangling or looping — is an error, never a guess.
func resolveWriteTarget(path string) (string, error) {
	cur := filepath.Clean(path)
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Join(append([]string{cur}, tail...)...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}
