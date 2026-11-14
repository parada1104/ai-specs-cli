// Package provider is the native port of the read-only resolution surface of
// lib/_internal/provider_install.py: resolve_provider (:382) plus its helpers
// (detect_platform :85, expected_binary_name :106, VERSION_RE :39,
// _version_tuple :311, _version_at_least :316, _run_version :322, cache_root
// :340, managed_binary_path :350, _read_receipt :354, _managed_candidates
// :363, _sha256 :192). It is the single authority the doctor dep-check and the
// recipe-MCP materializer (PR4b-2) share, so the two never diverge.
//
// Faithfulness notes (divergences pinned by the tests, never claimed as parity):
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
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

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

// providerVersionRe is VERSION_RE's `v?(\d+(?:\.\d+)+)` body (anchored) with the
// Unicode `\d` class spelled `[\p{Nd}]` and the lookarounds RE2 rejects
// applied by hand in versionTupleAt.
var providerVersionRe = regexp.MustCompile(`^v?[\p{Nd}]+(?:\.[\p{Nd}]+)+`)

// VersionTuple mirrors provider_install._version_tuple, including the
// `(?<!\d)`/`(?!\d)` digit boundaries and the leading optional `v`. At least
// one `.` is required; an unmatched text yields nil (Python's ()). Components
// are arbitrary precision, matching Python's unbounded int.
func VersionTuple(text string) []*big.Int {
	for i := range text {
		if tuple, ok := versionTupleAt(text, i); ok {
			return tuple
		}
	}
	return nil
}

// versionTupleAt is one anchored re.search(VERSION_RE, text[start:]): it matches
// only at byte offset start and enforces Python's (?<!\d) lookbehind (the rune
// before start is not an Nd digit) and (?!\d) lookahead (the rune after the
// match is not an Nd digit).
func versionTupleAt(text string, start int) ([]*big.Int, bool) {
	if start > 0 {
		if prev, _ := utf8.DecodeLastRuneInString(text[:start]); isDecimalDigit(prev) {
			return nil, false
		}
	}
	loc := providerVersionRe.FindStringIndex(text[start:])
	if loc == nil {
		return nil, false
	}
	end := start + loc[1]
	if end < len(text) {
		if next, _ := utf8.DecodeRuneInString(text[end:]); isDecimalDigit(next) {
			return nil, false
		}
	}
	match := text[start:end]
	if match[0] == 'v' {
		match = match[1:]
	}
	parts := strings.Split(match, ".")
	out := make([]*big.Int, 0, len(parts))
	for _, part := range parts {
		n, ok := parseDecimalInt(part)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// isDecimalDigit is Python's `\d`, the Unicode decimal-digit category Nd.
func isDecimalDigit(r rune) bool { return unicode.Is(unicode.Nd, r) }

// parseDecimalInt is Python's int() for a run of Nd digits: each digit maps to
// its numeric value and the result is an arbitrary-precision integer.
func parseDecimalInt(digits string) (*big.Int, bool) {
	var b strings.Builder
	b.Grow(len(digits))
	for _, r := range digits {
		value, ok := decimalDigitValue(r)
		if !ok {
			return nil, false
		}
		b.WriteByte(byte('0' + value))
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	if !ok {
		return nil, false
	}
	return n, true
}

// decimalDigitValue returns an Nd digit's numeric value. Unicode lays the digits
// of each set out contiguously, so the value is the code point's offset inside
// its Nd range, modulo ten for the mathematical blocks that merge several sets
// into one range.
func decimalDigitValue(r rune) (int, bool) {
	if !isDecimalDigit(r) {
		return 0, false
	}
	for _, rng := range unicode.Nd.R16 {
		if rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int((r-rune(rng.Lo))/rune(rng.Stride)) % 10, true
		}
	}
	for _, rng := range unicode.Nd.R32 {
		if rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int((r-rune(rng.Lo))/rune(rng.Stride)) % 10, true
		}
	}
	return 0, false
}

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
	return compareVersionTuples(current, want) >= 0
}

// compareVersionTuples mirrors Python's tuple comparison for int tuples: the
// shorter equal-prefix tuple is smaller.
func compareVersionTuples(a, b []*big.Int) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := a[i].Cmp(b[i]); c != 0 {
			return c
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

func joinVersion(parts []*big.Int) string {
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = part.String()
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

// managedCandidates mirrors provider_install._managed_candidates, including its
// listdir order: Readdirnames preserves the raw OS order that Python's
// Path.iterdir follows, where os.ReadDir would impose a sort Python never does.
func managedCandidates(root, goos, goarch string) []string {
	fh, err := os.Open(root)
	if err != nil {
		return nil
	}
	names, err := fh.Readdirnames(-1)
	fh.Close()
	if err != nil {
		return nil
	}
	target := goos + "-" + goarch
	expected := ExpectedBinaryName(goos)
	var paths []string
	for _, name := range names {
		candidate := filepath.Join(root, name, target, expected)
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
	var bestVersion []*big.Int
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
		if compareVersionTuples(VersionTuple(version), VersionTuple(tag)) != 0 {
			// A receipt that disagrees with the executable is not a trust anchor.
			continue
		}
		v := VersionTuple(version)
		if bestPath == "" || compareVersionTuples(v, bestVersion) > 0 {
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
