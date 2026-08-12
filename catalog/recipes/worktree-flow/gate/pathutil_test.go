package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealPathSymlinkedTmp covers the macOS fixture case where /tmp resolves
// through a symlink to /private/tmp: the existing prefix must be resolved, and
// the nonexistent tail appended lexically (design decision D11).
func TestRealPathSymlinkedTmp(t *testing.T) {
	target := filepath.Join(os.TempDir(), "worktree-gate-pathutil-test-"+t.Name())
	t.Cleanup(func() { _ = os.RemoveAll(target) })

	realTmp := RealPath(os.TempDir())
	if realTmp == os.TempDir() {
		t.Skip("tmpdir is not symlinked; RealPath prefix resolution not exercised")
	}

	if got := RealPath(filepath.Join(os.TempDir(), "missing")); got != filepath.Join(realTmp, "missing") {
		t.Fatalf("RealPath(missing tail) = %q, want %q", got, filepath.Join(realTmp, "missing"))
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(target, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RealPath(deep); got != filepath.Join(realTmp, "worktree-gate-pathutil-test-"+t.Name(), "a", "b", "c") {
		t.Fatalf("RealPath(deep) = %q, want %q", got, filepath.Join(realTmp, "worktree-gate-pathutil-test-"+t.Name(), "a", "b", "c"))
	}
}

// TestRealPathSymlinkPrefix exercises resolution through an actual symlink
// mid-path: a symlinked directory component must be resolved, not kept as-is.
func TestRealPathSymlinkPrefix(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := RealPath(filepath.Join(link, "sub", "missing"))
	// t.TempDir() may itself sit under a symlink (e.g. /var -> /private/var on
	// macOS), so the expectation is anchored on the resolved base: the point of
	// the test is that the link component resolves to real.
	want := filepath.Join(RealPath(dir), "real", "sub", "missing")
	if got != want {
		t.Fatalf("RealPath = %q, want %q", got, want)
	}
}

// TestRealPathRelative mirrors Python os.path.realpath on a relative input: it
// must be absolutized against the process cwd and resolved.
func TestRealPathRelative(t *testing.T) {
	got := RealPath(".")
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Clean(want)
	// The cwd may itself contain symlinks (e.g. /tmp on macOS), so compare
	// against the resolved cwd.
	want = RealPath(want)
	if got != want {
		t.Fatalf("RealPath(.) = %q, want %q", got, want)
	}
}

// TestInsideComponentWise is the security regression for design decision D12:
// sibling prefixes must never count as inside, including the /repo vs
// /repo-evil case from the design doc.
func TestInsideComponentWise(t *testing.T) {
	cases := []struct {
		name, path, root string
		want             bool
	}{
		{"same", "/repo", "/repo", true},
		{"child", "/repo/sub", "/repo", true},
		{"deep child", "/repo/a/b/c", "/repo", true},
		{"sibling prefix", "/repo-evil", "/repo", false},
		{"sibling prefix child", "/repo-evil/x", "/repo", false},
		{"file under sibling", "/repoevil", "/repo", false},
		{"relative path", "repo/sub", "/repo", false},
		{"relative root", "/repo/sub", "repo", false},
		{"root of fs", "/etc", "/", true},
		{"parent not inside child", "/repo", "/repo/sub", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Inside(tc.path, tc.root); got != tc.want {
				t.Fatalf("Inside(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

// TestInsideCleaning mirrors os.path.commonpath semantics: lexical noise
// (trailing slashes, "..") is cleaned before comparing.
func TestInsideCleaning(t *testing.T) {
	if !Inside("/repo//sub/", "/repo/") {
		t.Fatal("Inside should clean trailing slashes")
	}
	if !Inside("/repo/sub/../sub", "/repo") {
		t.Fatal("Inside should clean .. components")
	}
}

// TestExistingAncestor covers deep nonexistent trees, the deepest existing
// directory, an existing file (whose parent is returned), and the walk hitting
// the filesystem root.
func TestExistingAncestor(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, path, want string
	}{
		{"existing dir", sub, sub},
		{"deep nonexistent", filepath.Join(sub, "missing", "deeper", "x"), sub},
		{"existing file", file, sub},
		{"relative", "relative-missing-pathutil-" + t.Name(), func() string {
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			return wd
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExistingAncestor(tc.path); got != tc.want {
				t.Fatalf("ExistingAncestor(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestExistingAncestorRootWalk asserts the reachable root contract: a path
// whose entire directory chain under "/" does not exist resolves to the
// filesystem root. (The "" branch in ExistingAncestor is defensive only — it
// fires when the root itself does not exist, which cannot happen on a live
// filesystem.)
func TestExistingAncestorRootWalk(t *testing.T) {
	probe := string(filepath.Separator) + "definitely-not-a-real-dir-" + strings.ReplaceAll(t.Name(), "/", "-")
	got := ExistingAncestor(probe)
	want := string(filepath.Separator)
	if got != want {
		t.Fatalf("ExistingAncestor(%q) = %q, want %q", probe, got, want)
	}
}
