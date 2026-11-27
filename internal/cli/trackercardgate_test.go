package cli

// Port parity for `ai-specs tracker-card-gate` (cli-served-recipes T6, S3/F2):
// the retired catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh is
// now the CLI verb, so these tests pin the shell host's exit codes and message
// bytes, in both of the modes it served.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/worktree-gate/ledger"
)

// trackerGateFixtureRepo creates a committed repository on main and returns it
// with the ledger common dir and branch identity.
func trackerGateFixtureRepo(t *testing.T) (repo, common, branch string) {
	t.Helper()
	repo = newGateFixture(t)
	commonDir := gitOutput(t, repo, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repo, commonDir)
	}
	return repo, sharedRealPath(commonDir), gitOutput(t, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
}

func sharedRealPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// bindTrackerLedger writes the tracker witness and, when withItem is set, the
// open provider-backed item that satisfies a checkpoint.
func bindTrackerLedger(t *testing.T, common, branch string, withItem bool) {
	t.Helper()
	witness := ledger.WitnessPath(common)
	if err := os.MkdirAll(filepath.Dir(witness), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"v": 1, "capability": "tracker", "state": "bound",
		"recipe_id": "trello-mcp-workflow", "candidates": []string{},
		"written_at": "2026-09-13T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(witness, body, 0o600); err != nil {
		t.Fatal(err)
	}
	store := ledger.Store{V: ledger.StoreVersion}
	if withItem {
		store.OpenItem(ledger.ItemIdentity{CommonDir: common, Branch: branch}, "trello-mcp-workflow",
			time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))
		store.Items[len(store.Items)-1].ItemID = "card-1"
	}
	if err := ledger.SaveStore(ledger.TrackerStorePath(common), store); err != nil {
		t.Fatal(err)
	}
}

// writeTrackerSnapshot publishes the sync snapshot the verb reads (S2).
func writeTrackerSnapshot(t *testing.T, repo, home, mode string) {
	t.Helper()
	snapshot := projectcache.RuntimeSnapshot{
		GateMode: "always", GateScope: "auto", RepoTopology: "auto",
		Protected: "main development", WorktreesDir: ".worktrees",
		IntegrationBranch: "main", TrackerGateMode: mode,
	}
	if _, err := projectcache.WriteRuntimeSnapshot(repo, home, snapshot); err != nil {
		t.Fatal(err)
	}
}

// runTrackerGate drives the verb as the renderers wire it: no arguments, one
// event on stdin, the installing project's root exported.
func runTrackerGate(t *testing.T, home, projectRoot, cwd, input string) (int, string, string) {
	t.Helper()
	t.Setenv("AI_SPECS_PROJECT_ROOT", projectRoot)
	t.Setenv("AI_SPECS_HOOK_AGENT", "claude")
	t.Chdir(cwd)
	var stdout, stderr strings.Builder
	code := Run([]string{"tracker-card-gate"}, home, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// runTrackerGateHost drives the direct host mode the merge skills and this
// repository's AGENTS.md document.
func runTrackerGateHost(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := Run(append([]string{"tracker-card-gate"}, args...), home, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func trackerBlockedMessage(checkpoint, reason string) string {
	return "tracker-card-gate: blocked at " + checkpoint + " — " + reason +
		" (Tracker advisory: reported, not enforced).\n"
}

// trackerLoadStore reads the ledger store the fixture bound.
func trackerLoadStore(t *testing.T, common string) ledger.Store {
	t.Helper()
	store, err := ledger.LoadTrackerStore(ledger.TrackerStorePath(common))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// trackerWriteCard writes one change's ## Tracker card onto disk.
func trackerWriteCard(t *testing.T, repo, change, body string) {
	t.Helper()
	folder := filepath.Join(repo, "openspec", "changes", change)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "proposal.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An event with no path and no gated shell command is never graded: exit 0,
// nothing on either stream.
func TestTrackerCardGateNoCardInEventIsSilent(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	for _, input := range []string{
		`{"tool_name":"Read","cwd":"` + repo + `","tool_input":{"file_path":""}}`,
		`{"tool_name":"Bash","cwd":"` + repo + `","tool_input":{"command":"git status"}}`,
		`not json at all`,
	} {
		code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("event %s = (%d, %q, %q), want silence", input, code, stdout, stderr)
		}
	}
}

// A production path write is graded at apply-start: a bound ledger with no card
// reports the block and still allows the write (advisory, exit 0).
func TestTrackerCardGatePathWriteWithoutCardIsAdvisory(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := trackerBlockedMessage("apply-start", "needs-item")
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	if store := trackerLoadStore(t, common); len(store.Items) != 0 {
		t.Fatalf("a card-less grade wrote %d rows, want 0", len(store.Items))
	}
}

// A satisfied checkpoint (the open card satisfies it) grades allow and says
// nothing, in hook and direct host mode alike.
func TestTrackerCardGateSatisfiedCheckpointIsSilent(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, true)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	input := `{"tool_name":"Edit","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	if code, stdout, stderr := runTrackerGate(t, home, repo, repo, input); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("hook = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	if store := trackerLoadStore(t, common); len(store.Items) != 1 {
		t.Fatalf("a satisfied grade wrote %d rows, want the single existing row", len(store.Items))
	}
	for _, args := range [][]string{
		{"--root", repo, "--checkpoint", "pre-merge"},
		{"--root", repo, "--checkpoint", "archive-close"},
		{"--root", repo, "--stage", "pre-merge"},
		{"--root", repo, "--stage", "pre-archive"},
	} {
		if code, stdout, stderr := runTrackerGateHost(t, home, args...); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("host %v = (%d, %q, %q), want silence", args, code, stdout, stderr)
		}
	}
}

// A violated checkpoint reports the same block line the shell host printed, and
// the direct host mode still exits 0.
func TestTrackerCardGateViolatedCheckpointIsAdvisory(t *testing.T) {
	for _, checkpoint := range []string{"pre-merge", "archive-close"} {
		t.Run(checkpoint, func(t *testing.T) {
			repo, common, branch := trackerGateFixtureRepo(t)
			bindTrackerLedger(t, common, branch, false)
			home := t.TempDir()
			writeTrackerSnapshot(t, repo, home, "always")
			code, stdout, stderr := runTrackerGateHost(t, home, "--root", repo, "--checkpoint", checkpoint)
			want := trackerBlockedMessage(checkpoint, "needs-item")
			if code != 0 || stdout != "" || stderr != want {
				t.Fatalf("host = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
			}
		})
	}
}

// `--stage` is the retained alias: it selects the same checkpoint and conflicts
// with an explicit --checkpoint, exactly as the shell host did.
func TestTrackerCardGateStageAliasAndUsageErrors(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	usage := "usage: ai-specs tracker-card-gate --root <project-root> " +
		"[--checkpoint pre-merge|archive-close | --stage pre-merge|pre-archive] [slug]\n"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing root", args: []string{"--checkpoint", "pre-merge"},
			want: "tracker-card-gate: --root is required for the direct host mode\n" + usage},
		{name: "invalid checkpoint", args: []string{"--root", repo, "--checkpoint", "bogus"},
			want: "tracker-card-gate: invalid --checkpoint 'bogus'\n" + usage},
		{name: "invalid stage", args: []string{"--root", repo, "--stage", "bogus"},
			want: "tracker-card-gate: invalid --stage 'bogus'\n" + usage},
		{name: "checkpoint conflicts with stage", args: []string{"--root", repo, "--stage", "pre-archive", "--checkpoint", "pre-merge"},
			want: "tracker-card-gate: --checkpoint pre-merge conflicts with --stage pre-archive\n" + usage},
		{name: "unknown option", args: []string{"--root", repo, "--bogus"},
			want: "tracker-card-gate: unknown option '--bogus'\n" + usage},
		{name: "unexpected argument", args: []string{"--root", repo, "one", "two"},
			want: "tracker-card-gate: unexpected argument 'two'\n" + usage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runTrackerGateHost(t, home, tc.args...)
			if code != 2 || stdout != "" || stderr != tc.want {
				t.Fatalf("host = (%d, %q, %q), want (2, %q, %q)", code, stdout, stderr, "", tc.want)
			}
		})
	}
}

// The host mode resolves the root through git and fails open when it is not a
// work tree.
func TestTrackerCardGateHostWithoutGitWorkTreeFailsOpen(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	code, stdout, stderr := runTrackerGateHost(t, home, "--root", dir, "--checkpoint", "pre-merge")
	want := "tracker-card-gate: no git work tree at '" + dir + "'; failing open\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("host = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
}

// openspec/** and the agent's own config are never graded, and a path outside
// the repository is not a production write.
func TestTrackerCardGateIgnoresPlanningAndAgentConfig(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	outside := filepath.Join(t.TempDir(), "lib", "app.go")
	for _, target := range []string{
		"openspec/changes/x/proposal.md",
		".claude/settings.json",
		".claude/hooks/guard.sh",
		"docs/readme.md",
		outside,
	} {
		input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"` + target + `"}}`
		if code, stdout, stderr := runTrackerGate(t, home, repo, repo, input); code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("path %s = (%d, %q, %q), want silence", target, code, stdout, stderr)
		}
	}
}

// A `gh pr create` shell command is graded at pr-review.
func TestTrackerCardGateShellPRCreateGradesPrReview(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "always")
	input := `{"tool_name":"Bash","cwd":"` + repo + `","tool_input":{"command":"gh pr create --fill"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := trackerBlockedMessage("pr-review", "needs-item")
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
}

// The gate mode hint comes from the sync snapshot: `off` skips the checkpoint
// before grading, and the shell host discarded the predicate's own skip notice,
// so the verb is silent for the same input.
func TestTrackerCardGateReadsGateModeFromSnapshot(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "off")
	code, stdout, stderr := runTrackerGateHost(t, home, "--root", repo, "--checkpoint", "pre-merge")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("host = (%d, %q, %q), want silence at off", code, stdout, stderr)
	}
}

// apply-start binds the card the change evidence names: one write, one ledger
// row, and one stderr line (S3).
func TestTrackerCardGateApplyStartBindsTheCard(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	trackerWriteCard(t, repo, "demo", "## Tracker\n- **card_id**: `card-1`\n- url: `https://example.test/card/1`\n")

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := "tracker-card-gate: bound card card-1 at apply-start (ledger row recorded).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	store := trackerLoadStore(t, common)
	if len(store.Items) != 1 {
		t.Fatalf("store rows = %d, want 1", len(store.Items))
	}
	item := store.Items[0]
	if item.ItemID != "card-1" || item.URL != "https://example.test/card/1" || item.Identity.Change != "demo" {
		t.Fatalf("bound item = %+v, want card-1 / url / demo", item)
	}
}

// A dormant ledger never binds and never writes a row.
func TestTrackerCardGateApplyStartDormantWritesNothing(t *testing.T) {
	repo, common, _ := trackerGateFixtureRepo(t)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	trackerWriteCard(t, repo, "demo", "## Tracker\ncard_id: card-1\n")

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("gate = (%d, %q, %q), want silence", code, stdout, stderr)
	}
	if _, err := os.Stat(ledger.TrackerStorePath(common)); !os.IsNotExist(err) {
		t.Fatalf("dormant grade wrote a store: %v", err)
	}
}

// A missing card in warn mode is no longer silent: the actionable needs-item
// line names the checkpoint and identity even though the decision is allow
// (S5, T4 regression).
func TestTrackerCardGateApplyStartWithoutCardReportsNeedsItemInWarnMode(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := "tracker-card-gate: needs-item at apply-start for " + branch +
		" — link or create the tracker item, or record the exemption (Tracker advisory).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	if store := trackerLoadStore(t, common); len(store.Items) != 0 {
		t.Fatalf("a card-less grade wrote %d rows, want 0", len(store.Items))
	}
}

// The same needs-item line is reported at the review checkpoints, so a missing
// row is visible where the merge skills grade (S5).
func TestTrackerCardGateReviewCheckpointReportsNeedsItemInWarnMode(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	code, stdout, stderr := runTrackerGateHost(t, home, "--root", repo, "--checkpoint", "pre-merge")
	want := "tracker-card-gate: needs-item at pre-merge for " + branch +
		" — link or create the tracker item, or record the exemption (Tracker advisory).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("host = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
}

// trackerGateMissingTrackerMessage is the strong advisory an existing change
// document without a ## Tracker section produces. Pinned verbatim so a later
// edit cannot silently soften it.
func trackerGateMissingTrackerMessage(checkpoint, identity, document string) string {
	return "tracker-card-gate: missing-tracker-section at " + checkpoint + " for " + identity +
		": " + document + " has no ## Tracker section — add `## Tracker` with `card_id` " +
		"(or record the exemption in tracker.none); reported, never blocking (Tracker advisory).\n"
}

// S13: an ODD plan that exists but has no ## Tracker section reports the strong
// line naming the plan, writes no ledger row and still exits 0.
func TestTrackerCardGateReportsAMissingTrackerSectionInAnODDPlan(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	gitTest(t, repo, "checkout", "-q", "-b", "feat/odd-demo")
	if err := os.MkdirAll(filepath.Join(repo, "odd", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "odd", "tasks", "odd-demo.md"),
		[]byte("# odd-demo\n\n## Specs\n\n- S1 — the plan.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := trackerGateMissingTrackerMessage("apply-start", "feat/odd-demo", "odd/tasks/odd-demo.md")
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	if store := trackerLoadStore(t, common); len(store.Items) != 0 {
		t.Fatalf("a section-less plan wrote %d rows, want 0", len(store.Items))
	}
}

// S13: an OpenSpec change without a ## Tracker section names that document too.
func TestTrackerCardGateReportsAMissingTrackerSectionInAnOpenSpecChange(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	trackerWriteCard(t, repo, "demo", "# demo\n\n## Why\n\nNo tracker yet.\n")

	code, stdout, stderr := runTrackerGateHost(t, home, "--root", repo, "--checkpoint", "pre-merge")
	want := trackerGateMissingTrackerMessage("pre-merge", "demo", "openspec/changes/demo/proposal.md")
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("host = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	if store := trackerLoadStore(t, common); len(store.Items) != 0 {
		t.Fatalf("a section-less change wrote %d rows, want 0", len(store.Items))
	}
}

// PRESERVE S13: an ODD plan with a ## Tracker card still takes the bind path and
// never the missing-section line.
func TestTrackerCardGateApplyStartBindsAnODDPlanCard(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")
	gitTest(t, repo, "checkout", "-q", "-b", "feat/odd-demo")
	if err := os.MkdirAll(filepath.Join(repo, "odd", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "odd", "tasks", "odd-demo.md"),
		[]byte("## Tracker\n- **card_id**: `card-odd`\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"tool_name":"Write","cwd":"` + repo + `","tool_input":{"file_path":"lib/app.go"}}`
	code, stdout, stderr := runTrackerGate(t, home, repo, repo, input)
	want := "tracker-card-gate: bound card card-odd at apply-start (ledger row recorded).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("gate = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
	store := trackerLoadStore(t, common)
	if len(store.Items) != 1 || store.Items[0].ItemID != "card-odd" || store.Items[0].Identity.Change != "" {
		t.Fatalf("store = %+v, want one branch-level card-odd row", store.Items)
	}
}

// PRESERVE S13: a change with no document at all keeps the generic needs-item
// line. TestTrackerCardGateApplyStartWithoutCardReportsNeedsItemInWarnMode pins
// the same bytes; this one adds the host checkpoint.
func TestTrackerCardGateWithoutAnyDocumentKeepsTheGenericLine(t *testing.T) {
	repo, common, branch := trackerGateFixtureRepo(t)
	bindTrackerLedger(t, common, branch, false)
	home := t.TempDir()
	writeTrackerSnapshot(t, repo, home, "warn")

	code, stdout, stderr := runTrackerGateHost(t, home, "--root", repo, "--checkpoint", "pre-merge")
	want := "tracker-card-gate: needs-item at pre-merge for " + branch +
		" — link or create the tracker item, or record the exemption (Tracker advisory).\n"
	if code != 0 || stdout != "" || stderr != want {
		t.Fatalf("host = (%d, %q, %q), want (0, %q, %q)", code, stdout, stderr, "", want)
	}
}
