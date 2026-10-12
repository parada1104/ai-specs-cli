package provider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/schema"
)

// --- fixtures ---------------------------------------------------------------

// requireTarget returns the runtime provider target or skips the caller when the
// host has no supported artifact (detect_platform's ("","")).
func requireTarget(t *testing.T) (string, string) {
	t.Helper()
	goos, goarch := DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		t.Skipf("unsupported provider platform %q/%q", goos, goarch)
	}
	return goos, goarch
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type receiptOverrides struct {
	status     *string
	repository *string
	target     *string
	releaseTag *string
	digest     *string
	omitTag    bool
	omitDigest bool
}

func strp(s string) *string { return &s }

func writeReceipt(t *testing.T, bin string, receipt map[string]any) {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(bin), "install.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeManaged materializes <home>/cache/bin/jinna/<versionDir>/<target>/jinna
// and its install.json receipt, applying the overrides.
func writeManaged(t *testing.T, home, versionDir, printedVersion string, ov receiptOverrides) {
	t.Helper()
	goos, goarch := requireTarget(t)
	bin := ManagedBinaryPath(filepath.Join(home, "cache", "bin", "jinna"), versionDir, goos, goarch)
	writeExecutable(t, bin, "#!/bin/sh\necho jinna "+printedVersion+"\n")
	receipt := map[string]any{
		"status":        "verified",
		"repository":    Repository,
		"target":        goos + "-" + goarch,
		"release_tag":   "v" + versionDir,
		"binary_sha256": sha256Hex(t, bin),
	}
	if ov.status != nil {
		receipt["status"] = *ov.status
	}
	if ov.repository != nil {
		receipt["repository"] = *ov.repository
	}
	if ov.target != nil {
		receipt["target"] = *ov.target
	}
	if ov.releaseTag != nil {
		receipt["release_tag"] = *ov.releaseTag
	}
	if ov.digest != nil {
		receipt["binary_sha256"] = *ov.digest
	}
	if ov.omitTag {
		delete(receipt, "release_tag")
	}
	if ov.omitDigest {
		delete(receipt, "binary_sha256")
	}
	writeReceipt(t, bin, receipt)
}

// --- pure helpers -----------------------------------------------------------

// TestVersionTuple pins provider_install._version_tuple, including the
// digit-boundary rule RE2 cannot express directly.
func TestVersionTuple(t *testing.T) {
	cases := []struct{ text, want string }{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"release v0.1.0", "0.1.0"},
		{"123", ""},
		{"", ""},
		{"a1.2.3", "1.2.3"},
		{"3v1.2.3", "1.2.3"},
		{"1v2.3", "2.3"},
		{"x99.4.5", "99.4.5"},
	}
	for _, tc := range cases {
		if got := joinVersion(VersionTuple(tc.text)); got != tc.want {
			t.Errorf("VersionTuple(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestVersionTupleDivergences pins the documented divergences with their
// direction: Python is exact (unbounded int, Unicode `\d`) while the Go port
// saturates and only accepts ASCII digits.
func TestVersionTupleDivergences(t *testing.T) {
	if got := VersionTuple("99999999999999999999.1"); !reflect.DeepEqual(got, []int{math.MaxInt64, 1}) {
		t.Errorf("overflow tuple = %v, want [%d 1] (Python returns the unbounded int)", got, int64(math.MaxInt64))
	}
	if got := VersionTuple("\u0661\u0662.\u0663"); got != nil {
		t.Errorf("Arabic-Indic digits = %v, want nil (Python's \\d accepts them)", got)
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		have, min string
		want      bool
	}{
		{"2.0.0", "1.9.9", true},
		{"1.9.9", "2.0.0", false},
		{"1.2", "1.2.0", false}, // raw tuple: (1,2) < (1,2,0), unlike the zero-padded dep_check
		{"1.2.0", "1.2", true},
		{"", "1.0.0", false},
		{"1.0.0", "", true},
	}
	for _, tc := range cases {
		if got := versionAtLeast(tc.have, tc.min); got != tc.want {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.have, tc.min, got, tc.want)
		}
	}
}

func TestDetectPlatform(t *testing.T) {
	cases := []struct{ system, machine, goos, goarch string }{
		{"darwin", "arm64", "darwin", "arm64"},
		{"macOS", "aarch64", "darwin", "arm64"},
		{"Linux", "x86_64", "linux", "amd64"},
		{"windows", "AMD64", "windows", "amd64"},
		{"plan9", "arm64", "", ""},
		{"linux", "mips", "", ""},
	}
	for _, tc := range cases {
		system, machine := tc.system, tc.machine
		goos, goarch := DetectPlatform(&system, &machine)
		if goos != tc.goos || goarch != tc.goarch {
			t.Errorf("DetectPlatform(%q, %q) = %q/%q, want %q/%q", tc.system, tc.machine, goos, goarch, tc.goos, tc.goarch)
		}
	}
	// A pointer to "" is Python's provided empty string, distinct from nil.
	empty := ""
	if goos, goarch := DetectPlatform(&empty, &empty); goos != "" || goarch != "" {
		t.Errorf("DetectPlatform(&\"\", &\"\") = %q/%q, want unsupported", goos, goarch)
	}
}

func TestExpectedBinaryNameAndManagedPath(t *testing.T) {
	if got := ExpectedBinaryName("windows"); got != "jinna.exe" {
		t.Errorf("ExpectedBinaryName(windows) = %q", got)
	}
	if got := ExpectedBinaryName("linux"); got != "jinna" {
		t.Errorf("ExpectedBinaryName(linux) = %q", got)
	}
	if got := ManagedBinaryPath("r", "1.2.3", "linux", "amd64"); got != filepath.Join("r", "1.2.3", "linux-amd64", "jinna") {
		t.Errorf("ManagedBinaryPath = %q", got)
	}
}

// TestCacheRoot pins provider_install.cache_root: explicit argument, then
// AI_SPECS_HOME, then ~/.cache/ai-specs, each expanduser + resolve(strict=False).
func TestCacheRoot(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	want := filepath.Join(resolvedReal, "cache", "bin", "jinna")

	if got := CacheRoot(&link); got != want {
		t.Errorf("CacheRoot(link) = %q, want %q", got, want)
	}
	t.Setenv("AI_SPECS_HOME", link)
	if got := CacheRoot(nil); got != want {
		t.Errorf("CacheRoot(env link) = %q, want %q", got, want)
	}

	// An empty AI_SPECS_HOME is falsy in Python and falls through to the home
	// default; resolvePath resolves the existing prefix of a missing suffix.
	home := t.TempDir()
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AI_SPECS_HOME", "")
	want = filepath.Join(resolvedHome, ".cache", "ai-specs", "cache", "bin", "jinna")
	if got := CacheRoot(nil); got != want {
		t.Errorf("CacheRoot(default) = %q, want %q", got, want)
	}
}

// --- resolution -------------------------------------------------------------

func TestResolvePathSource(t *testing.T) {
	requireTarget(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "jinna")
	writeExecutable(t, bin, "#!/bin/sh\necho jinna 1.4.2\n")
	t.Setenv("PATH", dir)

	res, err := Resolve(&schema.CliDep{Binary: "jinna", MinVersion: "1.0.0"}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Source != SourcePath || !res.Verified || res.Version != "1.4.2" {
		t.Fatalf("resolution = %+v", res)
	}
	if res.Command != "jinna" || res.Path == nil || *res.Path != bin {
		t.Fatalf("command/path = %q/%v, want jinna/%q", res.Command, res.Path, bin)
	}
	if res.Repository != Repository {
		t.Fatalf("repository = %q", res.Repository)
	}
}

func TestResolveManagedVerified(t *testing.T) {
	goos, goarch := requireTarget(t)
	home := t.TempDir()
	writeManaged(t, home, "2.1.0", "2.1.0", receiptOverrides{})
	t.Setenv("PATH", t.TempDir())

	res, err := Resolve(&schema.CliDep{Binary: "jinna"}, &home)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantPath := ManagedBinaryPath(CacheRoot(&home), "2.1.0", goos, goarch)
	if res.Source != SourceManaged || !res.Verified || res.Version != "2.1.0" || res.ReleaseTag != "v2.1.0" {
		t.Fatalf("resolution = %+v", res)
	}
	if res.Command != wantPath || res.Path == nil || *res.Path != wantPath {
		t.Fatalf("command/path = %q/%v, want %q", res.Command, res.Path, wantPath)
	}
}

// TestResolveReceiptRejections pins every trust-check rejection in the managed
// scan: a single rejected candidate degrades to unresolved.
func TestResolveReceiptRejections(t *testing.T) {
	cases := []struct {
		name       string
		versionDir string
		printed    string
		minVersion string
		ov         receiptOverrides
	}{
		{"status", "1.2.3", "1.2.3", "", receiptOverrides{status: strp("failed")}},
		{"repository", "1.2.3", "1.2.3", "", receiptOverrides{repository: strp("someone/else")}},
		{"target", "1.2.3", "1.2.3", "", receiptOverrides{target: strp("plan9-mips")}},
		{"tag-non-version", "1.2.3", "1.2.3", "", receiptOverrides{releaseTag: strp("nightly")}},
		{"tag-missing", "1.2.3", "1.2.3", "", receiptOverrides{omitTag: true}},
		{"digest-missing", "1.2.3", "1.2.3", "", receiptOverrides{omitDigest: true}},
		{"digest-mismatch", "1.2.3", "1.2.3", "", receiptOverrides{digest: strp(strings.Repeat("0", 64))}},
		{"version-tag-mismatch", "1.2.3", "1.2.4", "", receiptOverrides{}},
		{"below-min", "1.2.3", "1.2.3", "2.0.0", receiptOverrides{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeManaged(t, home, tc.versionDir, tc.printed, tc.ov)
			t.Setenv("PATH", t.TempDir())

			res, err := Resolve(&schema.CliDep{Binary: "jinna", MinVersion: tc.minVersion}, &home)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if res.Verified || res.Source != SourceUnresolved || res.Version != "" {
				t.Fatalf("resolution = %+v, want unresolved", res)
			}
		})
	}
}

// TestResolveKeepsSetButEmptyBinary pins gap 2: Python getattr returns the
// set-but-empty binary rather than defaulting to jinna.
func TestResolveKeepsSetButEmptyBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	res, err := Resolve(&schema.CliDep{Binary: ""}, &home)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Binary != "" || res.Command != "" || res.Source != SourceUnresolved {
		t.Fatalf("resolution = %+v, want an empty binary kept", res)
	}
}

// TestResolveWinnerRecheckFailure pins gap 4: the winner re-check runs outside
// every try in Python, so its InstallError propagates.
func TestResolveWinnerRecheckFailure(t *testing.T) {
	goos, goarch := requireTarget(t)
	home := t.TempDir()
	counter := filepath.Join(t.TempDir(), "count")
	bin := ManagedBinaryPath(CacheRoot(&home), "1.2.3", goos, goarch)
	writeExecutable(t, bin, "#!/bin/sh\nif [ -f \""+counter+"\" ]; then exit 1; fi\n: > \""+counter+"\"\necho jinna 1.2.3\n")
	writeReceipt(t, bin, map[string]any{
		"status": "verified", "repository": Repository, "target": goos + "-" + goarch,
		"release_tag": "v1.2.3", "binary_sha256": sha256Hex(t, bin),
	})
	t.Setenv("PATH", t.TempDir())

	if _, err := Resolve(&schema.CliDep{Binary: "jinna"}, &home); err == nil ||
		err.Error() != "provider version check returned no usable version" {
		t.Fatalf("err = %v, want the winner re-check InstallError", err)
	}
}

// TestRunVersionMessages pins provider_install._run_version's error strings,
// including the Python str(exc) reproductions for the timeout and OS errors.
func TestRunVersionMessages(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent")
	if _, err := runVersion(missing, time.Second); err == nil ||
		err.Error() != "provider version check failed: [Errno 2] No such file or directory: '"+missing+"'" {
		t.Errorf("missing err = %v", err)
	}

	notExec := filepath.Join(dir, "not-exec")
	if err := os.WriteFile(notExec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runVersion(notExec, time.Second); err == nil ||
		err.Error() != "provider version check failed: [Errno 13] Permission denied: '"+notExec+"'" {
		t.Errorf("permission err = %v", err)
	}

	noVersion := filepath.Join(dir, "jinna-noversion")
	writeExecutable(t, noVersion, "#!/bin/sh\necho hello\n")
	if _, err := runVersion(noVersion, time.Second); err == nil ||
		err.Error() != "provider version check returned no usable version" {
		t.Errorf("no-version err = %v", err)
	}

	nonzero := filepath.Join(dir, "jinna-nonzero")
	writeExecutable(t, nonzero, "#!/bin/sh\necho jinna 1.2.3\nexit 1\n")
	if _, err := runVersion(nonzero, time.Second); err == nil ||
		err.Error() != "provider version check returned no usable version" {
		t.Errorf("nonzero err = %v", err)
	}

	slow := filepath.Join(dir, "jinna-slow")
	writeExecutable(t, slow, "#!/bin/sh\nsleep 5\n")
	if _, err := runVersion(slow, 100*time.Millisecond); err == nil ||
		err.Error() != "provider version check failed: Command '["+pyRepr(slow)+", 'version']' timed out after 0.1 seconds" {
		t.Errorf("timeout err = %v", err)
	}
}

// --- Python differential ----------------------------------------------------

func providerRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "provider_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

func providerPythonAuthority(t *testing.T) (python, authority string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	authority = filepath.Join(root, "lib", "_internal", "provider_install.py")
	if _, err := os.Stat(authority); err != nil {
		t.Skipf("provider_install.py not available under %s: %v", root, err)
	}
	return python, authority
}

func runProviderRef(t *testing.T, python, driver string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(python, append([]string{driver}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.Bytes()
}

type providerRefResult struct {
	Binary     string   `json:"binary"`
	Command    string   `json:"command"`
	Version    string   `json:"version"`
	Source     string   `json:"source"`
	Path       *string  `json:"path"`
	Repository string   `json:"repository"`
	ReleaseTag string   `json:"release_tag"`
	Target     []string `json:"target"`
	Verified   bool     `json:"verified"`
	Error      string   `json:"error"`
}

type providerRefVersion struct {
	Version string `json:"version"`
	Error   string `json:"error"`
}

func assertResolutionMatchesRef(t *testing.T, got Resolution, ref providerRefResult) {
	t.Helper()
	if got.Binary != ref.Binary || got.Command != ref.Command || got.Version != ref.Version ||
		got.Source != ref.Source || got.Repository != ref.Repository || got.ReleaseTag != ref.ReleaseTag ||
		got.Verified != ref.Verified {
		t.Fatalf("resolution diverges\n go = %+v\n py = %+v", got, ref)
	}
	if (got.Path == nil) != (ref.Path == nil) || (got.Path != nil && *got.Path != *ref.Path) {
		t.Fatalf("path diverges: go=%v python=%v", got.Path, ref.Path)
	}
	if got.Target[0] != ref.Target[0] || got.Target[1] != ref.Target[1] {
		t.Fatalf("target diverges: go=%v python=%v", got.Target, ref.Target)
	}
}

// TestProviderResolveDifferentialAgainstPython pins Resolve against the real
// provider_install.resolve_provider on every shared fixture: the PATH source,
// the managed source with a verified receipt, each receipt rejection, the
// min_version gate, and the set-but-empty binary.
func TestProviderResolveDifferentialAgainstPython(t *testing.T) {
	python, authority := providerPythonAuthority(t)
	driver := providerRefPath(t)

	type managedCase struct {
		versionDir string
		printed    string
		ov         receiptOverrides
	}
	cases := []struct {
		name        string
		binary      string
		minVersion  string
		pathVersion string
		managed     *managedCase
	}{
		{"path source", "jinna", "", "1.4.2", nil},
		{"path source min satisfied", "jinna", "1.0.0", "1.4.2", nil},
		{"path below min falls through", "jinna", "2.0.0", "1.4.2", nil},
		{"path unparseable falls through", "jinna", "", "hello", nil},
		{"managed verified", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{}}},
		{"managed status rejected", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{status: strp("failed")}}},
		{"managed repository rejected", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{repository: strp("someone/else")}}},
		{"managed target rejected", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{target: strp("plan9-mips")}}},
		{"managed tag non-version", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{releaseTag: strp("nightly")}}},
		{"managed digest mismatch", "jinna", "", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{digest: strp(strings.Repeat("0", 64))}}},
		{"managed version-tag mismatch", "jinna", "", "", &managedCase{"2.1.0", "2.2.0", receiptOverrides{}}},
		{"managed below min", "jinna", "3.0.0", "", &managedCase{"2.1.0", "2.1.0", receiptOverrides{}}},
		{"unresolved empty cache", "jinna", "", "", nil},
		{"empty binary kept", "", "", "", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			pathDir := t.TempDir()
			if tc.pathVersion != "" {
				writeExecutable(t, filepath.Join(pathDir, "jinna"), "#!/bin/sh\necho jinna "+tc.pathVersion+"\n")
			}
			t.Setenv("PATH", pathDir)
			if tc.managed != nil {
				writeManaged(t, home, tc.managed.versionDir, tc.managed.printed, tc.managed.ov)
			}

			raw := runProviderRef(t, python, driver, "resolve", authority, tc.binary, tc.minVersion, home)
			var ref providerRefResult
			if err := json.Unmarshal(raw, &ref); err != nil {
				t.Fatalf("decode ref %s: %v", raw, err)
			}
			got, err := Resolve(&schema.CliDep{Binary: tc.binary, MinVersion: tc.minVersion}, &home)
			if ref.Error != "" {
				if err == nil || err.Error() != ref.Error {
					t.Fatalf("error diverges: go=%v python=%q", err, ref.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected go error: %v (python %+v)", err, ref)
			}
			assertResolutionMatchesRef(t, got, ref)
		})
	}
}

// TestRunVersionDifferentialAgainstPython pins the error strings of the
// reproduced cases (missing path, permission, no version, non-zero exit,
// timeout) to the real provider_install._run_version.
func TestRunVersionDifferentialAgainstPython(t *testing.T) {
	python, authority := providerPythonAuthority(t)
	driver := providerRefPath(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "absent")
	noVersion := filepath.Join(dir, "jinna-noversion")
	writeExecutable(t, noVersion, "#!/bin/sh\necho hello\n")
	nonzero := filepath.Join(dir, "jinna-nonzero")
	writeExecutable(t, nonzero, "#!/bin/sh\necho jinna 1.2.3\nexit 1\n")
	ok := filepath.Join(dir, "jinna-ok")
	writeExecutable(t, ok, "#!/bin/sh\necho jinna 3.4.5\n")
	slow := filepath.Join(dir, "jinna-slow")
	writeExecutable(t, slow, "#!/bin/sh\nsleep 5\n")

	cases := []struct{ name, path, timeout string }{
		{"ok", ok, "5"},
		{"missing", missing, "5"},
		{"no-version", noVersion, "5"},
		{"nonzero-exit", nonzero, "5"},
		{"timeout", slow, "0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := runProviderRef(t, python, driver, "runversion", authority, tc.path, tc.timeout)
			var ref providerRefVersion
			if err := json.Unmarshal(raw, &ref); err != nil {
				t.Fatalf("decode ref %s: %v", raw, err)
			}
			timeout, err := time.ParseDuration(tc.timeout + "s")
			if err != nil {
				t.Fatal(err)
			}
			got, err := runVersion(tc.path, timeout)
			if ref.Error != "" {
				if err == nil || err.Error() != ref.Error {
					t.Fatalf("error diverges: go=%v python=%q", err, ref.Error)
				}
				return
			}
			if err != nil || got != ref.Version {
				t.Fatalf("version diverges: go=%q err=%v python=%q", got, err, ref.Version)
			}
		})
	}
}
