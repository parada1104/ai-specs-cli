package shared

// Recipe.toml capability-acquisition readers, promoted from the gate main
// package (recipe_toml.go) by GO-07 S14a so the gate binary and the root native
// materializer acquire the top-level [recipe] metadata and the [provides]
// primitive claims through ONE reader.
//
// The Python standard library parser is the acquisition boundary; it selects
// nothing (no binding, no conflict, no policy), so every decision stays in the
// shared graders (CheckTagConflicts / CheckPrimitiveConflicts). The bounded
// execution reuses the writer's embedded-Python seam (runRecipeConfigPython)
// rather than forking a second copy of the bounded stdlib-TOML execution: that
// seam is already the shared package's one such runner.
//
// A recipe the parser cannot deserialize, whose [recipe] table is absent, or
// whose id/name/provides ids are missing or invalid is a hard parser failure
// (the Python authority raises RecipeValidationError for the same shapes and
// the calling bridge falls back to Python), never a silent omission. The tag
// reader is the exception: a recipe it cannot fully acquire is omitted, exactly
// as the tag grader treats a recipe its loader could not read.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// recipeTagMetadataReader is the acquisition boundary for the tag-conflict
// grader. It reads only the top-level [recipe] metadata that grader needs — the
// recipe id (never the catalog directory name), the declared tags, and the
// conflicts_with declarations — and prints them as one JSON object keyed by the
// requested recipe id. It selects nothing: grouping and severity stay in Go.
//
// A recipe the parser cannot deserialize, whose [recipe] table is absent, whose
// id is not a non-empty string, or whose tags/conflicts_with are not arrays of
// non-empty strings is omitted: the Python loader raises for the same shapes, so
// the tag grader never sees that recipe.
const recipeTagMetadataReader = `import json, os, sys, tomllib
catalog = sys.argv[1]
out = {}
for rid in sys.argv[2:]:
    try:
        with open(os.path.join(catalog, rid, "recipe.toml"), "rb") as handle:
            data = tomllib.load(handle)
    except Exception:
        continue
    table = data.get("recipe")
    if not isinstance(table, dict):
        continue
    recipe_id = table.get("id")
    if not isinstance(recipe_id, str) or not recipe_id.strip():
        continue
    tags = table.get("tags", [])
    if not isinstance(tags, list) or any(not isinstance(t, str) or not t.strip() for t in tags):
        continue
    conflicts = table.get("conflicts_with", [])
    if not isinstance(conflicts, list) or any(not isinstance(c, str) or not c.strip() for c in conflicts):
        continue
    out[rid] = {"id": recipe_id.strip(), "tags": tags, "conflicts_with": conflicts}
print(json.dumps(out))
`

// recipePrimitivesReader is the acquisition boundary for the primitive-conflict
// grader. It reads only what the grader consumes: the [recipe] id and name (the
// name owns claims in the conflict registry, never the TOML id) and the ordered
// skill, command and mcp ids under [provides], preserving TOML array order. It
// selects nothing: registry ownership, collision pairing and severity stay in
// Go.
//
// Unlike the sibling readers, a recipe this reader cannot fully acquire is a
// hard parser failure, not an omission: the Python authority
// (check_recipe_conflicts) raises RecipeValidationError for a missing directory,
// an unreadable or unparseable recipe.toml, a missing/invalid [recipe] id or
// name, and a provides entry that is not an object with a non-empty id — so the
// calling bridge must fall back to Python (exit 2 upstream) instead of acting on
// a partial decision. Shapes Python swallows are swallowed here too: a
// non-table [provides] and a non-list skills/commands/mcp value contribute no
// claims, and ids are stripped exactly as _require_string strips them.
const recipePrimitivesReader = `import json, os, sys, tomllib
catalog = sys.argv[1]

def fail(rid, msg):
    sys.stderr.write("recipe %s: %s\n" % (rid, msg))
    sys.exit(1)

def require_string(entry, key, rid, where):
    if not isinstance(entry, dict):
        fail(rid, "%s: expected object" % where)
    value = entry.get(key)
    if not isinstance(value, str) or not value.strip():
        fail(rid, "%s: missing or invalid required field '%s'" % (where, key))
    return value.strip()

out = {}
for rid in sys.argv[2:]:
    try:
        with open(os.path.join(catalog, rid, "recipe.toml"), "rb") as handle:
            data = tomllib.load(handle)
    except Exception:
        fail(rid, "recipe.toml not found or not parseable")
    table = data.get("recipe")
    if not isinstance(table, dict):
        fail(rid, "[recipe] must be a table")
    recipe_id = require_string(table, "id", rid, "[recipe]")
    name = require_string(table, "name", rid, "[recipe]")
    provides = data.get("provides", {})
    if not isinstance(provides, dict):
        provides = {}
    claims = {}
    for kind in ("skills", "commands", "mcp"):
        raw = provides.get(kind)
        if not isinstance(raw, list):
            claims[kind] = []
            continue
        claims[kind] = [require_string(item, "id", rid, "provides.%s[%d]" % (kind, idx)) for idx, item in enumerate(raw)]
    out[rid] = {"id": recipe_id, "name": name, "skills": claims["skills"], "commands": claims["commands"], "mcp": claims["mcp"]}
print(json.dumps(out))
`

// LoadRecipeTagMetadata acquires the top-level [recipe] id, tags and
// conflicts_with of every enabled recipe in one bounded parser run, preserving
// the enabled order so first-seen tag ordering is reproducible downstream. A
// recipe whose recipe.toml is missing or is not a plain file is never handed to
// the parser and is absent from the result, matching the Python wrapper that
// skips a recipe without a readable recipe.toml.
func LoadRecipeTagMetadata(catalogDir string, recipeIDs []string) ([]RecipeTagMetadata, error) {
	readable := make([]string, 0, len(recipeIDs))
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		if recipeTomlRegularFile(filepath.Join(catalogDir, rid, "recipe.toml")) == nil {
			readable = append(readable, rid)
		}
	}
	if len(readable) == 0 {
		return []RecipeTagMetadata{}, nil
	}
	raw, err := runRecipeConfigPython(catalogDir, recipeTagMetadataReader, append([]string{catalogDir}, readable...), nil)
	if err != nil {
		return nil, err
	}
	var byRecipe map[string]RecipeTagMetadata
	if err := json.Unmarshal(raw, &byRecipe); err != nil {
		return nil, fmt.Errorf("deserialized tag metadata: %w", err)
	}
	ordered := make([]RecipeTagMetadata, 0, len(readable))
	for _, rid := range readable {
		if metadata, ok := byRecipe[rid]; ok {
			ordered = append(ordered, metadata)
		}
	}
	return ordered, nil
}

// LoadRecipePrimitives acquires every enabled recipe's primitive claims in one
// bounded parser run, preserving the enabled order so conflict ordering is
// reproducible downstream, and deduplicating repeated recipe ids like the
// sibling readers. Unlike the tag reader there is no readable-file pre-check: a
// recipe whose recipe.toml is missing or unparseable must reach the parser and
// fail, because the Python authority raises for the same shapes and the calling
// bridge falls back to it on the exit 2 this loader produces.
func LoadRecipePrimitives(catalogDir string, recipeIDs []string) ([]RecipePrimitives, error) {
	seen := make(map[string]bool, len(recipeIDs))
	unique := make([]string, 0, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		unique = append(unique, rid)
	}
	if len(unique) == 0 {
		return []RecipePrimitives{}, nil
	}
	raw, err := runRecipeConfigPython(catalogDir, recipePrimitivesReader, append([]string{catalogDir}, unique...), nil)
	if err != nil {
		return nil, err
	}
	var byRecipe map[string]RecipePrimitives
	if err := json.Unmarshal(raw, &byRecipe); err != nil {
		return nil, fmt.Errorf("deserialized recipe primitives: %w", err)
	}
	ordered := make([]RecipePrimitives, 0, len(unique))
	for _, rid := range unique {
		if recipe, ok := byRecipe[rid]; ok {
			ordered = append(ordered, recipe)
		}
	}
	return ordered, nil
}

// recipeTomlRegularFile refuses anything that is not a plain file before it is
// opened: a read of a FIFO or device would block acquisition, and a directory
// cannot hold this content. The check is a cheapest-first guard, not a security
// boundary, and mirrors the gate main package's regularFile.
func recipeTomlRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file (%s)", info.Mode().Type())
	}
	return nil
}
