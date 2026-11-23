package doctor

import (
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

// hookRecipeBody is a minimal valid recipe with one runtime hook.
func hookRecipeBody(rid string) string {
	return "[recipe]\n" +
		"id = \"" + rid + "\"\n" +
		"name = \"My Recipe\"\n" +
		"description = \"desc\"\n" +
		"version = \"1.0.0\"\n\n" +
		"[[provides.hooks]]\n" +
		"id = \"my-hook\"\n" +
		"event = \"pre-tool-use\"\n" +
		"script = \"hooks/gate.sh\"\n"
}

func writeHookRecipe(t *testing.T, home, rid string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "catalog", "recipes", rid, "recipe.toml"), hookRecipeBody(rid))
}

func TestCheckGateProvenanceEarlyReturns(t *testing.T) {
	t.Run("no-catalog", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, overrideManifest("myrecipe", ""))
		d := New(root, t.TempDir())
		d.checkGateProvenance()
		assertChecks(t, d, []Check{})
	})
	t.Run("no-manifest", func(t *testing.T) {
		home := t.TempDir()
		writeHookRecipe(t, home, "myrecipe")
		d := New(t.TempDir(), home)
		d.checkGateProvenance()
		assertChecks(t, d, []Check{})
	})
}

func TestCheckGateProvenanceBaselineMatch(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeHookRecipe(t, home, "myrecipe")
	rel := "ai-specs/recipes/myrecipe/hooks/gate.sh"
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "#!/bin/sh\nexit 0\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	writeFile(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
		"[managed.\""+rel+"\"]\nsha256 = \""+lock.Sha256Bytes([]byte("#!/bin/sh\nexit 0\n"))+"\"\n")

	d := New(root, home)
	d.checkGateProvenance()
	assertChecks(t, d, []Check{})
}

func TestCheckGateProvenanceDeduplicatesRepeatedHookPaths(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	body := hookRecipeBody("myrecipe") + "\n[[provides.hooks]]\nid = \"my-hook-again\"\nevent = \"pre-tool-use\"\nscript = \"other/gate.sh\"\n"
	writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "recipe.toml"), body)
	rel := "ai-specs/recipes/myrecipe/hooks/gate.sh"
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "#!/bin/sh\nexit 0\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	writeFile(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
		"[managed.\""+rel+"\"]\nsha256 = \""+lock.Sha256Bytes([]byte("older bytes"))+"\"\n")

	d := New(root, home)
	d.checkGateProvenance()
	assertChecks(t, d, []Check{
		{WARN, "gate-provenance", rel + " is user-modified; sync will preserve it",
			"rm " + rel + " && ai-specs sync (or ai-specs sync --refresh-gates)"},
	})
}

func TestCheckGateProvenanceUserModified(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeHookRecipe(t, home, "myrecipe")
	rel := "ai-specs/recipes/myrecipe/hooks/gate.sh"
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "#!/bin/sh\nexit 0\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	writeFile(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
		"[managed.\""+rel+"\"]\nsha256 = \""+lock.Sha256Bytes([]byte("older bytes"))+"\"\n")

	d := New(root, home)
	d.checkGateProvenance()
	assertChecks(t, d, []Check{
		{WARN, "gate-provenance", rel + " is user-modified; sync will preserve it",
			"rm " + rel + " && ai-specs sync (or ai-specs sync --refresh-gates)"},
	})
}

func TestCheckGateProvenanceUntracked(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeHookRecipe(t, home, "myrecipe")
	rel := "ai-specs/recipes/myrecipe/hooks/gate.sh"
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "#!/bin/sh\nexit 0\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))

	d := New(root, home)
	d.checkGateProvenance()
	assertChecks(t, d, []Check{
		{WARN, "gate-provenance",
			rel + " has no recorded provenance; sync will preserve it " +
				"and record a baseline only when the CLI renders the gate",
			"rm " + rel + " && ai-specs sync (or ai-specs sync --refresh-gates)"},
	})
}

func TestCheckGateProvenanceMissingTrackedScript(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeHookRecipe(t, home, "myrecipe")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	d := New(root, home)
	d.checkGateProvenance()
	assertChecks(t, d, []Check{})
}

// TestCheckGateProvenanceSkipsCLIServedScripts pins the single-owner rule: a
// stale launcher left at a CLI-served hook path is reported by
// checkWorktreeGate, never as a provenance question.
func TestCheckGateProvenanceSkipsCLIServedScripts(t *testing.T) {
	for _, script := range []string{"worktree-gate.sh", "tracker-card-gate.sh", "plan-build-gate.sh"} {
		t.Run(script, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			body := "[recipe]\n" +
				"id = \"myrecipe\"\n" +
				"name = \"My Recipe\"\n" +
				"description = \"desc\"\n" +
				"version = \"1.0.0\"\n\n" +
				"[[provides.hooks]]\n" +
				"id = \"my-hook\"\n" +
				"event = \"pre-tool-use\"\n" +
				"script = \"hooks/" + script + "\"\n"
			writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "recipe.toml"), body)
			writeFile(t, filepath.Join(root, "ai-specs", "recipes", "myrecipe", "hooks", script), "#!/bin/sh\nexit 0\n")
			writeManifest(t, root, overrideManifest("myrecipe", ""))

			d := New(root, home)
			d.checkGateProvenance()
			assertChecks(t, d, []Check{})
		})
	}
}
