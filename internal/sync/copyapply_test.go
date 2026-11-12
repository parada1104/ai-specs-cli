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

	"ai-specs.dev/ai-specs/internal/projectcache"
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
// Frozen Python-oracle differential + plan->apply boundary composition
// ---------------------------------------------------------------------------

// The legacy live leg drove the real Python copy entry points the materialize
// loop calls (materialize_bundled_skill, materialize_command, materialize_doc)
// with the bridge sink (go_apply_copy) replaced by a recorder, and compared the
// recorded kind/id/src/dest items with the native PlanCopyItems. It was
// captured once into testdata/copyapply_golden.json: the fixture catalog the
// leg built, and the single case's ids, the cache roots Python resolved and the
// recorded items.
//
// The payload carries host-dependent segments only as tokens: the temporary
// base directory as <BASE> and the per-project cache key directory (a sha256 of
// the project's absolute path) as <CACHE>. Modes are not frozen at all: the
// fixture files are written by the shared helpers whose modes follow the
// process umask, and this package also runs t.Parallel() tests that must not
// have the process umask pinned under them. A run of this test under
// `umask 077` proves it.
//
// The test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const copyApplyGoldenFile = "copyapply_golden.json"

// copyApplyGoldenCases is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle
// of coverage — kept in the test, never derived from the bundle — so a bundle
// that is missing, renamed, duplicated or reordered cannot validate silently.
var copyApplyGoldenCases = []string{
	"one recipe with a bundled skill, a dependency skill, a command and a doc",
}

// The two tokens every host-dependent path segment is frozen as.
const (
	copyApplyBaseToken  = "<BASE>"
	copyApplyCacheToken = "<CACHE>"
)

// copyPlanItem is the four-field projection both sides compare: kind, id, src
// and dest. The command envelope's commands_dir rides along but is not part of
// the compared plan surface.
type copyPlanItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

// copyApplyFrozenRoots is the three caller-resolved cache roots in tokenized
// form.
type copyApplyFrozenRoots struct {
	RecipeSkillsRoot string `json:"recipe_skills_root"`
	CommandsDir      string `json:"commands_dir"`
	ProjectRoot      string `json:"project_root"`
}

// copyApplyNode is one frozen catalog node: a directory by path only, or a
// regular file by path and bytes.
type copyApplyNode struct {
	Path    string `json:"path"`
	Dir     bool   `json:"dir,omitempty"`
	Content string `json:"content,omitempty"`
}

// copyApplyFrozenCase is the frozen record of the one case the live leg graded.
type copyApplyFrozenCase struct {
	Name  string               `json:"name"`
	IDs   []string             `json:"ids"`
	Roots copyApplyFrozenRoots `json:"roots"`
	Items []copyPlanItem       `json:"items"`
}

// copyApplyGolden is the frozen bundle: oracle provenance, the catalog the case
// ran over and the per-case record.
type copyApplyGolden struct {
	Oracle  map[string]string     `json:"oracle"`
	Catalog []copyApplyNode       `json:"catalog"`
	Cases   []copyApplyFrozenCase `json:"cases"`
}

// copyApplyCatalog writes the fixture catalog the live leg built and returns the
// three roots: one recipe declaring a bundled skill, a non-bundled dependency
// skill, a command and a doc, with the real source files Python's doc path
// pre-checks. base is the already-resolved temp base, so every derived path
// shares one space with the frozen payload.
func copyApplyCatalog(t *testing.T, base string) (catalog, projectRoot, cliHome string) {
	t.Helper()
	catalog = filepath.Join(base, "catalog")
	projectRoot = filepath.Join(base, "project")
	cliHome = filepath.Join(base, "home")
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
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "skills", "alpha-skill", "SKILL.md"), "skill body")
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "commands", "alpha.md"), "command body")
	writeCopyFixtureFile(t, filepath.Join(catalog, "alpha", "README.md"), "doc body")
	return catalog, projectRoot, cliHome
}

// copyApplyTree lists one tree as sorted root-relative nodes: a directory by
// path and a regular file by path and bytes. Modes are omitted because they
// follow the process umask.
func copyApplyTree(t *testing.T, root string) []copyApplyNode {
	t.Helper()
	out := []copyApplyNode{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &os.PathError{Op: "snapshot", Path: path, Err: os.ErrInvalid}
		}
		if info.IsDir() {
			out = append(out, copyApplyNode{Path: filepath.ToSlash(rel), Dir: true})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, copyApplyNode{Path: filepath.ToSlash(rel), Content: string(data)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// compareCopyApplyCatalog requires identical node inventories: the same paths
// in the same order, the same kind and the same bytes.
func compareCopyApplyCatalog(t *testing.T, label string, got, want []copyApplyNode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d nodes, want %d\n native = %v\n frozen = %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Path != want[i].Path || got[i].Dir != want[i].Dir {
			t.Fatalf("%s: node %d is %+v, want %+v", label, i, got[i], want[i])
		}
		if got[i].Content != want[i].Content {
			t.Fatalf("%s: %s bytes differ:\n native = %q\n frozen = %q", label, want[i].Path, got[i].Content, want[i].Content)
		}
	}
}

// tokenizeCopyApply replaces the two host-dependent path segments with their
// frozen tokens, so both sides compare in the golden's host-independent space:
// the per-project cache key directory (a sha256 of the project's absolute path)
// as <CACHE> and the temporary base directory as <BASE>. The cache key is the
// cache root's last element only, so the rest of the cache shape stays frozen;
// it is replaced first because it lives under the base.
func tokenizeCopyApply(text, base, cacheRoot string) string {
	text = strings.ReplaceAll(text, cacheRoot, filepath.Join(filepath.Dir(cacheRoot), copyApplyCacheToken))
	return strings.ReplaceAll(text, base, copyApplyBaseToken)
}

// loadCopyApplyGolden reads the frozen bundle and validates its identity
// inventory, its oracle metadata and the tokenized shape of every frozen path
// before any comparison runs.
func loadCopyApplyGolden(t *testing.T) copyApplyGolden {
	t.Helper()
	var golden copyApplyGolden
	if err := json.Unmarshal(readGoldenBundle(t, copyApplyGoldenFile), &golden); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", copyApplyGoldenFile, err)
	}
	for _, key := range []string{"python", "script", "driver"} {
		if golden.Oracle[key] == "" {
			t.Fatalf("frozen oracle %s carries no %s metadata", copyApplyGoldenFile, key)
		}
	}
	names := make([]string, len(golden.Cases))
	for i, rec := range golden.Cases {
		names[i] = rec.Name
		if len(rec.Items) == 0 {
			t.Fatalf("frozen record %q carries no items", rec.Name)
		}
		// Every host-dependent segment is a token: a capture that leaked an
		// absolute path cannot validate here.
		paths := []string{rec.Roots.RecipeSkillsRoot, rec.Roots.CommandsDir, rec.Roots.ProjectRoot}
		for _, item := range rec.Items {
			paths = append(paths, item.Src, item.Dest)
		}
		for _, path := range paths {
			if !strings.HasPrefix(path, copyApplyBaseToken+"/") {
				t.Fatalf("frozen record %q path %q must start with %s/", rec.Name, path, copyApplyBaseToken)
			}
		}
	}
	for _, node := range golden.Catalog {
		if filepath.IsAbs(node.Path) || strings.HasPrefix(node.Path, "..") {
			t.Fatalf("frozen catalog path %q must be root-relative", node.Path)
		}
	}
	validateGoldenIdentity(t, "copyapply", names, copyApplyGoldenCases)
	return golden
}

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

// TestCopyApplyPythonDifferential pins PlanCopyItems against the frozen record
// of the real Python copy entry points (materialize_bundled_skill,
// materialize_command, materialize_doc) for one recipe with a bundled skill, a
// non-bundled dep skill, a command and a doc. The frozen roots are the cache
// roots Python resolved; the native plan derives today's roots through the same
// mirrored helpers, so that derivation is pinned too. Both sides must produce
// the same kind, id, src and dest in the same order: 3 items, with the dep
// skill absent on both sides, so an empty comparison cannot pass.
func TestCopyApplyPythonDifferential(t *testing.T) {
	golden := loadCopyApplyGolden(t)
	ids := []string{"alpha"}
	if len(golden.Cases) != len(copyApplyGoldenCases) {
		t.Fatalf("frozen bundle has %d cases, inventory %d", len(golden.Cases), len(copyApplyGoldenCases))
	}
	rec := golden.Cases[0]

	// The base is resolved up front so every fixture path and every derived root
	// shares one space (macOS resolves /var to /private/var).
	base := projectcache.ResolvePath(t.TempDir())
	catalog, projectRoot, cliHome := copyApplyCatalog(t, base)
	// The fixture the freeze was captured from is an input of the payload: a
	// drifted catalog fails here, not as an opaque payload mismatch.
	compareCopyApplyCatalog(t, "catalog input", copyApplyTree(t, catalog), golden.Catalog)

	if !reflect.DeepEqual(rec.IDs, ids) {
		t.Fatalf("%s: frozen ids %v, corpus %v", rec.Name, rec.IDs, ids)
	}

	// The frozen roots are the ones the legacy authority resolved; the native
	// caller derives today's roots through the same mirrored helpers.
	roots := CopyRoots{
		RecipeSkillsRoot: projectcache.RecipeSkillsRoot(projectRoot, cliHome),
		CommandsDir:      projectcache.CommandsDir(projectRoot, cliHome),
		ProjectRoot:      projectRoot,
	}
	cacheRoot := projectcache.CacheRoot(projectRoot, cliHome)
	tokenize := func(text string) string { return tokenizeCopyApply(text, base, cacheRoot) }

	gotRoots := copyApplyFrozenRoots{
		RecipeSkillsRoot: tokenize(roots.RecipeSkillsRoot),
		CommandsDir:      tokenize(roots.CommandsDir),
		ProjectRoot:      tokenize(roots.ProjectRoot),
	}
	if gotRoots != rec.Roots {
		t.Fatalf("cache roots diverge\n--- go ---\n%+v\n--- frozen ---\n%+v", gotRoots, rec.Roots)
	}

	// Non-vacuity on the frozen side: 3 items, all three kinds, no dep skill.
	if len(rec.Items) != 3 {
		t.Fatalf("the frozen authority recorded %d item(s), want 3: %#v", len(rec.Items), rec.Items)
	}
	frozenKinds := map[string]bool{}
	for _, item := range rec.Items {
		frozenKinds[item.Kind] = true
		if item.ID == "alpha-dep" {
			t.Fatalf("the non-bundled dep skill must not be planned: %#v", item)
		}
	}
	for _, kind := range []string{"bundled-skill", "command", "doc"} {
		if !frozenKinds[kind] {
			t.Fatalf("the frozen record is missing kind %q: %#v", kind, rec.Items)
		}
	}

	got := planToCopyItems(PlanCopyItems(catalog, ids, roots))

	// Non-vacuity on the native side too, before the equality check.
	if len(got) != 3 {
		t.Fatalf("native plan produced %d item(s), want 3: %#v", len(got), got)
	}
	for i, item := range got {
		if item.ID == "alpha-dep" {
			t.Fatalf("the non-bundled dep skill must not be planned: %#v", item)
		}
		got[i].Src = tokenize(item.Src)
		got[i].Dest = tokenize(item.Dest)
	}

	if !reflect.DeepEqual(got, rec.Items) {
		t.Fatalf("copy plan diverges\n--- go ---\n%#v\n--- frozen ---\n%#v", got, rec.Items)
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
