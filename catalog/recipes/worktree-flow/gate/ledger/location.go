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

// EmptyLedger is a fresh v2 ledger with every section present and empty.
func EmptyLedger() LedgerFile {
	return LedgerFile{
		V:        LedgerVersion,
		Worktree: WorktreeStore{Items: []WorktreeItem{}},
		Tracker:  Store{Items: []Item{}, OptOuts: []ScopedOptOut{}},
		Plan:     PlanSection{Items: []PlanItem{}},
	}
}

// View composes the read-only contract from the file's sections (L8).
func (f LedgerFile) View() View { return View{Worktree: f.Worktree, Tracker: f.Tracker} }

// StoreLocation is the ledger file one invocation reads and writes: Path is THE
// file, Migration is T5's mixed tracker file, and Plan is the checkout's plan
// predecessor; both sources are absorbed on read and never written. `--store` names
// Path and nothing else.
type StoreLocation struct {
	Path      string
	Migration string
	Plan      PlanSource
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
	// Path is the ledger file this read or write reached: the file whose version of a
	// row answers when a source disagrees, so a note can name it (T5 R1).
	Path string
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
		// T5 R1: name the file that wins, not just the one that lost, so a reader can
		// tell which version answered without opening either path.
		winner := r.Path
		if winner == "" {
			winner = "the ledger file"
		}
		fmt.Fprintf(w, "%s: %d ledger row(s) differ in %s because an older ai-specs wrote them; %s is the file that answers\n",
			prefix, r.Shadowed, r.LegacyRows, winner)
	}
	if r.LegacyUnreadable != nil {
		fmt.Fprintf(w, "%s: the predecessor ledger %s is unreadable (%v); its rows are missing from this answer\n",
			prefix, r.LegacyRows, r.LegacyUnreadable)
	}
	if r.BackupFailed != nil {
		fmt.Fprintf(w, "%s: could not write a backup of a predecessor ledger (%v); the ledger was not written, so nothing was replaced\n",
			prefix, r.BackupFailed)
	}
}

// Load reads the ledger file and migrates every predecessor source into sections in
// memory: a v1 file at Path is normalised, and Migration contributes the rows the
// file lacks. Only fs.ErrNotExist means absent — any other read error is corruption,
// so a permission error or a symlink loop can never become "empty and writable".
func (l StoreLocation) Load() (LedgerFile, StoreReport, error) {
	report := StoreReport{Path: l.Path}
	if l.Path == "" {
		return EmptyLedger(), report, nil
	}

	file, err := l.loadFile(&report)
	if err != nil {
		return EmptyLedger(), report, err
	}
	// The plan row this checkout owns is migrated before the predecessor is read, so
	// the seeding lands in the row that already belongs to it.
	adoptPlanRow(&file, l.Plan.Identity)
	l.absorbPlanSource(&file, &report)
	settleCloses(&file)
	return file, report, nil
}

// loadFile reads Path and every predecessor except the plan source, which the caller
// absorbs after it, so the seeding happens once on every read path.
func (l StoreLocation) loadFile(report *StoreReport) (LedgerFile, error) {
	switch err := statExact(l.Path); {
	case err == nil:
		file, stale, err := readLedgerFile(l.Path)
		if err != nil {
			return EmptyLedger(), err
		}
		if stale == nil {
			// Already v2: its own version answers, and a source only adds what it lacks.
			l.absorbMigration(&file, report)
			return file, nil
		}
		// Path is still v1, so it is a stale predecessor: 0.28.0 wrote it and T5 never
		// did. T5's mixed tracker file is the newest superset, so it is the base and
		// the v1 file only contributes the rows it lacks (L17 ALTO-1).
		base := EmptyLedger()
		l.absorbMigration(&base, report)
		if added := mergeLegacyMissing(&base, *stale); added > 0 {
			report.Absorbed += added
		}
		if report.LegacyRows == "" {
			// Name the file the rows actually came from: a corrupt source keeps the
			// name absorbMigration reported, never the ledger's own path (L19).
			report.LegacyRows = l.Path
		}
		return base, nil
	case errors.Is(err, fs.ErrNotExist):
		file := EmptyLedger()
		l.absorbMigration(&file, report)
		return file, nil
	default:
		return EmptyLedger(), &StoreCorruptError{Path: l.Path, Err: err}
	}
}

// absorbMigration merges T5's mixed tracker file into the sections. A source that is
// absent contributes nothing; any other error is corruption.
func (l StoreLocation) absorbMigration(file *LedgerFile, report *StoreReport) {
	if l.Migration == "" || l.Migration == l.Path {
		return
	}
	switch err := statExact(l.Migration); {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		report.LegacyUnreadable = err
		report.LegacyRows = l.Migration
		return
	}
	data, err := os.ReadFile(l.Migration)
	if err != nil {
		// A source that cannot be read is reported, never fatal: this build never
		// writes it, so the ledger's own answer stands (L17 MEDIO-3).
		report.LegacyUnreadable = err
		report.LegacyRows = l.Migration
		return
	}
	legacy, err := decodeLegacy(data, l.Migration)
	if err != nil {
		report.LegacyUnreadable = err
		report.LegacyRows = l.Migration
		return
	}
	absorbed, shadowed := mergeLegacy(file, legacy)
	report.Absorbed += absorbed
	report.Shadowed += shadowed
	report.LegacyRows = l.Migration
}

// Save writes the file atomically, after keeping a copy of every predecessor it is
// about to supersede. A copy that cannot be written fails the save closed: a
// predecessor with no backup must never be replaced (L17 MEDIO-2).
//
// own names the sections the caller owns. Every other section is re-read from disk
// before the write, so no caller can put a row into a capability it does not own
// (S3): the whole-store write that could land a row in the wrong ledger is what the
// ownership contract removes.
func (l StoreLocation) Save(file LedgerFile, own ...Section) (StoreReport, error) {
	report := StoreReport{BackupFailed: errors.Join(
		l.backupPredecessor(l.Path), l.backupPredecessor(l.Migration), l.copyPredecessor(l.Plan.Path))}
	if report.BackupFailed != nil {
		return report, fmt.Errorf("ledger: refusing to write %s without a predecessor backup: %w", l.Path, report.BackupFailed)
	}
	kept, err := l.keepUnowned(file, own)
	if err != nil {
		return report, err
	}
	data, err := encodeLedgerFile(kept)
	if err != nil {
		return report, err
	}
	return report, writeAtomic(l.Path, data)
}

// keepUnowned restores every section the caller did not name from a fresh read. A
// read that fails fails the write closed: an unowned section whose current content
// cannot be known must not be guessed.
func (l StoreLocation) keepUnowned(file LedgerFile, own []Section) (LedgerFile, error) {
	if len(own) == 0 {
		return LedgerFile{}, fmt.Errorf("%w: a write names the sections it owns", ErrInvalidWrite)
	}
	owned := make(map[Section]bool, len(own))
	for _, section := range own {
		owned[section] = true
	}
	// One full read per write, whatever the caller names: the sections it does not own
	// come from disk, the rows it absorbed on load ride along as the report promises,
	// and a ledger nobody can read is never replaced, even by an all-section write.
	onDisk, _, err := l.Load()
	if err != nil {
		return LedgerFile{}, err
	}
	if !owned[SectionWorktree] {
		file.Worktree = onDisk.Worktree
	}
	if !owned[SectionTracker] {
		file.Tracker = onDisk.Tracker
	}
	if !owned[SectionPlan] {
		file.Plan = onDisk.Plan
	}
	return file, nil
}

// Backup keeps the one-shot predecessor copies without writing the ledger file.
// Callers do this before the first write of an invocation that mutates.
func (l StoreLocation) Backup() StoreReport {
	return StoreReport{BackupFailed: errors.Join(
		l.backupPredecessor(l.Path), l.backupPredecessor(l.Migration), l.copyPredecessor(l.Plan.Path))}
}

// backupPredecessor keeps a copy of the bytes a v1 predecessor actually holds: an
// earlier backup is reused only when it holds exactly these bytes, otherwise the copy
// gets a name of its own, so a predecessor that grew is never replaced under a stale
// copy (L17 MEDIO-2). A v2 file is not a predecessor and is not copied; a missing
// source has nothing to copy.
func (l StoreLocation) backupPredecessor(path string) error {
	data, ok, err := predecessorBytes(path)
	if err != nil || !ok || !isV1(data) {
		return err
	}
	return keepCopy(path, data)
}

// copyPredecessor keeps a one-shot copy of a source whatever shape it has: the plan
// predecessor is a line file, not a v1 store, and it is still replaced by the first
// write that seeds its events into the ledger. A missing source has nothing to copy.
func (l StoreLocation) copyPredecessor(path string) error {
	data, ok, err := predecessorBytes(path)
	if err != nil || !ok {
		return err
	}
	return keepCopy(path, data)
}

// predecessorBytes reads the bytes a predecessor holds. A path that is absent, or
// that is not a regular file, holds no predecessor bytes: a directory where a
// ledger file belongs is reported on load as an unreadable source and is not a
// failed backup, so it never disables the write (L17 MEDIO-3). A regular file whose
// bytes cannot be read is still an error, and the save fails closed.
func predecessorBytes(path string) ([]byte, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case !info.Mode().IsRegular():
		return nil, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// keepCopy writes path's bytes to a one-shot copy: an existing copy is reused only
// when it holds exactly these bytes, otherwise the copy gets a name of its own, so a
// predecessor that grew is never replaced under a stale copy (L17 MEDIO-2).
func keepCopy(path string, data []byte) error {
	for n := 0; ; n++ {
		name := path + ".v1-backup"
		if n > 0 {
			name = fmt.Sprintf("%s.v1-backup.%d", path, n)
		}
		existing, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			return writeAtomic(name, data)
		}
		if err != nil {
			return err
		}
		if string(existing) == string(data) {
			return nil
		}
	}
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

// readLedgerFile decodes Path. A v1 predecessor is returned as the store it is, so the
// caller can decide how much of it is still current: only a v2 file has a version of
// its own that answers.
func readLedgerFile(path string) (LedgerFile, *LegacyStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EmptyLedger(), nil, &StoreCorruptError{Path: path, Err: err}
	}
	if isV1(data) {
		legacy, err := decodeLegacy(data, path)
		if err != nil {
			return EmptyLedger(), nil, err
		}
		return legacyAsLedger(legacy), &legacy, nil
	}
	var file LedgerFile
	if err := json.Unmarshal(data, &file); err != nil {
		return EmptyLedger(), nil, &StoreCorruptError{Path: path, Err: err}
	}
	if file.V != LedgerVersion {
		return EmptyLedger(), nil, &StoreCorruptError{Path: path, Err: fmt.Errorf("unsupported ledger version %d", file.V)}
	}
	return file, nil, nil
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
		// sections something is one row "part of this answer". Shadowed likewise counts
		// a row once, however many of its fields disagree (L17).
		contributed := false
		differs := false
		share := worktreeShare(row)
		if share.Status == "" && len(share.Decisions) == 0 {
			// A source row with no lifecycle at all (a normalised tracker file) has
			// nothing for the worktree section to learn.
		} else if at, ok := seenWorktree[share.ID]; ok {
			if mergeCloses(&worktree.Items[at], share.Decisions) > 0 {
				contributed = true
			}
			if worktree.Items[at].Status != share.Status {
				differs = true
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
				differs = true
			}
		} else {
			seenTracker[record.ID] = len(tracker.Items)
			tracker.Items = append(tracker.Items, record)
			contributed = true
		}
		if contributed {
			absorbed++
		}
		if differs {
			shadowed++
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

// mergeLegacyMissing merges only the rows the sections do not already have. It is what
// a stale v1 file contributes when T5's newer tracker file is the base (L17 ALTO-1): it
// never overrides a newer status, card or close.
func mergeLegacyMissing(file *LedgerFile, legacy LegacyStore) (absorbed int) {
	worktreeAt := make(map[string]int, len(file.Worktree.Items))
	for i, row := range file.Worktree.Items {
		worktreeAt[row.ID] = i
	}
	seenTracker := make(map[string]bool, len(file.Tracker.Items))
	for _, item := range file.Tracker.Items {
		seenTracker[item.ID] = true
	}
	for _, row := range legacy.Items {
		share := worktreeShare(row)
		if at, ok := worktreeAt[share.ID]; ok {
			// The newer file owns the record and the status, but a close 0.28.0 wrote
			// after T5 copied the row is still terminal (D17): merge it and let
			// settleCloses act. Nothing else is taken from the stale row.
			if mergeCloses(&file.Worktree.Items[at], share.Decisions) > 0 {
				absorbed++
			}
		} else if share.Status != "" || len(share.Decisions) > 0 {
			worktreeAt[share.ID] = len(file.Worktree.Items)
			file.Worktree.Items = append(file.Worktree.Items, share)
			absorbed++
		}
		record := trackerShare(row)
		if !seenTracker[record.ID] {
			seenTracker[record.ID] = true
			file.Tracker.Items = append(file.Tracker.Items, record)
		}
	}
	// A scoped opt-out is a human decision that exists in no other file, so it is
	// merged exactly as the primary merge does — without it the upgrade answers
	// needs-item where 0.28.0 answered opt-out (L19).
	seenOptOut := make(map[string]bool, len(file.Tracker.OptOuts))
	for _, optOut := range file.Tracker.OptOuts {
		seenOptOut[optOutKey(optOut)] = true
	}
	for _, optOut := range legacy.OptOuts {
		key := optOutKey(optOut)
		if seenOptOut[key] {
			continue
		}
		seenOptOut[key] = true
		file.Tracker.OptOuts = append(file.Tracker.OptOuts, optOut)
		absorbed++
	}
	return absorbed
}

// settleCloses makes every row that carries a close closed: a close is terminal (D17),
// whatever order the sources were merged in (L17 ALTO-2).
func settleCloses(file *LedgerFile) {
	for i := range file.Worktree.Items {
		for _, d := range file.Worktree.Items[i].Decisions {
			if d.Kind == DecisionClose {
				file.Worktree.Items[i].Status = StatusClosed
				break
			}
		}
	}
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
		file.Plan.Items = []PlanItem{}
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
