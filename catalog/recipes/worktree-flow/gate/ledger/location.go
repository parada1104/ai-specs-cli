package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// TrackerStorePath is the split tracker ledger this build writes:
// <git-common-dir>/ai-specs/ledgers/tracker.json (S11/S14). The store moved out
// of the single pre-split file so each capability can own a file without
// changing what the CLI exposes.
func TrackerStorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledgers", "tracker.json")
}

// WorktreeStorePath is the worktree ledger this build writes:
// <git-common-dir>/ai-specs/ledgers/worktree.json. The lifecycle — which row is
// open, when it closed — is the worktree capability's own state, so it has its
// own file (S3).
func WorktreeStorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledgers", "worktree.json")
}

// PlanStorePath is the plan-build-flow ledger location:
// <git-common-dir>/ai-specs/ledgers/plan.json. Declared here so the three ledgers
// share one location vocabulary; no plan row is written yet.
func PlanStorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledgers", "plan.json")
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

// WorktreeLocation is the worktree ledger's location under a repository's common
// dir: the split store plus the pre-split file it is handed forward from.
func WorktreeLocation(gitCommonDir string) StoreLocation {
	return StoreLocation{Path: WorktreeStorePath(gitCommonDir), Legacy: StorePath(gitCommonDir)}
}

// PlanLocation is the plan ledger's location under a repository's common dir: the
// split store plus the pre-split file it is handed forward from.
func PlanLocation(gitCommonDir string) StoreLocation {
	return StoreLocation{Path: PlanStorePath(gitCommonDir), Legacy: StorePath(gitCommonDir)}
}

// StoreAt is an explicitly named store file: no fallback, no backup.
func StoreAt(path string) StoreLocation { return StoreLocation{Path: path} }

// Ledgers is the located ledger set one invocation reads and writes: the worktree
// lifecycle and the tracker record, each resolved once so a command never answers
// from a different pair of files than the one it writes.
type Ledgers struct {
	Worktree StoreLocation
	Tracker  StoreLocation
}

// LedgersAt resolves every ledger under a repository's common dir, each with the
// pre-split store as its one-release fallback.
func LedgersAt(gitCommonDir string) Ledgers {
	return Ledgers{Worktree: WorktreeLocation(gitCommonDir), Tracker: TrackerLocation(gitCommonDir)}
}

// LedgersAtStore resolves the ledger set from an explicitly named tracker file:
// no fallback and no backup for either, with the worktree ledger as the sibling
// file this build writes beside it.
func LedgersAtStore(path string) Ledgers {
	return Ledgers{
		Worktree: StoreLocation{Path: filepath.Join(filepath.Dir(path), "worktree.json")},
		Tracker:  StoreAt(path),
	}
}

// Load reads both ledgers and joins them into the read-only View (L8). Each
// ledger's report is returned separately so a problem is always attributed to the
// file that had it, and a corrupt file on one side still leaves the other side's
// rows readable: only the error it returns is shared.
func (l Ledgers) Load() (View, LedgerReports, error) {
	worktree, worktreeReport, worktreeErr := l.Worktree.LoadWorktree()
	tracker, trackerReport, trackerErr := l.Tracker.Load()
	reports := LedgerReports{Worktree: worktreeReport, Tracker: trackerReport}
	// A pre-split file this build cannot read is reported per ledger, not fatal:
	// it is not this build's file, and each ledger still answers from its own. A
	// corrupt split file stays fatal, so the grade reports it as unevaluable.
	worktreeErr = legacyOnly(worktreeErr, l.Worktree, &reports.Worktree)
	trackerErr = legacyOnly(trackerErr, l.Tracker, &reports.Tracker)
	return View{Worktree: worktree, Tracker: tracker}, reports, errors.Join(worktreeErr, trackerErr)
}

// legacyOnly turns a corrupt pre-split file into a report instead of a failure.
// Only the pre-split path is downgraded: a corrupt split file is still the error
// the caller grades as unevaluable.
func legacyOnly(err error, loc StoreLocation, report *StoreReport) error {
	var corrupt *StoreCorruptError
	if err == nil || loc.Legacy == "" || !errors.As(err, &corrupt) || corrupt.Path != loc.Legacy {
		return err
	}
	report.LegacyUnreadable = err
	return nil
}

// Backup attempts the one-shot pre-split backup of every ledger that still falls
// back to it, and returns the reports of the attempts. Callers do this before the
// first write, so a repository that only has the pre-split store keeps a copy.
func (l Ledgers) Backup() LedgerReports {
	return LedgerReports{
		Worktree: StoreReport{BackupFailed: l.Worktree.BackupLegacy()},
		Tracker:  StoreReport{BackupFailed: l.Tracker.BackupLegacy()},
	}
}

// save writes the ledgers a mutation changed, and only those: one verb owns one
// ledger's rows. The worktree ledger goes first, so a partial write can never
// leave a tracker row claiming an item the lifecycle file does not know.
func (l Ledgers) save(view View, dirty ledgerDirty) error {
	if dirty.worktree {
		if _, err := l.Worktree.SaveWorktree(view.Worktree); err != nil {
			return err
		}
	}
	if dirty.tracker {
		if _, err := l.Tracker.Save(view.Tracker); err != nil {
			return err
		}
	}
	return nil
}

// ledgerDirty is which of the ledgers a mutation changed, so a verb never writes
// a file it did not own.
type ledgerDirty struct {
	worktree bool
	tracker  bool
}

// LedgerReports is one StoreReport per ledger, so a repository with more than one
// ledger never gets one blended line about "the store".
type LedgerReports struct {
	Worktree StoreReport
	Tracker  StoreReport
}

// Note writes one line per problem, naming the ledger that had it. A clean read or
// write writes nothing.
func (r LedgerReports) Note(w io.Writer, prefix string) {
	r.Worktree.Note(w, prefix+" worktree-ledger")
	r.Tracker.Note(w, prefix+" tracker-ledger")
}

// StoreReport is what reaching the store had to do beyond a plain read or write.
// Nothing here is fatal: it exists so the CLI can say it out loud instead of
// dropping rows or a backup in silence. It is never persisted.
type StoreReport struct {
	// Absorbed is how many rows the pre-split store carried that the split ledger
	// did not have. They are part of the returned store, so grading sees them, and
	// the next write carries them into the split file.
	Absorbed int
	// Shadowed is how many rows the pre-split store also has with different content
	// in the fields this ledger owns: an older ai-specs recorded a decision on a row
	// the split ledger already had. The split ledger's version answers; the
	// disagreement is reported so nobody assumes the two files say the same thing.
	Shadowed int
	// LegacyRows is the pre-split file those rows came from, for the message.
	LegacyRows string
	// LegacyUnreadable is the pre-split store that could not be read while the
	// split ledger answered. The read still succeeded, without its rows.
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

// codec is one ledger's own view of its file: its empty value, the JSON codec for
// its shape, and how it takes its share of a pre-split row. It is what keeps one
// set of migration invariants for every ledger instead of a copy per file.
type codec[T any] struct {
	empty  func() T
	decode func(path string, data []byte) (T, error)
	encode func(T) ([]byte, error)
	// absorb adds this ledger's own share of the pre-split rows it is missing, by
	// row id, and counts the rows both files have with different content. A
	// disagreement in a field this ledger does not own is never counted.
	absorb func(*T, LegacyStore) (absorbed, shadowed int)
}

// trackerCodec is the tracker ledger's file shape: the tracker rows plus the
// lifecycle-scoped opt-outs, with no status column and no close decision (L8).
var trackerCodec = codec[Store]{
	empty: func() Store { return Store{V: StoreVersion} },
	decode: func(path string, data []byte) (Store, error) {
		var store Store
		if err := json.Unmarshal(data, &store); err != nil {
			return Store{}, &StoreCorruptError{Path: path, Err: err}
		}
		if store.V != StoreVersion {
			return Store{}, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported store version %d", store.V)}
		}
		return store, nil
	},
	encode: func(store Store) ([]byte, error) {
		if store.V == 0 {
			store.V = StoreVersion
		}
		if store.Items == nil {
			store.Items = []Item{}
		}
		if store.OptOuts == nil {
			store.OptOuts = []ScopedOptOut{}
		}
		data, err := json.MarshalIndent(store, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	},
	absorb: absorbTracker,
}

// worktreeCodec is the worktree ledger's file shape: the lifecycle rows only.
var worktreeCodec = codec[WorktreeStore]{
	empty: func() WorktreeStore { return WorktreeStore{V: StoreVersion} },
	decode: func(path string, data []byte) (WorktreeStore, error) {
		var store WorktreeStore
		if err := json.Unmarshal(data, &store); err != nil {
			return WorktreeStore{}, &StoreCorruptError{Path: path, Err: err}
		}
		if store.V != StoreVersion {
			return WorktreeStore{}, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported store version %d", store.V)}
		}
		return store, nil
	},
	encode: func(store WorktreeStore) ([]byte, error) {
		if store.V == 0 {
			store.V = StoreVersion
		}
		if store.Items == nil {
			store.Items = []WorktreeItem{}
		}
		data, err := json.MarshalIndent(store, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	},
	absorb: absorbWorktree,
}

// Load reads the tracker ledger at the location. A missing Path falls back to
// Legacy for this release version, unless there is none. A corrupt split file is
// reported and never replaced by the older file: history must not come back as
// live state. A corrupt fallback with no split file is reported, never read as an
// empty store.
func (l StoreLocation) Load() (Store, StoreReport, error) { return loadLedger(l, trackerCodec) }

// Save writes the tracker ledger at the location atomically and never rewrites
// the pre-split one. Callers hold the store lock.
func (l StoreLocation) Save(store Store) (StoreReport, error) {
	return saveLedger(l, trackerCodec, store)
}

// LoadWorktree reads the worktree ledger at the location with the same
// one-version double read: a missing split file falls back to the pre-split store,
// a corrupt split file is reported and never replaced by it, and the rows the
// split file lacks are part of the answer — each ledger taking only its own share
// of a pre-split row.
func (l StoreLocation) LoadWorktree() (WorktreeStore, StoreReport, error) {
	return loadLedger(l, worktreeCodec)
}

// SaveWorktree writes the worktree ledger at the location atomically and never
// rewrites the pre-split one. Callers hold the store lock.
func (l StoreLocation) SaveWorktree(store WorktreeStore) (StoreReport, error) {
	return saveLedger(l, worktreeCodec, store)
}

// loadLedger is the one read every ledger performs. When both files exist the rows
// of the pre-split store that the split ledger does not have are part of the
// answer: until this release reaches every hook, an older ai-specs can still
// record a decision in the pre-split file, and silently ignoring it would drop a
// human opt-out or let a second item be opened for an identity that already has
// one. Rows already present keep their position and bytes.
func loadLedger[T any](l StoreLocation, c codec[T]) (T, StoreReport, error) {
	var report StoreReport
	if l.Path == "" {
		return c.empty(), report, nil
	}
	_, err := os.Stat(l.Path)
	switch {
	case err == nil:
		split, err := c.read(l.Path)
		if err != nil {
			return c.empty(), report, err
		}
		return absorbLegacy(split, l, c, &report)
	case !errors.Is(err, fs.ErrNotExist):
		return c.empty(), report, &StoreCorruptError{Path: l.Path, Err: err}
	}
	if l.Legacy == "" {
		return c.empty(), report, nil
	}
	legacy, err := LoadLegacyStore(l.Legacy)
	if err != nil {
		return c.empty(), report, err
	}
	// No split file yet: the pre-split file is the answer, and this ledger takes
	// its own share of it. Nothing is "missing", so nothing is reported.
	share := c.empty()
	c.absorb(&share, legacy)
	return share, report, nil
}

// absorbLegacy merges the pre-split rows the split ledger is missing. A corrupt
// pre-split file never fails the read — the split ledger answered — but it is
// reported, because its rows cannot be shown.
func absorbLegacy[T any](split T, l StoreLocation, c codec[T], report *StoreReport) (T, StoreReport, error) {
	if l.Legacy == "" {
		return split, *report, nil
	}
	legacy, err := LoadLegacyStore(l.Legacy)
	if err != nil {
		report.LegacyUnreadable = err
		return split, *report, nil
	}
	absorbed, shadowed := c.absorb(&split, legacy)
	report.Absorbed, report.Shadowed, report.LegacyRows = absorbed, shadowed, l.Legacy
	return split, *report, nil
}

// read decodes the split file, reporting a corrupt one as *StoreCorruptError.
func (c codec[T]) read(path string) (T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return c.empty(), &StoreCorruptError{Path: path, Err: err}
	}
	return c.decode(path, data)
}

// saveLedger is the one write every ledger performs: it keeps the one-shot copy
// of the pre-split file, then replaces the split file atomically and never
// rewrites the pre-split one, so its rows stay readable byte-identically.
func saveLedger[T any](l StoreLocation, c codec[T], store T) (StoreReport, error) {
	var report StoreReport
	if l.Legacy != "" {
		report.BackupFailed = l.BackupLegacy()
	}
	data, err := c.encode(store)
	if err != nil {
		return report, err
	}
	return report, writeAtomic(l.Path, data)
}

// optOutKey identifies one scoped opt-out record for de-duplication.
func optOutKey(o ScopedOptOut) string {
	return o.Key + "\x1f" + o.Checkpoint + "\x1f" + o.At
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
