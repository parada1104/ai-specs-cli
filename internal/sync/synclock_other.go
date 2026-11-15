//go:build !unix

package sync

// shortcut: non-unix platforms have no advisory sync lock.
func acquireSyncLock(root, home string) (func(), error) {
	return func() {}, nil
}
