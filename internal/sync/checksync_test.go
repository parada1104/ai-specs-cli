package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The legacy cli_version.py check-sync oracle was captured once into
// testdata/checksync_golden.json: per fixture, the exit code, the empty stdout
// and the stderr text byte for byte, with the temporary project root and cli
// home tokenized as <DIR> (the only host-dependent part of the payload). The
// test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const checkSyncGoldenFile = "checksync_golden.json"

// checkSyncGoldenCases is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle of
// coverage — kept in the test, never derived from the bundle — so a bundle that
// is missing, renamed, duplicated or reordered cannot validate silently.
var checkSyncGoldenCases = []string{
	"no pin",
	"matching pin",
	"matching min",
	"mismatching pin policy error",
	"below minimum policy error",
	"mismatching pin with policy warn",
	"unknown installed version",
	"missing manifest",
	"invalid manifest",
}

// checkSyncGoldenCase is one frozen fixture: the inputs pinned so drift fails as
// input drift, plus the legacy exit code, stdout and stderr.
type checkSyncGoldenCase struct {
	Name           string `json:"name"`
	Manifest       string `json:"manifest"`
	ManifestAbsent bool   `json:"manifest_absent"`
	Version        string `json:"version"`
	VersionAbsent  bool   `json:"version_absent"`
	Ignore         bool   `json:"ignore"`
	RC             int    `json:"rc"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
}

// loadCheckSyncGolden reads the frozen bundle and validates its identity
// inventory and oracle metadata before any comparison runs.
func loadCheckSyncGolden(t *testing.T) []checkSyncGoldenCase {
	t.Helper()
	var bundle struct {
		Oracle map[string]string     `json:"oracle"`
		Cases  []checkSyncGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, checkSyncGoldenFile), &bundle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", checkSyncGoldenFile, err)
	}
	if bundle.Oracle["python"] == "" {
		t.Fatalf("frozen oracle %s carries no python version metadata", checkSyncGoldenFile)
	}
	names := make([]string, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		names[i] = rec.Name
	}
	validateGoldenIdentity(t, "checksync", names, checkSyncGoldenCases)
	return bundle.Cases
}

// checkSyncCase is one corpus fixture. It writes the fixture files into a fresh
// temp dir pair and returns the project root and cli home the native leg runs
// against.
type checkSyncCase struct {
	name           string
	manifest       string // "" when the manifest is absent
	manifestAbsent bool
	version        string // "" when the VERSION file is absent
	versionAbsent  bool
	ignore         bool
}

// TestCheckSyncDifferential compares the native CheckSync port against the
// frozen legacy oracle for every fixture a sync can meet: no [tool] pin, a
// matching pin, a matching minimum, a failing pin, a failing minimum, a failing
// pin under --ignore-cli-version, an unknown installed version, a missing
// manifest and an invalid [tool]. Exit code and stderr must match byte for byte.
//
// stdout is asserted empty in the frozen payload too: the legacy shim only ever
// wrote to stderr, and the port returns no stdout at all.
func TestCheckSyncDifferential(t *testing.T) {
	frozen := loadCheckSyncGolden(t)

	const noPin = "[project]\nname = 'x'\n"

	cases := []checkSyncCase{
		{name: "no pin", manifest: noPin, version: "1.2.3\n"},
		{name: "matching pin", manifest: noPin + "\n[tool]\nversion = '1.2.3'\n", version: "1.2.3\n"},
		{name: "matching min", manifest: noPin + "\n[tool]\nmin_version = '1.0.0'\n", version: "1.2.3\n"},
		{name: "mismatching pin policy error", manifest: noPin + "\n[tool]\nversion = '2.0.0'\n", version: "1.2.3\n"},
		{name: "below minimum policy error", manifest: noPin + "\n[tool]\nmin_version = '2.0.0'\n", version: "1.2.3\n"},
		{name: "mismatching pin with policy warn", manifest: noPin + "\n[tool]\nversion = '2.0.0'\n", version: "1.2.3\n", ignore: true},
		{name: "unknown installed version", manifest: noPin + "\n[tool]\nversion = '1.2.3'\n", versionAbsent: true},
		{name: "missing manifest", manifestAbsent: true, version: "1.2.3\n"},
		{
			name:     "invalid manifest",
			manifest: noPin + "\n[tool]\nversion = '1.2.3'\nmin_version = '1.0.0'\n",
			version:  "1.2.3\n",
		},
	}

	if len(cases) != len(checkSyncGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(checkSyncGoldenCases))
	}

	for i, tc := range cases {
		if tc.name != checkSyncGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, checkSyncGoldenCases[i])
		}
		rec := frozen[i]
		if rec.Manifest != tc.manifest || rec.ManifestAbsent != tc.manifestAbsent ||
			rec.Version != tc.version || rec.VersionAbsent != tc.versionAbsent ||
			rec.Ignore != tc.ignore {
			t.Fatalf("frozen inputs differ from the corpus for %q:\n--- frozen ---\nmanifest=%t %q\nversion=%t %q\nignore=%t\n--- corpus ---\nmanifest=%t %q\nversion=%t %q\nignore=%t",
				tc.name,
				rec.ManifestAbsent, rec.Manifest, rec.VersionAbsent, rec.Version, rec.Ignore,
				tc.manifestAbsent, tc.manifest, tc.versionAbsent, tc.version, tc.ignore)
		}

		t.Run(tc.name, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			if !tc.manifestAbsent {
				writeCheckSyncFile(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), tc.manifest)
			}
			if !tc.versionAbsent {
				writeCheckSyncFile(t, filepath.Join(home, "VERSION"), tc.version)
			}

			goRC, goErr := CheckSync(root, home, tc.ignore)

			if goRC != rec.RC {
				t.Errorf("exit code: native = %d, frozen = %d (frozen stderr %q)", goRC, rec.RC, rec.Stderr)
			}
			// The legacy shim wrote to stderr only; the port returns no stdout.
			if rec.Stdout != "" {
				t.Errorf("frozen record carries stdout %q; the legacy shim only writes stderr", rec.Stdout)
			}
			gotErr := tokenizeCheckSyncDirs(goErr, root, home)
			if gotErr != rec.Stderr {
				t.Errorf("stderr mismatch:\n native = %q\n frozen = %q", gotErr, rec.Stderr)
			}
			// CheckSync only reads: the fixture must be untouched.
			if !tc.manifestAbsent {
				data, err := os.ReadFile(filepath.Join(root, "ai-specs", "ai-specs.toml"))
				if err != nil {
					t.Fatalf("re-read manifest: %v", err)
				}
				if string(data) != tc.manifest {
					t.Errorf("manifest mutated by CheckSync: %q", data)
				}
			}
			if !tc.versionAbsent {
				data, err := os.ReadFile(filepath.Join(home, "VERSION"))
				if err != nil {
					t.Fatalf("re-read VERSION: %v", err)
				}
				if string(data) != tc.version {
					t.Errorf("VERSION mutated by CheckSync: %q", data)
				}
			}
		})
	}
}

// tokenizeCheckSyncDirs replaces the temporary project root and cli home with
// <DIR> so the comparison happens in the golden's host-independent space. Both
// the dir as created and its symlink-resolved form are replaced, because
// CheckSync resolves both paths before using them (Path.resolve in the legacy
// shim), and macOS resolves /var to /private/var.
func tokenizeCheckSyncDirs(text, root, home string) string {
	var forms []string
	for _, dir := range []string{root, home} {
		forms = append(forms, dir)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
			forms = append(forms, resolved)
		}
	}
	sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	for _, form := range forms {
		text = strings.ReplaceAll(text, form, "<DIR>")
	}
	return text
}

func writeCheckSyncFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
