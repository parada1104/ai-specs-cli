package gates

// The tier is one derivation, in both modes (change-ledger-v2 T1, S1/S2): the
// artifacts the change carries decide none/light/standard, and ODD computes the
// same three values instead of its own `odd` label. The boundary is that
// `none` survives only when there is neither a plan nor a delta.

import (
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// oddTierRepo builds an odd planning root carrying the requested artifacts.
func oddTierRepo(t *testing.T, plan, delta bool) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"odd/tasks", "odd/changes", "odd/specs"} {
		mkdirAll(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	if plan {
		writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), "# plan\n")
	}
	if delta {
		writeFile(t, filepath.Join(root, "odd", "changes", "thing", "specs", "capability", "spec.md"), oddDelta)
	}
	return root
}

// openspecTierRepo builds an active openspec change folder carrying the
// requested artifacts.
func openspecTierRepo(t *testing.T, plan, delta bool) string {
	t.Helper()
	root := t.TempDir()
	folder := filepath.Join(root, "openspec", "changes", "thing")
	mkdirAll(t, folder)
	if plan {
		writeFile(t, filepath.Join(folder, "tasks.md"), "# plan\n")
	}
	if delta {
		writeFile(t, filepath.Join(folder, "specs", "capability", "spec.md"), oddDelta)
	}
	return root
}

func TestTierDerivationTable(t *testing.T) {
	cases := []struct {
		name  string
		mode  string
		plan  bool
		delta bool
		want  string
	}{
		{"odd/no-artifacts", planbuild.ModeOdd, false, false, TierNone},
		{"odd/plan-only", planbuild.ModeOdd, true, false, TierLight},
		{"odd/delta-only", planbuild.ModeOdd, false, true, TierLight},
		{"odd/plan-and-delta", planbuild.ModeOdd, true, true, TierStandard},
		{"openspec/no-artifacts", planbuild.ModeOpenspec, false, false, TierNone},
		{"openspec/plan-only", planbuild.ModeOpenspec, true, false, TierLight},
		{"openspec/delta-only", planbuild.ModeOpenspec, false, true, TierLight},
		{"openspec/plan-and-delta", planbuild.ModeOpenspec, true, true, TierStandard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.mode == planbuild.ModeOdd {
				got = CheckPremergeOdd(oddTierRepo(t, tc.plan, tc.delta), "thing").Tier
			} else {
				got = CheckPrearchive(openspecTierRepo(t, tc.plan, tc.delta), "thing", nil).Tier
			}
			if got != tc.want {
				t.Fatalf("Tier = %q, want %q", got, tc.want)
			}
		})
	}
}
