package sync

// Write containment for agent destinations (T6c), extended to the project tree
// and the CLI caches (T6d). The mechanism lives in internal/projectcache
// (ContainWrite/ResolveWriteTarget) so the sync package and projectcache share
// one implementation; these wrappers pin the three policies.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/projectcache"
	shared "ai-specs.dev/worktree-gate/shared"
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

// cachePathGuard applies homeWriteGuard to a destination the materializer
// derives from the CLI home. A caller with no home (unit seams) or with a
// destination outside it is not a CLI-cache write, so it is left alone.
func cachePathGuard(home, path string) error {
	root := projectcache.AISpecsHome(home)
	if root == "" || !shared.TemplatePathContained(root, path) {
		return nil
	}
	return homeWriteGuard(root, path, true)
}

// cacheEntryGuard is homeWriteGuard for a cache directory or entry another run
// may create or remove concurrently: a path that has vanished cannot redirect a
// write, so that transient listing race is not a refusal.
func cacheEntryGuard(root, path string, followLeaf bool) error {
	err := homeWriteGuard(root, path, followLeaf)
	if err == nil {
		return nil
	}
	if _, statErr := os.Lstat(path); os.IsNotExist(statErr) {
		return nil
	}
	return err
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
	// Every ai-specs-managed directory under ai-specs/ must resolve inside the
	// project before any step runs: leftover, recipe-command, legacy-origin and
	// recipe-artifact cleanup all delete or write beneath them (T6f).
	managedDirs := []string{
		"commands", "skills", "bin", "recipes", ".deps", ".recipe",
		".resolved-skills", ".internal",
	}
	for _, target := range targets {
		aiSpecs := filepath.Join(target, "ai-specs")
		add(projectWriteGuard, root, aiSpecs)
		for _, name := range managedDirs {
			add(projectWriteGuard, root, filepath.Join(aiSpecs, name))
		}
		for _, recipe := range managedRecipeDirs(filepath.Join(aiSpecs, "recipes")) {
			add(projectWriteGuard, root, recipe)
		}
	}
	homeReal := projectcache.AISpecsHome(home)
	// CacheRoot RESOLVES the project-key directory, so build the lexical path:
	// a symlink there is exactly what must refuse.
	cacheKey := filepath.Join(homeReal, "cache", "projects", projectcache.CacheKey(root))
	add(homeWriteGuard, homeReal, filepath.Join(cacheKey, ".deps"))
	add(homeWriteGuard, homeReal, filepath.Join(cacheKey, ".recipe"))
	add(homeWriteGuard, homeReal, filepath.Join(cacheKey, "resolved-skills"))
	// Materialization copies a live recipe command into `<cache>/commands` and
	// refreshes a gate snapshot under `<cache>/backups`: both follow a symlink
	// leaf or ancestor, so a redirected entry refuses before the first write.
	for _, dir := range []string{
		filepath.Join(cacheKey, "commands"),
		filepath.Join(cacheKey, "backups"),
	} {
		add(cacheEntryGuard, homeReal, dir)
		for _, entry := range dirEntries(dir) {
			add(cacheEntryGuard, homeReal, entry)
		}
	}
	// The per-skill orphan prune deletes `<cache>/.recipe/<recipe>/skills/<skill>`
	// (T7), so a redirected recipe directory must refuse before the first write
	// rather than at the orphan step. Only DIRECTORY entries (and links that
	// resolve to one) are covered: a link to an unrelated file (e.g. a
	// `.recipe/README.md`) is never mutated and must not block the run.
	for _, recipe := range managedRecipeDirs(filepath.Join(cacheKey, ".recipe")) {
		add(homeWriteGuard, homeReal, recipe)
		add(homeWriteGuard, homeReal, filepath.Join(recipe, "skills"))
	}
	// Deletion authority for the per-skill prune is the recipe's ownership
	// marker: a marker that is not a REGULAR file is never trusted and refuses
	// before any delete or write (S1).
	if err := requireRegularRecipeSkillMarkers(filepath.Join(cacheKey, ".recipe")); err != nil {
		fmt.Fprintf(errW, "    ✗ %s\n", err)
		return 1
	}
	for _, c := range checks {
		if err := c.guard(c.root, c.path, true); err != nil {
			fmt.Fprintf(errW, "    ✗ %s\n", err)
			return 1
		}
	}
	return 0
}

// cachePreflight refuses a redirected CLI-cache recipe or resolved-skills root
// BEFORE a sync-agent run's first write (S1): the materialize fallback copies
// skill copies under `.recipe`, writes the ownership record next to them, and
// the flatten wipes `resolved-skills` outright. Both roots belong to ai-specs,
// so any symlink below the CLI home is a refusal.
func cachePreflight(root, home string, errW io.Writer) int {
	homeReal := projectcache.AISpecsHome(home)
	if homeReal == "" {
		return 0 // the cache steps report the missing home themselves
	}
	cacheKey := filepath.Join(homeReal, "cache", "projects", projectcache.CacheKey(root))
	checks := []string{
		filepath.Join(cacheKey, ".recipe"),
		filepath.Join(cacheKey, "resolved-skills"),
		filepath.Join(cacheKey, "commands"),
		filepath.Join(cacheKey, "backups"),
	}
	// The fallback materializer replaces a bundled skill through
	// `<recipe>/skills/<skill>`: a redirected skills directory would make its
	// RemoveAll recurse into the link target (JD-A-002).
	for _, recipe := range managedRecipeDirs(filepath.Join(cacheKey, ".recipe")) {
		checks = append(checks, recipe, filepath.Join(recipe, "skills"))
	}
	for _, dir := range []string{
		filepath.Join(cacheKey, "commands"),
		filepath.Join(cacheKey, "backups"),
	} {
		checks = append(checks, dirEntries(dir)...)
	}
	for _, path := range checks {
		if err := cacheEntryGuard(homeReal, path, true); err != nil {
			fmt.Fprintf(errW, "    ✗ %s\n", err)
			return 1
		}
	}
	if err := requireRegularRecipeSkillMarkers(filepath.Join(cacheKey, ".recipe")); err != nil {
		fmt.Fprintf(errW, "    ✗ %s\n", err)
		return 1
	}
	return 0
}

// requireRegularRecipeSkillMarkers refuses a non-regular per-recipe ownership
// marker anywhere under the cache's `.recipe` root, so the whole run is
// write-free instead of refusing after the prune (S1).
func requireRegularRecipeSkillMarkers(recipeRoot string) error {
	for _, recipe := range managedRecipeDirs(recipeRoot) {
		if err := requireRegularRecipeSkillMarker(filepath.Join(recipe, recipeSkillRecordName)); err != nil {
			return err
		}
	}
	return nil
}

// dirEntries lists the direct children of dir (files and links included).
// An unreadable or missing directory has none.
func dirEntries(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return paths
}

// managedRecipeDirs lists the recipe directories under ai-specs/recipes so the
// preflight can contain each one (a symlinked `recipes/<id>` is managed). Only
// DIRECTORY entries count: cleanup and migration delete or write beneath
// recipe dirs, never through a file link nobody mutates (e.g. a
// `recipes/README.md` symlink), which must not block the run.
func managedRecipeDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(dir, entry.Name()))
			continue
		}
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		// A link counts as a recipe dir when it resolves to one; a link to a file
		// or a dangling link is not a recipe directory.
		path := filepath.Join(dir, entry.Name())
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}
