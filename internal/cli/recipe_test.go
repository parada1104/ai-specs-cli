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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/sync"
	"ai-specs.dev/ai-specs/internal/tui"
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

// ── GO-20 U6b (T2): differential for the native `ai-specs recipe add` ──
//
// Same discipline as the U6a table above: the WRAPPER (`bash
// lib/recipe-add.sh <args>` — the argument contract, line for line) and the
// built binary (`<bin> recipe add <args>`) run against byte-identical TWIN
// projects (the verb mutates, so one shared project cannot serve both legs),
// under one shared, pinned environment, comparing stdout, stderr and the exit
// code after normalizing each leg's own project root — PLUS the resulting
// project trees (the appended manifest bytes are the contract: S5 and the
// parity contract §3 placeholder rendering) and a snapshot of the CLI home
// around the whole table (recipe add writes NOTHING in the home: no cache, no
// vendor, no bytecode — PYTHONDONTWRITEBYTECODE keeps the python legs from
// leaving __pycache__ behind).
//
// The TTY legs are NOT exercised here: runOne captures both streams, so both
// legs take the non-TTY path Python itself takes (recipe-add.py:151's
// `tty = sys.stdin.isatty() and sys.stdout.isatty()` is false on both). The
// TTY tail is split since U6d-2 (L9): the "Configure now?" wizard leg is
// native — pinned by TestRecipeAddTailIsNativeOnATerminal's configure-leg
// subtests — while the env offer and the CLI-dep gate are U6e's, which flips
// them onto their own native seams and ends the delegation entirely.

// recipeAddRichRecipe is the fixture recipe that carries everything the verb
// renders: six config fields (required / defaults of several TOML types /
// bare optional) and every provides primitive, so one twin comparison pins
// the placeholder bytes, the "next sync will materialize" listing and BOTH
// Next steps lines (has_config and has_mcp_env) at once.
const recipeAddRichRecipe = `[recipe]
id = "cfg"
name = "cfg recipe"
description = "fixture recipe cfg"
version = "1.0.0"

[config.tags]
required = false
type = "list"
default = ["alpha", "beta"]

[config.board_id]
required = true
type = "string"

[config.epic_list]
required = false
type = "string"

[config.default_list]
required = false
type = "string"
default = "In Progress"

[config.auto_go]
required = false
type = "boolean"
default = true

[config.retries]
required = false
type = "integer"
default = 3

[provides]
skills = [{ id = "s1", source = "bundled" }, { id = "s2", source = "bundled" }]
commands = [{ id = "c1", path = "commands/c1.md" }]

[[provides.mcp]]
id = "m1"
command = "m1"
env = { M1_TOKEN = "$M1_TOKEN" }

[[provides.templates]]
source = "t/src"
target = "t/tgt"

[[provides.docs]]
source = "d/src"
target = "d/tgt"
`

// buildRecipeAddFixture materializes one case's project shape: ok/dup/broken
// write their manifest, empty only creates the project directory (the
// "Project not initialized" branch).
func buildRecipeAddFixture(t *testing.T, name, proj string) {
	t.Helper()
	switch name {
	case "ok":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"),
			"[project]\nname = \"u6b\"\n")
	case "dup":
		// alpha is already ENABLED — the duplicate refusal must fire before
		// any write (test_recipe_add's idempotence contract).
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"),
			"[project]\nname = \"u6b\"\n\n[recipes.alpha]\nenabled = true\n")
	case "broken":
		// A TOML syntax error (`True`): the dup reader raises and is swallowed
		// (recipe-add.py's `except Exception: pass`), then the write step's
		// guard must restore this exact manifest
		// (test_recipe_add's rollback fixture).
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"),
			"[project]\nname = \"u6b\"\n\n[recipes.old.config]\nflag = True\n")
	case "empty":
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown recipe-add fixture %q", name)
	}
}

// recipeAddGuardDetailRE normalizes the ONE thing the two parsers word
// differently: the embedded tomllib/Go parse-error detail inside the frozen
// guard message (ADR 0002 — parse-error detail is diagnostic). Everything
// else of that message — the id, the framing, "was not modified" — is
// byte-compared, and so is the untouched manifest the restore must leave.
var recipeAddGuardDetailRE = regexp.MustCompile(
	`(?s)invalid TOML manifest \(.*\)\. ai-specs\.toml was not modified\.`)

// TestRecipeAddDifferentialAgainstShell is T2's oracle: same case table,
// both routes, byte-identical stdout/stderr/exit code over twin projects,
// plus identical resulting trees (the appended manifest is part of the
// contract) and a CLI home proven untouched around the whole table.
func TestRecipeAddDifferentialAgainstShell(t *testing.T) {
	skillsOracle(t, "lib/recipe-add.sh")
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "recipe-add.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/recipe-add.sh unavailable: %v", err)
	}

	// One shared home carrying the real lib (the wrapper execs
	// python3 $AI_SPECS_HOME/lib/_internal/recipe-add.py) and a fixture
	// catalogue as a REAL directory — every home here carries a catalogue so
	// the resolver's module-path candidate is not byte-exercised (L3's
	// deviation 3, unchanged by this slice).
	home := recipeFixtureHome(t, root, "alpha", "test-fixture", "bad id")
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "cfg", "recipe.toml"),
		recipeAddRichRecipe)

	type diffCase struct {
		name      string
		fixture   string
		args      []string // "<PROJ>" is replaced by each leg's project root
		cwdProj   bool     // run each leg with cwd = its own project dir
		untouched bool     // both legs must leave the project byte-identical
	}
	cases := []diffCase{
		// ── the write: the append, the placeholder, the listing, guidance ──
		{name: "catalog recipe added to a manifest without it", fixture: "ok",
			args: []string{"alpha", "<PROJ>"}},
		{name: "config recipe placeholder, listing and next steps", fixture: "ok",
			args: []string{"cfg", "<PROJ>"}},
		// ── pre-write refusals (nothing may be written) ──
		{name: "already enabled duplicate refused", fixture: "dup",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		{name: "id absent from the catalog", fixture: "ok",
			args: []string{"ghost-recipe", "<PROJ>"}, untouched: true},
		{name: "internal test recipe refused", fixture: "ok",
			args: []string{"test-fixture", "<PROJ>"}, untouched: true},
		{name: "catalog path keeps the id uncleaned", fixture: "ok",
			args: []string{"nope/../alpha", "<PROJ>"}, untouched: true},
		{name: "project not initialized", fixture: "empty",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		// ── the wrapper's argument contract ──
		{name: "missing id", fixture: "ok", untouched: true},
		{name: "help", fixture: "ok", args: []string{"--help"}, untouched: true},
		{name: "unknown flag", fixture: "ok", args: []string{"--bogus"}, untouched: true},
		{name: "second positional", fixture: "ok",
			args: []string{"alpha", "<PROJ>", "extra"}, untouched: true},
		{name: "positional after dashdash", fixture: "ok",
			args: []string{"--", "alpha", "<PROJ>"}},
		{name: "third positional after dashdash", fixture: "ok",
			args: []string{"--", "alpha", "<PROJ>", "extra"}, untouched: true},
		{name: "target defaults to cwd", fixture: "ok",
			args: []string{"alpha"}, cwdProj: true},
		// ── the write step's guard: restore the original on invalid TOML ──
		{name: "append over an invalid manifest restores the original", fixture: "broken",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		{name: "valid manifest, invalid append (space id) restores", fixture: "ok",
			args: []string{"bad id", "<PROJ>"}, untouched: true},
	}

	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
			buildRecipeAddFixture(t, tc.fixture, projA)
			buildRecipeAddFixture(t, tc.fixture, projB)
			beforeA := skillsProjectTree(t, projA)
			beforeB := skillsProjectTree(t, projB)

			legArgs := func(proj string) []string {
				out := make([]string, 0, len(tc.args))
				for _, a := range tc.args {
					out = append(out, strings.ReplaceAll(a, "<PROJ>", proj))
				}
				return out
			}
			dirLeg := func(proj string) string {
				if tc.cwdProj {
					return proj
				}
				return scratch
			}
			env := skillsListEnv(home)

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, legArgs(projA)...), env, dirLeg(projA))
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "add"}, legArgs(projB)...), env, dirLeg(projB))

			normalize := func(b []byte, proj string) string {
				s := string(b)
				// The shell resolves the cwd through getcwd (e.g. /var →
				// /private/var on macOS) when a leg runs with cwd = project;
				// replace the resolved spelling first so both spellings
				// normalize to the same placeholder.
				if resolved, err := filepath.EvalSymlinks(proj); err == nil && resolved != proj {
					s = strings.ReplaceAll(s, resolved, "<PROJ>")
				}
				s = strings.ReplaceAll(s, proj, "<PROJ>")
				return recipeAddGuardDetailRE.ReplaceAllString(s,
					"invalid TOML manifest (<parse detail>). ai-specs.toml was not modified.")
			}
			if got, want := normalize(goOut, projB), normalize(legOut, projA); got != want {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if got, want := normalize(goErr, projB), normalize(legErr, projA); got != want {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}

			afterA := skillsProjectTree(t, projA)
			afterB := skillsProjectTree(t, projB)
			if !reflect.DeepEqual(afterA, afterB) {
				t.Errorf("project trees diverge after the two legs:\n bash: %v\n go:   %v", afterA, afterB)
			}
			if tc.untouched {
				if !reflect.DeepEqual(beforeA, afterA) {
					t.Errorf("bash leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeA, afterA)
				}
				if !reflect.DeepEqual(beforeB, afterB) {
					t.Errorf("go leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeB, afterB)
				}
			}
		})
	}

	// The verb writes NOTHING in the CLI home — no cache, no vendor, no
	// bytecode — so the snapshot must be fully unchanged.
	homeAfter := skillsTreeSnapshot(t, home)
	if !reflect.DeepEqual(homeBefore, homeAfter) {
		for key, marker := range homeAfter {
			if before, ok := homeBefore[key]; !ok || before != marker {
				t.Errorf("the verb wrote in the CLI home: %s: %q -> %q (existed before=%v)",
					key, before, marker, ok)
			}
		}
		for key := range homeBefore {
			if _, ok := homeAfter[key]; !ok {
				t.Errorf("the verb removed %s from the CLI home", key)
			}
		}
	}
}

// recipeAddWizardFixture is the TTY-tail fixture: a recipe WITH config fields
// (so the configure leg exists) plus the extra that decides how far the tail
// reaches — nothing (wizard only), an mcp env (the env-offer seam) or a
// CLI dep (the dep-gate seam). id/name/description/version are the four
// strings both parsers demand.
func recipeAddWizardFixture(id, extra string) string {
	return "[recipe]\n" +
		"id = \"" + id + "\"\n" +
		"name = \"" + id + " recipe\"\n" +
		"description = \"fixture recipe " + id + "\"\n" +
		"version = \"1.0.0\"\n" +
		"\n[config.board_id]\nrequired = true\ntype = \"string\"\n" +
		"\n[config.default_list]\nrequired = false\ntype = \"string\"\ndefault = \"In Progress\"\n" +
		"\n[config.note]\nrequired = false\n" +
		extra
}

const (
	// recipeAddMcpEnvTail makes has_mcp_env true: recipe-add.py's tail reaches
	// env_scaffold.offer_harness_env — the leg U6e wires to sync.OfferHarnessEnv.
	recipeAddMcpEnvTail = "\n[[provides.mcp]]\nid = \"m1\"\ncommand = \"m1\"\nenv = { M1_TOKEN = \"$M1_TOKEN\" }\n"
	// recipeAddCliDepTail makes recipe.cli_deps non-empty: with config fields
	// the tail's wizard leg runs the native CLI-dep gate first — internal/deps
	// through depgate.go's adapter, U7-1 T3.
	recipeAddCliDepTail = "\n[deps]\ncli = [{ binary = \"jinn\", purpose = \"provider CLI\" }]\n"
)

// TestRecipeAddTailIsNativeOnATerminal pins the FLIPPED TTY tail (U6e, T3b
// second half): off the TTY the port owns the verb end to end, unchanged. On
// a TTY there is NO delegation left — U6e wires recipe-add.py:247-275's two
// remaining legs onto their seams, the CLI dep gate (native since U7-1 T3:
// recipeDepGate over internal/deps.Gate) and the env offer
// (sync.OfferHarnessEnv),
// and the wizard runs natively — so lib/recipe.sh never executes for any
// recipe anymore. Every subtest pins legacyRan() FALSE, the native manifest
// result and the seam call in the script's own order: blank line →
// "Configure now?" → gate (where configure_selected_recipes puts it, or the
// tail's own pre-guidance position) → wizard → write → env offer.
func TestRecipeAddTailIsNativeOnATerminal(t *testing.T) {
	home := t.TempDir()
	skillsWriteFile(t, filepath.Join(home, "lib", "recipe.sh"),
		"#!/usr/bin/env bash\nprintf 'legacy seam: %s\\n' \"$*\"\nprintf ran > \"$AI_SPECS_HOME/legacy-ran\"\n")
	recipeSeedCatalog(t, home, "alpha")
	for id, body := range map[string]string{
		"plaincfg": recipeAddWizardFixture("plaincfg", ""),
		"cfgenv":   recipeAddWizardFixture("cfgenv", recipeAddMcpEnvTail),
		"cfgdep":   recipeAddWizardFixture("cfgdep", recipeAddCliDepTail),
	} {
		skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"), body)
	}
	marker := filepath.Join(home, "legacy-ran")

	// Every subtest starts from a fresh project and a clean marker.
	setup := func(t *testing.T) (project, manifest string) {
		t.Helper()
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		project = t.TempDir()
		manifest = filepath.Join(project, "ai-specs", "ai-specs.toml")
		skillsWriteFile(t, manifest, "[project]\nname = \"seam\"\n")
		return project, manifest
	}
	legacyRan := func() bool { _, err := os.Stat(marker); return err == nil }
	readManifest := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	// TTY + a configure leg (config fields, no env, no cli deps) → the seam
	// must NOT fire: the native wizard runs over scripted answers, writes the
	// answered value back, omits the blank one, and the configure leg takes
	// the place of the non-TTY "Next steps" guidance (recipe-add.py:247).
	t.Run("TTY with a configure leg runs the native wizard", func(t *testing.T) {
		project, manifest := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		p.textQ = []scriptedTextAnswer{
			{answers: []string{"B-7"}},         // board_id (required, sorted first)
			{answers: []string{"In Progress"}}, // default_list (the prefill comes back)
			{answers: []string{""}},            // note — blank → omitted
		}
		open := func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
			return p, func() {}, 0
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "plaincfg", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, recipeTailSeams{openWizard: open})
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy seam fired for a native configure leg")
		}
		want := "Recipe 'plaincfg' added to the manifest.\n" +
			"The next sync will materialize:\n  (no primitives declared)\n\n"
		if stdout.String() != want {
			t.Errorf("stdout = %q, want %q (the leg prints the blank line, then only scripted prompts)",
				stdout.String(), want)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		transcript := strings.Join(p.transcript, "\n")
		for _, want := range []string{
			"confirm: Configure now? | def=true | answer=true ok=true",
			`text: plaincfg.board_id (required, string) | def=""`,
			`text: plaincfg.default_list (optional, string) | def="In Progress"`,
			`text: plaincfg.note (optional, string) | def=""`,
			`answer: "" ok=true`, // the blank note — accepted, then omitted
		} {
			if !strings.Contains(transcript, want) {
				t.Errorf("wizard transcript missing %q:\n%s", want, transcript)
			}
		}
		data := readManifest(t, manifest)
		if !strings.Contains(data, `board_id = "B-7"`) {
			t.Errorf("the wizard's answer was not written back: manifest = %q", data)
		}
		if !strings.Contains(data, `default_list = "In Progress"`) {
			t.Errorf("the accepted default must survive: manifest = %q", data)
		}
		if !strings.Contains(data, `# note = ""  # optional`) {
			t.Errorf("the blank answer must be omitted, keeping the placeholder comment: manifest = %q", data)
		}
		if strings.Contains(data, "Next steps") {
			t.Errorf("the configure leg replaces the Next steps guidance: manifest = %q", data)
		}
	})

	// Declining the confirm mirrors recipe-add.py:249-251 exactly: the frozen
	// later line, exit 0, and no wizard write.
	t.Run("TTY configure leg declined keeps the placeholders", func(t *testing.T) {
		project, manifest := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: false, ok: true}}
		open := func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
			return p, func() {}, 0
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "plaincfg", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, recipeTailSeams{openWizard: open})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if legacyRan() {
			t.Errorf("the legacy seam fired for a declined native confirm")
		}
		want := "Recipe 'plaincfg' added to the manifest.\n" +
			"The next sync will materialize:\n  (no primitives declared)\n\n" +
			"You can configure later with: ai-specs configure-recipes\n"
		if stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
		if len(p.textQ) != 0 || len(p.transcript) != 1 {
			t.Errorf("the wizard must not run after a declined confirm: transcript = %#v", p.transcript)
		}
		data := readManifest(t, manifest)
		if !strings.Contains(data, `board_id = ""  # REQUIRED`) {
			t.Errorf("placeholders must stay unwritten: manifest = %q", data)
		}
	})

	// The PRODUCTION entry with a TTY flag whose streams cannot open a raw
	// terminal: the seam decision happens before any prompt, so the native
	// leg runs and the opener's refusal answers like a declined confirm
	// (recorded in L9) — never the legacy script.
	t.Run("TTY configure leg with an unopenable TUI stays native", func(t *testing.T) {
		project, manifest := setup(t)
		var stdout, stderr bytes.Buffer
		code := runRecipeAdd([]string{"recipe", "add", "plaincfg", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy seam fired although the native leg could run")
		}
		if !strings.HasSuffix(stdout.String(),
			"You can configure later with: ai-specs configure-recipes\n") {
			t.Errorf("stdout = %q, want the decline tail after the listing", stdout.String())
		}
		if data := readManifest(t, manifest); !strings.Contains(data, "[recipes.plaincfg]") {
			t.Errorf("the native leg must append before the opener refuses: manifest = %q", data)
		}
	})

	// TTY + mcp env: U6e flipped this leg — the verb never delegates anymore.
	// The wizard runs over scripted answers, then the ENV OFFER seam takes the
	// project root (production wires sync.OfferHarnessEnv through it), and the
	// legacy script never executes.
	t.Run("TTY reaching the env offer runs the native tail", func(t *testing.T) {
		project, manifest := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		p.textQ = []scriptedTextAnswer{
			{answers: []string{"B-7"}},         // board_id (required, sorted first)
			{answers: []string{"In Progress"}}, // default_list (the prefill comes back)
			{answers: []string{""}},            // note — blank → omitted
		}
		var envRoot string
		seams := recipeTailSeams{
			openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
				return p, func() {}, 0
			},
			envOffer: func(root string, _ []string) error {
				envRoot = root
				return nil
			},
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "cfgenv", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although the env offer has a native seam now")
		}
		if envRoot != project {
			t.Errorf("env offer root = %q, want the project root %q", envRoot, project)
		}
		data := readManifest(t, manifest)
		if !strings.Contains(data, "[recipes.cfgenv]") {
			t.Errorf("the native append is missing: manifest = %q", data)
		}
		if !strings.Contains(data, `board_id = "B-7"`) {
			t.Errorf("the wizard's answer was not written back: manifest = %q", data)
		}
	})

	// recipe-add.py:273-274: an exception escaping the env offer words ONE
	// frozen line (no detail) and the verb still exits 0 — the append stands.
	t.Run("env offer failure words the script's catch line", func(t *testing.T) {
		project, _ := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		p.textQ = []scriptedTextAnswer{
			{answers: []string{"B-7"}},
			{answers: []string{"In Progress"}},
			{answers: []string{""}},
		}
		seams := recipeTailSeams{
			openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
				return p, func() {}, 0
			},
			envOffer: func(string, []string) error { return errors.New("boom") },
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "cfgenv", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (the offer soft-fails), stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although the env offer soft-fails natively")
		}
		if !strings.Contains(stdout.String(), "  ! could not configure environment variables\n") {
			t.Errorf("stdout = %q, want the script's frozen catch line", stdout.String())
		}
	})

	// TTY + config fields + cli deps: U6e's dep-gate seam (S37's python3
	// driver in production) sits where configure_selected_recipes puts it —
	// after the confirm, BEFORE the wizard — so this leg stays native too.
	t.Run("TTY reaching the dep gate runs the native tail", func(t *testing.T) {
		project, manifest := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		p.textQ = []scriptedTextAnswer{
			{answers: []string{"B-7"}},
			{answers: []string{"In Progress"}},
			{answers: []string{""}},
		}
		var gated []string
		seams := recipeTailSeams{
			openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
				return p, func() {}, 0
			},
			depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
				gated = append(gated, loaded.ID)
				return wizardDepGateOutcome{Proceed: true}
			},
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "cfgdep", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although the dep gate has a native seam now")
		}
		if len(gated) != 1 || gated[0] != "cfgdep" {
			t.Errorf("gate calls = %v, want [cfgdep] at configure_selected_recipes' position", gated)
		}
		data := readManifest(t, manifest)
		if !strings.Contains(data, `board_id = "B-7"`) {
			t.Errorf("the wizard must run AFTER a proceeding gate: manifest = %q", data)
		}
	})

	// A declined gate mirrors configure_selected_recipes' skip: the frozen
	// Skipped line on stderr, no wizard, no write — and exit 0.
	t.Run("dep gate declined skips the wizard write", func(t *testing.T) {
		project, manifest := setup(t)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		seams := recipeTailSeams{
			openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
				return p, func() {}, 0
			},
			depGate: func(loaded *schema.Recipe) wizardDepGateOutcome { return wizardDepGateOutcome{} },
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "cfgdep", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although the declined gate is handled natively")
		}
		if want := "Skipped cfgdep (missing CLI deps)\n"; stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
		if len(p.textQ) != 0 || len(p.transcript) != 1 {
			t.Errorf("the wizard must not run after a declined gate: transcript = %#v", p.transcript)
		}
		if strings.Contains(readManifest(t, manifest), `board_id = "B-7"`) {
			t.Errorf("a declined gate must leave the placeholders unwritten")
		}
	})

	// A gate ERROR is not a decline: gateOutcome reports it with Proceed still
	// false, so the error arm must be tested FIRST — otherwise the decline path
	// swallows it and the command reports a skip where it should word its own
	// catch (review R3-001).
	t.Run("dep gate error keeps its own catch instead of the decline path", func(t *testing.T) {
		boom := errors.New("open /dev/tty: no such device or address")

		t.Run("with config fields", func(t *testing.T) {
			project, manifest := setup(t)
			p := &scriptedWizardPrompts{t: t}
			p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
			seams := recipeTailSeams{
				openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
					return p, func() {}, 0
				},
				depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
					return wizardDepGateOutcome{Err: boom}
				},
			}
			var stdout, stderr bytes.Buffer
			code := runRecipeAddFlow([]string{"recipe", "add", "cfgdep", project}, home,
				strings.NewReader(""), &stdout, &stderr, "", true, seams)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (configure_selected_recipes' except); stderr = %q", code, stderr.String())
			}
			if strings.Contains(stderr.String(), "Skipped cfgdep (missing CLI deps)") {
				t.Errorf("a gate error must not be worded as a decline: stderr = %q", stderr.String())
			}
			if !strings.Contains(stdout.String(), "  ! could not open the configuration wizard\n") {
				t.Errorf("stdout = %q, want recipe-add.py:266-267's frozen catch line", stdout.String())
			}
			if len(p.textQ) != 0 || len(p.transcript) != 1 {
				t.Errorf("the wizard must not run after a gate error: transcript = %#v", p.transcript)
			}
			if strings.Contains(readManifest(t, manifest), `board_id = "B-7"`) {
				t.Errorf("a gate error must leave the placeholders unwritten")
			}
		})

		t.Run("without config fields", func(t *testing.T) {
			project, _ := setup(t)
			skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "enverr", "recipe.toml"),
				"[recipe]\n"+
					"id = \"enverr\"\n"+
					"name = \"enverr recipe\"\n"+
					"description = \"fixture recipe enverr\"\n"+
					"version = \"1.0.0\"\n"+
					"\n[deps]\ncli = [{ binary = \"jinn\", purpose = \"provider CLI\" }]\n"+
					recipeAddMcpEnvTail)
			p := &scriptedWizardPrompts{t: t}
			p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
			envCalled := false
			seams := recipeTailSeams{
				openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
					return p, func() {}, 0
				},
				depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
					return wizardDepGateOutcome{Err: boom}
				},
				envOffer: func(string, []string) error {
					envCalled = true
					return nil
				},
			}
			var stdout, stderr bytes.Buffer
			code := runRecipeAddFlow([]string{"recipe", "add", "enverr", project}, home,
				strings.NewReader(""), &stdout, &stderr, "", true, seams)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
			}
			if want := "  ! CLI dependency check skipped: " + boom.Error() + "\n"; !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want recipe-add.py:76's frozen catch line %q", stdout.String(), want)
			}
			if strings.Contains(stdout.String(), "required CLI dependencies are still missing") {
				t.Errorf("a gate error must not print the decline's guidance block: stdout = %q", stdout.String())
			}
			if !envCalled {
				t.Errorf("the env leg must still run after a gate error (recipe-add.py:269)")
			}
		})
	})

	// recipe-add.py:256-261's OTHER gate position: a recipe with NO config
	// fields gates in the tail itself, and a decline prints the guidance block
	// (the dep_install guidance half ported) before the env leg still runs.
	t.Run("gate decline without config fields prints the guidance block", func(t *testing.T) {
		project, manifest := setup(t)
		skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "envdep", "recipe.toml"),
			"[recipe]\n"+
				"id = \"envdep\"\n"+
				"name = \"envdep recipe\"\n"+
				"description = \"fixture recipe envdep\"\n"+
				"version = \"1.0.0\"\n"+
				"\n[deps]\ncli = [{ binary = \"jinn\", purpose = \"provider CLI\" }]\n"+
				recipeAddMcpEnvTail)
		p := &scriptedWizardPrompts{t: t}
		p.confirmQ = []scriptedConfirm{{answer: true, ok: true}}
		var gated []string
		envCalled := false
		seams := recipeTailSeams{
			openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
				return p, func() {}, 0
			},
			depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
				gated = append(gated, loaded.ID)
				return wizardDepGateOutcome{}
			},
			envOffer: func(string, []string) error {
				envCalled = true
				return nil
			},
		}
		var stdout, stderr bytes.Buffer
		code := runRecipeAddFlow([]string{"recipe", "add", "envdep", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although both tail legs are native now")
		}
		if len(gated) != 1 || gated[0] != "envdep" {
			t.Errorf("gate calls = %v, want [envdep] at the tail's pre-guidance position", gated)
		}
		want := "Recipe 'envdep' added to the manifest.\n" +
			"The next sync will materialize:\n  - mcp: m1\n\n\n" +
			"  - jinn: install 'jinn' manually\n" +
			"required CLI dependencies are still missing; install them and re-run, or continue with:\n" +
			"  ai-specs configure-recipes\n"
		if stdout.String() != want {
			t.Errorf("stdout = %q, want %q (the script's guidance block)", stdout.String(), want)
		}
		if !envCalled {
			t.Errorf("the env leg must still run after the guidance block (recipe-add.py:269)")
		}
		if !strings.Contains(readManifest(t, manifest), "[recipes.envdep]") {
			t.Errorf("the native append is missing")
		}
	})

	// TTY with nothing to configure (no config fields, no mcp env): Python's
	// tail takes its else branch, so the native route owns the whole run —
	// the seam never fires and there is no confirm.
	t.Run("TTY with nothing to configure is native", func(t *testing.T) {
		project, manifest := setup(t)
		var stdout, stderr bytes.Buffer
		code := runRecipeAdd([]string{"recipe", "add", "alpha", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran although there is nothing interactive to delegate")
		}
		if want := "Recipe 'alpha' added to the manifest.\nThe next sync will materialize:\n  (no primitives declared)\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
		if !strings.Contains(readManifest(t, manifest), "[recipes.alpha]\nenabled = true\n") {
			t.Errorf("manifest missing the native append")
		}
	})

	// non-TTY → the seam never fires on any recipe: the port appends itself.
	t.Run("non-TTY never fires the seam", func(t *testing.T) {
		project, manifest := setup(t)
		var stdout, stderr bytes.Buffer
		code := runRecipeAdd([]string{"recipe", "add", "alpha", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", false)
		if code != 0 {
			t.Fatalf("non-tty leg exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if legacyRan() {
			t.Errorf("the legacy script ran on a non-TTY")
		}
		if want := "Recipe 'alpha' added to the manifest.\nThe next sync will materialize:\n  (no primitives declared)\n"; stdout.String() != want {
			t.Errorf("non-tty stdout = %q, want %q", stdout.String(), want)
		}
		if !strings.Contains(readManifest(t, manifest), "[recipes.alpha]\nenabled = true\n") {
			t.Errorf("manifest = %q, want the native [recipes.alpha] append", readManifest(t, manifest))
		}
	})
}

// TestRecipeAddNativeNeedsNoBashOrPython is the routing proof for the U6b
// goal: with PATH emptied the legacy route could not run at all (the shim
// execs bash lib/recipe.sh → recipe-add.sh → python3), while the native
// route appends the [recipes.<id>] block itself — `recipe add` no longer
// execs the shim off the TTY, and no bash and no Python run on it (S1).
func TestRecipeAddNativeNeedsNoBashOrPython(t *testing.T) {
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

	out, errOut, code := runOne(bin, []string{"recipe", "add", "alpha", project}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	if !strings.Contains(string(out), "Recipe 'alpha' added to the manifest.") {
		t.Errorf("stdout = %q, want the frozen success line", out)
	}
	data, err := os.ReadFile(filepath.Join(project, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[recipes.alpha]\nenabled = true\n") {
		t.Errorf("manifest = %q, want the native append", data)
	}
}

// TestRecipeAddMissingPathDeviation pins the frozen `cd` failure the way the
// U5a/U6a tables do: exit 1 with the `cd: <path>: <reason>` shape, WITHOUT
// bash's own `lib/recipe-add.sh: line N: ` prefix (the S12/ADR 0002 class —
// the shell's line-number noise is not fabricated). The legacy leg is checked
// for the path and the strerror wording only, deliberately not byte-matched.
func TestRecipeAddMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "recipe-add.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"recipe", "add", "alpha", missing}, home, nil, &stdout, &stderr)
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

	legOut, legErr, legCode := runOne("bash", []string{script, "alpha", missing}, env, dir)
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

// ── GO-20 U6c (T3): differential for the native `ai-specs recipe remove` ──
//
// Same discipline as the U5b remove table: the WRAPPER (`bash
// lib/recipe-remove.sh <args>` — the argument loop, the heredoc surgery and
// the lock cleanup that follows it) and the built binary (`<bin> recipe
// remove <args>`) run against byte-identical TWIN projects (the verb
// mutates, so one shared project cannot serve both legs), under one shared,
// pinned environment, comparing stdout, stderr and the exit code after
// normalizing each leg's own project root — PLUS the resulting project
// trees, where the manifest bytes AND the lock bytes are the contract (S5:
// the segment surgery is config.RemoveRecipeSegments, already
// differential-tested against recipe_remove_ref.py in internal/config; the
// lock step is recipe-remove.sh:167-203 through internal/lock), and a
// snapshot of the CLI home around the whole table (the verb writes NOTHING
// in the home).
//
// The shell leg's lock write goes through lock.py's worktree-gate bridge,
// so WORKTREE_GATE_BIN is pinned to a freshly built gate binary
// (skillsRemoveEnv + skillsRemoveGateBinary, the U5b pattern): with no pin
// the bridge fails open with a GO_LOCK_WRITE_BRIDGE_FALLBACK warning on
// stderr that the native leg never prints, and the byte comparison would
// measure the fail-open bridge instead of the verb.

// The manifest shapes: a recipe with a config sub-table (and a sibling whose
// multi-line string with blank lines must survive byte-for-byte, D2's class),
// a plain enabled recipe next to a sibling, a manifest without any
// [recipes.*] table, and test_recipe_remove.py's valid→invalid guard fixture
// (removing `foo` cuts a multi-line basic string open).
const (
	recipeRemoveManifestCfg = `[project]
name = "u6c"

[recipes.bar]
enabled = true
notes = """
first

third
"""

[recipes.alpha]
enabled = true

[recipes.alpha.config]
key = "value"
`
	recipeRemoveManifestLocked = `[project]
name = "u6c"

[recipes.bar]
enabled = true

[recipes.alpha]
enabled = true
`
	recipeRemoveManifestBarOnly = `[project]
name = "u6c"

[recipes.bar]
enabled = true
`
	recipeRemoveManifestNoRecipes = `[project]
name = "u6c"
`
	recipeRemoveManifestBreaks = `[project]
name = "test"

[recipes.bar]
doc = """
[recipes.foo]
inner
"""

[recipes.foo]
enabled = true
`
	// The lock fixtures: `alpha` carries the recipe entry the cleanup must
	// drop while meta/managed/agents survive (parity contract D1's subject);
	// `other` proves the lock is only REWRITTEN when something was removed —
	// its leading comment would not survive a rewrite, and lockKept asserts
	// the bytes too.
	recipeRemoveLockAlpha = `# Managed by ai-specs. Do not edit by hand.

[meta]
cli_version = "0.24.0"
synced_at = "2026-01-01T00:00:00Z"

[recipes.alpha]
"skills/a.md" = "deadbeef"

[managed."AGENTS.md"]
sha256 = "aaaa"
recipe = "bar"
kind = "runtime-brief"
policy = "never-force"

[agents."pi"]
"AGENTS.md" = "cccc"
`
	recipeRemoveLockOther = `# Managed by ai-specs. Do not edit by hand.
# a rewrite would drop this comment, so its survival pins "no rewrite"

[recipes.other]
"skills/a.md" = "deadbeef"

[managed."AGENTS.md"]
sha256 = "aaaa"
kind = "runtime-brief"
`
)

// buildRecipeRemoveFixture materializes one case's project shape under proj.
func buildRecipeRemoveFixture(t *testing.T, name, proj string) {
	t.Helper()
	manifest := func(content string) {
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), content)
	}
	switch name {
	case "cfg":
		manifest(recipeRemoveManifestCfg)
	case "locked":
		manifest(recipeRemoveManifestLocked)
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".ai-specs.lock"), recipeRemoveLockAlpha)
	case "locked-other":
		manifest(recipeRemoveManifestLocked)
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".ai-specs.lock"), recipeRemoveLockOther)
	case "absent":
		manifest(recipeRemoveManifestBarOnly)
	case "norecipes":
		manifest(recipeRemoveManifestNoRecipes)
	case "breaks":
		manifest(recipeRemoveManifestBreaks)
	case "empty":
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown recipe-remove fixture %q", name)
	}
}

// recipeRemoveGuardDetailRE normalizes the ONE thing the two parsers word
// differently in the valid→invalid refusal: the embedded tomllib/Go
// parse-error detail after the frozen framing (ADR 0002 — parse-error detail
// is diagnostic; internal/config's differential compares the prefix through
// "invalid TOML:" the same way). Everything else of the message — the id,
// the framing — is byte-compared, and so is the untouched manifest.
var recipeRemoveGuardDetailRE = regexp.MustCompile(`(?s)would produce invalid TOML: .*`)

// TestRecipeRemoveDifferentialAgainstShell is T3's oracle: same case table,
// both routes, byte-identical stdout/stderr/exit code over twin projects,
// plus identical resulting trees (manifest AND lock bytes are part of the
// contract) and a CLI home proven untouched around the whole table.
func TestRecipeRemoveDifferentialAgainstShell(t *testing.T) {
	skillsOracle(t, "lib/recipe-remove.sh")
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "recipe-remove.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/recipe-remove.sh unavailable: %v", err)
	}
	gate := skillsRemoveGateBinary(t)
	home := skillsListHome(t, root)
	env := skillsRemoveEnv(home, gate)

	type diffCase struct {
		name        string
		fixture     string
		args        []string // "<PROJ>" is replaced by each leg's project root
		cwdProj     bool     // run each leg with cwd = its own project dir
		untouched   bool     // both legs must leave the project byte-identical
		lockKept    bool     // the manifest changes, the lock must not
		guardDetail bool     // normalize the embedded parse-error detail only
	}
	cases := []diffCase{
		// ── the write: the segment surgery and the lock cleanup ──
		{name: "recipe with config sub-table", fixture: "cfg",
			args: []string{"alpha", "<PROJ>"}},
		{name: "recipe with lock entries", fixture: "locked",
			args: []string{"alpha", "<PROJ>"}},
		{name: "lock keeps another recipe's entries, not rewritten", fixture: "locked-other",
			args: []string{"alpha", "<PROJ>"}, lockKept: true},
		// ── refusals: nothing may be written ──
		{name: "recipe absent from manifest", fixture: "absent",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		{name: "manifest with no recipes tables", fixture: "norecipes",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		{name: "deletion breaking valid TOML refused", fixture: "breaks",
			args: []string{"foo", "<PROJ>"}, untouched: true, guardDetail: true},
		// ── the wrapper's argument contract ──
		{name: "missing id", fixture: "cfg", untouched: true},
		{name: "help", fixture: "cfg", args: []string{"--help"}, untouched: true},
		{name: "unknown flag", fixture: "cfg", args: []string{"--bogus"}, untouched: true},
		{name: "third positional", fixture: "cfg",
			args: []string{"alpha", "<PROJ>", "extra"}, untouched: true},
		{name: "positional after dashdash", fixture: "cfg",
			args: []string{"--", "alpha", "<PROJ>"}},
		{name: "manifest not found", fixture: "empty",
			args: []string{"alpha", "<PROJ>"}, untouched: true},
		{name: "target defaults to cwd", fixture: "cfg",
			args: []string{"alpha"}, cwdProj: true},
	}

	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
			buildRecipeRemoveFixture(t, tc.fixture, projA)
			buildRecipeRemoveFixture(t, tc.fixture, projB)
			beforeA := skillsProjectTree(t, projA)
			beforeB := skillsProjectTree(t, projB)
			readLock := func(proj string) []byte {
				data, err := os.ReadFile(filepath.Join(proj, "ai-specs", ".ai-specs.lock"))
				if err != nil {
					return nil
				}
				return data
			}
			lockA, lockB := readLock(projA), readLock(projB)

			legArgs := func(proj string) []string {
				out := make([]string, 0, len(tc.args))
				for _, a := range tc.args {
					out = append(out, strings.ReplaceAll(a, "<PROJ>", proj))
				}
				return out
			}
			dirLeg := func(proj string) string {
				if tc.cwdProj {
					return proj
				}
				return scratch
			}

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, legArgs(projA)...), env, dirLeg(projA))
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "remove"}, legArgs(projB)...), env, dirLeg(projB))

			normalize := func(b []byte, proj string) string {
				s := string(b)
				// The shell resolves the cwd through getcwd (e.g. /var →
				// /private/var on macOS) when a leg runs with cwd = project;
				// replace the resolved spelling first so both spellings
				// normalize to the same placeholder.
				if resolved, err := filepath.EvalSymlinks(proj); err == nil && resolved != proj {
					s = strings.ReplaceAll(s, resolved, "<PROJ>")
				}
				s = strings.ReplaceAll(s, proj, "<PROJ>")
				if tc.guardDetail {
					s = recipeRemoveGuardDetailRE.ReplaceAllString(s,
						"would produce invalid TOML: <parse detail>")
				}
				return s
			}
			if got, want := normalize(goOut, projB), normalize(legOut, projA); got != want {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if got, want := normalize(goErr, projB), normalize(legErr, projA); got != want {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}

			afterA := skillsProjectTree(t, projA)
			afterB := skillsProjectTree(t, projB)
			if !reflect.DeepEqual(afterA, afterB) {
				t.Errorf("project trees diverge after the two legs:\n bash: %v\n go:   %v", afterA, afterB)
			}
			if tc.untouched {
				if !reflect.DeepEqual(beforeA, afterA) {
					t.Errorf("bash leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeA, afterA)
				}
				if !reflect.DeepEqual(beforeB, afterB) {
					t.Errorf("go leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeB, afterB)
				}
			}
			// The lock step must only REWRITE when something was removed
			// (recipe-remove.sh:167-203): a lock whose entries do not match
			// keeps its bytes, comment line included.
			if tc.lockKept {
				if got := readLock(projA); !bytes.Equal(got, lockA) {
					t.Errorf("bash leg rewrote a lock that must stay untouched:\n before %q\n after  %q", lockA, got)
				}
				if got := readLock(projB); !bytes.Equal(got, lockB) {
					t.Errorf("go leg rewrote a lock that must stay untouched:\n before %q\n after  %q", lockB, got)
				}
			}
		})
	}

	if after := skillsTreeSnapshot(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Errorf("the verb wrote into the CLI home:\n before %v\n after  %v", homeBefore, after)
	}
}

// TestRecipeRemoveNativeNeedsNoBashOrPython is the routing proof for the U6c
// goal: with PATH emptied the legacy route could not run at all (the shim
// execs `bash lib/recipe.sh …` → recipe-remove.sh → python3), while the
// native route performs the surgery itself — `recipe remove` no longer
// execs the shim, and no bash and no Python run on it (S1).
func TestRecipeRemoveNativeNeedsNoBashOrPython(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n\n[recipes.alpha]\nenabled = true\n")
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin, []string{"recipe", "remove", "alpha", project}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	if !strings.Contains(string(out), "  ✓ removed 1 section(s) for recipe 'alpha' from ") {
		t.Errorf("stdout = %q, want the frozen success line", out)
	}
	data, err := os.ReadFile(filepath.Join(project, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "[recipes.alpha]") {
		t.Errorf("manifest = %q, want the [recipes.alpha] segment gone", data)
	}
}

// TestRecipeRemoveMissingPathDeviation pins the frozen `cd` failure the way
// the U5a/U6a/U6b tables do: exit 1 with the `cd: <path>: <reason>` shape,
// WITHOUT bash's own `lib/recipe-remove.sh: line N: ` prefix (the S12/ADR
// 0002 class — the shell's line-number noise is not fabricated). The legacy
// leg is checked for the path and the strerror wording only, deliberately not
// byte-matched.
func TestRecipeRemoveMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "recipe-remove.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"recipe", "remove", "alpha", missing}, home, nil, &stdout, &stderr)
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

	legOut, legErr, legCode := runOne("bash", []string{script, "alpha", missing}, env, dir)
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

// ── GO-20 U6d-1 (T4): differential for the native `ai-specs recipe configure` ──
//
// Same discipline as the U6a/U6b/U6c tables: the LEGACY ROUTE (`bash
// lib/recipe.sh configure <args>` — recipe.sh's configure arm execs
// lib/_internal/recipe-configure.py whole) and the built binary
// (`<bin> recipe configure <args>`) run against byte-identical TWIN projects
// under one shared, pinned environment, comparing stdout, stderr and the exit
// code after normalizing each leg's own project root — PLUS the resulting
// project trees (the manifest bytes are the contract: S5 and the
// PROJECT_OWNED_KEYS routing) and a snapshot of the CLI home around the whole
// table (the verb writes NOTHING in the home).
//
// Env pins: isolated AI_SPECS_HOME (lib symlinked so the legacy arm runs,
// VERSION symlinked so both legs print the same installed preflight), the
// fixture catalogue as a REAL directory, WORKTREE_GATE_BIN pinned to a
// freshly built worktree-gate (skillsRemoveEnv + skillsRemoveGateBinary, the
// U5b pattern): recipe-config-write.py writes through the gate's
// `--write-recipe-config` bridge, and with no pin it fails open with a
// GO_RECIPE_CONFIG_BRIDGE_FALLBACK stderr line the native leg never prints —
// the byte comparison would measure the fail-open bridge instead of the verb.
// LC_ALL=C, PYTHONDONTWRITEBYTECODE=1, the LOUD tomllib probe (skillsOracle;
// run with PATH=/tmp/pybin:$PATH). --sync is NOT in this table: it runs the
// real sync spine, whose writes would measure sync instead of this verb; the
// leg is pinned separately over a STUB front door below.

// recipeConfigureFixtureRecipe is the catalogue fixture for the table:
// standard fields of every kind (string, enum, integer, boolean), the
// project-owned `repo_topology` field, a validation regex, and the one
// structured table DECLARED BETWEEN standard fields so the inspect document's
// schema order (fields first, each group in TOML declaration order — Python's
// dict insertion order) is part of every --inspect byte comparison.
const recipeConfigureFixtureRecipe = `[recipe]
id = "cfg"
name = "cfg recipe"
description = "fixture recipe cfg"
version = "1.0.0"

[config.board_id]
required = false
type = "string"

[config.mode]
required = false
type = "string"
enum = ["fast", "slow"]
default = "fast"

[config.reconcile]
scope_field = "board_id"

[config.retries]
required = false
type = "integer"
default = 3

[config.auto_go]
required = false
type = "boolean"
default = true

[config.repo_topology]
required = false
type = "string"
help_text = "Deprecated alias of [project].repo_topology"
`

// recipeConfigureMCPFixtureRecipe declares TWO [[provides.mcp]] presets OUT OF
// SORTED ORDER ("trello" before "engram") — B-001 (dual-blind review): the
// script's grounding.mcp.present is
// `sorted(name for name in required_mcp if name in manifest_mcp)`, so a port
// that appends in declaration order flips the two ids, and only a fixture like
// this makes the differential catch it (recipeConfigureFixtureRecipe declares
// no [[provides.mcp]] at all).
const recipeConfigureMCPFixtureRecipe = `[recipe]
id = "mcporder"
name = "mcporder recipe"
description = "fixture recipe mcporder"
version = "1.0.0"

[[provides.mcp]]
id = "trello"
command = "npx"

[[provides.mcp]]
id = "engram"
command = "npx"
`

// buildRecipeConfigureFixture materializes one case's project shape under proj.
func buildRecipeConfigureFixture(t *testing.T, name, proj string) {
	t.Helper()
	manifest := func(content string) {
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), content)
	}
	switch name {
	case "cfg":
		// A config block: an inline comment, an inline structured table and a
		// plain scalar — the write legs edit these bytes.
		manifest(`[project]
name = "u6d1"

[recipes.cfg]
enabled = true

[recipes.cfg.config]
mode = "fast"  # pinned
reconcile = { scope_field = "board_id" }
`)
	case "nocfg":
		manifest("[project]\nname = \"u6d1\"\n\n[recipes.cfg]\nenabled = true\n")
	case "mcpmanifest":
		// Both mcp presets present in the manifest's [mcp] table, declared
		// there in sorted order — the fixture's own recipe declares them
		// unsorted, so grounding.mcp.present can only match both legs if the
		// report SORTS it.
		manifest(`[project]
name = "u6d1"

[mcp.engram]
command = "npx"

[mcp.trello]
command = "npx"

[recipes.mcporder]
enabled = true
`)
	case "absent":
		manifest("[project]\nname = \"u6d1\"\n")
	case "toolpin":
		// An exact pin the installed VERSION can never match: preflight blocks
		// (exit 4) unless --ignore-cli-version says otherwise.
		manifest("[project]\nname = \"u6d1\"\n\n[tool]\nversion = \"99.9.9-must-not-match\"\n\n[recipes.cfg]\nenabled = true\n")
	case "empty":
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown recipe-configure fixture %q", name)
	}
}

// TestRecipeConfigureDifferentialAgainstScript is T4's oracle: same case
// table, both routes, byte-identical stdout/stderr/exit code over twin
// projects, plus identical resulting trees (manifest bytes are the contract)
// and a CLI home proven untouched around the whole table.
func TestRecipeConfigureDifferentialAgainstScript(t *testing.T) {
	skillsOracle(t, "lib/recipe.sh configure (lib/_internal/recipe-configure.py)")
	bin, root := smokeSetup(t)
	recipeScript := filepath.Join(root, "lib", "recipe.sh")
	pyScript := filepath.Join(root, "lib", "_internal", "recipe-configure.py")
	if _, err := os.Stat(recipeScript); err != nil {
		t.Skipf("legacy lib/recipe.sh unavailable: %v", err)
	}
	if _, err := os.Stat(pyScript); err != nil {
		t.Skipf("legacy lib/_internal/recipe-configure.py unavailable: %v", err)
	}
	gate := skillsRemoveGateBinary(t)
	home := recipeFixtureHome(t, root, "alpha")
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "cfg", "recipe.toml"),
		recipeConfigureFixtureRecipe)
	// B-001's fixture: a second recipe whose [[provides.mcp]] presets are
	// declared out of sorted order (written before the home snapshot below).
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "mcporder", "recipe.toml"),
		recipeConfigureMCPFixtureRecipe)

	type diffCase struct {
		name      string
		fixture   string   // "" → no project directory is created
		args      []string // "<PROJ>" is replaced by each leg's project root
		cwdProj   bool     // run each leg with cwd = its own project dir
		untouched bool     // both legs must leave the project byte-identical
		columns   string   // COLUMNS pin for the width-sensitive cases
	}
	cases := []diffCase{
		// ── --inspect: with config, without config, absent, compact JSON ──
		{name: "inspect with config", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--inspect"}, untouched: true},
		{name: "inspect recipe without config block", fixture: "nocfg",
			args: []string{"cfg", "<PROJ>", "--inspect"}, untouched: true},
		{name: "inspect recipe absent from manifest", fixture: "absent",
			args: []string{"cfg", "<PROJ>", "--inspect"}, untouched: true},
		{name: "inspect json", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--inspect", "--json"}, untouched: true},
		{name: "inspect defaults to cwd", fixture: "cfg",
			args: []string{"cfg", "--inspect"}, cwdProj: true, untouched: true},
		// B-001: grounding.mcp.present must be SORTED (recipe-configure.py's
		// `sorted(...)`), not the recipe's declaration order — the fixture
		// recipe declares trello before engram, so removing the sort flips the
		// two ids and this case fails.
		{name: "mcp present is sorted, not declaration order", fixture: "mcpmanifest",
			args: []string{"mcporder", "<PROJ>", "--inspect"}, untouched: true},
		// L6 doc correction (a): the interleaved trailing positional
		// (`cfg --inspect /path`) is accepted by Python 3.14 — the reference
		// this differential runs on (PATH=/tmp/pybin) — and by the port;
		// Python 3.11 rejects it with `unrecognized arguments` exit 2, so
		// running this case against ≤3.11 fails on the recorded divergence.
		{name: "interleaved positional fills the path slot", fixture: "cfg",
			args: []string{"cfg", "--inspect", "<PROJ>"}, untouched: true},
		// ── --set: inline, dotted, structured, project-owned ──
		{name: "set inline scalar key", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "retries=7"}},
		{name: "set str fallback raw value", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "board_id=ABC-1"}},
		{name: "set nested dotted key", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "reconcile.max_age_seconds=900"}},
		{name: "set structured table key", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", `reconcile={scope_field="list_id",max_age_seconds=60}`}},
		{name: "repo_topology lands in project", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "repo_topology=standalone"}},
		{name: "json set report", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--json", "--set", "auto_go=false"}},
		{name: "unchanged value is a no-op", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "mode=fast"}, untouched: true},
		// ── refusals and the dry run ──
		{name: "invalid enum value", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "mode=turbo"}, untouched: true},
		{name: "unknown config key", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set", "nope=1"}, untouched: true},
		{name: "dry-run writes nothing", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--dry-run", "--set", "retries=7"}, untouched: true},
		// ── preflight: blocked and its bypass ──
		{name: "tool pin blocks", fixture: "toolpin",
			args: []string{"cfg", "<PROJ>", "--set", "retries=1"}, untouched: true},
		{name: "ignore-cli-version bypasses the pin", fixture: "toolpin",
			args: []string{"cfg", "<PROJ>", "--ignore-cli-version", "--set", "retries=1"}},
		// ── guards: missing recipe, missing manifest ──
		{name: "missing recipe", fixture: "cfg",
			args: []string{"ghost-recipe", "<PROJ>", "--inspect"}, untouched: true},
		{name: "missing manifest", fixture: "empty",
			args: []string{"cfg", "<PROJ>", "--inspect"}, untouched: true},
		// ── the argument contract ──
		{name: "help", fixture: "cfg", args: []string{"--help"}, untouched: true},
		{name: "help at COLUMNS=60", fixture: "cfg", args: []string{"--help"},
			columns: "60", untouched: true},
		{name: "unknown flag", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--bogus"}, untouched: true},
		{name: "missing recipe_id", fixture: "cfg",
			args: []string{"--inspect"}, untouched: true},
		{name: "inspect with set conflict", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--inspect", "--set", "retries=1"}, untouched: true},
		{name: "neither inspect nor set", fixture: "cfg",
			args: []string{"cfg", "<PROJ>"}, untouched: true},
		{name: "ambiguous abbreviation", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--s", "x"}, untouched: true},
		{name: "set without a value", fixture: "cfg",
			args: []string{"cfg", "<PROJ>", "--set"}, untouched: true},
		{name: "positional after dashdash", fixture: "cfg",
			args: []string{"--", "cfg", "<PROJ>", "--inspect"}, untouched: true},
	}

	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
			if tc.fixture != "" {
				buildRecipeConfigureFixture(t, tc.fixture, projA)
				buildRecipeConfigureFixture(t, tc.fixture, projB)
			}
			beforeA := skillsProjectTree(t, projA)
			beforeB := skillsProjectTree(t, projB)

			legArgs := func(proj string) []string {
				out := make([]string, 0, len(tc.args))
				for _, a := range tc.args {
					out = append(out, strings.ReplaceAll(a, "<PROJ>", proj))
				}
				return out
			}
			dirLeg := func(proj string) string {
				if tc.cwdProj {
					return proj
				}
				return scratch
			}
			env := skillsRemoveEnv(home, gate)
			if tc.columns != "" {
				env = append(env, "COLUMNS="+tc.columns)
			}

			legOut, legErr, legCode := runOne("bash",
				append([]string{recipeScript, "configure"}, legArgs(projA)...), env, dirLeg(projA))
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "configure"}, legArgs(projB)...), env, dirLeg(projB))

			normalize := func(b []byte, proj string) string {
				s := string(b)
				// Python's Path.resolve() prints the realpath spelling of the
				// project root inside `manifest not found:`; normalize both
				// spellings of each leg's own root.
				if resolved, err := filepath.EvalSymlinks(proj); err == nil && resolved != proj {
					s = strings.ReplaceAll(s, resolved, "<PROJ>")
				}
				return strings.ReplaceAll(s, proj, "<PROJ>")
			}
			if got, want := normalize(goOut, projB), normalize(legOut, projA); got != want {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if got, want := normalize(goErr, projB), normalize(legErr, projA); got != want {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}

			afterA := skillsProjectTree(t, projA)
			afterB := skillsProjectTree(t, projB)
			if !reflect.DeepEqual(afterA, afterB) {
				t.Errorf("project trees diverge after the two legs:\n bash: %v\n go:   %v", afterA, afterB)
			}
			if tc.untouched {
				if !reflect.DeepEqual(beforeA, afterA) {
					t.Errorf("bash leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeA, afterA)
				}
				if !reflect.DeepEqual(beforeB, afterB) {
					t.Errorf("go leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeB, afterB)
				}
			}
		})
	}

	// The verb writes NOTHING in the CLI home (no cache, no vendor, no
	// bytecode): the snapshot around the whole table must be unchanged.
	if after := skillsTreeSnapshot(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Errorf("the verb wrote into the CLI home:\n before %v\n after  %v", homeBefore, after)
	}
}

// ── GO-20 U6d-1 (T4): the --sync leg over a STUB front door ──
//
// The leg (recipe-configure.py's apply_project tail) runs
// `<home>/bin/ai-specs sync <root>` then `<home>/bin/ai-specs doctor <root>`
// as subprocesses and parses their CAPTURED output: the last `syncing …`
// line names sync.failed_step, the last `Summary: N OK, N INFO, N WARN, N
// ERROR` line becomes verify (see runRecipeConfigure's section header for the
// seam decision). Exercising it against the real sync spine would measure
// sync's own writes and doctor's own diagnostics instead of this verb, so
// both legs spawn the SAME STUB front door here: identical commands, env and
// exit codes, different report plumbing — and the Python reference and the
// native port must still agree byte for byte on stdout, stderr, exit code and
// the written manifest.
func TestRecipeConfigureSyncLegDifferential(t *testing.T) {
	skillsOracle(t, "lib/recipe.sh configure (lib/_internal/recipe-configure.py)")
	bin, root := smokeSetup(t)
	recipeScript := filepath.Join(root, "lib", "recipe.sh")
	if _, err := os.Stat(recipeScript); err != nil {
		t.Skipf("legacy lib/recipe.sh unavailable: %v", err)
	}
	gate := skillsRemoveGateBinary(t)

	// The stub home: lib symlinked (recipe.sh's configure arm resolves the
	// script through AI_SPECS_HOME), bin/ai-specs replaced by a controlled
	// stub, the fixture catalogue seeded as a real directory. No VERSION file
	// (installed = "unknown" is fine: the manifest carries no [tool] policy).
	home := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "lib"), filepath.Join(home, "lib")); err != nil {
		t.Fatal(err)
	}
	// B-004/A-002 (dual-blind review): the stub asserts the FULL spawn
	// contract, not just `$1` — invoked binary path, cwd (the resolved
	// project root), the argv root positional and the flag presence — and
	// records every violation in $STUB_CONTRACT_LOG before exiting 90, so a
	// leg that spawns the wrong command fails on its log AND on the pinned
	// exit codes even if BOTH legs violated identically (a shared wrong
	// answer would otherwise still byte-match).
	skillsWriteFile(t, filepath.Join(home, "bin", "ai-specs"), `#!/bin/sh
contract_fail() {
  echo "$1" >> "${STUB_CONTRACT_LOG:-/dev/null}"
  exit 90
}
[ "$0" = "${STUB_EXPECT_BIN:-}" ] || contract_fail "invoked as '$0', want '${STUB_EXPECT_BIN}'"
[ "$(pwd -P)" = "${STUB_EXPECT_ROOT:-}" ] || contract_fail "cwd '$(pwd -P)', want '${STUB_EXPECT_ROOT}'"
case "$1" in
sync)
  [ "$2" = "${STUB_EXPECT_ROOT:-}" ] || contract_fail "sync root '$2', want '${STUB_EXPECT_ROOT}'"
  if [ "${STUB_EXPECT_IGNORE:-0}" = "1" ]; then
    [ "$#" -eq 3 ] && [ "$3" = "--ignore-cli-version" ] || contract_fail "sync argv '$*' lacks --ignore-cli-version"
  else
    [ "$#" -eq 2 ] || contract_fail "sync argv '$*', want exactly 'sync <root>'"
  fi
  echo "  syncing recipes"
  if [ "${STUB_SYNC_EXIT:-0}" != "0" ]; then
    echo "  x materialize failed" >&2
    exit "${STUB_SYNC_EXIT}"
  fi
  ;;
doctor)
  [ "$2" = "${STUB_EXPECT_ROOT:-}" ] || contract_fail "doctor root '$2', want '${STUB_EXPECT_ROOT}'"
  [ "$#" -eq 2 ] || contract_fail "doctor argv '$*', want exactly 'doctor <root>'"
  if [ -n "${STUB_DOCTOR_SUMMARY:-}" ]; then
    echo "${STUB_DOCTOR_SUMMARY}"
  fi
  exit "${STUB_DOCTOR_EXIT:-0}"
  ;;
*)
  contract_fail "unexpected subcommand '$1'"
  ;;
esac
exit 0
`)
	if err := os.Chmod(filepath.Join(home, "bin", "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	recipeSeedCatalog(t, home, "cfg")
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "cfg", "recipe.toml"),
		recipeConfigureFixtureRecipe)

	type syncCase struct {
		name        string
		env         []string
		ignore      bool // --ignore-cli-version on the verb's own argv
		wantExit    int
		wantContent []string // substrings the report must carry (both legs)
	}
	const summary = "Summary: 1 OK, 0 INFO, 0 WARN, 0 ERROR"
	cases := []syncCase{
		{name: "sync fails: partial with the failed step",
			env:      []string{"STUB_SYNC_EXIT=1"},
			wantExit: 1,
			wantContent: []string{
				`"status": "partial"`, `"ran": true`, `"exit_code": 1`,
				`"failed_step": "recipes"`, `"lock_stamped": false`,
				"sync failed; previous writes were not rolled back and lock CLI version was not stamped",
			}},
		{name: "sync ok, doctor not ok: failed verification",
			env:      []string{"STUB_DOCTOR_EXIT=1", "STUB_DOCTOR_SUMMARY=" + summary},
			wantExit: 1,
			wantContent: []string{
				`"status": "failed"`, `"lock_stamped": true`, `"doctor_exit": 1`,
				`"parsed": true`, `"doctor verification failed"`,
			}},
		{name: "sync and doctor ok: parsed summary, status ok",
			env:      []string{"STUB_DOCTOR_SUMMARY=" + summary},
			wantExit: 0,
			wantContent: []string{
				`"status": "ok"`, `"doctor_exit": 0`, `"parsed": true`,
				`"ok": 1`, `"info": 0`, `"warn": 0`, `"error": 0`,
			}},
		{name: "doctor prints no summary: unparsed verify, still ok",
			env:      nil,
			wantExit: 0,
			wantContent: []string{
				`"status": "ok"`, `"doctor_exit": 0`, `"parsed": false`,
				"\"ok\": null",
			}},
		// A-002/B-004: no case used to combine --sync with
		// --ignore-cli-version, so the flag's trip through the sync argv was
		// unpinned. The stub now requires it as the THIRD argument.
		{name: "sync with --ignore-cli-version passes the flag through",
			env: []string{"STUB_DOCTOR_SUMMARY=" + summary}, ignore: true,
			wantExit: 0,
			wantContent: []string{
				`"status": "ok"`, `"ignore_cli_version": true`,
				`"exit_code": 0`, `"lock_stamped": true`,
			}},
	}
	baseEnv := skillsRemoveEnv(home, gate)
	homeBefore := skillsTreeSnapshot(t, home)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
			buildRecipeConfigureFixture(t, "cfg", projA)
			buildRecipeConfigureFixture(t, "cfg", projB)

			env := append([]string{}, baseEnv...)
			env = append(env, tc.env...)
			ignoreVal := "0"
			if tc.ignore {
				ignoreVal = "1"
			}
			logA := filepath.Join(dirA, "front-door-contract.log")
			logB := filepath.Join(dirB, "front-door-contract.log")
			// The contract expectation is per leg: each leg's own project root
			// (resolved, like both callers resolve it) and its own log.
			envFor := func(proj, contractLog string) []string {
				out := append([]string{}, env...)
				resolved := proj
				if r, err := filepath.EvalSymlinks(proj); err == nil && r != proj {
					resolved = r
				}
				return append(out,
					"STUB_EXPECT_BIN="+filepath.Join(home, "bin", "ai-specs"),
					"STUB_EXPECT_ROOT="+resolved,
					"STUB_EXPECT_IGNORE="+ignoreVal,
					"STUB_CONTRACT_LOG="+contractLog)
			}
			args := func(proj string) []string {
				out := []string{"cfg", proj, "--sync"}
				if tc.ignore {
					out = append(out, "--ignore-cli-version")
				}
				return append(out, "--set", "retries=7")
			}
			legOut, legErr, legCode := runOne("bash",
				append([]string{recipeScript, "configure"}, args(projA)...), envFor(projA, logA), dirA)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "configure"}, args(projB)...), envFor(projB, logB), dirB)
			for _, leg := range []struct {
				name string
				log  string
			}{
				{"bash", logA},
				{"go", logB},
			} {
				if data, rerr := os.ReadFile(leg.log); rerr == nil && len(data) > 0 {
					t.Errorf("%s leg violated the front-door contract (binary path/cwd/argv):\n%s",
						leg.name, data)
				}
			}

			normalize := func(b []byte, proj string) string {
				s := string(b)
				if resolved, err := filepath.EvalSymlinks(proj); err == nil && resolved != proj {
					s = strings.ReplaceAll(s, resolved, "<PROJ>")
				}
				return strings.ReplaceAll(s, proj, "<PROJ>")
			}
			if got, want := normalize(goOut, projB), normalize(legOut, projA); got != want {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if got, want := normalize(goErr, projB), normalize(legErr, projA); got != want {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if goCode != legCode || goCode != tc.wantExit {
				t.Errorf("exit code mismatch: go = %d, bash = %d, want %d",
					goCode, legCode, tc.wantExit)
			}
			for _, want := range tc.wantContent {
				if !strings.Contains(string(goOut), want) {
					t.Errorf("report missing %q:\n%s", want, goOut)
				}
			}
			// The write phase ran identically before the leg: same manifest bytes.
			afterA := skillsProjectTree(t, projA)
			afterB := skillsProjectTree(t, projB)
			if !reflect.DeepEqual(afterA, afterB) {
				t.Errorf("project trees diverge:\n bash: %v\n go:   %v", afterA, afterB)
			}
		})
	}
	if after := skillsTreeSnapshot(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Errorf("the sync leg wrote into the CLI home:\n before %v\n after  %v", homeBefore, after)
	}
}

// TestRecipeConfigureNativeNeedsNoBashOrPython is the routing proof for the
// U6d-1 goal: with PATH emptied the legacy route could not run at all
// (recipe.sh's configure arm execs python3), while the native route builds
// the inspect document itself (S1: zero Python under the product tree). The
// home carries ONLY a catalogue — no lib/, no bin/ — nothing the verb could
// delegate to.
func TestRecipeConfigureNativeNeedsNoBashOrPython(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	recipeSeedCatalog(t, home, "cfg")
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "cfg", "recipe.toml"),
		recipeConfigureFixtureRecipe)
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n\n[recipes.cfg]\nenabled = true\n")
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin, []string{"recipe", "configure", "cfg", project, "--inspect"},
		env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	for _, want := range []string{`"schema_version": 1`, `"id": "cfg"`, `"present_in_manifest": true`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

// TestRecipeConfigureRefusesATraversingRecipeID pins JD-R1-path-traversal-parity
// from the review's risk lens: the script joins the id verbatim, so `..` or an
// ABSOLUTE id resolved the recipe read outside the catalog root and --inspect
// reported an arbitrary directory's recipe.toml. The port refuses before any
// read, with the guard skills remove already uses — a deliberate fail-closed
// divergence, recorded in the unit document.
func TestRecipeConfigureRefusesATraversingRecipeID(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"fixture\"\n")
	env := skillsListEnv(home)

	for _, id := range []string{"../escape", "a/b", "/etc/passwd", "..", "."} {
		out, errOut, code := runOne(bin, []string{"recipe", "configure", id, project, "--inspect"},
			env, t.TempDir())
		if code != 2 {
			t.Errorf("id %q: exit = %d, want 2; stderr=%q", id, code, errOut)
		}
		if !strings.Contains(string(errOut), "refusing invalid recipe id") {
			t.Errorf("id %q: stderr = %q, want the refusal line", id, errOut)
		}
		if strings.Contains(string(out), "recipe.toml") {
			t.Errorf("id %q: stdout = %q, want no report for a refused id", id, out)
		}
	}
}

// TestRecipeConfigureDatetimePathsPinTheRecordedDeviation pins the NATIVE side
// of L6's datetime deviation as the dual-blind review corrected it (doc
// correction (c)): the datetime dies in the REPORT BUILDER, so the verb
// refuses BEFORE any write —
//
//   - --set over a config value that is a TOML datetime: exit 1, Go's
//     serialization error on stderr, NO report, manifest bytes untouched;
//   - --inspect over such a config: the serialization error is a
//     ConfigureError-shaped failure, so it reports exit 3 through
//     rejected(...) with the reason inside the JSON report (NOT exit 1).
//
// The Python reference diverges on BOTH (recorded, not hidden): json.dumps
// only runs at _emit, AFTER apply_project already wrote the manifest (exit 1
// TypeError traceback, file modified) — and its --inspect dies in json.dumps
// with an uncaught TypeError (exit 1, no report) where the port reports 3.
// That side is verified against lib/_internal/recipe-configure.py and
// recorded in the unit document; this pin guards the native behavior.
func TestRecipeConfigureDatetimePathsPinTheRecordedDeviation(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	recipeSeedCatalog(t, home, "cfg")
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "cfg", "recipe.toml"),
		recipeConfigureFixtureRecipe)
	manifestBytes := "[project]\nname = \"u6d1\"\n\n[recipes.cfg]\nenabled = true\n\n[recipes.cfg.config]\nboard_id = 1979-05-27T07:32:00Z\n"
	const serializationError = "Object of type datetime is not JSON serializable"
	env := skillsListEnv(home)

	newFixtureProject := func(t *testing.T) string {
		t.Helper()
		proj := t.TempDir()
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), manifestBytes)
		return proj
	}

	t.Run("--set refuses before any write: exit 1, no report, bytes untouched", func(t *testing.T) {
		proj := newFixtureProject(t)
		out, errOut, code := runOne(bin,
			[]string{"recipe", "configure", "cfg", proj, "--set", "board_id=new"}, env, t.TempDir())
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want NO report (the failure is fatal, before _emit)", out)
		}
		if !strings.Contains(string(errOut), serializationError) {
			t.Errorf("stderr = %q, want the serialization error", errOut)
		}
		data, err := os.ReadFile(filepath.Join(proj, "ai-specs", "ai-specs.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != manifestBytes {
			t.Errorf("manifest was modified before the refusal:\n got %q\nwant %q", data, manifestBytes)
		}
	})

	t.Run("--inspect reports exit 3 through rejected(...)", func(t *testing.T) {
		proj := newFixtureProject(t)
		out, errOut, code := runOne(bin,
			[]string{"recipe", "configure", "cfg", proj, "--inspect"}, env, t.TempDir())
		if code != 3 {
			t.Errorf("exit = %d, want 3 (rejected report, not an uncaught exit 1)", code)
		}
		if len(errOut) != 0 {
			t.Errorf("stderr = %q, want empty (the reason travels inside the report)", errOut)
		}
		for _, want := range []string{
			`"status": "rejected"`,
			`"reason": "` + serializationError + `"`,
		} {
			if !strings.Contains(string(out), want) {
				t.Errorf("stdout missing %q:\n%s", want, out)
			}
		}
		data, err := os.ReadFile(filepath.Join(proj, "ai-specs", "ai-specs.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != manifestBytes {
			t.Errorf("--inspect modified the manifest:\n got %q\nwant %q", data, manifestBytes)
		}
	})
}

// ── GO-20 U6e (T5): `configure-recipes` and the recipe dispatcher remainder ──
//
// Same discipline as the U6a table above: the SHELL route (`bash
// lib/recipe-config.sh <args>` / `bash lib/recipe.sh <args>`) and the built
// binary run against the same project under one shared, pinned environment,
// and stdout, stderr and the exit code must be byte for byte identical. What
// is DIFFERENT here is which paths are reachable: config_wizard.main's TTY
// gate sits BEFORE the recipe enumeration, so off a TTY every byte-comparable
// path is the wrapper's argument contract, the manifest guard and the gate
// message itself — the interactive loop (gate → wizard → write, the
// no-enabled-recipes line and the env offer) is in-process only, driven
// through the injected seams (no PTY in the table, L4's constraint
// unchanged), pinned by TestConfigureRecipes* below.

// configureRecipesUsageFixture is lib/recipe-config.sh's usage() heredoc as
// the LEGACY bytes (the production const mirrors it; the differential is what
// pins the two together).
const configureRecipesUsageFixture = `Usage: ai-specs configure-recipes [path] [--help]
Configure enabled recipes (config fields, CLI deps, ai-specs.env + root .envrc).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
`

// recipeDispatcherUsageFixture is lib/recipe.sh's usage() heredoc (the same
// bytes the dispatcher's native arms print).
const recipeDispatcherUsageFixture = `Usage: ai-specs recipe <subcommand> [args]
Subcommands:
  list [path]      List available and installed recipes
  add <id> [path]  Add a recipe to the manifest
  init <id> [path]   Print a read-only initialization brief
  configure <id> [path] [options]  Inspect or apply recipe config non-interactively
  remove <id> [path]  Remove a recipe from the manifest
Path defaults to current directory.
`

// configureRecipesFixtures builds the project shapes the differential runs:
// an initialized project (a manifest, no enabled recipes — enough to pass the
// guard and measure the TTY gate) and a directory with no ai-specs/ tree.
func configureRecipesFixtures(t *testing.T) (projInit, projNoManifest string) {
	t.Helper()
	base := t.TempDir()
	projInit = filepath.Join(base, "initialized")
	skillsWriteFile(t, filepath.Join(projInit, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"u6e\"\n")
	projNoManifest = filepath.Join(base, "no-manifest")
	if err := os.MkdirAll(projNoManifest, 0o755); err != nil {
		t.Fatal(err)
	}
	return projInit, projNoManifest
}

// TestConfigureRecipesDifferentialAgainstShell is the U6e byte-parity net for
// configure-recipes' NON-TTY paths: both legs are pipes, so the legacy route
// reaches exactly the wrapper loop, the manifest guard and config_wizard
// main's TTY gate — the gate message and exit 3, the guard message and exit
// 1, the usage and both exit-2 usage errors. The interactive loop is NOT in
// this table (it needs a terminal neither leg gets here); it is in-process,
// pinned by TestConfigureRecipesFlow below. A tree snapshot proves the whole
// table writes nothing.
func TestConfigureRecipesDifferentialAgainstShell(t *testing.T) {
	skillsOracle(t, "lib/recipe-config.sh")
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "recipe-config.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/recipe-config.sh unavailable: %v", err)
	}

	home := skillsListHome(t, root)
	projInit, projNoManifest := configureRecipesFixtures(t)

	type diffCase struct {
		name string
		args []string // after `configure-recipes` (Go) / after the script name (bash)
		dir  string   // working directory for both legs; "" → a fresh temp dir
	}
	cases := []diffCase{
		{"help", []string{"--help"}, ""},
		{"short help", []string{"-h"}, ""},
		{"unknown flag", []string{"--bogus"}, ""},
		{"second positional", []string{projInit, "extra"}, ""},
		{"positional after dashdash", []string{"--", projInit}, ""},
		{"missing manifest", []string{projNoManifest}, ""},
		{"initialized project off the TTY", []string{projInit}, ""},
		{"target defaults to cwd off the TTY", nil, projInit},
	}

	roots := []string{home, projInit, projNoManifest}
	before := skillsTreeSnapshot(t, roots...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.dir
			if dir == "" {
				dir = t.TempDir()
			}
			env := skillsListEnv(home)

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, tc.args...), env, dir)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"configure-recipes"}, tc.args...), env, dir)

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

// TestConfigureRecipesNativeNeedsNoBashOrPython is the routing proof for the
// U6e goal: with PATH emptied the legacy route could not run at all (the shim
// execs bash lib/recipe-config.sh → python3), while the native route answers
// the usage, the manifest guard and the TTY gate itself — no bash and no
// Python run on the verb (S1).
func TestConfigureRecipesNativeNeedsNoBashOrPython(t *testing.T) {
	bin, root := smokeSetup(t)
	home := skillsListHome(t, root)
	projInit, projNoManifest := configureRecipesFixtures(t)
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	t.Run("help", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"configure-recipes", "--help"}, env, t.TempDir())
		if code != 0 {
			t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
		}
		if string(out) != configureRecipesUsageFixture {
			t.Errorf("stdout = %q, want the usage heredoc", out)
		}
		if len(errOut) != 0 {
			t.Errorf("stderr = %q, want empty", errOut)
		}
	})
	t.Run("initialized project off the TTY", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"configure-recipes", projInit}, env, t.TempDir())
		if code != 3 {
			t.Errorf("exit = %d, want 3 (the TTY gate), stderr = %q", code, errOut)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want empty", out)
		}
		if string(errOut) != "configure-recipes requires an interactive TTY\n" {
			t.Errorf("stderr = %q, want the frozen gate message", errOut)
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"configure-recipes", projNoManifest}, env, t.TempDir())
		if code != 1 {
			t.Errorf("exit = %d, want 1 (the manifest guard), stderr = %q", code, errOut)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want empty", out)
		}
		want := "Project not initialized: missing " +
			filepath.Join(recipeConfigureResolve(projNoManifest), "ai-specs", "ai-specs.toml") + "\n"
		if string(errOut) != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
	})
}

// TestConfigureRecipesMissingPathDeviation pins the frozen `cd` failure the
// way the U5a/U6a tables do: exit 1 with the `cd: <path>: <reason>` shape,
// WITHOUT bash's own `lib/recipe-config.sh: line N: ` prefix (the S12/ADR
// 0002 class). The legacy leg is checked for the path and the strerror
// wording only, deliberately not byte-matched.
func TestConfigureRecipesMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "recipe-config.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"configure-recipes", missing}, home, nil, &stdout, &stderr)
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

// TestRecipeDispatcherDifferentialAgainstShell byte-compares lib/recipe.sh's
// OWN dispatcher arms against the native route: --help/-h/help on stdout
// (exit 0), the bare `recipe` on stderr (exit 2), unknown subcommands (the
// two lines, exit 1), and `recipe init` with its missing id — since GO-20
// U7-2 the Go verb owns both the brief and the wrapper grammar, so this table
// pins the native grammar against the legacy shell wrapper byte for byte.
func TestRecipeDispatcherDifferentialAgainstShell(t *testing.T) {
	skillsOracle(t, "lib/recipe.sh")
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "recipe.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/recipe.sh unavailable: %v", err)
	}
	home := skillsListHome(t, root)

	cases := []struct {
		name string
		args []string // after `recipe` (Go) / after the script name (bash)
	}{
		{"help flag", []string{"--help"}},
		{"short help flag", []string{"-h"}},
		{"help word", []string{"help"}},
		{"bare recipe", nil},
		{"unknown subcommand", []string{"bogus"}},
		{"unknown flag as subcommand", []string{"-list"}},
		{"near miss of a real subcommand", []string{"listx"}},
		{"init without its id", []string{"init"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env := skillsListEnv(home)

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, tc.args...), env, dir)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe"}, tc.args...), env, dir)

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
}

// TestRecipeDispatcherNativeNeedsNoBashOrPython is the routing proof that the
// recipe DISPATCHER's own arms (usage, bare, unknown) are native: with PATH
// emptied they must print their frozen bytes instead of failing to exec the
// shim. Since GO-20 U7-2 `recipe init` is native too (the wrapper grammar and
// the brief live in internal/recipe), so its help and missing-id arms run with
// no bash and no Python either.
func TestRecipeDispatcherNativeNeedsNoBashOrPython(t *testing.T) {
	bin, root := smokeSetup(t)
	home := skillsListHome(t, root)
	env := skillsListEnv(home)
	emptyPath := t.TempDir()
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	t.Run("help on stdout", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"recipe", "--help"}, env, t.TempDir())
		if code != 0 {
			t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
		}
		if string(out) != recipeDispatcherUsageFixture {
			t.Errorf("stdout = %q, want the usage heredoc", out)
		}
		if len(errOut) != 0 {
			t.Errorf("stderr = %q, want empty", errOut)
		}
	})
	t.Run("bare recipe on stderr", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"recipe"}, env, t.TempDir())
		if code != 2 {
			t.Errorf("exit = %d, want 2 (the frozen bare exit), stderr = %q", code, errOut)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want empty", out)
		}
		if string(errOut) != recipeDispatcherUsageFixture {
			t.Errorf("stderr = %q, want the usage heredoc on stderr", errOut)
		}
	})
	t.Run("unknown subcommand", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"recipe", "bogus"}, env, t.TempDir())
		if code != 1 {
			t.Errorf("exit = %d, want 1 (the frozen unknown exit), stderr = %q", code, errOut)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want empty", out)
		}
		want := "ai-specs recipe: unknown subcommand 'bogus'\nRun 'ai-specs recipe --help' for usage.\n"
		if string(errOut) != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
	})
	t.Run("recipe init help is native", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"recipe", "init", "--help"}, env, t.TempDir())
		if code != 0 {
			t.Errorf("exit = %d, want 0 (no bash/python on PATH), stderr = %q", code, errOut)
		}
		if !strings.HasPrefix(string(out), "Usage: ai-specs recipe init <id> [path] [--help]") {
			t.Errorf("stdout = %q, want the init usage heredoc", out)
		}
		if len(errOut) != 0 {
			t.Errorf("stderr = %q, want empty", errOut)
		}
	})
	t.Run("recipe init missing id is native", func(t *testing.T) {
		out, errOut, code := runOne(bin, []string{"recipe", "init"}, env, t.TempDir())
		if code != 2 {
			t.Errorf("exit = %d, want 2 (no bash/python on PATH), stderr = %q", code, errOut)
		}
		if len(out) != 0 {
			t.Errorf("stdout = %q, want empty", out)
		}
		if !strings.Contains(string(errOut), "ERROR: missing recipe id") ||
			!strings.Contains(string(errOut), "Usage: ai-specs recipe init <id> [path] [--help]") {
			t.Errorf("stderr = %q, want the missing-id error and usage", errOut)
		}
	})
}

// ── the configure-recipes loop, in-process ──

// configureRecipesLoopSeams is the scripted tail for the loop: a recording
// dep gate, a wizard opener handing out one shared scripted prompt set, and a
// recording env offer. The production wiring (recipeDepGate over
// internal/deps.Gate, openRecipeWizardPrompts, sync.OfferHarnessEnv) lives
// behind the same seams the init wizard and this slice's add tail already
// use — the loop
// itself needs a terminal, so it is pinned HERE, never through a PTY.
type configureRecipesLoopSeams struct {
	seams       recipeTailSeams
	gateOutcome wizardDepGateOutcome
	gated       []string
	opened      int
	envRoots    []string
	prompts     *scriptedWizardPrompts
	envOfferErr error
}

func newConfigureRecipesLoopSeams(t *testing.T) *configureRecipesLoopSeams {
	t.Helper()
	c := &configureRecipesLoopSeams{prompts: &scriptedWizardPrompts{t: t}}
	c.seams = recipeTailSeams{
		openWizard: func(stdin io.Reader, stdout io.Writer) (wizardPrompts, func(), int) {
			c.opened++
			return c.prompts, func() {}, 0
		},
		depGate: func(loaded *schema.Recipe) wizardDepGateOutcome {
			c.gated = append(c.gated, loaded.ID)
			return c.gateOutcome
		},
		envOffer: func(root string, _ []string) error {
			c.envRoots = append(c.envRoots, root)
			return c.envOfferErr
		},
	}
	return c
}

// TestConfigureRecipesFlow drives config_wizard.main + configure_selected_recipes
// in-process through the injected seams — the loop needs a terminal, which
// ttyNow stands in for (kernel TTY detection cannot be faked in-process, so
// the interactive surface is pinned HERE and the non-TTY surface by the
// differential above). Every subtest asserts the frozen bytes, the exit code
// and the seam call ORDER the Python makes.
func TestConfigureRecipesFlow(t *testing.T) {
	home := t.TempDir()
	recipeSeedCatalog(t, home, "wired", "nofields", "plain")
	// wired: config fields + cli deps — the whole loop path (gate → wizard →
	// write), with an EXISTING config value so the prefill step is exercised
	// for real (L9 pinned it by shape only; this is its divergent-value case).
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "wired", "recipe.toml"),
		recipeAddWizardFixture("wired", recipeAddCliDepTail))
	// nofields: cli deps and NO config fields — the gate fires BEFORE the
	// fields check (configure_selected_recipes' order), then the wizard never
	// opens.
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "nofields", "recipe.toml"),
		"[recipe]\n"+
			"id = \"nofields\"\n"+
			"name = \"nofields recipe\"\n"+
			"description = \"fixture recipe nofields\"\n"+
			"version = \"1.0.0\"\n"+
			"\n[deps]\ncli = [{ binary = \"jinn\", purpose = \"provider CLI\" }]\n")
	// plain: config fields, no cli deps — the gate must NOT be reached.
	skillsWriteFile(t, filepath.Join(home, "catalog", "recipes", "plain", "recipe.toml"),
		recipeAddWizardFixture("plain", ""))

	// setup writes a fresh project manifest and returns its path.
	setup := func(t *testing.T, body string) (project, manifest string) {
		t.Helper()
		project = t.TempDir()
		manifest = filepath.Join(project, "ai-specs", "ai-specs.toml")
		skillsWriteFile(t, manifest, body)
		return project, manifest
	}
	readManifest := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	runFlow := func(t *testing.T, project string, ttyNow bool, c *configureRecipesLoopSeams) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runConfigureRecipesFlow([]string{project}, home,
			strings.NewReader(""), &stdout, &stderr, ttyNow, c.seams)
		return code, stdout.String(), stderr.String()
	}

	initializedManifest := "[project]\nname = \"u6e\"\n"

	// The manifest guard is config_wizard.main's FIRST check: before the TTY
	// gate, before the recipe enumeration — its frozen message, exit 1.
	t.Run("missing manifest is the guard before anything else", func(t *testing.T) {
		project := t.TempDir()
		c := newConfigureRecipesLoopSeams(t)
		code, out, errOut := runFlow(t, project, true, c)
		if code != 1 {
			t.Errorf("exit = %d, want 1 (the manifest guard)", code)
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		want := "Project not initialized: missing " +
			filepath.Join(recipeConfigureResolve(project), "ai-specs", "ai-specs.toml") + "\n"
		if errOut != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
		if len(c.gated) != 0 || c.opened != 0 || len(c.envRoots) != 0 {
			t.Errorf("no seam may run before the guard: %+v", c)
		}
	})

	// config_wizard.main's TTY gate: the exact message on stderr, exit 3,
	// nothing else touched (off a terminal the loop is unreachable).
	t.Run("non-TTY stops at the gate", func(t *testing.T) {
		project, _ := setup(t, initializedManifest)
		c := newConfigureRecipesLoopSeams(t)
		code, out, errOut := runFlow(t, project, false, c)
		if code != 3 {
			t.Errorf("exit = %d, want 3 (the TTY gate)", code)
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		if errOut != "configure-recipes requires an interactive TTY\n" {
			t.Errorf("stderr = %q, want the frozen gate message", errOut)
		}
		if len(c.gated) != 0 || c.opened != 0 || len(c.envRoots) != 0 {
			t.Errorf("no seam may run past a refused TTY gate: %+v", c)
		}
	})

	// _enabled_recipe_ids is empty → the frozen stdout line, exit 0, and the
	// env offer never runs (main returns before it).
	t.Run("no enabled recipes is the frozen stdout line", func(t *testing.T) {
		project, _ := setup(t, initializedManifest+"\n[recipes.off]\nenabled = false\n")
		c := newConfigureRecipesLoopSeams(t)
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if out != "No enabled recipes found in the manifest.\n" {
			t.Errorf("stdout = %q, want the frozen line", out)
		}
		if errOut != "" {
			t.Errorf("stderr = %q, want empty", errOut)
		}
		if len(c.gated) != 0 || c.opened != 0 || len(c.envRoots) != 0 {
			t.Errorf("no seam may run with no enabled recipes: %+v", c)
		}
	})

	// read_recipes' isinstance gate: `enabled = "true"` is NOT enabled — a
	// string never counts, so the manifest still reports no enabled recipes.
	t.Run("enabled as a string does not count", func(t *testing.T) {
		project, _ := setup(t, initializedManifest+"\n[recipes.strbool]\nenabled = \"true\"\n")
		c := newConfigureRecipesLoopSeams(t)
		code, out, _ := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if out != "No enabled recipes found in the manifest.\n" {
			t.Errorf("stdout = %q, want the frozen line (bool-only enumeration)", out)
		}
	})

	// A manifest that does not PARSE: _enabled_recipe_ids lets the load
	// error escape (Python: traceback, exit 1). The exit code is kept; the
	// wording is ours (ADR 0002), asserted by its frozen prefix.
	t.Run("invalid manifest exits 1 on our own error line", func(t *testing.T) {
		project, _ := setup(t, "[project\n")
		c := newConfigureRecipesLoopSeams(t)
		code, out, errOut := runFlow(t, project, true, c)
		if code != 1 {
			t.Errorf("exit = %d, want 1 (the reference dies uncaught)", code)
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		if !strings.HasPrefix(errOut, "ERROR: configure-recipes failed: ") {
			t.Errorf("stderr = %q, want our own error line with that prefix", errOut)
		}
	})

	// THE LOOP: gate → wizard → write, with the existing config value driving
	// the prefill (L9's promised divergent-value case), then the env offer at
	// the end. Blank answers omit; nothing else prints.
	t.Run("gate wizard write with the existing-wins prefill", func(t *testing.T) {
		project, manifest := setup(t, initializedManifest+
			"\n[recipes.wired]\nenabled = true\n\n[recipes.wired.config]\nboard_id = \"OLD-GATE\"\n")
		c := newConfigureRecipesLoopSeams(t)
		c.gateOutcome = wizardDepGateOutcome{Proceed: true}
		c.prompts.textQ = []scriptedTextAnswer{
			{answers: []string{"NEW-GATE"}}, // board_id — the EXISTING value was the prefill
			{answers: []string{""}},         // default_list — blank → omitted
			{answers: []string{""}},         // note — blank → omitted
		}
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr = %q", code, errOut)
		}
		if out != "" || errOut != "" {
			t.Errorf("the loop prints nothing itself: stdout=%q stderr=%q", out, errOut)
		}
		if len(c.gated) != 1 || c.gated[0] != "wired" {
			t.Errorf("gate calls = %v, want [wired]", c.gated)
		}
		if c.opened != 1 {
			t.Errorf("wizard opens = %d, want 1", c.opened)
		}
		if len(c.envRoots) != 1 || c.envRoots[0] != recipeConfigureResolve(project) {
			t.Errorf("env offer roots = %v, want the resolved project root %q",
				c.envRoots, recipeConfigureResolve(project))
		}
		transcript := strings.Join(c.prompts.transcript, "\n")
		for _, want := range []string{
			`text: wired.board_id (required, string) | def="OLD-GATE"`,
			`text: wired.default_list (optional, string) | def="In Progress"`,
			`text: wired.note (optional, string) | def=""`,
		} {
			if !strings.Contains(transcript, want) {
				t.Errorf("wizard transcript missing %q:\n%s", want, transcript)
			}
		}
		data := readManifest(t, manifest)
		if !strings.Contains(data, `board_id = "NEW-GATE"`) {
			t.Errorf("the wizard's answer was not written back: manifest = %q", data)
		}
		if strings.Contains(data, `board_id = "OLD-GATE"`) {
			t.Errorf("the existing value must be REPLACED by the answer (the preflight transcript pins the OLD prefill): manifest = %q", data)
		}
	})

	// A declined gate: the frozen Skipped line on stderr, no wizard, no write
	// — and the env offer STILL runs (main calls _offer_envrc after the loop
	// whatever the loop did).
	t.Run("gate declined keeps the manifest and still offers env", func(t *testing.T) {
		project, manifest := setup(t, initializedManifest+
			"\n[recipes.wired]\nenabled = true\n\n[recipes.wired.config]\nboard_id = \"OLD-GATE\"\n")
		before := readManifest(t, manifest)
		c := newConfigureRecipesLoopSeams(t)
		c.gateOutcome = wizardDepGateOutcome{}
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr = %q", code, errOut)
		}
		if errOut != "Skipped wired (missing CLI deps)\n" {
			t.Errorf("stderr = %q, want the frozen Skipped line", errOut)
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		if c.opened != 0 {
			t.Errorf("wizard opens = %d, want 0 after a declined gate", c.opened)
		}
		if len(c.envRoots) != 1 {
			t.Errorf("env offer calls = %d, want 1 (main runs it after the loop)", len(c.envRoots))
		}
		if got := readManifest(t, manifest); got != before {
			t.Errorf("a declined gate must leave the manifest untouched:\n got %q\nwant %q", got, before)
		}
	})

	// A gate that could not run: configure_selected_recipes does not catch it
	// (Python: traceback, exit 1) — our own error line, same exit code.
	t.Run("gate failure exits 1 on our own error line", func(t *testing.T) {
		project, _ := setup(t, initializedManifest+"\n[recipes.wired]\nenabled = true\n")
		c := newConfigureRecipesLoopSeams(t)
		c.gateOutcome = wizardDepGateOutcome{Err: errors.New("boom")}
		code, out, errOut := runFlow(t, project, true, c)
		if code != 1 {
			t.Errorf("exit = %d, want 1 (the reference dies uncaught)", code)
		}
		if out != "" {
			t.Errorf("stdout = %q, want empty", out)
		}
		if want := "ERROR: configure-recipes failed: boom\n"; errOut != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
		if c.opened != 0 || len(c.envRoots) != 0 {
			t.Errorf("no wizard, no env offer past a failing gate: %+v", c)
		}
	})

	// The gate fires BEFORE the fields check: a recipe with cli_deps and NO
	// config fields is gated, then silently skipped by the wizard (no fields
	// to prompt), and the env offer still runs.
	t.Run("no config fields gates then skips the wizard", func(t *testing.T) {
		project, manifest := setup(t, initializedManifest+"\n[recipes.nofields]\nenabled = true\n")
		before := readManifest(t, manifest)
		c := newConfigureRecipesLoopSeams(t)
		c.gateOutcome = wizardDepGateOutcome{Proceed: true}
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr = %q", code, errOut)
		}
		if out != "" || errOut != "" {
			t.Errorf("the loop prints nothing for a field-less recipe: stdout=%q stderr=%q", out, errOut)
		}
		if len(c.gated) != 1 || c.gated[0] != "nofields" {
			t.Errorf("gate calls = %v, want [nofields] (the gate precedes the fields check)", c.gated)
		}
		if c.opened != 0 {
			t.Errorf("wizard opens = %d, want 0 (no config fields)", c.opened)
		}
		if len(c.envRoots) != 1 {
			t.Errorf("env offer calls = %d, want 1", len(c.envRoots))
		}
		if got := readManifest(t, manifest); got != before {
			t.Errorf("nothing is written for a field-less recipe:\n got %q\nwant %q", got, before)
		}
	})

	// A recipe without cli_deps never reaches the gate seam at all.
	t.Run("recipe without cli deps never reaches the gate", func(t *testing.T) {
		project, manifest := setup(t, initializedManifest+"\n[recipes.plain]\nenabled = true\n")
		c := newConfigureRecipesLoopSeams(t)
		c.prompts.textQ = []scriptedTextAnswer{
			{answers: []string{"B-9"}},
			{answers: []string{""}},
			{answers: []string{""}},
		}
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr = %q", code, errOut)
		}
		if out != "" || errOut != "" {
			t.Errorf("the loop prints nothing itself: stdout=%q stderr=%q", out, errOut)
		}
		if len(c.gated) != 0 {
			t.Errorf("gate calls = %v, want NONE for a recipe without cli deps", c.gated)
		}
		if c.opened != 1 {
			t.Errorf("wizard opens = %d, want 1", c.opened)
		}
		if !strings.Contains(readManifest(t, manifest), `board_id = "B-9"`) {
			t.Errorf("the wizard's answer was not written back")
		}
	})

	// A cancelled prompt is Python's falsy `values`: the write is skipped,
	// the env offer still runs, exit 0.
	t.Run("cancelled wizard writes nothing", func(t *testing.T) {
		project, manifest := setup(t, initializedManifest+"\n[recipes.plain]\nenabled = true\n")
		before := readManifest(t, manifest)
		c := newConfigureRecipesLoopSeams(t)
		c.prompts.textQ = []scriptedTextAnswer{{answers: nil}} // an empty queue cancels
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr = %q", code, errOut)
		}
		if out != "" || errOut != "" {
			t.Errorf("a cancelled wizard prints nothing itself: stdout=%q stderr=%q", out, errOut)
		}
		if len(c.envRoots) != 1 {
			t.Errorf("env offer calls = %d, want 1 (the offer is outside the loop)", len(c.envRoots))
		}
		if got := readManifest(t, manifest); got != before {
			t.Errorf("a cancelled wizard must not write:\n got %q\nwant %q", got, before)
		}
	})

	// config_wizard._offer_envrc's outer soft-fail: an error escaping the
	// offer words the yellow line (rich Console() → stdout) and configure-
	// recipes still exits 0 — the loop's writes stand.
	t.Run("env offer soft-fails on the Python's line", func(t *testing.T) {
		project, _ := setup(t, initializedManifest+"\n[recipes.plain]\nenabled = true\n")
		c := newConfigureRecipesLoopSeams(t)
		c.prompts.textQ = []scriptedTextAnswer{
			{answers: []string{"B-9"}},
			{answers: []string{""}},
			{answers: []string{""}},
		}
		c.envOfferErr = errors.New("boom")
		code, out, errOut := runFlow(t, project, true, c)
		if code != 0 {
			t.Errorf("exit = %d, want 0 (the offer soft-fails); stderr = %q", code, errOut)
		}
		if want := "Harness env offer skipped: boom\n"; out != want {
			t.Errorf("stdout = %q, want %q (rich Console() → stdout)", out, want)
		}
		if errOut != "" {
			t.Errorf("stderr = %q, want empty", errOut)
		}
	})
}

// TestDepGuidanceDisplay pins dep_install.resolve_install_plan's guidance half
// binary-by-binary: a known formula with brew on PATH renders the brew line
// (shutil.which), without brew (or unknown/npx binaries) it falls to
// `install '<binary>' manually`, and an install_url always wins the fallback.
// The display is what _print_cli_dep_guidance puts on the `  - <binary>: ` line
// (the block bytes themselves are pinned by the guidance subtest above, which
// was RED before the port).
func TestDepGuidanceDisplay(t *testing.T) {
	// A PATH whose directory holds a fake brew: exec.LookPath finds it, like
	// shutil.which. (PATH empty of brew → the guidance fallback.)
	fakeBin := t.TempDir()
	skillsWriteFile(t, filepath.Join(fakeBin, "brew"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "brew"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if got, want := depGuidanceDisplay("jq", ""), "brew install jq"; got != want {
			t.Errorf("brew branch: display = %q, want %q", got, want)
		}
	}

	t.Setenv("PATH", t.TempDir()) // no brew reachable
	if got, want := depGuidanceDisplay("jq", ""), "install 'jq' manually"; got != want {
		t.Errorf("guidance fallback: display = %q, want %q", got, want)
	}
	if got, want := depGuidanceDisplay("npx", ""), "install 'npx' manually"; got != want {
		t.Errorf("guidance-only binary: display = %q, want %q", got, want)
	}
	if got, want := depGuidanceDisplay("unknown-thing", "https://example.com/i"), "https://example.com/i"; got != want {
		t.Errorf("install_url fallback: display = %q, want %q", got, want)
	}
	if got, want := depGuidanceDisplay("bb", ""), "install 'bb' manually"; got != want {
		// bb has no apt package; without brew it is guidance-only too.
		t.Errorf("map entry without brew: display = %q, want %q", got, want)
	}
}

// envScopeRecipeBody is a catalog recipe whose MCP preset declares one $VAR
// harness env reference and nothing else: the only leg `recipe add` reaches is
// the env offer (no config fields, no CLI deps).
func envScopeRecipeBody(id, variable string) string {
	return fmt.Sprintf(`[recipe]
id = %q
name = %q
description = "d"
version = "1.0.0"

[[provides.mcp]]
id = "%s-mcp"
command = "echo"
env = { %s = "$%s" }
`, id, id, id, variable, variable)
}

// S5 (U7-3): the env offer's scope splits the two call sites. `recipe add`
// scopes the offer to the recipe it just added (recipe-add.py:273's
// `recipe_ids=[recipe_id]`), while the init wizard's tail stays aggregate
// (init_tui.py:391 passes nothing). Both run the real sync.OfferHarnessEnv over
// a scripted key source, so the split is proven at the prompt surface and in
// ai-specs.env — not merely at the seam's arguments.
func TestEnvOfferScopeSplitsRecipeAddFromInitWizard(t *testing.T) {
	seed := func(t *testing.T, dir string) {
		t.Helper()
		skillsWriteFile(t, filepath.Join(dir, "a-rec", "recipe.toml"), envScopeRecipeBody("a-rec", "A_TOKEN"))
		skillsWriteFile(t, filepath.Join(dir, "b-rec", "recipe.toml"), envScopeRecipeBody("b-rec", "B_TOKEN"))
	}

	// `recipe add a-rec` on a project that already enables b-rec: both are
	// enabled afterwards, and only the added recipe's variable is offered.
	t.Run("recipe add offers only the added recipe", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // no direnv: the S33 branch, no subprocess
		home := t.TempDir()
		seed(t, filepath.Join(home, "catalog", "recipes"))
		project := t.TempDir()
		skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
			"[project]\nname = \"scope\"\n\n[recipes.b-rec]\nenabled = true\nversion = \"1.0.0\"\n")

		var scoped []string
		var stdout, stderr bytes.Buffer
		keys := wizardScript(append(wizardTyped("ka"), enterText()...), tui.KeyEnter)
		seams := recipeTailSeams{
			openWizard: func(io.Reader, io.Writer) (wizardPrompts, func(), int) {
				return &scriptedWizardPrompts{t: t, confirmQ: []scriptedConfirm{{answer: true, ok: true}}}, func() {}, 0
			},
			envOffer: func(root string, recipeIDs []string) error {
				scoped = recipeIDs
				sync.OfferHarnessEnv(root, home, recipeIDs, &wizardEnvPrompts{out: &stdout, keys: keys}, &stdout, &stderr)
				return nil
			},
		}
		code := runRecipeAddFlow([]string{"recipe", "add", "a-rec", project}, home,
			strings.NewReader(""), &stdout, &stderr, "", true, seams)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr.String())
		}
		if len(scoped) != 1 || scoped[0] != "a-rec" {
			t.Errorf("offer scope = %v, want exactly [a-rec]", scoped)
		}
		out := stdout.String()
		if !strings.Contains(out, "  A_TOKEN — required by a-rec-mcp (a-rec)") {
			t.Errorf("stdout lacks the added recipe's variable:\n%s", out)
		}
		if strings.Contains(out, "B_TOKEN") {
			t.Errorf("the offer of one recipe prompted another enabled recipe's variable:\n%s", out)
		}
		env := envOfferBody(t, filepath.Join(project, "ai-specs.env"))
		if !strings.Contains(env, "A_TOKEN=ka") {
			t.Errorf("ai-specs.env lacks the prompted value:\n%s", env)
		}
		if strings.Contains(env, "B_TOKEN") {
			t.Errorf("ai-specs.env gained an out-of-scope variable:\n%s", env)
		}
	})

	// The init wizard's tail passes no scope: both enabled recipes' variables
	// are offered and both answers are written.
	t.Run("init wizard offers both enabled recipes", func(t *testing.T) {
		f := newWizardFixture(t)
		seed(t, filepath.Join(f.home, "catalog", "recipes"))
		t.Setenv("PATH", t.TempDir()) // no direnv: the S33 branch, no subprocess

		text := enterText() // name: keep "demo"
		text = append(text, wizardTyped("ka")...)
		text = append(text, enterText()...) // A_TOKEN (hidden)
		text = append(text, wizardTyped("kb")...)
		text = append(text, enterText()...) // B_TOKEN (hidden)
		res, rc := runInitWizard(f.request(wizardScript(text,
			tui.KeyEnter, // topology: auto
			tui.KeyEnter, // agents: defaults
			tui.KeySpace, // recipes: toggle a-rec
			tui.KeyDown,  // recipes: move to b-rec
			tui.KeySpace, // recipes: toggle b-rec
			tui.KeyEnter, // recipes: submit
			tui.KeyEnter, // wizard confirm
			tui.KeyEnter, // env offer confirm
		)))
		if rc != 0 {
			t.Fatalf("exit = %d, want 0; stderr = %q", rc, f.stderr.String())
		}
		if res.Staged == "" {
			t.Fatal("the wizard returned no staged manifest")
		}
		t.Cleanup(func() { cleanupInitStaging(res.Staged) })

		out := f.stdout.String()
		for _, want := range []string{
			"  A_TOKEN — required by a-rec-mcp (a-rec)",
			"  B_TOKEN — required by b-rec-mcp (b-rec)",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("the wizard's aggregate offer lacks %q:\n%s", want, out)
			}
		}
		env := envOfferBody(t, filepath.Join(f.target, "ai-specs.env"))
		for _, want := range []string{"A_TOKEN=ka", "B_TOKEN=kb"} {
			if !strings.Contains(env, want) {
				t.Errorf("ai-specs.env lacks %q:\n%s", want, env)
			}
		}
	})
}

// envOfferBody reads a file the env-offer tests assert on.
func envOfferBody(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// GO-20 U2e T2: the front-door re-entry (recipe configure runs `sync` and
// `doctor` through <home>/bin/ai-specs) pins the running binary for the
// launcher, so the launcher never needs a dist/ build inside an installed home.
func TestReentryEnvPinsRunningBinary(t *testing.T) {
	t.Setenv("AI_SPECS_GO_BIN", "/stale/other-build")
	env := reentryEnv("/home/x", "/bin/ai-specs-self")
	var pinned, homes []string
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "AI_SPECS_GO_BIN="):
			pinned = append(pinned, kv)
		case strings.HasPrefix(kv, "AI_SPECS_HOME="):
			homes = append(homes, kv)
		}
	}
	if len(pinned) != 1 || pinned[0] != "AI_SPECS_GO_BIN=/bin/ai-specs-self" {
		t.Fatalf("AI_SPECS_GO_BIN entries = %q", pinned)
	}
	if len(homes) != 1 || homes[0] != "AI_SPECS_HOME=/home/x" {
		t.Fatalf("AI_SPECS_HOME entries = %q", homes)
	}
}
