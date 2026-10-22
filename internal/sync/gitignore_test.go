package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
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

// gitignoreRefResult is the canonical JSON emitted by testdata/gitignore_ref.py.
// The render mode fills the FileBytes* fields; root-refresh fills the
// ResultingGitignore* fields. Pointers keep "absent file" distinct from an
// empty file.
type gitignoreRefResult struct {
	Stdout                   string  `json:"stdout"`
	Stderr                   string  `json:"stderr"`
	RC                       int     `json:"rc"`
	Action                   string  `json:"action"`
	FileBytesSHA256          *string `json:"file_bytes_sha256"`
	FileBytesB64             *string `json:"file_bytes_b64"`
	ResultingGitignoreSHA256 *string `json:"resulting_gitignore_sha256"`
	ResultingGitignoreB64    *string `json:"resulting_gitignore_b64"`
}

// artifact returns the (sha256, base64, present) triple for whichever output
// file the mode produced.
func (r gitignoreRefResult) artifact() (sha, b64 string, present bool) {
	if r.FileBytesB64 != nil {
		return derefString(r.FileBytesSHA256), *r.FileBytesB64, true
	}
	if r.ResultingGitignoreB64 != nil {
		return derefString(r.ResultingGitignoreSHA256), *r.ResultingGitignoreB64, true
	}
	return "", "", false
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

// runGitignoreRef runs the reference driver and returns its JSON stdout.
func runGitignoreRef(t *testing.T, root, script, mode, arg1, arg2 string) gitignoreRefResult {
	t.Helper()
	cmd := exec.Command("python3", script, mode, arg1, arg2)
	cmd.Dir = root
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("ref driver (%s) failed: %v\nstderr: %s", mode, err, errBuf.String())
	}
	var ref gitignoreRefResult
	if err := json.Unmarshal([]byte(out.String()), &ref); err != nil {
		t.Fatalf("parse ref JSON: %v\nraw: %s", err, out.String())
	}
	return ref
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

// gitignoreCase is one differential scenario. prepare populates the leg's
// isolated dir and returns the two positional arguments passed identically to
// the ref driver and the Go port. artifact returns the path of the file whose
// bytes must match ("" = none). goRun invokes the Go port.
type gitignoreCase struct {
	name     string
	mode     string // "render" | "root-refresh"
	prepare  func(t *testing.T, dir string) (arg1, arg2 string)
	artifact func(dir string) string
	goRun    func(arg1, arg2 string, stdout, stderr io.Writer) int
	// umask, when set, is applied to the whole subtest so both legs inherit the
	// same process umask; it is the only way to observe the Python
	// write_text() mode rule (0666 &^ umask for a new file).
	umask *int
	// wantMode, when set, is the exact permission both legs must leave on the
	// artifact.
	wantMode *os.FileMode
}

// TestGitignoreDifferential runs each case through both the Go port and the
// real Python module (via testdata/gitignore_ref.py, which execs the module in
// a subprocess) in isolated temp dirs, and requires byte equality of the
// emitted file bytes, stdout, stderr and exit code.
func TestGitignoreDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	refScript := filepath.Join(root, "internal", "sync", "testdata", "gitignore_ref.py")
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
			if tc.umask != nil {
				previous := syscall.Umask(*tc.umask)
				defer syscall.Umask(previous)
			}
			goDir, refDir := t.TempDir(), t.TempDir()

			goArg1, goArg2 := tc.prepare(t, goDir)
			refArg1, refArg2 := tc.prepare(t, refDir)

			var goOut, goErr bytes.Buffer
			goRC := tc.goRun(goArg1, goArg2, &goOut, &goErr)

			ref := runGitignoreRef(t, root, refScript, tc.mode, refArg1, refArg2)

			if goRC != ref.RC {
				t.Errorf("rc: go=%d ref=%d", goRC, ref.RC)
			}
			norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "<DIR>") }
			if got, want := norm(goOut.String(), goDir), norm(ref.Stdout, refDir); got != want {
				t.Errorf("stdout:\n  go:  %q\n  ref: %q", got, want)
			}
			if got, want := norm(goErr.String(), goDir), norm(ref.Stderr, refDir); got != want {
				t.Errorf("stderr:\n  go:  %q\n  ref: %q", got, want)
			}

			refSHA, refB64, refPresent := ref.artifact()

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
			if goPresent != refPresent {
				t.Fatalf("artifact presence: go=%v ref=%v", goPresent, refPresent)
			}
			if goPresent && tc.wantMode != nil {
				goMode := artifactMode(t, tc.artifact(goDir))
				refMode := artifactMode(t, tc.artifact(refDir))
				if goMode != refMode {
					t.Errorf("artifact mode: go=%04o ref=%04o", goMode, refMode)
				}
				if goMode != *tc.wantMode {
					t.Errorf("artifact mode: go=%04o want=%04o", goMode, *tc.wantMode)
				}
			}
			if goPresent {
				if got := sha256Hex(goBytes); got != refSHA {
					t.Errorf("artifact sha256: go=%s ref=%s", got, refSHA)
				}
				want, err := base64.StdEncoding.DecodeString(refB64)
				if err != nil {
					t.Fatalf("decode ref artifact: %v", err)
				}
				if !bytes.Equal(goBytes, want) {
					t.Errorf("artifact bytes:\n  go:  %q\n  ref: %q", goBytes, want)
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
// compared byte-for-byte; a directory fixture must simply remain a directory.
func assertUnchanged(t *testing.T, label string, target string, regular bool, want string) {
	t.Helper()
	if regular {
		if got := readFileAfterUnreadable(t, target); string(got) != want {
			t.Errorf("%s changed unreadable file: got %q want %q", label, got, want)
		}
		return
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Errorf("%s changed directory fixture %s (err=%v)", label, target, err)
	}
}

// runGitignoreModule execs one real lib/_internal module directly and returns
// its exit code and captured streams. Unlike runGitignoreRef it tolerates a
// non-zero exit, which is exactly what the read-failure cases expect.
func runGitignoreModule(t *testing.T, root, module string, args ...string) (int, string, string) {
	t.Helper()
	path := filepath.Join(root, "lib", "_internal", module)
	cmd := exec.Command("python3", append([]string{path}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	rc := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %s: %v", module, err)
		}
		rc = exitErr.ExitCode()
	}
	return rc, out.String(), errBuf.String()
}

// TestGitignoreDifferentialUnmatchedBeginMarker covers D2: a begin marker with
// no end marker makes both legs fail, and it freezes only the frozen
// properties — a non-zero exit, empty stdout and an untouched file. The text
// differs by contract (the Python authority raises a traceback, the port
// prints one line), so stderr is not compared.
func TestGitignoreDifferentialUnmatchedBeginMarker(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	repoTemplate := filepath.Join(root, "templates", "gitignore-root.tmpl")
	existing := "keep\n\n" + testRootMarkerBegin + "\nold agent line\n"

	goDir, refDir := t.TempDir(), t.TempDir()
	goGitignore := filepath.Join(goDir, ".gitignore")
	writeGitignoreInput(t, goGitignore, existing)

	var goOut, goErr bytes.Buffer
	goRC := RefreshRootGitignore(goDir, repoTemplate, &goOut, &goErr)

	refGitignore := filepath.Join(refDir, ".gitignore")
	writeGitignoreInput(t, refGitignore, existing)
	refRC, refOut, _ := runGitignoreModule(t, root, "gitignore-root-refresh.py", refDir, repoTemplate)

	if goRC == 0 || refRC == 0 {
		t.Fatalf("exit codes: go=%d ref=%d, want both non-zero", goRC, refRC)
	}
	if goRC != refRC {
		t.Errorf("exit codes: go=%d ref=%d", goRC, refRC)
	}
	if goOut.String() != refOut {
		t.Errorf("stdout: go=%q ref=%q", goOut.String(), refOut)
	}
	if goErr.Len() == 0 {
		t.Errorf("go printed no stderr diagnostic")
	}
	if got, err := os.ReadFile(goGitignore); err != nil || string(got) != existing {
		t.Errorf("go changed the file: got %q err=%v want %q", got, err, existing)
	}
	if got, err := os.ReadFile(refGitignore); err != nil || string(got) != existing {
		t.Errorf("ref changed the file: got %q err=%v want %q", got, err, existing)
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
// failed read of the root .gitignore or of the template must surface as rc 1
// with no write, matching the Python authority, which raises. Unlike
// TestGitignoreDifferential, stderr is not compared byte-for-byte because the
// Python leg emits an uncaught traceback while the Go port prints one line; the
// frozen properties are a non-zero rc and preserved bytes.
func TestGitignoreDifferentialReadFailures(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)
	repoTemplate := filepath.Join(root, "templates", "gitignore-root.tmpl")

	const existing = "keep-existing-root-content\n"

	t.Run("root .gitignore unreadable", func(t *testing.T) {
		goDir, refDir := t.TempDir(), t.TempDir()
		goGitignore := filepath.Join(goDir, ".gitignore")
		writeGitignoreInput(t, goGitignore, existing)
		goTarget, goRegular := makeUnreadable(t, goGitignore)

		var goOut, goErr bytes.Buffer
		goRC := RefreshRootGitignore(goDir, repoTemplate, &goOut, &goErr)
		if goRC == 0 {
			t.Errorf("go rc = 0, want non-zero (stdout=%q)", goOut.String())
		}
		if goErr.Len() == 0 {
			t.Errorf("go printed no stderr diagnostic")
		}
		assertUnchanged(t, "go", goTarget, goRegular, existing)

		refGitignore := filepath.Join(refDir, ".gitignore")
		writeGitignoreInput(t, refGitignore, existing)
		refTarget, refRegular := makeUnreadable(t, refGitignore)

		refRC, _, _ := runGitignoreModule(t, root, "gitignore-root-refresh.py", refDir, repoTemplate)
		if refRC == 0 {
			t.Errorf("ref rc = 0, want non-zero")
		}
		assertUnchanged(t, "ref", refTarget, refRegular, existing)
	})

	t.Run("template unreadable", func(t *testing.T) {
		goDir, refDir := t.TempDir(), t.TempDir()
		goGitignore := filepath.Join(goDir, ".gitignore")
		writeGitignoreInput(t, goGitignore, existing)
		goTemplate := filepath.Join(goDir, "template.tmpl")
		writeGitignoreInput(t, goTemplate, "# template\n")
		goTemplate, _ = makeUnreadable(t, goTemplate)

		var goOut, goErr bytes.Buffer
		goRC := RefreshRootGitignore(goDir, goTemplate, &goOut, &goErr)
		if goRC == 0 {
			t.Errorf("go rc = 0, want non-zero (stdout=%q)", goOut.String())
		}
		if goErr.Len() == 0 {
			t.Errorf("go printed no stderr diagnostic")
		}
		if got, err := os.ReadFile(goGitignore); err != nil || string(got) != existing {
			t.Errorf("go changed root .gitignore: got %q err=%v want %q", got, err, existing)
		}

		refGitignore := filepath.Join(refDir, ".gitignore")
		writeGitignoreInput(t, refGitignore, existing)
		refTemplate := filepath.Join(refDir, "template.tmpl")
		writeGitignoreInput(t, refTemplate, "# template\n")
		refTemplate, _ = makeUnreadable(t, refTemplate)

		refRC, _, _ := runGitignoreModule(t, root, "gitignore-root-refresh.py", refDir, refTemplate)
		if refRC == 0 {
			t.Errorf("ref rc = 0, want non-zero")
		}
		if got, err := os.ReadFile(refGitignore); err != nil || string(got) != existing {
			t.Errorf("ref changed root .gitignore: got %q err=%v want %q", got, err, existing)
		}
	})
}
