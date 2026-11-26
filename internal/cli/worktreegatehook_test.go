package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// runHookCLI drives `ai-specs worktree-gate --hook <id>` from a project cwd.
// No project root is exported: the verb falls back like an older hook.
func runHookCLI(t *testing.T, cwd, home, input string) (int, string, string) {
	return runHookCLIFor(t, "", cwd, home, input)
}

// runHookCLIFor drives the hook the way a rendered one runs: the runtime
// exports the installing project's root it resolved.
func runHookCLIFor(t *testing.T, projectRoot, cwd, home, input string) (int, string, string) {
	t.Helper()
	return runHookCLIAgent(t, "", projectRoot, cwd, home, input)
}

// runHookCLIAgent drives the hook exactly as the renderer for `agent` wires it.
func runHookCLIAgent(t *testing.T, agent, projectRoot, cwd, home, input string) (int, string, string) {
	t.Helper()
	t.Setenv("AI_SPECS_PROJECT_ROOT", projectRoot)
	t.Chdir(cwd)
	t.Setenv("AI_SPECS_HOOK_AGENT", agent)
	var stdout, stderr strings.Builder
	code := Run([]string{"worktree-gate", "--hook", "worktree-gate"}, home, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// S6: without a sync snapshot the gate allows the edit and hands the agent one
// message that asks it to put the decision to the user.
func TestWorktreeGateHookWithoutSnapshotFailsOpen(t *testing.T) {
	repo := newGateFixture(t)
	home := t.TempDir()
	// A write on the protected branch in the main worktree: the decision the
	// gate would make, which the snapshot-less hook must NOT make.
	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"README.md"}}`
	code, stdout, stderr := runHookCLI(t, repo, home, input)
	if code != 0 || stdout != "" {
		t.Fatalf("hook = (%d, %q), want fail-open (0, empty stdout)", code, stdout)
	}
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(worktreeGateNoSnapshotMessage, resolved) + "\n"
	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
	for _, phrase := range []string{"no ai-specs sync snapshot", "unguarded", "ask_user_choice", "`ai-specs sync`"} {
		if !strings.Contains(stderr, phrase) {
			t.Errorf("message lost %q: %s", phrase, stderr)
		}
	}
}

// S6/R4-nosnapshot-message-channel: the fail-open message must reach the AGENT
// on the channel each runtime reads on exit 0, and must still allow the edit.
func TestWorktreeGateHookDeliversNoSnapshotMessageToAgent(t *testing.T) {
	repo := newGateFixture(t)
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	message := fmt.Sprintf(worktreeGateNoSnapshotMessage, resolved)
	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"README.md"}}`
	claudeJSON := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":`
	if encoded, err := json.Marshal(message); err != nil {
		t.Fatal(err)
	} else {
		claudeJSON += string(encoded) + "}}"
	}
	cases := []struct {
		agent  string
		stdout string // "" means plain text: the message and a newline
	}{
		{agent: "claude", stdout: claudeJSON},
		{agent: "cursor", stdout: message + "\n"},
		{agent: "pi", stdout: message + "\n"},
		{agent: "omp", stdout: message + "\n"},
		{agent: "opencode", stdout: message + "\n"},
		{agent: "", stdout: ""},
	}
	for _, tc := range cases {
		t.Run("agent="+tc.agent, func(t *testing.T) {
			// No snapshot: the gate allows the edit whatever the agent channel.
			code, stdout, stderr := runHookCLIAgent(t, tc.agent, "", repo, t.TempDir(), input)
			if code != 0 {
				t.Fatalf("hook rc=%d, want fail-open 0", code)
			}
			if stdout != tc.stdout {
				t.Fatalf("stdout = %q, want %q", stdout, tc.stdout)
			}
			if stderr != message+"\n" {
				t.Fatalf("stderr = %q, want %q", stderr, message+"\n")
			}
		})
	}
}

// S2/T2: with a snapshot the decision is the runner's, from the snapshot's
// values — identical to the flags the retired launcher stamped.
func TestWorktreeGateHookDecidesFromSnapshot(t *testing.T) {
	input := func(repo string) string {
		return `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"README.md"}}`
	}
	blocked := "worktree-gate: refusing to Write 'README.md' on protected branch 'main' in the main worktree (<repo>). Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write.\n"
	cases := []struct {
		name    string
		mode    string
		wantRc  int
		wantErr func(repo string) string
	}{
		{name: "always blocks", mode: "always", wantRc: 2, wantErr: func(string) string { return blocked }},
		{name: "ask blocks and offers choices", mode: "ask", wantRc: 2, wantErr: func(string) string {
			return "worktree-gate: refusing to Write 'README.md' on protected branch 'main' in the main worktree (<repo>). " +
				"Ask the user where to put this work: (1) create a dedicated worktree (recommended), " +
				"(2) create a feature branch here, or (3) write on the protected branch with the user's explicit override.\n"
		}},
		{name: "off allows", mode: "off", wantRc: 0, wantErr: func(string) string { return "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newGateFixture(t)
			home := t.TempDir()
			if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
				GateMode:          tc.mode,
				GateScope:         "auto",
				RepoTopology:      "auto",
				Protected:         "main development",
				WorktreesDir:      ".worktrees",
				IntegrationBranch: "main",
			}); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := runHookCLI(t, repo, home, input(repo))
			wantStderr := tc.wantErr(repo)
			stdout, stderr = normalizeGateOutput(repo, stdout), normalizeGateOutput(repo, stderr)
			if code != tc.wantRc || stdout != "" || stderr != wantStderr {
				t.Fatalf("hook = (%d, %q, %q), want (%d, %q, %q)", code, stdout, stderr, tc.wantRc, "", wantStderr)
			}
		})
	}
}

// The launcher honoured WORKTREE_GATE_PROTECTED as a per-invocation override;
// the CLI hook keeps that contract.
func TestWorktreeGateHookHonoursProtectedOverride(t *testing.T) {
	repo := newGateFixture(t)
	home := t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main development", WorktreesDir: ".worktrees", IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKTREE_GATE_PROTECTED", "release")
	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"README.md"}}`
	if code, stdout, stderr := runHookCLI(t, repo, home, input); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("hook = (%d, %q, %q), want allowed", code, stdout, stderr)
	}
}

// A linked worktree finds the snapshot sync wrote in the main checkout, so
// working in a worktree never reports a missing snapshot.
func TestWorktreeGateHookFindsMainWorktreeSnapshot(t *testing.T) {
	repo := newGateFixture(t)
	home := t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main development", WorktreesDir: ".worktrees", IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(repo, ".worktrees", "feat")
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "worktree", "add", "-q", "-b", "feat", worktree)
	input := `{"tool_name":"Write","cwd":"` + worktree + `","tool_input":{"file_path":"README.md"}}`
	code, stdout, stderr := runHookCLI(t, worktree, home, input)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("worktree hook = (%d, %q, %q), want allowed (no missing-snapshot nag)", code, stdout, stderr)
	}
}

// R1-project-policy-binding: the snapshot is the INSTALLING project's, whatever
// directory the hook is launched from.
func TestWorktreeGateHookUsesEventProject(t *testing.T) {
	repo, home, launchDir := newGateFixture(t), t.TempDir(), t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main", WorktreesDir: ".worktrees", IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf(`{"tool_name":"Write","cwd":%q,"tool_input":{"file_path":"README.md"}}`, repo)
	code, stdout, stderr := runHookCLIFor(t, repo, launchDir, home, input)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "refusing to Write") {
		t.Fatalf("hook = (%d, %q, %q), want blocked (2, blocked message)", code, stdout, stderr)
	}
}

// A write inside a subdirectory of the synced project (an ai-specs.toml below
// the git toplevel) is judged by that project's snapshot: the event cwd only
// selects the file, never the policy.
func TestWorktreeGateHookUsesNestedEventProject(t *testing.T) {
	repo, home, launchDir := newGateFixture(t), t.TempDir(), t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main", WorktreesDir: ".worktrees", IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "modules", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf(`{"tool_name":"Write","cwd":%q,"tool_input":{"file_path":"README.md"}}`, nested)
	code, stdout, stderr := runHookCLIFor(t, repo, launchDir, home, input)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "refusing to Write") {
		t.Fatalf("hook = (%d, %q, %q), want blocked (2, blocked message)", code, stdout, stderr)
	}
}

// R1-project-policy-binding / R3-001: a nested, separately synced project neither
// disarms the installing project's policy nor lends its own to an unsynced
// nested repository.
func TestWorktreeGateHookBindsToInstallingProject(t *testing.T) {
	repo, home, parentOnly := newGateFixture(t), t.TempDir(), t.TempDir()
	writeSnapshot := func(cliHome, root, mode string) {
		snapshot := projectcache.RuntimeSnapshot{GateMode: mode, GateScope: "auto", RepoTopology: "auto", Protected: "main", WorktreesDir: ".worktrees", IntegrationBranch: "main"}
		if _, err := projectcache.WriteRuntimeSnapshot(root, cliHome, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	nested := filepath.Join(repo, "vendor", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, nested, "init", "-b", "main")
	event := fmt.Sprintf(`{"tool_name":"Write","cwd":%q,"tool_input":{"file_path":"README.md"}}`, nested)
	// The nested project turned the gate off; the outer always still holds.
	writeSnapshot(home, repo, "always")
	writeSnapshot(home, nested, "off")
	if code, _, stderr := runHookCLIFor(t, repo, repo, home, event); code != 2 || !strings.Contains(stderr, "refusing to Write") {
		t.Fatalf("hook = (%d, %q), want the installing project's always", code, stderr)
	}
	// In a cache that knows only the parent, the nested repo gets S6, not `off`.
	writeSnapshot(parentOnly, repo, "off")
	resolved, _ := filepath.EvalSymlinks(nested)
	want := fmt.Sprintf(worktreeGateNoSnapshotMessage, resolved) + "\n"
	if code, _, stderr := runHookCLIFor(t, nested, nested, parentOnly, event); code != 0 || stderr != want {
		t.Fatalf("hook = (%d, %q), want (%d, %q)", code, stderr, 0, want)
	}
}
