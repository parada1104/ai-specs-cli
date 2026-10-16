package config

import (
	"strings"

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
			inAgents = stripped == "[agents]"
			agentsSeen = agentsSeen || inAgents
		} else if inAgents && !enabledWritten && strings.HasPrefix(stripped, "enabled") {
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

	return atomicWrite(manifestPath, strings.Join(newLines, ""))
}

// insertEnabledLine appends the rendered `enabled = ...` line, forcing a
// newline onto an unterminated previous line first.
func insertEnabledLine(lines *[]string, value string) {
	if n := len(*lines); n > 0 && !strings.HasSuffix((*lines)[n-1], "\n") {
		(*lines)[n-1] += "\n"
	}
	*lines = append(*lines, "enabled = "+value+"\n")
}
