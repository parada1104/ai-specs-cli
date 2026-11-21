package cli

// JD-A-005: `ai-specs upgrade` must honor AI_SPECS_RELEASE_BASE, exactly as the
// launcher's self-heal does. Without it the engine's dist refresh always hit
// the default GitHub URL, so a mirror (or a local release base) never saw the
// request and an offline install could not be repaired.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/upgrade"
)

func TestUpgradeEnvHonoursReleaseBase(t *testing.T) {
	t.Setenv("AI_SPECS_RELEASE_BASE", "http://127.0.0.1:1/mirror")
	env := upgradeEnv("/tmp/ai-specs-home")
	if env.Home != "/tmp/ai-specs-home" {
		t.Errorf("Home = %q", env.Home)
	}
	if env.ReleaseBase != "http://127.0.0.1:1/mirror" {
		t.Errorf("ReleaseBase = %q, want the AI_SPECS_RELEASE_BASE value", env.ReleaseBase)
	}
}

// TestUpgradeRouteDownloadsFromTheReleaseBase runs the real binary as a binary
// install (no checkout .git, so no git step) against a local mirror: the asset
// can only be acquired from AI_SPECS_RELEASE_BASE.
func TestUpgradeRouteDownloadsFromTheReleaseBase(t *testing.T) {
	bin, _ := smokeSetup(t)

	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	userHome := filepath.Join(root, "home")
	home := filepath.Join(userHome, ".ai-specs")
	installed := filepath.Join(userHome, ".local", "bin", "ai-specs")
	for _, dir := range []string{filepath.Join(home, "bin"), filepath.Dir(installed)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyExecutable(t, bin, installed)

	payload := []byte("#!/bin/sh\necho 1.0.0\n")
	asset := upgrade.AssetName(runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(payload)
	if err := os.WriteFile(filepath.Join(home, "bin", "SHA256SUMS"),
		[]byte(hex.EncodeToString(sum[:])+"  "+asset+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		if r.URL.Path != "/"+asset {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	env := append(smokeEnv(root),
		"HOME="+userHome,
		"AI_SPECS_HOME="+home,
		"AI_SPECS_RELEASE_BASE="+srv.URL,
	)
	stdout, stderr, code := runOne(installed, []string{"upgrade"}, env, root)
	if code != upgrade.ExitOK {
		t.Fatalf("upgrade rc = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if len(hits) != 1 || hits[0] != "/"+asset {
		t.Fatalf("release base requests = %v, want one request for /%s", hits, asset)
	}
	if got, err := os.ReadFile(installed); err != nil || string(got) != string(payload) {
		t.Errorf("the verified asset must become the entry: %q (err %v)", got, err)
	}
	dist := filepath.Join(home, "dist", "ai-specs")
	if got, err := os.ReadFile(dist); err != nil || string(got) != string(payload) {
		t.Errorf("the launcher copy must be refreshed: %q (err %v)", got, err)
	}
	stamp, err := os.ReadFile(filepath.Join(home, "dist", "VERSION"))
	if err != nil {
		t.Fatalf("read launcher copy stamp: %v", err)
	}
	fields := strings.Fields(string(stamp))
	if len(fields) != 3 || fields[0] != "1.0.0" {
		t.Errorf("launcher copy stamp = %q, want '<version> <sha256> <size>:<mtime>'", stamp)
	}
}
