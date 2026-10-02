package skills

import (
	"bytes"
	"os"
	"path/filepath"
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

func writeSkill(t *testing.T, dir string) {
	t.Helper()
	writeFileT(t, filepath.Join(dir, "SKILL.md"), "---\nname: sample\nversion: 1.0.0\n---\nbody\n")
}

// buildSkillTree fabricates all four tiers plus duplicates inside a tier. It
// returns (root, home) and the cache root.
func buildSkillTree(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	cache := projectcache.CacheRoot(root, home)

	writeSkill(t, filepath.Join(root, "ai-specs", "skills", "shared"))
	writeSkill(t, filepath.Join(root, "ai-specs", "skills", "localonly"))

	writeSkill(t, filepath.Join(cache, ".recipe", "rec-a", "skills", "shared"))
	writeSkill(t, filepath.Join(cache, ".recipe", "rec-a", "skills", "recipeonly"))
	writeSkill(t, filepath.Join(cache, ".recipe", "rec-b", "skills", "shared"))

	writeSkill(t, filepath.Join(root, "ai-specs", ".deps", "dep-a", "skills", "deponly"))
	writeSkill(t, filepath.Join(cache, ".deps", "dep-b", "skills", "shared"))
	writeSkill(t, filepath.Join(cache, ".deps", "dep-b", "skills", "depcache"))

	writeSkill(t, filepath.Join(cache, ".bundled", "skills", "shared"))
	writeSkill(t, filepath.Join(cache, ".bundled", "skills", "bundledonly"))

	return root, home, cache
}

func TestCollectSkillsPrecedence(t *testing.T) {
	root, home, cache := buildSkillTree(t)

	var stderr bytes.Buffer
	got := CollectSkillsTo(root, home, &stderr)

	want := map[string]ResolvedSkill{
		"shared":      {Source: "local", Path: filepath.Join(root, "ai-specs", "skills", "shared")},
		"localonly":   {Source: "local", Path: filepath.Join(root, "ai-specs", "skills", "localonly")},
		"recipeonly":  {Source: "recipe", Path: filepath.Join(cache, ".recipe", "rec-a", "skills", "recipeonly")},
		"deponly":     {Source: "dep", Path: filepath.Join(root, "ai-specs", ".deps", "dep-a", "skills", "deponly")},
		"depcache":    {Source: "dep", Path: filepath.Join(cache, ".deps", "dep-b", "skills", "depcache")},
		"bundledonly": {Source: "bundled", Path: filepath.Join(cache, ".bundled", "skills", "bundledonly")},
	}
	if len(got) != len(want) {
		t.Fatalf("resolved %d skills, want %d: %#v", len(got), len(want), got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %+v, want %+v", id, got[id], w)
		}
	}

	// rec-b duplicates "shared" after rec-a; first-seen (rec-a) wins and a
	// warning names the first-seen recipe.
	if !bytes.Contains(stderr.Bytes(), []byte("skill 'shared' found in multiple recipes; using first-seen from 'rec-a'")) {
		t.Errorf("missing recipe duplicate warning, stderr=%q", stderr.String())
	}
}

func TestCollectSkillsDepTierFirstSeen(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cache := projectcache.CacheRoot(root, home)

	// Same id in the in-project dep root and the cache dep root: the
	// in-project root is scanned first.
	writeSkill(t, filepath.Join(root, "ai-specs", ".deps", "dep-inproj", "skills", "dupe"))
	writeSkill(t, filepath.Join(cache, ".deps", "dep-cache", "skills", "dupe"))

	var stderr bytes.Buffer
	got := CollectSkillsTo(root, home, &stderr)
	if got["dupe"].Source != "dep" {
		t.Fatalf("dupe source = %q, want dep", got["dupe"].Source)
	}
	if got["dupe"].Path != filepath.Join(root, "ai-specs", ".deps", "dep-inproj", "skills", "dupe") {
		t.Fatalf("dupe path = %q, want in-project dep", got["dupe"].Path)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("skill 'dupe' found in multiple deps; using first-seen from 'dep-inproj'")) {
		t.Errorf("missing dep duplicate warning, stderr=%q", stderr.String())
	}
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body := SplitFrontmatter("---\nname: x\n---\nbody\n")
	if fm != "name: x" || body != "body\n" {
		t.Fatalf("got fm=%q body=%q", fm, body)
	}
	if fm, body := SplitFrontmatter("no frontmatter"); fm != "" || body != "no frontmatter" {
		t.Fatalf("missing-open: fm=%q body=%q", fm, body)
	}
	if fm, body := SplitFrontmatter("---\nname: x\n"); fm != "" || body != "---\nname: x\n" {
		t.Fatalf("unterminated: fm=%q body=%q", fm, body)
	}
}

func TestParseFrontmatter(t *testing.T) {
	data, err := ParseFrontmatter("name: demo\nversion: \"1.0.0\"\ntags: [a, 'b']\ndescription: |\n  line one\n  line two\n")
	if err != nil {
		t.Fatal(err)
	}
	if data["name"] != "demo" {
		t.Errorf("name = %#v", data["name"])
	}
	if data["version"] != "1.0.0" {
		t.Errorf("version = %#v", data["version"])
	}
	if tags, ok := data["tags"].([]string); !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %#v", data["tags"])
	}
	if data["description"] != "line one line two" {
		t.Errorf("description = %#v", data["description"])
	}
}

func TestParseFrontmatterErrors(t *testing.T) {
	if _, err := ParseFrontmatter("  indented: 1\n"); err == nil {
		t.Error("expected unexpected-indentation error")
	}
	if _, err := ParseFrontmatter("no colon here\n"); err == nil {
		t.Error("expected unsupported-line error")
	}
}

func TestStripQuotesAndInlineList(t *testing.T) {
	cases := map[string]string{`"x"`: "x", `'y'`: "y", `z`: "z", `""`: "", `"`: `"`}
	for in, want := range cases {
		if got := StripQuotes(in); got != want {
			t.Errorf("StripQuotes(%q) = %q, want %q", in, got, want)
		}
	}
	got := SplitInlineList(`["a", 'b' , c]`)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("SplitInlineList = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitInlineList[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if out := SplitInlineList("[]"); len(out) != 0 {
		t.Errorf("SplitInlineList([]) = %#v", out)
	}
}
