package sync

// S13 (change-ledger-v2 T4): the render-level assertion pins the template text,
// but only executing the generated wrapper proves what the caller actually sees.
// The real producers write a multi-line block whose actionable line comes last —
// `TRACKER_LEDGER_MODE=ask ai-specs tracker-card-gate` writes this exact six-line
// shape — so the test replays that shape instead of a friendly single line.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// trackerAskStderr is the stderr a real tracker-card-gate `ask` run writes with
// exit 0: four evidence lines, the choices line, and the advisory last.
const trackerAskStderr = "  local: (unavailable)\n" +
	"  remote: (unavailable)\n" +
	"  code: (unavailable)\n" +
	"  git: (unavailable)\n" +
	"  choices: exempt, continue\n" +
	"tracker-card-gate: apply-start needs a decision but no terminal is available; " +
	"no opt-out was recorded — Tracker advisory, continuing without blocking.\n"

const advisoryDriveScript = `
import assert from "node:assert/strict";
const expected = __EXPECTED__;
for (const adapter of ["pi", "omp"]) {
  const { default: register } = await import("./." + adapter + "/extensions/worktree-flow-worktree-gate.ts");
  for (const status of ["0", "2"]) {
    process.env.HOOK_EXIT = status;
    let call;
    const messages = [];
    register({
      on(_, fn) { call = fn; },
      // A host that refuses the message must not change the hook's decision.
      sendMessage(m) { messages.push(m); return Promise.reject(new Error("host refused")); },
    });
    const result = call({ toolName: "write", input: { path: "src/app.go" } });
    if (status === "0") {
      assert.equal(result, undefined, adapter + ": an advisory is not a block");
      assert.deepEqual(messages, [{ customType: "ai-specs-advisory", content: expected.trim(), display: true }],
        adapter + ": the whole advisory is relayed once, visibly");
    } else {
      assert.equal(result?.block, true, adapter + ": exit 2 still blocks");
      assert.equal(result.reason, expected, adapter + ": the refusal keeps the raw stderr");
      assert.deepEqual(messages, [], adapter + ": a refusal sends no advisory");
    }
  }
}
`

// TestPiAndOmpRelayTheWholeTrackerAdvisory executes the generated adapters. Node
// strips the types (the only TypeScript syntax the template emits), so the test
// needs nothing but a Node that can run it, and skips otherwise.
func TestPiAndOmpRelayTheWholeTrackerAdvisory(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable: cannot execute the generated wrapper")
	}
	if err := exec.Command(node, "--experimental-strip-types", "-e", "").Run(); err != nil {
		t.Skipf("node type stripping unavailable: %v", err)
	}
	home, _ := spineEnv(t)
	project := spineProject(t, cliServedManifest)
	if rc, stdout, stderr := runCLIServedSync(t, home, project); rc != 0 {
		t.Fatalf("sync rc=%d\n%s\n%s", rc, stdout, stderr)
	}

	cliHome := t.TempDir()
	spineWrite(t, cliHome+"/dist/ai-specs", `#!/bin/sh
printf '  local: (unavailable)\n  remote: (unavailable)\n  code: (unavailable)\n  git: (unavailable)\n  choices: exempt, continue\n%s\n' \
  'tracker-card-gate: apply-start needs a decision but no terminal is available; no opt-out was recorded — Tracker advisory, continuing without blocking.' >&2
exit "${HOOK_EXIT:-0}"
`, 0o755)

	script := strings.Replace(advisoryDriveScript, "__EXPECTED__", mcpJSONString(trackerAskStderr), 1)
	cmd := exec.Command(node, "--experimental-strip-types", "--unhandled-rejections=strict",
		"--input-type=module", "-e", script)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "AI_SPECS_HOME="+cliHome)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("executing the generated wrappers failed: %v\n%s", err, output)
	}
}
