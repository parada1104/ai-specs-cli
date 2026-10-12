package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/worktree-gate/shared"
)

// Config merge decision for recipe materialization. The pure decision core now
// lives in ai-specs.dev/worktree-gate/shared (GO-07 S13 PR2) so the gate
// binary's --plan-merge-config surface and the root native sync authority share
// one implementation. This file keeps the historical lowercase names as thin
// type aliases and wrappers so every existing gate test compiles unchanged;
// only the --plan-merge-config flag wiring stays here, and it calls the shared
// core.

// Historical aliases: the moved shared types under their original local names.
type (
	mergeConfigField   = shared.MergeConfigField
	mergeConfigTable   = shared.MergeConfigTable
	mergeConfigPair    = shared.MergeConfigPair
	mergeConfigRequest = shared.MergeConfigRequest
	mergeConfigResult  = shared.MergeConfigResult
)

// planMergeConfig is the historical wrapper over the shared decision core.
func planMergeConfig(req mergeConfigRequest) (mergeConfigResult, error) {
	return shared.PlanMergeConfig(req)
}

// decodeOrdered is the historical wrapper over the shared ordered decoder,
// kept so the recipe-config writer keeps its original call shape.
func decodeOrdered(data []byte) (any, error) {
	return shared.DecodeOrdered(data)
}

// runPlanMergeConfig is the --plan-merge-config command: decode the JSON
// envelope from stdin, merge, print one JSON object on stdout. A malformed
// envelope is a process-level failure (exit 2); a semantic validation error is
// reported in the envelope's error field with exit 0.
func runPlanMergeConfig(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-merge-config: read stdin: %v\n", err)
		return 2
	}
	var req mergeConfigRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-merge-config: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(req.RecipeName) == "" {
		fmt.Fprintf(stderr, "worktree-gate: --plan-merge-config: missing recipe_name\n")
		return 2
	}
	result, err := planMergeConfig(req)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-merge-config: %v\n", err)
		return 2
	}
	payload, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-merge-config: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
