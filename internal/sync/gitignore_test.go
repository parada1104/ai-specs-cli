package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	cases := []gitignoreCase{
		renderCase("render manifest without deps", manifestNoDeps),
		renderCase("render manifest with two deps", manifestTwoDeps),
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
