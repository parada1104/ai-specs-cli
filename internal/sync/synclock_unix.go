//go:build unix

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

func acquireSyncLock(root, home string) (func(), error) {
	cacheRoot := projectcache.CacheRoot(root, home)
	if err := os.MkdirAll(cacheRoot, 0o777); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(cacheRoot, "sync.lock"), os.O_CREATE|os.O_RDWR, 0o666)
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
		time.Sleep(min(100*time.Millisecond, time.Until(deadline)))
	}
}
