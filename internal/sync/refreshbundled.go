package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// bundledLockRel is LOCK_REL in refresh-bundled.py: "ai-specs/.ai-specs.lock".
const bundledLockRel = "ai-specs/.ai-specs.lock"

// RefreshBundled is the native port of lib/_internal/refresh-bundled.py's
// main(): a pure cache-repair verb. It flattens CLI-bundled skills and
// commands into the cache (never the project surface), removes leftover
// in-project copies from older CLIs, and normalizes the lock to the
// [meta]/[managed]/[deps]/[agents] writer shape. The Python module stays under
// lib/_internal/ as the differential oracle until U2e.
//
// Exit contract, quoted from the script: 0 on success; 1 with
// "ERROR: <project>/ai-specs not found. Run `ai-specs init` first." on stderr
// when the project surface is missing; 2 for a usage error (unreachable here,
// the sync spine always supplies both paths). A refresh failure keeps the
// partial cache of S6 — destinations are deleted before the copy, so a failed
// copy leaves the new tree partially written and no rollback happens.
func RefreshBundled(projectRoot, cliHome string, stdout, stderr io.Writer) int {
	project := projectcache.ResolvePath(projectRoot)
	cliSource := projectcache.ResolvePath(cliHome)

	if !isDirPath(filepath.Join(project, "ai-specs")) {
		fmt.Fprintf(stderr, "ERROR: %s/ai-specs not found. Run `ai-specs init` first.\n", project)
		return 1
	}

	// CLI-bundled skills and commands flatten into the cache — never into the
	// project surface.
	nSkills, err := flattenBundledSkills(project, cliSource)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err)
		return 1
	}
	nCommands, err := flattenBundledCommands(project, cliSource)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err)
		return 1
	}

	lockPath := filepath.Join(project, bundledLockRel)
	// The legacy lock is loaded BEFORE the cleanup so the [skills] hashes are
	// still in memory; write_lock below normalizes them away.
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err)
		return 1
	}

	aiSpecs := filepath.Join(project, "ai-specs")
	projectcache.RemoveBundledSkillLeftovers(aiSpecs, cliSource, bundledSkillHashes(lk.Skills), stdout, stderr)
	projectcache.RemoveBundledCommandLeftovers(aiSpecs, cliSource, nil, stdout, stderr)
	projectcache.RemoveRecipeCommandLeftovers(project, cliSource, nil, nil, stdout, stderr)

	if err := lock.WriteLock(lockPath, lk); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err)
		return 1
	}

	if nSkills > 0 {
		fmt.Fprintf(stdout, "  \u21e2 flattened %d bundled skill(s) \u2192 cache/.bundled/skills\n", nSkills)
	}
	if nCommands > 0 {
		fmt.Fprintf(stdout, "  \u21e2 flattened %d bundled command(s) \u2192 cache/.bundled/commands\n", nCommands)
	}
	if nSkills == 0 && nCommands == 0 {
		fmt.Fprintln(stdout, "  = all bundled up-to-date")
	}
	return 0
}

// bundledSkillHashes narrows the loaded lock's verbatim [skills] section to the
// string hash map projectcache.RemoveBundledSkillLeftovers expects. A
// non-string value can never equal a hex digest, so dropping it keeps Python's
// match result.
func bundledSkillHashes(skills map[string]map[string]any) map[string]map[string]string {
	out := make(map[string]map[string]string, len(skills))
	for id, files := range skills {
		m := make(map[string]string, len(files))
		for name, value := range files {
			if s, ok := value.(string); ok {
				m[name] = s
			}
		}
		out[id] = m
	}
	return out
}

// flattenBundledSkills mirrors flatten_bundled_skills: rebuild
// {cache}/.bundled/skills from {cli_source}/bundled-skills and return the
// number of skill directories copied.
func flattenBundledSkills(project, cliSource string) (int, error) {
	dest := filepath.Join(projectcache.BundledSkillsRoot(project, cliSource), "skills")
	if _, err := os.Stat(dest); err == nil {
		if err := projectcache.RemoveTreePy(dest); err != nil {
			return 0, err
		}
	}
	src := filepath.Join(cliSource, "bundled-skills")
	count := 0
	if !isDirPath(src) {
		return 0, nil
	}
	if err := os.MkdirAll(dest, 0o777); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		skillDir := filepath.Join(src, name)
		if !isDirPath(skillDir) {
			continue
		}
		if err := projectcache.CopyTree(skillDir, filepath.Join(dest, name)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// flattenBundledCommands mirrors flatten_bundled_commands: rebuild
// {cache}/.bundled/commands from every *.md directly under
// {cli_source}/bundled-commands (flat, no subdirectories) and return the
// number of files copied.
func flattenBundledCommands(project, cliSource string) (int, error) {
	dest := projectcache.BundledCommandsRoot(project, cliSource)
	if _, err := os.Stat(dest); err == nil {
		if err := projectcache.RemoveTreePy(dest); err != nil {
			return 0, err
		}
	}
	src := filepath.Join(cliSource, "bundled-commands")
	count := 0
	if !isDirPath(src) {
		return 0, nil
	}
	if err := os.MkdirAll(dest, 0o777); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		srcFile := filepath.Join(src, name)
		if _, err := os.Stat(srcFile); err != nil {
			continue
		}
		if err := copyFileMeta(srcFile, filepath.Join(dest, name)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// copyFileMeta mirrors shutil.copy2 (file bytes plus copystat: mode and mtime).
// projectcache.copy2 is the same helper but unexported, and this unit's allowed
// surfaces exclude that package.
func copyFileMeta(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
