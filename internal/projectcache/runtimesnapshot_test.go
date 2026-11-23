package projectcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The snapshot is the S2 authority: the CLI hook verbs read it instead of
// ai-specs.toml, so what sync resolved must round-trip exactly.
func TestRuntimeSnapshotRoundTrip(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	want := RuntimeSnapshot{
		GateMode:          "ask",
		GateScope:         "subrepo",
		RepoTopology:      "monorepo-submodules",
		Protected:         "main development release",
		WorktreesDir:      ".worktrees",
		IntegrationBranch: "development",
	}
	path, err := WriteRuntimeSnapshot(project, home, want)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if path != RuntimeSnapshotPath(project, home) || filepath.Base(path) != RuntimeSnapshotFile {
		t.Fatalf("path = %q, want %q", path, RuntimeSnapshotPath(project, home))
	}
	got, found, err := ReadRuntimeSnapshot(project, home)
	if err != nil || !found {
		t.Fatalf("read: found=%v err=%v", found, err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	// The write is atomic: the same directory holds no temp leftover.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".runtime-") {
			t.Errorf("temp leftover %s", entry.Name())
		}
	}
	// A rewrite replaces the values in place.
	want.GateMode = "off"
	if _, err := WriteRuntimeSnapshot(project, home, want); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got, _, _ := ReadRuntimeSnapshot(project, home); got.GateMode != "off" {
		t.Fatalf("rewrite = %+v", got)
	}
}

func TestRuntimeSnapshotAbsentAndIncomplete(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	if _, found, err := ReadRuntimeSnapshot(project, home); found || err != nil {
		t.Fatalf("absent snapshot: found=%v err=%v", found, err)
	}
	path := RuntimeSnapshotPath(project, home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		"not toml at all [",
		"[other]\ngate_mode = 'ask'\n",
		"[worktree_gate]\ngate_mode = 'ask'\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, found, err := ReadRuntimeSnapshot(project, home); found || err != nil {
			t.Fatalf("incomplete snapshot %q: found=%v err=%v", body, found, err)
		}
	}
}

// The snapshot lives inside the CLI-home cache, so a redirect there refuses
// instead of writing outside it.
func TestRuntimeSnapshotRefusesSymlinkedDestination(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := RuntimeSnapshotPath(project, home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRuntimeSnapshot(project, home, RuntimeSnapshot{GateMode: "ask"}); err == nil {
		t.Fatal("write through a symlinked snapshot must refuse")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep\n" {
		t.Fatalf("outside file changed: %q err=%v", data, err)
	}
}
