package ledger

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// TrackerStorePath is the split tracker store this build writes:
// <git-common-dir>/ai-specs/ledgers/tracker.json (S11/S14). The store moved out
// of the single pre-split file so each capability can own a file without
// changing what the CLI exposes.
func TrackerStorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledgers", "tracker.json")
}

// StoreLocation is one ledger store on disk: Path is the file this build writes,
// Legacy is the pre-split single store read once for the one-version double read
// (S11), and an empty Legacy means "no fallback and no backup" — a file that was
// named explicitly is exactly the file it says, whatever its directory is called.
type StoreLocation struct {
	Path   string
	Legacy string
}

// TrackerLocation is the tracker ledger's location under a repository's common
// dir: the split store plus the pre-split file it is handed forward from.
func TrackerLocation(gitCommonDir string) StoreLocation {
	return StoreLocation{Path: TrackerStorePath(gitCommonDir), Legacy: StorePath(gitCommonDir)}
}

// StoreAt is an explicitly named store file: no fallback, no backup.
func StoreAt(path string) StoreLocation { return StoreLocation{Path: path} }

// Load reads the store at the location. A missing Path falls back to Legacy for
// this release version, unless there is none. A corrupt or unknown-version file
// — split or legacy — is reported as a *StoreCorruptError and never replaced by
// the other file: history must not come back as live state, and a corrupt
// fallback is never read as an empty store.
func (l StoreLocation) Load() (Store, error) {
	if l.Path == "" {
		return Store{V: StoreVersion}, nil
	}
	_, err := os.Stat(l.Path)
	switch {
	case err == nil:
		return LoadStore(l.Path)
	case !errors.Is(err, fs.ErrNotExist):
		return Store{}, &StoreCorruptError{Path: l.Path, Err: err}
	}
	if l.Legacy == "" {
		return Store{V: StoreVersion}, nil
	}
	return LoadStore(l.Legacy)
}

// Save writes the store at the location atomically and never rewrites the
// pre-split one, so its rows stay readable byte-identically for this release.
// The first handoff also keeps a one-shot copy of the pre-split store: it is the
// plan's mitigation for a migration that loses rows. Callers hold the store lock.
func (l StoreLocation) Save(store Store) error {
	if l.Legacy != "" {
		backupLegacyOnce(l.Legacy)
	}
	return SaveStore(l.Path, store)
}

// backupLegacyOnce copies the pre-split store to <legacy>.v1-backup before the
// first split write, once. The copy is atomic, so a partial file is never later
// mistaken for a complete backup, and an existing backup is never replaced.
// Best effort by construction: the pre-split file is never modified in this
// release, so a failed copy loses nothing.
func backupLegacyOnce(legacy string) {
	backup := legacy + ".v1-backup"
	if _, err := os.Stat(backup); err == nil {
		return
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		return
	}
	_ = writeAtomic(backup, data)
}
