package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolvePath resolves symlinks in the longest existing prefix of path, like
// Python's Path.resolve() for a path that does not exist yet.
func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	remainder := ""
	current := abs
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, remainder), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

func isWithin(base, path string) bool {
	return strings.HasPrefix(path, base+string(filepath.Separator))
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func planningRoot(root string) (string, error) {
	if !isDir(filepath.Join(root, "openspec")) {
		return "", fmt.Errorf("--root %s is not a planning root: %s is missing; pass the resolved "+
			"planning root, never the process cwd", root, filepath.Join(root, "openspec"))
	}
	return root, nil
}

func validateSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("invalid change slug '%s': a slug is one path segment with no separators", slug)
	}
	return nil
}

// canonicalTarget resolves a canonical destination inside openspec/specs/ or
// reports why it may not be used.
func canonicalTarget(specsBase, domain string) (string, error) {
	if !slugRe.MatchString(domain) {
		return "", fmt.Errorf("invalid spec domain name '%s'", domain)
	}
	target := filepath.Join(specsBase, domain, "spec.md")
	base, err := resolvePath(specsBase)
	if err != nil {
		return "", err
	}
	resolved, err := resolvePath(target)
	if err != nil {
		return "", err
	}
	if resolved != base && !isWithin(base, resolved) {
		return "", fmt.Errorf("canonical spec path %s escapes the repository planning tree at %s",
			target, specsBase)
	}
	for _, path := range []string{filepath.Dir(specsBase), specsBase,
		filepath.Join(specsBase, domain), target} {
		if isSymlink(path) {
			return "", fmt.Errorf("refusing to promote through symlinked path %s", path)
		}
	}
	return target, nil
}

// atomicWrite writes content through a same-directory temp file and a rename.
func atomicWrite(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	handle, err := os.CreateTemp(dir, ".spec-promotion-*.tmp")
	if err != nil {
		return err
	}
	temp := handle.Name()
	committed := false
	defer func() {
		if !committed {
			_ = handle.Close()
			_ = os.Remove(temp)
		}
	}()
	if _, err := handle.WriteString(content); err != nil {
		return err
	}
	if err := handle.Sync(); err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	committed = true
	return nil
}
