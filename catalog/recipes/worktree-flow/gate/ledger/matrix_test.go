package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Invariant M over the one-file design (L13 + L14): whatever the layout and the
// entry point, after a write the worktree section represents every status and every
// close decision the predecessor sources held, the sources are never rewritten, and
// a write-free cell stays write-free.

const matrixStamp = "2026-09-13T12:00:00Z"

func matrixClosed(id, common, branch string) LegacyItem {
	return LegacyItem{
		Status: StatusClosed,
		Item: Item{
			ID: id, Identity: ItemIdentity{CommonDir: common, Branch: branch},
			ItemID: "card-" + id, ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{
				{At: matrixStamp, Kind: DecisionOpen},
				{At: matrixStamp, Checkpoint: CheckpointArchiveClose, Kind: DecisionClose},
			},
		},
	}
}

func matrixLive(ident ItemIdentity) LegacyItem {
	return LegacyItem{
		Status: StatusOpen,
		Item: Item{
			ID: "live", Identity: ident, ItemID: "card-1", ProviderID: "trello-mcp-workflow",
			Decisions: []Decision{{At: matrixStamp, Kind: DecisionOpen}},
		},
	}
}

func matrixHistory(ident ItemIdentity) LegacyStore {
	return LegacyStore{V: 1, Items: []LegacyItem{
		matrixClosed("hist-a", ident.CommonDir, "old-a"),
		matrixClosed("hist-b", ident.CommonDir, "old-b"),
		matrixLive(ident),
	}}
}

func writeLegacyFixture(t *testing.T, path string, store LegacyStore) []byte {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := writeAtomic(path, data); err != nil {
		t.Fatal(err)
	}
	return data
}

// matrixCell is one layout seen through one entry point.
type matrixCell struct {
	ledgerPath string
	t5Path     string
	loc        StoreLocation
}

// matrixLayouts are the predecessor layouts a repository can be found in.
func matrixLayouts() []struct {
	name string
	seed func(t *testing.T, cell matrixCell, ident ItemIdentity)
} {
	return []struct {
		name string
		seed func(t *testing.T, cell matrixCell, ident ItemIdentity)
	}{
		{"v1-only", func(t *testing.T, cell matrixCell, ident ItemIdentity) {
			writeLegacyFixture(t, cell.ledgerPath, matrixHistory(ident))
		}},
		{"t5-only", func(t *testing.T, cell matrixCell, ident ItemIdentity) {
			if cell.t5Path == "" {
				t.Skip("this entry point has no T5 source")
			}
			writeLegacyFixture(t, cell.t5Path, matrixHistory(ident))
		}},
		{"v1+t5", func(t *testing.T, cell matrixCell, ident ItemIdentity) {
			writeLegacyFixture(t, cell.ledgerPath, matrixHistory(ident))
			if cell.t5Path == "" {
				return
			}
			writeLegacyFixture(t, cell.t5Path, LegacyStore{V: 1, Items: []LegacyItem{
				matrixClosed("t5-only-row", ident.CommonDir, "t5-old"),
			}})
		}},
		{"v2", func(t *testing.T, cell matrixCell, ident ItemIdentity) {
			file := legacyAsLedger(matrixHistory(ident))
			if _, err := cell.loc.Save(file); err != nil {
				t.Fatal(err)
			}
		}},
	}
}

// matrixEntries are the entry points: the repository-derived location and two
// explicit --store names. --store names THE file, so it has no migration source.
func matrixEntries() []struct {
	name string
	cell func(dir string) matrixCell
} {
	return []struct {
		name string
		cell func(dir string) matrixCell
	}{
		{"repository-derived", func(dir string) matrixCell {
			return matrixCell{ledgerPath: StorePath(dir), t5Path: TrackerStorePath(dir), loc: LedgerAt(dir)}
		}},
		{"--store state.json", func(dir string) matrixCell {
			path := filepath.Join(dir, storeFileName)
			return matrixCell{ledgerPath: path, loc: StoreAt(path)}
		}},
		{"--store custom.json", func(dir string) matrixCell {
			path := filepath.Join(dir, "custom.json")
			return matrixCell{ledgerPath: path, loc: StoreAt(path)}
		}},
	}
}

// matrixOps are the operations, driven through the same mutation path the CLI uses.
func matrixOps() []struct {
	name          string
	firstNoWrite  bool
	repeatNoWrite bool
	act           func(loc StoreLocation, ident ItemIdentity, now time.Time) error
} {
	bind := func(loc StoreLocation, ident ItemIdentity, now time.Time) error {
		out, err := ApplyWrite(loc, ApplyWriteRequest{
			Identity: ident, Checkpoint: CheckpointApplyStart,
			Request: WriteRequest{Kind: WriteBind, ItemID: "card-2"},
		}, now)
		if err != nil {
			return err
		}
		if !out.Applied && out.Reason != WriteReasonUnchanged {
			return fmt.Errorf("bind = %+v, want applied or unchanged", out)
		}
		return nil
	}
	closeItem := func(loc StoreLocation, ident ItemIdentity, now time.Time) error {
		out, err := ApplyWrite(loc, ApplyWriteRequest{
			Identity: ident, Checkpoint: CheckpointArchiveClose, Request: WriteRequest{Kind: WriteClose},
		}, now)
		if err != nil {
			return err
		}
		if !out.Applied && out.Reason != WriteReasonAlreadyClosed {
			return fmt.Errorf("close = %+v, want applied or already-closed", out)
		}
		return nil
	}
	return []struct {
		name          string
		firstNoWrite  bool
		repeatNoWrite bool
		act           func(loc StoreLocation, ident ItemIdentity, now time.Time) error
	}{
		{"grade", true, true, func(loc StoreLocation, ident ItemIdentity, now time.Time) error {
			file, _, err := loc.Load()
			if err != nil {
				return err
			}
			view := file.View()
			got := Grade(Input{
				Checkpoint: CheckpointApplyStart, Mode: ModeWarn, Binding: boundBinding(),
				Identity: Identity{CommonDir: ident.CommonDir, Branch: ident.Branch, Key: ident.Key()},
				View:     view,
				Evidence: Evidence{Local: "card-1", Remote: "card-1"},
				Now:      now,
			})
			if got.Decision != DecisionAllow || got.Reason != "" {
				return fmt.Errorf("grade = %q/%q, want allow", got.Decision, got.Reason)
			}
			return nil
		}},
		{"open", false, true, func(loc StoreLocation, ident ItemIdentity, now time.Time) error {
			out, err := ApplyWrite(loc, ApplyWriteRequest{
				Identity: ident, Checkpoint: CheckpointApplyStart, Request: WriteRequest{Kind: WriteOpen},
			}, now)
			if err != nil {
				return err
			}
			if out.Reason != WriteReasonAlreadyOpen {
				return fmt.Errorf("open = %+v, want already-open", out)
			}
			return nil
		}},
		{"bind", false, true, bind},
		{"close", false, true, closeItem},
		{"bind-then-close", false, false, func(loc StoreLocation, ident ItemIdentity, now time.Time) error {
			if err := bind(loc, ident, now); err != nil {
				return err
			}
			return closeItem(loc, ident, now.Add(time.Minute))
		}},
	}
}

// fileBytes is the exact bytes of a file, or a marker for its absence.
func fileBytes(t *testing.T, path string) string {
	t.Helper()
	if path == "" {
		return "<none>"
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// lifecycleOf is what a v1 store says about each row's lifecycle.
type lifecycleOf struct {
	status string
	closes []string
}

func sourceLifecycle(t *testing.T, path string) map[string]lifecycleOf {
	t.Helper()
	out := map[string]lifecycleOf{}
	if path == "" || fileBytes(t, path) == "<absent>" {
		return out
	}
	data := []byte(fileBytes(t, path))
	if !isV1(data) {
		// Already v2: its own worktree section is the lifecycle it holds.
		var file LedgerFile
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("read source %s: %v", path, err)
		}
		for _, row := range file.Worktree.Items {
			entry := out[row.ID]
			entry.status = row.Status
			for _, d := range row.Decisions {
				if d.Kind == DecisionClose {
					entry.closes = append(entry.closes, d.At+"|"+d.Checkpoint)
				}
			}
			out[row.ID] = entry
		}
		return out
	}
	legacy, err := decodeLegacy(data, path)
	if err != nil {
		t.Fatalf("read source %s: %v", path, err)
	}
	for _, row := range legacy.Items {
		entry := out[row.ID]
		if row.Status != "" {
			entry.status = row.Status
		}
		for _, d := range row.Decisions {
			if d.Kind == DecisionClose {
				entry.closes = append(entry.closes, d.At+"|"+d.Checkpoint)
			}
		}
		out[row.ID] = entry
	}
	return out
}

// assertRepresentsSources checks that the ledger file's worktree section holds every
// status and close the pre-operation sources carried.
func assertRepresentsSources(t *testing.T, loc StoreLocation, sources map[string]lifecycleOf) {
	t.Helper()
	file := loadFile(t, loc)
	rows := map[string]WorktreeItem{}
	for _, row := range file.Worktree.Items {
		rows[row.ID] = row
	}
	for id, want := range sources {
		if want.status == "" && len(want.closes) == 0 {
			continue
		}
		row, ok := rows[id]
		if !ok {
			t.Fatalf("row %q was in a source and is not in the worktree section", id)
		}
		if want.status == StatusClosed && row.Status != StatusClosed {
			t.Fatalf("row %q was closed in a source and is %q in the ledger", id, row.Status)
		}
		for _, close := range want.closes {
			found := false
			for _, d := range row.Decisions {
				if d.Kind == DecisionClose && d.At+"|"+d.Checkpoint == close {
					found = true
				}
			}
			if !found {
				t.Fatalf("close %q of row %q was in a source and is not in the ledger", close, id)
			}
		}
	}
}

// TestInvariantMOverEveryLayoutEntryAndOperation is L13's deliverable on the
// one-file design: every predecessor layout, every entry point and every operation.
func TestInvariantMOverEveryLayoutEntryAndOperation(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 30, 0, 0, time.UTC)
	for _, layout := range matrixLayouts() {
		for _, entry := range matrixEntries() {
			for _, op := range matrixOps() {
				layout, entry, op := layout, entry, op
				t.Run(layout.name+"/"+entry.name+"/"+op.name, func(t *testing.T) {
					dir := t.TempDir()
					ident := ItemIdentity{CommonDir: dir, Branch: "main"}
					cell := entry.cell(dir)
					layout.seed(t, cell, ident)

					before := fileBytes(t, cell.ledgerPath)
					t5Before := fileBytes(t, cell.t5Path)
					sources := sourceLifecycle(t, cell.ledgerPath)
					for id, want := range sourceLifecycle(t, cell.t5Path) {
						if _, ok := sources[id]; !ok {
							sources[id] = want
						}
					}

					if err := op.act(cell.loc, ident, now); err != nil {
						t.Fatalf("%s refused: %v", op.name, err)
					}
					after := fileBytes(t, cell.ledgerPath)
					if op.firstNoWrite && after != before {
						t.Fatalf("a %s wrote to the ledger", op.name)
					}
					if after != before {
						// The cell wrote: everything the sources held must be represented.
						assertRepresentsSources(t, cell.loc, sources)
					}
					if got := fileBytes(t, cell.t5Path); got != t5Before {
						t.Fatalf("the T5 predecessor was rewritten")
					}
					// Only a v1 predecessor is backed up; a v2 file is not a source.
					if after != before && isV1([]byte(before)) {
						if backup := fileBytes(t, cell.ledgerPath+".v1-backup"); backup != before {
							t.Fatalf("the one-shot backup does not hold the predecessor bytes")
						}
					}
					if after != before && !isV1([]byte(before)) {
						if backup := fileBytes(t, cell.ledgerPath+".v1-backup"); backup != "<absent>" {
							t.Fatalf("a v2 ledger grew a predecessor backup")
						}
					}

					if op.repeatNoWrite {
						if err := op.act(cell.loc, ident, now.Add(2*time.Minute)); err != nil {
							t.Fatalf("the repeated %s refused: %v", op.name, err)
						}
						if fileBytes(t, cell.ledgerPath) != after {
							t.Fatalf("the repeated %s wrote to the ledger", op.name)
						}
					}
				})
			}
		}
	}
}
