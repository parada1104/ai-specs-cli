package planbuild

// Branch/plan pairing (plan-build-odd-companion S3): the change name is the last
// path segment of the branch and its plan lives at odd/tasks/<change>.md.

import (
	"path/filepath"
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
