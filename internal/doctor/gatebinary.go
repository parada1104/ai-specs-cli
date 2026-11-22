package doctor

// Shared read-only helpers for the installed CLI and managed hook overrides.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/lock"
)

func installedCLIPath(home string) string { return filepath.Join(home, "dist", "ai-specs") }

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// cliWorktreeGateSelftest runs the same subcommand used by the installed
// launchers and returns diagnostic output on failure.
func cliWorktreeGateSelftest(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "worktree-gate", "--selftest")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil {
		return ""
	}
	if ctx.Err() != nil {
		return ctx.Err().Error()
	}
	if detail := strings.TrimSpace(stderr.String()); detail != "" {
		return detail
	}
	if detail := strings.TrimSpace(stdout.String()); detail != "" {
		return detail
	}
	return "selftest failed"
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
