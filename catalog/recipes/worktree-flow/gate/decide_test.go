package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitFixture builds a real git repository in a temp dir and returns its root.
// The repo is left on a branch named by wantBranch.
func gitFixture(t *testing.T, wantBranch string) string {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-qm", "fixture")
	git("checkout", "-q", "-B", wantBranch)
	return root
}

func TestDecideProtectedMainBlocks(t *testing.T) {
	root := gitFixture(t, "main")
	d := Decide(filepath.Join(root, "src.py"), root, "auto", "auto", []string{"main", "development"})
	if d.Allow {
		t.Fatal("Decide on protected main = allow, want block")
	}
	if d.Branch != "main" {
		t.Fatalf("Decide block branch = %q, want main", d.Branch)
	}
}

func TestDecideFeatureBranchAllows(t *testing.T) {
	root := gitFixture(t, "feature-x")
	if d := Decide(filepath.Join(root, "src.py"), root, "auto", "auto", []string{"main", "development"}); !d.Allow {
		t.Fatalf("Decide on feature branch = block(%q), want allow", d.Branch)
	}
}

func TestDecideDevelopmentBlocks(t *testing.T) {
	root := gitFixture(t, "development")
	d := Decide(filepath.Join(root, "a.txt"), root, "auto", "auto", []string{"main", "development"})
	if d.Allow {
		t.Fatal("Decide on protected development = allow, want block")
	}
	if d.Branch != "development" {
		t.Fatalf("Decide block branch = %q, want development", d.Branch)
	}
}

func TestDecideOutsideRepoAllows(t *testing.T) {
	root := gitFixture(t, "main")
	external := t.TempDir()
	if d := Decide(filepath.Join(external, "out.txt"), root, "auto", "auto", []string{"main"}); !d.Allow {
		t.Fatalf("Decide outside any repo = block(%q), want allow", d.Branch)
	}
}

func TestDecideLinkedWorktreeAllows(t *testing.T) {
	root := gitFixture(t, "main")
	wt := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "-C", root, "worktree", "add", "-q", "-b", "feat", wt)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	if d := Decide(filepath.Join(wt, "x.py"), wt, "auto", "auto", []string{"main", "development"}); !d.Allow {
		t.Fatalf("Decide inside linked worktree = block(%q), want allow", d.Branch)
	}
}

func TestDecideNonexistentTargetInsideRepoBlocks(t *testing.T) {
	// A write target that does not exist yet is still a repo write and MUST
	// block on a protected branch (corpus correction 1.20: a nonexistent path
	// must not fail open). Legacy's existing_ancestor walks up to the repo
	// root, which exists.
	root := gitFixture(t, "main")
	ghost := filepath.Join(root, "does-not-exist", "deep", "x.py")
	d := Decide(ghost, root, "auto", "auto", []string{"main"})
	if d.Allow {
		t.Fatalf("Decide on nonexistent target inside protected repo = allow, want block")
	}
	if d.Branch != "main" {
		t.Fatalf("Decide block branch = %q, want main", d.Branch)
	}
}

func TestIsClaudeException(t *testing.T) {
	cases := []struct {
		raw  string
		abs  string
		want bool
	}{
		{".claude/settings.json", "/repo/.claude/settings.json", true},
		{"/repo/.claude/settings.local.json", "/repo/.claude/settings.local.json", true},
		{"x", "/repo/.claude/hooks/pre-commit", true},
		{"src.py", "/repo/src.py", false},
		{"settings.json", "/repo/settings.json", false},
		{".claude/settings.json", "/other/.claude/settings.json", true},
		{"src.py", "/repo/.claude/notes.txt", false},
	}
	for _, tc := range cases {
		if got := IsClaudeException(tc.raw, tc.abs); got != tc.want {
			t.Fatalf("IsClaudeException(%q, %q) = %v, want %v", tc.raw, tc.abs, got, tc.want)
		}
	}
}

func TestBlockMessageVerbatim(t *testing.T) {
	// Byte-identical to the frozen reference (worktree-gate-legacy.sh:527-529),
	// including the U+2014 em dash and the /worktree-new guidance.
	want := "worktree-gate: refusing to Write '/repo/src.py' on protected branch 'main' in the main worktree. Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write."
	if got := BlockMessage(false, "Write", "/repo/src.py", "main"); got != want {
		t.Fatalf("path block message:\n got %q\nwant %q", got, want)
	}
	wantShell := "worktree-gate: refusing shell command that writes '/repo/out.log' on protected branch 'main' in the main worktree — using bash/shell to write here bypasses the worktree gate. Create a dedicated worktree first (e.g. /worktree-new) and run there — exploration ends at the first write."
	if got := BlockMessage(true, "Bash", "/repo/out.log", "main"); got != wantShell {
		t.Fatalf("shell block message:\n got %q\nwant %q", got, wantShell)
	}
}

func TestBlockMessageDefaultTool(t *testing.T) {
	got := BlockMessage(false, "", "/repo/x", "main")
	if !strings.HasPrefix(got, "worktree-gate: refusing to edit '/repo/x' on protected branch 'main'") {
		t.Fatalf("default-tool message = %q, want 'edit' fallback", got)
	}
}
