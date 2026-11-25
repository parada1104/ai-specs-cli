package planbuild

// The commit ancestry helper (plan-build-openspec-odd-align P3): a plan's Commit
// cell is graded by asking git for the exit code, never by parsing its output.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitRepo is a repository with two commits; it returns the root and the first
// commit's sha, so "HEAD" and "an ancestor" both have a fixture.
func commitRepo(t *testing.T) (root, first string) {
	t.Helper()
	root = t.TempDir()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "test")
	gitRun(t, root, "commit", "--allow-empty", "-q", "-m", "first")
	first = strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	gitRun(t, root, "commit", "--allow-empty", "-q", "-m", "second")
	return root, first
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(output)
}

func TestCommitIsAncestor(t *testing.T) {
	root, first := commitRepo(t)
	head := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))

	for _, tc := range []struct {
		name   string
		commit string
		want   bool
	}{
		{"head", head, true},
		{"first commit", first, true},
		{"unknown sha", "0123456789abcdef0123456789abcdef01234567", false},
		{"path traversal", "../../etc/passwd", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CommitIsAncestor(root, tc.commit); got != tc.want {
				t.Errorf("CommitIsAncestor(%q) = %v, want %v", tc.commit, got, tc.want)
			}
		})
	}
}

// A root that is not inside any repository has no ancestry to report.
func TestCommitIsAncestorOutsideARepository(t *testing.T) {
	if CommitIsAncestor(t.TempDir(), "0123456") {
		t.Fatal("a non-repository root must answer false")
	}
}

// changedRepo is a repository whose second commit modifies one file; it returns
// the root and the first commit as the diff base.
func changedRepo(t *testing.T) (root, base string) {
	t.Helper()
	root = t.TempDir()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "kept.txt")
	gitRun(t, root, "commit", "-q", "-m", "first")
	base = strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "changed.txt")
	gitRun(t, root, "commit", "-q", "-m", "second")
	return root, base
}

func TestChangedFilesListsTheChangedPath(t *testing.T) {
	root, base := changedRepo(t)
	files, ok := ChangedFiles(root, base)
	if !ok {
		t.Fatal("ChangedFiles over a real diff = false, want true")
	}
	if len(files) != 1 || files[0] != "changed.txt" {
		t.Fatalf("ChangedFiles = %q, want [changed.txt]", files)
	}
}

// No diff means no answer: an unknown base, an option-shaped ref and a root
// outside any repository are all false, never a partial list.
func TestChangedFilesReportsNoDiff(t *testing.T) {
	root, _ := changedRepo(t)
	for _, tc := range []struct{ name, root, base string }{
		{"unknown base", root, "no-such-base"},
		{"option shaped base", root, "--all"},
		{"empty base", root, ""},
		{"outside a repository", t.TempDir(), "HEAD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if files, ok := ChangedFiles(tc.root, tc.base); ok {
				t.Fatalf("ChangedFiles = (%q, true), want false", files)
			}
		})
	}
}

// trailerRepo is a two-commit repository whose second commit message is the
// given body; base is the first commit, so base..HEAD is exactly that body.
func trailerRepo(t *testing.T, body string) (root, base string) {
	t.Helper()
	root, base = commitRepo(t)
	// commitRepo already made the second commit; rewrite the range with one more.
	gitRun(t, root, "commit", "--allow-empty", "-q", "-m", body)
	base = strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD~1"))
	return root, base
}

// The Risk trailer is the odd-mode answer to a high-risk change: only
// `Risk: item N (reason)` with N in 1..6 counts, so `Risk: none` never does.
func TestCommitsWithRiskTrailer(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"item with evidence", "feat: x\n\nRisk: item 2 (migrations rewrite the schema)\n", true},
		{"lowercase risk", "feat: x\n\nrisk: item 6 (installer paths)\n", true},
		{"bare none", "feat: x\n\nRisk: none\n", false},
		{"item out of range", "feat: x\n\nRisk: item 7 (no such item)\n", false},
		{"no trailer", "feat: x\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, base := trailerRepo(t, tc.body)
			if got := CommitsWithRiskTrailer(root, base); got != tc.want {
				t.Fatalf("CommitsWithRiskTrailer = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCommitsWithRiskTrailerOutsideARepository(t *testing.T) {
	if CommitsWithRiskTrailer(t.TempDir(), "HEAD") {
		t.Fatal("a non-repository root must answer false")
	}
}
