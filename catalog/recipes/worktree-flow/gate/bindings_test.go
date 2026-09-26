package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// capabilityConflictOf builds the expected conflict value for one graded
// capability, matching the JSON contract emitted on stdout.
func capabilityConflictOf(id, severity string, recipes ...string) capabilityConflict {
	return capabilityConflict{Type: "capability", ID: id, Recipes: recipes, Severity: severity}
}

// TestGradeBindings mirrors resolve_bindings + check_capability_conflicts from
// the Python authority: explicit binding validation first, then auto-bind of a
// capability declared by exactly one enabled recipe, with duplicate explicit
// bindings graded fatal and unresolved ambiguity graded warning.
func TestGradeBindings(t *testing.T) {
	cases := []struct {
		name     string
		enabled  []string
		caps     map[string][]string
		explicit []manifestBinding
		want     bindingResolution
	}{
		{
			name:    "auto-binds the single provider of a capability",
			enabled: []string{"alpha", "beta"},
			caps:    map[string][]string{"alpha": {"tracker"}, "beta": {}},
			want: bindingResolution{
				Bindings:  map[string]string{"tracker": "alpha"},
				Conflicts: []capabilityConflict{},
				Errors:    []string{},
			},
		},
		{
			name:    "ambiguous capability is not bound and warns with every provider",
			enabled: []string{"alpha", "beta"},
			caps:    map[string][]string{"alpha": {"vcs"}, "beta": {"vcs"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{capabilityConflictOf("vcs", "warning", "alpha", "beta")},
				Errors:    []string{},
			},
		},
		{
			name:     "explicit binding overrides auto-bind and clears the ambiguity",
			enabled:  []string{"alpha", "beta"},
			caps:     map[string][]string{"alpha": {"vcs"}, "beta": {"vcs"}},
			explicit: []manifestBinding{{Capability: "vcs", Recipe: "beta"}},
			want: bindingResolution{
				Bindings:  map[string]string{"vcs": "beta"},
				Conflicts: []capabilityConflict{},
				Errors:    []string{},
			},
		},
		{
			name:     "explicit binding to a recipe that does not declare the capability is an error",
			enabled:  []string{"alpha", "beta"},
			caps:     map[string][]string{"alpha": {"vcs"}, "beta": {}},
			explicit: []manifestBinding{{Capability: "tracker", Recipe: "alpha"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{},
				Errors: []string{
					"explicit binding for capability 'tracker' references recipe 'alpha' which does not declare that capability",
				},
			},
		},
		{
			name:     "explicit binding to a disabled or unknown recipe is an error",
			enabled:  []string{"alpha"},
			caps:     map[string][]string{"alpha": {"vcs"}},
			explicit: []manifestBinding{{Capability: "vcs", Recipe: "ghost"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{},
				Errors: []string{
					"explicit binding for capability 'vcs' references disabled/unknown recipe 'ghost'",
				},
			},
		},
		{
			name: "an unreadable enabled recipe still references no capability",
			// "ghost" is enabled but its recipe.toml could not be read, so it
			// declares nothing: the explicit binding is refused as undeclared,
			// not as a disabled/unknown recipe.
			enabled:  []string{"alpha", "ghost"},
			caps:     map[string][]string{"alpha": {}},
			explicit: []manifestBinding{{Capability: "vcs", Recipe: "ghost"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{},
				Errors: []string{
					"explicit binding for capability 'vcs' references recipe 'ghost' which does not declare that capability",
				},
			},
		},
		{
			name:     "duplicate explicit binding is a fatal conflict and a resolution error",
			enabled:  []string{"alpha", "beta"},
			caps:     map[string][]string{"alpha": {"vcs"}, "beta": {"vcs"}},
			explicit: []manifestBinding{{Capability: "vcs", Recipe: "alpha"}, {Capability: "vcs", Recipe: "beta"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{capabilityConflictOf("vcs", "fatal", "alpha", "beta")},
				Errors:    []string{"duplicate explicit binding for capability 'vcs'"},
			},
		},
		{
			name:     "duplicate explicit binding to one recipe lists that recipe once",
			enabled:  []string{"alpha", "beta"},
			caps:     map[string][]string{"alpha": {"vcs"}, "beta": {}},
			explicit: []manifestBinding{{Capability: "vcs", Recipe: "alpha"}, {Capability: "vcs", Recipe: "alpha"}},
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{capabilityConflictOf("vcs", "fatal", "alpha")},
				Errors:    []string{"duplicate explicit binding for capability 'vcs'"},
			},
		},
		{
			name:    "single-provider capabilities auto-bind while ambiguous ones only warn",
			enabled: []string{"alpha", "beta", "gamma"},
			caps: map[string][]string{
				"alpha": {"tracker"},
				"beta":  {"vcs"},
				"gamma": {"vcs", "other"},
			},
			want: bindingResolution{
				Bindings:  map[string]string{"tracker": "alpha", "other": "gamma"},
				Conflicts: []capabilityConflict{capabilityConflictOf("vcs", "warning", "beta", "gamma")},
				Errors:    []string{},
			},
		},
		{
			name: "empty inputs resolve to an empty result",
			want: bindingResolution{
				Bindings:  map[string]string{},
				Conflicts: []capabilityConflict{},
				Errors:    []string{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gradeBindings(tc.enabled, tc.caps, tc.explicit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("gradeBindings() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestGradeBindingsJSONShape pins the exact stdout contract a later Python
// bridge consumes: one JSON object with bindings, conflicts and errors.
func TestGradeBindingsJSONShape(t *testing.T) {
	got := gradeBindings(
		[]string{"alpha", "beta", "gamma"},
		map[string][]string{"alpha": {"tracker"}, "beta": {"vcs"}, "gamma": {"vcs"}},
		[]manifestBinding{{Capability: "tracker", Recipe: "alpha"}},
	)
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// encoding/json sorts map keys, so the binding object order is deterministic.
	want := `{"bindings":{"tracker":"alpha"},"conflicts":[{"type":"capability","id":"vcs","recipes":["beta","gamma"],"severity":"warning"}],"errors":[]}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}

// TestGradeBindingsConflictOrderIsDeterministic checks that conflict output
// follows the first-seen capability order, independent of map iteration.
func TestGradeBindingsConflictOrderIsDeterministic(t *testing.T) {
	got := gradeBindings(
		[]string{"alpha", "beta"},
		map[string][]string{"alpha": {"z-cap", "a-cap"}, "beta": {"z-cap", "a-cap"}},
		nil,
	)
	wantIDs := []string{"z-cap", "a-cap"}
	if len(got.Conflicts) != len(wantIDs) {
		t.Fatalf("conflicts = %#v, want %d entries", got.Conflicts, len(wantIDs))
	}
	for i, id := range wantIDs {
		if got.Conflicts[i].ID != id {
			t.Fatalf("conflict[%d].ID = %q, want %q", i, got.Conflicts[i].ID, id)
		}
	}
}

// TestParseManifestBindings mirrors toml-read.read_bindings leniency: only
// string capability/recipe pairs survive, and an absent payload is no binding.
func TestParseManifestBindings(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []manifestBinding
		wantErr bool
	}{
		{name: "blank payload is no binding", raw: "", want: nil},
		{name: "empty array is no binding", raw: "[]", want: nil},
		{
			name: "complete pair survives",
			raw:  `[{"capability":"vcs","recipe":"alpha"}]`,
			want: []manifestBinding{{Capability: "vcs", Recipe: "alpha"}},
		},
		{
			name: "missing recipe becomes an empty string",
			raw:  `[{"capability":"vcs"}]`,
			want: []manifestBinding{{Capability: "vcs"}},
		},
		{name: "invalid JSON is an error", raw: "{", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseManifestBindings(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseManifestBindings(%q) = %#v, want an error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseManifestBindings(%q): %v", tc.raw, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseManifestBindings(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestRunResolveBindingsCommand exercises the flag wiring end to end: the
// command prints one JSON object and exits 0 even when it reports resolution
// errors, while an unusable invocation exits 2. It runs the real TOML
// acquisition subprocess, so it is skipped in short mode and without python3.
func TestRunResolveBindingsCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("command test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "alpha", "\n[[capabilities]]\nid = \"vcs\"\n")
	writeRecipeToml(t, catalogDir, "beta", "\n[[capabilities]]\nid = \"vcs\"\n")

	t.Run("missing catalog dir exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-bindings")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("invalid bindings JSON exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--bindings", "{")
		if code != 2 || stdout != "" {
			t.Fatalf("code = %d stdout = %q, want 2 with no stdout", code, stdout)
		}
	})

	t.Run("ambiguity warns but exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--recipe", "beta", "--write-witness=false")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		var got bindingResolution
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout %q is not the result JSON: %v", stdout, err)
		}
		want := bindingResolution{
			Bindings:  map[string]string{},
			Conflicts: []capabilityConflict{capabilityConflictOf("vcs", "warning", "alpha", "beta")},
			Errors:    []string{},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("result = %#v, want %#v", got, want)
		}
	})

	t.Run("resolution error is data and exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--bindings", `[{"capability":"vcs","recipe":"ghost"}]`, "--write-witness=false")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		var got bindingResolution
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout %q is not the result JSON: %v", stdout, err)
		}
		if len(got.Errors) != 1 || !strings.Contains(got.Errors[0], "disabled/unknown recipe 'ghost'") {
			t.Fatalf("errors = %#v, want one disabled/unknown recipe error", got.Errors)
		}
	})
}
