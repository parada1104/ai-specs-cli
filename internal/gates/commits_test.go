package gates

// The commit recorded per task (plan-build-openspec-odd-align P3): every Commit
// cell that is not "pendiente" must name a commit that is an ancestor of HEAD on
// the branch under review.

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitsRepo is a repository with two commits, so the ancestor rule has a real
// history to grade; it returns the root and the first commit's sha.
func commitsRepo(t *testing.T) (root, first string) {
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
	if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).
		CombinedOutput(); err != nil {
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

// planWithCommit is a light-tier plan whose only task records the given Commit
// cell, so a test about the commit rule carries no other rule.
func planWithCommit(commit string) string {
	return "Depth: light\n\n## Specs\n\n- **S1.** Fixture requirement\n\n## Tasks\n\n" +
		"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1 | Fixture task | inline | " + commit + " |\n"
}

// rawArchive writes an archived change with exactly the given tasks text.
// rawArchiveAt writes one exact archive folder name, so a test can choose a
// dated archive (graded as a plan) or the undated legacy fallback.
func rawArchiveAt(t *testing.T, root, archiveName, tasks string) {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", "archive", archiveName)
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), tasks)
}

func rawArchive(t *testing.T, root, slug, tasks string) {
	t.Helper()
	rawArchiveAt(t, root, slug, tasks)
}

func TestCommitCellThatIsAnAncestorDoesNotBlock(t *testing.T) {
	root, first := commitsRepo(t)
	rawArchive(t, root, "ancestor-sha", planWithCommit(first))
	if result := CheckPremerge(root, "ancestor-sha", tierOpt(TierLight)); !result.OK {
		t.Fatalf("ancestor sha = %+v, want OK", result)
	}
}

func TestCommitCellThatIsNotAnAncestorBlocksByNameAndValue(t *testing.T) {
	root, _ := commitsRepo(t)
	sha := "0123456789abcdef0123456789abcdef01234567"
	rawArchive(t, root, "unknown-sha", planWithCommit(sha))

	result := CheckPremerge(root, "unknown-sha", tierOpt(TierLight))
	if result.OK {
		t.Fatal("an unknown sha must block")
	}
	assertBlockerContains(t, result, "archive/unknown-sha/tasks.md: task T1 cites commit "+sha+
		" which is not an ancestor of HEAD on this branch: commit the task or write \"pendiente\"")
}

func TestPendingCommitCellsDoNotBlock(t *testing.T) {
	root, _ := commitsRepo(t)
	for name, value := range map[string]string{
		"empty":      "",
		"pendiente":  "pendiente",
		"pending":    "pending",
		"emdash":     "—",
		"hyphen":     "-",
		"backticked": "`pendiente`",
	} {
		t.Run(name, func(t *testing.T) {
			slug := "pending-" + name
			rawArchive(t, root, slug, planWithCommit(value))
			if result := CheckPremerge(root, slug, tierOpt(TierLight)); !result.OK {
				t.Fatalf("pending cell %q = %+v, want OK", value, result)
			}
		})
	}
}

// A tasks.md without task rows is graded as a plan, never for commits. The
// fixture is a dated archive: a legacy undated archive is not read as a plan
// (F5).
func TestTasksWithoutPlanRowsIsNotGradedForCommits(t *testing.T) {
	root, _ := commitsRepo(t)
	rawArchiveAt(t, root, "2026-01-02-legacy-no-rows", "Depth: light\n- [x] una tarea suelta\n")

	result := CheckPremerge(root, "legacy-no-rows", tierOpt(TierLight))
	if result.OK {
		t.Fatal("a free-form tasks.md still blocks as a plan")
	}
	assertBlockerLacks(t, result, "not an ancestor")
}
