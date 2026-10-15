package sync

// GO-07 S18: the read-only brief view the doctor's brief probes now run in
// process. These tests pin it against the real Python chain the bridge used
// (internal/sync/testdata/briefref_ref.py drives the same three legacy modules
// in the same order), state by state.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

// briefRefOut is one oracle run.
type briefRefOut struct {
	Error        *string `json:"error"`
	State        *string `json:"state"`
	WouldWriteSH string  `json:"would_write_sha"`
	RenderStdout string  `json:"render_stdout"`
	Dead         *bool   `json:"dead"`
}

// briefRefAuthority locates python3, the oracle driver and the legacy lib dir.
func briefRefAuthority(t *testing.T) (python, driver, lib string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	lib = filepath.Join(root, "lib", "_internal")
	if _, err := os.Stat(filepath.Join(lib, "recipe-materialize.py")); err != nil {
		t.Skipf("recipe-materialize.py not available: %v", err)
	}
	driver = filepath.Join(root, "internal", "sync", "testdata", "briefref_ref.py")
	return python, driver, lib
}

// runBriefRef runs the oracle for one fixture. The gate binary is pinned so
// Python's build_resolved_config and the native projection share one decision
// core — this test pins the brief wiring, not core-vs-fallback parity, which
// the S13/S14 differentials already own.
func runBriefRef(t *testing.T, python, driver, lib, root, home string) briefRefOut {
	t.Helper()
	cmd := exec.Command(python, driver, root, home, lib)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"WORKTREE_GATE_BIN="+materializeGateBinary(t),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	var out briefRefOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode oracle %s: %v", stdout.String(), err)
	}
	return out
}

// briefRecipeToml is a minimal catalog recipe: one brief fragment on the
// workflow_rules section.
const briefRecipeToml = `[recipe]
id = "alpha"
name = "Alpha"
version = "1.0"
description = "A probe recipe with one brief fragment."

[provides.brief]
workflow_rules = ["always run the focused tests"]
`

// briefRecipeNoFragmentsToml is a valid catalog recipe that declares no brief
// fragments: the fragment attach leaves it with `{}`.
const briefRecipeNoFragmentsToml = `[recipe]
id = "alpha"
name = "Alpha"
version = "1.0"
description = "A probe recipe without brief fragments."
`

// briefRecipeBoundToml is a catalog recipe that declares the VCS capability,
// so an explicit [[bindings]] entry resolves and reaches the rendered brief.
// Its id is a known VCS label, so the renderer emits no unknown-VCS warning.
const briefRecipeBoundToml = `[recipe]
id = "git-pr-flow"
name = "GitHub PR Flow"
version = "1.0"
description = "A probe recipe that declares the VCS capability."

[[capabilities]]
id = "vcs-pr-flow"

[provides.brief]
workflow_rules = ["always run the focused tests"]
`

// briefRecipeInvalidToml is a recipe the loader rejects (a required field is
// missing): both legs skip it silently, exactly like Python's per-recipe
// `except Exception: pass`.
const briefRecipeInvalidToml = `[recipe]
id = "alpha"
name = "Alpha"

[provides.brief]
workflow_rules = ["never attached"]
`

// newBriefProject builds a project root with the given manifest and, when
// recipeToml is non-empty, the alpha catalog recipe under a fresh CLI home.
func newBriefProject(t *testing.T, manifest, recipeToml string) (root, home string) {
	t.Helper()
	return newBriefProjectRecipe(t, manifest, "alpha", recipeToml)
}

// newBriefProjectRecipe is newBriefProject with an explicit catalog recipe id
// (the auto-binding scenarios need an id the VCS label table knows).
func newBriefProjectRecipe(t *testing.T, manifest, recipeID, recipeToml string) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if recipeToml != "" {
		dir := filepath.Join(home, "catalog", "recipes", recipeID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(recipeToml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, home
}

// writeBriefBaseline writes a lock whose AGENTS.md baseline is baseline, using
// the ported lock writer — the same file both legs read.
func writeBriefBaseline(t *testing.T, root string, baseline []byte) {
	t.Helper()
	tomlPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	key, err := briefLockKey(tomlPath, filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("lock key: %v", err)
	}
	lk := lock.NewLock()
	lock.SetBriefBaseline(lk, key, lock.Sha256Bytes(baseline))
	if err := lock.WriteLock(briefLockPath(tomlPath), lk); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}

func writeBrief(t *testing.T, root string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBuildBriefViewDifferentialAgainstPython pins the would-write bytes and the
// effective state against the Python chain, over every state a doctor run can
// report.
func TestBuildBriefViewDifferentialAgainstPython(t *testing.T) {
	python, driver, lib := briefRefAuthority(t)
	const manifest = "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n"

	// The reference bytes both legs must agree on come from the Python oracle's
	// own render, so a drift in the render is visible as a sha mismatch.
	root, home := newBriefProject(t, manifest, briefRecipeToml)
	ref := runBriefRef(t, python, driver, lib, root, home)
	if ref.Error != nil {
		t.Fatalf("oracle reported %s", *ref.Error)
	}
	if ref.WouldWriteSH == "" || ref.State == nil || *ref.State != "missing" {
		t.Fatalf("oracle = %+v, want a rendered brief in the missing state", ref)
	}
	view, err := BuildBriefView(BriefViewRequest{ProjectRoot: root, AISpecsHome: home})
	if err != nil {
		t.Fatalf("BuildBriefView: %v", err)
	}
	if got := lock.Sha256Bytes(view.WouldWrite); got != ref.WouldWriteSH {
		t.Errorf("would-write sha = %s\nwant %s\nbytes:\n%s", got, ref.WouldWriteSH, view.WouldWrite)
	}
	if view.State != *ref.State {
		t.Errorf("state = %q, want %q", view.State, *ref.State)
	}

	rendered := view.WouldWrite

	scenarios := []struct {
		name     string
		agents   []byte // nil: no AGENTS.md
		baseline []byte // nil: no lock baseline
	}{
		{name: "missing"},
		{name: "managed_current", agents: rendered, baseline: rendered},
		{name: "managed_stale", agents: appended(rendered, "\nstale tail\n"), baseline: appended(rendered, "\nstale tail\n")},
		{name: "untracked", agents: []byte("# hand written\n")},
		{name: "user_modified", agents: []byte("# hand written\n"), baseline: rendered},
		{name: "marker", agents: []byte("# brief\n" + runtimeBriefMarker + "\n")},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			scRoot, scHome := newBriefProject(t, manifest, briefRecipeToml)
			if scenario.agents != nil {
				writeBrief(t, scRoot, scenario.agents)
			}
			if scenario.baseline != nil {
				writeBriefBaseline(t, scRoot, scenario.baseline)
			}

			scRef := runBriefRef(t, python, driver, lib, scRoot, scHome)
			if scRef.Error != nil {
				t.Fatalf("oracle reported %s", *scRef.Error)
			}
			scView, viewErr := BuildBriefView(BriefViewRequest{ProjectRoot: scRoot, AISpecsHome: scHome})
			if viewErr != nil {
				t.Fatalf("BuildBriefView: %v", viewErr)
			}
			if scView.State != *scRef.State {
				t.Errorf("state = %q, want %q", scView.State, *scRef.State)
			}
			if got := lock.Sha256Bytes(scView.WouldWrite); got != scRef.WouldWriteSH {
				t.Errorf("would-write sha = %s, want %s", got, scRef.WouldWriteSH)
			}
		})
	}

	t.Run("explicit bindings reach the rendered brief", func(t *testing.T) {
		// The auto-binding step belongs to the provenance chain: an explicit
		// [[bindings]] entry must reach the render on both legs. The recipe id is
		// a known VCS label, so the renderer emits no unknown-VCS warning — that
		// warning goes to stdout, which the legacy bridge used for its JSON
		// response (a bridge defect recorded as a follow-up in the unit doc).
		boundManifest := "[project]\nname = \"probe\"\n\n[recipes.git-pr-flow]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n\n[[bindings]]\ncapability = \"vcs-pr-flow\"\nrecipe = \"git-pr-flow\"\n"
		boundRoot, boundHome := newBriefProjectRecipe(t, boundManifest, "git-pr-flow", briefRecipeBoundToml)
		boundRef := runBriefRef(t, python, driver, lib, boundRoot, boundHome)
		if boundRef.Error != nil {
			t.Fatalf("oracle reported %s", *boundRef.Error)
		}
		if boundRef.RenderStdout != "" {
			t.Errorf("the fixture must render without a stdout warning, got %q", boundRef.RenderStdout)
		}
		boundView, boundErr := BuildBriefView(BriefViewRequest{ProjectRoot: boundRoot, AISpecsHome: boundHome})
		if boundErr != nil {
			t.Fatalf("BuildBriefView: %v", boundErr)
		}
		if got := lock.Sha256Bytes(boundView.WouldWrite); got != boundRef.WouldWriteSH {
			t.Errorf("would-write sha = %s, want %s\nbytes:\n%s", got, boundRef.WouldWriteSH, boundView.WouldWrite)
		}
		if boundView.State != *boundRef.State {
			t.Errorf("state = %q, want %q", boundView.State, *boundRef.State)
		}
		// Non-vacuous: the resolved binding must show up in the rendered bytes.
		if !strings.Contains(string(boundView.WouldWrite), "GitHub") {
			t.Errorf("the bound VCS label is missing from the rendered brief:\n%s", boundView.WouldWrite)
		}
	})

	t.Run("invalid brief mode fails on both sides", func(t *testing.T) {
		// _render_lines validates the [brief] mode keys first and raises, which
		// the bridge answered as "undetermined".
		badManifest := manifest + "\n[brief]\nworkflow_rules_mode = \"nope\"\n"
		badRoot, badHome := newBriefProject(t, badManifest, briefRecipeToml)
		badRef := runBriefRef(t, python, driver, lib, badRoot, badHome)
		_, nativeErr := BuildBriefView(BriefViewRequest{ProjectRoot: badRoot, AISpecsHome: badHome})
		if badRef.Error == nil {
			t.Fatal("the oracle rendered an invalid mode")
		}
		if nativeErr == nil {
			t.Fatal("the native view rendered an invalid mode")
		}
		if !strings.Contains(nativeErr.Error(), "invalid mode") {
			t.Errorf("native error = %v, want the ValueError parity message", nativeErr)
		}
	})
}

// appended returns content with suffix appended (a distinct, non-current brief).
func appended(content []byte, suffix string) []byte {
	return append(append([]byte(nil), content...), suffix...)
}

// TestBriefHasDeadRecipeFragmentsDifferentialAgainstPython pins the
// dead-fragments predicate over the same fixtures, including its own shorter
// chain (projection plus fragments, without the catalog defaults or the
// auto-bindings the provenance chain applies).
func TestBriefHasDeadRecipeFragmentsDifferentialAgainstPython(t *testing.T) {
	python, driver, lib := briefRefAuthority(t)

	scenarios := []struct {
		name       string
		manifest   string
		recipeToml string
	}{
		{
			name:       "fragments with render disabled",
			manifest:   "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n\n[brief]\nrender = false\n",
			recipeToml: briefRecipeToml,
		},
		{
			name:       "fragments with render enabled",
			manifest:   "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n",
			recipeToml: briefRecipeToml,
		},
		{
			name:       "enabled recipe declares no fragments",
			manifest:   "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n",
			recipeToml: briefRecipeNoFragmentsToml,
		},
		{
			name:       "catalog recipe the loader rejects declares nothing",
			manifest:   "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n\n[brief]\nrender = false\n",
			recipeToml: briefRecipeInvalidToml,
		},
		{
			name:     "enabled recipe missing from the catalog",
			manifest: "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n",
		},
		{
			name:     "recipe declared but not enabled",
			manifest: "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = false\n",
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			root, home := newBriefProject(t, scenario.manifest, scenario.recipeToml)
			ref := runBriefRef(t, python, driver, lib, root, home)
			if ref.Error != nil {
				t.Fatalf("oracle reported %s", *ref.Error)
			}
			if ref.Dead == nil {
				t.Fatalf("oracle returned no dead value: %+v", ref)
			}
			got, err := BriefHasDeadRecipeFragments(BriefViewRequest{ProjectRoot: root, AISpecsHome: home})
			if err != nil {
				t.Fatalf("BriefHasDeadRecipeFragments: %v", err)
			}
			if got != *ref.Dead {
				t.Errorf("dead = %v, want %v", got, *ref.Dead)
			}
		})
	}
}

// TestBriefHasDeadRecipeFragmentsPredicate pins the predicate's own edges on
// hand-built documents: only a truthy section value counts, so a declared but
// empty section is not dead, an absent or non-object fragments value declares
// nothing, and a missing recipe entry is skipped.
func TestBriefHasDeadRecipeFragmentsPredicate(t *testing.T) {
	docFromJSON := func(raw string) *mcpObj {
		t.Helper()
		v, err := mcpParseJSONOrdered([]byte(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		obj, ok := v.(*mcpObj)
		if !ok {
			t.Fatalf("decode %s: got %T", raw, v)
		}
		return obj
	}

	cases := []struct {
		name string
		doc  *mcpObj
		want bool
	}{
		{
			name: "non-empty section",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{"alpha":{"brief_fragments":{"workflow_rules":[{"text":"x"}]}}}}`),
			want: true,
		},
		{
			name: "declared but empty section",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{"alpha":{"brief_fragments":{"workflow_rules":[]}}}}`),
		},
		{
			name: "empty fragments object",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{"alpha":{"brief_fragments":{}}}}`),
		},
		{
			name: "absent fragments key",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{"alpha":{}}}`),
		},
		{
			name: "absent recipe entry",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{}}`),
		},
		{
			name: "fragments value is not an object",
			doc:  docFromJSON(`{"enabled":["alpha"],"recipes":{"alpha":{"brief_fragments":"x"}}}`),
		},
		{
			name: "disabled recipe with fragments",
			doc:  docFromJSON(`{"enabled":[],"recipes":{"alpha":{"brief_fragments":{"workflow_rules":["x"]}}}}`),
		},
		{
			name: "no enabled key at all",
			doc:  docFromJSON(`{"recipes":{"alpha":{"brief_fragments":{"workflow_rules":["x"]}}}}`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := briefHasDeadRecipeFragments(tc.doc); got != tc.want {
				t.Errorf("briefHasDeadRecipeFragments = %v, want %v", got, tc.want)
			}
		})
	}
}
