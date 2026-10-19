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
	skillsWriteFile(t, filepath.Join(home, "bin", "ai-specs"), `#!/bin/sh
case "$1" in
sync)
  echo "  syncing recipes"
  if [ "${STUB_SYNC_EXIT:-0}" != "0" ]; then
    echo "  x materialize failed" >&2
    exit "${STUB_SYNC_EXIT}"
  fi
  ;;
doctor)
  if [ -n "${STUB_DOCTOR_SUMMARY:-}" ]; then
    echo "${STUB_DOCTOR_SUMMARY}"
  fi
  exit "${STUB_DOCTOR_EXIT:-0}"
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
			args := func(proj string) []string {
				return []string{"cfg", proj, "--sync", "--set", "retries=7"}
			}
			legOut, legErr, legCode := runOne("bash",
				append([]string{recipeScript, "configure"}, args(projA)...), env, dirA)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"recipe", "configure"}, args(projB)...), env, dirB)

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
