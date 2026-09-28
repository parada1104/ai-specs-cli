package target

// Go port of the util.py topology chain that lib/_internal/target-resolve.py
// actually uses: TopologyResolution, detect_submodules, _run_git_config_paths,
// _run_submodule_status, resolve_repo_topology, project_manifest_data,
// _legacy_recipe_topology, topology_config, project_repo_topology, and
// ProjectTopology. Unrelated util.py code is deliberately not ported.
//
// Semantics (docs/go-migration-parity-contract.md): `auto` never resolves to
// monorepo-apps; git failures degrade to standalone without raising; unknown
// configured values take the auto branch (and are reported as "auto").

import (
	"os"
	"os/exec"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

const (
	legacyTopologyRecipeID        = "worktree-flow"
	repoTopologyKey               = "repo_topology"
	topologySourceProject         = "project"
	topologySourceLegacy          = "legacy-recipe"
	topologySourceDefault         = "default"
	legacyRepoTopologyDeprecation = "recipes.worktree-flow.config.repo_topology is deprecated; set " +
		"[project].repo_topology instead"
)

// TopologyResolution mirrors util.TopologyResolution.
type TopologyResolution struct {
	Resolved          string   // "standalone" | "monorepo-apps" | "monorepo-submodules"
	Configured        string   // "auto" | one of the above
	Via               string   // "config" (explicit) | "auto" (detected)
	Submodules        []string // initialized submodule paths (rel to repo_root)
	GitmodulesPresent bool
}

// ProjectTopology mirrors util.ProjectTopology. Deprecation is "" for None.
type ProjectTopology struct {
	Resolved          string
	Configured        string
	Via               string
	Source            string // "project" | "legacy-recipe" | "default"
	Deprecation       string
	Submodules        []string
	GitmodulesPresent bool
}

// runGitConfigPaths returns the submodule paths registered in .gitmodules
// via `git config -f .gitmodules --get-regexp`. Empty set on any failure.
func runGitConfigPaths(repoRoot string) map[string]bool {
	out, code, ok := runGit(repoRoot, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	paths := map[string]bool{}
	if !ok || code != 0 {
		return paths
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Python line.split(None, 1): first whitespace-run token + rest.
		idx := strings.IndexFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		if idx < 0 {
			continue
		}
		paths[strings.TrimSpace(line[idx+1:])] = true
	}
	return paths
}

// runSubmoduleStatus returns raw `git submodule status` lines (non-recursive).
func runSubmoduleStatus(repoRoot string) []string {
	out, code, ok := runGit(repoRoot, "submodule", "status")
	if !ok || code != 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// runGit runs git -C repoRoot args; ok=false when git could not run at all
// (OSError/FileNotFoundError in the Python original).
func runGit(repoRoot string, args ...string) (stdout string, exitCode int, ok bool) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, isExit := err.(*exec.ExitError); isExit {
			return string(out), ee.ExitCode(), true
		}
		return "", 1, false
	}
	return string(out), 0, true
}

// detectSubmodules mirrors util.detect_submodules: (gitmodules_present,
// initialized submodule paths). Non-recursive (v1). Status prefixes
// ' ', '+', 'U' count as initialized; '-' is skipped.
func detectSubmodules(repoRoot string) (bool, []string) {
	if !isFile(repoRoot + "/.gitmodules") {
		return false, nil
	}
	registered := runGitConfigPaths(repoRoot)
	var initialized []string
	for _, line := range runSubmoduleStatus(repoRoot) {
		if line == "" {
			continue
		}
		prefix := line[0]
		rest := strings.Fields(line[1:]) // Python line[1:].split()
		if len(rest) < 2 {
			continue
		}
		path := rest[1]
		if !registered[path] {
			continue
		}
		if prefix != '-' {
			initialized = append(initialized, path)
		}
	}
	sort.Strings(initialized)
	return true, initialized
}

// resolveRepoTopology mirrors util.resolve_repo_topology.
func resolveRepoTopology(repoRoot, configValue string) TopologyResolution {
	configured := configValue
	if configured == "" {
		configured = "auto"
	}
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = "auto"
	}

	if configured == "standalone" || configured == "monorepo-apps" {
		return TopologyResolution{
			Resolved:   configured,
			Configured: configured,
			Via:        "config",
		}
	}

	// configured == "monorepo-submodules" or "auto" (or any unknown value).
	// The Python original wraps detection in try/except and degrades to
	// standalone; detection here never errors, so the degradation path is
	// structural (git absent/failing → empty submodules).
	present, subs := detectSubmodules(repoRoot)
	if configured == "monorepo-submodules" {
		return TopologyResolution{
			Resolved:          "monorepo-submodules",
			Configured:        configured,
			Via:               "config",
			Submodules:        subs,
			GitmodulesPresent: present,
		}
	}
	resolved := "standalone"
	if len(subs) > 0 {
		resolved = "monorepo-submodules"
	}
	return TopologyResolution{
		Resolved:          resolved,
		Configured:        "auto",
		Via:               "auto",
		Submodules:        subs,
		GitmodulesPresent: present,
	}
}

// projectManifestData mirrors util.project_manifest_data: best-effort parse
// of <root>/ai-specs/ai-specs.toml; nil when absent or unreadable.
func projectManifestData(projectRoot string) *toml.Table {
	manifest := projectRoot + "/ai-specs/ai-specs.toml"
	if !isFile(manifest) {
		return nil
	}
	data, err := toml.Parse([]byte(readFileOrEmpty(manifest)))
	if err != nil {
		return nil
	}
	return data
}

// legacyRecipeTopology mirrors util._legacy_recipe_topology.
func legacyRecipeTopology(data *toml.Table) string {
	recipes := tableOf(data, "recipes")
	if recipes == nil {
		return ""
	}
	recipe := tableOf(recipes, legacyTopologyRecipeID)
	if recipe == nil {
		return ""
	}
	config := tableOf(recipe, "config")
	if config == nil {
		config = recipe // flat style: keys directly under the recipe table
	}
	value, ok := config.Get(repoTopologyKey)
	if !ok {
		return ""
	}
	s, isStr := value.(string)
	if !isStr || strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimSpace(s)
}

// topologyConfig mirrors util.topology_config: (configured, source,
// deprecation). [project].repo_topology wins; the worktree-flow recipe key is
// a deprecated compatibility alias; else "auto"/default.
func topologyConfig(data *toml.Table) (configured, source, deprecation string) {
	project := tableOf(data, "project")
	if project != nil {
		if value, ok := project.Get(repoTopologyKey); ok {
			if s, isStr := value.(string); isStr && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s), topologySourceProject, ""
			}
		}
	}
	if legacy := legacyRecipeTopology(data); legacy != "" {
		return legacy, topologySourceLegacy, legacyRepoTopologyDeprecation
	}
	return "auto", topologySourceDefault, ""
}

// projectRepoTopology mirrors util.project_repo_topology. A nil data falls
// back to a best-effort manifest read.
func projectRepoTopology(projectRoot string, data *toml.Table) ProjectTopology {
	if data == nil {
		data = projectManifestData(projectRoot)
	}
	configured, source, deprecation := topologyConfig(data)
	resolution := resolveRepoTopology(projectRoot, configured)
	return ProjectTopology{
		Resolved:          resolution.Resolved,
		Configured:        resolution.Configured,
		Via:               resolution.Via,
		Source:            source,
		Deprecation:       deprecation,
		Submodules:        resolution.Submodules,
		GitmodulesPresent: resolution.GitmodulesPresent,
	}
}

// --- small fs / toml helpers --------------------------------------------------

// isFile mirrors Path.is_file(): exists and not a directory (follows symlinks).
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// tableOf is the nil-safe Table lookup mirroring dict.get + isinstance(dict).
func tableOf(t *toml.Table, key string) *toml.Table {
	if t == nil {
		return nil
	}
	v, ok := t.Table(key)
	if !ok {
		return nil
	}
	return v
}
