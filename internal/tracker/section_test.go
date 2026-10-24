package tracker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseSectionReadsRecognizedKeysOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "proposal.md")
	writeFile(t, p, "# Title\n\n## Tracker\n\n- **card_id**: `6aa703fdcf61a90ec702d58b` # trello card\n- pr: `https://x/pr/1`\nrandom: nope\n\n## Other\ncard_id: ignored\n")
	got, err := ParseSection([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if got["card_id"] != "6aa703fdcf61a90ec702d58b" {
		t.Fatalf("card_id = %q", got["card_id"])
	}
	if got["pr"] != "https://x/pr/1" {
		t.Fatalf("pr = %q", got["pr"])
	}
	if _, ok := got["random"]; ok {
		t.Fatal("unrecognized key must be ignored")
	}
}

func TestParseSectionFirstArtifactWithASectionWins(t *testing.T) {
	dir := t.TempDir()
	proposal := filepath.Join(dir, "proposal.md")
	tasks := filepath.Join(dir, "tasks.md")
	writeFile(t, proposal, "no section here\n")
	writeFile(t, tasks, "## Tracker\ncard_id: abc\n")
	got, err := ParseSection([]string{proposal, tasks})
	if err != nil {
		t.Fatal(err)
	}
	if got["card_id"] != "abc" {
		t.Fatalf("card_id = %q, want abc from tasks.md", got["card_id"])
	}
}

func TestParseSectionFirstKeyWinsAndFencesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.md")
	writeFile(t, p, "## Tracker\ncard_id: first\ncard_id: second\n```\n## Tracker\ncard_id: fenced\n```\n")
	got, err := ParseSection([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if got["card_id"] != "first" {
		t.Fatalf("card_id = %q, want first", got["card_id"])
	}
}

func TestParseSectionMissingFilesYieldEmpty(t *testing.T) {
	got, err := ParseSection([]string{filepath.Join(t.TempDir(), "absent.md")})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty map, nil", got, err)
	}
}

func TestParseSectionInvalidUTF8PropagatesAnError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.md")
	writeFile(t, p, "## Tracker\ncard_id: \xff\xfe\n")
	if _, err := ParseSection([]string{p}); err == nil {
		t.Fatal("invalid UTF-8 must propagate, as Python's read_text does")
	}
}
