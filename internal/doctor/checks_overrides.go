package doctor

// Native port of doctor._check_stale_template_overrides (check 15 of the
// roster) plus the small util.py surface it shares with the gate-provenance
// check: render_override_bytes, project_owned_recipe_config, the lock-backed
// managed-override read, and Path(target).as_posix() normalization.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/target"
	"ai-specs.dev/ai-specs/internal/toml"
)

// repoTopologyPlaceholder mirrors util.REPO_TOPOLOGY_PLACEHOLDER.
const repoTopologyPlaceholder = "__WORKTREE_REPO_TOPOLOGY__"

// checkStaleTemplateOverrides is doctor._check_stale_template_overrides.
func (d *Doctor) checkStaleTemplateOverrides() {
	catalog := filepath.Join(d.Home, "catalog", "recipes")
	if !isDir(catalog) {
		return
	}
	if !isFile(d.manifestPath()) {
		return
	}
	data := d.manifestData()
	if data == nil {
		return
	}
	managed := d.loadManagedOverrides()

	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return
	}
	for _, rid := range recipes.Keys() {
		entry, ok := recipes.Table(rid)
		if !ok || entry == nil {
			continue
		}
		enabled, isBool := entry.Bool("enabled")
		if !isBool || !enabled {
			continue
		}
		recipeDir := filepath.Join(catalog, rid)
		recipeToml := filepath.Join(recipeDir, "recipe.toml")
		if !isFile(recipeToml) {
			continue
		}
		recipe, err := schema.LoadRecipeToml(recipeToml)
		if err != nil {
			continue
		}
		cfgTable, _ := entry.Table("config")
		mergedCfg := projectOwnedRecipeConfig(d.Root, data, rid, cfgTable)
		for _, tpl := range recipe.Templates {
			if tpl.Condition != "not_exists" {
				continue
			}
			src := pyJoin(recipeDir, tpl.Source)
			dest := pyJoin(d.Root, tpl.Target)
			if !isFile(dest) || !isFile(src) {
				continue
			}
			wouldWrite, err := renderOverrideBytes(src, mergedCfg)
			if err != nil {
				continue
			}
			state := classifyManagedOverride(dest, managed[posixAsPosix(tpl.Target)], wouldWrite, true)
			policy := tpl.UpdatePolicy
			if policy == "" {
				policy = "auto"
			}
			var message string
			switch state {
			case "user_modified":
				message = tpl.Target + " is user-modified; sync will preserve it"
			case "untracked":
				destBytes, destErr := os.ReadFile(dest)
				srcBytes, srcErr := os.ReadFile(src)
				if destErr != nil || srcErr != nil {
					continue
				}
				diskSHA := lock.Sha256Bytes(destBytes)
				renderedSHA := lock.Sha256Bytes(wouldWrite)
				legacyCatalogSHA := lock.Sha256Bytes(srcBytes)
				if diskSHA == renderedSHA || diskSHA == legacyCatalogSHA {
					continue
				}
				message = tpl.Target + " has missing ownership metadata; sync will preserve the existing file " +
					"without assigning ownership. Leave it unchanged to preserve it, or remove it and " +
					"rerun sync to restore the current recipe version"
			case "managed_stale":
				if policy == "auto" {
					continue
				}
				message = fmt.Sprintf("%s is managed-stale and policy is %s; sync will preserve it", tpl.Target, policy)
			default:
				continue
			}
			d.add(WARN, "stale-override", message, "rm "+tpl.Target+" && ai-specs sync")
		}
	}
}

// loadManagedOverrides mirrors the lock read both override checks perform:
// <root>/ai-specs/.ai-specs.lock's [managed] section, or an empty map when the
// lock is missing or malformed (the legacy `except Exception: managed = {}`).
func (d *Doctor) loadManagedOverrides() map[string]map[string]any {
	lk, err := lock.LoadLock(filepath.Join(d.Root, "ai-specs", ".ai-specs.lock"))
	if err != nil {
		return map[string]map[string]any{}
	}
	return lk.Managed
}

// renderOverrideBytes mirrors util.render_override_bytes: the catalog bytes
// with the repo-topology placeholder replaced by the resolved value; the
// catalog bytes unchanged when the placeholder is absent.
func renderOverrideBytes(catalogSrc string, mergedCfg map[string]any) ([]byte, error) {
	data, err := os.ReadFile(catalogSrc)
	if err != nil {
		return nil, err
	}
	token := []byte(repoTopologyPlaceholder)
	if !bytes.Contains(data, token) {
		return data, nil
	}
	topology := "auto"
	if mergedCfg != nil {
		if value, present := mergedCfg["repo_topology"]; present {
			topology = pyStrValue(value)
		}
	}
	return bytes.ReplaceAll(data, token, []byte(topology)), nil
}

// projectOwnedRecipeConfig mirrors util.project_owned_recipe_config: the recipe
// config with the project-owned repo_topology applied for worktree-flow.
func projectOwnedRecipeConfig(projectRoot string, data *toml.Table, recipeID string, config *toml.Table) map[string]any {
	merged := map[string]any{}
	if config != nil {
		for key, value := range config.Any() {
			merged[key] = value
		}
	}
	if recipeID == "worktree-flow" {
		merged["repo_topology"] = target.ProjectRepoTopology(projectRoot, data).Configured
	}
	return merged
}

// posixAsPosix mirrors Path(p).as_posix(): drop empty and "." components,
// collapse separators, and keep ".." and leading "/".
func posixAsPosix(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	resolved := strings.Join(out, "/")
	if strings.HasPrefix(p, "/") {
		resolved = "/" + resolved
	}
	if resolved == "" {
		return "."
	}
	return resolved
}

// pyJoin mirrors Python's `Path(base) / part`: an absolute part replaces the
// base instead of being appended.
func pyJoin(base, part string) string {
	if filepath.IsAbs(part) {
		return filepath.FromSlash(part)
	}
	return filepath.Join(base, filepath.FromSlash(part))
}
