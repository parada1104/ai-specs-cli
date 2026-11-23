package sync

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
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
	// The id is shell-quoted; the two hooks are worktree-gate and -shell.
	command := "worktree-gate --hook 'worktree-gate"
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
			// S6: the adapter names its runtime, so the CLI can shape the
			// fail-open message for that runtime's allow-with-message channel.
			agentExport := map[string]string{
				"claude":   "export AI_SPECS_HOOK_AGENT=claude",
				"cursor":   "export AI_SPECS_HOOK_AGENT=cursor",
				"pi":       `AI_SPECS_HOOK_AGENT: "pi"`,
				"omp":      `AI_SPECS_HOOK_AGENT: "omp"`,
				"opencode": "", // opencode has no allow-with-message channel
			}[agent]
			if agentExport != "" && !strings.Contains(body, agentExport) {
				t.Fatalf("%s does not name its runtime (%q):\n%s", rel, agentExport, body)
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

// fakeCLI installs a stand-in for the installed CLI at $AI_SPECS_HOME/dist/ai-specs
// that records its argv and replies with the CLI's fail-open no-snapshot output.
func fakeCLI(t *testing.T, home, stdout string, rc int) string {
	t.Helper()
	args := filepath.Join(home, "argv.txt")
	spineWrite(t, filepath.Join(home, "dist", "ai-specs"),
		"#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+args+"\"\nprintf '%s' \""+stdout+"\"\nexit "+strconv.Itoa(rc)+"\n", 0o755)
	return args
}

// S6/R4-nosnapshot-message-channel (cursor): the generated wrapper turns the
// CLI's exit-0 stdout into Cursor's agent_message and still allows the action.
func TestCLIServedCursorWrapperDeliversMessageOnAllow(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	message := "worktree-gate: /tmp/project has no ai-specs sync snapshot"
	cliHome := t.TempDir()
	argv := fakeCLI(t, cliHome, message, 0)

	wrapper := filepath.Join(project, ".cursor", "hooks", "worktree-flow-worktree-gate-shell.sh")
	cmd := exec.Command("bash", wrapper)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "AI_SPECS_HOME="+cliHome, "CURSOR_PROJECT_DIR="+project)
	cmd.Stdin = strings.NewReader(`{"tool_name":"Shell","tool_input":{"command":"git commit"}}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run wrapper: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	want := `{"permission":"allow","agent_message":"` + message + `"}`
	if stdout.String() != want {
		t.Fatalf("wrapper stdout = %q, want %q", stdout.String(), want)
	}
	// The adapter still runs the CLI hook with its agent flag, never blocking.
	if recorded := readFileString(t, argv); recorded != "worktree-gate\n--hook\nworktree-gate-shell\n" {
		t.Fatalf("CLI argv = %q", recorded)
	}
}

// S6/R4-nosnapshot-message-channel (pi/omp): the generated extension hands the
// CLI's exit-0 stdout to the model and returns nothing, so the call proceeds.
func TestCLIServedPiExtensionDeliversMessageOnAllow(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	for agent, rel := range map[string]string{
		"pi":  ".pi/extensions/worktree-flow-worktree-gate-shell.ts",
		"omp": ".omp/extensions/worktree-flow-worktree-gate-shell.ts",
	} {
		t.Run(agent, func(t *testing.T) {
			body := readFileString(t, filepath.Join(project, filepath.FromSlash(rel)))
			if !strings.Contains(body, `AI_SPECS_HOOK_AGENT: "`+agent+`"`) {
				t.Fatalf("%s does not pass the runtime to the CLI:\n%s", rel, body)
			}
			want := "    if (res.status === 2) {\n" +
				"      return { block: true, reason: res.stderr || \"blocked by ai-specs runtime hook\" };\n" +
				"    }\n" +
				"    if (res.status === 0 && res.stdout) {\n" +
				"      // Delivery is best effort: the gate already allowed the edit.\n" +
				"      try { void Promise.resolve(pi.sendMessage({ customType: \"ai-specs-worktree-gate\", content: res.stdout.trim(), display: false })).catch(() => {}); } catch {}\n" +
				"    }\n"
			if !strings.Contains(body, want) {
				t.Fatalf("%s lost its allow-with-message channel:\n%s", rel, body)
			}
		})
	}
}

// S1/S6: the CLI-served command quotes the hook id, so an id with spaces or
// shell metacharacters reaches the CLI as one argument instead of shell source.
func TestCLIServedHookCommandQuotesHookID(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	const unsafeID = "worktree ' gate; printf unexpected; #"
	hook := &schema.RuntimeHook{ID: unsafeID, Script: "hooks/worktree-gate.sh"}
	command := cliServedHookCommand("worktree-flow", hook)
	if command == "" {
		t.Fatal("worktree-flow worktree-gate hook must be CLI-served")
	}
	cliHome := t.TempDir()
	argv := fakeCLI(t, cliHome, "", 0)
	// Run the rendered command from a script file (not `bash -c`) so the
	// generated quoting is what parses the id.
	script := filepath.Join(t.TempDir(), "run-hook.sh")
	spineWrite(t, script, "#!/bin/sh\n"+command+"\n", 0o755)
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "AI_SPECS_HOME="+cliHome)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run command: %v\nstderr: %s", err, stderr.String())
	}
	if recorded := readFileString(t, argv); recorded != "worktree-gate\n--hook\n"+unsafeID+"\n" {
		t.Fatalf("CLI argv = %q, want the id as one argument", recorded)
	}
}
