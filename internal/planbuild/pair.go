package planbuild

// Branch/plan pairing (plan-build-odd-companion S3): in odd mode the plan of a
// change is odd/tasks/<change>.md, where <change> is the last segment of the
// current branch name.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	return TasksPath(root, ModeOdd, change)
}

// TasksPath is the plan path of the mode: in odd mode the plan belongs to the
// feature (odd/tasks/<change>.md) and in openspec to the change folder.
func TasksPath(root, mode, change string) string {
	if mode == ModeOdd {
		return filepath.Join(root, "odd", "tasks", change+".md")
	}
	return filepath.Join(root, "openspec", "changes", change, "tasks.md")
}

// ChangeFolder is the change directory of the mode.
func ChangeFolder(root, mode, change string) string {
	return filepath.Join(ChangesBase(root, mode), change)
}

// ConfigPath is the project configuration of the mode.
func ConfigPath(root, mode string) string {
	if mode == ModeOdd {
		return filepath.Join(root, "odd", "config.yaml")
	}
	return filepath.Join(root, "openspec", "config.yaml")
}

// ArchiveBase is the archived-change tree of the mode.
func ArchiveBase(root, mode string) string {
	return filepath.Join(ChangesBase(root, mode), "archive")
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

// ErrNoChange and ErrAmbiguousChange classify the two ladder refusals.
var (
	ErrNoChange        = errors.New("no active change")
	ErrAmbiguousChange = errors.New("ambiguous change")
)

// ActiveChanges lists the change directories of the mode, sorted, excluding
// archive. The tree is read literally, never as a glob; non-directories and
// symlinks are skipped, and any read error yields no candidates.
func ActiveChanges(root, mode string) []string {
	entries, err := os.ReadDir(ChangesBase(root, mode))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == "archive" || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// ValidChangeName reports whether change is usable as one path segment. Every
// caller that turns a change name into a path uses this predicate, so an
// override like "../../etc" can never become a directory outside the tree.
func ValidChangeName(change string) bool {
	if change == "" || change == "." || change == ".." {
		return false
	}
	return !strings.ContainsAny(change, "/\\\x00")
}

// NormalizeChange normalizes a change name that arrived from the CLI: a
// branch-shaped value keeps its last path segment through ChangeFromBranch, the
// same helper the branch pairing uses, so CI can pass GITHUB_HEAD_REF (e.g.
// "feat/alpha") directly. A value with an empty, "." or ".." segment is returned
// unchanged, so the caller's ValidChangeName refusal still names it instead of
// silently reinterpreting a traversal attempt as a different change.
func NormalizeChange(name string) string {
	if !strings.Contains(name, "/") {
		return name
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return name
		}
	}
	if normalized, ok := ChangeFromBranch(name); ok {
		return normalized
	}
	return name
}

// ResolveChange is the one change ladder: an explicit override wins, then a
// branch that pairs an existing change folder or its surviving plan, then the
// only active change; anything else refuses instead of guessing which change is
// being built. A branch-shaped override is normalized through NormalizeChange.
func ResolveChange(root, mode, branch, override string) (string, error) {
	if override != "" {
		return NormalizeChange(override), nil
	}
	if name, ok := ChangeFromBranch(branch); ok {
		if info, err := os.Stat(ChangeFolder(root, mode, name)); err == nil && info.IsDir() {
			return name, nil
		}
		// An archived odd change keeps its plan at odd/tasks/<name>.md, so the
		// branch still pairs after archiving; in openspec the plan moved into
		// the archive with the folder and the slug (or --change) is required.
		if PlanIsValidForMode(root, mode, name) {
			return name, nil
		}
	}
	active := ActiveChanges(root, mode)
	switch len(active) {
	case 1:
		return active[0], nil
	case 0:
		return "", fmt.Errorf("%w under %s and the branch %q does not name one; write %s and retry",
			ErrNoChange, ChangesBase(root, mode), branch, TasksPath(root, mode, "<change>"))
	}
	return "", fmt.Errorf("%w under %s: %d active changes (%s); pair the branch with one of them or pass --change",
		ErrAmbiguousChange, ChangesBase(root, mode), len(active), strings.Join(active, ", "))
}

// PlanIsValid is the single paired-plan predicate every gate uses: the plan is a
// regular file and its real path stays inside the repository, so a symlinked
// odd/tasks cannot point the rule at a file elsewhere. PlanIsValid is the odd
// alias of PlanIsValidForMode.
func PlanIsValid(repoRoot, change string) bool {
	return PlanIsValidForMode(repoRoot, ModeOdd, change)
}

// PlanIsValidForMode is the one plan predicate, over the mode's plan path.
func PlanIsValidForMode(repoRoot, mode, change string) bool {
	plan := TasksPath(repoRoot, mode, change)
	info, err := os.Lstat(plan)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	real, err := RealPath(plan)
	if err != nil {
		return false
	}
	root, err := RealPath(repoRoot)
	if err != nil {
		return false
	}
	return real == root || strings.HasPrefix(real, root+string(filepath.Separator))
}

// RealPath resolves one path to its absolute real form. Every containment
// predicate compares through this helper, so a relative root (`--root .`) grades
// exactly like the absolute one.
func RealPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
