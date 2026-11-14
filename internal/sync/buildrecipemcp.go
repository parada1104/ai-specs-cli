package sync

// Native authority for the recipe-MCP build (GO-07 S13 PR4b-2).
//
// BuildRecipeMCP mirrors the Python authority build_recipe_mcp
// (lib/_internal/recipe-materialize.py:3179): shallow-copy the project manifest
// mcp mapping (manifest order first), append each recipe's [[provides.mcp]]
// presets in recipe_ids order then recipe declaration order, and resolve
// per-key conflicts in favour of the manifest with one warning per conflicting
// key. resolveProviderMarkers mirrors _resolve_provider_markers (:3131): read
// each recipe again, and for every merged server whose command is the literal
// marker "{dep:jinna}" resolve the jinna provider through internal/provider
// and replace the command in place, or drop the server and warn.
//
// It is added, not wired: S14 owns the sync-pipeline wiring, and sync.go keeps
// execing the Python materializer. The recipe-mcp JSON serialization (json.dump
// with indent=2 plus a trailing newline, at :4108-4110) stays with the caller,
// which reuses mcpJSONDumpsIndent2.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// recipePreset is one [[provides.mcp]] entry: its id and its config mapping
// (every key except "id") in TOML declaration order.
type recipePreset struct {
	id     string
	config *mcpObj
}

// recipeMCP is the slice of a recipe this authority needs: the recipe name for
// warnings, its cli_deps, and its ordered mcp presets.
type recipeMCP struct {
	name    string
	cliDeps []*schema.CliDep
	presets []recipePreset
}

// mcpObjDelete removes k from the ordered object, matching dict.pop(k, None):
// removing an absent key is a no-op and the remaining keys keep their order.
func mcpObjDelete(o *mcpObj, k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

// warnf mirrors warn (:174): "  ! <msg>\n" written to stderr.
func warnf(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "  ! "+format+"\n", args...)
}

// readRecipeMCP is the read_recipe seam (:216) narrowed to what this authority
// consumes: a missing recipe directory raises "recipe directory not found:
// <dir>" before schema.LoadRecipeToml is reached, exactly like Python's
// is_dir() guard; every other load failure propagates from schema.
func readRecipeMCP(catalogDir, rid string) (*recipeMCP, error) {
	recipeDir := filepath.Join(catalogDir, rid)
	if info, err := os.Stat(recipeDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("recipe directory not found: %s", recipeDir)
	}
	path := filepath.Join(recipeDir, "recipe.toml")
	recipe, err := schema.LoadRecipeToml(path)
	if err != nil {
		return nil, err
	}
	presets, err := recipeMCPPresets(path)
	if err != nil {
		return nil, err
	}
	return &recipeMCP{name: recipe.Name, cliDeps: recipe.CliDeps, presets: presets}, nil
}

// recipeMCPPresets re-reads recipe.toml to recover the [[provides.mcp]] config
// key order: schema.Recipe exposes the presets as an unordered map, but the
// serialized recipe MCP is encoded with sort_keys=False, so TOML declaration
// order is observable and must be preserved.
func recipeMCPPresets(path string) ([]recipePreset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := toml.Parse(raw)
	if err != nil {
		return nil, err
	}
	provides, ok := doc.Table("provides")
	if !ok || provides == nil {
		return nil, nil
	}
	tables, ok := provides.Tables("mcp")
	if !ok {
		return nil, nil
	}
	out := make([]recipePreset, 0, len(tables))
	for _, tbl := range tables {
		if tbl == nil {
			continue
		}
		id, _ := tbl.String("id")
		config := newMCPObj()
		for _, k := range tbl.Keys() {
			if k == "id" {
				continue
			}
			v, _ := tbl.Get(k)
			config.set(k, mcpToOrdered(v))
		}
		out = append(out, recipePreset{id: id, config: config})
	}
	return out, nil
}

// BuildRecipeMCP mirrors build_recipe_mcp (:3179). manifestMCP is the already
// ordered project manifest mcp mapping; the returned mapping is ordered and the
// caller serializes it. aiSpecsHome is nil exactly when the caller passes no
// ai-specs home (Python's None), which skips provider-marker resolution.
func BuildRecipeMCP(catalogDir string, recipeIDs []string, manifestMCP *mcpObj, aiSpecsHome *string, stderr io.Writer) (*mcpObj, error) {
	merged := newMCPObj()
	if manifestMCP != nil {
		for _, sid := range manifestMCP.keys {
			if cfg, ok := manifestMCP.vals[sid].(*mcpObj); ok {
				merged.set(sid, mcpShallowCopy(cfg))
			} else {
				merged.set(sid, manifestMCP.vals[sid])
			}
		}
	}
	for _, rid := range recipeIDs {
		recipe, err := readRecipeMCP(catalogDir, rid)
		if err != nil {
			return nil, err
		}
		for _, preset := range recipe.presets {
			existing, present := merged.get(preset.id)
			if !present {
				merged.set(preset.id, mcpShallowCopy(preset.config))
				continue
			}
			manifestCfg, isObj := existing.(*mcpObj)
			if !isObj {
				// The real callers cannot produce a non-mapping manifest value:
				// read_mcp normalizes every value to a dict, and Go's
				// mcpReadServers / mcpNormalizeServer always returns *mcpObj. At
				// this same point Python's build_recipe_mcp raises earlier, in the
				// shallow-copy stage at :3192: dict(cfg) fails with ValueError
				// "dictionary update sequence element #0 has length 1; 2 is
				// required" (not TypeError). A direct-API caller that reaches this
				// branch therefore observes a divergence: Python raises, Go keeps
				// the value. Recorded, not silently redefined.
				continue
			}
			for _, key := range preset.config.keys {
				if _, present := manifestCfg.get(key); present {
					warnf(stderr, "recipe '%s' mcp.id='%s' key '%s' conflicts with project manifest (manifest wins)", recipe.name, preset.id, key)
				} else {
					manifestCfg.set(key, preset.config.vals[key])
				}
			}
		}
	}
	if aiSpecsHome != nil {
		resolveProviderMarkers(merged, catalogDir, recipeIDs, *aiSpecsHome, stderr)
	}
	return merged, nil
}

// resolveProviderMarkers mirrors _resolve_provider_markers (:3131). It never
// raises: a recipe that cannot be read, a missing jinna declaration, and an
// unresolved or failed provider resolution each warn and skip or drop the
// server.
func resolveProviderMarkers(merged *mcpObj, catalogDir string, recipeIDs []string, aiSpecsHome string, stderr io.Writer) {
	home := aiSpecsHome
	for _, rid := range recipeIDs {
		recipe, err := readRecipeMCP(catalogDir, rid)
		if err != nil {
			warnf(stderr, "recipe '%s': cannot read recipe for provider resolution (%s)", rid, err.Error())
			continue
		}
		dependencies := map[string]*schema.CliDep{}
		for _, dep := range recipe.cliDeps {
			dependencies[dep.Binary] = dep
		}
		for _, preset := range recipe.presets {
			config, present := merged.get(preset.id)
			if !present {
				continue
			}
			configObj, isObj := config.(*mcpObj)
			if !isObj {
				continue
			}
			command, _ := configObj.get("command")
			commandStr, isStr := command.(string)
			if !isStr || commandStr != "{dep:jinna}" {
				continue
			}
			dep, declared := dependencies["jinna"]
			if !declared {
				mcpObjDelete(merged, preset.id)
				warnf(stderr, "recipe '%s': missing jinna dependency declaration", recipe.name)
				continue
			}
			resolution, err := provider.Resolve(dep, &home)
			if err != nil {
				mcpObjDelete(merged, preset.id)
				warnf(stderr, "recipe '%s': provider jinna could not be resolved (%s); run ai-specs configure-recipes interactively to install it", recipe.name, err.Error())
				continue
			}
			if resolution.Verified {
				resolved := mcpShallowCopy(configObj)
				resolved.set("command", resolution.Command)
				merged.set(preset.id, resolved)
				continue
			}
			mcpObjDelete(merged, preset.id)
			warnf(stderr, "recipe '%s': provider jinna is unresolved; run ai-specs configure-recipes interactively to install it", recipe.name)
		}
	}
}
