// Package provider is the native port of the read-only resolution surface of
// lib/_internal/provider_install.py: resolve_provider (:382) plus its helpers
// (detect_platform :85, expected_binary_name :106, VERSION_RE :39,
// _version_tuple :311, _version_at_least :316, _run_version :322, cache_root
// :340, managed_binary_path :350, _read_receipt :354, _managed_candidates
// :363, _sha256 :192). It is the single authority the doctor dep-check and the
// recipe-MCP materializer (PR4b-2) share, so the two never diverge.
//
// Faithfulness notes (divergences pinned by the tests, never claimed as parity):
//   - _managed_candidates order: Python's Path.iterdir follows listdir order,
//     while os.ReadDir sorts names. Observable only when two candidates tie on
//     their version tuple, because max/`>` then keeps the first.
//   - _version_tuple overflow: Python ints are unbounded; strconv.Atoi
//     saturates an out-of-range component at math.MaxInt64, so a component past
//     2^63 is mis-ordered rather than rejected.
//   - _run_version error text: the timeout and no-usable-version messages are
//     reproduced byte for byte; an OS error is reproduced for the errno families
//     Go surfaces, and otherwise falls back to Go's wording.
//   - detect_platform uses the Go runtime's GOOS/GOARCH (build-time), which is
//     the port of platform.system()/platform.machine() for the native binary.
package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ai-specs.dev/ai-specs/internal/schema"
)

// Repository is provider_install.PROVIDER_REPOSITORY.
const Repository = "parada1104/jinna-provider"

// Resolution source values (ProviderResolution.source).
const (
	SourcePath       = "path"
	SourceManaged    = "managed"
	SourceUnresolved = "unresolved"
)

// InstallError mirrors provider_install.InstallError. Error returns the Python
// str(exc) for every case runVersion reproduces.
type InstallError struct{ Message string }

func (e *InstallError) Error() string { return e.Message }

// Resolution mirrors the frozen provider_install.ProviderResolution dataclass
// field for field; Path is nil where Python holds None.
type Resolution struct {
	Binary     string
	Command    string
	Version    string
	Source     string
	Path       *string
	Repository string
	ReleaseTag string
	Target     [2]string
	Verified   bool
}

var providerSystems = map[string]string{
	"darwin": "darwin", "macos": "darwin", "linux": "linux", "windows": "windows",
}

var providerMachines = map[string]string{
	"arm64": "arm64", "aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64", "x64": "amd64",
}

var supportedPlatforms = map[[2]string]bool{
	{"darwin", "arm64"}:  true,
	{"darwin", "amd64"}:  true,
	{"linux", "arm64"}:   true,
	{"linux", "amd64"}:   true,
	{"windows", "amd64"}: true,
}

// DetectPlatform mirrors provider_install.detect_platform. A nil argument is
// Python's None (use the runtime platform); a non-nil argument is the provided
// value, so a pointer to "" is distinct from nil exactly as Python's "" is
// distinct from None.
func DetectPlatform(system, machine *string) (string, string) {
	rawSystem, rawMachine := runtime.GOOS, runtime.GOARCH
	if system != nil {
		rawSystem = *system
	}
	if machine != nil {
		rawMachine = *machine
	}
	goos := providerSystems[strings.ToLower(strings.TrimSpace(rawSystem))]
	goarch := providerMachines[strings.ToLower(strings.TrimSpace(rawMachine))]
	if !supportedPlatforms[[2]string{goos, goarch}] {
		return "", ""
	}
	return goos, goarch
}

// ExpectedBinaryName is provider_install.expected_binary_name.
func ExpectedBinaryName(goos string) string {
	if goos == "windows" {
		return "jinna.exe"
	}
	return "jinna"
}

// providerVersionRe is VERSION_RE's `v?(\d+(?:\.\d+)+)` body (anchored) without
// the lookarounds RE2 rejects; VersionTuple enforces the digit boundaries by
// hand. Python's `\d` also matches non-ASCII decimal digits while `[0-9]` does
// not, a divergence pinned by TestVersionTupleUnicodeDigiDivergence.
var providerVersionRe = regexp.MustCompile(`^v?[0-9]+(?:\.[0-9]+)+`)

// VersionTuple mirrors provider_install._version_tuple, including the
// `(?<!\d)`/`(?!\d)` digit boundaries and the leading optional `v`. At least
// one `.` is required; an unmatched text yields nil (Python's ()).
func VersionTuple(text string) []int {
	for i := 0; i < len(text); i++ {
		if i > 0 && isASCIIDigit(text[i-1]) {
			continue
		}
		loc := providerVersionRe.FindStringIndex(text[i:])
		if loc == nil {
			continue
		}
		end := i + loc[1]
		if end < len(text) && isASCIIDigit(text[end]) {
			continue
		}
		match := text[i:end]
		if match[0] == 'v' {
			match = match[1:]
		}
		parts := strings.Split(match, ".")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			// Overflow saturates at math.MaxInt64 (Python ints are unbounded): a pinned
			// divergence, since the value is not rejected.
			n, _ := strconv.Atoi(part)
			out = append(out, n)
		}
		return out
	}
	return nil
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// versionAtLeast is provider_install._version_at_least: raw Python tuple
// comparison (a shorter prefix tuple is smaller). This is deliberately NOT the
// zero-padded dep_check._version_ge.
func versionAtLeast(have, minimum string) bool {
	want := VersionTuple(minimum)
	if len(want) == 0 {
		return true
	}
	current := VersionTuple(have)
	if len(current) == 0 {
		return false
	}
	return compareIntTuples(current, want) >= 0
}

// compareIntTuples mirrors Python's tuple comparison for int tuples: the
// shorter equal-prefix tuple is smaller.
func compareIntTuples(a, b []int) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func joinVersion(parts []int) string {
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = strconv.Itoa(part)
	}
	return strings.Join(out, ".")
}

// versionTimeout is provider_install._run_version's default timeout.
const versionTimeout = 5 * time.Second

// runVersion mirrors provider_install._run_version: `<path> version`, 5s
// timeout, both streams concatenated stdout-first, and a non-zero exit or an
// unparseable output is an InstallError. Its message reproduces Python's
// str(exc) for the reproduced cases (see the package comment).
func runVersion(path string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", &InstallError{Message: "provider version check failed: " + timeoutMessage(path, timeout)}
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", &InstallError{Message: "provider version check failed: " + osErrorMessage(err, path)}
		}
	}
	version := VersionTuple(stdout.String() + stderr.String())
	if exitCode != 0 || len(version) == 0 {
		return "", &InstallError{Message: "provider version check returned no usable version"}
	}
	return joinVersion(version), nil
}

// timeoutMessage reproduces subprocess.TimeoutExpired.__str__ for the list-argv
// case: "Command '['<path>', 'version']' timed out after 5.0 seconds".
func timeoutMessage(path string, timeout time.Duration) string {
	return "Command '[" + pyRepr(path) + ", 'version']' timed out after " + pyFloat(timeout) + " seconds"
}

// osErrorMessage reproduces Python's OSError str "[Errno N] <strerror>: '<path>'"
// for the errno families Go surfaces through *os.PathError; otherwise Go's
// message stands.
func osErrorMessage(err error, path string) string {
	var pathErr *os.PathError
	var errno syscall.Errno
	if errors.As(err, &pathErr) && errors.As(pathErr.Err, &errno) {
		return fmt.Sprintf("[Errno %d] %s: %s", int(errno), capitalizeFirst(errno.Error()), pyRepr(path))
	}
	return err.Error()
}

func capitalizeFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// pyFloat renders a duration's seconds like Python's str(float): shortest
// round-trip with a trailing ".0" for integral values.
func pyFloat(d time.Duration) string {
	s := strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// pyRepr is the subset of Python's str repr the timeout message needs:
// single-quoted unless the string holds a single quote and no double quote.
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\':
			b.WriteString(`\\`)
		case c == quote:
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// CacheRoot mirrors provider_install.cache_root: an explicit AI_SPECS_HOME
// argument, then the AI_SPECS_HOME env var (empty is falsy, so it falls
// through), then ~/.cache/ai-specs; then expanduser + resolve(strict=False);
// then "cache/bin/jinna".
func CacheRoot(aiSpecsHome *string) string {
	home := ""
	switch {
	case aiSpecsHome != nil:
		home = *aiSpecsHome
	case os.Getenv("AI_SPECS_HOME") != "":
		home = os.Getenv("AI_SPECS_HOME")
	default:
		if userHome, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(userHome, ".cache", "ai-specs")
		}
	}
	return filepath.Join(resolvePath(expandUser(home)), "cache", "bin", "jinna")
}

// expandUser is Path.expanduser for the `~` / `~/...` forms (the `~user` form
// is not supported; a divergence no shared fixture exercises).
func expandUser(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if userHome, err := os.UserHomeDir(); err == nil {
			return filepath.Join(userHome, p[1:])
		}
	}
	return p
}

// resolvePath mirrors Path.resolve(strict=False): make absolute, then resolve
// symlinks from the longest existing prefix and re-append any missing tail
// unchanged, so a not-yet-created cache directory still resolves its parents.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	existing := abs
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(append([]string{resolved}, reverseStrings(tail)...)...)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		tail = append(tail, filepath.Base(existing))
		existing = parent
	}
	return filepath.Clean(abs)
}

func reverseStrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}

// ManagedBinaryPath is provider_install.managed_binary_path.
func ManagedBinaryPath(root, version, goos, goarch string) string {
	return filepath.Join(root, version, goos+"-"+goarch, ExpectedBinaryName(goos))
}

// readReceipt mirrors provider_install._read_receipt: the sibling install.json,
// or an empty (nil) map on any read/parse error or non-object JSON.
func readReceipt(candidate string) map[string]any {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(candidate), "install.json"))
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	return data
}

// managedCandidates mirrors provider_install._managed_candidates. See the
// package comment for the listdir-order divergence.
func managedCandidates(root, goos, goarch string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	target := goos + "-" + goarch
	expected := ExpectedBinaryName(goos)
	var paths []string
	for _, entry := range entries {
		candidate := filepath.Join(root, entry.Name(), target, expected)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			paths = append(paths, candidate)
		}
	}
	return paths
}

// sha256File is provider_install._sha256.
func sha256File(path string) (string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, fh); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Resolve mirrors provider_install.resolve_provider: PATH first, then the
// newest trust-checked managed candidate. A non-nil error is the winner
// re-check's InstallError, which Python leaves outside every try so it
// propagates; every other failure degrades to an unresolved Resolution.
func Resolve(dep *schema.CliDep, aiSpecsHome *string) (Resolution, error) {
	goos, goarch := DetectPlatform(nil, nil)
	base := Resolution{
		Binary:     dep.Binary,
		Command:    dep.Binary,
		Repository: Repository,
		Target:     [2]string{goos, goarch},
	}

	if pathCandidate, err := exec.LookPath(dep.Binary); err == nil {
		if version, verr := runVersion(pathCandidate, versionTimeout); verr == nil &&
			versionAtLeast(version, dep.MinVersion) {
			candidate := pathCandidate
			base.Version = version
			base.Source = SourcePath
			base.Path = &candidate
			base.Verified = true
			return base, nil
		}
	}

	root := CacheRoot(aiSpecsHome)
	target := goos + "-" + goarch
	var bestVersion []int
	var bestPath, bestTag string
	for _, candidate := range managedCandidates(root, goos, goarch) {
		receipt := readReceipt(candidate)
		if status, _ := receipt["status"].(string); status != "verified" {
			continue
		}
		if repository, _ := receipt["repository"].(string); repository != Repository {
			continue
		}
		if receiptTarget, _ := receipt["target"].(string); receiptTarget != target {
			continue
		}
		tag, ok := receipt["release_tag"].(string)
		if !ok || len(VersionTuple(tag)) == 0 {
			continue
		}
		digest, ok := receipt["binary_sha256"].(string)
		if !ok {
			continue
		}
		sum, err := sha256File(candidate)
		if err != nil || sum != digest {
			continue
		}
		version, err := runVersion(candidate, versionTimeout)
		if err != nil || !versionAtLeast(version, dep.MinVersion) {
			continue
		}
		if compareIntTuples(VersionTuple(version), VersionTuple(tag)) != 0 {
			// A receipt that disagrees with the executable is not a trust anchor.
			continue
		}
		v := VersionTuple(version)
		if bestPath == "" || compareIntTuples(v, bestVersion) > 0 {
			bestVersion, bestPath, bestTag = v, candidate, tag
		}
	}
	if bestPath != "" {
		version, err := runVersion(bestPath, versionTimeout)
		if err != nil {
			return base, err
		}
		candidate := bestPath
		base.Command = bestPath
		base.Version = version
		base.Source = SourceManaged
		base.Path = &candidate
		base.ReleaseTag = bestTag
		base.Verified = true
		return base, nil
	}

	base.Source = SourceUnresolved
	return base, nil
}
