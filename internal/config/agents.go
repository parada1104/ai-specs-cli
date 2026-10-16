package config

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/toml"
)

// EnabledAgents is toml-read.read_agents(...).get("enabled", []): the strings of
// the manifest's [agents].enabled list, in manifest order. A missing manifest, a
// missing [agents] table and a missing, blank or mistyped key all read as an
// empty list — which is the state the hub's Agents picker starts from.
func EnabledAgents(manifestPath string) []string {
	data, err := LoadManifest(manifestPath)
	if err != nil {
		return []string{}
	}
	return normalizedStringList(tableOr(data, "agents"), "enabled")
}

// SetEnabledAgents is a byte-faithful port of hub.py's _run_agents_submenu
// write. It rewrites the [agents].enabled list in place through the raw line
// editor — not a TOML re-serialization — so every other byte of the manifest,
// comments included, survives.
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
	newLines := make([]string, 0, len(lines)+2)
	inAgents := false
	agentsSeen := false
	enabledWritten := false
	for _, line := range lines {
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
// `[agents]  ` all name the same table; `[agents.foo]` and `[[deps]]` do not.
func tableName(stripped string) string {
	s := strings.TrimPrefix(stripped, "[")
	if i := strings.IndexAny(s, "] \t"); i >= 0 {
		s = s[:i]
	}
	return s
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
