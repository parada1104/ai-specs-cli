package sync

import (
	"os"
	"path/filepath"
	"testing"
)

const spineManifest = "[project]\nname = 'spine-t7b'\n\n[agents]\nenabled = ['claude']\n"

func spineEnv(t *testing.T) (home, scratch string) {
	t.Helper()
	home = gitignoreRepoRoot(t)
	scratch = t.TempDir()
	for _, p := range []string{filepath.Join(scratch, "tmp"), filepath.Join(scratch, "home")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	t.Setenv("AI_SPECS_GATE_OFFLINE", "1")
	t.Setenv("AI_SPECS_NO_NETWORK", "1")
	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", filepath.Join(home, "tests", "fixtures", "kepano-obsidian-skills"))
	t.Setenv("TMPDIR", filepath.Join(scratch, "tmp"))
	t.Setenv("HOME", filepath.Join(scratch, "home"))
	t.Setenv("LC_ALL", "C")
	t.Setenv("PYTHONDONTWRITEBYTECODE", "1")
	return home, scratch
}

func spineProject(t *testing.T, manifest string, subrepos ...string) string {
	t.Helper()
	project := t.TempDir()
	for _, sub := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(project, "ai-specs", sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	spineWrite(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), manifest, 0o644)
	spineWrite(t, filepath.Join(project, "AGENTS.md"), "# manual runtime brief\n", 0o644)
	for _, rel := range subrepos {
		spineWrite(t, filepath.Join(project, rel, ".gitkeep"), "", 0o644)
	}
	return project
}

func spineWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
