package cli

// `ai-specs changes next` and the postures (change-ledger-v2 T8, S5/S10/S16): the
// transition→posture map, advise by default, `require` per project, and R3-R5 as
// advice or as blockers. The map is unit-tested without a repository; the posture is
// driven by the built CLI over real repositories, because the thresholds read a diff.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/gates"
)

// L5 item 6: the checkpoint a phase is heading to. Every other phase has no
// posture-governed transition, so it has no checkpoint at all.
func TestStatusCheckpointOfAPhase(t *testing.T) {
	for _, tc := range []struct{ phase, want string }{
		{statusOpen, gates.CheckpointMerge},
		{statusPlanned, gates.CheckpointMerge},
		{statusMerged, ""},
		{statusCleaned, ""},
		{statusIntegration, ""},
		{statusWorkstream, ""},
		{statusUnknown, ""},
	} {
		if got := statusCheckpoint(tc.phase); got != tc.want {
			t.Errorf("statusCheckpoint(%q) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

// L5 item 6: advise is the default posture; `require` is per checkpoint and opt-in;
// and a phase with no checkpoint has no posture at all, so a workstream is never
// subject to one (S16).
func TestStatusPostureOfACheckpoint(t *testing.T) {
	declared := gates.Posture{Require: []string{gates.CheckpointMerge}}
	for _, tc := range []struct {
		name       string
		posture    gates.Posture
		checkpoint string
		want       string
	}{
		{"nothing declared", gates.Posture{}, gates.CheckpointMerge, statusPostureAdvise},
		{"declared for this checkpoint", declared, gates.CheckpointMerge, statusPostureRequire},
		{"declared for another checkpoint", gates.Posture{Require: []string{"archive-close"}},
			gates.CheckpointMerge, statusPostureAdvise},
		{"no checkpoint at all", declared, "", statusPostureNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusPosture(tc.posture, tc.checkpoint); got != tc.want {
				t.Fatalf("statusPosture = %q, want %q", got, tc.want)
			}
		})
	}
}

// nextFlowRun runs one `changes next` step of the flow matrix.
func nextFlowRun(t *testing.T, bin, repo string, args ...string) (string, string, int) {
	t.Helper()
	return flowRun(t, bin, repo, "", append([]string{"changes", "next", "--root", repo}, args...)...)
}

// mergeSizedWork commits a product change over the CLI's line threshold, so R3 is a
// fact of the repository rather than a fixture of the assertion.
func mergeSizedWork(t *testing.T, repo string) {
	t.Helper()
	flowWrite(t, repo, "internal/gates/big.go", strings.Repeat("line\n", 1200))
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-q", "-m", "feat: a change over the line threshold")
}

// S5/S10/T8.1: advise by default. The high-risk change reports its advice in both
// verbs and never changes the exit.
func TestChangesNextFlowAdvisesByDefault(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowCommitManifest(t, repo)
	mergeSizedWork(t, repo)

	stdout, stderr, code := nextFlowRun(t, bin, repo)
	if code != 0 || stderr != "" {
		t.Fatalf("changes next = (%d, %q, %q), want advice and the resolved exit", code, stdout, stderr)
	}
	for _, want := range []string{
		"next: ai-specs gates premerge",
		"checkpoint: merge (posture advise)",
		"advice: R3:",
		"lines = 1000",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("next %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "blocked:") {
		t.Errorf("next %q refuses a project that declared no posture", stdout)
	}

	// The same facts in `changes status`: one collection, two verbs.
	status, statusErr, statusCode := statusFlowRun(t, bin, repo)
	if statusCode != 0 || statusErr != "" {
		t.Fatalf("changes status = (%d, %q, %q), want it to advise too", statusCode, status, statusErr)
	}
	for _, want := range []string{"posture:  advise at the merge checkpoint", "advice:   R3:"} {
		if !strings.Contains(status, want) {
			t.Errorf("status %q does not contain %q", status, want)
		}
	}
}

// L5 item 6/S5: with `require = ["merge"]` the same rule joins the blockers and the
// verb refuses with the exits named.
func TestChangesNextFlowRefusesUnderARequiredPosture(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nrequire = [\"merge\"]\n")
	flowCommitManifest(t, repo)
	mergeSizedWork(t, repo)

	stdout, stderr, code := nextFlowRun(t, bin, repo)
	if code != 2 {
		t.Fatalf("changes next = %d (%q, %q), want the refusal exit", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "checkpoint: merge (posture require)") {
		t.Errorf("next %q does not report the required posture", stdout)
	}
	if !strings.Contains(stdout, "blocked: R3:") {
		t.Fatalf("next %q does not name the rule", stdout)
	}
	for _, want := range []string{"1000", "tier light", "risk_thresholds"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("next %q does not name the exit %q", stdout, want)
		}
	}

	// `--ledger` narrows the facts and never the posture or the exit (L5 item 7).
	if _, _, code := nextFlowRun(t, bin, repo); code != 2 {
		t.Fatalf("changes next = %d, want the narrowed report to keep the exit", code)
	}
	narrowed, _, code := statusFlowRun(t, bin, repo, "--ledger", "plan")
	if code != 2 || !strings.Contains(narrowed, "posture:  require at the merge checkpoint") {
		t.Fatalf("status --ledger plan = (%d, %q), want the posture and the exit unchanged", code, narrowed)
	}
}

// S16: a workstream has no checkpoint, so it is never subject to a posture — even
// when the project requires one.
func TestChangesNextFlowLeavesAWorkstreamWithoutAPosture(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nrequire = [\"merge\"]\n")
	flowCommitManifest(t, repo)

	stdout, stderr, code := nextFlowRun(t, bin, repo)
	if code != 0 || stderr != "" {
		t.Fatalf("changes next = (%d, %q, %q), want a workstream to owe nothing", code, stdout, stderr)
	}
	if strings.Contains(stdout, "checkpoint:") {
		t.Errorf("next %q gives a workstream a checkpoint", stdout)
	}
	for _, want := range []string{"posture: none", "next: nothing owed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("next %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "blocked:") || strings.Contains(stdout, "advice:") {
		t.Errorf("next %q applies a posture-governed rule to a workstream", stdout)
	}
}

// L5 item 8: the JSON shape a host reads.
func TestChangesNextJSONShape(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowCommitManifest(t, repo)
	mergeSizedWork(t, repo)

	stdout, stderr, code := nextFlowRun(t, bin, repo, "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("changes next --json = (%d, %q, %q), want the report", code, stdout, stderr)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("changes next --json printed %q: %v", stdout, err)
	}
	var keys []string
	for key := range report {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"advice", "blockers", "checkpoint", "next", "phase", "posture", "unknown"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("--json keys = %v, want %v", keys, want)
	}
	for key, expected := range map[string]string{
		"phase": statusOpen, "checkpoint": gates.CheckpointMerge, "posture": statusPostureAdvise,
	} {
		var value string
		if err := json.Unmarshal(report[key], &value); err != nil || value != expected {
			t.Fatalf("%s = %q (%v), want %q", key, value, err, expected)
		}
	}
	var advice []string
	if err := json.Unmarshal(report["advice"], &advice); err != nil || len(advice) != 1 ||
		!strings.HasPrefix(advice[0], "R3: ") {
		t.Fatalf("advice = %v (%v), want the one R3 line", advice, err)
	}
	var blockers []string
	if err := json.Unmarshal(report["blockers"], &blockers); err != nil || len(blockers) != 0 {
		t.Fatalf("blockers = %v (%v), want an empty list under the default posture", blockers, err)
	}
}

// The argument contract: usage errors keep the family's 2 and print no report, and
// `--help` prints the verb's usage.
func TestChangesNextArgumentContract(t *testing.T) {
	for _, args := range [][]string{
		{"changes", "next"},
		{"changes", "next", "--root"},
		{"changes", "next", "--root", "--json"},
		{"changes", "next", "--root", t.TempDir(), "extra"},
		{"changes", "next", "--root", t.TempDir(), "--unknown"},
	} {
		code, stdout, _ := runChangesCLI(t, args...)
		if code != 2 || stdout != "" {
			t.Errorf("%v = (%d, %q), want the usage exit and no report", args, code, stdout)
		}
	}
	code, stdout, stderr := runChangesCLI(t, "changes", "next", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "changes next") {
		t.Fatalf("--help = (%d, %q, %q), want the usage on stdout", code, stdout, stderr)
	}
	if code, stdout, _ := runChangesCLI(t, "changes", "--help"); code != 0 ||
		!strings.Contains(stdout, "next --root <repo-root>") {
		t.Fatalf("family help = (%d, %q), want it announcing the next verb", code, stdout)
	}
}
