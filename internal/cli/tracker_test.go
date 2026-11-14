package cli

// `ai-specs tracker slug|evidence` (GO-20 U2e, unit U-C): the native verbs that
// replace the ledger_bridge.py calls in tracker-card-gate.sh. The host is
// fail-open, so stdout/stderr and exit codes are pinned here.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTrackerCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, t.TempDir(), nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func trackerWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trackerRoot(t *testing.T, slug string) string {
	t.Helper()
	root := t.TempDir()
	if slug != "" {
		if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTrackerFamilyRoutes(t *testing.T) {
	cases := []struct {
		args     []string
		wantKind routeKind
	}{
		{[]string{"tracker", "slug"}, routeTrackerSlug},
		{[]string{"tracker", "slug", "/tmp"}, routeTrackerSlug},
		{[]string{"tracker", "evidence"}, routeTrackerEvidence},
		{[]string{"tracker", "field"}, routeTrackerField},
		{[]string{"tracker", "field", "a.b"}, routeTrackerField},
		{[]string{"tracker", "classify"}, routeTrackerClassify},
		{[]string{"tracker", "classify", "x"}, routeTrackerClassify},
		{[]string{"tracker"}, routeTracker},
		{[]string{"tracker", "--help"}, routeTracker},
		{[]string{"tracker", "help"}, routeTracker},
		{[]string{"tracker", "slugged"}, routeTracker},
		{[]string{"slug", "/tmp"}, routeUnknown},
	}
	for _, tc := range cases {
		if got := Route(tc.args).kind; got != tc.wantKind {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, got, tc.wantKind)
		}
	}
}

func TestTrackerFamilyHelpUsageAndUnknownSubcommand(t *testing.T) {
	code, stdout, stderr := runTrackerCLI(t, "tracker", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs tracker <subcommand>") {
		t.Fatalf("tracker --help: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	code, stdout, stderr = runTrackerCLI(t, "tracker")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "Usage: ai-specs tracker <subcommand>") {
		t.Fatalf("bare tracker: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	code, stdout, stderr = runTrackerCLI(t, "tracker", "bogus")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown subcommand 'bogus'") {
		t.Fatalf("tracker bogus: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestRootHelpAnnouncesTheTrackerFamily(t *testing.T) {
	code, stdout, stderr := runTrackerCLI(t, "help")
	if code != 0 || stderr != "" {
		t.Fatalf("help: exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "tracker slug|evidence") {
		t.Errorf("root help %q does not announce the tracker family", stdout)
	}
}

func TestTrackerSlugPrintsTheActiveChange(t *testing.T) {
	root := trackerRoot(t, "demo-change")
	code, stdout, stderr := runTrackerCLI(t, "tracker", "slug", root)
	if code != 0 || stderr != "" || stdout != "demo-change\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestTrackerSlugPrintsAnEmptyLineWhenThereIsNone(t *testing.T) {
	root := trackerRoot(t, "")
	code, stdout, stderr := runTrackerCLI(t, "tracker", "slug", root)
	if code != 0 || stderr != "" || stdout != "\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestTrackerEvidenceWritesThePayload(t *testing.T) {
	root := trackerRoot(t, "demo-change")
	trackerWrite(t, filepath.Join(root, "openspec", "changes", "demo-change", "proposal.md"),
		"## Tracker\n- **card_id**: `6aa703fdcf61a90ec702d58b`\n- pr: `https://example/pr/1`\n")
	out := filepath.Join(t.TempDir(), "evidence.json")

	code, stdout, stderr := runTrackerCLI(t, "tracker", "evidence", root, "demo-change", out)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "6aa703fdcf61a90ec702d58b", "git": "6aa703fdcf61a90ec702d58b"}`
	if string(raw) != want {
		t.Fatalf("evidence = %s, want %s", raw, want)
	}
}

func TestTrackerEvidenceEmptySlugWritesTheEmptyPayload(t *testing.T) {
	root := trackerRoot(t, "")
	out := filepath.Join(t.TempDir(), "evidence.json")
	code, _, stderr := runTrackerCLI(t, "tracker", "evidence", root, "", out)
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"local": "", "remote": "", "code": "", "git": ""}` {
		t.Fatalf("evidence = %s", raw)
	}
}

func TestTrackerEvidenceErrorWritesNothing(t *testing.T) {
	root := trackerRoot(t, "demo-change")
	trackerWrite(t, filepath.Join(root, "openspec", "changes", "demo-change", "tasks.md"),
		"## Tracker\ncard_id: \xff\n")
	out := filepath.Join(t.TempDir(), "evidence.json")

	code, _, stderr := runTrackerCLI(t, "tracker", "evidence", root, "demo-change", out)
	if code != 1 || !strings.Contains(stderr, "ERROR:") {
		t.Fatalf("exit = %d, stderr = %q; want 1 and an ERROR line", code, stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("evidence file must not be written on error: %v", err)
	}
}

func TestTrackerVerbsRejectMissingArguments(t *testing.T) {
	cases := [][]string{
		{"tracker", "slug"},
		{"tracker", "evidence", "/tmp"},
		{"tracker", "evidence", "/tmp", "slug"},
	}
	for _, args := range cases {
		code, stdout, stderr := runTrackerCLI(t, args...)
		if code != 2 || stdout != "" {
			t.Errorf("%q: exit = %d, stdout = %q; want 2/empty", args, code, stdout)
		}
		if !strings.Contains(stderr, "ERROR:") || !strings.Contains(stderr, "--help") {
			t.Errorf("%q: stderr = %q; want an ERROR line and a --help pointer", args, stderr)
		}
	}
}

func TestTrackerVerbHelp(t *testing.T) {
	for _, args := range [][]string{
		{"tracker", "slug", "--help"},
		{"tracker", "evidence", "--help"},
	} {
		code, stdout, stderr := runTrackerCLI(t, args...)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs tracker") {
			t.Errorf("%q: exit = %d, stdout = %q, stderr = %q", args, code, stdout, stderr)
		}
	}
}
