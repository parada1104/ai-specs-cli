package sync

// Differential contract tests for the native per-recipe loop (GO-07 S14a WU2).
//
// Each scenario is driven twice against the REAL Python orchestration
// (testdata/materialize_ref.py, pinned to the freshly built worktree-gate
// binary) and against sync.MaterializeRecipes, on the SAME project and home
// paths so every absolute path (cache roots, command destinations, dep sources)
// is byte-identical. Between the two legs the project tree and the CLI cache
// are rebuilt from the scenario definition, so each leg starts from the same
// pristine state and the comparison is of behaviour, not of order. The
// comparison covers rc, stdout, stderr, the three documents, the ENTIRE project
// + cache file tree and the .ai-specs.lock bytes.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/gatebinary"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// materializeLoopRecipe is one catalog recipe directory: its recipe.toml body
// plus every extra file written under the recipe dir, keyed by slash path.
type materializeLoopRecipe struct {
	body  string
	files map[string]string
}

// materializeLoopScenario is one differential case.
type materializeLoopScenario struct {
	name string
	// recipes is the catalog, keyed by recipe id.
	recipes map[string]materializeLoopRecipe
	// manifest is <project>/ai-specs/ai-specs.toml.
	manifest string
	// projectSeed is written under the project before each leg.
	projectSeed map[string]string
	// cacheSeed is written under the project's cache root before each leg
	// (the cache root is wiped between legs).
	cacheSeed map[string]string
	// depSource is written under <home>/dep-src; recipe bodies may reference it
	// with the __DEP_SRC__ token.
	depSource         map[string]string
	recipeMCPOut      bool
	resolvedConfigOut bool
	resolvedHooksOut  bool
	refreshGates      bool
	lockManaged       map[string]string // target -> sha256, seeded into the lock
	addLibSymlink     bool              // provide <home>/lib -> repo lib for delegations
}

func materializeLoopRecipeBody(id, name, extra string) materializeLoopRecipe {
	return materializeLoopRecipe{
		body:  syncRecipeHeader(id, name) + "\n" + extra,
		files: map[string]string{},
	}
}

// materializeLoopProvision (re)creates the scenario on the fixed project/home
// paths: the project tree (manifest, seeds, lock) and the CLI cache. The
// catalog, the lib symlink and the dep source are written once by the caller.
func materializeLoopProvision(t *testing.T, sc materializeLoopScenario, home, project string) {
	t.Helper()
	if err := os.RemoveAll(project); err != nil {
		t.Fatalf("reset project: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "ai-specs"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeMaterializeManifest(t, project, []byte(sc.manifest))
	for rel, content := range sc.projectSeed {
		path := filepath.Join(project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir seed %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write seed %s: %v", rel, err)
		}
	}
	if len(sc.lockManaged) > 0 {
		lk := lock.NewLock()
		for target, sha := range sc.lockManaged {
			lock.SetManagedOverride(lk, target, sha, nil, nil, nil, nil)
		}
		if err := lock.WriteLock(filepath.Join(project, "ai-specs", ".ai-specs.lock"), lk); err != nil {
			t.Fatalf("seed lock: %v", err)
		}
	}
	cache := projectcache.CacheRoot(project, home)
	if err := os.RemoveAll(cache); err != nil {
		t.Fatalf("reset cache: %v", err)
	}
	for rel, content := range sc.cacheSeed {
		path := filepath.Join(cache, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir cache seed %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write cache seed %s: %v", rel, err)
		}
	}
}

// materializeLoopWriteStatic writes the parts of the scenario that survive both
// legs: the catalog, the lib symlink the delegations resolve through, and the
// dep source.
func materializeLoopWriteStatic(t *testing.T, sc materializeLoopScenario, home string) {
	t.Helper()
	for id, recipe := range sc.recipes {
		dir := filepath.Join(home, "catalog", "recipes", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir recipe %s: %v", id, err)
		}
		body := strings.ReplaceAll(recipe.body, "__DEP_SRC__", filepath.Join(home, "dep-src"))
		if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write recipe %s: %v", id, err)
		}
		for rel, content := range recipe.files {
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir recipe file %s: %v", rel, err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("write recipe file %s: %v", rel, err)
			}
		}
	}
	if sc.addLibSymlink {
		repoLib, err := filepath.Abs(filepath.Join("..", "..", "lib"))
		if err != nil {
			t.Fatalf("resolve repo lib: %v", err)
		}
		if err := os.Symlink(repoLib, filepath.Join(home, "lib")); err != nil {
			t.Fatalf("symlink lib: %v", err)
		}
	}
	for rel, content := range sc.depSource {
		path := filepath.Join(home, "dep-src", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir dep %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write dep %s: %v", rel, err)
		}
	}
}

// materializeLoopTree snapshots every regular file's bytes under the project
// and the CLI cache. cache meta.toml carries a wall-clock created_at, so it is
// excluded (it is not materialized content).
func materializeLoopTree(t *testing.T, home, project string) map[string]string {
	t.Helper()
	out := map[string]string{}
	collect := func(root, prefix string, skip func(rel string) bool) {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if skip != nil && skip(rel) {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			out[prefix+"/"+rel] = string(data)
			return nil
		})
	}
	collect(project, "PROJECT", nil)
	collect(filepath.Join(home, "cache"), "CACHE", func(rel string) bool {
		return filepath.Base(rel) == "meta.toml"
	})
	return out
}

func materializeLoopDiffTree(t *testing.T, want, got map[string]string) {
	t.Helper()
	keys := map[string]bool{}
	for key := range want {
		keys[key] = true
	}
	for key := range got {
		keys[key] = true
	}
	names := make([]string, 0, len(keys))
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		if want[key] != got[key] {
			t.Fatalf("%s differs\n--- python ---\n%q\n--- go ---\n%q", key, want[key], got[key])
		}
	}
}

func TestMaterializeLoopDifferential(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	_ = python
	authority, err := filepath.Abs(filepath.Join("..", "..", "lib", "_internal", "recipe-materialize.py"))
	if err != nil {
		t.Fatalf("resolve authority: %v", err)
	}
	if _, err := os.Stat(authority); err != nil {
		t.Skipf("recipe-materialize.py unavailable: %v", err)
	}
	driver := materializeRefPath(t)
	gate := materializeGateBinary(t)
	t.Setenv("AI_SPECS_GATE_OFFLINE", "1")

	for _, scenario := range materializeLoopScenarios() {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			home := t.TempDir()
			project := filepath.Join(t.TempDir(), "project")
			materializeLoopWriteStatic(t, scenario, home)

			pyDir := t.TempDir()
			goDir := t.TempDir()
			pyMCP, goMCP := "-", "-"
			if scenario.recipeMCPOut {
				pyMCP = filepath.Join(pyDir, "recipe-mcp.json")
				goMCP = filepath.Join(goDir, "recipe-mcp.json")
			}
			pyConfig, goConfig := "-", "-"
			if scenario.resolvedConfigOut {
				pyConfig = filepath.Join(pyDir, "resolved-config.json")
				goConfig = filepath.Join(goDir, "resolved-config.json")
			}
			pyHooks, goHooks := "-", "-"
			if scenario.resolvedHooksOut {
				pyHooks = filepath.Join(pyDir, "resolved-hooks.json")
				goHooks = filepath.Join(goDir, "resolved-hooks.json")
			}

			materializeLoopProvision(t, scenario, home, project)
			ref := runMaterializeLoopRef(t, gate, driver, authority, project, home, pyMCP, pyConfig, pyHooks, scenario.refreshGates)
			pyTree := materializeLoopTree(t, home, project)

			materializeLoopProvision(t, scenario, home, project)
			var mcpOut, configOut, hooksOut *string
			if scenario.recipeMCPOut {
				mcpOut = &goMCP
			}
			if scenario.resolvedConfigOut {
				configOut = &goConfig
			}
			if scenario.resolvedHooksOut {
				hooksOut = &goHooks
			}
			var stdout, stderr bytes.Buffer
			result, goErr := MaterializeRecipes(MaterializeRequest{
				ProjectRoot:       project,
				AISpecsHome:       home,
				RecipeMCPOut:      mcpOut,
				ResolvedConfigOut: configOut,
				ResolvedHooksOut:  hooksOut,
				RefreshGates:      scenario.refreshGates,
				Stdout:            &stdout,
				Stderr:            &stderr,
			})
			goRC := 0
			if goErr != nil {
				goRC = 1
			}
			goTree := materializeLoopTree(t, home, project)

			if goRC != ref.RC {
				t.Fatalf("rc: go=%d python=%d\n--- go stderr ---\n%s\n--- python stderr ---\n%s", goRC, ref.RC, stderr.String(), ref.Stderr)
			}
			if normalizeRecipeMCPPath(stdout.String()) != normalizeRecipeMCPPath(ref.Stdout) {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- python ---\n%q", stdout.String(), ref.Stdout)
			}
			if stderr.String() != ref.Stderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- python ---\n%q", stderr.String(), ref.Stderr)
			}
			if scenario.recipeMCPOut {
				assertDocumentEqual(t, "recipe-mcp", result.RecipeMCPDocument, ref.RecipeMCP)
			}
			if scenario.resolvedConfigOut {
				assertDocumentEqual(t, "resolved-config", result.ResolvedConfigDocument, ref.ResolvedConfig)
			}
			if scenario.resolvedHooksOut {
				assertDocumentEqual(t, "resolved-hooks", result.ResolvedHooksDocument, ref.ResolvedHooks)
			}
			materializeLoopDiffTree(t, pyTree, goTree)
		})
	}
}

// materializeLoopHash is sha256 over normalized bytes, matching the lock's own
// hash so pre-seeded managed entries classify against the same value.
func materializeLoopHash(content string) string {
	return lock.Sha256Bytes([]byte(content))
}

// runMaterializeLoopRef drives the Python oracle with the scenario's
// refresh-gates flag (the driver reads MATERIALIZE_REFRESH_GATES).
func runMaterializeLoopRef(t *testing.T, gate, driver, authority, project, home, mcpOut, configOut, hooksOut string, refreshGates bool) materializeRefOut {
	t.Helper()
	cmd := exec.Command("python3", driver, authority, project, home, mcpOut, configOut, hooksOut)
	cmd.Dir = t.TempDir()
	env := append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"WORKTREE_GATE_BIN="+gate,
		"AI_SPECS_GATE_OFFLINE=1",
	)
	if refreshGates {
		env = append(env, "MATERIALIZE_REFRESH_GATES=1")
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	var out materializeRefOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode ref %s: %v", stdout.String(), err)
	}
	return out
}

const materializeLoopHookScript = "#!/bin/sh\necho guard\n"

// materializeLoopHookRel is the materialized hook path for recipe alpha.
const materializeLoopHookRel = "ai-specs/recipes/alpha/hooks/guard.sh"

func materializeLoopScenarios() []materializeLoopScenario {
	skillRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[[provides.skills]]
id = "demo"
source = "bundled"

[[provides.commands]]
id = "review"
path = "commands/review.md"
`,
		files: map[string]string{
			"skills/demo/SKILL.md":     "# demo\n",
			"skills/demo/assets/a.txt": "asset\n",
			"commands/review.md":       "review body\n",
		},
	}

	templateRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[[provides.templates]]
source = "templates/t.txt"
target = "docs/fresh.txt"

[[provides.templates]]
source = "templates/t.txt"
target = "docs/current.txt"

[[provides.templates]]
source = "templates/t.txt"
target = "docs/user.txt"
`,
		files: map[string]string{"templates/t.txt": "hello\n"},
	}

	docRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[[provides.docs]]
source = "docs/fresh.md"
target = "guide/fresh.md"

[[provides.docs]]
source = "docs/cur.md"
target = "guide/cur.md"

[[provides.docs]]
source = "docs/user.md"
target = "guide/user.md"
`,
		files: map[string]string{
			"docs/fresh.md": "fresh doc\n",
			"docs/cur.md":   "cur doc\n",
			"docs/user.md":  "user doc\n",
		},
	}

	hookRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[[provides.hooks]]
id = "guard"
event = "pre-tool-use"
script = "hooks/guard.sh"
matcher = "Bash"
blocking = true
`,
		files: map[string]string{"hooks/guard.sh": materializeLoopHookScript},
	}

	envRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[config.A1_B]
required = false
default = "one"

[config.1_A]
required = false
default = 2

[config.a_B]
required = false
default = "three"

[config.A-B]
required = false
default = "four"

[config.lower]
required = false
default = "five"

[[provides.hooks]]
id = "guard"
event = "stop"
script = "hooks/guard.sh"
`,
		files: map[string]string{"hooks/guard.sh": materializeLoopHookScript},
	}

	failureRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[config.board_id]
required = false
default = "abcd1234"

[[hooks]]
event = "pre-sync"
action = "validate-config"
`,
		files: map[string]string{},
	}

	depRecipe := materializeLoopRecipe{
		body: syncRecipeHeader("alpha", "Alpha") + `
[[provides.skills]]
id = "dep1"
source = "dep"
url = "__DEP_SRC__"
`,
		files: map[string]string{},
	}

	gateRecipe := materializeLoopRecipe{
		body:  syncRecipeHeader("worktree-flow", "Worktree Flow") + "\n",
		files: map[string]string{},
	}
	// The offline gate-acquisition scenario seeds a committed trust root: since
	// WU3 the native acquisition fails closed without a committed digest (the
	// decided same-user boundary), a branch where Python instead attempts the
	// local build. The digest pins both graders to the shared local-build
	// branch, whose failure warning stays byte-identical.
	gateGoos, gateGoarch := gatebinary.DetectHostPlatform()
	gateRecipe.files["bin/SHA256SUMS"] = strings.Repeat("a", 64) + "  worktree-gate-" +
		gateGoos + "-" + gateGoarch + "\n"

	return []materializeLoopScenario{
		{
			name:         "bundled skill and command with an overwrite warning and the lock bytes",
			recipes:      map[string]materializeLoopRecipe{"alpha": skillRecipe},
			manifest:     "[recipes.alpha]\nenabled = true\n",
			cacheSeed:    map[string]string{"commands/review.md": "stale command\n"},
			recipeMCPOut: true,
		},
		{
			name:     "templates fresh managed-current and user-modified",
			recipes:  map[string]materializeLoopRecipe{"alpha": templateRecipe},
			manifest: "[recipes.alpha]\nenabled = true\n",
			projectSeed: map[string]string{
				"docs/current.txt": "hello\n",
				"docs/user.txt":    "edited\n",
			},
			lockManaged: map[string]string{
				"docs/current.txt": materializeLoopHash("hello\n"),
				"docs/user.txt":    materializeLoopHash("hello\n"),
			},
		},
		{
			name:     "docs fresh managed-current and user-modified",
			recipes:  map[string]materializeLoopRecipe{"alpha": docRecipe},
			manifest: "[recipes.alpha]\nenabled = true\n",
			projectSeed: map[string]string{
				"guide/cur.md":  "cur doc\n",
				"guide/user.md": "mine\n",
			},
			lockManaged: map[string]string{
				"guide/cur.md":  materializeLoopHash("cur doc\n"),
				"guide/user.md": materializeLoopHash("user doc\n"),
			},
		},
		{
			name:             "runtime hook fresh collects the resolved entry",
			recipes:          map[string]materializeLoopRecipe{"alpha": hookRecipe},
			manifest:         "[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n",
			resolvedHooksOut: true,
		},
		{
			name:             "runtime hook managed-current is skipped and baselined",
			recipes:          map[string]materializeLoopRecipe{"alpha": hookRecipe},
			manifest:         "[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n",
			projectSeed:      map[string]string{materializeLoopHookRel: materializeLoopHookScript},
			lockManaged:      map[string]string{materializeLoopHookRel: materializeLoopHash(materializeLoopHookScript)},
			resolvedHooksOut: true,
		},
		{
			name:             "runtime hook user-modified is preserved",
			recipes:          map[string]materializeLoopRecipe{"alpha": hookRecipe},
			manifest:         "[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n",
			projectSeed:      map[string]string{materializeLoopHookRel: "#!/bin/sh\necho edited\n"},
			lockManaged:      map[string]string{materializeLoopHookRel: materializeLoopHash(materializeLoopHookScript)},
			resolvedHooksOut: true,
		},
		{
			name:             "runtime hook --refresh-gates backs up and rewrites",
			recipes:          map[string]materializeLoopRecipe{"alpha": hookRecipe},
			manifest:         "[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n",
			projectSeed:      map[string]string{materializeLoopHookRel: "#!/bin/sh\necho edited\n"},
			lockManaged:      map[string]string{materializeLoopHookRel: materializeLoopHash(materializeLoopHookScript)},
			resolvedHooksOut: true,
			refreshGates:     true,
		},
		{
			name:             "resolved-hooks env exports only the UPPER env-shaped keys",
			recipes:          map[string]materializeLoopRecipe{"alpha": envRecipe},
			manifest:         "[recipes.alpha]\nenabled = true\n",
			resolvedHooksOut: true,
		},
		{
			name:          "dep skill through the documented delegation",
			recipes:       map[string]materializeLoopRecipe{"alpha": depRecipe},
			manifest:      "[recipes.alpha]\nenabled = true\n",
			addLibSymlink: true,
			depSource:     map[string]string{"SKILL.md": "# dep1\n\nbody\n", "assets/x.txt": "x\n"},
		},
		{
			name:         "execute_hooks failure fails the sync with the fatal line",
			recipes:      map[string]materializeLoopRecipe{"alpha": failureRecipe},
			manifest:     "[recipes.alpha]\nenabled = true\n",
			recipeMCPOut: false,
		},
		{
			name:          "worktree-flow gate acquisition degrades offline",
			recipes:       map[string]materializeLoopRecipe{"worktree-flow": gateRecipe},
			manifest:      "[recipes.worktree-flow]\nenabled = true\n",
			addLibSymlink: true,
			recipeMCPOut:  true,
		},
	}
}
