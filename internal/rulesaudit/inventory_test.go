package rulesaudit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseMDCMetaFallback(t *testing.T) {
	// A "# comment" line makes ParseFrontmatter raise, driving the tolerant
	// fallback path; description survives, globs are absent.
	meta := parseMDCMeta("description: Legacy notes\n# not yaml\nalwaysApply: false\n")
	if meta["description"] != "Legacy notes" {
		t.Errorf("description = %#v", meta["description"])
	}
	if _, ok := meta["globs"]; ok {
		t.Errorf("globs should be absent: %#v", meta)
	}
	if meta["alwaysApply"] != "false" {
		t.Errorf("alwaysApply = %#v", meta["alwaysApply"])
	}

	// Valid YAML returns the parsed map directly (globs as a list).
	meta = parseMDCMeta("description: Deploy\nglobs: [\"*.ts\", \"*.md\"]\nalwaysApply: true\n")
	globs, ok := meta["globs"].([]string)
	if !ok || len(globs) != 2 || globs[0] != "*.ts" {
		t.Fatalf("globs = %#v", meta["globs"])
	}

	// The regex fallback fills a key the line loop dropped.
	meta = parseMDCMeta("name: foo\n# junk line\ndescription:\n\nglobs: **/*.go\n")
	if meta["globs"] != "**/*.go" {
		t.Errorf("globs fallback = %#v", meta["globs"])
	}
}

func TestCoerceBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{true, true},
		{"true", true},
		{" TRUE ", true},
		{"1", true},
		{"yes", true},
		{"no", false},
		{"", false},
		{[]string{}, false},
		{[]string{"x"}, true},
	}
	for _, tc := range cases {
		if got := coerceBool(tc.in); got != tc.want {
			t.Errorf("coerceBool(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestKeywordInHaystackBoundaries(t *testing.T) {
	cases := []struct {
		keyword, haystack string
		want              bool
	}{
		{"worktree", "use worktree here", true},
		{"worktree", "worktrees are great", false},
		{"worktree", "xworktree", false},
		{"worktree", "a worktree", true},
		{"worktree", "worktree", true},
		{"pull request", "open a pull request now", true},
		{"pull request", "pullrequests", false},
		{"tdd", "tdd workflow", true},
		{"tdd", "tddx", false},
		{"project", "project rules", true},
		{"project", "projects", false},
	}
	for _, tc := range cases {
		if got := keywordInHaystack(tc.keyword, tc.haystack); got != tc.want {
			t.Errorf("keywordInHaystack(%q, %q) = %v, want %v", tc.keyword, tc.haystack, got, tc.want)
		}
	}
}

func TestStackHintsAndDefaultRecipes(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "package-lock.json"), "{}")
	writeFileT(t, filepath.Join(root, "pyproject.toml"), "[project]\n")
	writeFileT(t, filepath.Join(root, "go.mod"), "module x\n")

	hints := stackHints(root)
	wantHints := []string{"node", "python", "go"}
	if strings.Join(hints, ",") != strings.Join(wantHints, ",") {
		t.Fatalf("stackHints = %v, want %v", hints, wantHints)
	}
	recipes := defaultRecipesForStack(hints)
	wantRecipes := []string{"session-context", "worktree-flow", "tdd-flow", "git-pr-flow"}
	if strings.Join(recipes, ",") != strings.Join(wantRecipes, ",") {
		t.Fatalf("defaultRecipesForStack = %v, want %v", recipes, wantRecipes)
	}
	if got := defaultRecipesForStack([]string{"go"}); strings.Join(got, ",") != "session-context" {
		t.Fatalf("go-only recipes = %v", got)
	}
}

func TestExcerptCountsRunes(t *testing.T) {
	// 401 three-byte runes: the excerpt must cap at 400 code points, not bytes.
	long := strings.Repeat("é", 401)
	got := excerpt(long)
	if len([]rune(got)) != bodyExcerptLen {
		t.Fatalf("excerpt rune length = %d, want %d", len([]rune(got)), bodyExcerptLen)
	}
}

func TestBlankFences(t *testing.T) {
	text := "# Top\n\n```sh\n# not a heading\necho hi\n```\n\n## Real\n"
	blanked := blankFences(text)
	matches := headingRE.FindAllStringSubmatch(blanked, -1)
	var headings []string
	for _, m := range matches {
		headings = append(headings, m[1])
	}
	if len(headings) != 2 || headings[0] != "Top" || headings[1] != "Real" {
		t.Fatalf("headings = %v", headings)
	}
	// Length is preserved so offsets still align with the original text.
	if len(blanked) != len(text) {
		t.Fatalf("blanked length = %d, want %d", len(blanked), len(text))
	}
}

func TestBlankFencesUnterminated(t *testing.T) {
	text := "# Top\n\n~~~\n# hidden never closed\n"
	headings := headingRE.FindAllStringSubmatch(blankFences(text), -1)
	if len(headings) != 1 || headings[0][1] != "Top" {
		t.Fatalf("headings = %v", headings)
	}
}

func TestRunRejectsNonDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	writeFileT(t, file, "x")
	var stdout, stderr bytes.Buffer
	if code := Run(file, t.TempDir(), &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.HasPrefix(stderr.String(), "ERROR: not a directory: ") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

const emptyProjectJSON = `{
  "schema_version": 1,
  "mode": "B",
  "target": "%s",
  "classification_is_suggestion": true,
  "sources": {
    "cursor_rules": [],
    "cursorrules": {
      "status": "absent"
    },
    "agents_md_sections": [],
    "agents_md_present": false,
    "manifest": {
      "present": false
    },
    "resolved_skills": [],
    "recipe_catalog": [
      "worktree-flow",
      "git-pr-flow",
      "session-context",
      "tdd-flow",
      "trello-mcp-workflow",
      "vault-canonical-store"
    ],
    "atl_registry": {
      "present": false,
      "skill_ids": []
    }
  },
  "summary": {
    "cursor_rules": 0,
    "cursorrules": 0,
    "agents_md_sections": 0
  },
  "stack_hints": [],
  "recommendations": {
    "init": "ai-specs init",
    "default_recipes": [
      "session-context"
    ],
    "brief_hint": "Draft a runtime-brief [brief] section in AGENTS.md after init."
  }
}
`

func TestEmptyProjectPayload(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run(root, home, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	want := fmt.Sprintf(emptyProjectJSON, projectcache.ResolvePath(root))
	if stdout.String() != want {
		t.Fatalf("payload mismatch:\n--- got ---\n%s\n--- want ---\n%s", stdout.String(), want)
	}
}

// TestBlankFencesFrozenFenceParity pins R3-001 as frozen parity: CRLF closing
// fences and longer closing runs leave the fence unterminated in BOTH the Go
// port and the frozen oracle (lib/_internal/rules-inventory.py:417-420 —
// `^(?P=fence)[ \t]*$` matches neither \r nor a longer run), so both blank
// from the opening fence to EOF. Do NOT "fix" this before the post-cutover
// parity amendment.
func TestBlankFencesFrozenFenceParity(t *testing.T) {
	blankAll := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return '\n'
			}
			return ' '
		}, s)
	}
	cases := []struct {
		name string
		text string
	}{
		{"control: LF fence closes", "```\nbody\n```\n# Heading\n"},
		{"CRLF closing fence stays unterminated", "```\nbody\r\n```\r\n# Heading\n"},
		{"longer closing run stays unterminated", "```\nbody\n````\n# Heading\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := blankFences(tc.text)
			switch tc.name {
			case "control: LF fence closes":
				if got != "   \n    \n   \n# Heading\n" {
					t.Fatalf("control fence should close with heading preserved, got %q", got)
				}
			default:
				if want := blankAll(tc.text); got != want {
					t.Fatalf("unterminated-fence blank-to-EOF parity broken:\n got %q\nwant %q", got, want)
				}
			}
		})
	}
}
