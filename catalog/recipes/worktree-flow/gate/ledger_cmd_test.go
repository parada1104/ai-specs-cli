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
