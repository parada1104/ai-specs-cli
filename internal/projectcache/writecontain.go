package projectcache

// Write containment (T6d of sync-ownership): every write and delete ai-specs
// performs resolves where it would ACTUALLY land and refuses when that leaves
// the tree it is allowed to touch. One mechanism for the sync package's agent
// destinations, the project tree and this package's own cache/lock paths.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContainWrite refuses a write or delete at path whose resolved location leaves
// root. followLeaf follows an existing symlink at path itself (a real write
// through it); false checks only the parent chain, for writes that REPLACE the
// leaf (agent skills/instruction links, the ai-specs mirror). strict also
// refuses ANY symlink below root — cache and lock paths ai-specs owns, where a
// link is never part of the contract. protect refuses the project's `ai-specs/`
// and `.git/` trees (agent destinations only: the project tree legitimately
// writes inside `ai-specs/`).
func ContainWrite(root, path string, followLeaf, strict, protect bool) error {
	probe := path
	if !followLeaf {
		probe = filepath.Dir(path)
	}
	// Resolve the root the same way as the target: it may not exist yet, and
	// the platform's own prefix links (/var → /private/var) must not decide the
	// comparison.
	rootReal, err := ResolveWriteTarget(root)
	if err != nil {
		rootReal = ResolvePath(root)
	}
	if strict {
		if err := refuseSymlinkedAncestors(rootReal, probe); err != nil {
			return refusal("refuse to write %s: %v", path, err)
		}
		if info, err := os.Lstat(probe); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return refusal("refuse to write %s: destination is a symlink (%s)", path, probe)
		}
	}
	effective, err := ResolveWriteTarget(probe)
	if err != nil {
		return refusal("refuse to write %s: %v", path, err)
	}
	if !pathContained(rootReal, effective) {
		return refusal("refuse to write %s: it resolves outside the project root (%s)", path, effective)
	}
	if protect {
		for _, tree := range []string{"ai-specs", ".git"} {
			if pathContained(filepath.Join(rootReal, tree), effective) {
				return refusal("refuse to write %s: it resolves inside the protected %s/ tree (%s)", path, tree, effective)
			}
		}
	}
	return nil
}

// containmentRefusal marks a refusal DECIDED by containment, as opposed to a
// filesystem error from the operation itself: callers phrase the two
// differently (nothing was attempted vs. it was attempted and failed).
type containmentRefusal struct{ msg string }

func (e containmentRefusal) Error() string { return e.msg }

func refusal(format string, args ...any) error {
	return containmentRefusal{msg: fmt.Sprintf(format, args...)}
}

// isContainmentRefusal reports whether err is a containment decision.
func isContainmentRefusal(err error) bool {
	var r containmentRefusal
	return errors.As(err, &r)
}

// evalSymlinks is a test seam: containment races can only be reproduced by
// interleaving the real resolution, not by planting another symlink.
var evalSymlinks = filepath.EvalSymlinks

// ResolveWriteTarget reports where a write to path would land: an existing
// symlink (leaf or ancestor) resolves to its target, and a not-yet-existing
// tail is re-joined onto the deepest existing ancestor. A link that exists but
// cannot be resolved — dangling or looping — is an error, never a guess.
func ResolveWriteTarget(path string) (string, error) {
	// Relative inputs are resolved against the process cwd: the comparison is
	// between absolute resolved paths, never a mix.
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	cur := filepath.Clean(path)
	var tail []string
	for {
		resolved, err := evalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		info, lerr := os.Lstat(cur)
		if lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			// Only a link that exists and cannot resolve is broken; a plain
			// component is just gone, or was recreated by a concurrent run.
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Join(append([]string{cur}, tail...)...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// pathContained reports whether dest stays inside root in the cleaned lexical
// sense (no `..` escape); callers pass already-resolved paths.
func pathContained(root, dest string) bool {
	rel, err := filepath.Rel(root, dest)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// refuseSymlinkedAncestors walks the ancestor directories of path below root,
// top down, and reports the first one that is a symlink.
func refuseSymlinkedAncestors(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return nil
	}
	cur := root
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		if info, statErr := os.Lstat(cur); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("a path component is a symlink: %s", cur)
		}
	}
	return nil
}
