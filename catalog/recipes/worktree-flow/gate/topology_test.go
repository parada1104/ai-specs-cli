package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClassifyStandaloneUnproven(t *testing.T) {
	root := t.TempDir()
	if got := classify(root, root, "standalone"); got != ownerUnproven {
		t.Fatal(got)
	}
}

func TestModuleRecordsMissing(t *testing.T) {
	if got := moduleRecords(t.TempDir()); got != nil {
		t.Fatal(got)
	}
}

func gitTest(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// makeRemoteModule creates a bare remote plus a throwaway working clone so
// `submodule add file://<remote>` materializes the .git/modules/<rel> layout
// the reference module_records proves against (worktree-gate-legacy.sh:443).
// A local-path add does NOT — it leaves a gitfile pointing at the source, so
// common != expected and the record is unproven, exactly as the reference
// behaves.
func makeRemoteModule(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	gitTest(t, src, "init", "-q")
	gitTest(t, src, "config", "user.email", "t@t.t")
	gitTest(t, src, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("module\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, src, "add", "-A")
	gitTest(t, src, "commit", "-qm", "init")
	gitTest(t, src, "checkout", "-q", "-B", "main")
	remote := t.TempDir() + "/remote.git"
	if out, err := exec.Command("git", "clone", "--bare", "-q", src, remote).CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %v: %s", err, out)
	}
	return "file://" + remote
}

func makeSuper(t *testing.T, remote string) string {
	t.Helper()
	super := t.TempDir()
	gitTest(t, super, "init", "-q")
	gitTest(t, super, "config", "user.email", "t@t.t")
	gitTest(t, super, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(super, "ROOT"), []byte("super\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, super, "add", "-A")
	gitTest(t, super, "commit", "-qm", "root")
	cmd := exec.Command("git", "-C", super, "-c", "protocol.file.allow=always",
		"submodule", "add", "-q", remote, "apps/api")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("submodule add: %v: %s", err, out)
	}
	gitTest(t, super, "commit", "-qam", "add module")
	gitTest(t, super, "checkout", "-q", "-B", "main")
	return super
}

// TestModuleRecordsProvenInitializedSubmodule pins the full proof the
// reference module_records requires (worktree-gate-legacy.sh:412-449): a real
// .git, an initialized submodule whose status is not "-", a git-common-dir
// equal to .git/modules/<rel>, and an owner equal to the module itself. The
// pre-parity code returned paths without any proof — that was the differential
// the Go hook parameterization surfaced (task 2.17).
func TestModuleRecordsProvenInitializedSubmodule(t *testing.T) {
	remote := makeRemoteModule(t)
	super := makeSuper(t, remote)

	records := moduleRecords(super)
	if len(records) != 1 {
		t.Fatalf("expected 1 proven record, got %d (%+v)", len(records), records)
	}
	got := RealPath(records[0].module)
	want := RealPath(filepath.Join(super, "apps", "api"))
	if got != want {
		t.Fatalf("module = %q, want %q", got, want)
	}
	// classify: the subrepo itself is classified subrepo by its superrepo.
	if owner := classify(filepath.Join(super, "apps", "api"), records[0].common, "monorepo-submodules"); owner != ownerSub {
		t.Fatalf("subrepo owner = %q, want %q", owner, ownerSub)
	}
	if owner := classify(super, gitCommon(super), "monorepo-submodules"); owner != ownerSuper {
		t.Fatalf("superrepo owner = %q, want %q", owner, ownerSuper)
	}
}

// TestModuleRecordsFakeGitmodulesYieldsNone pins the ambiguity contract: a
// .gitmodules file with no backing repository proves nothing (the reference
// returns None), so the gate fails open rather than trusting a stub.
func TestModuleRecordsFakeGitmodulesYieldsNone(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitmodules"),
		[]byte("[submodule \"api\"]\n\tpath = apps/api\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := moduleRecords(root); got != nil {
		t.Fatalf("fake .gitmodules proved records: %+v", got)
	}
}

// TestModuleRecordsAmbiguousNestedYieldsNone pins the nested/duplicate
// ambiguity: overlapping registrations make the whole set unproven
// (worktree-gate-legacy.sh:431-434), so a superrepo with duplicate or nested
// submodule paths can never receive the openspec/changes central exception.
func TestModuleRecordsAmbiguousNestedYieldsNone(t *testing.T) {
	remote := makeRemoteModule(t)
	super := makeSuper(t, remote)

	gm := filepath.Join(super, ".gitmodules")
	f, err := os.OpenFile(gm, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[submodule.duplicate]\n\tpath = apps/api\n[submodule.nested]\n\tpath = apps/api/nested\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := moduleRecords(super); got != nil {
		t.Fatalf("ambiguous registrations proved records: %+v", got)
	}
}
