package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The plan ledger: the change's own flow (S3). It owns whether this worktree is a
// workstream at all, and later its exemption and archive decisions. Nothing here
// writes a lifecycle row or a tracker record, and nothing outside this file writes
// the plan section.

const (
	// PlanKindWorkstream and PlanKindWorkstreamEnd are the declaration and its end.
	// A later unit adds the plan exemption and the archive decision; a reader skips
	// a kind it does not know.
	PlanKindWorkstream    = "workstream"
	PlanKindWorkstreamEnd = "workstream-end"
)

// PlanIdentity is a plan row's key: the checkout instance, not the branch and not the
// path. A workstream declaration belongs to the worktree it was made in, so renaming the
// branch or moving the folder must neither lose its end nor resurrect it. Two parts make
// the instance: git's own name for the checkout, and the creation token this build
// writes into its admin directory — because git reuses that name when a worktree is
// removed and another is created in the same place, and a recreated worktree is not the
// one that declared anything. The path and the branch travel as data so a report can
// still name them.
type PlanIdentity struct {
	CommonDir string `json:"common_dir"`
	// Worktree is mainWorktree for the checkout that owns the git dir, and
	// "worktrees/<name>" for a linked one: the admin directory git keeps for it, which
	// no path or branch rename can change.
	Worktree string `json:"worktree"`
	// Created is the creation token of that admin directory: a random value written once
	// beside it, so a worktree git recreates under the same name is a different
	// checkout. An empty value is a row written by a build that had no token, and such a
	// row is not moved by adoptPlanRow.
	Created string `json:"created,omitempty"`
	Root    string `json:"root"`
	Branch  string `json:"branch,omitempty"`
}

// mainWorktree is the prepared name of the checkout that owns the git dir. A linked
// worktree cannot collide with it, because its own name is always prefixed.
const mainWorktree = "main"

// createdTokenFile is where the checkout's creation token lives, inside the admin
// directory git removes with the worktree: a recreated worktree starts without one.
const createdTokenFile = "ai-specs-created"

// Key is the row's matching key: common dir, the worktree name and the token of the
// instance, never the branch and never the path.
func (i PlanIdentity) Key() string {
	return i.CommonDir + "\x1f" + i.Worktree + "\x1f" + i.Created
}

// PlanIdentityAt derives the plan key for the checkout containing dir AND mints its
// creation token when the admin directory has none. Only the write path may call it: a
// read that minted a token would make a report a write (T7/L5-6). Unlike the item
// identity it needs no commit, so a declaration can be recorded before the branch has
// one; the branch and the path are recorded when git can name them.
func PlanIdentityAt(dir string, facts Facts) PlanIdentity {
	return planIdentityAt(dir, facts, true)
}

// PlanIdentityReadAt is the read form of the same identity: it reads the creation token
// and never writes one, so a read leaves the git admin directory byte-identical. A
// checkout that never declared anything has no token, and the key it derives matches no
// row — which is the honest answer, not a token minted to look like one.
func PlanIdentityReadAt(dir string, facts Facts) PlanIdentity {
	return planIdentityAt(dir, facts, false)
}

// planIdentityAt is the one derivation; mint is the only difference between the write
// path and every read path.
func planIdentityAt(dir string, facts Facts, mint bool) PlanIdentity {
	if facts == nil {
		facts = localFacts
	}
	common := commonDir(dir, facts)
	root := facts(dir, "rev-parse", "--show-toplevel")
	if root == "" {
		root = dir
	}
	gitDir, name := worktreeAdmin(dir, facts, common)
	return PlanIdentity{
		CommonDir: common,
		Worktree:  name,
		Created:   createdToken(gitDir, mint),
		Root:      realPath(root),
		Branch:    facts(dir, "symbolic-ref", "--quiet", "--short", "HEAD"),
	}
}

// worktreeAdmin is git's own name for this checkout and the admin directory it belongs
// to: <common>/worktrees/<name> for a linked worktree, <common> itself for the checkout
// that owns it.
func worktreeAdmin(dir string, facts Facts, common string) (gitDir, name string) {
	gitDir = facts(dir, "rev-parse", "--absolute-git-dir")
	if gitDir == "" {
		gitDir = facts(dir, "rev-parse", "--git-dir")
	}
	if gitDir == "" {
		return "", ""
	}
	gitDir = realPath(gitDir)
	if common != "" && gitDir == common {
		return gitDir, mainWorktree
	}
	return gitDir, "worktrees/" + filepath.Base(gitDir)
}

// createdToken is the creation token of one admin directory: read when it exists, and
// written once when it does not and mint is set. A read never mints, so it answers ""
// for a checkout that never declared anything. A token that cannot be written is no
// identity, so the value is left empty and a declaration fails closed rather than
// borrowing the name of a worktree that no longer exists.
func createdToken(adminDir string, mint bool) string {
	if adminDir == "" {
		return ""
	}
	path := filepath.Join(adminDir, createdTokenFile)
	if token := readToken(path); token != "" {
		return token
	}
	if !mint {
		return ""
	}
	token := newCreatedToken()
	if token == "" {
		return ""
	}
	// O_EXCL: a concurrent invocation that wins the race keeps its token, and both
	// processes then read the same one.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		_, writeErr := file.WriteString(token + "\n")
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return ""
		}
	}
	return readToken(path)
}

// readToken reads a token file, reporting nothing when it is absent or unusable.
func readToken(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// newCreatedToken is 16 random bytes, hex encoded. Randomness is what makes a recreated
// worktree unmistakably different from the one that was removed.
func newCreatedToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

// PlanSection is the plan section: one row per work, in stored order.
type PlanSection struct {
	Items []PlanItem `json:"items"`
}

// PlanItem is one work's plan decisions.
type PlanItem struct {
	Identity  PlanIdentity   `json:"identity"`
	Decisions []PlanDecision `json:"decisions"`
}

// PlanDecision is one plan-flow entry. `kind` names the decision; a workstream
// declaration carries the reason a human gave, the other kinds their choice and note.
type PlanDecision struct {
	At         string `json:"at"`
	Kind       string `json:"kind"`
	Checkpoint string `json:"checkpoint,omitempty"`
	Choice     string `json:"choice,omitempty"`
	Note       string `json:"note,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Workstream is the standing declaration for key: the last workstream or
// workstream-end decision wins, and a kind this build does not know is skipped. A
// missing row is no declaration.
func (s PlanSection) Workstream(key string) (declared bool, reason string) {
	if key == "" {
		return false, ""
	}
	for _, item := range s.Items {
		if item.Identity.Key() != key {
			continue
		}
		for _, decision := range item.Decisions {
			switch decision.Kind {
			case PlanKindWorkstream:
				declared, reason = true, decision.Reason
			case PlanKindWorkstreamEnd:
				declared, reason = false, ""
			}
		}
	}
	return declared, reason
}

// row returns the index of key's row and whether it exists.
func (s PlanSection) row(key string) (int, bool) {
	for i, item := range s.Items {
		if item.Identity.Key() == key {
			return i, true
		}
	}
	return -1, false
}

// appendDecision appends one decision to key's row, creating the row on first use.
func (s *PlanSection) appendDecision(ident PlanIdentity, decision PlanDecision) {
	if at, ok := s.row(ident.Key()); ok {
		s.Items[at].Decisions = append(s.Items[at].Decisions, decision)
		return
	}
	s.Items = append(s.Items, PlanItem{Identity: ident, Decisions: []PlanDecision{decision}})
}

// RecordPlanDecision appends one plan-flow decision to the plan section under the
// ledger lock and writes the file once. Every failure persists nothing: a checkout
// git cannot name, a corrupt ledger and a predecessor that cannot be backed up all
// fail closed, so the verb that records a declaration can exit non-zero instead of
// pretending it was recorded.
func RecordPlanDecision(loc StoreLocation, ident PlanIdentity, decision PlanDecision) (StoreReport, error) {
	if ident.CommonDir == "" || ident.Worktree == "" || ident.Created == "" {
		return StoreReport{}, fmt.Errorf("%w: the checkout cannot be named durably", ErrInvalidWrite)
	}
	if decision.At == "" || decision.Kind == "" {
		return StoreReport{}, fmt.Errorf("%w: a plan decision needs an at and a kind", ErrInvalidWrite)
	}
	var report StoreReport
	err := withStoreLock(loc.Path, func() error {
		file, loadReport, err := loc.Load()
		report = loadReport
		if err != nil {
			return err
		}
		file.Plan.appendDecision(ident, decision)
		saveReport, err := loc.Save(file, SectionPlan)
		report.BackupFailed = saveReport.BackupFailed
		return err
	})
	return report, err
}

// PlanSource is a checkout's plan-ledger predecessor: the append-only line file one
// build wrote before this section existed, plus the identity of the checkout that owns
// it, so its events seed exactly that row and no sibling inherits them.
type PlanSource struct {
	Path     string
	Identity PlanIdentity
}

// WithPlanSource attaches the checkout's plan-ledger predecessor: the file whose
// events seed the plan section of this checkout on load.
func (l StoreLocation) WithPlanSource(path string, ident PlanIdentity) StoreLocation {
	l.Plan = PlanSource{Path: path, Identity: ident}
	return l
}

// absorbPlanSource seeds the plan section from the checkout's JSONL predecessor: one
// PlanDecision per event, in file order, into the row of the checkout that owns the
// file. It only reads: the migration of a row inherited from an earlier key is
// adoptPlanRow's job, one step earlier in Load. The row answers when it already holds
// decisions, and is filled when it exists but is empty, so no event is lost and no
// superseded decision comes back. An absent predecessor is ordinary absence; a source
// that exists but cannot be read, or that is not a regular file, is reported and the
// ledger still answers.
func (l StoreLocation) absorbPlanSource(file *LedgerFile, report *StoreReport) {
	ident := l.Plan.Identity
	if l.Plan.Path == "" || ident.CommonDir == "" || ident.Worktree == "" {
		return
	}
	// A row that already holds decisions is the answer; only a missing or empty one
	// takes the predecessor's events. The predecessor is still inspected below, so an
	// unreadable or damaged one is reported either way.
	at, hasRow := file.Plan.row(ident.Key())
	info, err := os.Stat(l.Plan.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	case !info.Mode().IsRegular():
		// Reported without being read: a directory or a FIFO holds no events, and
		// reading one could block the whole invocation.
		report.LegacyUnreadable = fmt.Errorf("%s is not a regular file", l.Plan.Path)
		report.LegacyRows = l.Plan.Path
		return
	}
	data, err := os.ReadFile(l.Plan.Path)
	if err != nil {
		report.LegacyUnreadable = err
		report.LegacyRows = l.Plan.Path
		return
	}
	seeded, unreadable := decodePlanEvents(data)
	if unreadable > 0 {
		// The read stays lenient — an interrupted run must not block the ledger — but
		// a line nobody could read is said out loud instead of swallowed.
		report.LegacyUnreadable = fmt.Errorf("%s: %d line(s) could not be read", l.Plan.Path, unreadable)
		report.LegacyRows = l.Plan.Path
	}
	if hasRow && len(file.Plan.Items[at].Decisions) > 0 {
		return
	}
	if len(seeded) == 0 {
		return
	}
	if hasRow {
		file.Plan.Items[at].Decisions = append(file.Plan.Items[at].Decisions, seeded...)
	} else {
		file.Plan.Items = append(file.Plan.Items, PlanItem{Identity: ident, Decisions: seeded})
	}
	report.Absorbed += len(seeded)
	if report.LegacyRows == "" {
		report.LegacyRows = l.Plan.Path
	}
}

// adoptPlanRow migrates the row this checkout owns and refreshes the data it carries.
// It is a step of Load, before the predecessor is absorbed, and it never guesses:
//   - the row whose stable worktree name matches is this checkout's, so its recorded
//     root and branch are refreshed — that is what makes a branch rename and a folder
//     move keep the declaration without any path heuristic;
//   - a row written before the key became stable carries no worktree name, and is
//     rekeyed to this checkout only when it is the SINGLE candidate by common dir and
//     branch. Two candidates adopt nothing, and a row of a deleted worktree keeps its
//     name, so a checkout that later takes that branch starts clean instead of
//     inheriting a stranger's end;
//   - every other row is left alone.
func adoptPlanRow(file *LedgerFile, ident PlanIdentity) {
	if ident.CommonDir == "" || ident.Worktree == "" || ident.Created == "" {
		return
	}
	for i := range file.Plan.Items {
		row := &file.Plan.Items[i]
		if row.Identity.CommonDir == ident.CommonDir && row.Identity.Worktree == ident.Worktree &&
			row.Identity.Created == ident.Created {
			row.Identity.Root = ident.Root
			row.Identity.Branch = ident.Branch
			return
		}
	}
	// A row written before the key carried a worktree name has no identity of its own. It
	// is migrated only when it names this very folder and is the single candidate, so a
	// guess can never move one worktree's declaration into another. A row that carries a
	// name but no creation token is left alone: the build that wrote it is gone, and
	// borrowing its declaration is exactly what a recreated worktree must not do.
	candidate := -1
	for i, row := range file.Plan.Items {
		if row.Identity.CommonDir != ident.CommonDir || row.Identity.Worktree != "" {
			continue
		}
		if row.Identity.Root != ident.Root {
			continue
		}
		if candidate >= 0 {
			return
		}
		candidate = i
	}
	if candidate >= 0 {
		file.Plan.Items[candidate].Identity = ident
	}
}

// decodePlanEvents reads the predecessor's lines. A line that does not parse, or that
// names no event, is not an event: the count of those is returned so the caller can
// report it. Everything else is kept in file order, including an event without a
// timestamp, which the predecessor's own reader always honoured.
func decodePlanEvents(data []byte) (seeded []PlanDecision, unreadable int) {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event planEvent
		if json.Unmarshal([]byte(line), &event) != nil || event.Event == "" {
			unreadable++
			continue
		}
		seeded = append(seeded, PlanDecision{At: event.At, Kind: event.Event, Reason: event.Reason})
	}
	return seeded, unreadable
}

// planEvent is one line of the JSONL predecessor.
type planEvent struct {
	Event  string `json:"event"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}
