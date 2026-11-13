package changelog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
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

// TestRepositoryChangelogStaysScannable is the native half of the retired
// tests/test_changelog.py "against the real changelog" cases: the parser is
// applied to the repository CHANGELOG.md that ships with the CLI, not to the
// frozen copy the differential corpus uses.
func TestRepositoryChangelogStaysScannable(t *testing.T) {
	raw, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatalf("read repository CHANGELOG.md: %v", err)
	}
	text := string(raw)
	sections := ParseSections(text)
	got := versions(sections)
	for _, want := range []string{"0.22.0", "0.21.0"} {
		if !slices.Contains(got, want) {
			t.Errorf("released version %q missing from %v", want, got)
		}
	}
	if slices.Contains(got, "Unreleased") {
		t.Errorf("the unreleased heading was parsed as a section: %v", got)
	}
	seen := map[string]bool{}
	for _, version := range got {
		if seen[version] {
			t.Errorf("duplicate version %q after parsing", version)
		}
		seen[version] = true
	}
	for _, section := range sections {
		for _, bullet := range SummaryBullets(section, 3) {
			if n := len([]rune(bullet)); n > 100 {
				t.Errorf("%s: bullet is %d runes: %q", section.Version, n, bullet)
			}
		}
	}
	crossed := versions(CrossedVersions(text, "0.19.0", "0.22.0"))
	if want := []string{"0.22.0", "0.21.0", "0.20.1", "0.20.0"}; !slices.Equal(crossed, want) {
		t.Errorf("crossed 0.19.0..0.22.0 = %v, want %v", crossed, want)
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

// --- frozen oracle: Go RunCLI vs the captured `lib/_internal/changelog.py` ---

// The legacy differential used to exec python3 over this corpus on every run.
// RunCLI's output is a pure function of the changelog text and the
// (current, new, --notices) arguments — no clock, no host path, no directory
// name — so the legacy output was captured once into
// testdata/changelog_golden.json. The test never execs python3 and never reads
// lib/. The golden is a snapshot: regenerating it needs a human decision.

const changelogGoldenPath = "testdata/changelog_golden.json"

// realChangelogPath is a frozen copy of the repository CHANGELOG.md the oracle
// was captured from. Freezing the text keeps the golden valid as CHANGELOG.md
// keeps evolving, and the pinned corpus manifest still ties that text to the
// captured inputs.
const realChangelogPath = "testdata/real_changelog.md"

type frozenInvocation struct {
	Current string `json:"current"`
	New     string `json:"new"`
	Notices bool   `json:"notices"`
	Exit    int    `json:"exit"`
	Stdout  string `json:"stdout"`
}

type frozenChangelogCase struct {
	Name string             `json:"name"`
	Runs []frozenInvocation `json:"runs"`
}

type frozenChangelog struct {
	Oracle map[string]any        `json:"oracle"`
	Corpus []string              `json:"corpus"`
	Cases  []frozenChangelogCase `json:"cases"`
}

// expectedChangelogGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedChangelogGoldenCases = []string{
	"sample",
	"empty",
	"noheadings",
	"malformed",
	"nodate",
	"endash",
	"dupes",
	"fenced",
	"noticefence",
	"markup",
	"longbody",
	"unreleasedonly",
	"crlf",
	"real",
}

// changelogPairs is the (current, new) grid every corpus entry is run against,
// doubled over with and without --notices.
var changelogPairs = [][2]string{
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

type changelogCorpusCase struct{ name, text string }

func differentialCorpus(t *testing.T) []changelogCorpusCase {
	t.Helper()
	realChangelog, err := os.ReadFile(realChangelogPath)
	if err != nil {
		t.Fatalf("read frozen real changelog %s: %v", realChangelogPath, err)
	}

	longBody := "## [1.0.0] — 2026-01-01\n\n### Added\n"
	for i := 0; i < 8; i++ {
		longBody += "- bullet number " + string(rune('a'+i)) + " that runs a bit long and has no sentence break\n"
	}

	return []changelogCorpusCase{
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
}

// changelogCorpusManifest pins the corpus inputs the oracle was captured from
// as sorted "sha256  <case>.md" lines, so corpus drift fails as input drift
// instead of an opaque payload mismatch.
func changelogCorpusManifest(t *testing.T, dir string, corpus []changelogCorpusCase) []string {
	t.Helper()
	lines := make([]string, 0, len(corpus))
	for _, entry := range corpus {
		rel := entry.name + ".md"
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("manifest %s: %v", rel, err)
		}
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), rel))
	}
	sort.Strings(lines)
	return lines
}

func loadChangelogGolden(t *testing.T) frozenChangelog {
	t.Helper()
	raw, err := os.ReadFile(changelogGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", changelogGoldenPath, err)
	}
	var golden frozenChangelog
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", changelogGoldenPath, err)
	}
	return golden
}

// TestDifferentialCLI compares the Go port of the changelog CLI with the frozen
// legacy oracle over the whole corpus and the whole (current, new, --notices)
// grid: exit code, stdout, and empty stderr. Inputs are pinned by a corpus
// manifest and the runs are pinned by their arguments; a missing golden fails.
func TestDifferentialCLI(t *testing.T) {
	golden := loadChangelogGolden(t)
	corpus := differentialCorpus(t)

	if len(corpus) != len(expectedChangelogGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(corpus), len(expectedChangelogGoldenCases))
	}
	if len(golden.Cases) != len(expectedChangelogGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedChangelogGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedChangelogGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || corpus[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, corpus[i].name, want)
		}
	}

	tmp := t.TempDir()
	paths := make(map[string]string, len(corpus))
	for _, entry := range corpus {
		path := filepath.Join(tmp, entry.name+".md")
		if err := os.WriteFile(path, []byte(entry.text), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		paths[entry.name] = path
	}

	gotCorpus := changelogCorpusManifest(t, tmp, corpus)
	if strings.Join(gotCorpus, "\n") != strings.Join(golden.Corpus, "\n") {
		t.Fatalf("frozen inputs differ from the corpus:\n--- frozen ---\n%s\n--- corpus ---\n%s",
			strings.Join(golden.Corpus, "\n"), strings.Join(gotCorpus, "\n"))
	}

	compared := 0
	for i, entry := range corpus {
		runs := golden.Cases[i].Runs
		if want := len(changelogPairs) * 2; len(runs) != want {
			t.Fatalf("%s: frozen run count %d, corpus grid %d", entry.name, len(runs), want)
		}
		path := paths[entry.name]
		k := 0
		for _, pair := range changelogPairs {
			for _, notices := range []bool{false, true} {
				frozen := runs[k]
				k++
				if frozen.Current != pair[0] || frozen.New != pair[1] || frozen.Notices != notices {
					t.Fatalf("%s: frozen run %d is %v notices=%v, corpus %v notices=%v",
						entry.name, k-1, [2]string{frozen.Current, frozen.New}, frozen.Notices, pair, notices)
				}
				args := []string{"changelog.py", path, pair[0], pair[1]}
				if notices {
					args = append(args, "--notices")
				}
				var out, errOut bytes.Buffer
				code := RunCLI(args, &out, &errOut)
				if code != frozen.Exit {
					t.Errorf("%s %v notices=%v: exit go=%d legacy=%d", entry.name, pair, notices, code, frozen.Exit)
				}
				if out.String() != frozen.Stdout {
					t.Errorf("%s %v notices=%v: stdout mismatch\n go:     %q\n legacy: %q", entry.name, pair, notices, out.String(), frozen.Stdout)
					continue
				}
				if errOut.Len() != 0 {
					t.Errorf("%s %v notices=%v: stderr = %q, legacy wrote none", entry.name, pair, notices, errOut.String())
				}
				compared++
			}
		}
	}

	if want := len(expectedChangelogGoldenCases) * len(changelogPairs) * 2; compared != want {
		t.Fatalf("compared %d invocations, want %d", compared, want)
	}
	t.Logf("differential changelog CLI: compared %d invocations against the frozen oracle", compared)
}
