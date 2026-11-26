package home

import (
	"os"
	"path/filepath"
	"testing"
)

// makeRoot builds a fake install tree:
//
//	root/VERSION                   installation marker every verb reads
//	root/bin/ai-specs              real launcher
//	root/real/VERSION              installation marker of the second home
//	root/real/bin/ai-specs         real launcher used through the symlink chain
//	root/link/bin/ai-specs         intermediate symlink (relative, 2 hops)
//	root/alias                     entry symlink (absolute)
//
// The relative target ../../real/bin/ai-specs is anchored at the link's
// directory (root/link/bin), so it climbs out of bin/ and link/ and lands on
// root/real — the same two-level climb bash performs with
// SOURCE="$DIR/$SOURCE". The VERSION markers make these trees real homes, so
// the cases below assert resolution, not the fallback.
func makeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"bin", "real/bin", "link/bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{root, filepath.Join(root, "real")} {
		writeVersion(t, dir)
	}
	real := filepath.Join(root, "real", "bin", "ai-specs")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// link/bin/ai-specs -> ../../real/bin/ai-specs (relative target)
	if err := os.Symlink(filepath.Join("..", "..", "real", "bin", "ai-specs"),
		filepath.Join(root, "link", "bin", "ai-specs")); err != nil {
		t.Fatal(err)
	}
	// alias -> link/bin/ai-specs (absolute target)
	if err := os.Symlink(filepath.Join(root, "link", "bin", "ai-specs"),
		filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeVersion(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, VersionFile), []byte("0.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// installedLayout builds what install.sh produces: a REAL compiled binary at
// $INSTALL_BIN/ai-specs (default ~/.local/bin) and the data home at
// $HOME/.ai-specs. It returns the exec path an installed run would report.
func installedLayout(t *testing.T) (execPath, userHome string) {
	t.Helper()
	userHome = t.TempDir()
	t.Setenv("HOME", userHome)
	writeVersion(t, filepath.Join(userHome, DefaultDirName))
	binDir := filepath.Join(userHome, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	execPath = filepath.Join(binDir, "ai-specs")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return execPath, userHome
}

func TestResolveHomeEnvOverrideWinsVerbatim(t *testing.T) {
	got := ResolveHome("/some/bin/ai-specs", "/custom/home")
	if got != "/custom/home" {
		t.Fatalf("env override must be returned verbatim, got %q", got)
	}
}

func TestResolveHomeEnvOverrideBeatsWalk(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "bin", "ai-specs"), "/override/home")
	if got != "/override/home" {
		t.Fatalf("env override must win over the walk, got %q", got)
	}
}

func TestResolveHomePlainPath(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "bin", "ai-specs"), "")
	want := root
	if got != want {
		t.Fatalf("home = %q, want %q", got, want)
	}
}

func TestResolveHomeSymlinkChain(t *testing.T) {
	root := makeRoot(t)
	got := ResolveHome(filepath.Join(root, "alias"), "")
	want := filepath.Join(root, "real")
	if got != want {
		t.Fatalf("home = %q, want %q", got, want)
	}
}

// TestResolveHomeInstalledBinaryUsesDefaultHome is the installer defect: a real
// binary at ~/.local/bin must resolve to ~/.ai-specs, never to ~/.local, with no
// AI_SPECS_HOME in the environment.
func TestResolveHomeInstalledBinaryUsesDefaultHome(t *testing.T) {
	execPath, userHome := installedLayout(t)
	got := ResolveHome(execPath, "")
	want := filepath.Join(userHome, DefaultDirName)
	if got != want {
		t.Fatalf("installed binary: home = %q, want %q (the PATH directory is not a home)", got, want)
	}
}

// TestResolveHomeInstalledDistBinaryUsesHome covers the copy install.sh leaves
// at $AI_SPECS_HOME/dist/ai-specs, the path the generated hooks execute.
func TestResolveHomeInstalledDistBinaryUsesHome(t *testing.T) {
	_, userHome := installedLayout(t)
	execPath := filepath.Join(userHome, DefaultDirName, "dist", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ResolveHome(execPath, "")
	want := filepath.Join(userHome, DefaultDirName)
	if got != want {
		t.Fatalf("dist binary: home = %q, want %q", got, want)
	}
}

// TestResolveHomeKeepsGuessWithoutAnyHome documents the residual behavior: with
// no installation anywhere, resolution stays the legacy geometric answer
// instead of inventing a path.
func TestResolveHomeKeepsGuessWithoutAnyHome(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	binDir := filepath.Join(userHome, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	execPath := filepath.Join(binDir, "ai-specs")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ResolveHome(execPath, "")
	want := filepath.Join(userHome, ".local")
	if got != want {
		t.Fatalf("no home anywhere: home = %q, want the legacy guess %q", got, want)
	}
}

// TestResolveHomeIgnoresDistVersionStamp covers a supported override:
// INSTALL_BIN=<home>/dist/bin. dist/VERSION is install.sh's cache stamp
// ("<version> <sha256> <size>:<mtime>:<ctime>:<inode>"), not a home marker, so
// the entry's parent directory must not be mistaken for the home.
func TestResolveHomeIgnoresDistVersionStamp(t *testing.T) {
	_, userHome := installedLayout(t)
	homeDir := filepath.Join(userHome, DefaultDirName)
	writeFile := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join(homeDir, "dist", "VERSION"), "0.27.0 abcdef 123:456:789:42\n")
	execPath := filepath.Join(homeDir, "dist", "bin", "ai-specs")
	writeFile(execPath, "#!/bin/sh\n")

	got := ResolveHome(execPath, "")
	if got != homeDir {
		t.Fatalf("home = %q, want %q (a dist stamp is not a home marker)", got, homeDir)
	}
}

// TestResolveHomeCloserMarkedHomeWins pins the documented limit: when BOTH the
// entry's directory and the default install are marked homes, the closer one
// wins, exactly as the legacy geometric resolver did. Resolving that ambiguity
// needs a record of where the entry was installed, not a heuristic.
func TestResolveHomeCloserMarkedHomeWins(t *testing.T) {
	execPath, userHome := installedLayout(t)
	if err := os.WriteFile(filepath.Join(userHome, ".local", VersionFile), []byte("9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveHome(execPath, "")
	want := filepath.Join(userHome, ".local")
	if got != want {
		t.Fatalf("home = %q, want the closer marked home %q", got, want)
	}
}
