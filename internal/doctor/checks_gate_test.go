package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gateVersionScript is a stand-in verified gate binary: it reports the given
// version and passes --selftest.
func gateVersionScript(version string) string {
	return "#!/bin/sh\ncase \"$1\" in\n" +
		"  --version) printf '" + version + "\\n'; exit 0 ;;\n" +
		"  --selftest) exit 0 ;;\n" +
		"esac\nexit 1\n"
}

func writeCachedGate(t *testing.T, d *Doctor, body string) string {
	t.Helper()
	goos, goarch := detectGatePlatform()
	binary := gateCacheBinPath(d.Home, goos, goarch)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(binary), err)
	}
	writeExecutableFile(t, binary, body)
	return binary
}

// testCacheKiB is an independent sum of the cache bytes, so the OK message's
// cache figure is checked against the files on disk, not the implementation.
func testCacheKiB(t *testing.T, home string) int {
	t.Helper()
	var total int64
	root := filepath.Join(home, "cache", "bin", "worktree-gate")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, serr := os.Stat(path); serr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return int(total / 1024)
}

func TestCheckWorktreeGateNotInPlay(t *testing.T) {
	t.Run("no-manifest", func(t *testing.T) {
		d := New(t.TempDir(), t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
	t.Run("disabled", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = false\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
	t.Run("enabled-not-true", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = 1\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
	t.Run("recipe-not-a-table", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes]\nworktree-flow = \"x\"\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
}

func TestCheckWorktreeGateVerifiedBinary(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "VERSION"), "0.24.0\n")
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, home)
	binary := writeCachedGate(t, d, gateVersionScript("0.24.0"))
	writeFile(t, binary+".verified", "status=verified\nversion=0.24.0\ndigest=abc\nselftest=passed\n")
	writeFile(t, filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate.sh"),
		"stamped_gate_version=\"0.24.0\"\nstamped_gate_impl=\"go\"\n")

	d.checkWorktreeGate()
	assertChecks(t, d, []Check{
		{OK, "worktree-gate",
			fmt.Sprintf("Go binary 0.24.0 at %s; selftest passed (cache %d KiB)", binary, testCacheKiB(t, d.Home)), ""},
	})
}

func TestCheckWorktreeGateNoUsableBinary(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, t.TempDir())
	goos, goarch := detectGatePlatform()
	binary := gateCacheBinPath(d.Home, goos, goarch)
	d.checkWorktreeGate()
	assertChecks(t, d, []Check{
		{ERROR, "worktree-gate",
			fmt.Sprintf("gate_impl=auto and no usable binary at %s; the gate is failing open", binary),
			"run ai-specs sync or ai-specs sync --refresh-gates"},
	})
}

func TestCheckWorktreeGateDigestMismatch(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "VERSION"), "0.24.0\n")
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	record := filepath.Join(home, "cache", "bin", "worktree-gate", "0.24.0", "last-digest-mismatch.txt")
	writeFile(t, record, "digest mismatch for worktree-gate-darwin-arm64\n")
	d := New(root, home)
	d.checkWorktreeGate()
	assertChecks(t, d, []Check{
		{ERROR, "worktree-gate",
			"digest mismatch for worktree-gate-darwin-arm64",
			"run ai-specs sync to re-acquire; the rejected artifact was never executed"},
	})

	// An empty record falls back to the frozen default message.
	writeFile(t, record, "")
	d2 := New(root, home)
	d2.checkWorktreeGate()
	assertChecks(t, d2, []Check{
		{ERROR, "worktree-gate",
			"gate binary digest mismatch recorded at last acquisition",
			"run ai-specs sync to re-acquire; the rejected artifact was never executed"},
	})
}

func TestCheckWorktreeGateBashRetired(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n"+
			"[recipes.worktree-flow.config]\ngate_impl = \"bash\"\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{
			{ERROR, "worktree-gate",
				"gate_impl=bash is retired; set auto or go, then ai-specs sync",
				"doctor is read-only; set gate_impl to auto or go, then run ai-specs sync"},
		})
	})
	t.Run("stamped", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
		d := New(root, t.TempDir())
		writeFile(t, filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate.sh"),
			"stamped_gate_impl=\"bash\"\n")
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{
			{ERROR, "worktree-gate",
				"gate_impl=bash is retired; set auto or go, then ai-specs sync",
				"doctor is read-only; set gate_impl to auto or go, then run ai-specs sync"},
		})
	})
}

func TestCheckWorktreeGateVersionMismatch(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, t.TempDir())
	writeCachedGate(t, d, gateVersionScript("0.25.0"))
	writeFile(t, filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate.sh"),
		"stamped_gate_version=\"0.24.0\"\n")
	d.checkWorktreeGate()
	assertChecks(t, d, []Check{
		{WARN, "worktree-gate",
			"gate binary version 0.25.0 does not match the stamped version 0.24.0",
			"run ai-specs sync to re-acquire for the installed CLI version"},
	})
}

func TestCheckWorktreeGateSelftestFailure(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, t.TempDir())
	binary := writeCachedGate(t, d, "#!/bin/sh\n"+
		"case \"$1\" in\n"+
		"  --version) printf '0.24.0\\n' ;;\n"+
		"  --selftest) printf 'selftest exploded\\n' 1>&2; exit 2 ;;\n"+
		"esac\nexit 0\n")
	d.checkWorktreeGate()
	assertChecks(t, d, []Check{
		{ERROR, "worktree-gate",
			fmt.Sprintf("gate binary at %s failed --selftest: selftest exploded; the gate is not enforcing", binary),
			"run ai-specs sync to re-acquire or re-build"},
	})
}

func TestCheckWorktreeGateLeftoverInfo(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, t.TempDir())
	writeCachedGate(t, d, gateVersionScript("0.24.0"))
	writeFile(t, filepath.Join(d.Root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate-legacy.sh"),
		"#!/bin/sh\n")
	d.checkWorktreeGate()
	if len(d.Checks) != 2 {
		t.Fatalf("checks = %+v", d.Checks)
	}
	if d.Checks[0] != (Check{INFO, "worktree-gate",
		"leftover worktree-gate-legacy.sh is inert and is not a governed asset",
		"rm ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh"}) {
		t.Errorf("first check = %+v", d.Checks[0])
	}
	if d.Checks[1].Severity != OK {
		t.Errorf("second check = %+v", d.Checks[1])
	}
	if !strings.Contains(d.Checks[1].Message, "Go binary 0.24.0 at ") {
		t.Errorf("second message = %q", d.Checks[1].Message)
	}
}
