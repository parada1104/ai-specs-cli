package planledger

// The plan ledger (change-ledger-v2 T3, L3): one append-only JSONL file at the
// repository root, mode-agnostic, one JSON object per line. T3 records only the
// workstream decision — the event the discriminator has to read — and later units
// append their own events to the same file without touching these.

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Path is the ledger's repository-relative location. One file per checkout, so a
// worktree carries its own declaration and its siblings cannot inherit it.
const Path = ".ai-specs/plan-ledger.jsonl"

// The two events this unit writes. Later units add theirs; a reader that meets an
// unknown event skips it.
const (
	EventWorkstream    = "workstream"
	EventWorkstreamEnd = "workstream-end"
)

// Event is one appended fact. `event` and `at` are the ledger's own fields; the
// rest names the decision.
type Event struct {
	Event  string `json:"event"`
	At     string `json:"at"`
	Reason string `json:"reason,omitempty"`
}

// File is the absolute ledger path of a repository root.
func File(root string) string { return filepath.Join(root, filepath.FromSlash(Path)) }

// Append writes one event as exactly one line, creating the directory on first
// use. One Write of a whole line is what keeps an interrupted run from leaving a
// half event behind, and nothing is ever rewritten: the file only grows.
func Append(root string, event Event) error {
	if event.Event == "" {
		return errors.New("plan ledger: the event name is empty")
	}
	if event.At == "" {
		return errors.New("plan ledger: the event time is empty")
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := File(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// Workstream is the standing workstream decision: the last workstream or
// workstream-end event wins, and a line the ledger cannot parse is skipped rather
// than trusted. A missing ledger is no declaration.
func Workstream(root string) (declared bool, reason string) {
	file, err := os.Open(File(root))
	if err != nil {
		return false, ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		switch event.Event {
		case EventWorkstream:
			declared, reason = true, event.Reason
		case EventWorkstreamEnd:
			declared, reason = false, ""
		}
	}
	return declared, reason
}
