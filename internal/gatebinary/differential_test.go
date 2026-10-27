// Differential between the native Acquire and the frozen legacy oracle of
// Python's gate_binary.acquire (T7/C2c, T5). The oracle was captured once
// through the legacy driver on the same fixture layout; the test replays every
// case against the native Acquire and compares the status projection and the
// cache tree byte for byte with the frozen record. No Python runs here.
//
// The decided same-user divergences are excluded on purpose and covered by the
// package's own tests instead:
//
//   - no committed digest: the native acquisition fails closed (76541ae) while
//     Python attempts the local build;
//   - cache ownership/permissions: the native acquisition refuses a cache that
//     breaks the same-user boundary; Python proceeds;
//   - cache directory modes: the native acquisition creates 0700 directories,
//     Python uses the umask default.
package gatebinary

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// acquireGoldenPath is the frozen oracle: one record per case, keyed by case
// name, each holding the status projection and the cache tree after acquire.
const acquireGoldenPath = "testdata/acquire_golden.json"

// acquireGoldenCases is the complete case list; the golden must hold exactly
// one record per name.
var acquireGoldenCases = []string{
	"01-offline-no-cached-binary",
	"02-offline-local-build-no-source",
	"03-download-installs-verified",
	"04-download-digest-mismatch",
	"05-unsupported-platform",
}

type acquireRecord struct {
	Status map[string]any    `json:"status"`
	Tree   map[string]string `json:"tree"`
}

type acquireGolden struct {
	Cases map[string]acquireRecord `json:"cases"`
}

// differentialStatus is the status projection the oracle compares on: the keys
// Python's acquire documents (the Go Verification evidence and the Go-only
// boundary behavior are out of scope).
type differentialStatus struct {
	Attempted bool      `json:"attempted"`
	Installed bool      `json:"installed"`
	Platform  [2]string `json:"platform"`
	CachePath string    `json:"cache_path"`
	Warn      *string   `json:"warn"`
	Mismatch  *string   `json:"mismatch"`
}

func projectAcquireStatus(status AcquireStatus) map[string]any {
	return map[string]any{
		"attempted":  status.Attempted,
		"installed":  status.Installed,
		"platform":   status.Platform,
		"cache_path": status.CachePath,
		"warn":       status.Warn,
		"mismatch":   status.Mismatch,
	}
}

func normalizedJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// diffTree maps every regular file under <home>/cache to its bytes, so the
// installed binary, the receipt and the mismatch record compare byte for byte
// (directory modes are excluded: they are part of the decided divergence).
func diffTree(t *testing.T, home string) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := filepath.Join(home, "cache")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk cache: %v", err)
	}
	return files
}

// acquireTokens holds the per-run values the oracle replaces by fixed tokens:
// the scratch home (raw and symlink-resolved forms), the fake-go directory and
// the loopback asset server URL. Every other host-shaped string is compared as
// written.
type acquireTokens struct {
	home  string
	tools string
	base  string
}

func (k acquireTokens) text(s string) string {
	if k.home != "" {
		s = strings.ReplaceAll(s, k.home, "<HOME>")
	}
	if k.tools != "" {
		s = strings.ReplaceAll(s, k.tools, "<TOOLS>")
	}
	if k.base != "" {
		s = strings.ReplaceAll(s, k.base, "<BASE>")
	}
	return s
}

func (k acquireTokens) value(v any) any {
	switch x := v.(type) {
	case string:
		return k.text(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = k.value(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, item := range x {
			out[key] = k.value(item)
		}
		return out
	default:
		return v
	}
}

func (k acquireTokens) tree(files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for rel, content := range files {
		out[rel] = k.text(content)
	}
	return out
}

func loadAcquireGolden(t *testing.T) acquireGolden {
	t.Helper()
	raw, err := os.ReadFile(acquireGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", acquireGoldenPath, err)
	}
	var golden acquireGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", acquireGoldenPath, err)
	}
	if len(golden.Cases) != len(acquireGoldenCases) {
		t.Fatalf("frozen oracle holds %d records; the cases need exactly %d",
			len(golden.Cases), len(acquireGoldenCases))
	}
	for _, name := range acquireGoldenCases {
		if _, ok := golden.Cases[name]; !ok {
			t.Fatalf("frozen oracle has no record for case %s", name)
		}
	}
	return golden
}

// resolvedHome is the scratch home in its symlink-resolved form: the native
// Acquire reports resolved paths, so the tokens must match that form.
func resolvedHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(newHome(t))
	if err != nil {
		t.Fatalf("resolve home: %v", err)
	}
	return home
}

func TestAcquireDifferentialAgainstFrozenOracle(t *testing.T) {
	t.Setenv("AI_SPECS_GATE_BUILD", "")
	golden := loadAcquireGolden(t)
	fakeGoDir := t.TempDir()
	writeExec(t, filepath.Join(fakeGoDir, "go"), "#!/bin/sh\nexit 1\n")
	srv, _ := assetServer(t, gateScript())
	platform := [2]string{"darwin", "arm64"}

	// run executes the native Acquire on a fresh home and compares the status
	// projection and the cache tree with the frozen record of the same case.
	run := func(name, digest string, opts AcquireOptions, goos, goarch string, tools string) {
		t.Run(name, func(t *testing.T) {
			home := resolvedHome(t)
			if digest != "" {
				writeSums(t, home, digest, AssetName(goos, goarch))
			}
			goOpts := opts
			goOpts.GateImpl = "auto"
			goOpts.Home = home
			goOpts.GOOS, goOpts.GOARCH = goos, goarch
			keys := acquireTokens{home: home, tools: tools, base: opts.BaseURL}
			got := keys.value(normalizedJSON(t, projectAcquireStatus(Acquire(goOpts))))
			gotTree := keys.tree(diffTree(t, home))

			record := golden.Cases[name]
			wantStatus := record.Status
			if !reflect.DeepEqual(got, wantStatus) {
				t.Fatalf("status:\n--- go ---\n%v\n--- frozen oracle ---\n%v", got, wantStatus)
			}
			if !reflect.DeepEqual(gotTree, record.Tree) {
				t.Fatalf("cache tree:\n--- go ---\n%v\n--- frozen oracle ---\n%v", gotTree, record.Tree)
			}
		})
	}

	// Shared branch: offline, no cached binary, no local-build tool — the
	// missing-digest branch is never reached.
	run(acquireGoldenCases[0], "",
		AcquireOptions{Offline: true, LookPath: func(string) (string, error) {
			return "", errors.New("no go")
		}},
		platform[0], platform[1], "")

	// Shared branch: offline with a toolchain and a committed digest, but no
	// gate Go source in the home — both graders fail the build the same way.
	run(acquireGoldenCases[1], strings.Repeat("a", 64),
		AcquireOptions{Offline: true, LookPath: func(string) (string, error) {
			return filepath.Join(fakeGoDir, "go"), nil
		}},
		platform[0], platform[1], fakeGoDir)

	// Shared branch: the download installs the digest-verified binary plus its
	// receipt, byte for byte.
	run(acquireGoldenCases[2], sha256Hex(gateScript()),
		AcquireOptions{BaseURL: srv.URL}, platform[0], platform[1], "")

	// Shared branch: a downloaded artifact that misses the committed digest is
	// deleted, recorded and never executed.
	run(acquireGoldenCases[3], strings.Repeat("a", 64),
		AcquireOptions{BaseURL: srv.URL}, platform[0], platform[1], "")

	// Shared branch: a platform outside the published matrix warns and
	// acquires nothing.
	run(acquireGoldenCases[4], "",
		AcquireOptions{}, "solaris", "riscv", "")
}

// Negative control: the normalizer replaces only the exact home, tools and
// asset URL; any other host-shaped string survives as written.
func TestAcquireNormalizeLeavesOtherStringsAlone(t *testing.T) {
	k := acquireTokens{home: "/scratch/home", tools: "/scratch/tools", base: "http://127.0.0.1:1"}
	got := k.text("/scratch/home/cache and /scratch/homeextra and /elsewhere, /scratch/tools, http://127.0.0.1:1/v9")
	want := "<HOME>/cache and <HOME>extra and /elsewhere, <TOOLS>, <BASE>/v9"
	if got != want {
		t.Fatalf("normalized %q, want %q", got, want)
	}
}
