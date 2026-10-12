package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"ai-specs.dev/worktree-gate/shared"
)

// Capability binding resolution and capability conflict grading. The pure
// decision core now lives in ai-specs.dev/worktree-gate/shared (GO-07 S13 PR1)
// so the gate command and the root native sync authority share one
// implementation. This file keeps the historical lowercase names as thin type
// aliases and wrappers so every existing call site and test compiles unchanged;
// only the --resolve-bindings flag wiring (runResolveBindings) stays here, and
// it calls the shared core.

// Historical aliases: the moved shared types under their original local names.
type (
	manifestBinding    = shared.ManifestBinding
	capabilityConflict = shared.CapabilityConflict
	bindingResolution  = shared.BindingResolution
	capabilityIndex    = shared.CapabilityIndex
)

// buildCapabilityIndex is the historical wrapper over the shared core.
func buildCapabilityIndex(enabledIDs []string, capsByRecipe map[string][]string) capabilityIndex {
	return shared.BuildCapabilityIndex(enabledIDs, capsByRecipe)
}

// resolveBindings is the historical wrapper over the shared core.
func resolveBindings(idx capabilityIndex, explicit []manifestBinding) (map[string]string, []string) {
	return shared.ResolveBindings(idx, explicit)
}

// capabilityConflicts is the historical wrapper over the shared core.
func capabilityConflicts(idx capabilityIndex, explicit []manifestBinding) []capabilityConflict {
	return shared.CapabilityConflicts(idx, explicit)
}

// gradeBindings is the historical wrapper over the shared core.
func gradeBindings(enabledIDs []string, capsByRecipe map[string][]string, explicit []manifestBinding) bindingResolution {
	return shared.GradeBindings(enabledIDs, capsByRecipe, explicit)
}

// parseManifestBindings is the historical wrapper over the shared core.
func parseManifestBindings(raw string) ([]manifestBinding, error) {
	return shared.ParseManifestBindings(raw)
}

// bindingsOptions is the parsed --resolve-bindings flag surface. projectRoot is
// the repository the durable witness is written under; writeWitness is opt-out so
// a caller (or test) can resolve bindings without touching the ledger.
type bindingsOptions struct {
	catalogDir   string
	recipeIDs    []string
	bindings     string
	projectRoot  string
	writeWitness bool
}

// runResolveBindings is the --resolve-bindings command: acquire the enabled
// recipes' capabilities through the TOML seam, grade bindings and conflicts,
// persist the durable tracker witness, and print one JSON object on stdout.
// Resolution errors are data (exit 0); only a process-level failure — unusable
// flags or an unavailable parser — exits 2.
func runResolveBindings(opts bindingsOptions, stdout, stderr io.Writer) int {
	if opts.catalogDir == "" {
		fmt.Fprintln(stderr, "worktree-gate: --resolve-bindings requires --catalog-dir")
		return 2
	}
	explicit, err := parseManifestBindings(opts.bindings)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: invalid --bindings JSON: %v\n", err)
		return 2
	}
	capsByRecipe, err := loadRecipeCapabilities(opts.catalogDir, opts.recipeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: %v\n", err)
		return 2
	}
	result := gradeBindings(opts.recipeIDs, capsByRecipe, explicit)
	if opts.writeWitness && len(result.Errors) == 0 {
		// An unresolved binding is not persisted: the Python authority aborts
		// before its witness write on a resolution error, so writing a guessed
		// unbound state here would invent a decision the resolver never made.
		// The root defaults to cwd and the write is best-effort: a missing Git
		// common dir skips silently, a write failure is a warning, never an exit.
		projectRoot := opts.projectRoot
		if projectRoot == "" {
			projectRoot = "."
		}
		witness := trackerWitnessPayload(
			opts.recipeIDs, capsByRecipe, result.Bindings,
			trackingDeclared(projectRoot), witnessWrittenAt(time.Now()),
		)
		if warning := writeTrackerWitness(projectRoot, witness).Warning; warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}
	payload, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --resolve-bindings: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
