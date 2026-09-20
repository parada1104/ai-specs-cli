package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Resolved-config projection, moved from the Python authority
// build_resolved_config (lib/_internal/recipe-materialize.py:1776). Go owns the
// projection decision — bindings, per-recipe config, enabled order, project
// root and topology — as a pure function of the manifest. Python keeps a
// fail-open compatibility bridge this slice.
//
// Topology placement mirrors Python util.resolve_repo_topology: an explicit
// standalone/monorepo-apps value passes through; monorepo-submodules and auto
// inspect the worktree; a git failure degrades to standalone without raising.

const (
	topologySourceProject      = "project"
	topologySourceLegacyRecipe = "legacy-recipe"
	topologySourceDefault      = "default"
	topologyViaConfig          = "config"
	topologyViaAuto            = "auto"
)

// manifestEntry is one ordered [recipes.<id>] pair as the reader emits it: the
// id plus the raw table. A non-table value carries a nil Table and is skipped
// by the projection.
type manifestEntry struct {
	ID    string         `json:"id"`
	Table map[string]any `json:"table"`
}

// manifestPayload is the decoded reader envelope: manifest-ordered recipe
// pairs, the raw [[bindings]] list, and the raw [project] table.
type manifestPayload struct {
	Recipes  []manifestEntry  `json:"recipes"`
	Bindings []map[string]any `json:"bindings"`
	Project  map[string]any   `json:"project"`
}

// resolvedTopology is the topology member of the resolved config. Every key is
// always present; Submodules is [] rather than null.
type resolvedTopology struct {
	Resolved          string   `json:"resolved"`
	Configured        string   `json:"configured"`
	Via               string   `json:"via"`
	Source            string   `json:"source"`
	Submodules        []string `json:"submodules"`
	GitmodulesPresent bool     `json:"gitmodules_present"`
}

// resolvedConfig is the stdout contract of --plan-resolved-config.
type resolvedConfig struct {
	Bindings    map[string]string         `json:"bindings"`
	Recipes     map[string]map[string]any `json:"recipes"`
	Enabled     []string                  `json:"enabled"`
	ProjectRoot string                    `json:"project_root"`
	Topology    resolvedTopology          `json:"topology"`
}

// projectRecipes mirrors the Python recipes loop: config merges the
// [recipes.<id>.config] table with flat keys, drops enabled/version, and
// preserves any other flat key. A nil or empty table is skipped.
func projectRecipes(entries []manifestEntry) map[string]map[string]any {
	out := make(map[string]map[string]any, len(entries))
	for _, e := range entries {
		if len(e.Table) == 0 {
			continue
		}
		config := make(map[string]any, len(e.Table))
		for key, value := range e.Table {
			if key == "enabled" || key == "version" {
				continue
			}
			if key == "config" {
				if table, ok := value.(map[string]any); ok {
					for innerKey, innerValue := range table {
						config[innerKey] = innerValue
					}
					continue
				}
			}
			config[key] = value
		}
		out[e.ID] = config
	}
	return out
}

// projectEnabled returns the ids whose table sets enabled to exactly the bool
// true, in manifest order.
func projectEnabled(entries []manifestEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if enabled, ok := e.Table["enabled"].(bool); ok && enabled {
			out = append(out, e.ID)
		}
	}
	return out
}

// projectBindings builds {capability: recipe} from the explicit manifest
// bindings. An entry missing either non-empty string is skipped.
func projectBindings(bindings []map[string]any) map[string]string {
	out := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		capability, capOK := binding["capability"].(string)
		recipe, recipeOK := binding["recipe"].(string)
		if !capOK || !recipeOK || capability == "" || recipe == "" {
			continue
		}
		out[capability] = recipe
	}
	return out
}

// topologyConfigured returns the configured topology and where it came from.
// [project].repo_topology wins; the worktree-flow recipe key is a deprecated
// compatibility alias; else auto.
func topologyConfigured(project map[string]any, entries []manifestEntry) (string, string) {
	if configured, ok := nonEmptyString(project["repo_topology"]); ok {
		return configured, topologySourceProject
	}
	for _, e := range entries {
		if e.ID != "worktree-flow" {
			continue
		}
		if legacy, ok := legacyRepoTopology(e.Table); ok {
			return legacy, topologySourceLegacyRecipe
		}
	}
	return "auto", topologySourceDefault
}

// legacyRepoTopology reads recipes.worktree-flow's repo_topology: the config
// sub-table when present, otherwise the flat keys minus enabled/version.
func legacyRepoTopology(table map[string]any) (string, bool) {
	if len(table) == 0 {
		return "", false
	}
	config := table
	if inner, ok := table["config"].(map[string]any); ok {
		config = inner
	} else {
		flat := make(map[string]any, len(table))
		for key, value := range table {
			if key == "enabled" || key == "version" {
				continue
			}
			flat[key] = value
		}
		config = flat
	}
	return nonEmptyString(config["repo_topology"])
}

// nonEmptyString returns a trimmed, non-empty string value.
func nonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

// detectSubmodules mirrors Python util.detect_submodules: no .gitmodules means
// nothing to inspect; otherwise git reports the registered paths and the
// initialized ones. Git failures degrade to empty sets, never to an error.
func detectSubmodules(root string) (bool, []string, error) {
	if !isFile(filepath.Join(root, ".gitmodules")) {
		return false, []string{}, nil
	}
	registered := map[string]bool{}
	for _, line := range strings.Split(git(root, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		registered[fields[1]] = true
	}
	var initialized []string
	for _, line := range strings.Split(git(root, "submodule", "status"), "\n") {
		if line == "" {
			continue
		}
		prefix := line[0]
		fields := strings.Fields(line[1:])
		if len(fields) < 2 {
			continue
		}
		path := fields[1]
		if !registered[path] {
			continue
		}
		if prefix != '-' {
			initialized = append(initialized, path)
		}
	}
	return true, sortedUnique(initialized), nil
}

// resolveRepoTopology resolves one configured value against the worktree. The
// Source field is filled by buildResolvedConfig, which owns the manifest read.
func resolveRepoTopology(root, configured string) resolvedTopology {
	if configured == "standalone" || configured == "monorepo-apps" {
		return resolvedTopology{Resolved: configured, Configured: configured, Via: topologyViaConfig, Submodules: []string{}}
	}
	if configured == "monorepo-submodules" {
		present, subs, err := detectSubmodules(root)
		if err != nil {
			return resolvedTopology{Resolved: "standalone", Configured: configured, Via: topologyViaConfig, Submodules: []string{}}
		}
		return resolvedTopology{Resolved: configured, Configured: configured, Via: topologyViaConfig, Submodules: subs, GitmodulesPresent: present}
	}
	present, subs, err := detectSubmodules(root)
	if err != nil {
		return resolvedTopology{Resolved: "standalone", Configured: "auto", Via: topologyViaAuto, Submodules: []string{}}
	}
	resolved := "standalone"
	if len(subs) > 0 {
		resolved = "monorepo-submodules"
	}
	return resolvedTopology{Resolved: resolved, Configured: "auto", Via: topologyViaAuto, Submodules: subs, GitmodulesPresent: present}
}

// buildResolvedConfig assembles the whole envelope from the decoded manifest.
func buildResolvedConfig(payload manifestPayload, projectRoot string) resolvedConfig {
	configured, source := topologyConfigured(payload.Project, payload.Recipes)
	topology := resolveRepoTopology(projectRoot, configured)
	topology.Source = source
	return resolvedConfig{
		Bindings:    projectBindings(payload.Bindings),
		Recipes:     projectRecipes(payload.Recipes),
		Enabled:     projectEnabled(payload.Recipes),
		ProjectRoot: RealPath(projectRoot),
		Topology:    topology,
	}
}

// resolvedConfigReader is the TOML boundary for the projection: the standard
// library parser deserializes the manifest and prints the manifest-ordered
// recipes pairs, the raw bindings list and the raw project table. It selects
// nothing — every projection decision stays in Go — and it is isolated (-I -B)
// under the same bounded execution as the existing manifest seams.
const resolvedConfigReader = `import json, sys, tomllib
with open(sys.argv[1], "rb") as handle:
    data = tomllib.load(handle)
recipes = data.get("recipes")
if not isinstance(recipes, dict):
    recipes = {}
bindings = data.get("bindings")
if not isinstance(bindings, list):
    bindings = []
project = data.get("project")
if not isinstance(project, dict):
    project = {}
print(json.dumps({
    "recipes": [[rid, value] for rid, value in recipes.items()],
    "bindings": bindings,
    "project": project,
}))
`

// manifestEnvelope decodes the reader output before projection. Recipes stay as
// raw pairs so a non-table recipe value is skipped rather than rejected.
type manifestEnvelope struct {
	Recipes  [][]json.RawMessage `json:"recipes"`
	Bindings []json.RawMessage   `json:"bindings"`
	Project  map[string]any      `json:"project"`
}

// decodeManifestPayload turns the reader output into the projection payload.
// Non-table recipe values carry a nil Table; non-table bindings are skipped.
func decodeManifestPayload(raw []byte) (manifestPayload, error) {
	var envelope manifestEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return manifestPayload{}, fmt.Errorf("deserialized manifest: %w", err)
	}
	payload := manifestPayload{
		Recipes:  make([]manifestEntry, 0, len(envelope.Recipes)),
		Bindings: make([]map[string]any, 0, len(envelope.Bindings)),
		Project:  envelope.Project,
	}
	if payload.Project == nil {
		payload.Project = map[string]any{}
	}
	for _, pair := range envelope.Recipes {
		if len(pair) != 2 {
			continue
		}
		var id string
		if err := json.Unmarshal(pair[0], &id); err != nil {
			return manifestPayload{}, fmt.Errorf("deserialized manifest: recipe id: %w", err)
		}
		entry := manifestEntry{ID: id}
		if err := json.Unmarshal(pair[1], &entry.Table); err != nil {
			entry.Table = nil
		}
		payload.Recipes = append(payload.Recipes, entry)
	}
	for _, rawBinding := range envelope.Bindings {
		var binding map[string]any
		if err := json.Unmarshal(rawBinding, &binding); err != nil {
			continue
		}
		payload.Bindings = append(payload.Bindings, binding)
	}
	return payload, nil
}

// runResolvedConfigParser runs the projection reader under the bounded
// execution shared with the other manifest seams: one deadline, capped stdout
// and stderr, an explicit isolated interpreter, and classified failures.
func runResolvedConfigParser(root, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestParseTimeout)
	defer cancel()

	interpreter, err := manifestInterpreter(root)
	if err != nil {
		return nil, err
	}
	stdout := &cappedBuffer{max: acquisitionLimit}
	stderr := &cappedBuffer{max: manifestErrorLimit}
	cmd := exec.CommandContext(ctx, interpreter, "-I", "-B", "-c", resolvedConfigReader, path)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = manifestParseTimeout

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("standard TOML parser timed out after %s", manifestParseTimeout)
	}
	if runErr != nil {
		return nil, classifyManifestParserError(runErr)
	}
	if stdout.truncated {
		return nil, fmt.Errorf("standard TOML parser returned more than %d bytes", acquisitionLimit)
	}
	return stdout.buf.Bytes(), nil
}

// runPlanResolvedConfig is the --plan-resolved-config command: read the project
// manifest through the TOML seam, project it, and print one JSON envelope. A
// missing or unparsable manifest is a process-level failure (exit 2, no stdout).
func runPlanResolvedConfig(projectRoot string, stdout, stderr io.Writer) int {
	path := manifestPath(projectRoot)
	if err := regularFile(path); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-resolved-config: manifest %s: %v\n", path, err)
		return 2
	}
	raw, err := runResolvedConfigParser(projectRoot, path)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-resolved-config: manifest %s: %v\n", path, err)
		return 2
	}
	payload, err := decodeManifestPayload(raw)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-resolved-config: manifest %s: %v\n", path, err)
		return 2
	}
	encoded, err := json.Marshal(buildResolvedConfig(payload, projectRoot))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-resolved-config: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}
