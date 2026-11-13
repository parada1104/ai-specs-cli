package config

// GO-20 U3b-2 advisory remediation (odd/tasks/go-20-u3b-advisories.md). Each test
// here fails against the reviewed candidate and pins one advisory fix.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSetEnabledAgentsIgnoresStructuralLinesInsideMultilineStrings pins
// R1-multiline-string-header-swallow (S4). A line that only looks structural
// because it sits inside a multi-line string is content, never a header and never
// the `enabled` key: writing there corrupted the string AND silently dropped the
// caller's selection while reporting success.
func TestSetEnabledAgentsIgnoresStructuralLinesInsideMultilineStrings(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"basic": {
			body: "[project]\nname = \"t\"\ndesc = \"\"\"\n[agents]\nnot a table\n\"\"\"\n\n[agents]\nenabled = [\"pi\"]\n",
			want: "[project]\nname = \"t\"\ndesc = \"\"\"\n[agents]\nnot a table\n\"\"\"\n\n[agents]\nenabled = [\"cursor\"]\n",
		},
		"literal": {
			body: "[project]\nname = \"t\"\ndesc = '''\nenabled = [\"ghost\"]\n'''\n\n[agents]\nenabled = [\"pi\"]\n",
			want: "[project]\nname = \"t\"\ndesc = '''\nenabled = [\"ghost\"]\n'''\n\n[agents]\nenabled = [\"cursor\"]\n",
		},
		"no agents table yet": {
			body: "[project]\nname = \"t\"\ndesc = \"\"\"\n[agents]\n\"\"\"\n",
			want: "[project]\nname = \"t\"\ndesc = \"\"\"\n[agents]\n\"\"\"\n\n[agents]\nenabled = [\"cursor\"]\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := manifestAt(t, tc.body)
			if err := SetEnabledAgents(path, []string{"cursor"}); err != nil {
				t.Fatalf("SetEnabledAgents: %v", err)
			}
			if got := readBack(t, path); got != tc.want {
				t.Errorf("manifest after write:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestEnabledAgentsReportsUnreadableManifest pins S6: a manifest that exists
// but is not valid TOML is NOT "no agents enabled". The read must report the
// error so the hub's Agents picker can refuse to open instead of starting
// unchecked and persisting that emptiness over the user's real list.
func TestEnabledAgentsReportsUnreadableManifest(t *testing.T) {
	path := manifestAt(t, "[project\nname = \"")
	if _, err := EnabledAgents(path); err == nil {
		t.Fatal("EnabledAgents over an unparsable manifest returned nil error, want a read error")
	}
}

// TestTableNameRejectsArrayOfTables pins R1-array-of-tables-header (S10):
// `[[agents]]` is an array of tables, never the `[agents]` table, and must be
// recognised by name rather than by the accident of the bracket scan.
func TestTableNameRejectsArrayOfTables(t *testing.T) {
	for header, want := range map[string]string{
		"[agents]":        "agents",
		"[agents] # note": "agents",
		"[agents.foo]":    "agents.foo",
		"[[deps]]":        "",
		"[[agents]]":      "",
		"[agents]  ":      "agents",
	} {
		if got := tableName(header); got != want {
			t.Errorf("tableName(%q) = %q, want %q", header, got, want)
		}
	}
}

// TestSetEnabledAgentsWritesGroupWritableManifest pins R4-atomicwrite-readonly-refusal
// (S9): the refusal message says "a manifest nobody can write", so the test has to be
// whether anybody can write it. A manifest whose write bit sits on group or other is
// writable by the user's rename and must not be refused.
func TestSetEnabledAgentsWritesGroupWritableManifest(t *testing.T) {
	body := "[project]\nname = \"t\"\n"
	path := manifestAt(t, body)
	if err := os.Chmod(path, 0o466); err != nil {
		t.Fatal(err)
	}

	if err := SetEnabledAgents(path, []string{"pi"}); err != nil {
		t.Fatalf("SetEnabledAgents refused a group-writable manifest: %v", err)
	}
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"pi\"]\n"
	if got := readBack(t, path); got != want {
		t.Errorf("manifest after write:\n got %q\nwant %q", got, want)
	}
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o466 {
		t.Errorf("mode after write = %o, want 466 (preserved)", perm)
	}
}
