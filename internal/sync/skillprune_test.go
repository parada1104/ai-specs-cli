package sync

// T7b (S3 "borrar siempre lo gestionado"): per-skill orphans inside an ENABLED
// recipe. Dropping a skill from `[provides].skills` must remove the copy
// ai-specs materialized for it under `<cache>/.recipe/<recipe>/skills/<skill>`,
// because the resolved-skills flatten reads that cache tree as its source of
// truth. Ownership is recorded next to the copies (the `.ai-specs-owned-skills`
// marker the materializer writes), so a hand-made SKILL.md-shaped directory is
// never deleted, and a cache written before the record existed is adopted on
// the first run and pruned only from the next one on (T2's rule).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	// A hand-created skill copy under the recipe's cache is NOT recorded as
	// ai-specs-managed, so it survives the prune even in SKILL.md shape.
	unowned := filepath.Join(recipeSkills, "two", "skills", "handmade")
	if err := os.MkdirAll(unowned, 0o755); err != nil {
		t.Fatalf("mkdir unowned: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unowned, "SKILL.md"), []byte("# handmade\n"), 0o644); err != nil {
		t.Fatalf("write unowned: %v", err)
	}
	// A legacy lock may still carry [recipes.<id>.skills.<skill>]; it is read but
	// never emitted, and it does not license the prune (the cache marker does).
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
	if _, err := os.Stat(filepath.Join(unowned, "SKILL.md")); err != nil {
		t.Errorf("an unrecorded skill copy must survive: %v", err)
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

// TestMaterializeAdoptsCacheWithoutOwnershipRecord pins the upgrade path: a
// cache written before the ownership record existed has no marker, so the first
// run records what the recipe declares and prunes nothing unrecorded.
func TestMaterializeAdoptsCacheWithoutOwnershipRecord(t *testing.T) {
	t.Setenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES", "")
	const both = `, { id = "bb", source = "bundled" }`
	f := writeMaterializeFixture(t, map[string]string{
		"two": skillPruneRecipeBody(both),
	}, "[recipes.two]\nenabled = true\n")
	writeSkillPruneSkill(t, f.home, "two", "aa")
	writeSkillPruneSkill(t, f.home, "two", "bb")
	// Simulate the pre-record cache: the copies exist, the marker does not.
	skills := filepath.Join(projectcache.RecipeSkillsRoot(f.project, f.home), "two", "skills")
	for _, skill := range []string{"aa", "bb"} {
		if err := os.MkdirAll(filepath.Join(skills, skill), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", skill, err)
		}
		if err := os.WriteFile(filepath.Join(skills, skill, "SKILL.md"), []byte("# "+skill+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", skill, err)
		}
	}

	// The first run after the upgrade records the declared set, pruning nothing.
	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("adopting materialize: %v", err)
	}
	for _, skill := range []string{"aa", "bb"} {
		if _, err := os.Stat(filepath.Join(skills, skill, "SKILL.md")); err != nil {
			t.Errorf("adoption pruned %s: %v", skill, err)
		}
	}

	// From the next run on the recorded skill is managed: dropping it prunes it.
	if err := os.WriteFile(filepath.Join(f.home, "catalog", "recipes", "two", "recipe.toml"), []byte(skillPruneRecipeBody("")), 0o644); err != nil {
		t.Fatalf("rewrite recipe: %v", err)
	}
	if err := runSkillPruneMaterialize(t, f); err != nil {
		t.Fatalf("materialize after the drop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skills, "bb")); !os.IsNotExist(err) {
		t.Errorf("the recorded dropped skill survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skills, "aa", "SKILL.md")); err != nil {
		t.Errorf("the still-provided skill is gone: %v", err)
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

// TestMaterializeRefusesNonRegularOwnershipMarker pins that deletion authority
// comes only from a regular marker file inside the cache: a symlinked or
// directory marker is never trusted, so the run refuses before the first delete
// (S1).
func TestMaterializeRefusesNonRegularOwnershipMarker(t *testing.T) {
	t.Setenv("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES", "")
	for _, kind := range []string{"symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			const both = `, { id = "bb", source = "bundled" }`
			f := writeMaterializeFixture(t, map[string]string{
				"two": skillPruneRecipeBody(both),
			}, "[recipes.two]\nenabled = true\n")
			writeSkillPruneSkill(t, f.home, "two", "aa")
			writeSkillPruneSkill(t, f.home, "two", "bb")
			if err := runSkillPruneMaterialize(t, f); err != nil {
				t.Fatalf("seed materialize: %v", err)
			}
			recipeRoot := filepath.Join(projectcache.RecipeSkillsRoot(f.project, f.home), "two")
			// A hand-made SKILL.md-shaped copy the marker will claim as its own.
			handmade := filepath.Join(recipeRoot, "skills", "handmade")
			if err := os.MkdirAll(handmade, 0o755); err != nil {
				t.Fatalf("mkdir handmade: %v", err)
			}
			if err := os.WriteFile(filepath.Join(handmade, "SKILL.md"), []byte("# user-authored\n"), 0o644); err != nil {
				t.Fatalf("write handmade: %v", err)
			}
			marker := filepath.Join(recipeRoot, recipeSkillRecordName)
			if err := os.Remove(marker); err != nil {
				t.Fatalf("remove marker: %v", err)
			}
			var outsideRecord string
			if kind == "symlink" {
				outsideRecord = filepath.Join(t.TempDir(), "record")
				if err := os.WriteFile(outsideRecord, []byte("handmade\n"), 0o644); err != nil {
					t.Fatalf("write outside record: %v", err)
				}
				if err := os.Symlink(outsideRecord, marker); err != nil {
					t.Fatalf("symlink marker: %v", err)
				}
			} else if err := os.Mkdir(marker, 0o755); err != nil {
				t.Fatalf("mkdir marker: %v", err)
			}

			// The recipe drops bb: a trusted marker would prune bb anyway, but the
			// point here is that no delete runs at all.
			if err := os.WriteFile(filepath.Join(f.home, "catalog", "recipes", "two", "recipe.toml"), []byte(skillPruneRecipeBody("")), 0o644); err != nil {
				t.Fatalf("rewrite recipe: %v", err)
			}

			err := runSkillPruneMaterialize(t, f)
			if err == nil {
				t.Fatal("a non-regular ownership marker must refuse the run")
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("the refusal does not name the marker: %v", err)
			}
			for _, path := range []string{
				filepath.Join(handmade, "SKILL.md"),
				filepath.Join(recipeRoot, "skills", "aa", "SKILL.md"),
				filepath.Join(recipeRoot, "skills", "bb", "SKILL.md"),
			} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("the refusal deleted %s: %v", path, err)
				}
			}
			if info, err := os.Lstat(marker); err != nil || info.Mode().IsRegular() {
				t.Errorf("the refusal replaced the marker: %v %v", info, err)
			}
			if kind == "symlink" {
				if got, err := os.ReadFile(outsideRecord); err != nil || string(got) != "handmade\n" {
					t.Errorf("the outside record changed: %q %v", got, err)
				}
			}
		})
	}
}
