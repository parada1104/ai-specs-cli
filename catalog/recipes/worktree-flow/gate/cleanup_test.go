package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
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

// TestCleanupProvesMergeForNewlinePathInTree is the falsifying half of the
// NUL-delimiter contract.
//
// TestCleanupPreservesNewlinePathInTreeProof only asserts that an unmerged
// newline path stays preserved — but "unmerged" is the outcome for ANY failed
// proof, so a regression from -z to line splitting would mangle the path,
// fail the proof, and still produce the expected output. That test cannot
// detect the bug it documents.
//
// Here the newline path IS genuinely landed on the base with identical
// content, so only a correctly reassembled path can prove it. Splitting the
// diff output on newlines yields "line" and "break.txt", neither of which is a
// tree entry, and the branch would be wrongly preserved.
func TestCleanupProvesMergeForNewlinePathInTree(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-newline-landed")
	name := "line\nbreak.txt"
	if err := os.WriteFile(filepath.Join(wt, name), []byte("branch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "newline")
	// The base lands the same content plus an unrelated file. The extra file
	// makes the combined patch id differ, so the tree-entry proof — the only
	// one that reads the path back — is what has to decide.
	if err := os.WriteFile(filepath.Join(root, name), []byte("branch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "base-extra.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", ".")
	cleanupGitTest(t, root, "commit", "-qm", "land newline path plus unrelated work")
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would remove feat-newline-landed") {
		t.Fatalf("newline path was not reassembled from the NUL-delimited diff: %q", stdout.String())
	}
}

// TestRemoteDeletionFailureLeavesLocalBranchForRetry pins the recovery order.
//
// Deleting the local branch before the remote one destroys the only handle a
// rerun has. Worktree and local branch are both gone, so the surviving remote
// branch is invisible to every later pass and the failure is unrecoverable
// without manual intervention. The remote must go first: it is the step that
// can fail for reasons outside this machine.
func TestRemoteDeletionFailureLeavesLocalBranchForRetry(t *testing.T) {
	root := makeCleanupRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}
	cleanupGitTest(t, root, "remote", "add", "origin", remote)
	cleanupGitTest(t, root, "push", "-q", "-u", "origin", "main")
	wt := addCleanupWorktree(t, root, "feat-orphan")
	if err := os.WriteFile(filepath.Join(wt, "orphan.txt"), []byte("orphan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "orphan")
	cleanupGitTest(t, wt, "push", "-q", "-u", "origin", "feat-orphan")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge orphan", "feat-orphan")
	// The remote becomes unreachable between the merge and the cleanup — a
	// network outage, a revoked token, a protected-branch rule.
	if err := os.RemoveAll(remote); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code == 0 {
		t.Fatalf("unreachable remote reported success: stdout=%q", stdout.String())
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/feat-orphan").Run(); err != nil {
		t.Fatalf("local branch was deleted before the remote failure, leaving nothing for a rerun to retry: stdout=%q", stdout.String())
	}
	// Surviving is only half the contract. Prove the rerun actually finishes
	// the job once the remote is reachable again: the stale-branch sweep has
	// to rediscover a branch whose worktree is already gone.
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}
	cleanupGitTest(t, root, "push", "-q", "origin", "main")
	cleanupGitTest(t, root, "push", "-q", "origin", "feat-orphan")
	var rerunOut, rerunErr bytes.Buffer
	if code := runCleanup(root, cfg, &rerunOut, &rerunErr); code != 0 {
		t.Fatalf("rerun exit=%d stdout=%q stderr=%q", code, rerunOut.String(), rerunErr.String())
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/feat-orphan").Run(); err == nil {
		t.Fatalf("rerun left the local branch behind: %q", rerunOut.String())
	}
	if got := cleanupGitTest(t, root, "ls-remote", "--heads", "origin", "feat-orphan"); got != "" {
		t.Fatalf("rerun left the remote branch behind: %q", got)
	}
}

// --- Flow-agnostic VCS close (T3) -------------------------------------------

// cleanupCommonLedgerPath returns the store path the cleanup path writes, using
// the same realpath(common-dir) derivation it does.
func cleanupCommonLedgerPath(t *testing.T, root string) string {
	t.Helper()
	common := RealPath(gitCommon(root))
	if common == "" {
		t.Fatal("test fixture has no git common dir")
	}
	return ledger.StorePath(common)
}

// readCleanupLedger loads the seeded store, failing the test on error.
func readCleanupLedger(t *testing.T, path string) ledger.Store {
	t.Helper()
	store, err := ledger.LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore(%s): %v", path, err)
	}
	return store
}

// seedCleanupLedger writes one open item whose identity includes a change slug,
// mirroring the real binding. Cleanup only has common dir + branch.
func seedCleanupLedger(t *testing.T, root, branch, change string) string {
	t.Helper()
	common := RealPath(gitCommon(root))
	path := ledger.StorePath(common)
	var store ledger.Store
	store.OpenItem(ledger.ItemIdentity{CommonDir: common, Branch: branch, Change: change}, "test-recipe", time.Now())
	if err := ledger.SaveStore(path, store); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	return path
}

func assertCleanupItemClosedAtArchiveClose(t *testing.T, path, branch string) {
	t.Helper()
	store := readCleanupLedger(t, path)
	if len(store.Items) != 1 {
		t.Fatalf("items = %+v, want one stored row", store.Items)
	}
	item := store.Items[0]
	if item.Status != ledger.StatusClosed {
		t.Fatalf("item %s status = %q, want closed", item.ID, item.Status)
	}
	last := item.Decisions[len(item.Decisions)-1]
	if last.Kind != ledger.DecisionClose || last.Checkpoint != ledger.CheckpointArchiveClose {
		t.Fatalf("last decision = %+v, want close at archive-close", last)
	}
	if item.Identity.Branch != branch {
		t.Fatalf("closed item branch = %q, want %q", item.Identity.Branch, branch)
	}
}

// TestCleanupClosesLedgerItemBeforeRemovingMergedWorktree is the core T3 seam:
// the merged worktree and its local branch are removed, and the matching ledger
// item — stored with a change slug cleanup never sees — is closed at
// archive-close.
func TestCleanupClosesLedgerItemBeforeRemovingMergedWorktree(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-ledger")
	if err := os.WriteFile(filepath.Join(wt, "ledger.txt"), []byte("ledger\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "ledger")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge ledger", "feat-ledger")
	path := seedCleanupLedger(t, root, "feat-ledger", "some-change-slug")

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("cleanup exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("merged worktree survived cleanup: err=%v", err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/feat-ledger").Run(); err == nil {
		t.Fatal("local branch survived cleanup")
	}
	assertCleanupItemClosedAtArchiveClose(t, path, "feat-ledger")
}

// TestCleanupDryRunNeverMutatesLedger pins the dry-run boundary: the candidate
// is reported but the ledger row stays open and the store is byte-identical.
func TestCleanupDryRunNeverMutatesLedger(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-ledger-dry")
	if err := os.WriteFile(filepath.Join(wt, "dry.txt"), []byte("dry\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "dry")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge dry", "feat-ledger-dry")
	path := seedCleanupLedger(t, root, "feat-ledger-dry", "dry-change")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("dry run removed the worktree: %v", err)
	}
	store := readCleanupLedger(t, path)
	if store.Items[0].Status != ledger.StatusOpen {
		t.Fatalf("dry run closed the item: status=%q", store.Items[0].Status)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("dry run rewrote the ledger store")
	}
}

// TestCleanupPreservesCandidateWhenLedgerStoreIsUnevaluable pins fail-closed for
// destructive cleanup: an unreadable ledger must not remove the candidate.
func TestCleanupPreservesCandidateWhenLedgerStoreIsUnevaluable(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-ledger-corrupt")
	if err := os.WriteFile(filepath.Join(wt, "corrupt.txt"), []byte("corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "corrupt")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge corrupt", "feat-ledger-corrupt")
	path := cleanupCommonLedgerPath(t, root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code == 0 {
		t.Fatalf("cleanup reported success despite an unevaluable ledger: stdout=%s", stdout.String())
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("candidate was removed despite a ledger failure: %v", err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/feat-ledger-corrupt").Run(); err != nil {
		t.Fatal("local branch was removed despite a ledger failure")
	}
}

// TestCleanupWithoutMatchingLedgerItemStillRemoves pins the no-op half: a store
// with no open row for the branch must not block removal or invent a row.
func TestCleanupWithoutMatchingLedgerItemStillRemoves(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-ledger-nomatch")
	if err := os.WriteFile(filepath.Join(wt, "nomatch.txt"), []byte("nomatch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "nomatch")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge nomatch", "feat-ledger-nomatch")
	// Seed an unrelated open item: it must survive untouched.
	other := seedCleanupLedger(t, root, "some-other-branch", "other-change")
	before, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("cleanup exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("unmatched ledger row blocked removal: err=%v", err)
	}
	after, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("an unmatched branch close mutated the store")
	}
}

// TestCleanupClosesLedgerForStaleMergedLocalBranch covers the second removal
// seam: a merged branch with no worktree still closes its ledger item before the
// local branch is deleted.
func TestCleanupClosesLedgerForStaleMergedLocalBranch(t *testing.T) {
	root := makeCleanupRepo(t)
	cleanupGitTest(t, root, "checkout", "-qb", "stale-ledger")
	if err := os.WriteFile(filepath.Join(root, "stale-ledger.txt"), []byte("landed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", ".")
	cleanupGitTest(t, root, "commit", "-qm", "stale ledger")
	cleanupGitTest(t, root, "checkout", "-q", "main")
	cleanupGitTest(t, root, "merge", "-q", "--no-ff", "-m", "merge stale-ledger", "stale-ledger")
	path := seedCleanupLedger(t, root, "stale-ledger", "stale-change")

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", false, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("cleanup exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/stale-ledger").Run(); err == nil {
		t.Fatal("merged stale branch survived cleanup")
	}
	assertCleanupItemClosedAtArchiveClose(t, path, "stale-ledger")
}

// --- Worktree port: provider merge-commit evidence (T2) ----------------------

// installFakeGh puts a `gh` script of the test's choosing first on PATH. The
// acquisition under test only ever reads provider evidence; the fake keeps the
// regression hermetic and returns the same JSON shape `gh pr list --json
// mergeCommit` does.
func installFakeGh(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// hideGhFromPath leaves only git reachable, so provider acquisition fails
// exactly as it does on a machine without the provider CLI. Cleanup must then
// preserve the candidate instead of guessing.
func hideGhFromPath(t *testing.T) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git is not on PATH: %v", err)
	}
	bin := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// squashMergeFixture builds the confirmed false negative: a clean linked
// worktree whose branch was squash-merged, then made unreachable to every local
// proof. Two branch commits make the squash a combined patch no single patch-id
// matches, and the base then advances over one of the branch's own paths with
// different content, so the combined-tree proof fails too.
//
// It returns the branch tip (provably not in base) and the squash merge commit
// (provably in base) so the tests can drive the provider both ways.
func squashMergeFixture(t *testing.T) (root, branchTip, mergeCommit string) {
	t.Helper()
	root = makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-squash-pr")
	if err := os.WriteFile(filepath.Join(wt, "pr-one.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "pr one")
	if err := os.WriteFile(filepath.Join(wt, "pr-two.txt"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "pr two")
	branchTip = cleanupGitTest(t, wt, "rev-parse", "HEAD")
	cleanupGitTest(t, root, "merge", "-q", "--squash", "feat-squash-pr")
	cleanupGitTest(t, root, "commit", "-qm", "squash merge feat-squash-pr")
	mergeCommit = cleanupGitTest(t, root, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "pr-one.txt"), []byte("advanced\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, root, "add", ".")
	cleanupGitTest(t, root, "commit", "-qm", "advance base over the branch path")
	return root, branchTip, mergeCommit
}

// TestCleanupProvesSquashMergeFromPRMergeCommit is the regression for the
// confirmed false negative: every local proof fails, but the provider records
// the squash merge commit and that commit is in the base, so the candidate is
// merged and cleanable.
func TestCleanupProvesSquashMergeFromPRMergeCommit(t *testing.T) {
	root, branchTip, mergeCommit := squashMergeFixture(t)
	installFakeGh(t, `printf '%s\n' '[{"headRefOid":"`+branchTip+`","mergeCommit":{"oid":"`+mergeCommit+`"}}]'`)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would remove feat-squash-pr") {
		t.Fatalf("a squash merge commit provably in base was not accepted: %q", stdout.String())
	}
}

// TestCleanupPreservesSquashMergeWhenPRCommitIsOutsideBase is the negative half.
// The same fixture proves the local proofs fail, so a provider commit that is
// NOT reachable from the base must leave the candidate preserved.
func TestCleanupPreservesSquashMergeWhenPRCommitIsOutsideBase(t *testing.T) {
	root, branchTip, _ := squashMergeFixture(t)
	installFakeGh(t, `printf '%s\n' '[{"headRefOid":"`+branchTip+`","mergeCommit":{"oid":"`+branchTip+`"}}]'`)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-squash-pr (unmerged)") {
		t.Fatalf("a provider commit outside the base was accepted: %q", stdout.String())
	}
}

// TestCleanupPreservesSquashMergeWhenGhIsMissing pins fail-closed acquisition:
// with no provider CLI there is no merge evidence, so the candidate survives
// even though the merge commit would have proven it.
func TestCleanupPreservesSquashMergeWhenGhIsMissing(t *testing.T) {
	root, _, _ := squashMergeFixture(t)
	hideGhFromPath(t)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-squash-pr (unmerged)") {
		t.Fatalf("a missing provider was not treated as no evidence: %q", stdout.String())
	}
}

// TestCleanupEvaluatesPRCommitAgainstEveryBaseCandidate pins the candidate set:
// the merge commit is only reachable from the remote-tracking base spelling, not
// from the local base ref. Checking a single spelling would wrongly preserve it.
func TestCleanupEvaluatesPRCommitAgainstEveryBaseCandidate(t *testing.T) {
	root, branchTip, mergeCommit := squashMergeFixture(t)
	cleanupGitTest(t, root, "remote", "add", "origin", filepath.Join(t.TempDir(), "origin.git"))
	// origin/main keeps the advanced history that contains the merge commit,
	// while the local base ref is rolled back behind it. Only the second
	// candidate spelling can still prove the merge.
	cleanupGitTest(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	cleanupGitTest(t, root, "update-ref", "refs/heads/main", mergeCommit+"^")
	installFakeGh(t, `printf '%s\n' '[{"headRefOid":"`+branchTip+`","mergeCommit":{"oid":"`+mergeCommit+`"}}]'`)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would remove feat-squash-pr") {
		t.Fatalf("the remote-tracking base candidate was not consulted: %q", stdout.String())
	}
}

// TestCleanupPreservesReusedBranchDespiteOlderMergedPR is the regression for
// the reused-branch defect: a branch is squash-merged, then reused for new
// unmerged commits. The provider still reports the old pull request and its
// merge commit is still reachable from base, but its recorded head is the old
// tip, not the reused tip. An unbound historical merge must never be accepted
// for the reused tip, so the branch is preserved.
func TestCleanupPreservesReusedBranchDespiteOlderMergedPR(t *testing.T) {
	root := makeCleanupRepo(t)
	wt := addCleanupWorktree(t, root, "feat-reused-pr")
	if err := os.WriteFile(filepath.Join(wt, "reused-one.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "merged head")
	mergedHead := cleanupGitTest(t, wt, "rev-parse", "HEAD")
	cleanupGitTest(t, root, "merge", "-q", "--squash", "feat-reused-pr")
	cleanupGitTest(t, root, "commit", "-qm", "squash merge feat-reused-pr")
	mergeCommit := cleanupGitTest(t, root, "rev-parse", "HEAD")
	// Reuse the branch: a new commit the base has never seen.
	if err := os.WriteFile(filepath.Join(wt, "reused-two.txt"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupGitTest(t, wt, "add", ".")
	cleanupGitTest(t, wt, "commit", "-qm", "reused unmerged work")
	reusedTip := cleanupGitTest(t, wt, "rev-parse", "HEAD")
	if reusedTip == mergedHead {
		t.Fatal("fixture did not reuse the branch")
	}
	installFakeGh(t, `printf '%s\n' '[{"headRefOid":"`+mergedHead+`","mergeCommit":{"oid":"`+mergeCommit+`"}}]'`)

	var stdout, stderr bytes.Buffer
	cfg := newCleanupConfig(root, ".worktrees", "main", "main", "standalone", true, nil)
	if code := runCleanup(root, cfg, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped feat-reused-pr (unmerged)") {
		t.Fatalf("an older merged pull request was accepted for the reused tip: %q", stdout.String())
	}
}

// TestGhMergeCommitsRunsInOwningRepository is the regression for the confirmed
// monorepo-submodules defect: cleanup is launched from the superproject, but a
// pass is bound to a submodule repoRoot. Inheriting the process cwd makes `gh`
// answer for the superproject while the caller evaluates the submodule, so the
// evidence is acquired from the wrong repository.
//
// The fake provider records the directory it actually ran in, which is the only
// observable that can tell the two repositories apart. `-P` (the /bin/pwd
// binary, not the shell builtin) reports the physical cwd, so an inherited PWD
// env var cannot mask the defect.
func TestGhMergeCommitsRunsInOwningRepository(t *testing.T) {
	repoRoot := t.TempDir()
	const wantHead = "headsha"
	// The alternate cases prove the pinned dir does not disturb the rest of the
	// contract: parsed evidence still comes back, the call still happened in the
	// owning root, and a head that cannot be bound to the candidate is discarded.
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{name: "no evidence", body: `printf '%s\n' '[]'`},
		{
			name: "one merge commit",
			body: `printf '%s\n' '[{"headRefOid":"` + wantHead + `","mergeCommit":{"oid":"abc123"}}]'`,
			want: []string{"abc123"},
		},
		{
			name: "head missing",
			body: `printf '%s\n' '[{"mergeCommit":{"oid":"abc123"}}]'`,
		},
		{
			name: "head mismatch",
			body: `printf '%s\n' '[{"headRefOid":"other","mergeCommit":{"oid":"abc123"}}]'`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "gh-cwd")
			installFakeGh(t, `/bin/pwd -P > '`+record+`'
`+tc.body)

			oids := ghMergeCommitsCleanup(repoRoot, "feat-cwd", wantHead)
			if strings.Join(oids, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("merge-commit evidence = %v, want %v", oids, tc.want)
			}
			raw, err := os.ReadFile(record)
			if err != nil {
				t.Fatalf("the fake provider never ran: %v", err)
			}
			if cwd := RealPath(strings.TrimSpace(string(raw))); cwd != RealPath(repoRoot) {
				t.Fatalf("gh ran in %q, want the owning repository root %q", cwd, RealPath(repoRoot))
			}
		})
	}
}
