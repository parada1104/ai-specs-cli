package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitFactsAndCommon(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.invalid"}, {"config", "user.name", "t"}} {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := git(dir, "rev-parse", "--show-toplevel"); RealPath(got) != RealPath(dir) {
		t.Fatalf("root %q", got)
	}
	if got := gitCommon(dir); got == "" {
		t.Fatal("empty common")
	}
}

func TestGitFactsInvalidFailsOpen(t *testing.T) {
	if got := git("/no/such/repo", "status"); got != "" {
		t.Fatalf("got %q", got)
	}
}
