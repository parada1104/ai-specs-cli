package main

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-specs.dev/worktree-gate/shared"
)

// reconcileStampOptions is the parsed --plan-reconcile-stamps flag surface.
type reconcileStampOptions struct {
	catalogDir string
	recipeIDs  []string
}

// runPlanReconcileStamps is the --plan-reconcile-stamps command: acquire the
// enabled recipes' stamp inputs through the TOML seam, build the stamps with
// the shared pure planner, and print one JSON object on stdout. Stamping is
// pure planning, so any planned result exits 0 — including a plan where every
// recipe was skipped — and only unusable flags or an unavailable parser exit 2.
func runPlanReconcileStamps(opts reconcileStampOptions, stdout, stderr io.Writer) int {
	if opts.catalogDir == "" {
		fmt.Fprintln(stderr, "worktree-gate: --plan-reconcile-stamps requires --catalog-dir")
		return 2
	}
	acquired, err := loadReconcileStampSources(opts.catalogDir, opts.recipeIDs)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-reconcile-stamps: %v\n", err)
		return 2
	}
	plan := shared.ReconcileStampPlan{Stamps: []shared.ReconcileStampEntry{}}
	for _, item := range acquired {
		plan.Stamps = append(plan.Stamps, shared.ReconcileStampEntry{ID: item.RecipeID, Stamp: shared.BuildReconcileStamp(item.Source)})
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-reconcile-stamps: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
