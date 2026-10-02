package projectcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
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

// TestPyStem pins PurePath.stem under the epic's Python reference (3.11-3.13):
// a dot that is the first character or the last character is not a suffix.
func TestPyStem(t *testing.T) {
	cases := map[string]string{
		"x.md":       "x",
		"x.tar.md":   "x.tar",
		".md":        ".md",
		".hidden.md": ".hidden",
		"..md":       ".",
		"a.":         "a.",
		"a":          "a",
		".a.md":      ".a",
	}
	for in, want := range cases {
		if got := pyStem(in); got != want {
			t.Errorf("pyStem(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPySuffix pins PurePath.suffix under the epic's Python reference
// (3.11-3.13): with i = name.rfind('.'), the suffix is name[i:] only when
// 0 < i < len(name)-1. So `.md` has an EMPTY suffix (kept by the filters) and
// `..md` has `.md` (removed), unlike Python 3.14 which changed leading dots.
func TestPySuffix(t *testing.T) {
	cases := map[string]string{
		"x.md":       ".md",
		"x.tar.md":   ".md",
		".md":        "",
		"..md":       ".md",
		".hidden.md": ".md",
		"a.md.bak":   ".bak",
		"a.":         "",
		"a":          "",
		"":           "",
	}
	for in, want := range cases {
		if got := pySuffix(in); got != want {
			t.Errorf("pySuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderProjectCacheMissingHome pins the accepted loud failure: with no
// cli_home and no AI_SPECS_HOME the port refuses instead of silently resolving
// the cache CWD-relative (Python falls back to its module repo root).
func TestRenderProjectCacheMissingHome(t *testing.T) {
	t.Setenv("AI_SPECS_HOME", "")
	var out, errBuf bytes.Buffer
	if rc := RenderProjectCache(t.TempDir(), "ensure", "", &out, &errBuf); rc != 1 {
		t.Errorf("rc = %d, want 1", rc)
	}
	if got := errBuf.String(); got != "error: AI_SPECS_HOME is not set\n" {
		t.Errorf("stderr = %q, want the missing-home error line", got)
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
	pcWrite(t, filepath.Join(dir, "deep", "a", "b", "target.txt"), "t\n", 0o644)
	mustSymlink(t, deep, filepath.Join(dir, "abslink"))
	mustSymlink(t, filepath.Join("deep", "a", "b", "c"), filepath.Join(dir, "rellink"))
	mustSymlink(t, filepath.Join(dir, "missing", "nope"), filepath.Join(dir, "dangling"))
	mustSymlink(t, filepath.Join(dir, "loop_b"), filepath.Join(dir, "loop_a"))
	mustSymlink(t, filepath.Join(dir, "loop_a"), filepath.Join(dir, "loop_b"))

	// R3-realpath-nonfinal-symlink shapes (X1 review-a6ac5ceb6e6867a1):
	// non-final symlinks with absolute/multi-component targets, links to files,
	// targets containing '..', chained links, and a dangling mid-path.
	if err := os.MkdirAll(filepath.Join(dir, "other", "place"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	pcWrite(t, filepath.Join(dir, "other", "target.txt"), "o\n", 0o644)
	pcWrite(t, filepath.Join(dir, "file.txt"), "f\n", 0o644)
	mustSymlink(t, filepath.Join(dir, "other", "place"), filepath.Join(dir, "d", "abslink"))
	mustSymlink(t, filepath.Join("..", "other", "place"), filepath.Join(dir, "d", "rellink"))
	mustSymlink(t, filepath.Join(dir, "file.txt"), filepath.Join(dir, "filelink"))
	mustSymlink(t, filepath.Join(dir, "d", "..", "other", "place"), filepath.Join(dir, "dotlink"))
	mustSymlink(t, filepath.Join(dir, "chain2"), filepath.Join(dir, "chain1"))
	mustSymlink(t, filepath.Join(dir, "other", "place"), filepath.Join(dir, "chain2"))

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
		dir + "/d/abslink/../target.txt",
		dir + "/d/rellink/../target.txt",
		dir + "/filelink/../file.txt",
		dir + "/dotlink/../target.txt",
		dir + "/chain1/../target.txt",
		dir + "/dangling/../target.txt",
		dir + "/loop_a/x",
		dir + "/loop_a/../x",
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

// The RemoveLegacyOrigin tests below pin the user-authorized SAFETY EXCEPTION
// (GO-07.S6): Python's remove_legacy_origin (project-cache.py L497-531) treats
// an existing destination as proof of a complete migration, so a retry after a
// failed (partial) copy deletes ai-specs/.recipe/ and silently loses the files
// the first run never copied. The Go port intentionally diverges: an existing
// destination whose completeness is unproven keeps the legacy originals and
// warns. Fresh fully-successful migrations still remove .recipe/.

// TestRemoveLegacyOriginRetryPreservesDotRecipe: a migration that fails partway
// leaves a PARTIAL destination; every retry (second and third) must keep
// ai-specs/.recipe/ (including the entry the first run never copied) and warn,
// never claim the origin was removed. The trigger is a dangling symlink, which
// is root-safe and persists across runs.
func TestRemoveLegacyOriginRetryPreservesDotRecipe(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	legacy := filepath.Join(root, "ai-specs", ".recipe")
	overrides := filepath.Join(legacy, "recipeA", "overrides")
	if err := os.MkdirAll(overrides, 0o755); err != nil {
		t.Fatal(err)
	}
	pcWrite(t, filepath.Join(overrides, "ok.md"), "ok\n", 0o644)
	mustSymlink(t, filepath.Join(root, "ai-specs", "missing", "nope"), filepath.Join(overrides, "dangling"))

	// First run: the dangling symlink fails the walk -> .recipe retained.
	var out1, err1 bytes.Buffer
	RemoveLegacyOrigin(root, home, &out1, &err1)
	if !isDir(legacy) {
		t.Fatalf("first run removed .recipe despite a failed migration; stderr=%q", err1.String())
	}
	if !strings.Contains(err1.String(), "failed to migrate overrides for 'recipeA': ") {
		t.Errorf("first run stderr = %q, want the migration-failure warning", err1.String())
	}
	if !isDir(filepath.Join(root, "ai-specs", "recipes", "recipeA", "overrides")) {
		t.Fatalf("expected a partial destination after the failed migration")
	}

	// Retries: the destination exists but completeness is unproven -> warn and
	// preserve, run after run.
	for i := 2; i <= 3; i++ {
		var out, errBuf bytes.Buffer
		RemoveLegacyOrigin(root, home, &out, &errBuf)
		if !isDir(legacy) {
			t.Fatalf("run %d removed .recipe; stdout=%q stderr=%q", i, out.String(), errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "already has overrides at ai-specs/recipes/recipeA/overrides/") {
			t.Errorf("run %d stderr = %q, want the existing-destination warning", i, errBuf.String())
		}
		if strings.Contains(out.String(), "removed leftover ai-specs/.recipe/") {
			t.Errorf("run %d claimed to remove .recipe; stdout=%q", i, out.String())
		}
		if _, err := os.Lstat(filepath.Join(overrides, "dangling")); err != nil {
			t.Errorf("run %d lost the uncopied legacy entry: %v", i, err)
		}
	}
}

// TestRemoveLegacyOriginExistingDestPreservesSourceAndUserContent: a
// pre-existing destination (possibly partial from a failed run) must never be
// overwritten or deleted, and the legacy source is retained for verification.
func TestRemoveLegacyOriginExistingDestPreservesSourceAndUserContent(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	overrides := filepath.Join(root, "ai-specs", ".recipe", "recipeA", "overrides")
	pcWrite(t, filepath.Join(overrides, "legacy.md"), "legacy\n", 0o644)
	dest := filepath.Join(root, "ai-specs", "recipes", "recipeA", "overrides")
	pcWrite(t, filepath.Join(dest, "user.md"), "user sentinel\n", 0o600)

	var out, errBuf bytes.Buffer
	RemoveLegacyOrigin(root, home, &out, &errBuf)

	if !isDir(filepath.Join(root, "ai-specs", ".recipe")) {
		t.Fatalf("removed .recipe with an unverified existing destination; stderr=%q", errBuf.String())
	}
	if got, err := os.ReadFile(filepath.Join(overrides, "legacy.md")); err != nil || string(got) != "legacy\n" {
		t.Errorf("legacy source not preserved: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "user.md")); err != nil || string(got) != "user sentinel\n" {
		t.Errorf("destination user content not preserved: %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "legacy.md")); err == nil {
		t.Errorf("existing destination was overwritten with the legacy content")
	}
	if strings.Contains(out.String(), "removed leftover ai-specs/.recipe/") {
		t.Errorf("claimed to remove .recipe; stdout=%q", out.String())
	}
	if !strings.Contains(errBuf.String(), "already has overrides at ai-specs/recipes/recipeA/overrides/") {
		t.Errorf("stderr = %q, want the existing-destination warning", errBuf.String())
	}
}

// TestRemoveLegacyOriginPartialMultiRecipeNoRemovalOnRetry: one recipe migrates
// fresh while another already has a destination; the retry must not remove
// .recipe/ just because one destination exists.
func TestRemoveLegacyOriginPartialMultiRecipeNoRemovalOnRetry(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	legacy := filepath.Join(root, "ai-specs", ".recipe")
	pcWrite(t, filepath.Join(legacy, "recipeA", "overrides", "a.md"), "a\n", 0o644)
	pcWrite(t, filepath.Join(legacy, "recipeB", "overrides", "b.md"), "b\n", 0o644)
	pcWrite(t, filepath.Join(root, "ai-specs", "recipes", "recipeB", "overrides", "keep.md"), "keep\n", 0o644)

	for i := 1; i <= 2; i++ {
		var out, errBuf bytes.Buffer
		RemoveLegacyOrigin(root, home, &out, &errBuf)
		if !isDir(legacy) {
			t.Fatalf("run %d removed .recipe/ although recipeB has an unverified destination; stdout=%q stderr=%q", i, out.String(), errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "already has overrides at ai-specs/recipes/recipeB/overrides/") {
			t.Errorf("run %d stderr = %q, want recipeB's existing-destination warning", i, errBuf.String())
		}
		if strings.Contains(out.String(), "removed leftover ai-specs/.recipe/") {
			t.Errorf("run %d claimed to remove .recipe/; stdout=%q", i, out.String())
		}
	}
	if got, err := os.ReadFile(filepath.Join(root, "ai-specs", "recipes", "recipeB", "overrides", "keep.md")); err != nil || string(got) != "keep\n" {
		t.Errorf("recipeB destination user content not preserved: %q err=%v", got, err)
	}
	if !isFile(filepath.Join(legacy, "recipeA", "overrides", "a.md")) {
		t.Errorf("legacy source for the fresh-migrated recipe was not retained")
	}
}

// TestRemoveLegacyOriginFreshMigrationRemovesDotRecipe: when every migration
// genuinely succeeds in this invocation, .recipe/ is still removed normally.
func TestRemoveLegacyOriginFreshMigrationRemovesDotRecipe(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	legacy := filepath.Join(root, "ai-specs", ".recipe")
	pcWrite(t, filepath.Join(legacy, "recipeA", "overrides", "a.md"), "a\n", 0o644)
	pcWrite(t, filepath.Join(legacy, "recipeB", "overrides", "b.md"), "b\n", 0o644)

	var out, errBuf bytes.Buffer
	RemoveLegacyOrigin(root, home, &out, &errBuf)
	if isDir(legacy) {
		t.Fatalf(".recipe/ not removed after fully successful migrations; stderr=%q", errBuf.String())
	}
	if !strings.Contains(out.String(), "removed leftover ai-specs/.recipe/") {
		t.Errorf("stdout = %q, want the removal confirmation", out.String())
	}
	if strings.Contains(errBuf.String(), "failed to migrate") || strings.Contains(errBuf.String(), "already has overrides") {
		t.Errorf("unexpected migration warning: %q", errBuf.String())
	}
	if got, err := os.ReadFile(filepath.Join(root, "ai-specs", "recipes", "recipeA", "overrides", "a.md")); err != nil || string(got) != "a\n" {
		t.Errorf("fresh migration content wrong: %q err=%v", got, err)
	}
}

// ============================================================================
// Commit B: full differential against the real Python module
// ============================================================================

type pcSpec struct {
	Mode        string         `json:"mode"`
	CLIAction   string         `json:"cli_action,omitempty"`
	CLIKind     string         `json:"cli_kind,omitempty"`
	CLIDest     string         `json:"cli_dest,omitempty"`
	FN          string         `json:"fn,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
	AISpecsHome string         `json:"ai_specs_home,omitempty"`
	CLIHome     string         `json:"cli_home,omitempty"`
	Root        string         `json:"root"`
	Home        string         `json:"home"`
	Sandbox     string         `json:"sandbox"`
}

type pcRef struct {
	Stdout string            `json:"stdout"`
	Stderr string            `json:"stderr"`
	RC     int               `json:"rc"`
	Files  map[string]string `json:"files"`
	Modes  map[string]int    `json:"modes"`
	Dirs   []string          `json:"dirs"`
	Links  map[string]string `json:"links"`
	Result json.RawMessage   `json:"result"`
	// PythonVersion is sys.version_info[:2] of the interpreter the oracle ran
	// on. Reported with the reference-gap pin, never branched on.
	PythonVersion []int `json:"python_version"`
}

// refDivergence pins the epic-reference gap documented by R3-001: Go mirrors
// the epic's Python reference (3.11-3.13, pathlib's rfind suffix rule) while a
// 3.14+ interpreter changed leading-dot handling. Under <=3.13 `..md` has
// suffix ".md" (the filters remove it; bundled_command_ids reports stem ".");
// the local 3.14 oracle gives suffix "" (keeps the file, reports "..md").
type refDivergence struct {
	// removed lists sandbox-relative files Go removes but Python 3.14 keeps.
	removed []string
	// goIDs/pythonIDs pin the diverging id lists for the ids surface (nil when
	// the divergence is a removed file instead).
	goIDs, pythonIDs []string
}

type pcCase struct {
	name string
	fn   string
	// CLI fields when fn == "".
	cliAction string
	cliKind   *string
	cliDest   string
	args      map[string]any

	seed      func(t *testing.T, root, home string)
	gitAdd    bool
	gitDetach bool
	afterGit  func(t *testing.T, root, home string)

	tracebackStderr bool
	pythonExc       string
	goErrContains   string
	stderrPrefix    string

	// safetyException marks the single user-authorized divergence from the
	// frozen Python oracle: RemoveLegacyOrigin preserves an existing overrides
	// destination (and ai-specs/.recipe/) instead of treating it as proof of a
	// complete migration. The differential still runs both legs; the harness
	// pins the exact divergence rather than requiring byte parity.
	safetyException bool

	// refDivergence pins the user-decided Python reference gap (R3-001): Go
	// follows the epic's <=3.13 pathlib semantics, the local 3.14 oracle
	// changed leading-dot handling. Both legs still run; everything outside the
	// named gap stays byte-strict.
	refDivergence *refDivergence
}

func TestProjectCacheDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	refScript := filepath.Join(repoRoot, "internal", "projectcache", "testdata", "projectcache_ref.py")

	for _, tc := range pcCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			T := t.TempDir()
			t.Cleanup(func() { pcChmodWritable(T) })
			root := filepath.Join(T, "project")
			home := filepath.Join(T, "home")

			spec := pcSpec{
				Mode:        "lib",
				FN:          tc.fn,
				Args:        tc.args,
				AISpecsHome: home,
				CLIHome:     home,
				Root:        root,
				Home:        home,
				Sandbox:     T,
			}
			if tc.fn == "" {
				spec.Mode = "cli"
				spec.CLIAction = tc.cliAction
				spec.CLIDest = tc.cliDest
				if tc.cliKind != nil {
					spec.CLIKind = *tc.cliKind
				}
			}

			// Leg 1: the real Python module (CLI subprocess or importlib call).
			pcSeedLeg(t, T, root, home, tc)
			ref := pcRunRef(t, refScript, spec)

			// Leg 2: the Go port on an identical, freshly reset tree.
			pcSeedLeg(t, T, root, home, tc)
			goOut, goErr, goRC, goResult := pcRunGo(t, spec, root, home)
			goFiles, goModes, goDirs, goLinks := pcSnapshot(t, T)

			if tc.safetyException {
				pcAssertSafetyException(t, ref, goOut, goErr, goFiles, goModes, goDirs, goLinks)
				return
			}

			if tc.refDivergence != nil {
				pcAssertRefDivergence(t, tc, ref, goOut, goErr, goFiles, goModes, goDirs, goLinks, goResult)
				return
			}

			if goOut != ref.Stdout {
				t.Errorf("stdout differs\n--- go ---\n%q\n--- ref ---\n%q", goOut, ref.Stdout)
			}
			if goRC != ref.RC {
				t.Errorf("rc: go=%d ref=%d", goRC, ref.RC)
			}
			if !pcTreeEqual(goFiles, ref.Files) {
				t.Errorf("files differ:\n%s", pcTreeDiff(goFiles, ref.Files))
			}
			if !pcModesEqual(goModes, ref.Modes) {
				t.Errorf("modes differ:\n  go:  %v\n  ref: %v", goModes, ref.Modes)
			}
			if !pcStringsEqual(goDirs, ref.Dirs) {
				t.Errorf("dirs differ:\n  go:  %v\n  ref: %v", goDirs, ref.Dirs)
			}
			if !pcTreeEqual(goLinks, ref.Links) {
				t.Errorf("links differ:\n  go:  %v\n  ref: %v", goLinks, ref.Links)
			}

			var refRes, goRes any
			if len(ref.Result) > 0 {
				_ = json.Unmarshal(ref.Result, &refRes)
			}
			if len(goResult) > 0 {
				_ = json.Unmarshal(goResult, &goRes)
			}
			if !reflect.DeepEqual(refRes, goRes) {
				t.Errorf("result differs: go=%v ref=%v", goRes, refRes)
			}

			if tc.tracebackStderr {
				if !strings.Contains(ref.Stderr, tc.pythonExc) {
					t.Errorf("ref stderr missing %q:\n%s", tc.pythonExc, ref.Stderr)
				}
				lines := strings.Split(strings.TrimSuffix(goErr, "\n"), "\n")
				if len(lines) != 1 || !strings.HasPrefix(lines[0], "error: ") {
					t.Errorf("go stderr is not exactly one `error: ` line: %q", goErr)
				}
				if tc.goErrContains != "" && !strings.Contains(goErr, tc.goErrContains) {
					t.Errorf("go stderr missing %q: %q", tc.goErrContains, goErr)
				}
				return
			}
			if tc.stderrPrefix != "" {
				if !strings.HasPrefix(ref.Stderr, tc.stderrPrefix) {
					t.Errorf("ref stderr missing prefix %q:\n%s", tc.stderrPrefix, ref.Stderr)
				}
				if !strings.HasPrefix(goErr, tc.stderrPrefix) {
					t.Errorf("go stderr missing prefix %q: %q", tc.stderrPrefix, goErr)
				}
				return
			}
			if goErr != ref.Stderr {
				t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
			}
		})
	}
}

// pcSeedLeg resets T and applies the case seed (plus optional git setup).
func pcSeedLeg(t *testing.T, T, root, home string, tc pcCase) {
	t.Helper()
	resetTree(t, T)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if tc.seed != nil {
		tc.seed(t, root, home)
	}
	if tc.gitAdd || tc.gitDetach {
		pcGit(t, root, "init", "-q")
		pcGit(t, root, "add", "-A")
	}
	if tc.gitDetach {
		pcGit(t, root, "-c", "user.name=t", "-c", "user.email=t@x", "commit", "-qm", "x")
		pcGit(t, root, "checkout", "-q", "--detach")
	}
	if tc.afterGit != nil {
		tc.afterGit(t, root, home)
	}
}

// resetTree makes every directory traversable again (a failed-removal case can
// leave a 0500 dir behind) and empties T.
func resetTree(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o755)
		} else {
			_ = os.Chmod(p, 0o644)
		}
		return nil
	})
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			t.Fatalf("reset %s: %v", dir, err)
		}
	}
}

// pcChmodWritable restores traversable permissions so TempDir cleanup can
// remove a tree left read-only by a failed-removal case.
func pcChmodWritable(dir string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
}

func pcGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func pcRunRef(t *testing.T, refScript string, spec pcSpec) pcRef {
	t.Helper()
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	cmd := exec.Command("python3", refScript)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stdin = bytes.NewReader(payload)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("ref driver failed: %v\nstderr: %s", err, errBuf.String())
	}
	var ref pcRef
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
}

// pcRunGo invokes the Go port with the same inputs the driver received.
func pcRunGo(t *testing.T, spec pcSpec, root, home string) (string, string, int, []byte) {
	t.Helper()
	var out, errBuf bytes.Buffer
	var result any
	rc := 0

	if spec.Mode == "cli" {
		t.Setenv("AI_SPECS_HOME", spec.AISpecsHome)
		arg := ""
		switch spec.CLIAction {
		case "path":
			if spec.CLIKind != "" {
				arg = spec.CLIKind
			}
		case "merge-commands":
			if spec.CLIDest != "" {
				arg = filepath.Join(root, spec.CLIDest)
			}
		}
		rc = RenderProjectCache(root, spec.CLIAction, arg, &out, &errBuf)
	} else {
		rc = pcDispatchLib(t, spec, root, spec.CLIHome, &out, &errBuf, &result)
	}

	resultJSON, _ := json.Marshal(result)
	return out.String(), errBuf.String(), rc, resultJSON
}

func pcDispatchLib(t *testing.T, spec pcSpec, root, cliHome string, out, errBuf *bytes.Buffer, result *any) int {
	t.Helper()
	args := spec.Args
	switch spec.FN {
	case "remove_bundled_skill_leftovers":
		RemoveBundledSkillLeftovers(filepath.Join(root, args["ai_specs"].(string)), cliHome,
			pcLockSkills(args), out, errBuf)
	case "remove_bundled_command_leftovers":
		RemoveBundledCommandLeftovers(filepath.Join(root, args["ai_specs"].(string)), cliHome,
			pcLockCommands(args), out, errBuf)
	case "remove_recipe_command_leftovers":
		RemoveRecipeCommandLeftovers(filepath.Join(root, pcArgStr(args, "project_root", "project")), cliHome,
			pcLockCommands(args), pcRecipeSources(args, root), out, errBuf)
	case "bundled_skill_ids":
		*result = BundledSkillIDs(cliHome)
	case "bundled_command_ids":
		*result = BundledCommandIDs(cliHome)
	case "_is_git_work_tree":
		*result = IsGitWorkTree(filepath.Join(root, args["project_root"].(string)))
	case "_git_ls_files":
		*result = GitLsFiles(filepath.Join(root, args["project_root"].(string)), args["pathspec"].(string))
	case "tracked_bundled_skill_leftovers":
		*result = TrackedBundledSkillLeftovers(filepath.Join(root, args["project_root"].(string)), cliHome)
	case "tracked_bundled_command_leftovers":
		*result = TrackedBundledCommandLeftovers(filepath.Join(root, args["project_root"].(string)), cliHome)
	case "format_tracked_bundled_remediation":
		*result = FormatTrackedBundledRemediation(
			pcArgStrs(args, "bundled_ids"), pcArgStr(args, "kind", "skill"),
			pcArgStr(args, "path_template", "ai-specs/skills/{name}"), pcArgBool(args, "recursive", true))
	case "remove_legacy_origin":
		RemoveLegacyOrigin(filepath.Join(root, args["project_root"].(string)), cliHome, out, errBuf)
	case "gate_backup_path":
		*result = GateBackupPath(filepath.Join(root, args["project_root"].(string)),
			args["rel_path"].(string), args["content_sha"].(string), cliHome)
	default:
		t.Fatalf("unknown lib function %q", spec.FN)
	}
	return 0
}

func pcLockSkills(args map[string]any) map[string]map[string]string {
	v, ok := args["lock_skills"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil // "disk" or null
	}
	out := map[string]map[string]string{}
	for sid, fv := range m {
		files := map[string]string{}
		for name, hv := range fv.(map[string]any) {
			files[name] = hv.(string)
		}
		out[sid] = files
	}
	return out
}

func pcLockCommands(args map[string]any) map[string]string {
	v, ok := args["lock_commands"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for name, hv := range m {
		out[name] = hv.(string)
	}
	return out
}

func pcRecipeSources(args map[string]any, root string) map[string]string {
	v, ok := args["recipe_sources"]
	if !ok {
		return nil
	}
	m := v.(map[string]any)
	out := map[string]string{}
	for name, rel := range m {
		out[name] = filepath.Join(root, rel.(string))
	}
	return out
}

func pcArgStr(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func pcArgStrs(args map[string]any, key string) []string {
	v, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = x.(string)
	}
	return out
}

func pcArgBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

var pcCreatedAt = regexp.MustCompile(`created_at = "[^"]*"`)

// pcSnapshot mirrors the reference driver's snapshot: relative files (base64
// with created_at collapsed), mode bits, directory list and symlink targets,
// excluding .git/.
func pcSnapshot(t *testing.T, T string) (map[string]string, map[string]int, []string, map[string]string) {
	t.Helper()
	files := map[string]string{}
	modes := map[string]int{}
	links := map[string]string{}
	dirs := []string{}
	err := filepath.WalkDir(T, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(T, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		key := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			links[key] = target
		case d.IsDir():
			dirs = append(dirs, key)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				// Permission-denied cases keep the file on disk but unreadable;
				// both snapshots mark it identically instead of aborting.
				files[key] = "<unreadable>"
				modes[key] = pcModeBits(info.Mode())
				return nil
			}
			files[key] = base64.StdEncoding.EncodeToString(pcCreatedAt.ReplaceAll(data, []byte(`created_at = "<TIME>"`)))
			modes[key] = pcModeBits(info.Mode())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", T, err)
	}
	sort.Strings(dirs)
	return files, modes, dirs, links
}

func pcModeBits(m os.FileMode) int {
	b := int(m.Perm())
	if m&os.ModeSetuid != 0 {
		b |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		b |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		b |= 0o1000
	}
	return b
}

func pcTreeEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func pcModesEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func pcStringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pcTreeDiff(a, b map[string]string) string {
	var sb strings.Builder
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		if a[k] != b[k] {
			sb.WriteString("  " + k + "\n    go:  " + a[k] + "\n    ref: " + b[k] + "\n")
		}
	}
	return sb.String()
}

// pcAssertSafetyException pins the one authorized parity exception: the real
// Python oracle treats an existing overrides destination as proof of a complete
// migration, deletes ai-specs/.recipe/ after a retry and silently drops the
// entries a failed first run never copied; the Go port preserves .recipe/,
// warns, and leaves the existing destination untouched. Everything outside
// ai-specs/.recipe/ must still match the oracle byte-for-byte.
func pcAssertSafetyException(t *testing.T, ref pcRef, goOut, goErr string, goFiles map[string]string, goModes map[string]int, goDirs []string, goLinks map[string]string) {
	t.Helper()
	const legacy = "project/ai-specs/.recipe"
	if !strings.Contains(ref.Stdout, "removed leftover ai-specs/.recipe/") {
		t.Errorf("expected the Python oracle to remove ai-specs/.recipe/ (the unsafe behavior this exception replaces); stdout=%q", ref.Stdout)
	}
	if strings.Contains(goOut, "removed leftover ai-specs/.recipe/") {
		t.Errorf("Go claimed to remove ai-specs/.recipe/; stdout=%q", goOut)
	}
	if !strings.Contains(goErr, "already has overrides at ai-specs/recipes/") {
		t.Errorf("Go did not warn about the existing destination; stderr=%q", goErr)
	}
	if !strings.Contains(goErr, "skipping removal of ai-specs/.recipe/") {
		t.Errorf("Go did not warn about preserving ai-specs/.recipe/; stderr=%q", goErr)
	}
	if !pcTreeEqual(pcDropLegacy(goFiles), ref.Files) {
		t.Errorf("files outside ai-specs/.recipe/ differ from the oracle:\n%s", pcTreeDiff(pcDropLegacy(goFiles), ref.Files))
	}
	if !pcModesEqual(pcDropLegacy(goModes), ref.Modes) {
		t.Errorf("modes outside ai-specs/.recipe/ differ:\n  go:  %v\n  ref: %v", pcDropLegacy(goModes), ref.Modes)
	}
	if !pcStringsEqual(pcDropLegacyDirs(goDirs), ref.Dirs) {
		t.Errorf("dirs outside ai-specs/.recipe/ differ:\n  go:  %v\n  ref: %v", pcDropLegacyDirs(goDirs), ref.Dirs)
	}
	if !pcTreeEqual(pcDropLegacy(goLinks), ref.Links) {
		t.Errorf("links outside ai-specs/.recipe/ differ:\n  go:  %v\n  ref: %v", pcDropLegacy(goLinks), ref.Links)
	}
	if _, ok := goFiles[legacy+"/recipeB/overrides/ov.md"]; !ok {
		t.Error("Go did not retain the unmigrated legacy overrides under ai-specs/.recipe/")
	}
}

// pcDropLegacy returns m without the ai-specs/.recipe/ subtree.
func pcDropLegacy[T any](m map[string]T) map[string]T {
	const legacy = "project/ai-specs/.recipe"
	out := make(map[string]T, len(m))
	for k, v := range m {
		if k == legacy || strings.HasPrefix(k, legacy+"/") {
			continue
		}
		out[k] = v
	}
	return out
}

// pcDropLegacyDirs returns dirs without the ai-specs/.recipe/ subtree.
func pcDropLegacyDirs(dirs []string) []string {
	const legacy = "project/ai-specs/.recipe"
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == legacy || strings.HasPrefix(d, legacy+"/") {
			continue
		}
		out = append(out, d)
	}
	return out
}

// pcAssertRefDivergence pins the R3-001 reference gap. Both legs ran on the
// same tree; only the leading-dot names in tc.refDivergence may differ. The
// pin records (never branches on) the oracle interpreter version.
func pcAssertRefDivergence(t *testing.T, tc pcCase, ref pcRef, goOut, goErr string, goFiles map[string]string, goModes map[string]int, goDirs []string, goLinks map[string]string, goResult []byte) {
	t.Helper()
	d := tc.refDivergence
	t.Logf("reference-gap pin %q: compared against Python %v and the epic's <=3.13 reference", tc.name, ref.PythonVersion)

	if d.goIDs != nil {
		// ids surface: disk/streams are identical, only the reported stem differs.
		if !pcTreeEqual(goFiles, ref.Files) {
			t.Errorf("files differ outside the reference gap:\n%s", pcTreeDiff(goFiles, ref.Files))
		}
		if !pcModesEqual(goModes, ref.Modes) {
			t.Errorf("modes differ:\n  go:  %v\n  ref: %v", goModes, ref.Modes)
		}
		if !pcStringsEqual(goDirs, ref.Dirs) {
			t.Errorf("dirs differ:\n  go:  %v\n  ref: %v", goDirs, ref.Dirs)
		}
		if !pcTreeEqual(goLinks, ref.Links) {
			t.Errorf("links differ:\n  go:  %v\n  ref: %v", goLinks, ref.Links)
		}
		if goOut != ref.Stdout {
			t.Errorf("stdout differs\n--- go ---\n%q\n--- ref ---\n%q", goOut, ref.Stdout)
		}
		if goErr != ref.Stderr {
			t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
		}
		var goIDs, refIDs []string
		_ = json.Unmarshal(goResult, &goIDs)
		_ = json.Unmarshal(ref.Result, &refIDs)
		if !reflect.DeepEqual(goIDs, d.goIDs) {
			t.Errorf("Go ids = %v, want %v (<=3.13 stem for '..md')", goIDs, d.goIDs)
		}
		if !reflect.DeepEqual(refIDs, d.pythonIDs) {
			t.Errorf("Python %v ids = %v, want %v (3.14 leading-dot stem)", ref.PythonVersion, refIDs, d.pythonIDs)
		}
		return
	}

	// Filter surface: Go removes d.removed under the <=3.13 suffix rule; the
	// local 3.14 oracle keeps those files. Everything else must stay strict.
	for _, p := range d.removed {
		if _, ok := goFiles[p]; ok {
			t.Errorf("Go kept %s; the <=3.13 suffix rule removes it", p)
		}
		if _, ok := ref.Files[p]; !ok {
			t.Errorf("Python %v did not keep %s; this pin targets the 3.14 leading-dot behavior", ref.PythonVersion, p)
		}
		if !strings.Contains(goOut, filepath.Base(p)) {
			t.Errorf("Go removed %s but did not report it; stdout=%q", p, goOut)
		}
		if strings.Contains(ref.Stdout, filepath.Base(p)) {
			t.Errorf("Python %v reported removing %s; the 3.14 oracle keeps it", ref.PythonVersion, p)
		}
	}
	if !pcTreeEqual(goFiles, pcDropPaths(ref.Files, d.removed)) {
		t.Errorf("files outside the reference gap differ:\n%s", pcTreeDiff(goFiles, pcDropPaths(ref.Files, d.removed)))
	}
	if !pcModesEqual(goModes, pcDropPaths(ref.Modes, d.removed)) {
		t.Errorf("modes outside the reference gap differ:\n  go:  %v\n  ref: %v", goModes, pcDropPaths(ref.Modes, d.removed))
	}
	if !pcStringsEqual(goDirs, ref.Dirs) {
		t.Errorf("dirs differ:\n  go:  %v\n  ref: %v", goDirs, ref.Dirs)
	}
	if !pcTreeEqual(goLinks, ref.Links) {
		t.Errorf("links differ:\n  go:  %v\n  ref: %v", goLinks, ref.Links)
	}
	if got, want := pcStripLinesWithBase(goOut, d.removed), pcStripLinesWithBase(ref.Stdout, d.removed); got != want {
		t.Errorf("stdout outside the reference gap differs\n--- go ---\n%q\n--- ref ---\n%q", got, want)
	}
	if goErr != ref.Stderr {
		t.Errorf("stderr differs\n--- go ---\n%q\n--- ref ---\n%q", goErr, ref.Stderr)
	}
}

// pcDropPaths returns m without the exact named keys.
func pcDropPaths[T any](m map[string]T, paths []string) map[string]T {
	drop := make(map[string]bool, len(paths))
	for _, p := range paths {
		drop[p] = true
	}
	out := make(map[string]T, len(m))
	for k, v := range m {
		if !drop[k] {
			out[k] = v
		}
	}
	return out
}

// pcStripLinesWithBase drops lines mentioning the base name of any path.
func pcStripLinesWithBase(s string, paths []string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		drop := false
		for _, p := range paths {
			if strings.Contains(line, filepath.Base(p)) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// ============================================================================
// Differential cases
// ============================================================================

func pcWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func pcStr(s string) *string { return &s }

// pcSeedBundled writes a small bundled-skills/commands surface.
func pcSeedBundled(t *testing.T, home string) {
	pcWrite(t, filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
	pcWrite(t, filepath.Join(home, "bundled-skills", "beta", "SKILL.md"), "# beta\n", 0o644)
	pcWrite(t, filepath.Join(home, "bundled-commands", "cmd1.md"), "# cmd1\n", 0o644)
}

func pcCases(t *testing.T) []pcCase {
	shaOld := sha256Hex([]byte("# old alpha\n"))

	return []pcCase{
		// ---- CLI: path kinds ----
		{name: "cli path root", cliAction: "path", cliKind: pcStr("root")},
		{name: "cli path recipe", cliAction: "path", cliKind: pcStr("recipe")},
		{name: "cli path deps", cliAction: "path", cliKind: pcStr("deps")},
		{name: "cli path bundled", cliAction: "path", cliKind: pcStr("bundled")},
		{name: "cli path commands", cliAction: "path", cliKind: pcStr("commands")},
		{name: "cli path resolved-skills", cliAction: "path", cliKind: pcStr("resolved-skills")},
		{name: "cli path unknown kind", cliAction: "path", cliKind: pcStr("nope")},
		{name: "cli path missing kind", cliAction: "path"},
		{name: "cli unknown action", cliAction: "bogus"},

		// ---- CLI: ensure ----
		{name: "cli ensure fresh", cliAction: "ensure"},
		{name: "cli ensure refresh stale root", cliAction: "ensure",
			seed: func(t *testing.T, root, home string) {
				meta := filepath.Join(CacheRoot(root, home), "meta.toml")
				pcWrite(t, meta, "project_root = \"/old\"\nfoo = 1\nproject_rootish = 2\n", 0o644)
			}},
		{name: "cli ensure blocked parent", cliAction: "ensure",
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "cache"), "block", 0o644)
			},
			tracebackStderr: true, pythonExc: "RuntimeError"},

		// ---- CLI: merge-commands ----
		{name: "cli merge fresh dest", cliAction: "merge-commands", cliDest: "out",
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(home, "bundled-commands", "cmd1.md"), "# cmd1\n", 0o755)
				pcWrite(t, filepath.Join(CacheRoot(root, home), "commands", "cmd2.md"), "# recipe cmd2\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "cmd2.md"), "# local cmd2\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "local.md"), "# local\n", 0o644)
			}},
		{name: "cli merge existing dest", cliAction: "merge-commands", cliDest: "out",
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "out", "stale.md"), "stale\n", 0o644)
				if err := os.MkdirAll(filepath.Join(root, "out", "nested"), 0o755); err != nil {
					t.Fatal(err)
				}
				pcWrite(t, filepath.Join(root, "out", "nested", "deep.md"), "deep\n", 0o644)
			}},
		{name: "cli merge missing dest", cliAction: "merge-commands"},
		{name: "cli merge symlink dest rmtree refuses", cliAction: "merge-commands", cliDest: "linkdest",
			seed: func(t *testing.T, root, home string) {
				if err := os.MkdirAll(filepath.Join(root, "realdest"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "realdest"), filepath.Join(root, "linkdest")); err != nil {
					t.Fatal(err)
				}
			},
			tracebackStderr: true, pythonExc: "OSError"},
		{name: "cli merge regular-file dest rmtree refuses", cliAction: "merge-commands", cliDest: "destfile",
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(root, "destfile"), "keep\n", 0o644)
			},
			tracebackStderr: true, pythonExc: "Error"},
		{name: "cli merge unreadable command aborts", cliAction: "merge-commands", cliDest: "out",
			seed: func(t *testing.T, root, home string) {
				cache := CacheRoot(root, home)
				pcWrite(t, filepath.Join(cache, ".bundled", "commands", "a.md"), "# a\n", 0o644)
				pcWrite(t, filepath.Join(cache, ".bundled", "commands", "b.md"), "# b\n", 0o000)
				pcWrite(t, filepath.Join(cache, "commands", "c.md"), "# c\n", 0o644)
			},
			tracebackStderr: true, pythonExc: "PermissionError"},
		{name: "cli merge bundled managed duplicate", cliAction: "merge-commands", cliDest: "out",
			seed: func(t *testing.T, root, home string) {
				cache := CacheRoot(root, home)
				pcWrite(t, filepath.Join(cache, ".bundled", "commands", "dup.md"), "# bundled\n", 0o644)
				pcWrite(t, filepath.Join(cache, "commands", "dup.md"), "# managed\n", 0o644)
			}},
		{name: "cli merge preserves source modes", cliAction: "merge-commands", cliDest: "out",
			seed: func(t *testing.T, root, home string) {
				cache := CacheRoot(root, home)
				pcWrite(t, filepath.Join(cache, ".bundled", "commands", "script.md"), "#!/bin/sh\n", 0o755)
				pcWrite(t, filepath.Join(cache, ".bundled", "commands", "ro.md"), "ro\n", 0o444)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "local.md"), "local\n", 0o600)
			}},

		// ---- remove_bundled_skill_leftovers ----
		{name: "remove bundled skill matches", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "beta", "SKILL.md"), "# edited\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "local", "SKILL.md"), "# local\n", 0o644)
			}},
		{name: "remove bundled skill lock hash", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# old alpha\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "beta", "SKILL.md"), "# beta\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
					"[skills.\"alpha\"]\n\"SKILL.md\" = \""+shaOld+"\"\n", 0o644)
			}},
		{name: "remove bundled skill malformed lock", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "beta", "SKILL.md"), "# edited\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"), "[skills.\n", 0o644)
			}},
		{name: "remove bundled skill git tracked", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
			},
			gitAdd: true},
		{name: "remove bundled skill oserror keeps dir", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				if err := os.Chmod(filepath.Join(root, "ai-specs", "skills", "alpha"), 0o500); err != nil {
					t.Fatal(err)
				}
			},
			stderrPrefix: "  ! failed to remove leftover ai-specs/skills/alpha/: "},
		{name: "remove bundled skill symlink dir kept", fn: "remove_bundled_skill_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				real := filepath.Join(root, "realalpha")
				pcWrite(t, filepath.Join(real, "SKILL.md"), "# alpha\n", 0o644)
				if err := os.MkdirAll(filepath.Join(root, "ai-specs", "skills"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, filepath.Join(root, "ai-specs", "skills", "alpha")); err != nil {
					t.Fatal(err)
				}
			},
			stderrPrefix: "  ! failed to remove leftover ai-specs/skills/alpha/: "},

		// ---- remove_bundled_command_leftovers ----
		{name: "remove bundled command matches", fn: "remove_bundled_command_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "cmd1.md"), "# cmd1\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "user.md"), "# user\n", 0o644)
			}},
		{name: "remove bundled command lock hash", fn: "remove_bundled_command_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "cmd1.md"), "# old cmd1\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
					"[commands]\n\"cmd1.md\" = \""+sha256Hex([]byte("# old cmd1\n"))+"\"\n", 0o644)
			},
		},
		{name: "remove bundled command suffix filter", fn: "remove_bundled_command_leftovers",
			args: map[string]any{"ai_specs": "ai-specs"},
			// R3-001: Go follows the epic's <=3.13 suffix rule, so '..md' has
			// suffix ".md" and is removed; the local 3.14 oracle keeps it.
			refDivergence: &refDivergence{removed: []string{"project/ai-specs/commands/..md"}},
			seed: func(t *testing.T, root, home string) {
				for _, n := range []string{".md", "..md", ".hidden.md", "a.md.bak"} {
					pcWrite(t, filepath.Join(home, "bundled-commands", n), "# "+n+"\n", 0o644)
					pcWrite(t, filepath.Join(root, "ai-specs", "commands", n), "# "+n+"\n", 0o644)
				}
			}},

		// ---- remove_recipe_command_leftovers ----
		{name: "remove recipe command leftovers", fn: "remove_recipe_command_leftovers",
			args: map[string]any{
				"project_root":   ".",
				"lock_commands":  map[string]any{"locked.md": sha256Hex([]byte("# locked\n"))},
				"recipe_sources": map[string]any{"src.md": "catalog/src.md"},
			},
			seed: func(t *testing.T, root, home string) {
				cache := CacheRoot(root, home)
				pcWrite(t, filepath.Join(cache, "commands", "managed.md"), "# managed\n", 0o644)
				pcWrite(t, filepath.Join(cache, "commands", "edited.md"), "# managed edited\n", 0o644)
				cmdDir := filepath.Join(root, "ai-specs", "commands")
				pcWrite(t, filepath.Join(cmdDir, "managed.md"), "# managed\n", 0o644)
				pcWrite(t, filepath.Join(cmdDir, "edited.md"), "# user edited\n", 0o644)
				pcWrite(t, filepath.Join(cmdDir, "locked.md"), "# locked\n", 0o644)
				pcWrite(t, filepath.Join(cmdDir, "src.md"), "# recipe source\n", 0o644)
				pcWrite(t, filepath.Join(cmdDir, "keep.md"), "# local keep\n", 0o644)
				pcWrite(t, filepath.Join(root, "catalog", "src.md"), "# recipe source\n", 0o644)
			}},
		{name: "remove recipe command suffix filter", fn: "remove_recipe_command_leftovers",
			args: map[string]any{"project_root": "."},
			// R3-001: same <=3.13 vs 3.14 gap; the cached '..md' provenance
			// matches so Go removes the project copy, Python 3.14 keeps it.
			refDivergence: &refDivergence{removed: []string{"project/ai-specs/commands/..md"}},
			seed: func(t *testing.T, root, home string) {
				cache := CacheRoot(root, home)
				for _, n := range []string{".md", "..md", ".hidden.md", "a.md.bak"} {
					pcWrite(t, filepath.Join(cache, "commands", n), "# "+n+"\n", 0o644)
					pcWrite(t, filepath.Join(root, "ai-specs", "commands", n), "# "+n+"\n", 0o644)
				}
			}},

		// ---- bundled id listings ----
		{name: "bundled skill ids", fn: "bundled_skill_ids",
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), "# a\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-skills", "zeta", "SKILL.md"), "# z\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-skills", "gamma", "README.md"), "# g\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-skills", "note.txt"), "n\n", 0o644)
			}},
		{name: "bundled command ids", fn: "bundled_command_ids",
			// R3-001: '..md' reports stem "." under the <=3.13 rule, "..md"
			// under the local 3.14 oracle; everything else is identical.
			refDivergence: &refDivergence{
				goIDs:     []string{".", ".hidden", "cmd1", "x.tar"},
				pythonIDs: []string{"..md", ".hidden", "cmd1", "x.tar"},
			},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "bundled-commands", "cmd1.md"), "# 1\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-commands", "x.tar.md"), "# 2\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-commands", ".hidden.md"), "# 3\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-commands", "..md"), "# 4\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-commands", "README.txt"), "# 5\n", 0o644)
				if err := os.MkdirAll(filepath.Join(home, "bundled-commands", "dir.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			}},

		// ---- git surface ----
		{name: "is git work tree true", fn: "_is_git_work_tree",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(root, "README.md"), "# r\n", 0o644)
			}, gitAdd: true},
		{name: "is git work tree false", fn: "_is_git_work_tree",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) { pcWrite(t, filepath.Join(root, "README.md"), "# r\n", 0o644) }},
		{name: "git ls files", fn: "_git_ls_files",
			args: map[string]any{"project_root": ".", "pathspec": "ai-specs/skills/alpha"},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# a\n", 0o644)
				pcWrite(t, filepath.Join(root, "other.txt"), "o\n", 0o644)
			}, gitAdd: true},
		{name: "tracked skill leftovers", fn: "tracked_bundled_skill_leftovers",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), "# a\n", 0o644)
				pcWrite(t, filepath.Join(home, "bundled-skills", "beta", "SKILL.md"), "# b\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md"), "# a\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "skills", "beta", "SKILL.md"), "# b\n", 0o644)
			},
			gitAdd: true,
			afterGit: func(t *testing.T, root, home string) {
				if err := os.Remove(filepath.Join(root, "ai-specs", "skills", "alpha", "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "tracked command leftovers detached", fn: "tracked_bundled_command_leftovers",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "bundled-commands", "cmd1.md"), "# c\n", 0o644)
				pcWrite(t, filepath.Join(root, "ai-specs", "commands", "cmd1.md"), "# c\n", 0o644)
			},
			gitAdd: true, gitDetach: true,
			afterGit: func(t *testing.T, root, home string) {
				if err := os.Remove(filepath.Join(root, "ai-specs", "commands", "cmd1.md")); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "tracked skill leftovers non-git", fn: "tracked_bundled_skill_leftovers",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				pcWrite(t, filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), "# a\n", 0o644)
			}},

		// ---- remediation text ----
		{name: "remediation skill", fn: "format_tracked_bundled_remediation",
			args: map[string]any{"bundled_ids": []any{"a", "b"}}},
		{name: "remediation command", fn: "format_tracked_bundled_remediation",
			args: map[string]any{"bundled_ids": []any{"a", "b"}, "kind": "command",
				"path_template": "ai-specs/commands/{name}.md", "recursive": false}},

		// ---- remove_legacy_origin ----
		// SAFETY EXCEPTION (user-authorized, GO-07.S6): the only differential case
		// that exercises an EXISTING overrides destination. Python deletes
		// ai-specs/.recipe/ after the retry; Go preserves it. The harness asserts
		// that exact divergence (see pcAssertSafetyException) and keeps every
		// byte outside ai-specs/.recipe/ strict.
		{name: "remove legacy origin", fn: "remove_legacy_origin",
			args:            map[string]any{"project_root": "."},
			safetyException: true,
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				ai := filepath.Join(root, "ai-specs")
				pcWrite(t, filepath.Join(ai, ".recipe", "recipeA", "overrides", "ov.md"), "# ov\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".recipe", "recipeB", "overrides", "ov.md"), "# ov b\n", 0o644)
				pcWrite(t, filepath.Join(ai, "recipes", "recipeB", "overrides", "keep.md"), "# keep\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".resolved-skills", "x", "SKILL.md"), "# x\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".internal", "y", "z"), "# z\n", 0o644)
				pcWrite(t, filepath.Join(ai, "bin", "premerge_guardian.py"), "# stale\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".deps", "dep", "marker"), "# d\n", 0o644)
				pcWrite(t, filepath.Join(ai, "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				pcWrite(t, filepath.Join(ai, "skills", "beta", "SKILL.md"), "# edited\n", 0o644)
			}},
		// Strict parity: the same surface with NO pre-existing destination, so
		// every migration genuinely succeeds and .recipe/ is removed normally.
		{name: "remove legacy origin fresh migration", fn: "remove_legacy_origin",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				pcSeedBundled(t, home)
				ai := filepath.Join(root, "ai-specs")
				pcWrite(t, filepath.Join(ai, ".recipe", "recipeA", "overrides", "ov.md"), "# ov\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".recipe", "recipeB", "overrides", "ov.md"), "# ov b\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".resolved-skills", "x", "SKILL.md"), "# x\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".internal", "y", "z"), "# z\n", 0o644)
				pcWrite(t, filepath.Join(ai, "bin", "premerge_guardian.py"), "# stale\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".deps", "dep", "marker"), "# d\n", 0o644)
				pcWrite(t, filepath.Join(ai, "skills", "alpha", "SKILL.md"), "# alpha\n", 0o644)
				pcWrite(t, filepath.Join(ai, "skills", "beta", "SKILL.md"), "# edited\n", 0o644)
			}},

		{name: "remove legacy origin symlink dir kept", fn: "remove_legacy_origin",
			args: map[string]any{"project_root": "project"},
			seed: func(t *testing.T, root, home string) {
				real := filepath.Join(root, "realrs")
				if err := os.MkdirAll(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(root, "project", "ai-specs"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, filepath.Join(root, "project", "ai-specs", ".resolved-skills")); err != nil {
					t.Fatal(err)
				}
			},
			stderrPrefix: "  ! failed to remove leftover ai-specs/.resolved-skills/: "},
		{name: "remove legacy origin symlinks resolved", fn: "remove_legacy_origin",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				ov := filepath.Join(root, "ai-specs", ".recipe", "recipeA", "overrides")
				pcWrite(t, filepath.Join(ov, "real.md"), "# real\n", 0o644)
				pcWrite(t, filepath.Join(ov, "sub", "inner.md"), "# inner\n", 0o755)
				mustSymlink(t, "real.md", filepath.Join(ov, "filelink.md"))
				mustSymlink(t, "sub", filepath.Join(ov, "dirlink"))
			}},
		{name: "remove legacy origin dangling symlink warns", fn: "remove_legacy_origin",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				ai := filepath.Join(root, "ai-specs")
				for _, rec := range []string{"recipeA", "recipeB"} {
					ov := filepath.Join(ai, ".recipe", rec, "overrides")
					pcWrite(t, filepath.Join(ov, "ok.md"), "# ok "+rec+"\n", 0o644)
					mustSymlink(t, filepath.Join(ai, "missing", "nope"), filepath.Join(ov, "dangling"))
				}
			},
			stderrPrefix: "  ! failed to migrate overrides for 'recipeA': "},
		{name: "remove legacy origin regular-file leftovers", fn: "remove_legacy_origin",
			args: map[string]any{"project_root": "."},
			seed: func(t *testing.T, root, home string) {
				ai := filepath.Join(root, "ai-specs")
				pcWrite(t, filepath.Join(ai, ".recipe"), "file\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".resolved-skills"), "file\n", 0o644)
				pcWrite(t, filepath.Join(ai, ".internal"), "file\n", 0o644)
			},
			stderrPrefix: "  ! failed to remove leftover ai-specs/.recipe/: "},

		// ---- gate_backup_path ----
		{name: "gate backup path", fn: "gate_backup_path",
			args: map[string]any{"project_root": "project", "rel_path": "ai-specs/hooks/gate.sh",
				"content_sha": "deadbeef"}},
	}
}
