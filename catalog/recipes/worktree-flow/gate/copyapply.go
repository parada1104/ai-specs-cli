package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Copy actuator for the materialize blind copiers (GO-08 WU2, strangler
// slice 5). Go owns the COPY DECISION + EXECUTION for the three copiers whose
// source is a local tree/file — materialize_bundled_skill, materialize_command
// and materialize_doc — while Python keeps hashing (set_recipe_skill_hashes +
// write_lock, already Go-delegated), printing, warnings and the fail-open
// fallback. materialize_dep_skill stays Python (true acquisition via
// vendor-skills.py) and materialize_template stays Python (slice 6 owns it).
//
// Command surface: --apply-copy reads one JSON envelope on stdin
//
//	{"items": [{"kind": "bundled-skill"|"command"|"doc", "id": "...",
//	            "src": "...", "dest": "...", "commands_dir": "..."}]}
//
// and prints one result envelope on stdout with exit 0:
//
//	{"results": [{"id": "...", "status": "ok"|"source-missing",
//	              "overwrite": true?}]}
//
// "overwrite" is emitted only for command items whose dest existed and
// differed (the Python warn condition: dest exists and is not a file, or the
// bytes differ). A missing source is a per-item "source-missing" result, NOT
// a transport failure — Python raises the exact RuntimeError at the same
// point. Invalid envelope or item execution failure prints {"error": "..."}
// on stdout with exit 2 (detail on stderr); the Python bridge fails open on
// those with its idempotent reference semantics (the bundled-skill fallback
// rmtree+copytree rewrites dest wholesale, so a partial Go copy is safe to
// redo, and copy2 is an idempotent overwrite), but a delivered exit-2 error
// envelope is a REFUSAL and fails closed on the Python side: it raises
// RuntimeError naming the Go error instead of falling back (GO-08 findings
// fix).
//
// The Python bridge sends one item per call (its dispatch loop already
// iterates per item), so per-item error semantics and the original
// RuntimeError ordering are preserved exactly; the envelope still accepts an
// ordered item list and executes it in order. "commands_dir" is accepted for
// envelope parity with the pinned contract but is informational: "dest" is
// authoritative.

// copyItem is one stdin envelope entry.
type copyItem struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Src         string `json:"src"`
	Dest        string `json:"dest"`
	CommandsDir string `json:"commands_dir"`
}

// copyRequest is the stdin envelope.
type copyRequest struct {
	Items []copyItem `json:"items"`
}

// copyResult is one stdout result entry. Overwrite is omitted when false.
type copyResult struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

// statMode mirrors Python's stat.S_IMODE: permission bits plus setuid/setgid/
// sticky, which copystat preserves and plain Perm() drops. The special bits
// are returned as Go FileMode flags (os.ModeSetuid, ...) — not raw 0o4000-
// style bits — so os.Chmod and os.WriteFile re-apply them through
// syscallMode; raw bits in the FileMode would be silently dropped.
func statMode(info os.FileInfo) os.FileMode {
	mode := info.Mode()
	perm := mode.Perm()
	if mode&os.ModeSetuid != 0 {
		perm |= os.ModeSetuid
	}
	if mode&os.ModeSetgid != 0 {
		perm |= os.ModeSetgid
	}
	if mode&os.ModeSticky != 0 {
		perm |= os.ModeSticky
	}
	return perm
}

// copyFileStat mirrors shutil.copy2: copy the content, then copystat — the
// source's S_IMODE permission bits and its mtime. The atime is set to the
// mtime rather than preserved: every later read refreshes it anyway, and
// nothing in the CLI consumes atimes (mtime is what copy2 parity needs).
// The content is read into memory in one piece: the copiers' inputs are
// bounded config/skill files, and this keeps the byte copy identical to
// copy2's read-then-write without streaming plumbing.
func copyFileStat(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, statMode(info)); err != nil {
		return err
	}
	if err := os.Chmod(dest, statMode(info)); err != nil {
		return err
	}
	mtime := info.ModTime()
	return os.Chtimes(dest, mtime, mtime)
}

// copyTree mirrors shutil.copytree(src, dest) with the default symlinks=False:
// dest must not exist; directories are recreated with the source's S_IMODE
// bits and, after their children, their mtime (copystat runs after children
// in the reference too); files go through copyFileStat. Symlinks are
// followed: a symlinked directory is recursed into, a symlinked file is
// copied as its target's content.
//
// Dir-mode parity: copytree creates each directory with makedirs' default
// mode (0o777 filtered by the process umask) and copystat applies the
// source's S_IMODE bits only after the children are copied. So the creation
// mode here is 0o777 (umask applies exactly as in Python) and the source's
// bits are chmod'ed on after the children — creating with the source bits
// directly would leave them umask-filtered and diverge from the reference.
// Child errors propagate immediately (first-error parity with copytree's
// default collect_errors=False).
func copyTree(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		// Defensive parity with copytree raising on a non-directory source;
		// applyCopyDecision already screens the item's source, so this only
		// fires for direct/internal misuse.
		return fmt.Errorf("copy tree: %s is not a directory", src)
	}
	if err := os.Mkdir(dest, 0o777); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childSrc := filepath.Join(src, entry.Name())
		childDest := filepath.Join(dest, entry.Name())
		childInfo, err := os.Stat(childSrc) // follow symlinks, like the reference
		if err != nil {
			return err
		}
		if childInfo.IsDir() {
			if err := copyTree(childSrc, childDest); err != nil {
				return err
			}
			continue
		}
		if err := copyFileStat(childSrc, childDest); err != nil {
			return err
		}
	}
	// copystat for the directory itself, after the children: the source's
	// S_IMODE bits (including setgid) are applied now, umask-independent.
	if err := os.Chmod(dest, statMode(info)); err != nil {
		return err
	}
	mtime := info.ModTime()
	if err := os.Chtimes(dest, mtime, mtime); err != nil {
		return err
	}
	return nil
}

// applyCopyDecision is the Go COPY DECISION for one item: validate the
// envelope entry, decide source presence and (for commands) the overwrite
// condition. It performs no filesystem mutation, so callers can observe the
// decision even when the execution that follows would fail (dest is a
// directory → Python's fallback re-runs the item; see runApplyCopy).
func applyCopyDecision(item *copyItem) (copyResult, error) {
	if item.Kind != "bundled-skill" && item.Kind != "command" && item.Kind != "doc" {
		return copyResult{}, fmt.Errorf("copy apply: unknown item kind %q", item.Kind)
	}
	if item.ID == "" || item.Src == "" || item.Dest == "" {
		return copyResult{}, errors.New("copy apply: item requires non-empty id, src and dest")
	}
	info, err := os.Stat(item.Src)
	if err != nil {
		if os.IsNotExist(err) {
			return copyResult{ID: item.ID, Status: "source-missing"}, nil
		}
		return copyResult{}, fmt.Errorf("copy apply: stat %s: %w", item.Src, err)
	}
	switch item.Kind {
	case "bundled-skill":
		if !info.IsDir() {
			return copyResult{ID: item.ID, Status: "source-missing"}, nil
		}
	case "command", "doc":
		if !info.Mode().IsRegular() {
			return copyResult{ID: item.ID, Status: "source-missing"}, nil
		}
	}
	result := copyResult{ID: item.ID, Status: "ok"}
	if item.Kind == "command" {
		// Python: dest.exists() and (not dest.is_file() or dest.read_bytes() != src.read_bytes())
		if destInfo, err := os.Stat(item.Dest); err == nil {
			if !destInfo.Mode().IsRegular() {
				result.Overwrite = true
			} else {
				srcBytes, err := os.ReadFile(item.Src)
				if err != nil {
					return copyResult{}, fmt.Errorf("copy apply: read %s: %w", item.Src, err)
				}
				destBytes, err := os.ReadFile(item.Dest)
				if err != nil {
					return copyResult{}, fmt.Errorf("copy apply: read %s: %w", item.Dest, err)
				}
				result.Overwrite = string(srcBytes) != string(destBytes)
			}
		}
	}
	return result, nil
}

// executeCopyItem is the Go COPY EXECUTION for one decided item, mirroring
// the reference bodies: bundled-skill replaces dest wholesale (rmtree +
// dest.parent.mkdir + copytree); command and doc are copy2 with a
// dest.parent.mkdir first. The parent is created with 0o777 so the process
// umask filters it exactly like Python's default mkdir(parents=True) mode.
func executeCopyItem(item *copyItem) error {
	switch item.Kind {
	case "bundled-skill":
		if info, err := os.Lstat(item.Dest); err == nil && info.IsDir() {
			if err := os.RemoveAll(item.Dest); err != nil {
				return err
			}
		} else if err == nil {
			// A non-directory dest: the reference's shutil.rmtree would raise,
			// which surfaces here as the same item failure.
			return fmt.Errorf("copy apply: dest %s is not a directory", item.Dest)
		}
		if err := os.MkdirAll(filepath.Dir(item.Dest), 0o755); err != nil {
			return err
		}
		return copyTree(item.Src, item.Dest)
	case "command", "doc":
		if err := os.MkdirAll(filepath.Dir(item.Dest), 0o755); err != nil {
			return err
		}
		return copyFileStat(item.Src, item.Dest)
	}
	return fmt.Errorf("copy apply: unknown item kind %q", item.Kind)
}

// runApplyCopy is the --apply-copy command: one JSON envelope on stdin, one
// JSON envelope on stdout. Exit 0 when every item was decided (ok or
// source-missing); exit 2 with {"error": ...} on stdout for an invalid
// envelope or an item execution failure. Items execute in envelope order.
//
// Trust model: the gate binary is caller-privileged — every flag writes
// where it is told — so the item src/dest paths are trusted exactly like the
// other flag inputs; there is deliberately no path confinement.
func runApplyCopy(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --apply-copy: read stdin: %v\n", err)
		return 2
	}
	var req copyRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintln(stdout, `{"error": `+pyJSONString("copy apply: invalid input JSON: "+err.Error())+`}`)
		return 2
	}
	results := make([]copyResult, 0, len(req.Items))
	for i := range req.Items {
		item := &req.Items[i]
		result, err := applyCopyDecision(item)
		if err != nil {
			fmt.Fprintln(stdout, `{"error": `+pyJSONString(err.Error())+`}`)
			return 2
		}
		if result.Status == "ok" {
			if err := executeCopyItem(item); err != nil {
				fmt.Fprintln(stdout, `{"error": `+pyJSONString(fmt.Sprintf("copy apply: item %s: %v", item.ID, err))+`}`)
				return 2
			}
		}
		results = append(results, result)
	}
	encoded, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --apply-copy: encode results: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}
