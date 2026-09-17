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

func parseEventJSON(t *testing.T, raw map[string]interface{}, processCwd string) Event {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ParseEvent(strings.NewReader(string(data)), processCwd)
}

func TestParseEventWriteCandidateSources(t *testing.T) {
	process := filepath.Join(t.TempDir(), "proc")
	if err := os.MkdirAll(process, 0o755); err != nil {
		t.Fatal(err)
	}
	eventDir := t.TempDir()
	wt := t.TempDir()

	t.Run("shell git -C overlay is command source", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name": "Bash",
			"cwd":       eventDir,
			"tool_input": map[string]interface{}{
				"command": "git -C " + wt + " mv rel-a rel-b",
			},
		}
		ev := parseEventJSON(t, raw, process)
		if !ev.CwdTrusted {
			t.Fatal("trusted event cwd")
		}
		if len(ev.Candidates) == 0 {
			t.Fatal("expected mv destination candidate")
		}
		var dest WriteCandidate
		for _, c := range ev.Candidates {
			if c.Path == "rel-b" {
				dest = c
			}
		}
		if dest.Path != "rel-b" {
			t.Fatalf("candidates = %+v, want rel-b", ev.Candidates)
		}
		if dest.Source != cwdSourceCommand || dest.Base != wt {
			t.Fatalf("rel-b = %+v, want source=command base=%s", dest, wt)
		}
	})

	t.Run("shell no changer uses event source when trusted", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name": "Bash",
			"cwd":       eventDir,
			"tool_input": map[string]interface{}{
				"command": "echo x > rel",
			},
		}
		ev := parseEventJSON(t, raw, process)
		if len(ev.Candidates) != 1 || ev.Candidates[0].Path != "rel" {
			t.Fatalf("candidates = %+v", ev.Candidates)
		}
		c := ev.Candidates[0]
		if c.Source != cwdSourceEvent || c.Base != eventDir {
			t.Fatalf("rel = %+v, want source=event", c)
		}
	})

	t.Run("shell unrecoverable cd is none even if event trusted", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name": "Bash",
			"cwd":       eventDir,
			"tool_input": map[string]interface{}{
				"command": `cd - && echo x > rel`,
			},
		}
		ev := parseEventJSON(t, raw, process)
		if !ev.CwdTrusted {
			t.Fatal("json cwd still trusted")
		}
		var rel WriteCandidate
		for _, c := range ev.Candidates {
			if c.Path == "rel" {
				rel = c
			}
		}
		if rel.Path != "rel" || rel.Source != cwdSourceNone {
			t.Fatalf("rel = %+v, want source=none", rel)
		}
	})

	t.Run("path trusted event source", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name":  "Write",
			"cwd":        eventDir,
			"tool_input": map[string]interface{}{"file_path": "a.py"},
		}
		ev := parseEventJSON(t, raw, process)
		if ev.Mode != "path" || !ev.CwdTrusted {
			t.Fatalf("mode/trusted = %s %v", ev.Mode, ev.CwdTrusted)
		}
		if len(ev.Candidates) != 1 {
			t.Fatalf("candidates = %+v", ev.Candidates)
		}
		c := ev.Candidates[0]
		if c.Path != "a.py" || c.Source != cwdSourceEvent || c.Base != eventDir {
			t.Fatalf("path cand = %+v", c)
		}
	})

	t.Run("path missing cwd is none; Cwd still records fallback", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name":  "Write",
			"tool_input": map[string]interface{}{"file_path": "a.py"},
		}
		ev := parseEventJSON(t, raw, process)
		if ev.CwdTrusted {
			t.Fatal("missing json cwd must not be trusted")
		}
		if ev.Cwd != process {
			t.Fatalf("Cwd = %q, want process fallback %q", ev.Cwd, process)
		}
		if ev.Candidates[0].Source != cwdSourceNone {
			t.Fatalf("path source = %+v, want none", ev.Candidates[0])
		}
	})

	t.Run("path absolute still classified with none source", func(t *testing.T) {
		abs := filepath.Join(eventDir, "abs.py")
		raw := map[string]interface{}{
			"tool_name":  "Write",
			"tool_input": map[string]interface{}{"file_path": abs},
		}
		ev := parseEventJSON(t, raw, process)
		if ev.Candidates[0].Path != abs {
			t.Fatalf("path = %+v", ev.Candidates[0])
		}
		if ev.Candidates[0].Source != cwdSourceNone {
			t.Fatalf("abs path source = %s, want none (untrusted event)", ev.Candidates[0].Source)
		}
	})

	t.Run("missing json cwd with git -C is still command source", func(t *testing.T) {
		raw := map[string]interface{}{
			"tool_name": "Bash",
			"tool_input": map[string]interface{}{
				"command": "git -C " + wt + " mv a b",
			},
		}
		ev := parseEventJSON(t, raw, process)
		if ev.CwdTrusted {
			t.Fatal("missing json cwd is untrusted")
		}
		if ev.Cwd != process {
			t.Fatalf("Cwd fallback = %q, want %q", ev.Cwd, process)
		}
		var b WriteCandidate
		for _, c := range ev.Candidates {
			if c.Path == "b" {
				b = c
			}
		}
		if b.Source != cwdSourceCommand || b.Base != wt {
			t.Fatalf("git -C dest = %+v, want command/%s", b, wt)
		}
	})
}

func TestParseEventExtractPass2OnceWithFinalS(t *testing.T) {
	process := t.TempDir()
	eventDir := t.TempDir()
	dirA := t.TempDir()
	raw := map[string]interface{}{
		"tool_name": "Bash",
		"cwd":       eventDir,
		"tool_input": map[string]interface{}{
			"command": "cd " + dirA + ` && python3 -c 'Path("rel").write_text("x")'`,
		},
	}
	ev := parseEventJSON(t, raw, process)
	var rel WriteCandidate
	n := 0
	for _, c := range ev.Candidates {
		if c.Path == "rel" {
			rel = c
			n++
		}
	}
	if n != 1 {
		t.Fatalf("extractPass2 must run once; rel count=%d candidates=%+v", n, ev.Candidates)
	}
	if rel.Source != cwdSourceCommand || rel.Base != dirA {
		t.Fatalf("pass2 rel = %+v, want command base=final S %s (not event cwd %s)", rel, dirA, eventDir)
	}
}
