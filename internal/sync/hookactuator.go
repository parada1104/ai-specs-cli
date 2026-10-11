package sync

// Native root authority for the runtime hook/gate actuator (GO-07 S12.3
// delivery B).
//
// The hook shell is the shared actuator core
// (ai-specs.dev/worktree-gate/shared.MaterializeHook) the gate binary's
// --materialize-hook surface also runs, so the root single binary and the gate
// share one decision + execution shell — never a second port. The shared core
// owns the rel-path traversal guard, the 8-placeholder render, the shared
// ownership classification (shared.ClassifyManagedOverride, the same core
// behind --plan-classify), the write + chmod 0755 and the refresh
// backup/rollback. This file only maps the root request onto the
// envelope-independent shared input and hands the outcome back.
//
// Python keeps the lock load/write (set_gate_baseline + write_lock), ALL
// printing, the refresh backup-path precomputation (project-cache ownership)
// and the gate-version resolution; the caller applies the returned record
// payload. It reproduces the Go-bridge dispatcher materialize_hook_script
// (lib/_internal/recipe-materialize.py:2085) and the historical Python body it
// falls back to, _python_materialize_hook_script (:2233).
//
// Boundary: no lock or manifest write happens here. Every call returns the
// gate-baseline record payload for the caller; S14 owns the lock write and the
// caller diagnostics.

import "ai-specs.dev/worktree-gate/shared"

// HookRequest is the root boundary input for one runtime hook actuation: the
// resolved project and recipe roots, the recipe-relative hook script, the
// merged recipe config (only gate_mode, gate_scope, repo_topology and
// gate_impl are read), the Python-resolved cli home and gate version, the
// refresh flag, the lock's managed[rel] subset (nil when untracked) and the
// Python-precomputed immutable refresh backup path. It mirrors
// shared.HookActuatorRequest field for field so the root owns its boundary
// type and S14 never assembles a gate-package envelope.
type HookRequest struct {
	ProjectRoot  string
	RecipeDir    string
	RecipeID     string
	Script       string
	Config       map[string]any
	CLIHome      string
	GateVersion  string
	Refresh      bool
	ManagedEntry *shared.ClassifyManagedEntry
	BackupPath   string
}

// MaterializeHook runs one runtime hook gate through the shared actuator core:
// the same rel-path guard, source presence, render, shared classification,
// write + chmod 0755 and refresh backup/rollback the gate's
// --materialize-hook surface uses, with the record payload returned instead of
// written. Decision refusals (missing source, an escaping rel path, an invalid
// config value, a symlinked destination, a failed write) come back as a
// *shared.Refusal carrying the exact historical string; any other error is an
// infrastructure failure.
//
// The refresh seam is nil. The shared code resolves nil to its own production
// writer (shared.HookGateRefreshWrite == shared.WriteTemplateContent), which is
// the same write+chmod step and symlink guard the gate surface uses. Passing
// nil keeps the root free of an injection seam — that seam exists only because
// the gate wrapper's in-package rollback test must inject a symlink swap the
// filesystem alone cannot create — and keeps the single source of truth for
// the production write step inside the shared package. Evidence that the
// nil-resolved default is the production writer: the differential refresh
// scenarios (backup+rewrite, rollback, symlink refusal) run through this exact
// call and match the real Python authority byte-for-byte, including the prior
// snapshot bytes in the immutable backup.
func MaterializeHook(req *HookRequest) (shared.HookActuatorResult, error) {
	in := &shared.HookActuatorRequest{
		ProjectRoot:  req.ProjectRoot,
		RecipeDir:    req.RecipeDir,
		RecipeID:     req.RecipeID,
		Script:       req.Script,
		Config:       req.Config,
		CLIHome:      req.CLIHome,
		GateVersion:  req.GateVersion,
		Refresh:      req.Refresh,
		ManagedEntry: req.ManagedEntry,
		BackupPath:   req.BackupPath,
	}
	return shared.MaterializeHook(in, nil)
}
