package cli

// Differential smoke test: the built Go binary vs the frozen output of the
// legacy `bash bin/ai-specs` launcher (testdata/smoke_legacy.golden.json). The
// oracle was captured before the flip (GO-20 U2e T2) from the pre-flip
// launcher over the same lib/ tree, with AI_SPECS_HOME pinned to the checkout
// and PYTHONDONTWRITEBYTECODE=1 per parity contract §6. After the flip the
// live launcher execs this binary, so a live run would compare Go with itself.
// Every case asserts byte-identical stdout, stderr and exit code after the
// scratch root (<ROOT>) and the case directory (<DIR>) are normalized.
//
// Only usage surfaces and verified side-effect-free invocations are
// exercised; anything that could write (init, refresh-bundled, recipe
// add/configure, skills add/remove, upgrade --dry-run, hub on an initialized
// project) is deliberately excluded.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	smokeOnce    sync.Once
	smokeBin     string
	smokeRoot    string
	smokeLocErr  error
	smokeBuildEr error
)

// locateRoot walks up from this source file to the directory containing both
// go.mod and bin/ai-specs (defensive for odd CI layouts).
func locateRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	dir, err := filepath.Abs(filepath.Dir(thisFile))
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			if fi2, err := os.Stat(filepath.Join(dir, "bin", "ai-specs")); err == nil && !fi2.IsDir() {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// smokeSetup builds cmd/ai-specs once per test run.
func smokeSetup(t *testing.T) (bin, root string) {
	t.Helper()
	smokeOnce.Do(func() {
		smokeRoot, smokeLocErr = locateRoot()
		if smokeLocErr != nil {
			return
		}
		dir, err := os.MkdirTemp("", "ai-specs-smoke-*")
		if err != nil {
			smokeBuildEr = err
			return
		}
		smokeBin = filepath.Join(dir, "ai-specs")
		build := exec.Command("go", "build", "-o", smokeBin, "./cmd/ai-specs")
		build.Dir = smokeRoot
		out, err := build.CombinedOutput()
		if err != nil {
			smokeBuildEr = err
			println(string(out))
		}
	})
	if smokeLocErr != nil {
		t.Skipf("repo root (go.mod + bin/ai-specs) not located: %v", smokeLocErr)
	}
	if smokeBuildEr != nil {
		t.Fatalf("go build ./cmd/ai-specs failed: %v", smokeBuildEr)
	}
	return smokeBin, smokeRoot
}

// smokeEnv pins AI_SPECS_HOME to the repo root (so the Go binary's home
// resolution matches the legacy tree) and suppresses __pycache__ writes.
func smokeEnv(root string) []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "AI_SPECS_HOME="),
			strings.HasPrefix(kv, "PYTHONDONTWRITEBYTECODE="):
		default:
			env = append(env, kv)
		}
	}
	return append(env,
		"AI_SPECS_HOME="+root,
		"PYTHONDONTWRITEBYTECODE=1",
	)
}

// runOne executes bin with args under env/dir and returns captured output
// and the exit code.
func runOne(bin string, args []string, env []string, dir string) (stdout, stderr []byte, code int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), exitCodeOf(err)
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1 // exec failure: must never happen for these cases
}

// legacyRecord is one frozen run of the pre-flip legacy launcher.
type legacyRecord struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// loadLegacySmokeGolden reads the frozen oracle. A missing or stale record fails
// the test: the oracle is never recomputed from the live tree.
func loadLegacySmokeGolden(t *testing.T) map[string]legacyRecord {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "smoke_legacy.golden.json"))
	if err != nil {
		t.Fatalf("read frozen legacy oracle: %v", err)
	}
	var recs map[string]legacyRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatalf("parse frozen legacy oracle: %v", err)
	}
	return recs
}

// pathForms returns the resolved form of p before the raw one. Resolved first:
// the raw path is a substring of its resolved form (/var vs /private/var).
func pathForms(p string) []string {
	if r, err := filepath.EvalSymlinks(p); err == nil && r != p {
		return []string{r, p}
	}
	return []string{p}
}

// smokeNormalizer maps one run's scratch paths to the placeholders of the frozen
// oracle: the repository root to <ROOT>, the case directory to <DIR>.
func smokeNormalizer(root, dir string) *strings.Replacer {
	var pairs []string
	for _, f := range pathForms(root) {
		pairs = append(pairs, f, "<ROOT>")
	}
	if dir != root {
		for _, f := range pathForms(dir) {
			pairs = append(pairs, f, "<DIR>")
		}
	}
	return strings.NewReplacer(pairs...)
}

// TestDifferentialSmoke compares the Go binary with the frozen legacy launcher.
func TestDifferentialSmoke(t *testing.T) {
	bin, root := smokeSetup(t)
	env := smokeEnv(root)
	frozen := loadLegacySmokeGolden(t)

	cases := []struct {
		name   string
		args   []string
		dir    string // empty: per-case temp cwd
		native bool   // compare against the embedded help bytes, not the launcher
	}{
		// help/-h: bin/ai-specs's heredoc is UNQUOTED (`cat <<EOF`), so the
		// `ai-specs` inside backticks in the hub description is
		// command-substituted at runtime — the legacy launcher's help output
		// depends on PATH and is never byte-identical to the heredoc source.
		// The contract mandates the source bytes (internal/cli/help.txt), so
		// these two cases pin the Go output to the embed instead of the
		// legacy launcher.
		{"help", []string{"help"}, "", true},
		{"help-alias", []string{"-h"}, "", true},
		{"version", []string{"version"}, "", false},
		{"recipe-usage", []string{"recipe"}, "", false},
		{"skills-usage", []string{"skills"}, "", false},
		{"add-dep-noargs", []string{"add-dep"}, "", false},
		// Bare invocation must reach hub.sh with the original (empty) argv —
		// legacy no-shift semantics — not panic on the verb slice shift.
		{"hub-bare-nontty", []string{}, "", false},
		{"hub-uninitialized-nontty", []string{"hub"}, "", false},
		{"rules-audit-missing-path", []string{"rules-audit", "does-not-exist"}, "", false},
		{"sync-missing-path", []string{"sync", "does-not-exist"}, "", false},
		// sync-agent usage surfaces: the heredoc, the exit-2 contract, and the
		// pre-write cd failure (no writes happen before the diagnostic).
		{"sync-agent-help", []string{"sync-agent", "--help"}, "", false},
		{"sync-agent-unknown-flag", []string{"sync-agent", "--bogus"}, "", false},
		{"sync-agent-second-positional", []string{"sync-agent", "a", "b"}, "", false},
		{"sync-agent-missing-path", []string{"sync-agent", "does-not-exist"}, "", false},
		// Trailing value-taking flags die rc 1 with NO output and no writes
		// (JD-A-002, the D20 defect class): `shift 2` fails under set -e
		// before the flag loop ends.
		{"sync-agent-flag-last-target", []string{"sync-agent", "--target"}, "", false},
		{"sync-agent-flag-last-source-root", []string{"sync-agent", "--source-root"}, "", false},
		// The repo root is an initialized ai-specs project; without a TTY
		// config_wizard.py exits 3 right after the read-only pre-checks.
		{"configure-recipes-nontty", []string{"configure-recipes"}, root, false},
		// Argument validation fires before any git/network operation in
		// lib/upgrade.sh (the parse loop is the first executable block).
		{"upgrade-unknown-arg", []string{"upgrade", "--bogus"}, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.dir
			if dir == "" {
				dir = t.TempDir() // one shared cwd per case for both runs
			}
			// Missing paths must not exist under either binary's cwd.
			args := make([]string, len(tc.args))
			for i, a := range tc.args {
				if a == "does-not-exist" {
					a = filepath.Join(dir, "does-not-exist")
				}
				args[i] = a
			}

			goOut, goErr, goCode := runOne(bin, args, env, dir)
			if tc.native {
				embedded, err := os.ReadFile(filepath.Join(root, "internal", "cli", "help.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(goOut, embedded) {
					t.Errorf("native help output differs from embedded help.txt (%d vs %d bytes)", goOut, len(embedded))
				}
				if len(goErr) != 0 || goCode != 0 {
					t.Errorf("native help: stderr = %q, exit = %d; want empty/0", goErr, goCode)
				}
				return
			}
			want, ok := frozen[tc.name]
			if !ok {
				t.Fatalf("no frozen legacy record for %s", tc.name)
			}
			norm := smokeNormalizer(root, dir)
			if got := norm.Replace(string(goOut)); got != want.Stdout {
				t.Errorf("stdout mismatch:\n go:     %q\n frozen: %q", got, want.Stdout)
			}
			if got := norm.Replace(string(goErr)); got != want.Stderr {
				t.Errorf("stderr mismatch:\n go:     %q\n frozen: %q", got, want.Stderr)
			}
			if goCode != want.Exit {
				t.Errorf("exit code mismatch: go = %d, frozen legacy = %d", goCode, want.Exit)
			}
		})
	}
}

// TestDoctorMissingPathDeviation pins the one DOCUMENTED deviation between the
// legacy launcher and the native doctor port for a nonexistent target path, so
// the differential table above does not need to hide it.
//
// lib/doctor.sh resolves the target with `cd "$TARGET_PATH" && pwd` and has no
// `-d` guard, so a nonexistent path dies inside bash: its own `cd:` diagnostic
// (with a script line number) goes to stderr and the process exits 1. The
// Python `is not a directory` guard is therefore dead code on the legacy path.
// The port skips the bash layer, preserves the frozen exit code 1, and reports
// the message the Python guard intended instead of fabricating a bash line
// number.
func TestDoctorMissingPathDeviation(t *testing.T) {
	bin, root := smokeSetup(t)
	env := smokeEnv(root)

	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}
	resolvedMissing := filepath.Join(resolvedDir, "does-not-exist")

	goOut, goErr, goCode := runOne(bin, []string{"doctor", missing}, env, dir)
	leg := loadLegacySmokeGolden(t)["doctor-missing-path"]

	// The frozen exit code is preserved on both sides; neither writes stdout.
	if goCode != 1 || leg.Exit != 1 {
		t.Errorf("exit codes: go = %d, frozen legacy = %d; want 1 for both", goCode, leg.Exit)
	}
	if len(goOut) != 0 || leg.Stdout != "" {
		t.Errorf("stdout: go = %q, frozen legacy = %q; want empty", goOut, leg.Stdout)
	}

	// Legacy (frozen): bash's own cd diagnostic names the missing path, normalized
	// to <DIR>. The line number is a bash artifact and is deliberately not matched.
	if !strings.Contains(leg.Stderr, "cd:") ||
		!strings.Contains(leg.Stderr, "<DIR>/does-not-exist") ||
		!strings.Contains(leg.Stderr, "No such file or directory") {
		t.Errorf("frozen legacy stderr = %q; want bash's `cd: <path>: No such file or directory`", leg.Stderr)
	}

	// Port: the Python guard's intended message, on the resolved target.
	want := "ERROR: " + resolvedMissing + " is not a directory.\n"
	if string(goErr) != want {
		t.Errorf("go stderr = %q, want %q", goErr, want)
	}
}
