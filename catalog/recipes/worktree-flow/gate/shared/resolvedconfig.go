package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Resolved-config projection: the pure decision core extracted from the gate
// main package (GO-07 S13 PR3) so the gate binary's --plan-resolved-config
// surface and the root native sync authority share one implementation. It was
// moved from the Python authority build_resolved_config
// (lib/_internal/recipe-materialize.py): Go owns the projection decision —
// bindings, per-recipe config, enabled order, project root and topology — as a
// pure function of the manifest. Python keeps a fail-open compatibility bridge.
//
// Topology placement mirrors Python util.resolve_repo_topology: an explicit
// standalone/monorepo-apps value passes through; monorepo-submodules and auto
// inspect the worktree; a git failure degrades to standalone without raising.
//
// Acquisition (running the TOML reader and parsing the flag) stays per-side;
// only the decision lives here. detectSubmodules cannot call the main package's
// unexported git()/isFile helpers across the boundary, so the git fact is
// forwarded as a function value (DetectSubmodulesWith), the seam shape already
// established by GitCommonDirWith.

const (
	// TopologySourceProject marks a [project].repo_topology value.
	TopologySourceProject = "project"
	// TopologySourceLegacyRecipe marks the deprecated worktree-flow recipe key.
	TopologySourceLegacyRecipe = "legacy-recipe"
	// TopologySourceDefault marks the auto fallback when nothing is configured.
	TopologySourceDefault = "default"
	// TopologyViaConfig marks an explicitly configured topology.
	TopologyViaConfig = "config"
	// TopologyViaAuto marks a detected topology.
	TopologyViaAuto = "auto"
)

// ManifestEntry is one ordered [recipes.<id>] pair as the reader emits it: the
// id plus the raw table. A non-table value carries a nil Table and is skipped
// by the projection.
type ManifestEntry struct {
	ID    string         `json:"id"`
	Table map[string]any `json:"table"`
}

// ManifestPayload is the decoded reader envelope: manifest-ordered recipe
// pairs, the raw [[bindings]] list, and the raw [project] table.
type ManifestPayload struct {
	Recipes  []ManifestEntry  `json:"recipes"`
	Bindings []map[string]any `json:"bindings"`
	Project  map[string]any   `json:"project"`
}

// ResolvedTopology is the topology member of the resolved config. Every key is
// always present; Submodules is [] rather than null.
type ResolvedTopology struct {
	Resolved          string   `json:"resolved"`
	Configured        string   `json:"configured"`
	Via               string   `json:"via"`
	Source            string   `json:"source"`
	Submodules        []string `json:"submodules"`
	GitmodulesPresent bool     `json:"gitmodules_present"`
}

// ResolvedConfig is the stdout contract of --plan-resolved-config.
type ResolvedConfig struct {
	Bindings    map[string]string         `json:"bindings"`
	Recipes     map[string]map[string]any `json:"recipes"`
	Enabled     []string                  `json:"enabled"`
	ProjectRoot string                    `json:"project_root"`
	Topology    ResolvedTopology          `json:"topology"`
}

// ProjectRecipes mirrors the Python recipes loop: config merges the
// [recipes.<id>.config] table with flat keys, drops enabled/version, and
// preserves any other flat key. A nil table (a non-table recipe value Python
// skips with isinstance(val, dict)) is dropped, but a present empty table is
// kept as {}, exactly as _python_build_resolved_config assigns
// recipes_out[rid] = config unconditionally.
//
// The flat keys are applied first and the config sub-table last. TOML requires
// the flat keys of [recipes.<id>] to be declared before the [recipes.<id>.config]
// header, so a name collision always resolves to the nested value in Python's
// document walk; applying the sub-table last reproduces that outcome without
// depending on Go's map iteration order.
func ProjectRecipes(entries []ManifestEntry) map[string]map[string]any {
	out := make(map[string]map[string]any, len(entries))
	for _, e := range entries {
		if e.Table == nil {
			continue
		}
		config := make(map[string]any, len(e.Table))
		nested, hasNested := e.Table["config"].(map[string]any)
		for key, value := range e.Table {
			if key == "enabled" || key == "version" {
				continue
			}
			if key == "config" && hasNested {
				continue
			}
			config[key] = value
		}
		if hasNested {
			for innerKey, innerValue := range nested {
				config[innerKey] = innerValue
			}
		}
		out[e.ID] = config
	}
	return out
}

// ProjectEnabled returns the ids whose table sets enabled to exactly the bool
// true, in manifest order.
func ProjectEnabled(entries []ManifestEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if enabled, ok := e.Table["enabled"].(bool); ok && enabled {
			out = append(out, e.ID)
		}
	}
	return out
}

// ProjectBindings builds {capability: recipe} from the explicit manifest
// bindings. An entry whose capability or recipe is falsy is skipped, exactly as
// _python_build_resolved_config's `if cap and rec` guard does; a truthy
// non-string scalar is coerced instead of dropped. The capability is rendered
// the way json.dumps renders a non-string dict key (an integer 5 keys as "5",
// a boolean true as "true") and the recipe is rendered with Python str(),
// because the shared envelope contract is a str-valued mapping.
func ProjectBindings(bindings []map[string]any) map[string]string {
	out := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		capability, capOK := pyBindingScalar(binding["capability"], true)
		recipe, recipeOK := pyBindingScalar(binding["recipe"], false)
		if !capOK || !recipeOK {
			continue
		}
		out[capability] = recipe
	}
	return out
}

// pyBindingScalar renders one [[bindings]] scalar and reports whether Python's
// `if capability and recipe` guard would keep it. Every falsy value (None,
// False, 0, 0.0, "", an empty container) is skipped. jsonKey selects Python's
// json.dumps object-key rendering (bools lowercase as true/false, matching the
// JSON envelope the bridge decodes) over the str() rendering used for values.
func pyBindingScalar(value any, jsonKey bool) (string, bool) {
	switch t := value.(type) {
	case nil:
		return "", false
	case bool:
		if !t {
			return "", false
		}
		if jsonKey {
			return "true", true
		}
		return "True", true
	case string:
		if t == "" {
			return "", false
		}
		return t, true
	case int:
		if t == 0 {
			return "", false
		}
		return strconv.Itoa(t), true
	case int64:
		if t == 0 {
			return "", false
		}
		return strconv.FormatInt(t, 10), true
	case float64:
		if t == 0 {
			return "", false
		}
		return pyString(t), true
	case json.Number:
		if !pyTruthy(t) {
			return "", false
		}
		return t.String(), true
	}
	return "", false
}

// TopologyConfigured returns the configured topology and where it came from.
// [project].repo_topology wins; the worktree-flow recipe key is a deprecated
// compatibility alias; else auto.
func TopologyConfigured(project map[string]any, entries []ManifestEntry) (string, string) {
	if configured, ok := NonEmptyString(project["repo_topology"]); ok {
		return configured, TopologySourceProject
	}
	for _, e := range entries {
		if e.ID != "worktree-flow" {
			continue
		}
		if legacy, ok := LegacyRepoTopology(e.Table); ok {
			return legacy, TopologySourceLegacyRecipe
		}
	}
	return "auto", TopologySourceDefault
}

// LegacyRepoTopology reads recipes.worktree-flow's repo_topology: the config
// sub-table when present, otherwise the flat keys minus enabled/version.
func LegacyRepoTopology(table map[string]any) (string, bool) {
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
	return NonEmptyString(config["repo_topology"])
}

// NonEmptyString returns a trimmed, non-empty string value.
func NonEmptyString(value any) (string, bool) {
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

// DetectSubmodules mirrors Python util.detect_submodules: no .gitmodules means
// nothing to inspect; otherwise git reports the registered paths and the
// initialized ones. Git failures degrade to empty sets, never to an error.
func DetectSubmodules(root string) (bool, []string, error) {
	return DetectSubmodulesWith(root, gitFact, isFileFact)
}

// DetectSubmodulesWith runs the detection with the supplied git-fact and
// file-probe functions. The gate main package injects its own helpers here, so
// no second git-fact implementation crosses the package boundary.
func DetectSubmodulesWith(root string, fact func(string, ...string) string, isFile func(string) bool) (bool, []string, error) {
	if !isFile(filepath.Join(root, ".gitmodules")) {
		return false, []string{}, nil
	}
	registered := map[string]bool{}
	for _, line := range strings.Split(fact(root, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		registered[fields[1]] = true
	}
	var initialized []string
	for _, line := range strings.Split(fact(root, "submodule", "status"), "\n") {
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
	return true, SortedUnique(initialized), nil
}

// isFileFact is the default file probe: exists and is not a directory. It
// mirrors the gate main package's isFile helper the detection was moved from.
func isFileFact(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ResolveRepoTopology resolves one configured value against the worktree. The
// Source field is filled by BuildResolvedConfig, which owns the manifest read.
func ResolveRepoTopology(root, configured string) ResolvedTopology {
	return ResolveRepoTopologyWith(root, configured, gitFact, isFileFact)
}

// ResolveRepoTopologyWith runs the resolution with the supplied git-fact and
// file-probe functions, so the gate main package keeps its own helpers for the
// whole detectSubmodules chain.
func ResolveRepoTopologyWith(root, configured string, fact func(string, ...string) string, isFile func(string) bool) ResolvedTopology {
	if configured == "standalone" || configured == "monorepo-apps" {
		return ResolvedTopology{Resolved: configured, Configured: configured, Via: TopologyViaConfig, Submodules: []string{}}
	}
	if configured == "monorepo-submodules" {
		present, subs, err := DetectSubmodulesWith(root, fact, isFile)
		if err != nil {
			return ResolvedTopology{Resolved: "standalone", Configured: configured, Via: TopologyViaConfig, Submodules: []string{}}
		}
		return ResolvedTopology{Resolved: configured, Configured: configured, Via: TopologyViaConfig, Submodules: subs, GitmodulesPresent: present}
	}
	present, subs, err := DetectSubmodulesWith(root, fact, isFile)
	if err != nil {
		return ResolvedTopology{Resolved: "standalone", Configured: "auto", Via: TopologyViaAuto, Submodules: []string{}}
	}
	resolved := "standalone"
	if len(subs) > 0 {
		resolved = "monorepo-submodules"
	}
	return ResolvedTopology{Resolved: resolved, Configured: "auto", Via: TopologyViaAuto, Submodules: subs, GitmodulesPresent: present}
}

// BuildResolvedConfig assembles the whole envelope from the decoded manifest.
func BuildResolvedConfig(payload ManifestPayload, projectRoot string) ResolvedConfig {
	configured, source := TopologyConfigured(payload.Project, payload.Recipes)
	topology := ResolveRepoTopology(projectRoot, configured)
	topology.Source = source
	return ResolvedConfig{
		Bindings:    ProjectBindings(payload.Bindings),
		Recipes:     ProjectRecipes(payload.Recipes),
		Enabled:     ProjectEnabled(payload.Recipes),
		ProjectRoot: RealPath(projectRoot),
		Topology:    topology,
	}
}

// ManifestEnvelope decodes the reader output before projection. Recipes stay as
// raw pairs so a non-table recipe value is skipped rather than rejected.
type ManifestEnvelope struct {
	Recipes  [][]json.RawMessage `json:"recipes"`
	Bindings []json.RawMessage   `json:"bindings"`
	Project  map[string]any      `json:"project"`
}

// DecodeManifestPayload turns the reader output into the projection payload.
// Non-table recipe values carry a nil Table; non-table bindings are skipped.
func DecodeManifestPayload(raw []byte) (ManifestPayload, error) {
	var envelope ManifestEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ManifestPayload{}, fmt.Errorf("deserialized manifest: %w", err)
	}
	payload := ManifestPayload{
		Recipes:  make([]ManifestEntry, 0, len(envelope.Recipes)),
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
			return ManifestPayload{}, fmt.Errorf("deserialized manifest: recipe id: %w", err)
		}
		entry := ManifestEntry{ID: id}
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
