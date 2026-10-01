package projectcache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
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

// ============================================================================
