package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Managed-override classification, ported from the Python authority
// util.classify_managed_override (lib/_internal/util.py:662). Go owns the pure
// ownership DECISION state; the Python bridge keeps only acquisition and stays
// a temporary fail-open fallback. The command is read-only: it reads the
// destination bytes and prints one JSON envelope, and never writes a file.
//
// The state order mirrors Python exactly:
//  1. destination is not a regular file            -> missing
//  2. managed entry absent or without a sha256     -> untracked
//  3. disk sha256 differs from the managed sha256  -> user_modified
//  4. no would-write bytes                         -> managed_current
//  5. disk sha256 equals the would-write sha256    -> managed_current
//     otherwise                                    -> managed_stale

const (
	classifyMissing        = "missing"
	classifyUntracked      = "untracked"
	classifyUserModified   = "user_modified"
	classifyManagedCurrent = "managed_current"
	classifyManagedStale   = "managed_stale"
)

// classifyInput is the --plan-classify stdin contract. ManagedEntry is the lock
// metadata table; only its sha256 member is read, and a nil/absent table is the
// untracked case. WouldWrite is the exact post-render text that sync would
// write; a nil pointer means the caller has no candidate bytes.
type classifyInput struct {
	Dest         string                `json:"dest"`
	ManagedEntry *classifyManagedEntry `json:"managed_entry"`
	WouldWrite   *string               `json:"would_write"`
}

// classifyManagedEntry is the subset of [managed.<path>] lock metadata the
// classifier reads. A missing or empty sha256 means the path is untracked.
type classifyManagedEntry struct {
	SHA256 string `json:"sha256"`
}

// classifyResult is the --plan-classify stdout contract. The SHA members are
// omitted when the state has no such input: no disk hash on missing, no managed
// hash on untracked, no would-write hash when it was absent.
type classifyResult struct {
	State            string `json:"state"`
	Dest             string `json:"dest"`
	DiskSHA256       string `json:"disk_sha256,omitempty"`
	ManagedSHA256    string `json:"managed_sha256,omitempty"`
	WouldWriteSHA256 string `json:"would_write_sha256,omitempty"`
}

// sha256Bytes mirrors Python util.sha256_bytes: SHA-256 over CRLF-normalized
// bytes, hex encoded. Both authorities must hash identically or the parity
// contract between the Python fallback and the Go grader breaks.
func sha256Bytes(data []byte) string {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

// classifyManagedOverride is the pure decision core. disk holds the destination
// bytes and present reports whether the destination is a regular file; a
// non-regular or absent destination is missing regardless of disk. It performs
// no I/O and never writes.
func classifyManagedOverride(present bool, disk []byte, managed *classifyManagedEntry, wouldWrite *string) classifyResult {
	result := classifyResult{State: classifyMissing}
	if !present {
		return result
	}
	diskSHA := sha256Bytes(disk)
	result.DiskSHA256 = diskSHA
	if managed == nil || managed.SHA256 == "" {
		result.State = classifyUntracked
		return result
	}
	result.ManagedSHA256 = managed.SHA256
	if diskSHA != managed.SHA256 {
		result.State = classifyUserModified
		return result
	}
	if wouldWrite == nil {
		result.State = classifyManagedCurrent
		return result
	}
	result.WouldWriteSHA256 = sha256Bytes([]byte(*wouldWrite))
	if diskSHA == result.WouldWriteSHA256 {
		result.State = classifyManagedCurrent
	} else {
		result.State = classifyManagedStale
	}
	return result
}

// readRegularFile mirrors Python Path.is_file(): any stat error or a non-regular
// entry reports present=false (the "missing" case). A read failure on a regular
// file is a process-level error, never a silent missing.
func readRegularFile(path string) (bool, []byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false, nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil, err
	}
	return true, data, nil
}

// runPlanClassify is the --plan-classify command: decode one JSON envelope from
// stdin, classify the destination, and print one JSON object on stdout. A
// malformed envelope, a missing dest, or an unreadable destination is a
// process-level failure (exit 2, no stdout). It never writes.
func runPlanClassify(stdin io.Reader, stdout, stderr io.Writer) int {
	var in classifyInput
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-classify: read stdin: %v\n", err)
		return 2
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		fmt.Fprintln(stderr, "worktree-gate: --plan-classify: empty input")
		return 2
	}
	if err := json.Unmarshal([]byte(text), &in); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-classify: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(in.Dest) == "" {
		fmt.Fprintln(stderr, "worktree-gate: --plan-classify: missing dest")
		return 2
	}
	present, disk, err := readRegularFile(in.Dest)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-classify: dest %s: %v\n", in.Dest, err)
		return 2
	}
	result := classifyManagedOverride(present, disk, in.ManagedEntry, in.WouldWrite)
	result.Dest = in.Dest
	payload, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --plan-classify: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
