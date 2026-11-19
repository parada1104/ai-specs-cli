package sync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

const commandRecipeEnabled = "[recipes.command-demo]\nenabled = true\n"
const commandRecipe = "[recipe]\nid = 'command-demo'\nname = 'Demo'\ndescription = 'Fixture'\nversion = '1.0.0'\n"

func commandPruneFixture(t *testing.T) (root, home, base string, run func() (int, string, string)) {
	t.Helper()
	home, _ = spineEnv(t)
	if err := os.Rename(filepath.Join(home, "catalog"), filepath.Join(home, "original-catalog")); err != nil {
		t.Fatal(err)
	}
	base = "[project]\nname = 'commands'\n[brief]\nrender = false\n[agents]\nenabled = ['claude', 'cursor', 'opencode', 'omp']\n"
	root = spinePlan(t, spineProject(t, base+commandRecipeEnabled)).Root
	recipe := filepath.Join(home, "catalog/recipes/command-demo")
	spineWrite(t, filepath.Join(recipe, "recipe.toml"), commandRecipe+"[provides]\ncommands = [{id = 'demo', path = 'demo.md'}]\n", 0o644)
	spineWrite(t, filepath.Join(recipe, "demo.md"), "# generated\n", 0o644)
	return root, home, base, func() (int, string, string) {
		var out, errW bytes.Buffer
		rc := Run([]string{"sync", root}, home, strings.NewReader(""), &out, &errW)
		return rc, out.String(), errW.String()
	}
}

func TestSyncPrunesRecipeCommands(t *testing.T) {
	for _, variant := range []string{"disabled", "removed", "dropped"} {
		t.Run(variant, func(t *testing.T) {
			root, home, base, run := commandPruneFixture(t)
			check := func(wantErr string) {
				t.Helper()
				if rc, out, errW := run(); rc != 0 || !strings.HasSuffix(out, "\n✓ ai-specs sync complete\n") || errW != wantErr {
					t.Errorf("sync rc=%d, stderr=%q, want %q; stdout=%s", rc, errW, wantErr, out)
				}
			}
			check("")
			cache := projectcache.CacheRoot(root, home)
			dirs := []string{cache + "/commands", cache + "/merged-commands", root + "/.claude/commands", root + "/.cursor/commands", root + "/.opencode/commands", root + "/.omp/commands"}
			for _, dir := range dirs {
				if !isRegularFilePath(filepath.Join(dir, "demo.md")) {
					t.Fatalf("seed missed %s", dir)
				}
			}
			user := filepath.Join(root, ".claude/commands/user.md")
			spineWrite(t, user, "# user\n", 0o644)
			manifest := map[string]string{"disabled": "[recipes.command-demo]\nenabled = false\n", "dropped": commandRecipeEnabled}[variant]
			spineWrite(t, materializeManifestPath(root), base+manifest, 0o644)
			if variant == "dropped" {
				spineWrite(t, filepath.Join(home, "catalog/recipes/command-demo/recipe.toml"), commandRecipe, 0o644)
			}
			for range 2 {
				check("    ! preserved non-managed file .claude/commands/user.md (move it to ai-specs/commands/ to manage it)\n")
				for _, dir := range dirs {
					if _, err := os.Lstat(filepath.Join(dir, "demo.md")); !os.IsNotExist(err) {
						t.Errorf("stale command survived in %s: %v", dir, err)
					}
				}
				if b, err := os.ReadFile(user); err != nil || string(b) != "# user\n" {
					t.Errorf("user command changed: %q, %v", b, err)
				}
			}
		})
	}
}

func TestSyncRecipeCommandPruneRefusal(t *testing.T) {
	root, home, base, run := commandPruneFixture(t)
	if rc, _, errW := run(); rc != 0 {
		t.Fatal(errW)
	}
	spineWrite(t, materializeManifestPath(root), base, 0o644)
	cache := projectcache.CacheRoot(root, home)
	leaf := filepath.Join(root, ".omp/commands/demo.md")
	if err := os.Remove(leaf); err != nil {
		t.Fatal(err)
	}
	spineWrite(t, filepath.Join(leaf, "user"), "keep", 0o644)
	for _, dir := range []string{root, cache} {
		before, modes := hooksSnapshot(t, dir)
		t.Cleanup(func() {
			after, afterModes := hooksSnapshot(t, dir)
			if !hooksTreeEqual(before, after) || !hooksModesEqual(modes, afterModes) {
				t.Errorf("refusal changed %s:\n%s", dir, hooksTreeDiff(before, after))
			}
		})
	}
	if rc, out, errW := run(); rc != 1 || errW != fmt.Sprintf("ERROR: cannot prune recipe commands: destination is a directory: %s\n", leaf) || strings.Contains(out, "  syncing ") {
		t.Errorf("refusal rc=%d, stderr=%q; stdout=%s", rc, errW, out)
	}
}
