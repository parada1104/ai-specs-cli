package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Recipe-config TOML writer core, ported 1:1 from the Python authority
// lib/_internal/recipe-config-write.py (update_recipe_config). Go owns the
// line-surgery decision; the standard TOML parser keeps every parse through
// the bounded embedded-Python seam (same boundary as recipe_toml.go), so no
// TOML parser (hand-written subset or new Go dependency) is introduced.
//
// Command surface: --write-recipe-config reads one JSON envelope
// {"manifest_path": "...", "recipe_id": "...", "values": {...}} on stdin and
// prints {"applied": true|false} on stdout with exit 0, or {"error": "<exact
// reference refusal string>"} on stdout with exit 2. Infrastructure failures
// report on stderr with exit 2.

// recipeConfigWriteRequest is the stdin envelope. Values are decoded with the
// ordered JSON decoder so dict key order matches the Python bridge's
// json.loads (insertion order, last value wins on duplicate keys).
type recipeConfigWriteRequest struct {
	ManifestPath string          `json:"manifest_path"`
	RecipeID     string          `json:"recipe_id"`
	Values       json.RawMessage `json:"values"`
}

// Embedded reader programs. Each prints one JSON object: {"config": ...},
// {"value": ...} or {"ok": ...} on success, {"error": "<message>"} on a
// failure the Python reference propagates. The parser itself is the trust
// boundary: isolated (-I -B) stdlib tomllib only, no repository code.
const recipeConfigReader = `import json, sys, tomllib
try:
    text = sys.stdin.buffer.read().decode("utf-8")
except UnicodeDecodeError as exc:
    print(json.dumps({"error": str(exc)}))
else:
    try:
        parsed = tomllib.loads(text)
    except tomllib.TOMLDecodeError:
        print(json.dumps({"config": {}}))
    else:
        recipes = parsed.get("recipes") or {}
        recipe = recipes.get(sys.argv[1]) if isinstance(recipes, dict) else None
        config = recipe.get("config") if isinstance(recipe, dict) else None
        print(json.dumps({"config": config if isinstance(config, dict) else {}}, default=str))
`

const recipeTomlValueReader = `import json, sys, tomllib
try:
    text = sys.stdin.buffer.read().decode("utf-8")
    value = tomllib.loads("value = " + text + "\n")["value"]
except (UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
    print(json.dumps({"error": str(exc)}))
else:
    print(json.dumps({"value": value}, default=str))
`

const recipeTomlTextValidator = `import json, sys, tomllib
try:
    tomllib.loads(sys.stdin.buffer.read().decode("utf-8"))
except (UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
    print(json.dumps({"ok": False, "error": str(exc)}))
else:
    print(json.dumps({"ok": True}))
`

// runRecipeConfigPython runs one embedded reader under bounded execution,
// mirroring runManifestParser: a deadline, bounded stdout and stderr, explicit
// interpreter resolution, and an interpreter refused inside the project root.
func runRecipeConfigPython(root, program string, args []string, stdin []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestParseTimeout)
	defer cancel()

	interpreter, err := manifestInterpreter(root)
	if err != nil {
		return nil, err
	}
	stdout := &cappedBuffer{max: acquisitionLimit}
	stderr := &cappedBuffer{max: manifestErrorLimit}
	cmd := exec.CommandContext(ctx, interpreter, "-I", "-B", "-c", program)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = manifestParseTimeout

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("standard TOML parser timed out after %s", manifestParseTimeout)
	}
	if runErr != nil {
		return nil, classifyManifestParserError(runErr)
	}
	if stdout.truncated {
		return nil, fmt.Errorf("standard TOML parser returned more than %d bytes", acquisitionLimit)
	}
	return stdout.buf.Bytes(), nil
}

// readRecipeConfigTable is _existing_config: the recipe's config table from
// the manifest text through the standard parser. A manifest that fails TOML
// parsing yields an empty table ({}), exactly as the Python reference does;
// every other failure is returned.
func readRecipeConfigTable(root string, manifestText []byte, recipeID string) (*orderedMap, error) {
	out, err := runRecipeConfigPython(root, recipeConfigReader, []string{recipeID}, manifestText)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Config json.RawMessage `json:"config"`
		Error  *string         `json:"error"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("deserialized recipe config: %w", err)
	}
	if payload.Error != nil {
		return nil, errors.New(*payload.Error)
	}
	decoded, err := decodeOrdered(payload.Config)
	if err != nil {
		return nil, fmt.Errorf("deserialized recipe config: %w", err)
	}
	config, ok := decoded.(*orderedMap)
	if !ok {
		config = newOrderedMap()
	}
	return config, nil
}

// parseInlineTomlValue is _parse_inline_value: parse one inline value text
// through the standard parser. A parse failure returns the exact reference
// exception text for the caller's refusal message.
func parseInlineTomlValue(root, valueText string) (any, error) {
	out, err := runRecipeConfigPython(root, recipeTomlValueReader, nil, []byte(valueText))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Value json.RawMessage `json:"value"`
		Error *string         `json:"error"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("deserialized inline value: %w", err)
	}
	if payload.Error != nil {
		return nil, errors.New(*payload.Error)
	}
	return decodeOrdered(payload.Value)
}

// validateTomlText is the final tomllib.loads gate over the candidate text.
// It returns the exact reference exception text on failure.
func validateTomlText(root, newText string) error {
	out, err := runRecipeConfigPython(root, recipeTomlTextValidator, nil, []byte(newText))
	if err != nil {
		return err
	}
	var payload struct {
		OK    bool    `json:"ok"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return fmt.Errorf("validated TOML text: %w", err)
	}
	if !payload.OK {
		if payload.Error != nil {
			return errors.New(*payload.Error)
		}
		return errors.New("invalid TOML")
	}
	return nil
}

// --- Python parity helpers ---

// tomlKey is _toml_key: bare keys stay bare, everything else is JSON-quoted.
func tomlKey(key string) string {
	if isBareTOMLKey(key) {
		return key
	}
	return pyJSONString(key)
}

func isBareTOMLKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// pyJSONString reproduces json.dumps(s) with ensure_ascii=True: named escapes
// for the CPython escape set, lowercase 4-digit hex for remaining control
// characters and everything >= 0x7f, surrogate pairs above the BMP.
func pyJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || r >= 0x7f:
			if r > 0xFFFF {
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlValue is toml_write.toml_value: bool first, numbers as their text,
// JSON-quoted strings, inline lists and tables, and the exact serialization
// refusal for unserializable values (JSON null decodes as None).
func tomlValue(v any) (string, error) {
	switch t := v.(type) {
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case json.Number:
		return t.String(), nil
	case string:
		return pyJSONString(t), nil
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			encoded, err := tomlValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, encoded)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *orderedMap:
		parts := make([]string, 0, len(t.keys))
		for _, k := range t.keys {
			encoded, err := tomlValue(t.values[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, k+" = "+encoded)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	case nil:
		return "", errors.New("cannot serialize NoneType to TOML")
	default:
		return "", fmt.Errorf("cannot serialize %T to TOML", v)
	}
}

// pyValueEqual mirrors Python's == over the decoded value domain, including
// the numeric edges Python accepts: bool is an int (True == 1), and int/float
// comparisons are numeric. Comparisons stay exact for integer pairs.
func pyValueEqual(a, b any) bool {
	aNum, aIsNum := pyNumericText(a)
	bNum, bIsNum := pyNumericText(b)
	if aIsNum || bIsNum {
		return aIsNum && bIsNum && pyNumEqual(aNum, bNum)
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case nil:
		return b == nil
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !pyValueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case *orderedMap:
		bv, ok := b.(*orderedMap)
		if !ok || len(av.keys) != len(bv.keys) {
			return false
		}
		for _, k := range av.keys {
			other, exists := bv.values[k]
			if !exists || !pyValueEqual(av.values[k], other) {
				return false
			}
		}
		return true
	}
	return false
}

// pyNumericText treats bool as its Python int value (True == 1) and reports
// the numeric text of json.Number values.
func pyNumericText(v any) (string, bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return "1", true
		}
		return "0", true
	case json.Number:
		return t.String(), true
	}
	return "", false
}

func isIntSyntax(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// pyNumEqual compares two numeric texts the way Python == does: exact for
// integer pairs, numeric (float64) otherwise. ponytail: int-vs-float equality
// beyond float64 precision (int > 2^53 against a float) is approximated, an
// unreachable corner for recipe config values.
func pyNumEqual(a, b string) bool {
	if isIntSyntax(a) && isIntSyntax(b) {
		ai, errA := strconv.ParseInt(a, 10, 64)
		bi, errB := strconv.ParseInt(b, 10, 64)
		if errA == nil && errB == nil {
			return ai == bi
		}
		return bigIntEqual(a, b)
	}
	af, errA := strconv.ParseFloat(a, 64)
	bf, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return a == b
	}
	return af == bf
}

func bigIntEqual(a, b string) bool {
	na := strings.TrimPrefix(a, "-")
	nb := strings.TrimPrefix(b, "-")
	na = strings.TrimLeft(na, "0")
	nb = strings.TrimLeft(nb, "0")
	if len(na) != len(nb) {
		return false
	}
	if na != nb {
		return false
	}
	negativeA := strings.HasPrefix(a, "-")
	negativeB := strings.HasPrefix(b, "-")
	if na == "" {
		return true
	}
	return negativeA == negativeB
}

// pySplitLines reproduces Python str.splitlines(keepends=True) so rejoining
// the lines reproduces the original bytes byte for byte.
func pySplitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '\r', '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			end := i + size
			if r == '\r' && end < len(s) && s[end] == '\n' {
				end++
			}
			lines = append(lines, s[start:end])
			start = end
			i = end
		default:
			i += size
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// splitInlineComment is _split_inline_comment: TOML string-aware inline
// comment splitting, byte-exact against the reference (the comment keeps the
// whitespace that separated it from the value).
func splitInlineComment(line string) (string, string) {
	quote := byte(0)
	triple := false
	escaped := false
	for idx := 0; idx < len(line); idx++ {
		c := line[idx]
		if triple {
			if tripleMarker(line, idx, quote) {
				triple = false
				idx += 2
				continue
			}
			continue
		}
		if quote == '"' {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			}
			continue
		}
		if tripleMarker(line, idx, '"') || tripleMarker(line, idx, '\'') {
			quote = c
			triple = true
			idx += 2
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '#' {
			start := idx
			for start > 0 && (line[start-1] == ' ' || line[start-1] == '\t') {
				start--
			}
			return line[:start], line[start:]
		}
	}
	return line, ""
}

func tripleMarker(line string, idx int, quote byte) bool {
	if idx+3 > len(line) {
		return false
	}
	return line[idx] == quote && line[idx+1] == quote && line[idx+2] == quote
}

// valueIsMultiline is _value_is_multiline: detect an unfinished array, table,
// or string value on one source line. Byte scanning is safe: the special
// characters (quotes, brackets, backslash) are all single-byte ASCII.
func valueIsMultiline(line string) bool {
	value, _ := splitInlineComment(line)
	equals := strings.Index(value, "=")
	if equals < 0 {
		return false
	}
	text := value[equals+1:]
	quote := byte(0)
	triple := false
	escaped := false
	depth := 0
	for idx := 0; idx < len(text); idx++ {
		c := text[idx]
		if triple {
			if tripleMarker(text, idx, quote) {
				triple = false
				idx += 2
				continue
			}
			continue
		}
		if quote != 0 {
			if quote == '"' && escaped {
				escaped = false
			} else if quote == '"' && c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if tripleMarker(text, idx, '"') || tripleMarker(text, idx, '\'') {
			quote = c
			triple = true
			idx += 2
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return quote != 0 || triple || depth > 0
}

// matchKeyLine is _key_line_re(key).match(line): leading whitespace, then the
// bare or JSON-quoted key, then optional whitespace and '='. It returns the
// captured indent. The alternation order matches the reference: bare first,
// quoted second.
func matchKeyLine(line, key string) (string, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	indent := line[:i]
	rest := line[i:]
	keyText := ""
	if strings.HasPrefix(rest, key) {
		keyText = key
	} else if quoted := pyJSONString(key); strings.HasPrefix(rest, quoted) {
		keyText = quoted
	} else {
		return "", false
	}
	rest = rest[len(keyText):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "=") {
		return indent, true
	}
	return "", false
}

// assignmentParts is _assignment_parts: (indent, value text, comment) for one
// key = value line, with the reference refusal when no '=' exists.
func assignmentParts(line string) (string, string, string, error) {
	prefix, comment := splitInlineComment(line)
	equals := strings.Index(prefix, "=")
	if equals < 0 {
		return "", "", "", errors.New("cannot rewrite a line without a key assignment")
	}
	head := prefix[:equals]
	indent := head[:len(head)-len(pyLstrip(head))]
	return indent, strings.TrimSpace(prefix[equals+1:]), strings.TrimRight(comment, "\r\n"), nil
}

// pyLstrip mirrors Python's default str.lstrip (Unicode whitespace).
func pyLstrip(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

// pyTrimSpace mirrors Python's str.strip (Unicode whitespace).
func pyTrimSpace(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace)
}

// --- reference algorithm port ---

type recipeConfigPair struct {
	key   string
	value any
}

type recipeConfigDottedPair struct {
	path  []string
	value any
}

// orderedPairs mirrors a Python dict built over the decoded values: first
// position on insert, last value on duplicate key.
type orderedPairs struct {
	keys []string
	vals map[string]any
}

func newOrderedPairs() *orderedPairs {
	return &orderedPairs{vals: map[string]any{}}
}

func (o *orderedPairs) set(key string, value any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

func (o *orderedPairs) get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// nestedGet is _nested_get: the value at a dotted path, or absent when any
// segment is missing or non-table.
func nestedGet(m *orderedMap, path []string) (any, bool) {
	node := any(m)
	for _, part := range path {
		asMap, ok := node.(*orderedMap)
		if !ok {
			return nil, false
		}
		v, exists := asMap.get(part)
		if !exists {
			return nil, false
		}
		node = v
	}
	return node, true
}

// nestedSet is _nested_set: set a dotted path, replacing non-table
// intermediates with fresh tables (existing keys keep their position).
func nestedSet(m *orderedMap, path []string, value any) {
	if len(path) == 0 {
		return
	}
	node := m
	for _, part := range path[:len(path)-1] {
		child, _ := node.get(part)
		childMap, isMap := child.(*orderedMap)
		if !isMap {
			childMap = newOrderedMap()
			node.set(part, childMap)
		}
		node = childMap
	}
	node.set(path[len(path)-1], value)
}

func dottedPairsToInlineTable(pairs []recipeConfigDottedPair) *orderedMap {
	table := newOrderedMap()
	for _, pair := range pairs {
		nestedSet(table, pair.path[1:], pair.value)
	}
	return table
}

func subtableHeader(recipeKey string, tablePath []string) string {
	parts := make([]string, 0, len(tablePath))
	for _, part := range tablePath {
		parts = append(parts, tomlKey(part))
	}
	return "[recipes." + recipeKey + ".config." + strings.Join(parts, ".") + "]"
}

func findRecipeHeader(lines []string, header string) int {
	for idx, line := range lines {
		if pyTrimSpace(line) == header {
			return idx
		}
	}
	return -1
}

// recipeRegionEnd is _recipe_region_end: the recipe region ends at the next
// header line that does not carry the recipe's [recipes.<key> prefix (the
// unclosed prefix is reference behavior, kept byte-exact).
func recipeRegionEnd(lines []string, start int, recipeKey string) int {
	prefix := "[recipes." + recipeKey
	for idx := start + 1; idx < len(lines); idx++ {
		stripped := pyTrimSpace(lines[idx])
		if strings.HasPrefix(stripped, "[") && !strings.HasPrefix(stripped, prefix) {
			return idx
		}
	}
	return len(lines)
}

func findConfigHeader(lines []string, start, end int, configHeader string) int {
	for idx := start; idx < end; idx++ {
		if pyTrimSpace(lines[idx]) == configHeader {
			return idx
		}
	}
	return -1
}

func isHeaderLine(line string) bool {
	return strings.HasPrefix(pyLstrip(line), "[")
}

func configBlockEnd(lines []string, start, regionEnd int) int {
	for idx := start + 1; idx < regionEnd; idx++ {
		if isHeaderLine(lines[idx]) {
			return idx
		}
	}
	return regionEnd
}

func insertLines(lines []string, at int, inserted []string) []string {
	out := make([]string, 0, len(lines)+len(inserted))
	out = append(out, lines[:at]...)
	out = append(out, inserted...)
	out = append(out, lines[at:]...)
	return out
}

// applyDottedUpdates is _apply_dotted_updates: an inline-table root updates in
// place, a header-table root updates leaf lines, an absent root is appended
// as a new inline table. Refusals are the exact reference strings.
func applyDottedUpdates(state *recipeConfigWriteState, grouped map[string][]recipeConfigDottedPair, order []string) (string, error) {
	inlineLines := map[string]int{}
	var headerRoots, newRoots []string
	for _, root := range order {
		idx := -1
		for candidate := state.configIdx + 1; candidate < state.blockEnd; candidate++ {
			if _, ok := matchKeyLine(state.lines[candidate], root); ok {
				idx = candidate
				break
			}
		}
		if idx >= 0 {
			inlineLines[root] = idx
			continue
		}
		header := subtableHeader(state.recipeKey, []string{root})
		found := false
		for candidate := state.recipeIdx; candidate < state.regionEnd; candidate++ {
			if pyTrimSpace(state.lines[candidate]) == header {
				found = true
				break
			}
		}
		if found {
			headerRoots = append(headerRoots, root)
		} else {
			newRoots = append(newRoots, root)
		}
	}

	for _, root := range order {
		idx, ok := inlineLines[root]
		if !ok {
			continue
		}
		if err := setInlinePaths(state, idx, root, grouped[root]); err != nil {
			return "", err
		}
	}

	for _, root := range headerRoots {
		for _, pair := range grouped[root] {
			if err := setHeaderPath(state, pair.path, pair.value); err != nil {
				return "", err
			}
		}
	}

	if len(newRoots) > 0 {
		blockEnd := configBlockEnd(state.lines, state.configIdx, state.regionEnd)
		var newLines []string
		for _, root := range newRoots {
			encoded, err := tomlValue(dottedPairsToInlineTable(grouped[root]))
			if err != nil {
				return "", err
			}
			newLines = append(newLines, tomlKey(root)+" = "+encoded+"\n")
		}
		state.lines = insertLines(state.lines, blockEnd, newLines)
	}
	return "", nil
}

// setInlinePaths is _set_inline_paths.
func setInlinePaths(state *recipeConfigWriteState, idx int, root string, pairs []recipeConfigDottedPair) error {
	if valueIsMultiline(state.lines[idx]) {
		return fmt.Errorf("cannot replace multiline value for key '%s'", root)
	}
	indent, valueText, comment, err := assignmentParts(state.lines[idx])
	if err != nil {
		return err
	}
	parsed, err := parseInlineTomlValue(state.root, valueText)
	if err != nil {
		return fmt.Errorf("cannot read the existing value for '%s': %v", root, err)
	}
	table, ok := parsed.(*orderedMap)
	if !ok {
		return fmt.Errorf("cannot update '%s': its value is not an inline table", root)
	}
	for _, pair := range pairs {
		nestedSet(table, pair.path[1:], pair.value)
	}
	encoded, err := tomlValue(table)
	if err != nil {
		return err
	}
	newline := ""
	if strings.HasSuffix(state.lines[idx], "\n") {
		newline = "\n"
	}
	state.lines[idx] = indent + tomlKey(root) + " = " + encoded + comment + newline
	return nil
}

// setHeaderPath is _set_header_path: replace or append one leaf line inside a
// declared header table, recomputing the region end fresh on every call.
func setHeaderPath(state *recipeConfigWriteState, path []string, value any) error {
	regionEnd := recipeRegionEnd(state.lines, state.recipeIdx, state.recipeKey)
	header := subtableHeader(state.recipeKey, path[:len(path)-1])
	headerIdx := -1
	for idx := state.recipeIdx; idx < regionEnd; idx++ {
		if pyTrimSpace(state.lines[idx]) == header {
			headerIdx = idx
			break
		}
	}
	if headerIdx < 0 {
		return fmt.Errorf("cannot update '%s': table %s not found", strings.Join(path, "."), header)
	}
	blockEnd := configBlockEnd(state.lines, headerIdx, regionEnd)
	leaf := path[len(path)-1]
	for idx := headerIdx + 1; idx < blockEnd; idx++ {
		indent, ok := matchKeyLine(state.lines[idx], leaf)
		if !ok {
			continue
		}
		if valueIsMultiline(state.lines[idx]) {
			return fmt.Errorf("cannot replace multiline value for key '%s'", leaf)
		}
		_, _, comment, err := assignmentParts(state.lines[idx])
		if err != nil {
			return err
		}
		encoded, err := tomlValue(value)
		if err != nil {
			return err
		}
		newline := ""
		if strings.HasSuffix(state.lines[idx], "\n") {
			newline = "\n"
		}
		state.lines[idx] = indent + tomlKey(leaf) + " = " + encoded + comment + newline
		return nil
	}
	encoded, err := tomlValue(value)
	if err != nil {
		return err
	}
	state.lines = insertLines(state.lines, blockEnd, []string{tomlKey(leaf) + " = " + encoded + "\n"})
	return nil
}

// recipeConfigWriteState carries the mutable line slice and region bounds
// through the dotted-update phase, mirroring the reference's in-place list
// mutation and stale-then-recomputed region indices.
type recipeConfigWriteState struct {
	lines     []string
	recipeIdx int
	regionEnd int
	configIdx int
	blockEnd  int
	recipeKey string
	root      string
}

// applyRecipeConfigWrite is update_recipe_config. It returns whether the
// manifest changed and, for refusals, the exact reference error string.
func applyRecipeConfigWrite(manifestPath, recipeID string, values *orderedMap) (bool, string, error) {
	if values == nil || len(values.keys) == 0 {
		return false, "", nil
	}
	root := filepath.Dir(manifestPath)
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, "", fmt.Errorf("read manifest: %w", err)
	}
	originalText := string(original)

	current, err := readRecipeConfigTable(root, original, recipeID)
	if err != nil {
		return false, "", err
	}

	plain := newOrderedPairs()
	dotted := newOrderedPairs()
	for _, key := range values.keys {
		if strings.Contains(key, ".") {
			dotted.set(key, values.values[key])
		} else {
			plain.set(key, values.values[key])
		}
	}

	pendingPlain := newOrderedPairs()
	for _, key := range plain.keys {
		value := plain.vals[key]
		cur, ok := current.get(key)
		if !ok || !pyValueEqual(cur, value) {
			pendingPlain.set(key, value)
		}
	}
	pendingDotted := newOrderedPairs()
	for _, key := range dotted.keys {
		value := dotted.vals[key]
		path := strings.Split(key, ".")
		cur, ok := nestedGet(current, path)
		if !ok || !pyValueEqual(cur, value) {
			pendingDotted.set(key, value)
		}
	}
	if len(pendingPlain.keys) == 0 && len(pendingDotted.keys) == 0 {
		return false, "", nil
	}

	conflictSet := map[string]bool{}
	for _, key := range pendingDotted.keys {
		conflictSet[strings.Split(key, ".")[0]] = true
	}
	var conflicts []string
	for _, key := range pendingPlain.keys {
		if conflictSet[key] {
			conflicts = append(conflicts, key)
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return false, "cannot combine a whole-table and a dotted update for key(s): " + strings.Join(conflicts, ", "), nil
	}

	grouped := map[string][]recipeConfigDottedPair{}
	var groupedOrder []string
	for _, key := range pendingDotted.keys {
		path := strings.Split(key, ".")
		rootKey := path[0]
		if _, ok := grouped[rootKey]; !ok {
			groupedOrder = append(groupedOrder, rootKey)
		}
		grouped[rootKey] = append(grouped[rootKey], recipeConfigDottedPair{path: path, value: pendingDotted.vals[key]})
	}
	sort.Strings(groupedOrder)

	recipeKey := tomlKey(recipeID)
	lines := pySplitLines(originalText)
	newText := ""

	if idx := findRecipeHeader(lines, "[recipes."+recipeKey+"]"); idx < 0 {
		blockLines := []string{
			"\n",
			"[recipes." + recipeKey + "]\n",
			"enabled = true\n",
			"\n",
			"[recipes." + recipeKey + ".config]\n",
		}
		for _, key := range sortedPairKeys(pendingPlain) {
			encoded, err := tomlValue(pendingPlain.vals[key])
			if err != nil {
				return false, err.Error(), nil
			}
			blockLines = append(blockLines, tomlKey(key)+" = "+encoded+"\n")
		}
		for _, rootKey := range groupedOrder {
			encoded, err := tomlValue(dottedPairsToInlineTable(grouped[rootKey]))
			if err != nil {
				return false, err.Error(), nil
			}
			blockLines = append(blockLines, tomlKey(rootKey)+" = "+encoded+"\n")
		}
		newText = originalText
		if !strings.HasSuffix(newText, "\n") && newText != "" {
			newText += "\n"
		}
		newText += strings.Join(blockLines, "")
	} else {
		state := &recipeConfigWriteState{
			lines:     lines,
			recipeIdx: idx,
			recipeKey: recipeKey,
			root:      root,
		}
		state.regionEnd = recipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
		state.configIdx = findConfigHeader(state.lines, state.recipeIdx, state.regionEnd, "[recipes."+recipeKey+".config]")
		if state.configIdx < 0 {
			state.lines = insertLines(state.lines, state.regionEnd, []string{"\n", "[recipes." + recipeKey + ".config]\n"})
			state.configIdx = state.regionEnd + 1
			state.regionEnd = recipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
		}
		state.blockEnd = configBlockEnd(state.lines, state.configIdx, state.regionEnd)

		var pending []recipeConfigPair
		for _, key := range sortedPairKeys(pendingPlain) {
			value := pendingPlain.vals[key]
			replaced := false
			for candidate := state.configIdx + 1; candidate < state.blockEnd; candidate++ {
				indent, ok := matchKeyLine(state.lines[candidate], key)
				if !ok {
					continue
				}
				if valueIsMultiline(state.lines[candidate]) {
					return false, fmt.Sprintf("cannot replace multiline value for key '%s'", key), nil
				}
				_, comment := splitInlineComment(state.lines[candidate])
				newline := ""
				if strings.HasSuffix(state.lines[candidate], "\n") {
					newline = "\n"
				}
				encoded, err := tomlValue(value)
				if err != nil {
					return false, err.Error(), nil
				}
				state.lines[candidate] = indent + tomlKey(key) + " = " + encoded + strings.TrimRight(comment, "\r\n") + newline
				replaced = true
				break
			}
			if !replaced {
				pending = append(pending, recipeConfigPair{key: key, value: value})
			}
		}

		if len(pending) > 0 {
			newLines := make([]string, 0, len(pending))
			for _, pair := range pending {
				encoded, err := tomlValue(pair.value)
				if err != nil {
					return false, err.Error(), nil
				}
				newLines = append(newLines, tomlKey(pair.key)+" = "+encoded+"\n")
			}
			state.lines = insertLines(state.lines, state.blockEnd, newLines)
			state.regionEnd = recipeRegionEnd(state.lines, state.recipeIdx, recipeKey)
			state.blockEnd = configBlockEnd(state.lines, state.configIdx, state.regionEnd)
		}

		if len(groupedOrder) > 0 {
			if _, err := applyDottedUpdates(state, grouped, groupedOrder); err != nil {
				return false, err.Error(), nil
			}
		}
		newText = strings.Join(state.lines, "")
	}

	if err := validateTomlText(root, newText); err != nil {
		return false, "invalid TOML after config write: " + err.Error(), nil
	}
	if newText != originalText {
		if err := os.WriteFile(manifestPath, []byte(newText), 0o644); err != nil {
			return false, "", fmt.Errorf("write manifest: %w", err)
		}
		return true, "", nil
	}
	return false, "", nil
}

func sortedPairKeys(pairs *orderedPairs) []string {
	sorted := append([]string(nil), pairs.keys...)
	sort.Strings(sorted)
	return sorted
}

// runWriteRecipeConfig is the --write-recipe-config command: one JSON envelope
// on stdin, one JSON envelope on stdout. Exit 0 on applied/no-op, exit 2 on a
// refusal (the exact reference string in the error envelope) or an
// infrastructure failure (diagnostic on stderr).
func runWriteRecipeConfig(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: read stdin: %v\n", err)
		return 2
	}
	var req recipeConfigWriteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: invalid input JSON: %v\n", err)
		return 2
	}
	var values *orderedMap
	if len(req.Values) > 0 {
		decoded, err := decodeOrdered(req.Values)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: invalid values: %v\n", err)
			return 2
		}
		if decoded != nil {
			m, ok := decoded.(*orderedMap)
			if !ok {
				fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: values must be a JSON object\n")
				return 2
			}
			values = m
		}
	}
	applied, refusal, err := applyRecipeConfigWrite(req.ManifestPath, req.RecipeID, values)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: %v\n", err)
		return 2
	}
	if refusal != "" {
		fmt.Fprintln(stdout, `{"error": `+pyJSONString(refusal)+`}`)
		return 2
	}
	appliedText := "false"
	if applied {
		appliedText = "true"
	}
	fmt.Fprintln(stdout, `{"applied": `+appliedText+`}`)
	return 0
}
