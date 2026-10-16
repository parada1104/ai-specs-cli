package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// descriptionCap is hub.py's desc[:80]. Python slices CODE POINTS, so the port
// truncates runes rather than bytes: a byte slice would cut a multi-byte
// character in half and print a replacement glyph in the picker.
const descriptionCap = 80

// SKILL.md's YAML front matter, then the description key inside it. The patterns
// are hub.py's own, kept verbatim so an edge case the regex decides (an unclosed
// block, a value that is not quoted) is decided the same way here.
var (
	frontmatterPattern = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---`)
	descriptionPattern = regexp.MustCompile(`(?m)^description:\s*(.+?)\s*$`)
)

// Description returns the SKILL.md front-matter description of a skill
// DIRECTORY, truncated to 80 code points, or "" when there is nothing to read —
// no SKILL.md, no front matter, no description key. It mirrors hub.py's
// _skill_description, which itself mirrors skills-list.sh, so the hub, `skills
// list` and the shell all show the same sentence.
func Description(skillDir string) string {
	raw, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	block := frontmatterPattern.FindSubmatch(raw)
	if block == nil {
		return ""
	}
	match := descriptionPattern.FindSubmatch(block[1])
	if match == nil {
		return ""
	}
	return truncateRunes(unquoteDescription(strings.TrimSpace(string(match[1]))))
}

// unquoteDescription strips one layer of matching quotes and undoes the escapes
// Python's replace chain undoes, in the same order: \" first, then \\.
func unquoteDescription(desc string) string {
	switch {
	case strings.HasPrefix(desc, `"`) && strings.HasSuffix(desc, `"`):
		inner := ""
		if len(desc) > 1 {
			inner = desc[1 : len(desc)-1]
		}
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		return strings.ReplaceAll(inner, `\\`, `\`)
	case strings.HasPrefix(desc, "'") && strings.HasSuffix(desc, "'"):
		if len(desc) <= 1 {
			return ""
		}
		return desc[1 : len(desc)-1]
	default:
		return desc
	}
}

// truncateRunes is Python's desc[:80].
func truncateRunes(desc string) string {
	runes := []rune(desc)
	if len(runes) <= descriptionCap {
		return desc
	}
	return string(runes[:descriptionCap])
}
