package cli

// `ai-specs tracker classify` (GO-20 U2e, unit U-F2b): reads one hook event JSON
// on stdin and prints the path/shell protocol tracker-card-gate.sh consumes. The
// expected bytes below were produced by the python reference on the same event.

import (
	"bytes"
	"strings"
	"testing"
)

func runTrackerClassifyCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"tracker", "classify"}, args...), t.TempDir(), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestTrackerClassifyPathEvent(t *testing.T) {
	event := `{"tool_name":"Write","cwd":"/repo","tool_input":{"file_path":"  a/b.md \n"}}`
	code, stdout, stderr := runTrackerClassifyCLI(t, event)
	if code != 0 || stderr != "" || stdout != "path\tWrite\t/repo\na/b.md\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestTrackerClassifyShellEvent(t *testing.T) {
	event := `{"cwd":"/repo","tool_input":{"command":"cd x && sudo gh pr create --draft"}}`
	code, stdout, stderr := runTrackerClassifyCLI(t, event)
	if code != 0 || stderr != "" || stdout != "shell\tBash\t/repo\npr_create\t\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestTrackerClassifySilentWhenNothingToReport(t *testing.T) {
	for _, event := range []string{`{"tool_input":{}}`, `{"tool_input":{"command":"gh pr merge"}}`, `{not json`, ``, `[1]`} {
		code, stdout, stderr := runTrackerClassifyCLI(t, event)
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("event %q: code=%d stdout=%q stderr=%q; want 0 and no output", event, code, stdout, stderr)
		}
	}
}

func TestTrackerClassifyUnclassifiableEventExitsOne(t *testing.T) {
	event := `{"tool_input":{"file_path":"\ud800bad.md"}}`
	code, stdout, stderr := runTrackerClassifyCLI(t, event)
	if code != 1 || stdout != "" || stderr == "" {
		t.Fatalf("code=%d stdout=%q stderr=%q; want 1, empty stdout, one stderr line", code, stdout, stderr)
	}
	if strings.Count(stderr, "\n") != 1 {
		t.Fatalf("stderr = %q; want exactly one line", stderr)
	}
}

func TestTrackerClassifyUsage(t *testing.T) {
	code, stdout, stderr := runTrackerClassifyCLI(t, "", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs tracker classify") {
		t.Fatalf("--help: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = runTrackerClassifyCLI(t, `{}`, "-h")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs tracker classify") {
		t.Fatalf("-h: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, args := range [][]string{{"--bogus"}, {"positional"}} {
		code, stdout, stderr = runTrackerClassifyCLI(t, `{}`, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "ERROR:") ||
			!strings.Contains(stderr, "Run 'ai-specs tracker classify --help' for usage.") {
			t.Fatalf("%q: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}
