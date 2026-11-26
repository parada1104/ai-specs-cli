package tracker

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestEvidencePayloadRejectsAnInvalidSlugFormat(t *testing.T) {
	root := t.TempDir()
	for _, slug := range []string{"..", "a/b", " demo", "demo ", "."} {
		if _, err := EvidencePayload(root, slug); err == nil || !strings.Contains(err.Error(), "invalid change slug") {
			t.Errorf("slug %q: err = %v, want invalid change slug", slug, err)
		}
	}
}

func trackerGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// R1-003: an explicitly empty slug means NO scope, exactly as at 75daa44e, so
// an unscoped caller never inherits a change inferred by discovery. Discovery
// stays in ResolveChangeDocument/ResolveProviderItem, and an explicit slug still
// scopes the evidence.
func TestEvidencePayloadEmptySlugIsNoScope(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	trackerMkdir(t, filepath.Join(root, "odd", "tasks"))
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"),
		"## Tracker\ncard_id: odd-card\n")
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "proposal.md"), "## Tracker\ncard_id: openspec-card\n")

	empty := `{"local": "", "remote": "", "code": "", "git": ""}`
	got, err := EvidencePayload(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.JSON()) != empty {
		t.Fatalf("empty slug JSON = %s, want %s", got.JSON(), empty)
	}

	got, err = EvidencePayload(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "openspec-card", "git": ""}`
	if string(got.JSON()) != want {
		t.Fatalf("scoped JSON = %s, want %s", got.JSON(), want)
	}
}

// R1-003: the empty slug with no OpenSpec change at all also stays empty, even
// when the branch names an ODD plan that records a card.
func TestEvidencePayloadEmptySlugIgnoresTheODDPlan(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	trackerMkdir(t, filepath.Join(root, "odd", "tasks"))
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"),
		"## Tracker\ncard_id: odd-card\n")

	got, err := EvidencePayload(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "", "git": ""}`
	if string(got.JSON()) != want {
		t.Fatalf("JSON = %s, want %s", got.JSON(), want)
	}
}

// OpenSpec still wins when an openspec change exists, byte-identically.
func TestEvidencePayloadPrefersTheOpenSpecChangeOverTheODDPlan(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	trackerMkdir(t, filepath.Join(root, "odd", "tasks"))
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"), "## Tracker\ncard_id: odd-card\n")
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	writeFile(t, filepath.Join(folder, "proposal.md"), "## Tracker\ncard_id: openspec-card\n")

	got, err := EvidencePayload(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"local": "", "remote": "", "code": "openspec-card", "git": ""}`
	if string(got.JSON()) != want {
		t.Fatalf("JSON = %s, want %s", got.JSON(), want)
	}
}

// The bind resolver returns the same card and keeps an ODD identity branch-level.
func TestResolveProviderItemKeepsTheODDIdentityBranchLevel(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	trackerMkdir(t, filepath.Join(root, "odd", "tasks"))
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"),
		"## Tracker\ncard_id: odd-card\nurl: https://example.test/odd\n")

	item, ok, err := ResolveProviderItem(root, "")
	if err != nil || !ok {
		t.Fatalf("item = %+v, ok = %v, err = %v", item, ok, err)
	}
	if item.CardID != "odd-card" || item.URL != "https://example.test/odd" || item.Change != "" {
		t.Fatalf("item = %+v, want odd-card / url / empty change", item)
	}
}

// With no card id the resolver reports no item, so a bind is never fabricated.
func TestResolveProviderItemWithoutACardIsNotBindeable(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	trackerMkdir(t, filepath.Join(root, "odd", "tasks"))
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"), "## Tracker\nlist: In Progress\n")

	if _, ok, err := ResolveProviderItem(root, ""); err != nil || ok {
		t.Fatalf("ok = %v, err = %v, want no item", ok, err)
	}
}

// S13: the resolver names the ODD plan it found and that the plan carries no
// ## Tracker section, so the gate can report the real cause instead of a bare
// "no card".
func TestResolveChangeDocumentReportsAnODDPlanWithoutTrackerSection(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	plan := filepath.Join(root, "odd", "tasks", "odd-feature.md")
	writeFile(t, plan, "# odd-feature\n\n## Specs\n\n- S1 — the plan.\n")

	doc, err := ResolveChangeDocument(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != plan || doc.HasTracker || doc.Slug != "" || len(doc.Fields) != 0 {
		t.Fatalf("doc = %+v, want %q, no section, no fields, empty slug", doc, plan)
	}
}

// S13: an OpenSpec change without a ## Tracker section names that document too.
func TestResolveChangeDocumentReportsAnOpenSpecChangeWithoutTrackerSection(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	proposal := filepath.Join(folder, "proposal.md")
	writeFile(t, proposal, "# demo\n\n## Why\n\nNo tracker yet.\n")

	doc, err := ResolveChangeDocument(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != proposal || doc.HasTracker || doc.Slug != "demo" || len(doc.Fields) != 0 {
		t.Fatalf("doc = %+v, want %q, no section, slug demo, no fields", doc, proposal)
	}
}

// S13: a document that has the section reports it and its fields, and the card
// resolution it feeds is unchanged.
func TestResolveChangeDocumentReportsTheSectionAndItsFields(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "demo")
	trackerMkdir(t, folder)
	proposal := filepath.Join(folder, "proposal.md")
	writeFile(t, proposal, "## Tracker\n- **card_id**: `card-1`\n")

	doc, err := ResolveChangeDocument(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != proposal || !doc.HasTracker || doc.Slug != "demo" || doc.Fields["card_id"] != "card-1" {
		t.Fatalf("doc = %+v, want %q with card-1", doc, proposal)
	}
}

// R3-003: a planning document that cannot be read surfaces the error instead of
// vanishing like an absent one, so a real card is never silently dropped.
func TestResolveChangeDocumentSurfacesAnUnreadableDocument(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name: "directory in its place",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unreadable mode",
			setup: func(t *testing.T, path string) {
				t.Helper()
				writeFile(t, path, "## Tracker\ncard_id: card-1\n")
				if err := os.Chmod(path, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "unreadable mode" && os.Geteuid() == 0 {
				t.Skip("root reads a 0000 file")
			}
			root := t.TempDir()
			folder := filepath.Join(root, "openspec", "changes", "demo")
			trackerMkdir(t, folder)
			proposal := filepath.Join(folder, "proposal.md")
			tc.setup(t, proposal)

			if _, err := ResolveChangeDocument(root, "demo"); err == nil {
				t.Fatal("an unreadable document must surface an error, not look absent")
			}
			if _, err := EvidencePayload(root, "demo"); err == nil {
				t.Fatal("evidence must surface the error instead of empty evidence")
			}
		})
	}
}

// S13: with no planning document at all the resolver names nothing, so the gate
// keeps its generic needs-item line.
func TestResolveChangeDocumentWithoutADocumentNamesNothing(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")

	doc, err := ResolveChangeDocument(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "" || doc.HasTracker || doc.Slug != "" || len(doc.Fields) != 0 {
		t.Fatalf("doc = %+v, want an empty document", doc)
	}
}

// An explicit slug that names no OpenSpec change must not borrow the branch's
// ODD plan: at 75daa44e such a slug produced no evidence at all, and returning
// another change's card would report success with the wrong metadata.
func TestResolveChangeDocumentExplicitUnknownSlugDoesNotReachTheODDPlan(t *testing.T) {
	root := t.TempDir()
	trackerGit(t, root, "init", "-b", "feat/odd-feature")
	writeFile(t, filepath.Join(root, "odd", "tasks", "odd-feature.md"),
		"## Tracker\ncard_id: odd-card\nurl: https://example.test/odd\n")

	doc, err := ResolveChangeDocument(root, "no-such-change")
	if err != nil {
		t.Fatalf("ResolveChangeDocument: %v", err)
	}
	if doc.Path != "" || len(doc.Fields) != 0 || doc.HasTracker {
		t.Fatalf("an explicit unknown slug resolved to %+v, want no document", doc)
	}

	evidence, err := EvidencePayload(root, "no-such-change")
	if err != nil {
		t.Fatalf("EvidencePayload: %v", err)
	}
	if evidence.Code != "" || evidence.Git != "" {
		t.Fatalf("evidence = %+v, want empty for an unknown explicit slug", evidence)
	}

	// Discovery (empty slug) still reaches the ODD plan.
	item, ok, err := ResolveProviderItem(root, "")
	if err != nil || !ok {
		t.Fatalf("discovery lost the ODD plan: ok=%v err=%v", ok, err)
	}
	if item.CardID != "odd-card" {
		t.Fatalf("discovered card = %q, want odd-card", item.CardID)
	}
}
