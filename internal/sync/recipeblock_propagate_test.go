package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncFailureAfterRecipeBlockPropagates(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest+"\n[brief]\nsummary_mode = 'bogus'\n")
	tomlPath := filepath.Join(project, "ai-specs", "ai-specs.toml")
	agentsPath := filepath.Join(project, "AGENTS.md")

	// The native AGENTS.md step's own failure status for this fixture. The
	// invalid-mode failure returns before any write, so probing the same
	// project leaves it untouched.
	var stepOut, stepErr bytes.Buffer
	want := renderAgentsStep(tomlPath, agentsPath, "", false, &stepOut, &stepErr)
	if want == 0 {
		t.Fatalf("the fixture must fail the native AGENTS.md step; stdout=%q stderr=%q", stepOut.String(), stepErr.String())
	}
	if !strings.Contains(stepErr.String(), "invalid mode") {
		t.Fatalf("native step stderr = %q, want the invalid-mode failure", stepErr.String())
	}

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != want {
		t.Fatalf("sync rc = %d, want the failing step's own %d\nstdout:\n%s\nstderr:\n%s", rc, want, out.String(), errW.String())
	}
	// The recipe block ran before the failure, and the failing step is announced.
	for _, label := range []string{
		"  syncing recipes\n",
		"  syncing AGENTS.md\n",
	} {
		if !strings.Contains(out.String(), label) {
			t.Errorf("stdout missing %q:\n%s", label, out.String())
		}
	}
	// The failure is surfaced, not swallowed, and no success is claimed.
	if !strings.Contains(errW.String(), "[brief].summary_mode: invalid mode") {
		t.Errorf("stderr = %q, want the step's own failure text", errW.String())
	}
	if strings.Contains(out.String(), "✓ ai-specs sync complete") {
		t.Errorf("the sync claimed success after a failing step:\n%s", out.String())
	}
	if data, err := os.ReadFile(agentsPath); err != nil || string(data) != "# manual runtime brief\n" {
		t.Errorf("the failing step wrote AGENTS.md: %q %v", data, err)
	}
}
