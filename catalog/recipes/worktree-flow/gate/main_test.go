package main

import (
	"bytes"
	"encoding/json"
	"os"
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
