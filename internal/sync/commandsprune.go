package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/worktree-gate/shared"
)

const RecipeCommandKind = "recipe-command"
const retiredCommandsFile = ".retired-commands.json"

type commandSync struct {
	owners map[string]string
	warned map[string]bool
}

// RecipeCommandNames uses the same declaration set for sync and doctor.
func RecipeCommandNames(root, home string) (map[string]string, error) {
	_, _, commands, err := loadPruneInputs(root, home)
	return commands, err
}

// Retired names are evidence for a warning, never authority to delete an agent file.
func StaleRecipeCommandNames(root, home string) map[string]bool {
	cache := projectcache.CacheRoot(root, home)
	var retired []string
	if raw, err := os.ReadFile(filepath.Join(cache, retiredCommandsFile)); err == nil {
		_ = json.Unmarshal(raw, &retired)
	}
	names := materializeStringSet(retired)
	for _, path := range globMDFilesSh(projectcache.CommandsDir(root, home)) {
		names[filepath.Base(path)] = true
	}
	catalog := filepath.Join(home, "catalog", "recipes")
	entries, _ := os.ReadDir(catalog)
	for _, entry := range entries {
		if recipe, err := schema.LoadRecipeToml(filepath.Join(catalog, entry.Name(), "recipe.toml")); err == nil {
			for _, command := range recipe.Commands {
				names[command.ID+".md"] = true
			}
		}
	}
	return names
}

func StaleRecipeCommandWarning(rel string) string {
	return rel + " looks like a stale ai-specs command but has no ownership record; delete it if you did not write it"
}

type commandRemoval struct {
	base, path, rel, hash string
	info                  os.FileInfo
	record                map[string]any
}

func (c *commandRemoval) stat() (os.FileInfo, string, error) {
	if exists, err := pruneFile(c.base, c.path); err != nil || !exists {
		return nil, "", err
	}
	info, err := os.Lstat(c.path)
	if err != nil {
		return nil, "", err
	}
	hash, err := lock.Sha256OfFile(c.path)
	return info, hash, err
}

func planCommandPrune(root, home string, paths []string, live map[string]string) (func(io.Writer) (map[string]bool, error), error) {
	home = projectcache.AISpecsHome(home)
	cache := projectcache.CacheRoot(root, home)
	commands := projectcache.CommandsDir(root, home)
	witness := filepath.Join(cache, retiredCommandsFile)
	if _, err := pruneFile(home, witness); err != nil {
		return nil, err
	}
	for _, dir := range []string{commands, filepath.Join(cache, "merged-commands")} {
		if err := shared.EnsureTemplateAncestorsReal(home, filepath.Join(dir, "entry")); err != nil {
			return nil, err
		}
		if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
			return nil, fmt.Errorf("destination is not a directory: %s", dir)
		}
	}
	entries, err := os.ReadDir(commands)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var doomed []commandRemoval
	for _, entry := range entries {
		if name := entry.Name(); filepath.Ext(name) == ".md" && live[name] == "" {
			doomed = append(doomed, commandRemoval{base: home, path: filepath.Join(commands, name)})
		}
	}
	lockPath := filepath.Join(root, bundledLockRel)
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	known := StaleRecipeCommandNames(root, home)
	dirs := map[string]bool{}
	for _, target := range paths {
		for _, platform := range agentPlatformMatrix {
			if platform.commandsDir != "" {
				dirs[filepath.Join(target, platform.commandsDir)] = true
			}
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(lk.Managed)) {
		rec, name, path := lk.Managed[rel], filepath.Base(rel), filepath.Join(root, rel)
		if rec["kind"] == RecipeCommandKind && fs.ValidPath(rel) && dirs[filepath.Dir(path)] && live[name] == "" {
			doomed = append(doomed, commandRemoval{base: root, path: path, rel: rel, record: rec})
			known[name] = true
		}
	}
	for i := range doomed {
		item := &doomed[i]
		item.info, item.hash, err = item.stat()
		if err != nil {
			return nil, err
		}
	}
	return func(w io.Writer) (map[string]bool, error) {
		fresh, err := lock.LoadLock(lockPath)
		if err != nil {
			return nil, err
		}
		var remove []string
		changed := false
		for _, item := range doomed {
			if item.rel != "" && !reflect.DeepEqual(fresh.Managed[item.rel], item.record) {
				continue // A template may have acquired this destination during materialization.
			}
			info, hash, err := item.stat()
			if err != nil {
				return nil, err
			}
			if info != nil {
				// Compare with preflight, not the lock hash: prior edits don't revoke ownership.
				if item.info == nil || !os.SameFile(info, item.info) || hash != item.hash {
					if item.rel == "" {
						return nil, fmt.Errorf("cache command changed after preflight: %s", item.path)
					}
					continue // Keep the file and its ownership record for the next sync.
				}
				remove = append(remove, item.path)
			}
			if item.rel != "" {
				delete(fresh.Managed, item.rel)
				changed = true
			}
		}
		for _, path := range remove {
			if err := os.Remove(path); err != nil {
				return nil, err
			}
		}
		if changed {
			if err := lock.WriteLock(lockPath, fresh); err != nil {
				return nil, err
			}
		}
		warned := map[string]bool{}
		for _, dir := range slices.Sorted(maps.Keys(dirs)) {
			for _, path := range globMDFilesSh(dir) {
				name := filepath.Base(path)
				rel, _ := filepath.Rel(root, path)
				if known[name] && live[name] == "" && fresh.Managed[rel] == nil && !isRegularFilePath(filepath.Join(root, "ai-specs", "commands", name)) && !isRegularFilePath(filepath.Join(projectcache.BundledCommandsRoot(root, home), name)) {
					fmt.Fprintln(w, "warning: "+StaleRecipeCommandWarning(rel))
					warned[path] = true
				}
			}
		}
		if len(doomed) > 0 && len(warned) > 0 {
			raw, _ := json.Marshal(materializeSortedSet(known))
			if _, err := pruneFile(home, witness); err != nil {
				return nil, err
			}
			if err := shared.WriteTemplateContent(witness, raw, 0o644); err != nil {
				return nil, err
			}
		}
		return warned, nil
	}, nil
}
