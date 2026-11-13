package cli

// GO-20 U1a: the native `upgrade` route, pinned at the process boundary, plus
// the two shell post-steps this port deliberately does NOT run.
//
// The discrimination is exact and needs no network: the fixture looks like a
// standard installation whose `lib/upgrade.sh` is a MARKER STUB, so a run that
// reaches the shell route leaves a marker behind. `git` is a stub that reports
// a fast-forward from 1.0.0 to 9.9.9, and the `python3` on PATH is a marker
// stub too, so the TUI-dependency refresh would be visible if it ran.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub writes an executable shell stub.
func writeStub(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// copyExecutable copies src to dst preserving the executable bit.
func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeRouteIsNative(t *testing.T) {
	if got := Route([]string{"upgrade"}).kind; got != routeUpgrade {
		t.Errorf("Route(upgrade).kind = %v, want routeUpgrade", got)
	}
	if got := Route([]string{"upgrade", "--dry-run"}).kind; got != routeUpgrade {
		t.Errorf("Route(upgrade --dry-run).kind = %v, want routeUpgrade", got)
	}
}

// TestUpgradeRunsNativelyAndSkipsTheShellPostSteps pins three things at once:
// the route runs the native engine (the fixture's lib/upgrade.sh stub is never
// executed), the fast-forward path still reports the crossed versions, and the
// two shell post-steps the port deliberately drops — the `pip` TUI-dependency
// refresh and `narrow-checkout.sh` — really do not run.
func TestUpgradeRunsNativelyAndSkipsTheShellPostSteps(t *testing.T) {
	bin, _ := smokeSetup(t)

	root := t.TempDir()
	// The port compares PHYSICAL paths (filepath.EvalSymlinks) where the shell
	// compares logical ones (`cd && pwd`), so a fixture under macOS's
	// /var -> /private/var symlink would abort the last install check. Resolving
	// the scratch root here keeps the fixture faithful to a real `$HOME`; the
	// logical-vs-physical difference itself is recorded in the unit doc.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	userHome := filepath.Join(root, "home")
	home := filepath.Join(userHome, ".ai-specs")
	installed := filepath.Join(home, "bin", "ai-specs")
	markers := map[string]string{
		"shell":  filepath.Join(root, "marker-lib-upgrade-sh"),
		"pip":    filepath.Join(root, "marker-pip"),
		"narrow": filepath.Join(root, "marker-narrow"),
	}

	for _, dir := range []string{
		filepath.Join(home, ".git"),
		filepath.Join(home, "lib", "_internal"),
		filepath.Join(userHome, ".local", "bin"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyExecutable(t, bin, installed)
	if err := os.Symlink(installed, filepath.Join(userHome, ".local", "bin", "ai-specs")); err != nil {
		t.Fatal(err)
	}

	// The shell route's implementation, replaced by a marker stub: if the
	// binary still shimmed `upgrade`, this would run and leave its marker.
	writeStub(t, filepath.Join(home, "lib", "upgrade.sh"), "echo ran > "+markers["shell"])
	writeStub(t, filepath.Join(home, "lib", "_internal", "narrow-checkout.sh"), "echo ran > "+markers["narrow"])

	pathDir := filepath.Join(root, "path")
	writeStub(t, filepath.Join(pathDir, "python3"), "echo ran > "+markers["pip"])
	writeStub(t, filepath.Join(pathDir, "git"), strings.Join([]string{
		`case "$*" in`,
		`  "-c core.fileMode=false status --porcelain") exit 0 ;;`,
		`  "status --porcelain") exit 0 ;;`,
		`  "rev-parse --verify origin/main") echo 2222222222222222222222222222222222222222; exit 0 ;;`,
		`  "merge-base --is-ancestor HEAD origin/main") exit 0 ;;`,
		`  "rev-parse HEAD") echo 1111111111111111111111111111111111111111; exit 0 ;;`,
		`  "rev-parse origin/main") echo 2222222222222222222222222222222222222222; exit 0 ;;`,
		`  "show origin/main:VERSION") echo 9.9.9; exit 0 ;;`,
		`  "fetch origin main") exit 0 ;;`,
		`  "merge --ff-only origin/main") printf '9.9.9\n' > "` + filepath.Join(home, "VERSION") + `"; exit 0 ;;`,
		`  *) exit 0 ;;`,
		`esac`,
	}, "\n"))

	env := []string{
		"PATH=" + pathDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + userHome,
		"AI_SPECS_HOME=" + home,
		"TMPDIR=" + root,
		"LC_ALL=C",
		"LANG=C",
	}
	stdout, stderr, code := runOne(installed, []string{"upgrade"}, env, root)
	if code != 0 {
		t.Fatalf("native upgrade rc = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(string(stdout), "Upgraded: 1.0.0 -> 9.9.9") {
		t.Errorf("stdout does not report the crossed versions:\n%s", stdout)
	}
	for name, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			t.Errorf("the %s step ran, but this port deliberately does not run it (%s)", name, marker)
		}
	}
}

// TestUpgradeUsageMatchesTheShellSurface pins the verb's usage bytes end to
// end: the native route prints internal/upgrade.Usage, which mirrors the
// lib/upgrade.sh heredoc, and the legacy launcher prints the same bytes.
func TestUpgradeUsageMatchesTheShellSurface(t *testing.T) {
	bin, root := smokeSetup(t)
	env := smokeEnv(root)

	nativeOut, nativeErr, nativeCode := runOne(bin, []string{"upgrade", "--help"}, env, root)
	legacyOut, legacyErr, legacyCode := runOne("bash", []string{filepath.Join(root, "bin", "ai-specs"), "upgrade", "--help"}, env, root)

	if nativeCode != legacyCode {
		t.Errorf("exit code: native %d, legacy %d", nativeCode, legacyCode)
	}
	if string(nativeOut) != string(legacyOut) {
		t.Errorf("stdout mismatch:\n native: %q\n legacy: %q", nativeOut, legacyOut)
	}
	if string(nativeErr) != string(legacyErr) {
		t.Errorf("stderr mismatch:\n native: %q\n legacy: %q", nativeErr, legacyErr)
	}
}
