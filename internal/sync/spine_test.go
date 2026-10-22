// T7b of U2a (odd/tasks/go-20-u2a-sync-helpers.md L10/L11).
//
// The two Python contract suites that drove the spine through a PATH-shadowed
// `python3` (tests/test_sync_run_step_errexit.py and
// tests/test_sync_output_verbosity.py) lost their injection point when the
// last Python sync step became native: L10 records that no Go→Python child
// remains on the sync path. These are their replacements, asserting the same
// observable effects at the seams the native spine actually has:
//
//   - runner.runStep with injected stepFunc steps (the errexit / capture /
//     compact / verbose contract),
//   - a PATH-shadowed mktemp for the temp-file sequence (runStep still execs
//     the real binary, on purpose — see step.go),
//   - the real Run / RunAgent entry points for framing, fan-out and notices,
//   - a PATH-shadowed python3 that fails, proving the spine spawns none.
package sync

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ── fixtures ──────────────────────────────────────────────────────────────

// spineEnv pins the hermetic environment every end-to-end case runs under and
// returns the CLI home (the repository root, where templates/ and the bundled
// assets live) and a scratch directory outside the worktree.
func spineEnv(t *testing.T) (home, scratch string) {
	t.Helper()
	home = gitignoreRepoRoot(t)
	scratch = t.TempDir()
	for _, p := range []string{filepath.Join(scratch, "tmp"), filepath.Join(scratch, "home")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	t.Setenv("AI_SPECS_GATE_OFFLINE", "1")
	t.Setenv("AI_SPECS_NO_NETWORK", "1")
	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", filepath.Join(home, "tests", "fixtures", "kepano-obsidian-skills"))
	t.Setenv("TMPDIR", filepath.Join(scratch, "tmp"))
	t.Setenv("HOME", filepath.Join(scratch, "home"))
	t.Setenv("LC_ALL", "C")
	t.Setenv("PYTHONDONTWRITEBYTECODE", "1")
	return home, scratch
}

// spineWrite writes one fixture file, creating parents.
func spineWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// spineProject builds the minimal project the Python suites used: a manifest
// with one enabled agent, empty skills/commands dirs, an existing AGENTS.md,
// and the declared subrepo directories when any are named.
func spineProject(t *testing.T, manifest string, subrepos ...string) string {
	t.Helper()
	project := t.TempDir()
	for _, sub := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(project, "ai-specs", sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	spineWrite(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), manifest, 0o644)
	spineWrite(t, filepath.Join(project, "AGENTS.md"), "# manual runtime brief\n", 0o644)
	for _, rel := range subrepos {
		spineWrite(t, filepath.Join(project, rel, ".gitkeep"), "", 0o644)
	}
	return project
}

const spineManifest = "[project]\nname = 'spine-t7b'\n\n[agents]\nenabled = ['claude']\n"

// spineFanoutManifest declares the two subrepos the fan-out cases resolve.
const spineFanoutManifest = "[project]\nname = 'spine-t7b'\nsubrepos = ['packages/a', 'packages/b']\n\n[agents]\nenabled = ['claude']\n"

// spinePlan returns the resolver plan the spine itself would consume, so a
// test compares against the resolved paths (t.TempDir is symlinked on macOS).
func spinePlan(t *testing.T, project string) plan {
	t.Helper()
	r := &runner{stderr: io.Discard}
	body, ok := r.resolvePlan(project)
	if !ok {
		t.Fatalf("resolvePlan(%s) failed", project)
	}
	var p plan
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	return p
}

// ── runStep seam ──────────────────────────────────────────────────────────

// newStepRunner builds a runner whose stream capture is the real runStep path.
func newStepRunner(t *testing.T, verbose bool) (*runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errW bytes.Buffer
	return &runner{
		home:    t.TempDir(),
		stdout:  &out,
		stderr:  &errW,
		verbose: verbose,
		env:     childEnv(t.TempDir()),
	}, &out, &errW
}

// stepFuncWriting returns a fake step that writes the given bytes to its
// captured streams and exits with rc.
func stepFuncWriting(stdout, stderr string, rc int) stepFunc {
	return func(out, errW io.Writer) int {
		io.WriteString(out, stdout)
		io.WriteString(errW, stderr)
		return rc
	}
}

// mktempShim is a PATH-shadowed mktemp that hands out recorded probe paths and
// can fail from a given call onward — the sequence control the Python suites
// got from tests/_sync_stub.py.
type mktempShim struct {
	probe string
}

func installMktempShim(t *testing.T) *mktempShim {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	probe := filepath.Join(dir, "probe")
	for _, p := range []string{bin, probe} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	spineWrite(t, filepath.Join(probe, ".count"), "0", 0o644)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + probe + `/args.log"
n=$(( $(cat "` + probe + `/.count" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "` + probe + `/.count"
fail_from="$(cat "` + probe + `/.fail_from" 2>/dev/null || true)"
if [ -n "$fail_from" ] && [ "$n" -ge "$fail_from" ]; then
    exit 1
fi
path="` + probe + `/temp$n"
: > "$path"
echo "$path"
`
	spineWrite(t, filepath.Join(bin, "mktemp"), script, 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &mktempShim{probe: probe}
}

// failFrom makes every mktemp call numbered n or later fail.
func (m *mktempShim) failFrom(t *testing.T, n int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.probe, ".fail_from"), []byte(strings.TrimSpace(itoa(n))), 0o644); err != nil {
		t.Fatalf("write .fail_from: %v", err)
	}
}

// survivors lists the probe temp files that still exist (the .count and
// .fail_from markers are not temps).
func (m *mktempShim) survivors(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(m.probe, "temp*"))
	if err != nil {
		t.Fatalf("glob survivors: %v", err)
	}
	return matches
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// shellSingleQuote wraps s so a POSIX shell reads it literally (shared with
// the remaining python3 shadow in vendorskills_test.go).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// TestRunStepPropagatesTheWrappedStatus: the wrapped command's status survives
// runStep instead of collapsing to 1 (Python: test_a_bare_failing_step_...).
func TestRunStepPropagatesTheWrappedStatus(t *testing.T) {
	r, _, _ := newStepRunner(t, false)
	if rc := r.runStep("bundled skills + commands", stepFuncWriting("", "", 7)); rc != 7 {
		t.Fatalf("runStep rc = %d, want 7", rc)
	}
	if rc := r.runStep("vendored skills", func(out, errW io.Writer) int { return 17 }); rc != 17 {
		t.Fatalf("runStep rc = %d, want 17", rc)
	}
}

// TestRunStepFailureReplaysFullUnfilteredOutput: a failing step prints its full
// captured stdout/stderr, glyph lines included, on the original streams
// (Python: test_a_failing_step_prints_its_full_output).
func TestRunStepFailureReplaysFullUnfilteredOutput(t *testing.T) {
	r, out, errW := newStepRunner(t, true) // even verbose must not matter here
	rc := r.runStep("bundled skills + commands", stepFuncWriting("STDOUT_MARK\n    ✓ detail\n", "STDERR_MARK\n", 5))
	if rc != 5 {
		t.Fatalf("runStep rc = %d, want 5", rc)
	}
	if got := out.String(); !strings.Contains(got, "STDOUT_MARK\n") || !strings.Contains(got, "    ✓ detail\n") {
		t.Errorf("stdout = %q, want the full unfiltered output", got)
	}
	if got := errW.String(); got != "STDERR_MARK\n" {
		t.Errorf("stderr = %q, want %q", got, "STDERR_MARK\n")
	}
}

// TestRunStepCleansTempsOnSuccessAndFailure: no capture temp survives either
// path (Python: test_no_temporary_files_survive_success_or_failure).
func TestRunStepCleansTempsOnSuccessAndFailure(t *testing.T) {
	shim := installMktempShim(t)
	r, _, _ := newStepRunner(t, false)

	if rc := r.runStep("bundled skills + commands", stepFuncWriting("", "", 0)); rc != 0 {
		t.Fatalf("success rc = %d", rc)
	}
	if got := shim.survivors(t); len(got) != 0 {
		t.Errorf("success path stranded temps: %v", got)
	}
	if rc := r.runStep("vendored skills", stepFuncWriting("", "", 7)); rc != 7 {
		t.Fatalf("failure rc = %d", rc)
	}
	if got := shim.survivors(t); len(got) != 0 {
		t.Errorf("failure path stranded temps: %v", got)
	}
}

// TestRunStepPartialMktempFailure: the first temp is removed when the second
// mktemp fails, and the step still runs against the live streams (Python:
// test_partial_mktemp_failure_runs_the_step_and_removes_the_first_temp).
func TestRunStepPartialMktempFailure(t *testing.T) {
	shim := installMktempShim(t)
	shim.failFrom(t, 2)
	r, out, errW := newStepRunner(t, false)

	if rc := r.runStep("bundled skills + commands", stepFuncWriting("STEP_RAN\n", "", 0)); rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if !strings.Contains(out.String(), "  syncing bundled skills + commands\n") {
		t.Errorf("step label missing: %q", out.String())
	}
	if !strings.Contains(out.String(), "STEP_RAN\n") {
		t.Errorf("the step must still run: %q", out.String())
	}
	if !strings.Contains(errW.String(), "  ! cannot create temporary files") {
		t.Errorf("stderr = %q, want the degraded-path announcement", errW.String())
	}
	if got := shim.survivors(t); len(got) != 0 {
		t.Errorf("the first temp leaked when the second failed: %v", got)
	}
}

// TestRunStepDegradedPathPropagatesTheWrappedStatus: a mktemp failure must not
// swallow the wrapped command's rc (Python:
// test_degraded_path_forwards_the_wrapped_status).
func TestRunStepDegradedPathPropagatesTheWrappedStatus(t *testing.T) {
	shim := installMktempShim(t)
	shim.failFrom(t, 1)
	r, _, _ := newStepRunner(t, false)
	if rc := r.runStep("bundled skills + commands", stepFuncWriting("", "", 17)); rc != 17 {
		t.Fatalf("rc = %d, want 17", rc)
	}
}

// TestRunStepDegradedPathAnnouncesUnfilteredOutput: with no captures there is
// no filtering, and the warning says so (Python:
// test_degraded_path_output_is_documented_as_unfiltered).
func TestRunStepDegradedPathAnnouncesUnfilteredOutput(t *testing.T) {
	shim := installMktempShim(t)
	shim.failFrom(t, 1)
	r, out, errW := newStepRunner(t, false)

	if rc := r.runStep("bundled skills + commands", stepFuncWriting("  ✓ detail line\n", "", 0)); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if !strings.Contains(out.String(), "  ✓ detail line\n") {
		t.Errorf("output must not be lost: %q", out.String())
	}
	if combined := strings.ToLower(out.String() + errW.String()); !strings.Contains(combined, "unfiltered") {
		t.Errorf("the warning must state that output is unfiltered: %q", combined)
	}
}

// TestRunStepMktempFailureNamesItself: a TMPDIR problem must not masquerade as
// the wrapped command failing (Python: test_mktemp_failure_names_itself).
func TestRunStepMktempFailureNamesItself(t *testing.T) {
	shim := installMktempShim(t)
	shim.failFrom(t, 1)
	r, out, errW := newStepRunner(t, false)

	if rc := r.runStep("bundled skills + commands", stepFuncWriting("", "", 0)); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	combined := strings.ToLower(out.String() + errW.String())
	if !strings.Contains(combined, "temporary") && !strings.Contains(combined, "tmpdir") {
		t.Errorf("mktemp failure was not named: %q", combined)
	}
}

// TestRunStepCompactFilterClassification pins the compact filter's exact
// classification through the capture path: glyph/blank lines dropped, notices
// and ordinary lines kept on their original streams (Python: t2_1, t2_2 and
// test_compact_filter_is_implementation_independent).
func TestRunStepCompactFilterClassification(t *testing.T) {
	const stdoutPayload = "" +
		"    ✓ bundled skill worktree-flow\n" +
		"    · symlink ok\n" +
		"    ⇢ flattened 1\n" +
		"    ▸ recipe session-context\n" +
		"    · template skipped (exists) ai-specs/foo.md\n" +
		"\n" +
		"   \n" +
		"keep-me\n" +
		"  ! warning\n" +
		"  ✗ error\n" +
		"  ℹ notice\n" +
		"plain-out\n" +
		"  ! warn-stdout\n"
	const stderrPayload = "  ✗ err-stderr\n  ℹ note-stderr\n"

	r, out, errW := newStepRunner(t, false)
	if rc := r.runStep("bundled skills + commands", stepFuncWriting(stdoutPayload, stderrPayload, 0)); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	got := out.String()
	for _, keep := range []string{"keep-me\n", "plain-out\n", "  ! warning\n", "  ✗ error\n", "  ℹ notice\n", "  ! warn-stdout\n"} {
		if !strings.Contains(got, keep) {
			t.Errorf("compact stdout dropped %q:\n%s", keep, got)
		}
	}
	for _, drop := range []string{"    ✓ bundled skill worktree-flow", "    · symlink ok", "    ⇢ flattened 1", "    ▸ recipe session-context", "template skipped"} {
		if strings.Contains(got, drop) {
			t.Errorf("compact stdout kept noisy line %q:\n%s", drop, got)
		}
	}
	// Stream separation: the stub notices must not cross streams.
	if strings.Contains(got, "err-stderr") || strings.Contains(got, "note-stderr") {
		t.Errorf("stderr notices leaked into stdout:\n%s", got)
	}
	if want := stderrPayload; errW.String() != want {
		t.Errorf("compact stderr = %q, want %q", errW.String(), want)
	}
	if strings.Contains(errW.String(), "warn-stdout") {
		t.Errorf("stdout notice leaked into stderr: %q", errW.String())
	}
}

// TestRunStepVerboseReplaysFullOutput: --verbose reproduces the step's full
// output byte-for-byte, including trailing blank lines (Python: t2_3, M3 and
// the verbose half of test_m1_template_skipped_cli_compact_drops_and_verbose_keeps).
func TestRunStepVerboseReplaysFullOutput(t *testing.T) {
	const payload = "    ✓ a\n    · b\n    · template skipped (exists) ai-specs/foo.md\n  ! c\ndetail\n\n\n"
	r, out, errW := newStepRunner(t, true)
	if rc := r.runStep("bundled skills + commands", stepFuncWriting(payload, "  ✗ d\n", 0)); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if want := "  syncing bundled skills + commands\n" + payload; out.String() != want {
		t.Errorf("verbose stdout = %q, want %q", out.String(), want)
	}
	if want := "  ✗ d\n"; errW.String() != want {
		t.Errorf("verbose stderr = %q, want %q", errW.String(), want)
	}
}

// TestRunStepFailureIsUnfilteredInBothModes: failure always prints the full
// stdout+stderr and propagates the status, with or without --verbose (Python:
// test_t2_4_failing_step_prints_full_output_and_status_both_modes).
func TestRunStepFailureIsUnfilteredInBothModes(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		r, out, errW := newStepRunner(t, verbose)
		rc := r.runStep("bundled skills + commands", stepFuncWriting("    ✓ detail\n    · more\n", "diag on stderr\n", 7))
		if rc != 7 {
			t.Fatalf("verbose=%v: rc = %d, want 7", verbose, rc)
		}
		if got := out.String(); !strings.Contains(got, "  syncing bundled skills + commands\n") ||
			!strings.Contains(got, "    ✓ detail\n") || !strings.Contains(got, "    · more\n") {
			t.Errorf("verbose=%v: stdout = %q, want the full unfiltered output", verbose, got)
		}
		if got := errW.String(); got != "diag on stderr\n" {
			t.Errorf("verbose=%v: stderr = %q", verbose, got)
		}
	}
}

// ── the spawn guard ───────────────────────────────────────────────────────

// TestSyncSpineSpawnsNoPython replaces TestSyncBundledStepIsNative and
// TestSyncAgentsStepIsNative: since L10 no sync step is a Python child, so the
// guard is the inverse of theirs. A python3 shim that logs its argv and exits 1
// must be reached by nothing while the native spine still does its work.
func TestSyncSpineSpawnsNoPython(t *testing.T) {
	logPath := failingPythonStub(t)
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)

	// Non-vacuity: the shim is provably on PATH and provably logs. Invoke it
	// once directly, require the invocation to fail (it exits 1) and the log to
	// appear, then remove the log so the spine's own run must leave it empty.
	// A python step reintroduced later would make the guarded read below fail.
	if err := exec.Command("python3", "-c", "pass").Run(); err == nil {
		t.Fatal("the failing python3 shim must exit non-zero")
	}
	if logged, err := os.ReadFile(logPath); err != nil || len(logged) == 0 {
		t.Fatalf("the python3 shim did not log its own invocation (err=%v, log=%q)", err, logged)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("reset the shim log: %v", err)
	}

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}
	if logged, err := os.ReadFile(logPath); err == nil {
		t.Fatalf("sync still spawns python3; log:\n%s", logged)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read python3 invocation log: %v", err)
	}

	// The native steps ran: their labels are printed and their file effects
	// exist (the labels are compact-mode visible; the ⇢ detail lines are not).
	for _, label := range []string{
		"  syncing ai-specs/.gitignore\n",
		"  syncing root .gitignore (agent block)\n",
		"  syncing bundled skills + commands\n",
		"  syncing vendored skills\n",
		"  syncing harness env (.envrc + ai-specs.env.example)\n",
		"  syncing AGENTS.md\n",
	} {
		if !strings.Contains(out.String(), label) {
			t.Errorf("stdout missing native step label %q:\n%s", label, out.String())
		}
	}
	for _, path := range []string{
		filepath.Join(project, "ai-specs", ".gitignore"),
		filepath.Join(project, "AGENTS.md"),
		filepath.Join(project, "CLAUDE.md"),
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("native step effect missing: %s (%v)", path, err)
		}
	}
}

// ── spine-level contracts ─────────────────────────────────────────────────

// TestSyncAbortsAfterAFailingStep: a later step's failure aborts the pipeline
// at that step instead of being swallowed (Python:
// test_errexit_is_active_after_a_successful_step). The failing step is real: an
// unmatched managed-block begin marker fails the root .gitignore refresh (D2),
// after the first step has already succeeded.
func TestSyncAbortsAfterAFailingStep(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)
	spineWrite(t, filepath.Join(project, ".gitignore"),
		gitignoreRootMarkerBegin+"\nstale\n", 0o644)

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != 1 {
		t.Fatalf("sync rc = %d, want 1 (stderr=%s)", rc, errW.String())
	}
	if !strings.Contains(out.String(), "  syncing ai-specs/.gitignore\n") {
		t.Errorf("the first step must have run:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  syncing root .gitignore (agent block)\n") {
		t.Errorf("the failing step must be announced:\n%s", out.String())
	}
	if !strings.Contains(errW.String(), "found begin marker without matching end marker") {
		t.Errorf("stderr = %q, want the step's own failure", errW.String())
	}
	for _, later := range []string{
		"  syncing bundled skills + commands\n",
		"  syncing AGENTS.md\n",
		"✓ ai-specs sync complete\n",
	} {
		if strings.Contains(out.String(), later) {
			t.Errorf("errexit was not honored: later output %q present:\n%s", later, out.String())
		}
	}
}

// TestSyncFailureAfterRecipeBlockPropagates replaces the Python
// RecipeCaptureContractTests.test_failures_after_the_block_still_propagate: its
// PATH stub of agents-render.py (rc 9) lost its seam when T3 made that step
// native (L8), so the stub never ran and the sync exited 0. The contract is
// PROPAGATION, not the literal 9: a step that runs after the recipe block and
// fails must make the whole sync exit with that step's own non-zero status and
// surface its message. The failure here is real — an invalid [brief] mode fails
// the native AGENTS.md step — and the expected status is measured from the
// native step rather than hard-coded.
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

// TestSyncResolvesTargetsBeforeAnyWrite: a failing target resolution aborts
// before any writes or the header (Python:
// test_t4_1_target_resolution_failure_aborts_before_the_header).
func TestSyncResolvesTargetsBeforeAnyWrite(t *testing.T) {
	home, _ := spineEnv(t)
	project := t.TempDir() // no manifest: the resolver's own failure

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != 1 {
		t.Fatalf("sync rc = %d, want 1", rc)
	}
	if !strings.Contains(errW.String(), "ERROR: target resolution failed before any writes.\n") {
		t.Errorf("stderr = %q", errW.String())
	}
	if !strings.HasPrefix(errW.String(), "error: ") {
		t.Errorf("resolver diagnostic must precede the framing line: %q", errW.String())
	}
	if strings.Contains(out.String(), "ai-specs sync") {
		t.Errorf("stdout must stay empty before resolution succeeds:\n%s", out.String())
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("read project: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("resolution failed after writes: %v", entries)
	}
}

// TestSyncFanoutPassesOncePerTarget: exactly one parent pass per resolved
// target, one header and one footer — the parent never falls through into the
// single-target body (Python: test_t1_1_public_root_fanout_...).
func TestSyncFanoutPassesOncePerTarget(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
	p := spinePlan(t, project)
	if len(p.Targets) != 3 {
		t.Fatalf("plan targets = %d, want 3 (%v)", len(p.Targets), p.Targets)
	}

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}
	for _, target := range p.Targets {
		if got := strings.Count(out.String(), " → "+target.Path+"\n"); got != 1 {
			t.Errorf("expected exactly one fan-out pass for %s, got %d:\n%s", target.Path, got, out.String())
		}
		if _, err := os.Stat(filepath.Join(target.Path, "ai-specs", ".gitignore")); err != nil {
			t.Errorf("target %s has no ai-specs/.gitignore: %v", target.Path, err)
		}
	}
	if got := strings.Count(out.String(), "\nai-specs sync\n"); got != 1 {
		t.Errorf("header count = %d, want 1:\n%s", got, out.String())
	}
	if got := strings.Count(out.String(), "✓ ai-specs sync complete"); got != 1 {
		t.Errorf("footer count = %d, want 1:\n%s", got, out.String())
	}
}

// TestSyncAgentFanoutFramingOnce: the standalone fan-out prints its header and
// footer once and one pass line per target; children never reprint the header
// (Python: test_t3_1_header_once_and_footer_not_for_children).
func TestSyncAgentFanoutFramingOnce(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
	p := spinePlan(t, project)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--all"}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync-agent rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}
	for _, target := range p.Targets {
		if got := strings.Count(out.String(), "  syncing "+target.Path+"\n"); got != 1 {
			t.Errorf("expected exactly one pass for %s, got %d:\n%s", target.Path, got, out.String())
		}
	}
	combined := out.String() + errW.String()
	if got := strings.Count(combined, "\nai-specs sync-agent\n"); got != 1 {
		t.Errorf("header count = %d, want 1:\n%s", got, combined)
	}
	if got := strings.Count(combined, "✓ sync-agent complete"); got != 1 {
		t.Errorf("footer count = %d, want 1:\n%s", got, combined)
	}
}

// TestSyncAgentFanoutStopsOnFirstTargetFailure: the first target's failure
// stops the loop, exits non-zero and names the target; later targets are never
// written (Python: test_t1_3_first_child_failure_stops_fanout_and_names_target).
func TestSyncAgentFanoutStopsOnFirstTargetFailure(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
	p := spinePlan(t, project)
	if len(p.Targets) < 2 {
		t.Fatalf("plan targets = %d, want >= 2", len(p.Targets))
	}
	// Fail the first target (the root) by planting a non-symlink where the
	// claude instructions link must go.
	spineWrite(t, filepath.Join(project, "CLAUDE.md"), "manual file — not a symlink\n", 0o644)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--all"}, home, strings.NewReader(""), &out, &errW); rc == 0 {
		t.Fatalf("sync-agent rc = 0, want non-zero\nstdout:\n%s", out.String())
	}
	if !strings.Contains(errW.String(), "ERROR: sync-agent failed for target: ") {
		t.Errorf("stderr = %q, want the target-failure line", errW.String())
	}
	if !strings.Contains(errW.String(), p.Targets[0].Path) {
		t.Errorf("stderr must name the failing target %s: %q", p.Targets[0].Path, errW.String())
	}
	if combined := out.String() + errW.String(); !strings.Contains(combined, "refuse to overwrite non-symlink") {
		t.Errorf("failure cause missing:\n%s", combined)
	}
	for _, later := range p.Targets[1:] {
		if _, err := os.Stat(filepath.Join(later.Path, "ai-specs", ".gitignore")); err == nil {
			t.Errorf("later target %s was processed after the first failure", later.Path)
		}
	}
}

// flattenRe is the child detail marker the verbose/compact contrast counts.
var flattenRe = regexp.MustCompile(`(?m)^\s*✓\s+flattened\b`)

// TestSyncVerboseForwardedToEveryTarget: children render full detail iff the
// parent was invoked with -v (Python: test_t2_6, test_h2_sync_verbose_... and
// test_h2_short_v_...). Both the long and the short flag are exercised.
func TestSyncVerboseForwardedToEveryTarget(t *testing.T) {
	for _, flag := range [][]string{nil, {"-v"}, {"--verbose"}} {
		home, _ := spineEnv(t)
		project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
		p := spinePlan(t, project)
		n := len(p.Targets)

		var out, errW bytes.Buffer
		args := append([]string{"sync", project}, flag...)
		if rc := Run(args, home, strings.NewReader(""), &out, &errW); rc != 0 {
			t.Fatalf("flag=%v: sync rc = %d\nstderr:\n%s", flag, rc, errW.String())
		}
		got := len(flattenRe.FindAllString(out.String(), -1))
		if len(flag) == 0 {
			if got != 0 {
				t.Errorf("compact mode must filter child detail; got %d:\n%s", got, out.String())
			}
			continue
		}
		if got < n {
			t.Errorf("flag=%v: -v must reach every child (flatten detail); got %d of %d:\n%s", flag, got, n, out.String())
		}
		if merged := regexp.MustCompile(`(?m)^\s*✓\s+merged\b`).FindAllString(out.String(), -1); len(merged) < n {
			t.Errorf("flag=%v: -v must reach every child (merge detail); got %d of %d:\n%s", flag, len(merged), n, out.String())
		}
		if !regexp.MustCompile(`✓\s+wrote\s+\S+/ai-specs/\.gitignore`).MatchString(out.String()) {
			t.Errorf("flag=%v: parent verbose detail missing (gitignore):\n%s", flag, out.String())
		}
	}

	// sync-agent's own public-root fan-out forwards the same flag to its
	// nested children: -v keeps the flatten detail, compact filters it.
	for _, verbose := range []bool{false, true} {
		home, _ := spineEnv(t)
		project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
		p := spinePlan(t, project)

		args := []string{"sync-agent", project, "--all"}
		if verbose {
			args = append(args, "-v")
		}
		var out, errW bytes.Buffer
		if rc := RunAgent(args, home, strings.NewReader(""), &out, &errW); rc != 0 {
			t.Fatalf("sync-agent verbose=%v: rc = %d\nstderr:\n%s", verbose, rc, errW.String())
		}
		got := len(flattenRe.FindAllString(out.String(), -1))
		if verbose && got < len(p.Targets) {
			t.Errorf("sync-agent -v must reach every child; got %d of %d:\n%s", got, len(p.Targets), out.String())
		}
		if !verbose && got != 0 {
			t.Errorf("sync-agent compact control showed flatten detail; got %d:\n%s", got, out.String())
		}
	}
}

// TestSyncAgentCompactHasNoDetailLeaks: standalone compact stdout leaks no
// ✓/·/⇢/▸ line except the intentional footer, while the work still ran
// (Python: test_f1_standalone_sync_agent_compact_has_no_leaked_detail_markers).
func TestSyncAgentCompactHasNoDetailLeaks(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--claude"}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync-agent rc = %d\nstderr:\n%s", rc, errW.String())
	}
	if leaked := spineLeakedDetailMarkers(out.String(), true); len(leaked) != 0 {
		t.Errorf("compact sync-agent leaked detail markers:\n%s\n\nfull stdout:\n%s", strings.Join(leaked, "\n"), out.String())
	}
	if !strings.Contains(out.String(), "✓ sync-agent complete") {
		t.Errorf("standalone footer missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  syncing claude\n") {
		t.Errorf("the work did not run (step label missing):\n%s", out.String())
	}
}

// TestSyncFanoutCompactHasNoChildDetailLeaks: the public-root fan-out leaks no
// child detail markers, prints its one footer and still runs every child
// (Python: test_f1_public_root_fanout_compact_has_no_child_detail_leaks).
func TestSyncFanoutCompactHasNoChildDetailLeaks(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineFanoutManifest, "packages/a", "packages/b")
	p := spinePlan(t, project)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--all"}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync-agent rc = %d\nstderr:\n%s", rc, errW.String())
	}
	if leaked := spineLeakedDetailMarkers(out.String(), true); len(leaked) != 0 {
		t.Errorf("fan-out compact mode leaked child detail:\n%s\n\nfull stdout:\n%s", strings.Join(leaked, "\n"), out.String())
	}
	if got := strings.Count(out.String(), "✓ sync-agent complete"); got != 1 {
		t.Errorf("footer count = %d, want 1:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "mode:        public root fan-out") {
		t.Errorf("fan-out mode line missing:\n%s", out.String())
	}
	if got := strings.Count(out.String(), "  syncing "); got < len(p.Targets) {
		t.Errorf("only %d syncing lines for %d targets:\n%s", got, len(p.Targets), out.String())
	}
}

// spineLeakedDetailMarkers returns every compact-mode detail line that must
// not appear, allowing the intentional top-level footer when asked.
func spineLeakedDetailMarkers(text string, allowFooter bool) []string {
	var leaked []string
	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimLeft(line, " \t\n\v\f\r")
		if stripped == "" || !hasNoisyPrefix(stripped) {
			continue
		}
		if allowFooter && stripped == "✓ sync-agent complete" {
			continue
		}
		leaked = append(leaked, line)
	}
	return leaked
}

// TestSyncMcpSkippedNoticeSurvivesCompaction: the notice keeps the ℹ marker,
// never the suppressed · (Python: test_t3_2_mcp_skipped_notice_...).
func TestSyncMcpSkippedNoticeSurvivesCompaction(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--claude"}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync-agent rc = %d\nstderr:\n%s", rc, errW.String())
	}
	combined := out.String() + errW.String()
	if !strings.Contains(combined, "ℹ mcp skipped (no [mcp.*] in manifest)") {
		t.Errorf("mcp skipped notice missing:\n%s", combined)
	}
	if regexp.MustCompile(`·\s*mcp skipped`).MatchString(combined) {
		t.Errorf("mcp skipped must not use the suppressed · marker:\n%s", combined)
	}
}

// TestSyncSkippedAgentsMdNoticeSurvivesCompaction: `[brief].render = false`
// keeps the ℹ marker and leaves the manual AGENTS.md untouched (Python:
// test_m2_skipped_agents_md_notice_survives_compaction).
func TestSyncSkippedAgentsMdNoticeSurvivesCompaction(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest+"\n[brief]\nrender = false\n")

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}
	combined := out.String() + errW.String()
	if !strings.Contains(combined, "ℹ skipped AGENTS.md (brief.render = false)") {
		t.Errorf("skipped AGENTS.md notice missing:\n%s", combined)
	}
	if regexp.MustCompile(`·\s*skipped AGENTS\.md`).MatchString(combined) {
		t.Errorf("skipped AGENTS.md must not use the suppressed · marker:\n%s", combined)
	}
	data, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil || string(data) != "# manual runtime brief\n" {
		t.Errorf("AGENTS.md was modified: %q %v", data, err)
	}
}

// TestSyncAgentCacheFailurePropagates: a read-only cache dies at the derivation
// seam, before any step work and without the footer (Python:
// test_t4_1_project_cache_failure_in_the_fanout_propagates).
func TestSyncAgentCacheFailurePropagates(t *testing.T) {
	_, scratch := spineEnv(t)
	project := spineProject(t, spineManifest)
	recipeMCP := filepath.Join(scratch, "recipe-mcp.json")
	spineWrite(t, recipeMCP, "{}\n", 0o644)

	home := filepath.Join(t.TempDir(), "cli-home")
	cache := filepath.Join(home, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.Chmod(cache, 0o555); err != nil {
		t.Fatalf("chmod cache: %v", err)
	}
	t.Cleanup(func() { os.Chmod(cache, 0o755) })

	var out, errW bytes.Buffer
	rc := RunAgent([]string{
		"sync-agent",
		"--source-root", project,
		"--target", project,
		"--recipe-mcp", recipeMCP,
	}, home, strings.NewReader(""), &out, &errW)
	if rc == 0 {
		t.Fatalf("sync-agent rc = 0, want non-zero\nstdout:\n%s", out.String())
	}
	combined := out.String() + errW.String()
	if !strings.Contains(combined, "cache not writable") {
		t.Errorf("cache failure not named:\n%s", combined)
	}
	if strings.Contains(combined, "✓ sync-agent complete") {
		t.Errorf("footer printed after a failure:\n%s", combined)
	}
	if strings.Contains(combined, "  syncing flatten resolved skills") {
		t.Errorf("the failure must fire before any step work:\n%s", combined)
	}
}

// TestSyncAgentSymlinkFailurePropagates: a refused instructions link exits the
// agent run non-zero and does not claim success (Python:
// test_t4_2_sync_one_agent_return_sites_propagate_symlink_failure).
func TestSyncAgentSymlinkFailurePropagates(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)
	spineWrite(t, filepath.Join(project, "CLAUDE.md"), "not-a-symlink\n", 0o644)

	var out, errW bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--claude"}, home, strings.NewReader(""), &out, &errW); rc == 0 {
		t.Fatalf("sync-agent rc = 0, want non-zero\nstdout:\n%s", out.String())
	}
	combined := out.String() + errW.String()
	if !strings.Contains(combined, "refuse to overwrite non-symlink") {
		t.Errorf("failure cause missing:\n%s", combined)
	}
	if strings.Contains(combined, "✓ sync-agent complete") {
		t.Errorf("success claimed after a swallowed failure:\n%s", combined)
	}
}
