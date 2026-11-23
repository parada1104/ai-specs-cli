package sync

// CLI-served worktree cleanup at sync time (cli-served-recipes T3, S1/S4).
//
// `.git/hooks/post-merge` is a generated one-line CLI hook that sync ALWAYS
// regenerates, and the old user-editable cleanup launcher override is retired:
// every copy is removed, including a modified one, with one notice.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

func readCleanupHook(t *testing.T, project string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, ".git", "hooks", "post-merge"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// S4: the hook is exactly the shebang plus the CLI exec line, and it is
// executable.
func TestSyncWritesOneLineCleanupPostMergeHook(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if got, want := readCleanupHook(t, project), string(worktreePostMergeHook()); got != want {
		t.Fatalf("post-merge hook = %q, want %q", got, want)
	}
	info, err := os.Stat(filepath.Join(project, ".git", "hooks", "post-merge"))
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("post-merge hook is not executable: %v %v", info, err)
	}
}

// S4: a user edit is overwritten, because the hook is not an override.
func TestSyncRegeneratesEditedCleanupPostMergeHook(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	hook := filepath.Join(project, ".git", "hooks", "post-merge")
	spineWrite(t, hook, "#!/bin/sh\necho user hook\n", 0o755)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("resync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if got, want := readCleanupHook(t, project), string(worktreePostMergeHook()); got != want {
		t.Fatalf("edited hook was preserved: %q", got)
	}
}

// S1/S4: a user-modified override is removed with one notice, a managed copy is
// removed silently, and sync never writes the launcher again.
func TestSyncRetiresWorktreeCleanupOverride(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		managed    bool
		wantNotice bool
	}{
		{name: "user-modified", body: "#!/bin/sh\necho mine\n", wantNotice: true},
		{name: "not-yet-synced", body: "#!/bin/sh\necho mine\n", wantNotice: true},
		{name: "managed", body: "#!/bin/sh\necho managed\n", managed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := spineEnv(t)
			project := spineProject(t, cliServedManifest)
			rel := retiredWorktreeCleanupOverride
			path := filepath.Join(project, filepath.FromSlash(rel))
			// "not-yet-synced" runs the migration on a project that never synced
			// the recipe, so nothing else exists yet.
			if tc.name != "not-yet-synced" {
				if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
					t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
				}
			}
			spineWrite(t, path, tc.body, 0o755)
			if tc.managed {
				lk, err := lock.LoadLock(filepath.Join(project, "ai-specs", ".ai-specs.lock"))
				if err != nil {
					t.Fatal(err)
				}
				recipe, source, kind, policy := "worktree-flow", "templates/worktree-cleanup.sh", "template", "auto"
				lock.SetManagedOverride(lk, rel, lock.Sha256Bytes([]byte(tc.body)), &recipe, &source, &kind, &policy)
				if err := lock.WriteLock(filepath.Join(project, "ai-specs", ".ai-specs.lock"), lk); err != nil {
					t.Fatal(err)
				}
			}
			rc, stdout, stderr := runCLIServedSync(t, home, project)
			if rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("override survived sync: %v", err)
			}
			notice := "  ✓ removed user-modified worktree-cleanup.sh override " + rel + " (ai-specs now serves cleanup from the CLI)\n"
			if got := strings.Contains(stderr, notice); got != tc.wantNotice {
				t.Fatalf("notice present = %v, want %v\nstderr: %q", got, tc.wantNotice, stderr)
			}
			// A second sync must not resurrect the launcher.
			if rc, _, stderr := runCLIServedSync(t, home, project); rc != 0 {
				t.Fatalf("resync rc=%d %s", rc, stderr)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("override was materialized again: %v", err)
			}
		})
	}
}
