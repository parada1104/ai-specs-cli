package cli

// Port parity for `ai-specs plan-build-gate` (cli-served-recipes T7, S3/F3): the
// retired catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh is now the CLI
// verb, so these tests pin the shell host's exit codes and message bytes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runPlanBuildGate drives the verb as the renderers wire it: no arguments, one
// event on stdin.
func runPlanBuildGateCLI(t *testing.T, cwd, input string) (int, string, string) {
	t.Helper()
	t.Chdir(cwd)
	var stdout, stderr strings.Builder
	code := Run([]string{"plan-build-gate"}, t.TempDir(), strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// planBuildGateEvent renders one pre-tool-use event for a repo-relative target.
func planBuildGateEvent(repo, tool, target string) string {
	return `{"tool_name":"` + tool + `","cwd":"` + repo + `","tool_input":{"file_path":"` + target + `"}}`
}

// planBuildNearestRefusal is the shell host's nearest-root refusal, verbatim.
func planBuildNearestRefusal(tool, rel string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — no active change folder " +
		"(openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan first, " +
		"then implement. Writing planning artifacts under openspec/changes/ is never blocked.\n"
}

// planBuildCentralRefusal is the shell host's central-root refusal, verbatim.
func planBuildCentralRefusal(tool, rel, central, sub string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — no active change folder found " +
		"under central planning root '" + central + "/openspec/changes/' for submodule '" + sub + "'. " +
		"Classify the change and write the central plan first; writes under that planning tree are allowed.\n"
}

// planBuildWritePlan writes one active change folder under root.
func planBuildWritePlan(t *testing.T, root, slug string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", slug), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "changes", slug, "tasks.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An unclassifiable event, a non-production target, the agent's own config, the
// planning tree and a path outside the repository all fail open in silence.
func TestPlanBuildGateIgnoresUnrelatedTargets(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	outside := filepath.Join(t.TempDir(), "src", "app.go")
	for _, input := range []string{
		`not json at all`,
		`{"tool_name":"Read","cwd":"` + repo + `","tool_input":{}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"docs/readme.md"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":".claude/settings.json"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":".claude/settings.local.json"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":".claude/hooks/guard.sh"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"pkg/.claude/settings.json"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"pkg/.claude/hooks/guard.sh"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"openspec/changes/x/proposal.md"}}`,
		`{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"` + outside + `"}}`,
	} {
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, input); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("event %s = (%d, %q, %q), want silence", input, code, stdout, stderr)
		}
	}
}

// A production write with no active change folder is refused with exit 2 and the
// nearest-root message, naming the tool and the repository-relative target.
func TestPlanBuildGateRefusesProductionWriteWithoutPlan(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	for _, tc := range []struct{ tool, target, rel string }{
		{"Write", "src/app.go", "src/app.go"},
		{"Edit", "lib/x.py", "lib/x.py"},
		{"NotebookEdit", "catalog/a/b.ipynb", "catalog/a/b.ipynb"},
		// No tool name: the shell host defaulted the verb to "edit".
		{"", "src/app.go", "src/app.go"},
	} {
		tool := tc.tool
		if tool == "" {
			tool = "edit"
		}
		input := planBuildGateEvent(repo, tc.tool, tc.target)
		want := planBuildNearestRefusal(tool, tc.rel)
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, input); code != 2 || stdout != "" || stderr != want {
			t.Fatalf("%s = (%d, %q, %q), want (2, %q, %q)", tc.target, code, stdout, stderr, "", want)
		}
	}
}

// An active change folder under the nearest repository root allows the write.
func TestPlanBuildGateAllowsWithActivePlan(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWritePlan(t, repo, "some-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// PLAN_BUILD_GATE_PATHS is scope only: it replaces the graded production
// directories, and a whitespace-only value falls back to the default.
func TestPlanBuildGateHonorsProductionPaths(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	t.Setenv("PLAN_BUILD_GATE_PATHS", "src")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "lib/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("lib outside scope = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	want := planBuildNearestRefusal("Write", "src/x.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("src in scope = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	t.Setenv("PLAN_BUILD_GATE_PATHS", "   ")
	want = planBuildNearestRefusal("Write", "lib/x.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "lib/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("blank scope = (%d, %q), want (2, %q)", code, stderr, want)
	}
}

// A target outside any git work tree fails open, as the shell host did.
func TestPlanBuildGateFailsOpenOutsideGitWorkTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "src", "app.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, dir, planBuildGateEvent(dir, "Write", target)); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// The work-start checkpoint grades before the artifact rule: with the ledger at
// `always` and no tracked item, the block line is the only output and the exit
// is 2 — the shell host returned before looking for a plan.
func TestPlanBuildGateLedgerBlockComesFirst(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	t.Setenv("TRACKER_LEDGER_MODE", "always")
	want := "plan-build-gate: blocked at work-start — needs-item\n"
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
}

// The default (warn) mode is not blocking and prints nothing for a needs-item
// outcome — warn resolves to decision `allow` in the predicate — so the artifact
// rule alone decides.
func TestPlanBuildGateLedgerWarnStaysSilent(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	want := planBuildNearestRefusal("Write", "src/x.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
}

// A satisfied ledger checkpoint leaves the artifact rule in charge.
func TestPlanBuildGateLedgerAllowKeepsArtifactRule(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, true)
	t.Setenv("TRACKER_LEDGER_MODE", "always")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != planBuildNearestRefusal("Write", "src/x.go") {
		t.Fatalf("gate = (%d, %q, %q), want the artifact refusal only", code, stdout, stderr)
	}
	planBuildWritePlan(t, repo, "some-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate with a plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// nestedRepo builds a submodule-shaped layout: a superproject directory that
// contains a git repository.
func nestedRepo(t *testing.T) (central, repo string) {
	t.Helper()
	central = sharedRealPath(t.TempDir())
	repo = filepath.Join(central, "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "init", "-b", "main")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	gitTest(t, repo, "config", "user.name", "test")
	gitTest(t, repo, "commit", "--allow-empty", "-m", "initial")
	return central, sharedRealPath(repo)
}

// stubCentralProof replaces the topology query for the rest of the test.
func stubCentralProof(t *testing.T, payload string, ok bool) {
	t.Helper()
	original := planBuildGateCentralRootQuerier
	t.Cleanup(func() { planBuildGateCentralRootQuerier = original })
	planBuildGateCentralRootQuerier = func(string) (string, bool) { return payload, ok }
}

// A proven central planning root with an active plan allows the write; without
// one the refusal names the central root and the submodule.
func TestPlanBuildGateCentralPlanningRoot(t *testing.T) {
	central, repo := nestedRepo(t)
	stubCentralProof(t, `{"central_root":"`+central+`","submodule":"app"}`, true)
	planBuildWritePlan(t, central, "central-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("proven central plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	if err := os.RemoveAll(filepath.Join(central, "openspec")); err != nil {
		t.Fatal(err)
	}
	want := planBuildCentralRefusal("Write", "src/x.go", central, "app")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("unproven plan = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
}

// Every unproven or malformed topology answer falls back to the nearest-root
// refusal instead of guessing.
func TestPlanBuildGateUnprovenTopologyFallsBack(t *testing.T) {
	central, repo := nestedRepo(t)
	want := planBuildNearestRefusal("Write", "src/x.go")
	for _, tc := range []struct {
		name    string
		payload string
		ok      bool
	}{
		{"query failed", "", false},
		{"empty payload", "", true},
		{"relative root", `{"central_root":"app","submodule":"app"}`, true},
		{"tab in payload", `{"central_root":"` + central + `\t","submodule":"app"}`, true},
		{"same as repository", `{"central_root":"` + repo + `","submodule":"app"}`, true},
		{"does not contain the repository", `{"central_root":"` + sharedRealPath(t.TempDir()) + `","submodule":"app"}`, true},
		{"not json", `not json`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubCentralProof(t, tc.payload, tc.ok)
			if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
				t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
			}
		})
	}
}

// The verb documents itself and refuses arguments it never accepted.
func TestPlanBuildGateUsage(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run([]string{"plan-build-gate", "--help"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Usage: ai-specs plan-build-gate") || stderr.String() != "" {
		t.Fatalf("--help = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"plan-build-gate", "--bogus"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "plan-build-gate: unexpected argument '--bogus'") {
		t.Fatalf("--bogus = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
}

// The verb is registered as a native route with its own kind.
func TestPlanBuildGateRouteIsNative(t *testing.T) {
	if got := Route([]string{"plan-build-gate"}).kind; got != routePlanBuildGate {
		t.Fatalf("route = %v, want routePlanBuildGate", got)
	}
}

// A JSON field reader identical to `hook field` keeps the verdict text parity.
func TestPlanBuildGateFieldTextParity(t *testing.T) {
	doc := `{"decision":"block","active":true,"prompt":{"evidence":{"local":"a"},"choices":["x","y"]}}`
	for _, tc := range []struct{ path, want string }{
		{"decision", "block"},
		{"active", "1"},
		{"prompt.evidence.local", "a"},
		{"prompt.choices", "x, y"},
		{"missing", ""},
	} {
		if got := planBuildGateField(doc, tc.path); got != tc.want {
			t.Errorf("field %s = %q, want %q", tc.path, got, tc.want)
		}
	}
	if got := planBuildGateField(`{`, "decision"); got != "" {
		t.Errorf("invalid JSON = %q, want empty", got)
	}
}
