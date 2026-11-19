//go:build unix

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/worktree-gate/shared"
)

// These modes preserve existing cache permissions; polling bounds lock retry granularity.
const (
	syncLockDirMode  = 0o777
	syncLockFileMode = 0o666
	syncLockPoll     = 100 * time.Millisecond
)

// acquireSyncLock serializes sync processes sharing the same cache root.
func acquireSyncLock(root, home string) (func(), error) {
	// CacheRoot resolves symlinks; check the lexical path before opening the lock.
	home = projectcache.AISpecsHome(home)
	if err := shared.EnsureTemplateAncestorsReal(home, filepath.Join(home, "cache", "projects", projectcache.CacheKey(root), "sync.lock")); err != nil {
		return nil, err
	}
	cacheRoot := projectcache.CacheRoot(root, home)
	if err := os.MkdirAll(cacheRoot, syncLockDirMode); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(cacheRoot, "sync.lock"), os.O_CREATE|os.O_RDWR, syncLockFileMode)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(syncLockWait)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = file.Close()
			return nil, err
		}
		if time.Until(deadline) <= 0 {
			_ = file.Close()
			return nil, fmt.Errorf("%w: %s", errSyncLockTimeout, syncLockWait)
		}
		time.Sleep(min(syncLockPoll, time.Until(deadline)))
	}
}
