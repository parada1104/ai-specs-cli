package cli

// `ai-specs changes workstream [--reason <text>] [--end] [--json]`
// (change-ledger-v2 T3, S16): the report, the append-only declaration, and the
// refusals. The verb reads the same collector the write gate reads, so the two can
// never disagree about what a worktree is.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// workstreamRepo is a real odd-mode repository on feat/thing with one committed
// baseline, so the diff against the integration branch is a fact the verb reads.
func workstreamRepo(t *testing.T) string {
	t.Helper()
	root := changesOddRepo(t)
	for _, pair := range [][2]string{{"user.email", "workstream@example.invalid"}, {"user.name", "workstream"}} {
		workstreamGit(t, root, "config", pair[0], pair[1])
	}
	changesWrite(t, filepath.Join(root, "README.md"), "# base\n")
	workstreamGit(t, root, "add", "-A")
	workstreamGit(t, root, "commit", "-q", "-m", "base")
	workstreamGit(t, root, "checkout", "-q", "-b", "feat/thing")
	return root
}

func workstreamGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func workstreamCommit(t *testing.T, root, rel, message string) {
	t.Helper()
	changesWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "package src\n")
	workstreamGit(t, root, "add", "-A")
	workstreamGit(t, root, "commit", "-q", "-m", message)
}

type workstreamJSON struct {
	Classification string   `json:"classification"`
	Mode           string   `json:"mode"`
	Branch         string   `json:"branch"`
	Change         string   `json:"change"`
	Plan           string   `json:"plan"`
	Delta          bool     `json:"delta"`
	Declared       bool     `json:"declared"`
	Reason         string   `json:"reason"`
	Divergence     []string `json:"divergence"`
	Ledger         string   `json:"ledger"`
}

func workstreamReport(t *testing.T, root string) workstreamJSON {
	t.Helper()
	code, stdout, stderr := runChangesCLI(t, "changes", "workstream", "--root", root, "--json")
	if code != 0 {
		t.Fatalf("changes workstream --json = (%d, %q, %q), want a report", code, stdout, stderr)
	}
	var report workstreamJSON
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("changes workstream --json printed %q: %v", stdout, err)
	}
	return report
}

// A fresh worktree owes nothing: the verb reports workstream, exit 0, and the
// report names the exit that turns it into a change.
func TestChangesWorkstreamReportsAFreshWorktree(t *testing.T) {
	root := workstreamRepo(t)
	code, stdout, stderr := runChangesCLI(t, "changes", "workstream", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("changes workstream = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"workstream", "owes", "make it a change"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}

	report := workstreamReport(t, root)
	if report.Classification != "workstream" || report.Change != "thing" || report.Branch != "feat/thing" {
		t.Fatalf("report = %+v, want a workstream paired to thing", report)
	}
	if report.Declared || len(report.Divergence) != 0 || report.Plan != "" || report.Delta {
		t.Fatalf("report = %+v, want no declaration, no divergence and no artifact", report)
	}
	if !strings.HasSuffix(report.Ledger, filepath.Join("ai-specs", "ledger", "state.json")) {
		t.Fatalf("Ledger = %q, want the ledger file the plan section lives in", report.Ledger)
	}
}

// The gap T2's private copy left: a product commit makes it a change, and so does
// a product file the branch never added.
func TestChangesWorkstreamReportsAChange(t *testing.T) {
	root := workstreamRepo(t)
	workstreamCommit(t, root, "src/app.go", "feat: product work")
	if report := workstreamReport(t, root); report.Classification != "change" ||
		len(report.Divergence) != 1 || report.Divergence[0] != "src/app.go" {
		t.Fatalf("report = %+v, want a change diverging on src/app.go", report)
	}

	untracked := workstreamRepo(t)
	changesWrite(t, filepath.Join(untracked, "src", "new.go"), "package src\n")
	report := workstreamReport(t, untracked)
	if report.Classification != "change" {
		t.Fatalf("report = %+v, want a change: an untracked product path is divergence", report)
	}
}

// The declaration is append-only, it outranks product divergence, and `--end`
// returns the worktree to the facts.
func TestChangesWorkstreamDeclaresAndEnds(t *testing.T) {
	root := workstreamRepo(t)
	ledgerFile := filepath.Join(root, ".git", "ai-specs", "ledger", "state.json")

	code, stdout, stderr := runChangesCLI(t, "changes", "workstream", "--root", root,
		"--reason", "audit of the ledger")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "declared") {
		t.Fatalf("declare = (%d, %q, %q), want the declaration recorded", code, stdout, stderr)
	}
	workstreamCommit(t, root, "src/app.go", "feat: product work")
	report := workstreamReport(t, root)
	if report.Classification != "workstream" || !report.Declared || report.Reason != "audit of the ledger" {
		t.Fatalf("report = %+v, want the declared workstream to stay silent", report)
	}

	if code, stdout, stderr := runChangesCLI(t, "changes", "workstream", "--root", root, "--end"); code != 0 ||
		stderr != "" || !strings.Contains(stdout, "ended") {
		t.Fatalf("end = (%d, %q, %q), want the declaration ended", code, stdout, stderr)
	}
	if report := workstreamReport(t, root); report.Classification != "change" || report.Declared {
		t.Fatalf("report = %+v, want a change once the declaration ended", report)
	}

	raw, err := os.ReadFile(ledgerFile)
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Plan struct {
			Items []struct {
				Decisions []struct {
					Kind string `json:"kind"`
				} `json:"decisions"`
			} `json:"items"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("plan section is not readable: %v (%s)", err, raw)
	}
	if len(stored.Plan.Items) != 1 || len(stored.Plan.Items[0].Decisions) != 2 {
		t.Fatalf("plan section = %s, want one row with the two decisions in order", raw)
	}
	kinds := []string{stored.Plan.Items[0].Decisions[0].Kind, stored.Plan.Items[0].Decisions[1].Kind}
	if kinds[0] != "workstream" || kinds[1] != "workstream-end" {
		t.Fatalf("plan decisions = %v, want workstream then workstream-end", kinds)
	}
	// The line file is a predecessor now: a declaration never creates it again.
	if _, err := os.Stat(filepath.Join(root, ".ai-specs", "plan-ledger.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("a declaration wrote the retired line file (stat err = %v)", err)
	}

	// Nothing is open any more, so a second `--end` names the fact instead of
	// appending a third event.
	code, _, stderr = runChangesCLI(t, "changes", "workstream", "--root", root, "--end")
	if code != 1 || !strings.Contains(stderr, "no workstream declaration is open") {
		t.Fatalf("second end = (%d, %q), want the refusal", code, stderr)
	}
	if raw, _ := os.ReadFile(ledgerFile); strings.Count(string(raw), `"kind"`) != 2 {
		t.Fatalf("ledger = %q, want no third decision", string(raw))
	}
}

// A worktree that carries a plan or a delta is a change, so the declaration is
// refused and the refusal names the rule.
func TestChangesWorkstreamRefusesToDeclareAChange(t *testing.T) {
	root := workstreamRepo(t)
	changesWrite(t, filepath.Join(root, "odd", "tasks", "thing.md"), "## Specs\n\n- **S1.** Rule\n")

	code, _, stderr := runChangesCLI(t, "changes", "workstream", "--root", root, "--reason", "audit")
	if code != 1 {
		t.Fatalf("declare with a plan = %d, want the refusal", code)
	}
	for _, want := range []string{"refusing", "odd/tasks/thing.md", "S16"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("refusal %q does not contain %q", stderr, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-specs", "plan-ledger.jsonl")); err == nil {
		t.Fatal("a refused declaration must record nothing")
	}
}

// The argument contract: both decisions at once, an empty reason and an unknown
// flag are usage errors, and `--help` prints the verb's usage.
func TestChangesWorkstreamArgumentContract(t *testing.T) {
	root := workstreamRepo(t)
	for _, args := range [][]string{
		{"changes", "workstream", "--root", root, "--reason", "audit", "--end"},
		{"changes", "workstream", "--root", root, "--reason", ""},
		// An option is never a value: `--reason --end` must not record a declaration
		// out of a misplaced separator, and `--reason --json` must not record the
		// option name as the reason.
		{"changes", "workstream", "--root", root, "--reason", "--end"},
		{"changes", "workstream", "--root", root, "--reason", "--json"},
		{"changes", "workstream", "--root", "--json"},
		{"changes", "workstream", "--root", root, "--nope"},
		{"changes", "workstream"},
	} {
		if code, _, _ := runChangesCLI(t, args...); code != 2 {
			t.Errorf("%v = %d, want the usage exit", args, code)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-specs", "plan-ledger.jsonl")); err == nil {
		t.Fatal("a refused invocation must record nothing")
	}
	code, stdout, stderr := runChangesCLI(t, "changes", "workstream", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "changes workstream") {
		t.Fatalf("--help = (%d, %q, %q), want the usage on stdout", code, stdout, stderr)
	}
}
