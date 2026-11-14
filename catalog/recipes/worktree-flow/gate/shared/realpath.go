package shared

import (
	"path/filepath"
)

// RealPath mirrors Python os.path.realpath (design decision D11): it never
// fails on a nonexistent tail, unlike filepath.EvalSymlinks. The existing
// prefix is resolved through symlinks component-wise, then the unresolved
// remainder is appended lexically. On any failure it returns the
// lexically-cleaned absolute path. This is what makes the macOS fixtures work,
// where /tmp -> /private/tmp and /var -> /private/var.
//
// Moved verbatim from the gate main package (pathutil.go) as part of GO-07 S13
// PR1, so the main package's RealPath and the shared witness writer share one
// resolution rule.
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
