package cli

// The native `init` verb's contract, pinned rule by rule (odd/tasks/go-20-u4-init.md
// T1, T2, T5, T8). The end-to-end differential against the legacy script lives in
// tests/test_init_native.py; these tests own the pieces whose failure the black box
// would report only as "the tree differs".

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
