package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupModeIsRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--cleanup", "--dry-run", "--base", "main", "--dir", t.TempDir()}, strings.NewReader(""), &stdout, &stderr)
	if strings.Contains(stderr.String(), "unknown flag") {
		t.Fatalf("cleanup mode was rejected by the gate flag parser: %s", stderr.String())
	}
	// This test executes from the assigned linked worktree. Cleanup is required
	// to reject linked-worktree invocation, proving registration without
	// weakening the main-worktree boundary.
	if code != 2 {
		t.Fatalf("cleanup from linked worktree exit = %d, want 2; stderr: %s", code, stderr.String())
	}
}

func TestProtectedNamesIncludeConfiguredBranches(t *testing.T) {
	protected := protectedBranchSet("release", "integration")
	for _, branch := range []string{"main", "master", "development", "staging", "release", "integration"} {
		if !isProtectedBranch(protected, branch) {
			t.Fatalf("protected set does not contain %q", branch)
		}
	}
}

func TestProtectedNameRefusesEveryDestructiveEntryPoint(t *testing.T) {
	protected := protectedBranchSet("release", "integration")
	for _, kind := range []string{"worktree removal", "local branch deletion", "remote branch deletion"} {
		t.Run(kind, func(t *testing.T) {
			err := assertDeletable(kind, "development", protected)
			if err == nil {
				t.Fatalf("assertDeletable(%q) allowed protected branch", kind)
			}
			if !strings.Contains(err.Error(), "refusing destructive cleanup") {
				t.Fatalf("error = %q, want loud protected refusal", err)
			}
		})
	}
}

func cleanupGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func makeCleanupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cleanupGitTest(t, root, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", "README.md")
	cleanupGitTest(t, root, "commit", "-qm", "init")
	return root
}

func addCleanupWorktree(t *testing.T, root, branch string) string {
	t.Helper()
	path := filepath.Join(root, ".worktrees", branch)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "worktree", "add", "-q", "-b", branch, path, "main")
	return path
}

func TestCleanupPatchProofRejectsRevertedSquash(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-reverted-go")
	if err := os.WriteFile(filepath.Join(wt, "reverted-one.txt"), []byte("feature one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "feature one")
	if err := os.WriteFile(filepath.Join(wt, "reverted-two.txt"), []byte("feature two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "feature two")
	cleanupGitTest(t, root, "merge", "-q", "--squash", "feat-reverted-go")
	cleanupGitTest(t, root, "commit", "-qm", "squash")
	cleanupGitTest(t, root, "revert", "--no-edit", "HEAD")
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-reverted-go (unmerged)") {
		t.Fatalf("reverted squash was not preserved: %q", stdout.String())
	}
}

func TestCleanupPreservesNewlinePathInTreeProof(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-newline-go")
	name := "line\nbreak.txt"
	if err := os.WriteFile(filepath.Join(wt, name), []byte("branch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "newline")
	// Unrelated base content forces the combined-tree proof rather than a
	// coincidental equal combined patch.
	if err := os.WriteFile(filepath.Join(root, "base-extra.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", ".")
	cleanupGitTest(t, root, "commit", "-qm", "unrelated")
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-newline-go (unmerged)") {
		t.Fatalf("newline branch was not conservatively preserved: %q", stdout.String())
	}
}

func TestCleanupDryRunVisitsEveryCandidate(t *testing.T) {
	root := makeCleanupRepo(t)
	for _, branch := range []string{"feat-one", "feat-two"} {
		wt := addCleanupWorktree(t, root, branch)
		name := filepath.Join(wt, branch+".txt")
		if err := os.WriteFile(name, []byte(branch+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cleanupGitTest(t, wt, "add", ".")
		cleanupGitTest(t, wt, "commit", "-qm", branch)
		cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge "+branch, branch)
	}
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	for _, branch := range []string{"feat-one", "feat-two"} {
		if !strings.Contains(stdout.String(), "would remove "+branch) {
			t.Fatalf("output=%q missing candidate %s", stdout.String(), branch)
		}
	}
}

func TestCleanupEnumeratesAllInitializedModules(t *testing.T) {
	// The topology fixture is exercised by the Python integration suite; this
	// unit assertion locks the structural slice contract used by cleanup.
	if got := enumerateCleanupPasses(t.TempDir(), "standalone", nil); len(got) != 1 {
		t.Fatalf("standalone passes=%d, want one root pass", len(got))
	}
}

func TestProtectedConfiguredNamesRefuseImmediately(t *testing.T) {
	protected := protectedBranchSet("release", "integration")
	for _, branch := range []string{"release", "integration"} {
		if err := assertDeletable("worktree removal", branch, protected); err == nil {
			t.Fatalf("configured branch %q was allowed", branch)
		}
	}
}

func TestProtectedNamesRefuseEachDestructiveWrapper(t *testing.T) {
	root := t.TempDir()
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	record := worktreeRecord{path: filepath.Join(root, ".worktrees", "main"), branch: "main"}
	var output bytes.Buffer
	checks := []struct {
		name string
		call func() error
	}{
		{name: "worktree removal", call: func() error {
			return removeWorktreeCleanup(root, record, "main", cfg, nil, &output)
		}},
		{name: "local branch deletion", call: func() error {
			return removeLocalBranchCleanup(root, record, cfg, &output)
		}},
		{name: "remote branch deletion", call: func() error {
			return removeRemoteBranchCleanup(root, record, "origin", cfg, &output)
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), "refusing destructive cleanup") {
				t.Fatalf("protected %s returned %v, want loud refusal", check.name, err)
			}
		})
	}
}

func TestRemoteBranchDeletionIsVerified(t *testing.T) {
	root := makeCleanupRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}
	cleanupGitTest(t, root, "remote", "add", "origin", remote)
	cleanupGitTest(t, root, "push", "-q", "-u", "origin", "main")
	wt := addCleanupWorktree(t, root, "feat-remote")
	if err := os.WriteFile(filepath.Join(wt, "remote.txt"), []byte("remote\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, wt, "add", ".")
	gitTest(t, wt, "commit", "-qm", "remote")
	gitTest(t, wt, "push", "-q", "-u", "origin", "feat-remote")
	gitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge remote", "feat-remote")
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("cleanup exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if got := cleanupGitTest(t, root, "ls-remote", "--heads", "origin", "feat-remote"); got != "" {
		t.Fatalf("remote branch survived cleanup: %q", got)
	}
	if !strings.Contains(stdout.String(), "verified remote origin/feat-remote absent") {
		t.Fatalf("output=%q missing independent remote verification", stdout.String())
	}
}

func TestCleanupRequiresPrimaryMainWorktree(t *testing.T) {
	root := makeCleanupRepo(t)
	linked := addCleanupWorktree(t, root, "feat-linked")
	if err := requirePrimaryCleanupCheckout(linked); err == nil {
		t.Fatal("cleanup allowed invocation from linked worktree")
	}
	if err := requirePrimaryCleanupCheckout(root); err != nil {
		t.Fatalf("primary checkout rejected: %v", err)
	}
}

func TestCleanupPreservesUnmergedAndDirty(t *testing.T) {
	root := makeCleanupRepo(t)
	unmerged := addCleanupWorktree(t, root, "feat-unmerged")
	if err := os.WriteFile(filepath.Join(unmerged, "unmerged.txt"), []byte("not landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, unmerged, "add", ".")
	cleanupGitTest(t, unmerged, "commit", "-qm", "unmerged")
	dirty := addCleanupWorktree(t, root, "feat-dirty")
	if err := os.WriteFile(filepath.Join(dirty, "dirty.txt"), []byte("landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, dirty, "add", ".")
	cleanupGitTest(t, dirty, "commit", "-qm", "dirty")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge dirty", "feat-dirty")
	if err := os.WriteFile(filepath.Join(dirty, "working.txt"), []byte("uncommitted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-unmerged (unmerged)") {
		t.Fatalf("output=%q missing unmerged refusal", stdout.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-dirty (dirty)") {
		t.Fatalf("output=%q missing dirty refusal", stdout.String())
	}
}
