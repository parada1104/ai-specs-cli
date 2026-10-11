package main

import (
	"time"

	"ai-specs.dev/worktree-gate/shared"
)

// Durable tracker binding witness (A4). The writer core now lives in
// ai-specs.dev/worktree-gate/shared (GO-07 S13 PR1) so the gate command and the
// root native sync authority write one document through one writer. This file
// keeps the historical lowercase names as aliases and wrappers so every existing
// call site and test compiles unchanged.

// Historical aliases: the moved shared types under their original local names.
type (
	witnessPayload = shared.WitnessPayload
	witnessOutcome = shared.WitnessOutcome
)

// witnessWrittenAt is the historical wrapper over the shared core.
func witnessWrittenAt(now time.Time) string {
	return shared.WitnessWrittenAt(now)
}

// trackingDeclared is the historical wrapper over the shared core.
func trackingDeclared(projectRoot string) bool {
	return shared.TrackingDeclared(projectRoot)
}

// trackerWitnessPayload is the historical wrapper over the shared core.
func trackerWitnessPayload(enabledIDs []string, capsByRecipe map[string][]string, resolved map[string]string, declared bool, writtenAt string) witnessPayload {
	return shared.TrackerWitnessPayload(enabledIDs, capsByRecipe, resolved, declared, writtenAt)
}

// marshalTrackerWitness is the historical wrapper over the shared core.
func marshalTrackerWitness(payload witnessPayload) ([]byte, error) {
	return shared.MarshalTrackerWitness(payload)
}

// writeTrackerWitness is the historical wrapper over the shared core.
func writeTrackerWitness(projectRoot string, payload witnessPayload) witnessOutcome {
	return shared.WriteTrackerWitness(projectRoot, payload)
}
