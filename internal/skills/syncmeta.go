package skills

// Native validation of one local SKILL.md's sync metadata (GO-20 U2e, unit
// U-D): the port of skill_contract.py's from_local_skill +
// normalize_local_skill + validate_sync_metadata + _cli_sync_metadata, behind
// `ai-specs skills validate <skill_md_path>` (the call
// bundled-skills/skill-sync/assets/sync.sh:15 makes).
//
// Faithful to the oracle: compatibility normalization (True) with its
// defaults and warnings in Python's order, the Python str(Path(arg)) error
// path for normalization and the RAW argv string for validation errors, and
// the exact json.dumps default-style line (", "/": " separators,
// ensure_ascii=True). json.dumps is NOT encoding/json here: that encoder
// HTML-escapes <, >, & and words \b/\f differently.
//
// Documented divergences (pinned by tests):
//   - invalid UTF-8: Python dies with a UnicodeDecodeError traceback; the verb
//     prints one `ERROR: …` line on stderr and exits 1 (same class as the
//     other U2e ports).
//   - a missing YAML frontmatter (and a frontmatter parse error) carries NO
//     path in the JSON: Python raises inside read_skill_text, before
//     from_local_skill's own path is attached, so "path" is null.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/config"
)

// LocalSkill is the normalize_local_skill subset the sync-metadata verb
// consumes.
type LocalSkill struct {
	Name       string
	Scope      []string
	AutoInvoke []string
	Warnings   []string
}

// SyncMetadata is validate_sync_metadata's payload plus the normalization
// warnings the CLI appends to it.
type SyncMetadata struct {
	Enabled    bool
	Name       string
	Scope      []string
	AutoInvoke []string
	Warnings   []string
}

// FromLocalSkillFile mirrors from_local_skill(path, compatibility=True):
// universal-newline read, frontmatter split, and compatibility normalization.
// Normalization errors carry filepath.Clean(path) (Python's str(Path(path))).
func FromLocalSkillFile(path string) (LocalSkill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalSkill{}, err
	}
	if !utf8.Valid(data) {
		return LocalSkill{}, fmt.Errorf("invalid UTF-8 in %s", path)
	}
	frontmatter, _ := SplitFrontmatter(universalNewlines(string(data)))
	if frontmatter == "" {
		// Raised inside read_skill_text, before the path is attached.
		return LocalSkill{}, &SkillContractError{Field: "frontmatter", Message: "missing YAML frontmatter"}
	}
	raw, err := ParseFrontmatter(frontmatter)
	if err != nil {
		return LocalSkill{}, err
	}
	return normalizeLocalSkill(raw, filepath.Clean(path))
}

// universalNewlines is Python's newline=None text read: "\r\n" and lone "\r"
// both become "\n".
func universalNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// normalizeLocalSkill mirrors normalize_local_skill(raw, body, path,
// compatibility=True); body is not part of the sync-metadata payload.
func normalizeLocalSkill(raw map[string]any, path string) (LocalSkill, error) {
	var warnings []string
	metadata := raw["metadata"]
	if metadata != nil && config.Truthy(metadata) {
		if _, ok := metadata.(map[string]any); !ok {
			return LocalSkill{}, &SkillContractError{Field: "metadata", Message: "must be a mapping", Path: path}
		}
	}
	meta, _ := metadata.(map[string]any)

	name, err := requireStringWarn(raw["name"], "name", "", "", path, &warnings)
	if err != nil {
		return LocalSkill{}, err
	}
	if !nameRE.MatchString(name) {
		return LocalSkill{}, &SkillContractError{Field: "name", Message: "must be lowercase kebab-case", Path: path}
	}
	if _, err := requireStringWarn(raw["description"], "description", "", "", path, &warnings); err != nil {
		return LocalSkill{}, err
	}
	if _, err := requireStringWarn(raw["license"], "license", "Apache-2.0", "using compatibility default `Apache-2.0`", path, &warnings); err != nil {
		return LocalSkill{}, err
	}
	if _, err := requireStringWarn(meta["author"], "metadata.author", "unknown", "using compatibility default `unknown` author", path, &warnings); err != nil {
		return LocalSkill{}, err
	}
	version, err := requireStringWarn(meta["version"], "metadata.version", "1.0", "using compatibility default `1.0` version", path, &warnings)
	if err != nil {
		return LocalSkill{}, err
	}
	if !versionRE.MatchString(version) {
		return LocalSkill{}, &SkillContractError{Field: "metadata.version", Message: "must look like a semantic version (for example `1.0` or `1.0.0`)", Path: path}
	}
	scope, err := normalizeStringListWarn(meta["scope"], "metadata.scope", path, &warnings)
	if err != nil {
		return LocalSkill{}, err
	}
	autoInvoke, err := normalizeStringListWarn(meta["auto_invoke"], "metadata.auto_invoke", path, &warnings)
	if err != nil {
		return LocalSkill{}, err
	}
	return LocalSkill{Name: name, Scope: scope, AutoInvoke: autoInvoke, Warnings: warnings}, nil
}

// requireStringWarn mirrors _require_string WITH a compatibility default: an
// absent/blank value yields def (and its warning), a non-string value is an
// error, and anything else is stripped.
func requireStringWarn(v any, field, def, label, path string, warnings *[]string) (string, error) {
	s, isStr := v.(string)
	if v == nil || (isStr && pyStrip(s) == "") {
		if def != "" {
			if label != "" {
				*warnings = append(*warnings, label+"; add `"+field+"` explicitly")
			}
			return def, nil
		}
		return "", &SkillContractError{Field: field, Message: "is required", Path: path}
	}
	if !isStr {
		return "", &SkillContractError{Field: field, Message: "must be a string", Path: path}
	}
	return pyStrip(s), nil
}

// normalizeStringListWarn mirrors _normalize_string_list(compatibility=True):
// a scalar becomes a warned one-item list, blank/absent values become [], and
// a non-list is refused.
func normalizeStringListWarn(v any, field, path string, warnings *[]string) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	var items []any
	switch x := v.(type) {
	case string:
		if pyStrip(x) == "" {
			return nil, nil
		}
		*warnings = append(*warnings, "converted scalar `"+field+"` to a one-item list")
		return []string{pyStrip(x)}, nil
	case []string:
		for _, s := range x {
			items = append(items, s)
		}
	case []any:
		items = x
	default:
		return nil, &SkillContractError{Field: field, Message: "must be a YAML list of strings", Path: path}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, &SkillContractError{Field: field, Message: "must contain only strings", Path: path}
		}
		if s = pyStrip(s); s == "" {
			return nil, &SkillContractError{Field: field, Message: "must not contain empty items", Path: path}
		}
		out = append(out, s)
	}
	return out, nil
}

// ValidateSyncMetadata mirrors validate_sync_metadata(skill, path=path_arg):
// its errors keep the RAW argv string (Python passes path_arg unchanged).
func ValidateSyncMetadata(skill LocalSkill, rawPath string) (SyncMetadata, error) {
	payload := SyncMetadata{Name: skill.Name, Warnings: skill.Warnings}
	if len(skill.Scope) == 0 && len(skill.AutoInvoke) == 0 {
		payload.Scope = []string{}
		payload.AutoInvoke = []string{}
		return payload, nil
	}
	if len(skill.Scope) == 0 {
		return SyncMetadata{}, &SkillContractError{Field: "metadata.scope", Message: "is required when metadata.auto_invoke is set", Path: rawPath}
	}
	if len(skill.AutoInvoke) == 0 {
		return SyncMetadata{}, &SkillContractError{Field: "metadata.auto_invoke", Message: "is required when metadata.scope is set", Path: rawPath}
	}
	payload.Enabled = true
	payload.Scope = skill.Scope
	payload.AutoInvoke = skill.AutoInvoke
	return payload, nil
}

// SyncMetadataJSON renders the payload exactly like
// json.dumps(payload) with Python's default separators and ensure_ascii=True:
// key order enabled, name, scope, auto_invoke, warnings.
func SyncMetadataJSON(m SyncMetadata) string {
	var sb strings.Builder
	sb.WriteString(`{"enabled": `)
	if m.Enabled {
		sb.WriteString("true")
	} else {
		sb.WriteString("false")
	}
	sb.WriteString(`, "name": ` + jsonQuote(m.Name))
	sb.WriteString(`, "scope": ` + jsonQuoteList(m.Scope))
	sb.WriteString(`, "auto_invoke": ` + jsonQuoteList(m.AutoInvoke))
	sb.WriteString(`, "warnings": ` + jsonQuoteList(m.Warnings))
	sb.WriteByte('}')
	return sb.String()
}

// SkillContractErrorJSON renders {"error": exc.as_dict()} exactly like
// json.dumps: field, message, path (null when Python saw no path).
func SkillContractErrorJSON(e *SkillContractError) string {
	path := "null"
	if e.Path != "" {
		path = jsonQuote(e.Path)
	}
	return `{"error": {"field": ` + jsonQuote(e.Field) + `, "message": ` + jsonQuote(e.Message) + `, "path": ` + path + `}}`
}

// jsonQuoteList renders a JSON array of strings; a nil slice is [].
func jsonQuoteList(items []string) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(jsonQuote(item))
	}
	sb.WriteByte(']')
	return sb.String()
}

// jsonQuote mirrors Python's json.dumps string escaping: ", \\ and the < 0x20
// controls (short forms for \b \f \n \r \t), everything else non-ASCII as
// lowercase \uXXXX (surrogate pairs beyond the BMP). Copied from the sibling
// encoders (internal/target/json.go); they are unexported per package.
func jsonQuote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 || r > 0x7e {
				const hex = "0123456789abcdef"
				for _, u := range utf16.Encode([]rune{r}) {
					sb.WriteString(`\u`)
					for shift := 12; shift >= 0; shift -= 4 {
						sb.WriteByte(hex[(u>>uint(shift))&0xf])
					}
				}
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
