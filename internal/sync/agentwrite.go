package sync

// Write containment for agent destinations (T6c), extended to the project tree
// and the CLI caches (T6d). The mechanism lives in internal/projectcache
// (ContainWrite/ResolveWriteTarget) so the sync package and projectcache share
// one implementation; these wrappers pin the three policies.

import (
	"fmt"
	"io"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// protectedTrees are never a valid agent destination, even inside the root:
// `ai-specs/` is the reconciled project tree and `.git/` the repository.
var protectedTrees = []string{"ai-specs", ".git"}

// agentWriteGuard contains an agent destination: a symlink that resolves inside
// the project root is allowed, and the protected `ai-specs/` / `.git/` trees are
// refused (agents never write there).
func agentWriteGuard(root, path string, followLeaf bool) error {
	return projectcache.ContainWrite(root, path, followLeaf, false, true)
}

// projectWriteGuard contains a project-tree write or delete (the ai-specs
// mirror, .gitignore and env scaffolding, AGENTS.md): links that resolve inside
// the project root are allowed; `ai-specs/` is a legitimate destination.
func projectWriteGuard(root, path string, followLeaf bool) error {
	return projectcache.ContainWrite(root, path, followLeaf, false, false)
}

// homeWriteGuard contains a cache or lock path in the CLI home: any symlink
// below that home refuses, because ai-specs owns those paths outright.
func homeWriteGuard(root, path string, followLeaf bool) error {
	return projectcache.ContainWrite(root, path, followLeaf, true, false)
}

// projectTreePreflight refuses the run before its first step when a
// project-tree destination is redirected outside the project (T6d): the
// scaffolding files the env step manages, the ai-specs/.gitignore, the brief,
// and every fan-out target's ai-specs/ mirror. Paths come from the existing
// helpers so the names live in one place.
func projectTreePreflight(root string, targets []string, errW io.Writer) int {
	paths := []string{
		filepath.Join(root, "ai-specs", ".gitignore"),
		filepath.Join(root, ".gitignore"),
		harnessEnvExamplePath(root),
		RootEnvRcPath(root),
		filepath.Join(root, "AGENTS.md"),
	}
	for _, target := range targets {
		paths = append(paths, filepath.Join(target, "ai-specs"))
	}
	for _, path := range paths {
		if err := projectWriteGuard(root, path, true); err != nil {
			fmt.Fprintf(errW, "    ✗ %s\n", err)
			return 1
		}
	}
	return 0
}
