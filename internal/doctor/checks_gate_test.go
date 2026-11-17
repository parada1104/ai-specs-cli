package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoctorCLI(t *testing.T, home, body string) string {
	t.Helper()
	binary := filepath.Join(home, "dist", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("mkdir CLI directory: %v", err)
	}
	writeExecutableFile(t, binary, body)
	return binary
}

func TestCheckWorktreeGateNotInPlay(t *testing.T) {
	t.Run("no-manifest", func(t *testing.T) {
		d := New(t.TempDir(), t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
	t.Run("disabled", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = false\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
	t.Run("enabled-not-true", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = 1\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{})
	})
}

func TestCheckWorktreeGateWarnsForLegacyLauncher(t *testing.T) {
	for _, tc := range []struct {
		name       string
		launcher   string
		wantLegacy bool
	}{
		{
			name:       "old launcher",
			launcher:   "bin=\"$home/cache/bin/worktree-gate/$stamped_gate_version/$_goos-$_goarch/worktree-gate\"\n# WORKTREE_GATE_VERIFY=1\nstamped_gate_impl=\"go\"\n",
			wantLegacy: true,
		},
		{
			name:     "new launcher",
			launcher: "cli_bin=\"${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs\"\nexec \"$cli_bin\" worktree-gate\n",
		},
		{name: "no launcher"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
			writeDoctorCLI(t, home, "#!/bin/sh\n[ \"$1\" = worktree-gate ] && [ \"$2\" = --selftest ] && { echo ok; exit 0; }\nexit 1\n")
			if tc.launcher != "" {
				writeFile(t, filepath.Join(root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate.sh"), tc.launcher)
			}
			d := New(root, home)
			d.checkWorktreeGate()
			if tc.wantLegacy {
				assertChecks(t, d, []Check{
					{WARN, "worktree-gate", "materialized worktree-gate.sh still resolves the retired separate gate binary", "run ai-specs sync (or ai-specs sync --refresh-gates if the file was customized and preserved)"},
					{OK, "worktree-gate", fmt.Sprintf("CLI at %s serves worktree-gate; selftest passed", installedCLIPath(d.Home)), ""},
				})
			} else {
				assertChecks(t, d, []Check{{OK, "worktree-gate", fmt.Sprintf("CLI at %s serves worktree-gate; selftest passed", installedCLIPath(d.Home)), ""}})
			}
		})
	}
}

func TestCheckWorktreeGateInstalledCLI(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	home := t.TempDir()
	writeDoctorCLI(t, home, "#!/bin/sh\n[ \"$1\" = worktree-gate ] && [ \"$2\" = --selftest ] && { echo ok; exit 0; }\nexit 1\n")
	d := New(root, home)
	d.checkWorktreeGate()
	want := Check{OK, "worktree-gate", fmt.Sprintf("CLI at %s serves worktree-gate; selftest passed", d.Home+"/dist/ai-specs"), ""}
	assertChecks(t, d, []Check{want})
	if got, line := d.Checks[0].Render(), want.Render(); got != line {
		t.Fatalf("doctor line = %q, want %q", got, line)
	}
}

func TestCheckWorktreeGateMissingCLIUsesFailOpenSeverity(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	home := t.TempDir()
	d := New(root, home)
	d.checkWorktreeGate()
	binary := installedCLIPath(d.Home)
	assertChecks(t, d, []Check{{ERROR, "worktree-gate",
		fmt.Sprintf("ai-specs CLI is missing or not executable at %s; the gate is failing open", binary),
		"run ai-specs sync"}})
}

func TestCheckWorktreeGateCLISelftestFailureUsesFailOpenSeverity(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	home := t.TempDir()
	writeDoctorCLI(t, home, "#!/bin/sh\necho selftest exploded >&2\nexit 1\n")
	d := New(root, home)
	d.checkWorktreeGate()
	assertChecks(t, d, []Check{{ERROR, "worktree-gate",
		fmt.Sprintf("ai-specs CLI at %s failed worktree-gate --selftest: selftest exploded; the gate is failing open", installedCLIPath(d.Home)),
		"run ai-specs sync"}})
}

func TestCheckWorktreeGateBashRetired(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n"+
			"[recipes.worktree-flow.config]\ngate_impl = \"bash\"\n")
		d := New(root, t.TempDir())
		d.checkWorktreeGate()
		assertChecks(t, d, []Check{{ERROR, "worktree-gate",
			"gate_impl=bash is retired; set auto or go, then ai-specs sync",
			"doctor is read-only; set gate_impl to auto or go, then run ai-specs sync"}})
	})
}

func TestCheckWorktreeGateLeftoverInfo(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.worktree-flow]\nenabled = true\n")
	writeDoctorCLI(t, home, "#!/bin/sh\necho ok\nexit 0\n")
	writeFile(t, filepath.Join(root, "ai-specs", "recipes", "worktree-flow", "hooks", "worktree-gate-legacy.sh"), "#!/bin/sh\n")
	d := New(root, home)
	d.checkWorktreeGate()
	if len(d.Checks) != 2 || d.Checks[0].Severity != INFO || d.Checks[0].Name != "worktree-gate" {
		t.Fatalf("checks = %+v; want legacy INFO followed by CLI result", d.Checks)
	}
	if got, want := d.Checks[1], (Check{OK, "worktree-gate", fmt.Sprintf("CLI at %s serves worktree-gate; selftest passed", installedCLIPath(d.Home)), ""}); got != want {
		t.Fatalf("second check = %+v, want %+v", got, want)
	}
	if !strings.Contains(d.Checks[0].Message, "leftover worktree-gate-legacy.sh is inert") {
		t.Errorf("leftover message = %q", d.Checks[0].Message)
	}
}
