package sync

// CLI-served planning gate (cli-served-recipes T7, S3/F3): sync serves the
// plan-build-flow hook from the `ai-specs plan-build-gate` verb, never from a
// materialized script, and retires the copy an older sync wrote.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// planBuildGateManifest enables every F1 agent and the recipe whose planning
// gate the CLI serves.
const planBuildGateManifest = `[project]
name = 'plan-build-gate'
[brief]
render = false
[agents]
enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']

[recipes.plan-build-flow]
enabled = true
`

const planBuildGateScriptPath = "ai-specs/recipes/plan-build-flow/hooks/plan-build-gate.sh"

// planBuildGateRenderings lists the per-agent files that must carry the verb.
// Cursor is absent on purpose: a file-write matcher has no Cursor target, so the
// renderer skips this hook for it (pre-existing behavior, unrelated to T7).
var planBuildGateRenderings = []string{
	".claude/settings.json",
	".opencode/plugin/*plan-build-gate*",
	".pi/extensions/*plan-build-gate*",
	".omp/extensions/*plan-build-gate*",
}

// planBuildGateFiles resolves the renderings for one project.
func planBuildGateFiles(t *testing.T, project string) []string {
	t.Helper()
	out := []string{}
	for _, pattern := range planBuildGateRenderings {
		matches, err := filepath.Glob(filepath.Join(project, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("no rendered hook for %s", pattern)
		}
		out = append(out, matches...)
	}
	return out
}

// S1/T7: no plan-build-gate.sh is materialized and every renderer wires the CLI
// verb. The recipe reads no run-time config, so it publishes no snapshot.
func TestSyncServesPlanBuildGateFromCLI(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, planBuildGateManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(planBuildGateScriptPath))); !os.IsNotExist(err) {
		t.Fatalf("plan-build-gate.sh must not be materialized: %v", err)
	}
	lk, err := lock.LoadLock(filepath.Join(project, "ai-specs/.ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lk.Managed[planBuildGateScriptPath] != nil {
		t.Fatalf("lock record must be gone: %v", lk.Managed[planBuildGateScriptPath])
	}
	// A plan-build-only project must keep the "no sync snapshot" policy: the
	// worktree gate's S6 notice still applies to it.
	if _, found, err := projectcache.ReadRuntimeSnapshot(project, home); err != nil || found {
		t.Fatalf("snapshot: found=%v err=%v, want none", found, err)
	}
	for _, path := range planBuildGateFiles(t, project) {
		body := readFileString(t, path)
		// The rendered command is the installed-CLI lookup plus the verb; JSON and
		// TS escape the quotes, the cursor wrapper does not.
		if !strings.Contains(strings.ReplaceAll(body, `\`, ""), `exec "$cli" plan-build-gate`) {
			t.Errorf("%s does not wire the plan-build-gate verb:\n%s", path, body)
		}
		if strings.Contains(body, "plan-build-gate.sh") {
			t.Errorf("%s still references the retired script:\n%s", path, body)
		}
	}
}

// S1/T7: the script an older sync materialized is retired — silently when it is
// still the recorded bytes, with one notice when the user edited it.
func TestSyncRetiresPlanBuildGateScript(t *testing.T) {
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
			project := spineProject(t, planBuildGateManifest)
			script := filepath.Join(project, filepath.FromSlash(planBuildGateScriptPath))
			body := "#!/bin/sh\nexit 0\n"
			spineWrite(t, script, body, 0o755)
			lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
			lk, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			lock.SetGateBaseline(lk, planBuildGateScriptPath, lock.Sha256Bytes([]byte(body)),
				strPtr("plan-build-flow"), strPtr("hooks/plan-build-gate.sh"))
			if err := lock.WriteLock(lockPath, lk); err != nil {
				t.Fatal(err)
			}
			if tc.edit != "" {
				spineWrite(t, script, tc.edit, 0o755)
			}
			rc, stdout, stderr := runCLIServedSync(t, home, project)
			if rc != 0 {
				t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
			}
			if _, err := os.Lstat(script); !os.IsNotExist(err) {
				t.Fatalf("retired script must be removed: %v", err)
			}
			after, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if after.Managed[planBuildGateScriptPath] != nil {
				t.Fatalf("lock record survived: %v", after.Managed[planBuildGateScriptPath])
			}
			notice := "removed user-modified plan-build-gate launcher " + planBuildGateScriptPath
			if noticed := strings.Contains(stderr, notice); noticed != tc.wantNotice {
				t.Fatalf("notice = %v, want %v; stderr=%q", noticed, tc.wantNotice, stderr)
			}
		})
	}
}

// planBuildGateClaudeCommand returns Claude's rendered command for the hook.
func planBuildGateClaudeCommand(t *testing.T, project string) string {
	t.Helper()
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, filepath.Join(project, ".claude", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	for _, entries := range doc.Hooks {
		for _, entry := range entries {
			for _, hook := range entry.Hooks {
				if strings.Contains(hook.Command, "plan-build-gate") {
					return hook.Command
				}
			}
		}
	}
	t.Fatal("no rendered command for plan-build-gate")
	return ""
}

// R1-tracker-cli-provenance: the rendered planning hook pins the installation
// that ran sync, so an agent started without AI_SPECS_HOME — or with a
// different ai-specs on PATH — still grades through it.
func TestPlanBuildGateHookBindsSyncTimeCLIHome(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, planBuildGateManifest)
	t.Setenv("AI_SPECS_HOME", home)
	scratch := t.TempDir()
	marker := filepath.Join(scratch, "marker")
	spineWrite(t, filepath.Join(home, "dist", "ai-specs"), "#!/bin/sh\nprintf 'sync-time' > "+marker+"\n", 0o755)
	if rc, _, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d %s", rc, stderr)
	}
	command := planBuildGateClaudeCommand(t, project)
	decoyDir := filepath.Join(scratch, "decoy")
	spineWrite(t, filepath.Join(decoyDir, "ai-specs"), "#!/bin/sh\nprintf 'decoy' > "+marker+"\n", 0o755)
	// The agent's environment carries no AI_SPECS_HOME and PATH resolves a
	// different ai-specs first: neither may decide which CLI grades the event.
	// The command is this repository's own renderer output, executed verbatim.
	cmd := exec.Command("bash", "-c", command)
	cmd.Env = []string{"PATH=" + decoyDir + ":" + os.Getenv("PATH"), "CLAUDE_PROJECT_DIR=" + project}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %v\n%s", err, out)
	}
	if got := readFileString(t, marker); got != "sync-time" {
		t.Fatalf("marker = %q, want the sync-time installation to receive the event", got)
	}
}
