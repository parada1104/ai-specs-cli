package cli

// Real-flow matrix for the workstream (change-ledger-v2 T3, S16): the built CLI
// over temporary repositories, so the exit codes, the bytes and the on-disk
// effects are the ones an agent sees.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestChangesWorkstreamFlow(t *testing.T) {
	bin, _ := smokeSetup(t)

	t.Run("a fresh worktree is a workstream and owes nothing", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		flowCommitManifest(t, repo)
		stdout, stderr, code := flowRun(t, bin, repo, "", "changes", "workstream", "--root", repo)
		if code != 0 || stderr != "" {
			t.Fatalf("workstream = (%d, %q, %q), want exit 0 and no refusal", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "workstream") || !strings.Contains(stdout, "make it a change") {
			t.Fatalf("report = %q, want a workstream naming its exit", stdout)
		}
		// No ledger, no item and no archive are demanded: the gate stays silent.
		if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"),
			"plan-build-gate", "--explain"); code != 0 ||
			stderr != "plan-build-gate: tier none; refutations none\n" {
			t.Fatalf("gate = (%d, %q), want the silent none tier", code, stderr)
		}
		if entries, err := filepath.Glob(filepath.Join(repo, ".ai-specs", "*")); err != nil || len(entries) != 0 {
			t.Fatalf("ledger = %v (%v), want no row written by a report", entries, err)
		}
	})

	t.Run("a product commit makes it a change with tier none", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		flowCommitManifest(t, repo)
		flowWrite(t, repo, "src/app.go", "package src\n")
		flowGit(t, repo, "add", "-A")
		flowGit(t, repo, "commit", "-q", "-m", "feat: product work")

		stdout, _, code := flowRun(t, bin, repo, "", "changes", "workstream", "--root", repo, "--json")
		if code != 0 || !strings.Contains(stdout, `"classification":"change"`) {
			t.Fatalf("report = (%d, %q), want a change", code, stdout)
		}
		if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"),
			"plan-build-gate", "--explain"); code != 0 ||
			stderr != "plan-build-gate: tier none; refutations none\n" {
			t.Fatalf("gate = (%d, %q), want the silent none tier", code, stderr)
		}
	})

	t.Run("a declared workstream stays silent until it ends", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		flowCommitManifest(t, repo)
		flowWrite(t, repo, "src/app.go", "package src\n")
		flowWrite(t, repo, "odd/specs/cap/spec.md", "## Purpose\n")
		flowGit(t, repo, "add", "-A")
		flowGit(t, repo, "commit", "-q", "-m", "feat: product work and a promoted spec")
		if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"),
			"plan-build-gate", "--explain"); code != 2 || !strings.Contains(stderr, "R1:") {
			t.Fatalf("gate before the declaration = (%d, %q), want R1", code, stderr)
		}

		if _, stderr, code := flowRun(t, bin, repo, "", "changes", "workstream",
			"--root", repo, "--reason", "audit of the ledger"); code != 0 {
			t.Fatalf("declare = (%d, %q), want it recorded", code, stderr)
		}
		if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"),
			"plan-build-gate", "--explain"); code != 0 ||
			stderr != "plan-build-gate: tier none; refutations none\n" {
			t.Fatalf("gate while declared = (%d, %q), want silence", code, stderr)
		}
		if stdout, _, code := flowRun(t, bin, repo, "", "changes", "workstream", "--root", repo, "--end"); code != 0 {
			t.Fatalf("end = (%d, %q), want it closed", code, stdout)
		}
		if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "src/app.go"),
			"plan-build-gate", "--explain"); code != 2 || !strings.Contains(stderr, "R1:") {
			t.Fatalf("gate after the end = (%d, %q), want R1 back", code, stderr)
		}
	})

	t.Run("a declaration on a worktree with a plan is refused", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		flowWrite(t, repo, "odd/tasks/thing.md", "## Specs\n\n- **S1.** Rule\n")
		_, stderr, code := flowRun(t, bin, repo, "", "changes", "workstream", "--root", repo, "--reason", "audit")
		if code != 1 || !strings.Contains(stderr, "odd/tasks/thing.md") || !strings.Contains(stderr, "S16") {
			t.Fatalf("declare = (%d, %q), want the refusal naming the artifact and the rule", code, stderr)
		}
	})

	t.Run("uncommitted product divergence makes it a change", func(t *testing.T) {
		repo := flowRepo(t, "odd", nil)
		flowWrite(t, repo, "src/new.go", "package src\n")
		stdout, _, code := flowRun(t, bin, repo, "", "changes", "workstream", "--root", repo, "--json")
		if code != 0 || !strings.Contains(stdout, `"classification":"change"`) ||
			!strings.Contains(stdout, `"src/new.go"`) {
			t.Fatalf("report = (%d, %q), want a change diverging on the untracked file", code, stdout)
		}
	})
}
