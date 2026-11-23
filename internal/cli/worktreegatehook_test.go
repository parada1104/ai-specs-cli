package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// runHookCLI drives `ai-specs worktree-gate --hook <id>` from a project cwd.
func runHookCLI(t *testing.T, cwd, home, input string) (int, string, string) {
	t.Helper()
	t.Chdir(cwd)
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

// R1-001/R4-snapshot-root-lookup: the gate judges the EVENT's cwd, so the
// snapshot lookup must use it too. A hook launched from an unrelated directory
// still has to find the snapshot of the project the event targets.
func TestWorktreeGateHookUsesEventProject(t *testing.T) {
	repo, home, launchDir := newGateFixture(t), t.TempDir(), t.TempDir()
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main", WorktreesDir: ".worktrees", IntegrationBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf(`{"tool_name":"Write","cwd":%q,"tool_input":{"file_path":"README.md"}}`, repo)
	code, stdout, stderr := runHookCLI(t, launchDir, home, input)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "refusing to Write") {
		t.Fatalf("hook = (%d, %q, %q), want blocked (2, blocked message)", code, stdout, stderr)
	}
}

// The same lookup walks UP from the event cwd: a write inside a subdirectory of
// the synced project (an ai-specs.toml below the git toplevel, or a submodule)
// is judged by that project's snapshot.
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
	code, stdout, stderr := runHookCLI(t, launchDir, home, input)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "refusing to Write") {
		t.Fatalf("hook = (%d, %q, %q), want blocked (2, blocked message)", code, stdout, stderr)
	}
}
