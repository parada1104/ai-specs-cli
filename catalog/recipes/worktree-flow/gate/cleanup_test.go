package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCleanupModeIsRegistered proves the --cleanup flag reaches its dispatch.
//
// It must not depend on where the test process happens to run. An earlier
// version asserted exit 2, which only held because the dev checkout is a linked
// worktree: requirePrimaryCleanupCheckout refuses only when absolute-git-dir
// differs from git-common-dir, and in a plain clone they are equal. Release CI
// uses actions/checkout@v4 (a plain clone), so that assertion failed there on
// every tag push and broke the release build before any binary was produced.
//
// Registration is what this test is for, so it asserts only registration.
// The main-worktree boundary has its own hermetic test
// (TestCleanupRequiresPrimaryMainWorktree).
func TestCleanupModeIsRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--cleanup", "--dry-run", "--base", "main", "--dir", t.TempDir()}, strings.NewReader(""), &stdout, &stderr)
	if strings.Contains(stderr.String(), "unknown flag") {
		t.Fatalf("cleanup mode was rejected by the gate flag parser: %s", stderr.String())
	}
	// Either outcome proves the flag was recognized and dispatched: 0 when the
	// checkout is primary and no candidate matches, 2 when it is a linked
	// worktree and the boundary refuses. Anything else means it never reached
	// the cleanup path.
	if code != 0 && code != 2 {
		t.Fatalf("cleanup dispatch exit = %d, want 0 or 2; stderr: %s", code, stderr.String())
	}
}

// TestCleanupFailsClosedOnFlagError: the gate fails OPEN on a flag-parse error
// because it is non-destructive and must never wedge editing. Cleanup is
// destructive and must NOT inherit that. A version-skewed binary that does not
// recognize a cleanup flag has to say so, not report success while doing
// nothing — those two outcomes are indistinguishable to the caller.
func TestCleanupFailsClosedOnFlagError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--cleanup", "--not-a-real-cleanup-flag"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("destructive cleanup failed open on a flag error (exit 0); stderr: %s", stderr.String())
	}
}

// A flag error without --cleanup must still fail open: that is the gate's
// deliberate contract and this change must not tighten it.
func TestGateStillFailsOpenOnFlagError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--not-a-real-gate-flag"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("gate flag error should fail open, got exit %d", code)
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

// TestCleanupContinuesAfterOneCandidateFails covers spec.md:198 — "it MUST not
// stop after the first candidate".
//
// Both worktrees are merged and eligible. The first one's remote branch is
// already gone (the ordinary case where someone deleted it through the GitHub
// UI), which used to surface as a hard error and abandon the entire pass, so
// the second worktree was never even attempted — no output line, no status.
func TestCleanupContinuesAfterOneCandidateFails(t *testing.T) {
	root := makeCleanupRepo(t)
	remote := t.TempDir()
	cleanupGitTest(t, remote, "init", "-q", "--bare")
	cleanupGitTest(t, root, "remote", "add", "origin", remote)

	for _, branch := range []string{"feat-alpha", "feat-beta"} {
		wt := addCleanupWorktree(t, root, branch)
		if err := os.WriteFile(filepath.Join(wt, branch+".txt"), []byte(branch+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cleanupGitTest(t, wt, "add", ".")
		cleanupGitTest(t, wt, "commit", "-qm", branch)
		cleanupGitTest(t, wt, "push", "-q", "origin", branch)
		cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge "+branch, branch)
	}
	// The first candidate's remote ref disappears out from under cleanup.
	cleanupGitTest(t, root, "push", "-q", "origin", "--delete", "feat-alpha")

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	code := runCleanup(root, cfg, &stdout, &stderr)
	got := stdout.String() + stderr.String()

	if !strings.Contains(got, "feat-beta") {
		t.Fatalf("second candidate was never reported — the pass aborted on the first.\ncode=%d\n%s", code, got)
	}
	if strings.Contains(got, "feat-alpha") && !strings.Contains(got, "feat-beta") {
		t.Fatalf("only the failing candidate was reported: %s", got)
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
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "remote")
	cleanupGitTest(t, wt, "push", "-q", "-u", "origin", "feat-remote")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge remote", "feat-remote")
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

func TestCleanupDiscoversStaleLocalBranchWithNoWorktree(t *testing.T) {
	root := makeCleanupRepo(t)
	cleanupGitTest(t, root, "checkout", "-qb", "stale-existing")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("branch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", "shared.txt")
	cleanupGitTest(t, root, "commit", "-qm", "stale branch")
	cleanupGitTest(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", "shared.txt")
	cleanupGitTest(t, root, "commit", "-qm", "landed")

	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$1\" = pr ] && [ \"$2\" = list ] && printf '%s\\n' '[]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", filepath.Dir(gh)+string(os.PathListSeparator)+oldPath)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	// The branch wrote shared.txt="branch\n"; main independently wrote
	// shared.txt="landed\n". Same path, different content, NO merge. The
	// branch's work never landed, so it must be preserved. An earlier version
	// of this test asserted "would remove", encoding the defect as the
	// contract: path presence alone was accepted as proof of merge.
	out := stdout.String()
	if strings.Contains(out, "would remove stale-existing") {
		t.Fatalf("a never-merged branch was scheduled for deletion because a "+
			"same-named path exists on the base: %q", out)
	}
	if !strings.Contains(out, "stale-existing") {
		t.Fatalf("stale local branch was not discovered at all: %q", out)
	}
}

// TestStaleBranchWithLandedContentIsStillRemovable keeps the feature honest:
// preserving everything would be safe and useless. A stale branch whose commit
// genuinely landed must still be cleaned up.
func TestStaleBranchWithLandedContentIsStillRemovable(t *testing.T) {
	root := makeCleanupRepo(t)
	cleanupGitTest(t, root, "checkout", "-qb", "stale-landed")
	if err := os.WriteFile(filepath.Join(root, "landed.txt"), []byte("landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", "landed.txt")
	cleanupGitTest(t, root, "commit", "-qm", "stale landed")
	cleanupGitTest(t, root, "checkout", "-q", "main")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge stale-landed", "stale-landed")

	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$1\" = pr ] && [ \"$2\" = list ] && printf '%s\\n' '[]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(gh)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would remove stale-landed") {
		t.Fatalf("a genuinely merged stale branch was not cleaned up: %q", stdout.String())
	}
}

func TestCleanupRefusesStaleBranchWhoseMergeCannotBeProven(t *testing.T) {
	root := makeCleanupRepo(t)
	cleanupGitTest(t, root, "checkout", "-qb", "stale-unproven")
	if err := os.WriteFile(filepath.Join(root, "only-here.txt"), []byte("never landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", ".")
	cleanupGitTest(t, root, "commit", "-qm", "unproven work")
	cleanupGitTest(t, root, "checkout", "-q", "main")

	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$1\" = pr ] && [ \"$2\" = list ] && printf '%s\\n' '[]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(gh)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "would remove stale-unproven") {
		t.Fatalf("a branch with no provable merge was scheduled for deletion: %q", stdout.String())
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
