package doctor

// GO-07 S18: the doctor's two brief probes run in process. These tests pin the
// discrimination hermetically — no python3, no bridge — so they fail if either
// probe goes back to the Python seam.

import (
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/sync"
)

// briefProbeHome writes the CLI home catalog the probes read: one recipe that
// declares a [provides.brief] fragment.
func briefProbeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "catalog", "recipes", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\nversion = \"1.0\"\ndescription = \"A probe recipe with one brief fragment.\"\n\n[provides.brief]\nworkflow_rules = [\"always run the focused tests\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestDoctorSpawnsNoPython pins the public outcome of the retired bridge: a
// parseable manifest with brief fragments runs every check without a python3
// child, and the brief checks still report the real state.
func TestDoctorSpawnsNoPython(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"t\"\n\n[recipes.alpha]\nenabled = true\n")
	log := shadowPython(t)

	d := New(root, briefProbeHome(t))
	d.Run()

	if _, err := os.Stat(log); err == nil {
		data, _ := os.ReadFile(log)
		t.Fatalf("doctor spawned python3: %q", data)
	}
	for _, c := range d.Checks {
		if c.Name == "doctor-bridge" {
			t.Errorf("doctor-bridge check still emitted: %+v", c)
		}
	}
}

// TestBriefProbesRunWithoutTheBridge pins both brief check outcomes in process,
// with no Python seam to degrade them.
func TestBriefProbesRunWithoutTheBridge(t *testing.T) {
	t.Run("provenance reports the real state", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"t\"\n\n[recipes.alpha]\nenabled = true\n")
		d := New(root, briefProbeHome(t))
		d.checkBriefProvenance()
		assertChecks(t, d, []Check{
			{INFO, "brief-provenance", "runtime brief ownership: missing", "ai-specs sync will create AGENTS.md"},
		})
	})

	t.Run("dead fragments warn with render disabled", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"t\"\n\n[recipes.alpha]\nenabled = true\n\n[brief]\nrender = false\n")
		d := New(root, briefProbeHome(t))
		dead, probeErr := sync.BriefHasDeadRecipeFragments(sync.BriefViewRequest{ProjectRoot: d.Root, AISpecsHome: d.Home})
		if probeErr != nil {
			t.Fatalf("probe error: %v", probeErr)
		}
		if !dead {
			t.Fatalf("probe = false for a recipe that declares a brief fragment")
		}
		d.checkBriefRenderPolicy()
		assertChecks(t, d, []Check{
			{WARN, "brief-fragments-unused", "enabled recipes declare [provides.brief] but render = false", "remove unused recipes or set [brief].render = true"},
		})
	})

	t.Run("no dead fragments without a declaration", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[project]\nname = \"t\"\n\n[recipes.alpha]\nenabled = true\n\n[brief]\nrender = false\n")
		d := New(root, briefProbeHome(t))
		// A project without a catalog recipe for the enabled id declares nothing.
		d.Home = t.TempDir()
		d.checkBriefRenderPolicy()
		assertChecks(t, d, []Check{})
	})
}
