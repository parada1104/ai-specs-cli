package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"ai-specs.dev/worktree-gate/shared"
)

// Resolved-config projection. The pure decision core now lives in
// ai-specs.dev/worktree-gate/shared (GO-07 S13 PR3) so the gate command and the
// root native sync authority share one implementation. This file keeps the
// historical lowercase names as thin type aliases and wrappers so every
// existing call site and test compiles unchanged; only the TOML reader seam
// (resolvedConfigReader / runResolvedConfigParser) and the --plan-resolved-config
// flag wrapper stay here, and they call the shared core.

// Historical aliases: the moved shared types under their original local names.
type (
	manifestEntry    = shared.ManifestEntry
	manifestPayload  = shared.ManifestPayload
	resolvedTopology = shared.ResolvedTopology
	resolvedConfig   = shared.ResolvedConfig
)

// Historical constants re-exported under their original local names.
const (
	topologySourceProject      = shared.TopologySourceProject
	topologySourceLegacyRecipe = shared.TopologySourceLegacyRecipe
	topologySourceDefault      = shared.TopologySourceDefault
	topologyViaConfig          = shared.TopologyViaConfig
	topologyViaAuto            = shared.TopologyViaAuto
)

// projectRecipes is the historical wrapper over the shared core.
func projectRecipes(entries []manifestEntry) map[string]map[string]any {
	return shared.ProjectRecipes(entries)
}

// projectEnabled is the historical wrapper over the shared core.
func projectEnabled(entries []manifestEntry) []string {
	return shared.ProjectEnabled(entries)
}

// projectBindings is the historical wrapper over the shared core.
func projectBindings(bindings []map[string]any) map[string]string {
	return shared.ProjectBindings(bindings)
}

// topologyConfigured is the historical wrapper over the shared core.
func topologyConfigured(project map[string]any, entries []manifestEntry) (string, string) {
	return shared.TopologyConfigured(project, entries)
}

// resolveRepoTopology is the historical wrapper over the shared core, forwarding
// the gate's own git fact and file probe through the seam.
func resolveRepoTopology(root, configured string) resolvedTopology {
	return shared.ResolveRepoTopologyWith(root, configured, git, isFile)
}

// buildResolvedConfig is the historical wrapper over the shared core.
func buildResolvedConfig(payload manifestPayload, projectRoot string) resolvedConfig {
	return shared.BuildResolvedConfig(payload, projectRoot)
}

// decodeManifestPayload is the historical wrapper over the shared decoder.
func decodeManifestPayload(raw []byte) (manifestPayload, error) {
	return shared.DecodeManifestPayload(raw)
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
