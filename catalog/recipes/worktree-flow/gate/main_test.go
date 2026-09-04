package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI invokes run() with an empty stdin and returns exit code, stdout and
// stderr so the whole flag/exit contract is testable in-process.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestVersionFlag(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--version")
	if code != 0 {
		t.Fatalf("--version exit = %d, want 0", code)
	}
	if stdout == "" {
		t.Fatalf("--version printed empty stdout, want a version")
	}
	if stderr != "" {
		t.Fatalf("--version stderr = %q, want empty", stderr)
	}
}

func TestSelftestOK(t *testing.T) {
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		if _, err2 := os.Stat("/bin/git"); err2 != nil {
			t.Skip("git not at a standard path; environment-dependent")
		}
	}
	code, stdout, stderr := runCLI(t, "--selftest")
	if code != 0 {
		t.Fatalf("--selftest exit = %d, want 0; stderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("--selftest stdout = %q, want %q", stdout, "ok\n")
	}
	if stderr != "" {
		t.Fatalf("--selftest stderr = %q, want empty", stderr)
	}
}

func TestSelftestFailsWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty PATH: git unresolvable
	code, stdout, stderr := runCLI(t, "--selftest")
	if code != 1 {
		t.Fatalf("--selftest without git exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("--selftest without git stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "selftest") {
		t.Fatalf("--selftest without git stderr = %q, want a selftest failure", stderr)
	}
}

func TestUnknownFlagFailsOpen(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--bogus")
	if code != 0 {
		t.Fatalf("unknown flag exit = %d, want 0 (fail-open)", code)
	}
	if stdout != "" {
		t.Fatalf("unknown flag stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "warning") {
		t.Fatalf("unknown flag stderr = %q, want a warning", stderr)
	}
}

func TestPlainRunFailsOpenWithEmptyStdout(t *testing.T) {
	code, stdout, stderr := runCLI(t)
	if code != 0 {
		t.Fatalf("plain run exit = %d, want 0", code)
	}
	if stdout != "" {
		t.Fatalf("plain run stdout = %q, want empty (only --version/--selftest/--explain write stdout)", stdout)
	}
	if stderr != "" {
		t.Fatalf("plain run stderr = %q, want empty", stderr)
	}
}

func TestGateModeOffExitsZero(t *testing.T) {
	code, stdout, _ := runCLI(t, "--gate-mode", "off")
	if code != 0 {
		t.Fatalf("--gate-mode off exit = %d, want 0", code)
	}
	if stdout != "" {
		t.Fatalf("--gate-mode off stdout = %q, want empty", stdout)
	}
}

func TestExplainEmitsJSONDiagnostic(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--explain", "--gate-mode", "always", "--gate-scope", "auto")
	if code != 0 {
		t.Fatalf("--explain exit = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("--explain stderr = %q, want empty", stderr)
	}
	var diag explainOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &diag); err != nil {
		t.Fatalf("--explain stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if diag.GateMode != "always" || diag.GateScope != "auto" {
		t.Fatalf("--explain diag = %+v, want gate_mode=always gate_scope=auto", diag)
	}
	if diag.Decision != "allow" {
		t.Fatalf("--explain decision = %q, want %q (Phase 0 skeleton)", diag.Decision, "allow")
	}
}

func TestFlagParseErrorFailsOpen(t *testing.T) {
	// A malformed flag VALUE must not abort either: warn and exit 0.
	code, stdout, stderr := runCLI(t, "--gate-mode")
	if code != 0 {
		t.Fatalf("missing flag value exit = %d, want 0 (fail-open)", code)
	}
	if stdout != "" {
		t.Fatalf("missing flag value stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "warning") {
		t.Fatalf("missing flag value stderr = %q, want a warning", stderr)
	}
}

func TestEffectiveBase(t *testing.T) {
	base, degrade := effectiveBase(WriteCandidate{Path: "/abs/file", Source: cwdSourceNone})
	if degrade || base != "" {
		t.Fatalf("absolute: base=%q degrade=%v, want no base no degrade", base, degrade)
	}
	base, degrade = effectiveBase(WriteCandidate{Path: "rel", Base: "/wt", Source: cwdSourceCommand})
	if degrade || base != "/wt" {
		t.Fatalf("command source: base=%q degrade=%v", base, degrade)
	}
	base, degrade = effectiveBase(WriteCandidate{Path: "rel", Base: "/evt", Source: cwdSourceEvent})
	if degrade || base != "/evt" {
		t.Fatalf("event source: base=%q degrade=%v", base, degrade)
	}
	base, degrade = effectiveBase(WriteCandidate{Path: "rel", Source: cwdSourceNone})
	if !degrade || base != "" {
		t.Fatalf("none+relative: base=%q degrade=%v, want degrade", base, degrade)
	}
}

func runEvent(t *testing.T, args []string, payload map[string]interface{}) (int, string, string) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(args, bytes.NewReader(data), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func shellEvent(cwd interface{}, command string) map[string]interface{} {
	m := map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": command},
	}
	if cwd != nil {
		m["cwd"] = cwd
	}
	return m
}

func pathEvent(cwd interface{}, filePath string) map[string]interface{} {
	m := map[string]interface{}{
		"tool_name":  "Write",
		"tool_input": map[string]interface{}{"file_path": filePath},
	}
	if cwd != nil {
		m["cwd"] = cwd
	}
	return m
}

func withCwd(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestRunCwdFidelityMatrix(t *testing.T) {
	primary := gitFixture(t, "main")
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("git", "-C", primary, "worktree", "add", "-q", "-b", "feat", wt).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	argsAlways := []string{"--gate-mode", "always", "--gate-scope", "auto", "--protected", "main development"}

	t.Run("git -C worktree relative allow", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "git -C "+wt+" mv rel-a rel-b"))
		if code != 0 {
			t.Fatalf("exit %d stderr=%q, want allow", code, stderr)
		}
		if strings.Contains(stderr, "protected-branch") {
			t.Fatalf("stderr=%q", stderr)
		}
	})

	t.Run("cd worktree relative allow", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "cd "+wt+" && echo x > rel"))
		if code != 0 {
			t.Fatalf("exit %d stderr=%q, want allow", code, stderr)
		}
	})

	t.Run("cd worktree then git -C primary blocks", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "cd "+wt+" && git -C "+primary+" mv a b"))
		if code != 2 {
			t.Fatalf("exit %d stderr=%q, want block", code, stderr)
		}
		if !strings.Contains(stderr, "/worktree-new") {
			t.Fatalf("primary block must keep /worktree-new: %q", stderr)
		}
	})

	t.Run("git -C A && echo rel uses event cwd and blocks", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "git -C "+wt+" && echo x > rel"))
		if code != 2 {
			t.Fatalf("overlay trap: exit %d stderr=%q, want block on event cwd", code, stderr)
		}
	})

	t.Run("cd - degrades in always", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "cd - && echo x > rel"))
		if code != 0 {
			t.Fatalf("cd - exit %d stderr=%q, want 0", code, stderr)
		}
		if strings.Contains(stderr, "protected-branch") || strings.Contains(stderr, "/worktree-new") {
			t.Fatalf("degrade must not block: %q", stderr)
		}
		if stderr == "" || !strings.Contains(stderr, DegradeMessage("always")) && !strings.Contains(stderr, "command cwd") {
			t.Fatalf("want DegradeMessage: %q", stderr)
		}
	})

	t.Run("cd dollar WT degrades", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, `cd "$WT" && echo x > rel`))
		if code != 0 {
			t.Fatalf("exit %d stderr=%q, want degrade", code, stderr)
		}
	})

	t.Run("echo rel trusted event cwd still blocks", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "echo x > rel"))
		if code != 2 {
			t.Fatalf("exit %d stderr=%q, want 2", code, stderr)
		}
	})

	t.Run("omitted event cwd relative does not block on PWD", func(t *testing.T) {
		withCwd(t, primary)
		code, _, stderr := runEvent(t, argsAlways, shellEvent(nil, "echo x > rel"))
		if code != 0 {
			t.Fatalf("omitted cwd exit %d stderr=%q, want 0 (not $PWD block)", code, stderr)
		}
		if strings.Contains(stderr, "protected-branch") {
			t.Fatalf("must not block-on-guess: %q", stderr)
		}
	})

	t.Run("cd protected primary blocks with worktree-new", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "cd "+primary+" && echo x > f"))
		if code != 2 {
			t.Fatalf("exit %d stderr=%q, want 2", code, stderr)
		}
		if !strings.Contains(stderr, "/worktree-new") || !strings.Contains(stderr, primary) {
			t.Fatalf("must name cwd and /worktree-new: %q", stderr)
		}
	})

	t.Run("absolute path inside primary blocks unchanged", func(t *testing.T) {
		abs := filepath.Join(primary, "abs.txt")
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "echo x > "+abs))
		if code != 2 {
			t.Fatalf("abs shell exit %d stderr=%q, want 2", code, stderr)
		}
		code, _, stderr = runEvent(t, argsAlways, pathEvent(nil, abs))
		if code != 2 {
			t.Fatalf("abs path-mode exit %d stderr=%q, want 2", code, stderr)
		}
	})

	t.Run("ask mode degrade exits 0 with DegradeMessage", func(t *testing.T) {
		args := []string{"--gate-mode", "ask", "--gate-scope", "auto", "--protected", "main development"}
		code, _, stderr := runEvent(t, args, shellEvent(primary, "cd - && echo x > rel"))
		if code != 0 {
			t.Fatalf("ask degrade exit %d, want 0", code)
		}
		if !strings.Contains(stderr, DegradeMessage("ask")) {
			t.Fatalf("stderr=%q, want ask DegradeMessage", stderr)
		}
		if strings.Contains(stderr, "WORKTREE_GATE_MODE=off") {
			t.Fatalf("degrade must not advertise off: %q", stderr)
		}
	})

	t.Run("gate_mode off exits 0 before recovery", func(t *testing.T) {
		args := []string{"--gate-mode", "off", "--protected", "main development"}
		code, stdout, stderr := runEvent(t, args, shellEvent(primary, "echo x > rel"))
		if code != 0 || stdout != "" {
			t.Fatalf("off exit %d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if strings.Contains(stderr, "command cwd") || strings.Contains(stderr, "refusing") {
			t.Fatalf("off must not evaluate: %q", stderr)
		}
	})

	t.Run("path relative trusted event still gates", func(t *testing.T) {
		code, _, stderr := runEvent(t, argsAlways, pathEvent(primary, "a.py"))
		if code != 2 {
			t.Fatalf("path relative exit %d stderr=%q, want 2", code, stderr)
		}
	})

	t.Run("blocked extra primary names cwd no worktree-new", func(t *testing.T) {
		other := gitFixture(t, "main")
		code, _, stderr := runEvent(t, argsAlways, shellEvent(primary, "cd "+other+" && echo x > f"))
		if code != 2 {
			t.Fatalf("extra primary exit %d stderr=%q, want 2", code, stderr)
		}
		if !strings.Contains(stderr, other) {
			t.Fatalf("must name other cwd: %q", stderr)
		}
		if strings.Contains(stderr, "/worktree-new") {
			t.Fatalf("non-session primary must not suggest /worktree-new: %q", stderr)
		}
	})
}

func TestExplainRunCommandCwdSource(t *testing.T) {
	primary := gitFixture(t, "main")
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("git", "-C", primary, "worktree", "add", "-q", "-b", "feat", wt).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	args := []string{"--explain", "--gate-mode", "always", "--gate-scope", "auto", "--protected", "main development"}
	code, stdout, stderr := runEvent(t, args, shellEvent(primary, "git -C "+wt+" mv rel-a rel-b"))
	if code != 0 {
		t.Fatalf("explain exit %d stderr=%q", code, stderr)
	}
	var diag explainOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &diag); err != nil {
		t.Fatalf("explain json: %v\n%s", err, stdout)
	}
	if diag.Decision != "allow" {
		t.Fatalf("decision=%q reason=%q, want allow", diag.Decision, diag.Reason)
	}
	if diag.CwdSource != string(cwdSourceCommand) {
		t.Fatalf("cwd_source=%q, want command", diag.CwdSource)
	}
	if diag.CommandCwd != wt {
		t.Fatalf("command_cwd=%q, want %s", diag.CommandCwd, wt)
	}
	if diag.Cwd != primary {
		t.Fatalf("Cwd diagnostic=%q, want event cwd %s", diag.Cwd, primary)
	}
	if diag.Reason == "protected-branch" {
		t.Fatal("explain must not report protected-branch for linked worktree git -C")
	}
}
