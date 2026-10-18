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
	"os"
	"os/exec"
	"path/filepath"
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
