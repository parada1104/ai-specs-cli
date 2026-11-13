package gates

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	fenceOpeningRe = regexp.MustCompile("^[ ]{0,3}(`{3,}|~{3,})")
	indentedCodeRe = regexp.MustCompile(`^ {4,}|^\t`)
)

// markdownContentLines drops fenced and indented code so a code block cannot
// forge evidence or criteria.
func markdownContentLines(lines []string) []string {
	content := make([]string, 0, len(lines))
	fenceMarker := ""
	fenceWidth := 0
	var fenceClose *regexp.Regexp
	for _, line := range lines {
		if fenceMarker != "" {
			if fenceClose.MatchString(line) {
				fenceMarker, fenceWidth, fenceClose = "", 0, nil
			}
			continue
		}
		if opening := fenceOpeningRe.FindStringSubmatch(line); opening != nil {
			token := opening[1]
			fenceMarker, fenceWidth = token[:1], len(token)
			fenceClose = regexp.MustCompile(fmt.Sprintf(`^[ ]{0,3}%s{%d,}[ \t]*$`,
				regexp.QuoteMeta(fenceMarker), fenceWidth))
			continue
		}
		if indentedCodeRe.MatchString(line) {
			continue
		}
		content = append(content, line)
	}
	return content
}

// splitLines mirrors Python's splitlines() for the line endings a planning
// artifact can carry.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// readText reads a planning artifact; a missing or unreadable file reads as
// empty, like the Python guardian's guarded reads.
func readText(path string) string {
	if !isFile(path) {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}
