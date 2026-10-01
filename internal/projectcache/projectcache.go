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
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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

// ResolvePath mirrors Path(p).resolve() with strict=False: absolute, symlinks
// resolved as far as possible, dangling symlinks resolving to their target.
// Kept package-local so projectcache does not depend on internal/skills (the
// other owner of a ported project-cache derivation).
func ResolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return resolveNonStrict(abs, 0)
}

// resolveNonStrict resolves every component of an absolute POSIX path,
// following symlinks even when their target does not exist. The link budget
// mirrors the kernel's 40-link bound so a symlink loop terminates.
func resolveNonStrict(path string, links int) string {
	if links > 40 {
		return filepath.Clean(path)
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, "/") {
		return clean
	}
	comps := strings.Split(clean[1:], "/")
	current := "/"
	for i, comp := range comps {
		if comp == "" {
			continue
		}
		if comp == ".." {
			current = filepath.Dir(current)
			continue
		}
		candidate := filepath.Join(current, comp)
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			current = candidate
			continue
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			current = candidate
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(current, target)
		}
		remainder := append([]string{target}, comps[i+1:]...)
		return resolveNonStrict(filepath.Join(remainder...), links+1)
	}
	return current
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
