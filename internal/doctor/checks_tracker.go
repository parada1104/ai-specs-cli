package doctor

// Native port of doctor._check_tracker_ledger and its acquisition helpers
// (ledger_bridge witness read and the literal recipe fallback). The
// gate_binary.resolve_verified_binary resolution the check consumes lives once
// in gatebinary.go with the other gate-binary subset helpers. Read-only:
// subprocesses are a git rev-parse probe and the gate binary's --ledger verdict
// query; nothing is written, created, or deleted.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// legacyTrackerRecipeID mirrors ledger_bridge.LEGACY_RECIPE_ID.
const legacyTrackerRecipeID = "trello-mcp-workflow"

// trackerCommonDir mirrors doctor._tracker_common_dir (and
// ledger_bridge._git_common_dir): the owning repo's Git common dir, or "" when
// it cannot be resolved.
func trackerCommonDir(root string) string {
	out, code, ok := runGitCapture(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok || code != 0 {
		return ""
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return ""
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	return resolvePy(candidate)
}

// trackerWitnessPath mirrors doctor._tracker_witness_path.
func trackerWitnessPath(common string) string {
	if common == "" {
		return ""
	}
	return filepath.Join(common, "ai-specs", "ledger", "witness.json")
}

// trackerDeclared mirrors doctor._tracker_declared: a config read, not a grade.
func trackerDeclared(root string) bool {
	for _, name := range []string{"config.yaml", "config.yml"} {
		candidate := filepath.Join(root, "openspec", name)
		if !isFile(candidate) {
			continue
		}
		raw, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		for _, line := range pySplitLines(string(raw)) {
			if strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "tracking:") {
				return true
			}
		}
	}
	return false
}

// trackerRecipeID mirrors ledger_bridge.recipe_id: the witness-bound recipe id,
// with the legacy literal fallback for a missing, corrupt, unknown-version,
// wrong-capability, or id-less witness.
func trackerRecipeID(root string) string {
	common := trackerCommonDir(root)
	if common == "" {
		return legacyTrackerRecipeID
	}
	raw, err := os.ReadFile(filepath.Join(common, "ai-specs", "ledger", "witness.json"))
	if err != nil {
		return legacyTrackerRecipeID
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return legacyTrackerRecipeID
	}
	if data == nil {
		return legacyTrackerRecipeID
	}
	v, _ := data["v"].(float64)
	if v != 1 {
		return legacyTrackerRecipeID
	}
	if capability, _ := data["capability"].(string); capability != "tracker" {
		return legacyTrackerRecipeID
	}
	bound, _ := data["recipe_id"].(string)
	if trimmed := strings.TrimSpace(bound); trimmed != "" {
		return trimmed
	}
	return legacyTrackerRecipeID
}

// trackerRecipeEnabled mirrors doctor._tracker_recipe_enabled.
func (d *Doctor) trackerRecipeEnabled(recipeID string) bool {
	if recipeID == "" {
		return false
	}
	data := d.manifestData()
	if data == nil {
		return false
	}
	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return false
	}
	entry, ok := recipes.Table(recipeID)
	if !ok || entry == nil {
		return false
	}
	enabled, isBool := entry.Bool("enabled")
	return isBool && enabled
}

// trackerWitnessState mirrors doctor._tracker_witness_state.
func trackerWitnessState(root string) string {
	witness := trackerWitnessPath(trackerCommonDir(root))
	if witness == "" || !isFile(witness) {
		return ""
	}
	raw, err := os.ReadFile(witness)
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	if data == nil {
		return ""
	}
	state, _ := data["state"].(string)
	return state
}

// trackerLedgerInPlay mirrors doctor._tracker_ledger_in_play: the relevance
// gate. It reads configuration and any already-written witness and never grades
// a link section.
func (d *Doctor) trackerLedgerInPlay() bool {
	recipeID := trackerRecipeID(d.Root)
	if recipeID != "" && d.trackerRecipeEnabled(recipeID) {
		return true
	}
	if trackerDeclared(d.Root) {
		return true
	}
	witness := trackerWitnessPath(trackerCommonDir(d.Root))
	return witness != "" && isFile(witness)
}

// trackerLedgerGuidance mirrors doctor._tracker_ledger_guidance: a
// presentation-only action hint keyed off the Go reason.
func trackerLedgerGuidance(reason string, severity Severity) string {
	if severity == ERROR {
		return "run ai-specs sync or ai-specs sync --refresh-gates"
	}
	switch reason {
	case "witness-missing":
		return "ai-specs sync"
	case "ambiguous":
		return `add [[bindings]] capability="tracker"`
	case "declared-not-bound":
		return "enable the provider recipe or remove the tracking declaration"
	case "conflict":
		return "adjudicate at the next checkpoint"
	case "unbound":
		return "enable one tracker recipe or ignore"
	}
	return ""
}

// --- gate_binary.resolve_verified_binary subset ------------------------------

// The verified-gate-binary resolution and its cache/trust-root helpers live once
// in gatebinary.go; the tracker ledger check calls them there.

// trackerLedgerBinary mirrors doctor._tracker_ledger_binary: a verified
// worktree-gate binary, or "" (fail closed to ERROR).
func (d *Doctor) trackerLedgerBinary() string {
	return resolveVerifiedGateBinary(d.Home)
}

// --- the check ---------------------------------------------------------------

// ledgerSeverity mirrors Severity(str(finding.get("severity") or "OK")) with
// the ValueError fallback to ERROR.
func ledgerSeverity(value any) Severity {
	var text string
	switch x := value.(type) {
	case nil:
		text = "OK"
	case string:
		if x == "" {
			text = "OK"
		} else {
			text = x
		}
	case bool:
		if !x {
			text = "OK"
		} else {
			text = pyStrValue(x)
		}
	default:
		text = pyStrValue(value)
	}
	switch text {
	case "OK":
		return OK
	case "INFO":
		return INFO
	case "WARN":
		return WARN
	case "ERROR":
		return ERROR
	}
	return ERROR
}

// ledgerText mirrors str(value or "") for the finding message / payload reason.
func ledgerText(value any) string {
	switch x := value.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if !x {
			return ""
		}
		return pyStrValue(x)
	default:
		return pyStrValue(value)
	}
}

// runTrackerLedger invokes the verified binary's non-blocking warn verdict and
// returns the `doctor` finding. ok is false when stdout was not the expected
// machine-readable object (the legacy exception / non-dict branch).
func runTrackerLedger(binary, root string) (finding map[string]any, reason string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary,
		"--ledger", "--checkpoint", "work-start", "--ledger-mode", "warn",
		"--project-root", root)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	_ = cmd.Run() // the legacy parses stdout regardless of the exit code
	if ctx.Err() != nil {
		return nil, "", false
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return nil, "", false
	}
	if payload == nil {
		return nil, "", false
	}
	finding, isMap := payload["doctor"].(map[string]any)
	if !isMap {
		return nil, "", false
	}
	return finding, ledgerText(payload["reason"]), true
}

// checkTrackerLedger is doctor._check_tracker_ledger.
func (d *Doctor) checkTrackerLedger() {
	if !d.trackerLedgerInPlay() {
		return
	}
	if trackerWitnessState(d.Root) == "bound" && !d.trackerRecipeEnabled("plan-build-flow") {
		// L5: work-start stays hosted by plan-build-flow. Where that recipe is
		// not enabled the checkpoint is unhosted while the other four keep
		// grading; doctor reports the limitation and issues no write.
		d.add(INFO, "tracker-ledger",
			"work-start is unhosted: enable plan-build-flow to grade that checkpoint",
			"enable the plan-build-flow recipe, or grade work-start with an explicit --write")
	}
	binary := d.trackerLedgerBinary()
	if binary == "" {
		d.add(ERROR, "tracker-ledger",
			"no verified worktree-gate binary; the tracker ledger is failing open",
			"run ai-specs sync or ai-specs sync --refresh-gates")
		return
	}
	finding, reason, ok := runTrackerLedger(binary, d.Root)
	if !ok {
		d.add(ERROR, "tracker-ledger",
			"tracker ledger verdict was not machine-readable; failing open",
			"run ai-specs sync or ai-specs sync --refresh-gates")
		return
	}
	severity := ledgerSeverity(finding["severity"])
	message := ledgerText(finding["message"])
	if message == "" {
		message = "tracker-ledger: " + reason
	}
	d.add(severity, "tracker-ledger", message, trackerLedgerGuidance(reason, severity))
}
