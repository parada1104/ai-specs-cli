package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Literal copies of the two marker constants, kept in the test so the case
// inputs are an independent oracle rather than a read-back of the
// implementation under test.
const (
	testRootMarkerBegin = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
	testRootMarkerEnd   = "# --- end ai-specs ---"
)

// gitignoreGoldenRecord is one frozen case of the former live differential
// against lib/_internal/gitignore-{render,root-refresh}.py (captured once via
// a scratch capture script; PR2-B S3). Stderr is nil exactly for the three
// module-direct records: the Python authority raises an uncaught traceback
// whose text embeds host paths, and the Go contract differs by design (one
// line), so those records freeze rc, stdout and the preserved artifact bytes
// only. ArtifactMode is the permission bits the authority left on the file.
type gitignoreGoldenRecord struct {
	Name            string  `json:"name"`
	Mode            string  `json:"mode"`
	Stdout          string  `json:"stdout"`
	Stderr          *string `json:"stderr"`
	RC              int     `json:"rc"`
	Action          string  `json:"action"`
	ArtifactPresent bool    `json:"artifact_present"`
	ArtifactSHA256  *string `json:"artifact_sha256"`
	ArtifactB64     *string `json:"artifact_b64"`
	ArtifactMode    *uint32 `json:"artifact_mode"`
}

// gitignoreGoldenCaseNames is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order. It is an independent oracle
// of coverage — kept in the test, never derived from the bundle — so a bundle
// that is missing, renamed or reordered records cannot validate silently.
var gitignoreGoldenCaseNames = []string{
	"render manifest without deps",
	"render manifest with two deps",
	"render new file under umask 022",
	"render new file under umask 002",
	"render keeps the mode of an existing file",
	"root-refresh appended under umask 002",
	"root-refresh appended on missing .gitignore",
	"root-refresh appended without trailing newline",
	"root-refresh appended to file ending in single newline",
	"root-refresh replaces existing block",
	"root-refresh replaces block with content after end marker",
	"root-refresh missing template is rc 1",
	"unmatched begin marker",
	"root .gitignore unreadable",
	"template unreadable",
}

// gitignoreGoldenKindCounts pins how many records each capture kind
// contributes, before any case loop runs.
var gitignoreGoldenKindCounts = map[string]int{
	"render":                     5,
	"root-refresh":               7,
	"module-unmatched":           1,
	"module-unreadable-root":     1,
	"module-unreadable-template": 1,
}

// loadGitignoreGolden reads the frozen bundle and validates its identity
// inventory: exact names in order, no duplicates, per-kind counts.
func loadGitignoreGolden(t *testing.T) map[string]gitignoreGoldenRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "gitignore_golden.json"))
	if err != nil {
		t.Fatalf("read frozen oracle: %v", err)
	}
	var bundle struct {
		Cases []gitignoreGoldenRecord `json:"cases"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if len(bundle.Cases) != len(gitignoreGoldenCaseNames) {
		t.Fatalf("frozen bundle has %d records, want %d", len(bundle.Cases), len(gitignoreGoldenCaseNames))
	}
	byName := make(map[string]gitignoreGoldenRecord, len(bundle.Cases))
	seen := make(map[string]bool, len(bundle.Cases))
	kindCounts := make(map[string]int)
	for i, rec := range bundle.Cases {
		if want := gitignoreGoldenCaseNames[i]; rec.Name != want {
			t.Fatalf("frozen record %d is %q, want %q (identity mismatch)", i, rec.Name, want)
		}
		if seen[rec.Name] {
			t.Fatalf("duplicate frozen record %q", rec.Name)
		}
		seen[rec.Name] = true
		kindCounts[rec.Mode]++
		byName[rec.Name] = rec
	}
	for kind, want := range gitignoreGoldenKindCounts {
		if got := kindCounts[kind]; got != want {
			t.Fatalf("frozen kind %q has %d records, want %d", kind, got, want)
		}
	}
	return byName
}

// goldenCase looks up one frozen record by its identity name.
func goldenCase(t *testing.T, golden map[string]gitignoreGoldenRecord, name string) gitignoreGoldenRecord {
	t.Helper()
	rec, ok := golden[name]
	if !ok {
		t.Fatalf("frozen record %q missing", name)
	}
	return rec
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// gitignoreRepoRoot returns the repository root (go test cwd is internal/sync).
func gitignoreRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s does not contain go.mod: %v", root, err)
	}
	return root
}

// writeGitignoreInput writes a fixture file, creating its parent directories.
func writeGitignoreInput(t *testing.T, path, content string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// gitignoreCase is one frozen differential scenario. prepare populates the
// leg's isolated dir and returns the two positional arguments passed to the Go
// port. artifact returns the path of the file whose bytes must match ("" =
// none). goRun invokes the Go port.
type gitignoreCase struct {
	name     string
	mode     string // "render" | "root-refresh"
	prepare  func(t *testing.T, dir string) (arg1, arg2 string)
	artifact func(dir string) string
	goRun    func(arg1, arg2 string, stdout, stderr io.Writer) int
	// umask, when set, is applied to the whole subtest so the Go leg inherits
	// the same process umask the frozen authority was captured with; it is the
	// only way to observe the write mode rule (0666 &^ umask for a new file).
	umask *int
	// wantMode, when set, is the exact permission the port must leave on the
	// artifact; it is an independent absolute pin kept next to the frozen
	// authority's own mode.
	wantMode *os.FileMode
}

// TestGitignoreDifferential runs each case through the Go port in an isolated
// temp dir and requires equality against the frozen authority record: exit
// code, tokenized stdout/stderr, and byte equality of the emitted file
// (PR2-B S3: the live python3 leg is replaced by testdata/gitignore_golden.json).
func TestGitignoreDifferential(t *testing.T) {
	golden := loadGitignoreGolden(t)
	root := gitignoreRepoRoot(t)
	repoTemplate := filepath.Join(root, "templates", "gitignore-root.tmpl")

	manifestNoDeps := "[project]\nname = \"x\"\n"
	manifestTwoDeps := "[project]\nname = \"x\"\n\n[[deps]]\nid = \"a\"\nsource = \"../a\"\n\n[[deps]]\nid = \"b\"\nsource = \"../b\"\n"

	renderOut := func(dir string) string { return filepath.Join(dir, "ai-specs", ".gitignore") }

	renderCase := func(name, manifest string) gitignoreCase {
		return gitignoreCase{
			name: name,
			mode: "render",
			prepare: func(t *testing.T, dir string) (string, string) {
				tomlPath := filepath.Join(dir, "ai-specs", "ai-specs.toml")
				writeGitignoreInput(t, tomlPath, manifest)
				return tomlPath, renderOut(dir)
			},
			artifact: renderOut,
			goRun: func(arg1, arg2 string, stdout, _ io.Writer) int {
				return RenderAiSpecsGitignore(arg1, arg2, stdout)
			},
		}
	}

	// Permission fixtures. umask 002 makes 0666 &^ umask (0664) differ from the
	// old fixed 0644, so the two write sites are observable; umask 022 pins the
	// ordinary case to 0644.
	var mask022, mask002 = 0o022, 0o002
	mode0644, mode0664, mode0600 := os.FileMode(0o644), os.FileMode(0o664), os.FileMode(0o600)

	renderWithMode := func(name string, mask *int, want os.FileMode) gitignoreCase {
		c := renderCase(name, manifestNoDeps)
		c.umask, c.wantMode = mask, &want
		return c
	}

	renderExistingMode := renderCase("render keeps the mode of an existing file", manifestNoDeps)
	renderExistingMode.prepare = func(t *testing.T, dir string) (string, string) {
		tomlPath := filepath.Join(dir, "ai-specs", "ai-specs.toml")
		writeGitignoreInput(t, tomlPath, manifestNoDeps)
		out := renderOut(dir)
		writeGitignoreInput(t, out, "stale content\n")
		if err := os.Chmod(out, mode0600); err != nil {
			t.Fatalf("chmod %s: %v", out, err)
		}
		return tomlPath, out
	}
	renderExistingMode.wantMode = &mode0600

	rootAppendUnderUmask := gitignoreCase{
		name:     "root-refresh appended under umask 002",
		mode:     "root-refresh",
		umask:    &mask002,
		wantMode: &mode0664,
		prepare: func(t *testing.T, dir string) (string, string) {
			return dir, repoTemplate
		},
		artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
		goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
			return RefreshRootGitignore(arg1, arg2, stdout, stderr)
		},
	}

	cases := []gitignoreCase{
		renderCase("render manifest without deps", manifestNoDeps),
		renderCase("render manifest with two deps", manifestTwoDeps),
		renderWithMode("render new file under umask 022", &mask022, mode0644),
		renderWithMode("render new file under umask 002", &mask002, mode0664),
		renderExistingMode,
		rootAppendUnderUmask,
		{
			name: "root-refresh appended on missing .gitignore",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				return dir, repoTemplate
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
		{
			name: "root-refresh appended without trailing newline",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				writeGitignoreInput(t, filepath.Join(dir, ".gitignore"), "keep-me")
				return dir, repoTemplate
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
		{
			name: "root-refresh appended to file ending in single newline",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				writeGitignoreInput(t, filepath.Join(dir, ".gitignore"), "keep-me\n")
				return dir, repoTemplate
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
		{
			name: "root-refresh replaces existing block",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				existing := "keep-before\n\n" + testRootMarkerBegin + "\nold agent line\n" + testRootMarkerEnd + "\n"
				writeGitignoreInput(t, filepath.Join(dir, ".gitignore"), existing)
				return dir, repoTemplate
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
		{
			name: "root-refresh replaces block with content after end marker",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				existing := testRootMarkerBegin + "\nold agent line\n" + testRootMarkerEnd + "\nsurvivor-line\n"
				writeGitignoreInput(t, filepath.Join(dir, ".gitignore"), existing)
				return dir, repoTemplate
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
		{
			name: "root-refresh missing template is rc 1",
			mode: "root-refresh",
			prepare: func(t *testing.T, dir string) (string, string) {
				return dir, filepath.Join(dir, "missing.tmpl")
			},
			artifact: func(dir string) string { return filepath.Join(dir, ".gitignore") },
			goRun: func(arg1, arg2 string, stdout, stderr io.Writer) int {
				return RefreshRootGitignore(arg1, arg2, stdout, stderr)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := goldenCase(t, golden, tc.name)
			if tc.umask != nil {
				previous := syscall.Umask(*tc.umask)
				defer syscall.Umask(previous)
			}
			goDir := t.TempDir()

			goArg1, goArg2 := tc.prepare(t, goDir)

			var goOut, goErr bytes.Buffer
			goRC := tc.goRun(goArg1, goArg2, &goOut, &goErr)

			if goRC != rec.RC {
				t.Errorf("rc: go=%d frozen=%d", goRC, rec.RC)
			}
			norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "<DIR>") }
			if got, want := norm(goOut.String(), goDir), rec.Stdout; got != want {
				t.Errorf("stdout:\n  go:     %q\n  frozen: %q", got, want)
			}
			if rec.Stderr == nil {
				t.Fatalf("frozen record %q dropped its stderr; differential records freeze it", tc.name)
			}
			if got, want := norm(goErr.String(), goDir), *rec.Stderr; got != want {
				t.Errorf("stderr:\n  go:     %q\n  frozen: %q", got, want)
			}

			var goBytes []byte
			goPresent := false
			if path := tc.artifact(goDir); path != "" {
				b, err := os.ReadFile(path)
				if err == nil {
					goBytes, goPresent = b, true
				} else if !os.IsNotExist(err) {
					t.Fatalf("read go artifact %s: %v", path, err)
				}
			}
			if goPresent != rec.ArtifactPresent {
				t.Fatalf("artifact presence: go=%v frozen=%v", goPresent, rec.ArtifactPresent)
			}
			if goPresent {
				if rec.ArtifactMode == nil {
					t.Fatalf("frozen record %q has an artifact but no mode", tc.name)
				}
				goMode := artifactMode(t, tc.artifact(goDir))
				if got, want := uint32(goMode), *rec.ArtifactMode; got != want {
					t.Errorf("artifact mode: go=%04o frozen=%04o", got, want)
				}
				if tc.wantMode != nil && goMode != *tc.wantMode {
					t.Errorf("artifact mode: go=%04o want=%04o", goMode, *tc.wantMode)
				}
				if got := sha256Hex(goBytes); rec.ArtifactSHA256 == nil || got != *rec.ArtifactSHA256 {
					t.Errorf("artifact sha256: go=%s frozen=%v", got, rec.ArtifactSHA256)
				}
				if rec.ArtifactB64 == nil {
					t.Fatalf("frozen record %q has an artifact but no bytes", tc.name)
				}
				want, err := base64.StdEncoding.DecodeString(*rec.ArtifactB64)
				if err != nil {
					t.Fatalf("decode frozen artifact: %v", err)
				}
				if !bytes.Equal(goBytes, want) {
					t.Errorf("artifact bytes:\n  go:     %q\n  frozen: %q", goBytes, want)
				}
			}
		})
	}
}

// artifactMode returns the permission bits of an existing artifact.
func artifactMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Mode().Perm()
}

// makeUnreadable makes path impossible to read for this process and returns the
// target to pass to the port under test plus whether it is still a regular file
// whose bytes must be preserved. chmod 000 is used when it actually denies
// reads (any non-root process); root bypasses file permissions, so the file is
// replaced with a directory, which os.ReadFile rejects at any privilege level.
func makeUnreadable(t *testing.T, path string) (target string, regular bool) {
	t.Helper()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 000 %s: %v", path, err)
	}
	if _, err := os.ReadFile(path); err != nil {
		return path, true
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("restore %s: %v", path, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	return path, false
}

// readFileAfterUnreadable restores read permissions before reading, so the
// caller can compare bytes that the operation under test must not have touched.
func readFileAfterUnreadable(t *testing.T, path string) []byte {
	t.Helper()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("restore perms on %s: %v", path, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s after restore: %v", path, err)
	}
	return b
}

// assertUnchanged verifies the read target was not rewritten. A regular file is
// compared byte-for-byte against the frozen authority bytes; a directory
// fixture must simply remain a directory.
func assertUnchanged(t *testing.T, label string, target string, regular bool, want []byte) {
	t.Helper()
	if regular {
		if got := readFileAfterUnreadable(t, target); !bytes.Equal(got, want) {
			t.Errorf("%s changed unreadable file: got %q want %q", label, got, want)
		}
		return
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Errorf("%s changed directory fixture %s (err=%v)", label, target, err)
	}
}

// TestGitignoreDifferentialUnmatchedBeginMarker covers D2: a begin marker with
// no end marker makes the port fail exactly as the frozen authority did — a
// non-zero exit, empty stdout and an untouched file (frozen bytes). The error
// text differs by contract (the Python authority raised a traceback, the port
// prints one line), so stderr is only required to be non-empty.
func TestGitignoreDifferentialUnmatchedBeginMarker(t *testing.T) {
	rec := goldenCase(t, loadGitignoreGolden(t), "unmatched begin marker")
	if rec.RC == 0 {
		t.Fatalf("frozen authority rc = 0; the unmatched-begin-marker authority must fail")
	}
	root := gitignoreRepoRoot(t)
	repoTemplate := filepath.Join(root, "templates", "gitignore-root.tmpl")
	existing := "keep\n\n" + testRootMarkerBegin + "\nold agent line\n"

	goDir := t.TempDir()
	goGitignore := filepath.Join(goDir, ".gitignore")
	writeGitignoreInput(t, goGitignore, existing)

	var goOut, goErr bytes.Buffer
	goRC := RefreshRootGitignore(goDir, repoTemplate, &goOut, &goErr)

	if goRC != rec.RC {
		t.Errorf("exit codes: go=%d frozen=%d", goRC, rec.RC)
	}
	if goOut.String() != rec.Stdout {
		t.Errorf("stdout: go=%q frozen=%q", goOut.String(), rec.Stdout)
	}
	if goErr.Len() == 0 {
		t.Errorf("go printed no stderr diagnostic")
	}
	if rec.ArtifactB64 == nil {
		t.Fatalf("frozen record has no preserved artifact bytes")
	}
	want, err := base64.StdEncoding.DecodeString(*rec.ArtifactB64)
	if err != nil {
		t.Fatalf("decode frozen artifact: %v", err)
	}
	if got, err := os.ReadFile(goGitignore); err != nil || !bytes.Equal(got, want) {
		t.Errorf("go changed the file: got %q err=%v want %q", got, err, want)
	}
}

// TestReadTextUniversalPropagatesReadError pins the core fix of
// R4-SilentReadSwallowDataLoss directly: a read failure must be reported, not
// converted into an empty string. A directory is used because os.ReadFile
// rejects it regardless of process privileges.
func TestReadTextUniversalPropagatesReadError(t *testing.T) {
	dir := t.TempDir()
	if got, err := readTextUniversal(dir); err == nil {
		t.Fatalf("readTextUniversal(directory) = %q, nil; want error", got)
	}
}

// TestGitignoreDifferentialReadFailures covers R4-SilentReadSwallowDataLoss: a
// failed read of the root .gitignore or of the template must surface as a
// non-zero rc with no write, exactly as the frozen authority recorded (rc 1,
// preserved bytes). The error text differs by contract (the Python authority
// raised an uncaught traceback, the port prints one line); the frozen
// properties are the rc and the preserved bytes.
func TestGitignoreDifferentialReadFailures(t *testing.T) {
	golden := loadGitignoreGolden(t)
	root := gitignoreRepoRoot(t)
	repoTemplate := filepath.Join(root, "templates", "gitignore-root.tmpl")

	t.Run("root .gitignore unreadable", func(t *testing.T) {
		rec := goldenCase(t, golden, "root .gitignore unreadable")
		if rec.RC == 0 {
			t.Fatalf("frozen authority rc = 0; the read-failure authority must fail")
		}
		goDir := t.TempDir()
		goGitignore := filepath.Join(goDir, ".gitignore")
		writeGitignoreInput(t, goGitignore, "keep-existing-root-content\n")
		goTarget, goRegular := makeUnreadable(t, goGitignore)

		var goOut, goErr bytes.Buffer
		goRC := RefreshRootGitignore(goDir, repoTemplate, &goOut, &goErr)
		if goRC != rec.RC {
			t.Errorf("rc: go=%d frozen=%d", goRC, rec.RC)
		}
		if goErr.Len() == 0 {
			t.Errorf("go printed no stderr diagnostic")
		}
		if rec.ArtifactB64 == nil {
			t.Fatalf("frozen record has no preserved artifact bytes")
		}
		want, err := base64.StdEncoding.DecodeString(*rec.ArtifactB64)
		if err != nil {
			t.Fatalf("decode frozen artifact: %v", err)
		}
		assertUnchanged(t, "go", goTarget, goRegular, want)
	})

	t.Run("template unreadable", func(t *testing.T) {
		rec := goldenCase(t, golden, "template unreadable")
		if rec.RC == 0 {
			t.Fatalf("frozen authority rc = 0; the read-failure authority must fail")
		}
		goDir := t.TempDir()
		goGitignore := filepath.Join(goDir, ".gitignore")
		writeGitignoreInput(t, goGitignore, "keep-existing-root-content\n")
		goTemplate := filepath.Join(goDir, "template.tmpl")
		writeGitignoreInput(t, goTemplate, "# template\n")
		goTemplate, _ = makeUnreadable(t, goTemplate)

		var goOut, goErr bytes.Buffer
		goRC := RefreshRootGitignore(goDir, goTemplate, &goOut, &goErr)
		if goRC != rec.RC {
			t.Errorf("rc: go=%d frozen=%d", goRC, rec.RC)
		}
		if goErr.Len() == 0 {
			t.Errorf("go printed no stderr diagnostic")
		}
		if rec.ArtifactB64 == nil {
			t.Fatalf("frozen record has no preserved artifact bytes")
		}
		want, err := base64.StdEncoding.DecodeString(*rec.ArtifactB64)
		if err != nil {
			t.Fatalf("decode frozen artifact: %v", err)
		}
		if got, err := os.ReadFile(goGitignore); err != nil || !bytes.Equal(got, want) {
			t.Errorf("go changed root .gitignore: got %q err=%v want %q", got, err, want)
		}
	})
}
