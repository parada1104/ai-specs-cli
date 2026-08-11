package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGitMemoDerivesEachFactOnce(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.invalid"},
		{"config", "user.name", "t"},
	} {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README"}, {"commit", "-qm", "init"}} {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			t.Fatal(err)
		}
	}

	// A `git` shim on PATH counts subprocess invocations and delegates to the
	// real binary (task 2.11's counting mechanism, unit level).
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shimDir := t.TempDir()
	counter := filepath.Join(shimDir, "count")
	script := fmt.Sprintf("#!/usr/bin/env bash\necho x >> %q\nexec %q \"$@\"\n", counter, realGit)
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	count := func() int {
		data, err := os.ReadFile(counter)
		if err != nil {
			return 0
		}
		return len(strings.Fields(string(data)))
	}

	// Four distinct facts derived twice each: memoization collapses the
	// repeats, so exactly four derivations hit the git binary.
	for i := 0; i < 2; i++ {
		gitMemo(dir, "rev-parse", "--show-toplevel")
		gitMemo(dir, "rev-parse", "--absolute-git-dir")
		gitMemo(dir, "symbolic-ref", "--short", "HEAD")
		gitCommon(dir)
	}
	if got := count(); got != 4 {
		t.Fatalf("memoized git facts invoked git %d times, want 4", got)
	}

	// A failed lookup is memoized too: the first miss invokes git once
	// (caching ""), the repeat must not re-invoke it.
	for i := 0; i < 2; i++ {
		if v := gitMemo("/no/such/dir", "status"); v != "" {
			t.Fatalf("got %q", v)
		}
	}
	if got := count(); got != 5 {
		t.Fatalf("failed lookup re-invoked git: %d calls, want 5 (4 facts + 1 first miss)", got)
	}
}
