package doctor

import (
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/target"
)

// writeOverrideRecipe writes <home>/catalog/recipes/<rid>/recipe.toml and
// returns the recipe directory.
func writeOverrideRecipe(t *testing.T, home, rid, body string) string {
	t.Helper()
	dir := filepath.Join(home, "catalog", "recipes", rid)
	writeFile(t, filepath.Join(dir, "recipe.toml"), body)
	return dir
}

// templateRecipeBody is a minimal valid recipe with one not_exists template.
func templateRecipeBody(rid, policy string) string {
	body := "[recipe]\n" +
		"id = \"" + rid + "\"\n" +
		"name = \"My Recipe\"\n" +
		"description = \"desc\"\n" +
		"version = \"1.0.0\"\n\n" +
		"[[provides.templates]]\n" +
		"source = \"templates/out.txt\"\n" +
		"target = \"out.txt\"\n" +
		"condition = \"not_exists\"\n"
	if policy != "" {
		body += "update_policy = \"" + policy + "\"\n"
	}
	return body
}

// overrideManifest enables one recipe, optionally with a config body.
func overrideManifest(rid, config string) string {
	body := "[recipes." + rid + "]\nenabled = true\n"
	if config != "" {
		body += "[recipes." + rid + ".config]\n" + config
	}
	return body
}

// writeOverrideLock writes a lock with a single [managed] entry.
func writeOverrideLock(t *testing.T, root, key, sha string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "ai-specs", ".ai-specs.lock"),
		"[managed.\""+key+"\"]\nsha256 = \""+sha+"\"\n")
}

func TestCheckStaleTemplateOverridesEarlyReturns(t *testing.T) {
	t.Run("no-catalog", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, overrideManifest("myrecipe", ""))
		d := New(root, t.TempDir())
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{})
	})
	t.Run("no-manifest", func(t *testing.T) {
		home := t.TempDir()
		writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
		d := New(t.TempDir(), home)
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{})
	})
}

func TestCheckStaleTemplateOverridesUserModified(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
	writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"), "catalog\n")
	writeFile(t, filepath.Join(root, "out.txt"), "user content\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	writeOverrideLock(t, root, "out.txt", lock.Sha256Bytes([]byte("something else\n")))

	d := New(root, home)
	d.checkStaleTemplateOverrides()
	assertChecks(t, d, []Check{
		{WARN, "stale-override", "out.txt is user-modified; sync will preserve it", "rm out.txt && ai-specs sync"},
	})
}

func TestCheckStaleTemplateOverridesUntracked(t *testing.T) {
	const untrackedMessage = "out.txt has missing ownership metadata; sync will preserve the existing file " +
		"without assigning ownership. Leave it unchanged to preserve it, or remove it and " +
		"rerun sync to restore the current recipe version"

	t.Run("distinct-content-warns", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
		writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"), "catalog\n")
		writeFile(t, filepath.Join(root, "out.txt"), "user content\n")
		writeManifest(t, root, overrideManifest("myrecipe", ""))
		d := New(root, home)
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{
			{WARN, "stale-override", untrackedMessage, "rm out.txt && ai-specs sync"},
		})
	})
	t.Run("matches-rendered-is-quiet", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
		writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"), "catalog\n")
		writeFile(t, filepath.Join(root, "out.txt"), "catalog\n")
		writeManifest(t, root, overrideManifest("myrecipe", ""))
		d := New(root, home)
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{})
	})
	t.Run("matches-legacy-catalog-is-quiet", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
		source := "topology=__WORKTREE_REPO_TOPOLOGY__\n"
		writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"), source)
		writeFile(t, filepath.Join(root, "out.txt"), source)
		writeManifest(t, root, overrideManifest("myrecipe", "repo_topology = \"monorepo-apps\"\n"))
		d := New(root, home)
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{})
	})
}

func TestCheckStaleTemplateOverridesManagedCurrent(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", ""))
	writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"), "catalog\n")
	writeFile(t, filepath.Join(root, "out.txt"), "catalog\n")
	writeManifest(t, root, overrideManifest("myrecipe", ""))
	writeOverrideLock(t, root, "out.txt", lock.Sha256Bytes([]byte("catalog\n")))

	d := New(root, home)
	d.checkStaleTemplateOverrides()
	assertChecks(t, d, []Check{})
}

func TestCheckStaleTemplateOverridesManagedStale(t *testing.T) {
	// The template renders the placeholder to the configured topology while the
	// disk carries "auto": managed bytes, but stale against the would-write.
	setup := func(t *testing.T, policy string) *Doctor {
		t.Helper()
		root := t.TempDir()
		home := t.TempDir()
		writeOverrideRecipe(t, home, "myrecipe", templateRecipeBody("myrecipe", policy))
		writeFile(t, filepath.Join(home, "catalog", "recipes", "myrecipe", "templates", "out.txt"),
			"topology=__WORKTREE_REPO_TOPOLOGY__\n")
		writeFile(t, filepath.Join(root, "out.txt"), "topology=auto\n")
		writeManifest(t, root, overrideManifest("myrecipe", "repo_topology = \"monorepo-apps\"\n"))
		writeOverrideLock(t, root, "out.txt", lock.Sha256Bytes([]byte("topology=auto\n")))
		return New(root, home)
	}

	t.Run("policy-confirm-warns", func(t *testing.T) {
		d := setup(t, "confirm")
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{
			{WARN, "stale-override", "out.txt is managed-stale and policy is confirm; sync will preserve it",
				"rm out.txt && ai-specs sync"},
		})
	})
	t.Run("policy-auto-is-quiet", func(t *testing.T) {
		d := setup(t, "")
		d.checkStaleTemplateOverrides()
		assertChecks(t, d, []Check{})
	})
}

func TestRenderOverrideBytes(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.txt")
	writeFile(t, plain, "no placeholder\n")
	got, err := renderOverrideBytes(plain, nil)
	if err != nil || string(got) != "no placeholder\n" {
		t.Fatalf("plain render = %q, %v", got, err)
	}

	token := filepath.Join(dir, "token.txt")
	writeFile(t, token, "a=__WORKTREE_REPO_TOPOLOGY__\n")
	got, err = renderOverrideBytes(token, map[string]any{"repo_topology": "monorepo-apps"})
	if err != nil || string(got) != "a=monorepo-apps\n" {
		t.Fatalf("token render = %q, %v", got, err)
	}
	got, err = renderOverrideBytes(token, nil)
	if err != nil || string(got) != "a=auto\n" {
		t.Fatalf("token default render = %q, %v", got, err)
	}
}

func TestProjectOwnedRecipeConfig(t *testing.T) {
	root := t.TempDir()
	data := parseTOML(t, "[project]\nname = \"x\"\nrepo_topology = \"standalone\"\n")

	merged := projectOwnedRecipeConfig(root, data, "worktree-flow", parseTOML(t, "other = 1\n"))
	if merged["repo_topology"] != "standalone" || merged["other"] != int64(1) {
		t.Fatalf("worktree-flow merge = %v", merged)
	}

	merged = projectOwnedRecipeConfig(root, data, "other-recipe", parseTOML(t, "other = 1\n"))
	if _, present := merged["repo_topology"]; present {
		t.Fatalf("non-worktree-flow merge must not inject repo_topology: %v", merged)
	}
}

func TestPosixAsPosix(t *testing.T) {
	cases := map[string]string{
		"":          ".",
		"a/b":       "a/b",
		"./a/b":     "a/b",
		"a//b":      "a/b",
		"a/./b":     "a/b",
		"a/../b":    "a/../b",
		"a/b/":      "a/b",
		"/abs/path": "/abs/path",
	}
	for in, want := range cases {
		if got := posixAsPosix(in); got != want {
			t.Errorf("posixAsPosix(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProjectRepoTopologyWrapper pins the exported target wrapper against hand
// computed expectations on the same fixtures the previous doctor-local chain
// consumed; the unchanged checks_topology_test.go pins the check that now
// routes through it.
func TestProjectRepoTopologyWrapper(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name       string
		manifest   string
		resolved   string
		configured string
		via        string
		source     string
	}{
		{"project", "[project]\nname = \"x\"\nrepo_topology = \"standalone\"\n", "standalone", "standalone", "config", "project"},
		{"legacy-recipe", "[recipes.worktree-flow]\nenabled = true\n[recipes.worktree-flow.config]\nrepo_topology = \"standalone\"\n", "standalone", "standalone", "config", "legacy-recipe"},
		{"default", "[project]\nname = \"x\"\n", "standalone", "auto", "auto", "default"},
	}
	for _, tc := range cases {
		data := parseTOML(t, tc.manifest)
		got := target.ProjectRepoTopology(root, data)
		if got.Resolved != tc.resolved || got.Configured != tc.configured || got.Via != tc.via || got.Source != tc.source {
			t.Errorf("%s: %+v, want resolved=%s configured=%s via=%s source=%s",
				tc.name, got, tc.resolved, tc.configured, tc.via, tc.source)
		}
	}
	if got := target.ProjectRepoTopology(root, parseTOML(t, "[project]\nname = \"x\"\n")).Deprecation; got != "" {
		t.Errorf("default deprecation = %q, want empty", got)
	}
	legacy := target.ProjectRepoTopology(root, parseTOML(t, "[recipes.worktree-flow]\nenabled = true\n[recipes.worktree-flow.config]\nrepo_topology = \"standalone\"\n"))
	if legacy.Deprecation == "" {
		t.Error("legacy-recipe source must carry the deprecation marker")
	}
}
