package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

// TestVersionTupleParityAgainstPython pins provider_install._version_tuple
// through the frozen authority in testdata/provider_golden.json: a component
// past the int64 range keeps its full value (Python ints are unbounded) and
// Unicode Nd digits are accepted and mapped to their numeric value. Both were
// recorded divergences until S14-Δ.
func TestVersionTupleParityAgainstPython(t *testing.T) {
	oracle := loadFrozenOracle(t)

	cases := []struct{ name, text string }{
		{"ascii", "jinna 1.2.3"},
		{"overflow", "jinna 99999999999999999999.1"},
		{"overflow-mixed", "jinna 1.99999999999999999999"},
		{"arabic-indic", "jinna \u0661\u0662.\u0663"},
		{"devanagari", "jinna \u0966\u0967.\u0968"},
		{"nd-then-ascii", "jinna \u06613.4"},
		{"nd-before-v", "jinna \u0661v2.3"},
		{"no-version", "jinna 123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := oracle.record(t, "version-parity/"+tc.name, "runversion")
			if got := rec.inputText(t); got != tc.text {
				t.Fatalf("case input drift: frozen %q, built %q", got, tc.text)
			}
			var ref providerRefVersion
			if err := json.Unmarshal([]byte(rec.Python), &ref); err != nil {
				t.Fatalf("decode frozen ref %s: %v", rec.Python, err)
			}
			got := joinVersion(VersionTuple(tc.text))
			if ref.Error != "" {
				if len(VersionTuple(tc.text)) != 0 {
					t.Fatalf("VersionTuple(%q) = %q, want no version (frozen: %q)", tc.text, got, ref.Error)
				}
				return
			}
			if got != ref.Version {
				t.Fatalf("VersionTuple(%q) = %q, frozen = %q", tc.text, got, ref.Version)
			}
		})
	}
}

// TestVersionTupleUnicode16NdResidual pins a DELIBERATE, enumerated residual,
// not parity: the native port's `\p{Nd}` table comes from the Go toolchain in
// use (Go 1.24.13 carries Unicode 15.0.0), while CPython 3.14.7's `\d` follows
// Unicode 16.0.0. Unicode 16.0.0 adds 80 Nd code points that CPython accepts
// and the Go table misses, so the Python authority parses a version containing
// them and the native port cannot. This is a Unicode table-generation skew,
// never a logic gap: a future Go release carrying the Unicode 16 Nd table
// closes it by itself. That is why each failure below says the pin is
// deliberate -- when it starts failing, remove the pin on purpose instead of
// deleting the assertion by accident.
//
// The representative code points are the first of each Unicode 16.0 Nd block
// that Go 1.24.13 is missing, observed by enumerating `unicode.Nd` against
// CPython's `\d`: 10D40-10D49, 116D0-116E3, 11BF0-11BF9, 16130-16139,
// 16D70-16D79, 1CCF0-1CCF9, 1E5F1-1E5FA. The numeric values are driven by the
// real authority, never hard-coded.
func TestVersionTupleUnicode16NdResidual(t *testing.T) {
	oracle := loadFrozenOracle(t)

	missing := []rune{0x10D40, 0x116D0, 0x11BF0, 0x16130, 0x16D70, 0x1CCF0, 0x1E5F1}
	for _, cp := range missing {
		t.Run(fmt.Sprintf("U+%05X", cp), func(t *testing.T) {
			text := "jinna " + string(cp) + "." + string(cp+1)
			rec := oracle.record(t, fmt.Sprintf("unicode16-residual/U+%05X", cp), "runversion")
			if got := rec.inputText(t); got != text {
				t.Fatalf("case input drift: frozen %q, built %q", got, text)
			}
			var ref providerRefVersion
			if err := json.Unmarshal([]byte(rec.Python), &ref); err != nil {
				t.Fatalf("decode frozen ref %s: %v", rec.Python, err)
			}
			// Frozen Python authority (Unicode 16.0 `\d`) accepts the code points
			// and returns the numeric tuple; the block's first two digits are 0 and
			// 1, so the version is "0.1".
			if ref.Error != "" || ref.Version != "0.1" {
				t.Fatalf("frozen authority for U+%05X = %+v, want version 0.1: the deliberate residual assumes the authority accepts this digit", cp, ref)
			}
			// Native port (Go 1.24.13 Unicode 15.0 `\p{Nd}`) still rejects it.
			if got := VersionTuple(text); got != nil {
				t.Fatalf("native VersionTuple(%q) = %q, want nil: this residual is DELIBERATE -- Go 1.24.13 carries Unicode 15.0, so U+%05X sits outside unicode.Nd while the frozen CPython authority accepts it. Passing here means the toolchain gained the Unicode 16 table; remove this pin on purpose rather than deleting the assertion.", text, joinVersion(got), cp)
			}
		})
	}
}

// TestVersionTupleUnicodeBoundary pins the Nd-aware `(?<!\d)` lookbehind: an Nd
// digit directly before a candidate suppresses a match starting there, exactly
// where Python's VERSION_RE suppresses it. The suppression is at the match
// position, not the value: Python's search finds the same tuple at the later
// position, which is why value-level parity lives in
// TestVersionTupleParityAgainstPython. The `(?!\d)` lookahead is Nd-aware too,
// but cannot fire because the greedy Nd run already consumes any adjacent Nd
// digit; it is kept for faithfulness to the Python pattern.
func TestVersionTupleUnicodeBoundary(t *testing.T) {
	nd := "\u0661" // ARABIC-INDIC DIGIT ONE, two UTF-8 bytes
	if got := joinVersion(VersionTuple(nd + "3.4")); got != "13.4" {
		t.Fatalf("VersionTuple(%q) = %q, want the Nd digit mapped to 13.4", nd+"3.4", got)
	}
	// The 'v' sits at byte len(nd); the preceding Nd digit must suppress it.
	if _, ok := versionTupleAt(nd+"v2.3", len(nd)); ok {
		t.Fatalf("versionTupleAt(%q, %d) matched; the Nd lookbehind must suppress a match there", nd+"v2.3", len(nd))
	}
	if _, ok := versionTupleAt("xv2.3", 1); !ok {
		t.Fatalf("versionTupleAt(%q, 1) did not match; a non-digit lookbehind must allow it", "xv2.3")
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

// TestManagedCandidatesNoOrdering pins that managedCandidates adds no ordering
// of its own: it must return the version directories in the raw OS order that
// provider_install._managed_candidates inherits from Path.iterdir, never the
// sorted order os.ReadDir would impose. The raw-order comparison is
// deterministic on any filesystem (it holds whatever that order is); the
// "not sorted" assertion applies only when the filesystem happens to hand out
// a non-sorted listing, and is skipped with a log otherwise.
func TestManagedCandidatesNoOrdering(t *testing.T) {
	goos, goarch := requireTarget(t)
	oracle := loadFrozenOracle(t)
	rec := oracle.record(t, "managed-candidates/no-ordering", "candidates")
	var inputs struct {
		RootDirs  []string `json:"root_dirs"`
		EmptyDirs []string `json:"empty_dirs"`
		Goos      string   `json:"goos"`
		Goarch    string   `json:"goarch"`
	}
	if err := json.Unmarshal(rec.Inputs, &inputs); err != nil {
		t.Fatalf("decode frozen inputs: %v", err)
	}
	if inputs.Goos != "<GOOS>" || inputs.Goarch != "<GOARCH>" {
		t.Fatalf("case input host drift: frozen %s/%s, want the <GOOS>/<GOARCH> tokens", inputs.Goos, inputs.Goarch)
	}
	root := t.TempDir()
	for _, name := range inputs.RootDirs {
		writeExecutable(t, ManagedBinaryPath(root, name, goos, goarch), "#!/bin/sh\necho jinna 1.0.0\n")
	}
	for _, name := range inputs.EmptyDirs {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got := managedCandidates(root, goos, goarch)
	expected := rawManagedCandidates(t, root, goos, goarch)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("managedCandidates reordered the directory\n go  = %q\n raw = %q", got, expected)
	}

	// Direct frozen-oracle parity: the authority walked this same directory,
	// so both legs must agree entry for entry.
	var ref struct {
		Candidates []string `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(rec.Python, "<TMP>", root)), &ref); err != nil {
		t.Fatalf("decode frozen ref %s: %v", rec.Python, err)
	}
	for i := range ref.Candidates {
		ref.Candidates[i] = replaceHostTokens(ref.Candidates[i], goos, goarch)
	}
	if !reflect.DeepEqual(got, ref.Candidates) {
		t.Fatalf("candidate order diverges from the frozen oracle\n go = %q\n frozen = %q", got, ref.Candidates)
	}

	sorted := append([]string(nil), expected...)
	sort.Strings(sorted)
	if reflect.DeepEqual(expected, sorted) {
		t.Log("raw directory order is already sorted here; the no-sort assertion is vacuous on this filesystem")
		return
	}
	if reflect.DeepEqual(got, sorted) {
		t.Fatalf("managedCandidates returned the sorted order %q, want the raw order %q", got, expected)
	}
}

// rawManagedCandidates reproduces the unsorted listing managedCandidates must
// preserve, using Readdirnames directly.
func rawManagedCandidates(t *testing.T, root, goos, goarch string) []string {
	t.Helper()
	fh, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	names, err := fh.Readdirnames(-1)
	fh.Close()
	if err != nil {
		t.Fatal(err)
	}
	target := goos + "-" + goarch
	var expected []string
	for _, name := range names {
		candidate := filepath.Join(root, name, target, ExpectedBinaryName(goos))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			expected = append(expected, candidate)
		}
	}
	return expected
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

// TestResolveTieKeepsFirstCandidate pins the tie-break: two managed candidates
// whose version tuples are equal leave the winner to the candidate order, which
// Python's ordered `max` and the Go loop both resolve by keeping the first.
// "1.2.3" and "01.2.3" share the tuple (1,2,3) because int("01") == 1.
func TestResolveTieKeepsFirstCandidate(t *testing.T) {
	goos, goarch := requireTarget(t)
	oracle := loadFrozenOracle(t)
	rec := oracle.record(t, "resolve-tie/keeps-first", "resolve")
	var inputs struct {
		Binary      string               `json:"binary"`
		MinVersion  string               `json:"min_version"`
		PathVersion string               `json:"path_version"`
		Managed     []frozenManagedInput `json:"managed"`
	}
	if err := json.Unmarshal(rec.Inputs, &inputs); err != nil {
		t.Fatalf("decode frozen inputs: %v", err)
	}
	if inputs.Binary != "jinna" || inputs.MinVersion != "" || inputs.PathVersion != "" || len(inputs.Managed) != 2 {
		t.Fatalf("frozen resolve-tie inputs drifted: %+v", inputs)
	}

	home := t.TempDir()
	writeManaged(t, home, "1.2.3", "1.2.3", receiptOverrides{})
	writeManaged(t, home, "01.2.3", "1.2.3", receiptOverrides{})
	t.Setenv("PATH", t.TempDir())

	root := CacheRoot(&home)
	candidates := managedCandidates(root, goos, goarch)
	if len(candidates) != 2 {
		t.Fatalf("managed candidates = %q, want the two tied receipts", candidates)
	}
	if joinVersion(VersionTuple("1.2.3")) != joinVersion(VersionTuple("01.2.3")) {
		t.Fatalf("fixture is not a tie: 1.2.3 and 01.2.3 must share a tuple")
	}

	raw := strings.ReplaceAll(rec.Python, "<HOME>", home)
	var ref providerRefResult
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		t.Fatalf("decode frozen ref %s: %v", raw, err)
	}
	normalizeProviderRefHost(&ref, goos, goarch)
	got, err := Resolve(&schema.CliDep{Binary: "jinna"}, &home)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertResolutionMatchesRef(t, got, ref)
	if got.Command != candidates[0] {
		t.Fatalf("tie winner = %q, want the first candidate %q", got.Command, candidates[0])
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

// --- frozen Python oracle ---------------------------------------------------

// frozenCase is one captured provider_ref.py run: its mode, the exact inputs
// that produced it, and the raw stdout JSON.
type frozenCase struct {
	Mode   string          `json:"mode"`
	Inputs json.RawMessage `json:"inputs"`
	Python string          `json:"python"`
}

func (c frozenCase) inputText(t *testing.T) string {
	t.Helper()
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(c.Inputs, &in); err != nil {
		t.Fatalf("decode frozen inputs: %v", err)
	}
	return in.Text
}

type frozenOracle struct {
	Oracle struct {
		Python string `json:"python"`
	} `json:"oracle"`
	Cases map[string]frozenCase `json:"cases"`
}

// loadFrozenOracle reads the frozen authority bundle. A missing bundle is a
// failure, never a skip: the bundle IS the oracle.
func loadFrozenOracle(t *testing.T) *frozenOracle {
	t.Helper()
	path := filepath.Join("testdata", "provider_golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen oracle: %v", err)
	}
	var oracle frozenOracle
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", path, err)
	}
	if oracle.Oracle.Python == "" || len(oracle.Cases) == 0 {
		t.Fatalf("frozen oracle %s carries no oracle identity or no cases", path)
	}
	return &oracle
}

// record returns the frozen case, failing when it is absent or captured in a
// different mode.
func (o *frozenOracle) record(t *testing.T, name, mode string) frozenCase {
	t.Helper()
	rec, ok := o.Cases[name]
	if !ok {
		t.Fatalf("frozen oracle is missing case %q", name)
	}
	if rec.Mode != mode {
		t.Fatalf("frozen case %q has mode %q, want %q", name, rec.Mode, mode)
	}
	return rec
}

// replaceHostTokens maps the frozen host placeholders to the runtime target.
// Callers apply it only to the exact fields that carry the host.
func replaceHostTokens(s, goos, goarch string) string {
	s = strings.ReplaceAll(s, "<GOOS>-<GOARCH>", goos+"-"+goarch)
	s = strings.ReplaceAll(s, "<GOOS>", goos)
	s = strings.ReplaceAll(s, "<GOARCH>", goarch)
	return s
}

// normalizeProviderRefHost maps the host placeholders in the target and the
// host-carrying command/path fields, leaving every other field as written.
func normalizeProviderRefHost(ref *providerRefResult, goos, goarch string) {
	if len(ref.Target) == 2 {
		ref.Target[0] = replaceHostTokens(ref.Target[0], goos, goarch)
		ref.Target[1] = replaceHostTokens(ref.Target[1], goos, goarch)
	}
	ref.Command = replaceHostTokens(ref.Command, goos, goarch)
	if ref.Path != nil {
		path := replaceHostTokens(*ref.Path, goos, goarch)
		ref.Path = &path
	}
}

// TestProviderHostNormalizeLeavesOtherFieldsAlone is the negative control for
// the scoped tokenizer: a host-looking placeholder in a non-host field is
// compared as written and is not mapped to the runtime target.
func TestProviderHostNormalizeLeavesOtherFieldsAlone(t *testing.T) {
	path := "<TMP>/x/<GOOS>-<GOARCH>/jinna"
	ref := providerRefResult{
		Command:    path,
		Path:       &path,
		Repository: "<GOOS>-<GOARCH>",
		ReleaseTag: "<GOOS>",
		Target:     []string{"<GOOS>", "<GOARCH>"},
	}
	normalizeProviderRefHost(&ref, "linux", "amd64")
	if ref.Repository != "<GOOS>-<GOARCH>" {
		t.Fatalf("repository was normalized: %q", ref.Repository)
	}
	if ref.ReleaseTag != "<GOOS>" {
		t.Fatalf("release_tag was normalized: %q", ref.ReleaseTag)
	}
	if ref.Target[0] != "linux" || ref.Target[1] != "amd64" {
		t.Fatalf("target was not normalized: %q", ref.Target)
	}
	if ref.Command != "<TMP>/x/linux-amd64/jinna" {
		t.Fatalf("command was not normalized only on the target segment: %q", ref.Command)
	}
	if ref.Path == nil || *ref.Path != "<TMP>/x/linux-amd64/jinna" {
		t.Fatalf("path was not normalized only on the target segment: %v", ref.Path)
	}
}

type frozenManagedInput struct {
	VersionDir string            `json:"version_dir"`
	Printed    string            `json:"printed"`
	Overrides  map[string]string `json:"overrides"`
}

type frozenResolveInputs struct {
	Binary      string              `json:"binary"`
	MinVersion  string              `json:"min_version"`
	PathVersion string              `json:"path_version"`
	Managed     *frozenManagedInput `json:"managed"`
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

// TestProviderResolveDifferentialAgainstPython pins Resolve against the frozen
// provider_install.resolve_provider bundle on every shared fixture: the PATH
// source, the managed source with a verified receipt, each receipt rejection,
// the min_version gate, and the set-but-empty binary.
func TestProviderResolveDifferentialAgainstPython(t *testing.T) {
	goos, goarch := requireTarget(t)
	oracle := loadFrozenOracle(t)

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
			rec := oracle.record(t, "resolve-differential/"+tc.name, "resolve")
			var inputs frozenResolveInputs
			if err := json.Unmarshal(rec.Inputs, &inputs); err != nil {
				t.Fatalf("decode frozen inputs: %v", err)
			}
			wantManaged := tc.managed == nil
			if inputs.Binary != tc.binary || inputs.MinVersion != tc.minVersion || inputs.PathVersion != tc.pathVersion || (inputs.Managed == nil) != wantManaged {
				t.Fatalf("case input drift: frozen %+v, built %+v", inputs, tc)
			}
			if tc.managed != nil {
				if inputs.Managed.VersionDir != tc.managed.versionDir || inputs.Managed.Printed != tc.managed.printed {
					t.Fatalf("case managed input drift: frozen %+v, built %+v", inputs.Managed, tc.managed)
				}
				wantOverrides := receiptOverridesMap(tc.managed.ov)
				if !reflect.DeepEqual(inputs.Managed.Overrides, wantOverrides) {
					t.Fatalf("case managed overrides drift: frozen %+v, built %+v", inputs.Managed.Overrides, wantOverrides)
				}
			}

			home := t.TempDir()
			pathDir := t.TempDir()
			if tc.pathVersion != "" {
				writeExecutable(t, filepath.Join(pathDir, "jinna"), "#!/bin/sh\necho jinna "+tc.pathVersion+"\n")
			}
			t.Setenv("PATH", pathDir)
			if tc.managed != nil {
				writeManaged(t, home, tc.managed.versionDir, tc.managed.printed, tc.managed.ov)
			}

			raw := strings.ReplaceAll(rec.Python, "<PATH>", pathDir)
			raw = strings.ReplaceAll(raw, "<HOME>", home)
			var ref providerRefResult
			if err := json.Unmarshal([]byte(raw), &ref); err != nil {
				t.Fatalf("decode frozen ref %s: %v", raw, err)
			}
			normalizeProviderRefHost(&ref, goos, goarch)
			got, err := Resolve(&schema.CliDep{Binary: tc.binary, MinVersion: tc.minVersion}, &home)
			if ref.Error != "" {
				if err == nil || err.Error() != ref.Error {
					t.Fatalf("error diverges: go=%v frozen=%q", err, ref.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected go error: %v (frozen %+v)", err, ref)
			}
			assertResolutionMatchesRef(t, got, ref)
		})
	}
}

// receiptOverridesMap renders the overrides a case applies to the base receipt,
// matching the shape captured in provider_golden.json.
func receiptOverridesMap(ov receiptOverrides) map[string]string {
	out := map[string]string{}
	if ov.status != nil {
		out["status"] = *ov.status
	}
	if ov.repository != nil {
		out["repository"] = *ov.repository
	}
	if ov.target != nil {
		out["target"] = *ov.target
	}
	if ov.releaseTag != nil {
		out["release_tag"] = *ov.releaseTag
	}
	if ov.digest != nil {
		out["binary_sha256"] = *ov.digest
	}
	return out
}

// TestRunVersionDifferentialAgainstPython pins the error strings of the
// reproduced cases (missing path, permission, no version, non-zero exit,
// timeout) to the frozen provider_install._run_version bundle.
func TestRunVersionDifferentialAgainstPython(t *testing.T) {
	oracle := loadFrozenOracle(t)

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
			rec := oracle.record(t, "runversion-differential/"+tc.name, "runversion")
			var inputs struct {
				Fixture string `json:"fixture"`
				Timeout string `json:"timeout"`
			}
			if err := json.Unmarshal(rec.Inputs, &inputs); err != nil {
				t.Fatalf("decode frozen inputs: %v", err)
			}
			if inputs.Fixture != tc.name || inputs.Timeout != tc.timeout {
				t.Fatalf("case input drift: frozen %+v, built %+v", inputs, tc)
			}
			raw := strings.ReplaceAll(rec.Python, "<TMP>", dir)
			var ref providerRefVersion
			if err := json.Unmarshal([]byte(raw), &ref); err != nil {
				t.Fatalf("decode frozen ref %s: %v", raw, err)
			}
			timeout, err := time.ParseDuration(tc.timeout + "s")
			if err != nil {
				t.Fatal(err)
			}
			got, err := runVersion(tc.path, timeout)
			if ref.Error != "" {
				if err == nil || err.Error() != ref.Error {
					t.Fatalf("error diverges: go=%v frozen=%q", err, ref.Error)
				}
				return
			}
			if err != nil || got != ref.Version {
				t.Fatalf("version diverges: go=%q err=%v frozen=%q", got, err, ref.Version)
			}
		})
	}
}
