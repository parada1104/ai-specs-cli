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

// Ledger file names: the canonical name of each ledger is the whole contract an
// override resolves against (L12.4).
const (
	worktreeFileName = "worktree.json"
	trackerFileName  = "tracker.json"
)

// StoreLocation is one ledger store on disk: Path is the file this build writes,
// Legacy is the pre-split single store read once for the one-version double read
// (S11), and Inherit is another ledger's file that still carries this ledger's
// rows for one release — T5's mixed ledgers/tracker.json, the immediate
// predecessor of the split, which still holds each row's status and closes. An
// empty Legacy means "no fallback and no backup".
type StoreLocation struct {
	Path    string
	Legacy  string
	Inherit string
}

// sources are the pre-split files this ledger is handed forward from, in the order
// they answer: the file that already owns the row first, the shared pre-split
// single store last.
func (l StoreLocation) sources() []string {
	var sources []string
	if l.Inherit != "" {
		sources = append(sources, l.Inherit)
	}
	if l.Legacy != "" && l.Legacy != l.Inherit {
		sources = append(sources, l.Legacy)
	}
	return sources
}

// TrackerLocation is the tracker ledger's location under a repository's common
// dir: the split store plus the pre-split file it is handed forward from.
func TrackerLocation(gitCommonDir string) StoreLocation {
	return StoreLocation{Path: TrackerStorePath(gitCommonDir), Legacy: StorePath(gitCommonDir)}
}

// WorktreeLocation is the worktree ledger's location under a repository's common
// dir: the split store, the pre-split single store, and T5's mixed tracker file —
// the immediate predecessor of the field split, whose rows still carry status and
// close decisions. Without that second source a repository upgraded straight from
// T5 loses its lifecycle (L11).
func WorktreeLocation(gitCommonDir string) StoreLocation {
	return StoreLocation{
		Path:    WorktreeStorePath(gitCommonDir),
		Legacy:  StorePath(gitCommonDir),
		Inherit: TrackerStorePath(gitCommonDir),
	}
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

// LedgersAtStore resolves the ledger set from an explicitly named override. The
// name decides which ledger the override is (L12.4): an override naming
// worktree.json or tracker.json selects that ledger and derives the other's
// canonical file beside it, so two ledgers are never aliased onto one file. An
// override naming the pre-split state.json selects it as the read-only legacy
// source of both ledgers, so it is never rewritten. Any other name keeps T5's
// meaning — the tracker ledger's file — with the worktree sibling beside it.
func LedgersAtStore(path string) Ledgers {
	dir := filepath.Dir(path)
	worktree := filepath.Join(dir, worktreeFileName)
	tracker := filepath.Join(dir, trackerFileName)
	switch filepath.Base(path) {
	case worktreeFileName:
		return Ledgers{Worktree: StoreAt(path), Tracker: StoreAt(tracker)}
	case trackerFileName:
		return Ledgers{Worktree: StoreAt(worktree), Tracker: StoreAt(path)}
	case filepath.Base(StorePath("")):
		return Ledgers{
			Worktree: StoreLocation{Path: worktree, Legacy: path},
			Tracker:  StoreLocation{Path: tracker, Legacy: path},
		}
	}
	return Ledgers{Worktree: StoreAt(worktree), Tracker: StoreAt(path)}
}

// LockPath is the canonical lock for a ledger directory. One lock covers every
// ledger pair in that directory, so two overrides in it serialize even when they
// name different files (L12.3).
func LockPath(storePath string) string {
	return filepath.Join(filepath.Dir(storePath), ".ledger.lock")
}

// Load reads both ledgers and joins them into the read-only View (L8). Each
// ledger's report is returned separately so a problem is always attributed to the
// file that had it, and a corrupt file on one side still leaves the other side's
// rows readable: only the error it returns is shared.
func (l Ledgers) Load() (View, LedgerReports, error) {
	worktree, worktreeReport, worktreeErr := l.Worktree.LoadWorktree()
	tracker, trackerReport, trackerErr := l.Tracker.Load()
	// The tracker ledger reads its own file normalised: a tracker row carries no
	// close decision, because a close belongs to the worktree ledger (L8). Any
	// close the file still carries is in the worktree ledger's memory, and
	// inheritsClose makes the next tracker write land it there before the tracker
	// file loses it (L11).
	view := View{
		Worktree:      worktree,
		Tracker:       withoutCloseDecisions(tracker),
		inheritsClose: worktreeReport.carried,
	}
	return view, LedgerReports{Worktree: worktreeReport, Tracker: trackerReport},
		errors.Join(worktreeErr, trackerErr)
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
//
// Two rules make the pair behave like one write. A tracker write that would drop a
// close its own file still carries lands the worktree rows that hold it first and
// refuses if it cannot (L11). And a save that fails part way restores every file it
// already replaced, because a lock plus two atomic renames is not a transaction
// (L12.1).
func (l Ledgers) save(view View, dirty ledgerDirty) error {
	if dirty.tracker && view.inheritsClose {
		dirty.worktree = true
	}
	var steps []saveStep
	if dirty.worktree {
		steps = append(steps, saveStep{path: l.Worktree.Path, write: func() error {
			_, err := l.Worktree.SaveWorktree(view.Worktree)
			return err
		}})
	}
	if dirty.tracker {
		steps = append(steps, saveStep{path: l.Tracker.Path, write: func() error {
			_, err := l.Tracker.Save(view.Tracker)
			return err
		}})
	}
	return saveSteps(steps)
}

// saveStep is one file a mutation persists and the write that persists it.
type saveStep struct {
	path  string
	write func() error
}

// saveSteps writes the files in order and restores every file it already replaced
// when a later write fails, returning the original error alongside any restore
// failure so nothing is hidden (L12.1).
func saveSteps(steps []saveStep) error {
	var done []fileSnapshot
	for _, step := range steps {
		snapshot, err := captureFile(step.path)
		if err != nil {
			return errors.Join(err, restoreFiles(done))
		}
		if err := step.write(); err != nil {
			return errors.Join(err, restoreFiles(append(done, snapshot)))
		}
		done = append(done, snapshot)
	}
	return nil
}

// fileSnapshot is one file's pre-write bytes, or the fact that it did not exist.
type fileSnapshot struct {
	path    string
	data    []byte
	existed bool
}

func captureFile(path string) (fileSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileSnapshot{path: path}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{path: path, data: data, existed: true}, nil
}

// restoreFiles puts the snapshots back, newest first, and reports everything it
// could not restore instead of hiding it.
func restoreFiles(snapshots []fileSnapshot) error {
	var failures []error
	for i := len(snapshots) - 1; i >= 0; i-- {
		snapshot := snapshots[i]
		if !snapshot.existed {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				failures = append(failures, err)
			}
			continue
		}
		if err := writeAtomic(snapshot.path, snapshot.data); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
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
	// carried reports a close decision this ledger's file does not hold yet but a
	// file it is about to overwrite does, so the write has to land it first (L11).
	carried bool
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
	// absorb adds this ledger's own share of a pre-split source, by row id, and
	// reports what it had to add. A disagreement in a field this ledger does not
	// own is never counted.
	absorb func(*T, LegacyStore) merge
}

// merge is what absorbing one pre-split source added to a ledger.
type merge struct {
	// absorbed is the rows the ledger's own file did not have at all.
	absorbed int
	// shadowed is the rows both files have with different content in the fields
	// this ledger owns.
	shadowed int
	// carried reports a close decision the ledger's own file did not hold yet, so
	// the file must be written before that source is overwritten (L11).
	carried bool
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

// loadLedger is the one read every ledger performs. When the split file exists the
// rows of every source that it does not have are part of the answer: until this
// release reaches every hook, an older ai-specs can still record a decision in a
// pre-split file, and silently ignoring it would drop a human opt-out, let a
// second item be opened for an identity that already has one, or lose a close
// that only exists in T5's mixed tracker file. Rows already present keep their
// position and bytes.
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
		return absorbSources(split, l, c, &report), report, nil
	case !errors.Is(err, fs.ErrNotExist):
		return c.empty(), report, &StoreCorruptError{Path: l.Path, Err: err}
	}
	// No split file yet: every source this ledger is handed forward from is the
	// answer, and it takes its own share of each. A source that is present but
	// unreadable is unevaluable — corruption is never masked by a synthetic empty
	// store — exactly as the single-ledger read has always been (L11.5).
	share := c.empty()
	for _, source := range l.sources() {
		if !fileExists(source) {
			continue
		}
		legacy, err := LoadLegacyStore(source)
		if err != nil {
			return c.empty(), report, err
		}
		m := c.absorb(&share, legacy)
		if source == l.Inherit {
			report.carried = report.carried || m.carried
		}
		report.LegacyRows = source
	}
	return share, report, nil
}

// absorbSources merges this ledger's share of every pre-split source. A corrupt
// source never fails the read — the split file answered — but it is reported,
// because its rows cannot be shown.
func absorbSources[T any](split T, l StoreLocation, c codec[T], report *StoreReport) T {
	for _, source := range l.sources() {
		if !fileExists(source) {
			continue
		}
		legacy, err := LoadLegacyStore(source)
		if err != nil {
			report.LegacyUnreadable = err
			continue
		}
		m := c.absorb(&split, legacy)
		report.Absorbed += m.absorbed
		report.Shadowed += m.shadowed
		if source == l.Inherit {
			report.carried = report.carried || m.carried
		}
		report.LegacyRows = source
	}
	return split
}

// fileExists reports whether a source is there at all. A source that is absent
// simply has nothing to hand forward.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
