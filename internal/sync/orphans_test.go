package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/worktree-gate/shared"
)

// Native orphan-cleanup tests (GO-07 S10).
//
// CleanOrphans is the native port of the Python clean_orphans destructive
// executor. It reuses the shared PlanOrphans decision and the shared
// ApplyOrphans deletion core in-process, then owns the lock-pruning step
// (Python remove_recipe_lock_entries + write_lock) that the --apply-orphans
// command deliberately leaves to the caller. It also emits the success lines
// Python prints.
//
// The returned error is the lock channel: a lock read failure before deletion
// and a lock write failure after it must never be mistaken for a successful
// cleanup, even when the deletion status reads "applied".
//
// The tests pin the boundaries the Python oracle defines:
//   - success removes every planned directory, prints each removed line, and
//     prunes stale lock entries;
//   - a missing lock file is never created;
//   - stray files and final-component symlinks are preserved;
//   - the first remove failure stops the run (no retry) and preserves lock
//     bytes, while already-completed removals stay applied;
//   - a clean pre-apply failure deletes nothing and never prunes the lock;
//   - a malformed lock is a clean pre-apply stop (nothing deleted);
//   - a lock write failure is reported through the error channel.

// orphanRoots creates three absolute cache roots under one temp dir.
func orphanRoots(t *testing.T) shared.OrphanApplyRoots {
	t.Helper()
	base := t.TempDir()
	roots := shared.OrphanApplyRoots{
		RecipeSkills:  filepath.Join(base, "recipes"),
		DepsSkills:    filepath.Join(base, "deps"),
		InprojectDeps: filepath.Join(base, "inproject"),
	}
	for _, root := range []string{roots.RecipeSkills, roots.DepsSkills, roots.InprojectDeps} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir root %s: %v", root, err)
		}
	}
	return roots
}

// orphanDir materializes one cache directory with a payload so RemoveAll has
// content to delete.
func orphanDir(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(path, "payload"), 0o755); err != nil {
		t.Fatalf("mkdir orphan %s: %v", path, err)
	}
	return path
}

// orphanLockPath returns a lock path whose parent directory is never created,
// so the missing-lock case starts with no lock file.
func orphanLockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "ai-specs", ".ai-specs.lock")
}

// writeOrphanLock writes raw lock bytes (valid TOML) verbatim.
func writeOrphanLock(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir lock parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write lock %s: %v", path, err)
	}
}

// orphanStaleLockBody is a valid lock carrying one stale [recipes] owner. The
// legacy recipes section is read but never emitted by the writer, so a pruning
// rewrite collapses the file to the bare header: an observable bytes change.
func orphanStaleLockBody() string {
	return lock.LockHeader + "\n[recipes.\"old-recipe\".skills.\"demo\"]\n\"f.md\" = \"abc\"\n"
}

// readOrphanBytes returns a file's bytes, or fails the test.
func readOrphanBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// readOrphanBytesIfPresent returns a file's bytes, or "" when it is absent.
func readOrphanBytesIfPresent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestCleanOrphansSuccessPrunesStaleLock covers the happy path across all
// three scopes: each planned directory is removed in plan order, the success
// lines match Python byte for byte, and the stale lock entry is pruned.
func TestCleanOrphansSuccessPrunesStaleLock(t *testing.T) {
	roots := orphanRoots(t)
	recipe := orphanDir(t, roots.RecipeSkills, "old-recipe")
	dep := orphanDir(t, roots.DepsSkills, "old-dep")
	inproj := orphanDir(t, roots.InprojectDeps, "old-inproj")
	lockPath := orphanLockPath(t)
	writeOrphanLock(t, lockPath, orphanStaleLockBody())

	var calls []string
	remove := func(path string) error {
		calls = append(calls, path)
		return os.RemoveAll(path)
	}
	var out bytes.Buffer
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills:  []string{"old-recipe"},
			DepsSkills:    []string{"old-dep"},
			InprojectDeps: []string{"old-inproj"},
			LockRecipes:   []string{"old-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
		Output:   &out,
	}, remove)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusApplied {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
	}
	if len(outcome.Remaining) != 0 || outcome.Error != "" {
		t.Fatalf("remaining = %#v, error = %q, want a clean apply", outcome.Remaining, outcome.Error)
	}
	for _, path := range []string{recipe, dep, inproj} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("planned orphan %s still present (err=%v)", path, err)
		}
	}
	if want := []string{recipe, dep, inproj}; !reflect.DeepEqual(calls, want) {
		t.Errorf("remove calls = %#v, want %#v", calls, want)
	}
	wantOut := "  ✓ removed orphaned cache .recipe/old-recipe\n" +
		"  ✓ removed orphaned cache .deps/old-dep\n" +
		"  ✓ removed orphaned ai-specs/.deps/old-inproj\n" +
		"  ✓ removed stale lock entries for recipe 'old-recipe'\n"
	if out.String() != wantOut {
		t.Errorf("success output diverges\n--- got ---\n%q\n--- want ---\n%q", out.String(), wantOut)
	}
	if got := readOrphanBytes(t, lockPath); got != lock.LockHeader {
		t.Errorf("pruned lock bytes diverge\n--- got ---\n%q\n--- want ---\n%q", got, lock.LockHeader)
	}
}

// TestCleanOrphansMissingLockIsNotCreated pins Python's `if not
// lock_path.is_file(): return`: deletions still run, no lock file appears.
func TestCleanOrphansMissingLockIsNotCreated(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")
	lockPath := orphanLockPath(t)

	var out bytes.Buffer
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe"},
			LockRecipes:  []string{"old-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
		Output:   &out,
	}, os.RemoveAll)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusApplied {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan %s was not removed (err=%v)", orphan, err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("a missing lock must never be created (stat err=%v)", err)
	}
	if out.String() != "  ✓ removed orphaned cache .recipe/old-recipe\n" {
		t.Errorf("output = %q", out.String())
	}
}

// TestCleanOrphansPreservesStrayFilesAndSymlinks pins the shared executor's
// final-component guard: only directories planned as orphans are removed; a
// stray file and a symlink child are skipped without following the link.
func TestCleanOrphansPreservesStrayFilesAndSymlinks(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")

	stray := filepath.Join(roots.RecipeSkills, "stray.txt")
	if err := os.WriteFile(stray, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write stray: %v", err)
	}
	target := orphanDir(t, roots.RecipeSkills, "symlink-target")
	link := filepath.Join(roots.RecipeSkills, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var calls []string
	remove := func(path string) error {
		calls = append(calls, path)
		return os.RemoveAll(path)
	}
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe", "stray.txt", "link"},
		},
		Roots:    roots,
		LockPath: orphanLockPath(t),
	}, remove)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusApplied {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
	}
	if want := []string{orphan}; !reflect.DeepEqual(calls, want) {
		t.Errorf("remove calls = %#v, want only the directory %#v", calls, want)
	}
	if data, err := os.ReadFile(stray); err != nil || string(data) != "keep me" {
		t.Errorf("stray file changed: data=%q err=%v", data, err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink was removed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link mode = %v, want a symlink", info.Mode())
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("symlink target was touched: %v", err)
	}
}

// TestCleanOrphansStopsAfterFirstFailureAndPreservesLock pins the partial
// boundary: the first remove failure stops the run (the failing path is never
// retried), the already-completed removal stays applied, no success line is
// printed, and the lock bytes are preserved because pruning must not follow an
// uncertain outcome.
func TestCleanOrphansStopsAfterFirstFailureAndPreservesLock(t *testing.T) {
	roots := orphanRoots(t)
	first := orphanDir(t, roots.RecipeSkills, "first")
	second := orphanDir(t, roots.RecipeSkills, "second")
	third := orphanDir(t, roots.RecipeSkills, "third")
	lockPath := orphanLockPath(t)
	body := orphanStaleLockBody()
	writeOrphanLock(t, lockPath, body)

	var calls []string
	remove := func(path string) error {
		calls = append(calls, path)
		if len(calls) == 2 {
			return errors.New("injected remove failure")
		}
		return os.RemoveAll(path)
	}
	var out bytes.Buffer
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{filepath.Base(first), filepath.Base(second), filepath.Base(third)},
			LockRecipes:  []string{"old-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
		Output:   &out,
	}, remove)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusPartial {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusPartial, outcome.Error)
	}
	if want := []string{first, second}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("remove calls = %#v, want %#v (no retry after the first failure)", calls, want)
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Errorf("completed removal %s was not applied (err=%v)", first, err)
	}
	wantRemaining := []shared.OrphanApplyEntry{
		{Scope: "recipe_skills", Name: "second", Uncertain: true},
		{Scope: "recipe_skills", Name: "third"},
	}
	if !reflect.DeepEqual(outcome.Remaining, wantRemaining) {
		t.Errorf("remaining = %#v, want %#v", outcome.Remaining, wantRemaining)
	}
	if len(outcome.Removed) != 1 || outcome.Removed[0].Name != "first" {
		t.Errorf("removed = %#v, want exactly first", outcome.Removed)
	}
	if out.String() != "" {
		t.Errorf("a partial apply must not print a success line, got %q", out.String())
	}
	if got := readOrphanBytes(t, lockPath); got != body {
		t.Errorf("lock bytes changed after a partial failure\n--- got ---\n%q\n--- want ---\n%q", got, body)
	}
}

// TestCleanOrphansPreApplyFailureSkipsDeletionAndLockPrune pins the clean
// pre-apply boundary: a malformed envelope deletes nothing, and the lock is
// not pruned, so the caller keeps the established fallback path.
func TestCleanOrphansPreApplyFailureSkipsDeletionAndLockPrune(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")
	lockPath := orphanLockPath(t)
	body := orphanStaleLockBody()
	writeOrphanLock(t, lockPath, body)

	var calls []string
	remove := func(path string) error {
		calls = append(calls, path)
		return os.RemoveAll(path)
	}
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe"},
			LockRecipes:  []string{"old-recipe"},
		},
		Roots: shared.OrphanApplyRoots{
			RecipeSkills:  "relative-not-absolute",
			DepsSkills:    roots.DepsSkills,
			InprojectDeps: roots.InprojectDeps,
		},
		LockPath: lockPath,
	}, remove)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusPreApplyFailed {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusPreApplyFailed, outcome.Error)
	}
	if len(calls) != 0 {
		t.Errorf("remove calls = %#v, want none before a pre-apply failure", calls)
	}
	if _, err := os.Lstat(orphan); err != nil {
		t.Errorf("orphan %s was touched before validation (err=%v)", orphan, err)
	}
	if got := readOrphanBytes(t, lockPath); got != body {
		t.Errorf("a pre-apply failure must not prune the lock\n--- got ---\n%q\n--- want ---\n%q", got, body)
	}
}

// TestCleanOrphansSuccessWithoutStaleEntriesLeavesLockUntouched pins Python's
// `if removed_any: write_lock(...)`: when no stale entry was removed the lock
// is not rewritten, so non-canonical bytes survive verbatim.
func TestCleanOrphansSuccessWithoutStaleEntriesLeavesLockUntouched(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")
	lockPath := orphanLockPath(t)
	// The extra newline is non-canonical: a rewrite would collapse it.
	body := lock.LockHeader + "\n"
	writeOrphanLock(t, lockPath, body)

	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe"},
			// LockRecipes matches the enabled set: nothing stale to prune.
			LockRecipes:      []string{"active-recipe"},
			EnabledRecipeIDs: []string{"active-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
	}, os.RemoveAll)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}

	if outcome.Status != shared.StatusApplied {
		t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan %s was not removed (err=%v)", orphan, err)
	}
	if got := readOrphanBytes(t, lockPath); got != body {
		t.Errorf("lock must stay byte-identical when no stale entry was removed\n--- got ---\n%q\n--- want ---\n%q", got, body)
	}
}

// TestCleanOrphansLockReadFailureIsCleanPreApply pins the lock READ boundary:
// Python load_lock parses before any deletion, so a malformed lock stops the
// run with nothing deleted and the lock bytes intact. The error channel is the
// only success signal, so a caller can never mistake it for a cleanup.
func TestCleanOrphansLockReadFailureIsCleanPreApply(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")
	lockPath := orphanLockPath(t)
	body := "a = \"unterminated\n"
	writeOrphanLock(t, lockPath, body)

	var calls []string
	remove := func(path string) error {
		calls = append(calls, path)
		return os.RemoveAll(path)
	}
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe"},
			LockRecipes:  []string{"old-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
	}, remove)
	if err == nil {
		t.Fatal("a malformed lock must surface through the error channel, not look successful")
	}
	if outcome.Status != shared.StatusPreApplyFailed {
		t.Errorf("status = %q, want %q", outcome.Status, shared.StatusPreApplyFailed)
	}
	if len(calls) != 0 {
		t.Errorf("remove calls = %#v, want none after a lock read failure", calls)
	}
	if _, statErr := os.Lstat(orphan); statErr != nil {
		t.Errorf("orphan %s was touched before the lock parsed (err=%v)", orphan, statErr)
	}
	if got := readOrphanBytes(t, lockPath); got != body {
		t.Errorf("a lock read failure must not rewrite the lock\n--- got ---\n%q\n--- want ---\n%q", got, body)
	}
}

// TestCleanOrphansLockWriteFailureIsReported pins the lock WRITE boundary:
// deletion succeeds, but the prune rewrite fails, so the error channel (and
// only it) reports the failure. The lock bytes stay untouched. The failure is
// deterministic and permission-free: the lock path itself carries a control
// character, which lock.WriteLock refuses before writing.
func TestCleanOrphansLockWriteFailureIsReported(t *testing.T) {
	roots := orphanRoots(t)
	orphan := orphanDir(t, roots.RecipeSkills, "old-recipe")
	lockPath := filepath.Join(t.TempDir(), "ai-specs", ".ai-specs\n.lock")
	body := orphanStaleLockBody()
	writeOrphanLock(t, lockPath, body)

	var out bytes.Buffer
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills: []string{"old-recipe"},
			LockRecipes:  []string{"old-recipe"},
		},
		Roots:    roots,
		LockPath: lockPath,
		Output:   &out,
	}, os.RemoveAll)
	if err == nil {
		t.Fatal("a refused lock write must surface through the error channel")
	}
	if outcome.Status != shared.StatusApplied {
		t.Errorf("status = %q, want %q (the deletion itself completed)", outcome.Status, shared.StatusApplied)
	}
	if _, statErr := os.Lstat(orphan); !os.IsNotExist(statErr) {
		t.Errorf("orphan %s was not removed (err=%v)", orphan, statErr)
	}
	if !strings.Contains(out.String(), "removed stale lock entries") {
		t.Errorf("the prune attempt should be announced before the write, got %q", out.String())
	}
	if got := readOrphanBytes(t, lockPath); got != body {
		t.Errorf("a refused lock write must leave the lock untouched\n--- got ---\n%q\n--- want ---\n%q", got, body)
	}
}

// --- differential parity against the frozen Python oracle ------------------

// frozenOrphans is the frozen Python-oracle outcome for every differential
// fixture, captured live once from the real clean_orphans authority and
// serialized base-relative, so the test rebuilds identical trees without
// running Python.
type frozenOrphans struct {
	Cases []frozenOrphanCase `json:"cases"`
}

// frozenOrphanCase is one captured fixture: the build recipe, the
// base-relative roots and lock path (exactly where Python resolved them), the
// plan input Python derived from the untouched tree, Python's byte-exact
// stdout, and its post-run tree/lock facts.
type frozenOrphanCase struct {
	Name      string                  `json:"name"`
	Build     frozenOrphanBuild       `json:"build"`
	Roots     shared.OrphanApplyRoots `json:"roots"`
	LockPath  string                  `json:"lock_path"`
	PlanInput shared.OrphanPlanInput  `json:"plan_input"`
	Stdout    string                  `json:"stdout"`
	Tree      frozenOrphanTrees       `json:"tree"`
	LockInput string                  `json:"lock_input"`
	Lock      string                  `json:"lock"`
}

// frozenOrphanBuild mirrors the capture build: the child names per scope root
// and the stale lock recipe ids (nil builds no lock file).
type frozenOrphanBuild struct {
	Recipes     []string `json:"recipes"`
	Deps        []string `json:"deps"`
	Inproject   []string `json:"inproject"`
	LockRecipes []string `json:"lock_recipes"`
}

// frozenOrphanTrees holds the post-run tree A snapshots, relative to each
// scope root, in the same shape orphanTreeSnapshot produces.
type frozenOrphanTrees struct {
	RecipeSkills  map[string]string `json:"recipe_skills"`
	DepsSkills    map[string]string `json:"deps_skills"`
	InprojectDeps map[string]string `json:"inproject_deps"`
}

// orphanTreeSnapshot maps every relative path under root to its content (a
// trailing slash marks a directory), so two trees compare structurally.
func orphanTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			snap[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snap[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return snap
}

// expectedOrphanGoldenCases is the independent golden inventory pinned by the
// post-merge review of PR #493 (Cody's coverage finding): the golden file
// must carry exactly these captured cases, in order and without duplicates,
// so deleting or duplicating a record fails the test instead of silently
// shrinking both the stimulus and the expectation set.
var expectedOrphanGoldenCases = []string{
	"all scopes orphaned with a stale lock",
	"nothing orphaned is a silent no-op",
	"missing lock is never created",
}

// TestCleanOrphansDifferentialAgainstPython pins CleanOrphans against the
// frozen output of the real Python clean_orphans for the same fixtures: the
// first case exercises every scope plus a stale lock, the second proves the
// no-op, and the third pins the missing-lock boundary. Byte equality of the
// captured stdout, of the rebuilt tree structure, and of the lock bytes pins
// the whole observable oracle behavior without running Python.
func TestCleanOrphansDifferentialAgainstPython(t *testing.T) {
	frozenJSON, err := os.ReadFile(filepath.Join("testdata", "orphans_golden.json"))
	if err != nil {
		t.Fatalf("read frozen oracle: %v", err)
	}
	var frozen frozenOrphans
	if err := json.Unmarshal(frozenJSON, &frozen); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if len(frozen.Cases) == 0 {
		t.Fatal("the frozen oracle must carry at least one case")
	}
	// Independent golden inventory: exact case set, cardinality, and
	// uniqueness, checked before the case loop so a mutated golden cannot
	// yield an empty or silently reduced comparison.
	if len(frozen.Cases) != len(expectedOrphanGoldenCases) {
		t.Fatalf("frozen oracle carries %d cases, want exactly %d", len(frozen.Cases), len(expectedOrphanGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedOrphanGoldenCases {
		if got := frozen.Cases[i].Name; got != want {
			t.Fatalf("frozen case %d name = %q, want %q", i, got, want)
		}
		if seen[want] {
			t.Fatalf("duplicate frozen case name %q", want)
		}
		seen[want] = true
	}
	// Non-vacuity: the first fixture really removed orphaned cache
	// directories, so the comparisons below cannot pass on empty pairs.
	if !strings.Contains(frozen.Cases[0].Stdout, "removed orphaned cache") {
		t.Fatalf("frozen first-case stdout lost the removal lines: %q", frozen.Cases[0].Stdout)
	}

	for _, tc := range frozen.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			base := t.TempDir()
			root := func(rel string) string {
				return filepath.Join(base, filepath.FromSlash(rel))
			}
			roots := shared.OrphanApplyRoots{
				RecipeSkills:  root(tc.Roots.RecipeSkills),
				DepsSkills:    root(tc.Roots.DepsSkills),
				InprojectDeps: root(tc.Roots.InprojectDeps),
			}
			lockPath := root(tc.LockPath)

			// Rebuild tree B exactly as the capture's Python build created it.
			for _, scope := range []struct {
				relRoot string
				names   []string
			}{
				{tc.Roots.RecipeSkills, tc.Build.Recipes},
				{tc.Roots.DepsSkills, tc.Build.Deps},
				{tc.Roots.InprojectDeps, tc.Build.Inproject},
			} {
				dir := root(scope.relRoot)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir scope root %s: %v", dir, err)
				}
				if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("ignore me"), 0o644); err != nil {
					t.Fatalf("write stray: %v", err)
				}
				for _, name := range scope.names {
					if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
						t.Fatalf("mkdir %s: %v", name, err)
					}
					if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte("x"), 0o644); err != nil {
						t.Fatalf("write skill: %v", err)
					}
				}
			}
			if tc.LockInput != "" {
				writeOrphanLock(t, lockPath, tc.LockInput)
			}

			var out bytes.Buffer
			outcome, err := CleanOrphans(OrphanCleanupInput{
				PlanInput: tc.PlanInput,
				Roots:     roots,
				LockPath:  lockPath,
				Output:    &out,
			}, os.RemoveAll)
			if err != nil {
				t.Fatalf("CleanOrphans: %v", err)
			}
			if outcome.Status != shared.StatusApplied {
				t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
			}
			if out.String() != tc.Stdout {
				t.Errorf("stdout diverges\n--- go ---\n%q\n--- frozen python ---\n%q", out.String(), tc.Stdout)
			}
			if got, want := orphanTreeSnapshot(t, roots.RecipeSkills), tc.Tree.RecipeSkills; !reflect.DeepEqual(got, want) {
				t.Errorf("recipe tree diverges\n--- go ---\n%#v\n--- frozen python ---\n%#v", got, want)
			}
			if got, want := orphanTreeSnapshot(t, roots.DepsSkills), tc.Tree.DepsSkills; !reflect.DeepEqual(got, want) {
				t.Errorf("deps tree diverges\n--- go ---\n%#v\n--- frozen python ---\n%#v", got, want)
			}
			if got, want := orphanTreeSnapshot(t, roots.InprojectDeps), tc.Tree.InprojectDeps; !reflect.DeepEqual(got, want) {
				t.Errorf("in-project tree diverges\n--- go ---\n%#v\n--- frozen python ---\n%#v", got, want)
			}
			if got := readOrphanBytesIfPresent(t, lockPath); got != tc.Lock {
				t.Errorf("lock bytes diverge\n--- go ---\n%q\n--- frozen python ---\n%q", got, tc.Lock)
			}
		})
	}
}

// --- native acquisition + empty-scope boundary (S5-4 /c/) ----------------

// TestMaterializeOrphanInputsListDirectoriesOnly pins the native acquisition
// that replaced Python's _dir_child_names + _orphan_plan_input: only direct
// child DIRECTORIES are listed (a stray file and a symlink child are ignored),
// sorted, a missing root yields nothing, and the resolved roots are the three
// project-cache roots. Python's
// test_non_directory_children_are_never_listed_as_orphans asserted the same
// listing through the bridge.
func TestMaterializeOrphanInputsListDirectoriesOnly(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "project")
	home := t.TempDir()
	recipeDir := projectcache.RecipeSkillsRoot(projectRoot, home)
	for _, name := range []string{"b", "a"} {
		if err := os.MkdirAll(filepath.Join(recipeDir, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(recipeDir, "stray.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write stray: %v", err)
	}
	if err := os.Symlink(filepath.Join(recipeDir, "a"), filepath.Join(recipeDir, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// The deps root is deliberately missing and the in-project root empty.
	lockPath := filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")
	writeOrphanLock(t, lockPath, lock.LockHeader+
		"\n[recipes.\"old\".skills.\"demo\"]\n\"f.md\" = \"abc\"\n\n[recipes.\"a\".skills.\"demo\"]\n\"f.md\" = \"abc\"\n")

	input, roots := materializeOrphanInputs(projectRoot, home, []string{"b"}, []string{"z"})

	if want := []string{"a", "b"}; !reflect.DeepEqual(input.RecipeSkills, want) {
		t.Errorf("RecipeSkills = %#v, want %#v (directories only, sorted)", input.RecipeSkills, want)
	}
	if len(input.DepsSkills) != 0 {
		t.Errorf("DepsSkills = %#v, want nothing for a missing root", input.DepsSkills)
	}
	if len(input.InprojectDeps) != 0 {
		t.Errorf("InprojectDeps = %#v, want nothing for an empty root", input.InprojectDeps)
	}
	if want := []string{"a", "old"}; !reflect.DeepEqual(input.LockRecipes, want) {
		t.Errorf("LockRecipes = %#v, want %#v (sorted)", input.LockRecipes, want)
	}
	if want := []string{"b"}; !reflect.DeepEqual(input.EnabledRecipeIDs, want) {
		t.Errorf("EnabledRecipeIDs = %#v, want %#v", input.EnabledRecipeIDs, want)
	}
	if want := []string{"z"}; !reflect.DeepEqual(input.ExpectedDepIDs, want) {
		t.Errorf("ExpectedDepIDs = %#v, want %#v", input.ExpectedDepIDs, want)
	}
	wantRoots := shared.OrphanApplyRoots{
		RecipeSkills:  recipeDir,
		DepsSkills:    projectcache.DepsSkillsRoot(projectRoot, home),
		InprojectDeps: projectcache.InprojectDepsRoot(projectRoot),
	}
	if roots != wantRoots {
		t.Errorf("roots = %#v, want %#v", roots, wantRoots)
	}
}

// TestCleanOrphansEmptyRootsIsASilentNoOp pins Python's empty-dirs case: with
// no directory child in any scope nothing is planned, so the run is applied and
// completely silent and a missing lock file is never created.
func TestCleanOrphansEmptyRootsIsASilentNoOp(t *testing.T) {
	roots := orphanRoots(t)
	lockPath := orphanLockPath(t)
	var out bytes.Buffer
	outcome, err := CleanOrphans(OrphanCleanupInput{
		PlanInput: shared.OrphanPlanInput{
			RecipeSkills:     []string{},
			DepsSkills:       []string{},
			InprojectDeps:    []string{},
			EnabledRecipeIDs: []string{"a"},
			ExpectedDepIDs:   []string{"x"},
		},
		Roots:    roots,
		LockPath: lockPath,
		Output:   &out,
	}, os.RemoveAll)
	if err != nil {
		t.Fatalf("CleanOrphans: %v", err)
	}
	if outcome.Status != shared.StatusApplied {
		t.Errorf("status = %q, want %q", outcome.Status, shared.StatusApplied)
	}
	if out.String() != "" {
		t.Errorf("output = %q, want silence", out.String())
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Errorf("a missing lock must never be created (stat err=%v)", statErr)
	}
}
