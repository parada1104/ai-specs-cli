package planbuild

// The local no-plan declaration (plan-build-openspec-odd-align S19): a
// per-branch git config entry that never becomes an artifact.

import (
	"testing"
)

// ceremonyRepo is a repository on a real change branch.
func ceremonyRepo(t *testing.T) (root, branch string) {
	t.Helper()
	root, _ = commitRepo(t)
	branch = "feat/thing"
	gitRun(t, root, "checkout", "-b", branch)
	return root, branch
}

func TestCeremonyNoneIsAbsentByDefault(t *testing.T) {
	root, branch := ceremonyRepo(t)
	if CeremonyNone(root, branch) {
		t.Fatalf("CeremonyNone(%q) = true before any declaration", branch)
	}
	if CeremonyNone(root, "") {
		t.Fatal("CeremonyNone with an empty branch = true")
	}
}

func TestDeclareCeremonyNoneThenReadsTrue(t *testing.T) {
	root, branch := ceremonyRepo(t)
	if err := DeclareCeremonyNone(root, branch); err != nil {
		t.Fatalf("DeclareCeremonyNone: %v", err)
	}
	if !CeremonyNone(root, branch) {
		t.Fatalf("CeremonyNone(%q) = false after the declaration", branch)
	}
}

// The declaration is per branch: declaring one branch leaves every other branch
// without a declaration.
func TestCeremonyNoneIsPerBranch(t *testing.T) {
	root, branch := ceremonyRepo(t)
	if err := DeclareCeremonyNone(root, branch); err != nil {
		t.Fatalf("DeclareCeremonyNone: %v", err)
	}
	if CeremonyNone(root, "feat/other") {
		t.Fatal("CeremonyNone(feat/other) = true; the declaration is not per branch")
	}
}

// Any stored value other than none is not a declaration.
func TestCeremonyNoneIgnoresAnotherStoredValue(t *testing.T) {
	root, branch := ceremonyRepo(t)
	gitRun(t, root, "config", "--local", ceremonyConfigKey(branch), "plan")
	if CeremonyNone(root, branch) {
		t.Fatal("CeremonyNone = true for a stored value other than none")
	}
}

// Outside a repository there is no declaration to read and none to write.
func TestCeremonyNoneOutsideARepository(t *testing.T) {
	root := t.TempDir()
	if CeremonyNone(root, "feat/thing") {
		t.Fatal("CeremonyNone outside a repository = true")
	}
	if err := DeclareCeremonyNone(root, "feat/thing"); err == nil {
		t.Fatal("DeclareCeremonyNone outside a repository = nil error")
	}
}

// An empty branch can neither be read nor written: the guard names it instead of
// letting git resolve a truncated key.
func TestCeremonyNoneRejectsAnEmptyBranch(t *testing.T) {
	root, _ := ceremonyRepo(t)
	if CeremonyNone(root, "") {
		t.Fatal("CeremonyNone with an empty branch = true")
	}
	if err := DeclareCeremonyNone(root, ""); err == nil {
		t.Fatal("DeclareCeremonyNone with an empty branch = nil error")
	}
}
