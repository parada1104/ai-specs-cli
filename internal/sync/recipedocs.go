package sync

// CLI-served recipe docs (cli-served-recipes T5, S1).
//
// A recipe README is repository documentation, not project material: the
// catalog keeps catalog/recipes/<id>/README.md, and sync neither writes it into
// ai-specs/recipes/<id>/ nor leaves an older copy there. Every copy an older
// sync recorded is removed — including a user-edited one (S1). A copy no record
// proves ai-specs wrote is left alone: sync never deletes what it cannot prove
// it owns.

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ai-specs.dev/ai-specs/internal/lock"
)

// recipeREADMEServesCLI reports whether a recipe's declared doc target is the
// README inside its own ai-specs/recipes/<id> directory (the CLI serves it).
func recipeREADMEServesCLI(recipeID, target string) bool {
	return posixAsPosix(target) == "ai-specs/recipes/"+recipeID+"/README.md"
}

// recipeREADMERecord reports whether a managed lock record names one of those
// targets, in a spelling the project tree can hold.
func recipeREADMERecord(rel string, record map[string]any) bool {
	recipe, _ := record["recipe"].(string)
	if kind, _ := record["kind"].(string); kind != "doc" || !fs.ValidPath(rel) || strings.Contains(rel, `\`) {
		return false
	}
	return recipeREADMEServesCLI(recipe, rel)
}

// retireRecipeREADMEDocs removes the recipe README copies older syncs recorded.
// It runs on every sync, after materialization, so nothing regenerates them.
func retireRecipeREADMEDocs(root, lockPath string, out io.Writer) error {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	var doomed []string
	for rel, record := range lk.Managed {
		if recipeREADMERecord(rel, record) {
			doomed = append(doomed, rel)
		}
	}
	if len(doomed) == 0 {
		return nil
	}
	slices.Sort(doomed)
	for _, rel := range doomed {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := projectWriteGuard(root, path, false); err != nil {
			return err
		}
		recorded, _ := lk.Managed[rel]["sha256"].(string)
		current, hashErr := lock.Sha256OfFile(path)
		if recorded == "" || hashErr == nil && current != recorded {
			fmt.Fprintf(out, "  ✓ removed user-modified recipe README %s (ai-specs keeps recipe docs in its catalog)\n", rel)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := removeEmptiedParents(filepath.Dir(path), path); err != nil {
			return err
		}
		delete(lk.Managed, rel)
	}
	return lock.WriteLock(lockPath, lk)
}
