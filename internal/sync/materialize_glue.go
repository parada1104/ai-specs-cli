package sync

// Acquisition glue for the native materializer spine (GO-07 S14a WU1).
//
// materialize.go owns the orchestration order, the verbatim prints and the
// frozen markers; this file owns only the per-side acquisition the Python
// authority performs with lib/_internal/toml-read.py, project-cache and
// lock.py. The decisions stay in the merged native authorities (shared and
// internal/sync); no decision is re-derived here.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	shared "ai-specs.dev/worktree-gate/shared"
)

// errNonObjectStamp reports a planned stamp that is not a JSON object. The
// shared planner always returns an object, so this is defensive.
var errNonObjectStamp = errors.New("reconcile stamp is not an object")

// materializeManifestPath is the manifest the Python authority reads for every
// step (<project_root>/ai-specs/ai-specs.toml).
func materializeManifestPath(projectRoot string) string {
	return filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
}

// materializeRecipe is one [recipes.<id>] entry as read_recipes emits it: the
// manifest id, its legacy version pin, and whether the manifest enables it.
type materializeRecipe struct {
	ID      string
	Version string
	Enabled bool
}

// loadMaterializeRecipes mirrors toml-read.read_recipes ordered by document
// order: a non-table recipe value is skipped, a missing enabled key reads as
// false, and a missing version reads as the empty string.
func loadMaterializeRecipes(manifest *toml.Table) []materializeRecipe {
	recipes, _ := manifest.Table("recipes")
	if recipes == nil {
		return nil
	}
	out := make([]materializeRecipe, 0, len(recipes.Keys()))
	for _, id := range recipes.Keys() {
		value, _ := recipes.Get(id)
		entry, ok := value.(*toml.Table)
		if !ok {
			continue
		}
		recipe := materializeRecipe{ID: id}
		if enabled, ok := entry.Bool("enabled"); ok {
			recipe.Enabled = enabled
		}
		if version, ok := entry.String("version"); ok {
			recipe.Version = version
		}
		out = append(out, recipe)
	}
	return out
}

// loadManifestBindings mirrors toml-read.read_bindings: [[bindings]] entries
// whose capability and recipe are both present strings, in document order.
func loadManifestBindings(manifest *toml.Table) []shared.ManifestBinding {
	out := []shared.ManifestBinding{}
	value, ok := manifest.Get("bindings")
	if !ok {
		return out
	}
	var items []any
	switch typed := value.(type) {
	case []any:
		items = typed
	case []*toml.Table:
		for _, table := range typed {
			items = append(items, table)
		}
	default:
		return out
	}
	for _, item := range items {
		table, ok := item.(*toml.Table)
		if !ok {
			continue
		}
		capability, hasCapability := table.String("capability")
		recipe, hasRecipe := table.String("recipe")
		if hasCapability && hasRecipe {
			out = append(out, shared.ManifestBinding{Capability: capability, Recipe: recipe})
		}
	}
	return out
}

// loadManifestDepIDs mirrors toml-read.read_deps only for the `id` values the
// materializer consumes: top-level [[deps]] tables with a non-empty id. A
// `deps` value that is not a list (a [[deps.cli]] table shape) yields nothing,
// exactly as Python's isinstance(raw_deps, list) guard does.
func loadManifestDepIDs(manifest *toml.Table) []string {
	value, ok := manifest.Get("deps")
	if !ok {
		return []string{}
	}
	var items []any
	switch typed := value.(type) {
	case []any:
		items = typed
	case []*toml.Table:
		for _, table := range typed {
			items = append(items, table)
		}
	default:
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, item := range items {
		table, ok := item.(*toml.Table)
		if !ok {
			continue
		}
		id, hasID := table.String("id")
		if !hasID || id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// enabledAgentNames mirrors _enabled_agents: the manifest [agents].enabled
// list coerced with Python str(), falsy entries dropped, best-effort.
func enabledAgentNames(manifest *toml.Table) []any {
	out := []any{}
	agents, _ := manifest.Table("agents")
	if agents == nil {
		return out
	}
	value, ok := agents.Get("enabled")
	if !ok {
		return out
	}
	var items []any
	switch typed := value.(type) {
	case []any:
		items = typed
	case []*toml.Table:
		for _, table := range typed {
			items = append(items, table)
		}
	default:
		return out
	}
	for _, item := range items {
		if !pyTruthyMaterialize(item) {
			continue
		}
		out = append(out, tomlPyStr(item))
	}
	return out
}

// dirChildNames mirrors _dir_child_names: direct child directory names of a
// materialized scope, sorted, files ignored. A missing path yields nothing.
func dirChildNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// lockRecipeNames mirrors `sorted(lock.get("recipes") or {})`.
func lockRecipeNames(lockPath string) []string {
	lk, err := lock.LoadLock(lockPath)
	if err != nil || lk == nil {
		return nil
	}
	names := make([]string, 0, len(lk.Recipes))
	for name := range lk.Recipes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// materializeOrphanInputs assembles the --plan-orphans/--apply-orphans envelope
// the Python authority builds with _orphan_plan_input, plus the three resolved
// cache roots. The roots come from project-cache; the child names from the
// filesystem; the lock recipe names from internal/lock.
func materializeOrphanInputs(projectRoot, cliHome string, enabledRecipeIDs []string, expectedDepIDs []string) (shared.OrphanPlanInput, shared.OrphanApplyRoots) {
	recipeDir := projectcache.RecipeSkillsRoot(projectRoot, cliHome)
	depsDir := projectcache.DepsSkillsRoot(projectRoot, cliHome)
	inprojectDir := projectcache.InprojectDepsRoot(projectRoot)
	lockPath := filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")

	input := shared.OrphanPlanInput{
		RecipeSkills:     dirChildNames(recipeDir),
		DepsSkills:       dirChildNames(depsDir),
		InprojectDeps:    dirChildNames(inprojectDir),
		LockRecipes:      lockRecipeNames(lockPath),
		EnabledRecipeIDs: sortedStrings(enabledRecipeIDs),
		ExpectedDepIDs:   sortedStrings(expectedDepIDs),
	}
	roots := shared.OrphanApplyRoots{
		RecipeSkills:  recipeDir,
		DepsSkills:    depsDir,
		InprojectDeps: inprojectDir,
	}
	return input, roots
}

// materializeStampOrdered adapts one planned ReconcileStampEntry stamp onto the
// writer's ordered value domain through the same JSON round-trip the Python
// bridge performs: encoding/json sorts object keys, so the stamp the writer
// sees has deterministic (sorted) key order at every level.
func materializeStampOrdered(entry shared.ReconcileStampEntry) (*shared.OrderedMap, error) {
	raw, err := json.Marshal(entry.Stamp)
	if err != nil {
		return nil, err
	}
	decoded, err := shared.DecodeOrdered(raw)
	if err != nil {
		return nil, err
	}
	table, ok := decoded.(*shared.OrderedMap)
	if !ok {
		return nil, errNonObjectStamp
	}
	return table, nil
}

// materializeResolvedConfigDocument renders json.dumps(resolved, indent=2,
// sort_keys=True) + "\n" over the shared projection. The projection is marshaled
// and decoded into the ordered model so the sort-keys writer matches Python's
// dict rendering exactly.
func materializeResolvedConfigDocument(resolved shared.ResolvedConfig) (string, error) {
	return materializeEnrichedResolvedConfigDocument(resolved, "")
}

// materializeEnrichedResolvedConfigDocument is materializeResolvedConfigDocument
// plus the S14b catalog enrichment when catalogDir is non-empty. The JSON round
// trip is deliberate: Python decodes the Go gate's envelope, so the projection's
// values carry that envelope's rendering, while enrichment runs afterwards on
// the ordered document with the catalog values' own types.
func materializeEnrichedResolvedConfigDocument(resolved shared.ResolvedConfig, catalogDir string) (string, error) {
	raw, err := json.Marshal(resolved)
	if err != nil {
		return "", err
	}
	decoded, err := mcpParseJSONOrdered(raw)
	if err != nil {
		return "", err
	}
	if catalogDir != "" {
		doc, ok := decoded.(*mcpObj)
		if !ok {
			return "", errNonObjectStamp
		}
		enrichResolvedConfig(doc, resolved.Enabled, catalogDir)
		if err := mcpJSONUnserializable(doc); err != nil {
			return "", err
		}
	}
	return hooksJSONDumpsSorted(decoded) + "\n", nil
}

// materializeResolvedHooksDocument renders {"enabled_agents": [...], "hooks":
// [...]} exactly like json.dump(..., indent=2, sort_keys=True) + "\n".
func materializeResolvedHooksDocument(enabledAgents []any, hooks []any) string {
	doc := newMCPObj()
	doc.set("enabled_agents", enabledAgents)
	doc.set("hooks", hooks)
	return hooksJSONDumpsSorted(doc) + "\n"
}

// recipeCommandSources mirrors the provenance loop's command map: command file
// name -> catalog source path. A recipe that cannot be read propagates its
// error, exactly as read_recipe does.
func recipeCommandSources(catalogDir string, enabled []materializeRecipe) (map[string]string, []string, error) {
	sources := map[string]string{}
	depIDs := []string{}
	for _, entry := range enabled {
		recipeDir := filepath.Join(catalogDir, entry.ID)
		recipe, err := schema.LoadRecipeToml(filepath.Join(recipeDir, "recipe.toml"))
		if err != nil {
			return nil, nil, err
		}
		for _, skill := range recipe.Skills {
			if skill.Source == "dep" {
				depIDs = append(depIDs, skill.ID)
			}
		}
		for _, cmd := range recipe.Commands {
			sources[cmd.ID+".md"] = filepath.Join(recipeDir, cmd.Path)
		}
	}
	return sources, depIDs, nil
}

// reconcileStampRecipeName mirrors _reconcile_stamp_recipe_name: the declared
// recipe name for a writer-failure warning, best effort (the id on any read
// failure).
func reconcileStampRecipeName(catalogDir, recipeID string) string {
	recipe, err := schema.LoadRecipeToml(filepath.Join(catalogDir, recipeID, "recipe.toml"))
	if err != nil || recipe == nil || recipe.Name == "" {
		return recipeID
	}
	return recipe.Name
}

// sortedStrings returns a sorted, deduplicated copy of values.
func sortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// pyTruthyMaterialize mirrors Python truthiness for the TOML scalar domain.
func pyTruthyMaterialize(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case toml.BigInt:
		return len(typed) > 0 && string(typed) != "0"
	case []any:
		return len(typed) > 0
	case []*toml.Table:
		return len(typed) > 0
	case *toml.Table:
		return typed != nil && len(typed.Keys()) > 0
	}
	return true
}

// tomlPyStr mirrors Python str() for the scalar manifest values considered
// here. Table/list values are outside this acquisition's reach.
func tomlPyStr(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return mcpJSONFloat(typed)
	case toml.BigInt:
		return string(typed)
	case nil:
		return "None"
	}
	return ""
}
