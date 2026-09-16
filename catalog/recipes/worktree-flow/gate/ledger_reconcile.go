package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// Remote reconciliation wiring (T2). The comparison grader itself is the pure
// ledger.Reconcile contract; this file only resolves its inputs and reports the
// outcome. Three rules shape the wiring:
//
//   - Go owns the binding: the provider comes from the witness, the item from the
//     item the grade selected, the scope and the declared expectations from the
//     project manifest. Nothing here reads Item.Status as an expected remote state,
//     and nothing compares provider vocabulary: scope_field, property and
//     config_field are opaque config names declared by the recipe.
//   - A missing mapping or a missing config value is an explicit non-agreeing
//     outcome, never a default and never a guess.
//   - The result is a sidecar next to the verdict: it never changes the graded
//     exit code, never writes the store, and never records a decision. Agreeing is
//     a conditional check of the declared expectations for the requested event,
//     not evidence that the event was delivered.

// Wiring-level non-agreeing outcomes. They classify input resolution, not the
// comparison itself; every comparison outcome comes from ledger.Reconcile.
const (
	// reconcileUnconfigured: the manifest, the recipe's reconcile mapping, or a
	// config value the mapping selects is missing or unusable.
	reconcileUnconfigured = "unconfigured"
	// reconcileUnmappedEvent: no declared expectation covers the requested event,
	// so the comparison is pending/unreconciled.
	reconcileUnmappedEvent = "unmapped-event"
	// reconcileEventMismatch: the observation does not report the requested event.
	// The caller selecting an event never proves that event happened.
	reconcileEventMismatch = "event-mismatch"
	// reconcileInvalidObservation: the observation payload is missing, malformed,
	// or carries unknown policy-bearing fields.
	reconcileInvalidObservation = "observation-invalid"
)

// reconcileEventFacet is the neutral finding name used when the observation does
// not report the requested event.
const reconcileEventFacet = "event"

// manifestMaxAgeSeconds is the largest declared freshness window a time.Duration
// can hold: floor(MaxInt64 / int64(time.Second)) seconds. Converting a larger
// value would overflow, and a slightly smaller negative value is converted into a
// positive window by the same overflow, so both bounds are checked before any
// conversion happens.
const manifestMaxAgeSeconds = math.MaxInt64 / int64(time.Second)

// reconcileObservation is the closed observation payload the transport reports:
// the provider/scope/item identity, the observed-at stamp it saw, the event it
// observed, and the declared property values. Any other key is rejected, so a
// caller cannot smuggle expectations in through the payload.
type reconcileObservation struct {
	ProviderID string            `json:"provider_id"`
	Scope      string            `json:"scope"`
	ItemID     string            `json:"item_id"`
	ObservedAt string            `json:"observed_at"`
	Event      string            `json:"event"`
	Properties map[string]string `json:"properties"`
}

// reconcileConfig is the recipe-declared adaptation, read from the project
// manifest at [recipes.<id>.config.reconcile]. It selects config fields, never
// provider states: each expectation names an opaque property and the config field
// whose value it must show.
type reconcileConfig struct {
	ScopeField    string                 `json:"scope_field"`
	MaxAgeSeconds *int64                 `json:"max_age_seconds"`
	Expectations  []reconcileExpectation `json:"expectations"`
}

// reconcileExpectation is one declared requirement for one event.
type reconcileExpectation struct {
	Event       string `json:"event"`
	Property    string `json:"property"`
	ConfigField string `json:"config_field"`
}

// ledgerReconcileJSON is the reconciliation sidecar. It is emitted on stdout only
// when --reconcile was supplied, so an invocation without it keeps the exact
// legacy verdict bytes. requested_event is what the caller asked to compare and
// observed_event is what the transport reported: the two are never conflated, and
// the outcome never claims more than the declared properties showed.
type ledgerReconcileJSON struct {
	Outcome        string           `json:"outcome"`
	RequestedEvent string           `json:"requested_event"`
	ObservedEvent  string           `json:"observed_event"`
	ProviderID     string           `json:"provider_id"`
	Scope          string           `json:"scope"`
	ItemID         string           `json:"item_id"`
	ObservedAt     string           `json:"observed_at"`
	Findings       []ledger.Finding `json:"findings"`
	Detail         string           `json:"detail,omitempty"`
}

// reconcileSidecar resolves and grades one explicit comparison. The check order
// is fixed so one input always yields one outcome: observation, configuration,
// event mapping, observed event, then the declared properties.
func reconcileSidecar(root string, binding ledger.Binding, verdict ledger.Verdict, obsPath, event string, now time.Time) *ledgerReconcileJSON {
	out := &ledgerReconcileJSON{
		RequestedEvent: event,
		ProviderID:     binding.RecipeID,
		ItemID:         boundItemID(verdict.Item),
	}

	obs, err := loadReconcileObservation(obsPath)
	if err != nil {
		out.Outcome = reconcileInvalidObservation
		out.Detail = fmt.Sprintf("observation %s: %v", obsPath, err)
		return out
	}
	out.ObservedEvent = obs.Event
	out.ObservedAt = obs.ObservedAt

	scope, expected, maxAge, err := resolveReconcileMapping(root, binding.RecipeID, event)
	if err != nil {
		out.Outcome = reconcileUnconfigured
		out.Detail = err.Error()
		return out
	}
	out.Scope = scope
	if len(expected) == 0 {
		out.Outcome = reconcileUnmappedEvent
		out.Detail = fmt.Sprintf("recipe %q declares no expectation for event %q", binding.RecipeID, event)
		return out
	}
	if obs.Event != event {
		out.Outcome = reconcileEventMismatch
		out.Findings = []ledger.Finding{eventFinding(event, obs.Event)}
		return out
	}

	result := ledger.Reconcile(ledger.ReconcileInput{
		ProviderID: binding.RecipeID,
		Scope:      scope,
		ItemID:     out.ItemID,
		Expected:   expected,
		Observed: &ledger.Observation{
			ProviderID: obs.ProviderID,
			Scope:      obs.Scope,
			ItemID:     obs.ItemID,
			ObservedAt: obs.ObservedAt,
			Properties: obs.Properties,
		},
		Now:    now,
		MaxAge: maxAge,
	})
	out.Outcome = result.Outcome
	out.Findings = result.Findings
	return out
}

// eventFinding reports the observed event against the requested one, reusing the
// comparison grader's finding vocabulary: an absent event is missing, another
// event is different.
func eventFinding(requested, observed string) ledger.Finding {
	status := ledger.FindingDifferent
	if observed == "" {
		status = ledger.FindingMissing
	}
	return ledger.Finding{Property: reconcileEventFacet, Expected: requested, Observed: observed, Status: status}
}

// boundItemID is the native id of the item the grade bound, or "" when the grade
// selected none. A nil item means no safe target: the comparison then reports an
// unbound identity instead of borrowing an unrelated item. Binding a closed item
// after a merge is a known open question and is deliberately not guessed here.
func boundItemID(item *ledger.Item) string {
	if item == nil {
		return ""
	}
	return item.ItemID
}

// loadReconcileObservation reads the transport's observation. It is a trust
// boundary, so the payload is strict: unknown fields and trailing content are
// rejected rather than silently ignored, and a non-string property value is
// invalid input rather than a coerced comparison. Only a plain file is opened, and
// it is read under the acquisition size budget, so a FIFO, a device or an endless
// file cannot block or flood the comparison.
func loadReconcileObservation(path string) (reconcileObservation, error) {
	if err := regularFile(path); err != nil {
		return reconcileObservation{}, err
	}
	handle, err := os.Open(path)
	if err != nil {
		return reconcileObservation{}, err
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, acquisitionLimit+1))
	if err != nil {
		return reconcileObservation{}, err
	}
	if len(data) > acquisitionLimit {
		return reconcileObservation{}, fmt.Errorf("larger than %d bytes", acquisitionLimit)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var obs reconcileObservation
	if err := dec.Decode(&obs); err != nil {
		return reconcileObservation{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return reconcileObservation{}, errors.New("unexpected trailing content after the observation object")
	}
	return obs, nil
}

// resolveReconcileMapping reads the recipe-declared mapping from the project
// manifest and resolves one event's declared scope, expectations and freshness.
// An empty expectation list means the event is unmapped, which the caller reports
// as pending. An error means the configuration itself is unusable.
func resolveReconcileMapping(root, recipeID, event string) (string, []ledger.Expectation, time.Duration, error) {
	config, err := readManifestRecipeConfig(root, recipeID)
	if err != nil {
		return "", nil, 0, err
	}
	raw, ok := config["reconcile"]
	if !ok {
		return "", nil, 0, fmt.Errorf("recipe %q declares no reconcile mapping in %s", recipeID, manifestPath(root))
	}
	var declared reconcileConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&declared); err != nil {
		return "", nil, 0, fmt.Errorf("recipe %q reconcile mapping: %w", recipeID, err)
	}

	scope, ok := configValue(config, declared.ScopeField)
	if declared.ScopeField == "" || !ok {
		return "", nil, 0, fmt.Errorf("recipe %q reconcile mapping: scope config field %q has no value", recipeID, declared.ScopeField)
	}

	var expected []ledger.Expectation
	for _, decl := range declared.Expectations {
		if decl.Event != event {
			continue
		}
		if decl.Property == "" || decl.ConfigField == "" {
			return "", nil, 0, fmt.Errorf("recipe %q reconcile mapping: expectation for event %q needs property and config_field", recipeID, event)
		}
		value, ok := configValue(config, decl.ConfigField)
		if !ok {
			return "", nil, 0, fmt.Errorf("recipe %q reconcile mapping: config field %q has no value", recipeID, decl.ConfigField)
		}
		expected = append(expected, ledger.Expectation{Name: decl.Property, Value: value})
	}

	// No freshness default is invented: an undeclared window is forwarded as zero
	// so the comparison grader classifies invalid input. A declared window is
	// validated against the representable range before any conversion, so a
	// non-positive or oversized value can never wrap into a usable window.
	var maxAge time.Duration
	if declared.MaxAgeSeconds != nil {
		seconds := *declared.MaxAgeSeconds
		if seconds <= 0 || seconds > manifestMaxAgeSeconds {
			return "", nil, 0, fmt.Errorf("recipe %q reconcile mapping: max_age_seconds %d is outside 1..%d",
				recipeID, seconds, manifestMaxAgeSeconds)
		}
		maxAge = time.Duration(seconds) * time.Second
	}
	return scope, expected, maxAge, nil
}

// manifestPath is the project manifest location for one project root.
func manifestPath(root string) string {
	return filepath.Join(root, "ai-specs", "ai-specs.toml")
}

// manifestRecipe is one [recipes.<id>] table. Only the raw config table is kept:
// the reconcile mapping inside it is decoded separately and strictly. Enabled is
// a pointer so an explicit enabled = false is distinguishable from an absent key.
type manifestRecipe struct {
	Enabled *bool                      `json:"enabled"`
	Config  map[string]json.RawMessage `json:"config"`
}

// Bounded parser execution. The parser is a trust boundary and an availability
// risk: it may not run forever, return unbounded output, or have its raw failure
// output surfaced. The limits are fixed constants, not configuration: bounding an
// acquisition subprocess is acquisition safety, not a new grader input.
const (
	manifestParseTimeout = 3 * time.Second
	// acquisitionLimit is the one size budget for acquisition in and out: the
	// parser's stdout and the observation payload share it.
	acquisitionLimit   = 1 << 20
	manifestErrorLimit = 4 << 10
)

// manifestReader is the whole TOML boundary: the Python standard library parser
// deserializes the manifest and prints its [recipes] table as JSON. It selects
// nothing — no field, no value, no comparison — so every mapping decision stays in
// Go and no TOML parser (hand-written subset or new Go dependency) is introduced.
// This mirrors the existing acquisition seam lib/_internal/toml-read.py, which is
// not reachable by path from the distributed binary.
const manifestReader = `import json, sys, tomllib
with open(sys.argv[1], "rb") as handle:
    data = tomllib.load(handle)
recipes = data.get("recipes")
print(json.dumps(recipes if isinstance(recipes, dict) else {}))
`

// cappedBuffer buffers at most max bytes and records that anything more arrived.
// It always reports a full write so a verbose child sees a healthy pipe instead of
// a short-write error, and never grows past max.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) <= room {
			w.buf.Write(p)
			return len(p), nil
		}
		w.buf.Write(p[:room])
	}
	w.truncated = true
	return len(p), nil
}

// readManifestRecipeConfig loads the bound recipe's config table from the project
// manifest through the standard parser. Every failure is returned so the caller
// can report it: a comparison with unreadable configuration never agrees, and a
// recipe the manifest explicitly disabled grants no authority at all.
func readManifestRecipeConfig(root, recipeID string) (map[string]json.RawMessage, error) {
	if recipeID == "" {
		return nil, errors.New("no tracker recipe is bound (missing or unbound witness)")
	}
	manifest := manifestPath(root)
	recipes, err := readManifestRecipes(manifest)
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", manifest, err)
	}
	recipe, ok := recipes[recipeID]
	if !ok {
		return nil, fmt.Errorf("manifest %s: recipe %q is not configured", manifest, recipeID)
	}
	if recipe.Enabled != nil && !*recipe.Enabled {
		return nil, fmt.Errorf("manifest %s: recipe %q is disabled", manifest, recipeID)
	}
	if recipe.Config == nil {
		return nil, fmt.Errorf("manifest %s: recipe %q has no config table", manifest, recipeID)
	}
	return recipe.Config, nil
}

// readManifestRecipes deserializes the manifest's [recipes] table. Provider
// adaptation lives in the manifest values, so this function is provider-neutral:
// it returns raw tables for the caller to decode.
func readManifestRecipes(path string) (map[string]manifestRecipe, error) {
	if err := regularFile(path); err != nil {
		return nil, err
	}
	root := filepath.Dir(filepath.Dir(path))
	out, err := runManifestParser(root, path)
	if err != nil {
		return nil, err
	}
	var recipes map[string]manifestRecipe
	if err := json.Unmarshal(out, &recipes); err != nil {
		return nil, fmt.Errorf("deserialized recipes: %w", err)
	}
	return recipes, nil
}

// runManifestParser runs the standard TOML parser under bounded execution: a
// deadline, a bounded I/O drain, capped stdout and capped stderr. A parser that
// hangs, floods or fails is reported through classifyManifestParserError, never by
// echoing its output. WaitDelay bounds the wait for the output pipes, so a child
// that leaves a helper holding them cannot extend the deadline: the whole call is
// bounded by twice manifestParseTimeout.
//
// The interpreter is resolved explicitly and then run isolated (-I -B): isolated
// mode ignores PYTHON* environment variables and user site-packages, so neither a
// repository-controlled nor an ambient module can shadow the stdlib imports the
// reader performs (R1-001). An interpreter resolving inside the project root is
// refused: the manifest must never select the code that parses it.
func runManifestParser(root, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestParseTimeout)
	defer cancel()

	interpreter, err := manifestInterpreter(root)
	if err != nil {
		return nil, err
	}
	stdout := &cappedBuffer{max: acquisitionLimit}
	stderr := &cappedBuffer{max: manifestErrorLimit}
	cmd := exec.CommandContext(ctx, interpreter, "-I", "-B", "-c", manifestReader, path)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = manifestParseTimeout

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("standard TOML parser timed out after %s", manifestParseTimeout)
	}
	if runErr != nil {
		return nil, classifyManifestParserError(runErr)
	}
	if stdout.truncated {
		return nil, fmt.Errorf("standard TOML parser returned more than %d bytes", acquisitionLimit)
	}
	return stdout.buf.Bytes(), nil
}

// manifestInterpreter resolves the ambient python3 without executing it and
// refuses an interpreter that lives inside the project root, where repository
// content could shadow the system interpreter. Resolution failures are named so
// a missing interpreter and a suspicious one are distinguishable without leaking
// filesystem detail beyond the resolved path class.
func manifestInterpreter(root string) (string, error) {
	interpreter, err := exec.LookPath("python3")
	if err != nil {
		return "", fmt.Errorf("standard TOML parser is unavailable: %v", err)
	}
	if abs, absErr := filepath.Abs(interpreter); absErr == nil {
		if absRoot, rootErr := filepath.Abs(root); rootErr == nil && (absRoot == abs || strings.HasPrefix(abs+string(filepath.Separator), absRoot)) {
			return "", errors.New("standard TOML parser interpreter resolves inside the project root")
		}
	}
	return interpreter, nil
}

// classifyManifestParserError names the failure without reproducing parser
// output: a missing interpreter and a rejected manifest are different problems,
// and neither message carries raw stderr, a traceback, or configuration content.
// stderr is captured only to keep the child from blocking on a full pipe.
func classifyManifestParserError(err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return fmt.Errorf("standard TOML parser is unavailable: %v", execErr.Err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("the manifest is not valid TOML (parser exit status %d)", exitErr.ExitCode())
	}
	return errors.New("standard TOML parser could not be started")
}

// regularFile refuses anything that is not a plain file before it is opened: a
// read of a FIFO or device would block acquisition, and a directory cannot hold
// this content. The check is a cheapest-first guard, not a security boundary.
func regularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file (%s)", info.Mode().Type())
	}
	return nil
}

// configValue resolves one configured string. A missing field or a non-string
// value is unusable config: it is reported, never defaulted or coerced.
func configValue(config map[string]json.RawMessage, field string) (string, bool) {
	raw, ok := config[field]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}
