package deps

// The probe's own contracts, moved with the code out of internal/doctor: the
// version-check runner, the version tuple helpers, the PATH probe's five arms,
// the enabled-recipe aggregation and the catalog reader. Doctor keeps only its
// result-to-check mapping (its depcheck_test.go), because the probe is no
// longer doctor's.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

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
		res := CheckOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "fakebin --version", MinVersion: "1.0.0",
			Required: true,
		}, "r", t.TempDir())
		if !res.OK || !res.Found || res.Version != "1.2.3" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("min-version-not-met", func(t *testing.T) {
		res := CheckOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "fakebin --version", MinVersion: "2.0.0",
			Required: true,
		}, "r", t.TempDir())
		if res.OK || !res.Found || res.Detail != "found 1.2.3 < required 2.0.0" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("version-unknown", func(t *testing.T) {
		res := CheckOne(&schema.CliDep{
			Binary: "fakebin", VersionCheck: "echo no version", MinVersion: "1.0.0",
		}, "r", t.TempDir())
		if !res.OK || !res.Found || res.Detail != "version unknown" {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("no-version-check", func(t *testing.T) {
		res := CheckOne(&schema.CliDep{Binary: "fakebin"}, "r", t.TempDir())
		if !res.OK || !res.Found {
			t.Errorf("result = %+v", res)
		}
	})
	t.Run("not-found", func(t *testing.T) {
		res := CheckOne(&schema.CliDep{Binary: "definitely-not-a-real-binary-xyz", Required: true}, "r", t.TempDir())
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

	results := CheckProject(root, home)
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

	results := CheckProject(root, home)
	if len(results) != 1 || results[0].RecipeID != "worktree-flow" {
		t.Fatalf("results = %+v, want only worktree-flow", results)
	}
}

// TestCheckProjectDepsUsesTheManifestKey pins check_project_deps' overwrite: the
// result carries the MANIFEST key, not the id the recipe.toml declares, so
// doctor's check text names what the user wrote in the manifest.
func TestCheckProjectDepsUsesTheManifestKey(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.dir-name]\nenabled = true\n")
	writeCatalogRecipe(t, home, "dir-name", catalogRecipe("declared-id",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))

	results := CheckProject(root, home)
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].RecipeID != "dir-name" {
		t.Errorf("RecipeID = %q, want the manifest key dir-name", results[0].RecipeID)
	}
}

// TestReadCatalogRecipe pins that the on-disk catalog is readable and that an
// unknown id is skipped.
func TestReadCatalogRecipe(t *testing.T) {
	home := t.TempDir()
	writeCatalogRecipe(t, home, "worktree-flow", catalogRecipe("worktree-flow",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"worktrees\"\nrequired = true\n"))

	recipe, err := ReadCatalogRecipe(home, "worktree-flow")
	if err != nil {
		t.Fatalf("ReadCatalogRecipe(worktree-flow): %v", err)
	}
	if len(recipe.CliDeps) != 1 || recipe.CliDeps[0].Binary != "git" {
		t.Errorf("cli_deps = %+v", recipe.CliDeps)
	}
	if _, err := ReadCatalogRecipe(home, "not-a-real-recipe"); err == nil {
		t.Error("expected an error for an unknown recipe id")
	}
}
