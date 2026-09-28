package target

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/toml"
)

// repoRoot returns the worktree root (the go test process runs with cwd =
// the package directory internal/target).
func repoRoot(t *testing.T) string {
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

type cliResult struct {
	stdout string
	stderr string
	code   int
}

// runPythonCLI drives `python3 lib/_internal/target-resolve.py <root>` with
// cwd = the worktree root, so sys.argv[0] is exactly
// "lib/_internal/target-resolve.py" — the literal the Go side claims too.
func runPythonCLI(t *testing.T, root string) cliResult {
	t.Helper()
	return runPythonCLIArgs(t, root)
}

func runPythonCLIArgs(t *testing.T, args ...string) cliResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", append([]string{"lib/_internal/target-resolve.py"}, args...)...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("python3 lib/_internal/target-resolve.py %v: %v", args, err)
		}
		return cliResult{outBuf.String(), errBuf.String(), ee.ExitCode()}
	}
	return cliResult{outBuf.String(), errBuf.String(), 0}
}

// runGoCLI drives the Go Run with the same argument list (program name
// excluded; Run owns the identical usage line).
func runGoCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	code := Run(args, &outBuf, &errBuf)
	return cliResult{outBuf.String(), errBuf.String(), code}
}

// compareDifferential asserts byte-identical stdout, stderr, and exit code.
func compareDifferential(t *testing.T, name string, py, gRes cliResult) {
	t.Helper()
	if py.code != gRes.code {
		t.Errorf("%s: exit code mismatch: python=%d go=%d\npython stderr: %q\ngo stderr: %q",
			name, py.code, gRes.code, py.stderr, gRes.stderr)
		return
	}
	if py.stdout != gRes.stdout {
		t.Errorf("%s: stdout bytes differ\n--- python ---\n%s\n--- go ---\n%s", name, py.stdout, gRes.stdout)
		return
	}
	if py.stderr != gRes.stderr {
		t.Errorf("%s: stderr bytes differ\n--- python ---\n%q\n--- go ---\n%q", name, py.stderr, gRes.stderr)
	}
}

// firstDiff returns a short human-readable pointer at the first differing byte.
func firstDiff(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return fmt.Sprintf("first diff at byte %d: %q vs %q", i, a[max(0, i-20):i+20], b[max(0, i-20):i+20])
		}
	}
	return fmt.Sprintf("lengths differ: %d vs %d", len(a), len(b))
}

// --- test tree helpers ------------------------------------------------------

type tree struct {
	manifest string
	dirs     []string
	files    []string
	links    map[string]string // link path -> target (absolute or relative)
}

// buildTree writes a manifest + dirs/files/symlinks under root.
func buildTree(t *testing.T, root string, spec tree) {
	t.Helper()
	for _, d := range spec.dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range spec.files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range spec.links {
		p := filepath.Join(root, link)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	if spec.manifest != "" {
		dir := filepath.Join(root, "ai-specs")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), []byte(spec.manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// runDifferential runs both CLIs against root with args (default: [root]) and
// compares bytes + exit code.
func runDifferential(t *testing.T, name, root string, goArgs []string) {
	t.Helper()
	if goArgs == nil {
		goArgs = []string{root}
	}
	py := runPythonCLI(t, root)
	gRes := runGoCLI(t, goArgs...)
	if py.code != gRes.code {
		t.Errorf("%s: exit code mismatch: python=%d go=%d\npython stderr: %q\ngo stderr: %q",
			name, py.code, gRes.code, py.stderr, gRes.stderr)
		return
	}
	if py.stdout != gRes.stdout {
		t.Errorf("%s: stdout bytes differ (%s)\n--- python ---\n%s\n--- go ---\n%s",
			name, firstDiff(py.stdout, gRes.stdout), py.stdout, gRes.stdout)
		return
	}
	if py.stderr != gRes.stderr {
		t.Errorf("%s: stderr bytes differ (%s)\n--- python ---\n%q\n--- go ---\n%q",
			name, firstDiff(py.stderr, gRes.stderr), py.stderr, gRes.stderr)
	}
}

// --- TestDifferentialPlanJSON ------------------------------------------------

// TestDifferentialPlanJSON compares the full plan JSON byte-for-byte for the
// two committed fixtures plus ≥8 synthetic roots covering the normalization,
// validation, worktrees_dir, and topology surfaces.
func TestDifferentialPlanJSON(t *testing.T) {
	root := repoRoot(t)

	// Committed fixtures, copied to temp dirs (both CLIs run against the copy).
	for _, fixture := range []string{"root-only", "multi-target"} {
		t.Run("fixture-"+fixture, func(t *testing.T) {
			tmp := t.TempDir()
			dir := filepath.Join(tmp, fixture)
			copyDir(t, filepath.Join(root, "tests", "fixtures", "target-resolve", fixture), dir)
			runDifferential(t, fixture, dir, nil)
		})
	}

	cases := []struct {
		name string
		tree tree
	}{
		{
			// "./" prefix, backslashes, and duplicate collapse via normpath.
			name: "backslash-and-duplicate",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"./pkg\\\\a\", \"pkg\\\\a\"]\n",
				dirs:     []string{"pkg/a"},
			},
		},
		{
			// Trailing slash + dot segments normalize to the same rel.
			name: "normpath-collapse",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"dup\", \"./dup\", \"dup/\", \"d1/../dup\"]\n",
				dirs:     []string{"dup", "d1"},
			},
		},
		{
			// Empty and whitespace entries are dropped by read_project.
			name: "empty-entries",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"\", \"   \"]\n",
			},
		},
		{
			// Non-string TOML scalars are filtered by read_project before
			// target-resolve sees them (pins the filter on both sides).
			name: "non-string-filtered",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [1, true, 0.5]\n",
			},
		},
		{
			name: "absolute-path-error",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"/etc\"]\n",
			},
		},
		{
			name: "escape-error",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"../outside\"]\n",
			},
		},
		{
			name: "missing-dir-error",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"ghost\"]\n",
			},
		},
		{
			name: "not-a-dir-error",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"f.txt\"]\n",
				files:    []string{"f.txt"},
			},
		},
		{
			name: "worktrees-config-dict",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow]\nenabled = true\nversion = \"1\"\n[recipes.worktree-flow.config]\nworktrees_dir = \"custom-wt\"\n",
			},
		},
		{
			name: "worktrees-flat-style",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow]\nenabled = true\nworktrees_dir = \"flat-wt\"\n",
			},
		},
		{
			// Falsy worktrees_dir falls back to ".worktrees" on both sides.
			name: "worktrees-falsy",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow.config]\nworktrees_dir = \"\"\n",
			},
		},
		{
			// Non-string worktrees_dir goes through str(value).
			name: "worktrees-int",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow.config]\nworktrees_dir = 3\n",
			},
		},
		{
			// A table value goes through str(dict): document-order keys,
			// repr-composed values.
			name: "worktrees-dict",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow.config]\nworktrees_dir = {a = 1, b = \"two\"}\n",
			},
		},
		{
			// Explicit monorepo-apps config: via=config, source=project.
			name: "topology-project",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nrepo_topology = \"monorepo-apps\"\n",
			},
		},
		{
			// Legacy recipe key: source=legacy-recipe, via=config.
			name: "topology-legacy-recipe",
			tree: tree{
				manifest: "[project]\nname = \"t\"\n[recipes.worktree-flow.config]\nrepo_topology = \"standalone\"\n",
			},
		},
		{
			// Unknown topology value takes the auto branch (configured
			// reported as "auto") but source stays "project".
			name: "topology-bogus",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nrepo_topology = \"bogus\"\n",
			},
		},
		{
			// Symlink inside the tree resolves to the real dir.
			name: "symlink-inside",
			tree: tree{
				manifest: "[project]\nname = \"t\"\nsubrepos = [\"alias\"]\n",
				dirs:     []string{"real"},
				links:    map[string]string{"alias": "real"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "proj")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			buildTree(t, dir, tc.tree)
			runDifferential(t, tc.name, dir, nil)
		})
	}

	// Error classes with cross-root symlinks need a sibling outside the root.
	t.Run("symlink-escape", func(t *testing.T) {
		tmp := t.TempDir()
		dir := filepath.Join(tmp, "proj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(tmp, "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		buildTree(t, dir, tree{
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"sneaky\"]\n",
			links:    map[string]string{"sneaky": outside},
		})
		runDifferential(t, "symlink-escape", dir, nil)
	})

	t.Run("symlink-dangling-outside", func(t *testing.T) {
		tmp := t.TempDir()
		dir := filepath.Join(tmp, "proj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(tmp, "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		buildTree(t, dir, tree{
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"dangle\"]\n",
			links:    map[string]string{"dangle": filepath.Join(outside, "nope")},
		})
		runDifferential(t, "symlink-dangling-outside", dir, nil)
	})

	t.Run("symlink-to-file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "proj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		buildTree(t, dir, tree{
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"lfile\"]\n",
			files:    []string{"f.txt"},
			links:    map[string]string{"lfile": "f.txt"},
		})
		runDifferential(t, "symlink-to-file", dir, nil)
	})

	t.Run("nonexistent-root", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing")
		py := runPythonCLI(t, dir)
		gRes := runGoCLI(t, dir)
		if py.code != 1 || gRes.code != 1 {
			t.Errorf("nonexistent-root: want exit 1, python=%d go=%d", py.code, gRes.code)
		}
		compareDifferential(t, "nonexistent-root", py, gRes)
	})
}

// --- TestDifferentialTopologySubmodules --------------------------------------

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	full := append([]string{"-c", "user.email=test@example.com", "-c", "user.name=Test"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s: %v\nstdout: %s\nstderr: %s", args, dir, err, out.String(), errb.String())
	}
}

func TestDifferentialTopologySubmodules(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Run("super-with-submodule", func(t *testing.T) {
		tmp := t.TempDir()
		child := filepath.Join(tmp, "child")
		super := filepath.Join(tmp, "super")

		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(child, "lib.txt"), []byte("c\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, child, "init", "-q", ".")
		gitRun(t, child, "add", "-A")
		gitRun(t, child, "commit", "-q", "-m", "child init")

		if err := os.MkdirAll(super, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(super, "README.md"), []byte("s\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, super, "init", "-q", ".")
		gitRun(t, super, "add", "-A")
		gitRun(t, super, "commit", "-q", "-m", "super init")
		gitRun(t, super, "-c", "protocol.file.allow=always", "submodule", "add", child, "sub")
		gitRun(t, super, "add", "-A")
		gitRun(t, super, "commit", "-q", "-m", "add sub")

		buildTree(t, super, tree{
			manifest: "[project]\nname = \"super\"\nsubrepos = [\"sub\"]\n",
		})
		py := runPythonCLI(t, super)
		gRes := runGoCLI(t, super)
		compareDifferential(t, "super-with-submodule", py, gRes)
		if py.code != 0 {
			t.Fatalf("super-with-submodule: expected exit 0, got %d (stderr: %s)", py.code, py.stderr)
		}
		for _, want := range []string{`"monorepo-submodules"`, `"via": "auto"`, `"sub"`} {
			if !strings.Contains(py.stdout, want) {
				t.Errorf("super-with-submodule: stdout missing %s", want)
			}
		}
	})

	t.Run("super-without-submodules", func(t *testing.T) {
		tmp := t.TempDir()
		super := filepath.Join(tmp, "plain")
		if err := os.MkdirAll(super, 0o755); err != nil {
			t.Fatal(err)
		}
		gitRun(t, super, "init", "-q", ".")
		buildTree(t, super, tree{manifest: "[project]\nname = \"plain\"\nsubrepos = []\n"})
		py := runPythonCLI(t, super)
		gRes := runGoCLI(t, super)
		compareDifferential(t, "super-without-submodules", py, gRes)
		if !strings.Contains(py.stdout, `"resolved": "standalone"`) {
			t.Errorf("super-without-submodules: stdout missing standalone topology:\n%s", py.stdout)
		}
	})
}

// --- TestDifferentialErrors --------------------------------------------------

func TestDifferentialErrors(t *testing.T) {
	t.Run("missing-manifest", func(t *testing.T) {
		dir := t.TempDir()
		py := runPythonCLI(t, dir)
		gRes := runGoCLI(t, dir)
		want := fmt.Sprintf("error: %s not found\n", filepath.Join(resolveNonStrict(absClean(dir)), "ai-specs", "ai-specs.toml"))
		if py.stderr != want || py.code != 1 {
			t.Errorf("missing-manifest python: code=%d stderr=%q want %q", py.code, py.stderr, want)
		}
		compareDifferential(t, "missing-manifest", py, gRes)
	})

	t.Run("usage-zero-args", func(t *testing.T) {
		py := runPythonCLIArgs(t)
		gRes := runGoCLI(t)
		want := "Usage: lib/_internal/target-resolve.py <project_root>\n"
		if py.stderr != want || py.code != 2 {
			t.Errorf("usage python: code=%d stderr=%q want %q", py.code, py.stderr, want)
		}
		compareDifferential(t, "usage-zero-args", py, gRes)
	})

	t.Run("usage-two-args", func(t *testing.T) {
		dir := t.TempDir()
		py := runPythonCLIArgs(t, dir, "extra")
		gRes := runGoCLI(t, dir, "extra")
		compareDifferential(t, "usage-two-args", py, gRes)
		if py.code != 2 {
			t.Errorf("usage-two-args: want exit 2, got %d", py.code)
		}
	})

	// ResolutionError JSON envelope, byte-for-byte, per invalid-entry class.
	classes := []struct {
		name     string
		manifest string
		tree     tree
		wantSub  string
	}{
		{
			name:     "absolute-path",
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"/abs\"]\n",
			wantSub:  `{"error": {"path": "/abs", "reason": "must be relative to the root"}}`,
		},
		{
			name:     "escape",
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"../out\"]\n",
			wantSub:  `{"error": {"path": "../out", "reason": "escapes the root"}}`,
		},
		{
			name:     "missing-dir",
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"ghost\"]\n",
			wantSub:  `{"error": {"path": "ghost", "reason": "directory does not exist"}}`,
		},
		{
			name:     "not-a-dir",
			manifest: "[project]\nname = \"t\"\nsubrepos = [\"f.txt\"]\n",
			tree:     tree{files: []string{"f.txt"}},
			wantSub:  `{"error": {"path": "f.txt", "reason": "path is not a directory"}}`,
		},
	}
	for _, tc := range classes {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "proj")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			buildTree(t, dir, tree{manifest: tc.manifest, files: tc.tree.files, dirs: tc.tree.dirs, links: tc.tree.links})
			py := runPythonCLI(t, dir)
			gRes := runGoCLI(t, dir)
			if !strings.Contains(py.stderr, tc.wantSub) || py.code != 1 {
				t.Errorf("%s python: code=%d stderr=%q want substring %q", tc.name, py.code, py.stderr, tc.wantSub)
			}
			compareDifferential(t, tc.name, py, gRes)
		})
	}
}

// --- Go-only pins ------------------------------------------------------------

func mustParseTOML(t *testing.T, src string) *toml.Table {
	t.Helper()
	data, err := toml.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse test toml: %v", err)
	}
	return data
}

// TestGitFailureDegradationGoOnly pins that git absence/failure degrades to
// (gitmodules_present=true, no submodules) inside detectSubmodules without
// erroring — resolve_repo_topology swallows and reports standalone.
func TestGitFailureDegradationGoOnly(t *testing.T) {
	dir := t.TempDir()
	// .gitmodules present but git is unreachable via PATH.
	if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte("[submodule \"x\"]\n\tpath = x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	present, subs := detectSubmodules(dir)
	if !present {
		t.Errorf("gitmodules_present = false, want true")
	}
	if len(subs) != 0 {
		t.Errorf("submodules = %v, want empty", subs)
	}
	res := resolveRepoTopology(dir, "auto")
	if res.Resolved != "standalone" || res.Via != "auto" {
		t.Errorf("resolveRepoTopology degraded to %+v, want standalone/auto", res)
	}
}

// TestNormalizeDeclaredRelpathGoOnly pins the pre-normalization candidate in
// error payloads and the posixpath.normpath edge cases.
func TestNormalizeDeclaredRelpathGoOnly(t *testing.T) {
	// posixpath.normpath edge cases plus error-candidate pinning.
	ok := map[string]string{
		"a//b":     "a/b",
		"a/./b":    "a/b",
		"a/b/../c": "a/c",
		"./":       ".",
		"  a  ":    "a",
		"a\\b":     "a/b",
		"a/..":     ".",
	}
	for raw, want := range ok {
		got, err := normalizeDeclaredRelpath(raw)
		if err != nil {
			t.Errorf("normalize(%q): unexpected error %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("normalize(%q) = %q, want %q", raw, got, want)
		}
	}
	if _, err := normalizeDeclaredRelpath("a/../.."); err == nil || err.(*ResolutionError).Reason != "escapes the root" {
		t.Errorf("normalize(\"a/../..\"): want escapes-the-root error, got %v", err)
	}
	if _, err := normalizeDeclaredRelpath(int64(1)); err == nil {
		t.Error("normalize(int 1): want must-be-a-string error")
	} else if err.(*ResolutionError).Rel != "1" {
		t.Errorf("normalize(int 1): rel = %q, want \"1\" (python repr)", err.(*ResolutionError).Rel)
	}
	if _, err := normalizeDeclaredRelpath(true); err == nil || err.(*ResolutionError).Rel != "True" {
		t.Errorf("normalize(true): want rel \"True\", got %v", err)
	}
	if _, err := normalizeDeclaredRelpath(0.5); err == nil || err.(*ResolutionError).Rel != "0.5" {
		t.Errorf("normalize(0.5): want rel \"0.5\", got %v", err)
	}
}

// TestTopologyUnitGoOnly pins topology_config source precedence without
// driving the CLI.
func TestTopologyUnitGoOnly(t *testing.T) {
	data := mustParseTOML(t, "[project]\nrepo_topology = \" monorepo-apps \"\n")
	configured, source, dep := topologyConfig(data)
	if configured != "monorepo-apps" || source != "project" || dep != "" {
		t.Errorf("topologyConfig = (%q, %q, %q)", configured, source, dep)
	}

	data = mustParseTOML(t, "[recipes.worktree-flow]\nenabled = true\nrepo_topology = \"standalone\"\n")
	configured, source, dep = topologyConfig(data)
	if configured != "standalone" || source != "legacy-recipe" || dep == "" {
		t.Errorf("topologyConfig legacy = (%q, %q, %q)", configured, source, dep)
	}

	data = mustParseTOML(t, "[project]\nname = \"x\"\n")
	configured, source, dep = topologyConfig(data)
	if configured != "auto" || source != "default" || dep != "" {
		t.Errorf("topologyConfig default = (%q, %q, %q)", configured, source, dep)
	}

	// Project value wins over legacy recipe.
	data = mustParseTOML(t, "[project]\nrepo_topology = \"standalone\"\n[recipes.worktree-flow.config]\nrepo_topology = \"monorepo-apps\"\n")
	configured, source, _ = topologyConfig(data)
	if configured != "standalone" || source != "project" {
		t.Errorf("topologyConfig precedence = (%q, %q)", configured, source)
	}
}

func TestSubmoduleDetectionUnitGoOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	super := filepath.Join(tmp, "super")
	child := filepath.Join(tmp, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, child, "init", "-q", ".")
	gitRun(t, child, "add", "-A")
	gitRun(t, child, "commit", "-q", "-m", "c")
	if err := os.MkdirAll(super, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, super, "init", "-q", ".")
	gitRun(t, super, "-c", "protocol.file.allow=always", "submodule", "add", child, "a-sub")
	gitRun(t, super, "-c", "protocol.file.allow=always", "submodule", "add", child, "b-sub")
	// a-sub left uninitialized (- prefix), b-sub initialized.
	gitRun(t, super, "submodule", "deinit", "-f", "a-sub")

	present, subs := detectSubmodules(super)
	if !present {
		t.Error("gitmodules_present = false, want true")
	}
	want := []string{"b-sub"}
	if fmt.Sprint(subs) != fmt.Sprint(want) {
		t.Errorf("submodules = %v, want %v (a-sub uninitialized must be skipped)", subs, want)
	}
	sort.Strings(subs)
}
