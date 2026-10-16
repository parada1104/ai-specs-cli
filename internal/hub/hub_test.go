package hub

// GO-20 U3a: the hub's non-interactive contract in Go. The interactive branch is
// bridged to hub.py until U3b, so these tests pin the mode decision, the output
// bytes, the exit code, the delegation argv and the flag/guard contract.

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDecideModeFourStateMatrix(t *testing.T) {
	cases := []struct {
		name        string
		initialized bool
		tty         bool
		want        Mode
	}{
		{"initialized+tty is the interactive hub", true, true, ModeInteractiveHub},
		{"initialized+no-tty is the status summary", true, false, ModeNoninteractiveStatus},
		{"uninitialized+tty offers init", false, true, ModeOfferInit},
		{"uninitialized+no-tty is the error", false, false, ModeErrorUninitialized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideMode(tc.initialized, tc.tty); got != tc.want {
				t.Errorf("DecideMode(%v, %v) = %v, want %v", tc.initialized, tc.tty, got, tc.want)
			}
		})
	}
}

// noninteractiveFixture is an initialized project whose manifest enables an agent.
func noninteractiveFixture(t *testing.T) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"t\"\n\n[agents]\nenabled = [\"claude\"]\n")
	return root, home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNoninteractiveOutputShape pins the frozen line formats without pinning the
// doctor's counts (which depend on the environment): the summary line must agree
// with the doctor's own severity counts, and the exit code must be the doctor's.
func TestNoninteractiveOutputShape(t *testing.T) {
	root, home := noninteractiveFixture(t)

	var out, errb bytes.Buffer
	rc := RunNoninteractive(root, home, &out, &errb)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("too few lines:\n%s", out.String())
	}
	if !regexp.MustCompile(`^ai-specs status — .+$`).MatchString(lines[0]) {
		t.Errorf("headline line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  version: ") {
		t.Errorf("version line = %q", lines[1])
	}
	if lines[2] != "  target: "+root {
		t.Errorf("target line = %q", lines[2])
	}
	summary := ""
	commandsAt := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "  Summary: ") {
			summary = line
		}
		if line == "Commands:" {
			commandsAt = i
		}
	}
	if summary == "" || commandsAt == -1 {
		t.Fatalf("missing Summary/Commands:\n%s", out.String())
	}
	if !regexp.MustCompile(`^  Summary: \d+ OK, \d+ INFO, \d+ WARN, \d+ ERROR$`).MatchString(summary) {
		t.Errorf("summary line = %q", summary)
	}
	// The blank line before "Commands:" and exactly one line per menu entry.
	if lines[commandsAt-1] != "" {
		t.Errorf("expected a blank line before Commands:, got %q", lines[commandsAt-1])
	}
	if got := len(lines) - commandsAt - 1; got != len(Menu()) {
		t.Errorf("menu lines = %d, want %d", got, len(Menu()))
	}
	if rc != DoctorExitCode(root, home) {
		t.Errorf("rc = %d, want the doctor's own exit code %d", rc, DoctorExitCode(root, home))
	}
}

// TestNoninteractiveUninitializedGuard pins the anti-hang contract: an
// uninitialized target without a TTY reports the two frozen stderr lines and
// exits 2, with no stdout at all.
func TestNoninteractiveUninitializedGuard(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	var out, errb bytes.Buffer
	rc := RunHub(Options{Target: root, Home: home, TTY: false}, nil, &out, &errb)

	if rc != 2 {
		t.Fatalf("rc = %d, want 2", rc)
	}
	want := "ERROR: no ai-specs project at " + root + "\nRun 'ai-specs init' to create one.\n"
	if errb.String() != want {
		t.Errorf("stderr = %q\nwant   = %q", errb.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty: %q", out.String())
	}
}

func TestDelegateArgvShape(t *testing.T) {
	const cli = "/opt/ai-specs"
	cases := []struct {
		name   string
		action Action
		extra  []string
		target string
		want   []string
	}{
		{"no extra appends the target", ActionDoctor, nil, "/p", []string{cli, "doctor", "/p"}},
		{"extra goes before the target", ActionRecipe, []string{"add", "demo"}, "/p", []string{cli, "recipe", "add", "demo", "/p"}},
		{"version carries no target", ActionVersion, nil, "", []string{cli, "version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := delegateArgv(cli, tc.action, tc.extra, tc.target)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Errorf("argv = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFlagContract(t *testing.T) {
	t.Run("help prints the usage and exits 0", func(t *testing.T) {
		var out, errb bytes.Buffer
		if rc := RunHub(Options{Args: []string{"--help"}, Home: t.TempDir()}, nil, &out, &errb); rc != 0 {
			t.Fatalf("rc = %d", rc)
		}
		if !strings.Contains(out.String(), "Usage: ai-specs hub [path] [--help]") {
			t.Errorf("usage = %q", out.String())
		}
	})
	t.Run("an unknown flag exits 2", func(t *testing.T) {
		var out, errb bytes.Buffer
		if rc := RunHub(Options{Args: []string{"--bogus"}, Home: t.TempDir()}, nil, &out, &errb); rc != 2 {
			t.Fatalf("rc = %d, want 2", rc)
		}
		if !strings.Contains(errb.String(), "unknown flag: --bogus") {
			t.Errorf("stderr = %q", errb.String())
		}
	})
	t.Run("a second positional exits 2", func(t *testing.T) {
		var out, errb bytes.Buffer
		if rc := RunHub(Options{Args: []string{"a", "b"}, Home: t.TempDir()}, nil, &out, &errb); rc != 2 {
			t.Fatalf("rc = %d, want 2", rc)
		}
		if !strings.Contains(errb.String(), "unexpected positional argument: b") {
			t.Errorf("stderr = %q", errb.String())
		}
	})
}

func TestMenuMatchesTheShellEntries(t *testing.T) {
	// The 12 entries of hub.py's _MENU, in order: a Go port that loses one is a
	// silent product regression, so the table is pinned by name.
	want := []string{
		"sync", "doctor", "agents", "skills", "recipe", "configure-recipes",
		"rules-audit", "upgrade", "version", "help", "init", "quit",
	}
	menu := Menu()
	if len(menu) != len(want) {
		t.Fatalf("menu has %d entries, want %d", len(menu), len(want))
	}
	for i, entry := range menu {
		if string(entry.Action) != want[i] {
			t.Errorf("menu[%d] = %q, want %q", i, entry.Action, want[i])
		}
		if entry.Title == "" || entry.Description == "" {
			t.Errorf("menu[%d] (%q) has an empty title or description", i, entry.Action)
		}
	}
}
