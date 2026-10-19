package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wantShim is the expected routing decision for a passthrough verb.
type wantShim struct {
	script    string
	invokedAs string
}

// TestRouteTableCoversEveryVerb checks the full dispatcher table: the shimmed
// verbs, the native verbs and their aliases, mirroring the case statement of
// bin/ai-specs (U5c note: add-dep and skills are native now, so the shim map
// holds only what bin/ai-specs still routes to a lib script from the Go
// dispatcher).
func TestRouteTableCoversEveryVerb(t *testing.T) {
	shims := map[string]wantShim{
		"recipe":            {script: "recipe.sh"},
		"configure-recipes": {script: "recipe-config.sh"},
	}
	for verb, want := range shims {
		r := Route([]string{verb})
		if r.kind != routeShim {
			t.Errorf("verb %q: kind = %v, want shim", verb, r.kind)
			continue
		}
		if r.script != want.script {
			t.Errorf("verb %q: script = %q, want %q", verb, r.script, want.script)
		}
		if r.invokedAs != want.invokedAs {
			t.Errorf("verb %q: invokedAs = %q, want %q", verb, r.invokedAs, want.invokedAs)
		}
	}

	natives := map[string]routeKind{
		"refresh-bundled": routeRefreshBundled,
		"sync":            routeSync,
		"sync-agent":      routeSyncAgent,
		"upgrade":         routeUpgrade,
		"hub":             routeHub,
		"init":            routeInit,
		"rules-audit":     routeRulesAudit, "version": routeVersion,
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
