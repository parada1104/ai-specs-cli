package upgrade

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitEnv pins everything git reads so the fixtures are hermetic: the
// repository under test lives under a temp HOME and no user/global config
// leaks in.
func setupGitEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (dir %s): %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

type fakeInstall struct {
	home     string
	aiSpecs  string
	exe      string
	localBin string
	bare     string
}

func (f fakeInstall) env() Env {
	return Env{Home: f.aiSpecs, UserHome: f.home, Executable: f.exe}
}

// newFakeInstall mirrors tests/test_upgrade.py setup_global_install: a git
// checkout at $HOME/.ai-specs pushed to a bare origin, plus the
// $HOME/.local/bin/ai-specs symlink into it.
func newFakeInstall(t *testing.T, version string) fakeInstall {
	t.Helper()
	raw := t.TempDir()
	home, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("resolve temp home: %v", err)
	}
	setupGitEnv(t, home)

	aiSpecs := filepath.Join(home, ".ai-specs")
	if err := os.MkdirAll(filepath.Join(aiSpecs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(aiSpecs, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(aiSpecs, "bin", "ai-specs")
	writeFile(t, exe, "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, filepath.Join(aiSpecs, "VERSION"), version+"\n", 0o644)

	localBin := filepath.Join(home, ".local", "bin", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(localBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, localBin); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	bare := filepath.Join(home, "origin.git")
	gitRun(t, home, "init", "--bare", "origin.git")
	gitRun(t, aiSpecs, "init", "-b", "main")
	gitRun(t, aiSpecs, "add", ".")
	gitRun(t, aiSpecs, "commit", "-m", "init")
	gitRun(t, aiSpecs, "remote", "add", "origin", bare)
	gitRun(t, aiSpecs, "push", "-u", "origin", "main")

	return fakeInstall{home: home, aiSpecs: aiSpecs, exe: exe, localBin: localBin, bare: bare}
}

// pushVersion advances the bare origin's main branch to a new VERSION.
func (f fakeInstall) pushVersion(t *testing.T, version string) {
	t.Helper()
	upstream := filepath.Join(f.home, "upstream")
	gitRun(t, f.home, "clone", "-b", "main", f.bare, upstream)
	writeFile(t, filepath.Join(upstream, "VERSION"), version+"\n", 0o644)
	gitRun(t, upstream, "add", "VERSION")
	gitRun(t, upstream, "commit", "-m", "release "+version)
	gitRun(t, upstream, "push", "origin", "main")
}

func TestParseArgsHelp(t *testing.T) {
	var out, errb bytes.Buffer
	_, code, done := ParseArgs([]string{"--help"}, &out, &errb)
	if code != ExitOK || !done {
		t.Fatalf("code=%d done=%v", code, done)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("usage not on stdout: %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestParseArgsUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	_, code, done := ParseArgs([]string{"--bogus"}, &out, &errb)
	if code != ExitNotStandard || !done {
		t.Fatalf("code=%d done=%v", code, done)
	}
	if !strings.Contains(errb.String(), "Unknown argument") {
		t.Fatalf("stderr = %q", errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", out.String())
	}
}

func TestParseArgsFlags(t *testing.T) {
	var out, errb bytes.Buffer
	o, code, done := ParseArgs([]string{"--dry-run", "--force", "-v"}, &out, &errb)
	if done || code != 0 {
		t.Fatalf("done=%v code=%d", done, code)
	}
	if !o.DryRun || !o.Force || !o.Verbose {
		t.Fatalf("options = %+v", o)
	}
	long, _, _ := ParseArgs([]string{"--verbose"}, &out, &errb)
	if !long.Verbose {
		t.Fatalf("--verbose not set: %+v", long)
	}
}

func TestDetectMissingHome(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	var errb bytes.Buffer
	_, code, ok := Detect(Env{Home: "", UserHome: f.home, Executable: f.exe}, &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "AI_SPECS_HOME is not set") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectExecutableOutside(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	devExe := filepath.Join(f.home, "dev", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(devExe), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, devExe, "x", 0o755)

	var errb bytes.Buffer
	_, code, ok := Detect(Env{Home: f.aiSpecs, UserHome: f.home, Executable: devExe}, &errb)
	if ok || code != ExitNotStandard {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "not the standard global installation") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectMissingGitDir(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	if err := os.RemoveAll(filepath.Join(f.aiSpecs, ".git")); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	_, code, ok := Detect(f.env(), &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), ".git directory") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectMissingLocalBin(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	if err := os.Remove(f.localBin); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	_, code, ok := Detect(f.env(), &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "missing or not a symlink") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectLocalBinNotSymlink(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	if err := os.Remove(f.localBin); err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.localBin, "not a link\n", 0o644)
	var errb bytes.Buffer
	_, code, ok := Detect(f.env(), &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "missing or not a symlink") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectBrokenSymlink(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	if err := os.Remove(f.localBin); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.home, "does-not-exist"), f.localBin); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	_, code, ok := Detect(f.env(), &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "symlink appears broken") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectSymlinkOutside(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	outside := filepath.Join(f.home, "outside", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, outside, "x", 0o755)
	if err := os.Remove(f.localBin); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, f.localBin); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	_, code, ok := Detect(f.env(), &errb)
	if ok || code != ExitBroken {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if !strings.Contains(errb.String(), "resolves outside ~/.ai-specs") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDetectValid(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	var errb bytes.Buffer
	inst, code, ok := Detect(f.env(), &errb)
	if !ok || code != 0 {
		t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
	}
	if inst.Home != f.aiSpecs || inst.LocalBin != f.localBin {
		t.Fatalf("install = %+v", inst)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	versionPath := filepath.Join(f.aiSpecs, "VERSION")
	before, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := Run([]string{"--dry-run"}, f.env(), &out, &errb); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	for _, want := range []string{
		"Dry-run: no changes will be made.",
		"Current version: 1.0.0",
		"Target version:  1.0.0",
		"Already up to date.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, out.String())
		}
	}

	after, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("VERSION changed during dry-run: %q -> %q", before, after)
	}
}

func TestDirtyTreeRefused(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	writeFile(t, filepath.Join(f.aiSpecs, "VERSION"), "9.9.9\n", 0o644)

	var out, errb bytes.Buffer
	if code := Run(nil, f.env(), &out, &errb); code != ExitPreflight {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "dirty") {
		t.Fatalf("stderr = %q", errb.String())
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Fatalf("stderr = %q", errb.String())
	}

	var forceOut, forceErr bytes.Buffer
	if code := Run([]string{"--force"}, f.env(), &forceOut, &forceErr); code != ExitOK {
		t.Fatalf("force code=%d stderr=%s", code, forceErr.String())
	}
	if !strings.Contains(forceErr.String(), "Warning: working tree is dirty") {
		t.Fatalf("stderr = %q", forceErr.String())
	}
}

func TestModeOnlyDirtDryRunThenReal(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	gitRun(t, f.aiSpecs, "config", "core.fileMode", "true")
	versionPath := filepath.Join(f.aiSpecs, "VERSION")
	if err := os.Chmod(versionPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if st := gitRun(t, f.aiSpecs, "status", "--porcelain"); strings.TrimSpace(st) == "" {
		t.Fatalf("fixture is not dirty after chmod")
	}

	// --dry-run previews only: it must not remediate the mode-only dirt.
	var out, errb bytes.Buffer
	if code := Run([]string{"--dry-run"}, f.env(), &out, &errb); code != ExitOK {
		t.Fatalf("dry-run code=%d stderr=%s", code, errb.String())
	}
	if strings.Contains(errb.String(), "Restoring file modes") {
		t.Fatalf("dry-run remediated the tree: %s", errb.String())
	}
	if fm, err := os.Lstat(versionPath); err != nil || fm.Mode().Perm() != 0o755 {
		t.Fatalf("dry-run changed mode: %v %v", fm, err)
	}

	// A real run restores the installer-altered modes and continues.
	var realOut, realErr bytes.Buffer
	if code := Run(nil, f.env(), &realOut, &realErr); code != ExitOK {
		t.Fatalf("real code=%d stderr=%s", code, realErr.String())
	}
	if !strings.Contains(realErr.String(), "Restoring file modes altered by a previous installer...") {
		t.Fatalf("stderr = %q", realErr.String())
	}
	if fm, err := os.Lstat(versionPath); err != nil || fm.Mode().Perm() != 0o644 {
		t.Fatalf("mode not restored: %v %v", fm, err)
	}
}

func TestRealFastForward(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	f.pushVersion(t, "2.0.0")

	var out, errb bytes.Buffer
	if code := Run(nil, f.env(), &out, &errb); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Upgraded: 1.0.0 -> 2.0.0") {
		t.Errorf("stdout missing upgrade line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Symlink integrity verified") {
		t.Errorf("stdout missing symlink verification:\n%s", out.String())
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(f.aiSpecs, "VERSION"))); got != "2.0.0" {
		t.Errorf("VERSION = %q", got)
	}
}

func TestRealDiverged(t *testing.T) {
	f := newFakeInstall(t, "1.0.0")
	writeFile(t, filepath.Join(f.aiSpecs, "local.txt"), "local\n", 0o644)
	gitRun(t, f.aiSpecs, "add", "local.txt")
	gitRun(t, f.aiSpecs, "commit", "-m", "local work")

	var out, errb bytes.Buffer
	if code := Run(nil, f.env(), &out, &errb); code != ExitPreflight {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "diverged") {
		t.Fatalf("stderr = %q", errb.String())
	}
}
