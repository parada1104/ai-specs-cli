package sync

import (
	"encoding/json"
	"os"
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

// The legacy brief-render-policy.py oracle was captured once into
// testdata/briefpolicy_golden.json: per case and per mode, the exit code and
// stdout byte for byte, plus the legacy stderr text with the temporary
// directory tokenized as <DIR> (the only host-dependent part of the payload).
// The test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const briefPolicyGoldenFile = "briefpolicy_golden.json"

// briefPolicyGoldenCases is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle of
// coverage — kept in the test, never derived from the bundle — so a bundle that
// is missing, renamed, duplicated or reordered cannot validate silently.
var briefPolicyGoldenCases = []string{
	"missing brief table",
	"render true",
	"render false",
	"invalid type",
	"bad toml",
	"missing manifest",
}

// briefPolicyGoldenModes is the per-case record order: the default gate first,
// then --validate.
var briefPolicyGoldenModes = []string{"default", "validate"}

// briefPolicyGoldenMode is one frozen legacy invocation: exit code, stdout and
// stderr exactly as brief-render-policy.py produced them.
type briefPolicyGoldenMode struct {
	Mode   string `json:"mode"`
	RC     int    `json:"rc"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// briefPolicyGoldenCase is one frozen manifest the sync gate can meet, with the
// manifest pinned so input drift fails as input drift.
type briefPolicyGoldenCase struct {
	Name     string                  `json:"name"`
	Manifest string                  `json:"manifest"`
	Missing  bool                    `json:"missing"`
	Modes    []briefPolicyGoldenMode `json:"modes"`
}

// loadBriefPolicyGolden reads the frozen bundle and validates its identity
// inventory and oracle metadata before any comparison runs.
func loadBriefPolicyGolden(t *testing.T) []briefPolicyGoldenCase {
	t.Helper()
	var bundle struct {
		Oracle map[string]string       `json:"oracle"`
		Cases  []briefPolicyGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, briefPolicyGoldenFile), &bundle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", briefPolicyGoldenFile, err)
	}
	if bundle.Oracle["python"] == "" {
		t.Fatalf("frozen oracle %s carries no python version metadata", briefPolicyGoldenFile)
	}
	names := make([]string, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		names[i] = rec.Name
		if len(rec.Modes) != len(briefPolicyGoldenModes) {
			t.Fatalf("frozen record %q has %d modes, want %d", rec.Name, len(rec.Modes), len(briefPolicyGoldenModes))
		}
	}
	validateGoldenIdentity(t, "briefpolicy", names, briefPolicyGoldenCases)
	return bundle.Cases
}

// TestBriefRenderPolicyDifferential compares the native policy core against the
// frozen legacy oracle for every fixture the sync gate can meet: a missing
// [brief] table, render = true, render = false, an invalid type, bad TOML, and a
// missing manifest, each through the default and the --validate mode. Exit code
// and stdout must match byte for byte.
//
// stderr is compared only as a class, not byte-wise: the two interpreters word
// the underlying error differently (Python's `[Errno 2] ...` / tomllib message
// versus Go's `open ...` / parse diagnostic). Both fail the same way, which is
// the contract sync observes — the exit code is ignored and the gate is the
// stdout verdict alone (D3). Recorded, not fixed, like L6's parse divergence.
func TestBriefRenderPolicyDifferential(t *testing.T) {
	frozen := loadBriefPolicyGolden(t)

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

	if len(cases) != len(briefPolicyGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(briefPolicyGoldenCases))
	}

	for i, tc := range cases {
		if tc.name != briefPolicyGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, briefPolicyGoldenCases[i])
		}
		rec := frozen[i]
		if rec.Missing != tc.missing || rec.Manifest != tc.manifest {
			t.Fatalf("frozen inputs differ from the corpus for %q:\n--- frozen ---\nmissing=%t %q\n--- corpus ---\nmissing=%t %q",
				tc.name, rec.Missing, rec.Manifest, tc.missing, tc.manifest)
		}
		for j, mode := range briefPolicyGoldenModes {
			want := rec.Modes[j]
			if want.Mode != mode {
				t.Fatalf("frozen record %q mode %d is %q, want %q", tc.name, j, want.Mode, mode)
			}
			validate := mode == "validate"
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "ai-specs.toml")
				if !tc.missing {
					if err := os.WriteFile(path, []byte(tc.manifest), 0o644); err != nil {
						t.Fatalf("write manifest: %v", err)
					}
				}

				gotOut, gotErr, gotRC := goBriefPolicy(t, path, validate)

				if gotRC != want.RC {
					t.Errorf("rc: go=%d frozen=%d (frozen stderr %q)", gotRC, want.RC, want.Stderr)
				}
				if gotOut != want.Stdout {
					t.Errorf("stdout:\n  go: %q\nfrozen: %q", gotOut, want.Stdout)
				}
				// stderr: both empty on success; on failure both report one
				// `error: ` line whose wording the interpreters differ on.
				if want.Stderr == "" {
					if gotErr != "" {
						t.Errorf("stderr: go=%q, want empty (frozen %q)", gotErr, want.Stderr)
					}
				} else if !strings.HasPrefix(gotErr, "error: ") {
					t.Errorf("stderr shape:\n  go: %q\nfrozen: %q", gotErr, want.Stderr)
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
