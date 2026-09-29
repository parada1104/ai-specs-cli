package changelog

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SAMPLE mirrors tests/test_changelog.py SAMPLE.
const sample = `# Changelog

All notable changes are documented in this file.

## [Unreleased]

## [0.22.0] — 2026-08-17

### Added
- Autocontained Go worktree gate.
- Subrepo planning context propagation.

### Upgrade notes
Run ` + "`ai-specs sync`" + ` in each project to acquire the verified Go worktree-gate
binary. Until you do, the gate falls back to Bash.

### Fixed
- Gate binary acquisition no longer 404s.

## [0.21.0] — 2026-08-05

### Added
- Topology-aware worktree gate scope.

## [0.20.1] — 2026-08-05

### Fixed
- Remove untouched legacy recipe command copies.

### Upgrade notes
Re-run ` + "`ai-specs sync`" + ` to drop stale command copies.

## [0.20.0] — 2026-08-05

### Added
- Cross-repo worktree artifact scope.
`

func versions(sections []Section) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.Version
	}
	return out
}

func TestParsesReleasedVersionsAndSkipsUnreleased(t *testing.T) {
	got := versions(ParseSections(sample))
	want := []string{"0.22.0", "0.21.0", "0.20.1", "0.20.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
}

func TestSectionCarriesDateAndBody(t *testing.T) {
	latest := ParseSections(sample)[0]
	if latest.Date != "2026-08-17" {
		t.Errorf("date = %q", latest.Date)
	}
	if !strings.Contains(latest.Body, "Autocontained Go worktree gate") {
		t.Errorf("body missing bullet: %q", latest.Body)
	}
	if strings.Contains(latest.Body, "Topology-aware") {
		t.Errorf("body leaked next section: %q", latest.Body)
	}
}

func TestRangeIsExclusiveOfCurrentInclusiveOfNew(t *testing.T) {
	got := versions(CrossedVersions(sample, "0.20.0", "0.22.0"))
	want := []string{"0.22.0", "0.21.0", "0.20.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("crossed = %v, want %v", got, want)
	}
}

func TestVersionsOrderedBySemverNotString(t *testing.T) {
	text := strings.Replace(sample, "## [0.21.0]", "## [0.9.0]", 1)
	got := versions(CrossedVersions(text, "0.8.0", "0.22.0"))
	want := []string{"0.22.0", "0.20.1", "0.20.0", "0.9.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("crossed = %v, want %v", got, want)
	}
}

func TestSameAndDowngradeCrossNothing(t *testing.T) {
	if got := CrossedVersions(sample, "0.22.0", "0.22.0"); got != nil {
		t.Errorf("same version crossed %v", versions(got))
	}
	if got := CrossedVersions(sample, "0.22.0", "0.21.0"); got != nil {
		t.Errorf("downgrade crossed %v", versions(got))
	}
}

func TestUpgradeNoticeStopsAtNextSubsection(t *testing.T) {
	crossed := CrossedVersions(sample, "0.21.0", "0.22.0")
	notice, ok := UpgradeNotice(crossed[0])
	if !ok {
		t.Fatal("expected a notice")
	}
	if !strings.Contains(notice, "ai-specs sync") || !strings.Contains(notice, "falls back to Bash") {
		t.Errorf("notice = %q", notice)
	}
	if strings.Contains(notice, "404") || strings.Contains(notice, "Fixed") {
		t.Errorf("notice bled into next H3: %q", notice)
	}
}

func TestSectionWithoutNotice(t *testing.T) {
	crossed := CrossedVersions(sample, "0.20.1", "0.21.0")
	if _, ok := UpgradeNotice(crossed[0]); ok {
		t.Error("expected no notice")
	}
}

func TestNoticesReplayOldestFirst(t *testing.T) {
	pairs := CrossedNotices(sample, "0.20.0", "0.22.0")
	got := make([]string, len(pairs))
	for i, p := range pairs {
		got[i] = p.Version
	}
	want := []string{"0.20.1", "0.22.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %v, want %v", got, want)
	}
}

func TestDegradation(t *testing.T) {
	if got := ReadSections("/definitely/absent/CHANGELOG.md"); got != nil {
		t.Errorf("missing file returned %v", got)
	}
	if got := ParseSections("no headings at all"); len(got) != 0 {
		t.Errorf("unparseable returned %v", got)
	}
	text := "## [not-a-version] — whenever\n\n- x\n\n## [0.22.0] — 2026-08-17\n\n- y\n"
	if got := versions(ParseSections(text)); !reflect.DeepEqual(got, []string{"0.22.0"}) {
		t.Errorf("malformed heading not skipped: %v", got)
	}
	if got := versions(CrossedVersions(sample, "not-a-version", "0.22.0")); !reflect.DeepEqual(got, []string{"0.22.0"}) {
		t.Errorf("unknown current should report target only: %v", got)
	}
}

func TestHeadingWithoutDate(t *testing.T) {
	sections := ParseSections("## [0.22.0]\n\n### Added\n- x\n")
	if len(sections) != 1 || sections[0].Date != "" {
		t.Fatalf("sections = %+v", sections)
	}
}

func TestAllDashSeparators(t *testing.T) {
	for _, dash := range []string{"\u2014", "\u2013", "-"} {
		text := "## [1.2.3] " + dash + " 2026-01-01\n\n### Added\n- x.\n"
		sections := ParseSections(text)
		if len(sections) != 1 || sections[0].Date != "2026-01-01" {
			t.Errorf("dash %q: sections = %+v", dash, sections)
		}
	}
}

func TestDuplicateHeadingsCollapsed(t *testing.T) {
	text := "## [1.1.0] — 2026-01-03\n\n### Added\n- newer.\n\n" +
		"## [1.0.0] — 2026-01-02\n\n### Added\n- dup a.\n\n" +
		"## [1.0.0] — 2026-01-02\n\n### Added\n- dup b.\n"
	if got := versions(ParseSections(text)); !reflect.DeepEqual(got, []string{"1.1.0", "1.0.0"}) {
		t.Errorf("duplicates not collapsed: %v", got)
	}
	if got := versions(CrossedVersions(text, "0.9.0", "1.1.0")); !reflect.DeepEqual(got, []string{"1.1.0", "1.0.0"}) {
		t.Errorf("crossed duplicates: %v", got)
	}
}

func TestFencedHeadingIsNotABoundary(t *testing.T) {
	text := "## [1.0.0] — 2026-01-01\n\n### Added\n- documented a markdown sample.\n\n" +
		"```markdown\n## [9.9.9] — not a real release\n```\n\n### Fixed\n- a real fix.\n"
	sections := ParseSections(text)
	if !reflect.DeepEqual(versions(sections), []string{"1.0.0"}) {
		t.Fatalf("fenced heading became a section: %v", versions(sections))
	}
	if !strings.Contains(sections[0].Body, "a real fix") {
		t.Errorf("body truncated at fence: %q", sections[0].Body)
	}
}

func TestNoticeNotTruncatedByFencedHeading(t *testing.T) {
	text := "## [1.0.0] — 2026-01-01\n\n### Upgrade notes\nRun this:\n\n" +
		"```sh\n### not a heading\n```\n\nThen you are done.\n\n### Fixed\n- unrelated.\n"
	notice, ok := UpgradeNotice(ParseSections(text)[0])
	if !ok || !strings.Contains(notice, "Then you are done.") {
		t.Fatalf("notice = %q ok=%v", notice, ok)
	}
	if strings.Contains(notice, "unrelated") {
		t.Errorf("notice bled past H3: %q", notice)
	}
}

func TestSummaryBullets(t *testing.T) {
	latest := ParseSections(sample)[0]
	bullets := SummaryBullets(latest, 3)
	// Three bullets: two from ### Added plus one from ### Fixed. The
	// ### Upgrade notes prose is excluded.
	want := []string{
		"Autocontained Go worktree gate.",
		"Subrepo planning context propagation.",
		"Gate binary acquisition no longer 404s.",
	}
	if !reflect.DeepEqual(bullets, want) {
		t.Fatalf("bullets = %v, want %v", bullets, want)
	}
	if strings.Contains(strings.Join(bullets, " "), "falls back to Bash") {
		t.Errorf("notice prose leaked into summary: %v", bullets)
	}
	if RemainingCount(latest, 50) != 0 {
		t.Errorf("remaining = %d, want 0", RemainingCount(latest, 50))
	}
}

func TestBulletMarkupStrippedAndJoined(t *testing.T) {
	section := Section{Version: "1.0.0", Body: "### Added\n- **Bold thing**: does `stuff`.\n"}
	if got := SummaryBullets(section, 3); !reflect.DeepEqual(got, []string{"Bold thing: does stuff."}) {
		t.Errorf("got %v", got)
	}
	section = Section{Version: "1.0.0", Body: "### Added\n- a thing that wraps\n  onto a second line.\n"}
	if got := SummaryBullets(section, 3); !reflect.DeepEqual(got, []string{"a thing that wraps onto a second line."}) {
		t.Errorf("got %v", got)
	}
}

func TestBulletCapAndRemaining(t *testing.T) {
	body := "### Added\n"
	for i := 0; i < 10; i++ {
		body += "- item.\n"
	}
	section := Section{Version: "1.0.0", Body: body}
	if got := SummaryBullets(section, 3); len(got) != 3 {
		t.Errorf("len = %d", len(got))
	}
	if got := RemainingCount(section, 3); got != 7 {
		t.Errorf("remaining = %d, want 7", got)
	}
}

func TestFirstSentenceAndTruncation(t *testing.T) {
	section := Section{Version: "1.0.0", Body: "### Added\n- **Thing**: the short claim. Then a long elaboration.\n"}
	if got := SummaryBullets(section, 3); !reflect.DeepEqual(got, []string{"Thing: the short claim."}) {
		t.Errorf("got %v", got)
	}
	long := Section{Version: "1.0.0", Body: "### Added\n- " + strings.Repeat("word ", 60) + "\n"}
	bullet := SummaryBullets(long, 3)[0]
	if len([]rune(bullet)) > 100 {
		t.Errorf("bullet too long: %d", len([]rune(bullet)))
	}
	if !strings.HasSuffix(bullet, "\u2026") {
		t.Errorf("bullet = %q", bullet)
	}
	alpha := Section{Version: "1.0.0", Body: "### Added\n- " + strings.Repeat("alpha ", 60) + "\n"}
	b := SummaryBullets(alpha, 3)[0]
	if strings.Contains(b, "alph\u2026") {
		t.Errorf("truncation split a word: %q", b)
	}
}

func TestSectionWithoutBullets(t *testing.T) {
	section := Section{Version: "1.0.0", Body: "prose only\n"}
	if got := SummaryBullets(section, 3); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestPySplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", []string{""}},
		{"a\n", []string{"a"}},
		{"a\n\n", []string{"a", ""}},
		{"a\r\nb", []string{"a", "b"}},
		{"a\rb", []string{"a", "b"}},
		{"a\vb", []string{"a", "b"}},
		{"a\u2028b", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := pySplitLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %q want %q", c.in, got, c.want)
				break
			}
		}
	}
}

func TestRunCLIUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := RunCLI([]string{"changelog.py", "a", "b"}, &out, &errOut); code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errOut.String(), "usage:") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunCLINothingToReport(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := RunCLI([]string{"changelog.py", "/nonexistent", "0.1.0", "0.2.0"}, &out, &errOut); code != 0 {
		t.Errorf("code = %d", code)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q", out.String())
	}
}

// --- differential: Go RunCLI vs `python3 lib/_internal/changelog.py` ---

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s does not contain go.mod: %v", root, err)
	}
	return root
}

func TestDifferentialCLI(t *testing.T) {
	root := repoRoot(t)
	pyScript := filepath.Join(root, "lib", "_internal", "changelog.py")
	if _, err := os.Stat(pyScript); err != nil {
		t.Fatalf("python reference missing: %v", err)
	}
	realChangelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read repo changelog: %v", err)
	}

	longBody := "## [1.0.0] — 2026-01-01\n\n### Added\n"
	for i := 0; i < 8; i++ {
		longBody += "- bullet number " + string(rune('a'+i)) + " that runs a bit long and has no sentence break\n"
	}

	corpus := []struct{ name, text string }{
		{"sample", sample},
		{"empty", ""},
		{"noheadings", "no headings at all"},
		{"malformed", "## [not-a-version] — whenever\n\n- x\n\n## [0.22.0] — 2026-08-17\n\n- y\n"},
		{"nodate", "## [0.22.0]\n\n### Added\n- x\n"},
		{"endash", "## [0.22.0] \u2013 2026-08-17\n\n### Added\n- a thing.\n"},
		{"dupes", "## [1.1.0] — 2026-01-03\n\n### Added\n- newer.\n\n## [1.0.0] — 2026-01-02\n\n### Added\n- a.\n\n## [1.0.0] — 2026-01-02\n\n### Added\n- b.\n"},
		{"fenced", "## [1.0.0] — 2026-01-01\n\n### Added\n- markdown sample.\n\n```markdown\n## [9.9.9] — not real\n```\n\n### Fixed\n- a real fix.\n"},
		{"noticefence", "## [1.0.0] — 2026-01-01\n\n### Upgrade notes\nRun:\n\n```sh\n### not a heading\n```\n\nThen done.\n\n### Fixed\n- unrelated.\n"},
		{"markup", "## [1.0.0] — 2026-01-01\n\n### Added\n- **Bold**: does `stuff`.\n- wraps\n  onto next line.\n\n### Upgrade notes\nDo the thing.\n"},
		{"longbody", longBody},
		{"unreleasedonly", "# c\n\n## [Unreleased]\n\n- x\n"},
		{"crlf", strings.ReplaceAll(sample, "\n", "\r\n")},
		{"real", string(realChangelog)},
	}
	pairs := [][2]string{
		{"0.20.0", "0.22.0"},
		{"0.21.0", "0.22.0"},
		{"0.22.0", "0.22.0"},
		{"0.22.0", "0.21.0"},
		{"not-a-version", "0.22.0"},
		{"0.20.1", "0.21.0"},
		{"0.19.0", "0.22.0"},
		{"1.0.0", "2.0.0"},
		{"0.0.0", "9.9.9"},
	}

	tmp := t.TempDir()
	compared := 0
	for _, entry := range corpus {
		path := filepath.Join(tmp, entry.name+".md")
		if err := os.WriteFile(path, []byte(entry.text), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		for _, pair := range pairs {
			for _, notices := range []bool{false, true} {
				args := []string{pyScript, path, pair[0], pair[1]}
				goArgs := []string{"changelog.py", path, pair[0], pair[1]}
				if notices {
					args = append(args, "--notices")
					goArgs = append(goArgs, "--notices")
				}
				cmd := exec.Command("python3", args...)
				var pyOut, pyErr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &pyOut, &pyErr
				runErr := cmd.Run()
				pyCode := 0
				if runErr != nil {
					var ee *exec.ExitError
					if !errors.As(runErr, &ee) {
						t.Fatalf("python3 %v: %v", args, runErr)
					}
					pyCode = ee.ExitCode()
				}
				var goOut, goErr bytes.Buffer
				goCode := RunCLI(goArgs, &goOut, &goErr)
				if goCode != pyCode {
					t.Errorf("%s %v notices=%v: exit go=%d python=%d", entry.name, pair, notices, goCode, pyCode)
				}
				if goOut.String() != pyOut.String() {
					t.Errorf("%s %v notices=%v: stdout mismatch\n go:     %q\n python: %q", entry.name, pair, notices, goOut.String(), pyOut.String())
					continue
				}
				compared++
			}
		}
	}
	t.Logf("differential changelog CLI: compared %d invocations", compared)
	if compared < 200 {
		t.Fatalf("only %d invocations compared — corpus unexpectedly small", compared)
	}
}
