package gates

// The workstream fact (change-ledger-v2 T3): the two divergences T2's private
// copy could not see, and the explicit append-only declaration S16 offers for the
// worktree the facts cannot separate.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

func workstreamWrite(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// L4: `git diff` against the merge base reads the working tree, so an uncommitted
// edit to a tracked file already showed up; a file the branch never added is the
// divergence it cannot see. A worktree that added one is a change, and an
// untracked file outside the production scope still is not.
func TestWriteFactsSeesUncommittedProductDivergence(t *testing.T) {
	root := writeRepo(t, nil)
	workstreamWrite(t, root, "src/new.go", "package src\n")
	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "docs/deploy.md")
	if facts.Workstream {
		t.Fatalf("Facts = %+v, want a change: an untracked product path is divergence", facts)
	}

	quiet := writeRepo(t, nil)
	workstreamWrite(t, quiet, "docs/notes.md", "# notes\n")
	if facts := WriteFactsFor(quiet, planbuild.ModeOdd, "thing", "docs/notes.md"); !facts.Workstream {
		t.Fatalf("Facts = %+v, want a workstream: docs is not a product path", facts)
	}
}

// S16: the declaration is the explicit decision for a worktree whose product work
// is deliberately not a change, it is read from the plan ledger, and `--end`
// returns the worktree to the facts. An artifact always outranks it, so no
// declaration can hide a change.
func TestWriteFactsHonoursTheWorkstreamDeclaration(t *testing.T) {
	root := writeRepo(t, nil)
	commitPath(t, root, "src/app.go", "feat: product work")
	commitPath(t, root, "odd/specs/cap/spec.md", "spec: promote the rule")
	if verdict := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go").Verdict(); verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want R1 before any declaration", verdict)
	}

	ledger := workstreamWrite(t, root, ".ai-specs/plan-ledger.jsonl",
		`{"event":"workstream","at":"2026-01-01T00:00:00Z","reason":"audit of the ledger"}`+"\n")
	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go")
	if !facts.Workstream {
		t.Fatalf("Facts = %+v, want the declared workstream", facts)
	}
	if !facts.Declared || facts.DeclaredReason != "audit of the ledger" {
		t.Fatalf("Facts = %+v, want the declaration and its reason", facts)
	}
	if verdict := facts.Verdict(); !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want no change refutation for a declared workstream", verdict)
	}

	// A plan is a change: the declaration never hides it.
	workstreamWrite(t, root, "odd/tasks/thing.md", "## Specs\n\n- **S1.** Rule\n")
	if facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go"); facts.Workstream {
		t.Fatalf("Facts = %+v, want a change: the plan outranks the declaration", facts)
	}
	if err := os.Remove(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Fatal(err)
	}

	// `--end` closes it: the worktree returns to the facts, which say change.
	appendLine(t, ledger, `{"event":"workstream-end","at":"2026-01-01T00:05:00Z"}`)
	facts = WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go")
	if facts.Workstream || facts.Declared {
		t.Fatalf("Facts = %+v, want the ended declaration ignored", facts)
	}
	if got, want := facts.Verdict().Rules(), "R1"; got != want {
		t.Fatalf("Rules() = %q, want %q once the workstream ended", got, want)
	}
}

// appendLine appends one raw line, so the test writes the ledger bytes instead of
// trusting the writer it is not testing.
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

// A ledger nobody can read is a fact this collection could not observe, and the
// verdict says so: the declaration lives in a file every worktree shares, so a corrupt
// one must never look like "this worktree is a change" without naming the cause.
func TestWriteFactsNamesAnUnreadablePlanLedger(t *testing.T) {
	root := writeRepo(t, nil)
	commitPath(t, root, "src/app.go", "feat: product work")
	ledger := filepath.Join(root, ".git", "ai-specs", "ledger", "state.json")
	if err := os.MkdirAll(filepath.Dir(ledger), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledger, []byte("{ corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go")
	if facts.DeclaredErr == nil {
		t.Fatalf("Facts = %+v, want the read error reported", facts)
	}
	if facts.Declared {
		t.Fatalf("Facts = %+v, want no declaration invented out of an unreadable ledger", facts)
	}
	verdict := facts.Verdict()
	found := false
	for _, note := range verdict.Notes {
		if strings.Contains(note, "plan ledger could not be loaded") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Notes = %v, want one naming the ledger", verdict.Notes)
	}
}
