package main

import (
	"os"
	"path/filepath"
	"strings"
)

// RealPath mirrors Python os.path.realpath (design decision D11): it never
// fails on a nonexistent tail, unlike filepath.EvalSymlinks. The existing
// prefix is resolved through symlinks component-wise, then the unresolved
// remainder is appended lexically. On any failure it returns the
// lexically-cleaned absolute path. This is what makes the macOS fixtures work,
// where /tmp -> /private/tmp and /var -> /private/var.
func RealPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	rest := abs
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(rest)
		if err == nil {
			if len(tail) == 0 {
				return resolved
			}
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
		parent := filepath.Dir(rest)
		if parent == rest {
			return filepath.Clean(abs)
		}
		tail = append([]string{filepath.Base(rest)}, tail...)
		rest = parent
	}
}

// Inside mirrors the reference inside() (worktree-gate-legacy.sh:384-388),
// including its ValueError -> False: it returns false when either side is not
// absolute. Cleaned paths are compared component-wise so /repo-evil is NOT
// inside /repo — a strings.HasPrefix port would be a real security bug
// (design decision D12).
func Inside(path, root string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	cleaned := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanRoot == string(filepath.Separator) {
		return true
	}
	return cleaned == cleanRoot || strings.HasPrefix(cleaned, cleanRoot+string(filepath.Separator))
}

// ExistingAncestor mirrors existing_ancestor() (worktree-gate-legacy.sh:390-398):
// absolutize, walk up until a component exists, return it if it is a directory
// else its parent, and return "" when the walk reaches the filesystem root
// without finding anything. Python's os.path.exists returns False for a broken
// symlink; os.Stat errors on one, which yields the same verdict.
func ExistingAncestor(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	probe := abs
	for {
		fi, statErr := os.Stat(probe)
		if statErr == nil {
			if fi.IsDir() {
				return probe
			}
			return filepath.Dir(probe)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return ""
		}
		probe = parent
	}
}
