package sync

// GO-07 S14c/S17: the standalone fan-out resolves both its resolved-config and
// its target plan natively.
//
// The discrimination is exact and needs no network: a PATH-stubbed python3
// records every invocation and delegates to the real interpreter, so the stub
// log shows which steps are still delegated and which are native. Before the
// S14c wiring the resolved-config step spawns recipe-materialize.py; before the
// S17 wiring the standalone plan spawns target-resolve.py. The fan-out itself
// is driven by the manifest's declared subrepos, which is the only plan shape a
// real resolver emits.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneMaterializeRunsNatively(t *testing.T) {
	scenarios := []struct {
		name     string
		targets  []string
		subrepos []string
	}{
		{name: "single-target body"},
		{name: "standalone fan-out", targets: []string{"one", "two"}, subrepos: []string{"one", "two"}},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			realPython, lookupErr := exec.LookPath("python3")
			if lookupErr != nil {
				t.Skipf("python3 not available: %v", lookupErr)
			}

			stubDir := t.TempDir()
			logPath := filepath.Join(stubDir, "invocations.log")
			root := t.TempDir()

			if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
				t.Fatal(err)
			}
			manifest := "[project]\nname = \"t\"\n"
			if len(scenario.subrepos) > 0 {
				quoted := make([]string, 0, len(scenario.subrepos))
				for _, rel := range scenario.subrepos {
					quoted = append(quoted, "\""+rel+"\"")
				}
				manifest += "subrepos = [" + strings.Join(quoted, ", ") + "]\n"
			}
			manifest += "\n[recipes.alpha]\nenabled = true\n"
			if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"),
				[]byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			recipeDir := filepath.Join(root, "catalog", "recipes", "alpha")
			if err := os.MkdirAll(recipeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(recipeDir, "recipe.toml"),
				[]byte(syncRecipeHeader("alpha", "Alpha")), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# brief\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			entries := make([]string, 0, len(scenario.targets))
			for _, name := range scenario.targets {
				target := filepath.Join(root, name)
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "AGENTS.md"), []byte("# child brief\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				entries = append(entries, "{\"path\":\""+target+"\"}")
			}
			planJSON := "{\"root\":\"" + root + "\",\"targets\":[" + strings.Join(entries, ",") + "]}"

			// The stub records every invocation and DELEGATES to the real
			// interpreter, because the gate binary's own readers are still
			// Python; only the resolver plan is synthesized so both fan-out
			// shapes are reachable.
			script := "#!/bin/sh\n" +
				"printf '%s\\n' \"$*\" >> " + logPath + "\n" +
				"if printf '%s\\n' \"$*\" | grep -q target-resolve.py; then\n" +
				"  cat <<'PLANEOF'\n" + planJSON + "\nPLANEOF\n" +
				"  exit 0\n" +
				"fi\n" +
				"exec " + realPython + " \"$@\"\n"
			if err := os.WriteFile(filepath.Join(stubDir, "python3"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}

			t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
			// The native core reaches the gate binary for the conflict and stamp
			// steps; pin the fixture-built one, exactly like the materializer
			// differentials do. It is a Go binary, so the python3 stub stays
			// unused by the materialize step by design.
			t.Setenv("WORKTREE_GATE_BIN", materializeGateBinary(t))
			t.Setenv("AI_SPECS_GATE_OFFLINE", "1")

			var out, errOut bytes.Buffer
			rc := RunAgent([]string{"sync-agent", root}, root, nil, &out, &errOut)
			if rc != 0 {
				t.Fatalf("rc = %d, want 0\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errOut.String())
			}

			logged, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("stub log: %v", err)
			}
			log := string(logged)
			if strings.Contains(log, "target-resolve.py") {
				t.Fatalf("the plan still spawns target-resolve.py:\n%s", log)
			}
			if strings.Contains(log, "recipe-materialize.py") {
				t.Fatalf("the materializer still spawns recipe-materialize.py:\n%s", log)
			}
			if strings.Contains(errOut.String(), "resolved-config generation failed") {
				t.Fatalf("native resolved-config failed:\n%s", errOut.String())
			}
		})
	}
}
