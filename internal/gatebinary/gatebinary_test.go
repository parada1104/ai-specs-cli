package gatebinary

// Tests for the native port of lib/_internal/gate_binary.py, mirroring
// tests/test_gate_binary_dist.py symptom by symptom. No test reaches the
// network: downloads go through an httptest server (or a deliberately dead
// one) and the local-build test uses a fake `go` on the injected LookPath.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- helpers ----------------------------------------------------------------

func newHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "VERSION"), "9.9.9\n")
	if err := os.MkdirAll(filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	return home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeExec(t *testing.T, path, content string) {
	t.Helper()
	writeFile(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func writeSums(t *testing.T, home, digest, asset string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS"),
		digest+"  "+asset+"\n")
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// gateScriptBody is the fixture shell binary answering --version and a passing
// --selftest.
const gateScriptBody = "#!/bin/sh\ncase \"$1\" in\n" +
	"  --version) printf '9.9.9\\n' ;;\n" +
	"  --selftest) exit 0 ;;\n" +
	"esac\nexit 0\n"

func gateScript() []byte { return []byte(gateScriptBody) }

// assetServer serves the asset payload; the returned counter records requests.
func assetServer(t *testing.T, payload []byte) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func darwinOpts(home, impl string) AcquireOptions {
	return AcquireOptions{GateImpl: impl, Home: home, GOOS: "darwin", GOARCH: "arm64"}
}

func withBaseURL(opts AcquireOptions, baseURL string) AcquireOptions {
	opts.BaseURL = baseURL
	return opts
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%q does not contain %q", haystack, needle)
	}
}

func deref(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("expected a non-nil string, got nil")
	}
	return *value
}

// --- platform mapping + cache path -----------------------------------------

func TestDetectPlatformMatrixAndRosetta(t *testing.T) {
	cases := []struct{ system, machine, wantOS, wantArch string }{
		{"Darwin", "arm64", "darwin", "arm64"},
		{"Darwin", "x86_64", "darwin", "amd64"}, // Rosetta on Apple Silicon
		{"Linux", "x86_64", "linux", "amd64"},
		{"Linux", "amd64", "linux", "amd64"},
		{"Linux", "aarch64", "linux", "arm64"},
	}
	for _, c := range cases {
		goos, goarch := DetectPlatform(c.system, c.machine)
		if goos != c.wantOS || goarch != c.wantArch {
			t.Errorf("DetectPlatform(%q, %q) = (%q, %q), want (%q, %q)",
				c.system, c.machine, goos, goarch, c.wantOS, c.wantArch)
		}
	}
}

func TestDetectPlatformUnknownIsEmpty(t *testing.T) {
	if goos, goarch := DetectPlatform("Windows", "amd64"); goos != "" || goarch != "amd64" {
		t.Fatalf("Windows/amd64 = (%q, %q)", goos, goarch)
	}
	if goos, goarch := DetectPlatform("Darwin", "mips"); goos != "darwin" || goarch != "" {
		t.Fatalf("Darwin/mips = (%q, %q)", goos, goarch)
	}
}

func TestCachePathIsVersionKeyed(t *testing.T) {
	home := newHome(t)
	p := CacheBinPath(home, "1.2.3", "linux", "amd64")
	want := filepath.Join(home, "cache", "bin", "worktree-gate", "1.2.3", "linux-amd64", "worktree-gate")
	if p != want {
		t.Fatalf("CacheBinPath = %q, want %q", p, want)
	}
}

func TestCachePathDefaultsToInstalledVersionAndHostPlatform(t *testing.T) {
	home := newHome(t)
	p := CacheBinPath(home, "", "", "")
	if got := filepath.Base(filepath.Dir(filepath.Dir(p))); got != "9.9.9" {
		t.Fatalf("version segment = %q, want 9.9.9", got)
	}
	if filepath.Base(p) != "worktree-gate" {
		t.Fatalf("binary name = %q", filepath.Base(p))
	}
}

// --- digest-verified install ------------------------------------------------

func TestDigestMatchInstallsExecutableBinary(t *testing.T) {
	home := newHome(t)
	asset := gateScript()
	writeSums(t, home, sha256Hex(asset), "worktree-gate-darwin-arm64")
	srv, _ := assetServer(t, asset)

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if !status.Installed {
		t.Fatalf("not installed: %+v", status)
	}
	info, err := os.Stat(status.CachePath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("installed binary missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary not executable: %v", info.Mode())
	}
	got, err := os.ReadFile(status.CachePath)
	if err != nil || string(got) != string(asset) {
		t.Fatalf("installed bytes differ: %v", err)
	}
	if !isFile(VerificationRecordPath(status.CachePath)) {
		t.Fatal("no verification receipt after install")
	}
	if status.Verification == nil || status.Verification.Reason != "acquired and verified" {
		t.Fatalf("verification = %+v", status.Verification)
	}
	if status.Mismatch != nil {
		t.Fatalf("unexpected mismatch: %v", *status.Mismatch)
	}
}

func TestStaleExecutableCacheIsRevalidatedAndReacquired(t *testing.T) {
	home := newHome(t)
	fresh := gateScript()
	writeSums(t, home, sha256Hex(fresh), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	writeExec(t, cache, "stale cache bytes")
	srv, hits := assetServer(t, fresh)

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if !status.Installed {
		t.Fatalf("not installed: %+v", status)
	}
	got, _ := os.ReadFile(cache)
	if string(got) != string(fresh) {
		t.Fatalf("cache bytes = %q, want fresh", got)
	}
	if *hits != 1 {
		t.Fatalf("download calls = %d, want 1", *hits)
	}
	if !isFile(VerificationRecordPath(cache)) {
		t.Fatal("successful cache refresh must leave current verification evidence")
	}
	if status.Verification == nil {
		t.Fatal("no verification evidence")
	}
	mustContain(t, status.Verification.Reason, "re-acquired")
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(cache), "rejected", "*")); len(matches) == 0 {
		t.Fatal("rejected cache candidate was not quarantined")
	}
	if isFile(DigestMismatchRecordPath(home, "9.9.9")) {
		t.Fatal("a successful re-acquisition must clear the mismatch record")
	}
	if status.Mismatch != nil {
		t.Fatalf("unexpected mismatch: %v", *status.Mismatch)
	}
}

func TestCacheVersionMismatchNotExecutedBeforeReacquisition(t *testing.T) {
	home := newHome(t)
	fresh := gateScript()
	writeSums(t, home, sha256Hex(fresh), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	writeExec(t, cache, string(fresh))
	srv, hits := assetServer(t, fresh)

	calls := 0
	restore := overrideVersionProbe(func(string) string {
		calls++
		if calls == 1 {
			return "8.8.8"
		}
		return "9.9.9"
	})
	defer restore()

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if !status.Installed {
		t.Fatalf("not installed: %+v", status)
	}
	if status.Verification == nil {
		t.Fatal("no verification evidence")
	}
	mustContain(t, status.Verification.Reason, "version")
	if *hits != 1 {
		t.Fatalf("download calls = %d, want 1", *hits)
	}
}

func TestCacheSelftestFailureForcesReacquisition(t *testing.T) {
	home := newHome(t)
	fresh := gateScript()
	writeSums(t, home, sha256Hex(fresh), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	writeExec(t, cache, string(fresh))
	srv, hits := assetServer(t, fresh)

	selftestCalls := 0
	restore := overrideSelftestProbe(func(string) string {
		selftestCalls++
		if selftestCalls == 1 {
			return "selftest failed"
		}
		return ""
	})
	defer restore()

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if !status.Installed {
		t.Fatalf("not installed: %+v", status)
	}
	if selftestCalls != 2 {
		t.Fatalf("selftest calls = %d, want 2", selftestCalls)
	}
	if *hits != 1 {
		t.Fatalf("download calls = %d, want 1", *hits)
	}
}

func TestVerificationReceiptFailureLeavesCacheUnselected(t *testing.T) {
	home := newHome(t)
	asset := gateScript()
	writeSums(t, home, sha256Hex(asset), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	// Make the receipt path an existing directory so the atomic rename fails.
	if err := os.MkdirAll(VerificationRecordPath(cache), 0o755); err != nil {
		t.Fatalf("mkdir receipt-path: %v", err)
	}
	srv, _ := assetServer(t, asset)

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if status.Installed {
		t.Fatalf("installed despite receipt failure: %+v", status)
	}
	if exists(cache) {
		t.Fatalf("cache candidate must be deleted after a receipt failure: %q", cache)
	}
	if isFile(VerificationRecordPath(cache)) {
		t.Fatal("no verification receipt may remain")
	}
	mustContain(t, deref(t, status.Warn), "receipt write failed")
}

func TestDigestMismatchNeverInstallsAndRecords(t *testing.T) {
	home := newHome(t)
	writeSums(t, home, strings.Repeat("0", 64), "worktree-gate-darwin-arm64")
	srv, _ := assetServer(t, []byte("corrupted bytes"))

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if status.Installed {
		t.Fatalf("installed corrupted bytes: %+v", status)
	}
	if status.Mismatch == nil {
		t.Fatal("digest mismatch was not recorded in the status")
	}
	if exists(status.CachePath) {
		t.Fatalf("cache path exists after mismatch: %q", status.CachePath)
	}
	mustContain(t, deref(t, status.Warn), "digest mismatch")
	if !isFile(DigestMismatchRecordPath(home, "9.9.9")) {
		t.Fatal("digest mismatch record was not written")
	}
}

func TestPartialDownloadNeverInstalled(t *testing.T) {
	home := newHome(t)
	writeSums(t, home, strings.Repeat("0", 64), "worktree-gate-darwin-arm64")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("half"))
	}))
	t.Cleanup(srv.Close)

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if status.Installed {
		t.Fatalf("installed a partial download: %+v", status)
	}
	if exists(status.CachePath) {
		t.Fatalf("cache path exists after partial download: %q", status.CachePath)
	}
	mustContain(t, deref(t, status.Warn), "download failed")
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(status.CachePath), ".*.download-*"))
	if len(leftovers) != 0 {
		t.Fatalf("partial download temp left behind: %v", leftovers)
	}
}

func TestDownloadStagesInPrivateTempDir(t *testing.T) {
	home := newHome(t)
	asset := gateScript()
	writeSums(t, home, sha256Hex(asset), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	dir := filepath.Dir(cache)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	// A symlink planted at the former predictable temp path must never be
	// followed, so its target stays untouched.
	victim := filepath.Join(t.TempDir(), "victim")
	writeFile(t, victim, "untouched")
	if err := os.Symlink(victim, filepath.Join(dir, ".worktree-gate.download")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	srv, _ := assetServer(t, asset)

	status := Acquire(withBaseURL(darwinOpts(home, "auto"), srv.URL))

	if !status.Installed {
		t.Fatalf("not installed: %+v", status)
	}
	if got, _ := os.ReadFile(victim); string(got) != "untouched" {
		t.Fatalf("pre-planted symlink target was written: %q", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".*.download-*"))
	if len(leftovers) != 0 {
		t.Fatalf("download temp leftovers remain: %v", leftovers)
	}
}

// --- local build ------------------------------------------------------------

// fakeGoScript parses `-o <out>` and writes the fixture gate script there.
const fakeGoScript = "#!/bin/sh\nout=\"\"\n" +
	"while [ $# -gt 0 ]; do\n" +
	"  case \"$1\" in\n" +
	"    -o) out=\"$2\"; shift 2 ;;\n" +
	"    *) shift ;;\n" +
	"  esac\n" +
	"done\n" +
	"cat > \"$out\" <<'GOEOF'\n" +
	gateScriptBody +
	"GOEOF\n" +
	"exit 0\n"

func TestOptInLocalBuildPopulatesCache(t *testing.T) {
	home := newHome(t)
	gateDir := filepath.Join(home, "catalog", "recipes", "worktree-flow", "gate")
	writeFile(t, filepath.Join(gateDir, "go.mod"), "module example.com/fake\n")
	fakeGo := filepath.Join(home, "fake-go")
	writeExec(t, fakeGo, fakeGoScript)
	t.Setenv("AI_SPECS_GATE_BUILD", "1")

	opts := darwinOpts(home, "auto")
	opts.LookPath = func(name string) (string, error) {
		if name == "go" {
			return fakeGo, nil
		}
		return "", exec.ErrNotFound
	}
	status := Acquire(opts)

	if !status.Installed {
		t.Fatalf("local build not installed: %+v", status)
	}
	info, err := os.Stat(status.CachePath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("built binary missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("built binary not executable: %v", info.Mode())
	}
	if !isFile(VerificationRecordPath(status.CachePath)) {
		t.Fatal("local build left no verification receipt")
	}
}

func TestLocalBuildMissingSourceAndToolchain(t *testing.T) {
	home := newHome(t)
	target := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	if ok, detail := LocalBuild(target, home, "9.9.9", "darwin", "arm64", notFound); ok {
		t.Fatal("local build succeeded without gate source")
	} else {
		mustContain(t, detail, "gate Go source not found")
	}
	writeFile(t, filepath.Join(home, "catalog", "recipes", "worktree-flow", "gate", "go.mod"), "module x\n")
	if ok, detail := LocalBuild(target, home, "9.9.9", "darwin", "arm64", notFound); ok {
		t.Fatal("local build succeeded without a go toolchain")
	} else {
		mustContain(t, detail, "go toolchain not found")
	}
}

// --- degradation matrix -----------------------------------------------------

func TestOfflineAutoAndGoDegradeWithoutBashFallback(t *testing.T) {
	for _, impl := range []string{"auto", "go"} {
		home := newHome(t)
		opts := darwinOpts(home, impl)
		opts.Offline = true
		opts.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		status := Acquire(opts)

		if !status.Attempted {
			t.Fatalf("%s: not attempted: %+v", impl, status)
		}
		if status.Installed {
			t.Fatalf("%s: installed while offline: %+v", impl, status)
		}
		warn := deref(t, status.Warn)
		mustContain(t, warn, "failing open")
		if strings.Contains(warn, "Bash") {
			t.Fatalf("%s: warn mentions Bash: %q", impl, warn)
		}
		if strings.Contains(strings.ToLower(warn), "bash fallback") {
			t.Fatalf("%s: warn mentions a bash fallback: %q", impl, warn)
		}
	}
}

func TestUnsupportedPlatformWarnsNoInstall(t *testing.T) {
	home := newHome(t)
	opts := darwinOpts(home, "auto")
	opts.GOOS, opts.GOARCH = "windows", "amd64"
	status := Acquire(opts)
	if status.Installed {
		t.Fatalf("installed on unsupported platform: %+v", status)
	}
	mustContain(t, deref(t, status.Warn), "unsupported platform")
	mustContain(t, deref(t, status.Warn), "darwin/arm64")
}

func TestUnsupportedPlatformUnknownFallback(t *testing.T) {
	home := newHome(t)
	restore := overrideHostPlatform(func() (string, string) { return "", "" })
	defer restore()
	status := Acquire(AcquireOptions{GateImpl: "auto", Home: home})
	mustContain(t, deref(t, status.Warn), "(?/?)")
}

func TestAcquisitionNeverRaisesWithoutTrustRoot(t *testing.T) {
	home := newHome(t)
	status := Acquire(darwinOpts(home, "auto"))
	if status.Installed {
		t.Fatalf("installed without a trust root: %+v", status)
	}
	mustContain(t, deref(t, status.Warn), "no committed digest")
}

func TestAcquisitionNeverRaisesOnDeadServer(t *testing.T) {
	home := newHome(t)
	writeSums(t, home, strings.Repeat("0", 64), "worktree-gate-darwin-arm64")
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	status := Acquire(withBaseURL(darwinOpts(home, "auto"), deadURL))
	if status.Installed {
		t.Fatalf("installed from a dead server: %+v", status)
	}
	mustContain(t, deref(t, status.Warn), "download failed")
}

// --- trust root + URL -------------------------------------------------------

func TestCommittedTrustRootListsTheReleaseMatrix(t *testing.T) {
	root := filepath.Join("..", "..")
	digests, ok := LoadExpectedDigests(root)
	if !ok {
		t.Fatal("the committed trust root must be readable")
	}
	for _, platform := range SupportedPlatforms {
		name := AssetName(platform[0], platform[1])
		digest, present := digests[name]
		if !present {
			t.Fatalf("committed SHA256SUMS is missing %s", name)
		}
		if len(digest) != 64 {
			t.Fatalf("%s digest length = %d", name, len(digest))
		}
		if strings.Trim(digest, "0") == "" {
			t.Fatalf("committed SHA256SUMS still holds a placeholder for %s", name)
		}
	}
}

func TestLoadExpectedDigestsFailsClosedOnUndecodableRoot(t *testing.T) {
	home := t.TempDir()
	sums := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	if err := os.MkdirAll(filepath.Dir(sums), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sums, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadExpectedDigests(home); ok {
		t.Fatal("an undecodable trust root must fail closed")
	}
	if digests, ok := LoadExpectedDigests(t.TempDir()); !ok || len(digests) != 0 {
		t.Fatalf("missing trust root = %v, %v; want empty, true", digests, ok)
	}
}

func TestAssetURLUsesCanonicalRepositoryOwner(t *testing.T) {
	got := AssetURL("9.9.9", "worktree-gate-darwin-arm64")
	want := "https://github.com/parada1104/ai-specs-cli/releases/download/v9.9.9/worktree-gate-darwin-arm64"
	if got != want {
		t.Fatalf("AssetURL = %q, want %q", got, want)
	}
}

func TestModuleHasNoDivergentRepositoryOwner(t *testing.T) {
	source, err := os.ReadFile("gatebinary.go")
	if err != nil {
		t.Fatalf("read gatebinary.go: %v", err)
	}
	if strings.Contains(string(source), "nnodes") {
		t.Fatal("gatebinary.go references the divergent 'nnodes' repository owner")
	}
}

// --- receipts, mismatch records, resolution ---------------------------------

func TestWriteAndReadVerificationRecord(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "worktree-gate")
	if err := WriteVerificationRecord(binary, "9.9.9", "abc123"); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	raw, err := os.ReadFile(VerificationRecordPath(binary))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	// Byte-identical with gate_binary.py:277-280, including the trailing newline.
	want := "status=verified\nversion=9.9.9\ndigest=abc123\nselftest=passed\n"
	if string(raw) != want {
		t.Fatalf("receipt bytes = %q, want %q", raw, want)
	}
	record := ReadVerificationRecord(binary)
	if record["status"] != "verified" || record["version"] != "9.9.9" ||
		record["digest"] != "abc123" || record["selftest"] != "passed" {
		t.Fatalf("record = %v", record)
	}
	if got := ReadVerificationRecord(filepath.Join(t.TempDir(), "absent")); len(got) != 0 {
		t.Fatalf("missing receipt = %v, want empty", got)
	}
	malformed := filepath.Join(t.TempDir(), "malformed")
	writeFile(t, VerificationRecordPath(malformed), "status=\n=value\nnovalue\nok=fine\n")
	record = ReadVerificationRecord(malformed)
	if len(record) != 1 || record["ok"] != "fine" {
		t.Fatalf("malformed receipt = %v, want {ok: fine}", record)
	}
}

func TestRecordAndClearDigestMismatch(t *testing.T) {
	home := newHome(t)
	RecordDigestMismatch("boom", home, "9.9.9")
	path := DigestMismatchRecordPath(home, "9.9.9")
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "boom\n" {
		t.Fatalf("mismatch record = %q, %v", raw, err)
	}
	ClearDigestMismatch(home, "9.9.9")
	if exists(path) {
		t.Fatal("mismatch record was not cleared")
	}
	// Clearing an absent record is a no-op, never an error.
	ClearDigestMismatch(home, "9.9.9")
}

func TestQuarantineCachedCandidateMovesBinaryAndReceipt(t *testing.T) {
	home := newHome(t)
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	writeExec(t, cache, "rejected bytes")
	writeFile(t, VerificationRecordPath(cache), "status=verified\n")

	rejected := QuarantineCachedCandidate(cache)
	if rejected == "" {
		t.Fatal("quarantine returned no path")
	}
	if !strings.HasPrefix(filepath.Base(rejected), "worktree-gate-") {
		t.Fatalf("rejected name = %q", filepath.Base(rejected))
	}
	if filepath.Base(filepath.Dir(rejected)) != "rejected" {
		t.Fatalf("rejected dir = %q", filepath.Dir(rejected))
	}
	if exists(cache) || exists(VerificationRecordPath(cache)) {
		t.Fatal("candidate or receipt left behind")
	}
	if !exists(rejected) || !exists(VerificationRecordPath(rejected)) {
		t.Fatal("quarantined candidate or receipt missing")
	}
}

func TestResolveVerifiedBinary(t *testing.T) {
	t.Setenv("WORKTREE_GATE_BIN", "")
	home := newHome(t)
	goos, goarch := DetectHostPlatform()
	cache := CacheBinPath(home, "9.9.9", goos, goarch)
	writeExec(t, cache, "#!/bin/sh\nexit 0\n")

	// No receipt: never selected.
	if got := ResolveVerifiedBinary(home); got != "" {
		t.Fatalf("candidate without receipt = %q, want empty", got)
	}
	// No committed digest: the receipt itself must attest a matching digest.
	writeFile(t, VerificationRecordPath(cache), "status=verified\n")
	if got := ResolveVerifiedBinary(home); got != "" {
		t.Fatalf("digestless receipt = %q, want empty", got)
	}
	writeFile(t, VerificationRecordPath(cache), "status=verified\ndigest="+strings.Repeat("b", 64)+"\n")
	if got := ResolveVerifiedBinary(home); got != "" {
		t.Fatalf("mismatched receipt digest = %q, want empty", got)
	}
	writeFile(t, VerificationRecordPath(cache),
		"status=verified\ndigest="+sha256Hex([]byte("#!/bin/sh\nexit 0\n"))+"\n")
	if got := ResolveVerifiedBinary(home); got != cache {
		t.Fatalf("receipted candidate without trust root = %q, want %q", got, cache)
	}
	// Receipt plus a matching committed digest: selected.
	writeSums(t, home, sha256Hex([]byte("#!/bin/sh\nexit 0\n")), "worktree-gate-"+goos+"-"+goarch)
	if got := ResolveVerifiedBinary(home); got != cache {
		t.Fatalf("verified candidate = %q, want %q", got, cache)
	}
	// Receipt plus a stale digest: rejected.
	writeSums(t, home, strings.Repeat("b", 64), "worktree-gate-"+goos+"-"+goarch)
	if got := ResolveVerifiedBinary(home); got != "" {
		t.Fatalf("stale digest = %q, want empty", got)
	}
	// An undecodable trust root fails closed even with a receipt.
	sums := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	if err := os.WriteFile(sums, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveVerifiedBinary(home); got != "" {
		t.Fatalf("undecodable trust root = %q, want empty", got)
	}
}

func TestResolveVerifiedBinaryOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "gate")
	writeExec(t, override, "#!/bin/sh\nexit 0\n")
	t.Setenv("WORKTREE_GATE_BIN", override)
	if got := ResolveVerifiedBinary(t.TempDir()); got != override {
		t.Fatalf("override = %q, want %q", got, override)
	}
	// A non-executable override is refused outright, never falling through.
	plain := filepath.Join(t.TempDir(), "plain")
	writeFile(t, plain, "nope")
	t.Setenv("WORKTREE_GATE_BIN", plain)
	if got := ResolveVerifiedBinary(t.TempDir()); got != "" {
		t.Fatalf("non-executable override = %q, want empty", got)
	}
}

func TestBinaryVersionAndSelftest(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "gate.sh")
	writeExec(t, bin, "#!/bin/sh\ncase \"$1\" in\n"+
		"  --version) printf '1.2.3\\n' ;;\n"+
		"  --selftest) exit 0 ;;\n"+
		"esac\nexit 0\n")
	if got := BinaryVersion(bin); got != "1.2.3" {
		t.Fatalf("BinaryVersion = %q, want 1.2.3", got)
	}
	if got := Selftest(bin); got != "" {
		t.Fatalf("Selftest = %q, want empty", got)
	}
	failing := filepath.Join(t.TempDir(), "gate.sh")
	writeExec(t, failing, "#!/bin/sh\necho 'boom' 1>&2\nexit 3\n")
	if got := Selftest(failing); got != "boom" {
		t.Fatalf("failing Selftest = %q, want boom", got)
	}
	if got := BinaryVersion(failing); got != "" {
		t.Fatalf("non-zero --version = %q, want empty", got)
	}
	silent := filepath.Join(t.TempDir(), "gate.sh")
	writeExec(t, silent, "#!/bin/sh\nexit 2\n")
	if got := Selftest(silent); got != "selftest failed" {
		t.Fatalf("silent Selftest = %q, want %q", got, "selftest failed")
	}
}

func TestCLIVersionAndDigestMismatchRecordPath(t *testing.T) {
	home := t.TempDir()
	if got := CLIVersion(home); got != "unknown" {
		t.Fatalf("CLIVersion without VERSION = %q", got)
	}
	writeFile(t, filepath.Join(home, "VERSION"), "  1.2.3  \n")
	if got := CLIVersion(home); got != "1.2.3" {
		t.Fatalf("CLIVersion = %q", got)
	}
	want := filepath.Join(home, "cache", "bin", "worktree-gate", "1.2.3", "last-digest-mismatch.txt")
	if got := DigestMismatchRecordPath(home, "1.2.3"); got != want {
		t.Fatalf("DigestMismatchRecordPath = %q, want %q", got, want)
	}
}

func TestCacheHitWritesReceiptAndClearsMismatch(t *testing.T) {
	home := newHome(t)
	asset := gateScript()
	writeSums(t, home, sha256Hex(asset), "worktree-gate-darwin-arm64")
	cache := CacheBinPath(home, "9.9.9", "darwin", "arm64")
	writeExec(t, cache, string(asset))
	RecordDigestMismatch("stale rejection", home, "9.9.9")

	status := Acquire(darwinOpts(home, "auto"))
	if !status.Installed {
		t.Fatalf("verified cache candidate not installed: %+v", status)
	}
	if !isFile(VerificationRecordPath(cache)) {
		t.Fatal("cache hit left no receipt")
	}
	if exists(DigestMismatchRecordPath(home, "9.9.9")) {
		t.Fatal("cache hit did not clear the mismatch record")
	}
	if status.Verification == nil || status.Verification.Reason != "digest, version, and self-test verified" {
		t.Fatalf("verification = %+v", status.Verification)
	}
}

// --- injectable seams -------------------------------------------------------

func overrideVersionProbe(fn func(string) string) func() {
	old := probeBinaryVersion
	probeBinaryVersion = fn
	return func() { probeBinaryVersion = old }
}

func overrideSelftestProbe(fn func(string) string) func() {
	old := probeSelftest
	probeSelftest = fn
	return func() { probeSelftest = old }
}

func overrideHostPlatform(fn func() (string, string)) func() {
	old := detectHostPlatform
	detectHostPlatform = fn
	return func() { detectHostPlatform = old }
}
