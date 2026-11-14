package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// ResolvedSkill is one entry of collect_skills: the winning source tier and
// the absolute skill directory (the dir containing SKILL.md).
type ResolvedSkill struct {
	Source string // "local" | "recipe" | "dep" | "bundled"
	Path   string
}

// CollectSkills mirrors skill-resolution.collect_skills(project_root) with the
// default cli_home (AI_SPECS_HOME, or the module's repo root in Python). Cache
// roots come from internal/projectcache, the single owner of the frozen key.
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
	recipeDir := projectcache.RecipeSkillsRoot(projectRoot, cliHome)
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
	roots := []string{projectcache.InprojectDepsRoot(projectRoot), projectcache.DepsSkillsRoot(projectRoot, cliHome)}
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
	skillsDir := filepath.Join(projectcache.BundledSkillsRoot(projectRoot, cliHome), "skills")
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
