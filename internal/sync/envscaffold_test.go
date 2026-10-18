package sync

// Tests for the native non-interactive env step (Go 20 / U4d-1, S23-S27 of
// odd/tasks/go-20-u4-init.md): the port of env_scaffold.py's main(),
// generate_env_example, ensure_root_envrc, collect_env_vars,
// _mcp_env_declarations, _quote_dotenv, _format_dotenv,
// missing_required_values and load_harness_env.
//
// The oracle strings below are literal copies of the Python constants
// (lib/_internal/env_scaffold.py:12-75) kept in the test so the expected
// bytes are an independent oracle rather than a read-back of the
// implementation under test.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	envOracleExampleHeader = `# ai-specs.env.example — committed template (safe to regenerate).
# Copy values into ai-specs.env (gitignored). Root .envrc loads it via direnv.
# Generated from enabled recipes' [[provides.mcp]] env references.
`
	envOracleEnvrcHeader = "# .envrc — direnv entry for this project\n" +
		"# Harness block managed by ai-specs; safe to add custom lines outside it.\n"
	envOracleBlock = "# managed-by: ai-specs (do not remove block)\n" +
		"dotenv_if_exists .env\n" +
		"dotenv_if_exists ai-specs.env\n" +
		"# end managed-by: ai-specs\n"
	envOracleNoVariables = "# (no env vars required by enabled recipes)\n"

	// Literal copies of the ENV_VAR_HELP sentences the fixtures reach
	// (lib/_internal/env_scaffold.py:49-69).
	envOracleHelpOpenProject = "Optional — `basic` is the provider's effective default when unset; use `bearer` for Bearer tokens"
	envOracleHelpTrelloToken = "Trello token — generate from https://trello.com/power-ups/admin (Power-Up → API key page → Token)"
)

// envFixtureRecipe is a presets-bearing catalog recipe: one variable with help
// text (TRELLO_TOKEN) and one with an ENV_EXAMPLE_DEFAULTS value
// (OPENPROJECT_AUTH=basic).
const envFixtureRecipe = `[recipe]
id = "env-fixture"
name = "Env Fixture"
description = "d"
version = "1.0.0"

[[provides.mcp]]
id = "envpreset"
command = "npx"
env = { TRELLO_TOKEN = "$TRELLO_TOKEN", OPENPROJECT_AUTH = "$OPENPROJECT_AUTH" }
`

// envWarnRecipe carries three required variables so the sorted missing-value
// warnings can be pinned against a partial ai-specs.env.
const envWarnRecipe = `[recipe]
id = "warn-fixture"
name = "Warn Fixture"
description = "d"
version = "1.0.0"

[[provides.mcp]]
id = "warnpreset"
command = "npx"
env = { TRELLO_API_KEY = "$TRELLO_API_KEY", TRELLO_TOKEN = "$TRELLO_TOKEN", CANONICAL_VAULT_PATH = "$CANONICAL_VAULT_PATH" }
`

// newEnvScaffoldWorkspace builds a throwaway project root with a manifest and
// a throwaway CLI home whose catalog carries recipeID (an empty recipeID
// writes a manifest with no [recipes.*] table at all).
func newEnvScaffoldWorkspace(t *testing.T, recipeID, recipeBody string, enabled bool) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	manifest := "[project]\nname = \"p\"\n"
	if recipeID != "" {
		writeSyncCatalogRecipe(t, filepath.Join(home, "catalog", "recipes"), recipeID, recipeBody)
		manifest += "\n[recipes." + recipeID + "]\nenabled = " + map[bool]string{true: "true", false: "false"}[enabled] +
			"\nversion = \"1.0.0\"\n"
	}
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, home
}

// envEvalPath is the test's independent oracle for Path.resolve() on an
// existing path (the stdlib symlink resolution, not the implementation's).
func envEvalPath(t *testing.T, path string) string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// runEnvStep calls the step entry point and returns its observable surface.
func runEnvStep(t *testing.T, root, home string) (rc int, stdout, stderr string) {
	t.Helper()
	var out, errW bytes.Buffer
	rc = RunEnvScaffold(root, home, &out, &errW)
	return rc, out.String(), errW.String()
}

// S23: a missing manifest is refused on stderr with the resolved absolute
// root, exit 1, nothing on stdout, and no files written.
func TestEnvScaffoldGuardRefusesMissingManifest(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	rc, stdout, stderr := runEnvStep(t, root, home)
	if rc != 1 {
		t.Errorf("exit code = %d, want 1", rc)
	}
	want := "Project not initialized: missing ai-specs/ai-specs.toml under " +
		envEvalPath(t, root) + "\n"
	if stderr != want {
		t.Errorf("stderr =\n%q\nwant\n%q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("root must stay empty, found %d entries", len(entries))
	}
}

// S24: exact example bytes for a presets-bearing recipe — header verbatim,
// sorted VAR=default  # <purpose> <help-sentence> lines, the help sentence
// appended when known, the ENV_EXAMPLE_DEFAULTS value pre-filled, and
// "Wrote <path>" on stdout.
func TestEnvScaffoldExampleBytes(t *testing.T) {
	root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
	rc, stdout, stderr := runEnvStep(t, root, home)
	if rc != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
	}
	want := envOracleExampleHeader + "\n" +
		"OPENPROJECT_AUTH=basic  # required by envpreset (env-fixture) " + envOracleHelpOpenProject + "\n" +
		"TRELLO_TOKEN=  # required by envpreset (env-fixture) " + envOracleHelpTrelloToken + "\n"
	raw, err := os.ReadFile(filepath.Join(root, "ai-specs.env.example"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Errorf("example bytes =\n%q\nwant\n%q", string(raw), want)
	}
	wantStdout := "Wrote " + filepath.Join(envEvalPath(t, root), "ai-specs.env.example") + "\n"
	if stdout != wantStdout {
		t.Errorf("stdout = %q, want %q", stdout, wantStdout)
	}
	// Both required variables have no value in ai-specs.env: sorted warnings
	// on stderr (the Python twin produces exactly these lines).
	wantStderr := "! OPENPROJECT_AUTH has no value in ai-specs.env — run ai-specs configure-recipes\n" +
		"! TRELLO_TOKEN has no value in ai-specs.env — run ai-specs configure-recipes\n"
	if stderr != wantStderr {
		t.Errorf("stderr =\n%q\nwant\n%q", stderr, wantStderr)
	}
	// The example generation must never create the runtime env file (S24).
	if _, err := os.Stat(filepath.Join(root, "ai-specs.env")); err == nil {
		t.Errorf("ai-specs.env must not be created by the example step")
	}
}

// S24: a disabled recipe contributes nothing, so the example is the literal
// no-variable line.
func TestEnvScaffoldExampleNoVariablesLiteral(t *testing.T) {
	root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, false)
	rc, _, stderr := runEnvStep(t, root, home)
	if rc != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
	}
	want := envOracleExampleHeader + "\n" + envOracleNoVariables
	raw, err := os.ReadFile(filepath.Join(root, "ai-specs.env.example"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Errorf("example bytes =\n%q\nwant\n%q", string(raw), want)
	}
}

// S24: a changed example keeps the previous bytes in .bak; an identical one
// is not rewritten at all (mtime untouched) and creates no .bak.
func TestEnvScaffoldExampleBackupOnlyWhenChanged(t *testing.T) {
	t.Run("changed keeps previous bytes in bak", func(t *testing.T) {
		root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
		example := filepath.Join(root, "ai-specs.env.example")
		if err := os.WriteFile(example, []byte("OLD\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		rc, _, stderr := runEnvStep(t, root, home)
		if rc != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
		}
		bak, err := os.ReadFile(example + ".bak")
		if err != nil {
			t.Fatalf("bak: %v", err)
		}
		if string(bak) != "OLD\n" {
			t.Errorf("bak bytes = %q, want %q", string(bak), "OLD\n")
		}
		raw, err := os.ReadFile(example)
		if err != nil {
			t.Fatal(err)
		}
		want := envOracleExampleHeader + "\n" +
			"OPENPROJECT_AUTH=basic  # required by envpreset (env-fixture) " + envOracleHelpOpenProject + "\n" +
			"TRELLO_TOKEN=  # required by envpreset (env-fixture) " + envOracleHelpTrelloToken + "\n"
		if string(raw) != want {
			t.Errorf("example bytes =\n%q\nwant\n%q", string(raw), want)
		}
	})

	t.Run("identical is not rewritten and creates no bak", func(t *testing.T) {
		root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
		example := filepath.Join(root, "ai-specs.env.example")
		want := envOracleExampleHeader + "\n" +
			"OPENPROJECT_AUTH=basic  # required by envpreset (env-fixture) " + envOracleHelpOpenProject + "\n" +
			"TRELLO_TOKEN=  # required by envpreset (env-fixture) " + envOracleHelpTrelloToken + "\n"
		if err := os.WriteFile(example, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-time.Hour)
		if err := os.Chtimes(example, past, past); err != nil {
			t.Fatal(err)
		}
		rc, _, stderr := runEnvStep(t, root, home)
		if rc != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
		}
		if _, err := os.Stat(example + ".bak"); err == nil {
			t.Errorf("identical content must not create .bak")
		}
		info, err := os.Stat(example)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(past) {
			t.Errorf("example was rewritten: mtime = %v, want %v", info.ModTime(), past)
		}
	})
}

// S25 shape 1: an absent .envrc is created with the two-line header, a blank
// line and the canonical block.
func TestEnvScaffoldEnvrcCreatedWithHeader(t *testing.T) {
	root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
	rc, _, stderr := runEnvStep(t, root, home)
	if rc != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".envrc"))
	if err != nil {
		t.Fatal(err)
	}
	want := envOracleEnvrcHeader + "\n" + envOracleBlock
	if string(raw) != want {
		t.Errorf(".envrc bytes =\n%q\nwant\n%q", string(raw), want)
	}
}

// S25 shape 2: a file carrying the block is replaced in place; the user's
// lines outside the block survive byte for byte. An already-canonical file is
// not rewritten (Python's sub produces identical text → no write).
func TestEnvScaffoldEnvrcReplaceInPlaceKeepsUserLines(t *testing.T) {
	prefix := "# my custom line\nuse nix\n"
	stale := "# managed-by: ai-specs (do not remove block)\n" +
		"dotenv_if_exists ai-specs/.env\n" +
		"# end managed-by: ai-specs"
	suffix := "\nexport FOO=bar\n"

	t.Run("stale block replaced around user lines", func(t *testing.T) {
		root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
		envrc := filepath.Join(root, ".envrc")
		if err := os.WriteFile(envrc, []byte(prefix+stale+suffix), 0o644); err != nil {
			t.Fatal(err)
		}
		rc, _, stderr := runEnvStep(t, root, home)
		if rc != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
		}
		raw, err := os.ReadFile(envrc)
		if err != nil {
			t.Fatal(err)
		}
		want := prefix + strings.TrimSuffix(envOracleBlock, "\n") + suffix
		if string(raw) != want {
			t.Errorf(".envrc bytes =\n%q\nwant\n%q", string(raw), want)
		}
	})

	t.Run("canonical block is not rewritten", func(t *testing.T) {
		root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
		envrc := filepath.Join(root, ".envrc")
		want := prefix + strings.TrimSuffix(envOracleBlock, "\n") + suffix
		if err := os.WriteFile(envrc, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-time.Hour)
		if err := os.Chtimes(envrc, past, past); err != nil {
			t.Fatal(err)
		}
		rc, _, stderr := runEnvStep(t, root, home)
		if rc != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
		}
		info, err := os.Stat(envrc)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(past) {
			t.Errorf(".envrc was rewritten: mtime = %v, want %v", info.ModTime(), past)
		}
	})
}

// S25 shape 3: an existing file without the block gets the block appended
// after a blank line (the Python sep rule: ensure the file ends with exactly
// one newline, then one blank line, then the block).
func TestEnvScaffoldEnvrcAppendedAfterBlankLine(t *testing.T) {
	cases := []struct {
		name string
		pre  string
		want string
	}{
		{"with trailing newline", "use nix\n", "use nix\n\n" + envOracleBlock},
		{"without trailing newline", "use nix", "use nix\n\n" + envOracleBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, home := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
			envrc := filepath.Join(root, ".envrc")
			if err := os.WriteFile(envrc, []byte(tc.pre), 0o644); err != nil {
				t.Fatal(err)
			}
			rc, _, stderr := runEnvStep(t, root, home)
			if rc != 0 {
				t.Fatalf("exit code = %d, want 0; stderr = %q", rc, stderr)
			}
			raw, err := os.ReadFile(envrc)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Errorf(".envrc bytes =\n%q\nwant\n%q", string(raw), tc.want)
			}
		})
	}
}

// S23: one warning per required variable with no non-empty value in
// ai-specs.env, sorted; non-fatal; "Wrote <path>" last on stdout; the
// runtime ai-specs.env itself is never touched.
func TestEnvScaffoldWarningsSortedAndNonFatal(t *testing.T) {
	root, home := newEnvScaffoldWorkspace(t, "warn-fixture", envWarnRecipe, true)
	envPath := filepath.Join(root, "ai-specs.env")
	original := "TRELLO_TOKEN=keep\nTRELLO_API_KEY=\nCANONICAL_VAULT_PATH=   \n"
	if err := os.WriteFile(envPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	rc, stdout, stderr := runEnvStep(t, root, home)
	if rc != 0 {
		t.Fatalf("exit code = %d, want 0 (warnings are non-fatal); stderr = %q", rc, stderr)
	}
	wantStderr := "! CANONICAL_VAULT_PATH has no value in ai-specs.env — run ai-specs configure-recipes\n" +
		"! TRELLO_API_KEY has no value in ai-specs.env — run ai-specs configure-recipes\n"
	if stderr != wantStderr {
		t.Errorf("stderr =\n%q\nwant\n%q", stderr, wantStderr)
	}
	wantStdout := "Wrote " + filepath.Join(envEvalPath(t, root), "ai-specs.env.example") + "\n"
	if stdout != wantStdout {
		t.Errorf("stdout = %q, want %q", stdout, wantStdout)
	}
	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("ai-specs.env was modified: %q, want %q", string(raw), original)
	}
}

// Differential against the Python authority: twin workspaces with the same
// manifest and the same catalog, one driven by
// `python3 lib/_internal/env_scaffold.py <root>` and one by the native step,
// compared on exit code, stdout, stderr and the produced file bytes.
func TestEnvScaffoldMatchesPythonAuthority(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	authority := filepath.Join(repoRoot, "lib", "_internal", "env_scaffold.py")
	if _, err := os.Stat(authority); err != nil {
		t.Skipf("env_scaffold.py not available under %s: %v", repoRoot, err)
	}

	rootA, homeA := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)
	rootB, homeB := newEnvScaffoldWorkspace(t, "env-fixture", envFixtureRecipe, true)

	// Python route: the module reads its catalog from AI_SPECS_HOME, exactly
	// like the sync child environment pins it.
	cmd := exec.Command(python, authority, rootA)
	cmd.Env = append(
		dropEnv(dropEnv(os.Environ(), "AI_SPECS_HOME"), "PYTHONDONTWRITEBYTECODE"),
		"AI_SPECS_HOME="+homeA,
		"PYTHONDONTWRITEBYTECODE=1",
	)
	var pyOut, pyErr bytes.Buffer
	cmd.Stdout = &pyOut
	cmd.Stderr = &pyErr
	pyRC := runExitCode(cmd, &pyErr, python)

	// Native route.
	goRC, goOut, goErr := runEnvStep(t, rootB, homeB)

	resA := envEvalPath(t, rootA)
	resB := envEvalPath(t, rootB)
	// The two roots differ only in path; normalize the Python stdout's
	// resolved root to the Go side's resolved root before comparing.
	wantOut := strings.ReplaceAll(pyOut.String(), resA, resB)

	if pyRC != goRC {
		t.Errorf("exit code: python = %d, go = %d", pyRC, goRC)
	}
	if goOut != wantOut {
		t.Errorf("stdout: go = %q, python = %q", goOut, wantOut)
	}
	if goErr != pyErr.String() {
		t.Errorf("stderr: go = %q, python = %q", goErr, pyErr.String())
	}
	for _, name := range []string{"ai-specs.env.example", ".envrc"} {
		pyBytes, err := os.ReadFile(filepath.Join(rootA, name))
		if err != nil {
			t.Fatalf("python-produced %s: %v", name, err)
		}
		goBytes, err := os.ReadFile(filepath.Join(rootB, name))
		if err != nil {
			t.Fatalf("go-produced %s: %v", name, err)
		}
		if !bytes.Equal(goBytes, pyBytes) {
			t.Errorf("%s differs:\npython:\n%q\ngo:\n%q", name, string(pyBytes), string(goBytes))
		}
	}
}

// _quote_dotenv port: quoting triggers on empty values and any of
// space/tab/newline/"'#$&*|\\, backslashes and quotes are escaped inside a
// double-quoted value; everything else stays bare.
func TestQuoteDotenv(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", `""`},
		{"plain", "plain"},
		{"a-b.c_1", "a-b.c_1"},
		{"with space", `"with space"`},
		{"tab\there", "\"tab\there\""},
		{"line\nbreak", "\"line\nbreak\""},
		{`say"hi`, `"say\"hi"`},
		{`back\slash`, `"back\\slash"`},
		{"a#b", `"a#b"`},
		{"a$b", `"a$b"`},
		{"a&b", `"a&b"`},
		{"a*b", `"a*b"`},
		{"a|b", `"a|b"`},
		{"a'b", `"a'b"`},
	}
	for _, tc := range cases {
		if got := quoteDotenv(tc.in); got != tc.want {
			t.Errorf("quoteDotenv(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// _format_dotenv port: header lines, one blank line, sorted VAR=quoted lines
// (or the literal no-variable line), one trailing newline.
func TestFormatDotenv(t *testing.T) {
	got := formatDotenv(
		map[string]string{"B": "x y", "A": "plain"},
		[]string{"# h1", "# h2"},
	)
	want := "# h1\n# h2\n\nA=plain\nB=\"x y\"\n"
	if got != want {
		t.Errorf("formatDotenv = %q, want %q", got, want)
	}
	got = formatDotenv(map[string]string{}, []string{"# h1", "# h2"})
	want = "# h1\n# h2\n\n" + envOracleNoVariables
	if got != want {
		t.Errorf("formatDotenv (empty) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// U4d-2 (S28-S33): the wizard's env offer — WriteHarnessEnv's merge, the three
// legacy migrations, DirenvAllow and OfferHarnessEnv behind an injected prompt
// interface. Every case is scripted (acceptance: scripted answers, no PTY); the
// oracle strings are literal copies of the Python's lines and constants, so the
// expected bytes stay an independent oracle.
// ---------------------------------------------------------------------------

const (
	// Literal copies of env_scaffold.write_env's header (:292-295).
	envOracleEnvHeader = "# ai-specs.env — generated by ai-specs configure-recipes\n" +
		"# gitignored. Regenerate values via: ai-specs configure-recipes\n"

	// Literal copies of the ENV_VAR_HELP sentences the prompt fixtures reach
	// (lib/_internal/env_scaffold.py:49-69).
	envOracleHelpTrelloAPIKey = "Trello API key — create at https://trello.com/power-ups/admin"
	envOracleHelpVaultPath    = "Absolute path to the project-scoped vault folder (the only env the " +
		"vault MCP reads). Example: /Users/you/.../vault/nnodes/proyectos/app. " +
		"Must be fully resolved — do not leave nested $OTHER_VAR unexpanded."

	// env_scaffold.offer_harness_env's exact lines (:635-639, :653-657).
	envOracleDirenvMissing = "  ! direnv is not installed or could not be run.\n" +
		"    Install it with: brew install direnv\n" +
		"    Then run: direnv allow\n"
	envOracleDirenvOK = "  ✓ direnv allow — variables are now active in this terminal\n"
)

// envAllowedRecipe is the constrained fixture: one variable with env_allowed
// (a select), one secret (password) and one plain value (text) — the prompt's
// three kinds, in the Python's declaration order.
const envAllowedRecipe = `[recipe]
id = "allowed-fixture"
name = "Allowed Fixture"
description = "d"
version = "1.0.0"

[[provides.mcp]]
id = "allowedpreset"
command = "echo"
env = { OPENPROJECT_BASE_URL = "$OPENPROJECT_BASE_URL", OPENPROJECT_API_TOKEN = "$OPENPROJECT_API_TOKEN", OPENPROJECT_AUTH = "$OPENPROJECT_AUTH" }
env_allowed = { OPENPROJECT_AUTH = ["basic", "bearer"] }
`

// envRecipeFixture is one enabled catalog recipe for newEnvOfferWorkspace.
type envRecipeFixture struct {
	id   string
	body string
}

// newEnvOfferWorkspace builds a project + home whose manifest enables every
// fixture recipe IN THE ORDER GIVEN: the banner iterates first-declaration
// order, so manifest order is part of the setup.
func newEnvOfferWorkspace(t *testing.T, fixtures ...envRecipeFixture) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	manifest := "[project]\nname = \"p\"\n"
	for _, fx := range fixtures {
		writeSyncCatalogRecipe(t, filepath.Join(home, "catalog", "recipes"), fx.id, fx.body)
		manifest += "\n[recipes." + fx.id + "]\nenabled = true\nversion = \"1.0.0\"\n"
	}
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, home
}

// envBody reads a file the tests assert byte-for-byte on.
func envBody(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// envExists reports whether path is a regular file.
func envExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// envPromptCall records one question the offer asked, so a test can assert the
// kind, the message, the choices and — for the select — the preselected default
// (_select_default's three steps reaching the prompt seam, S30).
type envPromptCall struct {
	kind        string // confirm | select | password | text
	message     string
	choices     []string
	initial     string
	instruction string
	answer      string
}

// scriptedEnvPrompts is the EnvOfferPrompts of the acceptance criteria: a test
// scripts the answers, cancelAt makes that prompt kind answer the cancelled
// None, and failAt makes it raise (questionary's exception path, which
// offer_harness_env reports with `Could not configure environment variables`).
type scriptedEnvPrompts struct {
	confirm  bool
	cancelAt string
	failAt   string
	answers  map[string]string
	calls    []envPromptCall
}

func (s *scriptedEnvPrompts) record(call envPromptCall) error {
	s.calls = append(s.calls, call)
	if s.cancelAt == call.kind {
		return ErrEnvOfferCancelled
	}
	if s.failAt == call.kind {
		return errors.New("scripted prompt failure")
	}
	return nil
}

func (s *scriptedEnvPrompts) Confirm(message string, def bool) (bool, error) {
	call := envPromptCall{
		kind:    "confirm",
		message: message,
		initial: strconv.FormatBool(def),
		answer:  strconv.FormatBool(s.confirm),
	}
	if err := s.record(call); err != nil {
		return false, err
	}
	return s.confirm, nil
}

func (s *scriptedEnvPrompts) Select(message string, choices []string, initial string) (string, error) {
	answer, scripted := s.answers[message]
	if !scripted {
		// No explicit answer: Enter accepts the preselected default — which is
		// exactly what the initial cursor makes a real user's Enter do.
		answer = initial
	}
	call := envPromptCall{kind: "select", message: message, choices: choices, initial: initial, answer: answer}
	if err := s.record(call); err != nil {
		return "", err
	}
	return answer, nil
}

func (s *scriptedEnvPrompts) Password(message, instruction string) (string, error) {
	call := envPromptCall{kind: "password", message: message, instruction: instruction, answer: s.answers[message]}
	if err := s.record(call); err != nil {
		return "", err
	}
	return s.answers[message], nil
}

func (s *scriptedEnvPrompts) Text(message, defaultValue string) (string, error) {
	call := envPromptCall{kind: "text", message: message, initial: defaultValue, answer: s.answers[message]}
	if err := s.record(call); err != nil {
		return "", err
	}
	return s.answers[message], nil
}

// callsOf filters the recorded calls by kind.
func (s *scriptedEnvPrompts) callsOf(kind string) []envPromptCall {
	var out []envPromptCall
	for _, c := range s.calls {
		if c.kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// bothPromptFixtures is the fixture pair every prompt/offer test uses: the
// warn recipe (secrets + a plain value) and the allowed recipe (the select).
func bothPromptFixtures() []envRecipeFixture {
	return []envRecipeFixture{
		{id: "warn-fixture", body: envWarnRecipe},
		{id: "allowed-fixture", body: envAllowedRecipe},
	}
}

// S31: the merge that never destroys. A blank/whitespace answer is skipped so
// a re-prompt with Enter cannot wipe a non-empty secret; a non-blank answer
// replaces the previous value (the re-prompt that UPDATES a secret, pinned by
// tests/test_env_scaffold.py::test_write_env_merges_preserves_extras); every
// other existing key keeps its value; the root .env is never touched.
func TestWriteHarnessEnvMergesAndNeverWipes(t *testing.T) {
	t.Run("a non-blank answer replaces its value and keeps extra keys", func(t *testing.T) {
		root := t.TempDir()
		envPath := filepath.Join(root, "ai-specs.env")
		if err := os.WriteFile(envPath, []byte("CUSTOM=1\nTRELLO_API_KEY=old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path, err := WriteHarnessEnv(root, map[string]string{"TRELLO_API_KEY": "new"})
		if err != nil {
			t.Fatalf("WriteHarnessEnv: %v", err)
		}
		if path != envPath {
			t.Errorf("path = %q, want %q", path, envPath)
		}
		body := envBody(t, envPath)
		if !strings.Contains(body, "CUSTOM=1\n") {
			t.Errorf("the extra key was lost:\n%q", body)
		}
		if !strings.Contains(body, "TRELLO_API_KEY=new\n") || strings.Contains(body, "TRELLO_API_KEY=old") {
			t.Errorf("the non-blank answer did not replace the value:\n%q", body)
		}
		if strings.Contains(body, "export ") {
			t.Errorf("write_env writes dotenv lines, not export lines:\n%q", body)
		}
	})

	t.Run("blank and whitespace answers never overwrite a non-empty secret", func(t *testing.T) {
		root := t.TempDir()
		if _, err := WriteHarnessEnv(root, map[string]string{
			"TRELLO_API_KEY": "secret",
			"TRELLO_TOKEN":   "tok",
		}); err != nil {
			t.Fatalf("first write: %v", err)
		}
		if _, err := WriteHarnessEnv(root, map[string]string{
			"TRELLO_API_KEY":       "",
			"TRELLO_TOKEN":         "   ",
			"CANONICAL_VAULT_PATH": "keep-me",
		}); err != nil {
			t.Fatalf("second write: %v", err)
		}
		body := envBody(t, filepath.Join(root, "ai-specs.env"))
		for _, want := range []string{"TRELLO_API_KEY=secret\n", "TRELLO_TOKEN=tok\n", "CANONICAL_VAULT_PATH=keep-me\n"} {
			if !strings.Contains(body, want) {
				t.Errorf("body lacks %q after a blank answer:\n%q", want, body)
			}
		}
	})

	t.Run("values are quoted and sorted under the Python header", func(t *testing.T) {
		root := t.TempDir()
		if _, err := WriteHarnessEnv(root, map[string]string{
			"CANONICAL_VAULT_PATH": "/path with spaces/x",
			"OPENPROJECT_AUTH":     "plain",
		}); err != nil {
			t.Fatalf("WriteHarnessEnv: %v", err)
		}
		want := envOracleEnvHeader +
			"\nCANONICAL_VAULT_PATH=\"/path with spaces/x\"\nOPENPROJECT_AUTH=plain\n"
		if got := envBody(t, filepath.Join(root, "ai-specs.env")); got != want {
			t.Errorf("ai-specs.env =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("the root .env is never touched", func(t *testing.T) {
		root := t.TempDir()
		appEnv := filepath.Join(root, ".env")
		if err := os.WriteFile(appEnv, []byte("APP=keep\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := WriteHarnessEnv(root, map[string]string{"TRELLO_API_KEY": "secret"}); err != nil {
			t.Fatalf("WriteHarnessEnv: %v", err)
		}
		if got := envBody(t, appEnv); got != "APP=keep\n" {
			t.Errorf("root .env changed: %q", got)
		}
		if !strings.Contains(envBody(t, filepath.Join(root, "ai-specs.env")), "TRELLO_API_KEY=secret\n") {
			t.Error("ai-specs.env was not written")
		}
	})
}

// S32: the nested dotenv migration parses, merges (existing non-empty values
// win) and only THEN renames to .bak, creating the root .envrc on the way.
func TestMigrateNestedHarnessEnvMigratesAndRenames(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	nested := filepath.Join(root, "ai-specs", ".env")
	if err := os.WriteFile(nested, []byte("TRELLO_API_KEY=legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateNestedHarnessEnv(root)
	if err != nil || !ran {
		t.Fatalf("migrateNestedHarnessEnv = (%v, %v), want (true, nil)", ran, err)
	}
	if !strings.Contains(envBody(t, filepath.Join(root, "ai-specs.env")), "TRELLO_API_KEY=legacy\n") {
		t.Error("the migrated key did not land in ai-specs.env")
	}
	if envExists(nested) {
		t.Error("the nested .env was not renamed")
	}
	if !envExists(nested + ".bak") {
		t.Error("no .env.bak after a successful migration")
	}
	if !strings.Contains(envBody(t, filepath.Join(root, ".envrc")), "dotenv_if_exists ai-specs.env") {
		t.Error("the root .envrc managed block was not ensured")
	}
}

// S32 / JD-6: comment-only content parses to nothing, so there is nothing to
// merge — and the file stays put instead of being renamed to a silent .bak.
func TestMigrateNestedHarnessEnvRenamesNothingWhenNothingParsed(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	nested := filepath.Join(root, "ai-specs", ".env")
	if err := os.WriteFile(nested, []byte("# only comments\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateNestedHarnessEnv(root)
	if err != nil || ran {
		t.Fatalf("migrateNestedHarnessEnv = (%v, %v), want (false, nil)", ran, err)
	}
	if !envExists(nested) {
		t.Error("a file with no parsed keys must stay in place")
	}
	if envExists(nested + ".bak") {
		t.Error("nothing parsed, so nothing may be renamed")
	}
	if envExists(filepath.Join(root, "ai-specs.env")) {
		t.Error("no ai-specs.env may be written when nothing parsed")
	}
}

// S32: export-prefixed keys inside the nested .env parse (the dotenv parser
// runs first and the export parser fills the gaps) before the rename.
func TestMigrateNestedHarnessEnvParsesExportLines(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	nested := filepath.Join(root, "ai-specs", ".env")
	if err := os.WriteFile(nested, []byte("export TRELLO_TOKEN=\"from-export\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateNestedHarnessEnv(root)
	if err != nil || !ran {
		t.Fatalf("migrateNestedHarnessEnv = (%v, %v), want (true, nil)", ran, err)
	}
	if !strings.Contains(envBody(t, filepath.Join(root, "ai-specs.env")), "TRELLO_TOKEN=from-export\n") {
		t.Error("the export line did not land in ai-specs.env")
	}
	if envExists(nested) || !envExists(nested+".bak") {
		t.Error("the nested file must be renamed after the write")
	}
}

// S32: the legacy .envrc export migration keeps existing non-empty values,
// fills the absent ones, renames to .envrc.bak and ensures the root .envrc.
func TestMigrateLegacyEnvrcKeepsExistingValuesAndFillsGaps(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	legacy := filepath.Join(root, "ai-specs", ".envrc")
	if err := os.WriteFile(legacy, []byte("export TRELLO_TOKEN=\"abc\"\nexport EMPTY=\"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, "ai-specs.env")
	if err := os.WriteFile(envPath, []byte("TRELLO_API_KEY=keep\nTRELLO_TOKEN=existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateLegacyEnvrc(root)
	if err != nil || !ran {
		t.Fatalf("migrateLegacyEnvrc = (%v, %v), want (true, nil)", ran, err)
	}
	body := envBody(t, envPath)
	if !strings.Contains(body, "TRELLO_API_KEY=keep\n") || !strings.Contains(body, "TRELLO_TOKEN=existing\n") {
		t.Errorf("existing values did not survive the merge:\n%q", body)
	}
	if envExists(legacy) || !envExists(legacy+".bak") {
		t.Error("the legacy .envrc must be renamed to .envrc.bak after the write")
	}
	if !envExists(filepath.Join(root, ".envrc")) {
		t.Error("the root .envrc managed block was not ensured")
	}
}

// S32 / JD-6 parity: a legacy .envrc without export lines parses to nothing
// and is left in place — no silent bak, no ai-specs.env.
func TestMigrateLegacyEnvrcRenamesNothingWhenNoExports(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	legacy := filepath.Join(root, "ai-specs", ".envrc")
	if err := os.WriteFile(legacy, []byte("# no exports\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateLegacyEnvrc(root)
	if err != nil || ran {
		t.Fatalf("migrateLegacyEnvrc = (%v, %v), want (false, nil)", ran, err)
	}
	if !envExists(legacy) || envExists(legacy+".bak") {
		t.Error("nothing parsed, so nothing may be renamed")
	}
	if envExists(filepath.Join(root, "ai-specs.env")) {
		t.Error("no ai-specs.env may be written when nothing parsed")
	}
}

// S32's order: the nested dotenv runs FIRST, so the export migration that
// follows only fills the gaps the nested one left.
func TestMigrateLegacyHarnessEnvRunsNestedFirst(t *testing.T) {
	root, _ := newEnvOfferWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "ai-specs", ".env"),
		[]byte("TRELLO_API_KEY=from-nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", ".envrc"),
		[]byte("export TRELLO_API_KEY=\"from-exports\"\nexport TRELLO_TOKEN=\"only-export\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, err := migrateLegacyHarnessEnv(root)
	if err != nil || !ran {
		t.Fatalf("migrateLegacyHarnessEnv = (%v, %v), want (true, nil)", ran, err)
	}
	body := envBody(t, filepath.Join(root, "ai-specs.env"))
	if !strings.Contains(body, "TRELLO_API_KEY=from-nested\n") {
		t.Errorf("the nested value lost to the export one (nested must run first):\n%q", body)
	}
	if !strings.Contains(body, "TRELLO_TOKEN=only-export\n") {
		t.Errorf("the export migration must fill the gap the nested left:\n%q", body)
	}
	for _, bak := range []string{".env.bak", ".envrc.bak"} {
		if !envExists(filepath.Join(root, "ai-specs", bak)) {
			t.Errorf("missing %s after the combined migration", bak)
		}
	}
}

// S30: _select_default's three steps — a matching existing value (case-
// insensitive), else the example default, else the first declared choice; a
// stale example default never answers a value outside the choices.
func TestSelectDefaultThreeSteps(t *testing.T) {
	choices := []string{"basic", "bearer"}

	t.Run("a configured value that matches a choice wins", func(t *testing.T) {
		got := selectDefault("OPENPROJECT_AUTH", choices,
			map[string]string{"OPENPROJECT_AUTH": "Bearer"})
		if got != "bearer" {
			t.Errorf("selectDefault = %q, want %q (case-insensitive match)", got, "bearer")
		}
	})

	t.Run("an unconfigured var takes the example default", func(t *testing.T) {
		got := selectDefault("OPENPROJECT_AUTH", choices, map[string]string{})
		if got != "basic" {
			t.Errorf("selectDefault = %q, want %q (ENV_EXAMPLE_DEFAULTS)", got, "basic")
		}
	})

	t.Run("a configured value outside the choices falls through to the example", func(t *testing.T) {
		got := selectDefault("OPENPROJECT_AUTH", choices,
			map[string]string{"OPENPROJECT_AUTH": "legacy"})
		if got != "basic" {
			t.Errorf("selectDefault = %q, want %q (a stale value must not be answered)", got, "basic")
		}
	})

	t.Run("no example default answers the first choice", func(t *testing.T) {
		got := selectDefault("TRELLO_TOKEN", choices, map[string]string{})
		if got != choices[0] {
			t.Errorf("selectDefault = %q, want the first choice %q", got, choices[0])
		}
	})

	t.Run("a stale example default never leaves the choices", func(t *testing.T) {
		saved := envExampleDefaults
		envExampleDefaults = map[string]string{"OPENPROJECT_AUTH": "obsolete"}
		defer func() { envExampleDefaults = saved }()
		got := selectDefault("OPENPROJECT_AUTH", choices, map[string]string{})
		if got != choices[0] {
			t.Errorf("selectDefault = %q, want a choice (%v)", got, choices)
		}
	})
}

// S30: the hidden-prompt predicate over the five keywords, case-insensitively.
func TestIsSecretVar(t *testing.T) {
	secrets := []string{
		"TRELLO_API_KEY", "TRELLO_TOKEN", "X_SECRET", "DB_PASSWORD",
		"SOME_APIKEY", "api_key", "github_token",
	}
	for _, variable := range secrets {
		if !isSecretVar(variable) {
			t.Errorf("isSecretVar(%q) = false, want true", variable)
		}
	}
	plain := []string{"OPENPROJECT_AUTH", "OPENPROJECT_BASE_URL", "CANONICAL_VAULT_PATH", "MODE"}
	for _, variable := range plain {
		if isSecretVar(variable) {
			t.Errorf("isSecretVar(%q) = true, want false", variable)
		}
	}
}

// S30: the banner in FIRST-DECLARATION order with each purpose and help
// sentence, the offer question, then the prompts SORTED — select where the
// recipe declares env_allowed (with _select_default's value preselected),
// hidden prompt for secrets, text otherwise, help printed before each prompt
// it belongs to. The whole stdout is pinned because the scripted prompts draw
// nothing themselves.
func TestPromptEnvVarsBannerAndPromptKinds(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	p := &scriptedEnvPrompts{confirm: true}
	var out bytes.Buffer
	values, err := promptEnvVars(root, home, p, &out)
	if err != nil {
		t.Fatalf("promptEnvVars: %v", err)
	}

	want := "\nRequired environment variables\n" +
		"  TRELLO_API_KEY — required by warnpreset (warn-fixture)\n" +
		"    ℹ️  " + envOracleHelpTrelloAPIKey + "\n" +
		"  TRELLO_TOKEN — required by warnpreset (warn-fixture)\n" +
		"    ℹ️  " + envOracleHelpTrelloToken + "\n" +
		"  CANONICAL_VAULT_PATH — required by warnpreset (warn-fixture)\n" +
		"    ℹ️  " + envOracleHelpVaultPath + "\n" +
		"  OPENPROJECT_BASE_URL — required by allowedpreset (allowed-fixture)\n" +
		"  OPENPROJECT_API_TOKEN — required by allowedpreset (allowed-fixture)\n" +
		"  OPENPROJECT_AUTH — required by allowedpreset (allowed-fixture)\n" +
		"    ℹ️  " + envOracleHelpOpenProject + "\n" +
		"\n" +
		// the per-prompt help lines, in the SORTED loop that follows
		"ℹ️  " + envOracleHelpVaultPath + "\n" +
		"ℹ️  " + envOracleHelpOpenProject + "\n" +
		"ℹ️  " + envOracleHelpTrelloAPIKey + "\n" +
		"ℹ️  " + envOracleHelpTrelloToken + "\n"
	if got := out.String(); got != want {
		t.Errorf("stdout =\n%q\nwant\n%q", got, want)
	}

	// One call per variable, sorted, with the right kind for each.
	wantOrder := []struct {
		kind    string
		message string
	}{
		{"text", "CANONICAL_VAULT_PATH"},
		{"password", "OPENPROJECT_API_TOKEN"},
		{"select", "OPENPROJECT_AUTH"},
		{"text", "OPENPROJECT_BASE_URL"},
		{"password", "TRELLO_API_KEY"},
		{"password", "TRELLO_TOKEN"},
	}
	all := p.calls
	if len(all) != len(wantOrder)+1 { // + the offer question
		t.Fatalf("prompt calls = %d, want %d: %+v", len(all), len(wantOrder)+1, all)
	}
	if all[0].kind != "confirm" || all[0].message != "Configure these values now?" || all[0].initial != "true" {
		t.Errorf("first call = %+v, want the offer question with default yes", all[0])
	}
	for i, w := range wantOrder {
		got := all[i+1]
		if got.kind != w.kind || got.message != w.message {
			t.Errorf("call %d = (%s, %q), want (%s, %q)", i, got.kind, got.message, w.kind, w.message)
		}
	}

	// The select carries the declared choices and the preselected default.
	selects := p.callsOf("select")
	if len(selects) != 1 {
		t.Fatalf("select calls = %d, want 1", len(selects))
	}
	if got := selects[0].choices; len(got) != 2 || got[0] != "basic" || got[1] != "bearer" {
		t.Errorf("select choices = %v, want [basic bearer] in declaration order", got)
	}
	if selects[0].initial != "basic" {
		t.Errorf("select initial = %q, want the example default %q", selects[0].initial, "basic")
	}

	// The hidden prompt carries questionary's instruction; the text prompts do not.
	for _, call := range p.callsOf("password") {
		if call.instruction != "(hidden input)" {
			t.Errorf("password(%s) instruction = %q, want %q", call.message, call.instruction, "(hidden input)")
		}
	}
	for _, call := range p.callsOf("text") {
		if call.instruction != "" {
			t.Errorf("text(%s) instruction = %q, want empty", call.message, call.instruction)
		}
	}

	// Every variable answered; the select's answer is its preselected default
	// (what a real Enter answers without moving the cursor).
	if values["OPENPROJECT_AUTH"] != "basic" {
		t.Errorf("values[OPENPROJECT_AUTH] = %q, want the preselected default", values["OPENPROJECT_AUTH"])
	}
	if len(values) != 6 {
		t.Errorf("values = %v, want one entry per variable", values)
	}
}

// S30: re-prompting keeps a valid configured value — the existing value
// reaches the select as its preselected default instead of resetting to the
// example one (tests/test_env_scaffold.py's prefers_existing_value case).
func TestPromptEnvVarsSelectDefaultPrefersExistingValue(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, envRecipeFixture{id: "allowed-fixture", body: envAllowedRecipe})
	envPath := filepath.Join(root, "ai-specs.env")
	if err := os.WriteFile(envPath, []byte("OPENPROJECT_AUTH=Bearer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &scriptedEnvPrompts{confirm: true}
	if _, err := promptEnvVars(root, home, p, &bytes.Buffer{}); err != nil {
		t.Fatalf("promptEnvVars: %v", err)
	}
	selects := p.callsOf("select")
	if len(selects) != 1 {
		t.Fatalf("select calls = %d, want 1", len(selects))
	}
	if selects[0].initial != "bearer" {
		t.Errorf("select initial = %q, want %q (the configured value, canonically cased)", selects[0].initial, "bearer")
	}
}

// S30: cancelling ANY prompt cancels the offer's prompt phase, and a DECLINED
// offer question answers the same None the Python returns for decline and
// cancel alike (the S30 amendment — offer_harness_env returns on None).
func TestPromptEnvVarsCancelAtEachPrompt(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)

	t.Run("declining the offer question", func(t *testing.T) {
		p := &scriptedEnvPrompts{confirm: false}
		values, err := promptEnvVars(root, home, p, &bytes.Buffer{})
		if !errors.Is(err, ErrEnvOfferCancelled) {
			t.Errorf("err = %v, want ErrEnvOfferCancelled", err)
		}
		if values != nil {
			t.Errorf("values = %v, want nil (nothing to write)", values)
		}
	})

	for _, kind := range []string{"confirm", "select", "password", "text"} {
		kind := kind
		t.Run("cancel at the "+kind+" prompt", func(t *testing.T) {
			p := &scriptedEnvPrompts{confirm: true, cancelAt: kind}
			values, err := promptEnvVars(root, home, p, &bytes.Buffer{})
			if !errors.Is(err, ErrEnvOfferCancelled) {
				t.Errorf("err = %v, want ErrEnvOfferCancelled", err)
			}
			if values != nil {
				t.Errorf("values = %v, want nil", values)
			}
		})
	}
}

// S30: the text prompt hands back the RAW buffer (questionary.text does not
// trim); the never-wipe rule belongs to write_env's blank check.
func TestPromptEnvVarsReturnsTheRawBuffer(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	p := &scriptedEnvPrompts{
		confirm: true,
		answers: map[string]string{"CANONICAL_VAULT_PATH": "  /padded/path  "},
	}
	values, err := promptEnvVars(root, home, p, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promptEnvVars: %v", err)
	}
	if got := values["CANONICAL_VAULT_PATH"]; got != "  /padded/path  " {
		t.Errorf("value = %q, want the raw buffer", got)
	}
}

// fakeDirenvDir installs a `direnv` stub on a temp PATH that records its argv
// and exits 0, so the offer's allow branch is deterministic and never writes
// outside the test's temp tree.
func fakeDirenvDir(t *testing.T, record string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n{\nprintf '%s\\n' \"$@\"\n} > '" + record + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "direnv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// S29: the offer's happy path in order — migrate, collect, prompt, write
// ai-specs.env (blank answers skipped, sorted, quoted, Python header),
// regenerate the example, ensure the managed .envrc block, run direnv allow —
// every status line on the Python's stream.
func TestOfferHarnessEnvFullFlow(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	record := filepath.Join(t.TempDir(), "direnv-argv")
	t.Setenv("PATH", fakeDirenvDir(t, record))

	p := &scriptedEnvPrompts{
		confirm: true,
		answers: map[string]string{
			"CANONICAL_VAULT_PATH": "/vault/keep",
			"TRELLO_API_KEY":       "sekret",
		},
	}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	resolved := envEvalPath(t, root)
	// The scripted answers land, the blank ones are skipped, the select's
	// preselected default is what Enter would have answered.
	want := envOracleEnvHeader +
		"\nCANONICAL_VAULT_PATH=/vault/keep\nOPENPROJECT_AUTH=basic\nTRELLO_API_KEY=sekret\n"
	if got := envBody(t, filepath.Join(root, "ai-specs.env")); got != want {
		t.Errorf("ai-specs.env =\n%q\nwant\n%q", got, want)
	}
	if !envExists(filepath.Join(root, "ai-specs.env.example")) {
		t.Error("the example was not regenerated")
	}
	if !strings.Contains(envBody(t, filepath.Join(root, ".envrc")), "dotenv_if_exists ai-specs.env") {
		t.Error("the root .envrc managed block was not ensured")
	}

	// The status lines, in the Python's order, on the Python's streams.
	wantTail := "✓ wrote " + filepath.Join(resolved, "ai-specs.env") + "\n" +
		"✓ root .envrc managed block → " + filepath.Join(resolved, ".envrc") + "\n" +
		envOracleDirenvOK
	if !strings.HasSuffix(out.String(), wantTail) {
		t.Errorf("stdout tail = %q, want suffix %q", out.String(), wantTail)
	}
	if !strings.Contains(out.String(), "Required environment variables") {
		t.Errorf("stdout lacks the banner:\n%s", out.String())
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty on success", errW.String())
	}

	// direnv allow ran against the RESOLVED root with its own argv captured.
	argv := envBody(t, record)
	if want := "allow\n" + resolved + "\n"; argv != want {
		t.Errorf("direnv argv = %q, want %q", argv, want)
	}
}

// S30's amendment: declining the offer question is a CANCEL of the whole
// offer — no ai-specs.env, no example, no .envrc work, no direnv allow (the
// reference wins: prompt_env_vars returns None and offer_harness_env
// returns on None).
func TestOfferHarnessEnvDeclineStopsEverything(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	record := filepath.Join(t.TempDir(), "direnv-argv")
	t.Setenv("PATH", fakeDirenvDir(t, record))

	p := &scriptedEnvPrompts{confirm: false}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	for _, path := range []string{
		filepath.Join(root, "ai-specs.env"),
		filepath.Join(root, "ai-specs.env.example"),
		filepath.Join(root, ".envrc"),
	} {
		if envExists(path) {
			t.Errorf("%s must not exist after a declined offer", path)
		}
	}
	if _, err := os.Stat(record); err == nil {
		t.Error("direnv allow ran after a declined offer")
	}
	// The banner and the offer question were drawn; nothing follows them.
	if !strings.HasSuffix(out.String(), "\n\n") {
		t.Errorf("stdout = %q, want it to end with the banner's blank line", out.String())
	}
	if strings.Contains(out.String(), "✓") {
		t.Errorf("a declined offer printed a status line:\n%s", out.String())
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty", errW.String())
	}
}

// S30: cancelling a per-variable prompt cancels the offer before anything is
// written — the write phase never starts.
func TestOfferHarnessEnvCancelMidPromptWritesNothing(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	record := filepath.Join(t.TempDir(), "direnv-argv")
	t.Setenv("PATH", fakeDirenvDir(t, record))

	p := &scriptedEnvPrompts{confirm: true, cancelAt: "password"}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	for _, path := range []string{
		filepath.Join(root, "ai-specs.env"),
		filepath.Join(root, "ai-specs.env.example"),
		filepath.Join(root, ".envrc"),
	} {
		if envExists(path) {
			t.Errorf("%s must not exist after a cancelled offer", path)
		}
	}
	if _, err := os.Stat(record); err == nil {
		t.Error("direnv allow ran after a cancelled offer")
	}
	if !strings.Contains(out.String(), "Required environment variables") {
		t.Errorf("the banner was never drawn:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Could not") {
		t.Errorf("a cancel is not an error line:\n%s", out.String())
	}
}

// S31 through the offer: blank answers preserve the prior ai-specs.env (the
// offer still regenerates the example and the managed block, like the Python).
func TestOfferHarnessEnvBlankAnswersKeepSecrets(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	envPath := filepath.Join(root, "ai-specs.env")
	if err := os.WriteFile(envPath, []byte(
		"TRELLO_API_KEY=keep-key\nTRELLO_TOKEN=keep-tok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // no direnv: the three-line branch, no side effects

	p := &scriptedEnvPrompts{confirm: true} // every answer blank / the select default
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	body := envBody(t, envPath)
	if !strings.Contains(body, "TRELLO_API_KEY=keep-key\n") || !strings.Contains(body, "TRELLO_TOKEN=keep-tok\n") {
		t.Errorf("blank answers wiped the secrets:\n%q", body)
	}
	// direnv is absent: the Python's three-line message on stderr, and the
	// stdout tail is the .envrc status line (no direnv line at all).
	if got := errW.String(); got != envOracleDirenvMissing {
		t.Errorf("stderr =\n%q\nwant\n%q", got, envOracleDirenvMissing)
	}
	wantTail := "✓ root .envrc managed block → " + filepath.Join(envEvalPath(t, root), ".envrc") + "\n"
	if !strings.HasSuffix(out.String(), wantTail) {
		t.Errorf("stdout tail = %q, want suffix %q", out.String(), wantTail)
	}
}

// S29's soft-fail lines, one per Python branch: a migration failure prints
// `Legacy harness env migration skipped: …` and the offer CONTINUES.
func TestOfferHarnessEnvSoftFailMigrationSkipped(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	nested := filepath.Join(root, "ai-specs", ".env")
	if err := os.WriteFile(nested, []byte{0xff, 0xfe, 0x0a}, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	p := &scriptedEnvPrompts{confirm: false} // stop right after the banner
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	wantPrefix := "Legacy harness env migration skipped: cannot read " +
		filepath.Join(envEvalPath(t, root), "ai-specs", ".env") + ": invalid UTF-8\n"
	if !strings.HasPrefix(out.String(), wantPrefix) {
		t.Errorf("stdout =\n%q\nwant prefix %q", out.String(), wantPrefix)
	}
	// The offer continued into the prompt phase after the skipped migration.
	if !strings.Contains(out.String(), "Required environment variables") {
		t.Errorf("the offer aborted after a skipped migration:\n%s", out.String())
	}
	if envExists(nested + ".bak") {
		t.Error("a failed migration must not rename the file")
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty (soft-fails land on stdout)", errW.String())
	}
}

// S29's `Could not write harness env: …`: the write phase's failure stops the
// phase exactly where the Python's except → return stops it — no example, no
// .envrc, no direnv.
func TestOfferHarnessEnvSoftFailCouldNotWrite(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	// ai-specs.env as a directory: Python's write_text raises, and so does
	// the port's truncating write.
	if err := os.MkdirAll(filepath.Join(root, "ai-specs.env"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "direnv-argv")
	t.Setenv("PATH", fakeDirenvDir(t, record))

	p := &scriptedEnvPrompts{confirm: true, answers: map[string]string{"TRELLO_API_KEY": "sekret"}}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	if !strings.Contains(out.String(), "Could not write harness env: ") {
		t.Errorf("stdout lacks the soft-fail line:\n%s", out.String())
	}
	if envExists(filepath.Join(root, "ai-specs.env.example")) {
		t.Error("the write failure must stop the phase before the example")
	}
	if envExists(filepath.Join(root, ".envrc")) {
		t.Error("the write failure must stop the phase before the .envrc")
	}
	if _, err := os.Stat(record); err == nil {
		t.Error("the write failure must stop the offer before direnv allow")
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty (soft-fail, exit unchanged)", errW.String())
	}
}

// S29's `Could not configure environment variables: …`: a prompt that raises
// (questionary's exception path) is reported and stops the offer without a
// traceback and without writes.
func TestOfferHarnessEnvSoftFailConfigure(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	t.Setenv("PATH", t.TempDir())

	p := &scriptedEnvPrompts{confirm: true, failAt: "confirm"}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	want := "Could not configure environment variables: scripted prompt failure\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("stdout =\n%q\nwant line %q", out.String(), want)
	}
	if envExists(filepath.Join(root, "ai-specs.env")) {
		t.Error("a failed prompt phase must not write ai-specs.env")
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty (soft-fails land on stdout)", errW.String())
	}
}

// S33: with direnv absent the Python's install offer is DEFERRED (named in
// OfferHarnessEnv), and the three-line message lands on stderr — the offer
// itself has already written everything else.
func TestOfferHarnessEnvDirenvAbsentPrintsTheThreeLines(t *testing.T) {
	root, home := newEnvOfferWorkspace(t, bothPromptFixtures()...)
	t.Setenv("PATH", t.TempDir())

	p := &scriptedEnvPrompts{confirm: true}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	if got := errW.String(); got != envOracleDirenvMissing {
		t.Errorf("stderr =\n%q\nwant\n%q", got, envOracleDirenvMissing)
	}
	if !strings.Contains(out.String(), "✓ wrote ") || strings.Contains(out.String(), "direnv allow") {
		t.Errorf("stdout = %q, want the write lines and no direnv line", out.String())
	}
	if !envExists(filepath.Join(root, "ai-specs.env")) {
		t.Error("the offer must still write ai-specs.env without direnv")
	}
}

// S28's own guard, mirrored inside the offer (env_scaffold's collect →
// return): with nothing to collect the offer says nothing and writes nothing.
func TestOfferHarnessEnvWithoutVarsIsSilent(t *testing.T) {
	root, home := newEnvOfferWorkspace(t)
	t.Setenv("PATH", t.TempDir())

	p := &scriptedEnvPrompts{confirm: true}
	var out, errW bytes.Buffer
	OfferHarnessEnv(root, home, p, &out, &errW)

	if out.String() != "" {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	if errW.String() != "" {
		t.Errorf("stderr = %q, want empty", errW.String())
	}
	if envExists(filepath.Join(root, ".envrc")) {
		t.Error("no .envrc work may happen with nothing to collect")
	}
}
