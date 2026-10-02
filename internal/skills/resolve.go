package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ResolvedSkill is one entry of collect_skills: the winning source tier and
// the absolute skill directory (the dir containing SKILL.md).
type ResolvedSkill struct {
	Source string // "local" | "recipe" | "dep" | "bundled"
	Path   string
}

// CollectSkills mirrors skill-resolution.collect_skills(project_root) with the
// default cli_home (AI_SPECS_HOME, or the module's repo root in Python).
// Warnings go to os.Stderr.
func CollectSkills(projectRoot, cliHome string) map[string]ResolvedSkill {
	return CollectSkillsTo(projectRoot, cliHome, os.Stderr)
}

// CollectSkillsTo is CollectSkills with an explicit warning sink. Precedence
// is local > recipe > dep > bundled; first-seen wins inside each tier.
func CollectSkillsTo(projectRoot, cliHome string, stderr io.Writer) map[string]ResolvedSkill {
	resolved := map[string]ResolvedSkill{}

	for id, path := range scanLocalSkills(projectRoot) {
		resolved[id] = ResolvedSkill{Source: "local", Path: path}
	}
	for id, path := range scanRecipeSkills(projectRoot, cliHome, stderr) {
		if _, ok := resolved[id]; !ok {
			resolved[id] = ResolvedSkill{Source: "recipe", Path: path}
		}
	}
	for id, path := range scanDepSkills(projectRoot, cliHome, stderr) {
		if _, ok := resolved[id]; !ok {
			resolved[id] = ResolvedSkill{Source: "dep", Path: path}
		}
	}
	for id, path := range scanBundledSkills(projectRoot, cliHome) {
		if _, ok := resolved[id]; !ok {
			resolved[id] = ResolvedSkill{Source: "bundled", Path: path}
		}
	}
	return resolved
}

// warn mirrors skill-resolution.warn.
func warn(w io.Writer, msg string) { fmt.Fprintf(w, "  ! %s\n", msg) }

// scanLocalSkills mirrors _scan_local_skills: ai-specs/skills/{id}/SKILL.md.
func scanLocalSkills(projectRoot string) map[string]string {
	out := map[string]string{}
	skillsDir := filepath.Join(projectRoot, "ai-specs", "skills")
	for _, e := range readDirSorted(skillsDir) {
		child := filepath.Join(skillsDir, e.Name())
		if isDir(child) && isFile(filepath.Join(child, "SKILL.md")) {
			out[e.Name()] = child
		}
	}
	return out
}

// scanRecipeSkills mirrors _scan_recipe_skills:
// {cache}/.recipe/{recipe-id}/skills/{id}/SKILL.md.
func scanRecipeSkills(projectRoot, cliHome string, stderr io.Writer) map[string]string {
	recipeDir := recipeSkillsRoot(projectRoot, cliHome)
	out := map[string]string{}
	for _, re := range readDirSorted(recipeDir) {
		recipeChild := filepath.Join(recipeDir, re.Name())
		if !isDir(recipeChild) {
			continue
		}
		skillsDir := filepath.Join(recipeChild, "skills")
		if !isDir(skillsDir) {
			continue
		}
		for _, se := range readDirSorted(skillsDir) {
			skillChild := filepath.Join(skillsDir, se.Name())
			if !isDir(skillChild) || !isFile(filepath.Join(skillChild, "SKILL.md")) {
				continue
			}
			if prev, ok := out[se.Name()]; ok {
				warn(stderr, fmt.Sprintf(
					"skill '%s' found in multiple recipes; using first-seen from '%s'",
					se.Name(), ownerName(prev)))
			} else {
				out[se.Name()] = skillChild
			}
		}
	}
	return out
}

// scanDepSkills mirrors _scan_dep_skills: the in-project dep root is scanned
// first, then the CLI cache root; both are the "dep" tier and first-seen wins.
func scanDepSkills(projectRoot, cliHome string, stderr io.Writer) map[string]string {
	out := map[string]string{}
	roots := []string{inprojectDepsRoot(projectRoot), depsSkillsRoot(projectRoot, cliHome)}
	for _, depsDir := range roots {
		if !isDir(depsDir) {
			continue
		}
		for _, de := range readDirSorted(depsDir) {
			depChild := filepath.Join(depsDir, de.Name())
			if !isDir(depChild) {
				continue
			}
			skillsDir := filepath.Join(depChild, "skills")
			if !isDir(skillsDir) {
				continue
			}
			for _, se := range readDirSorted(skillsDir) {
				skillChild := filepath.Join(skillsDir, se.Name())
				if !isDir(skillChild) || !isFile(filepath.Join(skillChild, "SKILL.md")) {
					continue
				}
				if prev, ok := out[se.Name()]; ok {
					warn(stderr, fmt.Sprintf(
						"skill '%s' found in multiple deps; using first-seen from '%s'",
						se.Name(), ownerName(prev)))
				} else {
					out[se.Name()] = skillChild
				}
			}
		}
	}
	return out
}

// scanBundledSkills mirrors _scan_bundled_skills: {cache}/.bundled/skills/{id}.
func scanBundledSkills(projectRoot, cliHome string) map[string]string {
	skillsDir := filepath.Join(bundledSkillsRoot(projectRoot, cliHome), "skills")
	out := map[string]string{}
	for _, e := range readDirSorted(skillsDir) {
		child := filepath.Join(skillsDir, e.Name())
		if isDir(child) && isFile(filepath.Join(child, "SKILL.md")) {
			out[e.Name()] = child
		}
	}
	return out
}

// ownerName mirrors Path.parents[1].name: the recipe/dep id two levels up
// from the skill dir (.../{id}/skills/{skill}).
func ownerName(skillPath string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(skillPath)))
}

// --- project-cache.py path helpers ------------------------------------------

var basenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SanitizeBasename mirrors project-cache._sanitize_basename.
func SanitizeBasename(name string) string {
	cleaned := strings.Trim(basenameUnsafe.ReplaceAllString(name, "-"), "-._")
	if cleaned == "" {
		return "project"
	}
	return cleaned
}

// CacheKey mirrors project-cache.cache_key.
func CacheKey(projectRoot string) string {
	resolved := ResolvePath(projectRoot)
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])[:12] + "-" + SanitizeBasename(filepath.Base(resolved))
}

// aiSpecsHome mirrors project-cache._ai_specs_home. The Python fallback (the
// module's repo root) has no Go equivalent; callers that hit it should pass an
// explicit cliHome. AI_SPECS_HOME is honoured when cliHome is empty.
func aiSpecsHome(cliHome string) string {
	if cliHome != "" {
		return ResolvePath(cliHome)
	}
	if env := os.Getenv("AI_SPECS_HOME"); env != "" {
		return ResolvePath(env)
	}
	return ""
}

// CacheRoot mirrors project-cache.cache_root: the per-project cache directory.
func CacheRoot(projectRoot, cliHome string) string {
	home := aiSpecsHome(cliHome)
	return ResolvePath(filepath.Join(home, "cache", "projects", CacheKey(projectRoot)))
}

func recipeSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".recipe")
}

func depsSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".deps")
}

func bundledSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".bundled")
}

// inprojectDepsRoot mirrors project-cache.inproject_deps_root; unlike the
// cache roots it does not resolve project_root.
func inprojectDepsRoot(projectRoot string) string {
	return filepath.Join(projectRoot, "ai-specs", ".deps")
}

// --- filesystem helpers mirroring Python Path semantics ---------------------

// readDirSorted returns the directory entries sorted by name, or nil when the
// directory does not exist (mirrors iterdir on a missing dir via callers'
// is_dir guards; ReadDir's error is discarded like a failed is_dir check).
func readDirSorted(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries
}

// isFile mirrors Path.is_file(): a regular file, following symlinks.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// isDir mirrors Path.is_dir().
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ResolvePath mirrors Path(p).resolve() with strict=False. Python uses
// posixpath.realpath(strict=False): a component-by-component pathwalk that
// resolves each component's symlinks BEFORE `..` pops against the resolved
// prefix. A dangling component leaves the remaining tail unresolved and a
// symlink loop returns the looping link unresolved (the `seen` map).
//
// A Clean-first implementation (filepath.Abs/filepath.Clean) is WRONG here: it
// collapses `..` before symlink resolution, so `link/../x` diverges whenever
// the link points at a different depth than its parent. This duplicates
// internal/projectcache.ResolvePath (package-local by design).
func ResolvePath(p string) string {
	return realpathPy(p)
}

// realpathPy mirrors posixpath.realpath(filename, strict=False). See the
// ResolvePath comment. A NUL sentinel stands in for CPython's None marker.
func realpathPy(filename string) string {
	const (
		sep    = "/"
		root   = "/"
		curdir = "."
		pardir = ".."
		marker = "\x00"
	)
	parts := strings.Split(filename, sep)
	rest := make([]string, len(parts))
	for i, part := range parts {
		rest[len(parts)-1-i] = part
	}
	partCount := len(rest)

	path := root
	if !strings.HasPrefix(filename, sep) {
		if wd, err := os.Getwd(); err == nil {
			path = wd
		}
	}

	seen := map[string]*string{}
	for partCount > 0 {
		name := rest[len(rest)-1]
		rest = rest[:len(rest)-1]
		if name == marker {
			key := rest[len(rest)-1]
			rest = rest[:len(rest)-1]
			resolved := path
			seen[key] = &resolved
			continue
		}
		partCount--
		if name == "" || name == curdir {
			continue
		}
		if name == pardir {
			if idx := strings.LastIndex(path, sep); idx > 0 {
				path = path[:idx]
			} else {
				path = root
			}
			continue
		}
		newpath := path + sep + name
		if path == root {
			newpath = path + name
		}
		info, err := os.Lstat(newpath)
		if err != nil {
			path = newpath
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			path = newpath
			continue
		}
		if cached, ok := seen[newpath]; ok {
			if cached != nil {
				path = *cached
			} else {
				path = newpath
			}
			continue
		}
		target, err := os.Readlink(newpath)
		if err != nil {
			path = newpath
			continue
		}
		if strings.HasPrefix(target, sep) {
			path = root
		}
		seen[newpath] = nil
		rest = append(rest, newpath, marker)
		targetParts := strings.Split(target, sep)
		for i := len(targetParts) - 1; i >= 0; i-- {
			rest = append(rest, targetParts[i])
		}
		partCount += len(targetParts)
	}
	return path
}
