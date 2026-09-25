package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Lock-file writer core, ported 1:1 from the Python authority
// lib/_internal/lock.py:86-136 (write_lock + _toml_string + the atomic
// replace). Go owns the byte-exact emission; there is no TOML validation
// after the write, exactly like the Python reference, and no byte-equality
// no-op: every invocation rewrites the lock.
//
// Command surface: --write-lock reads one JSON envelope on stdin
//
//	{"lock_path": "<abs path>",
//	 "meta": {"cli_version": "...", "synced_at": "..."},
//	 "managed": {"<path>": {"sha256": "...", "recipe": "...", "source": "...",
//	                        "kind": "...", "policy": "..."}},
//	 "agents": {"<harness>": {"<filename>": "hash"}}}
//
// and prints {"written": true} on stdout with exit 0. Structured/refusal
// errors (invalid envelope, non-string values, empty lock_path) print
// {"error": "<string>"} on stdout with exit 2; infrastructure failures (I/O
// errors) report a diagnostic on stderr with exit 2. All values arrive as
// strings (the Python bridge pre-stringifies); the nested agents shape
// matches the Python lock dict {harness: {filename: hash}} byte for byte.

// lockHeader is LOCK_HEADER from lib/_internal/lock.py:9-15, copied
// character-for-character. TestLockHeaderByteIdentity pins it; a divergence
// must fail the test.
const lockHeader = `# Managed by ai-specs. Do not edit by hand.
# Provenance stamp: [meta] records the CLI version and timestamp of the last
# sync. [managed.*] records integrity only for CLI-owned override targets;
# it is not a general content-integrity manifest. git covers the committed
# project surface; skill/recipe/dep content hashes are not tracked.
`

// lockWriteRequest is the stdin envelope. Values are typed as strings so a
// non-string value in the JSON becomes an UnmarshalTypeError, which the CLI
// reports as a structured refusal (the future Python bridge pre-stringifies
// everything with str() parity).
type lockWriteRequest struct {
	LockPath string                       `json:"lock_path"`
	Meta     map[string]string            `json:"meta"`
	Managed  map[string]lockManagedEntry  `json:"managed"`
	Agents   map[string]map[string]string `json:"agents"`
}

// lockManagedEntry is one [managed."<path>"] record; empty values are
// skipped on emission (Python: `value is not None and value != ""`).
type lockManagedEntry struct {
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// lockTOMLString is _toml_string (lib/_internal/lock.py:81-83): escape the
// backslash first, then the double quote. Control characters are emitted raw
// — never \n, \t or any other escape sequence.
func lockTOMLString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// renderLock is the write_lock body (lib/_internal/lock.py:86-122): fixed
// section order [meta] → [managed."<path>"] (sorted, empty values skipped) →
// [agents."<harness>"] (sorted, filenames sorted), assembled as
// "\n".join(lines).rstrip("\n") + "\n".
func renderLock(req *lockWriteRequest) string {
	lines := []string{lockHeader}

	if len(req.Meta) > 0 {
		lines = append(lines, "[meta]")
		// Only cli_version then synced_at, empty values skipped (Python:
		// `if meta.get("cli_version")`). Unknown meta keys are ignored,
		// matching the reference's fixed key emission.
		if req.Meta["cli_version"] != "" {
			lines = append(lines, "cli_version = "+lockTOMLString(req.Meta["cli_version"]))
		}
		if req.Meta["synced_at"] != "" {
			lines = append(lines, "synced_at = "+lockTOMLString(req.Meta["synced_at"]))
		}
		lines = append(lines, "")
	}

	paths := make([]string, 0, len(req.Managed))
	for path := range req.Managed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		entry := req.Managed[path]
		if entry.SHA256 == "" {
			continue
		}
		lines = append(lines, "[managed."+lockTOMLString(path)+"]")
		// Fixed key order sha256, recipe, source, kind, policy; empty
		// values skipped exactly like the reference.
		for _, kv := range []struct{ key, value string }{
			{"sha256", entry.SHA256},
			{"recipe", entry.Recipe},
			{"source", entry.Source},
			{"kind", entry.Kind},
			{"policy", entry.Policy},
		} {
			if kv.value != "" {
				lines = append(lines, kv.key+" = "+lockTOMLString(kv.value))
			}
		}
		lines = append(lines, "")
	}

	harnesses := make([]string, 0, len(req.Agents))
	for harness := range req.Agents {
		harnesses = append(harnesses, harness)
	}
	sort.Strings(harnesses)
	for _, harness := range harnesses {
		files := req.Agents[harness]
		if len(files) == 0 {
			continue
		}
		lines = append(lines, "[agents."+lockTOMLString(harness)+"]")
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			lines = append(lines, lockTOMLString(name)+" = "+lockTOMLString(files[name]))
		}
		lines = append(lines, "")
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// writeLockFile replaces lockPath with the rendered lock via a temp file in
// the lock's parent directory plus rename, mirroring the Python reference
// (lock.py:123-136): the parent is created first, a failed write removes the
// temp and leaves the original untouched. The temp file keeps CreateTemp's
// 0600 mode, which is exactly tempfile.mkstemp's default — mode parity with
// the Python reference is intentional. The lock is always rewritten; there
// is deliberately no byte-equality no-op.
func writeLockFile(req *lockWriteRequest) error {
	parent := filepath.Dir(req.LockPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".ai-specs.lock.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(renderLock(req)); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, req.LockPath); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// runWriteLock is the --write-lock command: one JSON envelope on stdin, one
// JSON envelope on stdout. Exit 0 on success, exit 2 on a structured refusal
// ({"error": ...} on stdout) or an infrastructure failure (diagnostic on
// stderr).
func runWriteLock(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-lock: read stdin: %v\n", err)
		return 2
	}
	var req lockWriteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			fmt.Fprintln(stdout, `{"error": `+pyJSONString("lock write: non-string value at "+typeErr.Field)+`}`)
		} else {
			fmt.Fprintln(stdout, `{"error": `+pyJSONString("lock write: invalid input JSON: "+err.Error())+`}`)
		}
		return 2
	}
	if req.LockPath == "" {
		fmt.Fprintln(stdout, `{"error": "lock write: lock_path must be a non-empty path"}`)
		return 2
	}
	if err := writeLockFile(&req); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-lock: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, `{"written": true}`)
	return 0
}
