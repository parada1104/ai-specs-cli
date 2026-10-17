package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/toml"
)

// EnabledAgents is toml-read.read_agents(...).get("enabled", []): the strings of
// the manifest's [agents].enabled list, in manifest order. A missing manifest, a
// missing [agents] table and a missing, blank or mistyped key all read as an
// empty list — which is the state the hub's Agents picker starts from.
//
// A manifest that exists but cannot be READ or PARSED is different, and says so: an
// empty picker over a broken manifest would persist that emptiness over the user's
// real list (odd/tasks/go-20-u3b-advisories.md S6).
func EnabledAgents(manifestPath string) ([]string, error) {
	data, err := LoadManifest(manifestPath)
	if err != nil {
		var missing *ManifestNotFoundError
		if errors.As(err, &missing) {
			return []string{}, nil
		}
		return nil, err
	}
	return normalizedStringList(tableOr(data, "agents"), "enabled"), nil
}

// SetEnabledAgents is a byte-faithful port of hub.py's _run_agents_submenu
// write. It rewrites the [agents].enabled list in place through the raw line
// editor — not a TOML re-serialization — so every other byte of the manifest,
// comments included, survives. Two measured qualifications on "every byte": line
// endings are normalized to LF first (readManifestText's documented parity rule, so a
// CRLF manifest is rewritten throughout), and a line inside a multi-line """ or '''
// string is content, never structure.
//
// D12: it NEVER appends a second [agents] block. When the table already exists,
// the key is inserted inside it (before the next header, or at EOF) or replaces
// the existing `enabled` line; only a manifest with no [agents] table at all
// gets one appended.
func SetEnabledAgents(manifestPath string, agents []string) error {
	value, err := toml.TOMLValue(agents)
	if err != nil {
		return err
	}
	original, err := readManifestText(manifestPath)
	if err != nil {
		return err
	}
	// The line editor splits through []rune, so an invalid byte would become
	// U+FFFD and the corruption would be persisted. Refuse instead, leaving the
	// file byte-identical.
	if !utf8.ValidString(original) {
		return fmt.Errorf("refusing to rewrite %s: contents are not valid UTF-8", manifestPath)
	}

	lines := splitLinesKeepEnds(original)
	content := lineStartsInsideString(original)
	newLines := make([]string, 0, len(lines)+2)
	inAgents := false
	agentsSeen := false
	enabledWritten := false
	for i, line := range lines {
		// A line that begins inside a multi-line string is string CONTENT, never
		// a header and never the `enabled` key: treating it as structural writes
		// the new value inside the user's string and leaves the real table
		// untouched, which is silent corruption reporting success
		// (odd/tasks/go-20-u3b-advisories.md S4).
		if content[i] {
			newLines = append(newLines, line)
			continue
		}
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "[") {
			if inAgents && !enabledWritten {
				// [agents] had no `enabled` key: insert it inside the table,
				// before the next header.
				insertEnabledLine(&newLines, value)
				enabledWritten = true
			}
			// Match the table NAME, not the exact string: `[agents] # note` is
			// the same table, and appending a second block would make the
			// manifest invalid.
			inAgents = tableName(stripped) == "agents"
			agentsSeen = agentsSeen || inAgents
		} else if inAgents && !enabledWritten && isEnabledKey(stripped) {
			newLines = append(newLines, "enabled = "+value+"\n")
			enabledWritten = true
			continue
		}
		newLines = append(newLines, line)
	}

	if inAgents && !enabledWritten {
		// [agents] is the last table and has no `enabled` key.
		insertEnabledLine(&newLines, value)
		enabledWritten = true
	}

	if !agentsSeen {
		if len(newLines) > 0 && !strings.HasSuffix(newLines[len(newLines)-1], "\n") {
			newLines[len(newLines)-1] += "\n"
		}
		newLines = append(newLines, "\n[agents]\nenabled = "+value+"\n")
	}

	newContent := strings.Join(newLines, "")
	if err := guardValidToInvalid(original, newContent,
		"ERROR: updating agents would produce invalid TOML: "); err != nil {
		return err
	}
	return atomicWrite(manifestPath, newContent)
}

// insertEnabledLine appends the rendered `enabled = ...` line, forcing a
// newline onto an unterminated previous line first.
func insertEnabledLine(lines *[]string, value string) {
	if n := len(*lines); n > 0 && !strings.HasSuffix((*lines)[n-1], "\n") {
		(*lines)[n-1] += "\n"
	}
	*lines = append(*lines, "enabled = "+value+"\n")
}

// tableName is the NAME of a TOML table header line: the text between the first
// '[' and the first ']' or whitespace. `[agents]`, `[agents] # note` and
// `[agents]  ` all name the same table. An array of tables (`[[agents]]`,
// `[[deps]]`) is not a table header at all, so it names nothing.
func tableName(stripped string) string {
	name, ok := strings.CutPrefix(stripped, "[")
	if !ok || strings.HasPrefix(name, "[") {
		return ""
	}
	if i := strings.IndexAny(name, "] \t"); i >= 0 {
		name = name[:i]
	}
	return name
}

// lineStartsInsideString reports, per line, whether that line BEGINS inside a TOML
// multi-line string (`"""` or `'''`). Only multi-line strings can carry a line that
// looks structural, so only they need tracking; single-line strings and comments are
// consumed within their own line.
func lineStartsInsideString(text string) []bool {
	lines := splitLinesKeepEnds(text)
	starts := make([]bool, len(lines))
	in := multiLineNone
	for i, line := range lines {
		starts[i] = in != multiLineNone
		in = scanStringState(line, in)
	}
	return starts
}

type multiLine int

const (
	multiLineNone    multiLine = iota
	multiLineBasic             // """ ... """
	multiLineLiteral           // ''' ... '''
)

// scanStringState walks one line and returns the multi-line string state at its end:
// OUTSIDE, INSIDE a basic string, or INSIDE a literal one. Comment text ends a line's
// scan because a `#` outside a string runs to the end of the line, and a backslash
// escapes the next rune inside a basic string, so `\"\"\"` does not close it.
func scanStringState(line string, in multiLine) multiLine {
	rs := []rune(line)
	for i := 0; i < len(rs); {
		switch in {
		case multiLineNone:
			switch rs[i] {
			case '#':
				return multiLineNone
			case '"':
				if runLength(rs, i, '"') >= 3 {
					in, i = multiLineBasic, i+3
					continue
				}
				i = skipBasicString(rs, i)
			case '\'':
				if runLength(rs, i, '\'') >= 3 {
					in, i = multiLineLiteral, i+3
					continue
				}
				i = skipLiteralString(rs, i)
			default:
				i++
			}
		case multiLineBasic:
			if rs[i] == '\\' {
				i += 2
				continue
			}
			if rs[i] == '"' {
				// TOML allows up to two quotes before the closing delimiter, so a
				// run of three closes the string and the run length is its width.
				if n := runLength(rs, i, '"'); n >= 3 {
					in, i = multiLineNone, i+n
					continue
				}
				i += runLength(rs, i, '"')
				continue
			}
			i++
		case multiLineLiteral:
			if rs[i] == '\'' {
				if n := runLength(rs, i, '\''); n >= 3 {
					in, i = multiLineNone, i+n
					continue
				}
				i += runLength(rs, i, '\'')
				continue
			}
			i++
		}
	}
	return in
}

// runLength counts the runes equal to c starting at i.
func runLength(rs []rune, i int, c rune) int {
	n := 0
	for i+n < len(rs) && rs[i+n] == c {
		n++
	}
	return n
}

// skipBasicString returns the index just past a single-line basic string starting at
// the quote at i, honouring backslash escapes.
func skipBasicString(rs []rune, i int) int {
	for i++; i < len(rs); {
		switch rs[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return len(rs)
}

// skipLiteralString returns the index just past a single-line literal string, which
// carries no escapes at all.
func skipLiteralString(rs []rune, i int) int {
	for i++; i < len(rs); i++ {
		if rs[i] == '\'' {
			return i + 1
		}
	}
	return len(rs)
}

// isEnabledKey reports whether a non-header line assigns the `enabled` key: the
// text before '=' must be exactly "enabled". A prefix match would also swallow
// `enabled_extra`, silently dropping that key when the line is replaced.
func isEnabledKey(stripped string) bool {
	eq := strings.IndexByte(stripped, '=')
	if eq < 0 {
		return false
	}
	return strings.TrimSpace(stripped[:eq]) == "enabled"
}
