// Package gatebinary is the native port of lib/_internal/gate_binary.py: gate
// binary acquisition, verification and the version-keyed cache layout.
//
// The acquired binary lives at
// <home>/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate,
// so each CLI version resolves its own binary. The invariants are the Python
// module's: digest before execution (the committed SHA256SUMS trust root), an
// atomic install with a receipt, no unverified bytes ever executed, an opt-in
// local build, and a fail-open degradation for every failure. Acquire never
// returns an error; it returns a status the caller turns into warnings.
//
// This package is the single read/verify/acquire authority. It deliberately
// does not reuse internal/upgrade (whose Acquire has another contract: no
// self-test, no receipt, no quarantine, and a different trust-root parser) nor
// internal/doctor (which keeps read-only twins and, in a later commit, will
// delegate to this package).
package gatebinary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// GateBinEnv is the explicit verified-binary override the launcher and every
// host honors (resolution step 1, before the cache).
const GateBinEnv = "WORKTREE_GATE_BIN"

const (
	// MismatchFilename holds the last acquisition mismatch under the
	// version-keyed cache directory.
	MismatchFilename = "last-digest-mismatch.txt"
	// VerificationSuffix is the sidecar carrying the acquisition receipt.
	VerificationSuffix = ".verified"
	// RepoOwner is the canonical GitHub repository owner: the released
	// worktree-gate assets are attached to this repository's Releases, so the
	// download URL must use it or every sync acquisition 404s.
	RepoOwner = "parada1104"
	// RepoName is the canonical repository name.
	RepoName = "ai-specs-cli"
	// DefaultBaseURL is the release download base (no trailing slash).
	DefaultBaseURL = "https://github.com/" + RepoOwner + "/" + RepoName + "/releases/download"

	buildTimeout    = 300 * time.Second
	selftestTimeout = 30 * time.Second
	versionTimeout  = 10 * time.Second
	// downloadTimeout is a deliberate divergence: Python's urlretrieve has no
	// timeout; the native client bounds a hung release download at 60 s, the
	// same budget internal/upgrade uses.
	downloadTimeout = 60 * time.Second
)

// SupportedPlatforms is the published release matrix. A platform outside it
// gets no binary target; acquisition warns naming the alternatives.
var SupportedPlatforms = [][2]string{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
}

// Injectable seams for the tests and for callers that own their own probes.
// They mirror the module-level functions the Python tests monkeypatch.
var (
	detectHostPlatform = DetectHostPlatform
	probeBinaryVersion = BinaryVersion
	probeSelftest      = Selftest
)

// CLIVersion is the installed CLI version (the version key for the cache
// layout): <home>/VERSION stripped, or "unknown".
func CLIVersion(home string) string {
	raw, err := os.ReadFile(filepath.Join(home, "VERSION"))
	if err != nil {
		return "unknown"
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "unknown"
	}
	return text
}

// DetectPlatform maps `uname -s` / `uname -m` to a (goos, goarch) pair.
// `Darwin`→`darwin`, `Linux`→`linux`; `arm64|aarch64`→`arm64`,
// `x86_64|amd64`→`amd64`. Anything else → "". A Rosetta-translated shell on
// Apple Silicon reports `x86_64`, which selects darwin-amd64.
func DetectPlatform(unameS, unameM string) (string, string) {
	goos := ""
	switch unameS {
	case "Darwin":
		goos = "darwin"
	case "Linux":
		goos = "linux"
	}
	goarch := ""
	switch unameM {
	case "arm64", "aarch64":
		goarch = "arm64"
	case "x86_64", "amd64":
		goarch = "amd64"
	}
	return goos, goarch
}

// DetectHostPlatform is DetectPlatform for the running Go process.
func DetectHostPlatform() (string, string) {
	goos := ""
	switch runtime.GOOS {
	case "darwin", "linux":
		goos = runtime.GOOS
	}
	goarch := ""
	switch runtime.GOARCH {
	case "arm64", "amd64":
		goarch = runtime.GOARCH
	}
	return goos, goarch
}

// AssetName is the release asset name for a (goos, goarch) pair.
func AssetName(goos, goarch string) string { return "worktree-gate-" + goos + "-" + goarch }

// CacheBinPath is the version-keyed cache path for the host platform binary.
// An empty version defaults to CLIVersion(home); an empty goos/goarch defaults
// to the host platform.
func CacheBinPath(home, version, goos, goarch string) string {
	if version == "" {
		version = CLIVersion(home)
	}
	if goos == "" || goarch == "" {
		hostOS, hostArch := detectHostPlatform()
		if goos == "" {
			goos = hostOS
		}
		if goarch == "" {
			goarch = hostArch
		}
	}
	return filepath.Join(home, "cache", "bin", "worktree-gate", version, goos+"-"+goarch, "worktree-gate")
}

// DigestMismatchRecordPath is the recorded-mismatch location the doctor reads.
func DigestMismatchRecordPath(home, version string) string {
	if version == "" {
		version = CLIVersion(home)
	}
	return filepath.Join(home, "cache", "bin", "worktree-gate", version, MismatchFilename)
}

// RecordDigestMismatch persists the last acquisition mismatch for the doctor
// check. A write failure is swallowed: a diagnostic must not mask the failure
// it describes.
func RecordDigestMismatch(message, home, version string) {
	path := DigestMismatchRecordPath(home, version)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(message+"\n"), 0o644)
}

// ClearDigestMismatch clears a prior rejection once a replacement is fully
// verified. A stale diagnostic must not make a verified asset fail.
func ClearDigestMismatch(home, version string) {
	_ = os.Remove(DigestMismatchRecordPath(home, version))
}

// LoadExpectedDigests parses the committed SHA256SUMS trust root into
// {asset_name: lowercase_hex_digest}. ok is false when the file exists but
// cannot be read or decoded: the doctor's fail-closed branch (empty trust root
// and unreadable trust root are different states). A missing file is an empty
// root with ok true.
func LoadExpectedDigests(home string) (map[string]string, bool) {
	sumsPath := filepath.Join(home, "catalog", "recipes", "worktree-flow", "bin", "SHA256SUMS")
	digests := map[string]string{}
	if !isFile(sumsPath) {
		return digests, true
	}
	raw, err := os.ReadFile(sumsPath)
	if err != nil {
		return nil, false
	}
	if !utf8.Valid(raw) {
		return nil, false
	}
	for _, line := range splitLines(string(raw)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexFunc(line, unicode.IsSpace)
		if idx < 0 {
			continue
		}
		digest := line[:idx]
		name := strings.TrimSpace(line[idx:])
		if len(digest) == 64 && strings.HasPrefix(name, "worktree-gate-") {
			digests[name] = strings.ToLower(digest)
		}
	}
	return digests, true
}

// expectedDigest reads one committed digest; an unreadable trust root is "no
// digest" here (Acquire must never fail), while ResolveVerifiedBinary fails
// closed through LoadExpectedDigests' ok flag directly.
func expectedDigest(home, asset string) (string, bool) {
	digests, _ := LoadExpectedDigests(home)
	digest, present := digests[asset]
	return digest, present
}

// Sha256Of is the lowercase hex SHA-256 of a file.
func Sha256Of(path string) (string, error) {
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

// VerificationRecordPath is the sidecar carrying the latest acquisition
// verification receipt.
func VerificationRecordPath(binaryPath string) string { return binaryPath + VerificationSuffix }

// ResolveVerifiedBinary returns the executable gate binary to run, or "" when
// none is verified:
//
//  1. $WORKTREE_GATE_BIN when it is a file and executable — the documented
//     debugging/pinning escape that never silently falls through.
//  2. the version-keyed cache candidate, accepted only with its .verified
//     receipt AND — when the committed trust root is readable — a sha256 that
//     still matches it. An unreadable or undecodable trust root fails closed.
//
// It never warns or errors: each caller decides its own fail-open posture.
func ResolveVerifiedBinary(home string) string {
	if override := os.Getenv(GateBinEnv); override != "" {
		if isFile(override) && isExecutable(override) {
			return override
		}
		return ""
	}
	goos, goarch := detectHostPlatform()
	if goos == "" || goarch == "" {
		return ""
	}
	candidate := CacheBinPath(home, "", goos, goarch)
	if !isFile(candidate) || !isExecutable(candidate) || !isFile(VerificationRecordPath(candidate)) {
		return ""
	}
	expected, ok := LoadExpectedDigests(home)
	if !ok {
		return ""
	}
	if digest, present := expected[AssetName(goos, goarch)]; present {
		observed, err := Sha256Of(candidate)
		if err != nil {
			return ""
		}
		if observed != digest {
			return ""
		}
	}
	return candidate
}

// WriteVerificationRecord atomically writes the evidence the launcher hot path
// consumes: a temp file in the destination directory, then a rename. The bytes
// are identical to gate_binary.py:277-280, including the trailing newline.
func WriteVerificationRecord(binaryPath, version, digest string) error {
	path := VerificationRecordPath(binaryPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	content := "status=verified\nversion=" + version + "\ndigest=" + digest + "\nselftest=passed\n"
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// ReadVerificationRecord reads a sidecar receipt without treating malformed
// data as evidence: only `key=value` lines with both sides non-empty survive.
func ReadVerificationRecord(binaryPath string) map[string]string {
	result := map[string]string{}
	path := VerificationRecordPath(binaryPath)
	if !isFile(path) {
		return result
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range splitLines(string(raw)) {
		key, value, found := strings.Cut(line, "=")
		if found && key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

// VerificationEvidence is the dict verify_cached_binary returns.
type VerificationEvidence struct {
	Verified       bool   `json:"verified"`
	ExpectedDigest string `json:"expected_digest"`
	ObservedDigest string `json:"observed_digest"`
	Version        string `json:"version"`
	Selftest       string `json:"selftest"`
	Receipt        string `json:"receipt,omitempty"`
	Reason         string `json:"reason"`
}

// VerifyOptions mirrors verify_cached_binary's keyword-only switches.
type VerifyOptions struct {
	// RequireReceipt also validates the .verified sidecar's status, version,
	// digest and self-test.
	RequireReceipt bool
	// AllowMissingDigest accepts bytes with no committed digest (the local
	// build path).
	AllowMissingDigest bool
}

// VerifyCachedBinary revalidates cache bytes against the committed digest, the
// expected version and a self-test, joining every failure reason with "; ".
// The version/self-test probes only run after the trust-root digest matches: a
// stale or unknown executable is never executed merely to inspect it.
func VerifyCachedBinary(binaryPath, home, version, goos, goarch string, opts VerifyOptions) VerificationEvidence {
	assetName := AssetName(goos, goarch)
	expected, present := expectedDigest(home, assetName)
	evidence := VerificationEvidence{
		Verified:       false,
		ExpectedDigest: expected,
		Selftest:       "not_run",
		Receipt:        VerificationRecordPath(binaryPath),
	}
	reasons := []string{}
	if !isFile(binaryPath) || !isExecutable(binaryPath) {
		evidence.Reason = "cache candidate is missing or not executable"
		return evidence
	}
	observed, err := Sha256Of(binaryPath)
	if err != nil {
		evidence.Reason = fmt.Sprintf("cache digest read failed: %v", err)
		return evidence
	}
	evidence.ObservedDigest = observed
	if !present && !opts.AllowMissingDigest {
		evidence.Reason = fmt.Sprintf("no committed digest for %s", assetName)
		return evidence
	}
	if present && observed != expected {
		evidence.Reason = fmt.Sprintf("digest mismatch: expected %s, got %s", expected, observed)
		return evidence
	}

	reportedVersion := probeBinaryVersion(binaryPath)
	selftest := probeSelftest(binaryPath)
	evidence.Version = reportedVersion
	evidence.Selftest = selftestOrPassed(selftest)
	// The two re-checks below are unreachable: the early returns above already
	// handled both cases. They are kept for line-for-line fidelity with the
	// Python legacy.
	if !present && !opts.AllowMissingDigest {
		reasons = append(reasons, fmt.Sprintf("no committed digest for %s", assetName))
	} else if present && observed != expected {
		reasons = append(reasons, fmt.Sprintf("digest mismatch: expected %s, got %s", expected, observed))
	}
	if reportedVersion != version {
		reasons = append(reasons, fmt.Sprintf("version mismatch: expected %s, got %s", version, orUnknown(reportedVersion)))
	}
	if selftest != "" {
		reasons = append(reasons, fmt.Sprintf("self-test failed: %s", selftest))
	}
	receipt := ReadVerificationRecord(binaryPath)
	if opts.RequireReceipt {
		if receipt["status"] != "verified" {
			reasons = append(reasons, "verification receipt is missing a verified status")
		}
		if receipt["version"] != version {
			reasons = append(reasons, "verification receipt version is missing or stale")
		}
		if receipt["digest"] != observed {
			reasons = append(reasons, "verification receipt digest is missing or stale")
		}
		if receipt["selftest"] != "passed" {
			reasons = append(reasons, "verification receipt does not record a passing self-test")
		}
	}
	if len(reasons) > 0 {
		evidence.Reason = strings.Join(reasons, "; ")
	} else {
		evidence.Reason = "digest, version, and self-test verified"
	}
	evidence.Verified = len(reasons) == 0
	return evidence
}

// QuarantineCachedCandidate moves rejected cache bytes aside so the executable
// path is never selected, carrying the receipt along, and returns the rejected
// path ("" when nothing was moved).
func QuarantineCachedCandidate(binaryPath string) string {
	if !exists(binaryPath) {
		return ""
	}
	digest, err := Sha256Of(binaryPath)
	if err != nil {
		digest = "unknown"
	}
	rejectedDir := filepath.Join(filepath.Dir(binaryPath), "rejected")
	if err := os.MkdirAll(rejectedDir, 0o755); err != nil {
		return ""
	}
	rejected := filepath.Join(rejectedDir, filepath.Base(binaryPath)+"-"+digest)
	if err := os.Rename(binaryPath, rejected); err != nil {
		return ""
	}
	receipt := VerificationRecordPath(binaryPath)
	if exists(receipt) {
		_ = os.Rename(receipt, VerificationRecordPath(rejected))
	}
	return rejected
}

// LocalBuild builds the in-repo Go source into the destination and returns
// (ok, detail). It never touches the network. lookPath resolves the toolchain
// (nil means exec.LookPath); CGO is disabled and GOOS/GOARCH are pinned so the
// output matches the release matrix.
func LocalBuild(binaryPath, home, version, goos, goarch string, lookPath func(string) (string, error)) (bool, string) {
	gateDir := filepath.Join(home, "catalog", "recipes", "worktree-flow", "gate")
	if !isFile(filepath.Join(gateDir, "go.mod")) {
		return false, fmt.Sprintf("gate Go source not found at %s", gateDir)
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	goBin, err := lookPath("go")
	if err != nil {
		return false, "go toolchain not found on PATH"
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return false, fmt.Sprintf("OSError: %v", err)
	}
	tmp := filepath.Join(filepath.Dir(binaryPath), "."+filepath.Base(binaryPath)+".build-tmp")
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "build", "-trimpath", "-buildvcs=false",
		"-ldflags", fmt.Sprintf("-s -w -X main.version=%s", version), "-o", tmp, ".")
	cmd.Dir = gateDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() == context.DeadlineExceeded {
			return false, "TimeoutExpired: go build timed out after 300 seconds"
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = "go build failed"
		}
		return false, detail
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Sprintf("OSError: %v", err)
	}
	if err := os.Rename(tmp, binaryPath); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Sprintf("OSError: %v", err)
	}
	return true, ""
}

// BinaryVersion is the binary's stripped --version stdout, "" on any failure.
func BinaryVersion(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binaryPath, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Selftest runs `--selftest`: "" on success, else the failure text (stderr,
// then stdout, then the literal fallback; a timeout/start error reports its Go
// error text).
func Selftest(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), selftestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "--selftest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return ""
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		detail := stderr.String()
		if detail == "" {
			detail = stdout.String()
		}
		if detail == "" {
			detail = "selftest failed"
		}
		return strings.TrimSpace(detail)
	}
	return strings.TrimSpace(err.Error())
}

// DegradationHint is the fail-open advice appended to every acquisition
// warning.
func DegradationHint(gateImpl string) string {
	return fmt.Sprintf("gate_impl=%s: gate is failing open (run 'ai-specs doctor')", gateImpl)
}

// AssetURL is the release download URL for one asset from the canonical
// repository owner.
func AssetURL(version, assetName string) string {
	return DefaultBaseURL + "/v" + version + "/" + assetName
}

// AcquireOptions is the injection boundary. Empty Home falls back to
// $AI_SPECS_HOME; empty GOOS/GOARCH fall back to the host platform; an empty
// BaseURL is the canonical GitHub release base; a nil HTTP client is a 60 s
// client; a nil LookPath is exec.LookPath.
type AcquireOptions struct {
	GateImpl string
	Home     string
	Offline  bool
	GOOS     string
	GOARCH   string
	BaseURL  string
	HTTP     *http.Client
	LookPath func(string) (string, error)
}

// AcquireStatus carries Python acquire()'s keys. Warn and Mismatch are
// pointers because Python distinguishes None from a string; Verification is
// present only for the paths that build evidence.
type AcquireStatus struct {
	Attempted    bool                  `json:"attempted"`
	Installed    bool                  `json:"installed"`
	Platform     [2]string             `json:"platform"`
	CachePath    string                `json:"cache_path"`
	Warn         *string               `json:"warn"`
	Mismatch     *string               `json:"mismatch"`
	Verification *VerificationEvidence `json:"verification,omitempty"`
}

// Acquire acquires the gate binary for the host platform. It never returns an
// error and never raises: every failure warns and degrades, `auto` and `go`
// both fail open with a recorded doctor ERROR, and there is no Bash fallback.
func Acquire(opts AcquireOptions) AcquireStatus {
	home := opts.Home
	if home == "" {
		home = os.Getenv("AI_SPECS_HOME")
	}
	version := CLIVersion(home)
	goos, goarch := detectHostPlatform()
	if opts.GOOS != "" {
		goos = opts.GOOS
	}
	if opts.GOARCH != "" {
		goarch = opts.GOARCH
	}
	client := opts.HTTP
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	out := AcquireStatus{Platform: [2]string{goos, goarch}}

	// Platform outside the published matrix (or unknown) → no binary target.
	if goos == "" || goarch == "" || !supportedPlatform(goos, goarch) {
		out.Attempted = true
		out.Warn = strPtr(fmt.Sprintf(
			"worktree-gate: unsupported platform (%s/%s); no binary acquired — "+
				"available: darwin/arm64, darwin/amd64, linux/amd64, linux/arm64",
			orQuestion(goos), orQuestion(goarch)))
		return out
	}

	binaryPath := CacheBinPath(home, version, goos, goarch)
	out.CachePath = binaryPath
	out.Attempted = true

	assetName := AssetName(goos, goarch)
	expected, expectedPresent := expectedDigest(home, assetName)
	cacheRejection := ""
	hasCacheRejection := false

	// Cache hits are evidence candidates, not acceptance: revalidate every
	// current digest/version/self-test before writing a launcher receipt.
	if isFile(binaryPath) && isExecutable(binaryPath) {
		evidence := VerifyCachedBinary(binaryPath, home, version, goos, goarch, VerifyOptions{})
		out.Verification = &evidence
		if evidence.Verified {
			if err := WriteVerificationRecord(binaryPath, version, evidence.ObservedDigest); err != nil {
				mismatch := fmt.Sprintf(
					"worktree-gate: verification receipt write failed for %s: %v; candidate never executed",
					binaryPath, err)
				RecordDigestMismatch(mismatch, home, version)
				out.Mismatch = &mismatch
				out.Warn = &mismatch
				return out
			}
			ClearDigestMismatch(home, version)
			out.Mismatch = nil
			out.Installed = true
			return out
		}
		reason := evidence.Reason
		if reason == "" {
			reason = "verification failed"
		}
		recovery := QuarantineCachedCandidate(binaryPath)
		cacheRejection = fmt.Sprintf(
			"worktree-gate: rejected cached candidate at %s: %s; recovery=%s; candidate never executed",
			binaryPath, reason, orNone(recovery))
		hasCacheRejection = true
		RecordDigestMismatch(cacheRejection, home, version)
		out.Mismatch = &cacheRejection
	}

	// Opt-in local build: AI_SPECS_GATE_BUILD=1, or offline with go present.
	wantBuild := os.Getenv("AI_SPECS_GATE_BUILD") == "1"
	goOnPath := false
	if _, err := lookPath("go"); err == nil {
		goOnPath = true
	}
	if wantBuild || (opts.Offline && goOnPath) {
		ok, detail := LocalBuild(binaryPath, home, version, goos, goarch, lookPath)
		if ok {
			evidence := VerifyCachedBinary(binaryPath, home, version, goos, goarch,
				VerifyOptions{AllowMissingDigest: true})
			localDigestOK := !expectedPresent || evidence.ObservedDigest == expected
			localVersionOK := evidence.Version == version
			localSelftestOK := evidence.Selftest == "passed"
			if localDigestOK && localVersionOK && localSelftestOK {
				if err := WriteVerificationRecord(binaryPath, version, evidence.ObservedDigest); err != nil {
					mismatch := fmt.Sprintf(
						"worktree-gate: local verification receipt write failed at %s: %v; candidate never executed",
						binaryPath, err)
					RecordDigestMismatch(mismatch, home, version)
					out.Mismatch = &mismatch
					out.Warn = &mismatch
					return out
				}
				ClearDigestMismatch(home, version)
				out.Mismatch = nil
				out.Installed = true
				out.Verification = &evidence
				return out
			}
			reason := evidence.Reason
			if reason == "" {
				reason = "local build verification failed"
			}
			recovery := QuarantineCachedCandidate(binaryPath)
			mismatch := fmt.Sprintf(
				"worktree-gate: locally built binary rejected at %s: %s; recovery=%s; candidate never executed",
				binaryPath, reason, orNone(recovery))
			RecordDigestMismatch(mismatch, home, version)
			out.Mismatch = &mismatch
			out.Warn = &mismatch
			return out
		}
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: local build failed (%s); %s",
			detail, DegradationHint(opts.GateImpl)))
		return out
	}

	if opts.Offline {
		prefix := ""
		if hasCacheRejection {
			prefix = cacheRejection + "; "
		}
		out.Warn = strPtr(prefix + "worktree-gate: offline with no cached binary; " +
			DegradationHint(opts.GateImpl))
		return out
	}

	if !expectedPresent {
		out.Warn = strPtr(fmt.Sprintf(
			"worktree-gate: no committed digest for %s in "+
				"catalog/recipes/worktree-flow/bin/SHA256SUMS; %s",
			assetName, DegradationHint(opts.GateImpl)))
		return out
	}

	url := baseURL + "/v" + version + "/" + assetName
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: download failed from %s (%v); %s",
			url, err, DegradationHint(opts.GateImpl)))
		return out
	}
	tmp := filepath.Join(filepath.Dir(binaryPath), "."+filepath.Base(binaryPath)+".download")
	defer func() {
		if exists(tmp) {
			_ = os.Remove(tmp)
		}
	}()

	if err := downloadTo(client, url, tmp); err != nil {
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: download failed from %s (%v); %s",
			url, err, DegradationHint(opts.GateImpl)))
		return out
	}
	actual, err := Sha256Of(tmp)
	if err != nil {
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: download failed from %s (%v); %s",
			url, err, DegradationHint(opts.GateImpl)))
		return out
	}
	if actual != expected {
		_ = os.Remove(tmp)
		mismatch := fmt.Sprintf(
			"worktree-gate: digest mismatch for %s: expected %s, got %s; artifact deleted and never executed",
			assetName, expected, actual)
		RecordDigestMismatch(mismatch, home, version)
		out.Mismatch = &mismatch
		out.Warn = &mismatch
		return out
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: download failed from %s (%v); %s",
			url, err, DegradationHint(opts.GateImpl)))
		return out
	}
	selftest := probeSelftest(tmp)
	reportedVersion := probeBinaryVersion(tmp)
	if selftest != "" || reportedVersion != version {
		mismatch := fmt.Sprintf(
			"worktree-gate: downloaded binary rejected for %s: version=%s expected_version=%s; "+
				"selftest=%s; artifact never installed",
			assetName, orUnknown(reportedVersion), version, selftestOrPassed(selftest))
		RecordDigestMismatch(mismatch, home, version)
		out.Mismatch = &mismatch
		out.Warn = &mismatch
		out.Verification = &VerificationEvidence{
			Verified:       false,
			ExpectedDigest: expected,
			ObservedDigest: actual,
			Version:        reportedVersion,
			Selftest:       selftestOrPassed(selftest),
			Reason:         mismatch,
		}
		return out
	}
	if err := os.Rename(tmp, binaryPath); err != nil {
		out.Warn = strPtr(fmt.Sprintf("worktree-gate: download failed from %s (%v); %s",
			url, err, DegradationHint(opts.GateImpl)))
		return out
	}
	if err := WriteVerificationRecord(binaryPath, version, actual); err != nil {
		_ = os.Remove(binaryPath)
		mismatch := fmt.Sprintf(
			"worktree-gate: verification receipt write failed for %s: %v; candidate never executed",
			binaryPath, err)
		RecordDigestMismatch(mismatch, home, version)
		out.Mismatch = &mismatch
		out.Warn = &mismatch
		return out
	}
	out.Installed = true
	ClearDigestMismatch(home, version)
	out.Mismatch = nil
	reason := "acquired and verified"
	if hasCacheRejection {
		reason = "re-acquired after " + cacheRejection
	}
	out.Verification = &VerificationEvidence{
		Verified:       true,
		ExpectedDigest: expected,
		ObservedDigest: actual,
		Version:        reportedVersion,
		Selftest:       "passed",
		Receipt:        VerificationRecordPath(binaryPath),
		Reason:         reason,
	}
	return out
}

// --- helpers ----------------------------------------------------------------

func downloadTo(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func supportedPlatform(goos, goarch string) bool {
	for _, platform := range SupportedPlatforms {
		if platform[0] == goos && platform[1] == goarch {
			return true
		}
	}
	return false
}

// splitLines mirrors Python str.splitlines for the line boundaries the trust
// root and receipts can contain (\n, \r\n, \r).
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// isFile mirrors Path.is_file(): a regular file, following symlinks.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// exists mirrors Path.exists(): false only on a "not found" stat error.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isExecutable mirrors os.access(path, os.X_OK) via the mode bits.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func orQuestion(value string) string {
	if value == "" {
		return "?"
	}
	return value
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func selftestOrPassed(selftest string) string {
	if selftest == "" {
		return "passed"
	}
	return selftest
}

func strPtr(value string) *string { return &value }
