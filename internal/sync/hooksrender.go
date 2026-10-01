package sync

// Byte-exact Go port of lib/_internal/hooks-render.py (GO-07.S5).
//
// Scope: library only. hooks-render.py's sole caller is sync-agent.sh:538 and
// the Bash spine keeps shelling out to it until the fan-out moves to Go in S15,
// so there is deliberately no GO_SYNC_STEP_* flag and hooks-render.py stays
// alive. RenderHooks mirrors the script's observable surface for one harness:
// every written file (bytes and modes), stderr warnings, and the exit code.
//
// The script's main() argv handling (usage + exit 2 on argc != 3) stays in Bash
// until S15; a library entry receives its three arguments directly, so there is
// nothing left to reproduce for that leg.
//
// Malformed hook fields are reproduced at the exact access point: a missing key
// Python indexes with hook['key'] is a KeyError, a non-str used in str + x is a
// TypeError, a non-str matcher passed to .split is an AttributeError, and so on
// (see hooksKeyError / hooksTypeConcat / hooksAttrSplit). Each aborts the loop
// with rc 1 exactly where Python would, leaving the same already-written tree.
//
// JSON literal parity (NaN/Infinity, lone surrogates, big ints, depth) and the
// json.dumps writer come from the S4 machinery in mcprender.go. Only the
// sort_keys=True variant the settings/hooks writers need is added here.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const hooksManagedKey = "_ai_specs_managed"

// hooksEventMap mirrors EVENT_MAP: abstract event → native event per harness.
// A nil value means the harness has no native mapping (warn-and-skip).
var hooksEventMap = map[string]map[string]any{
	"pre-tool-use": {
		"claude":   "PreToolUse",
		"cursor":   "beforeShellExecution",
		"opencode": "tool.execute.before",
		"pi":       "tool_call",
		"omp":      "tool_call",
	},
	"post-tool-use": {
		"claude":   "PostToolUse",
		"cursor":   "afterShellExecution",
		"opencode": "tool.execute.after",
		"pi":       "tool_result",
		"omp":      "tool_result",
	},
	"session-start": {
		"claude":   "SessionStart",
		"cursor":   "sessionStart",
		"opencode": nil,
		"pi":       "session_start",
		"omp":      "session_start",
	},
	"stop": {
		"claude":   "Stop",
		"cursor":   "stop",
		"opencode": nil,
		"pi":       "agent_end",
		"omp":      "agent_end",
	},
}

// hooksOMPExtImport mirrors OMP_EXT_IMPORT (omp's ExtensionAPI package).
const hooksOMPExtImport = "@oh-my-pi/pi-coding-agent"

// hooksFileWriteTokens mirrors _FILE_WRITE_TOKENS.
var hooksFileWriteTokens = map[string]bool{
	"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true,
}

// hooksJSONDumpsSorted renders v exactly like json.dumps(v, indent=2,
// sort_keys=True): the S4 indent writer with object keys sorted. Python sorts
// str keys by code point; UTF-8 byte order matches code point order, so
// sort.Strings is exact (WTF-8 lone surrogates keep that order too).
func hooksJSONDumpsSorted(v any) string {
	var sb strings.Builder
	hooksWriteJSONIndentSorted(&sb, v, 0)
	return sb.String()
}

func hooksWriteJSONIndentSorted(sb *strings.Builder, v any, depth int) {
	switch x := v.(type) {
	case *mcpObj:
		if x == nil || x.len() == 0 {
			sb.WriteString("{}")
			return
		}
		keys := append([]string(nil), x.keys...)
		sort.Strings(keys)
		mcpWriteJSONBlock(sb, "{", "}", len(keys), depth, func(i int) {
			sb.WriteString(mcpJSONString(keys[i]))
			sb.WriteString(": ")
			hooksWriteJSONIndentSorted(sb, x.vals[keys[i]], depth+1)
		})
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		mcpWriteJSONBlock(sb, "[", "]", len(x), depth, func(i int) {
			hooksWriteJSONIndentSorted(sb, x[i], depth+1)
		})
	default:
		mcpWriteJSONCompact(sb, v)
	}
}

// hooksJSONDumpsCompact renders v like json.dumps(v) with no indent and the
// default ", " / ": " separators. The S4 compact writer only covers scalars, so
// composites are handled here (the TS `MATCHER` literal can be any JSON value
// through the matcher-or-empty-string json.dumps call).
func hooksJSONDumpsCompact(v any) string {
	var sb strings.Builder
	hooksWriteJSONCompact(&sb, v)
	return sb.String()
}

func hooksWriteJSONCompact(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case *mcpObj:
		if x == nil {
			sb.WriteString("null")
			return
		}
		sb.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(mcpJSONString(k))
			sb.WriteString(": ")
			hooksWriteJSONCompact(sb, x.vals[k])
		}
		sb.WriteByte('}')
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			hooksWriteJSONCompact(sb, e)
		}
		sb.WriteByte(']')
	default:
		mcpWriteJSONCompact(sb, v)
	}
}

// hooksPyType names a value the way Python exception messages do.
func hooksPyType(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case int, int64, mcpBigInt:
		return "int"
	case float64:
		return "float"
	case []any:
		return "list"
	case *mcpObj:
		return "dict"
	default:
		return "object"
	}
}

func hooksKeyError(key string) error { return fmt.Errorf("KeyError: '%s'", key) }
func hooksUnhashable(v any) error {
	return fmt.Errorf("TypeError: unhashable type: '%s'", hooksPyType(v))
}
func hooksAttrSplit(v any) error {
	return fmt.Errorf("AttributeError: '%s' object has no attribute 'split'", hooksPyType(v))
}
func hooksAttrItems(v any) error {
	return fmt.Errorf("AttributeError: '%s' object has no attribute 'items'", hooksPyType(v))
}
func hooksTypeConcat(v any) error {
	return fmt.Errorf("TypeError: can only concatenate str (not %q) to str", hooksPyType(v))
}

// hooksGet mirrors hook.get(key): nil when the key is absent.
func hooksGet(h *mcpObj, key string) any {
	if h == nil {
		return nil
	}
	v, _ := h.get(key)
	return v
}

// hooksItem mirrors hook[key] presence: (value, true) only when the key exists.
func hooksItem(h *mcpObj, key string) (any, bool) {
	if h == nil {
		return nil, false
	}
	return h.get(key)
}

// hooksGetDefault mirrors hook.get(key, def): the default is used only when the
// key is absent (a present null stays null, like Python).
func hooksGetDefault(h *mcpObj, key, def string) string {
	v, ok := hooksItem(h, key)
	if !ok {
		return def
	}
	return pyStr(v)
}

// hooksGetDefaultValue mirrors hook.get(key, def) with the raw value (used where
// a non-str default participates in Python's truthiness/iteration).
func hooksGetDefaultValue(h *mcpObj, key string) any {
	v, ok := hooksItem(h, key)
	if !ok {
		return ""
	}
	return v
}

// hooksRequirePyStr mirrors hook[key] evaluated in an f-string / str() context:
// a missing key is a KeyError, any present value is str()-coerced.
func hooksRequirePyStr(h *mcpObj, key string) (string, error) {
	v, ok := hooksItem(h, key)
	if !ok {
		return "", hooksKeyError(key)
	}
	return pyStr(v), nil
}

// hooksRequireStr mirrors hook[key] used in `str_value + hook[key]`: a missing
// key is a KeyError, a present non-str is a TypeError.
func hooksRequireStr(h *mcpObj, key string) (string, error) {
	v, ok := hooksItem(h, key)
	if !ok {
		return "", hooksKeyError(key)
	}
	s, ok := v.(string)
	if !ok {
		return "", hooksTypeConcat(v)
	}
	return s, nil
}

// hooksShimParts mirrors _managed_id/_shim_basename's `hook['recipe']` and
// `hook['id']` f-string accesses.
func hooksShimParts(h *mcpObj) (string, string, error) {
	recipe, err := hooksRequirePyStr(h, "recipe")
	if err != nil {
		return "", "", err
	}
	id, err := hooksRequirePyStr(h, "id")
	if err != nil {
		return "", "", err
	}
	return recipe, id, nil
}

// hooksManagedID mirrors _managed_id.
func hooksManagedID(h *mcpObj) (string, error) {
	recipe, id, err := hooksShimParts(h)
	if err != nil {
		return "", err
	}
	return "ai-specs:hooks:" + recipe + ":" + id, nil
}

// hooksShimBasename mirrors _shim_basename.
func hooksShimBasename(h *mcpObj) (string, error) {
	recipe, id, err := hooksShimParts(h)
	if err != nil {
		return "", err
	}
	return recipe + "-" + id, nil
}

// hooksWarn mirrors _warn: prints "  ! <msg>" to stderr and returns msg.
func hooksWarn(stderr io.Writer, msg string) string {
	fmt.Fprintf(stderr, "  ! %s\n", msg)
	return msg
}

// hooksMatcherTargetsFileWrites mirrors _matcher_targets_file_writes: a falsy
// matcher is False, a truthy non-str is Python's .split AttributeError.
func hooksMatcherTargetsFileWrites(matcher any) (bool, error) {
	if !mcpJSONTruthy(matcher) {
		return false, nil
	}
	s, ok := matcher.(string)
	if !ok {
		return false, hooksAttrSplit(matcher)
	}
	for _, p := range strings.Split(s, "|") {
		if hooksFileWriteTokens[pyStrip(p)] {
			return true, nil
		}
	}
	return false, nil
}

// hooksEnvDict mirrors `env = hook.get("env") or {}` where the script then uses
// env as a dict (claude's env.items() and the TS adapters' sorted(env.items())).
// The bool reports whether Python's `if env:` was truthy.
func hooksEnvDict(h *mcpObj) (*mcpObj, bool, error) {
	v := hooksGet(h, "env")
	if !mcpJSONTruthy(v) {
		return newMCPObj(), false, nil
	}
	o, ok := v.(*mcpObj)
	if !ok {
		return nil, false, hooksAttrItems(v)
	}
	return o, true, nil
}

// hooksEnvAssignments mirrors _env_assignments: KEY="value" pairs for a dict,
// but the same operations (sorted(env), env[k]) for whatever else the field
// held — a list of valid int indices is tolerated, everything else raises.
func hooksEnvAssignments(env any) (string, error) {
	if !mcpJSONTruthy(env) {
		return "", nil
	}
	switch x := env.(type) {
	case *mcpObj:
		keys := append([]string(nil), x.keys...)
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			v := strings.ReplaceAll(pyStr(x.vals[k]), `"`, `\"`)
			parts = append(parts, k+`="`+v+`"`)
		}
		return strings.Join(parts, " "), nil
	case []any:
		return hooksEnvAssignmentsList(x)
	case string:
		// sorted(str) yields characters; str[char] is a TypeError.
		return "", fmt.Errorf("TypeError: string indices must be integers")
	default:
		return "", fmt.Errorf("TypeError: '%s' object is not iterable", hooksPyType(env))
	}
}

// hooksEnvAssignmentsList mirrors `for k in sorted(list): str(list[k])`.
func hooksEnvAssignmentsList(list []any) (string, error) {
	sorted, err := hooksPySorted(list)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(sorted))
	for _, k := range sorted {
		if _, ok := k.(mcpBigInt); ok {
			// CPython fits the index into an ssize_t before the range check; a
			// value wider than int64 always overflows, so the fit error wins.
			return "", fmt.Errorf("IndexError: cannot fit 'int' into an index-sized integer")
		}
		idx, ok := hooksPyIntIndex(k)
		if !ok {
			return "", fmt.Errorf("TypeError: list indices must be integers or slices, not %s", hooksPyType(k))
		}
		if idx < -len(list) || idx >= len(list) {
			return "", fmt.Errorf("IndexError: list index out of range")
		}
		if idx < 0 {
			idx += len(list)
		}
		v := strings.ReplaceAll(pyStr(list[idx]), `"`, `\"`)
		parts = append(parts, pyStr(k)+`="`+v+`"`)
	}
	return strings.Join(parts, " "), nil
}

// hooksPySorted mirrors Python's sorted() for the JSON scalar types.
func hooksPySorted(list []any) ([]any, error) {
	out := append([]any(nil), list...)
	var cerr error
	sort.SliceStable(out, func(i, j int) bool {
		if cerr != nil {
			return false
		}
		c, err := hooksPyCompare(out[i], out[j])
		if err != nil {
			cerr = err
			return false
		}
		return c < 0
	})
	if cerr != nil {
		return nil, cerr
	}
	return out, nil
}

func hooksPyNumeric(v any) (float64, bool) {
	switch x := v.(type) {
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func hooksPyCompare(a, b any) (int, error) {
	// An arbitrary-precision mcpBigInt must compare numerically and exactly:
	// float64 rounding can flip an order (10**20+1 == 1e20 as float64, but
	// 10**20+1 > 1e20 exactly).
	if _, ok := a.(mcpBigInt); ok {
		return hooksBigIntCompare(a, b)
	}
	if _, ok := b.(mcpBigInt); ok {
		return hooksBigIntCompare(a, b)
	}
	if an, ok := hooksPyNumeric(a); ok {
		if bn, ok := hooksPyNumeric(b); ok {
			switch {
			case an < bn:
				return -1, nil
			case an > bn:
				return 1, nil
			default:
				return 0, nil
			}
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs), nil
	}
	return 0, hooksCompareTypeError(a, b)
}

// hooksPyRat returns the exact rational value of a JSON number, or false for a
// NaN/Inf float or a non-number.
func hooksPyRat(v any) (*big.Rat, bool) {
	switch x := v.(type) {
	case mcpBigInt:
		r, ok := new(big.Rat).SetString(string(x))
		return r, ok
	case bool:
		if x {
			return new(big.Rat).SetInt64(1), true
		}
		return new(big.Rat).SetInt64(0), true
	case int:
		return new(big.Rat).SetInt64(int64(x)), true
	case int64:
		return new(big.Rat).SetInt64(x), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(x), true
	}
	return nil, false
}

// hooksBigIntCompare mirrors Python's exact ordering when one side is an
// mcpBigInt: it swaps so the big int is on the left and negates, so int/float
// comparisons stay exact (never rounded through float64). A NaN compares equal
// (Python: both < and > are False) and ±Inf follow Python.
func hooksBigIntCompare(a, b any) (int, error) {
	sign := 1
	if _, ok := a.(mcpBigInt); !ok {
		a, b = b, a
		sign = -1
	}
	ar, ok := hooksPyRat(a)
	if !ok {
		return 0, hooksCompareTypeError(a, b)
	}
	if y, isFloat := b.(float64); isFloat && (math.IsNaN(y) || math.IsInf(y, 0)) {
		switch {
		case math.IsNaN(y):
			return 0, nil
		case math.IsInf(y, 1):
			return -sign, nil
		default:
			return sign, nil
		}
	}
	br, ok := hooksPyRat(b)
	if !ok {
		return 0, hooksCompareTypeError(a, b)
	}
	return sign * ar.Cmp(br), nil
}

func hooksCompareTypeError(a, b any) error {
	return fmt.Errorf("TypeError: '<' not supported between instances of '%s' and '%s'",
		hooksPyType(a), hooksPyType(b))
}

// hooksPyIntIndex reports whether v is a valid Python list index (int or bool).
func hooksPyIntIndex(v any) (int, bool) {
	switch x := v.(type) {
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case int:
		return x, true
	case int64:
		return int(x), true
	}
	return 0, false
}

// hooksEnvLines mirrors the TS `ENV` initializer: sorted keys, JSON-quoted key
// and str(value), each line indented.
func hooksEnvLines(env *mcpObj, indent string) string {
	if env == nil || env.len() == 0 {
		return ""
	}
	keys := append([]string(nil), env.keys...)
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(indent)
		sb.WriteString(mcpJSONString(k))
		sb.WriteString(": ")
		sb.WriteString(mcpJSONString(pyStr(env.vals[k])))
		sb.WriteString(",\n")
	}
	return sb.String()
}

// hooksModuleScriptDecl mirrors _module_script_decl.
func hooksModuleScriptDecl(scriptPath string) string {
	return "const SCRIPT = fileURLToPath(new URL(\"../../" + scriptPath + "\", import.meta.url));\n"
}

// hooksMatcherLiteral mirrors json.dumps of the matcher, or an empty string.
func hooksMatcherLiteral(h *mcpObj) string {
	v := hooksGet(h, "matcher")
	if !mcpJSONTruthy(v) {
		v = ""
	}
	return hooksJSONDumpsCompact(v)
}

// hooksLoadJSONFile mirrors _load_json_file exactly:
//
//   - a path that is not a regular file → {}
//   - an unreadable file (OSError) → {}
//   - invalid JSON (JSONDecodeError) → {}
//   - a non-object JSON value → {}
//   - invalid UTF-8 (UnicodeDecodeError, not caught by Python) → error
//   - a depth overflow (RecursionError, not caught by Python) → error
func hooksLoadJSONFile(path string) (*mcpObj, error) {
	if !mcpIsFile(path) {
		return newMCPObj(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return newMCPObj(), nil
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s: invalid UTF-8", path)
	}
	v, err := mcpParseJSONOrdered(data)
	if err != nil {
		if errors.Is(err, errMCPJSONTooDeep) {
			return nil, err
		}
		return newMCPObj(), nil
	}
	if o, ok := v.(*mcpObj); ok {
		return o, nil
	}
	return newMCPObj(), nil
}

// hooksWriteJSONFile mirrors _write_json_file: mkdir parents, then
// json.dumps(indent=2, sort_keys=True) + "\n".
func hooksWriteJSONFile(path string, data *mcpObj) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(hooksJSONDumpsSorted(data)+"\n"), 0o666)
}

// hooksWriteText mirrors pathlib.Path.write_text for generated content. Python
// opens the path before encoding either part: a lone surrogate in the PATH
// fails the open (no file created), a lone surrogate in the CONTENT fails after
// the open (the file is left created/truncated empty). json.dumps-escaped
// content never carries a raw surrogate; only module paths, cursor wrappers and
// TS adapters interpolate raw hook fields.
func hooksWriteText(path, content string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return err
		}
	}
	if mcpHasLoneSurrogate(path) {
		return fmt.Errorf("UnicodeEncodeError: 'utf-8' codec can't encode a lone surrogate in the path")
	}
	if mcpHasLoneSurrogate(content) {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return err
		}
		_ = f.Close()
		return fmt.Errorf("UnicodeEncodeError: 'utf-8' codec can't encode a lone surrogate in generated content")
	}
	return os.WriteFile(path, []byte(content), 0o666)
}

// hooksManagedBucket mirrors the idempotent managed-entry replacement shared by
// the claude and cursor writers: keep every entry except a previous managed one
// for this hook id, then append the fresh entry.
func hooksManagedBucket(bucketVal any, managedID string) []any {
	bucket := mcpAsList(bucketVal)
	out := make([]any, 0, len(bucket)+1)
	for _, e := range bucket {
		o, ok := e.(*mcpObj)
		if !ok {
			out = append(out, e)
			continue
		}
		v, ok := o.get(hooksManagedKey)
		if ok {
			if s, ok := v.(string); ok && s == managedID {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// hooksObjOrNew returns container[key] when it is a JSON object, else a new
// object (mirrors `if not isinstance(hooks, dict): hooks = {}`).
func hooksObjOrNew(container *mcpObj, key string) *mcpObj {
	if o, ok := hooksGet(container, key).(*mcpObj); ok {
		return o
	}
	return newMCPObj()
}

// --- Claude (direct, exit-code native) ---------------------------------------

func hooksRenderClaude(h *mcpObj, nativeEvent, projectRoot string, _ io.Writer) error {
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	// _load_json_file runs before _managed_id, so a bad settings file wins over
	// a missing recipe/id.
	settings, err := hooksLoadJSONFile(settingsPath)
	if err != nil {
		return err
	}
	hooks := hooksObjOrNew(settings, "hooks")
	managedID, err := hooksManagedID(h)
	if err != nil {
		return err
	}
	bucket := hooksManagedBucket(hooksGet(hooks, nativeEvent), managedID)

	entry := newMCPObj()
	entry.set(hooksManagedKey, managedID)
	if mcpJSONTruthy(hooksGet(h, "matcher")) {
		entry.set("matcher", hooksGet(h, "matcher"))
	}
	scriptPath, err := hooksRequireStr(h, "script_path")
	if err != nil {
		return err
	}
	commandHook := newMCPObj()
	commandHook.set("type", "command")
	commandHook.set("command", "$CLAUDE_PROJECT_DIR/"+scriptPath)
	env, needsEnv, err := hooksEnvDict(h)
	if err != nil {
		return err
	}
	if needsEnv {
		envOut := newMCPObj()
		for _, k := range env.keys {
			envOut.set(k, pyStr(env.vals[k]))
		}
		commandHook.set("env", envOut)
	}
	entry.set("hooks", []any{commandHook})
	bucket = append(bucket, entry)

	hooks.set(nativeEvent, bucket)
	settings.set("hooks", hooks)
	return hooksWriteJSONFile(settingsPath, settings)
}

// --- Cursor (shell wrapper, decision via stdout JSON) ------------------------

func hooksRenderCursor(h *mcpObj, nativeEvent, projectRoot string, stderr io.Writer) error {
	// `hook["event"] == "pre-tool-use" and _matcher_targets_file_writes(...)`;
	// a non-str event is merely unequal, so the matcher is never evaluated.
	eventVal, ok := hooksItem(h, "event")
	if !ok {
		return hooksKeyError("event")
	}
	if s, isStr := eventVal.(string); isStr && s == "pre-tool-use" {
		targets, err := hooksMatcherTargetsFileWrites(hooksGetDefaultValue(h, "matcher"))
		if err != nil {
			return err
		}
		if targets {
			// Cursor has no pre-file-write hook: a file-write matcher has no target.
			recipe, err := hooksRequirePyStr(h, "recipe")
			if err != nil {
				return err
			}
			id, err := hooksRequirePyStr(h, "id")
			if err != nil {
				return err
			}
			hooksWarn(stderr, fmt.Sprintf(
				"cursor: no pre-file-write hook exists; skipping hook '%s:%s' (matcher '%s') for cursor",
				recipe, id, hooksGetDefault(h, "matcher", "")))
			return nil
		}
	}

	base, err := hooksShimBasename(h)
	if err != nil {
		return err
	}
	scriptPath, err := hooksRequireStr(h, "script_path")
	if err != nil {
		return err
	}
	script := "$CURSOR_PROJECT_DIR/" + scriptPath
	envPrefix, err := hooksEnvAssignments(hooksGet(h, "env"))
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# GENERATED by ai-specs \u2014 do not edit. Runs the recipe hook script and\n")
	b.WriteString("# maps exit 2 \u2192 Cursor deny (decision channel is stdout JSON, snake_case).\n")
	b.WriteString("script=\"" + script + "\"\n")
	b.WriteString("input=\"$(cat)\"\n")
	b.WriteString("out=\"$(printf %s \"$input\" | ")
	if envPrefix != "" {
		b.WriteString(envPrefix + " ")
	}
	b.WriteString("\"$script\")\"; code=$?\n")
	b.WriteString("if [ \"$code\" = 2 ]; then\n")
	b.WriteString("  printf '{\"permission\":\"deny\",\"agent_message\":%s}' \"$(printf %s \"$out\" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))')\"\n")
	b.WriteString("else\n")
	b.WriteString("  printf '{\"permission\":\"allow\"}'\n")
	b.WriteString("fi\n")
	b.WriteString("exit 0\n")

	wrapperPath := filepath.Join(projectRoot, ".cursor", "hooks", base+".sh")
	if err := hooksWriteText(wrapperPath, b.String()); err != nil {
		return err
	}
	if err := os.Chmod(wrapperPath, 0o755); err != nil {
		return err
	}

	// Reference the wrapper from .cursor/hooks.json under the native event.
	hooksJSONPath := filepath.Join(projectRoot, ".cursor", "hooks.json")
	cfg, err := hooksLoadJSONFile(hooksJSONPath)
	if err != nil {
		return err
	}
	hooks := hooksObjOrNew(cfg, "hooks")
	managedID, err := hooksManagedID(h)
	if err != nil {
		return err
	}
	bucket := hooksManagedBucket(hooksGet(hooks, nativeEvent), managedID)
	entry := newMCPObj()
	entry.set(hooksManagedKey, managedID)
	entry.set("command", "./.cursor/hooks/"+base+".sh")
	bucket = append(bucket, entry)
	hooks.set(nativeEvent, bucket)
	cfg.set("hooks", hooks)
	return hooksWriteJSONFile(hooksJSONPath, cfg)
}

// --- OpenCode (TS plugin, decision via throw) --------------------------------

func hooksRenderOpencode(h *mcpObj, _ string, projectRoot string) error {
	recipe, id, err := hooksShimParts(h)
	if err != nil {
		return err
	}
	env, _, err := hooksEnvDict(h)
	if err != nil {
		return err
	}
	envLines := hooksEnvLines(env, "      ")
	event, err := hooksRequirePyStr(h, "event")
	if err != nil {
		return err
	}
	scriptPath, err := hooksRequirePyStr(h, "script_path")
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("// GENERATED by ai-specs \u2014 do not edit.\n")
	b.WriteString("// Runtime hook " + recipe + ":" + id + " (event " + event + ").\n")
	b.WriteString("// Spawns the single materialized recipe script with the normalized event\n")
	b.WriteString("// on stdin; exit 2 -> throw (block). NOTE: tool.execute.before does NOT\n")
	b.WriteString("// fire for subagent (#5894) or MCP (#2319) tool calls.\n")
	b.WriteString("import { spawnSync } from \"node:child_process\";\n")
	b.WriteString("import { fileURLToPath } from \"node:url\";\n")
	b.WriteString("import { isAbsolute } from \"node:path\";\n")
	b.WriteString("import { statSync } from \"node:fs\";\n\n")
	b.WriteString("// The materialized launcher resolves beside this generated module at\n")
	b.WriteString("// runtime, so relocating the plugin keeps working from any process cwd.\n")
	b.WriteString(hooksModuleScriptDecl(scriptPath))
	b.WriteString("const MATCHER = " + hooksMatcherLiteral(h) + ";\n")
	b.WriteString("const ENV = {\n")
	b.WriteString(envLines)
	b.WriteString("};\n\n")
	b.WriteString("// One deterministic value drives both the event cwd and the child\n")
	b.WriteString("// process cwd: a string, absolute, existing directory wins after outer\n")
	b.WriteString("// whitespace is trimmed; anything else falls back to the process cwd.\n")
	b.WriteString("const normalizeDirectory = (raw) => {\n")
	b.WriteString("  if (typeof raw !== \"string\") return process.cwd();\n")
	b.WriteString("  const trimmed = raw.trim();\n")
	b.WriteString("  if (trimmed === \"\" || !isAbsolute(trimmed)) return process.cwd();\n")
	b.WriteString("  let isDir = false;\n")
	b.WriteString("  try { isDir = statSync(trimmed).isDirectory(); } catch { isDir = false; }\n")
	b.WriteString("  if (!isDir) return process.cwd();\n")
	b.WriteString("  return trimmed;\n")
	b.WriteString("};\n\n")
	b.WriteString("export const plugin = async ({ project, directory }) => {\n")
	b.WriteString("  return {\n")
	b.WriteString("    \"tool.execute.before\": async (input, output) => {\n")
	b.WriteString("      const toolName = input?.tool ?? output?.tool ?? \"\";\n")
	b.WriteString("      if (MATCHER) {\n")
	b.WriteString("        // Case-insensitive: OpenCode tool ids may be lowercase while\n")
	b.WriteString("        // recipe matchers use Claude-style names (Write, Edit, Bash).\n")
	b.WriteString("        const re = new RegExp(`^(?:${MATCHER})$`, \"i\");\n")
	b.WriteString("        if (!re.test(toolName)) return;\n")
	b.WriteString("      }\n")
	b.WriteString("      const cwd = normalizeDirectory(directory);\n")
	b.WriteString("      const event = {\n")
	b.WriteString("        event: \"pre-tool-use\",\n")
	b.WriteString("        tool_name: toolName,\n")
	b.WriteString("        tool_input: output?.args ?? {},\n")
	b.WriteString("        cwd,\n")
	b.WriteString("      };\n")
	b.WriteString("      let res;\n")
	b.WriteString("      try {\n")
	b.WriteString("        res = spawnSync(SCRIPT, {\n")
	b.WriteString("          input: JSON.stringify(event),\n")
	b.WriteString("          env: { ...process.env, ...ENV },\n")
	b.WriteString("          cwd,\n")
	b.WriteString("          encoding: \"utf8\",\n")
	b.WriteString("        });\n")
	b.WriteString("      } catch {\n")
	b.WriteString("        return; // thrown child-process exception: fail open\n")
	b.WriteString("      }\n")
	b.WriteString("      if (res?.status !== 2) {\n")
	b.WriteString("        // non-2 exit, child spawn error, or missing status: fail open\n")
	b.WriteString("        return;\n")
	b.WriteString("      }\n")
	b.WriteString("      throw new Error(res.stderr || \"blocked by ai-specs runtime hook\");\n")
	b.WriteString("    },\n")
	b.WriteString("  };\n")
	b.WriteString("};\n")

	path := filepath.Join(projectRoot, ".opencode", "plugin", hooksShimName(recipe, id)+".ts")
	return hooksWriteText(path, b.String())
}

// --- Pi / Omp (TS extension, decision via return {block:true}) --------------

func hooksRenderPi(h *mcpObj, _ string, projectRoot string) error {
	return hooksRenderPiExt(h, filepath.Join(projectRoot, ".pi", "extensions"),
		"@earendil-works/pi-coding-agent", "pi")
}

func hooksRenderOmp(h *mcpObj, _ string, projectRoot string) error {
	return hooksRenderPiExt(h, filepath.Join(projectRoot, ".omp", "extensions"),
		hooksOMPExtImport, "omp")
}

// hooksRenderPiExt renders the shared pi/omp TS extension; the two differ only
// in the ExtensionAPI import path and the adapter name in the comment.
func hooksRenderPiExt(h *mcpObj, dir, extImport, adapter string) error {
	recipe, id, err := hooksShimParts(h)
	if err != nil {
		return err
	}
	env, _, err := hooksEnvDict(h)
	if err != nil {
		return err
	}
	envLines := hooksEnvLines(env, "  ")
	event, err := hooksRequirePyStr(h, "event")
	if err != nil {
		return err
	}
	scriptPath, err := hooksRequirePyStr(h, "script_path")
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("// GENERATED by ai-specs \u2014 do not edit.\n")
	b.WriteString("// Runtime hook " + recipe + ":" + id + " (event " + event + ").\n")
	b.WriteString("import type { ExtensionAPI } from \"" + extImport + "\";\n")
	b.WriteString("import { spawnSync } from \"node:child_process\";\n")
	b.WriteString("import { fileURLToPath } from \"node:url\";\n\n")
	b.WriteString("// The materialized launcher resolves beside this generated module at\n")
	b.WriteString("// runtime; the " + adapter + " event context stays process cwd (no workspace root\n")
	b.WriteString("// is claimed by this adapter).\n")
	b.WriteString(hooksModuleScriptDecl(scriptPath))
	b.WriteString("const MATCHER = " + hooksMatcherLiteral(h) + ";\n")
	b.WriteString("const ENV: Record<string, string> = {\n")
	b.WriteString(envLines)
	b.WriteString("};\n\n")
	b.WriteString("export default function (pi: ExtensionAPI) {\n")
	b.WriteString("  pi.on(\"tool_call\", (call: any) => {\n")
	b.WriteString("    const toolName = call?.toolName ?? call?.tool_name ?? call?.name ?? \"\";\n")
	b.WriteString("    if (MATCHER) {\n")
	b.WriteString("      // Case-insensitive: pi/omp tool names are lowercase (write, edit)\n")
	b.WriteString("      // while matchers use Claude-style names (Write, Edit).\n")
	b.WriteString("      const re = new RegExp(`^(?:${MATCHER})$`, \"i\");\n")
	b.WriteString("      if (!re.test(toolName)) return;\n")
	b.WriteString("    }\n")
	b.WriteString("    // pi/omp tool input uses `path`; normalize to `file_path` so hook\n")
	b.WriteString("    // scripts written against the Claude event shape keep working.\n")
	b.WriteString("    const rawInput = call?.input ?? call?.arguments ?? {};\n")
	b.WriteString("    const event = {\n")
	b.WriteString("      event: \"pre-tool-use\",\n")
	b.WriteString("      tool_name: toolName,\n")
	b.WriteString("      tool_input: { ...rawInput, file_path: rawInput.file_path ?? rawInput.path ?? rawInput.notebook_path },\n")
	b.WriteString("      cwd: process.cwd(),\n")
	b.WriteString("    };\n")
	b.WriteString("    const res = spawnSync(SCRIPT, {\n")
	b.WriteString("      input: JSON.stringify(event),\n")
	b.WriteString("      env: { ...process.env, ...ENV },\n")
	b.WriteString("      encoding: \"utf8\",\n")
	b.WriteString("    });\n")
	b.WriteString("    if (res.status === 2) {\n")
	b.WriteString("      return { block: true, reason: res.stderr || \"blocked by ai-specs runtime hook\" };\n")
	b.WriteString("    }\n")
	b.WriteString("    // any other exit code: fail-open (allow).\n")
	b.WriteString("  });\n")
	b.WriteString("}\n")

	path := filepath.Join(dir, hooksShimName(recipe, id)+".ts")
	return hooksWriteText(path, b.String())
}

func hooksShimName(recipe, id string) string { return recipe + "-" + id }

// --- Dispatch ----------------------------------------------------------------

// RenderHooks is the byte-exact port of hooks-render.py's render()/main() body
// for a library entry point. It renders every applicable hook for one agent,
// returning the process exit code and writing the same files, stderr warnings
// and stdout the script would. stdout is always empty (the script only warns on
// stderr); the argv/usage/exit-2 contract of main() stays in Bash until S15.
func RenderHooks(resolvedHooksPath, agent, projectRoot string, stdout, stderr io.Writer) int {
	data, err := hooksLoadJSONFile(resolvedHooksPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	hooksVal, _ := data.get("hooks")
	hooks, ok := hooksVal.([]any)
	if !ok {
		return 0
	}

	for _, hv := range hooks {
		h, ok := hv.(*mcpObj)
		if !ok {
			continue
		}
		eventVal, present := h.get("event")
		event := ""
		if present {
			switch eventVal.(type) {
			case []any, *mcpObj:
				// EVENT_MAP.get(unhashable) raises TypeError.
				fmt.Fprintf(stderr, "error: %s\n", hooksUnhashable(eventVal))
				return 1
			}
			event = pyStr(eventVal)
		}
		mapping, known := hooksEventMap[event]
		if !known {
			hooksWarn(stderr, fmt.Sprintf(
				"unknown abstract event '%s' for hook '%s:%s' \u2014 skipped",
				event, pyStr(hooksGet(h, "recipe")), pyStr(hooksGet(h, "id"))))
			continue
		}
		native := mapping[agent]
		if native == nil {
			hooksWarn(stderr, fmt.Sprintf(
				"event '%s' has no native mapping for harness '%s' (recipe '%s', hook '%s') \u2014 skipped",
				event, agent, pyStr(hooksGet(h, "recipe")), pyStr(hooksGet(h, "id"))))
			continue
		}
		nativeEvent, _ := native.(string)

		var werr error
		switch agent {
		case "claude":
			werr = hooksRenderClaude(h, nativeEvent, projectRoot, stderr)
		case "cursor":
			werr = hooksRenderCursor(h, nativeEvent, projectRoot, stderr)
		case "opencode":
			werr = hooksRenderOpencode(h, nativeEvent, projectRoot)
		case "pi":
			werr = hooksRenderPi(h, nativeEvent, projectRoot)
		case "omp":
			werr = hooksRenderOmp(h, nativeEvent, projectRoot)
		default:
			hooksWarn(stderr, fmt.Sprintf(
				"harness '%s' has no runtime-hook renderer (recipe '%s', hook '%s') \u2014 skipped",
				agent, pyStr(hooksGet(h, "recipe")), pyStr(hooksGet(h, "id"))))
		}
		if werr != nil {
			fmt.Fprintf(stderr, "error: %s\n", werr)
			return 1
		}
	}
	return 0
}
