package gates

// The seam contract (change-ledger-v2 T2, L5): one fact collection per write, and
// one verdict that both the refusal and `--explain` are read from.

import (
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
