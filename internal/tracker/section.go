// Package tracker is the native port of the two ledger_bridge verbs the
// tracker checkpoint host calls (GO-20 U2e, U-C): `slug` and `evidence`. It is
// acquisition only: it reads the planning tree and never grades. The ## Tracker
// validity rule stays with the gate binary's --ledger predicate.
package tracker

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// pairRE mirrors trello_link._PAIR_RE: an optional list marker, bold markers,
// a key, and the value after the colon.
var pairRE = regexp.MustCompile(`^\s*(?:[-*]\s+)?\*{0,2}([A-Za-z_][A-Za-z0-9_]*)\*{0,2}\s*:\s*(.*)$`)

var (
	trackerHeadingRE = regexp.MustCompile(`^##\s+Tracker\s*$`)
	h2RE             = regexp.MustCompile(`^##\s+`)
)

// recognizedKeys mirrors trello_link._RECOGNIZED_KEYS.
var recognizedKeys = map[string]bool{"card_id": true, "shortlink": true, "url": true, "list": true, "pr": true}

// splitLines mirrors Python str.splitlines(): it splits on \n, \r, \r\n, \v, \f,
// \x1c-\x1e, \x85, \u2028 and \u2029, and drops the terminators.
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		next := i + size
		switch r {
		case '\r':
			if next < len(text) && text[next] == '\n' {
				next++
			}
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
		default:
			i = next
			continue
		}
		lines = append(lines, text[start:i])
		start = next
		i = next
	}
	if start < len(text) {
		lines = append(lines, text[start:])
	}
	return lines
}

// fenceState tracks Markdown fences the way trello_link does: a fence opens on a
// line starting with ``` or ~~~ and closes on a line starting with the same marker.
type fenceState struct {
	in     bool
	marker string
}

// step reports whether line is a fence line; it updates the state when it is.
func (f *fenceState) step(stripped string) bool {
	if !strings.HasPrefix(stripped, "```") && !strings.HasPrefix(stripped, "~~~") {
		return false
	}
	marker := stripped[:3]
	if !f.in {
		f.in = true
		f.marker = marker
	} else if strings.HasPrefix(stripped, f.marker) {
		f.in = false
		f.marker = ""
	}
	return true
}

// extractTrackerBody mirrors trello_link._extract_tracker_body: the lines after the
// first ## Tracker heading (outside fences) up to the next ## heading (outside fences).
func extractTrackerBody(text string) (string, bool) {
	lines := splitLines(text)
	start := -1
	var fence fenceState
	for i, line := range lines {
		stripped := strings.TrimLeftFunc(line, unicode.IsSpace)
		if fence.step(stripped) {
			continue
		}
		if fence.in {
			continue
		}
		if trackerHeadingRE.MatchString(line) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	var body []string
	fence = fenceState{}
	for _, line := range lines[start:] {
		stripped := strings.TrimLeftFunc(line, unicode.IsSpace)
		if fence.step(stripped) {
			body = append(body, line)
			continue
		}
		if !fence.in && h2RE.MatchString(line) {
			break
		}
		body = append(body, line)
	}
	return strings.Join(body, "\n"), true
}

// cleanValue mirrors trello_link._clean_value: strip surrounding backticks and
// whitespace, and a trailing " #comment" outside backticks.
func cleanValue(raw string) string {
	value := strings.TrimSpace(raw)
	if at := strings.Index(value, " #"); at != -1 {
		before := value[:at]
		if strings.Count(before, "`")%2 == 0 {
			value = strings.TrimSpace(before)
		}
	}
	if len(value) >= 2 && strings.HasPrefix(value, "`") && strings.HasSuffix(value, "`") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

// parseSectionBody mirrors trello_link._parse_section_body: recognized keys only,
// first occurrence wins, keys lowercased, fenced lines and comments skipped.
func parseSectionBody(body string) map[string]string {
	out := map[string]string{}
	var fence fenceState
	for _, line := range splitLines(body) {
		stripped := strings.TrimLeftFunc(line, unicode.IsSpace)
		if fence.step(stripped) {
			continue
		}
		if fence.in || strings.TrimSpace(line) == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		m := pairRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToLower(m[1])
		if !recognizedKeys[key] {
			continue
		}
		if _, seen := out[key]; seen {
			continue
		}
		out[key] = cleanValue(m[2])
	}
	return out
}

// ParseSection mirrors trello_link.parse_tracker_section: the first path that is
// a readable regular file containing a ## Tracker section wins. Missing or
// unreadable files are skipped; invalid UTF-8 is an error, as in Python.
func ParseSection(paths []string) (map[string]string, error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !utf8.Valid(raw) {
			return nil, fmt.Errorf("%s: invalid UTF-8", path)
		}
		body, ok := extractTrackerBody(string(raw))
		if !ok {
			continue
		}
		return parseSectionBody(body), nil
	}
	return map[string]string{}, nil
}
