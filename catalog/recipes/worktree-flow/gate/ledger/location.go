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

// One ledger file, one section per capability (L14). The layout is not a contract
// (S14): what matters is that each capability writes only its own section, and that
// a single atomic rename makes a write all-or-nothing.

const (
	// LedgerVersion is the only schema version this build writes. A v1 file at the
	// same path is the pre-split predecessor and is migrated on read.
	LedgerVersion = 2

	// storeFileName is the ledger file's name. The pre-split v1 store used the same
	// path, so a v1 file there is normalised into sections in memory and the first
	// write rewrites it as v2.
	storeFileName = "state.json"
)

// StorePath is the ledger file's location: <git-common-dir>/ai-specs/ledger/state.json.
func StorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledger", storeFileName)
}

// TrackerStorePath is T5's mixed tracker file: the second migration source, read
// only, never written. Nothing in this build treats it as primary state.
func TrackerStorePath(gitCommonDir string) string {
	return filepath.Join(gitCommonDir, "ai-specs", "ledgers", "tracker.json")
}

// LockPath is the lock that serialises writers of one ledger file. It keeps T5's
// name, so two versions of the binary exclude each other.
func LockPath(storePath string) string { return storePath + ".lock" }

// LedgerFile is the one on-disk ledger: a version and one section per capability.
type LedgerFile struct {
	V        int           `json:"v"`
	Worktree WorktreeStore `json:"worktree"`
	Tracker  Store         `json:"tracker"`
	Plan     PlanSection   `json:"plan"`
}

// PlanSection is the plan-build-flow ledger's section. T6a declares the shape only:
// no plan row is written until the plan store lands.
type PlanSection struct {
	Items []json.RawMessage `json:"items"`
}

// EmptyLedger is a fresh v2 ledger with every section present and empty.
func EmptyLedger() LedgerFile {
	return LedgerFile{
		V:        LedgerVersion,
		Worktree: WorktreeStore{Items: []WorktreeItem{}},
		Tracker:  Store{Items: []Item{}, OptOuts: []ScopedOptOut{}},
		Plan:     PlanSection{Items: []json.RawMessage{}},
	}
}

// View composes the read-only contract from the file's sections (L8).
func (f LedgerFile) View() View { return View{Worktree: f.Worktree, Tracker: f.Tracker} }

// StoreLocation is the ledger file one invocation reads and writes: Path is THE
// file, and Migration is T5's mixed tracker file, absorbed on read and never
// written. `--store` names Path and nothing else.
type StoreLocation struct {
	Path      string
	Migration string
}

// LedgerAt is a repository's ledger: the v2 file plus T5's tracker file as the
// second migration source.
func LedgerAt(gitCommonDir string) StoreLocation {
	return StoreLocation{Path: StorePath(gitCommonDir), Migration: TrackerStorePath(gitCommonDir)}
}

// StoreAt is an explicitly named ledger file: exactly that file, no migration
// source beside it.
func StoreAt(path string) StoreLocation { return StoreLocation{Path: path} }

// StoreReport is what reaching the ledger had to do beyond a plain read or write.
// Nothing here is fatal: it exists so the CLI can say it out loud instead of
// dropping rows or a backup in silence. It is never persisted.
type StoreReport struct {
	// Absorbed is how many rows and opt-outs a predecessor source carried that the
	// ledger file did not have. They are part of the answer, and the next write
	// carries them into the file.
	Absorbed int
	// Shadowed is how many rows both the file and a source have with different
	// content: the file's version answers, and the disagreement is reported.
	Shadowed int
	// LegacyRows is the source those rows came from, for the message.
	LegacyRows string
	// LegacyUnreadable is a source that could not be read. The read still succeeded.
	LegacyUnreadable error
	// BackupFailed is a one-shot copy of a predecessor that could not be written
	// before the first v2 write.
	BackupFailed error
}

// Empty reports whether a read or write was a plain one.
func (r StoreReport) Empty() bool {
	return r.Absorbed == 0 && r.Shadowed == 0 && r.LegacyUnreadable == nil && r.BackupFailed == nil
}

// Note writes one line per problem to w. A clean read or write writes nothing.
func (r StoreReport) Note(w io.Writer, prefix string) {
	if r.Absorbed > 0 {
		fmt.Fprintf(w, "%s: %d ledger row(s) from %s are part of this answer; the next write carries them into the ledger file\n",
			prefix, r.Absorbed, r.LegacyRows)
	}
	if r.Shadowed > 0 {
		fmt.Fprintf(w, "%s: %d ledger row(s) differ in %s because an older ai-specs wrote them; the ledger file's version answers\n",
			prefix, r.Shadowed, r.LegacyRows)
	}
	if r.LegacyUnreadable != nil {
		fmt.Fprintf(w, "%s: the predecessor ledger %s is unreadable (%v); its rows are missing from this answer\n",
			prefix, r.LegacyRows, r.LegacyUnreadable)
	}
	if r.BackupFailed != nil {
		fmt.Fprintf(w, "%s: could not write a one-shot backup of a predecessor ledger (%v); those files are untouched, so no row is lost\n",
			prefix, r.BackupFailed)
	}
}

// Load reads the ledger file and migrates every predecessor source into sections in
// memory: a v1 file at Path is normalised, and Migration contributes the rows the
// file lacks. Only fs.ErrNotExist means absent — any other read error is corruption,
// so a permission error or a symlink loop can never become "empty and writable".
func (l StoreLocation) Load() (LedgerFile, StoreReport, error) {
	var report StoreReport
	if l.Path == "" {
		return EmptyLedger(), report, nil
	}

	switch err := statExact(l.Path); {
	case err == nil:
		file, isV1, err := readLedgerFile(l.Path)
		if err != nil {
			return EmptyLedger(), report, err
		}
		if isV1 {
			report.LegacyRows = l.Path
		}
		if err := l.absorbMigration(&file, &report); err != nil {
			return EmptyLedger(), report, err
		}
		return file, report, nil
	case errors.Is(err, fs.ErrNotExist):
		file := EmptyLedger()
		if err := l.absorbMigration(&file, &report); err != nil {
			return EmptyLedger(), report, err
		}
		return file, report, nil
	default:
		return EmptyLedger(), report, &StoreCorruptError{Path: l.Path, Err: err}
	}
}

// absorbMigration merges T5's mixed tracker file into the sections. A source that is
// absent contributes nothing; any other error is corruption.
func (l StoreLocation) absorbMigration(file *LedgerFile, report *StoreReport) error {
	if l.Migration == "" || l.Migration == l.Path {
		return nil
	}
	switch err := statExact(l.Migration); {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return &StoreCorruptError{Path: l.Migration, Err: err}
	}
	data, err := os.ReadFile(l.Migration)
	if err != nil {
		return &StoreCorruptError{Path: l.Migration, Err: err}
	}
	legacy, err := decodeLegacy(data, l.Migration)
	if err != nil {
		return err
	}
	absorbed, shadowed := mergeLegacy(file, legacy)
	report.Absorbed += absorbed
	report.Shadowed += shadowed
	report.LegacyRows = l.Migration
	return nil
}

// Save writes the whole file atomically, after keeping a one-shot copy of every
// predecessor source it is about to supersede. One rename is the transaction.
func (l StoreLocation) Save(file LedgerFile) (StoreReport, error) {
	report := StoreReport{BackupFailed: errors.Join(l.backupPredecessor(l.Path), l.backupPredecessor(l.Migration))}
	data, err := encodeLedgerFile(file)
	if err != nil {
		return report, err
	}
	return report, writeAtomic(l.Path, data)
}

// Backup keeps the one-shot predecessor copies without writing the ledger file.
// Callers do this before the first write of an invocation that mutates.
func (l StoreLocation) Backup() StoreReport {
	return StoreReport{BackupFailed: errors.Join(l.backupPredecessor(l.Path), l.backupPredecessor(l.Migration))}
}

// backupPredecessor keeps one copy of a v1 predecessor as <path>.v1-backup. A v2
// file is not a predecessor, so it is not copied; an existing backup is never
// replaced, and a missing source has nothing to copy.
func (l StoreLocation) backupPredecessor(path string) error {
	if path == "" {
		return nil
	}
	backup := path + ".v1-backup"
	if _, err := os.Stat(backup); err == nil {
		return nil
	}
	switch err := statExact(path); {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !isV1(data) {
		return nil
	}
	return writeAtomic(backup, data)
}

// LegacyStore is the pre-split single store (v1), the shape both predecessor files
// have. This build only reads it.
type LegacyStore struct {
	V       int            `json:"v"`
	Items   []LegacyItem   `json:"items"`
	OptOuts []ScopedOptOut `json:"opt_outs"`
}

// LegacyItem is one v1 row: the tracker row plus the status column the worktree
// section owns now. Embedding keeps every shared field, tag and rule in one place.
type LegacyItem struct {
	Status string `json:"status"`
	Item
}

// readLedgerFile decodes Path, reporting whether it was a v1 predecessor.
func readLedgerFile(path string) (LedgerFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EmptyLedger(), false, &StoreCorruptError{Path: path, Err: err}
	}
	if isV1(data) {
		legacy, err := decodeLegacy(data, path)
		if err != nil {
			return EmptyLedger(), false, err
		}
		return legacyAsLedger(legacy), true, nil
	}
	var file LedgerFile
	if err := json.Unmarshal(data, &file); err != nil {
		return EmptyLedger(), false, &StoreCorruptError{Path: path, Err: err}
	}
	if file.V != LedgerVersion {
		return EmptyLedger(), false, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported ledger version %d", file.V)}
	}
	return file, false, nil
}

// isV1 reports whether the bytes are a v1 predecessor store. Anything that is not a
// versioned object is left to the decoder, so corruption is reported, not guessed.
func isV1(data []byte) bool {
	var probe struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.V == 1
}

// decodeLegacy reads a v1 predecessor store, reusing the item shape.
func decodeLegacy(data []byte, path string) (LegacyStore, error) {
	var store LegacyStore
	if err := json.Unmarshal(data, &store); err != nil {
		return LegacyStore{}, &StoreCorruptError{Path: path, Err: err}
	}
	if store.V != 1 {
		return LegacyStore{}, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported store version %d", store.V)}
	}
	return store, nil
}

// legacyAsLedger normalises a v1 store into sections: each row gives the worktree
// section its status and closes and the tracker section the record it owns.
func legacyAsLedger(legacy LegacyStore) LedgerFile {
	file := EmptyLedger()
	file.Tracker.OptOuts = append(file.Tracker.OptOuts, legacy.OptOuts...)
	absorbed, _ := mergeLegacy(&file, legacy)
	_ = absorbed
	return file
}

// mergeLegacy merges a v1 store into the sections: rows the file lacks are added to
// both sections, a close a worktree row is missing is merged into it, and opt-outs
// the tracker section lacks are added. Nothing is dropped and nothing is reordered.
func mergeLegacy(file *LedgerFile, legacy LegacyStore) (absorbed, shadowed int) {
	worktree := &file.Worktree
	seenWorktree := make(map[string]int, len(worktree.Items))
	for i, row := range worktree.Items {
		seenWorktree[row.ID] = i
	}
	tracker := &file.Tracker
	seenTracker := make(map[string]int, len(tracker.Items))
	for i, item := range tracker.Items {
		seenTracker[item.ID] = i
	}

	for _, row := range legacy.Items {
		// Absorbed counts source rows, not per-section edits: one row that gives both
		// sections something is one row "part of this answer".
		contributed := false
		share := worktreeShare(row)
		if share.Status == "" && len(share.Decisions) == 0 {
			// A source row with no lifecycle at all (a normalised tracker file) has
			// nothing for the worktree section to learn.
		} else if at, ok := seenWorktree[share.ID]; ok {
			if mergeCloses(&worktree.Items[at], share.Decisions) > 0 {
				contributed = true
			}
			if worktree.Items[at].Status != share.Status {
				shadowed++
			}
		} else {
			seenWorktree[share.ID] = len(worktree.Items)
			worktree.Items = append(worktree.Items, share)
			contributed = true
		}

		record := trackerShare(row)
		if at, ok := seenTracker[record.ID]; ok {
			// The ledger file's version answers; a disagreement in what the source
			// recorded is reported so nobody assumes the two agree.
			if !sameRecord(tracker.Items[at], record) {
				shadowed++
			}
		} else {
			seenTracker[record.ID] = len(tracker.Items)
			tracker.Items = append(tracker.Items, record)
			contributed = true
		}
		if contributed {
			absorbed++
		}
	}

	seenOptOut := make(map[string]int, len(tracker.OptOuts))
	for i, optOut := range tracker.OptOuts {
		seenOptOut[optOutKey(optOut)] = i
	}
	for _, optOut := range legacy.OptOuts {
		key := optOutKey(optOut)
		if _, ok := seenOptOut[key]; ok {
			continue
		}
		seenOptOut[key] = len(tracker.OptOuts)
		tracker.OptOuts = append(tracker.OptOuts, optOut)
		absorbed++
	}
	return absorbed, shadowed
}

// sameRecord reports whether two tracker rows are the same record. The source's
// version answering is the file's, so a difference is reported, never adopted.
func sameRecord(a, b Item) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && string(left) == string(right)
}

// encodeLedgerFile renders a file with every section present, so a fresh ledger
// always has the same shape.
func encodeLedgerFile(file LedgerFile) ([]byte, error) {
	if file.V == 0 {
		file.V = LedgerVersion
	}
	if file.Worktree.Items == nil {
		file.Worktree.Items = []WorktreeItem{}
	}
	if file.Tracker.Items == nil {
		file.Tracker.Items = []Item{}
	}
	if file.Tracker.OptOuts == nil {
		file.Tracker.OptOuts = []ScopedOptOut{}
	}
	if file.Plan.Items == nil {
		file.Plan.Items = []json.RawMessage{}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// statExact reports the file's state, distinguishing "absent" from any other error
// so corruption is never read as an empty ledger.
func statExact(path string) error {
	_, err := os.Stat(path)
	return err
}

// optOutKey identifies one scoped opt-out record for de-duplication.
func optOutKey(o ScopedOptOut) string {
	return o.Key + "\x1f" + o.Checkpoint + "\x1f" + o.At
}
