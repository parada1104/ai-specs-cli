package sync

// Differential contract tests for the native materializer spine (GO-07 S14a
// WU1). The Go authority is unwired; these tests drive the REAL Python
// orchestration through testdata/materialize_ref.py and require byte equality
// of the emitted lines and documents.
//
// The Python leg is pinned to the freshly built worktree-gate binary through
// WORKTREE_GATE_BIN so the fail-open GO_*_BRIDGE_FALLBACK warnings never mask
// the orchestration under test. Both legs run against the SAME project and home
// directories, so any absolute path embedded in a document is identical.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- harness ----------------------------------------------------------------

func materializeRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "materialize_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

var materializeGateDir string

// materializeGateBinary builds the worktree-gate binary once per test run and
// returns its path. The Python bridges require a verified binary; without it
// every bridge announces a GO_*_BRIDGE_FALLBACK warning that is not part of the
// orchestration under test.
func materializeGateBinary(t *testing.T) string {
	t.Helper()
	if materializeGateDir != "" {
		return filepath.Join(materializeGateDir, "worktree-gate")
	}
	module, err := filepath.Abs(filepath.Join("..", "..", "catalog", "recipes", "worktree-flow", "gate"))
	if err != nil {
		t.Fatalf("resolve gate module: %v", err)
	}
	dir, err := os.MkdirTemp("", "materialize-gate-")
	if err != nil {
		t.Fatalf("temp gate dir: %v", err)
	}
	out := filepath.Join(dir, "worktree-gate")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = module
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("worktree-gate build unavailable: %v: %s", err, stderr.String())
	}
	materializeGateDir = dir
	return out
}

type materializeRefOut struct {
	RC             int    `json:"rc"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	RecipeMCP      string `json:"recipe_mcp"`
	ResolvedConfig string `json:"resolved_config"`
	ResolvedHooks  string `json:"resolved_hooks"`
}

func runMaterializeRef(t *testing.T, gate, driver, authority, project, home, mcpOut, configOut, hooksOut string) materializeRefOut {
	t.Helper()
	cmd := exec.Command("python3", driver, authority, project, home, mcpOut, configOut, hooksOut)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"WORKTREE_GATE_BIN="+gate,
		"AI_SPECS_GATE_OFFLINE=1",
	)
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

// normalizeRecipeMCPPath replaces the nondeterministic mkstemp path on the
// RECIPE_MCP_TEMP marker so the two legs' marker lines compare equal.
func normalizeRecipeMCPPath(stdout string) string {
	lines := strings.Split(stdout, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			lines[i] = "RECIPE_MCP_TEMP:<TEMP>"
		}
	}
	return strings.Join(lines, "\n")
}

// markerPath extracts the path the leg printed on the RECIPE_MCP_TEMP marker.
func markerPath(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			return strings.TrimPrefix(line, "RECIPE_MCP_TEMP:")
		}
	}
	return ""
}

// --- fixture ----------------------------------------------------------------

// materializeFixture writes one project manifest and its catalog recipes. The
// returned paths are the SAME for both legs.
type materializeFixture struct {
	home    string
	project string
}

func writeMaterializeFixture(t *testing.T, recipes map[string]string, manifest string) materializeFixture {
	t.Helper()
	home := t.TempDir()
	project := t.TempDir()
	catalog := filepath.Join(home, "catalog", "recipes")
	for id, body := range recipes {
		dir := filepath.Join(catalog, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir recipe %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write recipe %s: %v", id, err)
		}
	}
	aiSpecs := filepath.Join(project, "ai-specs")
	if err := os.MkdirAll(aiSpecs, 0o755); err != nil {
		t.Fatalf("mkdir ai-specs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aiSpecs, "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return materializeFixture{home: home, project: project}
}

// --- scenarios --------------------------------------------------------------

type materializeScenario struct {
	name              string
	recipes           map[string]string
	manifest          string
	recipeMCPDoc      bool
	recipeMCPOut      bool
	resolvedConfigOut bool
	resolvedHooksOut  bool
}

func materializeRecipeBody(id, name string) string {
	return syncRecipeHeader(id, name)
}

func materializeRecipeBodyWith(text string) string {
	return text
}

func TestMaterializeSpineDifferential(t *testing.T) {
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

	scenarios := []materializeScenario{
		{
			name: "no-enabled branch writes both temp documents",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:          "[recipes.alpha]\nenabled = false\n",
			resolvedConfigOut: true,
			resolvedHooksOut:  true,
		},
		{
			name: "single enabled recipe and the empty resolved-hooks dump",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:         "[agents]\nenabled = [\"claude\"]\n\n[recipes.alpha]\nenabled = true\n",
			recipeMCPDoc:     true,
			resolvedHooksOut: true,
		},
		{
			name: "several enabled recipes in manifest order with an explicit recipe-MCP path",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
				"beta":  materializeRecipeBody("beta", "Beta"),
				"gamma": materializeRecipeBody("gamma", "Gamma"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n\n[recipes.gamma]\nenabled = true\n",
			recipeMCPDoc: true,
			recipeMCPOut: true,
		},
		{
			name: "recipe-MCP temp write without an output path prints the marker",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n",
			recipeMCPDoc: true,
		},
		{
			name: "fatal primitive conflict",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") + "[[provides.skills]]\nid = \"dup\"\nsource = \"bundled\"\npath = \"skills/dup\"\n"),
				"beta":  materializeRecipeBodyWith(syncRecipeHeader("beta", "Beta") + "[[provides.skills]]\nid = \"dup\"\nsource = \"bundled\"\npath = \"skills/dup\"\n"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			name: "schema-invalid primitive-coherent recipes report the same fatal conflict",
			recipes: map[string]string{
				"alpha": "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n\n[[provides.skills]]\nid = \"dup\"\n",
				"beta":  "[recipe]\nid = \"beta\"\nname = \"Beta\"\n\n[[provides.skills]]\nid = \"dup\"\n",
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			name: "schema-invalid tagged recipes report the same tag warning and fatal conflict",
			recipes: map[string]string{
				"alpha": "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\ntags = [\"vcs\"]\nconflicts_with = [\"beta\"]\n\n[[provides.skills]]\nid = \"dup\"\n",
				"beta":  "[recipe]\nid = \"beta\"\nname = \"Beta\"\ntags = [\"vcs\"]\n\n[[provides.skills]]\nid = \"dup\"\n",
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			name: "advisory tag conflict",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") + "tags = [\"vcs\"]\n"),
				"beta":  materializeRecipeBodyWith(syncRecipeHeader("beta", "Beta") + "tags = [\"vcs\"]\n"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: true,
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			fixture := writeMaterializeFixture(t, scenario.recipes, scenario.manifest)
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

			ref := runMaterializeRef(t, gate, driver, authority, fixture.project, fixture.home, pyMCP, pyConfig, pyHooks)

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
				ProjectRoot:       fixture.project,
				AISpecsHome:       fixture.home,
				RecipeMCPOut:      mcpOut,
				ResolvedConfigOut: configOut,
				ResolvedHooksOut:  hooksOut,
				Stdout:            &stdout,
				Stderr:            &stderr,
			})
			goRC := 0
			if goErr != nil {
				goRC = 1
			}
			if goRC != ref.RC {
				t.Fatalf("rc: go=%d python=%d\n--- go stderr ---\n%s\n--- python stderr ---\n%s", goRC, ref.RC, stderr.String(), ref.Stderr)
			}
			if normalizeRecipeMCPPath(stdout.String()) != normalizeRecipeMCPPath(ref.Stdout) {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- python ---\n%q", stdout.String(), ref.Stdout)
			}
			if stderr.String() != ref.Stderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- python ---\n%q", stderr.String(), ref.Stderr)
			}

			// recipe-MCP document: explicit path or the marker's temp file.
			switch {
			case scenario.recipeMCPOut:
				assertDocumentEqual(t, "recipe-mcp", result.RecipeMCPDocument, ref.RecipeMCP)
			case scenario.recipeMCPDoc:
				pyTemp := markerPath(ref.Stdout)
				goTemp := markerPath(stdout.String())
				if pyTemp == "" || goTemp == "" {
					t.Fatalf("RECIPE_MCP_TEMP marker missing: go=%q python=%q", stdout.String(), ref.Stdout)
				}
				pyDoc, err := os.ReadFile(pyTemp)
				if err != nil {
					t.Fatalf("read python temp: %v", err)
				}
				goDoc, err := os.ReadFile(goTemp)
				if err != nil {
					t.Fatalf("read go temp: %v", err)
				}
				assertDocumentEqual(t, "recipe-mcp", string(goDoc), string(pyDoc))
				if result.RecipeMCPPath != goTemp {
					t.Fatalf("result path = %q, want the marker path %q", result.RecipeMCPPath, goTemp)
				}
			}

			if scenario.resolvedConfigOut {
				assertDocumentEqual(t, "resolved-config", result.ResolvedConfigDocument, ref.ResolvedConfig)
			}
			if scenario.resolvedHooksOut {
				assertDocumentEqual(t, "resolved-hooks", result.ResolvedHooksDocument, ref.ResolvedHooks)
			}
		})
	}
}

func assertDocumentEqual(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s document:\n--- go ---\n%q\n--- python ---\n%q", label, got, want)
	}
}

// --- stamp-step manifest differential (GO-07 S14a follow-up) -----------------
//
// The stamp step MUTATES the user's manifest: the reconcile defaults the gate
// plans are written into [recipes.<id>.config] before any artifact actuation.
// These scenarios run the Python authority and the Go spine on two byte-identical
// project copies and compare the manifest bytes AFTER the write. The Python leg
// reaches the same shared writer through the gate's --write-recipe-config bridge
// (the reconcile-stamp decision and the write decision are both Go), so this is
// the strongest available check that the in-process PlanReconcileStamps ->
// materializeStampOrdered -> shared.ApplyRecipeConfigWrite path is byte-faithful.

// materializeReconcileRecipe is a schema-valid recipe declaring a reconcile
// mapping that selects two config-field defaults. Both the whole reconcile table
// and the two scalar defaults are stamped into the manifest.
func materializeReconcileRecipe(id, name string) string {
	return syncRecipeHeader(id, name) + `
[config.reconcile]
scope_field = "workflow"

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = "main"
`
}

// materializeSortedStampRecipe declares its [config.reconcile] keys out of
// alphabetical order and selects three defaults, so the inline-table key order
// of the written reconcile value (and of every nested expectation) is
// observable. The Go-side PlanReconcileStamps returns a map whose iteration
// order is random; only the JSON round-trip through the bridge's order makes the
// bytes deterministic and Python-identical.
func materializeSortedStampRecipe(id, name string) string {
	return syncRecipeHeader(id, name) + `
[config.reconcile]
scope_field = "workflow"
max_age_seconds = 3600

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"
config_field_when_set = "sync_state"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = "main"

[config.sync_state]
required = true
default = "open"
`
}

// writeMaterializeManifest writes one manifest into <project>/ai-specs.
func writeMaterializeManifest(t *testing.T, project string, data []byte) {
	t.Helper()
	dir := filepath.Join(project, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir ai-specs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// cloneMaterializeProject creates a fresh project carrying a byte-identical
// copy of srcProject's manifest, so each leg mutates its own copy.
func cloneMaterializeProject(t *testing.T, srcProject string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(srcProject, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatalf("read source manifest: %v", err)
	}
	dst := t.TempDir()
	writeMaterializeManifest(t, dst, data)
	return dst
}

// TestMaterializeSpineStampManifestDifferential compares the manifest bytes the
// two legs leave behind after the stamp step, for an added recipe (new values),
// an updated value, a no-op run, and a reconcile table whose declaration order
// differs from the sorted inline-table order the bridge emits.
func TestMaterializeSpineStampManifestDifferential(t *testing.T) {
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

	reconcileRecipes := map[string]string{"alpha": materializeReconcileRecipe("alpha", "Alpha")}
	sortedRecipes := map[string]string{"alpha": materializeSortedStampRecipe("alpha", "Alpha")}

	// The no-op input is the authority's own stamped output: the scenario proves
	// that re-running over an already-stamped manifest rewrites NOTHING, so the
	// input must be exactly what a correct first run produces.
	stamped := func(t *testing.T, recipes map[string]string, manifest string) string {
		t.Helper()
		fixture := writeMaterializeFixture(t, recipes, manifest)
		scratch := cloneMaterializeProject(t, fixture.project)
		ref := runMaterializeRef(t, gate, driver, authority, scratch, fixture.home, "-", "-", "-")
		if ref.RC != 0 {
			t.Fatalf("authority stamp run rc=%d stderr=%s", ref.RC, ref.Stderr)
		}
		data, err := os.ReadFile(filepath.Join(scratch, "ai-specs", "ai-specs.toml"))
		if err != nil {
			t.Fatalf("read stamped manifest: %v", err)
		}
		return string(data)
	}

	run := func(t *testing.T, recipes map[string]string, manifest string, untouched bool) {
		t.Helper()
		fixture := writeMaterializeFixture(t, recipes, manifest)
		pyProject := cloneMaterializeProject(t, fixture.project)
		goProject := cloneMaterializeProject(t, fixture.project)

		pyManifest := filepath.Join(pyProject, "ai-specs", "ai-specs.toml")
		goManifest := filepath.Join(goProject, "ai-specs", "ai-specs.toml")
		goBefore, err := os.Stat(goManifest)
		if err != nil {
			t.Fatalf("stat go manifest: %v", err)
		}

		ref := runMaterializeRef(t, gate, driver, authority, pyProject, fixture.home, "-", "-", "-")
		if ref.RC != 0 {
			t.Fatalf("python leg rc=%d stderr=%s", ref.RC, ref.Stderr)
		}
		var stdout, stderr bytes.Buffer
		if _, err := MaterializeRecipes(MaterializeRequest{
			ProjectRoot: goProject,
			AISpecsHome: fixture.home,
			Stdout:      &stdout,
			Stderr:      &stderr,
		}); err != nil {
			t.Fatalf("go leg failed: %v\nstderr: %s", err, stderr.String())
		}

		pyBytes, err := os.ReadFile(pyManifest)
		if err != nil {
			t.Fatalf("read python manifest: %v", err)
		}
		goBytes, err := os.ReadFile(goManifest)
		if err != nil {
			t.Fatalf("read go manifest: %v", err)
		}
		if !bytes.Equal(goBytes, pyBytes) {
			t.Fatalf("manifest bytes diverge\n--- go ---\n%q\n--- python ---\n%q", goBytes, pyBytes)
		}
		if untouched {
			goAfter, err := os.Stat(goManifest)
			if err != nil {
				t.Fatalf("stat go manifest after: %v", err)
			}
			if !os.SameFile(goBefore, goAfter) {
				t.Fatalf("a no-op run rewrote the manifest (inode changed): %q", goBytes)
			}
		}
	}

	t.Run("added recipe writes new config values", func(t *testing.T) {
		run(t, reconcileRecipes, "[recipes.alpha]\nenabled = true\n", false)
	})
	t.Run("updated value rewrites the stale config", func(t *testing.T) {
		run(t, reconcileRecipes, "[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nworkflow = \"stale\"\n", false)
	})
	t.Run("no-op run leaves the already-stamped manifest untouched", func(t *testing.T) {
		run(t, reconcileRecipes, stamped(t, reconcileRecipes, "[recipes.alpha]\nenabled = true\n"), true)
	})
	t.Run("sorted-key reconcile order matches the bridge", func(t *testing.T) {
		run(t, sortedRecipes, "[recipes.alpha]\nenabled = true\n", false)
	})
}
