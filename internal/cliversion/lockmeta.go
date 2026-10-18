package cliversion

// The native `cli_version.py stamp-meta` (lib/_internal/cli_version.py:206-223):
// the lock's [meta] provenance stamp. `init` wrote it through the
// refresh-bundled seam and `sync` exec'd the Python for it, so this is the one
// authority both call now (odd/tasks/go-20-u4-init.md T4c).

import (
	"path/filepath"
	"time"

	"ai-specs.dev/ai-specs/internal/lock"
)

// LockMetaPath is the lock the stamp targets: `<projectRoot>/ai-specs/.ai-specs.lock`.
func LockMetaPath(projectRoot string) string {
	return filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")
}

// StampLockMeta rewrites the lock's [meta] with the installed CLI version and a
// sync timestamp. The timestamp is KEPT when the existing stamp already names
// the same version: a re-run against the same install must not look like a new
// sync (Python: `if existing.cli_version == installed and existing.synced_at`).
//
// The clock is a parameter so the rule is testable without sleeping, and the
// format is the Python's `%Y-%m-%dT%H:%M:%SZ` on UTC.
func StampLockMeta(projectRoot, cliHome string, now time.Time) error {
	return stampLockMeta(projectRoot, cliHome, now.UTC().Format("2006-01-02T15:04:05Z"))
}

// StampLockMetaNow is StampLockMeta with the wall clock, for the two call sites.
func StampLockMetaNow(projectRoot, cliHome string) error {
	return StampLockMeta(projectRoot, cliHome, time.Now())
}

func stampLockMeta(projectRoot, cliHome, stamp string) error {
	lockPath := LockMetaPath(projectRoot)
	current, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	installed := ReadInstalledVersion(cliHome)
	syncedAt := stamp
	if current.Meta["cli_version"] == installed && current.Meta["synced_at"] != "" {
		syncedAt = current.Meta["synced_at"]
	}
	current.Meta["cli_version"] = installed
	current.Meta["synced_at"] = syncedAt
	return lock.WriteLock(lockPath, current)
}
