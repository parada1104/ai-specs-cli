package doctor

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// Native port of the project-cache.py surface doctor uses. The cache layout is
// frozen (`{AI_SPECS_HOME}/cache/projects/<key>/{commands,.bundled,resolved-skills}`).
// The cache key and root derivation are owned by internal/projectcache (the
// ported project-cache.py); doctor only appends its own trailing path segments.

// cacheKey delegates to internal/projectcache, the single owner of the ported
// project-cache.cache_key derivation.
func cacheKey(projectRoot string) string {
	return projectcache.CacheKey(projectRoot)
}

// projectCache memoizes the derived cache paths for one Doctor run.
type projectCache struct {
	root      string
	rootReady bool
}

func (d *Doctor) cacheRoot() string {
	if !d.projectCache.rootReady {
		d.projectCache.root = projectcache.CacheRoot(d.Root, d.Home)
		d.projectCache.rootReady = true
	}
	return d.projectCache.root
}

// commandsDir mirrors project-cache.commands_dir.
func (d *Doctor) commandsDir() string { return filepath.Join(d.cacheRoot(), "commands") }

// bundledSkillsRoot mirrors project-cache.bundled_skills_root (the ".bundled"
// namespace; doctor joins "skills" onto it).
func (d *Doctor) bundledSkillsRoot() string { return filepath.Join(d.cacheRoot(), ".bundled") }

// bundledCommandsRoot mirrors project-cache.bundled_commands_root.
func (d *Doctor) bundledCommandsRoot() string {
	return filepath.Join(d.bundledSkillsRoot(), "commands")
}

// resolvedSkillsDir mirrors project-cache.resolved_skills_dir.
func (d *Doctor) resolvedSkillsDir() string {
	return filepath.Join(d.cacheRoot(), "resolved-skills")
}

// bundledSkillNames mirrors doctor.bundled_skill_names(), sourced from the
// binary's embedded assets (card [Go 06]) instead of $AI_SPECS_HOME/bundled-skills.
// The legacy fallback literal is unreachable for an embedded tree.
func bundledSkillNames() []string {
	afs, err := assets.FS("bundled-skills")
	if err != nil {
		return nil
	}
	entries, err := fs.ReadDir(afs, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// bundledCommandNames mirrors doctor.bundled_command_names(): the visible .md
// stems under bundled-commands/, from the embedded assets.
func bundledCommandNames() []string {
	afs, err := assets.FS("bundled-commands")
	if err != nil {
		return nil
	}
	entries, err := fs.ReadDir(afs, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(names)
	return names
}

// bundledSkillIDs mirrors project-cache.bundled_skill_ids: bundled skill
// directories that ship a SKILL.md.
func bundledSkillIDs() []string {
	afs, err := assets.FS("bundled-skills")
	if err != nil {
		return nil
	}
	var ids []string
	for _, name := range bundledSkillNames() {
		if _, err := fs.Stat(afs, path.Join(name, "SKILL.md")); err == nil {
			ids = append(ids, name)
		}
	}
	return ids
}

// mdNames mirrors `{p.name for p in d.glob("*.md")}`: the entries in dir
// whose name ends in ".md", dotfiles excluded and directories included
// (Python's glob matches a directory named `*.md` too). Python's glob order is
// unspecified; callers sort the result when order is user-visible.
func mdNames(dir string) map[string]bool {
	names := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") {
			continue
		}
		names[name] = true
	}
	return names
}

// runGitCapture runs `git -C root args...`; ok=false when git could not run at
// all (Python's OSError branch).
func runGitCapture(root string, args ...string) (stdout string, code int, ok bool) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, isExit := err.(*exec.ExitError); isExit {
			return string(out), ee.ExitCode(), true
		}
		return "", 1, false
	}
	return string(out), 0, true
}

// isGitWorkTree mirrors project-cache._is_git_work_tree.
func isGitWorkTree(root string) bool {
	out, code, ok := runGitCapture(root, "rev-parse", "--is-inside-work-tree")
	return ok && code == 0 && strings.TrimSpace(out) == "true"
}

// gitLsFiles mirrors project-cache._git_ls_files.
func gitLsFiles(root, pathspec string) []string {
	out, code, ok := runGitCapture(root, "ls-files", "--", pathspec)
	if !ok || code != 0 {
		return nil
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

// trackedBundledLeftovers mirrors project-cache._tracked_bundled_leftovers.
func trackedBundledLeftovers(root string, bundledIDs []string, pathTemplate string) []string {
	if !isGitWorkTree(root) {
		return nil
	}
	var leftovers []string
	for _, id := range bundledIDs {
		rel := strings.ReplaceAll(pathTemplate, "{name}", id)
		if exists(filepath.Join(root, filepath.FromSlash(rel))) {
			continue
		}
		if len(gitLsFiles(root, rel)) > 0 {
			leftovers = append(leftovers, id)
		}
	}
	return leftovers
}
