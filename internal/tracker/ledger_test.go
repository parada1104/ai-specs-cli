package tracker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func trackerMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestChangeSlugIsTheSingleActiveEntry(t *testing.T) {
	root := t.TempDir()
	trackerMkdir(t, filepath.Join(root, "openspec", "changes", "one-change"))
	trackerMkdir(t, filepath.Join(root, "openspec", "changes", "archive", "old-change"))
	if got := ChangeSlug(root); got != "one-change" {
		t.Fatalf("ChangeSlug = %q, want one-change", got)
	}

	trackerMkdir(t, filepath.Join(root, "openspec", "changes", "two-change"))
	if got := ChangeSlug(root); got != "" {
		t.Fatalf("ChangeSlug with two active changes = %q, want empty", got)
	}
}

func TestChangeSlugMissingTreeIsEmpty(t *testing.T) {
	if got := ChangeSlug(t.TempDir()); got != "" {
		t.Fatalf("ChangeSlug without openspec = %q, want empty", got)
	}
}

func TestChangeDirResolutionOrder(t *testing.T) {
	root := t.TempDir()
	changes := filepath.Join(root, "openspec", "changes")
	trackerMkdir(t, filepath.Join(changes, "active"))
	trackerMkdir(t, filepath.Join(changes, "archive", "2026-01-01-dated"))
	trackerMkdir(t, filepath.Join(changes, "archive", "2026-03-04-dated"))
	trackerMkdir(t, filepath.Join(changes, "archive", "legacy"))

	if got, ok := changeDir(root, "active"); !ok || got != filepath.Join(changes, "active") {
		t.Fatalf("active = %q, %v", got, ok)
	}
	if got, ok := changeDir(root, "dated"); !ok || got != filepath.Join(changes, "archive", "2026-03-04-dated") {
		t.Fatalf("newest dated = %q, %v", got, ok)
	}
	if got, ok := changeDir(root, "legacy"); !ok || got != filepath.Join(changes, "archive", "legacy") {
		t.Fatalf("legacy = %q, %v", got, ok)
	}
	// A change that does not exist yet resolves to the active-shaped fallback.
	if got, ok := changeDir(root, "not-yet"); !ok || got != filepath.Join(changes, "not-yet") {
		t.Fatalf("fallback = %q, %v", got, ok)
	}
}

func TestChangeDirRefusesInvalidSlugsAndEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	changes := filepath.Join(root, "openspec", "changes")
	trackerMkdir(t, changes)
	outside := filepath.Join(t.TempDir(), "escaped")
	trackerMkdir(t, outside)
	if err := os.Symlink(outside, filepath.Join(changes, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, slug := range []string{"", "..", "../escape", "/abs", " spaced"} {
		if _, ok := changeDir(root, slug); ok {
			t.Errorf("changeDir accepted invalid slug %q", slug)
		}
	}
	if _, ok := changeDir(root, "escape"); ok {
		t.Fatal("a symlink escaping the planning tree must be refused")
	}
}

func TestTrackerNoneReasonReadsTheFirstNonEmptyLine(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "tracker.none"), "\n  no card for this spike  \nsecond line\n")

	reason, present, err := trackerNoneReason(root, "demo")
	if err != nil || !present || reason != "no card for this spike" {
		t.Fatalf("reason = %q, present = %v, err = %v", reason, present, err)
	}
}

func TestTrackerNoneReasonAbsentAndFallback(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)

	if _, present, err := trackerNoneReason(root, "demo"); err != nil || present {
		t.Fatalf("absent tracker.none: present = %v, err = %v", present, err)
	}

	writeFile(t, filepath.Join(folder, "tracker.none"), "\n \n")
	reason, present, err := trackerNoneReason(root, "demo")
	if err != nil || !present || reason != "tracker.none" {
		t.Fatalf("empty tracker.none: reason = %q, present = %v, err = %v", reason, present, err)
	}
}

func TestTrackerNoneReasonNeverCreatesOrDeletesTheFile(t *testing.T) {
	root := t.TempDir()
	if _, _, _ = trackerNoneReason(root, "demo"); true {
		path := filepath.Join(root, "openspec", "changes", "demo", "tracker.none")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("tracker.none was created: %v", err)
		}
	}
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "tracker.none"), "reason\n")
	_, _, _ = trackerNoneReason(root, "demo")
	if _, err := os.Stat(filepath.Join(folder, "tracker.none")); err != nil {
		t.Fatalf("tracker.none was removed: %v", err)
	}
}

func TestTrackerNoneReasonInvalidUTF8Propagates(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "tracker.none"), "reason \xff\n")
	if _, _, err := trackerNoneReason(root, "demo"); err == nil {
		t.Fatal("invalid UTF-8 must propagate")
	}
}

func TestEvidencePayloadBuildsCodeAndGitFromTheTrackerSection(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "proposal.md"),
		"## Tracker\n- **card_id**: `6aa703fdcf61a90ec702d58b`\n- pr: `https://example/pr/1`\n")

	got, err := EvidencePayload(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "6aa703fdcf61a90ec702d58b", "git": "6aa703fdcf61a90ec702d58b"}`
	if string(got.JSON()) != want {
		t.Fatalf("JSON = %s, want %s", got.JSON(), want)
	}
}

func TestEvidencePayloadWithoutPRBlanksGit(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), "## Tracker\ncard_id: 6aa703fdcf61a90ec702d58b\n")

	got, err := EvidencePayload(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "6aa703fdcf61a90ec702d58b", "git": ""}`
	if string(got.JSON()) != want {
		t.Fatalf("JSON = %s, want %s", got.JSON(), want)
	}
}

func TestEvidencePayloadEmptyCases(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "proposal.md"), "## Tracker\ncard_id: abc\n")
	writeFile(t, filepath.Join(folder, "tracker.none"), "exempt\n")

	empty := `{"local": "", "remote": "", "code": "", "git": ""}`
	for _, slug := range []string{"", "demo", "absent"} {
		got, err := EvidencePayload(root, slug)
		if err != nil {
			t.Fatalf("slug %q: %v", slug, err)
		}
		if string(got.JSON()) != empty {
			t.Errorf("slug %q: JSON = %s, want %s", slug, got.JSON(), empty)
		}
	}
}

func TestEvidencePayloadInvalidUTF8PropagatesAnError(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), "## Tracker\ncard_id: \xff\n")
	if _, err := EvidencePayload(root, "demo"); err == nil {
		t.Fatal("invalid UTF-8 must propagate")
	}
}

// TestEvidenceJSONEscapesLikePythonJSONDumps pins the exact bytes, including
// Python's ensure_ascii escaping of non-ASCII and its non-escaping of HTML.
func TestEvidenceJSONEscapesLikePythonJSONDumps(t *testing.T) {
	cases := map[string]string{
		`<&>`:          `"<&>"`,
		"café":         `"caf\u00e9"`,
		"tab\there":    `"tab\there"`,
		"quote\"slash": `"quote\"slash"`,
	}
	for input, want := range cases {
		var got map[string]string
		raw := string((Evidence{Code: input}).JSON())
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("invalid JSON for %q: %v (%s)", input, err, raw)
		}
		if got["code"] != input {
			t.Fatalf("round-trip of %q = %q", input, got["code"])
		}
		if !contains(raw, want) {
			t.Errorf("JSON for %q = %s; want substring %s", input, raw, want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
