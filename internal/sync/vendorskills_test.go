package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// TestSyncVendoredSkillsDifferential runs the real vendor-skills.py and the
// native port over twin projects and compares the whole project tree (bytes,
// modes), the lock bytes, exit code, stdout and stderr.
func TestSyncVendoredSkillsDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repo := gitignoreRepoRoot(t)
	module := filepath.Join(repo, "lib", "_internal", "vendor-skills.py")
	fixture := filepath.Join(repo, "tests", "fixtures", "kepano-obsidian-skills")
	source := "https://github.com/kepano/obsidian-skills.git"

	gitRepo := t.TempDir()
	writeRefreshFile(t, filepath.Join(gitRepo, "SKILL.md"), "---\ndescription: git dep\n---\n# git\n", 0o644)
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "init"},
		{"tag", "v1"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRepo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	manifest := func(extra string) string {
		return "[project]\nname = 'vendor-diff'\n\n[agents]\nenabled = ['claude']\n" + extra
	}

	cases := []struct {
		name     string
		manifest string
	}{
		{"no deps", manifest("")},
		{"dep with ref", manifest("\n[[deps]]\nid = 'dep-ref'\nsource = '" + gitRepo + "'\nref = 'v1'\n")},
		{"dep with path and ancillary dirs", manifest("\n[[deps]]\nid = 'dep-path'\nsource = '" + source + "'\npath = 'skills/obsidian-markdown'\n")},
		{"missing source", manifest("\n[[deps]]\nid = 'dep-missing'\nsource = '" + missing + "'\n")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pyProject, nativeProject := t.TempDir(), t.TempDir()
			writeRefreshFile(t, filepath.Join(pyProject, "ai-specs", "ai-specs.toml"), tc.manifest, 0o644)
			writeRefreshFile(t, filepath.Join(nativeProject, "ai-specs", "ai-specs.toml"), tc.manifest, 0o644)

			cmd := exec.Command("python3", module, pyProject)
			cmd.Dir = repo
			cmd.Env = append(withoutEnv(os.Environ(), "WORKTREE_GATE_BIN"),
				"PYTHONDONTWRITEBYTECODE=1",
				"AI_SPECS_VENDOR_FIXTURE_ROOT="+fixture,
			)
			var pyOut, pyErr bytes.Buffer
			cmd.Stdout = &pyOut
			cmd.Stderr = &pyErr
			pyRC := 0
			if err := cmd.Run(); err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("run python vendor-skills: %v", err)
				}
				pyRC = exitErr.ExitCode()
			}

			var nativeOut, nativeErr bytes.Buffer
			t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", fixture)
			nativeRC := SyncVendoredSkills(nativeProject, t.TempDir(), &nativeOut, &nativeErr)

			if nativeRC != pyRC {
				t.Fatalf("exit code: native = %d, python = %d\nnative stderr:\n%s\npython stderr:\n%s",
					nativeRC, pyRC, nativeErr.String(), pyErr.String())
			}
			if got, want := nativeOut.String(), pyOut.String(); got != want {
				t.Errorf("stdout:\n native = %q\n python = %q", got, want)
			}
			gotErr := stripBridgeFallback(strings.ReplaceAll(nativeErr.String(), nativeProject, "<project>"))
			wantErr := stripBridgeFallback(strings.ReplaceAll(pyErr.String(), pyProject, "<project>"))
			if tc.name == "missing source" {
				// The clone failure is a Python traceback vs. one native error
				// line: both fail loudly, only the text differs (L6 class).
				if gotErr == "" || wantErr == "" {
					t.Errorf("missing source stderr: native = %q, python = %q", gotErr, wantErr)
				}
			} else if gotErr != wantErr {
				t.Errorf("stderr:\n native = %q\n python = %q", gotErr, wantErr)
			}

			compareRefreshTree(t, "project", snapshotRefreshTree(t, pyProject), snapshotRefreshTree(t, nativeProject))
		})
	}
}

// vendorRecipeOracle calls vendor-skills.py's sync_dep_target with
// in_project=False, the exact entry the recipe-dep delegation uses.
const vendorRecipeOracle = `import importlib.util, json, sys
from pathlib import Path
vendor_path = Path(sys.argv[1])
sys.path.insert(0, str(vendor_path.parent))
spec = importlib.util.spec_from_file_location("vendor_skills_internal", vendor_path)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
mod.sync_dep_target(json.loads(sys.argv[2]), Path(sys.argv[3]), cli_home=Path(sys.argv[4]))
`

// TestRecipeDepSkillDifferential runs the real sync_dep_target (in_project=False)
// and the native recipe-dep route over twin projects, comparing stdout and the
// cache .deps tree.
func TestRecipeDepSkillDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	repo := gitignoreRepoRoot(t)
	module := filepath.Join(repo, "lib", "_internal", "vendor-skills.py")
	fixture := filepath.Join(repo, "tests", "fixtures", "kepano-obsidian-skills")
	source := "https://github.com/kepano/obsidian-skills.git"
	dep := map[string]any{"id": "dep1", "source": source, "path": "skills/obsidian-markdown"}
	payload, err := json.Marshal(dep)
	if err != nil {
		t.Fatal(err)
	}

	pyProject, nativeProject := t.TempDir(), t.TempDir()
	pyHome, nativeHome := t.TempDir(), t.TempDir()
	for _, p := range []string{pyProject, nativeProject} {
		writeRefreshFile(t, filepath.Join(p, "ai-specs", "ai-specs.toml"), "[project]\nname = 'x'\n", 0o644)
	}

	cmd := exec.Command("python3", "-c", vendorRecipeOracle, module, string(payload), pyProject, pyHome)
	cmd.Dir = repo
	cmd.Env = append(withoutEnv(os.Environ(), "WORKTREE_GATE_BIN"),
		"PYTHONDONTWRITEBYTECODE=1",
		"AI_SPECS_VENDOR_FIXTURE_ROOT="+fixture,
	)
	var pyOut, pyErr bytes.Buffer
	cmd.Stdout = &pyOut
	cmd.Stderr = &pyErr
	pyRC := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run python sync_dep_target: %v", err)
		}
		pyRC = exitErr.ExitCode()
	}

	var nativeOut, nativeErr bytes.Buffer
	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", fixture)
	nativeErrValue := materializeDepSkill(nativeProject, nativeHome, &schema.SkillRef{
		ID: "dep1", Source: "dep", URL: source, Path: "skills/obsidian-markdown",
	}, &nativeOut, &nativeErr)
	nativeRC := 0
	if nativeErrValue != nil {
		nativeRC = 1
		nativeErr.WriteString(nativeErrValue.Error() + "\n")
	}

	if nativeRC != pyRC {
		t.Fatalf("exit code: native = %d, python = %d\nnative stderr:\n%s\npython stderr:\n%s",
			nativeRC, pyRC, nativeErr.String(), pyErr.String())
	}
	if got, want := nativeOut.String(), pyOut.String(); got != want {
		t.Errorf("stdout:\n native = %q\n python = %q", got, want)
	}
	if pyErr.Len() != 0 || nativeErr.Len() != 0 {
		t.Errorf("stderr: native = %q, python = %q", nativeErr.String(), pyErr.String())
	}

	compareRefreshTree(t,
		"cache/.deps",
		snapshotRefreshTree(t, filepath.Join(refreshCacheRoot(t, pyHome), ".deps")),
		snapshotRefreshTree(t, filepath.Join(refreshCacheRoot(t, nativeHome), ".deps")))
}

// TestSyncVendoredSkillsStepIsNative drives the whole sync spine with a
// PATH-shadowed `python3` that logs every invocation, and requires the vendored
// skills step to run without executing vendor-skills.py.
func TestSyncVendoredSkillsStepIsNative(t *testing.T) {
	realPython, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	logPath := filepath.Join(dir, "python.log")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	wrapper := "#!/bin/bash\nprintf '%s\\n' \"$*\" >> " + shellSingleQuote(logPath) +
		"\nexec " + shellSingleQuote(realPython) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "python3"), []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write python3 shadow: %v", err)
	}

	project := filepath.Join(dir, "project")
	for _, sub := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(project, "ai-specs", sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	manifest := "[project]\nname = 'native-vendor'\n\n[agents]\nenabled = ['claude']\n"
	if err := os.WriteFile(filepath.Join(project, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AI_SPECS_GATE_OFFLINE", "1")
	t.Setenv("AI_SPECS_NO_NETWORK", "1")
	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", filepath.Join(root, "tests", "fixtures", "kepano-obsidian-skills"))
	t.Setenv("TMPDIR", filepath.Join(dir, "tmp"))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("LC_ALL", "C")
	t.Setenv("PYTHONDONTWRITEBYTECODE", "1")
	for _, p := range []string{os.Getenv("TMPDIR"), os.Getenv("HOME")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	// Prove the shadow records invocations before relying on its silence.
	probe := exec.Command("python3", "-c", "pass")
	if out, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("probe shadow: %v\n%s", err, out)
	}
	if log, err := os.ReadFile(logPath); err != nil || len(log) == 0 {
		t.Fatalf("shadow probe logged nothing (%v): %q", err, log)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("clear probe log: %v", err)
	}

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, root, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		// No Python child at all is the expected state; an absent log means
		// the shadow was never invoked, which is also a pass.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read python3 invocation log: %v", err)
	}
	if logged := string(log); strings.Contains(logged, "vendor-skills.py") {
		t.Errorf("sync still execs vendor-skills.py; log:\n%s", logged)
	}
}
