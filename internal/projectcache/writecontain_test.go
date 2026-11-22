package projectcache

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubEvalSymlinks replaces the resolution seam for the duration of a test.
func stubEvalSymlinks(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := evalSymlinks
	evalSymlinks = fn
	t.Cleanup(func() { evalSymlinks = previous })
}

func realEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}
	return resolved
}

// A component that another sync run recreated between the failed resolution and
// the following Lstat is not a broken link: resolution must keep walking up to
// the deepest existing ancestor instead of failing the write.
func TestResolveWriteTargetToleratesComponentRecreatedBetweenCalls(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cache", "projects", "key")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(dir, "sync.lock")
	want := filepath.Join(realEvalSymlinks(t, dir), "sync.lock")

	previous := evalSymlinks
	injected := false
	stubEvalSymlinks(t, func(path string) (string, error) {
		if !injected && path == dir {
			injected = true
			return "", &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
		}
		return previous(path)
	})

	got, err := ResolveWriteTarget(leaf)
	if err != nil {
		t.Fatalf("ResolveWriteTarget(%s) errored on a recreated component: %v", leaf, err)
	}
	if got != want {
		t.Fatalf("ResolveWriteTarget = %q, want %q", got, want)
	}
	if err := ContainWrite(root, leaf, true, true, false); err != nil {
		t.Fatalf("ContainWrite refused the lock path: %v", err)
	}
}

// PRESERVE: a link that exists but cannot be resolved is still an error.
func TestResolveWriteTargetRefusesUnresolvableLinks(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string) string{
		"dangling symlink": func(t *testing.T, dir string) string {
			link := filepath.Join(dir, "dangling")
			if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(link, "leaf")
		},
		"symlink loop": func(t *testing.T, dir string) string {
			a, b := filepath.Join(dir, "loop-a"), filepath.Join(dir, "loop-b")
			if err := os.Symlink(b, a); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(a, b); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(a, "leaf")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := setup(t, dir)
			got, err := ResolveWriteTarget(path)
			if err == nil {
				t.Fatalf("ResolveWriteTarget(%s) = %q, want an error", path, got)
			}
			if err := ContainWrite(filepath.Dir(path), path, true, true, false); err == nil {
				t.Fatalf("ContainWrite accepted %s", path)
			} else if !isContainmentRefusal(err) {
				t.Fatalf("ContainWrite error = %v, want a containment refusal", err)
			}
		})
	}
}

// PRESERVE: a symlinked cache dir/entry resolving outside the root is refused.
func TestContainWriteRefusesSymlinksLeavingRoot(t *testing.T) {
	outside := t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "cache", "projects")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cache", "projects", "key", "sync.lock")

	err := ContainWrite(root, path, true, true, false)
	if err == nil {
		t.Fatalf("ContainWrite accepted %s", path)
	}
	if !isContainmentRefusal(err) {
		t.Fatalf("ContainWrite error = %v, want a containment refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "key")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("write landed outside the root: %v", statErr)
	}
}

// PRESERVE: the project's ai-specs/ and .git/ trees stay protected.
func TestContainWriteRefusesProtectedTrees(t *testing.T) {
	root := t.TempDir()
	for _, tree := range []string{"ai-specs", ".git"} {
		path := filepath.Join(root, tree, "agents", "brief.md")
		err := ContainWrite(root, path, false, false, true)
		if err == nil {
			t.Fatalf("ContainWrite accepted %s", path)
		}
		if !strings.Contains(err.Error(), "protected "+tree+"/ tree") {
			t.Fatalf("ContainWrite error = %v, want the protected %s/ refusal", err, tree)
		}
	}
}
