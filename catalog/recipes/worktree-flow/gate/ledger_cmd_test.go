package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// ledgerT0 is a fixed clock for store fixtures.
var ledgerT0 = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

// ledgerGit runs git in dir and fails the test on error.
func ledgerGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// ledgerRepo creates a committed repository and returns its path plus the
// identity the ledger derives for it (A2).
func ledgerRepo(t *testing.T) (dir, common, branch string) {
	t.Helper()
	dir = t.TempDir()
	ledgerGit(t, dir, "init", "-q")
	ledgerGit(t, dir, "config", "user.email", "t@example.invalid")
	ledgerGit(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledgerGit(t, dir, "add", "README")
	ledgerGit(t, dir, "commit", "-qm", "init")
	common = RealPath(gitCommon(dir))
	branch = ledgerGit(t, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	// Use a fresh reader here: a memoized one would cache HEAD and hide a later
	// detach performed by a test in the same process.
	fresh := ledger.Facts(func(d string, args ...string) string { return git(d, args...) })
	derived := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, Facts: fresh})
	if derived.CommonDir != common || derived.Branch != branch {
		t.Fatalf("test identity (%s/%s) != ledger identity (%s/%s)", common, branch, derived.CommonDir, derived.Branch)
	}
	return dir, common, branch
}

// writeLedgerWitness writes a bound/unbound witness at the design default path.
func writeLedgerWitness(t *testing.T, common, state, recipeID string) string {
	t.Helper()
	path := ledger.WitnessPath(common)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"v": 1, "capability": "tracker", "state": state,
		"recipe_id": recipeID, "candidates": []string{}, "written_at": "2026-09-13T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// saveLedgerStore writes a store with one open item carrying itemID.
func saveLedgerStore(t *testing.T, common, branch, itemID string, mutate func(*ledger.Store)) string {
	t.Helper()
	path := ledger.StorePath(common)
	store := ledger.Store{V: ledger.StoreVersion}
	store.OpenItem(ledger.ItemIdentity{CommonDir: common, Branch: branch}, "trello-mcp-workflow", ledgerT0)
	store.Items[len(store.Items)-1].ItemID = itemID
	if mutate != nil {
		mutate(&store)
	}
	if err := ledger.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeLedgerEvidence writes an evidence file (remote/code/git sides).
func writeLedgerEvidence(t *testing.T, sides map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.json")
	body, err := json.Marshal(sides)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// decodeLedgerOut parses one ledger verdict object from stdout.
func decodeLedgerOut(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("ledger stdout is not one JSON object: %v: %q", err, stdout)
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func assertKeys(t *testing.T, label string, m map[string]any, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := sortedKeys(m)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s keys = %v, want %v", label, got, want)
	}
}

// TestLedgerJSONContractKeys pins the design's exact stdout JSON keys and the
// identity/doctor sub-object keys.
func TestLedgerJSONContractKeys(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", "work-start", "--ledger-mode", "warn", "--project-root", dir)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	assertKeys(t, "verdict", out, "capability", "active", "checkpoint", "mode", "decision", "reason", "identity", "item", "conflict", "prompt", "doctor")

	if out["capability"] != "tracker" {
		t.Fatalf("capability = %v, want tracker", out["capability"])
	}
	if out["checkpoint"] != "work-start" {
		t.Fatalf("checkpoint = %v, want work-start", out["checkpoint"])
	}
	if out["mode"] != "warn" {
		t.Fatalf("mode = %v, want warn", out["mode"])
	}
	if out["decision"] != "allow" {
		t.Fatalf("decision = %v, want allow", out["decision"])
	}
	if out["active"] != true {
		t.Fatalf("active = %v, want true", out["active"])
	}
	if out["item"] != nil || out["conflict"] != nil || out["prompt"] != nil {
		t.Fatalf("item/conflict/prompt = %v/%v/%v, want all null", out["item"], out["conflict"], out["prompt"])
	}

	ident, ok := out["identity"].(map[string]any)
	if !ok {
		t.Fatalf("identity = %v, want an object", out["identity"])
	}
	assertKeys(t, "identity", ident, "common_dir", "branch", "change", "key")
	if ident["change"] != nil {
		t.Fatalf("identity.change = %v, want null when no change folder resolves", ident["change"])
	}

	doctor, ok := out["doctor"].(map[string]any)
	if !ok {
		t.Fatalf("doctor = %v, want an object", out["doctor"])
	}
	assertKeys(t, "doctor", doctor, "severity", "name", "message")
	if doctor["name"] != "tracker-ledger" {
		t.Fatalf("doctor.name = %v, want tracker-ledger", doctor["name"])
	}
}

// TestLedgerExitCodeContract pins exit 0 for allow/ask/dormant/unevaluable and
// exit 2 only for block.
func TestLedgerExitCodeContract(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", nil)

	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantDecide string
	}{
		{"allow in warn", []string{"--checkpoint", "apply-start", "--ledger-mode", "warn"}, 0, "allow"},
		{"ask in ask", []string{"--checkpoint", "pre-merge", "--ledger-mode", "ask", "--store", emptyLedgerStorePath(t)}, 0, "ask"},
		{"block in always", []string{"--checkpoint", "pre-merge", "--ledger-mode", "always", "--store", emptyLedgerStorePath(t)}, 2, "block"},
		{"dormant without witness", []string{"--checkpoint", "work-start", "--witness", filepath.Join(t.TempDir(), "missing.json")}, 0, "dormant"},
		{"unevaluable with corrupt store", []string{"--checkpoint", "work-start", "--store", corruptLedgerStore(t)}, 0, "unevaluable"},
	}
	for _, tc := range cases {
		args := append([]string{"--ledger", "--project-root", dir}, tc.args...)
		code, stdout, stderr := runCLI(t, args...)
		if code != tc.wantCode {
			t.Fatalf("%s: exit = %d, want %d; stderr: %s", tc.name, code, tc.wantCode, stderr)
		}
		out := decodeLedgerOut(t, stdout)
		if out["decision"] != tc.wantDecide {
			t.Fatalf("%s: decision = %v, want %s", tc.name, out["decision"], tc.wantDecide)
		}
	}
}

// emptyLedgerStorePath is a store path that does not exist: a missing store
// reads as an empty item set.
func emptyLedgerStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "ai-specs", "ledger", "state.json")
}

// corruptLedgerStore writes an invalid store file and returns its path.
func corruptLedgerStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLedgerDormantWhenWitnessMissing pins that a missing witness is dormant and
// never bound or guessed (A4/D6).
func TestLedgerDormantWhenWitnessMissing(t *testing.T) {
	dir, _, _ := ledgerRepo(t)
	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "work-start", "--ledger-mode", "always",
		"--project-root", dir, "--witness", filepath.Join(t.TempDir(), "missing.json"))
	if code != 0 {
		t.Fatalf("dormant exit = %d, want 0", code)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "dormant" || out["active"] != false {
		t.Fatalf("dormant verdict = %v (active %v)", out["decision"], out["active"])
	}
	if out["reason"] != "witness-missing" {
		t.Fatalf("reason = %v, want witness-missing", out["reason"])
	}
	doctor := out["doctor"].(map[string]any)
	if doctor["severity"] != "WARN" {
		t.Fatalf("doctor severity = %v, want WARN", doctor["severity"])
	}
}

// TestLedgerWarnNeverBlocksViaCLI pins that a warn-mode conflict is reported but
// allowed with exit 0.
func TestLedgerWarnNeverBlocksViaCLI(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", nil)
	evidence := writeLedgerEvidence(t, map[string]string{"remote": "card-remote"})

	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "pr-review", "--ledger-mode", "warn",
		"--project-root", dir, "--evidence", evidence)
	if code != 0 {
		t.Fatalf("warn conflict exit = %d, want 0", code)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "allow" {
		t.Fatalf("decision = %v, want allow in warn", out["decision"])
	}
	if out["conflict"] == nil {
		t.Fatalf("conflict = null, want the reported disagreement")
	}
	doctor := out["doctor"].(map[string]any)
	if doctor["severity"] != "WARN" {
		t.Fatalf("doctor severity = %v, want WARN for a recorded conflict", doctor["severity"])
	}
}

// TestLedgerDecidePersistsThenRegradeAllows pins the full adjudication round trip
// through the CLI: conflict asks, --decide persists, the re-grade allows, and a
// fresh invocation also allows.
func TestLedgerDecidePersistsThenRegradeAllows(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", nil)
	evidence := writeLedgerEvidence(t, map[string]string{"remote": "card-remote"})
	base := []string{"--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask", "--project-root", dir, "--evidence", evidence}

	code, stdout, _ := runCLI(t, base...)
	if code != 0 {
		t.Fatalf("conflict exit = %d, want 0", code)
	}
	if out := decodeLedgerOut(t, stdout); out["decision"] != "ask" {
		t.Fatalf("pre-decision = %v, want ask", out["decision"])
	}

	decideArgs := append(append([]string{}, base...), "--decide", `{"choice":"local"}`)
	code, stdout, stderr := runCLI(t, decideArgs...)
	if code != 0 {
		t.Fatalf("--decide exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "allow" || out["reason"] != "adjudicated" {
		t.Fatalf("post-decision = %v/%v, want allow/adjudicated", out["decision"], out["reason"])
	}
	if out["conflict"] != nil {
		t.Fatalf("post-decision conflict = %v, want null", out["conflict"])
	}

	code, stdout, _ = runCLI(t, base...)
	if code != 0 {
		t.Fatalf("re-grade exit = %d, want 0", code)
	}
	if out := decodeLedgerOut(t, stdout); out["decision"] != "allow" {
		t.Fatalf("fresh re-grade = %v, want allow from the persisted decision", out["decision"])
	}
}

// TestLedgerEmptyStoreAskOptOutPersistsAndAllows pins the fixed fresh-binding ask
// path end to end: no item exists, the explicit human opt-out is recorded
// checkpoint-scoped, the answered checkpoint allows, no tracked item is
// synthesized, and the next checkpoint asks again (D19).
func TestLedgerEmptyStoreAskOptOutPersistsAndAllows(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)
	base := []string{"--ledger", "--checkpoint", "apply-start", "--ledger-mode", "ask", "--project-root", dir}

	code, stdout, _ := runCLI(t, base...)
	if code != 0 {
		t.Fatalf("pre-decision exit = %d, want 0", code)
	}
	pre := decodeLedgerOut(t, stdout)
	if pre["decision"] != "ask" || pre["reason"] != "needs-item" || pre["item"] != nil {
		t.Fatalf("pre-decision = %v/%v (item %v), want ask/needs-item with no item",
			pre["decision"], pre["reason"], pre["item"])
	}

	decideArgs := append(append([]string{}, base...),
		"--decide", `{"checkpoint":"apply-start","kind":"opt-out","choice":"continue"}`)
	code, stdout, stderr := runCLI(t, decideArgs...)
	if code != 0 {
		t.Fatalf("--decide exit = %d, want 0; stderr: %s", code, stderr)
	}
	post := decodeLedgerOut(t, stdout)
	if post["decision"] != "allow" || post["reason"] != "opt-out" {
		t.Fatalf("post-decision = %v/%v, want allow/opt-out", post["decision"], post["reason"])
	}

	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 0 {
		t.Fatalf("items = %+v, want no synthesized tracked item", store.Items)
	}
	if len(store.OptOuts) != 1 || store.OptOuts[0].Checkpoint != "apply-start" {
		t.Fatalf("opt_outs = %+v, want exactly one apply-start scoped opt-out", store.OptOuts)
	}

	code, stdout, _ = runCLI(t, "--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask", "--project-root", dir)
	if code != 0 {
		t.Fatalf("next checkpoint exit = %d, want 0", code)
	}
	next := decodeLedgerOut(t, stdout)
	if next["decision"] != "ask" || next["reason"] != "needs-item" {
		t.Fatalf("next checkpoint = %v/%v, want ask/needs-item", next["decision"], next["reason"])
	}
}

// TestLedgerDecidePersistFailureExits2 pins the fail-closed rule: a --decide that
// cannot be persisted exits 2.
func TestLedgerDecidePersistFailureExits2(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	code, _, stderr := runCLI(t, "--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask",
		"--project-root", dir, "--decide", `{"choice":"local"}`)
	if code != 2 {
		t.Fatalf("failed --decide exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "decide") {
		t.Fatalf("failed --decide stderr = %q, want a decide failure", stderr)
	}
}

// TestLedgerInvalidDecideJSONExits2 pins that an unparsable human answer also
// fails closed.
func TestLedgerInvalidDecideJSONExits2(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", nil)
	code, _, stderr := runCLI(t, "--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask",
		"--project-root", dir, "--decide", "not-json")
	if code != 2 {
		t.Fatalf("invalid --decide exit = %d, want 2; stderr: %s", code, stderr)
	}
}

// TestLedgerFlagParseFailsOpen pins the gate precedent for --ledger verdict calls.
func TestLedgerFlagParseFailsOpen(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--ledger", "--bogus-flag")
	if code != 0 {
		t.Fatalf("--ledger parse error exit = %d, want 0 (fail-open)", code)
	}
	if stdout != "" {
		t.Fatalf("--ledger parse error stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "warning") {
		t.Fatalf("--ledger parse error stderr = %q, want a fail-open warning", stderr)
	}
}

// TestLedgerUnknownCheckpointIsUnevaluable pins fail-open for an unrecognized
// checkpoint value.
func TestLedgerUnknownCheckpointIsUnevaluable(t *testing.T) {
	dir, _, _ := ledgerRepo(t)
	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "bogus", "--project-root", dir)
	if code != 0 {
		t.Fatalf("unknown checkpoint exit = %d, want 0", code)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "unevaluable" || out["reason"] != "unknown-checkpoint" {
		t.Fatalf("unknown checkpoint = %v/%v, want unevaluable/unknown-checkpoint", out["decision"], out["reason"])
	}
}

// TestLedgerIdentityUnavailableModes pins detached HEAD behavior per mode.
func TestLedgerIdentityUnavailableModes(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	ledgerGit(t, dir, "checkout", "-q", "--detach")

	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "always", "--project-root", dir)
	if code != 2 {
		t.Fatalf("always/detached exit = %d, want 2", code)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "block" || out["reason"] != "identity_unavailable" {
		t.Fatalf("always/detached = %v/%v, want block/identity_unavailable", out["decision"], out["reason"])
	}

	code, stdout, _ = runCLI(t, "--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "warn", "--project-root", dir)
	if code != 0 {
		t.Fatalf("warn/detached exit = %d, want 0", code)
	}
	if out := decodeLedgerOut(t, stdout); out["decision"] != "allow" {
		t.Fatalf("warn/detached = %v, want allow", out["decision"])
	}
}

// TestLedgerTwoOpenItemsConflict pins the collision as a conflict, not a pick.
func TestLedgerTwoOpenItemsConflict(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", func(s *ledger.Store) {
		s.OpenItem(ledger.ItemIdentity{CommonDir: common, Branch: branch}, "trello-mcp-workflow", ledgerT0)
	})
	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--ledger-mode", "always", "--project-root", dir)
	if code != 2 {
		t.Fatalf("collision exit = %d, want 2", code)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "block" || out["reason"] != "conflict" {
		t.Fatalf("collision = %v/%v, want block/conflict", out["decision"], out["reason"])
	}
	if out["item"] != nil {
		t.Fatalf("collision selected an item: %v", out["item"])
	}
}

// TestLedgerItemPopulatedWhenOpenItem pins that a selected primary item is
// emitted, not just its verdict.
func TestLedgerItemPopulatedWhenOpenItem(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	saveLedgerStore(t, common, branch, "card-local", nil)
	code, stdout, _ := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--ledger-mode", "warn", "--project-root", dir)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	out := decodeLedgerOut(t, stdout)
	item, ok := out["item"].(map[string]any)
	if !ok {
		t.Fatalf("item = %v, want the selected object", out["item"])
	}
	if item["item_id"] != "card-local" || item["provider_id"] != "trello-mcp-workflow" {
		t.Fatalf("item = %v, want card-local/trello-mcp-workflow", item)
	}
}

// TestLedgerPersistsConflictSnapshotAndClearsOnDecide pins the persistence seam
// end to end: a conflict grade records the snapshot, and --decide clears it.
func TestLedgerPersistsConflictSnapshotAndClearsOnDecide(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := saveLedgerStore(t, common, branch, "card-local", nil)
	evidence := writeLedgerEvidence(t, map[string]string{"remote": "card-remote"})
	base := []string{"--ledger", "--checkpoint", "pre-merge", "--ledger-mode", "ask", "--project-root", dir, "--evidence", evidence}
	key := ledger.IdentityKey(common, branch, "")

	if code, _, _ := runCLI(t, base...); code != 0 {
		t.Fatalf("conflict grade exit = %d, want 0", code)
	}
	snapshot := recordedConflict(t, storePath, key)
	if snapshot == nil || snapshot.Sides.Local != "card-local" || snapshot.Sides.Remote != "card-remote" {
		t.Fatalf("recorded snapshot = %+v, want local card-local / remote card-remote", snapshot)
	}

	if code, _, stderr := runCLI(t, append(append([]string{}, base...), "--decide", `{"choice":"remote"}`)...); code != 0 {
		t.Fatalf("--decide exit = %d, want 0; stderr: %s", code, stderr)
	}
	if snapshot := recordedConflict(t, storePath, key); snapshot != nil {
		t.Fatalf("snapshot after --decide = %+v, want cleared", snapshot)
	}
}

// recordedConflict reads the primary item's current conflict snapshot, if any.
func recordedConflict(t *testing.T, storePath, key string) *ledger.Conflict {
	t.Helper()
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Primary(key)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot *ledger.Conflict
	if err := json.Unmarshal(item.Conflict, &snapshot); err != nil {
		t.Fatalf("item conflict is not a snapshot: %v (%s)", err, item.Conflict)
	}
	return snapshot
}

// TestLedgerExplainIsAnAlias pins that --explain under --ledger still emits the
// same ledger JSON.
func TestLedgerExplainIsAnAlias(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	code, stdout, stderr := runCLI(t, "--ledger", "--explain", "--checkpoint", "work-start", "--project-root", dir)
	if code != 0 {
		t.Fatalf("--ledger --explain exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	assertKeys(t, "verdict", out, "capability", "active", "checkpoint", "mode", "decision", "reason", "identity", "item", "conflict", "prompt", "doctor")
}

// ledgerWritePrefix is the argv prefix for a write against an explicit project root.
func ledgerWritePrefix(dir string) []string {
	return []string{"--ledger", "--checkpoint", "apply-start", "--project-root", dir}
}

// ledgerWritePrefixMode is the same prefix with an explicit ledger mode.
func ledgerWritePrefixMode(dir, mode string) []string {
	return []string{"--ledger", "--checkpoint", "apply-start", "--ledger-mode", mode, "--project-root", dir}
}

// ledgerWriteArgs is the --write argv fragment for one payload.
func ledgerWriteArgs(payload string) []string {
	return []string{"--write", payload}
}

// ledgerWriteRun runs one --ledger invocation whose argv ends with a --write payload.
func ledgerWriteRun(t *testing.T, prefix []string, payload string) (int, string, string) {
	t.Helper()
	args := append(append([]string{}, prefix...), ledgerWriteArgs(payload)...)
	return runCLI(t, args...)
}

// ledgerStoreBytes reads the store file, or nil when it does not exist.
func ledgerStoreBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return data
}

// ledgerStoreMtime is the store's modification time, used to prove a failed write
// did not even rewrite identical bytes.
func ledgerStoreMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

// TestLedgerWriteOpenThenIdempotentRetryViaCLI pins the CLI write vehicle end to
// end: the write sidecar is emitted on success, the post-write grade is
// re-graded, and a retried open is reported as already-open.
func TestLedgerWriteOpenThenIdempotentRetryViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)
	base := []string{"--ledger", "--checkpoint", "apply-start", "--ledger-mode", "always", "--project-root", dir}

	code, stdout, stderr := ledgerWriteRun(t, base, `{"kind":"open"}`)
	if code != 0 {
		t.Fatalf("open exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	side, ok := out["write"].(map[string]any)
	if !ok {
		t.Fatalf("write sidecar = %v, want an object", out["write"])
	}
	assertKeys(t, "write", side, "kind", "applied", "reason")
	if side["kind"] != "open" || side["applied"] != true {
		t.Fatalf("write sidecar = %v, want open/applied", side)
	}
	if out["decision"] != "allow" || out["item"] == nil {
		t.Fatalf("post-write grade = %v (item %v), want allow with the item", out["decision"], out["item"])
	}

	code, stdout, stderr = runCLI(t, append(append([]string{}, base...), ledgerWriteArgs(`{"kind":"open"}`)...)...)
	if code != 0 {
		t.Fatalf("retried open exit = %d, want 0; stderr: %s", code, stderr)
	}
	side = decodeLedgerOut(t, stdout)["write"].(map[string]any)
	if side["applied"] != false || side["reason"] != "already-open" {
		t.Fatalf("retried open sidecar = %v, want applied=false/already-open", side)
	}
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 {
		t.Fatalf("items = %d, want exactly one", len(store.Items))
	}
}

// TestLedgerWriteBindSeedsOpenLinkViaCLI pins the generic binding command end to
// end: one `--write` bind against an empty store opens and links in a single
// transaction, the write sidecar reports it applied, the post-write grade selects
// the new item, and a retried bind is an unchanged, byte-stable no-op.
func TestLedgerWriteBindSeedsOpenLinkViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)
	base := []string{"--ledger", "--checkpoint", "apply-start", "--ledger-mode", "always", "--project-root", dir}
	payload := `{"kind":"bind","item_id":"card-bind","url":"https://example.invalid/c/bind","native_type":"card","state":"in-progress","provider":{"list":"In Progress"}}`

	code, stdout, stderr := ledgerWriteRun(t, base, payload)
	if code != 0 {
		t.Fatalf("bind exit = %d, want 0; stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	side, ok := out["write"].(map[string]any)
	if !ok {
		t.Fatalf("write sidecar = %v, want an object", out["write"])
	}
	assertKeys(t, "bind write", side, "kind", "applied", "reason")
	if side["kind"] != "bind" || side["applied"] != true || side["reason"] != "" {
		t.Fatalf("bind write sidecar = %v, want bind/applied with no reason", side)
	}
	if out["decision"] != "allow" {
		t.Fatalf("post-bind grade = %v/%v, want allow", out["decision"], out["reason"])
	}
	item, ok := out["item"].(map[string]any)
	if !ok || item["item_id"] != "card-bind" || item["status"] != "open" {
		t.Fatalf("post-bind item = %v, want the freshly bound open item", out["item"])
	}

	before := ledgerStoreBytes(t, storePath)
	code, stdout, stderr = ledgerWriteRun(t, base, payload)
	if code != 0 {
		t.Fatalf("bind retry exit = %d, want 0; stderr: %s", code, stderr)
	}
	side = decodeLedgerOut(t, stdout)["write"].(map[string]any)
	if side["applied"] != false || side["reason"] != "unchanged" {
		t.Fatalf("bind retry sidecar = %v, want applied=false/unchanged", side)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("an idempotent bind retry must leave the store byte-identical")
	}

	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 {
		t.Fatalf("items = %d, want exactly one opened-and-linked row", len(store.Items))
	}
	opens, links := 0, 0
	for _, d := range store.Items[0].Decisions {
		switch d.Kind {
		case ledger.DecisionOpen:
			opens++
		case ledger.DecisionLink:
			links++
		}
	}
	if opens != 1 || links != 1 {
		t.Fatalf("decisions = %+v, want exactly one open and one link", store.Items[0].Decisions)
	}
}

// TestLedgerWriteBindIsRefusedForAChangeAmbiguousIdentityViaCLI pins the
// fail-closed posture on the generic bind too: two active change folders make the
// identity ambiguous, so a bind without an explicit slug exits 2 and persists
// nothing.
func TestLedgerWriteBindIsRefusedForAChangeAmbiguousIdentityViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	for _, slug := range []string{"alpha-change", "beta-change"} {
		if err := os.MkdirAll(filepath.Join(dir, "openspec", "changes", slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	storePath := ledger.StorePath(common)

	code, stdout, stderr := ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"bind","item_id":"card-ambiguous"}`)
	if code != 2 {
		t.Fatalf("ambiguous bind exit = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("ambiguous bind stdout = %q, want no JSON", stdout)
	}
	if ledgerStoreBytes(t, storePath) != nil {
		t.Fatal("a refused ambiguous bind must persist nothing")
	}

	code, _, stderr = ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"bind","item_id":"card-ambiguous","change":"alpha-change"}`)
	if code != 0 {
		t.Fatalf("explicit-slug bind exit = %d, want 0; stderr: %s", code, stderr)
	}
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 || store.Items[0].Identity.Change != "alpha-change" || store.Items[0].ItemID != "card-ambiguous" {
		t.Fatalf("store items = %+v, want one bound item under the explicit slug", store.Items)
	}
}

// TestLedgerWriteCloseWithSnapshotThenRetryViaCLI pins the live close path: an
// explicit close carrying the observed provider snapshot must persist state and
// provider, close the row, and still grade allow in the same invocation. Grade
// only selects open rows for new work (D17), so the close write reports its own
// just-closed row to that one post-write grade. The idempotent retry reports
// already-closed and grades the same closed row.
func TestLedgerWriteCloseWithSnapshotThenRetryViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	// An active change folder gives the work an identity slug, the realistic shape.
	if err := os.MkdirAll(filepath.Join(dir, "openspec", "changes", "tracker-reconcile-adoption"), 0o755); err != nil {
		t.Fatal(err)
	}
	storePath := ledger.StorePath(common)

	if code, _, stderr := ledgerWriteRun(t, ledgerWritePrefixMode(dir, "always"), `{"kind":"open"}`); code != 0 {
		t.Fatalf("open exit = %d, want 0; stderr: %s", code, stderr)
	}

	closePrefix := []string{"--ledger", "--checkpoint", "archive-close", "--ledger-mode", "always", "--project-root", dir}
	payload := `{"kind":"close","state":"done","provider":{"list":"Done"}}`
	code, stdout, stderr := ledgerWriteRun(t, closePrefix, payload)
	if code != 0 {
		t.Fatalf("close exit = %d, want 0 (allow); stderr: %s", code, stderr)
	}
	out := decodeLedgerOut(t, stdout)
	if out["decision"] != "allow" || out["reason"] != "" {
		t.Fatalf("post-close grade = %v/%v, want allow", out["decision"], out["reason"])
	}
	item, ok := out["item"].(map[string]any)
	if !ok {
		t.Fatalf("post-close item = %v, want the just-closed row", out["item"])
	}
	if item["status"] != "closed" || item["state"] != "done" {
		t.Fatalf("post-close item = %v, want status closed and state done", item)
	}
	provider, ok := item["provider"].(map[string]any)
	if !ok || provider["list"] != "Done" {
		t.Fatalf("post-close provider = %v, want the observed snapshot {list: Done}", item["provider"])
	}
	side, ok := out["write"].(map[string]any)
	if !ok {
		t.Fatalf("close write sidecar = %v, want an object", out["write"])
	}
	assertKeys(t, "close write", side, "kind", "applied", "reason")
	if side["kind"] != "close" || side["applied"] != true {
		t.Fatalf("close write sidecar = %v, want close/applied", side)
	}

	// The retry meets the same closed row: already-closed, and the post-write
	// grade still resolves to it instead of needs-item.
	code, stdout, stderr = ledgerWriteRun(t, closePrefix, payload)
	if code != 0 {
		t.Fatalf("close retry exit = %d, want 0 (allow); stderr: %s", code, stderr)
	}
	out = decodeLedgerOut(t, stdout)
	if out["decision"] != "allow" {
		t.Fatalf("post-close retry grade = %v/%v, want allow", out["decision"], out["reason"])
	}
	retryItem, ok := out["item"].(map[string]any)
	if !ok || retryItem["status"] != "closed" {
		t.Fatalf("post-close retry item = %v, want the closed row", out["item"])
	}
	side = out["write"].(map[string]any)
	if side["applied"] != false || side["reason"] != "already-closed" {
		t.Fatalf("close retry sidecar = %v, want applied=false/already-closed", side)
	}
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 || store.Items[0].Status != ledger.StatusClosed {
		t.Fatalf("close retry must keep one closed row: %+v", store.Items)
	}
}

// TestLedgerWriteCloseRetryRecoversStoredSlugViaCLI pins the close-only identity
// recovery: an idempotent close retry must target the row the first close
// persisted even when the change slug can no longer be derived — because its
// folder was archived, or because several active change folders made the identity
// ambiguous. Only the explicit close recovers the closed row's slug; a plain
// grade or a new open still never adopts a closed row (D17).
func TestLedgerWriteCloseRetryRecoversStoredSlugViaCLI(t *testing.T) {
	const slug = "tracker-reconcile-adoption"
	closePrefix := func(dir string) []string {
		return []string{"--ledger", "--checkpoint", "archive-close", "--ledger-mode", "always", "--project-root", dir}
	}
	const closePayload = `{"kind":"close","state":"done","provider":{"list":"Done"}}`

	cases := []struct {
		name  string
		drift func(t *testing.T, dir string)
	}{
		{
			name: "archived change folder",
			drift: func(t *testing.T, dir string) {
				active := filepath.Join(dir, "openspec", "changes", slug)
				archived := filepath.Join(dir, "openspec", "changes", "archive", "2026-01-01-"+slug)
				if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(active, archived); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "multiple active change folders",
			drift: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "openspec", "changes", "another-change"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, _ := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
			if err := os.MkdirAll(filepath.Join(dir, "openspec", "changes", slug), 0o755); err != nil {
				t.Fatal(err)
			}
			if code, _, stderr := ledgerWriteRun(t, ledgerWritePrefixMode(dir, "always"), `{"kind":"open"}`); code != 0 {
				t.Fatalf("open exit = %d, want 0; stderr: %s", code, stderr)
			}
			if code, stdout, stderr := ledgerWriteRun(t, closePrefix(dir), closePayload); code != 0 {
				t.Fatalf("close exit = %d, want 0 (allow); stderr: %s", code, stderr)
			} else if out := decodeLedgerOut(t, stdout); out["decision"] != "allow" {
				t.Fatalf("first close grade = %v/%v, want allow", out["decision"], out["reason"])
			}

			tc.drift(t, dir)

			code, stdout, stderr := ledgerWriteRun(t, closePrefix(dir), closePayload)
			if code != 0 {
				t.Fatalf("close retry exit = %d, want 0 (allow); stderr: %s", code, stderr)
			}
			out := decodeLedgerOut(t, stdout)
			if out["decision"] != "allow" {
				t.Fatalf("close retry grade = %v/%v, want allow", out["decision"], out["reason"])
			}
			side, ok := out["write"].(map[string]any)
			if !ok || side["applied"] != false || side["reason"] != "already-closed" {
				t.Fatalf("close retry sidecar = %v, want applied=false/already-closed", out["write"])
			}
			if item, ok := out["item"].(map[string]any); !ok || item["status"] != "closed" {
				t.Fatalf("close retry item = %v, want the closed row", out["item"])
			}
		})
	}
}

// TestLedgerWriteLinkPersistsNativeFieldsViaCLI pins the link verb through the CLI.
func TestLedgerWriteLinkPersistsNativeFieldsViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)
	base := []string{"--ledger", "--checkpoint", "apply-start", "--ledger-mode", "warn", "--project-root", dir}

	if code, _, stderr := ledgerWriteRun(t, base, `{"kind":"open"}`); code != 0 {
		t.Fatalf("open exit = %d, want 0; stderr: %s", code, stderr)
	}
	payload := `{"kind":"link","item_id":"6aa703fdcf61a90ec702d58b","url":"https://trello.com/c/ie4mQykZ","native_type":"card","state":"in-progress","provider":{"list":"In Progress"}}`
	code, stdout, stderr := ledgerWriteRun(t, base, payload)
	if code != 0 {
		t.Fatalf("link exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := decodeLedgerOut(t, stdout)["write"].(map[string]any); side["applied"] != true {
		t.Fatalf("link sidecar = %v, want applied", side)
	}
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Primary(ledger.IdentityKey(common, ledgerGit(t, dir, "symbolic-ref", "--quiet", "--short", "HEAD"), ""))
	if err != nil {
		t.Fatal(err)
	}
	if item.ItemID != "6aa703fdcf61a90ec702d58b" || item.NativeType != "card" || item.State != "in-progress" {
		t.Fatalf("item core fields = %+v, want the linked native fields", item)
	}
	if !strings.Contains(string(item.Provider), "In Progress") {
		t.Fatalf("opaque provider = %s, want the payload preserved", item.Provider)
	}
}

// TestLedgerWriteMutuallyExclusiveWithDecide pins the DW2 exclusivity: both flags
// exit 2 and persist nothing.
func TestLedgerWriteMutuallyExclusiveWithDecide(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := saveLedgerStore(t, common, branch, "card-local", nil)
	before := ledgerStoreBytes(t, storePath)

	code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--project-root", dir,
		"--write", `{"kind":"open"}`, "--decide", `{"choice":"local"}`)
	if code != 2 {
		t.Fatalf("mutually exclusive exit = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("mutually exclusive stdout = %q, want no JSON", stdout)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("a mutually exclusive invocation must persist nothing")
	}
}

// TestLedgerWriteFailedWriteExits2WithNoJSON pins the L6 output contract: a rejected
// write reports on stderr and emits no stdout JSON.
func TestLedgerWriteFailedWriteExits2WithNoJSON(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := saveLedgerStore(t, common, branch, "card-local", nil)
	before := ledgerStoreBytes(t, storePath)
	beforeMtime := ledgerStoreMtime(t, storePath)

	code, stdout, stderr := ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"invented"}`)
	if code != 2 {
		t.Fatalf("failed write exit = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("failed write stdout = %q, want no JSON", stdout)
	}
	if !strings.Contains(stderr, "ledger --write failed:") {
		t.Fatalf("failed write stderr = %q, want the --write failure prefix", stderr)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("a failed write must leave the store byte-identical")
	}
	if !ledgerStoreMtime(t, storePath).Equal(beforeMtime) {
		t.Fatal("a failed write must not rewrite the store at all")
	}
}

// TestLedgerWriteFlagParseFailsOpenButValidationFailsClosed pins the two postures:
// an unparsable flag on a verdict call fails open, while a successfully parsed
// write with a bad payload fails closed.
func TestLedgerWriteFlagParseFailsOpenButValidationFailsClosed(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")

	code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--project-root", dir,
		"--write", `{"kind":"open"}`, "--bogus-flag")
	if code != 0 {
		t.Fatalf("flag-parse error exit = %d, want 0 (fail open); stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("flag-parse error stdout = %q, want empty", stdout)
	}

	code, stdout, _ = ledgerWriteRun(t, ledgerWritePrefix(dir), "not-json")
	if code != 2 {
		t.Fatalf("invalid --write JSON exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Fatalf("invalid --write JSON stdout = %q, want empty", stdout)
	}
}

// TestLedgerWriteExemptHonoredAtEveryCheckpointViaCLI pins the tracker.none mapping
// end to end: the exempt write persists a reason and every checkpoint then allows
// with reason=exempt, even in always.
func TestLedgerWriteExemptHonoredAtEveryCheckpointViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)

	code, stdout, stderr := ledgerWriteRun(t, ledgerWritePrefixMode(dir, "always"), `{"kind":"exempt","reason":"no tracker for this spike"}`)
	if code != 0 {
		t.Fatalf("exempt exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := decodeLedgerOut(t, stdout)["write"].(map[string]any); side["applied"] != true {
		t.Fatalf("exempt sidecar = %v, want applied", side)
	}

	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 || store.Items[0].Exemption != "no tracker for this spike" {
		t.Fatalf("store = %+v, want one exempt item with the persisted reason", store.Items)
	}

	for _, cp := range ledger.Checkpoints {
		code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", cp, "--ledger-mode", "always", "--project-root", dir)
		if code != 0 {
			t.Fatalf("%s exempt exit = %d, want 0; stderr: %s", cp, code, stderr)
		}
		out := decodeLedgerOut(t, stdout)
		if out["decision"] != "allow" || out["reason"] != "exempt" {
			t.Fatalf("%s = %v/%v, want allow/exempt", cp, out["decision"], out["reason"])
		}
	}
}

// TestLedgerWriteChangeAmbiguousRefusesViaCLI pins the collision refusal through the
// CLI: two active change folders make the identity ambiguous, so an open without an
// explicit slug exits 2 and writes nothing.
func TestLedgerWriteChangeAmbiguousRefusesViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	for _, slug := range []string{"alpha-change", "beta-change"} {
		folder := filepath.Join(dir, "openspec", "changes", slug)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	storePath := ledger.StorePath(common)

	code, stdout, stderr := ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"open"}`)
	if code != 2 {
		t.Fatalf("ambiguous write exit = %d, want 2; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("ambiguous write stdout = %q, want no JSON", stdout)
	}
	if ledgerStoreBytes(t, storePath) != nil {
		t.Fatal("a refused ambiguous write must persist nothing")
	}

	code, stdout, stderr = ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"open","change":"alpha-change"}`)
	if code != 0 {
		t.Fatalf("explicit-slug write exit = %d, want 0; stderr: %s", code, stderr)
	}
	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 || store.Items[0].Identity.Change != "alpha-change" {
		t.Fatalf("store items = %+v, want one item under the explicit slug", store.Items)
	}
	if decodeLedgerOut(t, stdout)["item"] == nil {
		t.Fatal("the post-write grade must select the freshly opened item")
	}
}

// TestLedgerWriteUsesTheOwnerProjectRootOrCwd pins that writes follow the same owner
// project root as grades: an explicit --project-root and the process cwd reach one
// store, so a write issued either way lands on the same item.
func TestLedgerWriteUsesTheOwnerProjectRootOrCwd(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)

	t.Chdir(dir)
	code, stdout, stderr := ledgerWriteRun(t, []string{"--ledger", "--checkpoint", "apply-start"}, `{"kind":"open"}`)
	if code != 0 {
		t.Fatalf("cwd-relative write exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := decodeLedgerOut(t, stdout)["write"].(map[string]any); side["applied"] != true {
		t.Fatalf("cwd-relative write sidecar = %v, want applied", side)
	}

	code, stdout, stderr = ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"link","item_id":"card-cwd"}`)
	if code != 0 {
		t.Fatalf("project-root write exit = %d, want 0; stderr: %s", code, stderr)
	}
	if side := decodeLedgerOut(t, stdout)["write"].(map[string]any); side["applied"] != true {
		t.Fatalf("project-root write sidecar = %v, want applied", side)
	}

	store, err := ledger.LoadStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Items) != 1 || store.Items[0].ItemID != "card-cwd" {
		t.Fatalf("store items = %+v, want one item linked through the same store", store.Items)
	}
}

// TestLedgerWriteLeavesNoTempResidueViaCLI pins the atomic-write contract at the CLI
// boundary: a successful write leaves only the store beside its lock file.
func TestLedgerWriteLeavesNoTempResidueViaCLI(t *testing.T) {
	dir, common, _ := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")

	if code, _, stderr := ledgerWriteRun(t, ledgerWritePrefix(dir), `{"kind":"open"}`); code != 0 {
		t.Fatalf("write exit = %d, want 0; stderr: %s", code, stderr)
	}
	residue, err := filepath.Glob(filepath.Join(common, "ai-specs", "ledger", "state.json.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(residue) != 0 {
		t.Fatalf("temp residue after a CLI write: %v", residue)
	}
}

// TestLedgerGradeNeverWritesStore pins grade purity (L2): a checkpoint that grades
// needs-item creates, mutates and deletes nothing — not even a store file.
func TestLedgerGradeNeverWritesStore(t *testing.T) {
	dir, common, branch := ledgerRepo(t)
	writeLedgerWitness(t, common, "bound", "trello-mcp-workflow")
	storePath := ledger.StorePath(common)

	for _, cp := range ledger.Checkpoints {
		code, stdout, stderr := runCLI(t, "--ledger", "--checkpoint", cp, "--ledger-mode", "always", "--project-root", dir)
		if code != 2 {
			t.Fatalf("%s: exit = %d, want 2 (block); stderr: %s", cp, code, stderr)
		}
		out := decodeLedgerOut(t, stdout)
		if out["decision"] != "block" || out["reason"] != "needs-item" {
			t.Fatalf("%s = %v/%v, want block/needs-item", cp, out["decision"], out["reason"])
		}
		if ledgerStoreBytes(t, storePath) != nil {
			t.Fatalf("%s: grading needs-item created a store file", cp)
		}
	}

	// With an item present and no disagreement, grading leaves the bytes alone.
	storePath = saveLedgerStore(t, common, branch, "card-local", nil)
	before := ledgerStoreBytes(t, storePath)
	beforeMtime := ledgerStoreMtime(t, storePath)
	if code, _, stderr := runCLI(t, "--ledger", "--checkpoint", "apply-start", "--ledger-mode", "always", "--project-root", dir); code != 0 {
		t.Fatalf("consistent grade exit = %d, want 0; stderr: %s", code, stderr)
	}
	if string(before) != string(ledgerStoreBytes(t, storePath)) {
		t.Fatal("a consistent grade must not rewrite the store")
	}
	if !ledgerStoreMtime(t, storePath).Equal(beforeMtime) {
		t.Fatal("a consistent grade must not touch the store at all")
	}
}

// TestSelftestExercisesTheWriteSurface pins task 1.7: the offline self-check covers
// the write-request rules and the open-if-absent invariant, and the binary's
// existing success marker is unchanged.
func TestSelftestExercisesTheWriteSurface(t *testing.T) {
	if err := ledgerWriteSelftest(); err != nil {
		t.Fatalf("ledgerWriteSelftest: %v", err)
	}
	code, stdout, stderr := runCLI(t, "--selftest")
	if code != 0 {
		t.Fatalf("--selftest exit = %d, want 0; stderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("--selftest stdout = %q, want ok", stdout)
	}
}

// TestWorktreeExplainUnchangedWithoutLedger pins that the worktree --explain
// output shape is untouched by the ledger mode.
func TestWorktreeExplainUnchangedWithoutLedger(t *testing.T) {
	code, stdout, _ := runCLI(t, "--explain")
	if code != 0 {
		t.Fatalf("--explain exit = %d, want 0", code)
	}
	out := decodeLedgerOut(t, stdout)
	if _, ok := out["capability"]; ok {
		t.Fatalf("worktree --explain leaked a ledger capability key: %v", out)
	}
	for _, key := range []string{"gate_mode", "decision", "candidates"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("worktree --explain missing %q: %v", key, out)
		}
	}
}

// TestLedgerReconcileRejectedWithMutationFlags pins F4: reconciliation is a
// read-only sidecar, so combining it with a mutation vehicle is refused before
// any IO. Both payloads below would otherwise mutate the store.
func TestLedgerReconcileRejectedWithMutationFlags(t *testing.T) {
	obs := writeReconcileObs(t, reconcileObsBody(t, nil))
	cases := []struct {
		name string
		flag []string
	}{
		{"write", []string{"--write", `{"kind":"link","item_id":"6aa703fdcf61a90ec702d58b"}`}},
		{"decide", []string{"--decide", `{"choice":"local"}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, common, branch := ledgerRepo(t)
			writeLedgerWitness(t, common, "bound", reconcileRecipeID)
			storePath := saveLedgerStore(t, common, branch, "card-1", nil)
			before := ledgerStoreBytes(t, storePath)

			args := append(reconcileRunArgs(dir, obs, "delivery"), tc.flag...)
			code, stdout, stderr := runCLI(t, args...)
			if code != 2 {
				t.Fatalf("--reconcile with %s exit = %d, want 2; stderr: %s", tc.name, code, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want no JSON when the invocation is refused", stdout)
			}
			if !strings.Contains(stderr, "--reconcile") {
				t.Fatalf("stderr = %q, want the refusal to name --reconcile", stderr)
			}
			if string(before) != string(ledgerStoreBytes(t, storePath)) {
				t.Fatalf("a refused combination must persist nothing (%s)", tc.name)
			}
		})
	}
}
