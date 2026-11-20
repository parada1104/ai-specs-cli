package sync

// T7b (S3 "borrar siempre lo gestionado"): per-skill orphans inside an ENABLED
// recipe. Dropping a skill from `[provides].skills` must remove the copy
// ai-specs materialized for it under `<cache>/.recipe/<recipe>/skills/<skill>`
// and its `[recipes.<recipe>]` hash record, because the resolved-skills flatten
// reads that cache tree as its source of truth. A copy with no hash record is
// not provably ai-specs' output and stays (T2's rule).

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

func skillPruneRecipeBody(extra string) string {
	return `[recipe]
id = "two"
name = "Two"
description = "d"
version = "1.0.0"

[provides]
skills = [{ id = "aa", source = "bundled" }` + extra + `]
`
}

// writeSkillPruneSkill creates one bundled skill source in the fixture catalog.
func writeSkillPruneSkill(t *testing.T, home, recipeID, skillID string) {
	t.Helper()
	dir := filepath.Join(home, "catalog", "recipes", recipeID, "skills", skillID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill %s: %v", skillID, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+skillID+"\n"), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", skillID, err)
	}
}

func runSkillPruneMaterialize(t *testing.T, f materializeFixture) error {
	t.Helper()
	var stdout, stderr bytes.Buffer
	_, err := MaterializeRecipes(MaterializeRequest{
		ProjectRoot: f.project,
		AISpecsHome: f.home,
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	return err
}

func TestMaterializePrunesRecipeSkillNoLongerProvided(t *testing.T) {
	t.Setenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES", "")
	const both = `, { id = "bb", source = "bundled" }`
	f := writeMaterializeFixture(t, map[string]string{
		"two": skillPruneRecipeBody(both),
	}, "[recipes.two]\nenabled = true\n")
	writeSkillPruneSkill(t, f.home, "two", "aa")
	writeSkillPruneSkill(t, f.home, "two", "bb")
	// A project-local skill with the same name is the user's, never pruned.
	localBB := filepath.Join(f.project, "ai-specs", "skills", "bb")
	if err := os.MkdirAll(localBB, 0o755); err != nil {
		t.Fatalf("mkdir local skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localBB, "SKILL.md"), []byte("# local\n"), 0o644); err != nil {
		t.Fatalf("write local skill: %v", err)
	}

	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("seed materialize: %v", err)
	}
	recipeSkills := projectcache.RecipeSkillsRoot(f.project, f.home)
	for _, skill := range []string{"aa", "bb"} {
		if _, err := os.Stat(filepath.Join(recipeSkills, "two", "skills", skill, "SKILL.md")); err != nil {
			t.Fatalf("seed: skill %s not materialized: %v", skill, err)
		}
	}
	// An unrecorded directory nobody can prove ai-specs wrote must survive.
	manual := filepath.Join(recipeSkills, "two", "skills", "manual")
	if err := os.MkdirAll(manual, 0o755); err != nil {
		t.Fatalf("mkdir manual: %v", err)
	}
	// A stale SKILL.md-shaped copy (the shape the flatten collects) is an orphan
	// even though this run never declared it.
	stale := filepath.Join(recipeSkills, "two", "skills", "stale")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("# stale\n"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	// A legacy lock may still carry [recipes.<id>.skills.<skill>]; both records
	// must follow their copies (the lock WRITER no longer emits this table).
	lockPath := filepath.Join(f.project, "ai-specs", ".ai-specs.lock")
	legacy, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	legacy = append(legacy, []byte("\n[recipes.two.skills.aa]\nSKILL.md = \"aa\"\n\n[recipes.two.skills.bb]\nSKILL.md = \"bb\"\n")...)
	if err := os.WriteFile(lockPath, legacy, 0o644); err != nil {
		t.Fatalf("seed legacy lock: %v", err)
	}

	// The recipe drops bb.
	body := skillPruneRecipeBody("")
	if err := os.WriteFile(filepath.Join(f.home, "catalog", "recipes", "two", "recipe.toml"), []byte(body), 0o644); err != nil {
		t.Fatalf("rewrite recipe: %v", err)
	}
	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("materialize after the drop: %v", err)
	}

	if _, err := os.Stat(filepath.Join(recipeSkills, "two", "skills", "bb")); !os.IsNotExist(err) {
		t.Errorf("the dropped skill copy survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(recipeSkills, "two", "skills", "aa", "SKILL.md")); err != nil {
		t.Errorf("the still-provided skill copy is gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manual, "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("an unrecorded skill directory must survive: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("a stale SKILL.md copy must be pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localBB, "SKILL.md")); err != nil {
		t.Errorf("the project-local skill is gone: %v", err)
	}
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	// No [recipes.*] hash table survives a run (the writer drops it), so the
	// dropped skill cannot leave a stale ownership record behind either.
	if _, ok := lk.Recipes["two"]["bb"]; ok {
		t.Errorf("the dropped skill kept a hash record: %v", lk.Recipes["two"])
	}

	// A second run is a no-op: nothing is re-pruned and the lock is unchanged.
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a second run rewrote the lock:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestMaterializeRefusesSymlinkedRecipeCacheBeforePruning(t *testing.T) {
	t.Setenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES", "")
	const both = `, { id = "bb", source = "bundled" }`
	f := writeMaterializeFixture(t, map[string]string{
		"two": skillPruneRecipeBody(both),
	}, "[recipes.two]\nenabled = true\n")
	writeSkillPruneSkill(t, f.home, "two", "aa")
	writeSkillPruneSkill(t, f.home, "two", "bb")
	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("seed materialize: %v", err)
	}
	// Redirect the recipe's cache directory outside the CLI home.
	outside := t.TempDir()
	skills := filepath.Join(projectcache.RecipeSkillsRoot(f.project, f.home), "two", "skills")
	if err := os.Rename(skills, filepath.Join(outside, "skills")); err != nil {
		t.Fatalf("move skills out: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "skills"), skills); err != nil {
		t.Fatalf("symlink skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.home, "catalog", "recipes", "two", "recipe.toml"), []byte(skillPruneRecipeBody("")), 0o644); err != nil {
		t.Fatalf("rewrite recipe: %v", err)
	}

	if err := runSkillPruneMaterialize(t, f); err == nil {
		t.Fatal("a redirected recipe cache directory must refuse the run")
	}
	if _, err := os.Stat(filepath.Join(outside, "skills", "bb", "SKILL.md")); err != nil {
		t.Errorf("the refusal deleted an outside skill: %v", err)
	}
}
