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
)

// runPlanBuildGate drives the verb as the renderers wire it: no arguments, one
// event on stdin.
func runPlanBuildGateCLI(t *testing.T, cwd, input string) (int, string, string) {
	t.Helper()
	// The topology proof runs in a child process, and the test binary is not that
	// CLI: the default child proves nothing, the answer a standalone repository
	// gets. Tests that want a proof stub planBuildGateCentralRootQuerier.
	original := planBuildGateTopologyExecutable
	t.Cleanup(func() { planBuildGateTopologyExecutable = original })
	planBuildGateTopologyExecutable = func() (string, error) { return planBuildGateUnprovenBinary(t), nil }
	t.Chdir(cwd)
	var stdout, stderr strings.Builder
	code := Run([]string{"plan-build-gate"}, t.TempDir(), strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// planBuildGateUnprovenBinary is a stand-in CLI whose topology proof fails.
func planBuildGateUnprovenBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-specs")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
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

// R3-001: only a usable plan file counts. A directory named tasks.md — or a
// dangling symlink — is a glob match but not a plan, so a production write it
// would otherwise allow stays refused. A symlink to a real plan does count.
func TestPlanBuildGatePlanMustBeAFile(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "dir-plan", "tasks.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(repo, "openspec", "changes", "dangling-plan", "tasks.md")
	if err := os.MkdirAll(filepath.Dir(dangling), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "gone.md"), dangling); err != nil {
		t.Fatal(err)
	}
	want := planBuildNearestRefusal("Write", "src/x.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("non-file plans = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	// The dangling link resolves to a real plan: the gate follows symlinks.
	planBuildWritePlan(t, repo, "real-plan")
	if err := os.Remove(dangling); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "openspec", "changes", "real-plan", "tasks.md"), dangling); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/x.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("plan via symlink = (%d, %q, %q), want silence", code, stdout, stderr)
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

// R3-002: the topology query runs in a child process rooted at the probe
// directory, so two queries in one process each answer for their own directory
// and neither moves this process. A chdir implementation fails both checks.
func TestPlanBuildGateTopologyQueryIsPerProbeDir(t *testing.T) {
	first, second := sharedRealPath(t.TempDir()), sharedRealPath(t.TempDir())
	// The stand-in binary answers like the proof: its own working directory.
	script := filepath.Join(t.TempDir(), "ai-specs")
	body := "#!/bin/sh\nprintf '{\"central_root\":\"%s\",\"submodule\":\"app\"}\\n' \"$PWD\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	original := planBuildGateTopologyExecutable
	t.Cleanup(func() { planBuildGateTopologyExecutable = original })
	planBuildGateTopologyExecutable = func() (string, error) { return script, nil }
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{first, second} {
		out, ok := planBuildGateCentralRootQuerier(dir)
		if !ok {
			t.Fatalf("query %s failed", dir)
		}
		if got := planBuildGateField(out, "central_root"); got != dir {
			t.Fatalf("query %s = %q, want that directory", dir, got)
		}
	}
	if after, err := os.Getwd(); err != nil || after != cwd {
		t.Fatalf("cwd = %q (err %v), want %q: the query must not move this process", after, err, cwd)
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

// planBuildCommonDir is the repository's git common directory.
func planBuildCommonDir(t *testing.T, repo string) string {
	t.Helper()
	common := gitOutput(t, repo, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}
	return sharedRealPath(common)
}

// planBuildOddRefusal is the ODD plan refusal, verbatim.
func planBuildOddRefusal(tool, rel, change, branch string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — ODD mode is active but the ODD plan " +
		"'odd/tasks/" + change + ".md' is missing, is not a regular file, or resolves outside the " +
		"repository for branch '" + branch + "'. Write the plan first, or declare mode = \"openspec\" " +
		"in ai-specs/ai-specs.toml to use the OpenSpec flow. Writing planning artifacts under odd/ " +
		"is never blocked.\n"
}

// planBuildOddBranchRefusal is the ODD detached-HEAD refusal, verbatim.
func planBuildOddBranchRefusal(tool, rel string) string {
	return "plan-build-gate: refusing to " + tool + " '" + rel + "' — ODD mode is active but the current " +
		"branch cannot be resolved (detached HEAD), so no ODD plan can be paired. Write the plan at " +
		"'odd/tasks/<change>.md' on the change branch, or declare mode = \"openspec\" in " +
		"ai-specs/ai-specs.toml to use the OpenSpec flow.\n"
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

// The paired plan allows the production write.
func TestPlanBuildGateOddModeAllowsWithPlan(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteOddPlan(t, repo, "odd-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A missing plan refuses and names the path and both escapes.
func TestPlanBuildGateOddModeRefusesWithoutPlan(t *testing.T) {
	repo, branch := oddRepo(t)
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}

// A directory named like the plan is not a plan.
func TestPlanBuildGateOddModePlanMustBeRegularFile(t *testing.T) {
	repo, branch := oddRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "odd", "tasks", "odd-change.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}

// An OpenSpec tasks.md never satisfies the ODD gate.
func TestPlanBuildGateOddModeIgnoresOpenspecTasks(t *testing.T) {
	repo, branch := oddRepo(t)
	planBuildWritePlan(t, repo, "openspec-change")
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}

// Writing the plan that the gate requires is never blocked.
func TestPlanBuildGateOddModeAllowsPlanWrites(t *testing.T) {
	repo, _ := oddRepo(t)
	for _, target := range []string{"odd/tasks/odd-change.md", "odd/changes/odd-change/proposal.md"} {
		if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", target)); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("%s = (%d, %q, %q), want silence", target, code, stdout, stderr)
		}
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "odd", "tasks", "odd-change.md"))
}

// A symlink at the plan path is not a regular file of the repository: the gate
// refuses instead of following the link to a file elsewhere.
func TestPlanBuildGateOddModePlanMustNotBeASymlink(t *testing.T) {
	repo, branch := oddRepo(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	if err := os.WriteFile(outside, []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "odd", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "odd", "tasks", "odd-change.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}

// A branch with no commits yet is still a branch: HEAD is unborn, not detached,
// so the paired plan resolves and the write is allowed.
func TestPlanBuildGateOddModeAllowsOnAnUnbornBranch(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-b", "feat/odd-change")
	planBuildWriteMode(t, repo, "odd")
	planBuildWriteOddPlan(t, repo, "odd-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate on an unborn branch = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// A symlink in a parent component is not containment: odd/tasks pointing outside
// the repository does not satisfy the paired-plan rule.
func TestPlanBuildGateOddModeRefusesASymlinkedTasksDir(t *testing.T) {
	repo, branch := oddRepo(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "odd-change.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(repo, "odd", "tasks")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "odd", "tasks")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
}

// A detached HEAD refuses instead of pairing an arbitrary plan.
func TestPlanBuildGateOddModeDetachedHeadBlocks(t *testing.T) {
	repo, _ := oddRepo(t)
	planBuildWriteOddPlan(t, repo, "odd-change")
	gitTest(t, repo, "checkout", "--detach")
	want := planBuildOddBranchRefusal("Write", "src/app.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}

// A declared openspec mode is byte-identical to the pre-change gate: the
// nearest-root refusal without a change folder, silence with one.
func TestPlanBuildGateDeclaredOpenspecIsUnchanged(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "openspec")
	want := planBuildNearestRefusal("Write", "src/app.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
	planBuildWritePlan(t, repo, "some-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate with a plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// auto follows the Gentle AI state directory: present behaves as odd.
func TestPlanBuildGateAutoInfersOddFromStateDir(t *testing.T) {
	repo, branch := oddRepo(t)
	planBuildWriteMode(t, repo, "auto")
	if err := os.MkdirAll(filepath.Join(planBuildCommonDir(t, repo), "gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := planBuildOddRefusal("Write", "src/app.go", "odd-change", branch)
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
	planBuildWriteOddPlan(t, repo, "odd-change")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate with a plan = (%d, %q, %q), want silence", code, stdout, stderr)
	}
}

// auto without the state directory stays on the OpenSpec flow.
func TestPlanBuildGateAutoWithoutStateDirStaysOpenspec(t *testing.T) {
	repo := sharedRealPath(newGateFixture(t))
	planBuildWriteMode(t, repo, "auto")
	want := planBuildNearestRefusal("Write", "src/app.go")
	if code, stdout, stderr := runPlanBuildGateCLI(t, repo, planBuildGateEvent(repo, "Write", "src/app.go")); code != 2 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", want)
	}
	planBuildAssertAbsent(t, filepath.Join(repo, "src", "app.go"))
}
