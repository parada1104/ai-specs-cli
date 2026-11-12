package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// writeRefreshFile writes one fixture file, creating parents. The mode is set
// explicitly after the write so a fixture's 0o755 is not masked by the process
// umask: the frozen oracle records file modes, and this package must stay
// runnable under any umask.
func writeRefreshFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
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

// refreshBundledCase is one differential fixture: setup builds the project and
// cli-home tree the refresh runs over.
type refreshBundledCase struct {
	name  string
	setup func(t *testing.T, project, home string)
}

// refreshBundledCorpus is the in-test fixture corpus: the six project/cli-home
// shapes the live differential built, in capture order.
func refreshBundledCorpus() []refreshBundledCase {
	manifest := func(t *testing.T, project, _ string) {
		t.Helper()
		writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), "[project]\nname = 'x'\n", 0o644)
	}

	return []refreshBundledCase{
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
}

// The legacy refresh-bundled.py oracle was captured once into
// testdata/refreshbundled_golden.json: per fixture, the pre-run project and
// cli-home trees the setup wrote (so fixture drift fails as input drift), the
// exit code, stdout and stderr with the temporary project root and cli home
// tokenized as <DIR>, and the produced project and cache trees as sorted
// root-relative paths with their bytes and — for regular files only — their
// permission bits.
//
// Directory modes are deliberately not frozen: the cache tree is built with
// MkdirAll(0o777) plus copytree's copystat, so its directory modes follow the
// process umask, and this package also runs t.Parallel() tests that must not
// have the process umask pinned under them. Every file mode in the payload is
// set explicitly by a fixture or by copystat, so it is umask-independent. A
// run of this test under `umask 077` proves it.
//
// The test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const refreshBundledGoldenFile = "refreshbundled_golden.json"

// refreshBundledGoldenCases is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle of
// coverage — kept in the test, never derived from the bundle — so a bundle that
// is missing, renamed, duplicated or reordered cannot validate silently.
var refreshBundledGoldenCases = []string{
	"fresh project",
	"all bundled up to date",
	"stale cache rebuilt",
	"legacy copies removed and lock normalized",
	"lock without meta",
	"missing ai-specs directory",
}

// refreshBundledFile is one frozen tree node: a root-relative path, the bytes
// of a regular file (empty for a directory) and, for regular files only, the
// octal permission bits.
type refreshBundledFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

// refreshBundledGoldenCase is one frozen fixture: the pre-run inputs, the
// legacy exit code, stdout and stderr, and the produced project and cache
// trees.
type refreshBundledGoldenCase struct {
	Name         string               `json:"name"`
	InputProject []refreshBundledFile `json:"input_project"`
	InputHome    []refreshBundledFile `json:"input_home"`
	RC           int                  `json:"rc"`
	Stdout       string               `json:"stdout"`
	Stderr       string               `json:"stderr"`
	Project      []refreshBundledFile `json:"project"`
	Cache        []refreshBundledFile `json:"cache"`
}

// loadRefreshBundledGolden reads the frozen bundle and validates its identity
// inventory, its oracle metadata and the root-relative shape of every frozen
// path before any comparison runs.
func loadRefreshBundledGolden(t *testing.T) []refreshBundledGoldenCase {
	t.Helper()
	var bundle struct {
		Oracle map[string]string          `json:"oracle"`
		Cases  []refreshBundledGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, refreshBundledGoldenFile), &bundle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", refreshBundledGoldenFile, err)
	}
	if bundle.Oracle["python"] == "" {
		t.Fatalf("frozen oracle %s carries no python version metadata", refreshBundledGoldenFile)
	}
	names := make([]string, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		names[i] = rec.Name
		for _, tree := range [][]refreshBundledFile{rec.InputProject, rec.InputHome, rec.Project, rec.Cache} {
			for _, file := range tree {
				if filepath.IsAbs(file.Path) || strings.HasPrefix(file.Path, "..") {
					t.Fatalf("frozen record %q path %q must be root-relative", rec.Name, file.Path)
				}
			}
		}
	}
	validateGoldenIdentity(t, "refreshbundled", names, refreshBundledGoldenCases)
	return bundle.Cases
}

// refreshBundledTree lists one tree as sorted root-relative nodes: a directory
// as a bare path (its mode follows the umask) and a regular file with its bytes
// and octal mode. A missing root is an empty tree.
func refreshBundledTree(t *testing.T, root string) []refreshBundledFile {
	t.Helper()
	out := []refreshBundledFile{}
	if root == "" {
		return out
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("unexpected symlink %s", path)
		case info.IsDir():
			out = append(out, refreshBundledFile{Path: rel})
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, refreshBundledFile{
				Path:    rel,
				Content: string(data),
				Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// refreshBundledHomeInput lists the pre-run cli-home fixture with the
// per-project cache key — derived from the project's absolute temp path —
// replaced by <CACHE>, so the frozen input carries no host path.
func refreshBundledHomeInput(t *testing.T, home string) []refreshBundledFile {
	t.Helper()
	files := refreshBundledTree(t, home)
	for i, file := range files {
		parts := strings.Split(file.Path, "/")
		if len(parts) >= 3 && parts[0] == "cache" && parts[1] == "projects" {
			parts[2] = "<CACHE>"
			files[i].Path = strings.Join(parts, "/")
		}
	}
	return files
}

// compareRefreshBundledTree requires identical node inventories: the same paths
// in the same order, the same bytes and the same file modes.
func compareRefreshBundledTree(t *testing.T, label string, got, want []refreshBundledFile) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d nodes, want %d\n native = %v\n frozen = %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Path != want[i].Path {
			t.Fatalf("%s: node %d is %q, want %q", label, i, got[i].Path, want[i].Path)
		}
		if got[i].Content != want[i].Content || got[i].Mode != want[i].Mode {
			t.Errorf("%s: %s differs:\n native = %q mode %q\n frozen = %q mode %q",
				label, want[i].Path, got[i].Content, got[i].Mode, want[i].Content, want[i].Mode)
		}
	}
}

// TestRefreshBundledDifferential compares the native refresh port against the
// frozen legacy oracle for every fixture shape the live differential built: the
// fixture inputs, the exit code, stdout, stderr and the produced project and
// cache trees. The lock write's GO_LOCK_WRITE_BRIDGE_FALLBACK noise (the Python
// bridge's infrastructure warning, not refresh behaviour) is filtered from the
// native stderr, and the golden records no such line.
func TestRefreshBundledDifferential(t *testing.T) {
	frozen := loadRefreshBundledGolden(t)

	cases := refreshBundledCorpus()
	if len(cases) != len(refreshBundledGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(refreshBundledGoldenCases))
	}

	for i, tc := range cases {
		if tc.name != refreshBundledGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, refreshBundledGoldenCases[i])
		}
		rec := frozen[i]

		t.Run(tc.name, func(t *testing.T) {
			project, home := t.TempDir(), t.TempDir()
			tc.setup(t, project, home)

			// The fixture the freeze was captured from is an input of the payload:
			// a drifted setup fails here, not as an opaque payload mismatch.
			compareRefreshBundledTree(t, "project input", refreshBundledTree(t, project), rec.InputProject)
			compareRefreshBundledTree(t, "home input", refreshBundledHomeInput(t, home), rec.InputHome)

			var out, errW bytes.Buffer
			rc := RefreshBundled(project, home, &out, &errW)

			if rc != rec.RC {
				t.Errorf("exit code: native = %d, frozen = %d (frozen stderr %q)", rc, rec.RC, rec.Stderr)
			}
			// The project root and cli home are the only host-dependent values the
			// payload can carry (the missing-surface error names the resolved
			// root); both sides are compared in the tokenized space.
			if got := tokenizeCheckSyncDirs(out.String(), project, home); got != rec.Stdout {
				t.Errorf("stdout mismatch:\n native = %q\n frozen = %q", got, rec.Stdout)
			}
			if got := tokenizeCheckSyncDirs(stripBridgeFallback(errW.String()), project, home); got != rec.Stderr {
				t.Errorf("stderr mismatch:\n native = %q\n frozen = %q", got, rec.Stderr)
			}

			compareRefreshBundledTree(t, "project", refreshBundledTree(t, project), rec.Project)
			compareRefreshBundledTree(t, "cache", refreshBundledTree(t, refreshCacheRoot(t, home)), rec.Cache)
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
