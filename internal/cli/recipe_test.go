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
	"regexp"
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
// TTY interactive branches (the ensure_deps pre-gate and recipe-add.py:262-275's
// "Configure now?" wizard + env offer) stay on the legacy script through
// runRecipeAdd's documented seam — pinned by
// TestRecipeAddTTYSeamStaysOnLegacyScript and recorded in L4 (option (a)).

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

// TestRecipeAddTTYSeamStaysOnLegacyScript pins the U6b TTY decision (option
// (a), recorded in L4): on a terminal the WHOLE verb stays on the legacy
// lib/recipe.sh → recipe-add.sh route through a documented seam, because
// add_recipe's interactive branches (recipe-add.py:155-174's ensure_deps
// pre-gate and :262-275's "Configure now?" wizard + env offer) are not
// half-ported. Off the TTY — piped/CI, and both legs of the differential —
// the native route owns the verb end to end: the stub script must not run
// and the manifest append must come from the port.
func TestRecipeAddTTYSeamStaysOnLegacyScript(t *testing.T) {
	home := t.TempDir()
	skillsWriteFile(t, filepath.Join(home, "lib", "recipe.sh"),
		"#!/usr/bin/env bash\nprintf 'legacy seam: %s\\n' \"$*\"\nprintf ran > \"$AI_SPECS_HOME/legacy-ran\"\n")
	recipeSeedCatalog(t, home, "alpha")
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"seam\"\n")
	manifest := filepath.Join(project, "ai-specs", "ai-specs.toml")
	marker := filepath.Join(home, "legacy-ran")

	// TTY → recipe.sh's `add` arm runs, with the post-shift argv.
	var stdout, stderr bytes.Buffer
	code := runRecipeAdd([]string{"recipe", "add", "alpha", project}, home,
		strings.NewReader(""), &stdout, &stderr, "", true)
	if code != 0 {
		t.Fatalf("tty leg exit = %d, want 0; stderr = %q", code, stderr.String())
	}
	if want := "legacy seam: add alpha " + project + "\n"; stdout.String() != want {
		t.Errorf("tty stdout = %q, want %q", stdout.String(), want)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the legacy script did not run on the TTY: %v", err)
	}
	if data, err := os.ReadFile(manifest); err != nil || strings.Contains(string(data), "[recipes.alpha]") {
		t.Errorf("the seam stub must not append (err=%v, manifest=%q)", err, data)
	}

	// non-TTY → the seam never fires: the port appends the block itself.
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = runRecipeAdd([]string{"recipe", "add", "alpha", project}, home,
		strings.NewReader(""), &stdout, &stderr, "", false)
	if code != 0 {
		t.Fatalf("non-tty leg exit = %d, want 0; stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("the legacy script ran on a non-TTY (marker stat err=%v)", err)
	}
	if want := "Recipe 'alpha' added to the manifest.\nThe next sync will materialize:\n  (no primitives declared)\n"; stdout.String() != want {
		t.Errorf("non-tty stdout = %q, want %q", stdout.String(), want)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[recipes.alpha]\nenabled = true\n") {
		t.Errorf("manifest = %q, want the native [recipes.alpha] append", data)
	}
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
