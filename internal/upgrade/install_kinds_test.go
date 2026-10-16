package upgrade

// GO-20 U1b-2a: the engine must know BOTH install layouts before the installer
// starts producing the second one. A legacy install is the launcher symlinked
// into the checkout; a binary install is a real compiled file as the entry,
// with the checkout kept as the data home and the trust anchor.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newFakeBinaryInstall builds the post-flip layout: the checkout at
// $HOME/.ai-specs stays (data home + committed trust root), and the ENTRY is a
// real compiled file in $HOME/.local/bin instead of a symlink into the home.
func newFakeBinaryInstall(t *testing.T, version string) fakeInstall {
	t.Helper()
	raw := t.TempDir()
	home, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("resolve temp home: %v", err)
	}
	setupGitEnv(t, home)

	aiSpecs := filepath.Join(home, ".ai-specs")
	if err := os.MkdirAll(filepath.Join(aiSpecs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(aiSpecs, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(aiSpecs, "VERSION"), version+"\n", 0o644)

	localBin := filepath.Join(home, ".local", "bin", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(localBin), 0o755); err != nil {
		t.Fatal(err)
	}
	// The entry is a REAL file, not a symlink: that is what makes it a binary
	// install, and the running executable IS this file.
	writeFile(t, localBin, "#!/bin/sh\nexit 0\n", 0o755)

	bare := filepath.Join(home, "origin.git")
	gitRun(t, home, "init", "--bare", "origin.git")
	gitRun(t, aiSpecs, "init", "-b", "main")
	gitRun(t, aiSpecs, "add", ".")
	gitRun(t, aiSpecs, "commit", "-m", "init")
	gitRun(t, aiSpecs, "remote", "add", "origin", bare)
	gitRun(t, aiSpecs, "push", "-u", "origin", "main")

	return fakeInstall{home: home, aiSpecs: aiSpecs, exe: localBin, localBin: localBin, bare: bare}
}

func TestDetectClassifiesBothInstallKinds(t *testing.T) {
	t.Run("legacy: the launcher symlinked into the checkout", func(t *testing.T) {
		f := newFakeInstall(t, "1.0.0")
		var errb bytes.Buffer
		inst, code, ok := Detect(f.env(), &errb)
		if !ok || code != 0 {
			t.Fatalf("ok=%v code=%d stderr=%q", ok, code, errb.String())
		}
		if inst.Kind != KindLegacy {
			t.Errorf("Kind = %v, want KindLegacy", inst.Kind)
		}
	})

	t.Run("binary: a real compiled file as the entry", func(t *testing.T) {
		f := newFakeBinaryInstall(t, "1.0.0")
		var errb bytes.Buffer
		inst, code, ok := Detect(f.env(), &errb)
		if !ok || code != 0 {
			t.Fatalf("a binary install must be accepted: ok=%v code=%d stderr=%q", ok, code, errb.String())
		}
		if inst.Kind != KindBinary {
			t.Errorf("Kind = %v, want KindBinary", inst.Kind)
		}
		if inst.LocalBin != f.localBin {
			t.Errorf("LocalBin = %q, want %q", inst.LocalBin, f.localBin)
		}
	})

	t.Run("a symlink into the home is still legacy, never binary", func(t *testing.T) {
		// The classification must come from the entry's own kind, not from the
		// executable happening to sit in $INSTALL_BIN.
		f := newFakeInstall(t, "1.0.0")
		inst, _, _ := Detect(f.env(), &bytes.Buffer{})
		if inst.Kind == KindBinary {
			t.Error("a symlinked entry must not classify as a binary install")
		}
	})
}

func binaryAssetPayload(version string) []byte {
	return []byte("#!/bin/sh\necho " + version + "\n")
}

func TestBinaryKindAcquiresVerifiedAndSwaps(t *testing.T) {
	f := newFakeBinaryInstall(t, "1.0.0")
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	payload := binaryAssetPayload("1.1.0")
	sum := sha256.Sum256(payload)
	writeFile(t, filepath.Join(f.aiSpecs, "bin", "SHA256SUMS"),
		fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+asset) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	env := f.env()
	env.ReleaseBase = srv.URL
	var out, errb bytes.Buffer
	if code := Run(nil, env, &out, &errb); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}

	got, err := os.ReadFile(f.localBin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("the verified asset was not installed:\n got: %q\n want: %q", got, payload)
	}
}

func TestBinaryKindFailsClosedWithoutACommittedDigest(t *testing.T) {
	f := newFakeBinaryInstall(t, "1.0.0")
	before, err := os.ReadFile(f.localBin)
	if err != nil {
		t.Fatal(err)
	}
	// No SHA256SUMS at all: no committed digest means no install.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request may be made without a committed digest: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	env := f.env()
	env.ReleaseBase = srv.URL
	var out, errb bytes.Buffer
	code := Run(nil, env, &out, &errb)
	if code == ExitOK {
		t.Fatalf("a missing digest must fail closed; stdout=%s", out.String())
	}
	if !strings.Contains(errb.String(), AssetName(runtime.GOOS, runtime.GOARCH)) {
		t.Errorf("the refusal must name the asset: %q", errb.String())
	}
	after, err := os.ReadFile(f.localBin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the current binary must stay untouched when the digest is missing")
	}
}
