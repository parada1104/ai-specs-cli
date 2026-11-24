package planbuild

// Branch/plan pairing (plan-build-odd-companion S3): in odd mode the plan of a
// change is odd/tasks/<change>.md, where <change> is the last segment of the
// current branch name.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Branch is the current branch name, born or unborn; "" when HEAD is detached.
// symbolic-ref answers an unborn branch, where rev-parse --abbrev-ref fails.
func Branch(root string) string {
	out, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ChangeFromBranch returns the change name: the last path segment of the branch
// with every prefix stripped, so feat/x, fix/x and user/feat/x all give x. A
// detached HEAD, an empty branch or an empty/dot last segment resolves nothing.
func ChangeFromBranch(branch string) (string, bool) {
	if branch == "" || branch == "HEAD" {
		return "", false
	}
	change := branch
	if i := strings.LastIndex(branch, "/"); i >= 0 {
		change = branch[i+1:]
	}
	if change == "" || change == "." {
		return "", false
	}
	return change, true
}

// PlanPath is the ODD plan path of one change.
func PlanPath(root, change string) string {
	return filepath.Join(root, "odd", "tasks", change+".md")
}

// ChangesBase is the change tree of the mode; odd is the only odd tree.
func ChangesBase(root, mode string) string {
	if mode == ModeOdd {
		return filepath.Join(root, "odd", "changes")
	}
	return filepath.Join(root, "openspec", "changes")
}

// SpecsBase is the canonical spec tree of the mode.
func SpecsBase(root, mode string) string {
	if mode == ModeOdd {
		return filepath.Join(root, "odd", "specs")
	}
	return filepath.Join(root, "openspec", "specs")
}

// PlanIsValid is the single paired-plan predicate every gate uses: the plan is a
// regular file and its real path stays inside the repository, so a symlinked
// odd/tasks cannot point the rule at a file elsewhere.
func PlanIsValid(repoRoot, change string) bool {
	plan := PlanPath(repoRoot, change)
	info, err := os.Lstat(plan)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	real, err := filepath.EvalSymlinks(plan)
	if err != nil {
		return false
	}
	root, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		root = repoRoot
	}
	return real == root || strings.HasPrefix(real, root+string(filepath.Separator))
}
