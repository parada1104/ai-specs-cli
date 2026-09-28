package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// Surgical manifest writes: byte-exact ports of the python3 heredocs in
// lib/skills-add.sh, lib/recipe-remove.sh, and lib/skills-remove.sh.
//
// All operations work on raw bytes and never re-serialize parsed TOML;
// internal/toml.Parse is used only for the valid→invalid validate-guard.
// All writes are atomic (temp file in the target's directory + rename),
// preserving the original file mode (0o7777 mask).

// segment is one block of the manifest: an optional column-0 '[' header
// line plus every following line until the next such header. The preamble
// before the first header is its own segment (no header). An indented
// "  [other.table]" line is invisible to the segmenter and stays attached
// to the preceding block — mirroring the Python heredocs exactly.
type segment struct {
	header    string
	hasHeader bool
	lines     []string // includes the header line and each line's ending
}

// splitLinesKeepEnds ports Python str.splitlines(keepends=True): split on
// \n, \r\n, \r, \v, \f, \x1c-\x1e, NEL (U+0085), U+2028, U+2029, keeping
// each line's original ending. A file without a trailing newline keeps its
// final unterminated line.
func splitLinesKeepEnds(s string) []string {
	var lines []string
	start := 0
	i := 0
	runes := []rune(s)
	for i < len(runes) {
		r := runes[i]
		var end int
		switch r {
		case '\r':
			end = i + 1
			if end < len(runes) && runes[end] == '\n' {
				end++
			}
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, '\u2028', '\u2029':
			end = i + 1
		default:
			i++
			continue
		}
		lines = append(lines, string(runes[start:end]))
		start = end
		i = end
	}
	if start < len(runes) {
		lines = append(lines, string(runes[start:]))
	}
	return lines
}

// splitSegments ports the heredocs' segmenter: a new segment begins at each
// line whose FIRST character is '['.
func splitSegments(content string) []segment {
	var segments []segment
	current := segment{}
	for _, line := range splitLinesKeepEnds(content) {
		if strings.HasPrefix(line, "[") {
			if len(current.lines) > 0 || current.hasHeader {
				segments = append(segments, current)
			}
			current = segment{header: line, hasHeader: true, lines: []string{line}}
		} else {
			current.lines = append(current.lines, line)
		}
	}
	if len(current.lines) > 0 || current.hasHeader {
		segments = append(segments, current)
	}
	return segments
}

// joinSegments reassembles content from remaining segments, verbatim.
func joinSegments(segments []segment) string {
	var sb strings.Builder
	for _, seg := range segments {
		for _, line := range seg.lines {
			sb.WriteString(line)
		}
	}
	return sb.String()
}

// guardValidToInvalid refuses a write that regresses a valid manifest into
// an invalid one. A manifest that is ALREADY invalid may still be edited
// (frozen parity contract): only the valid→invalid regression is guarded.
// The error text mirrors the heredocs; the embedded parse-error detail is
// diagnostic (ADR 0002).
func guardValidToInvalid(original, newContent, errPrefix string) error {
	if _, err := toml.Parse([]byte(original)); err != nil {
		return nil // already invalid — guard does not fire
	}
	if _, err := toml.Parse([]byte(newContent)); err != nil {
		return fmt.Errorf("%s%s", errPrefix, err)
	}
	return nil
}

// atomicWrite replaces path with content via a temp file in the target's
// parent directory + rename, preserving the original file mode (0o7777
// mask, mirroring mkstemp + chmod + os.replace in the heredocs).
func atomicWrite(path, content string) error {
	var mode os.FileMode = 0o644
	if info, err := os.Stat(path); err == nil {
		m := info.Mode()
		mode = m.Perm() | m&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ai-specs.toml.*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// AppendDepsBlock is the port of the skills-add.sh heredoc: append a
// rendered [[deps]] block at EOF. Values are rendered with the heredoc's
// s() (backslash-then-quote escaping). Like the Python original, this does
// NO validation — its non-validation is recorded-defect behavior and is
// mirrored, not fixed.
func AppendDepsBlock(manifestPath, depID, url, subdir, scopeCSV, trigger, license, attribution, ref string) (string, error) {
	s := func(x string) string {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(x, `\`, `\\`), `"`, `\"`) + `"`
	}

	var scopes []string
	for _, x := range strings.Split(scopeCSV, ",") {
		if x = strings.TrimSpace(x); x != "" {
			scopes = append(scopes, x)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{"root"}
	}
	scopeParts := make([]string, len(scopes))
	for i, x := range scopes {
		scopeParts[i] = s(x)
	}

	block := []string{"", "[[deps]]"}
	block = append(block, "id = "+s(depID))
	block = append(block, "source = "+s(url))
	if subdir != "" {
		block = append(block, "path = "+s(subdir))
	}
	block = append(block, "scope = ["+strings.Join(scopeParts, ", ")+"]")
	if trigger != "" {
		block = append(block, "auto_invoke = ["+s(trigger)+"]")
	}
	if license != "" {
		block = append(block, "license = "+s(license))
	}
	if attribution != "" {
		block = append(block, "vendor_attribution = "+s(attribution))
	}
	if ref != "" {
		block = append(block, "ref = "+s(ref))
	}
	block = append(block, "")

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	text := string(content)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	newContent := text + strings.Join(block, "\n")
	if err := atomicWrite(manifestPath, newContent); err != nil {
		return "", err
	}
	return fmt.Sprintf("  ✓ appended [[deps]] for '%s' to %s", depID, manifestPath), nil
}

// RemoveRecipeSegments is the port of the recipe-remove.sh heredoc: remove
// ALL [recipes.<id>] / [recipes.<id>.*] segments matching the heredoc's
// header regex. When no segment matches, the file is untouched.
func RemoveRecipeSegments(manifestPath, recipeID string) (int, string, error) {
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return 0, "", err
	}
	segments := splitSegments(string(original))

	pattern := regexp.MustCompile(
		`^\s*\[\s*recipes\s*\.\s*` + regexp.QuoteMeta(recipeID) + `(\s*[.\]]|\s*$)`)
	var targets []int
	for i, seg := range segments {
		if !seg.hasHeader {
			continue
		}
		// TOML allows comments: [recipes.foo] # comment
		if pattern.MatchString(strings.TrimSpace(seg.header)) {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		return 0, "", fmt.Errorf("  ✗ recipe '%s' not found in %s", recipeID, manifestPath)
	}

	drop := make(map[int]bool, len(targets))
	for _, i := range targets {
		drop[i] = true
	}
	remaining := make([]segment, 0, len(segments)-len(targets))
	for i, seg := range segments {
		if !drop[i] {
			remaining = append(remaining, seg)
		}
	}
	newContent := joinSegments(remaining)

	if err := guardValidToInvalid(string(original), newContent,
		fmt.Sprintf("ERROR: removing recipe '%s' would produce invalid TOML: ", recipeID)); err != nil {
		return 0, "", err
	}
	if err := atomicWrite(manifestPath, newContent); err != nil {
		return 0, "", err
	}
	return len(targets), fmt.Sprintf("  ✓ removed %d section(s) for recipe '%s' from %s", len(targets), recipeID, manifestPath), nil
}

// RemoveDepSegment is the port of the skills-remove.sh heredoc: delete the
// FIRST [[deps]] segment whose id line matches the target depID. When no
// segment matches, the file is untouched.
func RemoveDepSegment(manifestPath, depID string) (string, error) {
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	segments := splitSegments(string(original))

	idRe := regexp.MustCompile(`(?m)^\s*id\s*=\s*"` + regexp.QuoteMeta(depID) + `"\s*$`)
	targetIdx := -1
	for i, seg := range segments {
		if !seg.hasHeader {
			continue
		}
		// TOML allows inline comments after section headers: [[deps]] # comment
		if !strings.HasPrefix(strings.TrimSpace(seg.header), "[[deps]]") {
			continue
		}
		if idRe.MatchString(joinSegments([]segment{seg})) {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return "", fmt.Errorf("  ✗ dep '%s' not found in %s", depID, manifestPath)
	}

	remaining := make([]segment, 0, len(segments)-1)
	for i, seg := range segments {
		if i != targetIdx {
			remaining = append(remaining, seg)
		}
	}
	newContent := joinSegments(remaining)

	if err := guardValidToInvalid(string(original), newContent,
		fmt.Sprintf("ERROR: removing skill '%s' would produce invalid TOML: ", depID)); err != nil {
		return "", err
	}
	if err := atomicWrite(manifestPath, newContent); err != nil {
		return "", err
	}
	return fmt.Sprintf("  ✓ removed [[deps]] '%s' from %s", depID, manifestPath), nil
}
