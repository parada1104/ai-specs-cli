package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// cliServedManifest enables every agent the F1 renderers cover and the
// worktree-flow recipe whose gate hooks the CLI serves.
const cliServedManifest = `[project]
name = 'cli-served'
[brief]
render = false
[agents]
enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']

[recipes.worktree-flow]
enabled = true
[recipes.worktree-flow.config]
gate_mode = 'ask'
integration_branch = 'development'
worktrees_dir = '.wt'
`

const cliServedLauncher = "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"

func runCLIServedSync(t *testing.T, home, project string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	rc := Run([]string{"sync", project}, home, strings.NewReader(""), &stdout, &stderr)
	return rc, stdout.String(), stderr.String()
}

// S2/T1: sync records the runtime config the CLI verbs read, exactly the values
// the retired launcher was stamped with.
func TestSyncWritesRuntimeSnapshot(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	snapshot, found, err := projectcache.ReadRuntimeSnapshot(project, home)
	if err != nil || !found {
		t.Fatalf("snapshot: found=%v err=%v", found, err)
	}
	want := projectcache.RuntimeSnapshot{
		GateMode:          "ask",
		GateScope:         "auto",
		RepoTopology:      "auto",
		Protected:         "main development",
		WorktreesDir:      ".wt",
		IntegrationBranch: "development",
	}
	if snapshot != want {
		t.Fatalf("snapshot = %+v, want %+v", snapshot, want)
	}
	// A second sync is idempotent.
	if rc, _, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("resync rc=%d %s", rc, stderr)
	}
	if again, _, _ := projectcache.ReadRuntimeSnapshot(project, home); again != want {
		t.Fatalf("resync snapshot = %+v", again)
	}
}

// S1/T2: no .sh launcher is materialized and every renderer wires the CLI verb.
func TestSyncServesWorktreeGateFromCLI(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, cliServedLauncher)); !os.IsNotExist(err) {
		t.Fatalf("launcher must not be materialized: %v", err)
	}
	lk, err := lock.LoadLock(filepath.Join(project, "ai-specs/.ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lk.Managed[cliServedLauncher] != nil {
		t.Fatalf("launcher lock record must be gone: %v", lk.Managed[cliServedLauncher])
	}
	command := "worktree-gate --hook worktree-gate"
	for _, rel := range []string{
		".claude/settings.json",
		".cursor/hooks/worktree-flow-worktree-gate-shell.sh",
		".opencode/plugin/worktree-flow-worktree-gate.ts",
		".pi/extensions/worktree-flow-worktree-gate.ts",
		".omp/extensions/worktree-flow-worktree-gate.ts",
	} {
		body := readFileString(t, filepath.Join(project, filepath.FromSlash(rel)))
		if !strings.Contains(body, command) {
			t.Errorf("%s does not wire %q:\n%s", rel, command, body)
		}
		if !strings.Contains(body, "/dist/ai-specs") {
			t.Errorf("%s does not resolve the installed CLI:\n%s", rel, body)
		}
		if strings.Contains(body, "worktree-gate.sh") {
			t.Errorf("%s still references the retired launcher:\n%s", rel, body)
		}
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// S1/T2: the launcher an older sync materialized is pruned, and a user-edited
// copy is removed with one notice instead of being preserved.
func TestSyncPrunesRetiredGateLauncher(t *testing.T) {
	for _, tc := range []struct {
		name       string
		edit       string
		wantNotice bool
	}{
		{name: "managed copy", edit: "", wantNotice: false},
		{name: "user-modified copy", edit: "# locally edited\n", wantNotice: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := spineEnv(t)
			project := spineProject(t, cliServedManifest)
			launcher := filepath.Join(project, filepath.FromSlash(cliServedLauncher))
			body := "#!/bin/sh\nexit 0\n"
			spineWrite(t, launcher, body, 0o755)
			lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
			lk, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			lock.SetGateBaseline(lk, cliServedLauncher, lock.Sha256Bytes([]byte(body)), strPtr("worktree-flow"), strPtr("hooks/worktree-gate.sh"))
			if err := lock.WriteLock(lockPath, lk); err != nil {
				t.Fatal(err)
			}
			if tc.edit != "" {
				spineWrite(t, launcher, tc.edit, 0o755)
			}
			rc, stdout, stderr := runCLIServedSync(t, home, project)
			if rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			if _, err := os.Lstat(launcher); !os.IsNotExist(err) {
				t.Fatalf("retired launcher must be removed: %v", err)
			}
			after, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if after.Managed[cliServedLauncher] != nil {
				t.Fatalf("lock record survived: %v", after.Managed[cliServedLauncher])
			}
			noticed := strings.Contains(stderr, "removed user-modified worktree-gate launcher "+cliServedLauncher)
			if noticed != tc.wantNotice {
				t.Fatalf("notice = %v, want %v; stderr=%q", noticed, tc.wantNotice, stderr)
			}
		})
	}
}

func strPtr(value string) *string { return &value }

// T2: every renderer wires the resolved entry's CLI command, never a script
// path, and the generated adapter still owns the harness decision channel.
func TestRenderHooksWiresCLICommandPerAgent(t *testing.T) {
	const command = "ai-specs worktree-gate --hook worktree-gate"
	resolved := `{"hooks":[{"recipe":"worktree-flow","id":"worktree-gate","event":"pre-tool-use",` +
		`"matcher":"Bash","blocking":true,"command":"` + command + `","env":{"WORKTREE_GATE_PROTECTED":"main development"}}]}`
	artifacts := map[string]string{
		"claude":   ".claude/settings.json",
		"cursor":   ".cursor/hooks/worktree-flow-worktree-gate.sh",
		"opencode": ".opencode/plugin/worktree-flow-worktree-gate.ts",
		"pi":       ".pi/extensions/worktree-flow-worktree-gate.ts",
		"omp":      ".omp/extensions/worktree-flow-worktree-gate.ts",
	}
	for agent, rel := range artifacts {
		t.Run(agent, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "resolved-hooks.json")
			if err := os.WriteFile(path, []byte(resolved), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if rc := RenderHooks(path, agent, dir, &stdout, &stderr); rc != 0 {
				t.Fatalf("render rc=%d stderr=%s", rc, stderr.String())
			}
			body := readFileString(t, filepath.Join(dir, filepath.FromSlash(rel)))
			if !strings.Contains(body, command) {
				t.Fatalf("%s does not wire %q:\n%s", rel, command, body)
			}
			if strings.Contains(body, "script_path") {
				t.Fatalf("%s still references a script path:\n%s", rel, body)
			}
			// R1-project-policy-binding: the root comes from the runtime.
			want := map[string]string{"claude": `AI_SPECS_PROJECT_ROOT=\"$CLAUDE_PROJECT_DIR\"`, "cursor": `AI_SPECS_PROJECT_ROOT="$CURSOR_PROJECT_DIR"`}[agent]
			if want == "" {
				want = "AI_SPECS_PROJECT_ROOT: PROJECT_ROOT"
			}
			if !strings.Contains(body, want) {
				t.Fatalf("%s does not bind %q:\n%s", rel, want, body)
			}
			// The block decision channel is unchanged per agent.
			switch agent {
			case "claude":
				if !strings.Contains(body, `"type": "command"`) {
					t.Fatalf("%s lost its direct command hook:\n%s", rel, body)
				}
			case "cursor":
				if !strings.Contains(body, `"permission":"deny"`) || !strings.Contains(body, "run_hook") {
					t.Fatalf("%s lost its deny mapping:\n%s", rel, body)
				}
			case "opencode":
				if !strings.Contains(body, "throw new Error(res.stderr") {
					t.Fatalf("%s lost its throw channel:\n%s", rel, body)
				}
			default:
				if !strings.Contains(body, "return { block: true, reason: res.stderr") {
					t.Fatalf("%s lost its block return:\n%s", rel, body)
				}
			}
		})
	}
}

// R1-002/R3-001: the snapshot is the config sync actually applied, so a run
// that fails after materialization must leave the previous one in place — a
// failed `off` sync cannot quietly disarm the gate.
func TestSyncFailedRunKeepsPreviousSnapshot(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, strings.Replace(cliServedManifest, "gate_mode = 'ask'", "gate_mode = 'off'", 1))
	previous := projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main development", WorktreesDir: ".wt", IntegrationBranch: "development",
	}
	if _, err := projectcache.WriteRuntimeSnapshot(project, home, previous); err != nil {
		t.Fatal(err)
	}
	// A later step fails after the recipe materialization: `.git` is not a
	// directory, so the post-merge template cannot be written.
	spineWrite(t, filepath.Join(project, ".git"), "not a directory\n", 0o644)
	rc, stdout, stderr := runCLIServedSync(t, home, project)
	if rc == 0 {
		t.Fatalf("sync with an unwritable template target must fail\n%s\n%s", stdout, stderr)
	}
	snapshot, found, err := projectcache.ReadRuntimeSnapshot(project, home)
	if err != nil || !found {
		t.Fatalf("snapshot: found=%v err=%v", found, err)
	}
	if snapshot != previous {
		t.Fatalf("snapshot = %+v, want the previous %+v", snapshot, previous)
	}
}

// R1-upgrade-enforcement-gap: with no previous snapshot to preserve, the first
// sync after the upgrade publishes it BEFORE the prune, so a later failure still
// leaves the gate enforced instead of failing open.
func TestSyncFirstRunPublishesSnapshotBeforePrune(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	// The fan-out fails after the prune: this unlocked path is a directory.
	if err := os.MkdirAll(filepath.Join(project, ".pi/extensions/worktree-flow-worktree-gate.ts"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc, stdout, stderr := runCLIServedSync(t, home, project)
	if rc == 0 {
		t.Fatalf("sync must fail\n%s\n%s", stdout, stderr)
	}
	snapshot, found, err := projectcache.ReadRuntimeSnapshot(project, home)
	if err != nil || !found || snapshot.GateMode != "ask" || snapshot.WorktreesDir != ".wt" {
		t.Fatalf("snapshot = %+v found=%v err=%v, want the resolved policy", snapshot, found, err)
	}
}
