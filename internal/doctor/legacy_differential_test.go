package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// This file pins the ported policy helpers against the legacy Python modules
// they replace, using the same differential discipline as
// internal/config/differential_test.go. Each test skips when python3 or the
// legacy module is unavailable.

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

func runPython(t *testing.T, args ...string) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	cmd := exec.Command(python, args...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python3 %s: %v\n%s", args[0], err, out)
	}
	return string(out)
}

func legacyModule(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "lib", "_internal", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("legacy %s unavailable: %v", name, err)
	}
	return path
}

const cliVersionOracle = `
import importlib.util, json, sys, tomllib
spec = importlib.util.spec_from_file_location("cli_version_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
out = []
for case in json.loads(sys.argv[2]):
    src = case.get("toml") or ""
    manifest = tomllib.loads(src) if src else {}
    severity, name, message = mod.evaluate_cli_version(
        installed=case["installed"], manifest=manifest, lock_meta=case["lock_meta"]
    )
    out.append({"severity": severity, "name": name, "message": message})
print(json.dumps(out))
`

// TestDifferentialCLIVersion compares the ported evaluate_cli_version with the
// legacy oracle across every branch of the policy matrix.
func TestDifferentialCLIVersion(t *testing.T) {
	script := legacyModule(t, "cli_version.py")
	empty := map[string]string{}
	cases := []struct {
		name      string
		installed string
		tomlSrc   string
		lockMeta  map[string]string
	}{
		{"no-pin-no-lock", "1.2.3", "", empty},
		{"no-pin-unknown-installed", "unknown", "", empty},
		{"no-pin-synced", "1.2.3", "", map[string]string{"cli_version": "1.2.3", "synced_at": "2024-01-01T00:00:00Z"}},
		{"no-pin-stale", "1.2.4", "", map[string]string{"cli_version": "1.2.3"}},
		{"no-pin-synced-at-only", "1.2.3", "", map[string]string{"synced_at": "2024-01-01T00:00:00Z"}},
		{"pin-exact-ok-no-lock", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", empty},
		{"pin-exact-ok-synced", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", map[string]string{"cli_version": "1.2.3"}},
		{"pin-exact-stale-sync", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", map[string]string{"cli_version": "1.2.0"}},
		{"pin-exact-mismatch", "1.2.3", "[tool]\nversion = \"2.0.0\"\n", empty},
		{"pin-exact-unknown-installed", "unknown", "[tool]\nversion = \"1.2.3\"\n", empty},
		{"pin-min-ok", "1.2.3", "[tool]\nmin_version = \"1.0.0\"\n", empty},
		{"pin-min-below", "0.9.0", "[tool]\nmin_version = \"1.0.0\"\n", empty},
		{"pin-min-equal", "1.0.0", "[tool]\nmin_version = \"1.0.0\"\n", empty},
		{"pin-both", "1.2.3", "[tool]\nversion = \"1.2.3\"\nmin_version = \"1.0.0\"\n", empty},
		{"pin-empty-string", "1.2.3", "[tool]\nversion = \"\"\n", empty},
		{"pin-whitespace-string", "1.2.3", "[tool]\nversion = \"  \"\n", empty},
		{"pin-non-string", "1.2.3", "[tool]\nversion = 5\n", empty},
		{"pin-policy-exact", "1.2.3", "[tool]\nversion = \"1.2.3\"\npolicy = \"exact\"\n", empty},
		{"pin-policy-bogus", "1.2.3", "[tool]\nversion = \"1.2.3\"\npolicy = \"bogus\"\n", empty},
		{"pin-policy-wrong-kind", "1.2.3", "[tool]\nmin_version = \"1.0.0\"\npolicy = \"exact\"\n", empty},
		{"policy-without-pin", "1.2.3", "[tool]\npolicy = \"exact\"\n", empty},
		{"tool-not-a-table", "1.2.3", "tool = 5\n", empty},
		{"tool-empty", "1.2.3", "[tool]\n", empty},
		{"tool-empty-inline", "1.2.3", "tool = {}\n", empty},
		{"prerelease-below-pin", "1.0.0-rc.1", "[tool]\nversion = \"1.0.0\"\n", empty},
		{"prerelease-above-min", "1.0.0", "[tool]\nmin_version = \"1.0.0-rc.1\"\n", empty},
		{"build-metadata", "1.2.3+build.5", "[tool]\nversion = \"1.2.3+build.5\"\n", empty},
		{"junk-installed-no-pin", "not-a-version", "", empty},
	}

	type result struct {
		Severity string `json:"severity"`
		Name     string `json:"name"`
		Message  string `json:"message"`
	}
	port := make([]result, 0, len(cases))
	payload := make([]map[string]any, 0, len(cases))
	for _, tc := range cases {
		var table *toml.Table
		if tc.tomlSrc != "" {
			parsed, err := toml.Parse([]byte(tc.tomlSrc))
			if err != nil {
				t.Fatalf("%s: internal/toml.Parse: %v", tc.name, err)
			}
			table = parsed
		}
		severity, name, message := evaluateCLIVersion(tc.installed, table, tc.lockMeta)
		port = append(port, result{severity.String(), name, message})
		payload = append(payload, map[string]any{
			"installed": tc.installed,
			"toml":      tc.tomlSrc,
			"lock_meta": tc.lockMeta,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal cases: %v", err)
	}
	var legacy []result
	if err := json.Unmarshal([]byte(runPython(t, "-c", cliVersionOracle, script, string(encoded))), &legacy); err != nil {
		t.Fatalf("decode oracle output: %v", err)
	}
	if len(legacy) != len(cases) {
		t.Fatalf("oracle returned %d results for %d cases", len(legacy), len(cases))
	}
	for i, tc := range cases {
		if legacy[i] != port[i] {
			t.Errorf("%s:\n legacy %+v\n port   %+v", tc.name, legacy[i], port[i])
		}
	}
}

const cacheKeyOracle = `
import importlib.util, json, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("project_cache_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
print(json.dumps([mod.cache_key(Path(p)) for p in json.loads(sys.argv[2])]))
`

// TestDifferentialCacheKey compares the ported cache_key with project-cache.py,
// which also pins resolvePy against Path.resolve() on real paths.
func TestDifferentialCacheKey(t *testing.T) {
	script := legacyModule(t, "project-cache.py")
	dir := t.TempDir()
	paths := []string{
		dir,
		filepath.Join(dir, "sub"),
		filepath.Join(dir, "a b c"),
		filepath.Join(dir, "..-weird-.."),
		filepath.Join(dir, "sub", ".."),
		filepath.Join(dir, "ñandú"),
		"/tmp",
	}
	encoded, err := json.Marshal(paths)
	if err != nil {
		t.Fatalf("marshal paths: %v", err)
	}
	var legacy []string
	if err := json.Unmarshal([]byte(runPython(t, "-c", cacheKeyOracle, script, string(encoded))), &legacy); err != nil {
		t.Fatalf("decode oracle output: %v", err)
	}
	if len(legacy) != len(paths) {
		t.Fatalf("oracle returned %d results for %d paths", len(legacy), len(paths))
	}
	for i, p := range paths {
		if got := cacheKey(p); got != legacy[i] {
			t.Errorf("cacheKey(%q) = %q, legacy %q", p, got, legacy[i])
		}
	}
}
