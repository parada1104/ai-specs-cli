package sync

// CLI-served Tracker card gate (cli-served-recipes T6, S3/F2): sync serves the
// recipe's tracker-card-gate hooks from the `ai-specs tracker-card-gate` verb,
// never from a materialized script, and retires the copy an older sync wrote.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// trackerGateManifest enables every F1 agent and the trello recipe whose tracker
// gate hooks the CLI serves.
const trackerGateManifest = `[project]
name = 'tracker-gate'
[brief]
render = false
[agents]
enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']

[recipes.trello-mcp-workflow]
enabled = true
[recipes.trello-mcp-workflow.config]
board_id = '69ec097f13e2d38ecd89a557'
`

const trackerGateScriptPath = "ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh"

// S1/T6: no tracker-card-gate.sh is materialized, the snapshot carries the
// recipe's gate_mode hint, and every renderer wires the CLI verb.
func TestSyncServesTrackerCardGateFromCLI(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, trackerGateManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(trackerGateScriptPath))); !os.IsNotExist(err) {
		t.Fatalf("tracker-card-gate.sh must not be materialized: %v", err)
	}
	lk, err := lock.LoadLock(filepath.Join(project, "ai-specs/.ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lk.Managed[trackerGateScriptPath] != nil {
		t.Fatalf("lock record must be gone: %v", lk.Managed[trackerGateScriptPath])
	}
	snapshot, found, err := projectcache.ReadRuntimeSnapshot(project, home)
	if err != nil || !found {
		t.Fatalf("snapshot: found=%v err=%v", found, err)
	}
	if snapshot.TrackerGateMode != "warn" {
		t.Fatalf("tracker gate mode = %q, want the recipe default 'warn'", snapshot.TrackerGateMode)
	}
	for _, rel := range []string{
		".claude/settings.json",
		".cursor/hooks/trello-mcp-workflow-tracker-card-gate-shell.sh",
		".opencode/plugin/trello-mcp-workflow-tracker-card-gate-shell.ts",
		".pi/extensions/trello-mcp-workflow-tracker-card-gate-shell.ts",
		".omp/extensions/trello-mcp-workflow-tracker-card-gate-shell.ts",
	} {
		body := readFileString(t, filepath.Join(project, filepath.FromSlash(rel)))
		// The rendered command is the installed-CLI lookup plus the verb; JSON and
		// TS escape the quotes, the cursor wrapper does not.
		if !strings.Contains(strings.ReplaceAll(body, `\`, ""), `exec "$cli" tracker-card-gate`) {
			t.Errorf("%s does not wire the tracker-card-gate verb:\n%s", rel, body)
		}
		if strings.Contains(body, "tracker-card-gate.sh") {
			t.Errorf("%s still references the retired script:\n%s", rel, body)
		}
	}
}

// S1/T6: the script an older sync materialized is retired — silently when it is
// still the recorded bytes, with one notice when the user edited it.
func TestSyncRetiresTrackerCardGateScript(t *testing.T) {
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
			project := spineProject(t, trackerGateManifest)
			script := filepath.Join(project, filepath.FromSlash(trackerGateScriptPath))
			body := "#!/bin/sh\nexit 0\n"
			spineWrite(t, script, body, 0o755)
			lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
			lk, err := lock.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			lock.SetGateBaseline(lk, trackerGateScriptPath, lock.Sha256Bytes([]byte(body)),
				strPtr("trello-mcp-workflow"), strPtr("hooks/tracker-card-gate.sh"))
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
			if after.Managed[trackerGateScriptPath] != nil {
				t.Fatalf("lock record survived: %v", after.Managed[trackerGateScriptPath])
			}
			notice := "removed user-modified tracker-card-gate launcher " + trackerGateScriptPath
			if noticed := strings.Contains(stderr, notice); noticed != tc.wantNotice {
				t.Fatalf("notice = %v, want %v; stderr=%q", noticed, tc.wantNotice, stderr)
			}
		})
	}
}

// R1-tracker-cli-provenance: the rendered tracker hook pins the installation
// that ran sync, so an agent started without AI_SPECS_HOME — or with a
// different ai-specs on PATH — still grades through it.
func TestTrackerCardGateHookBindsSyncTimeCLIHome(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, trackerGateManifest)
	t.Setenv("AI_SPECS_HOME", home)
	scratch := t.TempDir()
	marker := filepath.Join(scratch, "marker")
	spineWrite(t, filepath.Join(home, "dist", "ai-specs"), "#!/bin/sh\nprintf 'sync-time' > "+marker+"\n", 0o755)
	if rc, _, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d %s", rc, stderr)
	}
	decoyDir := filepath.Join(scratch, "decoy")
	spineWrite(t, filepath.Join(decoyDir, "ai-specs"), "#!/bin/sh\nprintf 'decoy' > "+marker+"\n", 0o755)
	// The agent's environment carries no AI_SPECS_HOME and PATH resolves a
	// different ai-specs first: neither may decide which CLI grades the event.
	cmd := exec.Command("bash", filepath.Join(project, ".cursor", "hooks",
		"trello-mcp-workflow-tracker-card-gate-shell.sh"))
	cmd.Env = []string{"PATH=" + decoyDir + ":" + os.Getenv("PATH"), "CURSOR_PROJECT_DIR=" + project}
	cmd.Stdin = strings.NewReader(`{"tool_name":"Shell","tool_input":{"command":"git status"}}`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %v\n%s", err, out)
	}
	if got := readFileString(t, marker); got != "sync-time" {
		t.Fatalf("marker = %q, want the sync-time installation to receive the event", got)
	}
}
