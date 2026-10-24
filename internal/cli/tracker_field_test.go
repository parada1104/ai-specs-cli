package cli

// `ai-specs tracker field <path>` (GO-20 U2e, unit U-F): reads JSON on stdin and
// prints the value at a dotted path, replacing the python3 JSON helpers in
// tracker-card-gate.sh. Output format is pinned here through the public Run.

import (
	"bytes"
	"strings"
	"testing"
)

func runTrackerField(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"tracker", "field"}, args...), t.TempDir(), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestTrackerFieldScalars(t *testing.T) {
	cases := []struct {
		name, json, path, want string
	}{
		{"string", `{"decision":"block"}`, "decision", "block\n"},
		{"true is 1", `{"active":true}`, "active", "1\n"},
		{"false is 0", `{"active":false}`, "active", "0\n"},
		{"null is empty", `{"reason":null}`, "reason", "\n"},
		{"missing is empty", `{"other":1}`, "reason", "\n"},
		{"integer", `{"n":42}`, "n", "42\n"},
		{"float keeps its text", `{"n":1.5}`, "n", "1.5\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runTrackerField(t, tc.json, tc.path)
			if code != 0 || stdout != tc.want || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q; want 0, %q, empty", code, stdout, stderr, tc.want)
			}
		})
	}
}

func TestTrackerFieldDottedPathAndList(t *testing.T) {
	body := `{"evidence":{"local":"a","remote":"","git":null},"choices":["continue","stop","ask"]}`

	code, stdout, _ := runTrackerField(t, body, "evidence.local")
	if code != 0 || stdout != "a\n" {
		t.Fatalf("evidence.local: code=%d stdout=%q", code, stdout)
	}
	code, stdout, _ = runTrackerField(t, body, "evidence.git")
	if code != 0 || stdout != "\n" {
		t.Fatalf("evidence.git (null): code=%d stdout=%q", code, stdout)
	}
	code, stdout, _ = runTrackerField(t, body, "evidence.absent.deeper")
	if code != 0 || stdout != "\n" {
		t.Fatalf("missing intermediate: code=%d stdout=%q", code, stdout)
	}
	code, stdout, _ = runTrackerField(t, body, "choices")
	if code != 0 || stdout != "continue, stop, ask\n" {
		t.Fatalf("list join: code=%d stdout=%q", code, stdout)
	}
	code, stdout, _ = runTrackerField(t, `{"choices":[]}`, "choices")
	if code != 0 || stdout != "\n" {
		t.Fatalf("empty list: code=%d stdout=%q", code, stdout)
	}
}

func TestTrackerFieldInvalidJSONFailsWithNothingPrinted(t *testing.T) {
	for _, body := range []string{"{not json", `{"a":1} trailing`, `[1,2]`, `"text"`} {
		code, stdout, stderr := runTrackerField(t, body, "decision")
		if code != 1 || stdout != "" {
			t.Fatalf("input %q: code=%d stdout=%q stderr=%q; want exit 1 and no stdout", body, code, stdout, stderr)
		}
	}
}

func TestTrackerFieldUsageErrors(t *testing.T) {
	code, stdout, stderr := runTrackerField(t, `{}`)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "missing required argument: <path>") {
		t.Fatalf("no path: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr = runTrackerField(t, `{}`, "a", "b")
	if code != 2 || !strings.Contains(stderr, "unexpected positional argument: b") {
		t.Fatalf("two paths: code=%d stderr=%q", code, stderr)
	}
	code, stdout, _ = runTrackerField(t, "", "--help")
	if code != 0 || !strings.Contains(stdout, "Usage: ai-specs tracker field <path>") {
		t.Fatalf("--help: code=%d stdout=%q", code, stdout)
	}
}
