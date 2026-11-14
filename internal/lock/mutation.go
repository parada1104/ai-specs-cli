// Mutation helpers ported 1:1 from lib/_internal/lock.py, preserving the
// Python setdefault chains: nil maps are created on demand, existing entry
// keys survive an upsert, and the Remove helpers report presence.
package lock

// SetManagedOverride upserts the last CLI-written bytes for one governed
// override target. Nil optional fields leave any existing value untouched
// (Python: value is not None); a non-nil empty string IS written.
func SetManagedOverride(lock *Lock, path, sha256 string, recipe, source, kind, policy *string) {
	if lock.Managed == nil {
		lock.Managed = map[string]map[string]any{}
	}
	// dict(managed.get(path) or {}): shallow-copy the existing entry.
	entry := map[string]any{}
	for k, v := range lock.Managed[path] {
		entry[k] = v
	}
	entry["sha256"] = sha256
	for key, value := range map[string]*string{
		"recipe": recipe, "source": source, "kind": kind, "policy": policy,
	} {
		if value != nil {
			entry[key] = *value
		}
	}
	lock.Managed[path] = entry
}

// SetGateBaseline records the last CLI-rendered bytes for a generated runtime
// hook (kind=gate, policy=auto): a matching baseline is treated as
// unmodified and may be force-updated.
func SetGateBaseline(lock *Lock, path, sha256 string, recipe, source *string) {
	SetManagedOverride(lock, path, sha256, recipe, source, strPtr("gate"), strPtr("auto"))
}

// SetBriefBaseline records the last CLI-rendered bytes for the runtime brief
// (kind=runtime-brief, policy=never-force): never force-refreshed after a
// user edit.
func SetBriefBaseline(lock *Lock, path, sha256 string) {
	SetManagedOverride(lock, path, sha256, nil, nil, strPtr("runtime-brief"), strPtr("never-force"))
}

// SetRecipeSkillHashes is set_recipe_skill_hashes: recipes.setdefault(
// recipe_id, {})[skill_name] = dict(hashes). The hashes map is copied.
func SetRecipeSkillHashes(lock *Lock, recipeID, skillName string, hashes map[string]string) {
	if lock.Recipes == nil {
		lock.Recipes = map[string]map[string]map[string]any{}
	}
	owner, ok := lock.Recipes[recipeID]
	if !ok {
		owner = map[string]map[string]any{}
		lock.Recipes[recipeID] = owner
	}
	files := make(map[string]any, len(hashes))
	for k, v := range hashes {
		files[k] = v
	}
	owner[skillName] = files
}

// SetDepSkillHashes is set_dep_skill_hashes: the deps-side mirror of
// SetRecipeSkillHashes.
func SetDepSkillHashes(lock *Lock, depID, skillName string, hashes map[string]string) {
	if lock.Deps == nil {
		lock.Deps = map[string]map[string]map[string]any{}
	}
	owner, ok := lock.Deps[depID]
	if !ok {
		owner = map[string]map[string]any{}
		lock.Deps[depID] = owner
	}
	files := make(map[string]any, len(hashes))
	for k, v := range hashes {
		files[k] = v
	}
	owner[skillName] = files
}

// RemoveRecipeLockEntries removes all lock entries for a recipe and reports
// whether anything was removed.
func RemoveRecipeLockEntries(lock *Lock, recipeID string) bool {
	if lock.Recipes == nil {
		return false
	}
	if _, ok := lock.Recipes[recipeID]; !ok {
		return false
	}
	delete(lock.Recipes, recipeID)
	return true
}

// RemoveDepLockEntries removes all lock entries for a dep and reports
// whether anything was removed.
func RemoveDepLockEntries(lock *Lock, depID string) bool {
	if lock.Deps == nil {
		return false
	}
	if _, ok := lock.Deps[depID]; !ok {
		return false
	}
	delete(lock.Deps, depID)
	return true
}

func strPtr(s string) *string { return &s }
