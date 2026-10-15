package cli

// GO-07 S19b: the two internal verbs lib/init.sh reaches through the seam
// (AI_SPECS_GO_BIN). Each mirrors one call site of the block it replaces, so
// the block's own control flow, messages and fallbacks stay in the shell.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// briefProject writes a manifest under a fresh project root and returns both.
func briefProject(t *testing.T, body string) (root, tomlPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlPath = filepath.Join(root, "ai-specs", "ai-specs.toml")
	if err := os.WriteFile(tomlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, tomlPath
}

// briefHome is a CLI home with an empty recipe catalog, which is all a project
// without enabled recipes needs.
func briefHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "catalog", "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestBriefRenderPolicyVerb(t *testing.T) {
	t.Run("render enabled by default", func(t *testing.T) {
		_, tomlPath := briefProject(t, "[project]\nname = \"t\"\n")
		var out, errOut bytes.Buffer
		if rc := runBriefRenderPolicy([]string{"brief-render-policy", tomlPath}, &out, &errOut); rc != 0 {
			t.Fatalf("rc = %d, want 0 (%s)", rc, errOut.String())
		}
		if out.String() != "true\n" {
			t.Errorf("stdout = %q, want %q", out.String(), "true\n")
		}
	})

	t.Run("render disabled by the manifest", func(t *testing.T) {
		_, tomlPath := briefProject(t, "[project]\nname = \"t\"\n\n[brief]\nrender = false\n")
		var out, errOut bytes.Buffer
		if rc := runBriefRenderPolicy([]string{"brief-render-policy", tomlPath}, &out, &errOut); rc != 0 {
			t.Fatalf("rc = %d, want 0 (%s)", rc, errOut.String())
		}
		if out.String() != "false\n" {
			t.Errorf("stdout = %q, want %q", out.String(), "false\n")
		}
	})

	t.Run("an invalid render value stays the fail-safe default", func(t *testing.T) {
		_, tomlPath := briefProject(t, "[project]\nname = \"t\"\n\n[brief]\nrender = \"nope\"\n")
		var out, errOut bytes.Buffer
		if rc := runBriefRenderPolicy([]string{"brief-render-policy", tomlPath}, &out, &errOut); rc != 0 {
			t.Fatalf("rc = %d, want 0 (%s)", rc, errOut.String())
		}
		if out.String() != "true\n" {
			t.Errorf("stdout = %q, want the fail-safe %q", out.String(), "true\n")
		}
	})

	t.Run("an unreadable manifest fails with the error prefix", func(t *testing.T) {
		// The shell consumes only stdout here: a failure prints nothing usable
		// and the block takes its disabled branch, exactly as the Python gate
		// did when its command substitution failed.
		missing := filepath.Join(t.TempDir(), "ai-specs.toml")
		var out, errOut bytes.Buffer
		if rc := runBriefRenderPolicy([]string{"brief-render-policy", missing}, &out, &errOut); rc != 1 {
			t.Fatalf("rc = %d, want 1", rc)
		}
		if out.String() != "" {
			t.Errorf("stdout = %q, want nothing", out.String())
		}
		if !strings.HasPrefix(errOut.String(), "error: ") {
			t.Errorf("stderr = %q, want the `error: ` prefix", errOut.String())
		}
	})
}

func TestBaselineBriefVerb(t *testing.T) {
	t.Run("materializes the resolved config and renders the brief", func(t *testing.T) {
		root, _ := briefProject(t, "[project]\nname = \"probe\"\n\n[agents]\nenabled = [\"claude\"]\n")
		home := briefHome(t)
		resolvedOut := filepath.Join(t.TempDir(), "resolved.json")
		var out, errOut bytes.Buffer
		rc := runBaselineBrief([]string{"baseline-brief", root, "--resolved-config-out", resolvedOut}, home, &out, &errOut)
		if rc != 0 {
			t.Fatalf("rc = %d, want 0\nstdout: %s\nstderr: %s", rc, out.String(), errOut.String())
		}
		brief, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if err != nil {
			t.Fatalf("AGENTS.md: %v", err)
		}
		if !strings.Contains(string(brief), "Runtime Brief") {
			t.Errorf("AGENTS.md does not look like a rendered brief:\n%s", brief)
		}
		if info, err := os.Stat(resolvedOut); err != nil || !info.Mode().IsRegular() {
			t.Errorf("the resolved-config document was not written: %v", err)
		}
	})

	t.Run("a failing materialize short-circuits the render", func(t *testing.T) {
		// A missing manifest is the materializer's own failure: the render must
		// not run, mirroring the shell's `&&`, and nothing may be written.
		root := t.TempDir()
		home := briefHome(t)
		var out, errOut bytes.Buffer
		rc := runBaselineBrief([]string{"baseline-brief", root, "--resolved-config-out", filepath.Join(t.TempDir(), "r.json")}, home, &out, &errOut)
		if rc == 0 {
			t.Fatalf("rc = 0, want a failure\nstdout: %s", out.String())
		}
		if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
			t.Error("AGENTS.md was written although the materialize failed")
		}
	})
}

func TestInternalBriefVerbsAreRoutedButNotAdvertised(t *testing.T) {
	if got := Route([]string{"brief-render-policy", "/tmp/x.toml"}).kind; got != routeBriefRenderPolicy {
		t.Errorf("route kind = %v, want routeBriefRenderPolicy", got)
	}
	if got := Route([]string{"baseline-brief", "/tmp/x"}).kind; got != routeBaselineBrief {
		t.Errorf("route kind = %v, want routeBaselineBrief", got)
	}
	var help bytes.Buffer
	if rc := runHelp(&help); rc != 0 {
		t.Fatalf("runHelp rc = %d", rc)
	}
	for _, verb := range []string{"brief-render-policy", "baseline-brief"} {
		if strings.Contains(help.String(), verb) {
			t.Errorf("%s is an internal seam and must not appear in help", verb)
		}
	}
}
