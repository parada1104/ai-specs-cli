package sync

// S13 (change-ledger-v2 T4): the advisory a hook writes on stderr with exit 0 was
// invisible. The generated pi/omp extension relayed only `res.stdout` on exit 0,
// so `plan-build-gate --explain` and the tracker advisory — the two things a
// person actually needs to see — never reached the agent.

import (
	"path/filepath"
	"strings"
	"testing"
)

// The relay carries the whole exit-0 stderr block, and the blocking path stays
// exactly what it was: an advisory must never be mistaken for a block.
//
// No line is selected on purpose. The real producers write a preamble before the
// line that matters — under `TRACKER_LEDGER_MODE=ask` the tracker advisory is six
// lines ending in the actionable one — so a first-line or filter heuristic
// relays the preamble and drops the advice.
func TestPiWrapperRelaysAnExitZeroAdvisory(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}
	for _, rel := range []string{
		".pi/extensions/worktree-flow-worktree-gate.ts",
		".omp/extensions/worktree-flow-worktree-gate.ts",
	} {
		body := readFileString(t, filepath.Join(project, filepath.FromSlash(rel)))
		for _, want := range []string{
			`const advisory = res.status === 0 ? (res.stderr || "").trim() : "";`,
			`customType: "ai-specs-advisory"`,
			"display: true",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not relay an exit-0 advisory (%q missing):\n%s", rel, want, body)
			}
		}
		// The advisory is one message with all of its lines: cutting it to a first
		// line is what made the tracker advisory arrive as `local: (unavailable)`.
		if strings.Contains(body, ".split(\"\\n\").map(") {
			t.Errorf("%s selects lines out of the advisory instead of relaying it:\n%s", rel, body)
		}
		if !strings.Contains(body, "if (res.status === 2) {") {
			t.Errorf("%s lost the blocking path:\n%s", rel, body)
		}
	}
}
