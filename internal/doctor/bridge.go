package doctor

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The one Go->Python seam of this port. It exists because one dependency stack
// behind doctor is not ported yet (recipe-materialize/agents-render for the
// brief provenance chain) and porting it is another card's scope. Every check,
// severity, message, ordering and exit code stays in Go; the bridge only
// returns raw analysis results, and each op degrades exactly the way the legacy
// in-process call did.

//go:embed bridge.py
var bridgeScript string

// bridgeTimeout bounds one bridge call. The brief provenance op rebuilds the
// whole resolved recipe graph, which is the slowest thing doctor does.
const bridgeTimeout = 300 * time.Second

// bridgeResponse holds one decoded result per requested op.
type bridgeResponse struct {
	ops map[string]json.RawMessage
}

// op decodes one op's result. A missing op or an unavailable bridge is an
// error, which each caller maps onto its own legacy degradation (skip the
// check, "undetermined", ...).
func (d *Doctor) op(name string, target any) error {
	if d.bridgeErr != nil {
		return d.bridgeErr
	}
	if d.bridge == nil {
		return fmt.Errorf("bridge result unavailable")
	}
	raw, ok := d.bridge.ops[name]
	if !ok {
		return fmt.Errorf("bridge ran no %s op", name)
	}
	return json.Unmarshal(raw, target)
}

type bridgeRequest struct {
	Root string   `json:"root"`
	Home string   `json:"home"`
	Ops  []string `json:"ops"`
}

// tomlErrorResult carries tomllib's diagnostic for an unparseable manifest.
type tomlErrorResult struct {
	Error *string `json:"error"`
}

// briefStateResult carries renderer.brief_effective_state.
type briefStateResult struct {
	State string `json:"state"`
}

// briefDeadResult carries has_dead_recipe_fragments; nil means the legacy
// call site swallowed a failure and emitted no check.
type briefDeadResult struct {
	Dead *bool `json:"dead"`
}

// bridgeOps is the set of ops this run needs, derived from the same native
// manifest read the checks use and listed in check order (the legacy warnings
// reach stderr in that order).
func (d *Doctor) bridgeOps() []string {
	d.loadManifest()
	if !d.manifest.exists {
		return nil
	}
	var ops []string
	if d.manifest.data == nil {
		// Unreadable or unparseable: _check_manifest renders tomllib's own
		// diagnostic as guidance.
		ops = append(ops, "toml_error")
	}
	enabled, err := briefRenderEnabled(d.manifest.data)
	if err != nil || enabled {
		// Render enabled (or an invalid render type, which
		// _brief_render_disabled treats as enabled): _check_brief_provenance
		// rebuilds the brief.
		ops = append(ops, "brief_state")
	} else if d.manifestHasContent() {
		// _check_brief_render_policy returns early on a falsy manifest.
		ops = append(ops, "brief_dead_fragments")
	}
	return ops
}

// runBridge performs the single batched Go->Python call for this run.
func (d *Doctor) runBridge() (*bridgeResponse, error) {
	ops := d.bridgeOps()
	if len(ops) == 0 {
		return &bridgeResponse{ops: map[string]json.RawMessage{}}, nil
	}
	payload, err := json.Marshal(bridgeRequest{Root: d.Root, Home: d.Home, Ops: ops})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), bridgeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", bridgeScript, d.Home, string(payload))
	// -B/PYTHONDONTWRITEBYTECODE keep the port read-only: the legacy doctor
	// wrote __pycache__ into $AI_SPECS_HOME/lib/_internal (parity defect D26).
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "PYTHONDONTWRITEBYTECODE=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stderr = d.Stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("python3 bridge timed out after %s", bridgeTimeout)
		}
		return nil, fmt.Errorf("python3 bridge failed: %w", err)
	}
	var parsed struct {
		Ops map[string]json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("python3 bridge returned an unreadable response: %w", err)
	}
	return &bridgeResponse{ops: parsed.Ops}, nil
}
