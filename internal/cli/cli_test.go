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

// TestRouteTableCoversEveryVerb checks the full dispatcher table: 11 shimmed
// verbs + 3 native verbs + their aliases, mirroring the case statement of
// bin/ai-specs.
func TestRouteTableCoversEveryVerb(t *testing.T) {
	shims := map[string]wantShim{
		"hub":               {script: "hub.sh"},
		"init":              {script: "init.sh"},
		"sync":              {script: "sync.sh"},
		"sync-agent":        {script: "sync-agent.sh"},
		"add-dep":           {script: "skills-add.sh", invokedAs: "ai-specs add-dep"},
		"skills":            {script: "skills.sh"},
		"doctor":            {script: "doctor.sh"},
		"rules-audit":       {script: "rules-audit.sh"},
		"recipe":            {script: "recipe.sh"},
		"configure-recipes": {script: "recipe-config.sh"},
		"upgrade":           {script: "upgrade.sh"},
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
		"version":         routeVersion,
		"-v":              routeVersion,
		"--version":       routeVersion,
		"help":            routeHelp,
		"-h":              routeHelp,
		"--help":          routeHelp,
	}
	for verb, want := range natives {
		if got := Route([]string{verb}).kind; got != want {
			t.Errorf("verb %q: kind = %v, want %v", verb, got, want)
		}
	}
}

func TestRouteBareInvocationRewritesToHub(t *testing.T) {
	r := Route(nil)
	if r.kind != routeShim || r.script != "hub.sh" {
		t.Fatalf("bare invocation must route to hub, got kind=%v script=%q", r.kind, r.script)
	}
}

// TestRunBareInvocationRoutesToHub executes the bare-invocation path
// end-to-end: Route(nil) rewrites to hub and runShim must pass the ORIGINAL
// (empty) argv to hub.sh (legacy no-shift semantics) instead of panicking on
// args[1:]. With an uninitialized temp cwd and buffered (non-TTY) writers,
// hub.sh's pre-guard fires: exit 2, no Python launch.
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
		t.Errorf("stderr = %q, want hub.sh pre-guard message\"no ai-specs project at ...\"", stderr.String())
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
