package planbuild

// The commit ancestry helper (plan-build-openspec-odd-align P3): a plan's Commit
// cell is graded by asking git for the exit code, never by parsing its output.

import (
	"os/exec"
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
