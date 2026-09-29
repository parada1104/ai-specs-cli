package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// writeCatalogRecipe materializes <home>/catalog/recipes/<id>/recipe.toml, the
// on-disk install catalog recipe-read.read_recipe reads (never the embedded
// asset tree).
func writeCatalogRecipe(t *testing.T, home, id, body string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"), body)
}

// catalogRecipe is the minimal valid recipe.toml (id/name/description/version
// are required by recipe_schema); extra is appended verbatim.
func catalogRecipe(id, extra string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"N\"\ndescription = \"D\"\nversion = \"1.0\"\n" + extra
}

// TestEmitRecipeDepChecks pins the DepResult -> Check mapping.
func TestEmitRecipeDepChecks(t *testing.T) {
	d := &Doctor{}
	d.emitRecipeDepChecks([]depResult{
		{Binary: "git", OK: true, Required: true, RecipeID: "worktree-flow"},
		{Binary: "npx", OK: false, Required: true, RecipeID: "trello-mcp-workflow",
			Purpose: "Run the MCP server", InstallURL: "https://example.com/install"},
		{Binary: "missing", OK: false, Required: true, RecipeID: "r", Purpose: "do things"},
		{Binary: "opt", OK: false, Required: false, RecipeID: "r2", Purpose: "optional thing"},
		{Binary: "opt2", OK: false, Required: false, RecipeID: "r3", Purpose: "optional thing",
			InstallURL: "https://example.com/opt"},
	})
	assertChecks(t, d, []Check{
		{OK, "recipe-dep", "git available for worktree-flow", ""},
		{WARN, "recipe-dep", "npx missing/unusable for trello-mcp-workflow: Run the MCP server",
			"https://example.com/install"},
		{WARN, "recipe-dep", "missing missing/unusable for r: do things", "install the required CLI"},
		{INFO, "recipe-dep", "optional opt not found for r2: optional thing", ""},
		{INFO, "recipe-dep", "optional opt2 not found for r3: optional thing", "https://example.com/opt"},
	})
}

// TestRunVersionCheck pins _run_version_check parity: stdout is concatenated
// before stderr, and a non-zero exit still returns the captured output.
func TestRunVersionCheck(t *testing.T) {
	if got := runVersionCheck("echo out; echo err 1>&2; exit 3"); got != "out\nerr\n" {
		t.Errorf("runVersionCheck = %q, want %q", got, "out\nerr\n")
	}
}

// TestParseVersion pins dep_check._parse_version.
func TestParseVersion(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"git version 2.39.3", "2.39.3"},
		{"v10.2.0", "10.2.0"},
		{"no digits here", ""},
		{"build 42", "42"},
		{"", ""},
	}
	for _, tc := range cases {
		got := joinVersion(parseVersion(tc.text))
		if got != tc.want {
			t.Errorf("parseVersion(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestVersionGE pins dep_check._version_ge.
func TestVersionGE(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"2.0.0", "1.9.9", true},
		{"1.9.9", "2.0.0", false},
		{"1.2", "1.2.0", true},
		{"1.2.0", "1.2", true},
		{"", "1.0.0", false},
		{"1.0.0", "", true},
	}
	for _, tc := range cases {
		got := versionGE(parseVersion(tc.have), parseVersion(tc.want))
		if got != tc.ok {
			t.Errorf("versionGE(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}

// TestProviderVersionTuple pins provider_install._version_tuple, including the
// digit-boundary rule RE2 cannot express directly.
func TestProviderVersionTuple(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"release v0.1.0", "0.1.0"},
		{"123", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := joinVersion(providerVersionTuple(tc.text))
		if got != tc.want {
			t.Errorf("providerVersionTuple(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestCheckOneProbe exercises the read-only PATH probe without depending on
// which binaries the host happens to ship.
func TestCheckOneProbe(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fakebin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'fake 1.2.3'\n"), 0o755); err != nil {
		t.Fatalf("write fakebin: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	t.Run("ok", func(t *testing.T) {
		res := checkOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "fakebin --version", MinVersion: "1.0.0",
			Required: true,
		}, "r", t.TempDir())
		if !res.OK || !res.Found || res.Version != "1.2.3" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("min-version-not-met", func(t *testing.T) {
		res := checkOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "fakebin --version", MinVersion: "2.0.0",
			Required: true,
		}, "r", t.TempDir())
		if res.OK || !res.Found || res.Detail != "found 1.2.3 < required 2.0.0" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("version-unknown", func(t *testing.T) {
		res := checkOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "echo no version", MinVersion: "1.0.0",
		}, "r", t.TempDir())
		if !res.OK || !res.Found || res.Detail != "version unknown" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("no-version-check", func(t *testing.T) {
		res := checkOne(&schema.CliDep{Binary: "fakebin"}, "r", t.TempDir())
		if !res.OK || !res.Found {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("not-found", func(t *testing.T) {
		res := checkOne(&schema.CliDep{Binary: "definitely-not-a-real-binary-xyz", Required: true}, "r", t.TempDir())
		if res.OK || res.Found || res.Detail != "not found on PATH" {
			t.Errorf("result = %+v", res)
		}
	})
}

// TestCheckProjectDeps pins the enabled-recipe aggregation against an on-disk
// install catalog: only enabled recipes contribute, missing recipes are
// skipped, and cli_deps are probed in manifest order.
func TestCheckProjectDeps(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, strings.Join([]string{
		"[recipes.worktree-flow]",
		"enabled = true",
		"[recipes.session-context]",
		"enabled = true",
		"[recipes.trello-mcp-workflow]",
		"enabled = false",
		"[recipes.not-a-real-recipe]",
		"enabled = true",
	}, "\n")+"\n")
	writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))
	writeCatalogRecipe(t, home, "session-context", catalogRecipe("session-context", ""))

	results := checkProjectDeps(root, home)
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].Binary != "git" || results[0].RecipeID != "worktree-flow" {
		t.Errorf("result = %+v, want git for worktree-flow", results[0])
	}
}

// TestCheckProjectDepsInvalidRecipeSkipped pins read_recipe parity: a recipe
// whose on-disk recipe.toml fails validation (an escaping runtime-hook script)
// contributes no dep results, and the remaining recipes still do (the legacy
// `except Exception: continue`).
func TestCheckProjectDepsInvalidRecipeSkipped(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, strings.Join([]string{
		"[recipes.bad-hook]",
		"enabled = true",
		"[recipes.worktree-flow]",
		"enabled = true",
	}, "\n")+"\n")
	writeCatalogRecipe(t, home, "bad-hook", catalogRecipe("bad-hook",
		"\n[[provides.hooks]]\nid = \"x\"\nevent = \"pre-tool-use\"\nscript = \"../evil.sh\"\n"+
			"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))
	writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))

	results := checkProjectDeps(root, home)
	if len(results) != 1 || results[0].RecipeID != "worktree-flow" {
		t.Fatalf("results = %+v, want only worktree-flow", results)
	}
}

// TestCheckProjectDepsMissingManifest pins the `return []` branch.
func TestCheckProjectDepsMissingManifest(t *testing.T) {
	if results := checkProjectDeps(t.TempDir(), t.TempDir()); results != nil {
		t.Errorf("results = %+v, want nil", results)
	}
}

// TestCheckRecipeCLIDeps exercises the doctor-facing wiring end to end against
// an on-disk install catalog.
func TestCheckRecipeCLIDeps(t *testing.T) {
	t.Run("no-recipes-table", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"x\"\n")
		d := New(root, t.TempDir())
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 0 {
			t.Errorf("checks = %+v, want none", d.Checks)
		}
	})
	t.Run("recipe-without-cli-deps", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeManifest(t, root, "[recipes.session-context]\nenabled = true\n")
		writeCatalogRecipe(t, home, "session-context", catalogRecipe("session-context", ""))
		d := New(root, home)
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 0 {
			t.Errorf("checks = %+v, want none", d.Checks)
		}
	})
	t.Run("git-recipe", func(t *testing.T) {
		if !whichBinary("git") {
			t.Skip("git not on PATH")
		}
		root := t.TempDir()
		home := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
		writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
			"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))
		d := New(root, home)
		d.checkRecipeCLIDeps()
		if len(d.Checks) != 1 {
			t.Fatalf("checks = %+v, want 1", d.Checks)
		}
		c := d.Checks[0]
		if c.Name != "recipe-dep" || !strings.HasSuffix(c.Message, "for worktree-flow") {
			t.Errorf("check = %+v", c)
		}
	})
}

// TestReadCatalogRecipe pins that the on-disk catalog is readable and that an
// unknown id is skipped.
func TestReadCatalogRecipe(t *testing.T) {
	home := t.TempDir()
	writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))

	recipe, err := readCatalogRecipe(home, "worktree-flow")
	if err != nil {
		t.Fatalf("readCatalogRecipe(worktree-flow): %v", err)
	}
	if len(recipe.CliDeps) != 1 || recipe.CliDeps[0].Binary != "git" {
		t.Errorf("cli_deps = %+v", recipe.CliDeps)
	}
	if _, err := readCatalogRecipe(home, "not-a-real-recipe"); err == nil {
		t.Error("expected an error for an unknown recipe id")
	}
}
