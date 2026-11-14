package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/worktree-gate/shared"
)

// CopyRoots carries the caller-resolved cache roots the copy plan needs.
type CopyRoots struct {
	RecipeSkillsRoot string
	CommandsDir      string
	ProjectRoot      string
}

// PlanCopyItems derives the copy items Python's materialize call sites send
// today (lib/_internal/recipe-materialize.py:4033-4051). It is pure
// derivation: the declared sources are never stat'ed, checked for existence,
// or copied here, so a plan succeeds for a catalog whose files are absent —
// the executor decides presence. Per enabled recipe it emits the bundled
// skills in declared order, then the commands, then the docs: that loop order
// is the Python oracle's (bundled skills, deps, commands, docs — the dep
// branch produces no item here because dep skill acquisition stays Python).
// A recipe whose recipe.toml is missing or fails to load is skipped, matching
// PlanReconcileStamps and the Python read_recipe call sites. Non-bundled
// skills are not planned: their sources belong to the dependency path.
func PlanCopyItems(catalogDir string, recipeIDs []string, roots CopyRoots) []shared.CopyItem {
	out := []shared.CopyItem{}
	for _, rid := range recipeIDs {
		recipe, err := schema.LoadRecipeToml(filepath.Join(catalogDir, rid, "recipe.toml"))
		if err != nil {
			continue
		}
		recipeDir := filepath.Join(catalogDir, rid)
		for _, skill := range recipe.Skills {
			if skill.Source != "bundled" {
				continue
			}
			out = append(out, shared.CopyItem{
				Kind: "bundled-skill",
				ID:   skill.ID,
				Src:  filepath.Join(recipeDir, "skills", skill.ID),
				Dest: filepath.Join(roots.RecipeSkillsRoot, rid, "skills", skill.ID),
			})
		}
		for _, cmd := range recipe.Commands {
			out = append(out, shared.CopyItem{
				Kind:        "command",
				ID:          cmd.ID,
				Src:         filepath.Join(recipeDir, cmd.Path),
				Dest:        filepath.Join(roots.CommandsDir, cmd.ID+".md"),
				CommandsDir: roots.CommandsDir,
			})
		}
		for _, doc := range recipe.Docs {
			out = append(out, shared.CopyItem{
				Kind: "doc",
				ID:   doc.Target,
				Src:  filepath.Join(recipeDir, doc.Source),
				Dest: filepath.Join(roots.ProjectRoot, doc.Target),
			})
		}
	}
	return out
}

// ApplyCopyItems executes the items in process through the shared copy
// authority (shared.RunApplyCopy) that the gate binary's --apply-copy surface
// uses. It runs in-memory: one marshalled envelope in, the result envelope
// out. No subprocess is spawned. On a delivered non-zero exit the error
// envelope names the failing item, and that message becomes the Go error. The
// shared authority performs no hashing, lock writes, printing, warnings or doc
// classification — those stay with the Python call sites until S14.
func ApplyCopyItems(items []shared.CopyItem) ([]shared.CopyResult, error) {
	payload, err := json.Marshal(shared.CopyRequest{Items: items})
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	if code := shared.RunApplyCopy(bytes.NewReader(payload), &stdout, &stderr); code != 0 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(stdout.Bytes(), &failure) == nil && failure.Error != "" {
			return nil, errors.New(failure.Error)
		}
		return nil, fmt.Errorf("copy apply: exit %d: %s", code, strings.TrimSpace(stderr.String()))
	}
	var envelope struct {
		Results []shared.CopyResult `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return nil, fmt.Errorf("copy apply: decode results: %w", err)
	}
	if envelope.Results == nil {
		envelope.Results = []shared.CopyResult{}
	}
	return envelope.Results, nil
}
