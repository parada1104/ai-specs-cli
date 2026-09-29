package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testSHA256 returns the lowercase hex SHA-256 of b.
func testSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mustWrite writes content to path with perm, failing the test on error.
func mustWrite(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mustRead returns the file contents, failing the test on error.
func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// assertNoDownloadLeftovers fails when any Acquire temp file survives.
func assertNoDownloadLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".download.") {
			t.Errorf("leftover download temp file: %s", e.Name())
		}
	}
}

func TestParseTrustRoot(t *testing.T) {
	valid := testSHA256([]byte("asset-bytes"))
	text := strings.Join([]string{
		"# comment line",
		"",
		"   ",
		valid + "  ai-specs-darwin-arm64",
		"deadbeef  short-digest",
		"single-field-only",
	}, "\n")

	root := ParseTrustRoot(text)
	if got := root["ai-specs-darwin-arm64"]; got != valid {
		t.Errorf("valid line digest = %q, want %q", got, valid)
	}
	if len(root) != 1 {
		t.Errorf("parsed %d entries, want 1: %#v", len(root), root)
	}
}

func TestDetectPlatform(t *testing.T) {
	supported := [][2]string{
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	}
	for _, p := range supported {
		goos, goarch := DetectPlatform(p[0], p[1])
		if goos != p[0] || goarch != p[1] {
			t.Errorf("DetectPlatform(%q, %q) = (%q, %q), want same pair", p[0], p[1], goos, goarch)
		}
	}
	unsupported := [][2]string{
		{"windows", "amd64"},
		{"linux", "386"},
		{"plan9", "arm64"},
	}
	for _, p := range unsupported {
		goos, goarch := DetectPlatform(p[0], p[1])
		if goos != "" || goarch != "" {
			t.Errorf("DetectPlatform(%q, %q) = (%q, %q), want (\"\", \"\")", p[0], p[1], goos, goarch)
		}
	}
}

func TestAcquireInstallsVerified(t *testing.T) {
	asset := []byte("NEW-BINARY-BYTES\x00")
	digest := testSHA256(asset)
	const assetName = "ai-specs-darwin-arm64"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+assetName {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(asset)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "ai-specs")
	mustWrite(t, target, "OLD", 0o644)

	res, err := Acquire(AcquireOptions{
		TrustRoot: TrustRoot{assetName: digest},
		BaseURL:   srv.URL,
		Asset:     assetName,
		Target:    target,
		HTTP:      srv.Client(),
		GOOS:      "darwin",
	})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if !res.Installed {
		t.Error("Installed = false, want true")
	}
	if res.Digest != digest {
		t.Errorf("Digest = %q, want %q", res.Digest, digest)
	}
	if got := mustRead(t, target); got != string(asset) {
		t.Errorf("target bytes = %q, want %q", got, string(asset))
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("target mode = %v, want 0755", fi.Mode().Perm())
	}
	assertNoDownloadLeftovers(t, dir)
}

func TestAcquireRejectsTamperedArtifact(t *testing.T) {
	const assetName = "ai-specs-darwin-arm64"
	expected := testSHA256([]byte("the-real-artifact"))
	tampered := []byte("TAMPERED-BYTES")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tampered)
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "ai-specs")
	mustWrite(t, target, "OLD", 0o755)

	res, err := Acquire(AcquireOptions{
		TrustRoot: TrustRoot{assetName: expected},
		BaseURL:   srv.URL,
		Asset:     assetName,
		Target:    target,
		HTTP:      srv.Client(),
		GOOS:      "darwin",
	})
	if err == nil {
		t.Fatal("Acquire succeeded on a tampered artifact, want error")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("error = %v, want it to contain %q", err, "digest mismatch")
	}
	if res.Installed {
		t.Error("Installed = true, want false")
	}
	if got := mustRead(t, target); got != "OLD" {
		t.Errorf("target bytes = %q, want %q (must stay untouched)", got, "OLD")
	}
	assertNoDownloadLeftovers(t, dir)
}

func TestAcquireRejectsMissingDigest(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ai-specs")
	mustWrite(t, target, "OLD", 0o755)

	// No server is started: a missing digest must fail before any network use.
	res, err := Acquire(AcquireOptions{
		TrustRoot: TrustRoot{},
		BaseURL:   "http://127.0.0.1:1",
		Asset:     "ai-specs-darwin-arm64",
		Target:    target,
	})
	if err == nil {
		t.Fatal("Acquire succeeded with an empty trust root, want error")
	}
	if res.Installed {
		t.Error("Installed = true, want false")
	}
	if got := mustRead(t, target); got != "OLD" {
		t.Errorf("target bytes = %q, want %q", got, "OLD")
	}
}

func TestAcquireInterruptedDownloadLeavesTargetIntact(t *testing.T) {
	const assetName = "ai-specs-darwin-arm64"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Promise far more than we send, then return early: the client sees
		// an unexpected EOF mid-body.
		w.Header().Set("Content-Length", "1048576")
		_, _ = w.Write([]byte("partial"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "ai-specs")
	mustWrite(t, target, "OLD", 0o755)

	res, err := Acquire(AcquireOptions{
		TrustRoot: TrustRoot{assetName: testSHA256([]byte("whatever"))},
		BaseURL:   srv.URL,
		Asset:     assetName,
		Target:    target,
		HTTP:      srv.Client(),
		GOOS:      "darwin",
	})
	if err == nil {
		t.Fatal("Acquire succeeded on an interrupted download, want error")
	}
	if res.Installed {
		t.Error("Installed = true, want false")
	}
	if got := mustRead(t, target); got != "OLD" {
		t.Errorf("target bytes = %q, want %q", got, "OLD")
	}
	assertNoDownloadLeftovers(t, dir)
}

func TestSwapBinaryUnix(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	target := filepath.Join(dir, "target")
	mustWrite(t, src, "NEW", 0o755)

	if err := SwapBinary(src, target, "linux"); err != nil {
		t.Fatalf("SwapBinary: %v", err)
	}
	if got := mustRead(t, target); got != "NEW" {
		t.Errorf("target bytes = %q, want %q", got, "NEW")
	}
}

func TestSwapBinaryWindows(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	target := filepath.Join(dir, "target")
	mustWrite(t, target, "OLD", 0o755)
	mustWrite(t, src, "NEW", 0o755)

	if err := SwapBinary(src, target, "windows"); err != nil {
		t.Fatalf("SwapBinary: %v", err)
	}
	if got := mustRead(t, target); got != "NEW" {
		t.Errorf("target bytes = %q, want %q", got, "NEW")
	}
	// Documented Windows contract: a running executable cannot be deleted, so the
	// successful swap deliberately leaves the replaced binary at target+".old"
	// for the next run to clean.
	if got := mustRead(t, target+".old"); got != "OLD" {
		t.Errorf("target.old bytes = %q, want %q", got, "OLD")
	}
}

func TestSwapBinaryWindowsMissingSrc(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist")
	target := filepath.Join(dir, "target")
	mustWrite(t, target, "OLD", 0o755)

	if err := SwapBinary(src, target, "windows"); err == nil {
		t.Fatal("SwapBinary succeeded with a missing src, want error")
	}
	if got := mustRead(t, target); got != "OLD" {
		t.Errorf("target bytes = %q, want %q (original must be restored)", got, "OLD")
	}
}
