package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/worktree-gate/shared"
)

// --materialize-hook, the thin JSON wrapper over the shared hook actuator core
// (shared/hookactuator.go, GO-07 S12.3). The decision + execution live in
// ai-specs.dev/worktree-gate/shared so the gate binary and (in delivery B) the
// root native sync authority share one shell; this file only decodes the
// stdin envelope, maps the result, and prints the stdout envelope / exit
// codes.
//
// Command surface: --materialize-hook reads one JSON envelope on stdin
//
//	{"project_root": "...", "recipe_dir": "...", "recipe_id": "...",
//	 "script": "hooks/gate.sh", "config": {...}, "cli_home": "...",
//	 "gate_version": "dev", "refresh": false,
//	 "managed_entry": {"sha256": "..."} | null, "backup_path": "..."}
//
// and prints one result envelope on stdout with exit 0:
//
//	{"rel": "...", "dest": "...", "wrote": true|false, "record": {...}|null,
//	 "message": "...", "warnings": [...], "backup": "...", "error": null}
//
// A refusal (missing source, invalid config value, symlinked destination,
// write failure) prints {"error": "<exact reference string>"} on stdout with
// exit 2 and touches no file. The Python bridge fails CLOSED on a delivered
// exit-2 error envelope and falls back to its historical Python body on
// infrastructure failures. The exit-2 taxonomy matches the template actuator:
// INPUT/INFRASTRUCTURE failures print a stderr diagnostic and NO envelope
// (Python fails open); DECISION refusals emit the error envelope (Python
// fails closed).

// Thin aliases keep this package's call sites and tests on the historical
// local names while the core lives in the shared package. errDestSymlink is
// already aliased alongside the template actuator (templateactuator.go) and is
// reused here.
var (
	hookScriptRelPath     = shared.HookScriptRelPath
	hookRelPathEscapes    = shared.HookRelPathEscapes
	renderHookGateContent = shared.RenderHookGateContent
)

// hookGateRefreshWrite is the historical refresh write seam. The shared
// MaterializeHook receives it per call, so an in-process rollback test can
// still substitute the write step while the default resolves to the shared
// production writer.
var hookGateRefreshWrite = shared.HookGateRefreshWrite

// hookActuatorOutput is the stdout contract.
//   - rel: the project-relative materialized path
//     (ai-specs/recipes/{recipe_id}/hooks/{basename(script)}).
//   - dest: the absolute destination.
//   - wrote: Go wrote the file itself (mkdir -p parent, rendered bytes,
//     chmod 0755). Python never re-writes.
//   - record: the gate-baseline payload, nil when nothing is recorded.
//   - message: the detail line WITHOUT the print indentation ("✓ hook script
//     {rel}", "· hook skipped (current) {rel}", …).
//   - warnings: exact warn() strings, in emission order (Python adds the
//     "  ! " prefix).
//   - backup: the backup path Go wrote for a refresh (empty otherwise); on
//     failure Go removes a backup it created itself.
//   - error: set with exit 2; no file is touched on refusal paths.
type hookActuatorOutput struct {
	Rel      string                     `json:"rel"`
	Dest     string                     `json:"dest"`
	Wrote    bool                       `json:"wrote"`
	Record   *shared.HookActuatorRecord `json:"record"`
	Message  string                     `json:"message"`
	Warnings []string                   `json:"warnings"`
	Backup   string                     `json:"backup,omitempty"`
	Error    *string                    `json:"error"`
}

// runMaterializeHook is the --materialize-hook command: one JSON envelope on
// stdin, one JSON envelope on stdout, exit 0/2.
func runMaterializeHook(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: read stdin: %v\n", err)
		return 2
	}
	var in shared.HookActuatorRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&in); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(in.ProjectRoot) == "" || strings.TrimSpace(in.RecipeID) == "" || strings.TrimSpace(in.Script) == "" {
		fmt.Fprintln(stderr, "worktree-gate: --materialize-hook: missing project_root, recipe_id or script")
		return 2
	}
	emit := func(out hookActuatorOutput) int {
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	result, err := shared.MaterializeHook(&in, hookGateRefreshWrite)
	if err != nil {
		var refusal *shared.Refusal
		if errors.As(err, &refusal) {
			// A refusal is the error envelope emit already knows how to print;
			// the only difference is the exit code (2 either way — emit's 2 is
			// its marshal-failure diagnostic, a refusal's 2 is the decision).
			message := refusal.Message
			if code := emit(hookActuatorOutput{Error: &message}); code != 0 {
				return code
			}
			return 2
		}
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: %v\n", err)
		return 2
	}
	return emit(hookActuatorOutput{
		Rel:      result.Rel,
		Dest:     result.Dest,
		Wrote:    result.Wrote,
		Record:   result.Record,
		Message:  result.Message,
		Warnings: result.Warnings,
		Backup:   result.Backup,
	})
}
