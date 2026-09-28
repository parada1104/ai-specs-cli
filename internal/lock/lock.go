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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

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

// pyStr mirrors the Python writer's `value is not None and value != ""`
// truthiness plus str(): nil and empty strings are falsy, everything else
// stringifies.
func pyStr(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return x, x != ""
	default:
		return fmt.Sprint(x), true
	}
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
		if sha, ok := pyStr(entry["sha256"]); !ok || sha == "" {
			continue
		}
		for _, key := range []string{"sha256", "recipe", "source", "kind", "policy"} {
			if value, ok := pyStr(entry[key]); ok {
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
			appendCheck("agents hash", pyStrValue(files[name]))
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
				appendCheck("deps hash", pyStrValue(files[rel]))
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

// pyStrValue is str(v) for the refusal walk, which stringifies unconditionally.
func pyStrValue(v any) string {
	s, _ := pyStr(v)
	return s
}

// renderLock is the _write_lock_python body: fixed section order
// [meta] → [managed."<path>"] (sorted, sha256-less entries and empty values
// skipped) → [deps."<id>".skills."<skill>"] (sorted, empty file maps
// skipped) → [agents."<harness>"] (sorted, empty maps skipped), assembled as
// "\n".join(lines).rstrip("\n") + "\n". The legacy [recipes] and [skills]
// sections are read but never emitted, on both sides (recorded-defect
// behavior, do not fix).
func renderLock(lock *Lock) string {
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
		if _, ok := pyStr(entry["sha256"]); !ok {
			continue
		}
		lines = append(lines, "[managed."+TOMLString(path)+"]")
		for _, key := range []string{"sha256", "recipe", "source", "kind", "policy"} {
			if value, ok := pyStr(entry[key]); ok {
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
				lines = append(lines, TOMLString(rel)+" = "+TOMLString(pyStrValue(files[rel])))
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
			lines = append(lines, TOMLString(name)+" = "+TOMLString(pyStrValue(files[name])))
		}
		lines = append(lines, "")
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
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
	parent := filepath.Dir(lockPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".ai-specs.lock.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(renderLock(lock)); err != nil {
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
