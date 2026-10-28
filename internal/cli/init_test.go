package cli

// The native `init` verb's contract, pinned rule by rule (odd/tasks/go-20-u4-init.md
// T1, T2, T5, T8). The end-to-end differential against the legacy script lives in
// tests/test_init_native.py; these tests own the pieces whose failure the black box
// would report only as "the tree differs".

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/tui"
)

func runInitArgs(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errW bytes.Buffer
	rc := runInit(append([]string{"init"}, args...), t.TempDir(), strings.NewReader(stdin), &out, &errW, "")
	return rc, out.String(), errW.String()
}

// TestInitArgumentContract pins the legacy parser: the flags, the two usage
// errors with their exact messages and exit code 2, and `--` discarding the
// rest of the line (init.sh `break`s).
func TestInitArgumentContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		args       []string
		wantRC     int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantRC:     2,
			wantStderr: "ERROR: unknown flag: --bogus\nRun 'ai-specs init --help' for usage.\n",
		},
		{
			name:       "extra positional",
			args:       []string{t.TempDir(), "second"},
			wantRC:     2,
			wantStderr: "ERROR: unexpected positional argument: second\n",
		},
		{
			name:       "trailing --name has no value",
			args:       []string{"--name"},
			wantRC:     1,
			wantStderr: errInitMissingNameValue,
		},
		{
			name:       "bare dash is an unknown flag",
			args:       []string{"-"},
			wantRC:     2,
			wantStderr: "ERROR: unknown flag: -\nRun 'ai-specs init --help' for usage.\n",
		},
		{
			name:       "help wins",
			args:       []string{"--help", "--bogus"},
			wantRC:     0,
			wantStdout: initUsage,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc, stdout, stderr := runInitArgs(t, "", tc.args...)
			if rc != tc.wantRC {
				t.Errorf("exit = %d, want %d", rc, tc.wantRC)
			}
			if tc.wantStdout != "" && stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.wantStdout)
			}
			if stderr != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantStderr)
			}
		})
	}

	// `--name=` with an empty value IS a value: the shell takes `${1#*=}`.
	opts, rc, done := parseInitArgs([]string{"init", "--name=", "/p"}, &bytes.Buffer{}, &bytes.Buffer{})
	if done || rc != 0 || opts.name != "" || !opts.nameExplicit || opts.target != "/p" {
		t.Errorf("parseInitArgs(--name= /p) = (%+v, %d, done=%v), want an explicit empty name and target /p", opts, rc, done)
	}

	// The shell `break`s at `--`: everything after it is dropped, so a flag
	// there is never parsed and never becomes a usage error.
	opts, rc, done = parseInitArgs([]string{"init", "--", "--bogus"}, &bytes.Buffer{}, &bytes.Buffer{})
	if done || rc != 0 {
		t.Fatalf("parseInitArgs(-- --bogus) = (%+v, %d, done=%v), want a parsed target-free run", opts, rc, done)
	}
	if opts.target != "" || opts.name != "" || opts.force || opts.adoptBrief {
		t.Errorf("opts = %+v, want every field at its default (the tail is discarded)", opts)
	}
}

// TestInitTargetRules pins the target errors and the name derived from the
// basename (init.sh L109-120).
func TestInitTargetRules(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "nope")
	rc, _, stderr := runInitArgs(t, "", missing)
	if rc != 1 {
		t.Errorf("exit = %d, want 1", rc)
	}
	want := "ERROR: target path does not exist or is not a directory: " + missing + "\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}

	dir := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, rc := resolveInitTarget(dir, &bytes.Buffer{}); rc != 0 || got != dir {
		t.Errorf("resolveInitTarget = (%q, %d), want (%q, 0)", got, rc, dir)
	}
	// The derived name is asserted through the verb, not through filepath.Base
	// on a literal this test built itself (JD-A-006): a run with no --name must
	// bake the directory's basename into the manifest. The home carries a stub
	// refresh-bundled so the run reaches its end without a real CLI install.
	home := t.TempDir()
	stubDir := filepath.Join(home, "lib", "_internal")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "refresh-bundled.py"), []byte("import sys\nsys.exit(0)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := runInit([]string{"init", dir, "--no-tui"}, home, strings.NewReader(""), &out, &errW, ""); rc != 0 {
		t.Fatalf("runInit = %d, want 0; stderr=%q", rc, errW.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `name = "my-app"`) {
		t.Errorf("manifest does not carry the basename:\n%s", firstLines(string(body), 30))
	}
}

// TestInitManifestIsNeverOverwritten pins S4: the manifest is user-owned, so a
// second run keeps it byte for byte whatever the name says.
func TestInitManifestIsNeverOverwritten(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.WriteFile(path, []byte("name = \"hand-edited\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := writeInitManifest("other", path, &out, &errW); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "name = \"hand-edited\"\n" {
		t.Errorf("manifest = %q, want the user's bytes untouched", body)
	}
	if !strings.Contains(out.String(), "✓ keep   ai-specs/ai-specs.toml") {
		t.Errorf("stdout = %q, want the keep line", out.String())
	}
}

// TestInitManifestRendersTheNameLiterally pins S8, the recorded divergence: the
// shell's `sed s/{{PROJECT_NAME}}/<name>/g` mangles a name carrying `&` (sed's
// whole-match reference), so the port substitutes literally.
func TestInitManifestRendersTheNameLiterally(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	var out, errW bytes.Buffer
	if rc := writeInitManifest("R&D/app", path, &out, &errW); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `name = "R&D/app"`) {
		t.Errorf("manifest name line missing or mangled:\n%s", firstLines(string(body), 30))
	}
	if !strings.Contains(out.String(), "✓ wrote  ai-specs/ai-specs.toml") {
		t.Errorf("stdout = %q, want the wrote line", out.String())
	}
}

// TestInitManifestEscapesTheName pins the other half of S8: the name lands in a
// TOML basic string, so a backslash, a quote or a control character must be
// escaped or the manifest is invalid and, because init never overwrites it, the
// damage is permanent (JD-A-003).
func TestInitManifestEscapesTheName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`a\q`:  `name = "a\\q"`,
		`a"b`:  `name = "a\"b"`,
		"a\nb": `name = "a\nb"`,
	}
	for name, want := range cases {
		path := filepath.Join(t.TempDir(), "ai-specs.toml")
		var out, errW bytes.Buffer
		if rc := writeInitManifest(name, path, &out, &errW); rc != 0 {
			t.Fatalf("name %q: exit = %d; stderr=%q", name, rc, errW.String())
		}
		body := readFile(t, path)
		if !strings.Contains(body, want) {
			t.Errorf("name %q: manifest does not carry %q:\n%s", name, want, firstLines(body, 30))
		}
	}
}

// TestInitManifestDirectoryIsNotDestroyed pins the S4 invariant the verifier
// probed: when ai-specs.toml is not a regular file, init fails and leaves the
// path exactly as it found it.
func TestInitManifestDirectoryIsNotDestroyed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := writeInitManifest("x", path, &out, &errW); rc != 1 {
		t.Fatalf("exit = %d, want 1", rc)
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		t.Fatalf("the directory was replaced: stat=%v err=%v", fi, err)
	}
}

// TestInitRootGitignoreBlock pins init.sh's append/strip pair, which is NOT
// gitignore-root-refresh.py's algorithm: one separating newline at most, keep
// when the marker is present, and on --force strip the marked region only.
func TestInitRootGitignoreBlock(t *testing.T) {
	t.Parallel()
	template := []byte(initGitignoreMarkerBegin + "\n.claude/\n" + initGitignoreMarkerEnd + "\n")

	t.Run("append to a missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".gitignore")
		action, err := applyRootGitignoreBlock(path, template, false)
		if err != nil || action != gitignoreAppended {
			t.Fatalf("action=%v err=%v, want appended", action, err)
		}
		if got := readFile(t, path); got != string(template) {
			t.Errorf("file = %q, want %q", got, template)
		}
	})

	t.Run("append after content without a trailing newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".gitignore")
		if err := os.WriteFile(path, []byte("node_modules/"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := applyRootGitignoreBlock(path, template, false); err != nil {
			t.Fatal(err)
		}
		want := "node_modules/\n" + string(template)
		if got := readFile(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("keep when the marker is present", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".gitignore")
		before := "keepme/\n" + initGitignoreMarkerBegin + "\nold\n" + initGitignoreMarkerEnd + "\n"
		if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
			t.Fatal(err)
		}
		action, err := applyRootGitignoreBlock(path, template, false)
		if err != nil || action != gitignoreKept {
			t.Fatalf("action=%v err=%v, want kept", action, err)
		}
		if got := readFile(t, path); got != before {
			t.Errorf("file = %q, want it untouched", got)
		}
	})

	t.Run("force strips the marked region and appends again", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".gitignore")
		before := "keepme/\n" + initGitignoreMarkerBegin + "\nold\n" + initGitignoreMarkerEnd + "\nafter/\n"
		if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
			t.Fatal(err)
		}
		action, err := applyRootGitignoreBlock(path, template, true)
		if err != nil || action != gitignoreRefreshed {
			t.Fatalf("action=%v err=%v, want refreshed", action, err)
		}
		want := "keepme/\nafter/\n" + string(template)
		if got := readFile(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})
}

// TestInitRootGitignoreKeepsASymlink pins the round-two regression: the shell
// appended with `>>`, which writes THROUGH a symlinked .gitignore. An atomic
// rename must resolve the link and replace its target, or the link becomes a
// regular file and the user's real one is stranded (JD-B-003).
func TestInitRootGitignoreKeepsASymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "shared.gitignore")
	if err := os.WriteFile(real, []byte("shared/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".gitignore")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	template := []byte(initGitignoreMarkerBegin + "\n.claude/\n" + initGitignoreMarkerEnd + "\n")
	if _, err := applyRootGitignoreBlock(link, template, false); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	body := readFile(t, real)
	if body != "shared/\n"+string(template) {
		t.Errorf("the link's target = %q, want the block appended through it", body)
	}
}

// TestInitRootGitignoreCreatesADanglingLinksTarget pins the same rule for a
// link whose target does not exist yet: `>>` would create it.
func TestInitRootGitignoreCreatesADanglingLinksTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "not-yet.gitignore")
	link := filepath.Join(dir, ".gitignore")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	template := []byte(initGitignoreMarkerBegin + "\n.claude/\n" + initGitignoreMarkerEnd + "\n")
	if _, err := applyRootGitignoreBlock(link, template, false); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the dangling symlink was replaced: fi=%v err=%v", fi, err)
	}
	if body := readFile(t, real); body != string(template) {
		t.Errorf("the created target = %q, want the template", body)
	}
}

// TestInitTUIDecision pins the gate: --tui forces it, --no-tui suppresses it,
// and auto needs both TTYs, no explicit name, no --force and no manifest.
func TestInitTUIDecision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ai-specs.toml")

	cases := []struct {
		name string
		opts initOptions
		in   bool
		want bool
	}{
		{"tui on is unconditional", initOptions{tuiMode: "on"}, false, true},
		{"tui off is unconditional", initOptions{tuiMode: "off"}, true, false},
		{"auto with both TTYs", initOptions{tuiMode: "auto"}, true, true},
		{"auto without a TTY", initOptions{tuiMode: "auto"}, false, false},
		{"auto with an explicit name", initOptions{tuiMode: "auto", nameExplicit: true}, true, false},
		{"auto with force", initOptions{tuiMode: "auto", force: true}, true, false},
	}
	for _, tc := range cases {
		if got := shouldInitTUI(tc.opts, tc.in, tomlPath); got != tc.want {
			t.Errorf("%s: shouldInitTUI = %v, want %v", tc.name, got, tc.want)
		}
	}

	if err := os.WriteFile(tomlPath, []byte("name = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if shouldInitTUI(initOptions{tuiMode: "auto"}, true, tomlPath) {
		t.Error("auto must not open the wizard over an existing manifest")
	}
}

// TestInitNextStepsNamesTheTargets pins T8's two substitutions.
func TestInitNextStepsNamesTheTargets(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	writeInitNextSteps("/p", "/p/ai-specs", &out)
	// Full equality, not a Contains per line: the heredoc is the contract and a
	// dropped or reordered line must fail here (JD-A-006).
	want := `
✓ ai-specs initialized at /p

Next steps:
  1. Edit  /p/ai-specs/ai-specs.toml
       - set [agents].enabled       (claude, cursor, opencode, codex, copilot, gemini)
       - add [[deps]]               (vendored skills from git)
       - add [mcp.*] sections       (MCP servers)
  2. Optional: add recommended policy skills via [[deps]] (see catalog/README.md in the ai-specs-cli repo)
  3. Run   ai-specs sync             (vendor deps + regenerate AGENTS.md + fan-out per agent)
  4. Commit:
       - ai-specs/ai-specs.toml
       - ai-specs/skills/ (your LOCAL skills only — CLI-bundled skills resolve from the cache)
       - ai-specs/recipes/*/overrides/ (declared recipe overrides only)
       - ai-specs/commands/<your-local-commands>.md
       - AGENTS.md   .gitignore   ai-specs/.ai-specs.lock
     (ai-specs/.deps/ and the rest of ai-specs/recipes/ are gitignored.)

Generated agent files (.claude/, .cursor/, opencode.json, .mcp.json, CLAUDE.md, ...) are
gitignored — they are regenerated by 'ai-specs sync-agent' on every clone.
`
	if out.String() != want {
		t.Errorf("next steps differ from the heredoc:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
}

// --- T4b-1: the wizard behind the verb -------------------------------------

// wizardTestHome is the isolated home whose only stub is the seam S9 keeps
// (refresh-bundled), so a run reaches its end without a real CLI install.
func wizardTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	stubDir := filepath.Join(home, "lib", "_internal")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "refresh-bundled.py"), []byte("import sys\nsys.exit(0)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// stubWizardOpener hands the verb a scripted wizard input, which is the seam
// the production opener fills with a raw-mode terminal.
func stubWizardOpener(keys *scriptedWizardKeys) initWizardOpener {
	return func(io.Reader, io.Writer) (tui.KeySource, func(), int) {
		return keys, func() {}, 0
	}
}

// TestInitForcedTUIWithoutATTYFallsBack pins the rc-3 branch of the native
// wizard at the verb level: S15's non-TTY refusal prints NOTHING itself, and
// init.sh's fallback line then hands the run to the classic path.
func TestInitForcedTUIWithoutATTYFallsBack(t *testing.T) {
	home := wizardTestHome(t)
	dir := filepath.Join(t.TempDir(), "no-tty-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := runInit([]string{"init", "--tui", dir}, home, strings.NewReader(""), &out, &errW, ""); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	// The wizard's refusal is the first thing on stderr and it printed nothing
	// itself (S15). The classic run that follows may add the brief-render
	// fallback this stub home has produced since T4a (the template manifest
	// enables session-context, which a catalog-less home cannot materialize),
	// so the pin is the fallback line FIRST, not an empty tail.
	lines := strings.Split(errW.String(), "\n")
	if len(lines) == 0 || lines[0] != "  · TUI unavailable — falling back to classic init" {
		t.Errorf("stderr = %q, want the fallback line first", errW.String())
	}
	if strings.Contains(out.String(), "Project name") {
		t.Errorf("a prompt was drawn without a TTY:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  tui:     no") {
		t.Errorf("banner lacks the no row:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "ai-specs", "ai-specs.toml")); err != nil {
		t.Errorf("the classic path did not write the manifest: %v", err)
	}
}

// TestInitTUIWithExistingManifestSkipsTheWizard pins init.sh's skip branch:
// --tui over a user-owned manifest prints the skip line BEFORE the banner and
// runs classic with tui: no, keeping the bytes (S4).
func TestInitTUIWithExistingManifestSkipsTheWizard(t *testing.T) {
	home := wizardTestHome(t)
	dir := filepath.Join(t.TempDir(), "has-manifest")
	tomlPath := filepath.Join(dir, "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	user := []byte("# hand-edited\nname = \"keepme\"\n")
	if err := os.WriteFile(tomlPath, user, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := runInit([]string{"init", "--tui", dir}, home, strings.NewReader(""), &out, &errW, ""); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	want := "  · skip TUI — ai-specs.toml already exists (never overwritten by init)\n"
	skipAt := strings.Index(out.String(), want)
	bannerAt := strings.Index(out.String(), "ai-specs init\n")
	if skipAt < 0 {
		t.Errorf("stdout lacks the skip line:\n%s", out.String())
	}
	if bannerAt < 0 || skipAt > bannerAt {
		t.Errorf("the skip line must precede the banner:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  tui:     no") {
		t.Errorf("banner lacks the no row:\n%s", out.String())
	}
	if got := readFile(t, tomlPath); got != string(user) {
		t.Errorf("the manifest changed:\n%s", got)
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want nothing (no wizard, no fallback)", errW.String())
	}
}

// TestInitWizardRunThroughTheVerb pins the confirmed flow end to end: the
// wizard's answer replaces the basename in the banner and the manifest (S14
// name adoption), the banner reports tui: yes, the manifest step installs the
// staged file with the (from TUI) marker, and the staging temps are gone when
// the verb returns.
func TestInitWizardRunThroughTheVerb(t *testing.T) {
	home := wizardTestHome(t)
	stagingDir := t.TempDir()
	t.Setenv("TMPDIR", stagingDir)
	dir := filepath.Join(t.TempDir(), "wiz-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Name prompt: clear the "wiz-app" prefill, type "brand", Enter; topology
	// and agents take their defaults; the catalog is empty, so confirm follows.
	text := []tui.TextKey{{Kind: tui.TextLineStart}, {Kind: tui.TextKillToEnd}}
	for _, r := range "brand" {
		text = append(text, tui.TextKey{Kind: tui.TextRune, Rune: r})
	}
	text = append(text, tui.TextKey{Kind: tui.TextEnter})
	keys := wizardScript(text, tui.KeyEnter, tui.KeyEnter, tui.KeyEnter)

	var out, errW bytes.Buffer
	rc := runInitFlow([]string{"init", "--tui", dir}, home, strings.NewReader(""),
		&out, &errW, "", stubWizardOpener(keys))
	if rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q stdout=%q", rc, errW.String(), out.String())
	}
	stdout := out.String()
	for _, want := range []string{
		"  name:    brand",
		"  tui:     yes",
		"  ✓ wrote  ai-specs/ai-specs.toml (from TUI)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	manifest := readFile(t, filepath.Join(dir, "ai-specs", "ai-specs.toml"))
	if !strings.Contains(manifest, `name = "brand"`) {
		t.Errorf("the wizard's name did not land in the manifest:\n%s", manifest)
	}
	if strings.Contains(manifest, "wiz-app") {
		t.Errorf("the prefill name leaked into the manifest:\n%s", manifest)
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ai-specs-init-tui-") {
			t.Errorf("staging temp survived the verb: %s", e.Name())
		}
	}
}

// TestInitWizardCancelThroughTheVerb pins S15 at the verb: exit 1, the shell's
// own cancel line on stderr, the prompt already drawn on stdout but NO banner
// (the shell exits before printing it) and nothing on disk.
func TestInitWizardCancelThroughTheVerb(t *testing.T) {
	home := wizardTestHome(t)
	dir := filepath.Join(t.TempDir(), "cancel-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	// No text events: the name prompt sees EOF, which is a cancel.
	rc := runInitFlow([]string{"init", "--tui", dir}, home, strings.NewReader(""),
		&out, &errW, "", stubWizardOpener(wizardScript(nil)))
	if rc != 1 {
		t.Fatalf("exit = %d, want 1 (cancel); stderr=%q", rc, errW.String())
	}
	// The wizard drew its own banner and Cancelled line on stderr first; the
	// shell's cancel line is the last thing the verb says.
	if !strings.Contains(errW.String(), "\nCancelled\n") {
		t.Errorf("stderr lacks the wizard's Cancelled line:\n%s", errW.String())
	}
	if !strings.HasSuffix(errW.String(), "ai-specs init: cancelled\n") {
		t.Errorf("stderr = %q, want it to end with the shell's cancel line", errW.String())
	}
	if !strings.Contains(out.String(), "? Project name:") {
		t.Errorf("the prompt was not drawn before the cancel:\n%s", out.String())
	}
	if strings.Contains(out.String(), "  tui: ") {
		t.Errorf("the banner was printed before the wizard cancelled:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "ai-specs")); !os.IsNotExist(err) {
		t.Errorf("the cancelled run wrote project files (stat err=%v)", err)
	}
}

// TestInitTUIEnvSeamNoLongerDelegates pins S5 of odd/tasks/go-20-u2b-init.md:
// AI_SPECS_INIT_TUI_PY is retired, so with the variable set init behaves exactly
// as without it — it never hands the whole verb to lib/init.sh. The stub
// writes a marker, which must stay absent.
func TestInitTUIEnvSeamNoLongerDelegates(t *testing.T) {
	home := t.TempDir()
	libDir := filepath.Join(home, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "init.sh-ran")
	stub := "#!/usr/bin/env bash\nprintf ran > " + marker + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(libDir, "init.sh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_SPECS_INIT_TUI_PY", "/tmp/some-init-tui.py")
	dir := filepath.Join(t.TempDir(), "seam-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	if rc := runInit([]string{"init", "--tui", dir}, home, strings.NewReader(""), &out, &errW, ""); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("init executed the legacy lib/init.sh (marker stat err=%v); stdout=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "✓ ai-specs initialized at ") {
		t.Errorf("the native verb did not reach its closing line:\n%s", out.String())
	}
}

// TestInitRefreshBundledRunsNative pins S3/S4 of odd/tasks/go-20-u2b-init.md:
// the init refresh step runs the native sync.RefreshBundled — its header line
// stays, the flatten lines come from the native flatten — and spawns no
// python3. python3 is shadowed on PATH and logs every invocation.
func TestInitRefreshBundledRunsNative(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "bundled-skills", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bundled-skills", "alpha", "SKILL.md"), []byte("# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "bundled-commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bundled-commands", "demo.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "refresh-app")
	if err := os.MkdirAll(filepath.Join(dir, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "python3.log")
	binDir := t.TempDir()
	shadow := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "python3"), []byte(shadow), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out, errW bytes.Buffer
	if rc := refreshBundledAtInit(home, dir, &out, &errW); rc != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
	}
	stdout := out.String()
	header := "▸ refresh-bundled --init"
	skills := "⇢ flattened 1 bundled skill(s) → cache/.bundled/skills"
	commands := "⇢ flattened 1 bundled command(s) → cache/.bundled/commands"
	for _, want := range []string{header, skills, commands} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if h, s := strings.Index(stdout, header), strings.Index(stdout, skills); h >= 0 && s >= 0 && h > s {
		t.Errorf("the header line came after the flatten line:\n%s", stdout)
	}
	if data, err := os.ReadFile(logPath); err == nil {
		if logged := strings.TrimSpace(string(data)); logged != "" {
			t.Errorf("the step spawned python3:\n%s", logged)
		}
	} else if !os.IsNotExist(err) {
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

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestInitWizardEnvOfferDeclineKeepsTheVerbFlow pins the goal's cancel rule at
// the VERB level: declining the env offer (S30's amendment — decline is a
// cancel of the offer) leaves the wizard's staged manifest in place, never
// writes ai-specs.env, and the classic continuation runs untouched — banner,
// "(from TUI)" install, brief, gitignores, next steps — with exit 0.
func TestInitWizardEnvOfferDeclineKeepsTheVerbFlow(t *testing.T) {
	home := wizardTestHome(t)
	// The catalog carries the env recipe the S28 gate collects.
	writeSyncCatalogRecipeForTest(t, filepath.Join(home, "catalog", "recipes"), "env-one", envOfferRecipeBody("env-one"))
	stagingDir := t.TempDir()
	t.Setenv("TMPDIR", stagingDir)
	dir := filepath.Join(t.TempDir(), "env-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	keys := wizardScript(enterText(),
		tui.KeyEnter, // topology: auto
		tui.KeyEnter, // agents: defaults
		tui.KeySpace, // recipes: toggle env-one on
		tui.KeyEnter, // recipes: submit
		tui.KeyEnter, // wizard confirm
		tui.KeyNo,    // decline the env offer (a cancel of the offer, S30)
		tui.KeyEnter,
	)
	var out, errW bytes.Buffer
	rc := runInitFlow([]string{"init", "--tui", dir}, home, strings.NewReader(""),
		&out, &errW, "", stubWizardOpener(keys))
	if rc != 0 {
		t.Fatalf("exit = %d, want 0 (the offer's cancel must not fail the verb); stderr=%q stdout=%q",
			rc, errW.String(), out.String())
	}

	stdout := out.String()
	for _, want := range []string{
		"Required environment variables",               // the offer ran…
		"  tui:     yes",                               // …and the wizard was adopted
		"  ✓ wrote  ai-specs/ai-specs.toml (from TUI)", // …and its manifest installed
		"✓ ai-specs initialized at ",                   // …and the classic continuation reached the end
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "✓ wrote "+filepath.Join(dir, "ai-specs.env")) {
		t.Errorf("a declined offer printed the env write line:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "ai-specs.env")); err == nil {
		t.Error("ai-specs.env was written after a declined offer")
	}
	if _, err := os.Stat(filepath.Join(dir, ".envrc")); err == nil {
		t.Error(".envrc was written after a declined offer")
	}
	manifest := readFile(t, filepath.Join(dir, "ai-specs", "ai-specs.toml"))
	if !strings.Contains(manifest, `name = "env-app"`) || !strings.Contains(manifest, "[recipes.env-one]") {
		t.Errorf("the staged manifest did not install:\n%s", manifest)
	}
	for _, e := range mustReadDir(t, stagingDir) {
		if strings.HasPrefix(e.Name(), "ai-specs-init-tui-") {
			t.Errorf("staging temp survived the verb: %s", e.Name())
		}
	}
}

// writeSyncCatalogRecipeForTest seeds one catalog recipe for the verb tests.
func writeSyncCatalogRecipeForTest(t *testing.T, catalogDir, id, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(catalogDir, id), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, id, "recipe.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mustReadDir lists a directory the test just created.
func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// TestInitRenderFailureWritesThePlaceholder drives the native init verb into a
// real baseline-render failure, nothing injected: the isolated home has no recipe
// catalog, so the template's enabled recipe cannot be materialized and
// RenderBaselineBrief returns non-zero inside renderInitBrief. The fallback
// contract is then observed end to end: exit 0, the warning, the placeholder when
// AGENTS.md was missing, and the user's bytes when it was present. The Python
// row test_native_chain_failure_writes_the_placeholder (bash seam) is retired
// only with lib/ in PR4b; this test is its native route equivalent.
func TestInitRenderFailureWritesThePlaceholder(t *testing.T) {
	const warning = "  ! render skipped — fallback placeholder written\n"
	const placeholder = "# AGENTS.md - Runtime context\n"

	run := func(t *testing.T, preexisting []byte) (dir string, stdout, stderr string) {
		t.Helper()
		base := t.TempDir()
		home := wizardTestHome(t)
		dir = filepath.Join(base, "app")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if preexisting != nil {
			if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), preexisting, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var out, errW bytes.Buffer
		if rc := runInit([]string{"init", dir}, home, strings.NewReader(""), &out, &errW, ""); rc != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%q", rc, errW.String())
		}
		if strings.Contains(out.String(), "render AGENTS.md (baseline brief)") {
			t.Fatalf("the render succeeded; the fallback path was not exercised:\n%s", out.String())
		}
		return dir, out.String(), errW.String()
	}

	t.Run("no AGENTS.md writes the placeholder", func(t *testing.T) {
		dir, _, stderr := run(t, nil)
		if !strings.Contains(stderr, warning) {
			t.Errorf("stderr lacks the warning:\n%q", stderr)
		}
		got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != placeholder {
			t.Errorf("AGENTS.md = %q, want the placeholder %q", got, placeholder)
		}
	})

	t.Run("a user AGENTS.md is kept byte for byte", func(t *testing.T) {
		user := []byte("# hand-written brief\n")
		dir, _, stderr := run(t, user)
		if !strings.Contains(stderr, warning) {
			t.Errorf("stderr lacks the warning:\n%q", stderr)
		}
		got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, user) {
			t.Errorf("AGENTS.md was overwritten: %q", got)
		}
	})
}
