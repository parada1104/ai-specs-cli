package sync

// CLI-served recipe docs at sync time (cli-served-recipes T5, S1).
//
// No recipe README.md is materialized into ai-specs/recipes/<id>/: the catalog
// keeps catalog/recipes/<id>/README.md as repository documentation. An older
// sync's copy is retired by the migration, including a user-edited one (one
// notice), because the project keeps no README there.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

const recipeREADMERel = "ai-specs/recipes/worktree-flow/README.md"

func recipeREADMENotice() string {
	return "  ✓ removed user-modified recipe README " + recipeREADMERel +
		" (ai-specs keeps recipe docs in its catalog)\n"
}

// S1: an enabled recipe that declares a README doc materializes none, records
// no ownership, and the catalog keeps the source.
func TestSyncDoesNotMaterializeRecipeREADME(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(recipeREADMERel))); !os.IsNotExist(err) {
		t.Fatalf("README was materialized into the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "catalog", "recipes", "worktree-flow", "README.md")); err != nil {
		t.Fatalf("catalog README is gone: %v", err)
	}
	lk, err := lock.LoadLock(filepath.Join(project, "ai-specs", ".ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := lk.Managed[recipeREADMERel]; recorded {
		t.Fatalf("lock recorded the CLI-served README")
	}
	// A resync must not resurrect it either.
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("resync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(recipeREADMERel))); !os.IsNotExist(err) {
		t.Fatalf("README came back on resync: %v", err)
	}
}

// S1: the migration removes an older sync's README copy — silently when the
// bytes are still the managed ones, with one notice when the user edited it. A
// copy no lock record proves ai-specs wrote is left alone: sync never deletes a
// file it cannot prove it owns.
func TestSyncPrunesRecipeREADMECopies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		edited     bool
		recorded   bool
		wantKept   bool
		wantNotice bool
	}{
		{name: "managed", recorded: true},
		{name: "user-modified", recorded: true, edited: true, wantNotice: true},
		{name: "untracked", wantKept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := spineEnv(t)
			project := spineProject(t, cliServedManifest)
			if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			path := filepath.Join(project, filepath.FromSlash(recipeREADMERel))
			spineWrite(t, path, "# old recipe readme\n", 0o644)
			lockPath := filepath.Join(project, "ai-specs", ".ai-specs.lock")
			if tc.recorded {
				lk, err := lock.LoadLock(lockPath)
				if err != nil {
					t.Fatal(err)
				}
				recipe, source, kind, policy := "worktree-flow", "README.md", "doc", "auto"
				lock.SetManagedOverride(lk, recipeREADMERel, lock.Sha256Bytes([]byte("# old recipe readme\n")),
					&recipe, &source, &kind, &policy)
				if err := lock.WriteLock(lockPath, lk); err != nil {
					t.Fatal(err)
				}
			}
			if tc.edited {
				spineWrite(t, path, "# my own notes\n", 0o644)
			}
			rc, stdout, stderr := runCLIServedSync(t, home, project)
			if rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			if _, err := os.Lstat(path); os.IsNotExist(err) == tc.wantKept {
				t.Fatalf("README kept = %v, want %v (stat err %v)", !tc.wantKept, tc.wantKept, err)
			}
			if got, want := strings.Contains(stderr, recipeREADMENotice()), tc.wantNotice; got != want {
				t.Fatalf("notice present = %v, want %v\nstderr: %q", got, want, stderr)
			}
			lk, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if tc.recorded {
				if _, recorded := lk.Managed[recipeREADMERel]; recorded {
					t.Fatalf("lock kept the retired README record")
				}
			}
		})
	}
}
