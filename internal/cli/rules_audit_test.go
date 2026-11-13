// Tests for the native `rules-audit` dispatcher (card [Go 08]): the argument
// handling, the frozen usage block and the frozen exit-code contract of
// lib/rules-audit.sh are reproduced by the Go route; the scanner itself lives
// in internal/rulesaudit.
package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func runRulesAuditCLI(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"rules-audit"}, args...), home, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRulesAuditRoutesNative(t *testing.T) {
	if got := Route([]string{"rules-audit"}).kind; got != routeRulesAudit {
		t.Fatalf("Route(rules-audit).kind = %v, want routeRulesAudit", got)
	}
}

func TestRunRulesAuditArgumentContract(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	cases := []struct {
		name  string
		args  []string
		check func(t *testing.T, code int, stdout, stderr string)
	}{
		{
			name: "--help prints the frozen usage and exits 0",
			args: []string{"--help"},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 0 {
					t.Errorf("exit = %d, want 0", code)
				}
				if stdout != rulesAuditUsage {
					t.Errorf("stdout = %q, want the frozen usage block %q", stdout, rulesAuditUsage)
				}
				if stderr != "" {
					t.Errorf("stderr = %q, want empty", stderr)
				}
			},
		},
		{
			name: "-h prints the frozen usage and exits 0",
			args: []string{"-h"},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 0 {
					t.Errorf("exit = %d, want 0", code)
				}
				if stdout != rulesAuditUsage {
					t.Errorf("stdout = %q, want the frozen usage block %q", stdout, rulesAuditUsage)
				}
				if stderr != "" {
					t.Errorf("stderr = %q, want empty", stderr)
				}
			},
		},
		{
			name: "-- stops parsing and every remaining argument is ignored",
			args: []string{project, "--", "--help", missing},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if strings.Contains(stdout, "Usage:") {
					t.Errorf("-- must stop parsing; usage was printed: %q", stdout)
				}
				if code != 0 {
					t.Errorf("exit = %d, want 0 (flags after -- are ignored); stderr=%q", code, stderr)
				}
				if !json.Valid([]byte(stdout)) {
					t.Errorf("stdout must be the JSON inventory; got %q", stdout)
				}
			},
		},
		{
			name: "unknown flag exits 2 with both stderr lines",
			args: []string{"--bogus"},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 2 {
					t.Errorf("exit = %d, want 2 (frozen unknown-flag contract)", code)
				}
				if stdout != "" {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				want := "ERROR: unknown flag: --bogus\nRun 'ai-specs rules-audit --help' for usage.\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
			},
		},
		{
			name: "two positionals exits 2 with the message",
			args: []string{project, "second"},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 2 {
					t.Errorf("exit = %d, want 2 (frozen unexpected-positional contract)", code)
				}
				if stdout != "" {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				want := "ERROR: unexpected positional argument: second\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
			},
		},
		{
			name: "nonexistent path exits 2 with the raw-path guard message",
			args: []string{missing},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 2 {
					t.Errorf("exit = %d, want 2 (bash -d guard contract)", code)
				}
				if stdout != "" {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				// The guard fires before resolution: the message names the path
				// exactly as given, never a resolved form.
				want := "ERROR: not a directory: " + missing + "\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
			},
		},
		{
			name: "a real project prints the JSON inventory and exits 0",
			args: []string{project},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 0 {
					t.Errorf("exit = %d, want 0; stderr=%q", code, stderr)
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
				}
				if got := payload["schema_version"]; got != float64(1) {
					t.Errorf("schema_version = %v, want 1", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runRulesAuditCLI(t, home, tc.args...)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestRunRulesAuditDefaultsToCurrentDirectory pins the no-positional default:
// the command scans the current directory (the temp project here).
func TestRunRulesAuditDefaultsToCurrentDirectory(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Chdir(project)

	code, stdout, stderr := runRulesAuditCLI(t, home)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"schema_version": 1`) {
		t.Errorf("stdout must carry the schema_version: 1 payload; got %q", stdout)
	}
}
