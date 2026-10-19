package cli

// GO-20 U6a (T1): differential for the native `ai-specs recipe list`.
// Same discipline as skills_test.go's U5a table: the shell script
// (`bash lib/recipe-list.sh <args>`) and the built binary (`<bin> recipe list
// <args>`) run against the SAME project under one shared, pinned environment,
// and stdout, stderr and the exit code must be byte for byte identical
// (odd/tasks/go-20-u6-recipe.md S3/S4).
//
// Hermeticity: AI_SPECS_HOME is pinned to an isolated home built like
// tests/_blackbox.py:isolated_home (install entries symlinked in, cache/ local
// to the temp dir), PYTHONDONTWRITEBYTECODE=1 keeps the python legs from
// leaving __pycache__ anywhere, and LC_ALL=C pins collation to byte order.
// The env builder, the home builder and the tree snapshot are skillsListEnv,
// skillsListHome and skillsTreeSnapshot — this verb shares them rather than
// growing a second copy.
//
// Deliberate limit: every home below CARRIES a catalog/recipes (a symlink to
// the repo's own catalogue, or a real directory the way
// isolated_home(catalog=False) builds one), so the resolver's SECOND candidate
// (recipe-list.py:30-41's module-path fallback) is not byte-exercised here —
// Python anchors it on lib/'s physical location while the port anchors it on
// the running binary (recipe.CatalogHome), and a differential through that
// branch would measure the anchors instead of the verb. It is pinned
// in-process by TestCatalogHomeFallsBackToInstallRoot (list_test.go) and
// TestRecipeListFallsBackToBinaryInstallRoot below.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// recipeSeedCatalog seeds real fixture recipes into home/catalog/recipes/<id>
// — populate_catalog's shape: a recipe directory with the four strings both
// parsers require (internal/schema and lib/_internal/recipe_schema.py demand
// id/name/description/version, trimmed).
func recipeSeedCatalog(t *testing.T, home string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"),
			"[recipe]\n"+
				"id = \""+id+"\"\n"+
				"name = \""+id+" recipe\"\n"+
				"description = \"fixture recipe "+id+"\"\n"+
				"version = \"1.0.0\"\n")
	}
}

// recipeFixtureHome builds an isolated home WITHOUT the repo catalogue
// symlink, then seeds real fixture recipes — a catalog both legs read through
// AI_SPECS_HOME's first candidate (recipe-list.py:30-41).
func recipeFixtureHome(t *testing.T, root string, ids ...string) string {
	t.Helper()
	home := skillsListHome(t, root, "catalog")
	recipeSeedCatalog(t, home, ids...)
	return home
}

// recipeListFixtures builds the project shapes the differential runs:
// a manifest enabling two recipes (one in the repo catalogue, one that has no
// catalogue entry at all) plus a disabled one, a filter project over the
// fixture catalogue, and a project with no ai-specs/ tree.
func recipeListFixtures(t *testing.T) (projTwo, projFilter, projNoManifest string) {
	t.Helper()
	base := t.TempDir()

	projTwo = filepath.Join(base, "two-recipes")
	skillsWriteFile(t, filepath.Join(projTwo, "ai-specs", "ai-specs.toml"),
		`[project]
name = "two-recipes"

[recipes.worktree-flow]
enabled = true

[recipes.ghost-recipe]
enabled = true

[recipes.tdd-flow]
enabled = false
`)

	projFilter = filepath.Join(base, "filter-project")
	skillsWriteFile(t, filepath.Join(projFilter, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"filter-project\"\n")

	projNoManifest = filepath.Join(base, "no-manifest")
	if err := os.MkdirAll(projNoManifest, 0o755); err != nil {
		t.Fatal(err)
	}
	return projTwo, projFilter, projNoManifest
}

// TestRecipeListDifferentialAgainstShell is the U6a oracle: same project, both
// routes, byte-identical stdout/stderr/exit code over the case table below,
// plus a tree snapshot proving neither leg writes anything (the verb is
// read-only: no cache, no manifest, no ai-specs/ tree).
func TestRecipeListDifferentialAgainstShell(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 unavailable (the shell leg parses TOML with it): %v", err)
	}
	// LOUD skip, same class as the U5a tables: recipe-list.py imports
	// tomllib (Python ≥3.11) through toml-read, so on a machine whose python3
	// is older the LEGACY route itself crashes and there is no oracle to
	// compare against — an environment gap, never a port defect.
	probe := exec.Command(python3, "-c", "import tomllib")
	probe.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, perr := probe.CombinedOutput(); perr != nil {
		t.Skipf("python3 CANNOT import tomllib, so lib/recipe-list.sh cannot run and the differential oracle is unavailable — probe `python3 -c 'import tomllib'` said %v: %s (run with a Python ≥3.11 on PATH, e.g. PATH=/tmp/pybin:$PATH go test ./internal/cli/)", perr, out)
	}
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "recipe-list.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/recipe-list.sh unavailable: %v", err)
	}

	// Full isolated home: the repo's own catalogue through the symlink — the
	// real recipes both legs must list identically.
	home := skillsListHome(t, root)
	// A home with an EMPTY catalogue (isolated_home(catalog=False)'s shape).
	emptyHome := skillsListHome(t, root, "catalog")
	if err := os.MkdirAll(filepath.Join(emptyHome, "catalog", "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A fixture catalogue carrying an internal test recipe and a dot
	// directory that must NOT reach the listing.
	filterHome := recipeFixtureHome(t, root, "alpha", "test-fixture", ".hidden")
	projTwo, projFilter, projNoManifest := recipeListFixtures(t)

	type diffCase struct {
		name string
		home string
		args []string // after `recipe list` (Go) / after the script name (bash)
		dir  string   // working directory for both legs; "" → a fresh temp dir
	}
	cases := []diffCase{
		{"two recipes enabled one in catalog one not", home, []string{projTwo}, ""},
		{"empty catalog", emptyHome, []string{projTwo}, ""},
		{"internal test recipe and dot dir filtered", filterHome, []string{projFilter}, ""},
		{"no manifest", home, []string{projNoManifest}, ""},
		{"help", home, []string{"--help"}, ""},
		{"unknown flag", home, []string{"--bogus"}, ""},
		{"second positional", home, []string{projTwo, "extra"}, ""},
		{"positional after dashdash", home, []string{"--", projTwo}, ""},
		{"second positional after dashdash", home, []string{"--", "alpha", "beta"}, ""},
		{"target defaults to cwd", home, nil, projTwo},
	}

	roots := []string{home, emptyHome, filterHome, projTwo, projFilter, projNoManifest}
	before := skillsTreeSnapshot(t, roots...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.dir
			if dir == "" {
				dir = t.TempDir()
			}
			env := skillsListEnv(tc.home)

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, tc.args...), env, dir)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "list"}, tc.args...), env, dir)

			if !bytes.Equal(goOut, legOut) {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", goOut, legOut)
			}
			if !bytes.Equal(goErr, legErr) {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", goErr, legErr)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}
		})
	}

	after := skillsTreeSnapshot(t, roots...)
	if !reflect.DeepEqual(before, after) {
		for key, marker := range after {
			if before[key] != marker {
				t.Errorf("read-only violated: %s: %q -> %q", key, before[key], marker)
			}
		}
		for key := range before {
			if _, ok := after[key]; !ok {
				t.Errorf("read-only violated: %s disappeared", key)
			}
		}
	}
}

// TestRecipeListNativeNeedsNoBashOrPython is the routing proof for the U6a
// goal: with PATH emptied the legacy route could not run at all (the shim
// execs `bash lib/recipe.sh …`), while the native route prints the listing —
// `recipe list` no longer execs the shim, and no bash and no Python run on it
// (S1: zero Python under the product tree). The home carries ONLY a catalogue:
// no lib/, no bin/ — nothing the native verb could delegate to.
func TestRecipeListNativeNeedsNoBashOrPython(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	recipeSeedCatalog(t, home, "alpha")
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n")
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin, []string{"recipe", "list", project}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	if !strings.Contains(string(out), "[available") || !strings.Contains(string(out), "alpha") {
		t.Errorf("stdout = %q, want the fixture catalogue's available alpha row", out)
	}
}

// TestRecipeListMissingPathDeviation pins the frozen `cd` failure the way the
// U5a table does: exit 1 with the `cd: <path>: <reason>` shape, WITHOUT
// bash's own `lib/recipe-list.sh: line N: ` prefix (the S12/ADR 0002 class —
// the shell's line-number noise is not fabricated). The legacy leg is checked
// for the path and the strerror wording only, deliberately not byte-matched.
func TestRecipeListMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "recipe-list.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"recipe", "list", missing}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen cd exit)", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	want := "cd: " + missing + ": No such file or directory\n"
	if stderr.String() != want {
		t.Errorf("go stderr = %q, want %q", stderr.String(), want)
	}

	legOut, legErr, legCode := runOne("bash", []string{script, missing}, env, dir)
	if legCode != 1 {
		t.Errorf("bash exit = %d, want 1", legCode)
	}
	if len(legOut) != 0 {
		t.Errorf("bash stdout = %q, want empty", legOut)
	}
	for _, part := range []string{"cd:", missing, "No such file or directory"} {
		if !strings.Contains(string(legErr), part) {
			t.Errorf("bash stderr = %q, want it to contain %q", legErr, part)
		}
	}
}

// TestRecipeListFallsBackToBinaryInstallRoot pins the verb's end of the
// resolver contract in-process: the install root the second candidate anchors
// on is derived from the RUNNING BINARY's path (bin/ai-specs's parent, via
// home.ResolveHome), the FIRST candidate still wins when the home carries a
// catalogue, and — the divergence pin — a home without a catalogue and a
// binary outside any catalogue root list NOTHING: the port does not go
// looking through lib/ the way Python anchors on Path(recipe-list.py).
func TestRecipeListFallsBackToBinaryInstallRoot(t *testing.T) {
	installRoot := t.TempDir()
	recipeSeedCatalog(t, installRoot, "root-recipe")
	// The binary need not exist: ResolveHome stops at the first non-symlink
	// and returns the parent of its directory, like bin/ai-specs's own loop.
	exePath := filepath.Join(installRoot, "bin", "ai-specs")
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n")

	// Home WITHOUT a catalogue → the binary's install root lists.
	homeNoCatalog := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := runRecipeList([]string{project}, homeNoCatalog, exePath, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "root-recipe") {
		t.Errorf("stdout = %q, want the install root's root-recipe", stdout.String())
	}

	// Home WITH a catalogue → the first candidate wins over the binary root.
	homeWithCatalog := t.TempDir()
	recipeSeedCatalog(t, homeWithCatalog, "home-recipe")
	stdout.Reset()
	stderr.Reset()
	if code := runRecipeList([]string{project}, homeWithCatalog, exePath, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "home-recipe") || strings.Contains(stdout.String(), "root-recipe") {
		t.Errorf("stdout = %q, want only the home's home-recipe", stdout.String())
	}

	// The divergence pin: binary outside any root that has a catalogue →
	// empty listing, exit 0 — never the repo's own catalogue through lib/.
	stdout.Reset()
	stderr.Reset()
	outdoorExe := filepath.Join(t.TempDir(), "bin", "ai-specs")
	if code := runRecipeList([]string{project}, t.TempDir(), outdoorExe, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "No recipes available.\n" {
		t.Errorf("stdout = %q, want the empty listing", stdout.String())
	}
}
