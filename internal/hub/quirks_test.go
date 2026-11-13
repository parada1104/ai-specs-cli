package hub

import (
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// TestHeadlineForCountsPinsTheFrozenWording pins the one place where the port
// deliberately reproduces a QUIRK: errors say "error(s)" literally, warnings
// pluralize properly, and errors win over warnings. A later refactor that
// "cleans up" either spelling changes user-visible bytes of the default verb.
func TestHeadlineForCountsPinsTheFrozenWording(t *testing.T) {
	cases := []struct {
		ok, info, warn, errCount int
		want                     string
	}{
		{0, 0, 0, 0, "healthy"},
		{12, 3, 0, 0, "healthy"},
		{5, 0, 1, 0, "1 warning"},
		{5, 0, 2, 0, "2 warnings"},
		{0, 0, 0, 1, "1 error(s)"},
		{0, 0, 0, 3, "3 error(s)"},
		{0, 0, 2, 1, "1 error(s)"},
		{0, 0, 9, 4, "4 error(s)"},
	}
	for _, tc := range cases {
		if got := headlineForCounts(severityCounts{OK: tc.ok, Info: tc.info, Warn: tc.warn, Error: tc.errCount}); got != tc.want {
			t.Errorf("headlineForCounts(%d, %d, %d, %d) = %q, want %q",
				tc.ok, tc.info, tc.warn, tc.errCount, got, tc.want)
		}
	}
}

// TestReadVersionFallsBackToUnknown pins the third frozen string: a missing
// VERSION file is "unknown", and a trailing newline in a real one is trimmed.
func TestReadVersionFallsBackToUnknown(t *testing.T) {
	if got := readVersion(t.TempDir()); got != "unknown" {
		t.Errorf("readVersion(no VERSION) = %q, want %q", got, "unknown")
	}

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("0.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readVersion(home); got != "0.24.0" {
		t.Errorf("readVersion(VERSION) = %q, want %q", got, "0.24.0")
	}
}

// TestWorktreeFlowEnabledKeepsPythonIsTrueSemantics pins the predicate behind the
// CONDITIONAL topology line. hub.py asks `wf.get("enabled") is True`, so a string
// "true" does not count; a port that used a truthiness helper here would print an
// extra line on manifests the shell version leaves alone.
func TestWorktreeFlowEnabledKeepsPythonIsTrueSemantics(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want bool
	}{
		{"no recipes section", "[project]\nname = \"x\"\n", false},
		{"recipes without worktree-flow", "[recipes.other]\nenabled = true\n", false},
		{"enabled true (boolean)", "[recipes.worktree-flow]\nenabled = true\n", true},
		{"enabled false (boolean)", "[recipes.worktree-flow]\nenabled = false\n", false},
		{"enabled \"true\" (string)", "[recipes.worktree-flow]\nenabled = \"true\"\n", false},
		{"enabled 1 (integer)", "[recipes.worktree-flow]\nenabled = 1\n", false},
		{"enabled missing", "[recipes.worktree-flow]\nversion = \"1\"\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := toml.Parse([]byte(tc.toml))
			if err != nil {
				t.Fatalf("fixture does not parse: %v", err)
			}
			if got := worktreeFlowEnabled(manifest); got != tc.want {
				t.Errorf("worktreeFlowEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFormatPanelTopology pins the panel's conditional topology label: an
// unconfigured via is treated as "auto" (hub.py's `topology_via or "auto"`),
// "auto" renders the auto→ form, and anything else renders `(via <x>)`.
func TestFormatPanelTopology(t *testing.T) {
	cases := []struct {
		resolved, via, want string
	}{
		{"standalone", "", "standalone (auto→standalone)"},
		{"standalone", "auto", "standalone (auto→standalone)"},
		{"monorepo-submodules", "config", "monorepo-submodules (via config)"},
	}
	for _, tc := range cases {
		if got := formatPanelTopology(tc.resolved, tc.via); got != tc.want {
			t.Errorf("formatPanelTopology(%q, %q) = %q, want %q", tc.resolved, tc.via, got, tc.want)
		}
	}
}

// TestHubTopologyHidesTheDefaultResolution pins the other half of the conditional
// line: a plain project says nothing about topology, so the summary has no
// `topology:` line at all.
func TestHubTopologyHidesTheDefaultResolution(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "[project]\nname = \"fixture\"\n"
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, via := hubTopology(root); resolved != "" || via != "" {
		t.Errorf("hubTopology(default manifest) = (%q, %q), want empty so the line is omitted", resolved, via)
	}
}
