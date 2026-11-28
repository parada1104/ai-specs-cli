package cli

// `ai-specs changes status --root <repo-root> [--json] [--explain]
// [--ledger worktree|plan|tracker]` (change-ledger-v2 T7, S3/S4/S12/S14/S16): the
// read-only composite. The phase precedence and the exit codes are unit-tested here
// without a repository; the read-only contract and the unknown facts are tested against
// real temporary repositories.

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// L5 item 1: the phase is derived from the facts, first match wins, and a fact that
// cannot separate two phases never silently resolves to the optimistic one.
func TestStatusPhasePrecedence(t *testing.T) {
	planned := statusEvidence{Checkout: true, Base: "main", Plan: true, PlanPath: "odd/tasks/thing.md"}
	cases := []struct {
		name     string
		evidence statusEvidence
		want     string
	}{
		{
			name:     "no checkout is unknown, never a phase",
			evidence: statusEvidence{},
			want:     statusUnknown,
		},
		{
			name: "a closed row with no open row is cleaned",
			evidence: statusEvidence{Checkout: true, Base: "main", ClosedRow: true, Plan: true,
				PlanPath: "odd/tasks/thing.md", Merged: true},
			want: statusCleaned,
		},
		{
			name: "an open row keeps the work out of cleaned",
			evidence: statusEvidence{Checkout: true, Base: "main", OpenRow: true, ClosedRow: true,
				Merged: true, Plan: true, PlanPath: "odd/tasks/thing.md"},
			want: statusMerged,
		},
		{
			name:     "a proven merge with a plan is merged",
			evidence: statusEvidence{Checkout: true, Base: "main", Merged: true, Plan: true, PlanPath: "odd/tasks/thing.md"},
			want:     statusMerged,
		},
		{
			name:     "an open row makes the work real",
			evidence: statusEvidence{Checkout: true, Base: "main", Merged: true, OpenRow: true},
			want:     statusMerged,
		},
		{
			name:     "a merged tip with no real work is not merged",
			evidence: statusEvidence{Checkout: true, Base: "main", Merged: true, Workstream: true, DivergenceKnown: true},
			want:     statusWorkstream,
		},
		{
			name:     "a plan is planned",
			evidence: planned,
			want:     statusPlanned,
		},
		{
			name:     "a delta is planned",
			evidence: statusEvidence{Checkout: true, Base: "main", Delta: true, DeltaPath: "odd/changes/thing/specs"},
			want:     statusPlanned,
		},
		{
			name:     "a change with no artifact is open",
			evidence: statusEvidence{Checkout: true, Base: "main", Divergence: []string{"src/app.go"}, DivergenceKnown: true},
			want:     statusOpen,
		},
		{
			name:     "a worktree that is not a change owes nothing",
			evidence: statusEvidence{Checkout: true, Base: "main", Workstream: true, DivergenceKnown: true},
			want:     statusWorkstream,
		},
		{
			name:     "a checkout on the integration branch is the integration line",
			evidence: statusEvidence{Checkout: true, Base: "main", Integration: "main", Workstream: true, DivergenceKnown: true},
			want:     statusIntegration,
		},
		{
			name: "an unanswered divergence cannot separate a change from a workstream",
			// Without the diff git could not answer, "no artifact" is compatible with
			// both remaining phases; reporting either one would be a guess.
			evidence: statusEvidence{Checkout: true, Base: "main"},
			want:     statusUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phase, why := resolveStatusPhase(tc.evidence)
			if phase != tc.want {
				t.Fatalf("phase = %q (%s), want %q", phase, why, tc.want)
			}
			if why == "" {
				t.Fatal("why is empty, want the facts that decided the phase")
			}
		})
	}
}

// L5 item 2: 0 a resolved phase, 2 the facts refuse the requested step, 3 a fact the
// answer needs is unknown — and the unknown wins, because a report that cannot answer
// must not be read as a refusal.
func TestStatusExitCode(t *testing.T) {
	unknown := []statusFact{unknownFact("base", statusSourceBase, "no base ref resolves")}
	cases := []struct {
		name     string
		needed   []statusFact
		blockers []string
		want     int
	}{
		{name: "a resolved phase", want: 0},
		{name: "the facts refuse the step", blockers: []string{"R1: the branch modifies a canonical spec"}, want: 2},
		{name: "a needed fact is unknown", needed: unknown, want: 3},
		{name: "unknown outranks a refusal", needed: unknown, blockers: []string{"R1: refused"}, want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := statusReport{needed: tc.needed, Blockers: tc.blockers}
			if got := statusExitCode(report); got != tc.want {
				t.Fatalf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// statusAdminTree fingerprints <repo>/.git/ai-specs, so "byte-identical" is compared
// instead of asserted in prose.
func statusAdminTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	base := filepath.Join(root, ".git", "ai-specs")
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			tree[rel+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return tree
}

// T7/L5 item 6: status is read-only by contract. A read must not mint the checkout's
// creation token, and the ledger tree must come out byte-identical.
func TestChangesStatusIsReadOnly(t *testing.T) {
	root := workstreamRepo(t)

	// The fresh checkout has no ledger and no creation token: a read must leave both
	// absent, because minting on read would make the report a write.
	before := statusAdminTree(t, root)
	token := filepath.Join(root, ".git", "ai-specs-created")
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatalf("the fixture already has a creation token (stat err = %v)", err)
	}
	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatalf("status minted %s: a read must never write the creation token", token)
	}
	if after := statusAdminTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("ledger tree after a read = %v, want it unchanged (%v)", after, before)
	}

	// With a ledger on disk, the same read leaves it byte-identical.
	if code, _, stderr := runChangesCLI(t, "changes", "workstream", "--root", root,
		"--reason", "audit of the ledger"); code != 0 {
		t.Fatalf("declare = (%d, %q), want it recorded", code, stderr)
	}
	declared := statusAdminTree(t, root)
	if len(declared) == 0 {
		t.Fatal("the declaration wrote no ledger, so the read-only comparison is vacuous")
	}
	for _, args := range [][]string{
		{"changes", "status", "--root", root},
		{"changes", "status", "--root", root, "--json"},
		{"changes", "status", "--root", root, "--explain"},
		{"changes", "status", "--root", root, "--ledger", "plan"},
	} {
		if code, _, _ := runChangesCLI(t, args...); code != 0 {
			t.Fatalf("%v = %d, want a report", args, code)
		}
		if after := statusAdminTree(t, root); !reflect.DeepEqual(declared, after) {
			t.Fatalf("%v changed the ledger tree: %v, want %v", args, after, declared)
		}
	}
}

// S16/T7.2: the workstream phase renders the facts that decided it and the exit that
// turns it into a change, and it owes nothing.
func TestChangesStatusRendersTheWorkstreamPhase(t *testing.T) {
	root := workstreamRepo(t)
	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"phase:    workstream", "no plan", "no delta", "no product-path divergence",
		"when product paths change, this becomes a change", "next:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "blocked:") {
		t.Errorf("report %q asks a workstream for something", stdout)
	}
}

// T7.1: an answered phase reports the facts and the capability lines; the plan tier comes
// from the same verdict the write gate grades, so the two can never disagree.
func TestChangesStatusReportsTheFactsOfAChange(t *testing.T) {
	root := workstreamRepo(t)
	workstreamCommit(t, root, "src/app.go", "feat: product work")

	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
	}
	for _, want := range []string{"phase:    open", "base main", "product paths diverging: src/app.go",
		"plan:     none (no artifacts)", "tracker:  not-applicable", "next:     ai-specs gates premerge"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}
}

// statusJSONFact is one decoded fact of the `--json` report.
type statusJSONFact struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Known  bool   `json:"known"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// statusJSONReport is the decoded `--json` report.
type statusJSONReport struct {
	Phase    string           `json:"phase"`
	Why      string           `json:"why"`
	Next     string           `json:"next"`
	Worktree []statusJSONFact `json:"worktree"`
	Plan     []statusJSONFact `json:"plan"`
	Tracker  []statusJSONFact `json:"tracker"`
	Review   []statusJSONFact `json:"review"`
	Blockers []string         `json:"blockers"`
	Unknown  []string         `json:"unknown"`
}

// statusDecodeReport decodes one `--json` report.
func statusDecodeReport(t *testing.T, stdout string) statusJSONReport {
	t.Helper()
	var report statusJSONReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("status --json printed %q: %v", stdout, err)
	}
	return report
}

// statusFactNamed resolves the named fact of one section.
func statusFactNamed(facts []statusJSONFact, name string) *statusJSONFact {
	for i := range facts {
		if facts[i].Name == name {
			return &facts[i]
		}
	}
	return nil
}

// L5 item 2: a fact git cannot answer is unknown with its reason and exit 3. The base is
// the fact a merge proof and a diff both need.
func TestChangesStatusExitsThreeWhenTheBaseIsUnknown(t *testing.T) {
	root := workstreamRepo(t)
	workstreamCommit(t, root, "src/app.go", "feat: product work")
	workstreamGit(t, root, "branch", "-D", "main")

	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root, "--json")
	if code != 3 {
		t.Fatalf("status = %d (%q), want the unknown-fact exit", code, stderr)
	}
	report := statusDecodeReport(t, stdout)
	if len(report.Unknown) == 0 || !strings.Contains(strings.Join(report.Unknown, "\n"), "base") {
		t.Fatalf("unknown = %v, want it naming the base fact", report.Unknown)
	}
	base := statusFactNamed(report.Worktree, "base")
	if base == nil || base.Known || base.Reason == "" {
		t.Fatalf("base fact = %+v, want an unknown fact with its reason", base)
	}
	if report.Phase != statusUnknown {
		t.Fatalf("phase = %q, want unknown: the phase cannot be derived without a base", report.Phase)
	}
	if report.Next == "" || report.Why == "" {
		t.Fatalf("report = %+v, want why and next on every phase", report)
	}
}

// Outside a git checkout no fact of the worktree can be read: the phase is unknown and
// the exit is 3, with a message that says what git could not name.
func TestChangesStatusOutsideAGitCheckout(t *testing.T) {
	root := t.TempDir()
	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 3 {
		t.Fatalf("status = %d (%q), want the unknown-fact exit", code, stderr)
	}
	if !strings.Contains(stdout, "phase:    unknown") || !strings.Contains(stdout, "git cannot name this checkout") {
		t.Fatalf("report = %q, want an unknown phase naming the checkout", stdout)
	}
	if !strings.Contains(stdout, "it is not a Git repository") {
		t.Fatalf("report = %q, want the facts saying what git could not name", stdout)
	}
}

// L5 item 3: `--ledger` narrows the report to one capability, and the phase stays the
// composite's.
func TestChangesStatusNarrowsToOneCapability(t *testing.T) {
	root := workstreamRepo(t)
	workstreamCommit(t, root, "src/app.go", "feat: product work")

	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root, "--json", "--ledger", "plan")
	if code != 0 {
		t.Fatalf("status = %d (%q), want the narrowed report", code, stderr)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("status --json printed %q: %v", stdout, err)
	}
	for _, absent := range []string{"worktree", "tracker", "review"} {
		if _, ok := report[absent]; ok {
			t.Errorf("report = %v, want no %s section under --ledger plan", report, absent)
		}
	}
	if _, ok := report["plan"]; !ok {
		t.Fatalf("report = %v, want the plan section", report)
	}
	var phase string
	if err := json.Unmarshal(report["phase"], &phase); err != nil || phase != statusOpen {
		t.Fatalf("phase = %q (%v), want the composite phase %q", phase, err, statusOpen)
	}
}

// L5 item 4: `--explain` prints every fact with the source that answered it.
func TestChangesStatusExplainsEveryFactWithItsSource(t *testing.T) {
	root := workstreamRepo(t)
	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root, "--explain")
	if code != 0 || stdout == "" {
		t.Fatalf("status --explain = (%d, %q), want the report on stdout", code, stdout)
	}
	for _, want := range []string{
		"changes status: worktree.common-dir = ",
		"(source: git rev-parse --path-format=absolute --git-common-dir)",
		"changes status: worktree.merge-proof = ",
		"changes status: plan.tier = ",
		"changes status: tracker.row = ",
		"changes status: review.receipt = ",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("explain %q does not contain %q", stderr, want)
		}
	}
}

// The facts refuse the requested step: a change that moved a canonical spec without a
// delta behind it is R1, so status reports it and exits 2.
func TestChangesStatusExitsTwoOnARefutedChange(t *testing.T) {
	root := workstreamRepo(t)
	workstreamCommit(t, root, "src/app.go", "feat: product work")
	workstreamCommit(t, root, "odd/specs/cap/spec.md", "spec: promote without a delta")

	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 2 {
		t.Fatalf("status = %d (%q), want the refusal exit", code, stderr)
	}
	if !strings.Contains(stdout, "blocked:  R1:") {
		t.Fatalf("report = %q, want the R1 blocker", stdout)
	}
}

// The argument contract: usage errors keep the family's 2 and print no report, and
// `--help` prints the verb's usage.
func TestChangesStatusArgumentContract(t *testing.T) {
	root := workstreamRepo(t)
	for _, args := range [][]string{
		{"changes", "status", "--root", root, "--ledger", "everything"},
		{"changes", "status", "--root", root, "--ledger"},
		{"changes", "status", "--root", root, "--rebuild"},
		{"changes", "status", "--root", "--json"},
		{"changes", "status", "--root", root, "extra"},
		{"changes", "status"},
	} {
		code, stdout, _ := runChangesCLI(t, args...)
		if code != 2 || stdout != "" {
			t.Errorf("%v = (%d, %q), want the usage exit and no report", args, code, stdout)
		}
	}
	if code, _, _ := runChangesCLI(t, "changes"); code != 2 {
		t.Errorf("bare changes = %d, want the family's usage exit", code)
	}
	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "changes status") {
		t.Fatalf("--help = (%d, %q, %q), want the usage on stdout", code, stdout, stderr)
	}
	if code, stdout, _ := runChangesCLI(t, "changes", "--help"); code != 0 ||
		!strings.Contains(stdout, "status --root <repo-root>") {
		t.Fatalf("family help = (%d, %q), want it announcing the status verb", code, stdout)
	}
}

// T7.3/S12: the review receipt is consumed from gentle-ai and relayed verbatim, and it
// never decides the exit.
func TestChangesStatusRelaysTheReviewReceipt(t *testing.T) {
	root := workstreamRepo(t)
	bin := filepath.Join(t.TempDir(), "gentle-ai")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"schema\":\"gentle-ai.review-assessment/v1\",\"risk\":\"passive\"," +
		"\"reasons\":[],\"changed_paths\":1,\"changed_lines\":2,\"candidate\":{\"kind\":\"base-diff\"," +
		"\"base_ref\":\"x\",\"consumed\":true},\"review_due\":false,\"review_due_reason\":\"passive\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENTLE_AI_BIN", bin)

	code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("status = (%d, %q, %q), want the receipt reported and the phase resolved", code, stdout, stderr)
	}
	for _, want := range []string{"review:   consumed=true review_due=false (passive) risk=passive"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report %q does not contain %q", stdout, want)
		}
	}

	// An unknown receipt is reported, and it never changes the exit.
	t.Setenv("GENTLE_AI_BIN", filepath.Join(t.TempDir(), "absent"))
	code, stdout, _ = runChangesCLI(t, "changes", "status", "--root", root)
	if code != 0 {
		t.Fatalf("status with no receipt = %d, want the phase's exit: the receipt is reported, not graded", code)
	}
	if !strings.Contains(stdout, "review:   unknown (") {
		t.Fatalf("report = %q, want the unknown receipt with its reason", stdout)
	}
}

// L8-2: the integration line is the repo's own declaration — the worktree-flow
// integration_branch and the resolved base — never a hardcoded name.
func TestStatusIntegrationBranchUsesTheRepoDeclaration(t *testing.T) {
	root := workstreamRepo(t)
	changesWrite(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
		"[recipes.worktree-flow]\nenabled = true\n\n"+
			"[recipes.worktree-flow.config]\nintegration_branch = \"trunk\"\n\n"+
			"[recipes.git-pr-flow.config]\nbase_branch = \"trunk\"\n")
	for _, tc := range []struct {
		name, branch, base, want string
	}{
		{"the declared integration branch", "trunk", "origin/trunk", "trunk"},
		{"a change branch", "feat/thing", "origin/trunk", ""},
		{"a detached checkout", "", "origin/trunk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusIntegrationBranch(root, tc.branch, tc.base); got != tc.want {
				t.Fatalf("statusIntegrationBranch(%q, %q) = %q, want %q", tc.branch, tc.base, got, tc.want)
			}
		})
	}
}

// L8-3: `not-applicable` is only the answer when the project enables no tracker
// recipe. With one enabled and no row, the report names the real state (no item
// bound, card none) and adds no step of its own: what to do about it is T9's.
func TestChangesStatusTrackerNotApplicableOnlyWithoutARecipe(t *testing.T) {
	plain := workstreamRepo(t)
	if _, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", plain); stderr != "" ||
		!strings.Contains(stdout, "tracker:  not-applicable (no tracker recipe enabled)") {
		t.Fatalf("report = (%q, %q), want the no-recipe not-applicable line", stdout, stderr)
	}

	for _, tc := range []struct {
		name    string
		enabled string
		want    string
	}{
		{"enabled", "true", "tracker:  no item bound, card: none"},
		{"disabled", "false", "tracker:  not-applicable (no tracker recipe enabled)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := workstreamRepo(t)
			changesWrite(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
				"[recipes.plan-build-flow]\nenabled = true\n\n[recipes.plan-build-flow.config]\nmode = \"odd\"\n\n"+
					"[recipes.trello-mcp-workflow]\nenabled = "+tc.enabled+"\n")
			workstreamGit(t, root, "add", "-A")
			workstreamGit(t, root, "commit", "-q", "-m", "docs: the tracker recipe")

			code, stdout, stderr := runChangesCLI(t, "changes", "status", "--root", root)
			if code != 0 || stderr != "" {
				t.Fatalf("status = (%d, %q, %q), want a silent report", code, stdout, stderr)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("report %q does not contain %q", stdout, tc.want)
			}
			// The tracker is reported, never graded: the next step stays the plan's.
			for _, line := range strings.Split(stdout, "\n") {
				if strings.HasPrefix(line, "next:") && strings.Contains(line, "tracker") {
					t.Fatalf("next line %q asks for the tracker", line)
				}
			}
			if tc.enabled == "true" && strings.Contains(stdout, "not-applicable") {
				t.Fatalf("report %q calls an enabled tracker not-applicable", stdout)
			}
		})
	}
}
