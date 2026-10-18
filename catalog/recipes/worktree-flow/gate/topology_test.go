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

// TestCentralFromCommonMarkerParsing pins the absorbed-layout parser against
// the reference (plan-build-gate.sh:238-253): only an EARLIER /.git/modules/
// marker is nested, so a valid superproject whose own path contains a "modules"
// component still resolves.
func TestCentralFromCommonMarkerParsing(t *testing.T) {
	t.Run("modules component in superproject path resolves", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "modules", "super")
		got, ok := centralFromCommon(filepath.Join(root, ".git", "modules", "apps", "api"))
		if !ok || got != RealPath(root) {
			t.Fatalf("got %q, %v; want %q, true", got, ok, RealPath(root))
		}
	})
	t.Run("modules-prefixed component in superproject path resolves", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "modules-parent")
		got, ok := centralFromCommon(filepath.Join(root, ".git", "modules", "apps", "api"))
		if !ok || got != RealPath(root) {
			t.Fatalf("got %q, %v; want %q, true", got, ok, RealPath(root))
		}
	})
	t.Run("nested .git/modules fails", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "super")
		nested := filepath.Join(root, ".git", "modules", "inner", ".git", "modules", "apps", "api")
		if got, ok := centralFromCommon(nested); ok {
			t.Fatalf("nested layout accepted: %q", got)
		}
	})
	t.Run("no modules marker fails", func(t *testing.T) {
		if got, ok := centralFromCommon(filepath.Join(t.TempDir(), "super", ".git")); ok {
			t.Fatalf("non-absorbed layout accepted: %q", got)
		}
	})
	t.Run("marker not under .git fails", func(t *testing.T) {
		if got, ok := centralFromCommon(filepath.Join(t.TempDir(), "super", "modules", "apps", "api")); ok {
			t.Fatalf("non-.git marker accepted: %q", got)
		}
	})
}

// TestLegacyCentralFailClosed pins the bounded non-absorbed fallback
// (plan-build-gate.sh:254-286): git's superproject fact is corroborated by the
// superproject's .gitmodules and a live (non-empty, non-"-") submodule status,
// and every missing fact fails closed. The git fact is injected so the proof is
// exercised without a second real-repository fixture.
func TestLegacyCentralFailClosed(t *testing.T) {
	newSuper := func(t *testing.T) (string, string) {
		t.Helper()
		super := t.TempDir()
		if err := os.MkdirAll(filepath.Join(super, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(super, ".gitmodules"),
			[]byte("[submodule \"api\"]\n\tpath = apps/api\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(super, "apps", "api")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: ../../.git/modules/apps/api\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return super, sub
	}
	factFor := func(super, status string) func(string, ...string) string {
		return func(_ string, args ...string) string {
			switch args[0] {
			case "rev-parse":
				return super
			case "submodule":
				return status
			}
			return ""
		}
	}

	t.Run("proven legacy submodule resolves", func(t *testing.T) {
		super, sub := newSuper(t)
		gotRoot, gotSub, ok := legacyCentral(RealPath(sub), factFor(super, "abc1234 apps/api (heads/main)"))
		if !ok || gotRoot != RealPath(super) || gotSub != "apps/api" {
			t.Fatalf("got %q, %q, %v; want %q, %q, true", gotRoot, gotSub, ok, RealPath(super), "apps/api")
		}
	})
	t.Run("empty status fails closed", func(t *testing.T) {
		super, sub := newSuper(t)
		if _, _, ok := legacyCentral(RealPath(sub), factFor(super, "")); ok {
			t.Fatal("empty submodule status proved a central root")
		}
	})
	t.Run("deinitialized status fails closed", func(t *testing.T) {
		super, sub := newSuper(t)
		if _, _, ok := legacyCentral(RealPath(sub), factFor(super, "-abc1234 apps/api")); ok {
			t.Fatal("deinitialized submodule proved a central root")
		}
	})
	t.Run("missing .gitmodules fails closed", func(t *testing.T) {
		super, sub := newSuper(t)
		if err := os.Remove(filepath.Join(super, ".gitmodules")); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := legacyCentral(RealPath(sub), factFor(super, "abc1234 apps/api")); ok {
			t.Fatal("superproject without .gitmodules proved a central root")
		}
	})
	t.Run("root not under superproject fails closed", func(t *testing.T) {
		super, _ := newSuper(t)
		if _, _, ok := legacyCentral(RealPath(t.TempDir()), factFor(super, "abc1234 apps/api")); ok {
			t.Fatal("unrelated root proved a central root")
		}
	})
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
