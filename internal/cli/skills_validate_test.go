package cli

// `ai-specs skills validate <skill_md_path>` (GO-20 U2e, unit U-D): the native
// replacement for `python3 lib/_internal/skill_contract.py sync-metadata <path>`
// (bundled-skills/skill-sync/assets/sync.sh:15). stdout/stderr bytes and exit
// codes are pinned against the Python oracle, except the documented native
// `ERROR:` prefix for invalid UTF-8 and the `--help` acceptance.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func runSkillsValidateCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, t.TempDir(), nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeValidateSkill(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSkillsValidateRoutes(t *testing.T) {
	cases := []struct {
		args     []string
		wantKind routeKind
	}{
		{[]string{"skills", "validate"}, routeSkillsValidate},
		{[]string{"skills", "validate", "/tmp/SKILL.md"}, routeSkillsValidate},
		{[]string{"skills", "validate", "--help"}, routeSkillsValidate},
	}
	for _, tc := range cases {
		if got := Route(tc.args).kind; got != tc.wantKind {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, got, tc.wantKind)
		}
	}
}

func TestSkillsValidateHelp(t *testing.T) {
	code, stdout, stderr := runSkillsValidateCLI(t, "skills", "validate", "--help")
	if code != 0 {
		t.Fatalf("rc = %d, want 0", code)
	}
	if stdout != skillsValidateUsage {
		t.Errorf("stdout = %q, want usage", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestSkillsValidateUsageErrors(t *testing.T) {
	cases := [][]string{
		{"skills", "validate"},
		{"skills", "validate", "a.md", "b.md"},
		{"skills", "validate", "--bogus"},
	}
	for _, args := range cases {
		code, stdout, stderr := runSkillsValidateCLI(t, args...)
		if code != 2 {
			t.Errorf("Run(%q) rc = %d, want 2", args, code)
		}
		if stdout != "" {
			t.Errorf("Run(%q) stdout = %q, want empty", args, stdout)
		}
		if stderr != skillsValidateUsage {
			t.Errorf("Run(%q) stderr = %q, want usage", args, stderr)
		}
	}
}

func TestSkillsValidateSuccess(t *testing.T) {
	path := writeValidateSkill(t, "---\nname: demo-skill\ndescription: x\nmetadata:\n  scope: [root]\n  auto_invoke: [\"When x\"]\n---\n")
	code, stdout, stderr := runSkillsValidateCLI(t, "skills", "validate", path)
	if code != 0 {
		t.Fatalf("rc = %d, want 0 (stderr %q)", code, stderr)
	}
	want := `{"enabled": true, "name": "demo-skill", "scope": ["root"], "auto_invoke": ["When x"], "warnings": ["using compatibility default ` + "`Apache-2.0`" + `; add ` + "`license`" + ` explicitly", "using compatibility default ` + "`unknown`" + ` author; add ` + "`metadata.author`" + ` explicitly", "using compatibility default ` + "`1.0`" + ` version; add ` + "`metadata.version`" + ` explicitly"]}` + "\n"
	if stdout != want {
		t.Errorf("stdout = %s, want %s", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestSkillsValidateError(t *testing.T) {
	path := writeValidateSkill(t, "---\nname: demo-skill\n---\n")
	code, stdout, stderr := runSkillsValidateCLI(t, "skills", "validate", path)
	if code != 1 {
		t.Fatalf("rc = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	want := `{"error": {"field": "description", "message": "is required", "path": "` + filepath.Clean(path) + `"}}` + "\n"
	if stderr != want {
		t.Errorf("stderr = %s, want %s", stderr, want)
	}
}

func TestSkillsValidateMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.md")
	code, stdout, stderr := runSkillsValidateCLI(t, "skills", "validate", missing)
	if code != 1 {
		t.Fatalf("rc = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("expected an error on stderr")
	}
}
