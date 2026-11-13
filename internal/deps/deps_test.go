package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

func depFor(binary string) *schema.CliDep {
	return &schema.CliDep{
		Binary:     binary,
		Purpose:    "uses " + binary,
		Required:   true,
		InstallURL: "https://example.com/" + binary,
	}
}

// TestCheckOneNotFound pins dep_check._check_one's missing arm: not found,
// never ok, and the "not found on PATH" detail the panel prints.
func TestCheckOneNotFound(t *testing.T) {
	got := CheckOne(depFor("definitely-not-a-real-binary-xyz"), "r", "")
	if got.Found || got.OK {
		t.Fatalf("CheckOne = %+v, want not found and not ok", got)
	}
	if got.Detail != "not found on PATH" {
		t.Errorf("Detail = %q, want %q", got.Detail, "not found on PATH")
	}
	if got.RecipeID != "r" {
		t.Errorf("RecipeID = %q, want r", got.RecipeID)
	}
}

// TestCheckOneFoundWithoutVersionCheck pins the plain found arm: no version is
// ever run when the dep declares no version_check.
func TestCheckOneFoundWithoutVersionCheck(t *testing.T) {
	got := CheckOne(depFor("git"), "r", "")
	if !got.Found || !got.OK {
		t.Fatalf("CheckOne = %+v, want found and ok", got)
	}
	if got.Version != "" || got.Detail != "" {
		t.Errorf("Version/Detail = %q/%q, want empty/empty", got.Version, got.Detail)
	}
}

// TestCheckOneVersionArms pins the three min_version answers.
func TestCheckOneVersionArms(t *testing.T) {
	cases := []struct {
		name         string
		versionCheck string
		minVersion   string
		wantOK       bool
		wantVersion  string
		wantDetail   string
	}{
		{"satisfied", "echo 2.5.0", "2.0", true, "2.5.0", ""},
		{"below the minimum", "echo 1.2.3", "2.0", false, "1.2.3", "found 1.2.3 < required 2.0"},
		{"unparseable output is unknown, not a failure", "true", "2.0", true, "", "version unknown"},
	}
	for _, tc := range cases {
		dep := depFor("git")
		dep.VersionCheck = tc.versionCheck
		dep.MinVersion = tc.minVersion
		got := CheckOne(dep, "r", "")
		if !got.Found {
			t.Fatalf("%s: Found = false, want true", tc.name)
		}
		if got.OK != tc.wantOK {
			t.Errorf("%s: OK = %v, want %v", tc.name, got.OK, tc.wantOK)
		}
		if got.Version != tc.wantVersion {
			t.Errorf("%s: Version = %q, want %q", tc.name, got.Version, tc.wantVersion)
		}
		if got.Detail != tc.wantDetail {
			t.Errorf("%s: Detail = %q, want %q", tc.name, got.Detail, tc.wantDetail)
		}
	}
}

// TestCheckRecipe pins dep_check.check_cli_deps: one Result per declared dep,
// in declaration order, each carrying the recipe id.
func TestCheckRecipe(t *testing.T) {
	recipe := &schema.Recipe{ID: "demo", CliDeps: []*schema.CliDep{
		depFor("git"),
		depFor("definitely-not-a-real-binary-xyz"),
	}}
	got := CheckRecipe(recipe, "")
	if len(got) != 2 {
		t.Fatalf("CheckRecipe returned %d results, want 2", len(got))
	}
	if got[0].Binary != "git" || !got[0].OK || got[0].RecipeID != "demo" {
		t.Errorf("result 0 = %+v", got[0])
	}
	if got[1].Binary != "definitely-not-a-real-binary-xyz" || got[1].OK {
		t.Errorf("result 1 = %+v", got[1])
	}
}

func writeCatalogRecipe(t *testing.T, home, id, body string) {
	t.Helper()
	path := filepath.Join(home, "catalog", "recipes", id, "recipe.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, projectRoot, body string) {
	t.Helper()
	path := filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalogRecipe(id, extra string) string {
	return "[recipe]\nid = \"" + id + "\"\nname = \"N\"\ndescription = \"D\"\nversion = \"1.0\"\n" + extra
}

// TestCheckProject pins dep_check.check_project_deps: one Result per cli_dep
// of every ENABLED recipe in manifest order, silently skipping disabled
// recipes and recipes the catalog cannot supply.
func TestCheckProject(t *testing.T) {
	home := t.TempDir()
	writeCatalogRecipe(t, home, "alpha", catalogRecipe("alpha",
		"\n[[deps.cli]]\nbinary = \"git\"\npurpose = \"P\"\nrequired = true\n"))
	writeCatalogRecipe(t, home, "beta", catalogRecipe("beta",
		"\n[[deps.cli]]\nbinary = \"definitely-not-a-real-binary-xyz\"\npurpose = \"Q\"\nrequired = false\n"))
	projectRoot := t.TempDir()
	writeManifest(t, projectRoot, "[recipes.alpha]\nenabled = true\n[recipes.beta]\nenabled = false\n[recipes.ghost]\nenabled = true\n")

	got := CheckProject(projectRoot, home)
	if len(got) != 1 {
		t.Fatalf("CheckProject returned %d results, want 1: %+v", len(got), got)
	}
	if got[0].Binary != "git" || got[0].RecipeID != "alpha" || !got[0].OK {
		t.Errorf("result = %+v", got[0])
	}
}

func TestCheckProjectWithoutManifest(t *testing.T) {
	if got := CheckProject(t.TempDir(), t.TempDir()); len(got) != 0 {
		t.Errorf("CheckProject without a manifest = %+v, want none", got)
	}
}

// TestResultFieldsAreTheLegacyOnes guards the projection the doctor mapping and
// the panel both consume: a zero Result must never grow a required field.
func TestResultFieldsAreTheLegacyOnes(t *testing.T) {
	var r Result
	empty := strings.TrimSpace(r.Binary + r.Version + r.InstallURL + r.Purpose + r.Detail + r.Source + r.ResolvedPath)
	if empty != "" || r.Found || r.OK || r.Required || r.RecipeID != "" {
		t.Errorf("zero Result is not zero: %+v", r)
	}
}
