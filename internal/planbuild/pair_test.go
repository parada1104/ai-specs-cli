package planbuild

// Branch/plan pairing (plan-build-odd-companion S3): the change name is the last
// path segment of the branch and its plan lives at odd/tasks/<change>.md.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ChangeFromBranch strips any prefix segment; unresolvable names refuse.
func TestChangeFromBranch(t *testing.T) {
	for _, tc := range []struct {
		branch string
		change string
		ok     bool
	}{
		{"feat/x", "x", true},
		{"fix/x", "x", true},
		{"x", "x", true},
		{"user/feat/x", "x", true},
		{"", "", false},
		{"HEAD", "", false},
		{"feat/", "", false},
		{".", "", false},
		{"feat/.", "", false},
	} {
		change, ok := ChangeFromBranch(tc.branch)
		if change != tc.change || ok != tc.ok {
			t.Errorf("ChangeFromBranch(%q) = (%q, %v), want (%q, %v)", tc.branch, change, ok, tc.change, tc.ok)
		}
	}
}

// The plan path is exactly odd/tasks/<change>.md at the repository root.
func TestPlanPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "odd", "tasks", "thing.md")
	if got := PlanPath(root, "thing"); got != want {
		t.Fatalf("PlanPath = %q, want %q", got, want)
	}
}

// Each mode selects its own planning tree; odd is the only odd tree.
func TestChangesAndSpecsBases(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		mode        string
		changesBase string
		specsBase   string
	}{
		{ModeOdd, filepath.Join(root, "odd", "changes"), filepath.Join(root, "odd", "specs")},
		{ModeOpenspec, filepath.Join(root, "openspec", "changes"), filepath.Join(root, "openspec", "specs")},
		{ModeAuto, filepath.Join(root, "openspec", "changes"), filepath.Join(root, "openspec", "specs")},
	} {
		if got := ChangesBase(root, tc.mode); got != tc.changesBase {
			t.Errorf("ChangesBase(%q) = %q, want %q", tc.mode, got, tc.changesBase)
		}
		if got := SpecsBase(root, tc.mode); got != tc.specsBase {
			t.Errorf("SpecsBase(%q) = %q, want %q", tc.mode, got, tc.specsBase)
		}
	}
}

// TasksPath is the plan path of the mode: the plan belongs to the feature in
// odd mode and to the change folder in openspec. PlanPath stays the odd alias.
func TestTasksPathPerMode(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ mode, want string }{
		{ModeOdd, filepath.Join(root, "odd", "tasks", "thing.md")},
		{ModeOpenspec, filepath.Join(root, "openspec", "changes", "thing", "tasks.md")},
		{ModeAuto, filepath.Join(root, "openspec", "changes", "thing", "tasks.md")},
	} {
		if got := TasksPath(root, tc.mode, "thing"); got != tc.want {
			t.Errorf("TasksPath(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
	if got := PlanPath(root, "thing"); got != TasksPath(root, ModeOdd, "thing") {
		t.Errorf("PlanPath = %q, want the odd TasksPath", got)
	}
}

// ChangeFolder, ConfigPath and ArchiveBase follow the mode's own tree.
func TestModePaths(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ mode, folder, config, archive string }{
		{ModeOdd,
			filepath.Join(root, "odd", "changes", "thing"),
			filepath.Join(root, "odd", "config.yaml"),
			filepath.Join(root, "odd", "changes", "archive")},
		{ModeOpenspec,
			filepath.Join(root, "openspec", "changes", "thing"),
			filepath.Join(root, "openspec", "config.yaml"),
			filepath.Join(root, "openspec", "changes", "archive")},
	} {
		if got := ChangeFolder(root, tc.mode, "thing"); got != tc.folder {
			t.Errorf("ChangeFolder(%q) = %q, want %q", tc.mode, got, tc.folder)
		}
		if got := ConfigPath(root, tc.mode); got != tc.config {
			t.Errorf("ConfigPath(%q) = %q, want %q", tc.mode, got, tc.config)
		}
		if got := ArchiveBase(root, tc.mode); got != tc.archive {
			t.Errorf("ArchiveBase(%q) = %q, want %q", tc.mode, got, tc.archive)
		}
	}
}

// ActiveChanges lists change directories only: archive, plain files and
// symlinks are not active changes, and the result is sorted.
func TestActiveChanges(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "openspec", "changes")
	for _, name := range []string{"b-change", "a-change", "archive"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "not-a-dir.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "a-change"), filepath.Join(base, "linked-change")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := ActiveChanges(root, ModeOpenspec)
	want := []string{"a-change", "b-change"}
	if len(got) != len(want) {
		t.Fatalf("ActiveChanges = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ActiveChanges = %q, want %q", got, want)
		}
	}
	if got := ActiveChanges(filepath.Join(root, "absent"), ModeOpenspec); got != nil {
		t.Errorf("ActiveChanges on a missing tree = %q, want nil", got)
	}
}

// Ladder step 1: an explicit override wins even when the branch pairs another
// existing change and other active changes exist.
func TestResolveChangeOverrideWins(t *testing.T) {
	root := t.TempDir()
	for _, mode := range []string{ModeOdd, ModeOpenspec} {
		if err := os.MkdirAll(ChangeFolder(root, mode, "paired"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveChange(root, mode, "feat/paired", "explicit")
		if err != nil || got != "explicit" {
			t.Errorf("ResolveChange(%q) = (%q, %v), want (explicit, nil)", mode, got, err)
		}
	}
}

// A branch-shaped override (CI passes GITHUB_HEAD_REF) is normalized to its
// last segment through the same helper the branch pairing uses.
func TestResolveChangeNormalizesABranchShapedOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(ChangeFolder(root, ModeOpenspec, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, override := range []string{"feat/alpha", "user/feat/alpha", "alpha"} {
		got, err := ResolveChange(root, ModeOpenspec, "", override)
		if err != nil || got != "alpha" {
			t.Errorf("ResolveChange(%q) = (%q, %v), want (alpha, nil)", override, got, err)
		}
	}
}

// Ladder step 2: a branch pairs its last segment when that change folder
// exists, in both modes.
func TestResolveChangePairsTheBranch(t *testing.T) {
	root := t.TempDir()
	for _, mode := range []string{ModeOdd, ModeOpenspec} {
		if err := os.MkdirAll(ChangeFolder(root, mode, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		// A second active change must not matter when the branch pairs one.
		if err := os.MkdirAll(ChangeFolder(root, mode, "y"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveChange(root, mode, "feat/x", "")
		if err != nil || got != "x" {
			t.Errorf("ResolveChange(%q, feat/x) = (%q, %v), want (x, nil)", mode, got, err)
		}
	}
}

// Ladder step 3: no pairing and exactly one active change resolves that one.
func TestResolveChangeSingleActive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(ChangeFolder(root, ModeOpenspec, "only-change"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveChange(root, ModeOpenspec, "main", "")
	if err != nil || got != "only-change" {
		t.Fatalf("ResolveChange = (%q, %v), want (only-change, nil)", got, err)
	}
}

// Ladder step 4a: nothing to pair and no active change is a refusal that names
// the plan path to write.
func TestResolveChangeNoActiveChange(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveChange(root, ModeOpenspec, "main", "")
	if !errors.Is(err, ErrNoChange) {
		t.Fatalf("error = %v, want ErrNoChange", err)
	}
	for _, want := range []string{
		"no active change under " + filepath.Join(root, "openspec", "changes"),
		`the branch "main" does not name one`,
		"write " + filepath.Join(root, "openspec", "changes", "<change>", "tasks.md") + " and retry",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// Ladder step 4b: two active changes with no branch pairing is ambiguous, names
// every candidate and points at --change.
func TestResolveChangeAmbiguous(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b-change", "a-change"} {
		if err := os.MkdirAll(ChangeFolder(root, ModeOpenspec, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, err := ResolveChange(root, ModeOpenspec, "main", "")
	if !errors.Is(err, ErrAmbiguousChange) {
		t.Fatalf("error = %v, want ErrAmbiguousChange", err)
	}
	for _, want := range []string{
		"ambiguous change under " + filepath.Join(root, "openspec", "changes"),
		"2 active changes (a-change, b-change)",
		"pass --change",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// The plan predicate is written once and selects the mode's plan path.
func TestPlanIsValidForMode(t *testing.T) {
	root := t.TempDir()
	oddPlan := TasksPath(root, ModeOdd, "thing")
	openspecPlan := TasksPath(root, ModeOpenspec, "thing")
	for _, path := range []string{oddPlan, openspecPlan} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# plan\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !PlanIsValidForMode(root, ModeOdd, "thing") || !PlanIsValid(root, "thing") {
		t.Error("odd plan is not valid")
	}
	if !PlanIsValidForMode(root, ModeOpenspec, "thing") {
		t.Error("openspec plan is not valid")
	}
	if PlanIsValidForMode(root, ModeOdd, "absent") {
		t.Error("an absent plan is valid")
	}
}

// After archiving, an odd change still pairs from the branch because its plan
// survives at odd/tasks/<change>.md while the delta folder is gone.
func TestResolveChangePairsAnArchivedOddChange(t *testing.T) {
	root := t.TempDir()
	plan := TasksPath(root, ModeOdd, "x")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ChangeFolder(root, ModeOdd, "x")); !os.IsNotExist(err) {
		t.Fatalf("delta folder exists: %v", err)
	}
	got, err := ResolveChange(root, ModeOdd, "feat/x", "")
	if err != nil || got != "x" {
		t.Errorf("ResolveChange(odd, feat/x) = (%q, %v), want (x, nil)", got, err)
	}
}

// In openspec the plan moved into the archive with the change folder, so the
// branch no longer pairs and the ladder refuses; a slug or --change is required.
func TestResolveChangeDoesNotPairAnArchivedOpenSpecChange(t *testing.T) {
	root := t.TempDir()
	archived := filepath.Join(ArchiveBase(root, ModeOpenspec), "2026-01-02-x", "tasks.md")
	if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archived, []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveChange(root, ModeOpenspec, "feat/x", "")
	if !errors.Is(err, ErrNoChange) {
		t.Fatalf("error = %v, want ErrNoChange", err)
	}
}
