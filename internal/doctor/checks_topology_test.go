package doctor

import "testing"

func TestCheckRepoTopologyPlainStandalone(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	d := New(root, t.TempDir())
	d.checkRepoTopology()
	assertChecks(t, d, []Check{
		{INFO, "repo-topology", "standalone (via auto; source: default; 0 initialized submodule(s))", ""},
	})
}

func TestCheckRepoTopologyQuietWhenDefaultDisabled(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"x\"\n")
	d := New(root, t.TempDir())
	d.checkRepoTopology()
	assertChecks(t, d, []Check{})
}

func TestCheckRepoTopologyMissingManifest(t *testing.T) {
	d := New(t.TempDir(), t.TempDir())
	d.checkRepoTopology()
	assertChecks(t, d, []Check{})
}

func TestCheckRepoTopologyDeprecated(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root,
		"[recipes.worktree-flow]\nenabled = true\n"+
			"[recipes.worktree-flow.config]\nrepo_topology = \"standalone\"\n")
	d := New(root, t.TempDir())
	d.checkRepoTopology()
	assertChecks(t, d, []Check{
		{INFO, "repo-topology", "standalone (via config; source: legacy-recipe; 0 initialized submodule(s))", ""},
		{WARN, "repo-topology-deprecated",
			"recipes.worktree-flow.config.repo_topology is deprecated; set [project].repo_topology instead", ""},
	})
}
