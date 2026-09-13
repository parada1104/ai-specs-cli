package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRun runs a git command in dir and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// repo creates a committed git repository in a temp dir (unit 1 harness).
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "t@example.invalid")
	gitRun(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "README")
	gitRun(t, dir, "commit", "-qm", "init")
	return dir
}

// resolvedDir is the realpath of dir, the form identity MUST use (A2).
func resolvedDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// mkChange creates openspec/changes/<rel> under root.
func mkChange(t *testing.T, root string, rel ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(append([]string{root, "openspec", "changes"}, rel...)...), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestDeriveIdentityA2 pins A2: realpath(git common-dir) + short branch, and an
// explicit identity_unavailable (never a heuristic) for the non-derivable cases.
func TestDeriveIdentityA2(t *testing.T) {
	t.Run("named branch in a repository", func(t *testing.T) {
		dir := repo(t)
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if !ident.Available() {
			t.Fatalf("identity unavailable: %+v", ident)
		}
		if want := filepath.Join(resolvedDir(t, dir), ".git"); ident.CommonDir != want {
			t.Fatalf("common dir = %q, want realpath %q", ident.CommonDir, want)
		}
		if ident.Branch == "" || strings.ContainsAny(ident.Branch, " \n") {
			t.Fatalf("branch = %q, want a short branch name", ident.Branch)
		}
		if want := ident.CommonDir + "\x1f" + ident.Branch; ident.Key != want {
			t.Fatalf("key = %q, want %q", ident.Key, want)
		}
	})

	t.Run("detached HEAD is unavailable", func(t *testing.T) {
		dir := repo(t)
		gitRun(t, dir, "checkout", "-q", "--detach")
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Reason != ReasonIdentityUnavailable {
			t.Fatalf("reason = %q, want %q (%+v)", ident.Reason, ReasonIdentityUnavailable, ident)
		}
		if ident.Key != "" || ident.Branch != "" {
			t.Fatalf("unavailable identity must not name a key or branch: %+v", ident)
		}
	})

	t.Run("unborn branch is unavailable", func(t *testing.T) {
		dir := t.TempDir()
		gitRun(t, dir, "init", "-q")
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Reason != ReasonIdentityUnavailable {
			t.Fatalf("reason = %q, want %q (%+v)", ident.Reason, ReasonIdentityUnavailable, ident)
		}
	})

	t.Run("no git common dir is unavailable", func(t *testing.T) {
		ident := DeriveIdentity(IdentityOptions{Dir: t.TempDir()})
		if ident.Reason != ReasonIdentityUnavailable {
			t.Fatalf("reason = %q, want %q (%+v)", ident.Reason, ReasonIdentityUnavailable, ident)
		}
		if ident.CommonDir != "" {
			t.Fatalf("common dir = %q, want empty", ident.CommonDir)
		}
	})
}

// TestDeriveIdentityUsesInjectedGitFacts proves the identity derivation is
// driven by the injected facts reader (the dispatcher passes the worktree gate's
// memoized gitfacts helpers) and that a nil reader falls back to its own.
func TestDeriveIdentityUsesInjectedGitFacts(t *testing.T) {
	dir := t.TempDir()
	common := filepath.Join(dir, ".git")
	canned := map[string]string{
		strings.Join([]string{"rev-parse", "--path-format=absolute", "--git-common-dir"}, " "): common,
		strings.Join([]string{"symbolic-ref", "--quiet", "--short", "HEAD"}, " "):              "feat/injected",
		strings.Join([]string{"rev-parse", "--verify", "--quiet", "HEAD"}, " "):                "0123456789abcdef",
	}
	facts := func(_ string, args ...string) string { return canned[strings.Join(args, " ")] }

	ident := DeriveIdentity(IdentityOptions{Dir: dir, Facts: facts})
	if ident.Branch != "feat/injected" {
		t.Fatalf("branch = %q, want the injected short branch", ident.Branch)
	}
	if want := filepath.Join(resolvedDir(t, dir), ".git"); ident.CommonDir != want {
		t.Fatalf("common dir = %q, want %q", ident.CommonDir, want)
	}
	if want := ident.CommonDir + "\x1ffeat/injected"; ident.Key != want {
		t.Fatalf("key = %q, want %q", ident.Key, want)
	}
}

// TestDeriveIdentitySlugEnrichment pins A11: exactly one active change folder
// enriches the identity; zero enriches nothing; several omit the slug and flag
// the change-ambiguous collision.
func TestDeriveIdentitySlugEnrichment(t *testing.T) {
	t.Run("exactly one active folder enriches", func(t *testing.T) {
		dir := repo(t)
		mkChange(t, dir, "one-slug")
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Change != "one-slug" {
			t.Fatalf("change = %q, want %q", ident.Change, "one-slug")
		}
		if ident.Collision != "" {
			t.Fatalf("collision = %q, want none", ident.Collision)
		}
		if want := ident.CommonDir + "\x1f" + ident.Branch + "\x1fone-slug"; ident.Key != want {
			t.Fatalf("key = %q, want %q", ident.Key, want)
		}
	})

	t.Run("zero active folders enrich nothing", func(t *testing.T) {
		dir := repo(t)
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Change != "" || ident.Collision != "" {
			t.Fatalf("change/collision = %q/%q, want empty/empty", ident.Change, ident.Collision)
		}
	})

	t.Run("archive is not an active folder", func(t *testing.T) {
		dir := repo(t)
		mkChange(t, dir, "archive", "2026-01-05-old-slug")
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Change != "" || ident.Collision != "" {
			t.Fatalf("change/collision = %q/%q, want empty/empty", ident.Change, ident.Collision)
		}
	})

	t.Run("several active folders omit the slug and collide", func(t *testing.T) {
		dir := repo(t)
		mkChange(t, dir, "one-slug")
		mkChange(t, dir, "two-slug")
		ident := DeriveIdentity(IdentityOptions{Dir: dir})
		if ident.Change != "" {
			t.Fatalf("change = %q, want omitted", ident.Change)
		}
		if ident.Collision != CollisionChangeAmbiguous {
			t.Fatalf("collision = %q, want %q", ident.Collision, CollisionChangeAmbiguous)
		}
		if !ident.Available() {
			t.Fatalf("ambiguity must not make identity unavailable: %+v", ident)
		}
	})

	t.Run("planning root is separate from the owner repo", func(t *testing.T) {
		dir := repo(t)
		planning := t.TempDir()
		mkChange(t, planning, "planning-slug")
		ident := DeriveIdentity(IdentityOptions{Dir: dir, PlanningRoot: planning})
		if ident.Change != "planning-slug" {
			t.Fatalf("change = %q, want the planning-root slug", ident.Change)
		}
		if want := filepath.Join(resolvedDir(t, dir), ".git"); ident.CommonDir != want {
			t.Fatalf("common dir = %q, want the owner repo %q", ident.CommonDir, want)
		}
	})
}

// TestDeriveIdentityKeepsStoredSlugWhenArchived pins the "change archived
// mid-item" row: the stored slug is kept and resolves through the archive order.
func TestDeriveIdentityKeepsStoredSlugWhenArchived(t *testing.T) {
	dir := repo(t)
	mkChange(t, dir, "archive", "2026-02-02-archived-slug")
	ident := DeriveIdentity(IdentityOptions{Dir: dir, StoredSlug: "archived-slug"})
	if ident.Change != "archived-slug" {
		t.Fatalf("change = %q, want the stored slug", ident.Change)
	}
	if ident.Collision != "" {
		t.Fatalf("collision = %q, want none", ident.Collision)
	}
	if got := ResolveChangeDir(dir, "archived-slug"); got != filepath.Join(dir, "openspec", "changes", "archive", "2026-02-02-archived-slug") {
		t.Fatalf("resolved dir = %q", got)
	}

	// A stored slug wins over active-folder ambiguity: the item already knows
	// which change it belongs to.
	mkChange(t, dir, "one-slug")
	mkChange(t, dir, "two-slug")
	if ident := DeriveIdentity(IdentityOptions{Dir: dir, StoredSlug: "archived-slug"}); ident.Change != "archived-slug" || ident.Collision != "" {
		t.Fatalf("stored slug must survive active-folder ambiguity: %+v", ident)
	}

	// New work with no stored slug still collides.
	if ident := DeriveIdentity(IdentityOptions{Dir: dir}); ident.Change != "" || ident.Collision != CollisionChangeAmbiguous {
		t.Fatalf("fresh derivation = %+v, want change-ambiguous", ident)
	}
}

// TestDeriveIdentityRenameIsNewIdentity pins "a rename is a new identity" (A2).
func TestDeriveIdentityRenameIsNewIdentity(t *testing.T) {
	dir := repo(t)
	before := DeriveIdentity(IdentityOptions{Dir: dir})
	gitRun(t, dir, "branch", "-m", "renamed-branch")
	after := DeriveIdentity(IdentityOptions{Dir: dir})
	if before.Key == after.Key {
		t.Fatalf("rename kept key %q", before.Key)
	}
	if after.Branch != "renamed-branch" {
		t.Fatalf("branch = %q, want renamed-branch", after.Branch)
	}
}

// TestDeriveIdentitySharesOneIdentityAcrossWorktrees pins D2: the same branch in
// two worktrees is one identity because the common dir is shared.
func TestDeriveIdentitySharesOneIdentityAcrossWorktrees(t *testing.T) {
	dir := repo(t)
	gitRun(t, dir, "branch", "-m", "shared-branch")
	wt := filepath.Join(t.TempDir(), "linked")
	gitRun(t, dir, "worktree", "add", "--force", "-q", wt, "shared-branch")

	main := DeriveIdentity(IdentityOptions{Dir: dir})
	linked := DeriveIdentity(IdentityOptions{Dir: wt})
	if main.Key == "" || main.Key != linked.Key {
		t.Fatalf("worktree identity divergence: main=%+v linked=%+v", main, linked)
	}
	if main.CommonDir != linked.CommonDir || main.Branch != linked.Branch {
		t.Fatalf("worktrees must share common dir and branch: main=%+v linked=%+v", main, linked)
	}
}

// TestResolveChangeDirArchiveOrder pins A11's archive-aware order against the
// semantics of tests/_change_paths.py::change_dir (active, latest dated archive,
// legacy undated, active-shaped fallback).
func TestResolveChangeDirArchiveOrder(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "openspec", "changes", "slug")
	latest := filepath.Join(root, "openspec", "changes", "archive", "2026-03-03-slug")
	older := filepath.Join(root, "openspec", "changes", "archive", "2026-01-01-slug")
	malformed := filepath.Join(root, "openspec", "changes", "archive", "2026-99-99-slug")
	legacy := filepath.Join(root, "openspec", "changes", "archive", "slug")
	for _, dir := range []string{latest, older, malformed, legacy} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got := ResolveChangeDir(root, "slug"); got != latest {
		t.Fatalf("dated archive order: got %q, want latest valid %q", got, latest)
	}
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveChangeDir(root, "slug"); got != active {
		t.Fatalf("active must win: got %q, want %q", got, active)
	}
	if err := os.RemoveAll(active); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(latest); err != nil {
		t.Fatal(err)
	}
	if got := ResolveChangeDir(root, "slug"); got != older {
		t.Fatalf("older dated archive: got %q, want %q", got, older)
	}
	if err := os.RemoveAll(older); err != nil {
		t.Fatal(err)
	}
	if got := ResolveChangeDir(root, "slug"); got != legacy {
		t.Fatalf("legacy undated archive: got %q, want %q", got, legacy)
	}
	if err := os.RemoveAll(legacy); err != nil {
		t.Fatal(err)
	}
	if got := ResolveChangeDir(root, "missing-slug"); got != filepath.Join(root, "openspec", "changes", "missing-slug") {
		t.Fatalf("fallback: got %q, want an active-shaped path", got)
	}
}

// TestActiveChangeSlugs pins the active-folder enumeration: directories only,
// archive excluded, name-sorted.
func TestActiveChangeSlugs(t *testing.T) {
	root := t.TempDir()
	mkChange(t, root, "zeta")
	mkChange(t, root, "alpha")
	mkChange(t, root, "archive", "2026-01-01-old")
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "changes", "loose-file.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ActiveChangeSlugs(root)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("active slugs = %v, want [alpha zeta]", got)
	}
	if got := ActiveChangeSlugs(t.TempDir()); len(got) != 0 {
		t.Fatalf("missing openspec/ must yield no slugs, got %v", got)
	}
}

// TestResolveChangeDirMatchesPythonHelper asserts the Go resolution order equals
// tests/_change_paths.py::change_dir (read-only reference) for the same fixture
// trees. It is skipped when python3 or the helper is unavailable, so the Go suite
// stays runnable on its own.
func TestResolveChangeDirMatchesPythonHelper(t *testing.T) {
	// The test process runs in the package dir (gate/ledger), so the repo root
	// is five levels up.
	helper := filepath.Join("..", "..", "..", "..", "..", "tests", "_change_paths.py")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	if _, err := os.Stat(helper); err != nil {
		t.Skipf("change-path helper not found: %v", err)
	}
	helper, err = filepath.Abs(helper)
	if err != nil {
		t.Fatal(err)
	}

	trees := []struct {
		name  string
		dirs  []string
		slugs []string
	}{
		{"active wins", []string{"changes/slug", "changes/archive/2026-03-03-slug"}, []string{"slug", "absent"}},
		{"latest dated wins", []string{"changes/archive/2026-01-01-slug", "changes/archive/2026-03-03-slug"}, []string{"slug"}},
		{"malformed date ignored", []string{"changes/archive/2026-99-99-slug", "changes/archive/2026-01-01-slug"}, []string{"slug"}},
		{"legacy undated", []string{"changes/archive/slug"}, []string{"slug"}},
		{"fallback", nil, []string{"absent"}},
		{"suffix not a slug match", []string{"changes/archive/2026-01-01-other"}, []string{"absent"}},
	}
	script := "import sys, pathlib\n" +
		"sys.path.insert(0, sys.argv[1])\n" +
		"import _change_paths\n" +
		"print(_change_paths.change_dir(pathlib.Path(sys.argv[2]), sys.argv[3]))\n"

	for _, tc := range trees {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, rel := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(root, "openspec", filepath.FromSlash(rel)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, slug := range tc.slugs {
				want := ResolveChangeDir(root, slug)
				out, err := exec.Command(python, "-c", script, filepath.Dir(helper), root, slug).Output()
				if err != nil {
					t.Fatalf("python helper: %v", err)
				}
				if got := strings.TrimSpace(string(out)); got != want {
					t.Fatalf("slug %q: go=%q python=%q", slug, want, got)
				}
			}
		})
	}
}
