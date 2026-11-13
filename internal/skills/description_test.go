package skills

// GO-20 U3b-2 (T4b): the SKILL.md front-matter description the hub's Skills
// submenu shows. It mirrors hub.py's _skill_description, which the comment there
// says also mirrors skills-list.sh, so U5's `skills list` is the second caller.
// Every case below pins one branch of that function, including the unquoting and
// the 80-character cap, because both are user-visible in the picker.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillDir(t *testing.T, id, content string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDescriptionReadsFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			"a plain description",
			"---\nname: demo\ndescription: Reconcile things safely\n---\n\n# Demo\n",
			"Reconcile things safely",
		},
		{
			"a double-quoted description is unquoted, escapes included",
			"---\ndescription: \"a \\\"quoted\\\" value\"\n---\n",
			"a \"quoted\" value",
		},
		{
			"a single-quoted description is unquoted",
			"---\ndescription: 'plain'\n---\n",
			"plain",
		},
		{
			"an 80-character cap keeps the head of a long description",
			"---\ndescription: " + strings.Repeat("x", 100) + "\n---\n",
			strings.Repeat("x", 80),
		},
		{
			"the cap counts code points, so a multi-byte description is not cut mid-character",
			"---\ndescription: " + strings.Repeat("é", 90) + "\n---\n",
			strings.Repeat("é", 80),
		},
		{
			"no frontmatter at all",
			"# Demo\n\ndescription: not frontmatter\n",
			"",
		},
		{
			"frontmatter without a description key",
			"---\nname: demo\n---\n",
			"",
		},
		{
			"an unclosed frontmatter block",
			"---\ndescription: unfinished\n",
			"",
		},
		{
			"only the first line of a multi-line-ish value, as the regex decides",
			"---\ndescription: first\nname: demo\n---\n",
			"first",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Description(skillDir(t, "demo", tc.content)); got != tc.want {
				t.Errorf("Description = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDescriptionWithoutSkillFile pins the two silent empties: a directory with no
// SKILL.md and a path that does not exist are both "", never an error — the hub
// prints "(none)" for them.
func TestDescriptionWithoutSkillFile(t *testing.T) {
	if got := Description(skillDir(t, "empty", "")); got != "" {
		t.Errorf("Description(no SKILL.md) = %q, want empty", got)
	}
	if got := Description(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Errorf("Description(missing dir) = %q, want empty", got)
	}
}
