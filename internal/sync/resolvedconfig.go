package sync

// Native root authority for the resolved-config projection (GO-07 S13 PR3).
//
// The decision core is the shared package
// (ai-specs.dev/worktree-gate/shared.BuildResolvedConfig) the gate binary's
// --plan-resolved-config surface also runs, so the root single binary and the
// gate share one projection — never a second port. This file owns only the root
// boundary: it acquires the project manifest natively and maps it onto the
// shared payload. S14 owns the sync-pipeline wiring; this authority is added,
// not called from the pipeline.
//
// It reproduces the Python fallback authority _python_build_resolved_config
// (lib/_internal/recipe-materialize.py:3763): the raw manifest [recipes.*]
// tables in document order, the raw [[bindings]] list and the raw [project]
// table, plus the project-owned topology the shared core resolves. The bridge
// is read-only: this authority performs no write of any kind.

import (
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/toml"
	"ai-specs.dev/worktree-gate/shared"
)

// ResolvedConfigRequest is the root boundary input for one resolved-config
// projection: the project root the manifest is read from and that the root is
// canonicalized against. It mirrors the Python authority's
// _python_build_resolved_config(project_root).
type ResolvedConfigRequest struct {
	ProjectRoot string
}

// BuildResolvedConfig acquires the project manifest natively and delegates the
// projection to the shared decision core. The manifest is read exactly as the
// Python authority reads it (<project_root>/ai-specs/ai-specs.toml); a missing
// or unparsable manifest is a Go error (the Python authority's
// FileNotFoundError / TOMLDecodeError transport failure).
func BuildResolvedConfig(req ResolvedConfigRequest) (shared.ResolvedConfig, error) {
	path := filepath.Join(req.ProjectRoot, "ai-specs", "ai-specs.toml")
	manifest, err := config.LoadManifest(path)
	if err != nil {
		return shared.ResolvedConfig{}, err
	}
	return shared.BuildResolvedConfig(acquireResolvedConfigPayload(manifest), req.ProjectRoot), nil
}

// acquireResolvedConfigPayload converts the parsed manifest into the shared
// payload: the raw [recipes.<id>] tables in manifest document order, the raw
// [[bindings]] list, and the raw [project] table. It selects nothing — every
// projection decision stays in the shared core.
//
// internal/config.readRecipes normalizes each recipe to {enabled, version,
// config} and drops flat keys, so it cannot feed the shared projectRecipes
// (which keeps flat keys, as the Python authority does); the raw table read
// here is the faithful native acquisition.
func acquireResolvedConfigPayload(manifest *toml.Table) shared.ManifestPayload {
	payload := shared.ManifestPayload{
		Recipes:  []shared.ManifestEntry{},
		Bindings: []map[string]any{},
		Project:  map[string]any{},
	}
	if recipes, ok := manifest.Table("recipes"); ok && recipes != nil {
		for _, id := range recipes.Keys() {
			value, _ := recipes.Get(id)
			entry := shared.ManifestEntry{ID: id}
			if table, isTable := value.(*toml.Table); isTable {
				entry.Table = table.Any()
			}
			payload.Recipes = append(payload.Recipes, entry)
		}
	}
	if bindings, ok := manifest.Get("bindings"); ok {
		for _, item := range resolvedConfigBindingTables(bindings) {
			payload.Bindings = append(payload.Bindings, item.Any())
		}
	}
	if project, ok := manifest.Table("project"); ok && project != nil {
		payload.Project = project.Any()
	}
	return payload
}

// resolvedConfigBindingTables normalizes the raw manifest bindings value to the
// table entries the shared core projects. internal/toml produces the
// [[bindings]] array of tables as []*toml.Table and an inline array
// (`bindings = [{...}]`) as []any, so both shapes are accepted; a []any mixing
// inline tables with non-table items keeps only the tables, exactly as the
// Python authority's `isinstance(b, dict)` check skips every non-dict element.
// Document order is preserved and every non-table item is ignored (never
// silently dropped when it is a table). A bare inline table (`bindings = {...}`)
// is not a list and carries no bindings, matching the Python `isinstance(...,
// list)` guard. This is the same acceptance set internal/config.readBindings
// uses ([]any of inline tables, []*toml.Table, or neither).
func resolvedConfigBindingTables(value any) []*toml.Table {
	switch items := value.(type) {
	case []*toml.Table:
		return items
	case []any:
		tables := make([]*toml.Table, 0, len(items))
		for _, item := range items {
			if table, isTable := item.(*toml.Table); isTable {
				tables = append(tables, table)
			}
		}
		return tables
	default:
		return nil
	}
}
