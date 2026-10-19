package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRouteTableCoversEveryVerb checks the full dispatcher table: the shim
// map (EMPTY since U6e), the native verbs and their aliases, mirroring the
// case statement of bin/ai-specs. The map stays declared with its lookup —
// the strangler mechanism itself — because runShim still execs legacy
// scripts directly (the init TUI hatch, `recipe init`'s route to
// recipe-init.sh, and the dep-gate seams' python3 -c), but no top-level verb
// resolves through the map anymore.
func TestRouteTableCoversEveryVerb(t *testing.T) {
	if len(shims) != 0 {
		t.Errorf("shims map = %v, want EMPTY — U6e retires every verb-level shim", shims)
	}

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
// and `init` routes STRAIGHT to lib/recipe-init.sh: recipe-init.py is
// unported (no U6 slicing row owns it), so that wrapper stays alive, but
// recipe.sh itself has no work left for this binary — the U6e retirement.
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
		// `init` skips recipe.sh and routes straight to its own wrapper —
		// the one recipe subcommand no U6 row ports.
		{[]string{"recipe", "init"}, routeRecipeInit, "recipe-init.sh"},
		{[]string{"recipe", "init", "worktree-flow"}, routeRecipeInit, "recipe-init.sh"},
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

func TestRunVersionIgnoresExtraArgs(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version", "--whatever", "junk"}, home, nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if stdout.String() != "9.9.9\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "9.9.9\n")
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
