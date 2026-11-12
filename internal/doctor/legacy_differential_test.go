package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/toml"
)

// This file pins the ported policy helpers against the legacy Python modules
// they replace. The evaluate_cli_version oracle (lib/_internal/cli_version.py)
// was captured once into testdata/cliversion_golden.json; the test never execs
// python3 or reads lib/. The golden is a snapshot: regenerating it needs a
// human decision.

const cliVersionGoldenPath = "testdata/cliversion_golden.json"

type cliVersionCase struct {
	name      string
	installed string
	tomlSrc   string
	lockMeta  map[string]string
}

type cliVersionResult struct {
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Message  string `json:"message"`
}

type cliVersionGoldenCase struct {
	Name      string            `json:"name"`
	Installed string            `json:"installed"`
	TOML      string            `json:"toml"`
	LockMeta  map[string]string `json:"lock_meta"`
	Result    cliVersionResult  `json:"result"`
}

type cliVersionGolden struct {
	Oracle map[string]any         `json:"oracle"`
	Cases  []cliVersionGoldenCase `json:"cases"`
}

var emptyLockMeta = map[string]string{}

// cliVersionCases covers every branch of the evaluate_cli_version policy matrix.
var cliVersionCases = []cliVersionCase{
	{"no-pin-no-lock", "1.2.3", "", emptyLockMeta},
	{"no-pin-unknown-installed", "unknown", "", emptyLockMeta},
	{"no-pin-synced", "1.2.3", "", map[string]string{"cli_version": "1.2.3", "synced_at": "2024-01-01T00:00:00Z"}},
	{"no-pin-stale", "1.2.4", "", map[string]string{"cli_version": "1.2.3"}},
	{"no-pin-synced-at-only", "1.2.3", "", map[string]string{"synced_at": "2024-01-01T00:00:00Z"}},
	{"pin-exact-ok-no-lock", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", emptyLockMeta},
	{"pin-exact-ok-synced", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", map[string]string{"cli_version": "1.2.3"}},
	{"pin-exact-stale-sync", "1.2.3", "[tool]\nversion = \"1.2.3\"\n", map[string]string{"cli_version": "1.2.0"}},
	{"pin-exact-mismatch", "1.2.3", "[tool]\nversion = \"2.0.0\"\n", emptyLockMeta},
	{"pin-exact-unknown-installed", "unknown", "[tool]\nversion = \"1.2.3\"\n", emptyLockMeta},
	{"pin-min-ok", "1.2.3", "[tool]\nmin_version = \"1.0.0\"\n", emptyLockMeta},
	{"pin-min-below", "0.9.0", "[tool]\nmin_version = \"1.0.0\"\n", emptyLockMeta},
	{"pin-min-equal", "1.0.0", "[tool]\nmin_version = \"1.0.0\"\n", emptyLockMeta},
	{"pin-both", "1.2.3", "[tool]\nversion = \"1.2.3\"\nmin_version = \"1.0.0\"\n", emptyLockMeta},
	{"pin-empty-string", "1.2.3", "[tool]\nversion = \"\"\n", emptyLockMeta},
	{"pin-whitespace-string", "1.2.3", "[tool]\nversion = \"  \"\n", emptyLockMeta},
	{"pin-non-string", "1.2.3", "[tool]\nversion = 5\n", emptyLockMeta},
	{"pin-policy-exact", "1.2.3", "[tool]\nversion = \"1.2.3\"\npolicy = \"exact\"\n", emptyLockMeta},
	{"pin-policy-bogus", "1.2.3", "[tool]\nversion = \"1.2.3\"\npolicy = \"bogus\"\n", emptyLockMeta},
	{"pin-policy-wrong-kind", "1.2.3", "[tool]\nmin_version = \"1.0.0\"\npolicy = \"exact\"\n", emptyLockMeta},
	{"policy-without-pin", "1.2.3", "[tool]\npolicy = \"exact\"\n", emptyLockMeta},
	{"tool-not-a-table", "1.2.3", "tool = 5\n", emptyLockMeta},
	{"tool-empty", "1.2.3", "[tool]\n", emptyLockMeta},
	{"tool-empty-inline", "1.2.3", "tool = {}\n", emptyLockMeta},
	{"prerelease-below-pin", "1.0.0-rc.1", "[tool]\nversion = \"1.0.0\"\n", emptyLockMeta},
	{"prerelease-above-min", "1.0.0", "[tool]\nmin_version = \"1.0.0-rc.1\"\n", emptyLockMeta},
	{"build-metadata", "1.2.3+build.5", "[tool]\nversion = \"1.2.3+build.5\"\n", emptyLockMeta},
	{"junk-installed-no-pin", "not-a-version", "", emptyLockMeta},
}

// expectedCLIVersionGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedCLIVersionGoldenCases = []string{
	"no-pin-no-lock", "no-pin-unknown-installed", "no-pin-synced", "no-pin-stale",
	"no-pin-synced-at-only", "pin-exact-ok-no-lock", "pin-exact-ok-synced",
	"pin-exact-stale-sync", "pin-exact-mismatch", "pin-exact-unknown-installed",
	"pin-min-ok", "pin-min-below", "pin-min-equal", "pin-both", "pin-empty-string",
	"pin-whitespace-string", "pin-non-string", "pin-policy-exact", "pin-policy-bogus",
	"pin-policy-wrong-kind", "policy-without-pin", "tool-not-a-table", "tool-empty",
	"tool-empty-inline", "prerelease-below-pin", "prerelease-above-min",
	"build-metadata", "junk-installed-no-pin",
}

func evaluateCLIVersionCase(t *testing.T, tc cliVersionCase) cliVersionResult {
	t.Helper()
	var table *toml.Table
	if tc.tomlSrc != "" {
		parsed, err := toml.Parse([]byte(tc.tomlSrc))
		if err != nil {
			t.Fatalf("%s: internal/toml.Parse: %v", tc.name, err)
		}
		table = parsed
	}
	severity, name, message := evaluateCLIVersion(tc.installed, table, tc.lockMeta)
	return cliVersionResult{severity.String(), name, message}
}

func loadCLIVersionGolden(t *testing.T) cliVersionGolden {
	t.Helper()
	raw, err := os.ReadFile(cliVersionGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", cliVersionGoldenPath, err)
	}
	var golden cliVersionGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", cliVersionGoldenPath, err)
	}
	return golden
}

// TestDifferentialCLIVersion compares the ported evaluate_cli_version with the
// frozen legacy oracle across every branch of the policy matrix.
func TestDifferentialCLIVersion(t *testing.T) {
	golden := loadCLIVersionGolden(t)

	if len(cliVersionCases) != len(expectedCLIVersionGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cliVersionCases), len(expectedCLIVersionGoldenCases))
	}
	if len(golden.Cases) != len(expectedCLIVersionGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedCLIVersionGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedCLIVersionGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || cliVersionCases[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, cliVersionCases[i].name, want)
		}
	}

	for i, tc := range cliVersionCases {
		frozen := golden.Cases[i]
		if frozen.Installed != tc.installed || frozen.TOML != tc.tomlSrc || !reflect.DeepEqual(frozen.LockMeta, tc.lockMeta) {
			t.Errorf("%s: frozen inputs %+v differ from corpus %+v", tc.name, frozen, tc)
			continue
		}
		if got := evaluateCLIVersionCase(t, tc); got != frozen.Result {
			t.Errorf("%s:\n legacy %+v\n port   %+v", tc.name, frozen.Result, got)
		}
	}
}

// TestDifferentialCacheKey pins doctor's cacheKey for real paths. The legacy
// project-cache.py output hashes the absolute resolved TempDir, so it is
// host-dependent and cannot be frozen; cacheKey delegates to
// projectcache.CacheKey, whose "/" value and ResolvePath-vs-realpath semantics
// are frozen in internal/projectcache. This test pins the derivation for the
// same paths the live differential used.
func TestDifferentialCacheKey(t *testing.T) {
	if got := cacheKey("/"); got != "8a5edab28263-project" {
		t.Errorf("cacheKey(\"/\") = %q, want frozen %q", got, "8a5edab28263-project")
	}
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
	for _, p := range paths {
		resolved := projectcache.ResolvePath(p)
		sum := sha256.Sum256([]byte(resolved))
		want := hex.EncodeToString(sum[:])[:12] + "-" + projectcache.SanitizeBasename(filepath.Base(resolved))
		if got := cacheKey(p); got != want {
			t.Errorf("cacheKey(%q) = %q, want %q", p, got, want)
		}
	}
	if cacheKey(filepath.Join(dir, "sub", "..")) != cacheKey(dir) {
		t.Errorf("cacheKey must resolve %q to the same key as %q", filepath.Join(dir, "sub", ".."), dir)
	}
}
