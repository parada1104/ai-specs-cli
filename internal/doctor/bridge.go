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
// behind doctor is not ported yet, and cannot be: tomllib's own manifest
// diagnostic, whose text is owned by the interpreter version. Every check,
// severity, message, ordering and exit code stays in Go; the bridge only
// returns raw analysis results, and its one op degrades exactly the way the
// legacy in-process call did. The brief probes (`brief_state`,
// `brief_dead_fragments`) ran here until GO-07 S18; they are native now, so a
// healthy project needs no Python at all.

//go:embed bridge.py
var bridgeScript string

// bridgeTimeout bounds one bridge call. Only the manifest diagnostic remains,
// which imports tomllib and parses one file.
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

// bridgeOps is the set of ops this run needs. Only tomllib's own manifest
// diagnostic is still interpreter-owned since GO-07 S18 (the brief probes run in
// process), so an unreadable or unparseable manifest is the only reason to
// spawn Python at all.
func (d *Doctor) bridgeOps() []string {
	d.loadManifest()
	if !d.manifest.exists {
		return nil
	}
	if d.manifest.data == nil {
		// Unreadable or unparseable: _check_manifest renders tomllib's own
		// diagnostic as guidance.
		return []string{"toml_error"}
	}
	return nil
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
