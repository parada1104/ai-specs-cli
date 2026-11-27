package gates

// The seam contract (change-ledger-v2 T2, L5): one fact collection per write, and
// one verdict that both the refusal and `--explain` are read from.

import (
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/tier"
)

// A refused write reports the tier and the rules the refusal names, because both
// come from the same verdict: R1 on the canonical tree and R2 on the same declared
// path fire together, and Rules() is what `--explain` prints.
func TestWriteVerdictDrivesBothTheRefusalAndTheExplain(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" +
			"high_risk_paths = [\"odd/specs/**\"]\n",
	})
	verdict := WriteFactsFor(root, planbuild.ModeOdd, "thing", "odd/specs/cap/spec.md").Verdict()

	if verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want the write refused", verdict)
	}
	if got, want := verdict.Rules(), "R1, R2"; got != want {
		t.Fatalf("Rules() = %q, want %q", got, want)
	}
	if got, want := verdict.Tier, tier.None; got != want {
		t.Fatalf("Tier = %q, want %q", got, want)
	}
	for _, blocker := range verdict.Blockers {
		if !strings.HasPrefix(blocker.String(), blocker.Rule+": ") {
			t.Errorf("blocker %q does not name its rule", blocker.String())
		}
	}
}

// The S16 fact is collected, never passed in: a worktree with no artifact and no
// product-path divergence is a workstream, and one product commit makes it a
// change.
func TestWriteFactsOwnsTheWorkstreamFact(t *testing.T) {
	root := writeRepo(t, map[string]string{"src/app.go": "package src\n"})

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "docs/deploy.md")
	if !facts.Workstream {
		t.Fatalf("Facts = %+v, want a workstream", facts)
	}

	commitPath(t, root, "src/app.go", "feat: product work")
	facts = WriteFactsFor(root, planbuild.ModeOdd, "thing", "docs/deploy.md")
	if facts.Workstream {
		t.Fatalf("Facts = %+v, want a change after a product commit", facts)
	}
	if got, want := facts.Verdict().Tier, tier.None; got != want {
		t.Fatalf("Tier = %q, want %q: a plan and a delta are what make a tier", got, want)
	}
}

// The delta R1 asks for is the exit it honours: a branch that promoted the rule
// and committed the delta behind it is allowed again, and the same branch without
// the delta is refused.
func TestWriteVerdictAllowsACommittedCanonicalEditOnceTheDeltaExists(t *testing.T) {
	root := writeRepo(t, nil)
	commitPath(t, root, "src/app.go", "feat: product work")
	commitPath(t, root, "odd/specs/cap/spec.md", "spec: promote the rule")

	refused := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go").Verdict()
	if got, want := refused.Rules(), "R1"; got != want {
		t.Fatalf("Rules() = %q, want %q before the delta", got, want)
	}

	commitPath(t, root, "odd/changes/thing/specs/cap/spec.md", "docs: the delta behind it")
	if verdict := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go").Verdict(); !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want the write allowed once the delta exists", verdict)
	}
}

// A workstream owes no diff-based evidence: a committed high-risk edit it cannot
// answer never traps an unrelated later write.
func TestWriteFactsDoesNotGradeAWorkstreamDiffForHighRisk(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" +
			"high_risk_paths = [\"docs/secret/**\"]\n",
	})
	commitPath(t, root, "docs/secret/runbook.md", "docs: a runbook edit")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "README.md")
	if !facts.Workstream {
		t.Fatalf("Facts = %+v, want a workstream", facts)
	}
	if verdict := facts.Verdict(); !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want no diff-based refutation for a workstream", verdict)
	}
}

// An artifact the gate never grades cannot require a refutation either: a committed
// plan that matches a declared pattern is not high-risk evidence the change owes.
func TestWriteFactsIgnoresPlanningPathsForHighRisk(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" +
			"high_risk_paths = [\"odd/**\"]\n",
	})
	commitPath(t, root, "odd/tasks/thing.md", "docs: the plan of this change")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "src/app.go")
	if facts.Workstream {
		t.Fatalf("Facts = %+v, want a change once it carries a plan", facts)
	}
	if verdict := facts.Verdict(); !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want no refutation from an ungraded plan file", verdict)
	}
}

// R2's openspec exit must be usable on a branch that pairs no change: it names
// that fact instead of an empty plan path.
func TestWriteVerdictNamesTheUnpairedBranchForAHighRiskTarget(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"openspec\"\n" +
			"high_risk_paths = [\"docs/**\"]\n",
	})
	verdict := WriteFactsFor(root, planbuild.ModeOpenspec, "", "docs/deploy.md").Verdict()
	if len(verdict.Blockers) != 1 || verdict.Blockers[0].Rule != "R2" {
		t.Fatalf("Blockers = %q, want one R2 blocker", verdict.Blockers)
	}
	message := verdict.Blockers[0].Message
	if !strings.Contains(message, "pairs no change") || strings.Contains(message, "carries no") {
		t.Fatalf("Message = %q, want the unpaired branch named instead of an empty plan path", message)
	}
}

// R2's openspec exit is honoured: a plan this branch committed is the evidence,
// even though the gate never grades the change tree. Before the plan provenance was
// read apart from the graded scope, R2 asked for a plan it had already discarded.
func TestWriteVerdictAcceptsACommittedPlanInOpenspec(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"openspec\"\n" +
			"high_risk_paths = [\"docs/**\"]\n",
	})
	commitPath(t, root, "openspec/changes/thing/tasks.md", "docs: the plan of this change")

	facts := WriteFactsFor(root, planbuild.ModeOpenspec, "thing", "docs/deploy.md")
	if !facts.HasPlan || !facts.PlanInDiff {
		t.Fatalf("Facts = %+v, want the committed plan read as evidence", facts)
	}
	verdict := facts.Verdict()
	if !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want the committed plan to answer R2", verdict)
	}
	if got, want := verdict.Tier, tier.Light; got != want {
		t.Fatalf("Tier = %q, want %q", got, want)
	}
}

// An unread base ref is a fact, not silence: the diff rules did not run, and the
// verdict says so instead of looking like "nothing to grade".
func TestWriteVerdictReportsAnUnresolvedBase(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "trunk")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "test")
	writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = [\"docs/**\"]\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "base")
	gitRun(t, root, "checkout", "-q", "-b", "feat/thing")

	verdict := WriteFactsFor(root, planbuild.ModeOdd, "thing", "docs/deploy.md").Verdict()
	if len(verdict.Notes) != 1 || !strings.Contains(verdict.Notes[0], "no base ref resolves") {
		t.Fatalf("Notes = %q, want the unread base reported", verdict.Notes)
	}
	if !verdict.Allowed() {
		t.Fatalf("Verdict = %+v, want a note to never block", verdict)
	}
}
