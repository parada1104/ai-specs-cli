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

// refutations is the blocker-only view the assertion cases read; the notes that
// name an unapplied refutation have their own test below.
func refutations(root, mode, change string, base *string) []string {
	blockers, _ := Refutations(root, mode, change, base)
	return blockers
}

// R-a: changing a canonical spec with no delta in the change is a promotion
// bypass, in both modes.
func TestRefuteRaBlocksACanonicalSpecChangeWithoutADelta(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, base := refuteRepo(t)
			spec := planbuild.SpecsBase(root, mode)[len(root)+1:] + "/cap/spec.md"
			commitPath(t, root, filepath.ToSlash(spec), "docs: edit the canonical spec")

			blockers := refutations(root, mode, "thing", &base)
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

// R-a is refuted by a delta in the diff: in the active change folder, or in
// its archive entry once the change was promoted and archived.
func TestRefuteRaPassesWhenTheChangeCarriesADelta(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		for _, folder := range []string{"thing", "2026-01-02-thing"} {
			t.Run(mode+"/"+folder, func(t *testing.T) {
				root, base := refuteRepo(t)
				spec := filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:]) + "/cap/spec.md"

				deltaFolder := filepath.ToSlash(planbuild.ChangeFolder(root, mode, "thing")[len(root)+1:])
				if folder != "thing" {
					deltaFolder = filepath.ToSlash(planbuild.ArchiveBase(root, mode)[len(root)+1:]) + "/" + folder
				}
				commitPath(t, root, spec, "docs: edit the canonical spec")
				commitPath(t, root, deltaFolder+"/specs/cap/spec.md", "docs: add the delta")

				if blockers := refutations(root, mode, "thing", &base); len(blockers) != 0 {
					t.Fatalf("Refutations = %q, want none for a change that carries a delta", blockers)
				}
			})
		}
	}
}

// R-a reads the diff, not the tree: a delta left by an earlier change proves nothing.
func TestRefuteRaIgnoresADeltaThatIsNotPartOfTheDiff(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		root, _ := refuteRepo(t)
		archive := filepath.ToSlash(planbuild.ArchiveBase(root, mode)[len(root)+1:])
		commitPath(t, root, archive+"/2026-01-02-thing/specs/cap/spec.md", "chore: earlier archive")
		base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
		commitPath(t, root, filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:])+"/cap/spec.md", "docs: edit")
		if blockers := refutations(root, mode, "thing", &base); len(blockers) == 0 {
			t.Fatalf("%s: want R-a over a delta that is not in the diff, got %q", mode, blockers)
		}
	}
}

// A raw non-ASCII component and both sides of a rename reach R-a.
func TestRefuteRaGradesRawAndRenamedPaths(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		for _, tc := range []struct{ name, spec, move string }{
			{"non-ascii", "/cap-ñ/spec.md", ""},
			{"rename", "/cap/spec.md", "moved-spec.md"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				root, base := refuteRepo(t)
				spec := filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:]) + tc.spec
				commitPath(t, root, spec, "docs: add the canonical spec")
				if tc.move != "" {
					base = strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
					gitRun(t, root, "mv", spec, tc.move)
					gitRun(t, root, "commit", "-q", "-m", "docs: move the spec out")
					entries, ok := planbuild.DiffEntries(root, base)
					parted, arrived := false, false
					for _, entry := range entries {
						parted = parted || (entry.Path == spec && !entry.Adds())
						arrived = arrived || (entry.Path == tc.move && entry.Adds())
					}
					if !ok || !parted || !arrived {
						t.Fatalf("DiffEntries = %+v, want the old path deleted and the new one added", entries)
					}
				}
				blockers := refutations(root, mode, "thing", &base)
				if !strings.Contains(joinedBlockers(blockers), "R-a:") ||
					!strings.Contains(joinedBlockers(blockers), spec) {
					t.Fatalf("Refutations = %q, want R-a naming %q", blockers, spec)
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

	blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base)
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

	commitPath(t, root, "openspec/changes/thing/tasks.md", "docs: add the plan")
	if blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
		t.Fatalf("Refutations = %q, want none once the change carries a plan", blockers)
	}

	root, base = refuteRepo(t, "go.mod")
	commitPath(t, root, "go.mod", "build: bump a module")
	commitPath(t, root, "openspec/changes/archive/2026-01-02-thing/tasks.md", "docs: archive the plan")
	if blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
		t.Fatalf("Refutations = %q, want none for an archived plan", blockers)
	}
}

// R-b reads the high-risk list from the trusted base revision.
func TestRefuteRbReadsThePolicyFromTheBaseRevision(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		root, base := refuteRepo(t, "go.mod")
		writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), "[recipes.plan-build-flow.config]\n")
		commitPath(t, root, "go.mod", "build: bump a module and delete the policy")
		if blockers := refutations(root, mode, "thing", &base); len(blockers) == 0 ||
			!strings.HasPrefix(joinedBlockers(blockers), "R-b:") {
			t.Fatalf("%s: Refutations = %q, want R-b from the base revision's policy", mode, blockers)
		}
	}
}

// R-b in odd: the evidence is a `Risk: item N (reason)` trailer on a commit of
// the range, and a bare `Risk: none` never satisfies it.
func TestRefuteRbRequiresARiskTrailerInOdd(t *testing.T) {
	root, base := refuteRepo(t, "go.mod")
	commitPath(t, root, "go.mod", "build: bump a module\n\nRisk: none\n")

	blockers := refutations(root, planbuild.ModeOdd, "thing", &base)
	if len(blockers) != 1 || !strings.HasPrefix(blockers[0], "R-b:") {
		t.Fatalf("Refutations = %q, want one R-b blocker", blockers)
	}
	for _, want := range []string{
		"go.mod",
		"no commit of `" + base + "..HEAD`",
		"Risk: item N (<reason>)",
		"`Risk: none` does not satisfy it",
		// The refusal is the one place the blocked agent reads, so it must say
		// which N applies instead of leaving 1..6 opaque.
		"1 data or irreversible effects that cannot be rebuilt",
		"2 security",
		"3 contracts others already consume",
		"4 concurrency",
		"5 delivery or environment",
		"6 no test would catch a regression",
	} {
		if !strings.Contains(blockers[0], want) {
			t.Errorf("blocker %q does not contain %q", blockers[0], want)
		}
	}

	gitRun(t, root, "commit", "--allow-empty", "-q", "-m", "build: bump a module\n\nRisk: item 2 (module graph)")
	if blockers := refutations(root, planbuild.ModeOdd, "thing", &base); len(blockers) != 0 {
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
			if blockers := refutations(root, mode, "thing", &base); len(blockers) != 0 {
				t.Fatalf("Refutations = %q, want none without a declared list", blockers)
			}
		})
	}
	if paths := HighRiskPaths(t.TempDir(), "HEAD"); len(paths) != 0 {
		t.Fatalf("HighRiskPaths over no manifest = %q, want none", paths)
	}
	root, base := refuteRepo(t, "go.mod")
	if paths := HighRiskPaths(root, ""); len(paths) != 0 {
		t.Fatalf("HighRiskPaths with no base = %q, want none", paths)
	}
	if paths := HighRiskPaths(root, base); len(paths) != 1 || paths[0] != "go.mod" {
		t.Fatalf("HighRiskPaths at the base = %q, want [go.mod]", paths)
	}
}

// Without a diff there is nothing to refute: a non-repository, an unresolvable
// base and a shallow clone all keep the `none` declaration valid.
func TestRefuteReturnsNothingWithoutADiff(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module x\n")
	if blockers := refutations(root, planbuild.ModeOdd, "thing", nil); len(blockers) != 0 {
		t.Fatalf("Refutations outside a repository = %q, want none", blockers)
	}
	base := "no-such-base"
	if blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
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

// The delta must be contributed by this diff. A deleted delta proves nothing, so
// editing a canonical spec is still a promotion bypass.
func TestRefuteRaBlocksADeletedDelta(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, _ := refuteRepo(t)
			delta := filepath.ToSlash(planbuild.ChangeFolder(root, mode, "thing")[len(root)+1:]) +
				"/specs/cap/spec.md"
			spec := filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:]) + "/cap/spec.md"
			commitPath(t, root, delta, "feat: add the delta")
			base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
			gitRun(t, root, "rm", "-q", delta)
			commitPath(t, root, spec, "docs: edit the canonical spec")

			blockers := refutations(root, mode, "thing", &base)
			if !strings.HasPrefix(joinedBlockers(blockers), "R-a:") {
				t.Fatalf("refutations = %q, want R-a over a deleted delta", blockers)
			}
		})
	}
}

// Renaming the delta out of the change tree is the same bypass as deleting it.
func TestRefuteRaBlocksADeltaRenamedOutOfTheTree(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, _ := refuteRepo(t)
			delta := filepath.ToSlash(planbuild.ChangeFolder(root, mode, "thing")[len(root)+1:]) +
				"/specs/cap/spec.md"
			spec := filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:]) + "/cap/spec.md"
			commitPath(t, root, delta, "feat: add the delta")
			base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
			gitRun(t, root, "mv", delta, "moved-delta.md")
			commitPath(t, root, spec, "docs: edit the canonical spec")

			blockers := refutations(root, mode, "thing", &base)
			if !strings.HasPrefix(joinedBlockers(blockers), "R-a:") {
				t.Fatalf("refutations = %q, want R-a over a delta renamed away", blockers)
			}
		})
	}
}

// A file added under specs/ is only a delta when it is Markdown: the rename of a
// delta to a non-Markdown name must not credit R-a (found by the slice-6 probe).
func TestRefuteRaBlocksADeltaRenamedToANonMarkdownFile(t *testing.T) {
	for _, mode := range []string{planbuild.ModeOdd, planbuild.ModeOpenspec} {
		t.Run(mode, func(t *testing.T) {
			root, _ := refuteRepo(t)
			delta := filepath.ToSlash(planbuild.ChangeFolder(root, mode, "thing")[len(root)+1:]) +
				"/specs/cap/spec.md"
			spec := filepath.ToSlash(planbuild.SpecsBase(root, mode)[len(root)+1:]) + "/cap/spec.md"
			commitPath(t, root, delta, "feat: add the delta")
			base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
			gitRun(t, root, "mv", delta, filepath.ToSlash(filepath.Dir(delta))+"/spec.txt")
			commitPath(t, root, spec, "docs: edit the canonical spec")

			blockers := refutations(root, mode, "thing", &base)
			if !strings.HasPrefix(joinedBlockers(blockers), "R-a:") {
				t.Fatalf("refutations = %q, want R-a over a delta renamed to .txt", blockers)
			}
		})
	}
}

// A date-prefixed name is the archive convention only: an active folder called
// 2026-01-01-thing names a different change and its plan must not answer for
// `thing` (R1-001 of the slice-6 review).
func TestRefuteRbIgnoresADatedActiveFolder(t *testing.T) {
	root, _ := refuteRepo(t, "go.mod")
	commitPath(t, root, "openspec/changes/2026-01-01-thing/tasks.md", "chore: add another change's plan")
	base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	commitPath(t, root, "go.mod", "build: bump a module")

	blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base)
	if !strings.HasPrefix(joinedBlockers(blockers), "R-b:") {
		t.Fatalf("refutations = %q, want R-b over a dated active folder", blockers)
	}
}

// A plan from an earlier change never answers R-b: only a plan this diff adds or
// modifies is evidence, or every archived change would answer for every later one.
func TestRefuteRbIgnoresAPlanFromAnEarlierChange(t *testing.T) {
	root, _ := refuteRepo(t, "go.mod")
	commitPath(t, root, "openspec/changes/archive/2020-01-01-thing/tasks.md", "chore: archive an earlier change")
	base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	commitPath(t, root, "go.mod", "build: bump a module")

	blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base)
	if len(blockers) != 1 || !strings.HasPrefix(blockers[0], "R-b:") {
		t.Fatalf("refutations = %q, want R-b over a stale archived plan", blockers)
	}
}

// A plan added in this diff is the evidence R-b asks for in openspec.
func TestRefuteRbPassesForAPlanAddedInThisDiff(t *testing.T) {
	root, base := refuteRepo(t, "go.mod")
	commitPath(t, root, "go.mod", "build: bump a module")
	commitPath(t, root, "openspec/changes/thing/tasks.md", "docs: add the plan")

	if blockers := refutations(root, planbuild.ModeOpenspec, "thing", &base); len(blockers) != 0 {
		t.Fatalf("refutations = %q, want none for a plan this diff adds", blockers)
	}
}

// A refutation that could not be applied is named, never silently skipped, and
// it is never a blocker: the small change still passes.
func TestRefuteNotesARefutationItCouldNotApply(t *testing.T) {
	root, _ := refuteRepo(t)
	unknown := "no-such-ref"
	blockers, notes := Refutations(root, planbuild.ModeOpenspec, "thing", &unknown)
	if len(blockers) != 0 || len(notes) != 1 {
		t.Fatalf("Refutations = (%q, %q), want no blocker and one note", blockers, notes)
	}
	for _, want := range []string{"R-a and R-b were not applied", "--base no-such-ref", "git rev-parse"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("note %q does not contain %q", notes[0], want)
		}
	}

	outside := t.TempDir()
	if _, notes := Refutations(outside, planbuild.ModeOpenspec, "thing", nil); len(notes) != 1 ||
		!strings.Contains(notes[0], "not a git repository") {
		t.Fatalf("notes outside a repository = %q, want one note naming the missing repository", notes)
	}

	if _, notes := Refutations(root, planbuild.ModeOpenspec, "thing", nil); len(notes) != 0 {
		t.Fatalf("notes over a resolvable base = %q, want none", notes)
	}
}

// writeRepo is a committed repository whose base branch is main and whose HEAD is
// feat/thing, so WriteRefutations resolves a base ref and grades later commits.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "test")
	writeFile(t, filepath.Join(root, "README.md"), "# base\n")
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "base")
	gitRun(t, root, "checkout", "-q", "-b", "feat/thing")
	return root
}

// R2 grades the pending write, not only the paths the branch already changed:
// on a clean branch the first write to a declared high-risk path is refused
// before it happens.
func TestWriteRefutationsGradeThePendingHighRiskTarget(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"ai-specs/ai-specs.toml": "[recipes.plan-build-flow.config]\nmode = \"odd\"\n" +
			"high_risk_paths = [\"src/**\"]\n",
	})
	blockers := WriteRefutations(root, planbuild.ModeOdd, "thing", "src/app.go")
	if len(blockers) != 1 || !strings.HasPrefix(blockers[0], "R2:") ||
		!strings.Contains(blockers[0], "src/app.go") {
		t.Fatalf("WriteRefutations = %q, want one R2 blocker naming the pending path", blockers)
	}
}

// The pre-archive stage grades the same refutations: a canonical spec edit with
// no delta is a promotion bypass there too.
func TestCheckPrearchiveBlocksACanonicalSpecEditWithoutADelta(t *testing.T) {
	root, base := refuteRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "thing")
	writeFile(t, filepath.Join(folder, "tasks.md"), "## Specs\n\n- **S1.** Fixture\n\n## Tasks\n\n"+
		"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n"+
		"| T1 | S1 | Fixture task | inline | pending |\n")
	commitPath(t, root, "openspec/specs/cap/spec.md", "docs: edit the canonical spec")

	result := CheckPrearchive(root, "thing", nil, &base)
	if result.OK {
		t.Fatalf("Result = %+v, want R-a to block the pre-archive stage", result)
	}
	assertBlockerContains(t, result, "R-a:")
}
