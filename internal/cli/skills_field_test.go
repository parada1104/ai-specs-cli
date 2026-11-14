package cli

// `ai-specs skills field <path>` (GO-20 U2e, T5 PR1): reads JSON on stdin and
// prints the value at a dotted path, replacing the three python3 JSON helpers in
// bundled-skills/skill-sync/assets/sync.sh. The shared runJSONField rules are
// pinned here through the public Run.

import (
	"bytes"
	"strings"
	"testing"
)

func runSkillsField(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"skills", "field"}, args...), t.TempDir(), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestSkillsFieldRoutes(t *testing.T) {
	cases := []struct {
		args     []string
		wantKind routeKind
	}{
		{[]string{"skills", "field"}, routeSkillsField},
		{[]string{"skills", "field", "name"}, routeSkillsField},
		{[]string{"skills", "field", "--help"}, routeSkillsField},
	}
	for _, tc := range cases {
		if got := Route(tc.args).kind; got != tc.wantKind {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, got, tc.wantKind)
		}
	}
}

func TestSkillsFieldValues(t *testing.T) {
	body := `{"enabled":true,"name":"demo","scope":["root"],"auto_invoke":[],"warnings":[]}`
	cases := []struct {
		name, path, want string
	}{
		{"string", "name", "demo\n"},
		{"list joins with a comma", "scope", "root\n"},
		{"empty list is an empty line", "auto_invoke", "\n"},
		{"missing key is an empty line", "absent", "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runSkillsField(t, body, tc.path)
			if code != 0 || stdout != tc.want || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q; want 0, %q, empty", code, stdout, stderr, tc.want)
			}
		})
	}
}

func TestSkillsFieldNonObjectStdin(t *testing.T) {
	code, stdout, stderr := runSkillsField(t, `[1,2]`, "name")
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr != "ERROR: stdin JSON is not an object\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestSkillsFieldUsageErrors(t *testing.T) {
	code, stdout, stderr := runSkillsField(t, `{}`)
	if code != 2 {
		t.Fatalf("missing path: code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("missing path: stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "ERROR: missing required argument: <path>") {
		t.Errorf("missing path: stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Run 'ai-specs skills field --help' for usage.") {
		t.Errorf("missing path: stderr = %q", stderr)
	}
}

func TestSkillsFieldHelp(t *testing.T) {
	code, stdout, stderr := runSkillsField(t, ``, "--help")
	if code != 0 {
		t.Fatalf("rc = %d, want 0", code)
	}
	if stdout != skillsFieldUsage {
		t.Errorf("stdout = %q, want usage", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}
