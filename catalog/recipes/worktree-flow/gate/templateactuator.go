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

// --materialize-template, the thin JSON wrapper over the shared template
// actuator core (shared/templateactuator.go, GO-07 S12.2). The decision +
// execution live in ai-specs.dev/worktree-gate/shared so the gate binary and
// the root native sync authority share one shell; this file only decodes the
// stdin envelope, maps the result, and prints the stdout envelope / exit
// codes.
//
// Command surface: --materialize-template reads one JSON envelope on stdin
//
//	{"project_root": "...", "recipe_dir": "...", "recipe_id": "...",
//	 "source": "...", "target": "...", "condition": "not_exists",
//	 "update_policy": "auto", "config": {"repo_topology": "...", ...},
//	 "managed_entry": {"sha256": "..."} | null}
//
// and prints one result envelope on stdout with exit 0:
//
//	{"dest": "...", "wrote": true|false, "record": {...}|null,
//	 "message": "...", "info": "...", "warnings": [...], "error": null}
//
// A refusal (invalid update policy, missing source, symlinked destination,
// execution failure) prints {"error": "<exact reference string>"} on stdout
// with exit 2 and touches no file. The Python bridge fails CLOSED on a
// delivered exit-2 error envelope and falls back to its historical Python body
// on infrastructure failures (no binary, bad JSON, malformed stdout envelope).
//
// Exit-2 taxonomy: INPUT/INFRASTRUCTURE failures (unreadable stdin, malformed
// input JSON, missing project_root/target, unreadable source/destination)
// print a stderr diagnostic and NO envelope, so Python falls back to its
// historical body; DECISION refusals emit the error envelope on stdout, so
// Python fails closed and never bypasses the decision.

// Thin aliases keep this package's call sites and tests on the historical
// local names while the core lives in the shared package.
var (
	renderTemplateBytes            = shared.RenderTemplateBytes
	resolveTemplateDest            = shared.ResolveTemplateDest
	writeTemplateContent           = shared.WriteTemplateContent
	ensureTemplateAncestorsReal    = shared.EnsureTemplateAncestorsReal
	pyConfigString                 = shared.PyConfigString
	templateConfigOr               = shared.TemplateConfigOr
	templateEscapingTargetRefusal  = shared.TemplateEscapingTargetRefusal
	templateSymlinkRefusal         = shared.TemplateSymlinkRefusal
	templateAncestorSymlinkRefusal = shared.TemplateAncestorSymlinkRefusal
	errDestSymlink                 = shared.ErrDestSymlink
	errAncestorSymlink             = shared.ErrAncestorSymlink
)

// templateActuatorOutput is the stdout contract.
//   - dest: the resolved absolute destination (what ResolveTemplateDest
//     produced, anchored when git emitted a repo-relative path).
//   - wrote: the actuator wrote the file itself (mkdir -p parent, rendered
//     bytes, chmod to the source permission bits). Python never re-writes.
//   - record: the managed-override payload, nil when nothing is recorded.
//   - message: the per-target detail line WITHOUT the print indentation
//     ("✓ template {target}" or "· template skipped (exists) {target}").
//   - info: optional info() line ("refreshed managed template {target}").
//   - warnings: exact warn() strings, in emission order.
//   - error: set with exit 2; no file is touched on refusal paths.
type templateActuatorOutput struct {
	Dest     string                `json:"dest"`
	Wrote    bool                  `json:"wrote"`
	Record   *shared.ManagedRecord `json:"record"`
	Message  string                `json:"message"`
	Info     string                `json:"info,omitempty"`
	Warnings []string              `json:"warnings"`
	Error    *string               `json:"error"`
}

// runMaterializeTemplate is the --materialize-template command: one JSON
// envelope on stdin, one JSON envelope on stdout, exit 0/2.
func runMaterializeTemplate(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: read stdin: %v\n", err)
		return 2
	}
	var in shared.TemplateRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&in); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(in.ProjectRoot) == "" || strings.TrimSpace(in.Target) == "" {
		fmt.Fprintln(stderr, "worktree-gate: --materialize-template: missing project_root or target")
		return 2
	}
	emitEnvelope := func(out templateActuatorOutput, exit int) int {
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-template: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return exit
	}

	result, err := shared.MaterializeTemplate(&in)
	if err != nil {
		var refusal *shared.Refusal
		if errors.As(err, &refusal) {
			message := refusal.Message
			return emitEnvelope(templateActuatorOutput{Error: &message}, 2)
		}
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: %v\n", err)
		return 2
	}
	return emitEnvelope(templateActuatorOutput{
		Dest:     result.Dest,
		Wrote:    result.Wrote,
		Record:   result.Record,
		Message:  result.Message,
		Info:     result.Info,
		Warnings: result.Warnings,
	}, 0)
}
