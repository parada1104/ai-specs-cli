package gates

// The merge-time refutations R3-R5 and the per-project posture (change-ledger-v2
// T8, S5/S10/S16): thresholds as CLI defaults overridable per project, the removed
// public contract, the plan whose `S#` rows carry no evidence, and the checkpoint
// posture that decides whether they refuse or only advise. Every case grades a real
// diff, because the facts are what makes a rule applicable.

import (
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// postureManifest is the manifest a fixture commits: odd mode, plus the extra
// [recipes.plan-build-flow.config] keys the case declares.
func postureManifest(extra string) string {
	return "[recipes.plan-build-flow]\nenabled = true\n\n[recipes.plan-build-flow.config]\nmode = \"odd\"\n" + extra
}

// postureRepo is a git repository whose baseline carries the manifest and the given
// files, so whatever a test commits afterwards is the whole diff against the base.
func postureRepo(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "config", "user.email", "posture@example.invalid")
	gitRun(t, root, "config", "user.name", "posture")
	writeFile(t, filepath.Join(root, "README.md"), "# base\n")
	writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), manifest)
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "baseline")
	gitRun(t, root, "checkout", "-q", "-b", "feat/thing")
	return root
}

// commitLines writes one file of exactly lines lines and commits it.
func commitLines(t *testing.T, root, rel string, lines int) {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), strings.Repeat("line\n", lines))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "change "+rel)
}

// postureRules names the rules the refutations fired, in order.
func postureRules(refutations []Refutation) string {
	if len(refutations) == 0 {
		return "none"
	}
	rules := make([]string, 0, len(refutations))
	for _, refutation := range refutations {
		rules = append(rules, refutation.Rule)
	}
	return strings.Join(rules, ", ")
}

// postureRefutation returns the one refutation of the named rule, nil when it did
// not fire.
func postureRefutation(refutations []Refutation, rule string) *Refutation {
	for i := range refutations {
		if refutations[i].Rule == rule {
			return &refutations[i]
		}
	}
	return nil
}

// S5/S10: the posture ships no declaration by default — advise, never refuse — and
// the thresholds are the CLI's until the project declares its own.
func TestPostureDefaultsAndProjectOverrides(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		posture := LoadPosture(postureRepo(t, postureManifest(""), nil))
		if len(posture.Require) != 0 || len(posture.Notes) != 0 {
			t.Fatalf("posture = %+v, want no declaration and no note", posture)
		}
		if posture.Thresholds != (Thresholds{Lines: 1000, Modules: 5}) {
			t.Fatalf("thresholds = %+v, want the CLI defaults 1000/5", posture.Thresholds)
		}
		if posture.Requires(CheckpointMerge) {
			t.Fatal("a project declaring nothing requires a posture: advise is the default")
		}
	})

	t.Run("declared", func(t *testing.T) {
		posture := LoadPosture(postureRepo(t, postureManifest(
			"require = [\"merge\"]\nrisk_thresholds = { lines = 2000, modules = 3 }\n"), nil))
		if len(posture.Notes) != 0 {
			t.Fatalf("notes = %v, want none for a well-formed declaration", posture.Notes)
		}
		if !posture.Requires(CheckpointMerge) {
			t.Fatalf("require = %v, want merge among them", posture.Require)
		}
		if posture.Requires("") || posture.Requires("archive-close") {
			t.Fatalf("require = %v, want only the named checkpoint", posture.Require)
		}
		if posture.Thresholds != (Thresholds{Lines: 2000, Modules: 3}) {
			t.Fatalf("thresholds = %+v, want the declared 2000/3", posture.Thresholds)
		}
	})

	t.Run("one threshold key omitted", func(t *testing.T) {
		posture := LoadPosture(postureRepo(t, postureManifest("risk_thresholds = { modules = 2 }\n"), nil))
		if posture.Thresholds != (Thresholds{Lines: 1000, Modules: 2}) {
			t.Fatalf("thresholds = %+v, want lines to keep the default", posture.Thresholds)
		}
	})
}

// L5 item 5: a malformed declaration is a note naming what is accepted, never a
// silent default and never a refusal.
func TestPostureNotesAMalformedDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		extra    string
		wantNote string
		want     Thresholds
		require  []string
	}{
		{
			name: "require is not an array", extra: "require = \"merge\"\n",
			wantNote: "require", want: Thresholds{Lines: 1000, Modules: 5},
		},
		{
			name: "an unknown checkpoint", extra: "require = [\"premerge\"]\n",
			wantNote: "premerge", want: Thresholds{Lines: 1000, Modules: 5},
		},
		{
			name: "a non-integer threshold", extra: "risk_thresholds = { lines = \"many\" }\n",
			wantNote: "risk_thresholds", want: Thresholds{Lines: 1000, Modules: 5},
		},
		{
			name: "a non-positive threshold", extra: "risk_thresholds = { modules = 0 }\n",
			wantNote: "risk_thresholds", want: Thresholds{Lines: 1000, Modules: 5},
		},
		{
			name: "thresholds that are not a table", extra: "risk_thresholds = 5\n",
			wantNote: "risk_thresholds", want: Thresholds{Lines: 1000, Modules: 5},
		},
		{
			name: "a valid checkpoint beside a malformed one", extra: "require = [\"merge\", 7]\n",
			wantNote: "require", want: Thresholds{Lines: 1000, Modules: 5}, require: []string{CheckpointMerge},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posture := LoadPosture(postureRepo(t, postureManifest(tc.extra), nil))
			if len(posture.Notes) == 0 {
				t.Fatalf("posture = %+v, want a note naming the malformed declaration", posture)
			}
			if !strings.Contains(strings.Join(posture.Notes, "\n"), tc.wantNote) {
				t.Fatalf("notes = %v, want them naming %q", posture.Notes, tc.wantNote)
			}
			if posture.Thresholds != tc.want {
				t.Fatalf("thresholds = %+v, want the defaults to stay (%+v)", posture.Thresholds, tc.want)
			}
			if strings.Join(posture.Require, ",") != strings.Join(tc.require, ",") {
				t.Fatalf("require = %v, want %v", posture.Require, tc.require)
			}
		})
	}
}

// S10/L5 item 2: R3 is the size and module threshold, and it counts product paths
// only.
func TestR3FiresOverTheLineThreshold(t *testing.T) {
	root := postureRepo(t, postureManifest(""), nil)
	commitLines(t, root, "internal/gates/big.go", 1001)

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if !merge.SizeKnown || merge.Lines != 1001 || merge.Modules != 1 {
		t.Fatalf("merge facts = %+v, want 1001 lines over 1 module", merge)
	}
	refutations := facts.MergeRefutations(merge, DefaultThresholds)
	fired := postureRefutation(refutations, "R3")
	if fired == nil {
		t.Fatalf("refutations = %v, want R3 over the line threshold", refutations)
	}
	for _, want := range []string{"1001", "lines = 1000", "no plan", "tier light", "risk_thresholds"} {
		if !strings.Contains(fired.Message, want) {
			t.Errorf("R3 %q does not name %q", fired.Message, want)
		}
	}
}

func TestR3FiresOverTheModuleThreshold(t *testing.T) {
	root := postureRepo(t, postureManifest(""), nil)
	for _, module := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		commitLines(t, root, module+"/main.go", 1)
	}
	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if merge.Modules != 6 || merge.Lines != 6 {
		t.Fatalf("merge facts = %+v, want 6 lines over 6 modules", merge)
	}
	fired := postureRefutation(facts.MergeRefutations(merge, DefaultThresholds), "R3")
	if fired == nil {
		t.Fatal("R3 did not fire over the module threshold")
	}
	if !strings.Contains(fired.Message, "modules = 5") {
		t.Fatalf("R3 %q does not name the module threshold", fired.Message)
	}
}

// T2 r1: a documentation or planning diff never trips the size rule, however many
// lines it moves: only product paths count.
func TestR3CountsProductPathsOnly(t *testing.T) {
	root := postureRepo(t, postureManifest(""), nil)
	commitLines(t, root, "odd/tasks/notes.md", 1500)
	commitLines(t, root, "openspec/changes/other/specs/spec.md", 1500)
	commitLines(t, root, "internal/gates/small.go", 5)
	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if !merge.SizeKnown || merge.Lines != 5 || merge.Modules != 1 {
		t.Fatalf("merge facts = %+v, want the planning trees excluded from the count", merge)
	}
	if refutations := facts.MergeRefutations(merge, DefaultThresholds); len(refutations) != 0 {
		t.Fatalf("refutations = %v, want none under the thresholds", refutations)
	}

	// A diff of planning trees alone is not a change at all (S16), so no merge-time
	// rule reaches it either.
	planningOnly := postureRepo(t, postureManifest(""), nil)
	commitLines(t, planningOnly, "odd/tasks/notes.md", 1500)
	planningFacts := WriteFactsFor(planningOnly, planbuild.ModeOdd, "thing", "")
	if refutations := planningFacts.MergeRefutations(MergeFactsFor(planningOnly, planningFacts),
		DefaultThresholds); len(refutations) != 0 {
		t.Fatalf("refutations = %v, want none for a planning-only worktree", refutations)
	}
}

// S10: the project's own thresholds decide, not the CLI's.
func TestR3HonoursTheProjectThresholds(t *testing.T) {
	raising := postureRepo(t, postureManifest("risk_thresholds = { lines = 2000 }\n"), nil)
	commitLines(t, raising, "internal/gates/big.go", 1200)
	raisingFacts := WriteFactsFor(raising, planbuild.ModeOdd, "thing", "")
	raisingMerge := MergeFactsFor(raising, raisingFacts)
	if got := raisingFacts.MergeRefutations(raisingMerge, LoadPosture(raising).Thresholds); len(got) != 0 {
		t.Fatalf("refutations = %v, want the raised threshold to silence R3", got)
	}

	lowering := postureRepo(t, postureManifest("risk_thresholds = { lines = 10 }\n"), nil)
	commitLines(t, lowering, "internal/gates/small.go", 20)
	loweringFacts := WriteFactsFor(lowering, planbuild.ModeOdd, "thing", "")
	loweringMerge := MergeFactsFor(lowering, loweringFacts)
	if fired := postureRefutation(loweringFacts.MergeRefutations(loweringMerge,
		LoadPosture(lowering).Thresholds), "R3"); fired == nil {
		t.Fatal("R3 did not fire over the lowered threshold")
	}
}

// L5 item 3: R4 is the removed public contract, declared by the project and read
// from the diff — never inferred from the shape of a line.
func TestR4FiresOnADeletedHighRiskPathWithoutAPlan(t *testing.T) {
	root := postureRepo(t, postureManifest("high_risk_paths = [\"internal/api/**\"]\n"),
		map[string]string{"internal/api/http.go": "package api\n"})
	gitRun(t, root, "rm", "-q", "internal/api/http.go")
	gitRun(t, root, "commit", "-q", "-m", "remove the endpoint")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if strings.Join(merge.Removed, ",") != "internal/api/http.go" {
		t.Fatalf("merge facts = %+v, want the deleted high-risk path", merge)
	}
	fired := postureRefutation(facts.MergeRefutations(merge, DefaultThresholds), "R4")
	if fired == nil {
		t.Fatal("R4 did not fire on the removed public contract")
	}
	for _, want := range []string{"internal/api/http.go", "high-risk", "tier light"} {
		if !strings.Contains(fired.Message, want) {
			t.Errorf("R4 %q does not name %q", fired.Message, want)
		}
	}
}

func TestR4StaysSilentWhenTheChangeCarriesAPlan(t *testing.T) {
	root := postureRepo(t, postureManifest("high_risk_paths = [\"internal/api/**\"]\n"),
		map[string]string{"internal/api/http.go": "package api\n"})
	gitRun(t, root, "rm", "-q", "internal/api/http.go")
	writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), posturePlan(""+
		"- **S1: PASS** — `go test ./...`, exit 0\n"))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "remove the endpoint with its plan")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	if !facts.HasPlan {
		t.Fatal("the fixture carries no plan, so the case proves nothing")
	}
	merge := MergeFactsFor(root, facts)
	if refutations := facts.MergeRefutations(merge, DefaultThresholds); len(refutations) != 0 {
		t.Fatalf("refutations = %v, want the plan to answer the removal", refutations)
	}
}

// posturePlan is a light-tier plan whose `## Verification` section is verify.
func posturePlan(verify string) string {
	plan := "## Specs\n\n- **S1.** One rule\n\n## Tasks\n\n" +
		"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1 | Do it | inline | pending |\n"
	if verify == "" {
		return plan
	}
	return plan + "\n## Verification\n\n" + verify
}

// postureTwoSpecPlan declares two requirements and grades however many rows verify
// names, so one of them can be left unanswered.
func postureTwoSpecPlan(verify string) string {
	return "## Specs\n\n- **S1.** One rule\n- **S2.** Another rule\n\n## Tasks\n\n" +
		"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1, S2 | Do it | inline | pending |\n\n## Verification\n\n" + verify
}

// L5 item 4: R5 is the plan whose `S#` rows carry no evidence, graded by the same
// verification grader the guardian reads.
func TestR5FiresOnAPlanWhoseSpecsCarryNoVerification(t *testing.T) {
	root := postureRepo(t, postureManifest(""), nil)
	writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), postureTwoSpecPlan(
		"- **S1: PASS** — `go test ./...`, exit 0\n"))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "plan: thing with a spec and no row for it")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if merge.Specs != 2 {
		t.Fatalf("merge facts = %+v, want the two declared S#", merge)
	}
	// S2 carries no row of its own, so the guardian's grader reports it and R5
	// refuses instead of reading an empty section as evidence.
	fired := postureRefutation(facts.MergeRefutations(merge, DefaultThresholds), "R5")
	if fired == nil {
		t.Fatalf("merge facts = %+v, want R5 on the unanswered plan", merge)
	}
	if !strings.Contains(fired.Message, "S2") {
		t.Fatalf("R5 %q does not name the requirement", fired.Message)
	}
	if !strings.Contains(fired.Message, "N/A") || !strings.Contains(fired.Message, "## Verification") {
		t.Fatalf("R5 %q does not name its exit", fired.Message)
	}
}

func TestR5StaysSilentWhenTheRowsCarryCommands(t *testing.T) {
	root := postureRepo(t, postureManifest(""), nil)
	writeFile(t, filepath.Join(root, "odd", "tasks", "thing.md"), posturePlan(
		"- **S1: PASS** — `go test ./...`, exit 0\n"))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "plan: thing")

	facts := WriteFactsFor(root, planbuild.ModeOdd, "thing", "")
	merge := MergeFactsFor(root, facts)
	if len(merge.Verification) != 0 {
		t.Fatalf("verification = %v, want no gap for a covered plan", merge.Verification)
	}
	if refutations := facts.MergeRefutations(merge, DefaultThresholds); len(refutations) != 0 {
		t.Fatalf("refutations = %v, want none", refutations)
	}
}

// S16: a worktree that is not a change owes nothing, so no posture-governed rule
// reaches it — and neither does an unpaired branch.
func TestNoMergeRuleAppliesToAWorkstreamOrAnUnpairedBranch(t *testing.T) {
	merge := MergeFacts{Lines: 5000, Modules: 9, SizeKnown: true, Specs: 2,
		Verification: []string{"verification is missing a row for S1"}, Removed: []string{"internal/api/http.go"}}
	posture := LoadPosture(postureRepo(t, postureManifest("require = [\"merge\"]\n"), nil))
	for _, facts := range []WriteFacts{
		{Workstream: true, Change: "thing", Scope: LoadProductScope(t.TempDir())},
		{Change: ""},
	} {
		if refutations := facts.MergeRefutations(merge, posture.Thresholds); len(refutations) != 0 {
			t.Fatalf("refutations = %v (%s), want none for %+v", refutations, postureRules(refutations), facts)
		}
		if notes := facts.MergeNotes(merge); len(notes) != 0 {
			t.Fatalf("notes = %v, want none for a worktree no rule governs", notes)
		}
	}
}

// An unanswered numstat is a note, never a pass and never a block (L5 item 2).
func TestMergeNotesNameTheFactTheyCouldNotObserve(t *testing.T) {
	facts := WriteFacts{Change: "thing"}
	if notes := facts.MergeNotes(MergeFacts{}); len(notes) != 1 ||
		!strings.Contains(notes[0], "R3") || !strings.Contains(notes[0], "numstat") {
		t.Fatalf("notes = %v, want one naming the unanswered line count", notes)
	}
	if notes := facts.MergeNotes(MergeFacts{SizeKnown: true}); len(notes) != 0 {
		t.Fatalf("notes = %v, want none when git answered", notes)
	}
}
