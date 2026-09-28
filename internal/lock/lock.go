// Package lock is the Go port of lib/_internal/lock.py: shared lock-file
// read/write helpers for ai-specs. It preserves the Python reference's
// byte-exact emission, its normalization semantics, and its atomicity
// guarantees (temp file + rename inside the lock's parent directory).
//
// Conventions mirror the worktree-gate module's lockwrite.go (the byte-exact
// port of the same writer), but this package is self-contained: the gate
// module is a separate Go module and is never imported here. The go_write_lock
// Python→Go bridge in lock.py is deliberate glue that disappears in the
// single binary and is NOT ported; this package is what the binary calls.
package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/toml"
)

// pyReprMaxDepth pins the Python recursion boundary for str()/repr() of
// nested values. Probe (Python 3.14.7, default 8 MiB stack): repr() of a
// nested list succeeds to depth ~69709 and raises RecursionError at ~69710.
// The cap sits above that failure point so Go never errors where Python
// succeeds on the probe machine, and returns a descriptive error instead of
// crashing at extreme depths. Self-referential *toml.Table sharing (not
// producible by toml.Parse, which builds trees) renders Python's ellipsis
// form instead.
const pyReprMaxDepth = 100000

// LockHeader is LOCK_HEADER from lib/_internal/lock.py, copied
// character-for-character. TestLockHeaderByteIdentity pins it.
const LockHeader = `# Managed by ai-specs. Do not edit by hand.
# Provenance stamp: [meta] records the CLI version and timestamp of the last
# sync. [managed.*] records integrity only for CLI-owned override targets;
# it is not a general content-integrity manifest. git covers the committed
# project surface; dep content hashes ([deps.*]) are tracked for drift
# detection; recipe/skill content hashes are not tracked.
`

// Lock is the in-memory lock shape, mirroring load_lock's returned dict.
// Table values are kept verbatim (map[string]any); emission sorts keys.
type Lock struct {
	// Skills is the legacy [skills] section, verbatim dicts.
	Skills map[string]map[string]any
	// Meta keeps only cli_version/synced_at, stripped non-blank strings.
	Meta map[string]string
	// Recipes/Deps are normalized to owner → skill → files
	// (_load_skill_groups); file values stay verbatim.
	Recipes map[string]map[string]map[string]any
	Deps    map[string]map[string]map[string]any
	// Agents is the [agents] section, verbatim dicts.
	Agents map[string]map[string]any
	// Managed entries are verbatim maps kept only when sha256 is a
	// non-blank string (stripped).
	Managed map[string]map[string]any
}

// NewLock returns a Lock with all six maps initialized empty (the shape
// load_lock returns for a missing file).
func NewLock() *Lock {
	return &Lock{
		Skills:  map[string]map[string]any{},
		Meta:    map[string]string{},
		Recipes: map[string]map[string]map[string]any{},
		Deps:    map[string]map[string]map[string]any{},
		Agents:  map[string]map[string]any{},
		Managed: map[string]map[string]any{},
	}
}

// Sha256Bytes hashes normalized bytes so CRLF/LF line endings are equivalent
// (sha256_bytes).
func Sha256Bytes(data []byte) string {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

// Sha256OfFile hashes the file's normalized bytes (sha256_of).
func Sha256OfFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return Sha256Bytes(data), nil
}

// TOMLString is _toml_string: escape the backslash first, then the double
// quote. Control characters are emitted raw — never \n, \t or any other
// escape sequence.
func TOMLString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// HasControlChar reports whether s contains an ASCII control character
// (below 0x20, or DEL 0x7f) — the same boundary the Python writer enforces.
func HasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// LoadLock is load_lock: a missing (or non-regular) path returns the
// zero-value lock with all six empty maps; otherwise the file is parsed and
// normalized. Meta keeps only cli_version/synced_at (stripped non-blank
// strings); managed entries keep every field verbatim and survive only when
// sha256 is a non-blank string (stripped); skills and agents stay verbatim
// dicts; recipes/deps go through the _load_skill_groups shape
// (owner → skill → files).
func LoadLock(lockPath string) (*Lock, error) {
	if info, err := os.Stat(lockPath); err != nil || !info.Mode().IsRegular() {
		// Path.is_file() parity: any stat failure or non-file is "missing".
		return NewLock(), nil
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}
	root, err := toml.Parse(data)
	if err != nil {
		return nil, err
	}

	lock := NewLock()

	// meta: only cli_version then synced_at, non-blank strings, stripped.
	if metaRaw, ok := root.Table("meta"); ok {
		for _, key := range []string{"cli_version", "synced_at"} {
			if v, ok := metaRaw.String(key); ok {
				if s := strings.TrimSpace(v); s != "" {
					lock.Meta[key] = s
				}
			}
		}
	}

	// managed: verbatim entries, kept only when sha256 is a non-blank
	// string, which is then stripped.
	if managedRaw, ok := root.Table("managed"); ok {
		for _, path := range managedRaw.Keys() {
			v, _ := managedRaw.Get(path)
			entryTable, ok := v.(*toml.Table)
			if !ok {
				continue
			}
			entry := entryTable.Any()
			sha, ok := entry["sha256"].(string)
			if !ok {
				continue
			}
			if s := strings.TrimSpace(sha); s != "" {
				entry["sha256"] = s
				lock.Managed[path] = entry
			}
		}
	}

	// skills and agents: verbatim dicts ({k: dict(v)} parity).
	copyVerbatimDicts(root, "skills", lock.Skills)
	copyVerbatimDicts(root, "agents", lock.Agents)

	// recipes and deps: _load_skill_groups normalization.
	lock.Recipes = loadSkillGroups(root, "recipes")
	lock.Deps = loadSkillGroups(root, "deps")

	return lock, nil
}

// copyVerbatimDicts copies section entries whose value is a table into dst
// as plain maps ({k: dict(v)} parity; non-table values have no dict shape).
func copyVerbatimDicts(root *toml.Table, key string, dst map[string]map[string]any) {
	section, ok := root.Table(key)
	if !ok {
		return
	}
	for _, name := range section.Keys() {
		v, _ := section.Get(name)
		tbl, ok := v.(*toml.Table)
		if !ok {
			continue
		}
		dst[name] = tbl.Any()
	}
}

// loadSkillGroups is _load_skill_groups: owner → skill name → dict of files.
func loadSkillGroups(root *toml.Table, key string) map[string]map[string]map[string]any {
	result := map[string]map[string]map[string]any{}
	section, ok := root.Table(key)
	if !ok {
		return result
	}
	for _, ownerID := range section.Keys() {
		ownerVal, _ := section.Get(ownerID)
		owner, ok := ownerVal.(*toml.Table)
		if !ok {
			continue
		}
		skillsTable, ok := owner.Table("skills")
		if !ok {
			result[ownerID] = map[string]map[string]any{}
			continue
		}
		files := map[string]map[string]any{}
		for _, skillName := range skillsTable.Keys() {
			v, _ := skillsTable.Get(skillName)
			tbl, ok := v.(*toml.Table)
			if !ok {
				continue
			}
			files[skillName] = tbl.Any()
		}
		result[ownerID] = files
	}
	return result
}

// pyStr mirrors the Python writer's str() plus its `value is not None and
// value != ""` truthiness (the bool return): nil and empty strings are
// falsy, everything else stringifies. bool → True/False, int64 → decimal,
// float64 → str(float) shortest form, list/dict → repr composition.
// An error means the nesting exceeds pyReprMaxDepth (Python raises
// RecursionError inside str()).
func pyStr(v any) (string, bool, error) {
	switch x := v.(type) {
	case nil:
		return "", false, nil
	case string:
		return x, x != "", nil
	case bool:
		if x {
			return "True", true, nil
		}
		return "False", true, nil
	case int64:
		return strconv.FormatInt(x, 10), true, nil
	case float64:
		return pyFloatStr(x), true, nil
	default:
		s, err := pyReprDepth(v, 0, nil)
		return s, true, err
	}
}

// pyTruthy mirrors Python truthiness for the sha256 entry gate
// (`entry.get("sha256")` plain truthiness, which skips False/0/[]/{}).
func pyTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case []*toml.Table:
		return len(x) > 0
	case *toml.Table:
		return x != nil && len(x.Keys()) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// pyRepr mirrors Python repr() for the composite value kinds reachable from
// TOML (str(list) == repr(list) composition).
func pyRepr(v any) (string, error) {
	return pyReprDepth(v, 0, nil)
}

// pyReprDepth is the shared recursion core: depth-capped (descriptive error
// instead of a stack overflow) and cycle-aware for *toml.Table (Python's
// '{...}' ellipsis for a table re-entered on the active path).
func pyReprDepth(v any, depth int, active map[*toml.Table]bool) (string, error) {
	if depth > pyReprMaxDepth {
		return "", fmt.Errorf("value nesting exceeds the maximum recursion depth (%d)", pyReprMaxDepth)
	}
	switch x := v.(type) {
	case string:
		return pyReprString(x), nil
	case nil:
		return "None", nil
	case bool:
		if x {
			return "True", nil
		}
		return "False", nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		return pyFloatStr(x), nil
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			p, err := pyReprDepth(e, depth+1, active)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case []*toml.Table:
		parts := make([]string, len(x))
		for i, e := range x {
			p, err := pyReprDepth(e, depth+1, active)
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case *toml.Table:
		if x == nil {
			return "{}", nil
		}
		if active[x] {
			return "{...}", nil
		}
		if active == nil {
			active = make(map[*toml.Table]bool)
		}
		active[x] = true
		defer delete(active, x)
		tableKeys := x.Keys()
		parts := make([]string, 0, len(tableKeys))
		for _, k := range tableKeys {
			val, _ := x.Get(k)
			p, err := pyReprDepth(val, depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case map[string]any:
		// ponytail: maps normalized by Table.Any() lose document order; emit
		// in sorted key order — byte-identical to Python's insertion order
		// whenever the producer wrote keys in sorted order.
		mapKeys := make([]string, 0, len(x))
		for k := range x {
			mapKeys = append(mapKeys, k)
		}
		sort.Strings(mapKeys)
		parts := make([]string, 0, len(mapKeys))
		for _, k := range mapKeys {
			p, err := pyReprDepth(x[k], depth+1, active)
			if err != nil {
				return "", err
			}
			parts = append(parts, pyReprString(k)+": "+p)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	default:
		return pyReprString(fmt.Sprintf("%v", v)), nil
	}
}

// pyReprString mirrors Python repr() for strings: quote switches to '"' when
// the value contains ' but no "; \\ \n \r \t and the active quote are escaped;
// every non-printable character becomes \\xNN / \\uNNNN / \\UNNNNNNNN (lowercase
// hex). Probe-pinned against Python 3.14 repr(): chars < 0x20 and 0x7f-0x9f
// emit \\xNN (e.g. '\\x85'); printable non-ASCII like é and non-BMP emoji stay
// raw; NBSP and U+2028 are non-printable and are escaped.
func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case rune(quote):
			sb.WriteByte('\\')
			sb.WriteByte(quote)
		default:
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || !unicode.IsPrint(r) {
				sb.WriteString(pyUnicodeEscape(r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}

// pyUnicodeEscape renders one non-printable rune in Python repr escape form
// (lowercase hex, \x for < 0x100, \u for the BMP, \U beyond).
func pyUnicodeEscape(r rune) string {
	const hex = "0123456789abcdef"
	digits := func(n int) string {
		out := make([]byte, n)
		for i := n - 1; i >= 0; i-- {
			out[i] = hex[r&0xf]
			r >>= 4
		}
		return string(out)
	}
	switch {
	case r < 0x100:
		return `\x` + digits(2)
	case r < 0x10000:
		return `\u` + digits(4)
	default:
		return `\U` + digits(8)
	}
}

// pyFloatStr renders f like Python str(float) (shortest round-trip repr):
// scientific notation when the decimal point position is <= -4 or > 16,
// otherwise decimal notation with a forced ".0" on integral values.
// Replicated from internal/config's formatPyFloat, which is outside this
// package's allowed edit surface.
func pyFloatStr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	neg := math.Signbit(f)
	a := math.Abs(f)
	if a == 0 {
		if neg {
			return "-0.0"
		}
		return "0.0"
	}
	e := strconv.FormatFloat(a, 'e', -1, 64) // e.g. "1.5e+22"
	marker := strings.IndexByte(e, 'e')
	digits := strings.Replace(e[:marker], ".", "", 1)
	exp10, _ := strconv.Atoi(e[marker+1:])
	decpt := exp10 + 1

	var s string
	if decpt <= -4 || decpt > 16 {
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		expStr := strconv.Itoa(exp10)
		if exp10 < 0 {
			expStr = strconv.Itoa(-exp10)
		}
		if len(expStr) < 2 {
			expStr = "0" + expStr
		}
		signChar := byte('+')
		if exp10 < 0 {
			signChar = '-'
		}
		s = mant + "e" + string(signChar) + expStr
	} else {
		switch {
		case decpt <= 0:
			s = "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= len(digits):
			s = digits + strings.Repeat("0", decpt-len(digits)) + ".0"
		default:
			s = digits[:decpt] + "." + digits[decpt:]
		}
	}
	if neg {
		s = "-" + s
	}
	return s
}

// refuseControlChars is _refuse_control_chars: walk every string that reaches
// the emitted surface in the exact deterministic order the Python walks it
// (lock_path, meta, managed sorted, agents sorted, deps sorted — note agents
// BEFORE deps, even though the emitter writes deps between managed and
// agents) and refuse the first string containing a control character, since
// TOMLString emits control characters raw inside a quoted string.
func refuseControlChars(lockPath string, lock *Lock) error {
	checks := make([][2]string, 0, 16)
	appendCheck := func(locator, value string) {
		checks = append(checks, [2]string{locator, value})
	}

	appendCheck("lock_path", lockPath)
	for _, key := range []string{"cli_version", "synced_at"} {
		if value := lock.Meta[key]; value != "" {
			appendCheck("meta."+key, value)
		}
	}
	managedPaths := make([]string, 0, len(lock.Managed))
	for path := range lock.Managed {
		managedPaths = append(managedPaths, path)
	}
	sort.Strings(managedPaths)
	for _, path := range managedPaths {
		appendCheck("managed path", path)
		entry := lock.Managed[path]
		if !pyTruthy(entry["sha256"]) {
			continue
		}
		for _, key := range []string{"sha256", "recipe", "source", "kind", "policy"} {
			value, ok, err := pyStr(entry[key])
			if err != nil {
				return err
			}
			if ok {
				appendCheck("managed."+key, value)
			}
		}
	}
	harnesses := make([]string, 0, len(lock.Agents))
	for harness := range lock.Agents {
		harnesses = append(harnesses, harness)
	}
	sort.Strings(harnesses)
	for _, harness := range harnesses {
		appendCheck("agents harness", harness)
		files := lock.Agents[harness]
		if len(files) == 0 {
			continue
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			appendCheck("agents filename", name)
			s, err := pyStrValue(files[name])
			if err != nil {
				return err
			}
			appendCheck("agents hash", s)
		}
	}
	depIDs := make([]string, 0, len(lock.Deps))
	for depID := range lock.Deps {
		depIDs = append(depIDs, depID)
	}
	sort.Strings(depIDs)
	for _, depID := range depIDs {
		appendCheck("deps dep id", depID)
		skills := lock.Deps[depID]
		skillNames := make([]string, 0, len(skills))
		for skill := range skills {
			skillNames = append(skillNames, skill)
		}
		sort.Strings(skillNames)
		for _, skill := range skillNames {
			appendCheck("deps skill name", skill)
			files := skills[skill]
			rels := make([]string, 0, len(files))
			for rel := range files {
				rels = append(rels, rel)
			}
			sort.Strings(rels)
			for _, rel := range rels {
				appendCheck("deps rel", rel)
				s, err := pyStrValue(files[rel])
				if err != nil {
					return err
				}
				appendCheck("deps hash", s)
			}
		}
	}

	for _, check := range checks {
		if HasControlChar(check[1]) {
			return fmt.Errorf("value for %s contains a control character", check[0])
		}
	}
	return nil
}

// pyStrValue is str(v) for the refusal walk, which stringifies
// unconditionally. An error means the nesting exceeds pyReprMaxDepth.
func pyStrValue(v any) (string, error) {
	s, _, err := pyStr(v)
	return s, err
}

// renderLock is the _write_lock_python body: fixed section order
// [meta] → [managed."<path>"] (sorted, falsy-sha256 entries and empty values
// skipped) → [deps."<id>".skills."<skill>"] (sorted, empty file maps
// skipped) → [agents."<harness>"] (sorted, empty maps skipped), assembled as
// "\n".join(lines).rstrip("\n") + "\n". The legacy [recipes] and [skills]
// sections are read but never emitted, on both sides (recorded-defect
// behavior, do not fix). An error means a value's nesting exceeds
// pyReprMaxDepth.
func renderLock(lock *Lock) (string, error) {
	lines := []string{LockHeader}

	if len(lock.Meta) > 0 {
		lines = append(lines, "[meta]")
		if v := lock.Meta["cli_version"]; v != "" {
			lines = append(lines, "cli_version = "+TOMLString(v))
		}
		if v := lock.Meta["synced_at"]; v != "" {
			lines = append(lines, "synced_at = "+TOMLString(v))
		}
		lines = append(lines, "")
	}

	paths := make([]string, 0, len(lock.Managed))
	for path := range lock.Managed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		entry := lock.Managed[path]
		if !pyTruthy(entry["sha256"]) {
			continue
		}
		lines = append(lines, "[managed."+TOMLString(path)+"]")
		for _, key := range []string{"sha256", "recipe", "source", "kind", "policy"} {
			value, ok, err := pyStr(entry[key])
			if err != nil {
				return "", err
			}
			if ok {
				lines = append(lines, key+" = "+TOMLString(value))
			}
		}
		lines = append(lines, "")
	}

	depIDs := make([]string, 0, len(lock.Deps))
	for depID := range lock.Deps {
		depIDs = append(depIDs, depID)
	}
	sort.Strings(depIDs)
	for _, depID := range depIDs {
		skills := lock.Deps[depID]
		skillNames := make([]string, 0, len(skills))
		for skill := range skills {
			skillNames = append(skillNames, skill)
		}
		sort.Strings(skillNames)
		for _, skill := range skillNames {
			files := skills[skill]
			if len(files) == 0 {
				continue
			}
			lines = append(lines, "[deps."+TOMLString(depID)+".skills."+TOMLString(skill)+"]")
			rels := make([]string, 0, len(files))
			for rel := range files {
				rels = append(rels, rel)
			}
			sort.Strings(rels)
			for _, rel := range rels {
				s, err := pyStrValue(files[rel])
				if err != nil {
					return "", err
				}
				lines = append(lines, TOMLString(rel)+" = "+TOMLString(s))
			}
			lines = append(lines, "")
		}
	}

	harnesses := make([]string, 0, len(lock.Agents))
	for harness := range lock.Agents {
		harnesses = append(harnesses, harness)
	}
	sort.Strings(harnesses)
	for _, harness := range harnesses {
		files := lock.Agents[harness]
		if len(files) == 0 {
			continue
		}
		lines = append(lines, "[agents."+TOMLString(harness)+"]")
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s, err := pyStrValue(files[name])
			if err != nil {
				return "", err
			}
			lines = append(lines, TOMLString(name)+" = "+TOMLString(s))
		}
		lines = append(lines, "")
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n", nil
}

// WriteLock is the write side of _write_lock_python: the control-character
// refusal runs FIRST, then the parent is created and the lock is atomically
// replaced via a 0600 temp file (".ai-specs.lock.*.tmp") in the lock's
// parent directory plus rename. A failed write removes the temp and leaves
// the original untouched. The lock is always rewritten; there is
// deliberately no byte-equality no-op.
func WriteLock(lockPath string, lock *Lock) error {
	if lock == nil {
		lock = NewLock()
	}
	if err := refuseControlChars(lockPath, lock); err != nil {
		return err
	}
	text, err := renderLock(lock)
	if err != nil {
		return err
	}
	parent := filepath.Dir(lockPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".ai-specs.lock.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, lockPath); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
