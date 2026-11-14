package sync

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCaptured runs cmd through the real runExitCode and returns its exit code
// together with everything the helper wrote to the stderr writer.
func runCaptured(t *testing.T, cmd *exec.Cmd, name string) (int, string) {
	t.Helper()
	var stderr bytes.Buffer
	code := runExitCode(cmd, &stderr, name)
	return code, stderr.String()
}

// A name that is on no PATH entry is "command not found": bash exits 127.
func TestRunExitCodeCommandNotFound(t *testing.T) {
	name := "definitely-not-a-real-command-go07s1"
	code, stderr := runCaptured(t, exec.Command(name), name)
	if code != 127 {
		t.Fatalf("exit code = %d, want 127; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "command not found") {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, "command not found")
	}
}

// A file that exists but lacks the execute bit is found-but-refused: bash
// exits 126, not 127.
func TestRunExitCodeNotExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-executable.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	code, stderr := runCaptured(t, exec.Command(path), path)
	if code != 126 {
		t.Fatalf("exit code = %d, want 126; stderr = %q", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "permission denied") {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, "permission denied")
	}
}

// A successful command returns 0 and writes nothing to stderr.
func TestRunExitCodeSuccess(t *testing.T) {
	code, stderr := runCaptured(t, exec.Command("true"), "true")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

// The *exec.ExitError path is untouched: the child's own status is returned.
func TestRunExitCodeExplicitStatus(t *testing.T) {
	code, stderr := runCaptured(t, exec.Command("sh", "-c", "exit 7"), "sh")
	if code != 7 {
		t.Fatalf("exit code = %d, want 7; stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}
