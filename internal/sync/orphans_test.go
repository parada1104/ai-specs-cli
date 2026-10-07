package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
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

// --- differential parity against the Python oracle -------------------------

// orphanRefDriver drives the REAL lib/_internal/recipe-materialize.py
// clean_orphans on two identical fixture trees. It builds both trees, runs the
// Python oracle on tree A, and prints the B-side facts the Go port needs plus
// the captured Python stdout. The pure-Python lock writer is pinned so the
// oracle never depends on a gate binary, and WORKTREE_GATE_BIN is cleared so
// the orphan bridge provably falls back to the retained Python authority (its
// GO_ORPHANS_BRIDGE_FALLBACK warning goes to stderr, which this driver does not
// capture).
const orphanRefDriver = `import contextlib
import importlib.util
import io
import json
import os
import sys
from pathlib import Path

os.environ.pop("WORKTREE_GATE_BIN", None)

repo_root = Path(sys.argv[1])
spec = json.load(sys.stdin)

mod_path = repo_root / "lib" / "_internal" / "recipe-materialize.py"
loader = importlib.util.spec_from_file_location("recipe_materialize_orphan_diff", mod_path)
mod = importlib.util.module_from_spec(loader)
sys.modules[loader.name] = mod
loader.loader.exec_module(mod)

# Pin the pure-Python lock writer: this oracle pins Python's lock bytes, not
# whatever gate binary may or may not be installed.
mod.write_lock = mod._lock_mod._write_lock_python

pc = mod._load_project_cache()
home = Path(spec["home"]).resolve()


def build(root, lock_recipes):
    (root / "ai-specs").mkdir(parents=True, exist_ok=True)
    for directory, names in (
        (pc.recipe_skills_root(root, cli_home=home), spec["recipes"]),
        (pc.deps_skills_root(root, cli_home=home), spec["deps"]),
        (pc.inproject_deps_root(root), spec["inproject"]),
    ):
        directory.mkdir(parents=True, exist_ok=True)
        (directory / "stray.txt").write_text("ignore me")
        for child in names:
            (directory / child).mkdir()
            (directory / child / "SKILL.md").write_text("x")
    if lock_recipes is not None:
        lines = ["# Managed by ai-specs. Do not edit by hand.", ""]
        for rid in lock_recipes:
            lines.append("[recipes.%s.skills.some-skill]" % rid)
            lines.append('SKILL.md = "deadbeef"')
            lines.append("")
        (root / "ai-specs" / ".ai-specs.lock").write_text("\n".join(lines) + "\n")


def roots_of(root):
    return {
        "recipe_skills": str(pc.recipe_skills_root(root, cli_home=home)),
        "deps_skills": str(pc.deps_skills_root(root, cli_home=home)),
        "inproject_deps": str(pc.inproject_deps_root(root)),
    }


def lock_text(root):
    path = root / "ai-specs" / ".ai-specs.lock"
    return path.read_text() if path.is_file() else ""


root_a = Path(spec["root_a"]).resolve()
root_b = Path(spec["root_b"]).resolve()
build(root_a, spec["lock_recipes"])
build(root_b, spec["lock_recipes"])

# plan_input from the untouched B tree, exactly as the acquisition step would.
roots_b = roots_of(root_b)
lock = mod.load_lock(root_b / "ai-specs" / ".ai-specs.lock")
plan_input = mod._orphan_plan_input(
    Path(roots_b["recipe_skills"]),
    Path(roots_b["deps_skills"]),
    Path(roots_b["inproject_deps"]),
    lock,
    set(spec["enabled"]),
    set(spec["expected"]),
)

buf = io.StringIO()
with contextlib.redirect_stdout(buf):
    mod.clean_orphans(root_a, set(spec["enabled"]), set(spec["expected"]), cli_home=home)

print(json.dumps({
    "roots_a": roots_of(root_a),
    "roots_b": roots_b,
    "lock_a": lock_text(root_a),
    "lock_path": str(root_b / "ai-specs" / ".ai-specs.lock"),
    "plan_input": plan_input,
    "stdout": buf.getvalue(),
}))
`

type orphanRefResult struct {
	RootsA    shared.OrphanApplyRoots `json:"roots_a"`
	RootsB    shared.OrphanApplyRoots `json:"roots_b"`
	LockA     string                  `json:"lock_a"`
	LockPath  string                  `json:"lock_path"`
	PlanInput shared.OrphanPlanInput  `json:"plan_input"`
	Stdout    string                  `json:"stdout"`
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

// TestCleanOrphansDifferentialAgainstPython runs the real Python clean_orphans
// on tree A and the native core on an identical tree B, then requires
// byte-identical stdout, identical trees, and identical lock bytes.
func TestCleanOrphansDifferentialAgainstPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "lib", "_internal", "recipe-materialize.py")); err != nil {
		t.Skipf("recipe-materialize.py not found under %s: %v", repoRoot, err)
	}
	driver := filepath.Join(t.TempDir(), "orphan_cleanup_ref.py")
	if err := os.WriteFile(driver, []byte(orphanRefDriver), 0o600); err != nil {
		t.Fatalf("write driver: %v", err)
	}

	cases := []struct {
		name string
		spec map[string]any
	}{
		{
			name: "all scopes orphaned with a stale lock",
			spec: map[string]any{
				"recipes": []string{"a", "gone"}, "deps": []string{"x", "gone-dep"},
				"inproject": []string{"gone-inproj"}, "lock_recipes": []string{"gone", "keep"},
				"enabled": []string{"keep"}, "expected": []string{"x"},
			},
		},
		{
			name: "nothing orphaned is a silent no-op",
			spec: map[string]any{
				"recipes": []string{"a"}, "deps": []string{"x"}, "inproject": []string{"x"},
				"lock_recipes": []string{"a"}, "enabled": []string{"a"}, "expected": []string{"x"},
			},
		},
		{
			name: "missing lock is never created",
			spec: map[string]any{
				"recipes": []string{"gone"}, "deps": []string{}, "inproject": []string{},
				"lock_recipes": nil, "enabled": []string{}, "expected": []string{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			home := filepath.Join(base, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatalf("mkdir home: %v", err)
			}
			tc.spec["home"] = home
			tc.spec["root_a"] = filepath.Join(base, "a")
			tc.spec["root_b"] = filepath.Join(base, "b")
			specJSON, err := json.Marshal(tc.spec)
			if err != nil {
				t.Fatalf("marshal spec: %v", err)
			}

			cmd := exec.Command("python3", driver, repoRoot)
			cmd.Stdin = bytes.NewReader(specJSON)
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "WORKTREE_GATE_BIN=")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
			}
			var ref orphanRefResult
			if err := json.Unmarshal(stdout.Bytes(), &ref); err != nil {
				t.Fatalf("parse oracle JSON: %v\nraw: %s", err, stdout.String())
			}

			var out bytes.Buffer
			outcome, err := CleanOrphans(OrphanCleanupInput{
				PlanInput: ref.PlanInput,
				Roots:     ref.RootsB,
				LockPath:  ref.LockPath,
				Output:    &out,
			}, os.RemoveAll)
			if err != nil {
				t.Fatalf("CleanOrphans: %v", err)
			}
			if outcome.Status != shared.StatusApplied {
				t.Fatalf("status = %q, want %q; error=%q", outcome.Status, shared.StatusApplied, outcome.Error)
			}

			if out.String() != ref.Stdout {
				t.Errorf("stdout diverges\n--- go ---\n%q\n--- python ---\n%q", out.String(), ref.Stdout)
			}
			if got, want := orphanTreeSnapshot(t, ref.RootsB.RecipeSkills), orphanTreeSnapshot(t, ref.RootsA.RecipeSkills); !reflect.DeepEqual(got, want) {
				t.Errorf("recipe tree diverges\n--- go ---\n%#v\n--- python ---\n%#v", got, want)
			}
			if got, want := orphanTreeSnapshot(t, ref.RootsB.DepsSkills), orphanTreeSnapshot(t, ref.RootsA.DepsSkills); !reflect.DeepEqual(got, want) {
				t.Errorf("deps tree diverges\n--- go ---\n%#v\n--- python ---\n%#v", got, want)
			}
			if got, want := orphanTreeSnapshot(t, ref.RootsB.InprojectDeps), orphanTreeSnapshot(t, ref.RootsA.InprojectDeps); !reflect.DeepEqual(got, want) {
				t.Errorf("in-project tree diverges\n--- go ---\n%#v\n--- python ---\n%#v", got, want)
			}
			if got := readOrphanBytesIfPresent(t, ref.LockPath); got != ref.LockA {
				t.Errorf("lock bytes diverge\n--- go ---\n%q\n--- python ---\n%q", got, ref.LockA)
			}
		})
	}
}
