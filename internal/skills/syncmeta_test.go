package skills

// `ai-specs skills validate` library surface (GO-20 U2e, unit U-D): the port of
// skill_contract.py's sync-metadata oracle (from_local_skill +
// normalize_local_skill + validate_sync_metadata + _cli_sync_metadata). Every
// expected string below was captured from the Python oracle.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSyncSkill(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// syncPayload loads a SKILL.md and renders the exact stdout line the verb
// prints, ending with the newline print() adds.
func syncPayload(t *testing.T, path string) (string, error) {
	t.Helper()
	skill, err := FromLocalSkillFile(path)
	if err != nil {
		return "", err
	}
	payload, err := ValidateSyncMetadata(skill, path)
	if err != nil {
		return "", err
	}
	return SyncMetadataJSON(payload), nil
}

const defaultsWarnings = `"warnings": ["using compatibility default ` + "`Apache-2.0`" +
	`; add ` + "`license`" + ` explicitly", "using compatibility default ` + "`unknown`" +
	` author; add ` + "`metadata.author`" + ` explicitly", "using compatibility default ` +
	"`1.0`" + ` version; add ` + "`metadata.version`" + ` explicitly"]`

func TestSyncMetadataEnabled(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: Does a demo thing\nlicense: MIT\nmetadata:\n  author: someone\n  version: \"2.0\"\n  scope: [root]\n  auto_invoke: [\"When demoing\"]\n---\nBody here.\n")
	got, err := syncPayload(t, path)
	if err != nil {
		t.Fatalf("syncPayload: %v", err)
	}
	want := `{"enabled": true, "name": "demo-skill", "scope": ["root"], "auto_invoke": ["When demoing"], "warnings": []}`
	if got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
}

func TestSyncMetadataDisabledWithDefaults(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: Does a demo thing\n---\nBody here.\n")
	got, err := syncPayload(t, path)
	if err != nil {
		t.Fatalf("syncPayload: %v", err)
	}
	want := `{"enabled": false, "name": "demo-skill", "scope": [], "auto_invoke": [], ` + defaultsWarnings + `}`
	if got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
}

func TestSyncMetadataScalarScopeWarning(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: x\nmetadata:\n  scope: root\n  auto_invoke: [\"When x\"]\n---\n")
	got, err := syncPayload(t, path)
	if err != nil {
		t.Fatalf("syncPayload: %v", err)
	}
	want := `{"enabled": true, "name": "demo-skill", "scope": ["root"], "auto_invoke": ["When x"], ` +
		defaultsWarnings[:len(defaultsWarnings)-1] + `, "converted scalar ` + "`metadata.scope`" + ` to a one-item list"]}`
	if got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
}

func TestSyncMetadataNonASCIIEscaped(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: café — ünïcode “quoted”\nmetadata:\n  scope: [\"scopé\"]\n  auto_invoke: [\"When ünïcode\"]\n---\n")
	got, err := syncPayload(t, path)
	if err != nil {
		t.Fatalf("syncPayload: %v", err)
	}
	want := `{"enabled": true, "name": "demo-skill", "scope": ["scop\u00e9"], "auto_invoke": ["When \u00fcn\u00efcode"], ` + defaultsWarnings + `}`
	if got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
}

func TestSyncMetadataCRLFEqualsLF(t *testing.T) {
	body := "---\nname: demo-skill\ndescription: x\nmetadata:\n  scope: [root]\n  auto_invoke: [\"When x\"]\n---\n"
	lf := writeSyncSkill(t, body)
	crlf := writeSyncSkill(t, strings.ReplaceAll(body, "\n", "\r\n"))
	want, err := syncPayload(t, lf)
	if err != nil {
		t.Fatalf("lf: %v", err)
	}
	got, err := syncPayload(t, crlf)
	if err != nil {
		t.Fatalf("crlf: %v", err)
	}
	if got != want {
		t.Errorf("crlf payload = %s, want %s", got, want)
	}
}

func TestSyncMetadataContractErrors(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		field      string
		message    string
		wantNoPath bool
	}{
		{"missing frontmatter", "No frontmatter here.\n", "frontmatter", "missing YAML frontmatter", true},
		{"parse error", "---\nname: demo-skill\n bad-indent: true\n---\n", "frontmatter", "unexpected indentation", true},
		{"missing name", "---\ndescription: x\n---\n", "name", "is required", false},
		{"missing description", "---\nname: demo-skill\n---\n", "description", "is required", false},
		{"bad name", "---\nname: Demo_Skill\ndescription: x\n---\n", "name", "must be lowercase kebab-case", false},
		{"bad version", "---\nname: demo-skill\ndescription: x\nmetadata:\n  version: \"1.x\"\n---\n", "metadata.version", "must look like a semantic version (for example `1.0` or `1.0.0`)", false},
		{"blank item", "---\nname: demo-skill\ndescription: x\nmetadata:\n  scope:\n    - \"\"\n    - root\n---\n", "metadata.scope", "must not contain empty items", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSyncSkill(t, tc.body)
			_, err := syncPayload(t, path)
			contract, ok := err.(*SkillContractError)
			if !ok {
				t.Fatalf("err = %v, want *SkillContractError", err)
			}
			if contract.Field != tc.field || contract.Message != tc.message {
				t.Errorf("error = %s: %s, want %s: %s", contract.Field, contract.Message, tc.field, tc.message)
			}
			if tc.wantNoPath {
				if contract.Path != "" {
					t.Errorf("path = %q, want empty (null)", contract.Path)
				}
			} else if contract.Path != filepath.Clean(path) {
				t.Errorf("path = %q, want %q", contract.Path, filepath.Clean(path))
			}
		})
	}
}

// TestValidateSyncMetadataErrorUsesRawPath pins Python's path_arg plumbing:
// from_local_skill raises with str(Path(arg)) (cleaned), but
// validate_sync_metadata's errors keep the RAW argv string.
func TestValidateSyncMetadataErrorUsesRawPath(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: x\nmetadata:\n  auto_invoke: [\"When x\"]\n---\n")
	skill, err := FromLocalSkillFile(path)
	if err != nil {
		t.Fatalf("FromLocalSkillFile: %v", err)
	}
	const rawPath = "/a/./b/../c/SKILL.md"
	_, err = ValidateSyncMetadata(skill, rawPath)
	contract, ok := err.(*SkillContractError)
	if !ok {
		t.Fatalf("err = %v, want *SkillContractError", err)
	}
	if contract.Field != "metadata.scope" || contract.Message != "is required when metadata.auto_invoke is set" {
		t.Errorf("error = %s: %s", contract.Field, contract.Message)
	}
	if contract.Path != rawPath {
		t.Errorf("path = %q, want raw %q", contract.Path, rawPath)
	}
	want := `{"error": {"field": "metadata.scope", "message": "is required when metadata.auto_invoke is set", "path": "/a/./b/../c/SKILL.md"}}`
	if got := SkillContractErrorJSON(contract); got != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

func TestValidateSyncMetadataSymmetricRequirement(t *testing.T) {
	path := writeSyncSkill(t, "---\nname: demo-skill\ndescription: x\nmetadata:\n  scope: [root]\n---\n")
	skill, err := FromLocalSkillFile(path)
	if err != nil {
		t.Fatalf("FromLocalSkillFile: %v", err)
	}
	_, err = ValidateSyncMetadata(skill, path)
	contract, ok := err.(*SkillContractError)
	if !ok {
		t.Fatalf("err = %v, want *SkillContractError", err)
	}
	if contract.Field != "metadata.auto_invoke" || contract.Message != "is required when metadata.scope is set" {
		t.Errorf("error = %s: %s", contract.Field, contract.Message)
	}
}

func TestSyncMetadataErrorJSONNullPath(t *testing.T) {
	contract := &SkillContractError{Field: "frontmatter", Message: "missing YAML frontmatter"}
	want := `{"error": {"field": "frontmatter", "message": "missing YAML frontmatter", "path": null}}`
	if got := SkillContractErrorJSON(contract); got != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

func TestFromLocalSkillFileInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: demo-skill\ndescription: \xff\xfe bad\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := FromLocalSkillFile(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := err.(*SkillContractError); ok {
		t.Errorf("err = %v, want a plain decode/read error (Python raises UnicodeDecodeError)", err)
	}
}
