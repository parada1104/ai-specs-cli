package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RED contract suite for strangler slice 7 (GO-10): the Go hook/gate actuator
// (`worktree-gate --materialize-hook`) that will replace the Python
// materialize_hook_script path in lib/_internal/recipe-materialize.py:959-1245
// (8 hook placeholders, managed-override state machine, refresh rollback).
//
// The symbols referenced here (runMaterializeHook, hookScriptRelPath,
// renderHookGateContent) do not exist yet, so this package intentionally fails
// to compile until the implementation work unit lands hookgateactuator.go.
// These tests ARE the contract: the implementation must make them pass
// byte-for-byte and string-for-string, and must REUSE the already-ported
// classifyManagedOverride pure core (classify.go) for the state machine — the
// ownership classification is never re-ported in slice 7 (single shared port
// with slice 6, util.py:651/808 via --plan-classify).
//
// Bridge contract (fail-open, matching GO_HOOK_GATE_BRIDGE_FALLBACK): one
// JSON envelope on stdin, one JSON envelope on stdout, exit 0/2. Exit 2 or
// any bridge failure makes the Python bridge warn once with the fallback
// token and run the historical Python path; Go never escalates the failure.
//
// Division of labor (mirrors the merged slices): Go owns rendering, actuation
// (write + chmod 0755), and the refresh backup/rollback. Python keeps the
// lock I/O (the record payload is handed back, never written by Go), the
// warning/message printing, the backup-path precomputation (project-cache
// ownership), and the gate-version resolution (the envelope carries the
// resolved version; "dev" fallback is Python's).

// hookGateInput mirrors the --materialize-hook stdin contract.
//   - Script is the recipe-relative hook.script (the schema guarantees it is
//     relative and inside the recipe directory; the actuator only reads it).
//   - Config carries the merged recipe config (only gate_mode, gate_scope,
//     repo_topology, and gate_impl are read).
//   - GateVersion is the Python-resolved installed version ("dev" fallback
//     already applied); Go substitutes it verbatim.
//   - ManagedEntry is the lock's managed[rel] subset — only sha256 is read;
//     nil means no recorded provenance.
//   - BackupPath is the Python-precomputed immutable snapshot path for a
//     refresh (empty when the destination does not exist yet).
type hookGateInput struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Script       string                `json:"script"`
	Config       map[string]any        `json:"config,omitempty"`
	CLIHome      string                `json:"cli_home,omitempty"`
	GateVersion  string                `json:"gate_version,omitempty"`
	Refresh      bool                  `json:"refresh,omitempty"`
	ManagedEntry *hookGateManagedEntry `json:"managed_entry,omitempty"`
	BackupPath   string                `json:"backup_path,omitempty"`
}

// hookGateManagedEntry is the subset of [managed.<path>] lock metadata the
// actuator reads (same shape as classifyManagedEntry).
type hookGateManagedEntry struct {
	SHA256 string `json:"sha256"`
}

// hookGateRecord is the lock payload the Python bridge must hand to
// lock.set_gate_baseline: target is the project-relative rel path, sha256 is
// the normalized (CRLF-folded) sha of the bytes the actuation produced, kind
// is always "gate", policy always "auto" (gates follow the auto update
// policy). A nil record means "write nothing to the lock".
type hookGateRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// hookGateOutput is the --materialize-hook stdout contract.
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
type hookGateOutput struct {
	Rel      string          `json:"rel"`
	Dest     string          `json:"dest"`
	Wrote    bool            `json:"wrote"`
	Record   *hookGateRecord `json:"record"`
	Message  string          `json:"message"`
	Warnings []string        `json:"warnings"`
	Backup   string          `json:"backup,omitempty"`
	Error    *string         `json:"error"`
}

// fixture body carries the gate-mode token, so the rendered content differs
// from the source bytes (this is what makes stale/user-modified states
// distinguishable in the end-to-end tests).
const hookFixtureBody = "#!/bin/sh\n# gate_mode=__WORKTREE_GATE_MODE__\n"
const hookFixtureRendered = "#!/bin/sh\n# gate_mode=always\n"

// writeHookSource writes the recipe source hook and returns a standard input
// (worktree-flow, script hooks/gate.sh, default config, version "dev").
func writeHookSource(t *testing.T, body string) hookGateInput {
	t.Helper()
	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	recipeDir := filepath.Join(base, "catalog", "recipes", "worktree-flow")
	if err := os.MkdirAll(filepath.Join(recipeDir, "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir recipe dir: %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	src := filepath.Join(recipeDir, "hooks", "gate.sh")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return hookGateInput{
		ProjectRoot: projectRoot,
		RecipeDir:   recipeDir,
		RecipeID:    "worktree-flow",
		Script:      "hooks/gate.sh",
		Config: map[string]any{
			"gate_mode":     "ask",
			"gate_scope":    "auto",
			"repo_topology": "auto",
			"gate_impl":     "auto",
		},
		GateVersion: "dev",
	}
}

func runHookGateActuatorCLI(t *testing.T, in hookGateInput) (int, hookGateOutput, string) {
	t.Helper()
	payload, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runMaterializeHook(bytes.NewReader(payload), &stdout, &stderr)
	var out hookGateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %q: %v", stdout.String(), err)
	}
	return code, out, stderr.String()
}

func hookFixtureRel(in hookGateInput) string {
	return fmt.Sprintf("ai-specs/recipes/%s/hooks/gate.sh", in.RecipeID)
}

func hookFixtureDest(in hookGateInput) string {
	return filepath.Join(in.ProjectRoot, filepath.FromSlash(hookFixtureRel(in)))
}

func wantGateRecord(in hookGateInput, sha string) *hookGateRecord {
	return &hookGateRecord{
		Target: hookFixtureRel(in),
		SHA256: sha,
		Recipe: in.RecipeID,
		Source: in.Script,
		Kind:   "gate",
		Policy: "auto",
	}
}

// --- pure unit tests: rel path and rendering, no CLI envelope ---

// TestHookScriptRelPath pins the harness-neutral materialized path: fixed
// ai-specs/recipes/{recipe_id}/hooks/ prefix with the script BASENAME, so
// nested recipe scripts flatten into the hook directory.
func TestHookScriptRelPath(t *testing.T) {
	cases := []struct{ recipeID, script, want string }{
		{"worktree-flow", "hooks/gate.sh", "ai-specs/recipes/worktree-flow/hooks/gate.sh"},
		{"trello-mcp-workflow", "scripts/tracker-card-gate.sh", "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"},
		{"plain", "gate.sh", "ai-specs/recipes/plain/hooks/gate.sh"},
	}
	for _, tc := range cases {
		if got := hookScriptRelPath(tc.recipeID, tc.script); got != tc.want {
			t.Errorf("hookScriptRelPath(%q, %q) = %q, want %q", tc.recipeID, tc.script, got, tc.want)
		}
	}
}

// TestRenderHookGateContentGateMode pins the two gate-mode tokens: defaults
// "always"/"warn" with a nil config or a missing key, the configured value
// otherwise, and a tokenless body left untouched.
func TestRenderHookGateContentGateMode(t *testing.T) {
	src := "a=__WORKTREE_GATE_MODE__\nb=__TRACKER_CARD_GATE_MODE__\n"
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"nil config", nil, "a=always\nb=warn\n"},
		{"missing keys", map[string]any{}, "a=always\nb=warn\n"},
		{"configured", map[string]any{"gate_mode": "ask"}, "a=ask\nb=ask\n"},
	}
	for _, tc := range cases {
		got, err := renderHookGateContent([]byte(src), tc.cfg, "", "dev")
		if err != nil {
			t.Fatalf("%s: render: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got, err := renderHookGateContent([]byte("plain\n"), map[string]any{"gate_mode": "ask"}, "", "dev"); err != nil || got != "plain\n" {
		t.Errorf("tokenless body modified: %q (err %v)", got, err)
	}
}

// TestRenderHookGateContentScope pins __WORKTREE_GATE_SCOPE__: default "auto"
// for a nil config, a missing key, or an EMPTY value (Python's `or default`),
// the configured value otherwise.
func TestRenderHookGateContentScope(t *testing.T) {
	src := []byte("scope=__WORKTREE_GATE_SCOPE__\n")
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"nil config", nil, "scope=auto\n"},
		{"missing key", map[string]any{}, "scope=auto\n"},
		{"empty value", map[string]any{"gate_scope": ""}, "scope=auto\n"},
		{"superrepo", map[string]any{"gate_scope": "superrepo"}, "scope=superrepo\n"},
		{"subrepo", map[string]any{"gate_scope": "subrepo"}, "scope=subrepo\n"},
	}
	for _, tc := range cases {
		got, err := renderHookGateContent(src, tc.cfg, "", "dev")
		if err != nil {
			t.Fatalf("%s: render: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRenderHookGateContentScopeInvalid pins the exact refusal message.
func TestRenderHookGateContentScopeInvalid(t *testing.T) {
	_, err := renderHookGateContent([]byte("scope=__WORKTREE_GATE_SCOPE__\n"), map[string]any{"gate_scope": "bogus"}, "", "dev")
	if err == nil || err.Error() != "invalid gate_scope 'bogus'; allowed: auto | superrepo | subrepo" {
		t.Errorf("err = %v, want the exact gate_scope refusal", err)
	}
}

// TestRenderHookGateContentTopology pins __WORKTREE_REPO_TOPOLOGY__: default
// "auto" (nil config / missing / empty) plus the exact refusal message.
func TestRenderHookGateContentTopology(t *testing.T) {
	src := []byte("topology=__WORKTREE_REPO_TOPOLOGY__\n")
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"nil config", nil, "topology=auto\n"},
		{"missing key", map[string]any{}, "topology=auto\n"},
		{"empty value", map[string]any{"repo_topology": ""}, "topology=auto\n"},
		{"standalone", map[string]any{"repo_topology": "standalone"}, "topology=standalone\n"},
		{"monorepo-submodules", map[string]any{"repo_topology": "monorepo-submodules"}, "topology=monorepo-submodules\n"},
	}
	for _, tc := range cases {
		got, err := renderHookGateContent(src, tc.cfg, "", "dev")
		if err != nil {
			t.Fatalf("%s: render: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
	if _, err := renderHookGateContent(src, map[string]any{"repo_topology": "monorepo"}, "", "dev"); err == nil ||
		err.Error() != "invalid repo_topology 'monorepo'; allowed: auto | standalone | monorepo-apps | monorepo-submodules" {
		t.Errorf("err = %v, want the exact repo_topology refusal", err)
	}
}

// TestRenderHookGateContentImpl pins __WORKTREE_GATE_IMPL__: default "auto"
// (nil config / missing / empty), "go" allowed, and the historical refusal
// that names the removed bash implementation.
func TestRenderHookGateContentImpl(t *testing.T) {
	src := []byte("impl=__WORKTREE_GATE_IMPL__\n")
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"nil config", nil, "impl=auto\n"},
		{"missing key", map[string]any{}, "impl=auto\n"},
		{"empty value", map[string]any{"gate_impl": ""}, "impl=auto\n"},
		{"go", map[string]any{"gate_impl": "go"}, "impl=go\n"},
	}
	for _, tc := range cases {
		got, err := renderHookGateContent(src, tc.cfg, "", "dev")
		if err != nil {
			t.Fatalf("%s: render: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
	if _, err := renderHookGateContent(src, map[string]any{"gate_impl": "bash"}, "", "dev"); err == nil ||
		err.Error() != "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go" {
		t.Errorf("err = %v, want the exact gate_impl refusal", err)
	}
}

// TestRenderHookGateContentVersion pins that the version is substituted
// verbatim: Python owns the installed-version resolution and the "dev"
// fallback; Go never invents a value.
func TestRenderHookGateContentVersion(t *testing.T) {
	src := []byte("v=__WORKTREE_GATE_VERSION__\n")
	if got, err := renderHookGateContent(src, nil, "", "dev"); err != nil || got != "v=dev\n" {
		t.Errorf("render = %q (err %v), want v=dev", got, err)
	}
	if got, err := renderHookGateContent(src, nil, "", "1.2.3"); err != nil || got != "v=1.2.3\n" {
		t.Errorf("render = %q (err %v), want v=1.2.3", got, err)
	}
}

// TestRenderHookGateContentTrackerHomes pins the tracker bridge tokens:
// absolute resolved paths when a CLI home is given, EMPTY strings when it is
// not (empty makes the host skip evidence acquisition and the tracker.none
// write — fail open).
func TestRenderHookGateContentTrackerHomes(t *testing.T) {
	home := t.TempDir()
	src := []byte("home=__TRACKER_CLI_HOME__\ninternal=__TRACKER_LIB_INTERNAL__\n")
	got, err := renderHookGateContent(src, nil, home, "dev")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := fmt.Sprintf("home=%s\ninternal=%s\n", home, filepath.Join(home, "lib", "_internal"))
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	got, err = renderHookGateContent(src, nil, "", "dev")
	if err != nil {
		t.Fatalf("render (no home): %v", err)
	}
	if got != "home=\ninternal=\n" {
		t.Errorf("render without home = %q, want empty substitutions", got)
	}
}

// --- end-to-end actuator tests through the CLI contract ---

// TestHookGateActuatorFreshWrite materializes into a missing destination:
// rendered bytes on disk, mode 0755 (source mode is never copied), the exact
// gate record payload, and the "✓ hook script" message.
func TestHookGateActuatorFreshWrite(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	dest := hookFixtureDest(in)
	if out.Rel != hookFixtureRel(in) {
		t.Errorf("rel = %q, want %q", out.Rel, hookFixtureRel(in))
	}
	if out.Dest != dest {
		t.Errorf("dest = %q, want %q", out.Dest, dest)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want true")
	}
	if out.Record == nil || *out.Record != *wantGateRecord(in, sha256Bytes([]byte(hookFixtureRendered))) {
		t.Errorf("record = %#v, want %#v", out.Record, wantGateRecord(in, sha256Bytes([]byte(hookFixtureRendered))))
	}
	if out.Message != fmt.Sprintf("✓ hook script %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
	if len(out.Warnings) != 0 || out.Backup != "" || out.Error != nil {
		t.Errorf("unexpected extras: warnings %#v backup %q error %#v", out.Warnings, out.Backup, out.Error)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != hookFixtureRendered {
		t.Errorf("dest bytes = %q, want %q", got, hookFixtureRendered)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dest mode = %o, want 755", info.Mode().Perm())
	}
}

// TestHookGateActuatorManagedCurrent: a destination whose bytes match both
// the baseline and the render is skipped with a record-only backfill (the
// second sync of an idempotent pair).
func TestHookGateActuatorManagedCurrent(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	if err := os.WriteFile(dest, []byte(hookFixtureRendered), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	in.ManagedEntry = &hookGateManagedEntry{SHA256: sha256Bytes([]byte(hookFixtureRendered))}
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want skip without rewrite")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(hookFixtureRendered)) {
		t.Errorf("record = %#v, want baseline backfill", out.Record)
	}
	if out.Message != fmt.Sprintf("· hook skipped (current) %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings = %#v, want none", out.Warnings)
	}
}

// TestHookGateActuatorManagedStaleRefresh: the baseline matches the disk
// bytes but the render drifted, so an ordinary sync force-updates the gate
// and re-records.
func TestHookGateActuatorManagedStaleRefresh(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	stale := "#!/bin/sh\n# gate_mode=warn\n"
	if err := os.WriteFile(dest, []byte(stale), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	in.ManagedEntry = &hookGateManagedEntry{SHA256: sha256Bytes([]byte(stale))}
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want force-update")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(hookFixtureRendered)) {
		t.Errorf("record = %#v, want re-recorded render sha", out.Record)
	}
	if out.Message != fmt.Sprintf("✓ hook refreshed (baseline matched) %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != hookFixtureRendered {
		t.Errorf("dest bytes = %q, want refreshed %q", got, hookFixtureRendered)
	}
}

// TestHookGateActuatorUserModifiedPreserved: a user-edited gate is preserved
// byte-for-byte with the exact warn string and no record.
func TestHookGateActuatorUserModifiedPreserved(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	userBytes := "#!/bin/sh\n# user customization\n"
	if err := os.WriteFile(dest, []byte(userBytes), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	in.ManagedEntry = &hookGateManagedEntry{SHA256: sha256Bytes([]byte(userBytes))}
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want preservation")
	}
	if out.Record != nil {
		t.Errorf("record = %#v, want nil", out.Record)
	}
	wantWarn := fmt.Sprintf("hook %s is user-modified; preserving existing bytes. Refresh with:\n  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", out.Rel, out.Rel)
	if len(out.Warnings) != 1 || out.Warnings[0] != wantWarn {
		t.Errorf("warnings = %#v, want [%q]", out.Warnings, wantWarn)
	}
	if out.Message != fmt.Sprintf("· hook skipped (user-modified) %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != userBytes {
		t.Errorf("dest bytes = %q, want untouched %q", got, userBytes)
	}
}

// TestHookGateActuatorNoProvenancePreserved: an existing destination without
// a lock baseline is NEVER seeded (unlike the template actuator) — preserve,
// exact warn string, no record.
func TestHookGateActuatorNoProvenancePreserved(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	if err := os.WriteFile(dest, []byte(hookFixtureRendered), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want preservation (never seed)")
	}
	if out.Record != nil {
		t.Errorf("record = %#v, want nil", out.Record)
	}
	wantWarn := fmt.Sprintf("hook %s has no recorded provenance; preserving existing bytes. A baseline is recorded only when the CLI renders the gate. Refresh with:\n  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", out.Rel, out.Rel)
	if len(out.Warnings) != 1 || out.Warnings[0] != wantWarn {
		t.Errorf("warnings = %#v, want [%q]", out.Warnings, wantWarn)
	}
	if out.Message != fmt.Sprintf("· hook skipped (no provenance) %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
}

// TestHookGateActuatorRefreshBacksUpAndRewrites: --refresh-gates replaces
// even a user-modified gate after its exact pre-refresh bytes land in the
// Python-precomputed immutable backup.
func TestHookGateActuatorRefreshBacksUpAndRewrites(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	prior := "#!/bin/sh\n# user customization\n"
	if err := os.WriteFile(dest, []byte(prior), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	in.Refresh = true
	in.BackupPath = filepath.Join(t.TempDir(), "backups", "relkey", sha256Bytes([]byte(prior))+".sh")
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want refreshed")
	}
	if out.Backup != in.BackupPath {
		t.Errorf("backup = %q, want %q", out.Backup, in.BackupPath)
	}
	backup, err := os.ReadFile(in.BackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != prior {
		t.Errorf("backup bytes = %q, want prior %q", backup, prior)
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(hookFixtureRendered)) {
		t.Errorf("record = %#v, want refreshed render sha", out.Record)
	}
	if out.Message != fmt.Sprintf("✓ hook refreshed %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != hookFixtureRendered {
		t.Errorf("dest bytes = %q, want %q", got, hookFixtureRendered)
	}
}

// TestHookGateActuatorRefreshRollback: a refresh that cannot complete the
// write restores the prior bytes and removes the backup it created (exit 2,
// all-or-nothing, lock never handed a record).
func TestHookGateActuatorRefreshRollback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("write refusal via file mode does not work as root")
	}
	in := writeHookSource(t, hookFixtureBody)
	dest := hookFixtureDest(in)
	prior := "#!/bin/sh\n# user customization\n"
	if err := os.WriteFile(dest, []byte(prior), 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	if err := os.Chmod(dest, 0o400); err != nil {
		t.Fatalf("chmod dest: %v", err)
	}
	in.Refresh = true
	in.BackupPath = filepath.Join(t.TempDir(), "backups", "relkey", sha256Bytes([]byte(prior))+".sh")
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr %q, out %#v)", code, stderr, out)
	}
	if out.Error == nil || !strings.Contains(*out.Error, dest) {
		t.Errorf("error = %#v, want a write-failure message naming %q", out.Error, dest)
	}
	if out.Record != nil {
		t.Errorf("record = %#v, want nil (no lock write after failure)", out.Record)
	}
	if _, err := os.Stat(in.BackupPath); !os.IsNotExist(err) {
		t.Errorf("backup still exists after rollback: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != prior {
		t.Errorf("dest bytes = %q, want restored prior %q", got, prior)
	}
	_ = os.Chmod(dest, 0o755) // cleanup for t.TempDir removal
}

// TestHookGateActuatorRefreshMissingDestNoBackup: a refresh into a missing
// destination has nothing to back up and behaves like a fresh write.
func TestHookGateActuatorRefreshMissingDestNoBackup(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	in.Refresh = true
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if !out.Wrote || out.Backup != "" {
		t.Errorf("wrote = %v backup = %q, want plain write without backup", out.Wrote, out.Backup)
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(hookFixtureRendered)) {
		t.Errorf("record = %#v, want gate baseline", out.Record)
	}
	if out.Message != fmt.Sprintf("✓ hook refreshed %s", out.Rel) {
		t.Errorf("message = %q", out.Message)
	}
}

// TestHookGateActuatorSourceMissing: a missing recipe source refuses with
// exit 2, the exact historical message, and no destination touched.
func TestHookGateActuatorSourceMissing(t *testing.T) {
	in := writeHookSource(t, hookFixtureBody)
	if err := os.Remove(filepath.Join(in.RecipeDir, filepath.FromSlash(in.Script))); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	code, out, stderr := runHookGateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr %q, out %#v)", code, stderr, out)
	}
	want := fmt.Sprintf("hook script not found: %s", filepath.Join(in.RecipeDir, filepath.FromSlash(in.Script)))
	if out.Error == nil || *out.Error != want {
		t.Errorf("error = %#v, want %q", out.Error, want)
	}
	if _, err := os.Stat(hookFixtureDest(in)); !os.IsNotExist(err) {
		t.Errorf("dest created despite refusal: %v", err)
	}
}
