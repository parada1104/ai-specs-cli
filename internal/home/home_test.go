package home

import (
	"os"
	"path/filepath"
	"testing"
)

// makeRoot builds a fake install tree:
//
//	root/bin/ai-specs              real launcher
//	root/real/bin/ai-specs         real launcher used through the symlink chain
//	root/link/bin/ai-specs         intermediate symlink (relative, 2 hops)
//	root/alias                     entry symlink (absolute)
//
// The relative target ../../real/bin/ai-specs is anchored at the link's
// directory (root/link/bin), so it climbs out of bin/ and link/ and lands on
// root/real — the same two-level climb bash performs with
// SOURCE="$DIR/$SOURCE".
func makeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"bin", "real/bin", "link/bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(root, "real", "bin", "ai-specs")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// link/bin/ai-specs -> ../../real/bin/ai-specs (relative target)
	if err := os.Symlink(filepath.Join("..", "..", "real", "bin", "ai-specs"),
		filepath.Join(root, "link", "bin", "ai-specs")); err != nil {
		t.Fatal(err)
	}
	// alias -> link/bin/ai-specs (absolute target)
	if err := os.Symlink(filepath.Join(root, "link", "bin", "ai-specs"),
		filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveHomeEnvOverrideWinsVerbatim(t *testing.T) {
	got := ResolveHome("/some/bin/ai-specs", "/custom/home")
	if got != "/custom/home" {
		t.Fatalf("env override must be returned verbatim, got %q", got)
	}
}

func TestResolveHomeEnvOverrideBeatsWalk(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "bin", "ai-specs"), "/override/home")
	if got != "/override/home" {
		t.Fatalf("env override must win over the walk, got %q", got)
	}
}

func TestResolveHomePlainPath(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "bin", "ai-specs"), "")
	want := root
	if got != want {
		t.Fatalf("home = %q, want %q", got, want)
	}
}

func TestResolveHomeSymlinkChain(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "alias"), "")
	want := filepath.Join(root, "real")
	if got != want {
		t.Fatalf("home = %q, want %q", got, want)
	}
}
