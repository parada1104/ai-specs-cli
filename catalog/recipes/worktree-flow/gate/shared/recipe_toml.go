package shared

// Recipe.toml capability-acquisition readers, promoted from the gate main
// package (recipe_toml.go) by GO-07 S14a so the gate binary and the root native
// materializer acquire the top-level [recipe] metadata and the [provides]
// primitive claims through ONE reader.
//
// Since GO-20 U2e T5 PR4b the readers parse in process through the parser in
// tomlparse.go instead of handing each recipe.toml to an ambient `python3 -c`
// tomllib helper: they are the last two production TOML seams (internal/sync
// calls both), and the gate binary must not need an interpreter. The Python
// reference's shapes are reproduced exactly; only the DIAGNOSTIC TEXT of a
// parser failure diverges (recorded TOLERANT in
// docs/go-migration-parity-contract.md), because it now names the recipe and the
// failing TOML line instead of exiting through an interpreter status.
//
// A recipe the parser cannot deserialize, whose [recipe] table is absent, or
// whose id/name/provides ids are missing or invalid is a hard parser failure
// (the Python authority raises RecipeValidationError for the same shapes and the
// calling bridge falls back to Python), never a silent omission. The tag
// reader is the exception: a recipe it cannot fully acquire is omitted, exactly
// as the tag grader treats a recipe its loader could not read.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadRecipeToml parses one recipe.toml. A missing or unreadable file and a
// parse failure both surface as the same error so the callers can shape their
// own reference message; the parse detail (never the file content) is kept.
func loadRecipeToml(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// recipeString is _require_string: the stripped string under key, or the exact
// reference refusal when it is absent, non-string or blank.
func recipeString(table *Table, key, recipeID, where string) (string, error) {
	value, ok := table.String(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("recipe %s: %s: missing or invalid required field '%s'", recipeID, where, key)
	}
	return strings.TrimSpace(value), nil
}

// providesEntries returns one [provides.<kind>] claim list as a slice of raw
// entries. The parser yields an array-of-tables as []*Table and an inline array
// as []any; both are the reference's `list`. A present non-list value contributes
// no claims (the reference's `if not isinstance(raw, list)`).
func providesEntries(provides *Table, kind string) []any {
	if tables, ok := provides.Tables(kind); ok {
		items := make([]any, len(tables))
		for i, table := range tables {
			items[i] = table
		}
		return items
	}
	if items, ok := provides.Array(kind); ok {
		return items
	}
	return nil
}

// recipeStringList reads an optional array-of-strings claim list. A non-list
// value contributes no claims (the reference uses `if not isinstance(raw, list)`),
// and every entry must be an object with a non-empty id.
func recipeStringList(provides *Table, kind, recipeID string) ([]string, error) {
	claims := []string{}
	raw := providesEntries(provides, kind)
	if raw == nil {
		return claims, nil
	}
	for idx, item := range raw {
		entry, ok := item.(*Table)
		if !ok {
			return nil, fmt.Errorf("recipe %s: provides.%s[%d]: expected object", recipeID, kind, idx)
		}
		id, err := recipeString(entry, "id", recipeID, fmt.Sprintf("provides.%s[%d]", kind, idx))
		if err != nil {
			return nil, err
		}
		claims = append(claims, id)
	}
	return claims, nil
}

// tagStrings reads one optional array-of-non-empty-strings field: absent means
// empty, anything that is not a list of non-blank strings makes the recipe
// unreadable (omitted) exactly as the reference's validation does.
func tagStrings(table *Table, key string) ([]string, bool) {
	values := []string{}
	raw, ok := table.Array(key)
	if !ok {
		if _, present := table.Get(key); present {
			return nil, false
		}
		return values, true
	}
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		values = append(values, text)
	}
	return values, true
}

// LoadRecipeTagMetadata acquires the top-level [recipe] id, tags and
// conflicts_with of every enabled recipe in one pass, preserving the enabled
// order so first-seen tag ordering is reproducible downstream. A recipe whose
// recipe.toml is missing, is not a plain file, cannot be parsed, or declares an
// unusable [recipe] table is absent from the result, matching the Python wrapper
// that skips a recipe it cannot read.
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
	ordered := make([]RecipeTagMetadata, 0, len(readable))
	for _, rid := range readable {
		root, err := loadRecipeToml(filepath.Join(catalogDir, rid, "recipe.toml"))
		if err != nil {
			continue
		}
		table, ok := root.Table("recipe")
		if !ok {
			continue
		}
		recipeID, ok := table.String("id")
		if !ok || strings.TrimSpace(recipeID) == "" {
			continue
		}
		tags, ok := tagStrings(table, "tags")
		if !ok {
			continue
		}
		conflicts, ok := tagStrings(table, "conflicts_with")
		if !ok {
			continue
		}
		ordered = append(ordered, RecipeTagMetadata{
			ID:            strings.TrimSpace(recipeID),
			Tags:          tags,
			ConflictsWith: conflicts,
		})
	}
	return ordered, nil
}

// LoadRecipePrimitives acquires every enabled recipe's primitive claims,
// preserving the enabled order so conflict ordering is reproducible downstream,
// and deduplicating repeated recipe ids like the sibling readers. Unlike the tag
// reader there is no readable-file pre-check: a recipe whose recipe.toml is
// missing or unparseable must fail, because the Python authority raises for the
// same shapes and the calling bridge falls back to it on the error this loader
// produces.
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
	ordered := make([]RecipePrimitives, 0, len(unique))
	for _, rid := range unique {
		root, err := loadRecipeToml(filepath.Join(catalogDir, rid, "recipe.toml"))
		if err != nil {
			return nil, fmt.Errorf("recipe %s: recipe.toml not found or not parseable", rid)
		}
		table, ok := root.Table("recipe")
		if !ok {
			return nil, fmt.Errorf("recipe %s: [recipe] must be a table", rid)
		}
		recipeID, err := recipeString(table, "id", rid, "[recipe]")
		if err != nil {
			return nil, err
		}
		name, err := recipeString(table, "name", rid, "[recipe]")
		if err != nil {
			return nil, err
		}
		provides := newTable(false)
		if value, ok := root.Get("provides"); ok {
			if asTable, ok := value.(*Table); ok {
				provides = asTable
			}
		}
		skills, err := recipeStringList(provides, "skills", rid)
		if err != nil {
			return nil, err
		}
		commands, err := recipeStringList(provides, "commands", rid)
		if err != nil {
			return nil, err
		}
		mcp, err := recipeStringList(provides, "mcp", rid)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, RecipePrimitives{
			ID: recipeID, Name: name, Skills: skills, Commands: commands, MCP: mcp,
		})
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
