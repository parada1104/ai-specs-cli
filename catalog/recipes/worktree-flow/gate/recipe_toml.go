package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
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

// runRecipeTomlParser runs the standard TOML parser under the same bounded
// execution as the manifest seam: one deadline, capped stdout and stderr, and
// WaitDelay bounding the output-pipe wait. The shared constants and helpers in
// ledger_reconcile.go are reused, so acquisition safety is not re-decided here.
//
// The interpreter is resolved with the catalog dir as the protected root and run
// isolated (-I -B): neither an ambient nor a catalog-controlled module can shadow
// the stdlib imports the reader performs, and the catalog must never select the
// code that parses it. The runner duplicates the manifest runner only because the
// existing one hardcodes its reader; generalizing it would mean editing frozen
// behavior outside this slice.
func runRecipeTomlParser(catalogDir string, recipeIDs []string) (map[string][]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestParseTimeout)
	defer cancel()

	interpreter, err := manifestInterpreter(catalogDir)
	if err != nil {
		return nil, err
	}
	stdout := &cappedBuffer{max: acquisitionLimit}
	stderr := &cappedBuffer{max: manifestErrorLimit}
	args := append([]string{"-I", "-B", "-c", recipeTomlReader, catalogDir}, recipeIDs...)
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
	var caps map[string][]string
	if err := json.Unmarshal(stdout.buf.Bytes(), &caps); err != nil {
		return nil, fmt.Errorf("deserialized capabilities: %w", err)
	}
	return caps, nil
}
