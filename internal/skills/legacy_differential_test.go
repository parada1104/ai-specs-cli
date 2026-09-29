package skills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Same differential discipline as internal/doctor/legacy_differential_test.go:
// skip when python3 or the legacy module is unavailable, otherwise pin the
// ported resolution against skill-resolution.py.

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

func legacyModule(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "lib", "_internal", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("legacy %s unavailable: %v", name, err)
	}
	return path
}

const skillResolutionOracle = `
import importlib.util, json, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("skill_resolution_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
root = Path(sys.argv[2])
home = Path(sys.argv[3])
resolved = mod.collect_skills(root, cli_home=home)
print(json.dumps({k: {"source": v[0], "path": str(v[1])} for k, v in resolved.items()}, indent=2))
`

// runPythonStdout runs python and returns stdout only (legacy warnings go to
// stderr and are discarded here so the JSON stays parseable).
func runPythonStdout(t *testing.T, args ...string) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	cmd := exec.Command(python, args...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python3 %s: %v\n%s", args[0], err, out)
	}
	return string(out)
}

func TestDifferentialCollectSkills(t *testing.T) {
	script := legacyModule(t, "skill-resolution.py")
	root, home, _ := buildSkillTree(t)

	var legacy map[string]ResolvedSkill
	raw := runPythonStdout(t, "-c", skillResolutionOracle, script, root, home)
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		t.Fatalf("decode oracle output: %v\n%s", err, raw)
	}

	got := CollectSkills(root, home)
	if len(got) != len(legacy) {
		t.Fatalf("skill count = %d, legacy %d\ngot    %#v\nlegacy %#v", len(got), len(legacy), got, legacy)
	}
	for id, want := range legacy {
		if got[id] != want {
			t.Errorf("%s:\n port   %+v\n legacy %+v", id, got[id], want)
		}
	}
}
