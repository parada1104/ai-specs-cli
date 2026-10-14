package doctor

// The gate doctor checks' read-only view of lib/_internal/gate_binary.py. The
// version-keyed cache layout, the committed digest trust root, verified-binary
// resolution and the --version/--selftest probes all delegate to
// internal/gatebinary, the single grader shared with acquisition; this file
// keeps only thin names for the checks plus the doctor-only cache-size report.
// It also carries the native managed-override classifier (the decision the
// gate binary would otherwise own through --plan-classify) so no other doctor
// file keeps a second copy and no host shells out for the decision.
//
// Delegating resolution is a deliberate divergence from doctor.py: a receipt
// without a committed digest, or a cache that breaks the same-user boundary,
// fails closed here (see the internal/gatebinary trust model).
//
// Acquisition is deliberately absent: doctor is read-only and must never
// download, build, chmod, quarantine or write a receipt.

import (
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/gatebinary"
	"ai-specs.dev/ai-specs/internal/lock"
)

// detectGatePlatform is the host (goos, goarch) pair; "" for anything outside
// darwin/linux + arm64/amd64.
func detectGatePlatform() (string, string) { return gatebinary.DetectHostPlatform() }

// gateCacheBinPath is the version-keyed cache path for the installed CLI.
func gateCacheBinPath(home, goos, goarch string) string {
	return gatebinary.CacheBinPath(home, "", goos, goarch)
}

// gateDigestMismatchRecordPath is the last recorded acquisition mismatch.
func gateDigestMismatchRecordPath(home string) string {
	return gatebinary.DigestMismatchRecordPath(home, "")
}

// loadExpectedGateDigests parses the committed SHA256SUMS trust root; ok is
// false when it exists but cannot be read or decoded.
func loadExpectedGateDigests(home string) (map[string]string, bool) {
	return gatebinary.LoadExpectedDigests(home)
}

// isExecutable mirrors os.access(path, os.X_OK) via the mode bits.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// resolveVerifiedGateBinary is the shared verified-binary resolution, or "".
func resolveVerifiedGateBinary(home string) string { return gatebinary.ResolveVerifiedBinary(home) }

// gateBinaryVersion is the stripped --version stdout, "" on any failure.
func gateBinaryVersion(binary string) string { return gatebinary.BinaryVersion(binary) }

// gateSelftest is "" on a passing --selftest, else the failure text.
func gateSelftest(binary string) string { return gatebinary.Selftest(binary) }

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
