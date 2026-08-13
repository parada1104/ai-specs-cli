package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseEventCwdNormalization proves the event-cwd normalization contract
// directly on ParseEvent (stabilize-workspace-context 2.2): outer whitespace
// is trimmed from a string before the absolute-existing-directory check; any
// unusable value falls back to the supplied process cwd; internal path bytes
// are preserved. Path and shell events share the same normalization.
func TestParseEventCwdNormalization(t *testing.T) {
	processCwd := filepath.Join(t.TempDir(), "process-cwd")
	if err := os.MkdirAll(processCwd, 0o755); err != nil {
		t.Fatal(err)
	}
	valid := t.TempDir()
	// Directory with an internal space: proves trimming never touches inner bytes.
	internalSpace := filepath.Join(t.TempDir(), "a b")
	if err := os.MkdirAll(internalSpace, 0o755); err != nil {
		t.Fatal(err)
	}
	plainFile := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(plainFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")

	pathEvent := func(cwd interface{}, filePath string) map[string]interface{} {
		m := map[string]interface{}{
			"event":     "pre-tool-use",
			"tool_name": "Write",
			"tool_input": map[string]interface{}{
				"file_path": filePath,
			},
		}
		if cwd != nil {
			m["cwd"] = cwd
		}
		return m
	}
	shellEvent := func(cwd interface{}, command string) map[string]interface{} {
		m := map[string]interface{}{
			"event":     "pre-tool-use",
			"tool_name": "Bash",
			"tool_input": map[string]interface{}{
				"command": command,
			},
		}
		if cwd != nil {
			m["cwd"] = cwd
		}
		return m
	}

	cases := []struct {
		name string
		raw  map[string]interface{}
		want string
		mode string
	}{
		{"path valid trimmed", pathEvent("  "+valid+"  ", "a.py"), valid, "path"},
		{"path valid trimmed internal space", pathEvent("\t"+internalSpace+"\n", "a.py"), internalSpace, "path"},
		{"path missing cwd", pathEvent(nil, "a.py"), processCwd, "path"},
		{"path whitespace only", pathEvent("   ", "a.py"), processCwd, "path"},
		{"path relative", pathEvent("relative/dir", "a.py"), processCwd, "path"},
		{"path nonexistent", pathEvent(nonexistent, "a.py"), processCwd, "path"},
		{"path non-directory", pathEvent(plainFile, "a.py"), processCwd, "path"},
		{"path non-string cwd", pathEvent(123, "a.py"), processCwd, "path"},
		{"shell valid trimmed", shellEvent("  "+valid+"  ", "echo x > out"), valid, "shell"},
		{"shell missing cwd", shellEvent(nil, "echo x > out"), processCwd, "shell"},
		{"shell whitespace only", shellEvent("   ", "echo x > out"), processCwd, "shell"},
		{"shell relative", shellEvent("relative/dir", "echo x > out"), processCwd, "shell"},
		{"shell nonexistent", shellEvent(nonexistent, "echo x > out"), processCwd, "shell"},
		{"shell non-string cwd", shellEvent(true, "echo x > out"), processCwd, "shell"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			ev := ParseEvent(strings.NewReader(string(data)), processCwd)
			if ev.Cwd != tc.want {
				t.Fatalf("Cwd = %q, want %q", ev.Cwd, tc.want)
			}
			if ev.Mode != tc.mode {
				t.Fatalf("Mode = %q, want %q", ev.Mode, tc.mode)
			}
		})
	}
}
