package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// StoreReport is what reaching the store had to do beyond a plain read or write.
// Nothing here is fatal: it exists so the CLI can say it out loud instead of
// dropping rows or a backup in silence. It is never persisted.
type StoreReport struct {
	// Absorbed is how many rows the pre-split store carried that the split store
	// did not have. They are part of the returned store, so grading sees them, and
	// the next write carries them into the split file.
	Absorbed int
	// Shadowed is how many rows the pre-split store also has with different content:
	// an older ai-specs recorded a decision on a row the split store already had.
	// The split store's version answers; the disagreement is reported so nobody
	// assumes the two files say the same thing.
	Shadowed int
	// LegacyRows is the pre-split file those rows came from, for the message.
	LegacyRows string
	// LegacyUnreadable is the pre-split store that could not be read while the
	// split store answered. The read still succeeded, without its rows.
	LegacyUnreadable error
	// BackupFailed is the one-shot copy of the pre-split rows that could not be
	// written before the first split write.
	BackupFailed error
}

// Empty reports whether a read or write was a plain one.
func (r StoreReport) Empty() bool {
	return r.Absorbed == 0 && r.Shadowed == 0 && r.LegacyUnreadable == nil && r.BackupFailed == nil
}

// Note writes one line per problem to w. A clean read or write writes nothing.
func (r StoreReport) Note(w io.Writer, prefix string) {
	if r.Absorbed > 0 {
		fmt.Fprintf(w, "%s: %d ledger row(s) written to the pre-split store %s by an older ai-specs are part of this answer; the next write carries them into the split store\n",
			prefix, r.Absorbed, r.LegacyRows)
	}
	if r.Shadowed > 0 {
		fmt.Fprintf(w, "%s: %d ledger row(s) differ in the pre-split store %s because an older ai-specs wrote them; the split store's version answers\n",
			prefix, r.Shadowed, r.LegacyRows)
	}
	if r.LegacyUnreadable != nil {
		fmt.Fprintf(w, "%s: the pre-split ledger store is unreadable (%v); its rows are missing from this answer\n",
			prefix, r.LegacyUnreadable)
	}
	if r.BackupFailed != nil {
		fmt.Fprintf(w, "%s: could not write the one-shot backup of the pre-split ledger store (%v); that store is untouched, so no row is lost\n",
			prefix, r.BackupFailed)
	}
}

// Load reads the store at the location. A missing Path falls back to Legacy for
// this release version, unless there is none. A corrupt split file is reported
// and never replaced by the older file: history must not come back as live state.
// A corrupt fallback with no split file is reported, never read as an empty store.
//
// When both files exist the rows of the pre-split store that the split store does
// not have are part of the answer: until this release reaches every hook, an older
// ai-specs can still record a decision in the pre-split file, and silently ignoring
// it would drop a human opt-out or let a second item be opened for an identity that
// already has one. Rows already present keep their position and bytes.
func (l StoreLocation) Load() (Store, StoreReport, error) {
	var report StoreReport
	if l.Path == "" {
		return Store{V: StoreVersion}, report, nil
	}
	_, err := os.Stat(l.Path)
	switch {
	case err == nil:
		store, err := LoadStore(l.Path)
		if err != nil {
			return Store{}, report, err
		}
		return l.absorb(store, &report)
	case !errors.Is(err, fs.ErrNotExist):
		return Store{}, report, &StoreCorruptError{Path: l.Path, Err: err}
	}
	if l.Legacy == "" {
		return Store{V: StoreVersion}, report, nil
	}
	store, err := LoadStore(l.Legacy)
	if err != nil {
		return Store{}, report, err
	}
	return store, report, nil
}

// absorb merges the pre-split rows the split store is missing. A corrupt
// pre-split file never fails the read — the split store answered — but it is
// reported, because its rows cannot be shown.
func (l StoreLocation) absorb(split Store, report *StoreReport) (Store, StoreReport, error) {
	if l.Legacy == "" {
		return split, *report, nil
	}
	legacy, err := LoadStore(l.Legacy)
	if err != nil {
		report.LegacyUnreadable = err
		return split, *report, nil
	}
	merged, absorbed, shadowed := mergeStores(split, legacy)
	report.Absorbed = absorbed
	report.Shadowed = shadowed
	report.LegacyRows = l.Legacy
	return merged, *report, nil
}

// mergeStores appends the pre-split rows and opt-outs the split store does not
// already have, by row id and by opt-out record, and counts the rows both files
// have with different content. It never reorders or rewrites an existing row, so
// the historical rows stay byte-identical in place.
func mergeStores(split, legacy Store) (Store, int, int) {
	absorbed, shadowed := 0, 0
	seenItem := make(map[string]int, len(split.Items))
	for i, item := range split.Items {
		seenItem[item.ID] = i
	}
	for _, item := range legacy.Items {
		if at, ok := seenItem[item.ID]; ok {
			if !sameRow(split.Items[at], item) {
				shadowed++
			}
			continue
		}
		seenItem[item.ID] = len(split.Items)
		split.Items = append(split.Items, item)
		absorbed++
	}
	seenOptOut := make(map[string]int, len(split.OptOuts))
	for i, optOut := range split.OptOuts {
		seenOptOut[optOutKey(optOut)] = i
	}
	for _, optOut := range legacy.OptOuts {
		key := optOutKey(optOut)
		if _, ok := seenOptOut[key]; ok {
			continue
		}
		seenOptOut[key] = len(split.OptOuts)
		split.OptOuts = append(split.OptOuts, optOut)
		absorbed++
	}
	return split, absorbed, shadowed
}

// sameRow reports whether the two files carry the same row. A row the split store
// wrote before an older ai-specs touched it is not rewritten in place: the split
// store's version answers and the difference is reported.
func sameRow(a, b Item) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && bytes.Equal(left, right)
}

// optOutKey identifies one scoped opt-out record for de-duplication.
func optOutKey(o ScopedOptOut) string {
	return o.Key + "\x1f" + o.Checkpoint + "\x1f" + o.At
}

// Save writes the store at the location atomically and never rewrites the
// pre-split one, so its rows stay readable byte-identically for this release.
// The first handoff keeps a one-shot copy of the pre-split store; a copy that
// cannot be written is reported, never silent. Callers hold the store lock.
func (l StoreLocation) Save(store Store) (StoreReport, error) {
	var report StoreReport
	if l.Legacy != "" {
		report.BackupFailed = l.BackupLegacy()
	}
	return report, SaveStore(l.Path, store)
}

// BackupLegacy keeps one copy of the pre-split store as <legacy>.v1-backup,
// once. It returns nil when there is nothing to copy (a fresh repository), when
// the backup already exists, and after a successful copy; the copy is atomic, so a
// partial file is never later mistaken for a complete backup. Callers that can
// report the failure do; Save reports it in its StoreReport.
func (l StoreLocation) BackupLegacy() error {
	if l.Legacy == "" {
		return nil
	}
	backup := l.Legacy + ".v1-backup"
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	data, err := os.ReadFile(l.Legacy)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return writeAtomic(backup, data)
}
