// Package changelog parses CHANGELOG.md version sections for the upgrade
// summary and the version-keyed upgrade notices.
//
// Byte-identical port of lib/_internal/changelog.py (card [Go 12]). Two
// surfaces read the same version-keyed sections:
//
//   - the version-crossing summary ("what did I just get"), newest first;
//   - version-keyed upgrade notices ("what must I do now"), oldest first, so
//     instructions apply in release order.
//
// Every entry point degrades to empty data rather than raising: by the time
// this parser runs the upgrade has already landed, so a malformed changelog
// must never turn a successful upgrade into a failed one.
//
// Python semantics are reproduced deliberately where they differ from a
// naive Go translation: str.splitlines() line boundaries, code-point-based
// length/slicing, the lookbehind sentence split, and Unicode str.strip().
package changelog

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// "## [0.22.0] — 2026-08-17", "## [0.22.0] - 2026-08-17", or "## [0.22.0]".
// The separator may be an em dash (U+2014), an en dash (U+2013), or a plain
// hyphen; the date is optional.
var (
	headingRE = regexp.MustCompile("^##[[:space:]]+\\[([0-9]+\\.[0-9]+\\.[0-9]+)\\](?:[[:space:]]*[\u2014\u2013-][[:space:]]*(\\S+))?[[:space:]]*$")
	anyH2RE   = regexp.MustCompile("^##[[:space:]]+")
	fenceRE   = regexp.MustCompile("^[[:space:]]*(?:```|~~~)")
	noticeHRE = regexp.MustCompile(`(?i)^###[[:space:]]+Upgrade notes[[:space:]]*$`)
	anyH3RE   = regexp.MustCompile("^###[[:space:]]+")
	semverRE  = regexp.MustCompile("^([0-9]+)\\.([0-9]+)\\.([0-9]+)$")
)

// NoticeHeading is the H3 that introduces a version's upgrade instructions.
const NoticeHeading = "### Upgrade notes"

// MaxBullet caps a rendered summary bullet (Python MAX_BULLET).
const MaxBullet = 100

// Section is one released version's changelog entry.
type Section struct {
	Version string
	Date    string // "" when the heading carried no date (Python None)
	Body    string
}

// Key is the semver sort key (Python Section.key).
func (s Section) Key() (key [3]int, ok bool) { return VersionKey(s.Version) }

// VersionKey returns the semver sort key, or ok=false when version is not a
// release version. String ordering is wrong here: "0.9.0" sorts after
// "0.22.0" as text but before it as a version.
func VersionKey(version string) (key [3]int, ok bool) {
	m := semverRE.FindStringSubmatch(pyStrip(version))
	if m == nil {
		return key, false
	}
	key[0], _ = strconv.Atoi(m[1])
	key[1], _ = strconv.Atoi(m[2])
	key[2], _ = strconv.Atoi(m[3])
	return key, true
}

// ParseSections returns released sections, newest first. `[Unreleased]` and
// any other non-semver heading are skipped: they are not something a user can
// cross.
func ParseSections(text string) []Section {
	if text == "" {
		return nil
	}
	lines := pySplitLines(text)
	fenced := fencedLines(lines)

	type start struct {
		index   int
		version string
		date    string
	}
	var starts []start
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		m := headingRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		starts = append(starts, start{index: i, version: m[1], date: m[2]})
	}

	sections := make([]Section, 0, len(starts))
	for _, st := range starts {
		// The body ends at the next H2 of any kind, not merely the next
		// released one, so an "[Unreleased]" heading cannot absorb a body.
		end := len(lines)
		for cursor := st.index + 1; cursor < len(lines); cursor++ {
			if !fenced[cursor] && anyH2RE.MatchString(lines[cursor]) {
				end = cursor
				break
			}
		}
		body := strings.Trim(strings.Join(lines[st.index+1:end], "\n"), "\n")
		sections = append(sections, Section{Version: st.version, Date: st.date, Body: body})
	}

	// Stable sort by semver descending; Python's sort is stable, so duplicate
	// versions keep document order (topmost first after the sort).
	sort.SliceStable(sections, func(i, j int) bool {
		ki, _ := sections[i].Key()
		kj, _ := sections[j].Key()
		return compareKeys(ki, kj) > 0
	})

	// Collapse duplicate version headings, keeping the first (newest-first
	// order means that is the topmost occurrence).
	deduped := make([]Section, 0, len(sections))
	seen := make(map[string]bool, len(sections))
	for _, s := range sections {
		if seen[s.Version] {
			continue
		}
		seen[s.Version] = true
		deduped = append(deduped, s)
	}
	return deduped
}

// fencedLines marks which lines sit inside a fenced code block.
func fencedLines(lines []string) []bool {
	inside := false
	flags := make([]bool, len(lines))
	for i, line := range lines {
		if fenceRE.MatchString(line) {
			// The fence delimiter itself is never a heading either way.
			flags[i] = true
			inside = !inside
			continue
		}
		flags[i] = inside
	}
	return flags
}

// ReadSections parses a changelog file, returning nil when it cannot be read.
func ReadSections(path string) []Section {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !utf8.Valid(data) {
		return nil
	}
	return ParseSections(string(data))
}

// SelectRange returns the sections in the crossed range (current, new], newest
// first. An unparseable current degrades to "report the target only" rather
// than dropping the summary.
func SelectRange(sections []Section, current, new string) []Section {
	newKey, newOK := VersionKey(new)
	if !newOK {
		return nil
	}
	currentKey, currentOK := VersionKey(current)
	if !currentOK {
		var out []Section
		for _, s := range sections {
			if k, ok := s.Key(); ok && k == newKey {
				out = append(out, s)
			}
		}
		return out
	}
	if !lessKey(currentKey, newKey) { // current >= new
		return nil
	}
	var out []Section
	for _, s := range sections {
		k, ok := s.Key()
		if ok && lessKey(currentKey, k) && !lessKey(newKey, k) { // current < k <= new
			out = append(out, s)
		}
	}
	return out
}

// CrossedVersions is a convenience wrapper over ParseSections + SelectRange.
func CrossedVersions(text, current, new string) []Section {
	return SelectRange(ParseSections(text), current, new)
}

// UpgradeNotice returns the section's `### Upgrade notes` prose, or ok=false
// when absent. The notice ends at the next H3 so a following subsection cannot
// bleed in.
func UpgradeNotice(section Section) (string, bool) {
	lines := pySplitLines(section.Body)
	fenced := fencedLines(lines)

	start := -1
	for i, line := range lines {
		if !fenced[i] && noticeHRE.MatchString(line) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for cursor := start; cursor < len(lines); cursor++ {
		if !fenced[cursor] && anyH3RE.MatchString(lines[cursor]) {
			end = cursor
			break
		}
	}
	notice := pyStrip(strings.Join(lines[start:end], "\n"))
	if notice == "" {
		return "", false
	}
	return notice, true
}

// summaryBulletsAll returns every bullet under a summary-worthy subsection as
// plain text. `### Upgrade notes` is excluded: it is an instruction, not a
// change, and it is rendered separately with more prominence.
func summaryBulletsAll(section Section) []string {
	lines := pySplitLines(section.Body)
	var bullets []string
	var current []string
	inSummary := false

	flush := func() {
		if len(current) > 0 {
			bullets = append(bullets, condense(plain(strings.Join(current, " "))))
			current = nil
		}
	}
	for _, line := range lines {
		if anyH3RE.MatchString(line) {
			flush()
			inSummary = !noticeHRE.MatchString(line)
			continue
		}
		if !inSummary {
			continue
		}
		stripped := pyStrip(line)
		switch {
		case strings.HasPrefix(stripped, "- "), strings.HasPrefix(stripped, "* "):
			flush()
			current = append(current, pyStrip(stripped[2:]))
		case len(current) > 0 && stripped != "":
			// A wrapped continuation line belongs to the bullet above it.
			current = append(current, stripped)
		case stripped == "":
			flush()
		}
	}
	flush()
	out := make([]string, 0, len(bullets))
	for _, b := range bullets {
		if b != "" {
			out = append(out, b)
		}
	}
	return out
}

// SummaryBullets returns up to limit plain-text bullets describing what
// changed.
func SummaryBullets(section Section, limit int) []string {
	all := summaryBulletsAll(section)
	if len(all) > limit {
		return all[:limit]
	}
	return all
}

// RemainingCount reports how many bullets SummaryBullets dropped.
func RemainingCount(section Section, limit int) int {
	n := len(summaryBulletsAll(section)) - limit
	if n < 0 {
		return 0
	}
	return n
}

// plain strips the inline markup that reads as noise in a terminal.
func plain(text string) string {
	text = boldRE.ReplaceAllString(text, "$1")
	text = codeRE.ReplaceAllString(text, "$1")
	return pyStrip(collapseSpace(text))
}

var (
	boldRE = regexp.MustCompile(`\*\*(.+?)\*\*`)
	codeRE = regexp.MustCompile("`([^`]+)`")
)

// condense reduces a changelog bullet to one scannable line.
func condense(text string) string {
	first := pyStrip(firstSentence(text))
	if first != "" && utf8.RuneCountInString(first) <= MaxBullet {
		return first
	}
	candidate := first
	if candidate == "" {
		candidate = text
	}
	if utf8.RuneCountInString(candidate) <= MaxBullet {
		return candidate
	}
	clipped := runeClip(candidate, MaxBullet-1)
	if idx := strings.LastIndex(clipped, " "); idx >= 0 {
		clipped = clipped[:idx]
	}
	return strings.TrimRight(clipped, " ,;:.") + "\u2026"
}

// firstSentence reproduces re.split(r"(?<=[.!?])\s+", text, maxsplit=1)[0]:
// the text up to and including the first sentence terminator that is followed
// immediately by whitespace, or the whole text when there is none.
func firstSentence(text string) string {
	for i, r := range text {
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		after := i + utf8.RuneLen(r)
		if after >= len(text) {
			continue
		}
		next, _ := utf8.DecodeRuneInString(text[after:])
		if isPySpace(next) {
			return text[:after]
		}
	}
	return text
}

// runeClip returns the first n code points of s (Python slicing semantics).
func runeClip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// collapseSpace reproduces re.sub(r"\s+", " ", text) with Python's Unicode
// whitespace set.
func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	for _, r := range s {
		if isPySpace(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// pyStrip reproduces Python str.strip() (no arguments): trim characters where
// str.isspace() is true, which is a superset of Go's unicode.IsSpace.
func pyStrip(s string) string {
	return strings.TrimFunc(s, isPySpace)
}

// isPySpace reports whether r is whitespace per Python str.isspace().
func isPySpace(r rune) bool {
	if r >= 0x1c && r <= 0x1f { // FILE/GROUP/RECORD/UNIT SEPARATOR
		return true
	}
	return unicode.IsSpace(r)
}

// pySplitLines reproduces str.splitlines(): the Python superset of universal
// newlines, and no trailing empty line when the text ends with a boundary.
func pySplitLines(s string) []string {
	var lines []string
	var cur []rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !isPyLineBoundary(r) {
			cur = append(cur, r)
			continue
		}
		lines = append(lines, string(cur))
		cur = cur[:0]
		if r == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
			i++
		}
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return lines
}

func isPyLineBoundary(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

func lessKey(a, b [3]int) bool { return compareKeys(a, b) < 0 }

// compareKeys returns -1, 0 or 1 for the semver ordering of two keys
// (Python tuple comparison).
func compareKeys(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// NoticePair is one (version, notice) pair.
type NoticePair struct {
	Version string
	Notice  string
}

// noticesFor returns (version, notice) pairs, oldest first, for already
// selected sections.
func noticesFor(sections []Section) []NoticePair {
	var pairs []NoticePair
	for i := len(sections) - 1; i >= 0; i-- {
		if notice, ok := UpgradeNotice(sections[i]); ok {
			pairs = append(pairs, NoticePair{Version: sections[i].Version, Notice: notice})
		}
	}
	return pairs
}

// CrossedNotices returns (version, notice) for each crossed version declaring
// one, oldest first. Oldest first is deliberate and differs from the summary
// order: notices are instructions, and instructions apply in release order.
func CrossedNotices(text, current, new string) []NoticePair {
	return noticesFor(CrossedVersions(text, current, new))
}

// EmitSummary renders the version-crossing summary (Python _emit_summary).
func EmitSummary(w io.Writer, sections []Section, limit int) {
	for _, section := range sections {
		header := section.Version
		if section.Date != "" {
			header = header + " \u2014 " + section.Date
		}
		fmt.Fprintf(w, "  %s\n", header)
		for _, bullet := range SummaryBullets(section, limit) {
			fmt.Fprintf(w, "    \u00b7 %s\n", bullet)
		}
		if dropped := RemainingCount(section, limit); dropped > 0 {
			fmt.Fprintf(w, "    \u00b7 and %d more\n", dropped)
		}
	}
}

// EmitNotices renders the version-keyed upgrade notices (Python _emit_notices).
func EmitNotices(w io.Writer, pairs []NoticePair) {
	for _, pair := range pairs {
		fmt.Fprintf(w, "  %s\n", pair.Version)
		for _, line := range pySplitLines(pair.Notice) {
			if pyStrip(line) != "" {
				fmt.Fprintf(w, "  %s\n", line)
			} else {
				fmt.Fprintln(w, "")
			}
		}
	}
}

const usage = "usage: changelog.py <changelog-path> <current> <new> [--notices]"

// RunCLI reproduces the Python module's __main__ shim: argv[0] is the program
// name. It prints nothing and exits 0 when there is nothing to report, so the
// caller can pipe unconditionally.
func RunCLI(argv []string, stdout, stderr io.Writer) int {
	if len(argv) < 4 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	path, current, new := argv[1], argv[2], argv[3]
	wantNotices := false
	for _, a := range argv[4:] {
		if a == "--notices" {
			wantNotices = true
		}
	}

	sections := ReadSections(path)
	if len(sections) == 0 {
		return 0
	}
	selected := SelectRange(sections, current, new)
	if len(selected) == 0 {
		return 0
	}
	if wantNotices {
		EmitNotices(stdout, noticesFor(selected))
	} else {
		EmitSummary(stdout, selected, 3)
	}
	return 0
}
