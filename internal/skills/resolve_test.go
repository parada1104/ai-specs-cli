package skills

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// pyRealpaths returns os.path.realpath(p) for each path via the real Python
// interpreter (the oracle for Path.resolve()/realpath semantics).
func pyRealpaths(t *testing.T, paths []string) []string {
	t.Helper()
	payload, err := json.Marshal(paths)
	if err != nil {
		t.Fatal(err)
	}
	script := "import json,os,sys;print(json.dumps([os.path.realpath(p) for p in json.load(sys.stdin)]))"
	cmd := exec.Command("python3", "-c", script)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python realpath: %v", err)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse realpath output: %v\n%s", err, out)
	}
	return got
}

// TestResolvePathMatchesPythonRealpath pins the posixpath.realpath(strict=False)
// algorithm shared with internal/projectcache. A Clean-first (filepath.Abs)
// implementation collapses `..` before symlink resolution and diverges here.
func TestResolvePathMatchesPythonRealpath(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "deep", "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(dir, "deep", "a", "b", "target.txt"), "t\n")
	symlinkT(t, deep, filepath.Join(dir, "abslink"))
	symlinkT(t, filepath.Join("deep", "a", "b", "c"), filepath.Join(dir, "rellink"))
	symlinkT(t, filepath.Join(dir, "missing", "nope"), filepath.Join(dir, "dangling"))
	symlinkT(t, filepath.Join(dir, "loop_b"), filepath.Join(dir, "loop_a"))
	symlinkT(t, filepath.Join(dir, "loop_a"), filepath.Join(dir, "loop_b"))

	candidates := []string{
		dir + "/abslink/../target.txt",
		dir + "/rellink/../target.txt",
		dir + "/dangling/..",
		dir + "/dangling/sub/../file",
		dir + "/loop_a",
		dir + "/loop_a/..",
		dir + "/deep/a/b/c/../../target.txt",
		dir + "/deep/a/./b//c",
		dir,
	}
	want := pyRealpaths(t, candidates)
	for i, p := range candidates {
		if got := ResolvePath(p); got != want[i] {
			t.Errorf("ResolvePath(%q) = %q, want python %q", p, got, want[i])
		}
	}
}

func symlinkT(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
