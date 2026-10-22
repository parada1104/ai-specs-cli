package sync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// writeRefreshFile writes one fixture file, creating parents.
func writeRefreshFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// bundledAssets writes the CLI home's bundled skills and commands.
func bundledAssets(t *testing.T, home string) {
	t.Helper()
	writeRefreshFile(t, filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
	writeRefreshFile(t, filepath.Join(home, "bundled-skills", "alpha", "reference.md"), "ref\n", 0o644)
	writeRefreshFile(t, filepath.Join(home, "bundled-skills", "alpha", "run.sh"), "#!/bin/sh\n", 0o755)
	writeRefreshFile(t, filepath.Join(home, "bundled-commands", "cmd-a.md"), "# cmd a\n", 0o644)
}

// refreshBundledCase is one differential fixture. setup builds the same
// project and cli-home tree in both legs.
type refreshBundledCase struct {
	name  string
	setup func(t *testing.T, project, home string)
}

// TestRefreshBundledDifferential runs the real refresh-bundled.py and the
// native port over twin fixtures and compares the project surface, the cache,
// the normalized lock, the exit code, stdout and stderr. The lock write's
// GO_LOCK_WRITE_BRIDGE_FALLBACK noise (the Python bridge's infrastructure
// warning, not refresh behaviour) is filtered from stderr.
func TestRefreshBundledDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repo := gitignoreRepoRoot(t)
	module := filepath.Join(repo, "lib", "_internal", "refresh-bundled.py")

	manifest := func(t *testing.T, project, _ string) {
		t.Helper()
		writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), "[project]\nname = 'x'\n", 0o644)
	}

	cases := []refreshBundledCase{
		{
			name: "fresh project",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				bundledAssets(t, home)
				manifest(t, project, home)
			},
		},
		{
			name: "all bundled up to date",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				if err := os.MkdirAll(home, 0o755); err != nil {
					t.Fatalf("mkdir home: %v", err)
				}
				manifest(t, project, home)
			},
		},
		{
			name: "stale cache rebuilt",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				bundledAssets(t, home)
				manifest(t, project, home)
				cache := projectcache.CacheRoot(project, home)
				writeRefreshFile(t, filepath.Join(cache, ".bundled", "skills", "stale", "SKILL.md"), "stale\n", 0o644)
				writeRefreshFile(t, filepath.Join(cache, ".bundled", "commands", "stale.md"), "stale\n", 0o644)
			},
		},
		{
			name: "legacy copies removed and lock normalized",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				bundledAssets(t, home)
				manifest(t, project, home)
				// Skill copy matches the current bundled source.
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				// Command copy differs from the source but matches the legacy
				// [commands] hash, the migration signal.
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "commands", "cmd-a.md"), "old command\n", 0o644)
				lockBody := fmt.Sprintf("[meta]\ncli_version = \"1.0.0\"\nsynced_at = \"2026-01-01T00:00:00Z\"\n\n"+
					"[skills.alpha]\n\"SKILL.md\" = \"%s\"\n\n"+
					"[commands]\n\"cmd-a.md\" = \"%s\"\n\n"+
					"[agents.claude]\n\"CLAUDE.md\" = \"deadbeef\"\n",
					lock.Sha256Bytes([]byte("# alpha\n")), lock.Sha256Bytes([]byte("old command\n")))
				writeRefreshFile(t, filepath.Join(project, "ai-specs", ".ai-specs.lock"), lockBody, 0o644)
			},
		},
		{
			name: "lock without meta",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				bundledAssets(t, home)
				manifest(t, project, home)
				lockBody := "[managed.\"ai-specs/foo.md\"]\nsha256 = \"  deadbeef  \"\nrecipe = \"r\"\n\n" +
					"[agents.claude]\n\"CLAUDE.md\" = \"deadbeef\"\n"
				writeRefreshFile(t, filepath.Join(project, "ai-specs", ".ai-specs.lock"), lockBody, 0o644)
			},
		},
		{
			name: "missing ai-specs directory",
			setup: func(t *testing.T, project, home string) {
				t.Helper()
				bundledAssets(t, home)
				if err := os.MkdirAll(project, 0o755); err != nil {
					t.Fatalf("mkdir project: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pyProject, nativeProject := t.TempDir(), t.TempDir()
			pyHome, nativeHome := t.TempDir(), t.TempDir()
			tc.setup(t, pyProject, pyHome)
			tc.setup(t, nativeProject, nativeHome)

			cmd := exec.Command("python3", module, pyProject, pyHome)
			cmd.Dir = repo
			cmd.Env = append(withoutEnv(os.Environ(), "WORKTREE_GATE_BIN"), "PYTHONDONTWRITEBYTECODE=1")
			var pyOut, pyErr bytes.Buffer
			cmd.Stdout = &pyOut
			cmd.Stderr = &pyErr
			pyRC := 0
			if err := cmd.Run(); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("run python refresh-bundled: %v", err)
				}
				pyRC = exitErr.ExitCode()
			}

			var nativeOut, nativeErr bytes.Buffer
			nativeRC := RefreshBundled(nativeProject, nativeHome, &nativeOut, &nativeErr)

			if nativeRC != pyRC {
				t.Fatalf("exit code: native = %d, python = %d\nnative stderr:\n%s\npython stderr:\n%s",
					nativeRC, pyRC, nativeErr.String(), pyErr.String())
			}
			if got, want := nativeOut.String(), pyOut.String(); got != want {
				t.Errorf("stdout:\n native = %q\n python = %q", got, want)
			}
			// The two legs run in different temp dirs, so a message that names the
			// project root compares with that root normalized away.
			gotErr := stripBridgeFallback(strings.ReplaceAll(nativeErr.String(), nativeProject, "<project>"))
			wantErr := stripBridgeFallback(strings.ReplaceAll(pyErr.String(), pyProject, "<project>"))
			if gotErr != wantErr {
				t.Errorf("stderr:\n native = %q\n python = %q", gotErr, wantErr)
			}

			compareRefreshTree(t, "project", snapshotRefreshTree(t, pyProject), snapshotRefreshTree(t, nativeProject))
			compareRefreshTree(t, "cache", snapshotRefreshTree(t, refreshCacheRoot(t, pyHome)), snapshotRefreshTree(t, refreshCacheRoot(t, nativeHome)))
		})
	}
}

// refreshCacheRoot returns the per-project cache directory under home, so the
// two legs' cache trees compare by relative path despite the cache key
// depending on the project's absolute path.
func refreshCacheRoot(t *testing.T, home string) string {
	t.Helper()
	projects := filepath.Join(home, "cache", "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	if len(entries) != 1 {
		t.Fatalf("cache projects entries = %d, want 1", len(entries))
	}
	return filepath.Join(projects, entries[0].Name())
}

// refreshTreeEntry is one tree node's comparable state.
type refreshTreeEntry struct {
	mode os.FileMode
	data []byte
	link string
}

// snapshotRefreshTree records every path under root (relative) with its mode,
// bytes or link target. A missing root is an empty tree.
func snapshotRefreshTree(t *testing.T, root string) map[string]refreshTreeEntry {
	t.Helper()
	out := map[string]refreshTreeEntry{}
	if root == "" {
		return out
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := refreshTreeEntry{mode: info.Mode()}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.link = target
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.data = data
		}
		out[rel] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// compareRefreshTree requires identical path sets, bytes, link targets and
// permission bits.
func compareRefreshTree(t *testing.T, label string, want, got map[string]refreshTreeEntry) {
	t.Helper()
	paths := make([]string, 0, len(want)+len(got))
	for path := range want {
		paths = append(paths, path)
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		wantEntry, wantOK := want[path]
		gotEntry, gotOK := got[path]
		switch {
		case !wantOK:
			t.Errorf("%s: extra path %s", label, path)
			continue
		case !gotOK:
			t.Errorf("%s: missing path %s", label, path)
			continue
		}
		if wantEntry.mode != gotEntry.mode {
			t.Errorf("%s: %s mode: native = %v, python = %v", label, path, gotEntry.mode, wantEntry.mode)
		}
		if !bytes.Equal(wantEntry.data, gotEntry.data) {
			t.Errorf("%s: %s bytes:\n native = %q\n python = %q", label, path, gotEntry.data, wantEntry.data)
		}
		if wantEntry.link != gotEntry.link {
			t.Errorf("%s: %s link: native = %q, python = %q", label, path, gotEntry.link, wantEntry.link)
		}
	}
}

// stripBridgeFallback removes the lock writer's GO_LOCK_WRITE_BRIDGE_FALLBACK
// warning, which is Python bridge infrastructure, not refresh behaviour.
func stripBridgeFallback(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "GO_LOCK_WRITE_BRIDGE_FALLBACK") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// withoutEnv returns env without the given keys.
func withoutEnv(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		drop := false
		for _, key := range keys {
			if strings.HasPrefix(entry, key+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, entry)
		}
	}
	return out
}
