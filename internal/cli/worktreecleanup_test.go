package cli

// `ai-specs worktree-cleanup` (cli-served-recipes T3, S1/S4/S6).
//
// The verb replaces the retired materialized launcher. These tests pin its two
// contracts: no snapshot means exit 2 and nothing removed, and a snapshot runs
// the existing cleanup on the snapshot's policy only.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// cleanupRepo builds a repository with one merged and one unmerged worktree
// under .worktrees, both with commits of their own.
func cleanupRepo(t *testing.T) (project string) {
	t.Helper()
	project = t.TempDir()
	gitTest(t, project, "init", "-q", "-b", "main")
	gitTest(t, project, "config", "user.email", "t@t")
	gitTest(t, project, "config", "user.name", "t")
	writeCleanupFile(t, filepath.Join(project, "base.txt"), "base\n")
	gitTest(t, project, "add", ".")
	gitTest(t, project, "commit", "-q", "-m", "base")
	for _, branch := range []string{"feat-merged", "feat-open"} {
		gitTest(t, project, "branch", branch)
		worktree := filepath.Join(project, ".worktrees", branch)
		gitTest(t, project, "worktree", "add", worktree, branch)
		writeCleanupFile(t, filepath.Join(worktree, branch+".txt"), branch+"\n")
		gitTest(t, worktree, "add", branch+".txt")
		gitTest(t, worktree, "commit", "-q", "-m", branch)
	}
	gitTest(t, project, "merge", "-q", "--no-ff", "-m", "merge", "feat-merged")
	return project
}

func writeCleanupFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runWorktreeCleanupVerb(t *testing.T, home, project string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(worktreeGateProjectRootEnv, project)
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"worktree-cleanup"}, args...), home, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// S6: no snapshot means fail-closed — exit 2, one message, nothing removed.
func TestWorktreeCleanupWithoutSnapshotDeletesNothing(t *testing.T) {
	project := cleanupRepo(t)
	home := t.TempDir()
	code, stdout, stderr := runWorktreeCleanupVerb(t, home, project)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (fail-closed)", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	resolved, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("worktree-cleanup: %s has no ai-specs sync snapshot, "+
		"so there is no cleanup policy to apply and nothing was removed. Run `ai-specs sync` in that project first.\n", resolved)
	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
	if _, err := os.Stat(filepath.Join(project, ".worktrees", "feat-merged")); err != nil {
		t.Fatalf("fail-closed cleanup removed a worktree: %v", err)
	}
}

// S4/S2: with a snapshot the pass is the existing cleanup, driven by the
// snapshot values (here a custom worktrees dir), never by ai-specs.toml.
func TestWorktreeCleanupUsesSnapshotPolicy(t *testing.T) {
	project := cleanupRepo(t)
	home := t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(project, home, projectcache.RuntimeSnapshot{
		GateMode:          "ask",
		GateScope:         "auto",
		RepoTopology:      "standalone",
		Protected:         "main development",
		WorktreesDir:      ".worktrees",
		IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runWorktreeCleanupVerb(t, home, project)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".worktrees", "feat-merged")); !os.IsNotExist(err) {
		t.Fatalf("merged worktree survived cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".worktrees", "feat-open")); err != nil {
		t.Fatalf("unmerged worktree was removed: %v", err)
	}
	if branches := gitOutput(t, project, "branch", "--list", "feat-merged"); branches != "" {
		t.Fatalf("merged branch survived cleanup: %q", branches)
	}
	if branches := gitOutput(t, project, "branch", "--list", "feat-open"); branches == "" {
		t.Fatal("unmerged branch was removed")
	}
}

// --dry-run mutates nothing, and an unknown flag is a usage error, not a
// partial pass.
func TestWorktreeCleanupRejectsUnknownFlag(t *testing.T) {
	project := cleanupRepo(t)
	home := t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(project, home, projectcache.RuntimeSnapshot{
		RepoTopology: "standalone", WorktreesDir: ".worktrees", IntegrationBranch: "main", Protected: "main",
	}); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runWorktreeCleanupVerb(t, home, project, "--not-a-flag")
	if code != 2 || !strings.Contains(stderr, "unknown argument: --not-a-flag") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".worktrees", "feat-merged")); err != nil {
		t.Fatalf("a rejected invocation removed a worktree: %v", err)
	}
}
