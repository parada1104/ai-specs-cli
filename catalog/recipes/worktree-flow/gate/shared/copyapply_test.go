package shared

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Tests for the copy primitives moved out of the gate main package in SX0c.1.
// Moved verbatim together with their code; the only change is the exported
// names.

func TestCopyFileStatPreservesModeAndMtime(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.sh")
	dest := filepath.Join(tmp, "dest", "dest.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(src, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CopyFileStat(src, dest); err != nil {
		t.Fatalf("CopyFileStat: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
	if !info.ModTime().Equal(past) {
		t.Fatalf("mtime = %v, want %v", info.ModTime(), past)
	}
	body, _ := os.ReadFile(dest)
	if string(body) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("content = %q", body)
	}
}

// TestCopyTreeDirModeParity pins the copytree dir-mode contract: the
// directory is created with the umask-filtered default mode and the source's
// S_IMODE bits are applied by the copystat step after the children. A 0o777
// source directory (which a 022 umask would strip from any creation mode)
// must therefore still land at 0o777 on the dest side.
func TestCopyTreeDirModeParity(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dest := filepath.Join(tmp, "dest")
	if err := os.Mkdir(src, 0o777); err != nil {
		t.Fatal(err)
	}
	// Mkdir is umask-filtered; chmod is not, so this pins a genuine 0o777
	// source mode regardless of the test process's umask.
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o777 {
		t.Fatalf("dest dir mode = %o, want 777 (copystat parity, umask-independent)", info.Mode().Perm())
	}
}

// TestCopyTreePreservesSetgidDirBit pins StatMode's S_IMODE parity: the
// setgid bit on a source directory (which copystat preserves and plain
// Perm() drops) survives the copy via the post-children chmod.
func TestCopyTreePreservesSetgidDirBit(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dest := filepath.Join(tmp, "dest")
	if err := os.Mkdir(src, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o775|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if srcInfo.Mode()&os.ModeSetgid == 0 {
		t.Skip("setgid bits not preserved in this environment")
	}
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dest); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if destInfo.Mode()&os.ModeSetgid == 0 || destInfo.Mode().Perm() != 0o775 {
		t.Fatalf("dest dir mode = %v, want setgid + 0755 (copystat S_IMODE parity)", destInfo.Mode())
	}
}
