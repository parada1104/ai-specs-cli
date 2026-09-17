package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// The reconciliation tests go through the CLI deliberately: the wiring this file
// pins is Go-resolved witness/item binding, recipe-declared configuration read
// from the project manifest, and one optional stdout sidecar that never changes
// the graded exit code.

// reconcileRecipeID is the fixture's bound provider id (the witness recipe id).
const reconcileRecipeID = "trello-mcp-workflow"

// reconcileManifestBody is the project-manifest fixture: the config values the
// recipe declares, plus the declared reconcile mapping that selects them.
const reconcileManifestBody = `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
default_list = "In Progress"
[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900
[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "delivery"
property = "list"
config_field = "default_list"
`

// writeReconcileManifest writes the project manifest at the design path.
func writeReconcileManifest(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeReconcileObs writes one raw observation payload and returns its path.
func writeReconcileObs(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "observation.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// reconcileObsBody builds a valid observation payload with mutate applied.
func reconcileObsBody(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	obs := map[string]any{
		"provider_id": reconcileRecipeID,
		"scope":       "board-1",
		"item_id":     "card-1",
		"observed_at": time.Now().UTC().Format(time.RFC3339),
		"event":       "delivery",
		"properties":  map[string]string{"list": "In Progress"},
	}
	if mutate != nil {
		mutate(obs)
	}
	body, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// reconcileFixture is a bound repo with one open item and a declared reconcile
// mapping, so a comparison has everything it needs except the observation.
func reconcileFixture(t *testing.T) string {
	t.Helper()
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	saveLedgerStore(t, common, branch, "card-1", nil)
	writeReconcileManifest(t, dir, reconcileManifestBody)
	return dir
}

// reconcileRunArgs is the argv fragment for one explicit comparison at
// apply-start, so other tests can extend the same invocation.
func reconcileRunArgs(dir, obsPath, event string) []string {
	return []string{
		"--ledger", "--checkpoint", "apply-start", "--project-root", dir,
		"--reconcile", obsPath, "--reconcile-event", event,
	}
}

// reconcileRun runs one explicit comparison at apply-start.
func reconcileRun(t *testing.T, dir, obsPath, event string, extra ...string) (int, string, string) {
	t.Helper()
	args := append(reconcileRunArgs(dir, obsPath, event), extra...)
	return runCLI(t, args...)
}

// reconcileSidecar extracts the reconcile sidecar from a ledger verdict.
func reconcileSidecarOf(t *testing.T, stdout string) map[string]any {
	t.Helper()
	out := decodeLedgerOut(t, stdout)
	side, ok := out["reconcile"].(map[string]any)
	if !ok {
		t.Fatalf("reconcile sidecar = %v, want an object; stdout: %s", out["reconcile"], stdout)
	}
	return side
}

// TestLedgerReconcileAgreesOnDeclaredPropertiesViaCLI pins the happy path: the
// observation matches the bound identity, the recipe-declared scope and every
// declared expectation, and the sidecar reports it without touching the store.
func TestLedgerReconcileSkillObservationShapeIsCurrent(t *testing.T) {
	// The skill is the producer contract: the observation payload keys it
	// documents must be exactly the keys the gate accepts, so a skill edit
	// and a gate change cannot drift apart silently.
	skillPath := filepath.Join("..", "..", "trello-mcp-workflow",
		"skills", "trello-mcp-workflow", "SKILL.md")
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read skill contract: %v", err)
	}
	const fence = "```json"
	start := strings.Index(string(raw), fence)
	if start < 0 {
		t.Fatal("skill carries no json payload example")
	}
	end := strings.Index(string(raw[start+len(fence):]), "```")
	if end < 0 {
		t.Fatal("json payload example is unterminated")
	}
	documented := map[string]bool{}
	var example map[string]any
	if err := json.Unmarshal([]byte(string(raw)[start+len(fence):start+len(fence)+end]), &example); err != nil {
		t.Fatalf("skill payload example is not valid json: %v", err)
	}
	for key := range example {
		documented[key] = true
	}
	accepted := map[string]bool{}
	typ := reflect.TypeOf(reconcileObservation{})
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" {
			accepted[tag] = true
		}
	}
	for key := range accepted {
		if !documented[key] {
			t.Fatalf("gate accepts key %q that the skill payload does not document", key)
		}
	}
	for key := range documented {
		if !accepted[key] {
			t.Fatalf("skill documents key %q that the gate rejects as unknown", key)
		}
	}
}

func TestLedgerReconcileAgreesOnDeclaredPropertiesViaCLI(t *testing.T) {
	dir := reconcileFixture(t)
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))
	storePath := ledger.StorePath(RealPath(gitCommon(dir)))
	before := ledgerStoreBytes(t, storePath)

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("agree exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	assertKeys(t, "reconcile", side, "outcome", "requested_event", "observed_event",
		"provider_id", "scope", "item_id", "observed_at", "findings")
	if side["outcome"] != "agree" {
		t.Fatalf("outcome = %v, want agree; sidecar: %v", side["outcome"], side)
	}
	if side["findings"] != nil {
		t.Fatalf("findings = %v, want null on agreement", side["findings"])
	}
	if side["requested_event"] != "delivery" || side["observed_event"] != "delivery" {
		t.Fatalf("events = %v/%v, want delivery/delivery", side["requested_event"], side["observed_event"])
	}
	if side["provider_id"] != reconcileRecipeID || side["scope"] != "board-1" || side["item_id"] != "card-1" {
		t.Fatalf("identity = %v/%v/%v, want the resolved binding", side["provider_id"], side["scope"], side["item_id"])
	}
	if side["observed_at"] == "" {
		t.Fatalf("observed_at = %v, want the reported stamp echoed", side["observed_at"])
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("reconciliation must not write the store")
	}
}

// TestLedgerReconcileWrongScopeIsIdentityMismatch pins that an observation for
// another provider scope never agrees, whatever the declared properties say.
func TestLedgerReconcileWrongScopeIsIdentityMismatch(t *testing.T) {
	dir := reconcileFixture(t)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["scope"] = "board-2"
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("wrong scope exit = %d, want 0 (the verdict decides the code); stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileIdentityMismatch {
		t.Fatalf("outcome = %v, want %s", side["outcome"], ledger.ReconcileIdentityMismatch)
	}
	findings, _ := side["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want exactly the scope facet", side["findings"])
	}
	finding := findings[0].(map[string]any)
	if finding["property"] != "scope" || finding["status"] != "different" {
		t.Fatalf("finding = %v, want scope/different", finding)
	}
}

// TestLedgerReconcileUnmappedEventIsPendingNotGuessed pins the explicit
// pending/unreconciled path: an event the recipe never declared is reported,
// never silently satisfied by another event's expectations.
func TestLedgerReconcileUnmappedEventIsPendingNotGuessed(t *testing.T) {
	dir := reconcileFixture(t)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = "release"
	}))

	for _, event := range []string{"release", ""} {
		code, stdout, stderr := reconcileRun(t, dir, obs, event)
		if code != 0 {
			t.Fatalf("event %q exit = %d, want 0; stderr: %s", event, code, stderr)
		}
		side := reconcileSidecarOf(t, stdout)
		if side["outcome"] != "unmapped-event" {
			t.Fatalf("event %q outcome = %v, want unmapped-event", event, side["outcome"])
		}
		if detail, _ := side["detail"].(string); detail == "" {
			t.Fatalf("event %q detail = empty, want the unmapped mapping named", event)
		}
	}
}

// TestLedgerReconcileEventMismatchIsNotDeliveryEvidence pins the honesty rule: a
// comparison the caller requested is never reported as observed just because it was
// asked for. The requested event is a mapped one, but the observation reports
// another event, so no property agreement may be claimed for it.
func TestLedgerReconcileEventMismatchIsNotDeliveryEvidence(t *testing.T) {
	dir := reconcileFixture(t)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = "merge"
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "event-mismatch" {
		t.Fatalf("outcome = %v, want event-mismatch; sidecar: %v", side["outcome"], side)
	}
	if side["requested_event"] != "delivery" || side["observed_event"] != "merge" {
		t.Fatalf("events = %v/%v, want delivery/merge", side["requested_event"], side["observed_event"])
	}
	findings, _ := side["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want exactly the event facet", side["findings"])
	}
	if finding := findings[0].(map[string]any); finding["property"] != "event" || finding["status"] != "different" {
		t.Fatalf("finding = %v, want event/different", finding)
	}
	if side["outcome"] == ledger.ReconcileAgree {
		t.Fatal("a selected event must never be reported as an observed one")
	}
}

// TestLedgerReconcileUnconfiguredIsExplicitNonAgree pins that missing mapping or
// missing config values are reported as unconfigured, never defaulted.
func TestLedgerReconcileUnconfiguredIsExplicitNonAgree(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		detail string
	}{
		{
			name: "no reconcile mapping declared",
			body: `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
default_list = "In Progress"
`,
			detail: "reconcile",
		},
		{
			name:   "scope field names no configured value",
			body:   strings.Replace(reconcileManifestBody, "board_id = \"board-1\"\n", "", 1),
			detail: "board_id",
		},
		{
			name:   "expectation names no configured value",
			body:   strings.Replace(reconcileManifestBody, "default_list = \"In Progress\"\n", "", 1),
			detail: "default_list",
		},
		{
			// Tracker-domain policy (ledger_mode/gate_mode) is not adapter data. The
			// mapping surface is closed, so a policy key smuggled into it is rejected
			// rather than read as a mapping field or silently ignored.
			name:   "policy key inside the adapter mapping",
			body:   strings.Replace(reconcileManifestBody, "scope_field = \"board_id\"", "scope_field = \"board_id\"\nledger_mode = \"always\"", 1),
			detail: "reconcile",
		},
		{
			name:   "scope field holds a non-string value",
			body:   strings.Replace(reconcileManifestBody, `board_id = "board-1"`, "board_id = 42", 1),
			detail: "board_id",
		},
		{
			name:   "unparsable manifest",
			body:   "this is not toml =\n",
			detail: "manifest",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, branch := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", reconcileRecipeID)
			saveLedgerStore(t, common, branch, "card-1", nil)
			writeReconcileManifest(t, dir, tc.body)
			obs := writeReconcileObs(t, reconcileObsBody(t, nil))

			code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
			}
			side := reconcileSidecarOf(t, stdout)
			if side["outcome"] != "unconfigured" {
				t.Fatalf("outcome = %v, want unconfigured; sidecar: %v", side["outcome"], side)
			}
			detail, _ := side["detail"].(string)
			if !strings.Contains(detail, tc.detail) {
				t.Fatalf("detail = %q, want it to name %q", detail, tc.detail)
			}
		})
	}
}

// TestLedgerReconcileUnboundWithoutBoundProvider pins that a dormant witness
// cannot supply a provider identity, so the comparison is unconfigured rather
// than guessed.
func TestLedgerReconcileUnboundWithoutBoundProvider(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "unbound", "")
	saveLedgerStore(t, common, branch, "card-1", nil)
	writeReconcileManifest(t, dir, reconcileManifestBody)
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("dormant exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "unconfigured" {
		t.Fatalf("outcome = %v, want unconfigured", side["outcome"])
	}
	if side["provider_id"] != "" {
		t.Fatalf("provider_id = %v, want empty without a bound recipe", side["provider_id"])
	}
	// A dormant grade selects no item at all, so no target may be implied.
	if side["item_id"] != "" {
		t.Fatalf("item_id = %v, want empty when no item was graded", side["item_id"])
	}
}

// TestLedgerReconcileExpectationsFollowConfiguration pins that expected values are
// configuration values, not constants in Go: changing the configured list changes
// both the agreement and the expected value the finding reports.
func TestLedgerReconcileExpectationsFollowConfiguration(t *testing.T) {
	const moved = "Ready to Deploy"
	body := strings.Replace(reconcileManifestBody, `default_list = "In Progress"`, `default_list = "`+moved+`"`, 1)

	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	saveLedgerStore(t, common, branch, "card-1", nil)
	writeReconcileManifest(t, dir, body)

	agreed := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["properties"] = map[string]string{"list": moved}
	}))
	code, stdout, stderr := reconcileRun(t, dir, agreed, "delivery")
	if code != 0 {
		t.Fatalf("agree exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := reconcileSidecarOf(t, stdout); side["outcome"] != ledger.ReconcileAgree {
		t.Fatalf("outcome = %v, want agree for the configured value", side["outcome"])
	}

	stale := writeReconcileObs(t, reconcileObsBody(t, nil))
	code, stdout, _ = reconcileRun(t, dir, stale, "delivery")
	if code != 0 {
		t.Fatalf("mismatch exit = %d, want 0", code)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcilePropertyMismatch {
		t.Fatalf("outcome = %v, want %s", side["outcome"], ledger.ReconcilePropertyMismatch)
	}
	findings, _ := side["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want the single declared property", side["findings"])
	}
	if finding := findings[0].(map[string]any); finding["expected"] != moved {
		t.Fatalf("finding = %v, want the configured expected value", finding)
	}
}

// TestLedgerReconcileNonProviderAdapterFeedsTheSameComparator pins the Tracker
// domain port boundary at the CLI: a recipe that names no provider drives the very
// same neutral expectation/observation comparator by declaring a mapping alone.
// Go hardcodes no provider, config field, or property name, so the adapter is
// purely declarative data and the core stays provider-neutral.
func TestLedgerReconcileNonProviderAdapterFeedsTheSameComparator(t *testing.T) {
	const adapterRecipeID = "fixture-tracker"
	const adapterManifest = `[recipes.fixture-tracker]
enabled = true
[recipes.fixture-tracker.config]
workspace_id = "workspace-1"
shipped_status = "Shipped"
[recipes.fixture-tracker.config.reconcile]
scope_field = "workspace_id"
max_age_seconds = 900
[[recipes.fixture-tracker.config.reconcile.expectations]]
event = "delivery"
property = "status"
config_field = "shipped_status"
`
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", adapterRecipeID)
	saveLedgerStore(t, common, branch, "item-1", nil)
	writeReconcileManifest(t, dir, adapterManifest)

	// The observation carries the adapter's neutral property, never a provider's.
	observation := func(status string) string {
		body, err := json.Marshal(map[string]any{
			"provider_id": adapterRecipeID,
			"scope":       "workspace-1",
			"item_id":     "item-1",
			"observed_at": time.Now().UTC().Format(time.RFC3339),
			"event":       "delivery",
			"properties":  map[string]string{"status": status},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	code, stdout, stderr := reconcileRun(t, dir, writeReconcileObs(t, observation("Shipped")), "delivery")
	if code != 0 {
		t.Fatalf("adapter agree exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileAgree {
		t.Fatalf("outcome = %v, want agree for the declarative adapter; sidecar: %v", side["outcome"], side)
	}
	if side["provider_id"] != adapterRecipeID || side["scope"] != "workspace-1" || side["item_id"] != "item-1" {
		t.Fatalf("identity = %v/%v/%v, want the adapter binding", side["provider_id"], side["scope"], side["item_id"])
	}

	code, stdout, _ = reconcileRun(t, dir, writeReconcileObs(t, observation("Backlog")), "delivery")
	if code != 0 {
		t.Fatalf("adapter mismatch exit = %d, want 0", code)
	}
	side = reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcilePropertyMismatch {
		t.Fatalf("outcome = %v, want %s for the adapter's configured value", side["outcome"], ledger.ReconcilePropertyMismatch)
	}
	if findings, _ := side["findings"].([]any); len(findings) != 1 {
		t.Fatalf("findings = %v, want the single adapter-declared property", side["findings"])
	}
}

// TestLedgerReconcileUnobservedEventIsNeverAgreement pins the second half of the
// honesty rule: an observation that reports no event cannot confirm the requested
// one.
func TestLedgerReconcileUnobservedEventIsNeverAgreement(t *testing.T) {
	dir := reconcileFixture(t)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = ""
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "event-mismatch" {
		t.Fatalf("outcome = %v, want event-mismatch", side["outcome"])
	}
	findings, _ := side["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want the event facet", side["findings"])
	}
	if finding := findings[0].(map[string]any); finding["status"] != "missing" {
		t.Fatalf("finding = %v, want missing for an unreported event", finding)
	}
}

// TestLedgerReconcileInvalidObservationIsRejected pins strict observation
// input: unknown policy-bearing keys, malformed JSON and a missing file are
// rejected as invalid input rather than silently accepted.
func TestLedgerReconcileInvalidObservationIsRejected(t *testing.T) {
	dir := reconcileFixture(t)
	cases := []struct {
		name string
		body string
	}{
		{"unknown policy-bearing field", `{"provider_id":"trello-mcp-workflow","scope":"board-1","item_id":"card-1","observed_at":"2026-09-13T12:00:00Z","event":"delivery","properties":{"list":"In Progress"},"expected":[{"name":"list","value":"In Progress"}]}`},
		{"malformed json", `{"provider_id":`},
		{"wrong property value type", `{"provider_id":"trello-mcp-workflow","scope":"board-1","item_id":"card-1","observed_at":"2026-09-13T12:00:00Z","event":"delivery","properties":{"list":42}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := writeReconcileObs(t, tc.body)
			code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
			}
			if side := reconcileSidecarOf(t, stdout); side["outcome"] != "observation-invalid" {
				t.Fatalf("outcome = %v, want observation-invalid", side["outcome"])
			}
		})
	}

	t.Run("missing observation file", func(t *testing.T) {
		code, stdout, _ := reconcileRun(t, dir, filepath.Join(t.TempDir(), "absent.json"), "delivery")
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if side := reconcileSidecarOf(t, stdout); side["outcome"] != "observation-invalid" {
			t.Fatalf("outcome = %v, want observation-invalid", side["outcome"])
		}
	})
}

// TestLedgerReconcileFreshnessIsDeclaredNeverDefaulted pins that a missing,
// stale or future-dated observation never agrees, and that an undeclared
// freshness window is invalid input rather than a live default.
func TestLedgerReconcileFreshnessIsDeclaredNeverDefaulted(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		at      time.Time
		outcome string
	}{
		{"stale observation", strings.Replace(reconcileManifestBody, "max_age_seconds = 900", "max_age_seconds = 60", 1), time.Now().Add(-time.Hour), ledger.ReconcileStaleObservation},
		{"future observation", reconcileManifestBody, time.Now().Add(time.Hour), ledger.ReconcileFutureObservedAt},
		{"undeclared window", strings.Replace(reconcileManifestBody, "max_age_seconds = 900\n", "", 1), time.Now(), ledger.ReconcileInvalidWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, branch := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", reconcileRecipeID)
			saveLedgerStore(t, common, branch, "card-1", nil)
			writeReconcileManifest(t, dir, tc.body)
			obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
				o["observed_at"] = tc.at.UTC().Format(time.RFC3339)
			}))

			code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
			}
			if side := reconcileSidecarOf(t, stdout); side["outcome"] != tc.outcome {
				t.Fatalf("outcome = %v, want %s", side["outcome"], tc.outcome)
			}
		})
	}
}

// TestLedgerReconcileUnboundItemNeverGuesses pins that a comparison with no
// safely bound item is unbound-identity, and that the sidecar rides along with
// the graded exit code instead of changing it.
func TestLedgerReconcileUnboundItemNeverGuesses(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	writeReconcileManifest(t, dir, reconcileManifestBody)
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery", "--ledger-mode", "always")
	if code != 2 {
		t.Fatalf("blocking grade exit = %d, want 2 (the grade decides); stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileUnboundIdentity {
		t.Fatalf("outcome = %v, want %s", side["outcome"], ledger.ReconcileUnboundIdentity)
	}
	if side["item_id"] != "" {
		t.Fatalf("item_id = %v, want empty when no item is bound", side["item_id"])
	}
	if ledgerStoreBytes(t, ledger.StorePath(RealPath(gitCommon(dir)))) != nil {
		t.Fatal("reconciliation must not create a store")
	}
}

// TestLedgerReconcilePreservesLegacyStdoutWhenNotRequested pins the sidecar
// contract: an invocation without --reconcile emits the exact legacy verdict,
// byte for byte, even when a mapping is configured.
func TestLedgerReconcilePreservesLegacyStdoutWhenNotRequested(t *testing.T) {
	dir := reconcileFixture(t)

	code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--ledger-mode", "warn", "--project-root", dir)
	if code != 0 {
		t.Fatalf("legacy exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	assertKeys(t, "verdict", out, "capability", "active", "checkpoint", "mode", "decision",
		"reason", "identity", "item", "conflict", "prompt", "doctor")
	if strings.Contains(stdout, "reconcile") {
		t.Fatalf("legacy stdout leaked a reconcile sidecar: %s", stdout)
	}
}

// TestLedgerReconcileRejectsDisabledRecipe pins F6: an explicit enabled = false
// withdraws the recipe, so a witness that still names it is stale and grants no
// authority. Without the guard the fixture below would agree.
func TestLedgerReconcileRejectsDisabledRecipe(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	storePath := saveLedgerStore(t, common, branch, "card-1", nil)
	before := ledgerStoreBytes(t, storePath)
	writeReconcileManifest(t, dir, strings.Replace(reconcileManifestBody, "enabled = true", "enabled = false", 1))
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("disabled recipe exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "unconfigured" {
		t.Fatalf("outcome = %v, want unconfigured for a disabled recipe; sidecar: %v", side["outcome"], side)
	}
	detail, _ := side["detail"].(string)
	if !strings.Contains(detail, "disabled") {
		t.Fatalf("detail = %q, want it to name the disabled recipe", detail)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("a rejected comparison must not write the store")
	}
}

// TestLedgerReconcileFreshnessWindowIsBounded pins F1: a declared window that is
// not positive, or wider than a time.Duration can represent, is unusable config
// rejected before any conversion. Values just below the negated bound would wrap
// into a positive window if they were converted first.
func TestLedgerReconcileFreshnessWindowIsBounded(t *testing.T) {
	if manifestMaxAgeSeconds != 9223372036 {
		t.Fatalf("manifestMaxAgeSeconds = %d, want floor(MaxInt64/int64(time.Second)) = 9223372036", manifestMaxAgeSeconds)
	}
	cases := []struct {
		name  string
		value string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"negative that wraps positive", "-9223372037"},
		{"extreme negative", "-9223372036854775808"},
		{"one above the maximum", "9223372037"},
		{"enormous", "99999999999999999999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, branch := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", reconcileRecipeID)
			saveLedgerStore(t, common, branch, "card-1", nil)
			writeReconcileManifest(t, dir, strings.Replace(reconcileManifestBody, "max_age_seconds = 900", "max_age_seconds = "+tc.value, 1))
			obs := writeReconcileObs(t, reconcileObsBody(t, nil))

			code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
			}
			side := reconcileSidecarOf(t, stdout)
			if side["outcome"] != "unconfigured" {
				t.Fatalf("max_age_seconds = %s outcome = %v, want unconfigured; sidecar: %v", tc.value, side["outcome"], side)
			}
			detail, _ := side["detail"].(string)
			if !strings.Contains(detail, "max_age_seconds") {
				t.Fatalf("detail = %q, want it to name the rejected field", detail)
			}
		})
	}

	t.Run("exact maximum is accepted", func(t *testing.T) {
		dir, common, branch := ledgerRepo(t)
		writeLedgerWitness(t, common, "bound", reconcileRecipeID)
		saveLedgerStore(t, common, branch, "card-1", nil)
		writeReconcileManifest(t, dir, strings.Replace(reconcileManifestBody, "max_age_seconds = 900", "max_age_seconds = 9223372036", 1))
		obs := writeReconcileObs(t, reconcileObsBody(t, nil))

		code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
		}
		side := reconcileSidecarOf(t, stdout)
		if side["outcome"] != ledger.ReconcileAgree {
			t.Fatalf("outcome = %v, want agree at the inclusive maximum; sidecar: %v", side["outcome"], side)
		}
	})
}

// TestLedgerReconcileRejectsNonRegularManifest pins F5's guard: a manifest that
// is not a plain file is refused before it is opened, so a FIFO or device cannot
// block acquisition.
func TestLedgerReconcileRejectsNonRegularManifest(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	saveLedgerStore(t, common, branch, "card-1", nil)
	if err := os.MkdirAll(manifestPath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "unconfigured" {
		t.Fatalf("outcome = %v, want unconfigured for a non-regular manifest", side["outcome"])
	}
	detail, _ := side["detail"].(string)
	if !strings.Contains(detail, "not a regular file") {
		t.Fatalf("detail = %q, want it to name the file type problem", detail)
	}
}

// fakeManifestParser shadows python3 on PATH with one shell script, so the
// parser's failure modes are exercised without touching the real interpreter.
func fakeManifestParser(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeManifestParserInDir shadows python3 on PATH with one shell script written
// into dir (used to place the interpreter inside the project root for R1-001).
func fakeManifestParserInDir(t *testing.T, dir, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestLedgerReconcileIsolatesTheManifestInterpreter pins R1-001: the manifest
// parser runs the ambient interpreter isolated and never an interpreter the
// repository itself supplies, so repository-controlled or ambient module
// shadowing cannot execute arbitrary code during --reconcile.
func TestLedgerReconcileIsolatesTheManifestInterpreter(t *testing.T) {
	t.Run("interpreter inside the project root is refused", func(t *testing.T) {
		root := t.TempDir()
		fakeManifestParserInDir(t, root, "echo '{}'\n")
		_, err := readManifestRecipes(writeReconcileManifest(t, root, reconcileManifestBody))
		if err == nil || !strings.Contains(err.Error(), "resolves inside the project root") {
			t.Fatalf("error = %v, want an in-root interpreter refusal", err)
		}
	})

	t.Run("parser runs isolated (-I -B)", func(t *testing.T) {
		fakeManifestParser(t, `if [ "$1" = "-I" ] && [ "$2" = "-B" ]; then echo '{"recipes":{}}'; else exit 9; fi`)
		recipes, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		if err != nil {
			t.Fatalf("readManifestRecipes: %v, want an isolated parser invocation", err)
		}
		if _, ok := recipes["recipes"]; !ok {
			t.Fatalf("recipes = %v, want the parsed manifest table shape", recipes)
		}
	})

	t.Run("ambient PYTHONPATH cannot shadow stdlib imports", func(t *testing.T) {
		shadow := t.TempDir()
		if err := os.WriteFile(filepath.Join(shadow, "json.py"), []byte("raise RuntimeError('shadowed')\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PYTHONPATH", shadow)
		recipes, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		if err != nil {
			t.Fatalf("readManifestRecipes: %v, want isolation to ignore the shadowing module", err)
		}
		if _, ok := recipes["trello-mcp-workflow"]; !ok {
			t.Fatalf("recipes = %v, want the parsed recipe table", recipes)
		}
	})
}

// TestLedgerReconcileBoundsTheManifestParser pins F5: the parser subprocess is
// bounded in time and output, and its failure is reported as bounded detail
// rather than raw parser output.
func TestLedgerReconcileBoundsTheManifestParser(t *testing.T) {
	t.Run("hung parser is killed", func(t *testing.T) {
		fakeManifestParser(t, "/bin/sleep 30\n")
		start := time.Now()
		_, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		elapsed := time.Since(start)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error = %v, want a timed-out parser failure", err)
		}
		// The documented bound is the deadline plus the I/O drain; anything near
		// the child's own 30s runtime means the execution was not bounded.
		if elapsed >= 2*manifestParseTimeout+time.Second {
			t.Fatalf("elapsed = %s, want the call bounded by 2x%s", elapsed, manifestParseTimeout)
		}
	})

	t.Run("oversized stdout is refused", func(t *testing.T) {
		fakeManifestParser(t, "dd if=/dev/zero bs=1024 count=2048 2>/dev/null\n")
		_, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		if err == nil || !strings.Contains(err.Error(), "more than") {
			t.Fatalf("error = %v, want a bounded-output failure", err)
		}
	})

	t.Run("failing parser leaks no output", func(t *testing.T) {
		fakeManifestParser(t, "echo 'Traceback (most recent call last):' >&2\necho 'board_id = \"secret-board\"' >&2\nexit 1\n")
		_, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		if err == nil {
			t.Fatal("error = nil, want the parser failure reported")
		}
		if !strings.Contains(err.Error(), "not valid TOML") {
			t.Fatalf("error = %v, want an actionable invalid-manifest message", err)
		}
		for _, leak := range []string{"Traceback", "secret-board", "board_id", "\n"} {
			if strings.Contains(err.Error(), leak) {
				t.Fatalf("error = %q, want no leaked parser output (%q)", err, leak)
			}
		}
	})

	t.Run("missing parser is named", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := readManifestRecipes(writeReconcileManifest(t, t.TempDir(), reconcileManifestBody))
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("error = %v, want a missing-parser failure", err)
		}
	})
}

// TestLedgerReconcileDoesNotRecordConflict pins that the documented read-only
// contract covers the whole path, not just the sidecar: a conflicting grade that
// would normally persist a snapshot must persist nothing when --reconcile is
// requested, and the legacy grade must still record it.
func TestLedgerReconcileDoesNotRecordConflict(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	storePath := saveLedgerStore(t, common, branch, "card-1", nil)
	writeReconcileManifest(t, dir, reconcileManifestBody)
	evidence := writeLedgerEvidence(t, map[string]string{"remote": "card-remote"})
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))
	key := ledger.IdentityKey(common, branch, "")
	base := []string{"--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask", "--project-root", dir, "--evidence", evidence}

	before := ledgerStoreBytes(t, storePath)
	args := append(append([]string{}, base...), "--reconcile", obs, "--reconcile-event", "delivery")
	code, stdout, stderr := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("reconcile exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := reconcileSidecarOf(t, stdout); side["outcome"] != ledger.ReconcileAgree {
		t.Fatalf("outcome = %v, want the comparison to still run; sidecar: %v", side["outcome"], side)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("--reconcile must not write the store, including the conflict snapshot")
	}
	if snapshot := recordedConflict(t, storePath, key); snapshot != nil {
		t.Fatalf("snapshot = %+v, want none recorded by a --reconcile invocation", snapshot)
	}
	if !strings.Contains(stderr, "conflict") {
		t.Fatalf("stderr = %q, want the suppressed conflict recording reported", stderr)
	}

	t.Run("legacy grade still records it", func(t *testing.T) {
		if code, _, stderr := runCLI(t, base...); code != 0 {
			t.Fatalf("legacy exit = %d, want 0; stderr: %s", code, stderr)
		}
		if snapshot := recordedConflict(t, storePath, key); snapshot == nil {
			t.Fatal("a plain grade must still record the conflict snapshot")
		}
	})
}

// TestLedgerReconcileObservationSizeIsBounded pins the same-class read bound: an
// oversized payload is refused as invalid input, even when it would otherwise be a
// valid, agreeing observation.
func TestLedgerReconcileObservationSizeIsBounded(t *testing.T) {
	dir := reconcileFixture(t)
	body := reconcileObsBody(t, func(o map[string]any) {
		o["properties"] = map[string]string{"list": "In Progress", "pad": strings.Repeat("x", 2*acquisitionLimit)}
	})
	if len(body) <= acquisitionLimit {
		t.Fatalf("fixture is %d bytes, want it above the %d byte bound", len(body), acquisitionLimit)
	}
	obs := writeReconcileObs(t, body)

	code, stdout, stderr := reconcileRun(t, dir, obs, "delivery")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != "observation-invalid" {
		t.Fatalf("outcome = %v, want observation-invalid for an oversized payload; sidecar: %v", side["outcome"], side)
	}
	detail, _ := side["detail"].(string)
	if !strings.Contains(detail, "larger than") {
		t.Fatalf("detail = %q, want it to name the size bound", detail)
	}
}

// reconcileDecisionCase is one non-agreeing outcome in the T3 decision-input
// contract: the sidecar must carry what an agent needs to present one explicit
// pending decision without guessing.
type reconcileDecisionCase struct {
	name string
	// manifest overrides the standard fixture; "" uses reconcileManifestBody.
	manifest string
	event    string
	// unbound grades with no selected item, so the bound identity has no item.
	unbound bool
	// zeroClock supplies a zero clock, so freshness cannot be validated.
	zeroClock bool
	// noObs makes the payload fail to load, so no stamp is echoed.
	noObs bool
	// observedAt overrides the RFC3339 stamp written into the payload.
	observedAt func(time.Time) string
	// mutate adjusts the payload after the stamp is set.
	mutate   func(map[string]any)
	outcome  string
	detail   bool
	findings bool
}

// reconcileDecisionObs writes one observation payload for a decision case,
// stamping observed_at first so every case is deterministic.
func reconcileDecisionObs(t *testing.T, now time.Time, tc reconcileDecisionCase) string {
	t.Helper()
	stamp := now.UTC().Format(time.RFC3339)
	if tc.observedAt != nil {
		stamp = tc.observedAt(now)
	}
	return writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["observed_at"] = stamp
		if tc.mutate != nil {
			tc.mutate(o)
		}
	}))
}

// TestLedgerReconcileNonAgreeOutcomesCarryDecisionInputs pins the T3 decision
// contract at the sidecar builder: every non-agreeing outcome carries the inputs
// an agent needs to present one explicit pending decision — the outcome, the
// requested vs observed event, the bound provider/scope/item identity, the
// observation stamp, and either structured findings or an explanatory detail.
// The table runs against the builder rather than the CLI so the whole outcome
// set is reachable, including input-resolution failures.
//
// ledger.ReconcileUnavailable is deliberately absent: the wiring always hands the
// comparator a non-nil observation, so that outcome has no sidecar path here.
func TestLedgerReconcileNonAgreeOutcomesCarryDecisionInputs(t *testing.T) {
	now := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	bound := ledger.Binding{State: ledger.WitnessBound, RecipeID: reconcileRecipeID}
	item := &ledger.Item{ItemID: "card-1"}

	// unconfiguredManifest has config values but no [config.reconcile] mapping.
	unconfiguredManifest := `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
default_list = "In Progress"
`
	// duplicatePropertyManifest declares the same property twice for one event,
	// which is unusable declarations rather than a comparison.
	duplicatePropertyManifest := reconcileManifestBody + `[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "delivery"
property = "list"
config_field = "board_id"
`

	cases := []reconcileDecisionCase{
		{
			name:     "unconfigured",
			manifest: unconfiguredManifest,
			event:    "delivery",
			outcome:  reconcileUnconfigured,
			detail:   true,
		},
		{
			name:    "unmapped event",
			event:   "release",
			outcome: reconcileUnmappedEvent,
			detail:  true,
		},
		{
			name:    "observation invalid",
			event:   "delivery",
			noObs:   true,
			mutate:  func(o map[string]any) { o["properties"] = map[string]any{"list": 42} },
			outcome: reconcileInvalidObservation,
			detail:  true,
		},
		{
			name:     "event mismatch",
			event:    "delivery",
			mutate:   func(o map[string]any) { o["event"] = "merge" },
			outcome:  reconcileEventMismatch,
			findings: true,
		},
		{
			name:     "unbound identity",
			event:    "delivery",
			unbound:  true,
			outcome:  ledger.ReconcileUnboundIdentity,
			findings: true,
		},
		{
			name:     "identity mismatch",
			event:    "delivery",
			mutate:   func(o map[string]any) { o["scope"] = "board-2" },
			outcome:  ledger.ReconcileIdentityMismatch,
			findings: true,
		},
		{
			name:      "invalid clock",
			event:     "delivery",
			zeroClock: true,
			outcome:   ledger.ReconcileInvalidClock,
		},
		{
			name:     "invalid window",
			manifest: strings.Replace(reconcileManifestBody, "max_age_seconds = 900\n", "", 1),
			event:    "delivery",
			outcome:  ledger.ReconcileInvalidWindow,
		},
		{
			name:     "invalid declarations",
			manifest: duplicatePropertyManifest,
			event:    "delivery",
			outcome:  ledger.ReconcileInvalidDeclarations,
		},
		{
			name:       "invalid observed-at",
			event:      "delivery",
			observedAt: func(time.Time) string { return "not-a-time" },
			outcome:    ledger.ReconcileInvalidObservedAt,
		},
		{
			name:       "future observed-at",
			event:      "delivery",
			observedAt: func(n time.Time) string { return n.Add(time.Hour).UTC().Format(time.RFC3339) },
			outcome:    ledger.ReconcileFutureObservedAt,
		},
		{
			name:       "stale observation",
			event:      "delivery",
			observedAt: func(n time.Time) string { return n.Add(-2 * time.Hour).UTC().Format(time.RFC3339) },
			outcome:    ledger.ReconcileStaleObservation,
		},
		{
			name:     "missing property",
			event:    "delivery",
			mutate:   func(o map[string]any) { o["properties"] = map[string]string{} },
			outcome:  ledger.ReconcileMissingProperty,
			findings: true,
		},
		{
			name:     "property mismatch",
			event:    "delivery",
			mutate:   func(o map[string]any) { o["properties"] = map[string]string{"list": "Done"} },
			outcome:  ledger.ReconcilePropertyMismatch,
			findings: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := tc.manifest
			if manifest == "" {
				manifest = reconcileManifestBody
			}
			root := t.TempDir()
			writeReconcileManifest(t, root, manifest)
			obs := reconcileDecisionObs(t, now, tc)

			clock := now
			if tc.zeroClock {
				clock = time.Time{}
			}
			verdict := ledger.Verdict{Item: item}
			if tc.unbound {
				verdict.Item = nil
			}

			side := reconcileSidecar(root, bound, verdict, obs, tc.event, clock)
			raw, err := json.Marshal(side)
			if err != nil {
				t.Fatalf("marshal sidecar: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("sidecar JSON: %v: %s", err, raw)
			}

			if got["outcome"] != tc.outcome {
				t.Fatalf("outcome = %v, want %s; sidecar: %s", got["outcome"], tc.outcome, raw)
			}
			// Decision inputs are always serialized, whatever the outcome, so a
			// decision can be presented without re-deriving anything.
			for _, key := range []string{"outcome", "requested_event", "provider_id", "scope",
				"item_id", "observed_event", "observed_at", "findings"} {
				if _, ok := got[key]; !ok {
					t.Fatalf("sidecar omits %q: %s", key, raw)
				}
			}
			if got["requested_event"] != tc.event {
				t.Fatalf("requested_event = %v, want %s", got["requested_event"], tc.event)
			}
			if got["provider_id"] != reconcileRecipeID {
				t.Fatalf("provider_id = %v, want the bound recipe id", got["provider_id"])
			}
			wantItem := "card-1"
			if tc.unbound {
				wantItem = ""
			}
			if got["item_id"] != wantItem {
				t.Fatalf("item_id = %v, want %q", got["item_id"], wantItem)
			}

			// The observation stamp is what the agent turns into an age; it is
			// echoed as read, and absent only when the payload never loaded.
			wantStamp := now.UTC().Format(time.RFC3339)
			if tc.observedAt != nil {
				wantStamp = tc.observedAt(now)
			}
			if tc.noObs {
				if got["observed_at"] != "" {
					t.Fatalf("observed_at = %v, want empty for an unloaded payload", got["observed_at"])
				}
			} else if got["observed_at"] != wantStamp {
				t.Fatalf("observed_at = %v, want the reported stamp %q", got["observed_at"], wantStamp)
			}

			// Findings are non-empty only for outcomes that classify an item;
			// for whole-input outcomes the field is present and empty, never
			// omitted, so the agent can always read it.
			findings, _ := got["findings"].([]any)
			if tc.findings && len(findings) == 0 {
				t.Fatalf("findings = %v, want a non-empty list for %s", got["findings"], tc.outcome)
			}
			if !tc.findings && len(findings) != 0 {
				t.Fatalf("findings = %v, want an explicitly empty list for %s", got["findings"], tc.outcome)
			}

			// Input-resolution outcomes carry no findings, so the bounded detail
			// is the explanation the agent presents.
			detail, _ := got["detail"].(string)
			if tc.detail && detail == "" {
				t.Fatalf("detail = empty, want the resolution failure named for %s", tc.outcome)
			}
		})
	}
}

// reconcileMergeManifestBody declares a merge expectation on the default Done
// list, so a closed-only fixture reaches the comparison with a mapped event.
const reconcileMergeManifestBody = `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
done_list = "Done"
[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900
[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "merge"
property = "list"
config_field = "done_list"
`

// reconcileMergeManifestPublishedBody is the same mapping with the optional
// conditional published field configured: the recipe-owned merge target becomes
// Published only because the project named that list. No default is invented.
const reconcileMergeManifestPublishedBody = `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
done_list = "Done"
published_list = "Published"
[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900
[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "merge"
property = "list"
config_field = "done_list"
config_field_when_set = "published_list"
`

// reconcileMergeManifestBadPublishedBody configures the optional field with the
// wrong type: unusable config is reported, never coerced into a merged target.
const reconcileMergeManifestBadPublishedBody = `[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = "board-1"
done_list = "Done"
published_list = 3
[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900
[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "merge"
property = "list"
config_field = "done_list"
config_field_when_set = "published_list"
`

// saveClosedReconcileStore stores exactly one closed row carrying itemID and
// returns the store path: a closed-only store has no primary (D17), which is the
// post-merge shape the comparison must bind without reopening the row.
func saveClosedReconcileStore(t *testing.T, common, branch, itemID string) string {
	t.Helper()
	return saveLedgerStore(t, common, branch, itemID, func(s *ledger.Store) {
		if err := s.CloseItem(s.Items[0].ID, ledger.Decision{At: ledgerT0.UTC().Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
	})
}

// TestLedgerReconcileBindsCorroboratedClosedItemAfterMerge pins the closed-item
// comparison path: with no primary, the observation's item id binds only because
// exactly one local closed row for the current identity corroborates it. The
// lookup is read-only and never makes the closed row primary (D17).
func TestLedgerReconcileBindsCorroboratedClosedItemAfterMerge(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	storePath := saveClosedReconcileStore(t, common, branch, "card-1")
	writeReconcileManifest(t, dir, reconcileMergeManifestBody)
	before := ledgerStoreBytes(t, storePath)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = "merge"
		o["properties"] = map[string]string{"list": "Done"}
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "merge")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileAgree {
		t.Fatalf("outcome = %v, want %s; sidecar: %v", side["outcome"], ledger.ReconcileAgree, side)
	}
	if side["item_id"] != "card-1" {
		t.Fatalf("item_id = %v, want the corroborated closed row id", side["item_id"])
	}
	if string(ledgerStoreBytes(t, storePath)) != string(before) {
		t.Fatal("reconciliation must not rewrite the store")
	}
}

// TestLedgerReconcileUncorroboratedClosedItemStaysUnbound pins that the
// observation's item id is never trusted on its own: with no local closed row
// carrying it, the comparison stays unbound-identity and disagrees.
func TestLedgerReconcileUncorroboratedClosedItemStaysUnbound(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	saveClosedReconcileStore(t, common, branch, "card-1")
	writeReconcileManifest(t, dir, reconcileMergeManifestBody)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = "merge"
		o["item_id"] = "card-999"
		o["properties"] = map[string]string{"list": "Done"}
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "merge")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileUnboundIdentity {
		t.Fatalf("outcome = %v, want %s", side["outcome"], ledger.ReconcileUnboundIdentity)
	}
	if side["item_id"] != "" {
		t.Fatalf("item_id = %v, want empty for an uncorroborated id", side["item_id"])
	}
}

// TestLedgerReconcileAmbiguousClosedItemStaysUnbound pins the ambiguous half: two
// closed rows for the identity carrying the same observed id are a human question,
// so the comparison stays unbound-identity instead of picking one.
func TestLedgerReconcileAmbiguousClosedItemStaysUnbound(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", reconcileRecipeID)
	saveLedgerStore(t, common, branch, "card-1", func(s *ledger.Store) {
		if err := s.CloseItem(s.Items[0].ID, ledger.Decision{At: ledgerT0.UTC().Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		at := ledgerT0.Add(time.Minute)
		s.OpenItem(ledger.ItemIdentity{CommonDir: common, Branch: branch}, reconcileRecipeID, at)
		s.Items[len(s.Items)-1].ItemID = "card-1"
		if err := s.CloseItem(s.Items[len(s.Items)-1].ID, ledger.Decision{At: at.UTC().Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
	})
	writeReconcileManifest(t, dir, reconcileMergeManifestBody)
	obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
		o["event"] = "merge"
		o["properties"] = map[string]string{"list": "Done"}
	}))

	code, stdout, stderr := reconcileRun(t, dir, obs, "merge")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	side := reconcileSidecarOf(t, stdout)
	if side["outcome"] != ledger.ReconcileUnboundIdentity {
		t.Fatalf("outcome = %v, want %s", side["outcome"], ledger.ReconcileUnboundIdentity)
	}
	if side["item_id"] != "" {
		t.Fatalf("item_id = %v, want empty for an ambiguous match", side["item_id"])
	}
}

// TestLedgerReconcileConditionalMergePrefersPublishedWhenConfigured pins the
// recipe-owned conditional mapping: the same declared expectation compares the
// published list only when the project configured it, and the default Done list
// otherwise. No provider vocabulary reaches the comparator.
func TestLedgerReconcileConditionalMergePrefersPublishedWhenConfigured(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		list    string
		outcome string
	}{
		{"configured published agrees", reconcileMergeManifestPublishedBody, "Published", ledger.ReconcileAgree},
		{"configured published rejects Done", reconcileMergeManifestPublishedBody, "Done", ledger.ReconcilePropertyMismatch},
		{"absent published keeps Done", reconcileMergeManifestBody, "Done", ledger.ReconcileAgree},
		{"misconfigured published is unconfigured", reconcileMergeManifestBadPublishedBody, "Published", reconcileUnconfigured},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, branch := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", reconcileRecipeID)
			saveClosedReconcileStore(t, common, branch, "card-1")
			writeReconcileManifest(t, dir, tc.body)
			obs := writeReconcileObs(t, reconcileObsBody(t, func(o map[string]any) {
				o["event"] = "merge"
				o["properties"] = map[string]string{"list": tc.list}
			}))

			code, stdout, stderr := reconcileRun(t, dir, obs, "merge")
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
			}
			if side := reconcileSidecarOf(t, stdout); side["outcome"] != tc.outcome {
				t.Fatalf("outcome = %v, want %s", side["outcome"], tc.outcome)
			}
		})
	}
}
