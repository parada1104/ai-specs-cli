package planbuild

// Mode resolution (plan-build-odd-companion S1/S2/S10): one execution mode per
// repository, explicit value first, then the Gentle AI state directory probe.

import (
	"os"
	"path/filepath"
	"testing"
)

// writeManifest writes one project manifest body at the repository convention.
func writeManifest(t *testing.T, root, body string) {
	t.Helper()
	path := ManifestPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// modeManifest is a manifest declaring one mode value.
func modeManifest(value string) string {
	return "[recipes.plan-build-flow]\nenabled = true\n\n" +
		"[recipes.plan-build-flow.config]\nmode = " + value + "\n"
}

// Resolve reads [recipes.plan-build-flow.config].mode and falls back to the
// Gentle AI state probe; every unusable input resolves auto → openspec.
func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		stateDir bool
		want     string
	}{
		{"explicit odd wins without state dir", modeManifest(`"odd"`), false, ModeOdd},
		{"explicit openspec wins with state dir", modeManifest(`"openspec"`), true, ModeOpenspec},
		{"auto with state dir is odd", modeManifest(`"auto"`), true, ModeOdd},
		{"auto without state dir is openspec", modeManifest(`"auto"`), false, ModeOpenspec},
		{"missing manifest is openspec", "", false, ModeOpenspec},
		{"invalid toml is openspec", "not = = toml\n", false, ModeOpenspec},
		{"non-string mode is auto", modeManifest(`1`), false, ModeOpenspec},
		{"unknown mode is auto", modeManifest(`"weird"`), false, ModeOpenspec},
		{"whitespace mode is auto", modeManifest(`" odd "`), false, ModeOpenspec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.manifest != "" {
				writeManifest(t, root, tc.manifest)
			}
			common := filepath.Join(root, ".git")
			if tc.stateDir {
				if err := os.MkdirAll(filepath.Join(common, "gentle-ai"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := Resolve(root, common); got != tc.want {
				t.Fatalf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

// An unresolvable Git common directory cannot prove Gentle AI state, so auto
// stays on the OpenSpec flow.
func TestResolveEmptyCommonDir(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, modeManifest(`"auto"`))
	if got := Resolve(root, ""); got != ModeOpenspec {
		t.Fatalf("Resolve = %q, want %q", got, ModeOpenspec)
	}
}

// A file named gentle-ai is not the state directory.
func TestResolveStateProbeNeedsADirectory(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, ".git")
	if err := os.MkdirAll(common, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(common, "gentle-ai"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(root, common); got != ModeOpenspec {
		t.Fatalf("Resolve = %q, want %q", got, ModeOpenspec)
	}
}

// ManifestPath is the repository-wide manifest convention.
func TestManifestPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if got := ManifestPath(root); got != want {
		t.Fatalf("ManifestPath = %q, want %q", got, want)
	}
}
