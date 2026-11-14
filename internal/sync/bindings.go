package sync

// Native root authority for capability binding resolution and the durable
// tracker witness (GO-07 S13 PR1).
//
// The decision core is the shared package
// (ai-specs.dev/worktree-gate/shared.GradeBindings) the gate binary's
// --resolve-bindings surface also runs, so the root single binary and the gate
// share one decision — never a second port. This file owns only the root
// boundary: it acquires the enabled recipes' declared capabilities natively and
// performs the same best-effort witness write the Python authority performs.
//
// It reproduces the Python fallback authority
// (lib/_internal/recipe-materialize.py): _python_resolve_bindings (:2614) plus
// the capability-conflict grader it is paired with, and the durable writer
// write_tracker_witness (:2763) through the shared producer.
//
// Boundary: S14 owns the sync-pipeline wiring; this authority is added, not
// called from the pipeline.

import (
	"os"
	"path/filepath"
	"time"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/worktree-gate/shared"
)

// BindingRequest is the root boundary input for one binding resolution: the
// enabled recipe ids in manifest order, the recipe catalog directory, the
// explicit manifest bindings, the repository the durable witness is written
// under, and whether the witness write is requested. It mirrors what the Python
// authority receives and is mapped onto the shared decision core.
type BindingRequest struct {
	EnabledRecipeIDs []string
	CatalogDir       string
	Bindings         []shared.ManifestBinding
	ProjectRoot      string
	WriteWitness     bool
}

// ResolveBindings grades capability bindings and conflicts through the shared
// core and, when WriteWitness is set and resolution produced no error, writes the
// durable tracker witness. Resolution errors are returned as data in
// Errors (never a Go error): the caller decides policy, exactly as the gate's
// --resolve-bindings envelope does.
//
// An unresolved binding is never persisted: the Python authority aborts before
// its witness write on a resolution error, so writing a guessed unbound state
// would invent a decision the resolver never made. The write is best-effort: no
// repository means nothing is written silently, and a write failure becomes a
// Warning instead of a failure.
func ResolveBindings(req BindingRequest) shared.BindingResolution {
	capsByRecipe := acquireRecipeCapabilities(req.CatalogDir, req.EnabledRecipeIDs)
	result := shared.GradeBindings(req.EnabledRecipeIDs, capsByRecipe, req.Bindings)
	if req.WriteWitness && len(result.Errors) == 0 {
		witness := shared.TrackerWitnessPayload(
			req.EnabledRecipeIDs, capsByRecipe, result.Bindings,
			shared.TrackingDeclared(req.ProjectRoot), shared.WitnessWrittenAt(time.Now()),
		)
		if warning := shared.WriteTrackerWitness(req.ProjectRoot, witness).Warning; warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}
	return result
}

// acquireRecipeCapabilities reads each enabled recipe's declared capability ids
// natively through internal/schema.LoadRecipeToml. A recipe.toml that is missing
// or is not a regular file is skipped, and a present file the loader rejects is
// skipped too: the Python authority (_python_resolve_bindings at
// lib/_internal/recipe-materialize.py:2614) wraps read_recipe in
// `except Exception: continue`, so an unreadable recipe declares nothing rather
// than propagating (unlike the tag-conflict acquisition, which has no
// try/except). Repeated ids are read once; the shared BuildCapabilityIndex still
// counts repeated enabled ids per occurrence, matching the Python provider list.
func acquireRecipeCapabilities(catalogDir string, recipeIDs []string) map[string][]string {
	capsByRecipe := make(map[string][]string, len(recipeIDs))
	seen := make(map[string]bool, len(recipeIDs))
	for _, rid := range recipeIDs {
		if seen[rid] {
			continue
		}
		seen[rid] = true
		path := filepath.Join(catalogDir, rid, "recipe.toml")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		recipe, err := schema.LoadRecipeToml(path)
		if err != nil {
			continue
		}
		caps := make([]string, 0, len(recipe.Capabilities))
		for _, capability := range recipe.Capabilities {
			caps = append(caps, capability.ID)
		}
		capsByRecipe[rid] = caps
	}
	return capsByRecipe
}
