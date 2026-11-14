package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shadowPython puts a python3 on PATH that records each invocation in the
// returned log file and fails. A run that spawns python3 leaves the log behind.
func shadowPython(t *testing.T) string {
	t.Helper()
	shadow := t.TempDir()
	log := filepath.Join(shadow, "python3.log")
	script := "#!/bin/sh\necho invoked >> \"" + log + "\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(shadow, "python3"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shadow+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// TestMalformedManifestGuidanceIsNative pins U2c: an unparseable manifest is
// diagnosed by the native parser, and doctor never spawns python3 to ask
// tomllib for its text.
func TestMalformedManifestGuidanceIsNative(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project\nname = \"x\"\n")
	log := shadowPython(t)

	d := New(root, t.TempDir())
	d.Run()

	if _, err := os.Stat(log); err == nil {
		data, _ := os.ReadFile(log)
		t.Fatalf("doctor spawned python3 for a malformed manifest: %q", data)
	}

	var guidance string
	found := false
	for _, c := range d.Checks {
		if c.Name == "manifest" && c.Severity == ERROR {
			guidance, found = c.Guidance, true
		}
	}
	if !found {
		t.Fatalf("no manifest ERROR check in %+v", d.Checks)
	}
	if !strings.HasPrefix(guidance, "toml: line ") {
		t.Errorf("guidance = %q, want the native parser diagnostic (toml: line N: ...)", guidance)
	}
}
