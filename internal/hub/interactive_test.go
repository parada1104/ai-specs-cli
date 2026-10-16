package hub

// GO-20 U3b-2 (T4c): the native interactive hub, driven entirely through the
// tui.KeySource seam and a stub <home>/bin/ai-specs launcher. Every expected
// string, label, order and branch comes from lib/_internal/hub.py; the stub makes
// a delegation observable as one line in <home>/calls.log, which is what lets a
// test prove the Python bridge is gone and the menu still works.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/tui"
)

// ── harness ──────────────────────────────────────────────────────────────────

// scriptedKeys is a tui.KeySource over an ordered script; once exhausted it
// reports io.EOF, which every prompt reads as an abort.
type scriptedKeys struct {
	keys []tui.Key
	i    int
}

func (s *scriptedKeys) NextKey() (tui.Key, error) {
	if s.i >= len(s.keys) {
		return 0, io.EOF
	}
	k := s.keys[s.i]
	s.i++
	return k, nil
}

// selectIndex presses Down i times then Enter — the cursor starts on entry 0.
func selectIndex(i int) []tui.Key {
	return append(downs(i), tui.KeyEnter)
}

// downs is i Down presses with no confirmation.
func downs(i int) []tui.Key {
	keys := make([]tui.Key, 0, i)
	for n := 0; n < i; n++ {
		keys = append(keys, tui.KeyDown)
	}
	return keys
}

func keyseq(seqs ...[]tui.Key) []tui.Key {
	var out []tui.Key
	for _, s := range seqs {
		out = append(out, s...)
	}
	return out
}

// Menu indices of hub.py's _MENU, which the tests navigate by.
const (
	menuDoctor  = 1
	menuAgents  = 2
	menuSkills  = 3
	menuRecipes = 4
	menuVersion = 8
	menuQuit    = 11
)

// hubStubScript is the executable hub.py's DelegateRunner would call. It records
// its argv, optionally materialises the manifest an init would create, and exits
// with $STUB_EXIT (default 0).
const hubStubScript = `#!/bin/sh
echo "$0" "$@" >> "$AI_SPECS_HOME/calls.log"
if [ "$1" = "init" ] && [ -n "$STUB_INIT_CREATES" ]; then
  mkdir -p "$2/ai-specs"
  printf '[project]\nname = "t"\n' > "$2/ai-specs/ai-specs.toml"
fi
exit "${STUB_EXIT:-0}"
`

// hubHome writes the stub launcher. It deliberately writes NO
// lib/_internal/hub.py, so any test that reaches the menu proves the bridge is
// gone.
func hubHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ai-specs"), []byte(hubStubScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func hubCLI(home string) string {
	return filepath.Join(home, "bin", "ai-specs")
}

func calls(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "calls.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func hubTargetWithManifest(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), body)
	return root
}

func initializedHubTarget(t *testing.T) string {
	return hubTargetWithManifest(t, "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"claude\"]\n")
}

func runHubInteractive(t *testing.T, root, home string, keys []tui.Key) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := RunHub(Options{Target: root, Home: home, TTY: true, Keys: &scriptedKeys{keys: keys}}, nil, &out, &errb)
	return rc, out.String(), errb.String()
}

func writeCatalogRecipe(t *testing.T, home, id, name, version string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "catalog", "recipes", id, "recipe.toml"),
		"[recipe]\nid = \""+id+"\"\nname = \""+name+"\"\ndescription = \"d\"\nversion = \""+version+"\"\n")
}

func writeLocalSkill(t *testing.T, root, id, desc string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "ai-specs", "skills", id, "SKILL.md"),
		"---\nname: "+id+"\ndescription: "+desc+"\n---\n\nbody\n")
}

// ── the loop ─────────────────────────────────────────────────────────────────

// TestInteractiveQuitViaMenu pins the menu round-trip: the panel is drawn, the 12
// frozen labels are offered, Quit exits 0 and delegates nothing.
func TestInteractiveQuitViaMenu(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	rc, out, errb := runHubInteractive(t, root, home, selectIndex(menuQuit))
	if rc != 0 {
		t.Fatalf("rc = %d, want 0 (stderr: %s)", rc, errb)
	}
	if !strings.Contains(out, "What do you want to do?") {
		t.Errorf("menu prompt missing:\n%s", out)
	}
	if !strings.Contains(out, "ai-specs") || !strings.Contains(out, "Summary:") {
		t.Errorf("status panel missing:\n%s", out)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("quit delegated %v, want nothing", got)
	}
}

// TestInteractiveAbortedMenuPromptIsQuit pins hub.py's `None -> Action.QUIT`.
func TestInteractiveAbortedMenuPromptIsQuit(t *testing.T) {
	cases := map[string][]tui.Key{
		"escape": {tui.KeyEscape},
		"ctrl-c": {tui.KeyCtrlC},
		"eof":    nil,
	}
	for name, keys := range cases {
		t.Run(name, func(t *testing.T) {
			root := initializedHubTarget(t)
			home := hubHome(t)
			rc, _, _ := runHubInteractive(t, root, home, keys)
			if rc != 0 {
				t.Fatalf("rc = %d, want 0", rc)
			}
			if got := calls(t, home); len(got) != 0 {
				t.Errorf("aborted menu delegated %v", got)
			}
		})
	}
}

// TestInteractiveVersionPrintsAndDoesNotPause pins the one menu entry that stays
// in the loop: it prints the bare version and re-prompts without a pause.
func TestInteractiveVersionPrintsAndDoesNotPause(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	writeFile(t, filepath.Join(home, "VERSION"), "9.9.9\n")

	keys := keyseq(selectIndex(menuVersion), selectIndex(menuQuit))
	rc, out, _ := runHubInteractive(t, root, home, keys)
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if !strings.Contains(out, "\n9.9.9\n") {
		t.Errorf("version was not printed on its own line:\n%s", out)
	}
	if strings.Contains(out, "Press Enter to return…") {
		t.Errorf("Version must not pause:\n%s", out)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("Version delegated %v", got)
	}
}

// TestInteractiveDelegatedActionRecordsArgvAndDone pins the delegated branch: the
// stub child receives DelegateRunner's argv verbatim, then the hub prints ✓ done
// and pauses.
func TestInteractiveDelegatedActionRecordsArgvAndDone(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	keys := keyseq(selectIndex(menuDoctor), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	rc, out, _ := runHubInteractive(t, root, home, keys)
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	want := hubCLI(home) + " doctor " + root
	got := calls(t, home)
	if len(got) != 1 || got[0] != want {
		t.Errorf("calls = %v, want [%s]", got, want)
	}
	if !strings.Contains(out, "✓ done") {
		t.Errorf("missing ✓ done:\n%s", out)
	}
}

// TestInteractiveNonZeroExitPrintsExitedLine pins the ✗ spelling.
func TestInteractiveNonZeroExitPrintsExitedLine(t *testing.T) {
	t.Setenv("STUB_EXIT", "7")
	root := initializedHubTarget(t)
	home := hubHome(t)

	keys := keyseq(selectIndex(menuDoctor), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)
	if !strings.Contains(out, "✗ exited 7") {
		t.Errorf("missing ✗ exited 7:\n%s", out)
	}
	if strings.Contains(out, "✓ done") {
		t.Errorf("unexpected ✓ done:\n%s", out)
	}
}

// TestInteractivePauseEOFEndsLoopWithZero pins hub.py's `if not pause(): return 0`.
func TestInteractivePauseEOFEndsLoopWithZero(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	rc, _, _ := runHubInteractive(t, root, home, selectIndex(menuDoctor))
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if got := calls(t, home); len(got) != 1 {
		t.Errorf("calls = %v, want the doctor delegation", got)
	}
}

// TestInteractiveRunHubWithInjectedKeysIsFullyNative pins the U3b deliverable: a
// TTY hub with injected keys drives the Go loop (the stub records the delegation)
// and needs no Python child — the fixture has no lib/_internal/hub.py.
func TestInteractiveRunHubWithInjectedKeysIsFullyNative(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	if _, err := os.Stat(filepath.Join(home, "lib", "_internal", "hub.py")); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has a python bridge: %v", err)
	}

	keys := keyseq(selectIndex(menuDoctor), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	rc, _, errb := runHubInteractive(t, root, home, keys)
	if rc != 0 {
		t.Fatalf("rc = %d, want the loop's code 0 (stderr: %s)", rc, errb)
	}
	if got := calls(t, home); len(got) != 1 {
		t.Errorf("calls = %v, want the Go loop's delegation", got)
	}
}

// ── recipes submenu ──────────────────────────────────────────────────────────

const (
	recipesList      = 0
	recipesAdd       = 1
	recipesRemove    = 2
	recipesConfigure = 3
)

func TestRecipesListDelegatesToList(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesList), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	want := hubCLI(home) + " recipe list " + root
	if got := calls(t, home); len(got) != 1 || got[0] != want {
		t.Errorf("calls = %v, want [%s]", got, want)
	}
	if !strings.Contains(out, "✓ done") {
		t.Errorf("missing ✓ done:\n%s", out)
	}
}

func TestRecipesAddUsesAvailablePickerAndDelegates(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	writeCatalogRecipe(t, home, "demo", "Demo", "1.2.3")

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesAdd), selectIndex(0), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	want := hubCLI(home) + " recipe add demo " + root
	if got := calls(t, home); len(got) != 1 || got[0] != want {
		t.Errorf("calls = %v, want [%s]", got, want)
	}
	if !strings.Contains(out, "Recipe to add:") {
		t.Errorf("missing add picker:\n%s", out)
	}
	if !strings.Contains(out, "Demo (demo)  v1.2.3") {
		t.Errorf("missing add label:\n%s", out)
	}
}

func TestRecipesConfigureDelegatesToConfigureRecipes(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesConfigure), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, _, _ = runHubInteractive(t, root, home, keys)

	want := hubCLI(home) + " configure-recipes " + root
	if got := calls(t, home); len(got) != 1 || got[0] != want {
		t.Errorf("calls = %v, want [%s]", got, want)
	}
}

func TestRecipesRemoveUsesInstalledPickerAndDelegates(t *testing.T) {
	root := hubTargetWithManifest(t, "[project]\nname = \"t\"\n\n[recipes.demo]\nenabled = true\n")
	home := hubHome(t)
	writeCatalogRecipe(t, home, "demo", "Demo", "1.2.3")

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesRemove), selectIndex(0), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	want := hubCLI(home) + " recipe remove demo " + root
	if got := calls(t, home); len(got) != 1 || got[0] != want {
		t.Errorf("calls = %v, want [%s]", got, want)
	}
	if !strings.Contains(out, "Demo (demo)  [installed]") {
		t.Errorf("missing remove label:\n%s", out)
	}
}

// TestRecipesPickerAbortReturnsToMenu pins `if rid is None: return None`: an
// aborted leaf picker re-prompts the menu and delegates nothing.
func TestRecipesPickerAbortReturnsToMenu(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	writeCatalogRecipe(t, home, "demo", "Demo", "1.2.3")

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesAdd), []tui.Key{tui.KeyEscape}, selectIndex(menuQuit))
	rc, out, _ := runHubInteractive(t, root, home, keys)
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("aborted picker delegated %v", got)
	}
	// The picker was offered once, and the loop re-rendered its panel (once per
	// iteration) after the abort, which is the "return to menu" contract.
	if n := strings.Count(out, "Recipe to add:"); n != 1 {
		t.Errorf("add picker shown %d times, want 1", n)
	}
	if n := strings.Count(out, "\u256d\u2500 ai-specs "); n != 2 {
		t.Errorf("panel rendered %d times, want 2 (menu before the picker, menu after the abort)", n)
	}
}

func TestRecipesAddWithEmptyCatalogPauses(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)

	keys := keyseq(selectIndex(menuRecipes), selectIndex(recipesAdd), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	if !strings.Contains(out, "No catalog recipes available to add.") {
		t.Errorf("missing empty-catalog line:\n%s", out)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("delegated %v", got)
	}
}

// ── skills submenu ───────────────────────────────────────────────────────────

const (
	skillsList    = 0
	skillsInspect = 1
)

// TestSkillsListPrintsFourBuckets pins the bucket order/titles and the entry
// format `  <id>  — <desc>`.
func TestSkillsListPrintsFourBuckets(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	writeLocalSkill(t, root, "foo", "Does foo")

	keys := keyseq(selectIndex(menuSkills), selectIndex(skillsList), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	for _, header := range []string{
		"── Bundled (CLI-shipped) ──",
		"── Local / vendored (project) ──",
		"── Provided by recipes / catalog ──",
		"── Registered deps ──",
	} {
		if !strings.Contains(out, header) {
			t.Errorf("missing bucket header %q:\n%s", header, out)
		}
	}
	if !strings.Contains(out, "  foo  — Does foo") {
		t.Errorf("missing skill entry:\n%s", out)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("skills list delegated %v", got)
	}
}

// TestSkillsInspectPrintsIdPathAndDesc pins the inspect output.
func TestSkillsInspectPrintsIdPathAndDesc(t *testing.T) {
	root := initializedHubTarget(t)
	home := hubHome(t)
	writeLocalSkill(t, root, "foo", "Does foo")

	keys := keyseq(selectIndex(menuSkills), selectIndex(skillsInspect), selectIndex(0), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	if !strings.Contains(out, "foo  [local]") {
		t.Errorf("missing inspect picker label:\n%s", out)
	}
	if !strings.Contains(out, "\nfoo\n") {
		t.Errorf("missing inspected id:\n%s", out)
	}
	wantPath := "  path: " + filepath.Join(root, "ai-specs", "skills", "foo", "SKILL.md")
	if !strings.Contains(out, wantPath) {
		t.Errorf("missing path %q:\n%s", wantPath, out)
	}
	if !strings.Contains(out, "  desc: Does foo") {
		t.Errorf("missing desc:\n%s", out)
	}
}

// ── agents submenu ───────────────────────────────────────────────────────────

func TestAgentsAppendsOneTableWhenAbsent(t *testing.T) {
	root := hubTargetWithManifest(t, "[project]\nname = \"t\"\n")
	home := hubHome(t)

	// Check claude (cursor starts on it), confirm, pause, quit.
	keys := keyseq(selectIndex(menuAgents), []tui.Key{tui.KeySpace, tui.KeyEnter}, []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	got := readManifest(t, root)
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"claude\"]\n"
	if got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
	if !strings.Contains(out, "  ✓ agents updated: claude") {
		t.Errorf("missing updated line:\n%s", out)
	}
	if !strings.Contains(out, "  Run sync to regenerate agent configs.") {
		t.Errorf("missing sync hint:\n%s", out)
	}
}

// TestAgentsInsertsIntoExistingTableWithoutEnabled pins D12: the key lands inside
// the existing [agents] table, and there is never a second block.
func TestAgentsInsertsIntoExistingTableWithoutEnabled(t *testing.T) {
	root := hubTargetWithManifest(t, "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\n\n[other]\nx = 2\n")
	home := hubHome(t)

	// Move to pi (index 6), check it, confirm, pause, quit.
	keys := keyseq(selectIndex(menuAgents), keyseq(downs(6), []tui.Key{tui.KeySpace, tui.KeyEnter}), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	got := readManifest(t, root)
	want := "[project]\nname = \"t\"\n\n[agents]\nfoo = 1\n\nenabled = [\"pi\"]\n[other]\nx = 2\n"
	if got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
	if n := strings.Count(got, "[agents]"); n != 1 {
		t.Errorf("[agents] appears %d times, want 1", n)
	}
	if !strings.Contains(out, "  ✓ agents updated: pi") {
		t.Errorf("missing updated line:\n%s", out)
	}
}

// TestAgentsReplacesExistingEnabledLine pins the replace branch: toggling a new
// agent rewrites the one `enabled` line instead of adding another.
func TestAgentsReplacesExistingEnabledLine(t *testing.T) {
	root := hubTargetWithManifest(t, "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"cursor\"]\n")
	home := hubHome(t)

	// Cursor is checked; move to pi (index 6), check it, confirm, pause, quit.
	keys := keyseq(selectIndex(menuAgents), keyseq(downs(6), []tui.Key{tui.KeySpace, tui.KeyEnter}), []tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	_, out, _ := runHubInteractive(t, root, home, keys)

	got := readManifest(t, root)
	want := "[project]\nname = \"t\"\n\n[agents]\nenabled = [\"cursor\", \"pi\"]\n"
	if got != want {
		t.Errorf("manifest = %q\nwant     = %q", got, want)
	}
	if n := strings.Count(got, "enabled ="); n != 1 {
		t.Errorf("enabled appears %d times, want 1", n)
	}
	if !strings.Contains(out, "  ✓ agents updated: cursor, pi") {
		t.Errorf("missing updated line:\n%s", out)
	}
}

// ── offer-init ───────────────────────────────────────────────────────────────

func TestOfferInitDeclineExitsWithoutMenu(t *testing.T) {
	root := t.TempDir()
	home := hubHome(t)

	rc, out, _ := runHubInteractive(t, root, home, []tui.Key{tui.KeyNo, tui.KeyEnter})
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if !strings.Contains(out, "No ai-specs project found at "+root+".") {
		t.Errorf("missing offer line:\n%s", out)
	}
	if !strings.Contains(out, "Run the init wizard now?") {
		t.Errorf("missing confirm prompt:\n%s", out)
	}
	if strings.Contains(out, "What do you want to do?") {
		t.Errorf("declined init must not reach the menu:\n%s", out)
	}
	if got := calls(t, home); len(got) != 0 {
		t.Errorf("declined init delegated %v", got)
	}
}

func TestOfferInitAcceptRunsInitThenSync(t *testing.T) {
	root := t.TempDir()
	home := hubHome(t)
	t.Setenv("STUB_INIT_CREATES", "1")

	keys := keyseq([]tui.Key{tui.KeyEnter}, selectIndex(menuQuit))
	rc, out, errb := runHubInteractive(t, root, home, keys)
	if rc != 0 {
		t.Fatalf("rc = %d, want 0 (stderr: %s)", rc, errb)
	}
	want := []string{hubCLI(home) + " init " + root, hubCLI(home) + " sync " + root}
	got := calls(t, home)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if !strings.Contains(out, "✓ init complete — running sync to generate agent configs…") {
		t.Errorf("missing init-complete line:\n%s", out)
	}
}

// TestOfferInitFailureExitsZeroWithoutSync pins `if rc != 0 or not
// is_initialized: return False`: a failed init is not a crash.
func TestOfferInitFailureExitsZeroWithoutSync(t *testing.T) {
	t.Setenv("STUB_EXIT", "3")
	root := t.TempDir()
	home := hubHome(t)

	rc, out, _ := runHubInteractive(t, root, home, []tui.Key{tui.KeyEnter})
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if got := calls(t, home); len(got) != 1 {
		t.Errorf("calls = %v, want only the failed init", got)
	}
	if strings.Contains(out, "✓ init complete") {
		t.Errorf("failed init must not claim success:\n%s", out)
	}
}

func readManifest(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
