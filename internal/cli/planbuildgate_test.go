package cli

// Port parity for `ai-specs plan-build-gate` (cli-served-recipes T7, S3/F3): the
// retired catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh is now the CLI
// verb, so these tests pin the shell host's exit codes and message bytes.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/gates"
	"ai-specs.dev/ai-specs/internal/planbuild"
)

// runPlanBuildGate drives the verb as the renderers wire it: no arguments, one
// event on stdin.
func runPlanBuildGateCLI(t *testing.T, cwd, input string) (int, string, string) {
	t.Helper()
	return runPlanBuildGateArgs(t, cwd, input)
}

// runPlanBuildGateArgs drives the verb with explicit arguments.
func runPlanBuildGateArgs(t *testing.T, cwd, input string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(cwd)
	var stdout, stderr strings.Builder
	code := Run(append([]string{"plan-build-gate"}, args...), t.TempDir(), strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// planBuildGateEvent renders one pre-tool-use event for a repo-relative target.
func planBuildGateEvent(repo, tool, target string) string {
	return `{"tool_name":"` + tool + `","cwd":"` + repo + `","tool_input":{"file_path":"` + target + `"}}`
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

// S1: a production write with no plan is allowed in both modes, and the inferred
// tier is none.
func TestPlanBuildGateAllowsProductionWriteWithoutPlan(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	for _, tc := range []struct{ tool, target string }{
		{"Write", "src/app.go"},
		{"Edit", "lib/x.py"},
		{"NotebookEdit", "catalog/a/b.ipynb"},
		// No tool name: the hook defaults the verb to "edit".
		{"", "src/app.go"},
	} {
		input := planBuildGateEvent(repo, tc.tool, tc.target)
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, input); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("%s = (%d, %q, %q), want silence", tc.target, code, stdout, stderr)
		}
	}
	_, stdout, stderr := runPlanBuildGateArgs(t, repo, planBuildGateEvent(repo, "Write", "src/app.go"), "--explain")
	if stdout != "" || stderr != "plan-build-gate: tier none; refutations none\n" {
		t.Fatalf("explain = (%q, %q), want the inferred none tier", stdout, stderr)
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
// directories, and a whitespace-only value falls back to the default. In scope a
// write with no plan and no product change is allowed.
func TestPlanBuildGateHonorsProductionPaths(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	t.Setenv("PLAN_BUILD_GATE_PATHS", "src")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "lib/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("lib outside scope = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("src in scope = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	t.Setenv("PLAN_BUILD_GATE_PATHS", "   ")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "lib/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("blank scope = (%d, %q, %q), want silence", code, stdout, stderr)
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
// outcome, and the write has nothing to grade, so it stays silent.
func TestPlanBuildGateLedgerWarnStaysSilent(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A satisfied ledger checkpoint leaves the write allowed, with or without a
// plan: no artifact is a precondition any more.
func TestPlanBuildGateLedgerAllowKeepsArtifactRule(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, true)
	t.Setenv("TRACKER_LEDGER_MODE", "always")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	planBuildWritePlan(t, repo, "some-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate with a plan = (%d, %q, %q), want silence", code, stdout, stderr)
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

// R4-hasplan-glob-metachar-root: the changes directory is read literally, so a
// repository path containing glob metacharacters neither hides a real plan nor
// matches a sibling's plan.
func TestPlanBuildGatePlanSurvivesGlobMetacharRoot(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	bracketed := filepath.Join(filepath.Dir(repo), "check[out]")
	if err := os.Rename(repo, bracketed); err != nil {
		t.Fatal(err)
	}
	planBuildWritePlan(t, bracketed, "bracketed-plan")
	if code, stdout, stderr := runPlanBuildGateCLI(t, bracketed, planBuildGateEvent(bracketed, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("plan under a bracketed root = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// --- ODD mode (plan-build-odd-companion S3/S10) ----------------------------

// planBuildWriteMode writes the manifest declaring one execution mode.
func planBuildWriteMode(t *testing.T, root, mode string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[recipes.plan-build-flow]\nenabled = true\n\n" +
		"[recipes.plan-build-flow.config]\nmode = \"" + mode + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// planBuildBranch checks out the change branch the plan pairs with.
func planBuildBranch(t *testing.T, repo, branch string) {
	t.Helper()
	gitTest(t, repo, "checkout", "-b", branch)
}

// planBuildWriteOddPlan writes the paired ODD plan.
func planBuildWriteOddPlan(t *testing.T, root, change string) {
	t.Helper()
	path := filepath.Join(root, "odd", "tasks", change+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// planBuildWriteOddDelta writes one delta under the paired odd change folder.
func planBuildWriteOddDelta(t *testing.T, root, change string) {
	t.Helper()
	path := filepath.Join(root, "odd", "changes", change, "specs", "cap", "spec.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// planBuildCommonDir is the repository's git common directory.
func planBuildCommonDir(t *testing.T, repo string) string {
	t.Helper()
	common := gitOutput(t, repo, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}
	return sharedRealPath(common)
}

// planBuildOddSpecsRefusal is R1's message for a hand write to the canonical
// spec tree, rendered as the gate prints it.
func planBuildOddSpecsRefusal(tool, rel, change string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — " +
		gates.CanonicalSpecRefusal(planbuild.ModeOdd, change, rel) + "\n"
}

// planBuildOddSpecsBranchRefusal is R1's message when HEAD is detached, so no
// change can be paired.
func planBuildOddSpecsBranchRefusal(tool, rel string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — " +
		gates.CanonicalSpecRefusal(planbuild.ModeOdd, "", rel) + "\n"
}

// planBuildAssertAbsent pins that a refused write touched nothing on disk.
func planBuildAssertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s: stat err %v, want the refused target to stay absent", path, err)
	}
}

// oddRepo is a repository declaring odd mode on a prefixed change branch.
func oddRepo(t *testing.T) (repo, branch string) {
	t.Helper()
	repo = sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "odd")
	branch = "feat/odd-change"
	planBuildBranch(t, repo, branch)
	return repo, branch
}

// S18: odd mode requires no plan — a small ODD change leaves no durable
// artifact, so a production write passes with no odd/tasks/<change>.md.
func TestPlanBuildGateOddModeAllowsProductionWriteWithoutPlan(t *testing.T) {
	repo, _ := oddRepo(t)
	for _, target := range []string{"src/app.go", "lib/x.py", "catalog/a/b.ipynb"} {
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target)); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("%s = (%d, %q, %q), want silence", target, code, stdout, stderr)
		}
	}
}

// A hand write to the canonical spec tree is a promotion bypass: it is refused
// naming the rule, the missing delta and both exits.
func TestPlanBuildGateOddModeRefusesHandWriteToCanonicalSpecs(t *testing.T) {
	repo, _ := oddRepo(t)
	target := "odd/specs/cap/spec.md"
	want := planBuildOddSpecsRefusal("Write", target, "odd-change")
	code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target))
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	for _, name := range []string{"odd/specs", "odd/changes/odd-change/specs/", "ai-specs specs promote"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("refusal %q does not name %q", stderr, name)
		}
	}
	planBuildAssertAbsent(t, filepath.Join(repo, filepath.FromSlash(target)))
}

// With the delta the paired change carries, the canonical tree write passes:
// promotion would write the same rule.
func TestPlanBuildGateOddModeAllowsCanonicalSpecsWriteWithDelta(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteOddDelta(t, repo, "odd-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "odd/specs/cap/spec.md")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// --explain reports the inferred tier and the rules on the canonical-spec path
// too, not only on the production-write path.
func TestPlanBuildGateOddModeExplainsCanonicalSpecWrite(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteOddDelta(t, repo, "odd-change")
	code, stdout, stderr := runPlanBuildGateArgs(t, repo,
		planBuildGateEvent(repo, "Write", "odd/specs/cap/spec.md"), "--explain")
	if code != 0 || stdout != "" || stderr != "plan-build-gate: tier light; refutations none\n" {
		t.Fatalf("explain = (%d, %q, %q), want the inferred tier line", code, stdout, stderr)
	}
}

// Planning artifacts stay writable: the odd tree and every openspec change.
func TestPlanBuildGateOddModeAllowsPlanningWrites(t *testing.T) {
	repo, _ := oddRepo(t)
	for _, target := range []string{
		"odd/tasks/odd-change.md",
		"odd/changes/odd-change/proposal.md",
		"odd/changes/odd-change/specs/cap/spec.md",
		"openspec/changes/other/tasks.md",
	} {
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target)); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("%s = (%d, %q, %q), want silence", target, code, stdout, stderr)
		}
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "odd", "tasks", "odd-change.md"))
}

// A branch with no commits yet is still a branch: HEAD is unborn, not detached,
// so the change pairs and the production write passes with no plan.
func TestPlanBuildGateOddModeAllowsOnAnUnbornBranch(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-b", "feat/odd-change")
	planBuildWriteMode(t, repo, "odd")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate on an unborn branch = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A detached HEAD pairs no change, so no delta can back a canonical tree write:
// it is refused naming the branch exit.
func TestPlanBuildGateOddModeDetachedHeadRefusesCanonicalSpecsWrite(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteOddDelta(t, repo, "odd-change")
	gitTest(t, repo, "checkout", "--detach")
	target := "odd/specs/cap/spec.md"
	want := planBuildOddSpecsBranchRefusal("Write", target)
	code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target))
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	for _, name := range []string{"detached HEAD", "change's branch", "promote"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("refusal %q does not name %q", stderr, name)
		}
	}
	planBuildAssertAbsent(t, filepath.Join(repo, filepath.FromSlash(target)))
}

// A declared openspec mode no longer resolves a mandatory change: a production
// write with no artifact is allowed.
func TestPlanBuildGateDeclaredOpenspecResolvesTheChange(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "openspec")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	planBuildWritePlan(t, repo, "some-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate with a plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// auto follows the Gentle AI state directory: present behaves as odd, so a
// production write needs no plan and only the canonical spec tree is graded.
func TestPlanBuildGateAutoInfersOddFromStateDir(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteMode(t, repo, "auto")
	if err := os.MkdirAll(filepath.Join(planBuildCommonDir(t, repo), "gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	target := "odd/specs/cap/spec.md"
	want := planBuildOddSpecsRefusal("Write", target, "odd-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target)); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, filepath.FromSlash(target)))
}

// auto without the state directory stays on the OpenSpec flow, which now also
// allows a production write with no plan.
func TestPlanBuildGateAutoWithoutStateDirStaysOpenspec(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "auto")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// --- OpenSpec change resolver (plan-build-openspec-odd-align P0) -----------

// planBuildWriteOpenspecPlan writes one active OpenSpec change plan.
func planBuildWriteOpenspecPlan(t *testing.T, root, change string) {
	t.Helper()
	path := filepath.Join(root, "openspec", "changes", change, "tasks.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Two active changes with a branch that pairs neither is no longer ambiguous for
// the gate: both writes are allowed because no refutation is proven.
func TestPlanBuildGateOpenspecAmbiguityRefusesAndPairingAllows(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteOpenspecPlan(t, repo, "alpha")
	planBuildWriteOpenspecPlan(t, repo, "beta")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("ambiguous gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	planBuildBranch(t, repo, "feat/alpha")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("paired gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A paired change folder whose tasks.md is missing is still allowed: the tier is
// none and nothing is refuted.
func TestPlanBuildGateOpenspecPairedFolderNeedsAPlan(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildBranch(t, repo, "feat/alpha")
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("folder without a plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// --- Local no-plan declaration (plan-build-openspec-odd-align T14/S19) ------

// In openspec mode the local declaration lets a production write through with no
// plan at all.
func TestPlanBuildGateOpenspecDeclarationAllowsWithoutAPlan(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "openspec")
	planBuildBranch(t, repo, "feat/alpha")
	if err := planbuild.DeclareCeremonyNone(repo, "feat/alpha"); err != nil {
		t.Fatalf("DeclareCeremonyNone: %v", err)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("declared gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// Without the declaration the same write is allowed: the declaration is inert.
func TestPlanBuildGateOpenspecWithoutDeclarationStillRefuses(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "openspec")
	planBuildBranch(t, repo, "feat/alpha")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("undeclared gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A declaration written for another branch changes nothing either.
func TestPlanBuildGateDeclarationOnAnotherBranchDoesNotApply(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "openspec")
	planBuildBranch(t, repo, "feat/alpha")
	if err := planbuild.DeclareCeremonyNone(repo, "feat/other"); err != nil {
		t.Fatalf("DeclareCeremonyNone: %v", err)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("other-branch declaration = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// The declaration does not touch odd mode: the canonical spec tree still needs a
// delta on the declared branch.
func TestPlanBuildGateDeclarationDoesNotRelaxOddSpecs(t *testing.T) {
	repo, _ := oddRepo(t)
	if err := planbuild.DeclareCeremonyNone(repo, "feat/odd-change"); err != nil {
		t.Fatalf("DeclareCeremonyNone: %v", err)
	}
	target := "odd/specs/cap/spec.md"
	want := planBuildOddSpecsRefusal("Write", target, "odd-change")
	code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target))
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("declared odd gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, filepath.FromSlash(target)))
}
