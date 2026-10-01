package projectcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ============================================================================
// Commit A: core unit tests
// ============================================================================

func TestSanitizeBasename(t *testing.T) {
	cases := map[string]string{
		"my project":        "my-project",
		"a/b":               "a-b",
		"---":               "project",
		"...":               "project",
		"weird@#$%name":     "weird-name",
		"  x  ":             "x",
		"\u00dcn\u00efcode": "n-code",
		"ok-name_1.2":       "ok-name_1.2",
		"":                  "project",
		"/":                 "project",
	}
	for in, want := range cases {
		if got := SanitizeBasename(in); got != want {
			t.Errorf("SanitizeBasename(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCacheKeyFrozen pins the FROZEN derivation against a value computed by the
// real Python module (python3 -c "import project-cache; cache_key(Path('/'))").
func TestCacheKeyFrozen(t *testing.T) {
	if got := CacheKey("/"); got != "8a5edab28263-project" {
		t.Errorf("CacheKey(\"/\") = %q, want %q", got, "8a5edab28263-project")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if CacheKey(link) != CacheKey(target) {
		t.Errorf("symlink key %q != target key %q (resolve() semantics)", CacheKey(link), CacheKey(target))
	}

	resolved := ResolvePath(target)
	sum := sha256.Sum256([]byte(resolved))
	want := hex.EncodeToString(sum[:])[:12] + "-" + SanitizeBasename(filepath.Base(resolved))
	if got := CacheKey(target); got != want {
		t.Errorf("CacheKey(%q) = %q, want %q", target, got, want)
	}
}

func TestEnsureCacheCreateRefresh(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	resolved := ResolvePath(proj)

	cache, err := EnsureCache(proj, home)
	if err != nil {
		t.Fatalf("EnsureCache: %v", err)
	}
	if !strings.HasPrefix(cache, ResolvePath(home)) {
		t.Errorf("cache %q not under cli_home %q", cache, ResolvePath(home))
	}
	meta := filepath.Join(cache, "meta.toml")
	data, err := os.ReadFile(meta)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	created := "project_root = \"" + resolved + "\"\n"
	if !strings.HasPrefix(string(data), created) {
		t.Errorf("meta create = %q, want prefix %q", data, created)
	}
	if !regexp.MustCompile(`created_at = "\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z"`).Match(data) {
		t.Errorf("meta create missing UTC created_at: %q", data)
	}

	// Refresh: every project_root-prefixed line is rewritten (project_rootish too).
	mustWrite(t, meta, "project_root = \"/old\"\nfoo = 1\nproject_rootish = 2\n")
	if _, err := EnsureCache(proj, home); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, meta)
	want := "project_root = \"" + resolved + "\"\nfoo = 1\nproject_root = \"" + resolved + "\"\n"
	if got != want {
		t.Errorf("refresh = %q, want %q", got, want)
	}

	// Absent root line is inserted at the top, newline-terminated.
	mustWrite(t, meta, "foo = 1\nbar = 2")
	if _, err := EnsureCache(proj, home); err != nil {
		t.Fatal(err)
	}
	want = "project_root = \"" + resolved + "\"\nfoo = 1\nbar = 2\n"
	if got := readFile(t, meta); got != want {
		t.Errorf("insert = %q, want %q", got, want)
	}

	// splitlines() collapses CRLF terminators.
	mustWrite(t, meta, "a = 1\r\nb = 2\r\n")
	if _, err := EnsureCache(proj, home); err != nil {
		t.Fatal(err)
	}
	want = "project_root = \"" + resolved + "\"\na = 1\nb = 2\n"
	if got := readFile(t, meta); got != want {
		t.Errorf("crlf = %q, want %q", got, want)
	}
}

func TestPathRoots(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	cache := CacheRoot(proj, home)
	cases := []struct {
		got, want string
	}{
		{RecipeSkillsRoot(proj, home), filepath.Join(cache, ".recipe")},
		{DepsSkillsRoot(proj, home), filepath.Join(cache, ".deps")},
		{BundledSkillsRoot(proj, home), filepath.Join(cache, ".bundled")},
		{BundledCommandsRoot(proj, home), filepath.Join(cache, ".bundled", "commands")},
		{CommandsDir(proj, home), filepath.Join(cache, "commands")},
		{BackupsRoot(proj, home), filepath.Join(cache, "backups")},
		{ResolvedSkillsDir(proj, home), filepath.Join(cache, "resolved-skills")},
		{InprojectDepsRoot(proj), filepath.Join(proj, "ai-specs", ".deps")},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("root = %q, want %q", c.got, c.want)
		}
	}
}

func TestGateBackupPath(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	got := GateBackupPath(proj, "ai-specs/skills/x", "abc123", home)
	want := filepath.Join(BackupsRoot(proj, home), sha256Hex([]byte("ai-specs/skills/x")), "abc123.sh")
	if got != want {
		t.Errorf("GateBackupPath = %q, want %q", got, want)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// pyRealpaths returns os.path.realpath(p) for each path, computed by the real
// Python interpreter (the oracle for Path.resolve()/realpath semantics).
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
// algorithm: each component's symlinks resolve BEFORE `..` pops against the
// resolved prefix; a dangling component leaves the tail unresolved; a symlink
// loop returns the looping link unresolved. Clean-first (filepath.Abs/Clean)
// collapses `..` before symlink resolution and diverges on every case below.
func TestResolvePathMatchesPythonRealpath(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "deep", "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "deep", "a", "b", "target.txt"), "t\n")
	mustSymlink(t, deep, filepath.Join(dir, "abslink"))
	mustSymlink(t, filepath.Join("deep", "a", "b", "c"), filepath.Join(dir, "rellink"))
	mustSymlink(t, filepath.Join(dir, "missing", "nope"), filepath.Join(dir, "dangling"))
	mustSymlink(t, filepath.Join(dir, "loop_b"), filepath.Join(dir, "loop_a"))
	mustSymlink(t, filepath.Join(dir, "loop_a"), filepath.Join(dir, "loop_b"))

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

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// TestRemoveLegacyOriginRetryPreservesThenRemovesDotRecipe pins the FROZEN
// retry flow of RemoveLegacyOrigin (project-cache.py L497-531). The first run
// fails a migration, so ai-specs/.recipe/ is retained for a later sync; the
// retry then sees the partial destination (dest.exists() -> continue), nothing
// fails in that run, and .recipe/ is removed. The unreadable 0o000 file is the
// deterministic failure trigger at this point in the history; the dangling
// symlink only becomes a failure source once copyTree follows links (the later
// copyTree fix). Both runs are oracle behavior - do not guard the partial dest.
func TestRemoveLegacyOriginRetryPreservesThenRemovesDotRecipe(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	overrides := filepath.Join(root, "ai-specs", ".recipe", "recipeA", "overrides")
	if err := os.MkdirAll(overrides, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overrides, "ok.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "ai-specs", "missing", "nope"), filepath.Join(overrides, "dangling")); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(overrides, "blocked.md")
	if err := os.WriteFile(unreadable, []byte("secret\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	// First run: the unreadable source aborts the migration -> .recipe retained.
	var out1, err1 bytes.Buffer
	RemoveLegacyOrigin(root, home, &out1, &err1)
	if !isDir(filepath.Join(root, "ai-specs", ".recipe")) {
		t.Fatalf("first run removed .recipe despite a failed migration; stderr=%q", err1.String())
	}
	if !strings.Contains(err1.String(), "failed to migrate overrides for 'recipeA': ") {
		t.Errorf("first run stderr = %q, want the migration-failure warning", err1.String())
	}
	// The partial destination exists, so the retry skips this recipe.
	dest := filepath.Join(root, "ai-specs", "recipes", "recipeA", "overrides")
	if !isDir(dest) {
		t.Fatalf("expected a partial destination after the failed migration")
	}

	// Second run: dest.exists() skips migration, nothing fails -> .recipe removed.
	var out2, err2 bytes.Buffer
	RemoveLegacyOrigin(root, home, &out2, &err2)
	if isDir(filepath.Join(root, "ai-specs", ".recipe")) {
		t.Fatalf("second run kept .recipe although the retry had nothing to migrate; stdout=%q stderr=%q", out2.String(), err2.String())
	}
}

// ============================================================================
