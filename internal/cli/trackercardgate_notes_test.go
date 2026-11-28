package cli

// T7.4 (S3, S14): the ledger's StoreReport is a note, never an error. The gate
// prints it before its own diagnostics, so the wrapper must relay it to the
// command's stderr and report the real error line, not the first stderr line.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/ledger"
)

// writeTrackerLedgerPredecessor writes the pre-split v1 tracker file with one
// row the ledger file does not have, so every read of the store reports it as
// an absorbed row. The row's identity belongs to another branch, so it is a
// note and never a row that satisfies the graded checkpoint.
func writeTrackerLedgerPredecessor(t *testing.T, common string) string {
	t.Helper()
	path := ledger.TrackerStorePath(common)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"v": 1,
		"items": []map[string]any{{
			"id":          "legacy-row",
			"identity":    map[string]string{"common_dir": common, "branch": "legacy-branch"},
			"item_id":     "card-legacy",
			"provider_id": "trello-mcp-workflow",
			"decisions":   []any{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A store note must never stand in for the error of a bind: with a non-empty
// store report AND a refused write, the note is relayed and the refusal is what
// the bind failure names.
func TestTrackerCardGateStoreNoteNeverMasksABindError(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	trackerWriteCard(t, repo, "demo", "## Tracker\n- **card_id**: `card-1`\n- url: `https://example.test/card/1`\n")

	predecessor := writeTrackerLedgerPredecessor(t, common)
	// A directory where the one-shot predecessor backup belongs: the backup cannot
	// be written, so the gate refuses the write after printing the store note.
	if err := os.MkdirAll(predecessor+".v1-backup", 0o755); err != nil {
		t.Fatal(err)
	}

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)

	want := "tracker-card-gate: bind failed at apply-start for demo " +
		"(worktree-gate: ledger write refused: the predecessor backup failed) " +
		"— needs-item; link or create the tracker item, or record the exemption (Tracker advisory).\n"
	if code != 0 || stdout != "" || !strings.Contains(stderr, want) {
		t.Fatalf("gate = (%d, %q, %q), want the bind failure naming the refused write (%q)", code, stdout, stderr, want)
	}
	note := "worktree-gate: 1 ledger row(s) from " + predecessor +
		" are part of this answer; the next write carries them into the ledger file"
	if !strings.Contains(stderr, note) {
		t.Fatalf("stderr = %q, want the store note relayed (%q)", stderr, note)
	}
}

// A clean store reports nothing, so the verb's own line stays the only one.
func TestTrackerCardGateCleanStorePrintsNoStoreNote(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	trackerWriteCard(t, repo, "demo", "## Tracker\n- **card_id**: `card-1`\n- url: `https://example.test/card/1`\n")

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := "tracker-card-gate: bound card card-1 at apply-start (ledger row recorded).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
}
