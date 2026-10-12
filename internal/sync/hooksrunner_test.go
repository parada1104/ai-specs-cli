package sync

// Contract tests for the native recipe hooks runner (GO-07 S13 PR4a).
//
// RunRecipeHooks mirrors the Python authority execute_hooks
// (lib/_internal/recipe-materialize.py:3069): it walks recipe.Hooks in
// declaration order, validates the merged config, writes the bootstrap-board
// marker, prints the deferred-action info lines, and warns on an unknown
// action. The runner is added, not wired; S14 owns the sync-pipeline wiring.
//
// The differential test at the bottom shells out to the real Python module via
// testdata/hooksrunner_ref.py (the bindings_test.go / hooksrender_test.go
// shape) and compares the marker file bytes, the raised error string and the
// captured info/warning lines scenario by scenario.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// hooksRunnerRefPath resolves the reference driver under this package (go test
// cwd is internal/sync).
func hooksRunnerRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "hooksrunner_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

// hooksRunnerRefOut is the canonical JSON testdata/hooksrunner_ref.py emits.
type hooksRunnerRefOut struct {
	Error  string `json:"error"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// runHooksRunnerRef runs the reference driver once and decodes its envelope.
func runHooksRunnerRef(t *testing.T, python, driver string, args ...string) hooksRunnerRefOut {
	t.Helper()
	cmd := exec.Command(python, append([]string{driver}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	var out hooksRunnerRefOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode ref %s: %v", stdout.String(), err)
	}
	return out
}

// hooksRunnerCatalog is the shared fixture catalog: one recipe per action and
// validation branch. Each recipe is a real recipe.toml read by both legs.
func hooksRunnerCatalog(t *testing.T) string {
	t.Helper()
	catalog := t.TempDir()
	header := syncRecipeHeader
	writeSyncCatalogRecipe(t, catalog, "hookorder", header("hookorder", "hookorder")+
		"[[hooks]]\nevent = \"sync\"\naction = \"link-trello-card\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"bootstrap-board\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"sync-card-state\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"comment-verification\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"not-a-real-action\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookbootstrap", header("hookbootstrap", "hookbootstrap")+
		"[[hooks]]\nevent = \"sync\"\naction = \"bootstrap-board\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookrequired", header("hookrequired", "hookrequired")+
		"[config.board_id]\nrequired = true\ntype = \"string\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookoptional", header("hookoptional", "hookoptional")+
		"[config.must]\nrequired = true\ntype = \"string\"\n"+
		"[config.skipped]\nrequired = false\ntype = \"string\"\nvalidation.regex = \"[0-9]+\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookfields", header("hookfields", "hookfields")+
		"[config.alpha]\nrequired = true\ntype = \"string\"\n"+
		"[config.zeta]\nrequired = true\ntype = \"string\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookboard", header("hookboard", "hookboard")+
		"[config.board_id]\nrequired = true\ntype = \"string\"\nvalidation.regex = \"[0-9a-f]{24}\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	writeSyncCatalogRecipe(t, catalog, "hookslug", header("hookslug", "hookslug")+
		"[config.slug]\nrequired = true\ntype = \"string\"\nvalidation.regex = \"[a-z]+\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	return catalog
}

// hooksRunnerScenario is one recipe + merged-config pair.
type hooksRunnerScenario struct {
	name   string
	recipe string
	merged map[string]any
}

func hooksRunnerScenarios() []hooksRunnerScenario {
	return []hooksRunnerScenario{
		{"all actions in declaration order", "hookorder", map[string]any{}},
		{"bootstrap marker with defaults", "hookbootstrap", map[string]any{}},
		{"bootstrap marker with config values", "hookbootstrap", map[string]any{
			"board_id": "69ec0a2099ea20956e371d62", "default_list": "Backlog", "epic_list": "Epics",
		}},
		{"required field missing", "hookrequired", map[string]any{}},
		{"field not present is skipped", "hookoptional", map[string]any{"must": "present"}},
		{"first missing field wins declaration order", "hookfields", map[string]any{}},
		{"board_id shortLink rejected", "hookboard", map[string]any{"board_id": "AbCd1234"}},
		{"board_id 24-hex regex matches", "hookboard", map[string]any{"board_id": "69ec0a2099ea20956e371d62"}},
		{"board_id regex mismatch", "hookboard", map[string]any{"board_id": "ZZZZZZZZZZZZZZZZZZZZZZZZ"}},
		{"slug regex matches", "hookslug", map[string]any{"slug": "goodslug"}},
		{"slug regex mismatch", "hookslug", map[string]any{"slug": "Bad Slug"}},
	}
}

// runNativeRecipeHooks runs the native runner and returns the captured streams
// plus the error string (empty when the runner returned nil).
func runNativeRecipeHooks(t *testing.T, catalog, recipe string, merged map[string]any, projectRoot, cliHome string) (string, string, string) {
	t.Helper()
	var out, errW bytes.Buffer
	err := RunRecipeHooks(HooksRequest{
		RecipePath:   filepath.Join(catalog, recipe, "recipe.toml"),
		MergedConfig: merged,
		ProjectRoot:  projectRoot,
		CLIHome:      cliHome,
	}, &out, &errW)
	message := ""
	if err != nil {
		message = err.Error()
	}
	return out.String(), errW.String(), message
}

// readHooksMarker returns the bootstrap-ready bytes and whether they exist.
func readHooksMarker(t *testing.T, root, cliHome, recipeID string) ([]byte, bool) {
	t.Helper()
	path := filepath.Join(projectcache.RecipeSkillsRoot(root, cliHome), recipeID, "bootstrap-ready")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		t.Fatalf("read marker %s: %v", path, err)
	}
	return data, true
}

// TestRecipeHooksDifferentialAgainstPython pins RunRecipeHooks against the real
// Python execute_hooks: the marker bytes, the raised error string and the
// printed info/warning lines must agree scenario by scenario.
func TestRecipeHooksDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := hooksRunnerRefPath(t)
	catalog := hooksRunnerCatalog(t)
	cliHome := t.TempDir()

	for _, tc := range hooksRunnerScenarios() {
		t.Run(tc.name, func(t *testing.T) {
			mergedJSON, err := json.Marshal(tc.merged)
			if err != nil {
				t.Fatal(err)
			}
			pyRoot := t.TempDir()
			goRoot := t.TempDir()

			ref := runHooksRunnerRef(t, python, driver, "run", authority, catalog, tc.recipe, pyRoot, cliHome, string(mergedJSON))
			goOut, goErr, goMessage := runNativeRecipeHooks(t, catalog, tc.recipe, tc.merged, goRoot, cliHome)

			if goMessage != ref.Error {
				t.Fatalf("error: go=%q python=%q", goMessage, ref.Error)
			}
			if goOut != ref.Stdout {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- python ---\n%q", goOut, ref.Stdout)
			}
			if goErr != ref.Stderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- python ---\n%q", goErr, ref.Stderr)
			}

			pyMarker, pyOK := readHooksMarker(t, pyRoot, cliHome, tc.recipe)
			goMarker, goOK := readHooksMarker(t, goRoot, cliHome, tc.recipe)
			if pyOK != goOK {
				t.Fatalf("marker presence: go=%v python=%v", goOK, pyOK)
			}
			if !bytes.Equal(pyMarker, goMarker) {
				t.Fatalf("marker bytes:\n--- go ---\n%q\n--- python ---\n%q", goMarker, pyMarker)
			}
		})
	}
}

// TestRecipeHooksDeferredAndUnknownLines pins the exact info/warning bytes and
// their stream, independent of the Python oracle.
func TestRecipeHooksDeferredAndUnknownLines(t *testing.T) {
	catalog := hooksRunnerCatalog(t)
	cliHome := t.TempDir()
	goOut, goErr, goMessage := runNativeRecipeHooks(t, catalog, "hookorder", map[string]any{}, t.TempDir(), cliHome)

	wantOut := "  \u2139 recipe 'hookorder': hook 'link-trello-card' deferred to agent runtime\n" +
		"  \u2139 recipe 'hookorder': hook 'sync-card-state' deferred to agent runtime\n" +
		"  \u2139 recipe 'hookorder': hook 'comment-verification' deferred to agent runtime\n"
	wantErr := "  ! recipe 'hookorder': unknown hook action 'not-a-real-action' (skipped)\n"
	if goOut != wantOut {
		t.Fatalf("stdout = %q, want %q", goOut, wantOut)
	}
	if goErr != wantErr {
		t.Fatalf("stderr = %q, want %q", goErr, wantErr)
	}
	if goMessage != "" {
		t.Fatalf("error = %q, want none", goMessage)
	}
}

// TestRecipeHooksMarkerBytes pins the marker bytes and the exact defaults.
func TestRecipeHooksMarkerBytes(t *testing.T) {
	catalog := hooksRunnerCatalog(t)
	cliHome := t.TempDir()
	cases := []struct {
		name   string
		merged map[string]any
		want   string
	}{
		{"defaults", map[string]any{},
			"board_id=\ndefault_list=In Progress\nepic_list=Epic\n"},
		{"config values win", map[string]any{
			"board_id": "69ec0a2099ea20956e371d62", "default_list": "Backlog", "epic_list": "Epics",
		}, "board_id=69ec0a2099ea20956e371d62\ndefault_list=Backlog\nepic_list=Epics\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if _, _, message := runNativeRecipeHooks(t, catalog, "hookbootstrap", tc.merged, root, cliHome); message != "" {
				t.Fatalf("unexpected error: %s", message)
			}
			got, ok := readHooksMarker(t, root, cliHome, "hookbootstrap")
			if !ok || string(got) != tc.want {
				t.Fatalf("marker = %q (exists=%v), want %q", got, ok, tc.want)
			}
		})
	}
}

// TestRecipeHooksErrorStrings pins every validation error string literally.
func TestRecipeHooksErrorStrings(t *testing.T) {
	catalog := hooksRunnerCatalog(t)
	cliHome := t.TempDir()
	cases := []struct {
		name   string
		recipe string
		merged map[string]any
		want   string
	}{
		{"required missing", "hookrequired", map[string]any{},
			"recipe 'hookrequired': hook 'validate-config' failed: missing required config field 'board_id'"},
		{"shortLink rejected", "hookboard", map[string]any{"board_id": "AbCd1234"},
			"recipe 'hookboard': hook 'validate-config' failed: field 'board_id' value 'AbCd1234' looks like a Trello shortLink; the real board ID is 24 hex characters (e.g., '69ec0a2099ea20956e371d62')"},
		{"regex mismatch", "hookslug", map[string]any{"slug": "Bad Slug"},
			"recipe 'hookslug': hook 'validate-config' failed: field 'slug' value 'Bad Slug' does not match required pattern '[a-z]+'"},
		{"declaration order", "hookfields", map[string]any{},
			"recipe 'hookfields': hook 'validate-config' failed: missing required config field 'alpha'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, message := runNativeRecipeHooks(t, catalog, tc.recipe, tc.merged, t.TempDir(), cliHome)
			if message != tc.want {
				t.Fatalf("error = %q, want %q", message, tc.want)
			}
		})
	}
}

// pythonFullMatchStatus runs Python's real re.fullmatch and reports "Y"/"N"/
// "ERR" (compiled, matched / compiled, no match / re.error).
func pythonFullMatchStatus(t *testing.T, python, pattern, value string) string {
	t.Helper()
	const script = "import re,sys\ntry:\n m=re.fullmatch(sys.argv[1],sys.argv[2])\nexcept re.error:\n print('ERR')\nelse:\n print('Y' if m else 'N')\n"
	out, err := exec.Command(python, "-c", script, pattern, value).Output()
	if err != nil {
		t.Fatalf("python fullmatch %q: %v", pattern, err)
	}
	return strings.TrimSpace(string(out))
}

// TestRecipeHooksRegexEngineDivergence records the RE2/Python gap the brief
// calls out. Go regexp is RE2, so the native runner diverges from Python in two
// named ways: Python-valid constructs RE2 cannot compile at all, and the \d \w
// \s classes that are Unicode-aware in Python but ASCII-only in RE2. The
// divergence is pinned here, never claimed as parity.
func TestRecipeHooksRegexEngineDivergence(t *testing.T) {
	python, _ := syncPythonAuthority(t)

	// (1) Python compiles these; RE2 does not, so the runner returns a compile
	// error instead of Python's match result.
	compileCases := []struct {
		name    string
		pattern string
	}{
		{"backreference", `(a)\1`},
		{"named backreference", `(?P<x>a)(?P=x)`},
		{"lookahead", `a(?=b)`},
		{"negative lookahead", `a(?!b)`},
		{"lookbehind", `(?<=a)b`},
		{"negative lookbehind", `(?<!a)b`},
	}
	for _, tc := range compileCases {
		t.Run("compile/"+tc.name, func(t *testing.T) {
			if got := pythonFullMatchStatus(t, python, tc.pattern, "ab"); got == "ERR" {
				t.Fatalf("python re rejected %q, so the recorded divergence no longer holds (status %s)", tc.pattern, got)
			}
			if _, err := hooksFullMatch(tc.pattern, "ab"); err == nil {
				t.Fatalf("Go regexp compiled %q; the recorded divergence changed", tc.pattern)
			}
		})
	}

	// (2) Both compile, but Python matches a Unicode character RE2's ASCII-only
	// class misses.
	unicodeCases := []struct {
		name    string
		pattern string
		value   string
	}{
		{"digit class", `\d`, "\u0663"}, // ARABIC-INDIC DIGIT THREE
		{"word class", `\w`, "\u00e9"},  // LATIN SMALL LETTER E WITH ACUTE
		{"space class", `\s`, "\u00a0"}, // NO-BREAK SPACE
	}
	for _, tc := range unicodeCases {
		t.Run("class/"+tc.name, func(t *testing.T) {
			if got := pythonFullMatchStatus(t, python, tc.pattern, tc.value); got != "Y" {
				t.Fatalf("python no longer matches %q against %q (status %s)", tc.pattern, tc.value, got)
			}
			matched, err := hooksFullMatch(tc.pattern, tc.value)
			if err != nil {
				t.Fatalf("Go regexp failed to compile %q: %v", tc.pattern, err)
			}
			if matched {
				t.Fatalf("Go regexp now matches %q against %q; the recorded divergence changed", tc.pattern, tc.value)
			}
		})
	}

	// (3) The fail-open mirror of (2): Python's \W \S \D and \B are
	// Unicode-aware, but RE2's are ASCII-only, so for a non-ASCII value RE2
	// MATCHES where Python does NOT. Python therefore raises the mismatch
	// error while the native runner accepts the value and lets sync continue.
	// Here the permissive side is the native one, so the native path is
	// fail-open for these constructs: it validates a pattern/value pair that
	// the Python authority rejects.
	failOpenClasses := []struct {
		name    string
		pattern string
		value   string
	}{
		{"non-word class", `\W`, "\u00e9"},  // LATIN SMALL LETTER E WITH ACUTE is a \w char in Python
		{"non-space class", `\S`, "\u00a0"}, // NO-BREAK SPACE is a \s char in Python
		{"non-digit class", `\D`, "\u0663"}, // ARABIC-INDIC DIGIT THREE is a \d char in Python
		{"non-boundary", `\B.`, "\u00e9"},   // \B before a non-ASCII letter differs: Python has no boundary there
	}
	for _, tc := range failOpenClasses {
		t.Run("failopen/"+tc.name, func(t *testing.T) {
			if got := pythonFullMatchStatus(t, python, tc.pattern, tc.value); got != "N" {
				t.Fatalf("python now matches %q against %q (status %s); the recorded fail-open divergence changed", tc.pattern, tc.value, got)
			}
			matched, err := hooksFullMatch(tc.pattern, tc.value)
			if err != nil {
				t.Fatalf("Go regexp failed to compile %q: %v", tc.pattern, err)
			}
			if !matched {
				t.Fatalf("Go regexp no longer matches %q against %q; the recorded fail-open divergence changed", tc.pattern, tc.value)
			}
		})
	}

	// (4) RE2-only syntax that Python cannot accept or reads differently. In
	// both shapes RE2 matches the value while Python fails, so the native path
	// is again the permissive, fail-open one: `\p{L}` makes Python raise
	// re.error (bad escape \p), and `[[:alpha:]]` is a POSIX class in RE2 but a
	// literal character set in Python (Python emits a FutureWarning and does not
	// match "a").
	re2Only := []struct {
		name    string
		pattern string
		value   string
		pyErr   bool
	}{
		{"unicode property", `\p{L}`, "a", true},
		{"posix class", `[[:alpha:]]`, "a", false},
	}
	for _, tc := range re2Only {
		t.Run("failopen/"+tc.name, func(t *testing.T) {
			want := "N"
			if tc.pyErr {
				want = "ERR"
			}
			if got := pythonFullMatchStatus(t, python, tc.pattern, tc.value); got != want {
				t.Fatalf("python status for %q is %q, want %q", tc.pattern, got, want)
			}
			matched, err := hooksFullMatch(tc.pattern, tc.value)
			if err != nil {
				t.Fatalf("Go regexp failed to compile %q: %v", tc.pattern, err)
			}
			if !matched {
				t.Fatalf("Go regexp no longer matches %q against %q; the recorded fail-open divergence changed", tc.pattern, tc.value)
			}
		})
	}
}

// TestRecipeHooksRegexPythonOnlyConstructs records the fail-closed side of the
// RE2/Python gap: constructs the real Python engine accepts but RE2 cannot
// compile, so the native runner returns a compile error and fails sync where
// Python validates the value. The one asymmetry below is the $ anchor: Python
// matches $ just before a trailing newline, while the native \A(?:...)\z
// wrapper does not, because Go's $ (without (?m)) anchors only at the very end
// of the text. Every case is pinned against the real Python engine.
func TestRecipeHooksRegexPythonOnlyConstructs(t *testing.T) {
	python, _ := syncPythonAuthority(t)

	cases := []struct {
		name    string
		pattern string
		value   string
	}{
		{"unicode escape", `\u0041`, "A"},
		{"end-of-text escape", `a\Z`, "a"},
		{"possessive quantifier", `a*+`, "aaa"},
		{"atomic group", `(?>a)`, "a"},
		{"comment group", `(?#x)a`, "a"},
		{"conditional", `(a)?(?(1)b|c)`, "c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pythonFullMatchStatus(t, python, tc.pattern, tc.value); got != "Y" {
				t.Fatalf("python no longer matches %q against %q (status %s)", tc.pattern, tc.value, got)
			}
			if _, err := hooksFullMatch(tc.pattern, tc.value); err == nil {
				t.Fatalf("Go regexp compiled %q; the recorded fail-closed divergence changed", tc.pattern)
			}
		})
	}

	t.Run("dollar before trailing newline", func(t *testing.T) {
		if got := pythonFullMatchStatus(t, python, "a$\n", "a\n"); got != "Y" {
			t.Fatalf("python no longer matches a$ before a trailing newline (status %s)", got)
		}
		matched, err := hooksFullMatch("a$\n", "a\n")
		if err != nil {
			t.Fatalf("Go regexp failed to compile: %v", err)
		}
		if matched {
			t.Fatalf("native fullmatch now matches a$ before a trailing newline; the recorded divergence changed")
		}
	})
}

// hooksRunnerReprCatalog is the fixture for the nested-value repr divergence:
// one recipe whose validate-config regex rejects the stringified value, so
// str(value) surfaces in the raised error message.
func hooksRunnerReprCatalog(t *testing.T) string {
	t.Helper()
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "hookrepr", syncRecipeHeader("hookrepr", "hookrepr")+
		"[config.nested]\nrequired = true\ntype = \"string\"\nvalidation.regex = \"[0-9]+\"\n"+
		"[[hooks]]\nevent = \"sync\"\naction = \"validate-config\"\n")
	return catalog
}

// TestRecipeHooksNestedValueReprDivergence records a PRE-EXISTING gap in the
// reused internal/config.PyStr port, not something this delivery introduced.
// PyStr mirrors Python str() for tomllib scalars and *toml.Table values, but a
// nested value that arrives here as a Go map[string]any falls through to fmt's
// %v (printing map[a:1] where Python prints {'a': 1}), and a nil nested inside
// a []any prints as an empty string (where Python prints None). These strings
// surface in this runner's validation error messages and marker bytes, so both
// sides are pinned against the real Python authority. A nested-value repr fix
// belongs to the repr-port owner (internal/config), not to the hooks runner.
func TestRecipeHooksNestedValueReprDivergence(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := hooksRunnerRefPath(t)
	catalog := hooksRunnerReprCatalog(t)
	cliHome := t.TempDir()

	cases := []struct {
		name    string
		merged  map[string]any
		pyValue string
		goValue string
	}{
		{"nested map", map[string]any{"nested": map[string]any{"a": 1}}, "{'a': 1}", "map[a:1]"},
		{"nil inside a slice", map[string]any{"nested": []any{"x", nil}}, "['x', None]", "['x', ]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mergedJSON, err := json.Marshal(tc.merged)
			if err != nil {
				t.Fatal(err)
			}
			ref := runHooksRunnerRef(t, python, driver, "run", authority, catalog, "hookrepr",
				t.TempDir(), cliHome, string(mergedJSON))
			_, _, goMessage := runNativeRecipeHooks(t, catalog, "hookrepr", tc.merged, t.TempDir(), cliHome)

			pyWant := "recipe 'hookrepr': hook 'validate-config' failed: field 'nested' value '" +
				tc.pyValue + "' does not match required pattern '[0-9]+'"
			goWant := "recipe 'hookrepr': hook 'validate-config' failed: field 'nested' value '" +
				tc.goValue + "' does not match required pattern '[0-9]+'"
			if ref.Error != pyWant {
				t.Fatalf("python error = %q, want %q", ref.Error, pyWant)
			}
			if goMessage != goWant {
				t.Fatalf("native error = %q, want %q", goMessage, goWant)
			}
			if goMessage == ref.Error {
				t.Fatalf("native and python now agree (%q); the recorded repr divergence changed", goMessage)
			}
		})
	}
}
