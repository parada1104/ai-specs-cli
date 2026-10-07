package sync

// T1 contract tests for the native root copy-apply authority (GO-07 S12.1).
//
// PlanCopyItems derives the copy items the Python materialize call sites send
// today (lib/_internal/recipe-materialize.py:4033-4051): per enabled recipe,
// bundled skills first, then commands, then docs. ApplyCopyItems executes them
// in process through the shared authority (shared.RunApplyCopy), the same
// single authority the gate binary's --apply-copy surface uses.
//
// Scope boundary: the T1 tests below are native and deterministic and pin the
// API shape (CopyRoots field names, item kind/id/src/dest). The T3 Python-oracle
// differential and the plan->apply composition boundary live at the bottom of
// this file.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// copyApplyRoots builds the three caller-resolved roots over one temp base.
// Every expected path derives through filepath.Join, like the authority.
func copyApplyRoots(t *testing.T) (string, CopyRoots) {
	t.Helper()
	base := t.TempDir()
	return filepath.Join(base, "catalog"), CopyRoots{
		RecipeSkillsRoot: filepath.Join(base, "cache", ".recipe"),
		CommandsDir:      filepath.Join(base, "cache", "commands"),
		ProjectRoot:      filepath.Join(base, "project"),
	}
}

// TestCopyApplyPlanThreeKinds pins the per-kind derivation for one recipe with
// a bundled skill, a command and a doc: exact kind, item id, source and
// destination for each. The declared sources are never created on disk, so the
// passing case also proves the plan is pure path derivation and not a stat or
// a copy.
func TestCopyApplyPlanThreeKinds(t *testing.T) {
	catalog, roots := copyApplyRoots(t)
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "Alpha")+`
[[provides.skills]]
id = "alpha-skill"
source = "bundled"

[[provides.commands]]
id = "alpha-cmd"
path = "commands/alpha.md"

[[provides.docs]]
source = "README.md"
target = "ai-specs/recipes/alpha/README.md"
`)

	got := PlanCopyItems(catalog, []string{"alpha"}, roots)
	want := []shared.CopyItem{
		{
			Kind: "bundled-skill",
			ID:   "alpha-skill",
			Src:  filepath.Join(catalog, "alpha", "skills", "alpha-skill"),
			Dest: filepath.Join(roots.RecipeSkillsRoot, "alpha", "skills", "alpha-skill"),
		},
		{
			Kind:        "command",
			ID:          "alpha-cmd",
			Src:         filepath.Join(catalog, "alpha", "commands", "alpha.md"),
			Dest:        filepath.Join(roots.CommandsDir, "alpha-cmd.md"),
			CommandsDir: roots.CommandsDir,
		},
		{
			Kind: "doc",
			ID:   "ai-specs/recipes/alpha/README.md",
			Src:  filepath.Join(catalog, "alpha", "README.md"),
			Dest: filepath.Join(roots.ProjectRoot, "ai-specs/recipes/alpha/README.md"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanCopyItems() =\n%#v\nwant\n%#v", got, want)
	}
}

// TestCopyApplyPlanOrder pins the two order rules at once: the enabled recipe
// order across two recipes (beta before alpha) and each recipe's declared
// category order (bundled skills, then commands, then docs). The dependency
// skill declared between two bundled skills proves a non-bundled source yields
// no item and does not shift the surviving items.
func TestCopyApplyPlanOrder(t *testing.T) {
	catalog, roots := copyApplyRoots(t)
	writeSyncCatalogRecipe(t, catalog, "beta", syncRecipeHeader("beta", "Beta")+`
[[provides.skills]]
id = "beta-skill"
source = "bundled"

[[provides.commands]]
id = "beta-cmd"
path = "commands/beta.md"
`)
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "Alpha")+`
[[provides.skills]]
id = "s1"
source = "bundled"

[[provides.skills]]
id = "s-dep"
source = "dep"
url = "https://example.invalid/x.git"

[[provides.skills]]
id = "s2"
source = "bundled"

[[provides.commands]]
id = "c1"
path = "commands/c1.md"

[[provides.commands]]
id = "c2"
path = "commands/c2.md"

[[provides.docs]]
source = "d1.md"
target = "ai-specs/recipes/alpha/d1.md"

[[provides.docs]]
source = "d2.md"
target = "ai-specs/recipes/alpha/d2.md"
`)

	got := PlanCopyItems(catalog, []string{"beta", "alpha"}, roots)
	want := []shared.CopyItem{
		{
			Kind: "bundled-skill",
			ID:   "beta-skill",
			Src:  filepath.Join(catalog, "beta", "skills", "beta-skill"),
			Dest: filepath.Join(roots.RecipeSkillsRoot, "beta", "skills", "beta-skill"),
		},
		{
			Kind:        "command",
			ID:          "beta-cmd",
			Src:         filepath.Join(catalog, "beta", "commands", "beta.md"),
			Dest:        filepath.Join(roots.CommandsDir, "beta-cmd.md"),
			CommandsDir: roots.CommandsDir,
		},
		{
			Kind: "bundled-skill",
			ID:   "s1",
			Src:  filepath.Join(catalog, "alpha", "skills", "s1"),
			Dest: filepath.Join(roots.RecipeSkillsRoot, "alpha", "skills", "s1"),
		},
		{
			Kind: "bundled-skill",
			ID:   "s2",
			Src:  filepath.Join(catalog, "alpha", "skills", "s2"),
			Dest: filepath.Join(roots.RecipeSkillsRoot, "alpha", "skills", "s2"),
		},
		{
			Kind:        "command",
			ID:          "c1",
			Src:         filepath.Join(catalog, "alpha", "commands", "c1.md"),
			Dest:        filepath.Join(roots.CommandsDir, "c1.md"),
			CommandsDir: roots.CommandsDir,
		},
		{
			Kind:        "command",
			ID:          "c2",
			Src:         filepath.Join(catalog, "alpha", "commands", "c2.md"),
			Dest:        filepath.Join(roots.CommandsDir, "c2.md"),
			CommandsDir: roots.CommandsDir,
		},
		{
			Kind: "doc",
			ID:   "ai-specs/recipes/alpha/d1.md",
			Src:  filepath.Join(catalog, "alpha", "d1.md"),
			Dest: filepath.Join(roots.ProjectRoot, "ai-specs/recipes/alpha/d1.md"),
		},
		{
			Kind: "doc",
			ID:   "ai-specs/recipes/alpha/d2.md",
			Src:  filepath.Join(catalog, "alpha", "d2.md"),
			Dest: filepath.Join(roots.ProjectRoot, "ai-specs/recipes/alpha/d2.md"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanCopyItems() order =\n%#v\nwant\n%#v", got, want)
	}
}

// TestCopyApplyPlanSkipsUnloadableRecipe pins the load boundary for a plan
// with no error return: an absent recipe directory, an unparseable recipe.toml
// and a recipe.toml without [recipe] are all skipped, like the stamp planner
// (PlanReconcileStamps). The surviving recipe's items keep their order.
func TestCopyApplyPlanSkipsUnloadableRecipe(t *testing.T) {
	catalog, roots := copyApplyRoots(t)
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "Alpha")+`
[[provides.skills]]
id = "alpha-skill"
source = "bundled"
`)
	writeSyncCatalogRecipe(t, catalog, "broken", "not = [valid\n")
	writeSyncCatalogRecipe(t, catalog, "norecipe", "tags = [\"x\"]\n")

	got := PlanCopyItems(catalog, []string{"ghost", "broken", "norecipe", "alpha"}, roots)
	want := []shared.CopyItem{{
		Kind: "bundled-skill",
		ID:   "alpha-skill",
		Src:  filepath.Join(catalog, "alpha", "skills", "alpha-skill"),
		Dest: filepath.Join(roots.RecipeSkillsRoot, "alpha", "skills", "alpha-skill"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanCopyItems() with unloadable recipes =\n%#v\nwant\n%#v", got, want)
	}
}

// TestCopyApplyPlanEmpty pins the empty-plan contract: a non-nil empty slice,
// never nil. An empty id list and a recipe that declares no copy item (only a
// dependency skill) both yield the same non-nil empty result.
func TestCopyApplyPlanEmpty(t *testing.T) {
	catalog, roots := copyApplyRoots(t)
	writeSyncCatalogRecipe(t, catalog, "deponly", syncRecipeHeader("deponly", "DepOnly")+`
[[provides.skills]]
id = "dep-skill"
source = "dep"
url = "https://example.invalid/x.git"
`)

	cases := []struct {
		name string
		ids  []string
	}{
		{"nil ids", nil},
		{"no recipes", []string{}},
		{"recipe without copy items", []string{"deponly"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanCopyItems(catalog, tc.ids, roots)
			if got == nil {
				t.Fatal("PlanCopyItems() = nil, want a non-nil empty slice")
			}
			if len(got) != 0 {
				t.Fatalf("PlanCopyItems() = %#v, want empty", got)
			}
		})
	}
}

// TestCopyApplyExecutor pins the in-process executor contract. The ok case
// copies a command item and reports status "ok"; source-missing reports the
// per-item status without an error; an empty item list is a non-nil empty
// result slice; an item whose execution fails surfaces a Go error naming the
// item.
func TestCopyApplyExecutor(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		base := t.TempDir()
		src := filepath.Join(base, "src", "c1.md")
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(base, "cache", "commands", "c1.md")

		got, err := ApplyCopyItems([]shared.CopyItem{{
			Kind:        "command",
			ID:          "c1",
			Src:         src,
			Dest:        dest,
			CommandsDir: filepath.Dir(dest),
		}})
		if err != nil {
			t.Fatalf("ApplyCopyItems() error = %v", err)
		}
		want := []shared.CopyResult{{ID: "c1", Status: "ok"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ApplyCopyItems() = %#v, want %#v", got, want)
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("dest not written: %v", err)
		}
		if string(data) != "hello" {
			t.Fatalf("dest content = %q, want %q", data, "hello")
		}
	})

	t.Run("source-missing", func(t *testing.T) {
		base := t.TempDir()
		dest := filepath.Join(base, "project", "ai-specs", "recipes", "x", "d.md")

		got, err := ApplyCopyItems([]shared.CopyItem{{
			Kind: "doc",
			ID:   "ai-specs/recipes/x/d.md",
			Src:  filepath.Join(base, "catalog", "x", "d.md"),
			Dest: dest,
		}})
		if err != nil {
			t.Fatalf("ApplyCopyItems() error = %v", err)
		}
		want := []shared.CopyResult{{ID: "ai-specs/recipes/x/d.md", Status: "source-missing"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ApplyCopyItems() = %#v, want %#v", got, want)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("a missing source must not write %s (stat err = %v)", dest, err)
		}
	})

	t.Run("empty items", func(t *testing.T) {
		got, err := ApplyCopyItems(nil)
		if err != nil {
			t.Fatalf("ApplyCopyItems(nil) error = %v", err)
		}
		if got == nil {
			t.Fatal("ApplyCopyItems(nil) = nil, want a non-nil empty slice")
		}
		if len(got) != 0 {
			t.Fatalf("ApplyCopyItems(nil) = %#v, want empty", got)
		}
	})

	t.Run("execution failure returns an error", func(t *testing.T) {
		base := t.TempDir()
		src := filepath.Join(base, "src-tree")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		// A bundled-skill dest that is a regular file is the reference's
		// rmtree failure: the executor must report it as a Go error.
		dest := filepath.Join(base, "blocker")
		if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := ApplyCopyItems([]shared.CopyItem{{
			Kind: "bundled-skill",
			ID:   "s1",
			Src:  src,
			Dest: dest,
		}})
		if err == nil {
			t.Fatal("a failed item execution must return a Go error")
		}
		if !strings.Contains(err.Error(), "s1") {
			t.Fatalf("error %q does not name the failed item id", err)
		}
	})
}

// ---------------------------------------------------------------------------
// T3: Python-oracle differential + plan->apply boundary composition
// ---------------------------------------------------------------------------

// copyPlanItem is the four-field projection both sides compare: kind, id, src
// and dest. The command envelope's commands_dir rides along but is not part of
// the compared plan surface.
type copyPlanItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

// copyOracleResult is the JSON copyPlanOracle emits.
type copyOracleResult struct {
	Roots struct {
		RecipeSkillsRoot string `json:"recipe_skills_root"`
		CommandsDir      string `json:"commands_dir"`
		ProjectRoot      string `json:"project_root"`
	} `json:"roots"`
	Items []copyPlanItem `json:"items"`
}

// copyPlanOracle drives the REAL Python copy entry points the materialize loop
// calls (materialize_bundled_skill, materialize_command, materialize_doc) in
// the same per-recipe order: bundled skills, commands, then docs. It replaces
// only the bridge sink (go_apply_copy) with a recorder, so the kind/id/src/dest
// Python would actually send are captured without copying anything, and it
// no-ops the lock write so the oracle stays read-only. It prints the cache
// roots it resolved plus the recorded items as one JSON object.
//
// Entry points used: read_recipe, materialize_bundled_skill,
// materialize_command, materialize_doc, _load_project_cache()'s
// recipe_skills_root and commands_dir. Two monkeypatches isolate the plan:
// go_apply_copy records the sent items instead of copying, and write_lock is a
// no-op so the oracle writes no lock file.
const copyPlanOracle = `
import contextlib, importlib.util, io, json, sys
from pathlib import Path

spec = importlib.util.spec_from_file_location("copy_plan_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

catalog = Path(sys.argv[2])
project_root = Path(sys.argv[3])
cli_home = Path(sys.argv[4])
ids = json.loads(sys.argv[5])

recorded = []

def _record(items):
    for item in items:
        recorded.append({
            "kind": item["kind"],
            "id": item["id"],
            "src": item["src"],
            "dest": item["dest"],
        })
    return [{"id": item["id"], "status": "ok"} for item in items]

mod.go_apply_copy = _record
mod.write_lock = lambda *args, **kwargs: None

sink = io.StringIO()
with contextlib.redirect_stdout(sink):
    for rid in ids:
        recipe = mod.read_recipe(catalog, rid)
        recipe_dir = catalog / rid
        for skill in recipe.skills:
            if skill.source == "bundled":
                mod.materialize_bundled_skill(recipe_dir, skill.id, project_root, rid, cli_home=cli_home)
        for cmd in recipe.commands:
            mod.materialize_command(recipe_dir, cmd, project_root, cli_home=cli_home)
        for doc in recipe.docs:
            mod.materialize_doc(recipe_dir, doc, project_root, rid)

pc = mod._load_project_cache()
print(json.dumps({
    "roots": {
        "recipe_skills_root": str(pc.recipe_skills_root(project_root, cli_home=cli_home)),
        "commands_dir": str(pc.commands_dir(project_root, cli_home=cli_home)),
        "project_root": str(project_root),
    },
    "items": recorded,
}))
`

// planToCopyItems projects the native plan onto the four compared fields.
func planToCopyItems(items []shared.CopyItem) []copyPlanItem {
	out := make([]copyPlanItem, 0, len(items))
	for _, item := range items {
		out = append(out, copyPlanItem{Kind: item.Kind, ID: item.ID, Src: item.Src, Dest: item.Dest})
	}
	return out
}

// writeCopyFixtureFile writes one fixture source file, creating its parent.
func writeCopyFixtureFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCopyApplyPythonDifferential compares the real native derivation against
// the real Python derivation (testdata-free: the driver above drives
// recipe-materialize.py) for one recipe with a bundled skill, a non-bundled
// dep skill, a command and a doc. Both sides must produce the same kind, id,
// src and dest in the same order: 3 items, with the dep skill absent on both
// sides, so an empty comparison cannot pass. The Python side reports the cache
// roots it resolved; the native plan runs over exactly those roots.
func TestCopyApplyPythonDifferential(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := filepath.Join(t.TempDir(), "copy_plan_oracle.py")
	if err := os.WriteFile(driver, []byte(copyPlanOracle), 0o600); err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()
	catalog := filepath.Join(base, "catalog")
	projectRoot := filepath.Join(base, "project")
	cliHome := filepath.Join(base, "home")
	writeSyncCatalogRecipe(t, catalog, "alpha", syncRecipeHeader("alpha", "Alpha")+`
[[provides.skills]]
id = "alpha-skill"
source = "bundled"

[[provides.skills]]
id = "alpha-dep"
source = "dep"
url = "https://example.invalid/x.git"

[[provides.commands]]
id = "alpha-cmd"
path = "commands/alpha.md"

[[provides.docs]]
source = "README.md"
target = "ai-specs/recipes/alpha/README.md"
`)
	// Real sources: the doc needs a file for Python's pre-bridge is_file check;
	// the others make the fixture realistic and feed the boundary composition.
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "skills", "alpha-skill", "SKILL.md"), "skill body")
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "commands", "alpha.md"), "command body")
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "README.md"), "doc body")

	ids := []string{"alpha"}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	raw := runSyncPythonOracle(t, python, driver, authority, catalog, projectRoot, cliHome, string(idsJSON))
	var ref copyOracleResult
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		t.Fatalf("parse oracle JSON: %v\nraw: %s", err, raw)
	}

	// Non-vacuity on the Python side: 3 items, all three kinds, no dep skill.
	if len(ref.Items) != 3 {
		t.Fatalf("python oracle planned %d item(s), want 3: %#v", len(ref.Items), ref.Items)
	}
	refKinds := map[string]bool{}
	for _, item := range ref.Items {
		refKinds[item.Kind] = true
		if item.ID == "alpha-dep" {
			t.Fatalf("the non-bundled dep skill must not be planned: %#v", item)
		}
	}
	for _, kind := range []string{"bundled-skill", "command", "doc"} {
		if !refKinds[kind] {
			t.Fatalf("python oracle is missing kind %q: %#v", kind, ref.Items)
		}
	}

	roots := CopyRoots{
		RecipeSkillsRoot: ref.Roots.RecipeSkillsRoot,
		CommandsDir:      ref.Roots.CommandsDir,
		ProjectRoot:      ref.Roots.ProjectRoot,
	}
	got := planToCopyItems(PlanCopyItems(catalog, ids, roots))

	// Non-vacuity on the native side too, before the equality check.
	if len(got) != 3 {
		t.Fatalf("native plan produced %d item(s), want 3: %#v", len(got), got)
	}
	for _, item := range got {
		if item.ID == "alpha-dep" {
			t.Fatalf("the non-bundled dep skill must not be planned: %#v", item)
		}
	}

	if !reflect.DeepEqual(got, ref.Items) {
		t.Fatalf("copy plan diverges\n--- go ---\n%#v\n--- python ---\n%#v", got, ref.Items)
	}
}

// TestCopyApplyDifferentialBoundary confirms the executor boundary against the
// real shared authority in process by composing PlanCopyItems with
// ApplyCopyItems, which the T1 executor test does not do. It covers an ok plan
// (every planned item copies), a source-missing planned item (reported without
// a write) and an empty plan (a non-nil empty result). The failure-to-error
// boundary is already proven by TestCopyApplyExecutor and is not repeated here.
func TestCopyApplyDifferentialBoundary(t *testing.T) {
	t.Run("ok plan copies every planned item", func(t *testing.T) {
		base := t.TempDir()
		catalog := filepath.Join(base, "catalog")
		roots := CopyRoots{
			RecipeSkillsRoot: filepath.Join(base, "cache", ".recipe"),
			CommandsDir:      filepath.Join(base, "cache", "commands"),
			ProjectRoot:      filepath.Join(base, "project"),
		}
		writeSyncCatalogRecipe(t, catalog, "beta", syncRecipeHeader("beta", "Beta")+`
[[provides.skills]]
id = "beta-skill"
source = "bundled"

[[provides.commands]]
id = "beta-cmd"
path = "commands/beta.md"

[[provides.docs]]
source = "BETA.md"
target = "ai-specs/recipes/beta/BETA.md"
`)
		writeCopyFixtureFile(t, filepath.Join(catalog, "beta", "skills", "beta-skill", "SKILL.md"), "skill body")
		writeCopyFixtureFile(t, filepath.Join(catalog, "beta", "commands", "beta.md"), "command body")
		writeCopyFixtureFile(t, filepath.Join(catalog, "beta", "BETA.md"), "doc body")

		plan := PlanCopyItems(catalog, []string{"beta"}, roots)
		results, err := ApplyCopyItems(plan)
		if err != nil {
			t.Fatalf("ApplyCopyItems(plan) error = %v", err)
		}
		want := []shared.CopyResult{
			{ID: "beta-skill", Status: "ok"},
			{ID: "beta-cmd", Status: "ok"},
			{ID: "ai-specs/recipes/beta/BETA.md", Status: "ok"},
		}
		if !reflect.DeepEqual(results, want) {
			t.Fatalf("composed plan->apply = %#v, want %#v", results, want)
		}
		for _, dest := range []struct{ path, body string }{
			{filepath.Join(roots.RecipeSkillsRoot, "beta", "skills", "beta-skill", "SKILL.md"), "skill body"},
			{filepath.Join(roots.CommandsDir, "beta-cmd.md"), "command body"},
			{filepath.Join(roots.ProjectRoot, "ai-specs", "recipes", "beta", "BETA.md"), "doc body"},
		} {
			data, err := os.ReadFile(dest.path)
			if err != nil {
				t.Fatalf("planned dest not written: %v", err)
			}
			if string(data) != dest.body {
				t.Fatalf("dest %s = %q, want %q", dest.path, data, dest.body)
			}
		}
	})

	t.Run("missing planned source reports source-missing without writing", func(t *testing.T) {
		base := t.TempDir()
		catalog := filepath.Join(base, "catalog")
		roots := CopyRoots{
			RecipeSkillsRoot: filepath.Join(base, "cache", ".recipe"),
			CommandsDir:      filepath.Join(base, "cache", "commands"),
			ProjectRoot:      filepath.Join(base, "project"),
		}
		writeSyncCatalogRecipe(t, catalog, "delta", syncRecipeHeader("delta", "Delta")+`
[[provides.commands]]
id = "delta-cmd"
path = "commands/delta.md"
`)

		plan := PlanCopyItems(catalog, []string{"delta"}, roots)
		results, err := ApplyCopyItems(plan)
		if err != nil {
			t.Fatalf("ApplyCopyItems(plan) error = %v", err)
		}
		want := []shared.CopyResult{{ID: "delta-cmd", Status: "source-missing"}}
		if !reflect.DeepEqual(results, want) {
			t.Fatalf("composed plan->apply = %#v, want %#v", results, want)
		}
		dest := filepath.Join(roots.CommandsDir, "delta-cmd.md")
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("a missing source must not write %s (stat err = %v)", dest, err)
		}
	})

	t.Run("empty plan yields a non-nil empty result", func(t *testing.T) {
		base := t.TempDir()
		catalog := filepath.Join(base, "catalog")
		roots := CopyRoots{
			RecipeSkillsRoot: filepath.Join(base, "cache", ".recipe"),
			CommandsDir:      filepath.Join(base, "cache", "commands"),
			ProjectRoot:      filepath.Join(base, "project"),
		}
		writeSyncCatalogRecipe(t, catalog, "deponly", syncRecipeHeader("deponly", "DepOnly")+`
[[provides.skills]]
id = "dep-skill"
source = "dep"
url = "https://example.invalid/x.git"
`)

		plan := PlanCopyItems(catalog, []string{"deponly"}, roots)
		if plan == nil || len(plan) != 0 {
			t.Fatalf("PlanCopyItems() = %#v, want a non-nil empty plan", plan)
		}
		results, err := ApplyCopyItems(plan)
		if err != nil {
			t.Fatalf("ApplyCopyItems(plan) error = %v", err)
		}
		if results == nil {
			t.Fatal("ApplyCopyItems(empty plan) = nil, want a non-nil empty slice")
		}
		if len(results) != 0 {
			t.Fatalf("ApplyCopyItems(empty plan) = %#v, want empty", results)
		}
	})
}
