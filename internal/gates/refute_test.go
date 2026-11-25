package gates

// Objective refutations of a `none` change (plan-build-openspec-odd-align
// S16/S17): R-a is a canonical spec change without a delta, R-b is a change to a
// path the project declares as high-risk. Both are graded from a real diff, so
// every case here builds a git fixture instead of inventing paths.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// refuteRepo is a git repository with one committed baseline. When highRisk is
// not empty, the manifest declares exactly that list.
func refuteRepo(t *testing.T, highRisk ...string) (root, base string) {
	t.Helper()
	root = t.TempDir()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "test")
	writeFile(t, filepath.Join(root, "README.md"), "# base\n")
	if len(highRisk) > 0 {
		quoted := make([]string, len(highRisk))
		for i, pattern := range highRisk {
			quoted[i] = fmt.Sprintf("%q", pattern)
		}
		writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
			"[recipes.plan-build-flow.config]\nmode = \"odd\"\nhigh_risk_paths = ["+
				strings.Join(quoted, ", ")+"]\n")
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "baseline")
	base = strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	return root, base
}

// commitPath writes one file at a repository-relative path and commits it.
func commitPath(t *testing.T, root, rel, message string) {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "changed\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", message)
}

func joinedBlockers(blockers []string) string { return strings.Join(blockers, "\n") }

// R-a: changing a canonical spec with no delta in the change is a promotion
// bypass, in both modes.
func TestRefuteRaBlocksACanonicalSpecChangeWithoutADelta(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, base := refuteRepo(t)
			spec := planbuild.SpecsBase(root, mode)[len(root)+1:] + "/cap/spec.md"
			commitPath(t, root, filepath.ToSlash(spec), "docs: edit the canonical spec")

			blockers := Refutations(root, mode, "thing", &base)
			if !strings.HasPrefix(joinedBlockers(blockers), "R-a:") {
				t.Fatalf("Refutations = %q, want one R-a blocker", blockers)
			}
			for _, want := range []string{
				filepath.ToSlash(spec),
				"carries no delta",
				"a promotion bypass",
				"ai-specs specs promote",
			} {
				if !strings.Contains(joinedBlockers(blockers), want) {
					t.Errorf("blocker %q does not contain %q", blockers, want)
				}
			}
		})
	}
}

// R-a is refuted by a delta wherever it lives: in the active change folder, or
// only in its archive entry once the change was promoted and archived.
func TestRefuteRaPassesWhenTheChangeCarriesADelta(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		for _, folder := range []string{"thing", "2026-01-02-thing"} {
			t.Run(mode+"/"+folder, func(t *testing.T) {
				root, base := refuteRepo(t)
				spec := planbuild.SpecsBase(root, mode)[len(root)+1:] + "/cap/spec.md"
				commitPath(t, root, filepath.ToSlash(spec), "docs: edit the canonical spec")

				deltaFolder := planbuild.ChangeFolder(root, mode, "thing")
				if folder != "thing" {
					deltaFolder = filepath.Join(planbuild.ArchiveBase(root, mode), folder)
				}
				writeFile(t, filepath.Join(deltaFolder, "specs", "cap", "spec.md"), "# spec\n")

				if blockers := Refutations(root, mode, "thing", &base); len(blockers) != 0 {
					t.Fatalf("Refutations = %q, want none for a change that carries a delta", blockers)
				}
			})
		}
	}
}

// R-b in openspec: a plan is the only place plan-build keeps verification
// evidence, so a high-risk path without one blocks until a plan exists.
func TestRefuteRbRequiresAPlanInOpenspec(t *testing.T) {
	root, base := refuteRepo(t, "go.mod")
	commitPath(t, root, "go.mod", "build: bump a module")

	blockers := Refutations(root, planbuild.ModeOpenspec, "thing", &base)
	if len(blockers) != 1 || !strings.HasPrefix(blockers[0], "R-b:") {
		t.Fatalf("Refutations = %q, want one R-b blocker", blockers)
	}
	for _, want := range []string{
		"go.mod",
		"declared by the project in ai-specs/ai-specs.toml",
		"carries no plan",
		"openspec/changes/thing/tasks.md",
		"tier light",
	} {
		if !strings.Contains(blockers[0], want) {
			t.Errorf("blocker %q does not contain %q", blockers[0], want)
		}
	}

	writeFile(t, filepath.Join(root, "openspec", "changes", "thing", "tasks.md"), "# plan\n")
	if blockers := Refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
		t.Fatalf("Refutations = %q, want none once the change carries a plan", blockers)
	}
}

// R-b in odd: the evidence is a `Risk: item N (reason)` trailer on a commit of
// the range, and a bare `Risk: none` never satisfies it.
func TestRefuteRbRequiresARiskTrailerInOdd(t *testing.T) {
	root, base := refuteRepo(t, "go.mod")
	commitPath(t, root, "go.mod", "build: bump a module\n\nRisk: none\n")

	blockers := Refutations(root, planbuild.ModeOdd, "thing", &base)
	if len(blockers) != 1 || !strings.HasPrefix(blockers[0], "R-b:") {
		t.Fatalf("Refutations = %q, want one R-b blocker", blockers)
	}
	for _, want := range []string{
		"go.mod",
		"no commit of `" + base + "..HEAD`",
		"Risk: item N (<reason>)",
		"`Risk: none` does not satisfy it",
	} {
		if !strings.Contains(blockers[0], want) {
			t.Errorf("blocker %q does not contain %q", blockers[0], want)
		}
	}

	gitRun(t, root, "commit", "--allow-empty", "-q", "-m", "build: bump a module\n\nRisk: item 2 (module graph)")
	if blockers := Refutations(root, planbuild.ModeOdd, "thing", &base); len(blockers) != 0 {
		t.Fatalf("Refutations = %q, want none for a range that carries the trailer", blockers)
	}
}

// R-b is only ever the project's own declaration: with no high_risk_paths list,
// a change to the very same path is not refuted.
func TestRefuteRbNeverFiresWithoutADeclaredList(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, base := refuteRepo(t)
			commitPath(t, root, "go.mod", "build: bump a module")
			if blockers := Refutations(root, mode, "thing", &base); len(blockers) != 0 {
				t.Fatalf("Refutations = %q, want none without a declared list", blockers)
			}
		})
	}
	if paths := HighRiskPaths(t.TempDir()); len(paths) != 0 {
		t.Fatalf("HighRiskPaths over no manifest = %q, want none", paths)
	}
}

// Without a diff there is nothing to refute: a non-repository, an unresolvable
// base and a shallow clone all keep the `none` declaration valid.
func TestRefuteReturnsNothingWithoutADiff(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module x\n")
	if blockers := Refutations(root, planbuild.ModeOdd, "thing", nil); len(blockers) != 0 {
		t.Fatalf("Refutations outside a repository = %q, want none", blockers)
	}
	base := "no-such-base"
	if blockers := Refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
		t.Fatalf("Refutations over an unknown base = %q, want none", blockers)
	}
	if ref := BaseRef(root, nil); ref != "" {
		t.Fatalf("BaseRef outside a repository = %q, want empty", ref)
	}
}

// The refutations reach both premerge entry points: a none change in a git
// repository with a high-risk diff is blocked even though it carries no artifact.
func TestPremergeRefutationsReachTheGuardian(t *testing.T) {
	t.Run("openspec", func(t *testing.T) {
		root, base := refuteRepo(t, "go.mod")
		commitPath(t, root, "go.mod", "build: bump a module")
		result := CheckPremerge(root, "thing", nil, &base)
		if result.OK || result.Tier != TierNone {
			t.Fatalf("Result = %+v, want the none tier blocked by R-b", result)
		}
		if !strings.HasPrefix(joinedBlockers(result.Blockers), "R-b:") {
			t.Fatalf("Blockers = %q, want one R-b blocker", result.Blockers)
		}
	})
	t.Run("odd", func(t *testing.T) {
		root, base := refuteRepo(t, "go.mod")
		commitPath(t, root, "go.mod", "build: bump a module")
		result := CheckPremergeOdd(root, "thing", &base)
		if result.OK || result.Tier != TierNone {
			t.Fatalf("Result = %+v, want the none tier blocked by R-b", result)
		}
		if !strings.HasPrefix(joinedBlockers(result.Blockers), "R-b:") {
			t.Fatalf("Blockers = %q, want one R-b blocker", result.Blockers)
		}
	})
}

// The none path of the openspec premerge: no folder, no archive entry and no
// plan is a valid declaration, so it passes with the none tier and no blocker.
func TestCheckPremergePassesANoneChange(t *testing.T) {
	result := CheckPremerge(newRepo(t), "nothing", nil)
	if !result.OK || len(result.Blockers) != 0 {
		t.Fatalf("Result = %+v, want OK with no blocker", result)
	}
	if result.Tier != TierNone {
		t.Fatalf("Tier = %q, want %q", result.Tier, TierNone)
	}
}

// The declared depth has to cohere with the derivation on the none path too: an
// explicit --tier standard cannot pass silently as none.
func TestNonePathBlocksADeclaredTierThatContradictsIt(t *testing.T) {
	result := CheckPremerge(newRepo(t), "nothing", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a declared standard tier with no artifact must block")
	}
	assertBlockerContains(t, result, "the tier is derived from the artifacts of the change",
		"write tasks.md (light) or tasks.md plus a specs/ delta (standard)")
}
