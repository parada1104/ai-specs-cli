package sync

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// checkSyncCase is one differential fixture. It writes the fixture files into
// a fresh temp dir and returns the project root and cli home the two legs run
// against.
type checkSyncCase struct {
	name   string
	ignore bool
	setup  func(t *testing.T, root, home string)
}

// TestCheckSyncDifferential runs every fixture through the real Python
// authority (lib/_internal/cli_version.py check-sync) and the native port, and
// requires the same exit code and the same stderr text. stdout is asserted
// empty for the Python leg because the shim only ever writes to stderr.
func TestCheckSyncDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repo := gitignoreRepoRoot(t)
	module := filepath.Join(repo, "lib", "_internal", "cli_version.py")

	manifest := func(body string) func(*testing.T, string, string) {
		return func(t *testing.T, root, _ string) {
			t.Helper()
			writeCheckSyncFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), body)
		}
	}
	version := func(v string) func(*testing.T, string, string) {
		return func(t *testing.T, _, home string) {
			t.Helper()
			writeCheckSyncFile(t, filepath.Join(home, "VERSION"), v)
		}
	}
	both := func(fs ...func(*testing.T, string, string)) func(*testing.T, string, string) {
		return func(t *testing.T, root, home string) {
			t.Helper()
			for _, f := range fs {
				f(t, root, home)
			}
		}
	}

	const noPin = "[project]\nname = 'x'\n"

	cases := []checkSyncCase{
		{
			name:  "no pin",
			setup: both(manifest(noPin), version("1.2.3\n")),
		},
		{
			name:  "matching pin",
			setup: both(manifest(noPin+"\n[tool]\nversion = '1.2.3'\n"), version("1.2.3\n")),
		},
		{
			name:  "matching min",
			setup: both(manifest(noPin+"\n[tool]\nmin_version = '1.0.0'\n"), version("1.2.3\n")),
		},
		{
			name:  "mismatching pin policy error",
			setup: both(manifest(noPin+"\n[tool]\nversion = '2.0.0'\n"), version("1.2.3\n")),
		},
		{
			name:  "below minimum policy error",
			setup: both(manifest(noPin+"\n[tool]\nmin_version = '2.0.0'\n"), version("1.2.3\n")),
		},
		{
			name:   "mismatching pin with policy warn",
			ignore: true,
			setup:  both(manifest(noPin+"\n[tool]\nversion = '2.0.0'\n"), version("1.2.3\n")),
		},
		{
			name:  "unknown installed version",
			setup: manifest(noPin + "\n[tool]\nversion = '1.2.3'\n"),
		},
		{
			name:  "missing manifest",
			setup: version("1.2.3\n"),
		},
		{
			name: "invalid manifest",
			setup: both(
				manifest(noPin+"\n[tool]\nversion = '1.2.3'\nmin_version = '1.0.0'\n"),
				version("1.2.3\n"),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			tc.setup(t, root, home)

			args := []string{module, "check-sync", root, home}
			if tc.ignore {
				args = append(args, "--ignore-cli-version")
			}
			cmd := exec.Command("python3", args...)
			var pyOut, pyErr bytes.Buffer
			cmd.Stdout = &pyOut
			cmd.Stderr = &pyErr
			pyRC := 0
			if err := cmd.Run(); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("run python check-sync: %v", err)
				}
				pyRC = exitErr.ExitCode()
			}
			if pyOut.Len() != 0 {
				t.Fatalf("python check-sync wrote stdout %q; the shim only writes stderr", pyOut.String())
			}

			goRC, goErr := CheckSync(root, home, tc.ignore)
			if goRC != pyRC {
				t.Errorf("exit code: native = %d, python = %d", goRC, pyRC)
			}
			if goErr != pyErr.String() {
				t.Errorf("stderr mismatch:\n native = %q\n python = %q", goErr, pyErr.String())
			}
		})
	}
}

func writeCheckSyncFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
