package config

// GO-20 U3b-2 (T4c): the Agents submenu's two manifest accessors. The read side
// mirrors toml-read.read_agents(...).get("enabled", []) and the write side is a
// byte-faithful port of hub.py's _run_agents_submenu editor, including D12: it
// never appends a second [agents] block.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifestAt(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readBack(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEnabledAgentsReadsListInManifestOrder(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"cursor\", \"pi\"]\n")
	got, err := EnabledAgents(path)
	if err != nil {
		t.Fatalf("EnabledAgents: %v", err)
	}
	if strings.Join(got, ",") != "cursor,pi" {
		t.Errorf("EnabledAgents = %v, want [cursor pi]", got)
	}
}

func TestEnabledAgentsMissingManifestIsEmpty(t *testing.T) {
	got, err := EnabledAgents(filepath.Join(t.TempDir(), "nope", "ai-specs.toml"))
	if err != nil {
		t.Fatalf("EnabledAgents over a missing manifest: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("EnabledAgents = %v, want empty", got)
	}
}

func TestEnabledAgentsMissingTableOrKeyIsEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"no agents table": "[project]\nname = \"t\"\n",
		"empty agents":    "[agents]\n",
		"non-list value":  "[agents]\nenabled = \"claude\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := EnabledAgents(manifestAt(t, body))
			if err != nil {
				t.Fatalf("EnabledAgents: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("EnabledAgents = %v, want empty", got)
			}
		})
	}
}

// TestSetEnabledAgentsAppendsOneAgentsTable pins the no-[agents]-yet branch:
// the table is appended once, after a forced trailing newline.
func TestSetEnabledAgentsAppendsOneAgentsTable(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n")
	if err := SetEnabledAgents(path, []string{"claude", "pi"}); err != nil {
		t.Fatal(err)
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"claude\", \"pi\"]\n"
	if got := readBack(t, path); got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
}

// TestSetEnabledAgentsInsertsInsideExistingTable pins the D12 branch: an
// [agents] table without `enabled` gets the key inserted inside it, before the
// next header — never a second [agents] block.
func TestSetEnabledAgentsInsertsInsideExistingTable(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\n\n[other]\nx = 2\n")
	if err := SetEnabledAgents(path, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\n\nenabled = [\"claude\"]\n[other]\nx = 2\n"
	if got := readBack(t, path); got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
	if n := strings.Count(readBack(t, path), "[agents]"); n != 1 {
		t.Errorf("[agents] appears %d times, want 1", n)
	}
}

// TestSetEnabledAgentsReplacesExistingEnabledLine pins the replace branch: the
// line is rewritten in place, not duplicated.
func TestSetEnabledAgentsReplacesExistingEnabledLine(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"cursor\"]\n")
	if err := SetEnabledAgents(path, []string{"pi", "omp"}); err != nil {
		t.Fatal(err)
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"pi\", \"omp\"]\n"
	if got := readBack(t, path); got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
	if n := strings.Count(readBack(t, path), "enabled ="); n != 1 {
		t.Errorf("enabled key appears %d times, want 1", n)
	}
}

// TestSetEnabledAgentsKeepsAgentsAsLastTable pins the "in_agents at EOF" branch.
func TestSetEnabledAgentsKeepsAgentsAsLastTable(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\n")
	if err := SetEnabledAgents(path, []string{"omp"}); err != nil {
		t.Fatal(err)
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\nenabled = [\"omp\"]\n"
	if got := readBack(t, path); got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
}

// TestSetEnabledAgentsEmptyListWritesEmptyLiteral pins toml_value([]) = "[]".
func TestSetEnabledAgentsEmptyListWritesEmptyLiteral(t *testing.T) {
	path := manifestAt(t, "[agents]\nenabled = [\"claude\"]\n")
	if err := SetEnabledAgents(path, []string{}); err != nil {
		t.Fatal(err)
	}
	if got := readBack(t, path); got != "[agents]\nenabled = []\n" {
		t.Errorf("manifest = %q", got)
	}
}

// TestSetEnabledAgentsRoundTrips proves the write is readable by the read side.
func TestSetEnabledAgentsRoundTrips(t *testing.T) {
	path := manifestAt(t, "[project]\nname = \"t\"\n")
	if err := SetEnabledAgents(path, []string{"cursor", "pi"}); err != nil {
		t.Fatal(err)
	}
	got, err := EnabledAgents(path)
	if err != nil {
		t.Fatalf("EnabledAgents: %v", err)
	}
	if strings.Join(got, ",") != "cursor,pi" {
		t.Errorf("round-trip = %v, want [cursor pi]", got)
	}
}
