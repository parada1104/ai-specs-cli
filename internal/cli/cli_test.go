package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// TestRouteTableCoversEveryVerb checks the full dispatcher table — the native
// verbs and their aliases, mirroring the case statement of bin/ai-specs — plus
// the shim route that survived U7; its last consumer, init.go's
// AI_SPECS_INIT_TUI_PY hatch, was retired in U2b. The verb→script MAP is gone (U7-1 T4) and so is `recipe init`'s shim
// route (U7-2); the exec path itself is not.
func TestRouteTableCoversEveryVerb(t *testing.T) {
	natives := map[string]routeKind{
		"configure-recipes": routeConfigureRecipes,
		"refresh-bundled":   routeRefreshBundled,
		"sync":              routeSync,
		"sync-agent":        routeSyncAgent,
		"upgrade":           routeUpgrade,
		"hub":               routeHub,
		"init":              routeInit,
		"rules-audit":       routeRulesAudit, "version": routeVersion,
		"-v":        routeVersion,
		"--version": routeVersion,
		"help":      routeHelp,
		"-h":        routeHelp,
		"--help":    routeHelp,
	}
	for verb, want := range natives {
		if got := Route([]string{verb}).kind; got != want {
			t.Errorf("verb %q: kind = %v, want %v", verb, got, want)
		}
	}

	// `doctor` is native now (card [Go 08]): it must resolve to routeDoctor and
	// must not be served by the legacy doctor.sh shim.
	if got := Route([]string{"doctor"}).kind; got != routeDoctor {
		t.Errorf("verb %q: kind = %v, want %v", "doctor", got, routeDoctor)
	}

	// `rules-audit` is native now (card [Go 08]): it must resolve to
	// routeRulesAudit and must not be served by the legacy rules-audit.sh shim.
	if got := Route([]string{"rules-audit"}).kind; got != routeRulesAudit {
		t.Errorf("verb %q: kind = %v, want %v", "rules-audit", got, routeRulesAudit)
	}

	// `upgrade` is native now (GO-20 U1a): it must resolve to routeUpgrade and
	// must not be served by the legacy upgrade.sh shim. The engine's exit codes
	// and abort messages stay FROZEN (parity contract section 2).
	if got := Route([]string{"upgrade"}).kind; got != routeUpgrade {
		t.Errorf("verb %q: kind = %v, want %v", "upgrade", got, routeUpgrade)
	}
}

func TestRouteBareInvocationRewritesToHub(t *testing.T) {
	r := Route(nil)
	if r.kind != routeHub {
		t.Fatalf("bare invocation must route to hub, got kind=%v", r.kind)
	}
}

// TestRouteSkillsListIsNativeBeforeTheShim pins the GO-20 U5a/U5b/U5c
// subcommand interceptions: `skills list`, `skills remove` and `skills add`
// resolve native, and the REMAINDER of lib/skills.sh's case statement (the
// bare `skills`, `--help`/`-h`/`help` and unknown subcommands) resolves to
// the native dispatcher route — nothing in the skills surface reaches the
// skills.sh shim anymore (U5c). The add-dep alias carries its own label,
// exactly the env var bin/ai-specs sets for its `add-dep)` arm.
func TestRouteSkillsListIsNativeBeforeTheShim(t *testing.T) {
	cases := []struct {
		args      []string
		want      routeKind
		invokedAs string
	}{
		{[]string{"skills", "list"}, routeSkillsList, ""},
		{[]string{"skills", "list", "--help"}, routeSkillsList, ""},
		{[]string{"skills", "list", "."}, routeSkillsList, ""},
		// U5b: `skills remove` is intercepted before the dispatcher too.
		{[]string{"skills", "remove"}, routeSkillsRemove, ""},
		{[]string{"skills", "remove", "some-id"}, routeSkillsRemove, ""},
		{[]string{"skills", "remove", "some-id", "."}, routeSkillsRemove, ""},
		{[]string{"skills", "remove", "--help"}, routeSkillsRemove, ""},
		// U5c: the add verb and its alias are native, each with its label.
		{[]string{"skills", "add"}, routeSkillsAdd, "ai-specs skills add"},
		{[]string{"skills", "add", "https://example.com/r.git"}, routeSkillsAdd, "ai-specs skills add"},
		{[]string{"skills", "add", "--help"}, routeSkillsAdd, "ai-specs skills add"},
		{[]string{"add-dep"}, routeAddDep, "ai-specs add-dep"},
		{[]string{"add-dep", "https://example.com/r.git"}, routeAddDep, "ai-specs add-dep"},
		// The dispatcher remainder: every non-add/list/remove arm of
		// lib/skills.sh's case statement, resolved natively (U5c).
		{[]string{"skills"}, routeSkills, ""},
		{[]string{"skills", "--help"}, routeSkills, ""},
		{[]string{"skills", "-h"}, routeSkills, ""},
		{[]string{"skills", "help"}, routeSkills, ""},
		{[]string{"skills", "bogus"}, routeSkills, ""},
		{[]string{"skills", "listx"}, routeSkills, ""},
		{[]string{"skills", "-list"}, routeSkills, ""},
	}
	for _, tc := range cases {
		r := Route(tc.args)
		if r.kind != tc.want {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, r.kind, tc.want)
			continue
		}
		if r.invokedAs != tc.invokedAs {
			t.Errorf("Route(%q) invokedAs = %q, want %q", tc.args, r.invokedAs, tc.invokedAs)
		}
	}
}

// TestRouteRecipeListIsNativeBeforeTheShim pins the GO-20 U6a/U6b/U6c/U6d-1/U6e
// subcommand interceptions: `recipe list`, `recipe add`, `recipe remove` and
// `recipe configure` resolve native, the dispatcher's REMAINING arms of
// lib/recipe.sh's case statement — the bare `recipe`, `--help`/`-h`/`help`
// and unknown subcommands — resolve to the native dispatcher route (U6e, S4),
// and since GO-20 U7-2 `init` is native too: the wrapper grammar and the
// brief moved to internal/recipe, so no recipe arm reaches a shell script.
// `recipe add` has NO delegation left either (U6e flipped its tail onto the
// native wizard, the dep-gate seam and the env offer): Route cannot see the
// streams, and the tail's wiring is pinned by
// TestRecipeAddTailIsNativeOnATerminal.
func TestRouteRecipeListIsNativeBeforeTheShim(t *testing.T) {
	cases := []struct {
		args   []string
		want   routeKind
		script string
	}{
		// The intercepted subcommands (native).
		{[]string{"recipe", "list"}, routeRecipeList, ""},
		{[]string{"recipe", "list", "."}, routeRecipeList, ""},
		{[]string{"recipe", "list", "--help"}, routeRecipeList, ""},
		{[]string{"recipe", "list", "--", "."}, routeRecipeList, ""},
		{[]string{"recipe", "add"}, routeRecipeAdd, ""},
		{[]string{"recipe", "add", "worktree-flow"}, routeRecipeAdd, ""},
		{[]string{"recipe", "add", "worktree-flow", "."}, routeRecipeAdd, ""},
		{[]string{"recipe", "add", "--help"}, routeRecipeAdd, ""},
		// U6c: `recipe remove` is intercepted before the dispatcher too (the
		// script has no interactive branch, so there is no TTY seam like add's).
		{[]string{"recipe", "remove"}, routeRecipeRemove, ""},
		{[]string{"recipe", "remove", "worktree-flow"}, routeRecipeRemove, ""},
		{[]string{"recipe", "remove", "worktree-flow", "."}, routeRecipeRemove, ""},
		{[]string{"recipe", "remove", "--help"}, routeRecipeRemove, ""},
		{[]string{"recipe", "remove", "--", "worktree-flow"}, routeRecipeRemove, ""},
		// U6d-1: `recipe configure` is intercepted before the dispatcher too
		// (recipe.sh's configure arm has no TTY branch, so there is no seam).
		{[]string{"recipe", "configure"}, routeRecipeConfigure, ""},
		{[]string{"recipe", "configure", "worktree-flow"}, routeRecipeConfigure, ""},
		{[]string{"recipe", "configure", "worktree-flow", "--inspect"}, routeRecipeConfigure, ""},
		{[]string{"recipe", "configure", "--help"}, routeRecipeConfigure, ""},
		// The dispatcher remainder resolves natively (U6e): usage on stdout,
		// bare on stderr, the unknown pair — never recipe.sh.
		{[]string{"recipe"}, routeRecipe, ""},
		{[]string{"recipe", "--help"}, routeRecipe, ""},
		{[]string{"recipe", "-h"}, routeRecipe, ""},
		{[]string{"recipe", "help"}, routeRecipe, ""},
		{[]string{"recipe", "bogus"}, routeRecipe, ""},
		{[]string{"recipe", "listx"}, routeRecipe, ""},
		{[]string{"recipe", "-list"}, routeRecipe, ""},
		{[]string{"recipe", "List"}, routeRecipe, ""},
		// `init` is native since U7-2: the wrapper grammar and the brief both
		// live in internal/recipe, so no script is exec'd.
		{[]string{"recipe", "init"}, routeRecipeInitVerb, ""},
		{[]string{"recipe", "init", "worktree-flow"}, routeRecipeInitVerb, ""},
		{[]string{"recipe", "list", "a", "b"}, routeRecipeList, ""},
	}
	for _, tc := range cases {
		r := Route(tc.args)
		if r.kind != tc.want {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, r.kind, tc.want)
			continue
		}
		if r.script != tc.script {
			t.Errorf("Route(%q) script = %q, want %q", tc.args, r.script, tc.script)
		}
	}
}

// TestRouteConfigureRecipesIsNative pins U6e's other retirement: the
// `configure-recipes` verb resolves native, never the recipe-config.sh shim
// (its whole shell wrapper is the verb's argument contract, now native).
func TestRouteConfigureRecipesIsNative(t *testing.T) {
	cases := [][]string{
		{"configure-recipes"},
		{"configure-recipes", "."},
		{"configure-recipes", "--help"},
		{"configure-recipes", "--", "."},
	}
	for _, args := range cases {
		r := Route(args)
		if r.kind != routeConfigureRecipes {
			t.Errorf("Route(%q) kind = %v, want %v", args, r.kind, routeConfigureRecipes)
		}
	}
}

// TestRunBareInvocationRoutesToHub executes the bare-invocation path
// end-to-end: Route(nil) rewrites to hub and the NATIVE hub (GO-20 U3a) must
// report an uninitialized non-TTY target itself: exit 2 with the two frozen
// stderr lines, no Python launch and no dependency on one.
func TestRunBareInvocationRoutesToHub(t *testing.T) {
	home, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // uninitialized project: pre-guard fires before Python
	var stdout, stderr bytes.Buffer
	code := Run(nil, home, nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2 (uninitialized + non-TTY pre-guard)", code)
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "no ai-specs project at") {
		t.Errorf("stderr = %q, want the hub's uninitialized message", stderr.String())
	}
}

func TestWorktreeGateRoutePinnedContracts(t *testing.T) {
	t.Setenv("WORKTREE_GATE_MODE", "")
	t.Setenv("WORKTREE_GATE_SCOPE", "")
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("basic", func(t *testing.T) {
		repo := t.TempDir()
		for _, tc := range []struct {
			name string
			args []string
			code int
			out  string
			err  string
		}{
			{"version", []string{"--version"}, 0, "dev\n", ""},
			{"selftest", []string{"--selftest"}, 0, "ok\n", ""},
			{"help", []string{"--help"}, 0, "", "usage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\nworktree-gate: warning: flag: help requested (failing open)\n"},
			{"explain", []string{"--explain"}, 0, "{\"mode\":\"\",\"tool\":\"\",\"cwd\":\"\",\"gate_mode\":\"\",\"gate_scope\":\"\",\"repo_topology\":\"\",\"candidates\":null,\"cwd_source\":\"\",\"decision\":\"allow\",\"branch\":\"\",\"reason\":\"no-blocking-candidate\"}\n", ""},
			{"topology-fail-closed", []string{"--resolve-central-root"}, 1, "", "worktree-gate: resolve-central-root: unproven submodule topology\n"},
			{"gate-parse-fail-open", []string{"--bad-gate-flag"}, 0, "", "flag provided but not defined: -bad-gate-flag\nusage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\nworktree-gate: warning: flag provided but not defined: -bad-gate-flag (failing open)\n"},
			{"cleanup-parse-fail-closed", []string{"--cleanup", "--bad-cleanup-flag"}, 2, "", "flag provided but not defined: -bad-cleanup-flag\nusage: worktree-gate [--gate-mode M] [--gate-scope S] [--repo-topology T] [--protected \"b1 b2\"] [--version] [--selftest] [--explain] [--resolve-central-root]\nworktree-cleanup: flag provided but not defined: -bad-cleanup-flag (refusing destructive run)\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				code, out, errOut := runGateCLI(t, repo, home, "", tc.args)
				assertGatePinned(t, repo, code, out, errOut, tc.code, tc.out, tc.err)
			})
		}
	})

	gateRepo := newGateFixture(t)
	eventCases := []struct {
		name, file string
		args       []string
		code       int
		out, err   string
	}{
		{"event-allow", filepath.Join(t.TempDir(), "outside", "x"), []string{"--gate-mode", "always", "--gate-scope", "auto", "--repo-topology", "standalone", "--protected", "main development"}, 0, "", ""},
		{"event-block", "README.md", []string{"--gate-mode", "always", "--gate-scope", "auto", "--repo-topology", "auto", "--protected", "main development"}, 2, "", "worktree-gate: refusing to Write 'README.md' on protected branch 'main' in the main worktree (<repo>). Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write.\n"},
	}
	for _, tc := range eventCases {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"tool_name":"Write","cwd":"` + gateRepo + `","tool_input":{"file_path":"` + tc.file + `"}}`
			code, out, errOut := runGateCLI(t, gateRepo, home, input, tc.args)
			assertGatePinned(t, gateRepo, code, out, errOut, tc.code, tc.out, tc.err)
		})
	}

	ledgerRepo := newGateFixture(t)
	commonRel := gitOutput(t, ledgerRepo, "rev-parse", "--git-common-dir")
	common := commonRel
	if !filepath.IsAbs(common) {
		common = filepath.Join(ledgerRepo, commonRel)
	}
	witness := filepath.Join(common, "ai-specs", "ledger", "witness.json")
	if err := os.MkdirAll(filepath.Dir(witness), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(witness, []byte(`{"v":1,"capability":"tracker","state":"bound","recipe_id":"trello-mcp-workflow","candidates":[],"written_at":"2026-09-13T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	storePath := ledger.TrackerStorePath(common)
	identity := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: ledgerRepo})
	ident := ledger.ItemIdentity{CommonDir: identity.CommonDir, Branch: identity.Branch}
	// One open row per ledger (S3): the lifecycle in the worktree ledger, the
	// provider record and its decisions in the tracker ledger, joined by the id
	// both rows carry. The CLI still emits the joined item with its status.
	locs := ledger.LedgersAtStore(storePath)
	if _, err := locs.Worktree.SaveWorktree(ledger.WorktreeStore{
		V: ledger.StoreVersion,
		Items: []ledger.WorktreeItem{{
			ID: "fixture-item", Identity: ident, Status: ledger.StatusOpen,
			Decisions: []ledger.WorktreeDecision{},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := locs.Tracker.Save(ledger.Store{
		V: ledger.StoreVersion,
		Items: []ledger.Item{{
			ID: "fixture-item", Identity: ident, ItemID: "card-local", ProviderID: "trello-mcp-workflow",
			Decisions: []ledger.Decision{
				{At: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), Kind: ledger.DecisionOpen},
				{At: "2026-09-13T12:30:00Z", Checkpoint: "pre-merge", Kind: ledger.DecisionAdjudicate, Choice: "local"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(evidence, []byte(`{"remote":"card-remote"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ledgerArgs := []string{"--ledger", "--checkpoint", "pre-merge", "--project-root", ledgerRepo, "--ledger-mode", "ask", "--witness", witness, "--store", storePath, "--evidence", evidence}
	code, out, errOut := runGateCLI(t, ledgerRepo, home, "", ledgerArgs)
	assertGatePinned(t, ledgerRepo, code, out, errOut, 0, `{"capability":"tracker","active":true,"checkpoint":"pre-merge","mode":"ask","decision":"allow","reason":"adjudicated","identity":{"common_dir":"<repo>/.git","branch":"main","change":null,"key":"<repo>/.git\u001fmain"},"item":{"id":"fixture-item","identity":{"common_dir":"<repo>/.git","branch":"main","change":""},"status":"open","item_id":"card-local","provider_id":"trello-mcp-workflow","native_type":"","url":"","state":"","provider":null,"exemption":"","conflict":null,"decisions":[{"at":"2026-09-13T12:00:00Z","checkpoint":"","kind":"open","choice":"","note":""},{"at":"2026-09-13T12:30:00Z","checkpoint":"pre-merge","kind":"adjudicate","choice":"local","note":""}]},"conflict":null,"prompt":null,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}`+"\n", "")

	cleanupRepo := newGateFixture(t)
	enableWorktreeFlow(t, cleanupRepo)
	wt := filepath.Join(cleanupRepo, ".worktrees", "feat-unmerged")
	gitTest(t, cleanupRepo, "worktree", "add", "-b", "feat-unmerged", wt)
	if err := os.WriteFile(filepath.Join(wt, "change"), []byte("unmerged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, wt, "add", ".")
	gitTest(t, wt, "commit", "-m", "unmerged")
	cleanupArgs := []string{"--cleanup", "--dir", ".worktrees", "--base", "main", "--integration-branch", "integration", "--topology", "standalone", "--dry-run"}
	code, out, errOut = runGateCLI(t, cleanupRepo, home, "", cleanupArgs)
	assertGatePinned(t, cleanupRepo, code, out, errOut, 0, "skipped feat-unmerged (unmerged)\n", "")

	dirtyRepo := newGateFixture(t)
	enableWorktreeFlow(t, dirtyRepo)
	dirtyWT := filepath.Join(dirtyRepo, ".worktrees", "feat-dirty")
	gitTest(t, dirtyRepo, "worktree", "add", "-b", "feat-dirty", dirtyWT)
	if err := os.WriteFile(filepath.Join(dirtyWT, "landed"), []byte("landed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, dirtyWT, "add", ".")
	gitTest(t, dirtyWT, "commit", "-m", "landed")
	gitTest(t, dirtyRepo, "merge", "--no-ff", "-m", "merge dirty", "feat-dirty")
	if err := os.WriteFile(filepath.Join(dirtyWT, "dirty"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runGateCLI(t, dirtyRepo, home, "", cleanupArgs)
	assertGatePinned(t, dirtyRepo, code, out, errOut, 0, "skipped feat-dirty (dirty)\n", "")

	detachedRepo := newGateFixture(t)
	enableWorktreeFlow(t, detachedRepo)
	detachedWT := filepath.Join(detachedRepo, ".worktrees", "detached")
	if err := os.MkdirAll(filepath.Dir(detachedWT), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, detachedRepo, "worktree", "add", "--detach", detachedWT, "main")
	code, out, errOut = runGateCLI(t, detachedRepo, home, "", cleanupArgs)
	assertGatePinned(t, detachedRepo, code, out, errOut, 0, "skipped detached (detached)\n", "")
}

// enableWorktreeFlow authorizes destructive cleanup in a fixture repo: cleanup
// runs only where the main checkout's manifest enables the recipe.
func enableWorktreeFlow(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte("[recipes.worktree-flow]\nenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newGateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitTest(t, root, "init", "-b", "main")
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	gitTest(t, root, "config", "user.name", "test")
	gitTest(t, root, "commit", "--allow-empty", "-m", "initial")
	return root
}

func normalizeGateOutput(repo, output string) string {
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		output = strings.ReplaceAll(output, resolved, "<repo>")
	}
	return strings.ReplaceAll(output, repo, "<repo>")
}

func assertGatePinned(t *testing.T, repo string, code int, stdout, stderr string, wantCode int, wantStdout, wantStderr string) {
	t.Helper()
	stdout = normalizeGateOutput(repo, stdout)
	stderr = normalizeGateOutput(repo, stderr)
	if code != wantCode || stdout != wantStdout || stderr != wantStderr {
		t.Fatalf("got (%d,%q,%q); want (%d,%q,%q)", code, stdout, stderr, wantCode, wantStdout, wantStderr)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

func gitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func TestExistingVersionVerbUnchanged(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("0.25.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"version"}, home, nil, &stdout, &stderr); code != 0 || stdout.String() != "0.25.2\n" || stderr.Len() != 0 {
		t.Fatalf("version = (%d,%q,%q)", code, stdout.String(), stderr.String())
	}
}

func runGateCLI(t *testing.T, cwd, home, input string, args []string) (int, string, string) {
	t.Helper()
	t.Chdir(cwd)
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"worktree-gate"}, args...), home, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRouteUnknownCommand(t *testing.T) {
	r := Route([]string{"bogus", "extra"})
	if r.kind != routeUnknown {
		t.Fatalf("unknown command: kind = %v, want unknown", r.kind)
	}
}

func TestRunUnknownCommandStderrAndExit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"bogus"}, "/tmp", nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	want := "ai-specs: unknown command 'bogus'\nRun 'ai-specs help' for usage.\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunVersionReadsHomeVersionFile(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("0.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version"}, home, nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if stdout.String() != "0.24.0\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "0.24.0\n")
	}
}

func TestRunVersionMissingFilePrintsUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if stdout.String() != "unknown\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "unknown\n")
	}
}

func TestRunVersionRejectsExtraArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version", "--whatever", "junk"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "ERROR: unexpected positional argument: --whatever\nRun 'ai-specs --help' for usage.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunHelpUnknownTopic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "foo"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "ERROR: unknown help topic: foo\nRun 'ai-specs help' for usage.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunHelpKnownTopicPrintsEmbeddedBytes(t *testing.T) {
	embedded, err := os.ReadFile(filepath.Join("help.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "sync"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 0 || !bytes.Equal(stdout.Bytes(), embedded) || stderr.Len() != 0 {
		t.Fatalf("Run(help sync) = (%d, stdout %d bytes, stderr %q), want (0, embedded help, empty)", code, stdout.Len(), stderr.String())
	}
}

func TestRunHelpPrintsEmbeddedBytes(t *testing.T) {
	embedded, err := os.ReadFile(filepath.Join("help.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !bytes.Equal(stdout.Bytes(), embedded) {
		t.Errorf("help output differs from embedded help.txt (%d vs %d bytes)",
			stdout.Len(), len(embedded))
	}
}

func TestRunHelpIgnoresExtraArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help", "extra"}, t.TempDir(), nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout.String(), "ai-specs — declarative") {
		t.Errorf("stdout = %q, want embedded help prefix", stdout.String())
	}
}
