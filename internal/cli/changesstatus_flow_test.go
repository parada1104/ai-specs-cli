package cli

// Real-flow matrix for `changes status` (change-ledger-v2 T7, S3/S4/S12/S14/S16): the
// built CLI over a temporary repository, so the phase, the exit code, the bytes and the
// on-disk effects are the ones an agent sees.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// statusFlowFact is one decoded fact of the `--json` report.
type statusFlowFact struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Known  bool   `json:"known"`
	Reason string `json:"reason"`
}

// statusFlowFactNamed resolves the named fact of one decoded section.
func statusFlowFactNamed(facts []statusFlowFact, name string) *statusFlowFact {
	for i := range facts {
		if facts[i].Name == name {
			return &facts[i]
		}
	}
	return nil
}

// statusFlowRun is one step of the flow: the report and the exit the built binary gave.
func statusFlowRun(t *testing.T, bin, repo string, args ...string) (string, string, int) {
	t.Helper()
	return flowRun(t, bin, repo, "", append([]string{"changes", "status", "--root", repo}, args...)...)
}

// flowCommitManifest commits the manifest flowRepo leaves uncommitted, so the manifest
// is not itself the product divergence a test is about.
func flowCommitManifest(t *testing.T, repo string) {
	t.Helper()
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-q", "-m", "docs: the manifest")
	flowGit(t, repo, "checkout", "-q", "main")
	flowGit(t, repo, "merge", "-q", "--ff-only", "feat/thing")
	flowGit(t, repo, "checkout", "-q", "feat/thing")
}

// TestChangesStatusFlow walks one repository through every phase L5 item 1 names:
// workstream -> open -> planned -> merged -> cleaned, asserting the reported phase and the
// exit code at each step.
func TestChangesStatusFlow(t *testing.T) {
	bin, _ := smokeSetup(t)
	// The receipt is an environment fact and never decides a phase: a missing binary is
	// the honest answer here, and it keeps the flow hermetic.
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowCommitManifest(t, repo)

	t.Run("workstream", func(t *testing.T) {
		stdout, stderr, code := statusFlowRun(t, bin, repo)
		if code != 0 || stderr != "" {
			t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
		}
		for _, want := range []string{"phase:    workstream", "no plan", "no delta",
			"no product-path divergence", "when product paths change, this becomes a change",
			"review:   unknown ("} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report %q does not contain %q", stdout, want)
			}
		}
	})

	t.Run("open", func(t *testing.T) {
		flowWrite(t, repo, "src/app.go", "package src\n")
		flowGit(t, repo, "add", "-A")
		flowGit(t, repo, "commit", "-q", "-m", "feat: product work")

		stdout, stderr, code := statusFlowRun(t, bin, repo)
		if code != 0 || stderr != "" {
			t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
		}
		for _, want := range []string{"phase:    open", "product paths diverging: src/app.go",
			"plan:     none (no artifacts)"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report %q does not contain %q", stdout, want)
			}
		}
	})

	t.Run("planned", func(t *testing.T) {
		flowWrite(t, repo, "odd/tasks/thing.md", changesPlan)
		flowGit(t, repo, "add", "-A")
		flowGit(t, repo, "commit", "-q", "-m", "plan: thing")

		stdout, stderr, code := statusFlowRun(t, bin, repo)
		if code != 0 || stderr != "" {
			t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
		}
		for _, want := range []string{"phase:    planned", "odd/tasks/thing.md",
			"plan:     light (plan only), refutations: none"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report %q does not contain %q", stdout, want)
			}
		}

		// The --json shape is the contract a host reads: the phase, the next step and one
		// fact list per capability, each fact naming its source and whether it is known.
		stdout, _, code = statusFlowRun(t, bin, repo, "--json")
		if code != 0 {
			t.Fatalf("status --json = %d, want the phase's exit", code)
		}
		var report map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("status --json printed %q: %v", stdout, err)
		}
		var keys []string
		for key := range report {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		want := []string{"blockers", "next", "phase", "plan", "review", "tracker", "unknown", "why", "worktree"}
		if strings.Join(keys, ",") != strings.Join(want, ",") {
			t.Fatalf("--json keys = %v, want %v", keys, want)
		}
		var phase string
		if err := json.Unmarshal(report["phase"], &phase); err != nil || phase != "planned" {
			t.Fatalf("phase = %q (%v), want planned", phase, err)
		}
		var plan []statusFlowFact
		if err := json.Unmarshal(report["plan"], &plan); err != nil {
			t.Fatalf("plan section = %q: %v", report["plan"], err)
		}
		tier := statusFlowFactNamed(plan, "tier")
		if tier == nil || !tier.Known || tier.Value != "light" || tier.Source == "" {
			t.Fatalf("tier fact = %+v, want a known light tier naming its source", tier)
		}
		// Every reported fact carries a source: a fact nobody can trace is not evidence.
		for _, section := range []string{"worktree", "plan", "tracker", "review"} {
			var facts []statusFlowFact
			if err := json.Unmarshal(report[section], &facts); err != nil {
				t.Fatalf("%s section = %q: %v", section, report[section], err)
			}
			if len(facts) == 0 {
				t.Errorf("%s section is empty, want its facts", section)
			}
			for _, fact := range facts {
				if fact.Source == "" {
					t.Errorf("%s.%s has no source", section, fact.Name)
				}
				if !fact.Known && fact.Reason == "" {
					t.Errorf("%s.%s is unknown with no reason", section, fact.Name)
				}
			}
		}
	})

	t.Run("merged", func(t *testing.T) {
		flowGit(t, repo, "checkout", "-q", "main")
		flowGit(t, repo, "merge", "-q", "--no-ff", "-m", "merge feat/thing", "feat/thing")
		flowGit(t, repo, "checkout", "-q", "feat/thing")

		stdout, stderr, code := statusFlowRun(t, bin, repo)
		if code != 0 || stderr != "" {
			t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
		}
		for _, want := range []string{"phase:    merged", "the local merge proof holds",
			"next:     ai-specs worktree-cleanup"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report %q does not contain %q", stdout, want)
			}
		}
	})

	t.Run("cleaned", func(t *testing.T) {
		// The lifecycle row is opened and closed through the gate's own write vehicle: the
		// close is what a cleanup pass records, and it needs no worktree removal.
		gate := func(checkpoint, write string) {
			t.Helper()
			if _, stderr, code := flowRun(t, bin, repo, "", "worktree-gate", "--ledger",
				"--ledger-mode", "warn", "--project-root", repo, "--checkpoint", checkpoint,
				"--write", write); code != 0 {
				t.Fatalf("worktree-gate --write %s = (%d, %q), want it recorded", write, code, stderr)
			}
		}
		gate("work-start", `{"kind":"open"}`)
		if stdout, _, code := statusFlowRun(t, bin, repo); code != 0 ||
			!strings.Contains(stdout, "phase:    merged") {
			t.Fatalf("status with an open row = (%d, %q), want the merge proof to still decide", code, stdout)
		}
		gate("archive-close", `{"kind":"close"}`)

		stdout, stderr, code := statusFlowRun(t, bin, repo)
		if code != 0 || stderr != "" {
			t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
		}
		for _, want := range []string{"phase:    cleaned", "the lifecycle row for this branch is closed",
			"tracker:  closed ", "next:     nothing"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("report %q does not contain %q", stdout, want)
			}
		}
	})
}

// L8-1: one product predicate. A change that only touches internal/ is a change
// (open), not a workstream, and the report names the diverging path — the folder
// list that ignored internal/ and cmd/ is gone.
func TestChangesStatusFlowCountsInternalAsProduct(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowCommitManifest(t, repo)
	flowWrite(t, repo, "internal/gates/gates.go", "package gates\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-q", "-m", "feat: internal work")

	stdout, stderr, code := statusFlowRun(t, bin, repo)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"phase:    open", "product paths diverging: internal/gates/gates.go"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "phase:    workstream") {
		t.Errorf("report %q calls a change to internal/ a workstream", stdout)
	}
}

// L8-1: the project's own product_exclude globs narrow that same predicate for the
// report and the gate: an excluded tree is not product work and does not owe a change.
func TestChangesStatusFlowAppliesTheProductExclude(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowWrite(t, repo, "ai-specs/ai-specs.toml", "[recipes.plan-build-flow]\nenabled = true\n\n"+
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nproduct_exclude = [\"internal/**\"]\n")
	flowCommitManifest(t, repo)
	flowWrite(t, repo, "internal/gates/gates.go", "package gates\n")
	flowGit(t, repo, "add", "-A")
	flowGit(t, repo, "commit", "-q", "-m", "feat: internal work")

	stdout, stderr, code := statusFlowRun(t, bin, repo)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"phase:    workstream", "no product-path divergence"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}
	// The same tree written while it is excluded owes no product checkpoint.
	if _, stderr, code := flowRun(t, bin, repo, flowEvent(repo, "internal/gates/gates.go"),
		"plan-build-gate", "--explain"); code != 0 ||
		stderr != "plan-build-gate: tier none; refutations none\n" {
		t.Fatalf("gate = (%d, %q), want the silent none tier", code, stderr)
	}
}

// L8-2: a checkout on the integration branch is the integration line itself, not a
// workstream and not a change: it reports the phase, exits 0 and names the cleanup.
func TestChangesStatusFlowReportsTheIntegrationBranch(t *testing.T) {
	bin, _ := smokeSetup(t)
	t.Setenv("GENTLE_AI_BIN", t.TempDir()+"/gentle-ai-absent")
	repo := flowRepo(t, "odd", nil)
	flowGit(t, repo, "checkout", "-q", "main")

	stdout, stderr, code := statusFlowRun(t, bin, repo)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"phase:    integration", "integration branch",
		"next:     ai-specs worktree-cleanup"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "phase:    workstream") {
		t.Errorf("report %q calls the integration branch a workstream", stdout)
	}
}
