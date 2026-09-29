// Tests for the native `doctor` dispatcher (card [Go 08]): lib/doctor.sh's
// argument handling, the frozen usage block and the frozen exit-code contract
// are reproduced by the Go route; the diagnostic itself lives in
// internal/doctor (its own tests pin the report/severity/exit framework).
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoctorProject creates a minimal real ai-specs project (manifest +
// generated-surface dirs) under root. It is deliberately small: the cli
// tests exercise dispatch and framing, not the check roster.
func writeDoctorProject(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	for _, sub := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	body := "[project]\nname = 'cli-doctor-test'\n\n[agents]\nenabled = ['claude']\n"
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// resolved returns the symlink-resolved form of an existing path, matching
// doctor.New's Path(p).resolve() semantics on the test's temp roots.
func resolved(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}
	return out
}

func runDoctorCLI(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"doctor"}, args...), home, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestDoctorRoutesNative(t *testing.T) {
	if got := Route([]string{"doctor"}).kind; got != routeDoctor {
		t.Fatalf("Route(doctor).kind = %v, want routeDoctor", got)
	}
}

func TestRunDoctorArgumentContract(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeDoctorProject(t, project)
	resolvedProject := resolved(t, project)
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	resolvedMissing := filepath.Join(resolved(t, filepath.Dir(missing)), "does-not-exist")

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
				if stdout != doctorUsage {
					t.Errorf("stdout = %q, want the frozen usage block %q", stdout, doctorUsage)
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
				if stdout != doctorUsage {
					t.Errorf("stdout = %q, want the frozen usage block %q", stdout, doctorUsage)
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
				if !strings.HasPrefix(stdout, "\nai-specs doctor\n  target: "+resolvedProject+"\n\n") {
					t.Errorf("stdout must frame the FIRST positional as the target; got %q", stdout)
				}
				if code == 2 {
					t.Errorf("exit = 2; the flag after -- must be ignored, stderr=%q", stderr)
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
				want := "ERROR: unknown flag: --bogus\nRun 'ai-specs doctor --help' for usage.\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
			},
		},
		{
			name: "one positional diagnoses that project",
			args: []string{project},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code == 2 {
					t.Errorf("exit = 2 for a single positional; stderr=%q", stderr)
				}
				if !strings.HasPrefix(stdout, "\nai-specs doctor\n  target: "+resolvedProject+"\n\n") {
					t.Errorf("stdout must frame the positional as the target; got %q", stdout)
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
			name: "nonexistent path exits 1 with the intended guard message",
			args: []string{missing},
			check: func(t *testing.T, code int, stdout, stderr string) {
				if code != 1 {
					t.Errorf("exit = %d, want 1 (frozen exit code preserved)", code)
				}
				if stdout != "" {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				want := "ERROR: " + resolvedMissing + " is not a directory.\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runDoctorCLI(t, home, tc.args...)
			tc.check(t, code, stdout, stderr)
		})
	}
}

// TestRunDoctorReportFraming pins the frozen blank-line / header / target /
// Summary framing on a real temp project.
func TestRunDoctorReportFraming(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeDoctorProject(t, project)

	code, stdout, stderr := runDoctorCLI(t, home, project)
	if code != 0 && code != 1 {
		t.Fatalf("exit = %d, want 0 or 1 (frozen rule); stderr=%q", code, stderr)
	}
	wantPrefix := "\nai-specs doctor\n  target: " + resolved(t, project) + "\n"
	if !strings.HasPrefix(stdout, wantPrefix) {
		t.Errorf("report must start with the frozen framing; got %q", stdout)
	}
	if !strings.Contains(stdout, "\nSummary: ") {
		t.Errorf("report must end with the Summary line; got %q", stdout)
	}
}
