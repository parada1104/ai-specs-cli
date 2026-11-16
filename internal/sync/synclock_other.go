//go:build !unix

package sync

// acquireSyncLock does not provide cross-process exclusion on non-unix platforms.
func acquireSyncLock(root, home string) (func(), error) {
	return func() {}, nil
}
