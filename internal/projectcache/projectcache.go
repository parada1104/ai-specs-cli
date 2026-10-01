// Package projectcache is the byte-exact Go port of
// lib/_internal/project-cache.py (GO-07.S6).
//
// Scope: library plus a RenderProjectCache mirror of the script's main() argv
// surface. The Python module stays alive until its callers
// (vendor-skills, refresh-bundled, recipe-materialize, skill-resolution,
// doctor, skills-list.sh) move to Go in their own cards, so there is no
// wiring here and no behaviour flag.
//
// The cache key derivation is FROZEN by docs/go-migration-parity-contract.md
// (§4 "Cache key derivation"):
//
//	$AI_SPECS_HOME/cache/projects/<sha256(realpath)[:12]>-<sanitized-basename>/
//
// Deriving it differently would orphan every existing on-disk project cache.
// The port uses Python Path.resolve() semantics (symlinks followed, dangling
// symlinks resolved component-by-component) for the project root and for an
// explicit cli_home.
//
// Ported helpers come in two halves: the cache/path core (this half) and the
// mutation + git-aware remediation + merge + CLI surface (the second half).
package projectcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/toml"
)

// --- Commit A: cache key, home, roots, ensure_cache -------------------------

var basenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SanitizeBasename mirrors _sanitize_basename.
func SanitizeBasename(name string) string {
	cleaned := strings.Trim(basenameUnsafe.ReplaceAllString(name, "-"), "-._")
	if cleaned == "" {
		return "project"
	}
	return cleaned
}

// ResolvePath mirrors Path(p).resolve() with strict=False. Python uses
// posixpath.realpath(strict=False): a component-by-component pathwalk that
// resolves each component's symlinks BEFORE `..` pops against the resolved
// prefix. A dangling component leaves the remaining tail unresolved and a
// symlink loop returns the looping link unresolved (the `seen` map).
//
// A Clean-first implementation (filepath.Abs/filepath.Clean) is WRONG here: it
// collapses `..` before symlink resolution, so `link/../x` diverges whenever
// the link points at a different depth than its parent.
// Kept package-local so projectcache does not depend on internal/skills (the
// other owner of a ported project-cache derivation).
func ResolvePath(p string) string {
	return realpathPy(p)
}

// realpathPy mirrors posixpath.realpath(filename, strict=False). The stack of
// pending parts uses a NUL sentinel in place of CPython's None marker: when a
// sentinel pops, the next stack entry is the symlink path whose fully-resolved
// target (the current path) is recorded in seen. The seen map both terminates
// symlink loops and caches resolved links.
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

// CacheKey mirrors cache_key: 12-char sha256 of realpath + sanitized basename.
func CacheKey(projectRoot string) string {
	resolved := ResolvePath(projectRoot)
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])[:12] + "-" + SanitizeBasename(filepath.Base(resolved))
}

// AISpecsHome mirrors _ai_specs_home for the cases the port can honour: an
// explicit cli_home is resolved first, then AI_SPECS_HOME. Python's final
// fallback (Path(__file__).resolve().parents[2], the module's repo root) has no
// Go source-layout equivalent; callers that would hit it must pass cli_home.
// The CLI shim always pins AI_SPECS_HOME, so RenderProjectCache never hits it.
func AISpecsHome(cliHome string) string {
	if cliHome != "" {
		return ResolvePath(cliHome)
	}
	if env := os.Getenv("AI_SPECS_HOME"); env != "" {
		return ResolvePath(env)
	}
	return ""
}

// CacheRoot mirrors cache_root. cli_home, when non-empty, is resolved (Python:
// Path(cli_home).resolve()); otherwise the resolved AI_SPECS_HOME is used.
func CacheRoot(projectRoot, cliHome string) string {
	home := AISpecsHome(cliHome)
	return ResolvePath(filepath.Join(home, "cache", "projects", CacheKey(projectRoot)))
}

// EnsureCache mirrors ensure_cache: create the cache root and write or refresh
// the meta.toml sidecar. On a mkdir failure Python raises RuntimeError
// ("cache not writable at <cache>: <exc>"); the same message shape is returned.
func EnsureCache(projectRoot, cliHome string) (string, error) {
	root := ResolvePath(projectRoot)
	cache := CacheRoot(root, cliHome)
	if err := os.MkdirAll(cache, 0o777); err != nil {
		return "", fmt.Errorf("cache not writable at %s: %s", cache, err)
	}

	meta := filepath.Join(cache, "meta.toml")
	if info, err := os.Stat(meta); err != nil || !info.Mode().IsRegular() {
		createdAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		content := "project_root = \"" + root + "\"\n" +
			"created_at = \"" + createdAt + "\"\n"
		if err := os.WriteFile(meta, []byte(content), 0o666); err != nil {
			return "", fmt.Errorf("cache not writable at %s: %s", cache, err)
		}
		return cache, nil
	}

	// Keep project_root current (worktree moves / renames).
	data, err := os.ReadFile(meta)
	if err != nil {
		return "", fmt.Errorf("cache not writable at %s: %s", cache, err)
	}
	lines := pySplitLines(string(data))
	out := make([]string, 0, len(lines)+1)
	sawRoot := false
	rootLine := "project_root = \"" + root + "\""
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "project_root") {
			out = append(out, rootLine)
			sawRoot = true
		} else {
			out = append(out, line)
		}
	}
	if !sawRoot {
		out = append([]string{rootLine}, out...)
	}
	if err := os.WriteFile(meta, []byte(strings.Join(out, "\n")+"\n"), 0o666); err != nil {
		return "", fmt.Errorf("cache not writable at %s: %s", cache, err)
	}
	return cache, nil
}

// pySplitLines mirrors str.splitlines(): splits on \n, \r, \r\n and the other
// Unicode line boundaries Python recognises.
func pySplitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isLineBoundary(r) {
			i += size
			continue
		}
		lines = append(lines, s[start:i])
		i += size
		if r == '\r' && i < len(s) && s[i] == '\n' {
			i++
		}
		start = i
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// isLineBoundary reports whether r terminates a line for str.splitlines().
func isLineBoundary(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
		return true
	}
	return false
}

// RecipeSkillsRoot mirrors recipe_skills_root.
func RecipeSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".recipe")
}

// DepsSkillsRoot mirrors deps_skills_root.
func DepsSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".deps")
}

// BundledSkillsRoot mirrors bundled_skills_root (the ".bundled" namespace).
func BundledSkillsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), ".bundled")
}

// BundledCommandsRoot mirrors bundled_commands_root.
func BundledCommandsRoot(projectRoot, cliHome string) string {
	return filepath.Join(BundledSkillsRoot(projectRoot, cliHome), "commands")
}

// InprojectDepsRoot mirrors inproject_deps_root: in-project, gitignored deps.
// Unlike the cache roots it does not resolve project_root.
func InprojectDepsRoot(projectRoot string) string {
	return filepath.Join(projectRoot, "ai-specs", ".deps")
}

// CommandsDir mirrors commands_dir.
func CommandsDir(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), "commands")
}

// BackupsRoot mirrors backups_root: cache-only immutable gate snapshots.
func BackupsRoot(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), "backups")
}

// GateBackupPath mirrors gate_backup_path:
// backups/<sha256(rel_path)>/<content_sha>.sh.
func GateBackupPath(projectRoot, relPath, contentSha, cliHome string) string {
	sum := sha256.Sum256([]byte(relPath))
	return filepath.Join(BackupsRoot(projectRoot, cliHome), hex.EncodeToString(sum[:]), contentSha+".sh")
}

// ResolvedSkillsDir mirrors resolved_skills_dir.
func ResolvedSkillsDir(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), "resolved-skills")
}

// --- filesystem helpers mirroring Python Path semantics ---------------------

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

// exists mirrors Path.exists() (following symlinks).
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readDirNames mirrors sorted(p.iterdir()): entry names sorted lexicographically,
// or nil when the directory cannot be read (callers guard with isDir first).
func readDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// normalizedBytes mirrors _normalized_bytes: CRLF collapsed to LF.
func normalizedBytes(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// warn mirrors _warn: "  ! <msg>" on stderr.
func warn(w io.Writer, msg string) {
	fmt.Fprintf(w, "  ! %s\n", msg)
}

// --- Commit B: mutations, git-aware remediation, merge, CLI -----------------

// removeTreePy mirrors shutil.rmtree: it refuses a symbolic link (shutil's
// guard so a symlink-to-directory is never silently unlinked) where
// os.RemoveAll would remove the link itself.
func removeTreePy(path string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Cannot call rmtree on a symbolic link: %s", path)
	}
	return os.RemoveAll(path)
}

// bundledHome mirrors the unresolved home used by the bundled-leftover helpers:
// Path(cli_home) when explicit, else the resolved AI_SPECS_HOME.
func bundledHome(cliHome string) string {
	if cliHome != "" {
		return cliHome
	}
	return AISpecsHome("")
}

// legacyLockSkillHashes mirrors _legacy_lock_skill_hashes (best-effort).
func legacyLockSkillHashes(aiSpecs string) map[string]map[string]string {
	root, ok := readLock(aiSpecs)
	if !ok {
		return map[string]map[string]string{}
	}
	skillsVal, ok := root.Get("skills")
	if !ok {
		return map[string]map[string]string{}
	}
	skillsTbl, ok := skillsVal.(*toml.Table)
	if !ok {
		return map[string]map[string]string{}
	}
	out := map[string]map[string]string{}
	for _, sid := range skillsTbl.Keys() {
		v, _ := skillsTbl.Get(sid)
		files, ok := v.(*toml.Table)
		if !ok {
			continue
		}
		m := map[string]string{}
		for _, name := range files.Keys() {
			if s, ok := files.String(name); ok {
				m[name] = s
			}
		}
		out[sid] = m
	}
	return out
}

// legacyLockCommandHashes mirrors _legacy_lock_command_hashes (best-effort).
func legacyLockCommandHashes(aiSpecs string) map[string]string {
	root, ok := readLock(aiSpecs)
	if !ok {
		return map[string]string{}
	}
	commandsVal, ok := root.Get("commands")
	if !ok {
		return map[string]string{}
	}
	commandsTbl, ok := commandsVal.(*toml.Table)
	if !ok {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, name := range commandsTbl.Keys() {
		if s, ok := commandsTbl.String(name); ok {
			out[name] = s
		}
	}
	return out
}

// readLock parses ai-specs/.ai-specs.lock best-effort: a missing file, an
// unreadable file, or a TOMLDecodeError all yield ok=false (mirrors the
// Python except (OSError, TOMLDecodeError) branch).
func readLock(aiSpecs string) (*toml.Table, bool) {
	data, err := os.ReadFile(filepath.Join(aiSpecs, ".ai-specs.lock"))
	if err != nil {
		return nil, false
	}
	root, err := toml.Parse(data)
	if err != nil {
		return nil, false
	}
	return root, true
}

// RemoveBundledSkillLeftovers mirrors remove_bundled_skill_leftovers. A nil
// lockSkills means "read the legacy lock from disk" (Python None); an explicit
// empty map mirrors a caller-supplied {}.
func RemoveBundledSkillLeftovers(aiSpecs, cliHome string, lockSkills map[string]map[string]string, stdout, stderr io.Writer) {
	home := bundledHome(cliHome)
	bundledSrc := filepath.Join(home, "bundled-skills")
	skillsDir := filepath.Join(aiSpecs, "skills")
	if !isDir(bundledSrc) || !isDir(skillsDir) {
		return
	}
	if lockSkills == nil {
		lockSkills = legacyLockSkillHashes(aiSpecs)
	}
	for _, name := range readDirNames(skillsDir) {
		child := filepath.Join(skillsDir, name)
		if !isDir(child) {
			continue
		}
		srcSkill := filepath.Join(bundledSrc, name, "SKILL.md")
		projSkill := filepath.Join(child, "SKILL.md")
		if !isFile(srcSkill) || !isFile(projSkill) {
			continue
		}
		projNorm, err := normalizedBytes(projSkill)
		if err != nil {
			continue
		}
		srcNorm, err := normalizedBytes(srcSkill)
		if err != nil {
			continue
		}
		matchesSource := bytes.Equal(projNorm, srcNorm)
		lockHash := ""
		if files, ok := lockSkills[name]; ok {
			lockHash = files["SKILL.md"]
		}
		matchesLock := lockHash != "" && sha256Hex(projNorm) == lockHash
		if !(matchesSource || matchesLock) {
			warn(stderr, fmt.Sprintf(
				"keeping customized ai-specs/skills/%s/ "+
					"(differs from CLI-bundled source and lock; resolve manually)", name))
			continue
		}
		if err := removeTreePy(child); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/skills/%s/: %s", name, err))
			continue
		}
		fmt.Fprintf(stdout, "  \u2713 removed leftover bundled skill ai-specs/skills/%s/\n", name)
	}

	leftovers := TrackedBundledSkillLeftovers(filepath.Dir(aiSpecs), home)
	if len(leftovers) > 0 {
		fmt.Fprintln(stdout, FormatTrackedBundledRemediation(
			leftovers, "skill", "ai-specs/skills/{name}", true))
	}
}

// RemoveBundledCommandLeftovers mirrors remove_bundled_command_leftovers.
func RemoveBundledCommandLeftovers(aiSpecs, cliHome string, lockCommands map[string]string, stdout, stderr io.Writer) {
	home := bundledHome(cliHome)
	bundledSrc := filepath.Join(home, "bundled-commands")
	localDir := filepath.Join(aiSpecs, "commands")
	if !isDir(bundledSrc) || !isDir(localDir) {
		return
	}
	if lockCommands == nil {
		lockCommands = legacyLockCommandHashes(aiSpecs)
	}
	for _, name := range readDirNames(localDir) {
		child := filepath.Join(localDir, name)
		if !isFile(child) || filepath.Ext(child) != ".md" {
			continue
		}
		srcCmd := filepath.Join(bundledSrc, name)
		if !isFile(srcCmd) {
			continue
		}
		projNorm, err := normalizedBytes(child)
		if err != nil {
			continue
		}
		srcNorm, err := normalizedBytes(srcCmd)
		if err != nil {
			continue
		}
		matchesSource := bytes.Equal(projNorm, srcNorm)
		lockHash := lockCommands[name]
		matchesLock := lockHash != "" && sha256Hex(projNorm) == lockHash
		if !(matchesSource || matchesLock) {
			warn(stderr, fmt.Sprintf(
				"keeping customized ai-specs/commands/%s "+
					"(differs from CLI-bundled source and lock; resolve manually)", name))
			continue
		}
		if err := os.Remove(child); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/commands/%s: %s", name, err))
			continue
		}
		fmt.Fprintf(stdout, "  \u2713 removed leftover bundled command ai-specs/commands/%s\n", name)
	}

	leftovers := TrackedBundledCommandLeftovers(filepath.Dir(aiSpecs), home)
	if len(leftovers) > 0 {
		fmt.Fprintln(stdout, FormatTrackedBundledRemediation(
			leftovers, "command", "ai-specs/commands/{name}.md", false))
	}
}

// RemoveRecipeCommandLeftovers mirrors remove_recipe_command_leftovers.
// recipeSources maps command file name -> source path (Python recipe_sources).
func RemoveRecipeCommandLeftovers(projectRoot, cliHome string, lockCommands map[string]string, recipeSources map[string]string, stdout, stderr io.Writer) {
	root := projectRoot
	aiSpecs := filepath.Join(root, "ai-specs")
	localDir := filepath.Join(aiSpecs, "commands")
	managedDir := CommandsDir(root, cliHome)
	if !isDir(localDir) {
		return
	}
	if lockCommands == nil {
		lockCommands = legacyLockCommandHashes(aiSpecs)
	}
	if recipeSources == nil {
		recipeSources = map[string]string{}
	}
	for _, name := range readDirNames(localDir) {
		child := filepath.Join(localDir, name)
		if !isFile(child) || filepath.Ext(child) != ".md" {
			continue
		}
		managed := filepath.Join(managedDir, name)
		source, hasSource := recipeSources[name]
		lockHash := lockCommands[name]
		hasManaged := isFile(managed)
		hasSourceFile := hasSource && isFile(source)
		if !hasManaged && !hasSourceFile && lockHash == "" {
			continue
		}
		projectBytes, err := normalizedBytes(child)
		if err != nil {
			continue
		}
		matchesManaged := false
		if hasManaged {
			if b, err := normalizedBytes(managed); err == nil {
				matchesManaged = bytes.Equal(projectBytes, b)
			}
		}
		matchesSource := false
		if hasSourceFile {
			if b, err := normalizedBytes(source); err == nil {
				matchesSource = bytes.Equal(projectBytes, b)
			}
		}
		matchesLock := lockHash != "" && sha256Hex(projectBytes) == lockHash
		if !(matchesManaged || matchesSource || matchesLock) {
			warn(stderr, fmt.Sprintf(
				"keeping local/customized ai-specs/commands/%s "+
					"(differs from recipe-managed cache/source and legacy provenance; resolve manually)", name))
			continue
		}
		if err := os.Remove(child); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/commands/%s: %s", name, err))
			continue
		}
		fmt.Fprintf(stdout, "  \u2713 removed leftover recipe command ai-specs/commands/%s\n", name)
	}
}

// BundledSkillIDs mirrors bundled_skill_ids.
func BundledSkillIDs(cliHome string) []string {
	root := filepath.Join(bundledHome(cliHome), "bundled-skills")
	if !isDir(root) {
		return []string{}
	}
	ids := []string{}
	for _, name := range readDirNames(root) {
		p := filepath.Join(root, name)
		if isDir(p) && isFile(filepath.Join(p, "SKILL.md")) {
			ids = append(ids, name)
		}
	}
	sort.Strings(ids)
	return ids
}

// BundledCommandIDs mirrors bundled_command_ids: the .md stems under
// bundled-commands/ (Python Path.stem semantics).
func BundledCommandIDs(cliHome string) []string {
	root := filepath.Join(bundledHome(cliHome), "bundled-commands")
	if !isDir(root) {
		return []string{}
	}
	ids := []string{}
	for _, name := range readDirNames(root) {
		p := filepath.Join(root, name)
		if isFile(p) && filepath.Ext(p) == ".md" {
			ids = append(ids, pyStem(name))
		}
	}
	sort.Strings(ids)
	return ids
}

// pyStem mirrors PurePath.stem for a bare file name.
func pyStem(name string) string {
	if i := strings.LastIndex(name, "."); i != -1 {
		stem := name[:i]
		// Stem must contain at least one non-dot character.
		if strings.TrimLeft(stem, ".") != "" {
			return stem
		}
	}
	return name
}

// IsGitWorkTree mirrors _is_git_work_tree.
func IsGitWorkTree(projectRoot string) bool {
	out, code, ok := runGitCapture(projectRoot, "rev-parse", "--is-inside-work-tree")
	return ok && code == 0 && strings.TrimSpace(out) == "true"
}

// GitLsFiles mirrors _git_ls_files.
func GitLsFiles(projectRoot, pathspec string) []string {
	out, code, ok := runGitCapture(projectRoot, "ls-files", "--", pathspec)
	if !ok || code != 0 {
		return []string{}
	}
	files := []string{}
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

// runGitCapture runs `git -C root args...`; ok=false when git could not be
// spawned (Python's OSError branch).
func runGitCapture(root string, args ...string) (string, int, bool) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, isExit := err.(interface{ ExitCode() int }); isExit {
			return string(out), ee.ExitCode(), true
		}
		return "", 1, false
	}
	return string(out), 0, true
}

// trackedBundledLeftovers mirrors _tracked_bundled_leftovers.
func trackedBundledLeftovers(projectRoot string, bundledIDs []string, pathTemplate string) []string {
	if !IsGitWorkTree(projectRoot) {
		return []string{}
	}
	leftovers := []string{}
	for _, id := range bundledIDs {
		rel := strings.ReplaceAll(pathTemplate, "{name}", id)
		if exists(filepath.Join(projectRoot, filepath.FromSlash(rel))) {
			continue
		}
		if len(GitLsFiles(projectRoot, rel)) > 0 {
			leftovers = append(leftovers, id)
		}
	}
	return leftovers
}

// TrackedBundledSkillLeftovers mirrors tracked_bundled_skill_leftovers.
func TrackedBundledSkillLeftovers(projectRoot, cliHome string) []string {
	return trackedBundledLeftovers(projectRoot, BundledSkillIDs(cliHome), "ai-specs/skills/{name}")
}

// TrackedBundledCommandLeftovers mirrors tracked_bundled_command_leftovers.
func TrackedBundledCommandLeftovers(projectRoot, cliHome string) []string {
	return trackedBundledLeftovers(projectRoot, BundledCommandIDs(cliHome), "ai-specs/commands/{name}.md")
}

// FormatTrackedBundledRemediation mirrors format_tracked_bundled_remediation.
// It never executes git.
func FormatTrackedBundledRemediation(bundledIDs []string, kind, pathTemplate string, recursive bool) string {
	paths := make([]string, len(bundledIDs))
	for i, id := range bundledIDs {
		paths[i] = strings.ReplaceAll(pathTemplate, "{name}", id)
	}
	flag := "--cached"
	if recursive {
		flag = "-r --cached"
	}
	return "  \u2139 git still tracks removed CLI-bundled " + kind + "(s): " +
		strings.Join(bundledIDs, ", ") + "\n" +
		"    To stop committing them (ai-specs will not modify the index):\n" +
		"    git rm " + flag + " " + strings.Join(paths, " ") + "\n" +
		"    # then commit when ready"
}

// RemoveLegacyOrigin mirrors remove_legacy_origin.
func RemoveLegacyOrigin(projectRoot, cliHome string, stdout, stderr io.Writer) {
	root := projectRoot
	aiSpecs := filepath.Join(root, "ai-specs")
	legacyRecipe := filepath.Join(aiSpecs, ".recipe")

	migrationFailed := false
	if isDir(legacyRecipe) {
		for _, name := range readDirNames(legacyRecipe) {
			recipeChild := filepath.Join(legacyRecipe, name)
			if !isDir(recipeChild) {
				continue
			}
			legacyOverrides := filepath.Join(recipeChild, "overrides")
			if !isDir(legacyOverrides) {
				continue
			}
			dest := filepath.Join(aiSpecs, "recipes", name, "overrides")
			// FROZEN retry semantics (project-cache.py L497-531): a recipe whose
			// destination already exists is skipped, never re-migrated. A prior
			// run that failed left a partial destination, so this retry skips it,
			// no migration fails in this run, and ai-specs/.recipe/ is removed
			// even though the destination may be incomplete. This is the oracle's
			// deliberate behavior; do not add a partial-destination guard here.
			if exists(dest) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
				warn(stderr, fmt.Sprintf("failed to migrate overrides for '%s': %s", name, err))
				migrationFailed = true
				continue
			}
			if err := copyTree(legacyOverrides, dest); err != nil {
				warn(stderr, fmt.Sprintf("failed to migrate overrides for '%s': %s", name, err))
				migrationFailed = true
				continue
			}
			fmt.Fprintf(stdout, "  \u2713 migrated overrides %s \u2192 ai-specs/recipes/%s/overrides/\n", name, name)
		}
	}

	// Only .recipe/ is legacy origin. ai-specs/.deps/ is now the toml-dep home
	// (gitignored) and must NOT be deleted.
	if exists(legacyRecipe) {
		if migrationFailed {
			warn(stderr, "skipping removal of ai-specs/.recipe/ \u2014 "+
				"one or more override migrations failed; re-run ai-specs sync to retry")
		} else if err := removeTreePy(legacyRecipe); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/.recipe/: %s", err))
		} else {
			fmt.Fprintln(stdout, "  \u2713 removed leftover ai-specs/.recipe/")
		}
	}

	for _, item := range []struct{ rel, label string }{
		{".resolved-skills", ".resolved-skills"},
		{".internal", ".internal"},
	} {
		path := filepath.Join(aiSpecs, item.rel)
		if !exists(path) {
			continue
		}
		if err := removeTreePy(path); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/%s/: %s", item.label, err))
			continue
		}
		fmt.Fprintf(stdout, "  \u2713 removed leftover ai-specs/%s/\n", item.label)
	}

	guardian := filepath.Join(aiSpecs, "bin", "premerge_guardian.py")
	if info, err := os.Lstat(guardian); err == nil && (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		if err := os.Remove(guardian); err != nil {
			warn(stderr, fmt.Sprintf("failed to remove leftover ai-specs/bin/premerge_guardian.py: %s", err))
		} else {
			fmt.Fprintln(stdout, "  \u2713 removed leftover ai-specs/bin/premerge_guardian.py")
		}
	}

	binDir := filepath.Join(aiSpecs, "bin")
	if isDir(binDir) {
		if entries, err := os.ReadDir(binDir); err == nil && len(entries) == 0 {
			if err := os.Remove(binDir); err != nil {
				warn(stderr, fmt.Sprintf("failed to remove empty ai-specs/bin/: %s", err))
			} else {
				fmt.Fprintln(stdout, "  \u2713 removed empty ai-specs/bin/")
			}
		} else if err != nil {
			warn(stderr, fmt.Sprintf("failed to remove empty ai-specs/bin/: %s", err))
		}
	}

	RemoveBundledSkillLeftovers(aiSpecs, cliHome, nil, stdout, stderr)
}

// MergeCommands mirrors merge_commands. Ascending precedence: bundled ->
// recipe-managed -> local hand-authored. Returns the file count in dest.
func MergeCommands(projectRoot, destDir, cliHome string, stdout, stderr io.Writer) (int, error) {
	if exists(destDir) {
		if err := removeTreePy(destDir); err != nil {
			return 0, err
		}
	}
	if err := os.MkdirAll(destDir, 0o777); err != nil {
		return 0, err
	}

	count := 0
	seen := map[string]bool{}

	bundled := BundledCommandsRoot(projectRoot, cliHome)
	if isDir(bundled) {
		for _, src := range globMDFiles(bundled) {
			name := filepath.Base(src)
			// Python's copy2 is not guarded: an unreadable source aborts the
			// whole merge, leaving the files copied before the failure.
			if err := copy2(src, filepath.Join(destDir, name)); err != nil {
				return count, err
			}
			seen[name] = true
			count++
		}
	}

	managed := CommandsDir(projectRoot, cliHome)
	if isDir(managed) {
		for _, src := range globMDFiles(managed) {
			name := filepath.Base(src)
			if err := copy2(src, filepath.Join(destDir, name)); err != nil {
				return count, err
			}
			if !seen[name] {
				count++
			}
			seen[name] = true
		}
	}

	local := filepath.Join(projectRoot, "ai-specs", "commands")
	if isDir(local) {
		for _, src := range globMDFiles(local) {
			name := filepath.Base(src)
			if seen[name] {
				warn(stderr, fmt.Sprintf(
					"command '%s' present in cache and ai-specs/commands/; local hand-authored wins",
					pyStem(name)))
			}
			if err := copy2(src, filepath.Join(destDir, name)); err != nil {
				return count, err
			}
			if !seen[name] {
				count++
			}
			seen[name] = true
		}
	}

	return count, nil
}

// globMDFiles mirrors sorted(dir.glob("*.md")) for regular files (symlinks to
// regular files included), returning paths in ascending name order.
func globMDFiles(dir string) []string {
	var out []string
	for _, name := range readDirNames(dir) {
		if filepath.Ext(name) != ".md" {
			continue
		}
		p := filepath.Join(dir, name)
		if isFile(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// copy2 mirrors shutil.copy2: file bytes plus copystat (mode bits and times).
func copy2(src, dst string) error {
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

// copyTree mirrors shutil.copytree(dst must not exist).
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("copytree: %s is not a directory", src)
	}
	if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
		return err
	}
	for _, name := range readDirNames(src) {
		s := filepath.Join(src, name)
		d := filepath.Join(dst, name)
		childInfo, err := os.Lstat(s)
		if err != nil {
			return err
		}
		switch {
		case childInfo.IsDir():
			if err := copyTree(s, d); err != nil {
				return err
			}
		case childInfo.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(s)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, d); err != nil {
				return err
			}
		default:
			if err := copy2(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderProjectCache mirrors project-cache.py's main(): the argv surface the
// Bash spine shells out to. projectRoot/action/arg correspond to argv[1..3];
// a missing path kind or merge dest is modelled by an empty arg. The
// len(argv) < 3 usage branch (no project root) stays in the caller.
//
// Exit codes: 0 on success, 2 for a usage/unknown-kind/unknown-action error,
// 1 when ensure_cache fails (Python's uncaught RuntimeError traceback).
func RenderProjectCache(projectRoot, action, arg string, stdout, stderr io.Writer) int {
	root := ResolvePath(projectRoot)

	switch action {
	case "ensure":
		cache, err := EnsureCache(root, "")
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintln(stdout, cache)
		return 0

	case "path":
		if arg == "" {
			fmt.Fprintln(stderr, "Usage: project-cache.py <project_root> path <kind>")
			return 2
		}
		mapping := map[string]func(string, string) string{
			"root":            CacheRoot,
			"resolved-skills": ResolvedSkillsDir,
			"commands":        CommandsDir,
			"recipe":          RecipeSkillsRoot,
			"deps":            DepsSkillsRoot,
			"bundled":         BundledSkillsRoot,
		}
		fn, ok := mapping[arg]
		if !ok {
			fmt.Fprintf(stderr, "unknown path kind: %s\n", arg)
			return 2
		}
		if _, err := EnsureCache(root, ""); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintln(stdout, fn(root, ""))
		return 0

	case "merge-commands":
		if arg == "" {
			fmt.Fprintln(stderr, "Usage: project-cache.py <project_root> merge-commands <dest>")
			return 2
		}
		if _, err := EnsureCache(root, ""); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		n, err := MergeCommands(root, arg, "", stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "  \u2713 merged %d command file(s) \u2192 %s\n", n, arg)
		return 0
	}

	fmt.Fprintf(stderr, "unknown action: %s\n", action)
	return 2
}
