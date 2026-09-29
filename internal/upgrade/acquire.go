// Package upgrade — acquire.go implements the single-binary self-replacement
// engine (card [Go 12]).
//
// Invariants (mirroring the gate acquisition discipline in
// lib/_internal/gate_binary.py):
//   - Digest before execution, always. The expected SHA-256 comes from a
//     committed SHA256SUMS trust root. A mismatched artifact is deleted and
//     never installed or executed.
//   - Atomic install. Download to a temp file IN the destination directory,
//     verify, chmod 0755, then rename into place. A partial download can never
//     replace a working binary.
//   - Any failure before the rename leaves the current binary intact; a failed
//     rename restores the original.
package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// TrustRoot maps asset name -> lowercase hex SHA-256.
type TrustRoot map[string]string

// ParseTrustRoot parses the committed SHA256SUMS format:
// `<64-hex>  <asset-name>` lines; comments and blank lines are ignored.
func ParseTrustRoot(text string) TrustRoot {
	root := TrustRoot{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		digest, name := strings.ToLower(fields[0]), fields[1]
		if len(digest) != 64 {
			continue
		}
		if _, err := hex.DecodeString(digest); err != nil {
			continue
		}
		root[name] = digest
	}
	return root
}

// LoadTrustRoot reads a trust-root file; a missing file yields an empty root
// (the caller fails closed because no digest means no install).
func LoadTrustRoot(path string) (TrustRoot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TrustRoot{}, nil
		}
		return nil, err
	}
	return ParseTrustRoot(string(data)), nil
}

// AssetName mirrors the gate release naming.
func AssetName(goos, goarch string) string { return "ai-specs-" + goos + "-" + goarch }

var supportedPlatforms = map[[2]string]bool{
	{"darwin", "arm64"}: true,
	{"darwin", "amd64"}: true,
	{"linux", "amd64"}:  true,
	{"linux", "arm64"}:  true,
}

// DetectPlatform maps goos/goarch to a supported release pair, or ("","").
func DetectPlatform(goos, goarch string) (string, string) {
	switch goos {
	case "darwin", "linux":
	default:
		return "", ""
	}
	switch goarch {
	case "arm64", "amd64":
	default:
		return "", ""
	}
	if !supportedPlatforms[[2]string{goos, goarch}] {
		return "", ""
	}
	return goos, goarch
}

// Sha256File returns the lowercase hex SHA-256 of a file.
func Sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// AcquireOptions configures one verified self-replacement.
type AcquireOptions struct {
	TrustRoot TrustRoot
	BaseURL   string // release download base, no trailing slash
	Asset     string
	Target    string // absolute path of the binary to replace
	HTTP      *http.Client
	GOOS      string // "" => runtime.GOOS
}

// AcquireResult reports what happened.
type AcquireResult struct {
	Installed bool
	Expected  string
	Digest    string
}

// Acquire downloads the asset, verifies its SHA-256 against the trust root
// BEFORE any replacement, and atomically installs it. On any failure the
// current target is left untouched and no unverified bytes are installed or
// executed.
func Acquire(opts AcquireOptions) (AcquireResult, error) {
	var res AcquireResult
	if opts.Asset == "" {
		return res, errors.New("acquire: empty asset name")
	}
	if opts.Target == "" {
		return res, errors.New("acquire: empty target path")
	}
	expected, ok := opts.TrustRoot[opts.Asset]
	if !ok {
		return res, fmt.Errorf("acquire: no committed digest for %s", opts.Asset)
	}
	res.Expected = expected

	client := opts.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	dir := filepath.Dir(opts.Target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(opts.Target)+".download.")
	if err != nil {
		return res, err
	}
	tmpName := tmp.Name()
	keep := false // set true only after a successful rename
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()

	resp, err := client.Get(strings.TrimRight(opts.BaseURL, "/") + "/" + opts.Asset)
	if err != nil {
		return res, fmt.Errorf("acquire: download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("acquire: download failed: HTTP %d", resp.StatusCode)
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		return res, fmt.Errorf("acquire: download interrupted: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return res, fmt.Errorf("acquire: sync failed: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	res.Digest = actual

	if !strings.EqualFold(actual, expected) {
		// Delete the unverified bytes; never install or execute them.
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		keep = true // nothing left to clean up
		return res, fmt.Errorf("acquire: digest mismatch for %s: expected %s, got %s; artifact deleted and never installed", opts.Asset, expected, actual)
	}

	if err := tmp.Chmod(0o755); err != nil {
		return res, fmt.Errorf("acquire: chmod failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return res, fmt.Errorf("acquire: close failed: %w", err)
	}
	if err := SwapBinary(tmpName, opts.Target, goos); err != nil {
		return res, fmt.Errorf("acquire: replace failed: %w", err)
	}
	keep = true
	res.Installed = true
	return res, nil
}

// SwapBinary installs src over target.
//
// Unix (darwin/linux): a single rename; a running process keeps its old inode
// and the path now serves the new bytes.
//
// Windows: a running executable cannot be overwritten, so target is renamed
// aside first, then src is renamed into place; if that rename fails the
// original is restored. The `.old` file is left for the next run to clean
// because a running executable cannot be deleted.
func SwapBinary(src, target, goos string) error {
	if goos == "windows" {
		return swapWindows(src, target)
	}
	return os.Rename(src, target)
}

func swapWindows(src, target string) error {
	old := target + ".old"
	_ = os.Remove(old)
	backedUp := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return err
		}
		backedUp = true
	}
	if err := os.Rename(src, target); err != nil {
		if backedUp {
			_ = os.Rename(old, target)
		}
		return err
	}
	return nil
}
