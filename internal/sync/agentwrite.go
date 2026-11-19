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
func projectTreePreflight(root, home string, targets []string, errW io.Writer) int {
	// Project-tree destinations follow the project rule; the CLI cache roots the
	// strict home rule (ai-specs owns them outright).
	type rootCheck struct {
		guard func(root, path string, followLeaf bool) error
		root  string
		path  string
	}
	var checks []rootCheck
	add := func(guard func(string, string, bool) error, root, path string) {
		checks = append(checks, rootCheck{guard: guard, root: root, path: path})
	}
	add(projectWriteGuard, root, filepath.Join(root, "ai-specs", ".gitignore"))
	add(projectWriteGuard, root, filepath.Join(root, ".gitignore"))
	add(projectWriteGuard, root, harnessEnvExamplePath(root))
	add(projectWriteGuard, root, RootEnvRcPath(root))
	add(projectWriteGuard, root, filepath.Join(root, "AGENTS.md"))
	// The orphan cleanup deletes `<root>/<name>` under these trees, so a
	// redirected root must refuse before the materialize step runs (T6e).
	add(projectWriteGuard, root, filepath.Join(root, "ai-specs", ".deps"))
	for _, target := range targets {
		add(projectWriteGuard, root, filepath.Join(target, "ai-specs"))
	}
	homeReal := projectcache.AISpecsHome(home)
	// CacheRoot RESOLVES the project-key directory, so build the lexical path:
	// a symlink there is exactly what must refuse.
	cacheKey := filepath.Join(homeReal, "cache", "projects", projectcache.CacheKey(root))
	add(homeWriteGuard, homeReal, filepath.Join(cacheKey, ".deps"))
	add(homeWriteGuard, homeReal, filepath.Join(cacheKey, ".recipe"))
	for _, c := range checks {
		if err := c.guard(c.root, c.path, true); err != nil {
			fmt.Fprintf(errW, "    ✗ %s\n", err)
			return 1
		}
	}
	return 0
}
