package doctor

// Native port of the read-only subset of lib/_internal/gate_binary.py the gate
// doctor checks share: platform detection, the version-keyed cache layout, the
// committed digest trust root, verified-binary resolution with its fail-closed
// receipt checks, the binary --version/--selftest probes, the cache-size report
// and the recorded digest-mismatch location. It also carries the native
// managed-override classifier (the decision the gate binary would otherwise own
// through --plan-classify) so no other doctor file keeps a second copy and no
// host shells out for the decision.
//
// Acquisition is deliberately absent: doctor is read-only and must never
// download, build, chmod, quarantine or write a receipt. Where the Python
// module acquires, this port only reads.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ai-specs.dev/ai-specs/internal/lock"
)

// gateCLIVersion mirrors gate_binary.cli_version: <home>/VERSION stripped, or
// "unknown".
func gateCLIVersion(home string) string {
	path := filepath.Join(home, "VERSION")
	if !isFile(path) {
		return "unknown"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "unknown"
	}
	return text
}

// detectGatePlatform mirrors gate_binary.detect_platform for the Go runtime's
// OS/arch pair (darwin/linux + arm64/amd64; anything else is no binary).
func detectGatePlatform() (string, string) {
	goos := ""
	switch runtime.GOOS {
	case "darwin":
		goos = "darwin"
	case "linux":
		goos = "linux"
	}
	goarch := ""
	switch runtime.GOARCH {
	case "arm64":
		goarch = "arm64"
	case "amd64":
		goarch = "amd64"
	}
	return goos, goarch
}

// gateCacheBinPath mirrors gate_binary.cache_bin_path.
func gateCacheBinPath(home, goos, goarch string) string {
	return filepath.Join(home, "cache", "bin", "worktree-gate", gateCLIVersion(home),
		goos+"-"+goarch, "worktree-gate")
}

// gateDigestMismatchRecordPath mirrors gate_binary.digest_mismatch_record_path.
func gateDigestMismatchRecordPath(home string) string {
	return filepath.Join(home, "cache", "bin", "worktree-gate", gateCLIVersion(home),
		"last-digest-mismatch.txt")
}

// loadExpectedGateDigests mirrors gate_binary.load_expected_digests. ok is
// false when the trust root exists but cannot be read or decoded, the legacy
// fail-closed branch.
func loadExpectedGateDigests(home string) (map[string]string, bool) {
	sumsPath := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	digests := map[string]string{}
	if !isFile(sumsPath) {
		return digests, true
	}
	raw, err := os.ReadFile(sumsPath)
	if err != nil {
		return nil, false
	}
	if !utf8.Valid(raw) {
		return nil, false
	}
	for _, line := range pySplitLines(string(raw)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexFunc(line, unicode.IsSpace)
		if idx < 0 {
			continue
		}
		digest := line[:idx]
		name := strings.TrimSpace(line[idx:])
		if len(digest) == 64 && strings.HasPrefix(name, "worktree-gate-") {
			digests[name] = strings.ToLower(digest)
		}
	}
	return digests, true
}

// isExecutable mirrors os.access(path, os.X_OK) via the mode bits.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// resolveVerifiedGateBinary mirrors gate_binary.resolve_verified_binary: the
// $WORKTREE_GATE_BIN override, else the version-keyed cache candidate accepted
// only with its .verified receipt and a matching committed trust-root digest.
func resolveVerifiedGateBinary(home string) string {
	if override := os.Getenv("WORKTREE_GATE_BIN"); override != "" {
		if isFile(override) && isExecutable(override) {
			return override
		}
		return ""
	}
	goos, goarch := detectGatePlatform()
	if goos == "" || goarch == "" {
		return ""
	}
	candidate := gateCacheBinPath(home, goos, goarch)
	if !isFile(candidate) || !isExecutable(candidate) || !isFile(candidate+".verified") {
		return ""
	}
	expected, ok := loadExpectedGateDigests(home)
	if !ok {
		return ""
	}
	if digest, present := expected["worktree-gate-"+goos+"-"+goarch]; present {
		observed, err := fileSHA256(candidate)
		if err != nil {
			return ""
		}
		if observed != digest {
			return ""
		}
	}
	return candidate
}

// gateBinaryVersion mirrors gate_binary.binary_version: the stripped --version
// stdout, or "" on any failure (including a non-zero exit).
func gateBinaryVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gateSelftest mirrors gate_binary._run_selftest: "" success, else the failure
// text (stderr, then stdout, then the literal fallback).
func gateSelftest(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--selftest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return ""
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		detail := stderr.String()
		if detail == "" {
			detail = stdout.String()
		}
		if detail == "" {
			detail = "selftest failed"
		}
		return strings.TrimSpace(detail)
	}
	// The legacy formats the exception type and message here; the Go timeout
	// and start errors have no byte-identical Python counterpart, so the port
	// reports the Go error text. The check only reaches this after the binary
	// was proven present and executable.
	return strings.TrimSpace(err.Error())
}

// gateCacheSize mirrors gate_binary.cache_size: the total regular-file bytes
// under the whole version-keyed cache root (all versions), following symlinks
// like Path.is_file()/stat().
func gateCacheSize(home string) int64 {
	root := filepath.Join(home, "cache", "bin", "worktree-gate")
	if !isDir(root) {
		return 0
	}
	var total int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// classifyManagedOverride is the native managed-override decision, the same one
// worktree-gate --plan-classify returns (catalog/recipes/worktree-flow/gate/
// classify.go) and the Python fail-open fallback mirrors: missing file, absent
// lock sha256, disk sha != managed sha, then current/stale against the exact
// would-write bytes. hasWouldWrite distinguishes "no candidate" from empty
// candidate bytes. It reads only; nothing is written.
func classifyManagedOverride(dest string, managed map[string]any, wouldWrite []byte, hasWouldWrite bool) string {
	info, err := os.Stat(dest)
	if err != nil || !info.Mode().IsRegular() {
		return "missing"
	}
	disk, err := os.ReadFile(dest)
	if err != nil {
		// The legacy authority would fail to read and abort; doctor only calls
		// this after is_file(), so an unreadable regular file is unreachable in
		// practice. Degrade to "missing" rather than fabricating ownership.
		return "missing"
	}
	diskSHA := lock.Sha256Bytes(disk)
	managedSHA, ok := managed["sha256"].(string)
	if managed == nil || !ok || managedSHA == "" {
		return "untracked"
	}
	if diskSHA != managedSHA {
		return "user_modified"
	}
	if !hasWouldWrite {
		return "managed_current"
	}
	if diskSHA == lock.Sha256Bytes(wouldWrite) {
		return "managed_current"
	}
	return "managed_stale"
}

// pyStrOr mirrors `str(value or default)` for the TOML value types the manifest
// carries; falsy values (None/False/0/""/empty) fall back to the default.
func pyStrOr(value any, fallback string) string {
	if !truthy(value) {
		return fallback
	}
	return pyStrValue(value)
}
