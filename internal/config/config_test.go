package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestLeadingBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.WriteFile(path, append([]byte{0xef, 0xbb, 0xbf}, []byte("[project]\nname = \"demo\"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	project, ok := manifest.Table("project")
	if !ok {
		t.Fatal("project table missing")
	}
	if got, ok := project.String("name"); !ok || got != "demo" {
		t.Errorf("project.name = %q, %v; want demo, true", got, ok)
	}
}

// TestReaderErrors covers the observable error surface of the readers
// (byte-identical message text to the Python CLI wrapper's error strings).
func TestReaderErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")
	_, err := LoadManifest(missing)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if got, want := err.Error(), missing+" not found"; got != want {
		t.Errorf("missing-file error: got %q, want %q", got, want)
	}

	data, err := LoadManifest(filepath.Join("testdata", "diff_empty.toml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	_, err = ReadSection(data, "nope")
	if err == nil {
		t.Fatal("expected error for unknown section")
	}
	if got, want := err.Error(), "unknown section 'nope'"; got != want {
		t.Errorf("unknown-section error: got %q, want %q", got, want)
	}
}
