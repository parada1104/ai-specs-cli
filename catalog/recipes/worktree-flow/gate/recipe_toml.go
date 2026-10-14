package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"ai-specs.dev/worktree-gate/shared"
)

// recipeTomlReader is the whole recipe.toml capability boundary: the Python
// standard library parser deserializes the enabled recipes and prints their
// declared capability ids as one JSON object keyed by recipe id. It selects
// nothing — no binding, no conflict, no policy — so every resolution decision
// stays in Go and no TOML parser (hand-written subset or new Go dependency) is
// introduced. It generalizes the manifest seam in ledger_reconcile.go and reuses
// the same bounded-execution helpers and limits.
//
// A recipe the parser cannot deserialize, or whose capability block is invalid
// exactly as recipe_schema._parse_capabilities defines it (non-object entry, a
// missing/empty/non-string id, or a duplicate id), is omitted from the object:
// Python's load_recipe_toml raises and callers swallow it, so the recipe
// declares nothing. A non-list capabilities value is not an error there and
// becomes an empty list here.
const recipeTomlReader = `import json, os, sys, tomllib
catalog = sys.argv[1]
out = {}
for rid in sys.argv[2:]:
    try:
        with open(os.path.join(catalog, rid, "recipe.toml"), "rb") as handle:
            data = tomllib.load(handle)
    except Exception:
        continue
    raw = data.get("capabilities")
    if not isinstance(raw, list):
        out[rid] = []
        continue
    ids = []
    ok = True
    for item in raw:
        if not isinstance(item, dict):
            ok = False
            break
        cap_id = item.get("id")
        if not isinstance(cap_id, str) or not cap_id.strip() or cap_id in ids:
            ok = False
            break
        ids.append(cap_id)
    if ok:
        out[rid] = ids
print(json.dumps(out))
`

// loadRecipeCapabilities acquires the declared capability ids of every enabled
// recipe in one bounded parser run. A recipe whose recipe.toml is missing or is
// not a plain file is never handed to the parser and is absent from the map;
// callers read that absence as "declares nothing", which is how the Python
// authority treats a recipe its loader could not read.
func loadRecipeCapabilities(catalogDir string, recipeIDs []string) (map[string][]string, error) {
	readable := make([]string, 0, len(recipeIDs))
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		if regularFile(filepath.Join(catalogDir, rid, "recipe.toml")) == nil {
			readable = append(readable, rid)
		}
	}
	if len(readable) == 0 {
		return map[string][]string{}, nil
	}
	return runRecipeTomlParser(catalogDir, readable)
}

// loadRecipePrimitives delegates to the shared reader so the gate binary and
// the root native materializer acquire the same primitive claims through one
// implementation (shared.LoadRecipePrimitives).
func loadRecipePrimitives(catalogDir string, recipeIDs []string) ([]shared.RecipePrimitives, error) {
	return shared.LoadRecipePrimitives(catalogDir, recipeIDs)
}

// reconcileStampReader is the acquisition boundary for the reconcile-stamp
// planner. It reads only what the planner consumes: the raw declared
// [config.reconcile] table (shape validation is deliberately not replicated —
// the decision layer type-asserts exactly the pieces it uses) and, for every
// recognized [config.<name>] config field, its declared default when the
// default key is present. Recognition follows recipe_schema._parse_config's
// fields/extra split: only a table section carrying a boolean required key is
// a config field, so non-standard sections (which land in extra and are
// invisible to the Python stamp decision) contribute nothing. TOML has no
// null, so a present default key always holds a non-None value, which is
// exactly Python's `field.default is not None` test.
//
// A recipe the parser cannot deserialize, whose [config] table is absent or
// not a table, or whose [config.reconcile] is absent or not a table is
// omitted: the Python stamp authority silently continues for the same shapes.
// Datetime values (the one TOML type JSON cannot carry) degrade to strings
// via default=str instead of failing the whole acquisition run.
const reconcileStampReader = `import json, os, sys, tomllib
catalog = sys.argv[1]
out = {}
for rid in sys.argv[2:]:
    try:
        with open(os.path.join(catalog, rid, "recipe.toml"), "rb") as handle:
            data = tomllib.load(handle)
    except Exception:
        continue
    config = data.get("config")
    if not isinstance(config, dict):
        continue
    reconcile = config.get("reconcile")
    if not isinstance(reconcile, dict):
        continue
    fields = {}
    for name, section in config.items():
        if name == "reconcile" or not isinstance(section, dict):
            continue
        if not isinstance(section.get("required"), bool):
            continue
        if "default" in section:
            fields[name] = section["default"]
    out[rid] = {"reconcile": reconcile, "fields": fields}
print(json.dumps(out, default=str))
`

// runRecipeTomlParser runs the capability reader under the bounded TOML
// execution and deserializes its JSON object.
func runRecipeTomlParser(catalogDir string, recipeIDs []string) (map[string][]string, error) {
	raw, err := runRecipeTomlReader(catalogDir, recipeIDs, recipeTomlReader)
	if err != nil {
		return nil, err
	}
	var caps map[string][]string
	if err := json.Unmarshal(raw, &caps); err != nil {
		return nil, fmt.Errorf("deserialized capabilities: %w", err)
	}
	return caps, nil
}

// loadRecipeTagMetadata delegates to the shared reader so the gate binary and
// the root native materializer acquire the same tag metadata through one
// implementation (shared.LoadRecipeTagMetadata).
func loadRecipeTagMetadata(catalogDir string, recipeIDs []string) ([]shared.RecipeTagMetadata, error) {
	return shared.LoadRecipeTagMetadata(catalogDir, recipeIDs)
}

// acquiredReconcileStamp is one enabled recipe's acquired stamp input, kept in
// enabled order so the emitted stamps list is deterministic.
type acquiredReconcileStamp struct {
	RecipeID string
	Source   shared.ReconcileStampSource
}

// loadReconcileStampSources acquires every enabled recipe's reconcile stamp
// inputs in one bounded parser run, preserving the enabled order and
// deduplicating repeated recipe ids like the sibling readers. A recipe whose
// recipe.toml is missing or is not a plain file is never handed to the parser
// and is absent from the result, matching the Python authority that silently
// continues when read_recipe raises.
func loadReconcileStampSources(catalogDir string, recipeIDs []string) ([]acquiredReconcileStamp, error) {
	readable := make([]string, 0, len(recipeIDs))
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		if regularFile(filepath.Join(catalogDir, rid, "recipe.toml")) == nil {
			readable = append(readable, rid)
		}
	}
	if len(readable) == 0 {
		return []acquiredReconcileStamp{}, nil
	}
	raw, err := runRecipeTomlReader(catalogDir, readable, reconcileStampReader)
	if err != nil {
		return nil, err
	}
	// UseNumber keeps TOML integers exact through the decode/re-encode round
	// trip instead of degrading them to float64.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var byRecipe map[string]shared.ReconcileStampSource
	if err := decoder.Decode(&byRecipe); err != nil {
		return nil, fmt.Errorf("deserialized reconcile stamps: %w", err)
	}
	ordered := make([]acquiredReconcileStamp, 0, len(readable))
	for _, rid := range readable {
		if source, ok := byRecipe[rid]; ok {
			ordered = append(ordered, acquiredReconcileStamp{RecipeID: rid, Source: source})
		}
	}
	return ordered, nil
}

// runRecipeTomlReader executes one stdlib-TOML recipe reader under the same
// bounded execution as the manifest seam: one deadline, capped stdout and
// stderr, and WaitDelay bounding the output-pipe wait. The shared constants and
// helpers in ledger_reconcile.go are reused, so acquisition safety is not
// re-decided here.
//
// The interpreter is resolved with the catalog dir as the protected root and run
// isolated (-I -B): neither an ambient nor a catalog-controlled module can shadow
// the stdlib imports the reader performs, and the catalog must never select the
// code that parses it. Every recipe.toml reader shares this runner so adding a
// reader cannot fork acquisition behavior.
func runRecipeTomlReader(catalogDir string, recipeIDs []string, reader string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestParseTimeout)
	defer cancel()

	interpreter, err := manifestInterpreter(catalogDir)
	if err != nil {
		return nil, err
	}
	stdout := &cappedBuffer{max: acquisitionLimit}
	stderr := &cappedBuffer{max: manifestErrorLimit}
	args := append([]string{"-I", "-B", "-c", reader, catalogDir}, recipeIDs...)
	cmd := exec.CommandContext(ctx, interpreter, args...)
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
