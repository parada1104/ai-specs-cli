package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

func parseTOML(t *testing.T, src string) *toml.Table {
	t.Helper()
	table, err := toml.Parse([]byte(src))
	if err != nil {
		t.Fatalf("toml.Parse(%q): %v", src, err)
	}
	return table
}

func assertChecks(t *testing.T, d *Doctor, want []Check) {
	t.Helper()
	if len(d.Checks) != len(want) {
		t.Fatalf("got %d checks %+v, want %d %+v", len(d.Checks), d.Checks, len(want), want)
	}
	for i := range want {
		if d.Checks[i] != want[i] {
			t.Errorf("check %d = %+v, want %+v", i, d.Checks[i], want[i])
		}
	}
}

func writeManifest(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// TestCheckRender pins the frozen report line format (Check.render).
func TestCheckRender(t *testing.T) {
	cases := []struct {
		name  string
		check Check
		want  string
	}{
		{
			name:  "guidance",
			check: Check{INFO, "cli-version", "hello", "fix it"},
			want:  "INFO   cli-version      hello  (fix it)",
		},
		{
			name:  "no-guidance",
			check: Check{ERROR, "manifest", "missing", ""},
			want:  "ERROR  manifest         missing",
		},
		{
			name:  "ok-severity",
			check: Check{OK, "bundled-skill", "cache .bundled/skills/x present", ""},
			want:  "OK     bundled-skill    cache .bundled/skills/x present",
		},
		{
			name:  "name-longer-than-padding",
			check: Check{WARN, "tracked-bundled-leftover", "2 removed", ""},
			want:  "WARN   tracked-bundled-leftover  2 removed",
		},
	}
	for _, tc := range cases {
		if got := tc.check.Render(); got != tc.want {
			t.Errorf("%s: Render() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestReportSkeleton pins the frozen report framing and summary line.
func TestReportSkeleton(t *testing.T) {
	d := &Doctor{Root: "/target"}
	d.add(OK, "manifest", "ai-specs/ai-specs.toml found")
	d.add(WARN, "agents-md", "warned", "fix")
	d.add(INFO, "cli-version", "noted")
	d.add(ERROR, "bundled-skill", "broken", "sync")
	var sb strings.Builder
	d.Report(&sb)
	want := "\n" +
		"ai-specs doctor\n" +
		"  target: /target\n" +
		"\n" +
		"  OK     manifest         ai-specs/ai-specs.toml found\n" +
		"  WARN   agents-md        warned  (fix)\n" +
		"  INFO   cli-version      noted\n" +
		"  ERROR  bundled-skill    broken  (sync)\n" +
		"\n" +
		"Summary: 1 OK, 1 INFO, 1 WARN, 1 ERROR\n"
	if got := sb.String(); got != want {
		t.Errorf("Report() =\n%q\nwant\n%q", got, want)
	}
}

// TestExitCodeRule pins the frozen exit rule: 1 iff at least one ERROR.
func TestExitCodeRule(t *testing.T) {
	cases := []struct {
		name string
		sevs []Severity
		want int
	}{
		{"no-checks", nil, 0},
		{"ok-only", []Severity{OK}, 0},
		{"info-and-warn", []Severity{INFO, WARN, INFO}, 0},
		{"one-error", []Severity{OK, ERROR, WARN}, 1},
	}
	for _, tc := range cases {
		d := &Doctor{Root: "/nowhere", Home: "/nowhere"}
		for _, sev := range tc.sevs {
			d.add(sev, "check", "message")
		}
		if got := d.exitCode(); got != tc.want {
			t.Errorf("%s: exitCode() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestBriefRenderEnabled mirrors brief_render_enabled, including the identity
// semantics of `render is False` and the ValueError text for other types.
func TestBriefRenderEnabled(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     bool
		wantErr  string
	}{
		{"no-brief-table", "project = { name = \"x\" }\n", true, ""},
		{"render-missing", "[brief]\n", true, ""},
		{"render-true", "[brief]\nrender = true\n", true, ""},
		{"render-false", "[brief]\nrender = false\n", false, ""},
		{"render-string", "[brief]\nrender = \"yes\"\n", false, "got str"},
		{"render-int-zero", "[brief]\nrender = 0\n", false, "got int"},
		{"render-float", "[brief]\nrender = 1.0\n", false, "got float"},
		{"render-list", "[brief]\nrender = [1]\n", false, "got list"},
		{"brief-not-a-table", "brief = 5\n", true, ""},
		{"brief-is-a-list", "brief = [1]\n", true, ""},
		{"brief-empty-inline", "brief = {}\n", true, ""},
	}
	for _, tc := range cases {
		got, err := briefRenderEnabled(parseTOML(t, tc.manifest))
		if got != tc.want {
			t.Errorf("%s: enabled = %v, want %v", tc.name, got, tc.want)
		}
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}
}

// TestBriefRenderDisabledTreatsInvalidAsEnabled pins _brief_render_disabled's
// ValueError branch.
func TestBriefRenderDisabledTreatsInvalidAsEnabled(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[brief]\nrender = \"nope\"\n")
	d := New(root, t.TempDir())
	if d.briefRenderDisabled() {
		t.Error("an invalid render type must count as render enabled")
	}
}

// TestCheckManifestStates covers the three _check_manifest outcomes.
func TestCheckManifestStates(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		d := New(t.TempDir(), t.TempDir())
		d.checkManifest()
		assertChecks(t, d, []Check{
			{ERROR, "manifest", "ai-specs/ai-specs.toml missing", "run ai-specs init"},
		})
	})
	t.Run("found", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"x\"\n")
		d := New(root, t.TempDir())
		d.checkManifest()
		// The message stays project-relative even though the target resolves
		// through a symlink (/var -> /private/var on macOS).
		assertChecks(t, d, []Check{
			{OK, "manifest", "ai-specs/ai-specs.toml found", ""},
		})
	})
	t.Run("unparseable", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project\nname = \"x\"\n")
		d := New(root, t.TempDir())
		d.checkManifest()
		// _check_manifest reports the file as found first, then the parse error.
		if len(d.Checks) != 2 {
			t.Fatalf("checks = %+v", d.Checks)
		}
		if d.Checks[0] != (Check{OK, "manifest", "ai-specs/ai-specs.toml found", ""}) {
			t.Errorf("first check = %+v", d.Checks[0])
		}
		c := d.Checks[1]
		if c.Severity != ERROR || c.Name != "manifest" || !strings.HasSuffix(c.Message, "is not parseable") {
			t.Errorf("check = %+v", c)
		}
	})
}

// TestLegacyRecipeVersions pins the "legacy version= key" predicate.
func TestLegacyRecipeVersions(t *testing.T) {
	table := parseTOML(t, strings.Join([]string{
		"[recipes.alpha]",
		"version = \"1.0.0\"",
		"[recipes.beta]",
		"version = \"\"",
		"[recipes.gamma]",
		"other = true",
		"[recipes.delta]",
		"version = false",
	}, "\n")+"\n")
	got := legacyRecipeVersions(table)
	want := []string{"alpha", "delta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("legacyRecipeVersions() = %v, want %v", got, want)
	}
	if got := legacyRecipeVersions(nil); got != nil {
		t.Errorf("legacyRecipeVersions(nil) = %v, want nil", got)
	}
	if got := legacyRecipeVersions(parseTOML(t, "recipes = 5\n")); got != nil {
		t.Errorf("legacyRecipeVersions(non-table) = %v, want nil", got)
	}
}

// TestCompareVersions pins the semver comparison, including the None branches.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0+build", "1.0.0+build", 0},
		{"1.0.0-rc.1", "1.0.0-rc.1", 0},
		{"unknown", "1.0.0", -1},
		{"1.0.0", "unknown", 1},
		{"unknown", "unknown", 0},
		{"junk", "1.0.0", -1},
		{"junk", "garbage", -1},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.left, tc.right); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}

// TestCachePathsLayout pins the frozen cache layout doctor inspects.
func TestCachePathsLayout(t *testing.T) {
	d := New("/tmp/example-project", "/tmp/example-home")
	key := cacheKey(d.Root)
	if !strings.HasSuffix(d.cacheRoot(), filepath.Join("cache", "projects", key)) {
		t.Errorf("cacheRoot() = %q, want suffix %q", d.cacheRoot(), filepath.Join("cache", "projects", key))
	}
	if got, want := d.commandsDir(), filepath.Join(d.cacheRoot(), "commands"); got != want {
		t.Errorf("commandsDir() = %q, want %q", got, want)
	}
	if got, want := d.bundledSkillsRoot(), filepath.Join(d.cacheRoot(), ".bundled"); got != want {
		t.Errorf("bundledSkillsRoot() = %q, want %q", got, want)
	}
	if got, want := d.bundledCommandsRoot(), filepath.Join(d.cacheRoot(), ".bundled", "commands"); got != want {
		t.Errorf("bundledCommandsRoot() = %q, want %q", got, want)
	}
	if got, want := d.resolvedSkillsDir(), filepath.Join(d.cacheRoot(), "resolved-skills"); got != want {
		t.Errorf("resolvedSkillsDir() = %q, want %q", got, want)
	}
}

// TestBundledAssetNames pins which bundled assets the diagnostics iterate.
func TestBundledAssetNames(t *testing.T) {
	wantSkills := []string{"harness-lifecycle", "harness-recipes", "harness-skills-deps", "skill-creator", "skill-sync"}
	if got := bundledSkillNames(); strings.Join(got, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("bundledSkillNames() = %v, want %v", got, wantSkills)
	}
	if got := bundledSkillIDs(); strings.Join(got, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("bundledSkillIDs() = %v, want %v", got, wantSkills)
	}
	wantCommands := []string{"rules-audit", "skills-as-rules"}
	if got := bundledCommandNames(); strings.Join(got, ",") != strings.Join(wantCommands, ",") {
		t.Errorf("bundledCommandNames() = %v, want %v", got, wantCommands)
	}
}

// TestBundledAssetsFromCache pins the cache-miss path of _check_bundled_assets.
func TestBundledAssetsFromCache(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	d := New(root, home)
	d.checkBundledAssets()
	var errors, oks int
	for _, c := range d.Checks {
		switch c.Severity {
		case OK:
			oks++
		case ERROR:
			errors++
		}
	}
	if errors != 5+2 || oks != 0 {
		t.Fatalf("checks = %+v", d.Checks)
	}
	if got := d.Checks[0].Message; got != "cache .bundled/skills/harness-lifecycle missing" {
		t.Errorf("first message = %q", got)
	}
	// Flatten the assets into the cache and re-run: everything resolves.
	skillsRoot := filepath.Join(d.bundledSkillsRoot(), "skills")
	for _, skill := range bundledSkillNames() {
		if err := os.MkdirAll(filepath.Join(skillsRoot, skill), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	commandsRoot := d.bundledCommandsRoot()
	if err := os.MkdirAll(commandsRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, command := range bundledCommandNames() {
		if err := os.WriteFile(filepath.Join(commandsRoot, command+".md"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	d2 := New(root, home)
	d2.checkBundledAssets()
	for _, c := range d2.Checks {
		if c.Severity != OK {
			t.Errorf("check = %+v", c)
		}
	}
}
