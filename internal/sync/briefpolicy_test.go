package sync

import (
	"os"
	"path/filepath"
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
