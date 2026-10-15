package cli

// GO-07 S19a: the seam a legacy script uses to reach a ported subcommand.
// The Go dispatcher exports its own path as AI_SPECS_GO_BIN into every shimmed
// child, so `lib/skills-add.sh` can run the native `sync` instead of the shell
// script — and a stale inherited value never leaks.

import (
	"strings"
	"testing"
)

// envValues returns every value of key in an os.Environ-shaped slice.
func envValues(env []string, key string) []string {
	prefix := key + "="
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, strings.TrimPrefix(kv, prefix))
		}
	}
	return out
}

func TestChildEnvExportsTheNativeDispatcherPath(t *testing.T) {
	t.Run("the running binary's path is exported", func(t *testing.T) {
		t.Setenv("AI_SPECS_GO_BIN", "/stale/other/ai-specs")
		env := childEnv("/home/x", "", "/usr/local/lib/ai-specs")
		got := envValues(env, "AI_SPECS_GO_BIN")
		if len(got) != 1 || got[0] != "/usr/local/lib/ai-specs" {
			t.Errorf("AI_SPECS_GO_BIN = %v, want exactly the running binary's path", got)
		}
	})

	t.Run("an unresolvable path leaves the seam unset", func(t *testing.T) {
		t.Setenv("AI_SPECS_GO_BIN", "/stale/other/ai-specs")
		env := childEnv("/home/x", "ai-specs add-dep", "")
		if got := envValues(env, "AI_SPECS_GO_BIN"); len(got) != 0 {
			t.Errorf("AI_SPECS_GO_BIN = %v, want it dropped: a script must fall back to its legacy route", got)
		}
		if got := envValues(env, "AI_SPECS_INVOKED_AS"); len(got) != 1 || got[0] != "ai-specs add-dep" {
			t.Errorf("AI_SPECS_INVOKED_AS = %v, want the alias preserved", got)
		}
		if got := envValues(env, "AI_SPECS_HOME"); len(got) != 1 || got[0] != "/home/x" {
			t.Errorf("AI_SPECS_HOME = %v, want the resolved home", got)
		}
	})
}
