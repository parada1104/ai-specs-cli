// Package skills is the native port of the skill-resolution and skill_contract
// Python surface consumed by `ai-specs rules-audit` (and reusable by `sync`):
// YAML frontmatter parsing plus multi-source skill resolution with the
// precedence local > recipe > dep > bundled.
//
// The port is byte-faithful to:
//
//	lib/_internal/skill_contract.py  — split_frontmatter, parse_frontmatter,
//	                                    _strip_quotes, _split_inline_list,
//	                                    SkillContractError
//	lib/_internal/skill-resolution.py — collect_skills and its scanners
//	lib/_internal/flatten-resolved-skills.py — Flatten
//
// Cache roots (cache_key / cache_root / *_skills_root) come from
// internal/projectcache, the single owner of the frozen cache key.
package skills

import "strings"

// SkillContractError mirrors skill_contract.SkillContractError. Its Error()
// string matches the Python f"{location}{field}: {message}" shape.
type SkillContractError struct {
	Field   string
	Message string
	// Path is the file the error refers to; empty when the Python caller
	// passed path=None.
	Path string
}

func (e *SkillContractError) Error() string {
	location := ""
	if e.Path != "" {
		location = e.Path + ": "
	}
	return location + e.Field + ": " + e.Message
}

// SplitLines mirrors Python str.splitlines() for every line boundary CPython
// recognises (the str/Unicode variant, not bytes). A trailing break yields no
// extra empty element.
func SplitLines(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			out = append(out, string(runes[start:i]))
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			start = i + 1
			continue
		}
		switch r {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\x85', '\u2028', '\u2029':
			out = append(out, string(runes[start:i]))
			start = i + 1
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

// SplitFrontmatter mirrors skill_contract.split_frontmatter.
func SplitFrontmatter(text string) (string, string) {
	if !strings.HasPrefix(text, "---") {
		return "", text
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", text
	}
	frontmatter := strings.TrimLeft(rest[:end], "\n")
	body := rest[end+4:]
	if strings.HasPrefix(body, "\n") {
		body = body[1:]
	}
	return frontmatter, body
}

// StripQuotes mirrors skill_contract._strip_quotes.
func StripQuotes(value string) string {
	v := strings.TrimSpace(value)
	if len(v) >= 2 && v[0] == v[len(v)-1] && (v[0] == '"' || v[0] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// SplitInlineList mirrors skill_contract._split_inline_list: it strips one
// outer pair of brackets and splits the remainder on commas.
func SplitInlineList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 {
		return nil
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(inner, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, StripQuotes(strings.TrimSpace(part)))
	}
	return out
}

var blockScalars = map[string]bool{">": true, "|": true, ">-": true, "|-": true}

// ParseFrontmatter mirrors skill_contract.parse_frontmatter. Values are
// string, []string, or map[string]any (the metadata block).
func ParseFrontmatter(frontmatter string) (map[string]any, error) {
	data := map[string]any{}
	lines := SplitLines(frontmatter)
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, " ") {
			return nil, &SkillContractError{Field: "frontmatter", Message: "unexpected indentation"}
		}
		if !strings.HasSuffix(line, ":") && !strings.Contains(line, ":") {
			return nil, &SkillContractError{Field: "frontmatter", Message: "unsupported line: " + line}
		}
		key, rest, _ := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		rest = strings.TrimSpace(rest)

		if key == "metadata" {
			metadata := map[string]any{}
			i++
			metadataIndent := -1
			for i < len(lines) {
				nested := lines[i]
				if strings.TrimSpace(nested) == "" {
					i++
					continue
				}
				indent := leadingSpaces(nested)
				if indent == 0 {
					break
				}
				if metadataIndent == -1 {
					metadataIndent = indent
				}
				if indent != metadataIndent {
					return nil, &SkillContractError{Field: "metadata", Message: "unsupported indentation: " + nested}
				}
				subkey, subrest, _ := strings.Cut(strings.TrimSpace(nested), ":")
				subrest = strings.TrimSpace(subrest)
				i++
				childIndent := metadataIndent + 2
				if n := nextContentIndent(lines, i); n != nil && *n != 0 {
					childIndent = *n
				}
				var value any
				switch {
				case blockScalars[subrest]:
					value, i = consumeBlock(lines, i, childIndent)
				case strings.HasPrefix(subrest, "[") && strings.HasSuffix(subrest, "]"):
					value = SplitInlineList(subrest)
				case subrest != "":
					value = StripQuotes(subrest)
				default:
					value, i = consumeList(lines, i, childIndent)
					if vs, ok := value.([]string); !ok || len(vs) == 0 {
						value, i = consumeBlock(lines, i, childIndent)
					}
				}
				metadata[subkey] = value
			}
			data[key] = metadata
			continue
		}

		i++
		childIndent := 1
		if n := nextContentIndent(lines, i); n != nil && *n != 0 {
			childIndent = *n
		}
		var value any
		switch {
		case blockScalars[rest]:
			value, i = consumeBlock(lines, i, childIndent)
		case strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]"):
			value = SplitInlineList(rest)
		default:
			value = StripQuotes(rest)
		}
		data[key] = value
	}
	return data, nil
}

// leadingSpaces mirrors len(line) - len(line.lstrip(" ")): ASCII spaces only.
func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// nextContentIndent mirrors _next_content_indent: indentation of the first
// non-blank line at or after start, or nil when none remains.
func nextContentIndent(lines []string, start int) *int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			n := leadingSpaces(lines[i])
			return &n
		}
	}
	return nil
}

// consumeBlock mirrors _consume_block.
func consumeBlock(lines []string, start, indent int) (string, int) {
	var parts []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			parts = append(parts, "")
			i++
			continue
		}
		if leadingSpaces(line) < indent {
			break
		}
		parts = append(parts, line[indent:])
		i++
	}
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, strings.TrimSpace(p))
		}
	}
	return strings.TrimSpace(strings.Join(kept, " ")), i
}

// consumeList mirrors _consume_list.
func consumeList(lines []string, start, indent int) ([]string, int) {
	var items []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if leadingSpaces(line) < indent {
			break
		}
		stripped := line[indent:]
		if !strings.HasPrefix(stripped, "- ") {
			break
		}
		items = append(items, StripQuotes(strings.TrimSpace(stripped[2:])))
		i++
	}
	return items, i
}
