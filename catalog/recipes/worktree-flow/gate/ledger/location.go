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

// legacyStorePathFor maps a split store path back to the pre-split single store
// it replaces: <common>/ai-specs/ledgers/<ledger>.json maps to
// <common>/ai-specs/ledger/state.json for every ledger in the split directory
// (S11). A path outside a `ledgers` directory — an explicit --store override —
// is its own legacy path, so it has no fallback.
func legacyStorePathFor(storePath string) string {
	clean := filepath.Clean(storePath)
	dir := filepath.Dir(clean)
	if filepath.Base(dir) != "ledgers" || filepath.Base(clean) == "" || filepath.Base(clean) == "." {
		return clean
	}
	return filepath.Join(filepath.Dir(dir), "ledger", "state.json")
}

// LoadTrackerStore reads the split store at storePath and, for one release
// version, falls back to the pre-split single store when the split file does not
// exist yet (S11). A corrupt split file is reported and never silently replaced
// by the older file — history must not come back as live state — and a corrupt
// fallback is reported rather than read as an empty store.
func LoadTrackerStore(storePath string) (Store, error) {
	_, err := os.Stat(storePath)
	switch {
	case err == nil:
		return LoadStore(storePath)
	case !errors.Is(err, fs.ErrNotExist):
		return Store{}, &StoreCorruptError{Path: storePath, Err: err}
	}
	legacy := legacyStorePathFor(storePath)
	if legacy == storePath {
		return Store{V: StoreVersion}, nil
	}
	return LoadStore(legacy)
}

// SaveTrackerStore writes the split store atomically and never rewrites the
// pre-split one, so its rows stay readable byte-identically for this release.
// The first handoff also keeps a one-shot copy of the pre-split store: it is the
// plan's mitigation for a migration that loses rows. Callers hold the store lock.
func SaveTrackerStore(storePath string, store Store) error {
	backupLegacyOnce(storePath)
	return SaveStore(storePath, store)
}

// backupLegacyOnce copies the pre-split store to <state.json>.v1-backup before
// the first split write, once. Best effort by construction: the pre-split file is
// never modified in this release, so a failed copy loses nothing.
func backupLegacyOnce(storePath string) {
	legacy := legacyStorePathFor(storePath)
	if legacy == storePath {
		return
	}
	backup := legacy + ".v1-backup"
	if _, err := os.Stat(backup); err == nil {
		return
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		return
	}
	_ = os.WriteFile(backup, data, 0o600)
}
