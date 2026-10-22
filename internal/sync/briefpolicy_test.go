package sync

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writePolicyManifest writes one manifest into a temp dir and returns its path.
func writePolicyManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

// TestBriefRenderPolicy pins brief-render-policy.py's semantics:
//   - absent/[brief].render = true → enabled;
//   - [brief].render = false → disabled;
//   - any non-boolean render value → fail-safe enabled in the default mode,
//     refused by --validate with the byte-exact ValueError message.
func TestBriefRenderPolicy(t *testing.T) {
	cases := []struct {
		name         string
		manifest     string
		wantEnabled  bool
		wantValidate string // "" = validation passes
	}{
		{
			name:        "absent defaults to enabled",
			manifest:    "[project]\nname = 'x'\n",
			wantEnabled: true,
		},
		{
			name:        "render true",
			manifest:    "[project]\nname = 'x'\n\n[brief]\nrender = true\n",
			wantEnabled: true,
		},
		{
			name:        "render false",
			manifest:    "[project]\nname = 'x'\n\n[brief]\nrender = false\n",
			wantEnabled: false,
		},
		{
			name:         "string is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = 'yes'\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got str",
		},
		{
			name:         "int is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = 1\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got int",
		},
		{
			name:         "float is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = 1.5\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got float",
		},
		{
			name:         "array is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = []\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got list",
		},
		{
			name:         "table is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = { why = 'not-a-bool' }\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got dict",
		},
		{
			name:         "big int is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = 123456789012345678901234567890\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got int",
		},
		{
			name:         "date is fail-safe enabled",
			manifest:     "[project]\nname = 'x'\n\n[brief]\nrender = 1979-05-27\n",
			wantEnabled:  true,
			wantValidate: "[brief].render must be a boolean (true or false); got date",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writePolicyManifest(t, tc.manifest)
			enabled, err := EvaluateBriefRender(path)
			if err != nil {
				t.Fatalf("EvaluateBriefRender error: %v", err)
			}
			if enabled != tc.wantEnabled {
				t.Errorf("enabled = %v, want %v", enabled, tc.wantEnabled)
			}
			verr := ValidateBriefRender(path)
			if tc.wantValidate == "" {
				if verr != nil {
					t.Errorf("ValidateBriefRender error = %v, want nil", verr)
				}
				return
			}
			if verr == nil {
				t.Fatalf("ValidateBriefRender = nil, want %q", tc.wantValidate)
			}
			if verr.Error() != tc.wantValidate {
				t.Errorf("ValidateBriefRender message:\n  go: %q\n want: %q", verr.Error(), tc.wantValidate)
			}
		})
	}
}

// TestBriefRenderPolicyMissingFile pins the load-error contract: a missing
// manifest is an error (the caller prints `error: ...` and treats the gate as
// disabled), matching brief-render-policy.py's FileNotFoundError branch.
func TestBriefRenderPolicyMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.toml")
	if _, err := EvaluateBriefRender(missing); err == nil {
		t.Errorf("EvaluateBriefRender(missing) = nil error, want error")
	}
	if err := ValidateBriefRender(missing); err == nil {
		t.Errorf("ValidateBriefRender(missing) = nil error, want error")
	}
}

// runPythonBriefPolicy runs the REAL brief-render-policy.py and returns its
// stdout, stderr and exit code — the oracle for the native policy core.
func runPythonBriefPolicy(t *testing.T, root, tomlPath string, validate bool) (string, string, int) {
	t.Helper()
	script := filepath.Join(root, "lib", "_internal", "brief-render-policy.py")
	args := []string{script, tomlPath}
	if validate {
		args = append(args, "--validate")
	}
	cmd := exec.Command("python3", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	rc := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run brief-render-policy.py: %v", err)
		}
		rc = exitErr.ExitCode()
	}
	return out.String(), errBuf.String(), rc
}

// TestBriefRenderPolicyDifferential compares the native policy core against the
// REAL brief-render-policy.py for every fixture the sync gate can meet: a
// missing [brief] table, render = true, render = false, an invalid type, bad
// TOML, and a missing manifest. Exit code and stdout must match byte for byte.
//
// stderr is compared only as a class, not byte-wise: the two interpreters word
// the underlying error differently (Python's `[Errno 2] ...` / tomllib message
// versus Go's `open ...` / parse diagnostic). Both fail the same way, which is
// the contract sync observes — the exit code is ignored and the gate is the
// stdout verdict alone (D3). Recorded, not fixed, like L6's parse divergence.
func TestBriefRenderPolicyDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)

	cases := []struct {
		name     string
		manifest string
		missing  bool
	}{
		{name: "missing brief table", manifest: "[project]\nname = 'x'\n"},
		{name: "render true", manifest: "[project]\nname = 'x'\n\n[brief]\nrender = true\n"},
		{name: "render false", manifest: "[project]\nname = 'x'\n\n[brief]\nrender = false\n"},
		{name: "invalid type", manifest: "[project]\nname = 'x'\n\n[brief]\nrender = 'yes'\n"},
		{name: "bad toml", manifest: "[project\nname = 'x'\n"},
		{name: "missing manifest", missing: true},
	}

	for _, tc := range cases {
		for _, validate := range []bool{false, true} {
			mode := "default"
			if validate {
				mode = "validate"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "ai-specs.toml")
				if !tc.missing {
					if err := os.WriteFile(path, []byte(tc.manifest), 0o644); err != nil {
						t.Fatalf("write manifest: %v", err)
					}
				}

				wantOut, wantErr, wantRC := runPythonBriefPolicy(t, root, path, validate)
				gotOut, gotErr, gotRC := goBriefPolicy(t, path, validate)

				if gotRC != wantRC {
					t.Errorf("rc: go=%d python=%d (stderr go=%q python=%q)", gotRC, wantRC, gotErr, wantErr)
				}
				if gotOut != wantOut {
					t.Errorf("stdout:\n  go: %q\npython: %q", gotOut, wantOut)
				}
				// stderr: both empty on success; on failure both report one
				// `error: ` line whose wording the interpreters differ on.
				if wantErr == "" {
					if gotErr != "" {
						t.Errorf("stderr: go=%q, python=%q (want empty)", gotErr, wantErr)
					}
				} else {
					if !strings.HasPrefix(gotErr, "error: ") || !strings.HasPrefix(wantErr, "error: ") {
						t.Errorf("stderr shape:\n  go: %q\npython: %q", gotErr, wantErr)
					}
				}
				// The policy only reads: the manifest must be untouched.
				if !tc.missing {
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("re-read manifest: %v", err)
					}
					if string(data) != tc.manifest {
						t.Errorf("manifest mutated by the policy: %q", data)
					}
				}
			})
		}
	}
}
