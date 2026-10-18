package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// Durable tracker binding witness (A4).
//
// This is the Go producer for the witness the Python authority writes in
// recipe-materialize.write_tracker_witness: the already-resolved tracker binding,
// persisted at <git-common-dir>/ai-specs/ledger/witness.json. There is no verdict
// logic here — resolveBindings stays the only binding producer, and an
// unresolved capability records candidate recipe ids instead of guessing a
// provider (D6). The ledger package owns the schema constants and the reader that
// accepts these bytes; the payload type is declared here because the ledger's own
// witness struct is deliberately unexported in its read-only package.

// witnessPayload is the on-disk witness. Fields are declared in the sorted-key
// order Python's json.dump(sort_keys=True) emits, so the bytes match the Python
// writer key for key. Candidates is always a list, never null, so an empty
// candidate set stays `[]` exactly as Python writes it.
type witnessPayload struct {
	Candidates []string `json:"candidates"`
	Capability string   `json:"capability"`
	RecipeID   string   `json:"recipe_id"`
	State      string   `json:"state"`
	V          int      `json:"v"`
	WrittenAt  string   `json:"written_at"`
}

// witnessOutcome is the best-effort durable write result. Path is empty when
// nothing was written (no repository, or a failed write); Warning is set only
// when a write was attempted and failed, and never turns into a process failure.
type witnessOutcome struct {
	Path    string
	Warning string
}

// witnessWrittenAt is the instant format the Python writer produces with
// strftime("%Y-%m-%dT%H:%M:%SZ"): UTC at second precision.
func witnessWrittenAt(now time.Time) string {
	return now.UTC().Format("2006-01-02T15:04:05Z")
}

// trackingDeclared reports whether openspec/config.yaml declares a top-level
// `tracking:` block. The declaration is supply, never activation (D6); it only
// separates declared-not-bound from plain unbound.
func trackingDeclared(projectRoot string) bool {
	data, err := os.ReadFile(filepath.Join(projectRoot, "openspec", "config.yaml"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "tracking:") {
			return true
		}
	}
	return false
}

// trackerWitnessPayload describes the already-resolved tracker binding, mirroring
// recipe-materialize.tracker_witness_payload. A bound capability names its
// resolved recipe and records no candidate; otherwise every enabled recipe
// declaring the tracker capability becomes a candidate in enabled order, and only
// more than one of them is ambiguous.
func trackerWitnessPayload(enabledIDs []string, capsByRecipe map[string][]string, resolved map[string]string, declared bool, writtenAt string) witnessPayload {
	recipeID := resolved[ledger.CapabilityTracker]
	candidates := []string{}
	state := ledger.WitnessBound
	if recipeID == "" {
		for _, rid := range enabledIDs {
			// A recipe the acquisition seam could not read declares nothing and
			// contributes no candidate, exactly as the Python loader swallows a
			// recipe it cannot read.
			for _, capability := range capsByRecipe[rid] {
				if capability == ledger.CapabilityTracker {
					candidates = append(candidates, rid)
					break
				}
			}
		}
		switch {
		case len(candidates) > 1:
			state = ledger.WitnessAmbiguous
		case declared:
			state = ledger.WitnessDeclaredNotBound
		default:
			state = ledger.WitnessUnbound
		}
	}
	return witnessPayload{
		Candidates: candidates,
		Capability: ledger.CapabilityTracker,
		RecipeID:   recipeID,
		State:      state,
		V:          ledger.WitnessVersion,
		WrittenAt:  writtenAt,
	}
}

// marshalTrackerWitness renders the payload exactly as the Python writer's
// json.dump(indent=2, sort_keys=True) plus a trailing newline. The struct is
// declared in sorted-key order, HTML escaping is off (Python escapes nothing),
// and the encoder appends the newline.
func marshalTrackerWitness(payload witnessPayload) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeTrackerWitness persists the payload under projectRoot's Git common dir. It
// is best-effort by contract: outside a repository there is no common dir and
// nothing is written, silently; a write error is reported as a warning and leaves
// the ledger dormant (a missing witness) instead of failing the caller.
func writeTrackerWitness(projectRoot string, payload witnessPayload) witnessOutcome {
	// The absolute-form lookup then the fallback is the Python
	// git_common_dir order; realpath makes a linked worktree and its main
	// checkout agree on one shared directory (A2).
	commonDir := gitCommon(projectRoot)
	if commonDir == "" {
		return witnessOutcome{}
	}
	data, err := marshalTrackerWitness(payload)
	if err != nil {
		return witnessOutcome{Warning: witnessWriteWarning(err)}
	}
	target := ledger.WitnessPath(RealPath(commonDir))
	if err := writeFileAtomic(target, data); err != nil {
		return witnessOutcome{Warning: witnessWriteWarning(err)}
	}
	return witnessOutcome{Path: target}
}

// witnessWriteWarning is the dormant-ledger warning carrying the write error. The
// ledger never re-derives bindings, so an unwritable witness must be visible and
// non-fatal, not silently bound.
func witnessWriteWarning(err error) string {
	return fmt.Sprintf("tracker witness not written (%v); ledger stays dormant", err)
}

// writeFileAtomic writes data to target through a sibling temporary file: create,
// write, fsync, then rename over the target, so a reader never observes a partial
// witness. The temporary file is removed on any failure and shares the target
// directory, so the rename stays within one filesystem.
func writeFileAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "witness.json.tmp.*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	// fsync before the rename: a crash must not leave a renamed but empty witness.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
