package shared

// Recipe-config TOML writer core, promoted from package main by GO-07 S14a so
// the gate binary's --write-recipe-config surface and the root native sync
// materializer share ONE grader of the line-surgery decision.
//
// Ported 1:1 from the Python authority lib/_internal/recipe-config-write.py
// (update_recipe_config). Go owns the line-surgery decision AND, since GO-20
// U2e T5, the three TOML reads it needs (config-table read, inline-value parse,
// post-write validation): they run in process through the parser appended at
// the end of this file instead of an embedded Python tomllib subprocess, so the
// gate binary needs no ambient interpreter for its writer.
//
// The parser is a copy of the root module's internal/toml, which the gate
// module (ai-specs.dev/worktree-gate) can never import: the go tool rejects a
// cross-module internal/ import. It lives in this file because this file is the
// only surface both seams share (the gate main package imports this package).
// internal/toml stays the root module's authoritative copy; the two are the same
// reader by construction, and a later slice can promote one authoritative home.
//
// The seam helpers below are private copies of the gate main package's
// manifest-acquisition seam (ledger_reconcile.go). The promotion could not
// share them: the gate's other readers keep their own copies behind separate
// edit surfaces, and the promoted writer must be reachable from the root module
// without importing package main. The remaining subprocess seam
// (runRecipeConfigPython) now serves only shared/recipe_toml.go's readers and
// the gate main package's resolved-config and recipe.toml acquisition.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// recipeConfigParseTimeout, recipeConfigAcquisitionLimit and
// recipeConfigErrorLimit mirror the gate main package's acquisition bounds.
const (
	recipeConfigParseTimeout     = 3 * time.Second
	recipeConfigAcquisitionLimit = 1 << 20
	recipeConfigErrorLimit       = 4 << 10
)

// recipeConfigCappedBuffer buffers at most max bytes and records that anything
// more arrived, mirroring the gate main package's cappedBuffer.
type recipeConfigCappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *recipeConfigCappedBuffer) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) <= room {
			w.buf.Write(p)
			return len(p), nil
		}
		w.buf.Write(p[:room])
	}
	w.truncated = true
	return len(p), nil
}

// recipeConfigInterpreter resolves the ambient python3 without executing it and
// refuses an interpreter that lives inside the project root.
func recipeConfigInterpreter(root string) (string, error) {
	interpreter, err := exec.LookPath("python3")
	if err != nil {
		return "", fmt.Errorf("standard TOML parser is unavailable: %v", err)
	}
	if abs, absErr := filepath.Abs(interpreter); absErr == nil {
		if absRoot, rootErr := filepath.Abs(root); rootErr == nil && (absRoot == abs || strings.HasPrefix(abs+string(filepath.Separator), absRoot)) {
			return "", errors.New("standard TOML parser interpreter resolves inside the project root")
		}
	}
	return interpreter, nil
}

// classifyRecipeConfigParserError names the failure without reproducing parser
// output, mirroring the gate main package's classifyManifestParserError.
func classifyRecipeConfigParserError(err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return fmt.Errorf("standard TOML parser is unavailable: %v", execErr.Err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("the manifest is not valid TOML (parser exit status %d)", exitErr.ExitCode())
	}
	return errors.New("standard TOML parser could not be started")
}

// runRecipeConfigPython runs one embedded reader under bounded execution: a
// deadline, bounded stdout and stderr, explicit interpreter resolution, and an
// interpreter refused inside the project root.
func runRecipeConfigPython(root, program string, args []string, stdin []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recipeConfigParseTimeout)
	defer cancel()

	interpreter, err := recipeConfigInterpreter(root)
	if err != nil {
		return nil, err
	}
	stdout := &recipeConfigCappedBuffer{max: recipeConfigAcquisitionLimit}
	stderr := &recipeConfigCappedBuffer{max: recipeConfigErrorLimit}
	cmd := exec.CommandContext(ctx, interpreter, "-I", "-B", "-c", program)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = recipeConfigParseTimeout

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("standard TOML parser timed out after %s", recipeConfigParseTimeout)
	}
	if runErr != nil {
		return nil, classifyRecipeConfigParserError(runErr)
	}
	if stdout.truncated {
		return nil, fmt.Errorf("standard TOML parser returned more than %d bytes", recipeConfigAcquisitionLimit)
	}
	return stdout.buf.Bytes(), nil
}

// ReadManifestRecipesJSON is the ledger-reconcile manifest reader: the [recipes]
// table of the manifest at path as one JSON object, {} when the manifest
// declares none or declares a non-table value under that key. Parsing happens
// in process, so the gate needs no ambient interpreter.
func ReadManifestRecipesJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, err := Parse(data)
	if err != nil {
		return nil, manifestNotValid(err)
	}
	table, ok := root.Table("recipes")
	if !ok {
		return []byte("{}"), nil
	}
	var out strings.Builder
	if err := appendJSONValue(&out, table); err != nil {
		return nil, errors.New("the manifest is not valid TOML")
	}
	return []byte(out.String()), nil
}

// manifestNotValid reports an unusable manifest without echoing its content: the
// parse diagnostic is reduced to the line it failed on, so no manifest value can
// reach the error text.
func manifestNotValid(err error) error {
	if line := tomlErrorLine(err); line > 0 {
		return fmt.Errorf("the manifest is not valid TOML (line %d)", line)
	}
	return errors.New("the manifest is not valid TOML")
}

// tomlErrorLine extracts N from the parser's "toml: line N: ..." diagnostic.
func tomlErrorLine(err error) int {
	const prefix = "toml: line "
	text := err.Error()
	if !strings.HasPrefix(text, prefix) {
		return 0
	}
	rest := text[len(prefix):]
	end := strings.Index(rest, ":")
	if end < 0 {
		return 0
	}
	line, convErr := strconv.Atoi(rest[:end])
	if convErr != nil {
		return 0
	}
	return line
}

// appendJSONValue renders a parsed TOML value as JSON. The retired reader let
// Python's json.dumps build this envelope and Go read it back, so the text has to
// be equivalent, not identical: no ", " / ": " separators, and the value domain
// is the same (a TOML date/time has no JSON form there either).
func appendJSONValue(out *strings.Builder, v any) error {
	switch t := v.(type) {
	case *Table:
		out.WriteByte('{')
		for i, key := range t.Keys() {
			if i > 0 {
				out.WriteByte(',')
			}
			out.WriteString(PyJSONString(key))
			out.WriteByte(':')
			value, _ := t.Get(key)
			if err := appendJSONValue(out, value); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case []*Table:
		items := make([]any, len(t))
		for i, item := range t {
			items[i] = item
		}
		return appendJSONValue(out, items)
	case []any:
		out.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := appendJSONValue(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case string:
		out.WriteString(PyJSONString(t))
	case bool:
		if t {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case int64:
		out.WriteString(strconv.FormatInt(t, 10))
	case BigInt:
		out.WriteString(string(t))
	case float64:
		out.WriteString(formatPyFloat(t))
	case DateTime:
		return errors.New("a TOML date/time has no JSON form")
	default:
		return fmt.Errorf("cannot render %T as JSON", v)
	}
	return nil
}

// readRecipeConfigTable is _existing_config: the recipe's config table from the
// manifest text. A manifest that fails to parse yields an empty table ({}),
// exactly as the Python reference does; only undecodable text is an error.
func readRecipeConfigTable(manifestText []byte, recipeID string) (*OrderedMap, error) {
	if !utf8.Valid(manifestText) {
		return nil, errors.New("the manifest is not valid UTF-8")
	}
	config := NewOrderedMap()
	root, err := Parse(manifestText)
	if err != nil {
		return config, nil
	}
	recipes, ok := root.Table("recipes")
	if !ok {
		return config, nil
	}
	recipe, ok := recipes.Table(recipeID)
	if !ok {
		return config, nil
	}
	table, ok := recipe.Table("config")
	if !ok {
		return config, nil
	}
	converted, ok := fromTOML(table).(*OrderedMap)
	if !ok {
		return config, nil
	}
	return converted, nil
}

// parseInlineTomlValue is _parse_inline_value: parse one inline value text. A
// parse failure is returned for the caller's refusal message.
func parseInlineTomlValue(valueText string) (any, error) {
	root, err := Parse([]byte("value = " + valueText + "\n"))
	if err != nil {
		return nil, err
	}
	value, _ := root.Get("value")
	return fromTOML(value), nil
}

// validateTomlText is the final parse gate over the candidate text.
func validateTomlText(newText string) error {
	_, err := Parse([]byte(newText))
	return err
}

// fromTOML lowers a parsed TOML value into the JSON domain the writer works in:
// the shapes DecodeOrdered produced for the reference's json.dumps envelope.
// Tables keep declaration order, numbers become json.Number carrying Python's
// text, and a TOML date/time becomes its Python str() because the reference
// lowered it with json.dumps(default=str) before the writer saw it.
func fromTOML(v any) any {
	switch t := v.(type) {
	case *Table:
		m := NewOrderedMap()
		for _, key := range t.Keys() {
			value, _ := t.Get(key)
			m.Set(key, fromTOML(value))
		}
		return m
	case []*Table:
		items := make([]any, 0, len(t))
		for _, item := range t {
			items = append(items, fromTOML(item))
		}
		return items
	case []any:
		items := make([]any, 0, len(t))
		for _, item := range t {
			items = append(items, fromTOML(item))
		}
		return items
	case int64:
		return json.Number(strconv.FormatInt(t, 10))
	case BigInt:
		return json.Number(string(t))
	case float64:
		return json.Number(formatPyFloat(t))
	case DateTime:
		return t.PyStr()
	default:
		return v
	}
}

// --- Python parity helpers ---

// tomlKey is _toml_key: bare keys stay bare, everything else is JSON-quoted.
func tomlKey(key string) string {
	if isBareTOMLKey(key) {
		return key
	}
	return PyJSONString(key)
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
		return PyJSONString(t), nil
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
	case *OrderedMap:
		parts := make([]string, 0, len(t.Keys()))
		for _, k := range t.Keys() {
			encoded, err := tomlValue(t.Values()[k])
			if err != nil {
				return "", err
			}
			// Divergence from the Python reference (toml_write.py emits raw
			// keys): Go quotes non-bare inline-table keys so the output is
			// valid TOML and round-trips through tomllib.
			parts = append(parts, tomlKey(k)+" = "+encoded)
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
	case *OrderedMap:
		bv, ok := b.(*OrderedMap)
		if !ok || len(av.Keys()) != len(bv.Keys()) {
			return false
		}
		for _, k := range av.Keys() {
			other, exists := bv.Values()[k]
			if !exists || !pyValueEqual(av.Values()[k], other) {
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
// integer pairs, numeric (float64) otherwise. Accepted corner (recorded in
// odd/tasks/go-toml-writer.md): int-vs-float equality beyond float64
// precision (int > 2^53 against a float) is approximated by the float64
// comparison — a corner recipe config values cannot reach in practice.
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
	} else if quoted := PyJSONString(key); strings.HasPrefix(rest, quoted) {
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
func nestedGet(m *OrderedMap, path []string) (any, bool) {
	node := any(m)
	for _, part := range path {
		asMap, ok := node.(*OrderedMap)
		if !ok {
			return nil, false
		}
		v, exists := asMap.Get(part)
		if !exists {
			return nil, false
		}
		node = v
	}
	return node, true
}

// nestedSet is _nested_set: set a dotted path, replacing non-table
// intermediates with fresh tables (existing keys keep their position).
func nestedSet(m *OrderedMap, path []string, value any) {
	if len(path) == 0 {
		return
	}
	node := m
	for _, part := range path[:len(path)-1] {
		child, _ := node.Get(part)
		childMap, isMap := child.(*OrderedMap)
		if !isMap {
			childMap = NewOrderedMap()
			node.Set(part, childMap)
		}
		node = childMap
	}
	node.Set(path[len(path)-1], value)
}

func dottedPairsToInlineTable(pairs []recipeConfigDottedPair) *OrderedMap {
	table := NewOrderedMap()
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
func applyDottedUpdates(state *recipeConfigWriteState, grouped map[string][]recipeConfigDottedPair, order []string) error {
	inlineLines := map[string]int{}
	var headerRoots, newRoots []string
	for _, rootKey := range order {
		idx := -1
		for candidate := state.configIdx + 1; candidate < state.blockEnd; candidate++ {
			if _, ok := matchKeyLine(state.lines[candidate], rootKey); ok {
				idx = candidate
				break
			}
		}
		if idx >= 0 {
			inlineLines[rootKey] = idx
			continue
		}
		header := subtableHeader(state.recipeKey, []string{rootKey})
		found := false
		for candidate := state.recipeIdx; candidate < state.regionEnd; candidate++ {
			if pyTrimSpace(state.lines[candidate]) == header {
				found = true
				break
			}
		}
		if found {
			headerRoots = append(headerRoots, rootKey)
		} else {
			newRoots = append(newRoots, rootKey)
		}
	}

	for _, rootKey := range order {
		idx, ok := inlineLines[rootKey]
		if !ok {
			continue
		}
		if err := setInlinePaths(state, idx, rootKey, grouped[rootKey]); err != nil {
			return err
		}
	}

	for _, rootKey := range headerRoots {
		for _, pair := range grouped[rootKey] {
			if err := setHeaderPath(state, pair.path, pair.value); err != nil {
				return err
			}
		}
	}

	if len(newRoots) > 0 {
		blockEnd := configBlockEnd(state.lines, state.configIdx, state.regionEnd)
		var newLines []string
		for _, rootKey := range newRoots {
			encoded, err := tomlValue(dottedPairsToInlineTable(grouped[rootKey]))
			if err != nil {
				return err
			}
			newLines = append(newLines, tomlKey(rootKey)+" = "+encoded+"\n")
		}
		state.lines = insertLines(state.lines, blockEnd, newLines)
	}
	return nil
}

// composeKeyLine rebuilds one `key = value` assignment in place: the encoded
// value, the line's original indent, its TOML-aware inline comment, and its
// trailing newline are all preserved. The three rewrite sites (flat-key
// replacement, inline-table roots, header-table leaves) share this
// composition so a serialized line always has exactly one shape.
func composeKeyLine(indent, key string, value any, comment string, hadNewline bool) (string, error) {
	encoded, err := tomlValue(value)
	if err != nil {
		return "", err
	}
	newline := ""
	if hadNewline {
		newline = "\n"
	}
	return indent + tomlKey(key) + " = " + encoded + comment + newline, nil
}

// setInlinePaths is _set_inline_paths.
func setInlinePaths(state *recipeConfigWriteState, idx int, rootKey string, pairs []recipeConfigDottedPair) error {
	if valueIsMultiline(state.lines[idx]) {
		return fmt.Errorf("cannot replace multiline value for key '%s'", rootKey)
	}
	indent, valueText, comment, err := assignmentParts(state.lines[idx])
	if err != nil {
		return err
	}
	parsed, err := parseInlineTomlValue(valueText)
	if err != nil {
		return fmt.Errorf("cannot read the existing value for '%s': %v", rootKey, err)
	}
	table, ok := parsed.(*OrderedMap)
	if !ok {
		return fmt.Errorf("cannot update '%s': its value is not an inline table", rootKey)
	}
	for _, pair := range pairs {
		nestedSet(table, pair.path[1:], pair.value)
	}
	state.lines[idx], err = composeKeyLine(indent, rootKey, table, comment, strings.HasSuffix(state.lines[idx], "\n"))
	return err
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
		state.lines[idx], err = composeKeyLine(indent, leaf, value, comment, strings.HasSuffix(state.lines[idx], "\n"))
		return err
	}
	composed, err := composeKeyLine("", leaf, value, "", true)
	if err != nil {
		return err
	}
	state.lines = insertLines(state.lines, blockEnd, []string{composed})
	return nil
}

// recipeConfigWriteState carries the mutable line slice and region bounds
// through the dotted-update phase, mirroring the reference's in-place list
// mutation and stale-then-recomputed region indices.
type recipeConfigWriteState struct {
	lines       []string
	recipeIdx   int
	regionEnd   int
	configIdx   int
	blockEnd    int
	recipeKey   string
	manifestDir string
}

// ApplyRecipeConfigWrite is update_recipe_config. It returns whether the
// manifest changed and, for refusals, the exact reference error string.
func ApplyRecipeConfigWrite(manifestPath, recipeID string, values *OrderedMap) (bool, string, error) {
	if values == nil || len(values.Keys()) == 0 {
		return false, "", nil
	}
	manifestDir := filepath.Dir(manifestPath)
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, "", fmt.Errorf("read manifest: %w", err)
	}
	originalText := string(original)

	current, err := readRecipeConfigTable(original, recipeID)
	if err != nil {
		return false, "", err
	}

	plain := newOrderedPairs()
	dotted := newOrderedPairs()
	for _, key := range values.Keys() {
		if strings.Contains(key, ".") {
			dotted.set(key, values.Values()[key])
		} else {
			plain.set(key, values.Values()[key])
		}
	}

	pendingPlain := newOrderedPairs()
	for _, key := range plain.keys {
		value := plain.vals[key]
		cur, ok := current.Get(key)
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
			lines:       lines,
			recipeIdx:   idx,
			recipeKey:   recipeKey,
			manifestDir: manifestDir,
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
				composed, cErr := composeKeyLine(indent, key, value, strings.TrimRight(comment, "\r\n"), strings.HasSuffix(state.lines[candidate], "\n"))
				if cErr != nil {
					return false, cErr.Error(), nil
				}
				state.lines[candidate] = composed
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
			if err := applyDottedUpdates(state, grouped, groupedOrder); err != nil {
				return false, err.Error(), nil
			}
		}
		newText = strings.Join(state.lines, "")
	}

	if err := validateTomlText(newText); err != nil {
		return false, "invalid TOML after config write: " + err.Error(), nil
	}
	if newText != originalText {
		if err := atomicWriteFile(manifestPath, []byte(newText)); err != nil {
			return false, "", fmt.Errorf("write manifest: %w", err)
		}
		return true, "", nil
	}
	return false, "", nil
}

// atomicWriteFile replaces path with data via a temp file in the same
// directory plus rename, so a crash mid-write can never leave a truncated
// manifest behind. On any error before the rename the original file is
// untouched.
func atomicWriteFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ai-specs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func sortedPairKeys(pairs *orderedPairs) []string {
	sorted := append([]string(nil), pairs.keys...)
	sort.Strings(sorted)
	return sorted
}
