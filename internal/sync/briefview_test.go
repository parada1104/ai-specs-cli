package sync

// GO-07 S18: the read-only brief view the doctor's brief probes now run in
// process. These tests pin it against the frozen record of the Python chain the
// bridge used (internal/sync/testdata/briefref_ref.py used to drive the same
// three legacy modules in the same order), state by state.
//
// PR2-B S6: the differential tests consume testdata/briefref_golden.json, the
// golden bundle captured once from the live Python 3.14.7 oracle through the
// same in-package fixture helpers below; no test execs Python anymore.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

// briefRefGoldenCase is one frozen record of testdata/briefref_golden.json: one
// oracle run's state and rendered sha in the view half, its dead-fragments value
// in the dead half, or the raised Python authority's message in Error.
type briefRefGoldenCase struct {
	Error        *string `json:"error"`
	State        *string `json:"state"`
	WouldWriteSH string  `json:"would_write_sha"`
	RenderStdout string  `json:"render_stdout"`
	Dead         *bool   `json:"dead"`
}

// briefRefViewGoldenNames is the hard-coded identity inventory of the frozen
// view half: exact case names in capture order (8 = 6 states + the explicit
// binding + the invalid-mode error). Kept in the test, never derived from the
// bundle.
var briefRefViewGoldenNames = []string{
	"missing",
	"managed_current",
	"managed_stale",
	"untracked",
	"user_modified",
	"marker",
	"explicit bindings reach the rendered brief",
	"invalid brief mode fails on both sides",
}

// briefRefDeadGoldenNames is the hard-coded identity inventory of the frozen
// dead-fragments half (6 records, capture order).
var briefRefDeadGoldenNames = []string{
	"fragments with render disabled",
	"fragments with render enabled",
	"enabled recipe declares no fragments",
	"catalog recipe the loader rejects declares nothing",
	"enabled recipe missing from the catalog",
	"recipe declared but not enabled",
}

// loadBriefRefGolden validates and indexes testdata/briefref_golden.json. The
// identity inventories are asserted before any case loop runs.
func loadBriefRefGolden(t *testing.T) (map[string]briefRefGoldenCase, map[string]briefRefGoldenCase) {
	t.Helper()
	var bundle struct {
		View struct {
			Names []string             `json:"names"`
			Cases []briefRefGoldenCase `json:"cases"`
		} `json:"view"`
		Dead struct {
			Names []string             `json:"names"`
			Cases []briefRefGoldenCase `json:"cases"`
		} `json:"dead"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "briefref_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}

	validateGoldenIdentity(t, "briefref view", bundle.View.Names, briefRefViewGoldenNames)
	if len(bundle.View.Cases) != len(bundle.View.Names) {
		t.Fatalf("briefref view: %d records for %d names", len(bundle.View.Cases), len(bundle.View.Names))
	}
	view := make(map[string]briefRefGoldenCase, len(bundle.View.Cases))
	viewErrors := 0
	for i, rec := range bundle.View.Cases {
		if rec.Error != nil {
			viewErrors++
			if rec.State != nil || rec.WouldWriteSH != "" {
				t.Fatalf("frozen view record %q mixes a raised error with a rendered brief", bundle.View.Names[i])
			}
		} else if rec.State == nil || rec.WouldWriteSH == "" {
			t.Fatalf("frozen view record %q has no rendered brief", bundle.View.Names[i])
		}
		view[bundle.View.Names[i]] = rec
	}
	if viewErrors != 1 {
		t.Fatalf("briefref view bundle has %d error records, want 1", viewErrors)
	}

	validateGoldenIdentity(t, "briefref dead", bundle.Dead.Names, briefRefDeadGoldenNames)
	if len(bundle.Dead.Cases) != len(bundle.Dead.Names) {
		t.Fatalf("briefref dead: %d records for %d names", len(bundle.Dead.Cases), len(bundle.Dead.Names))
	}
	dead := make(map[string]briefRefGoldenCase, len(bundle.Dead.Cases))
	deadTrue := 0
	for i, rec := range bundle.Dead.Cases {
		if rec.Error != nil || rec.Dead == nil {
			t.Fatalf("frozen dead record %q has no dead value", bundle.Dead.Names[i])
		}
		if *rec.Dead {
			deadTrue++
		}
		dead[bundle.Dead.Names[i]] = rec
	}
	if deadTrue != 2 {
		t.Fatalf("briefref dead bundle has %d dead=true records, want 2", deadTrue)
	}
	return view, dead
}

// frozenViewCase returns the frozen view record for name and requires the
// oracle to have rendered it.
func frozenViewCase(t *testing.T, golden map[string]briefRefGoldenCase, name string) briefRefGoldenCase {
	t.Helper()
	rec, ok := golden[name]
	if !ok {
		t.Fatalf("frozen record %q missing", name)
	}
	if rec.Error != nil {
		t.Fatalf("frozen record %q reports %s", name, *rec.Error)
	}
	return rec
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

// TestBuildBriefViewDifferentialAgainstFrozenOracle pins the would-write bytes
// and the effective state against the frozen Python chain record, over every
// state a doctor run can report.
func TestBuildBriefViewDifferentialAgainstFrozenOracle(t *testing.T) {
	viewGolden, _ := loadBriefRefGolden(t)
	const manifest = "[project]\nname = \"probe\"\n\n[recipes.alpha]\nenabled = true\n\n[agents]\nenabled = [\"claude\"]\n"

	// The reference bytes both legs must agree on come from the renderer's own
	// output, so a drift in the render is visible as a sha mismatch.
	root, home := newBriefProject(t, manifest, briefRecipeToml)
	ref := frozenViewCase(t, viewGolden, "missing")
	if *ref.State != "missing" {
		t.Fatalf("frozen record = %+v, want a rendered brief in the missing state", ref)
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

			scRef := frozenViewCase(t, viewGolden, scenario.name)
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
		boundRef := frozenViewCase(t, viewGolden, "explicit bindings reach the rendered brief")
		if boundRef.RenderStdout != "" {
			t.Errorf("the fixture must render without a stdout warning, frozen record has %q", boundRef.RenderStdout)
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
		badRef, ok := viewGolden["invalid brief mode fails on both sides"]
		if !ok {
			t.Fatalf("frozen record %q missing", "invalid brief mode fails on both sides")
		}
		if badRef.Error == nil || !strings.Contains(*badRef.Error, "invalid mode") {
			t.Fatalf("the frozen oracle rendered an invalid mode: %+v", badRef)
		}
		_, nativeErr := BuildBriefView(BriefViewRequest{ProjectRoot: badRoot, AISpecsHome: badHome})
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

// TestBriefHasDeadRecipeFragmentsDifferentialAgainstFrozenOracle pins the
// dead-fragments predicate over the same fixtures, including its own shorter
// chain (projection plus fragments, without the catalog defaults or the
// auto-bindings the provenance chain applies).
func TestBriefHasDeadRecipeFragmentsDifferentialAgainstFrozenOracle(t *testing.T) {
	_, deadGolden := loadBriefRefGolden(t)

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
			ref, ok := deadGolden[scenario.name]
			if !ok {
				t.Fatalf("frozen record %q missing", scenario.name)
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
