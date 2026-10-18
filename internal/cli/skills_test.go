package cli

// GO-20 U5a (T2): differential for the native `ai-specs skills list`.
// Same discipline as smoke_test.go: the same project is read through BOTH
// routes — `bash lib/skills-list.sh <root>` and `<binary> skills list <root>`
// — under one shared, pinned environment, and stdout, stderr and the exit
// code must be byte-identical (odd/tasks/go-20-u5-skills.md S3/S4).
//
// Hermeticity: AI_SPECS_HOME is pinned to an isolated home built like
// tests/_blackbox.py:isolated_home (install entries symlinked in, cache/
// local to the temp dir), so neither leg touches the real ~/.cache or the
// repository, and PYTHONDONTWRITEBYTECODE=1 keeps the python legs from
// leaving __pycache__ anywhere. LC_ALL=C pins bash's glob collation to byte
// order — the order the port (and Python's sorted) always produces.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// skillsListEnv is the environment both legs share: AI_SPECS_HOME pinned to
// the isolated home (stale AI_SPECS_* values stripped), bytecode writes
// suppressed, and C collation for locale-stable glob order.
func skillsListEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "AI_SPECS_HOME="),
			strings.HasPrefix(kv, "AI_SPECS_INVOKED_AS="),
			strings.HasPrefix(kv, "AI_SPECS_GO_BIN="),
			strings.HasPrefix(kv, "PYTHONDONTWRITEBYTECODE="),
			strings.HasPrefix(kv, "LC_ALL="):
		default:
			env = append(env, kv)
		}
	}
	return append(env,
		"AI_SPECS_HOME="+home,
		"PYTHONDONTWRITEBYTECODE=1",
		"LC_ALL=C",
	)
}

// skillsListHome builds an isolated home like tests/_blackbox.py's
// isolated_home: every install entry that exists in the repo is symlinked in
// (catalog and bundled-skills included, so the catalog section and the
// bundled-id exclusion read REAL data), plus a local cache/ dir. Entries
// named in skip are left out — a home without bundled-skills is what
// exercises the script's fallback id pair.
func skillsListHome(t *testing.T, root string, skip ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, entry := range []string{
		"bin", "lib", "catalog", "VERSION",
		"bundled-skills", "bundled-commands", "templates", "scripts",
	} {
		if slices.Contains(skip, entry) {
			continue
		}
		src := filepath.Join(root, entry)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Symlink(src, filepath.Join(home, entry)); err != nil {
			t.Fatalf("symlink %s into home: %v", entry, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	return home
}

// skillsWriteFile writes content under path, creating parent directories.
func skillsWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// skillsListFixtures builds the three project shapes the differential runs:
// a two-dep project (one vendored in the real .deps layout, one not synced),
// a project with no manifest at all, and a local-skills project (one skill
// with a description, one directory without SKILL.md, and one skill that must
// be EXCLUDED because it is a bundled id).
func skillsListFixtures(t *testing.T) (projDeps, projNoManifest, projLocal string) {
	t.Helper()
	base := t.TempDir()

	projDeps = filepath.Join(base, "two-deps")
	skillsWriteFile(t, filepath.Join(projDeps, "ai-specs", "ai-specs.toml"),
		`[project]
name = "two-deps"

[[deps]]
id = "vendored-skill"
source = "https://github.com/example/vendored.git"
ref = "v1.2.3"
path = "nested"
license = "MIT"
scope = ["root"]

[[deps]]
id = "unsynced-skill"
source = "https://github.com/example/unsynced.git"
rev = "deadbeef"
scope = ["root"]
`)
	// The vendored state reads the REAL .deps layout (skills-list.sh:118).
	skillsWriteFile(t, filepath.Join(projDeps, "ai-specs", ".deps",
		"vendored-skill", "skills", "vendored-skill", "SKILL.md"),
		"---\nname: vendored-skill\ndescription: Vendored description\n---\n# vendored-skill\n\nBody.\n")

	projNoManifest = filepath.Join(base, "no-manifest")
	if err := os.MkdirAll(projNoManifest, 0o755); err != nil {
		t.Fatal(err)
	}

	projLocal = filepath.Join(base, "local-skills")
	skillsWriteFile(t, filepath.Join(projLocal, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"local-skills\"\n")
	skillsWriteFile(t, filepath.Join(projLocal, "ai-specs", "skills", "local-skill", "SKILL.md"),
		"---\nname: local-skill\ndescription: A local skill description\n---\n# local-skill\n\nBody.\n")
	if err := os.MkdirAll(filepath.Join(projLocal, "ai-specs", "skills", "bare-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillsWriteFile(t, filepath.Join(projLocal, "ai-specs", "skills", "skill-creator", "SKILL.md"),
		"---\nname: skill-creator\ndescription: Bundled id, excluded from local\n---\n# skill-creator\n\nBody.\n")
	return projDeps, projNoManifest, projLocal
}

// skillsTreeSnapshot maps every path under roots to a marker: "D" for a
// directory, "L:<target>" for a symlink, "H:<sha256>" for a regular file —
// so a run that creates, writes, removes or retargets anything fails the
// comparison. Symlinked entries are NOT followed (WalkDir default), which is
// what keeps the symlinked catalog cheap and side-effect free.
func skillsTreeSnapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			key := root + string(filepath.Separator) + rel
			if d.IsDir() {
				out[key] = "D"
				return nil
			}
			info, ierr := os.Lstat(path)
			if ierr != nil {
				return ierr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				target, lerr := os.Readlink(path)
				if lerr != nil {
					return lerr
				}
				out[key] = "L:" + target
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			sum := sha256.Sum256(data)
			out[key] = "H:" + hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("snapshot %s: %v", root, err)
		}
	}
	return out
}

// TestSkillsListDifferentialAgainstShell is T2: every case runs the legacy
// script and the built binary on the SAME project and asserts byte-identical
// stdout, byte-identical stderr and an identical exit code. The whole table
// is additionally read-only: the project trees and both homes must be
// untouched afterwards (the verb must not create the cache directory or the
// project's ai-specs/ tree).
func TestSkillsListDifferentialAgainstShell(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 unavailable (the shell leg parses TOML/front matter with it): %v", err)
	}
	// LOUD skip, same class as "legacy module unavailable": skills-list.sh's
	// own heredocs import tomllib (Python ≥3.11), so on a machine whose
	// python3 is older the LEGACY route itself crashes and there is no oracle
	// to compare against — an environment gap, never a port defect. The probe
	// output makes the skip impossible to miss.
	probe := exec.Command(python3, "-c", "import tomllib")
	probe.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, perr := probe.CombinedOutput(); perr != nil {
		t.Skipf("python3 CANNOT import tomllib, so lib/skills-list.sh cannot run and the differential oracle is unavailable — probe `python3 -c 'import tomllib'` said %v: %s (run with a Python ≥3.11 on PATH, e.g. PATH=/tmp/pybin:$PATH go test ./internal/cli/)", perr, out)
	}
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "skills-list.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/skills-list.sh unavailable: %v", err)
	}

	home := skillsListHome(t, root)                           // full isolated home (bundled-skills present)
	fallbackHome := skillsListHome(t, root, "bundled-skills") // home WITHOUT bundled-skills → fallback id pair
	projDeps, projNoManifest, projLocal := skillsListFixtures(t)

	// S7: seed the CLI bundle in the cache through the ONE authority. If the
	// port's key drifted from project-cache.py's, the shell leg would print
	// the seeded skill and the binary would print `(none)` — the differential
	// fails, which is the proof the two spellings coincide.
	skillsWriteFile(t, filepath.Join(home, "cache", "projects",
		projectcache.CacheKey(projDeps), ".bundled", "skills", "cached-skill", "SKILL.md"),
		"---\nname: cached-skill\ndescription: Bundled description\n---\n# cached-skill\n\nBody.\n")

	type diffCase struct {
		name string
		home string
		args []string // after `skills list` (Go) / after the script name (bash)
		dir  string   // working directory for both legs; "" → a fresh temp dir
	}
	cases := []diffCase{
		{"two deps one vendored", home, []string{projDeps}, ""},
		{"no manifest", home, []string{projNoManifest}, ""},
		{"local skills with and without SKILL.md", home, []string{projLocal}, ""},
		{"local skills fallback bundled ids", fallbackHome, []string{projLocal}, ""},
		{"help", home, []string{"--help"}, ""},
		{"unknown flag", home, []string{"--bogus"}, ""},
		{"second positional", home, []string{projDeps, "extra"}, ""},
		{"positional after dashdash", home, []string{"--", projDeps}, ""},
		{"second positional after dashdash", home, []string{"--", "alpha", "beta"}, ""},
		{"target defaults to cwd", home, nil, projDeps},
	}

	roots := []string{projDeps, projNoManifest, projLocal, home, fallbackHome}
	before := skillsTreeSnapshot(t, roots...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.dir
			if dir == "" {
				dir = t.TempDir()
			}
			env := skillsListEnv(tc.home)

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, tc.args...), env, dir)
			goOut, goErr, goCode := runOne(bin,
				append([]string{"skills", "list"}, tc.args...), env, dir)

			if !bytes.Equal(goOut, legOut) {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", goOut, legOut)
			}
			if !bytes.Equal(goErr, legErr) {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", goErr, legErr)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}
		})
	}

	after := skillsTreeSnapshot(t, roots...)
	if !reflect.DeepEqual(before, after) {
		for key, marker := range after {
			if before[key] != marker {
				t.Errorf("read-only violated: %s: %q -> %q", key, before[key], marker)
			}
		}
		for key := range before {
			if _, ok := after[key]; !ok {
				t.Errorf("read-only violated: %s disappeared", key)
			}
		}
	}
}

// TestSkillsListNativeNeedsNoBashOrPython is the routing proof for the U5a
// goal: with PATH emptied the legacy route could not run at all (`bash
// lib/skills.sh …` → skills-list.sh → its python heredocs), while the native
// route prints the listing — `skills list` no longer execs the shim, and no
// Python runs on it (S1: zero Python under the product tree).
func TestSkillsListNativeNeedsNoBashOrPython(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	project := t.TempDir() // no ai-specs/ tree: read-only, not initialized
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin, []string{"skills", "list", project}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	if !strings.HasPrefix(string(out), "=== ai-specs skills list ===\n") {
		t.Errorf("stdout = %q, want the frozen header", out)
	}
}

// TestSkillsListInvalidManifestContinues pins the malformed-manifest branch
// in-process: the two invalid lines (stderr diagnostic + stdout skip line)
// and the listing CONTINUING through all four sections to exit 0. The exact
// parse-error wording is diagnostic only (ADR 0002) — our parser words its
// own — so this branch is shape-pinned here instead of byte-differentials.
func TestSkillsListInvalidManifestContinues(t *testing.T) {
	home := t.TempDir() // no catalog: the section ends with (not found)
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
		"[project]\nname = \"broken\"\n[[deps]]\nid = \n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "list", project}, home, nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(stderr.String(), "  ✗ manifest invalid: ") {
		t.Errorf("stderr = %q, want the frozen invalid-manifest diagnostic prefix", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "  (skipped — fix ai-specs.toml to list deps)\n") {
		t.Errorf("stdout = %q, want the skip line", out)
	}
	for _, header := range []string{
		"=== ai-specs skills list ===\n",
		"── Registered deps ([[deps]]) ──\n",
		"── Bundled skills (CLI-shipped) ──\n",
		"── Local skills (ai-specs/skills/) ──\n",
		"── Available catalog skills (catalog/skills/) ──\n",
	} {
		if !strings.Contains(out, header) {
			t.Errorf("stdout missing %q:\n%s", header, out)
		}
	}
	// The catalog section is last and has NO trailing blank line.
	wantTail := "── Available catalog skills (catalog/skills/) ──\n  (not found)\n"
	if !strings.HasSuffix(out, wantTail) {
		t.Errorf("stdout tail = %q, want suffix %q", tailOf(out, len(wantTail)), wantTail)
	}
}

func tailOf(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}

// ── GO-20 U5b (T4): differential for the native `ai-specs skills remove` ──
//
// Same discipline as the U5a table above: the shell script and the built
// binary run against byte-identical TWIN projects (the verb mutates, so one
// shared project cannot serve both legs), under one shared, pinned
// environment, and stdout, stderr and the exit code must be byte-identical
// after normalizing each leg's own project root out of the messages. The
// resulting project trees are compared too: manifest bytes, the pruned .deps
// copy and the lock file are all part of the contract (S5, and the D17/D18
// steps the script performs after the manifest write).

// skillsRemoveEnv is skillsListEnv's pinning plus WORKTREE_GATE_BIN pinned
// to a freshly built worktree-gate. The shell leg's lock.py writes through
// the gate's `--write-lock` bridge; with no pinned binary it fails open with
// a GO_LOCK_WRITE_BRIDGE_FALLBACK warning on stderr that the native leg never
// prints, and the byte comparison would then measure the fail-open bridge
// instead of the verb (same rationale as internal/sync/materialize_test.go).
func skillsRemoveEnv(home, gate string) []string {
	env := skillsListEnv(home)
	out := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "WORKTREE_GATE_BIN=") {
			continue
		}
		out = append(out, kv)
	}
	if gate != "" {
		out = append(out, "WORKTREE_GATE_BIN="+gate)
	}
	return out
}

var (
	skillsRemoveGateOnce sync.Once
	skillsRemoveGatePath string
	skillsRemoveGateErr  error
)

// skillsRemoveGateBinary builds the worktree-gate binary once per test run
// (the gate module ships no external dependencies, so the build needs no
// network) and returns its path. An unavailable build skips LOUDLY: without
// the pinned bridge the shell leg's lock write is the fail-open fallback and
// no byte comparison of the lock step is possible.
func skillsRemoveGateBinary(t *testing.T) string {
	t.Helper()
	skillsRemoveGateOnce.Do(func() {
		module, err := filepath.Abs(filepath.Join("..", "..", "catalog", "recipes", "worktree-flow", "gate"))
		if err != nil {
			skillsRemoveGateErr = err
			return
		}
		dir, err := os.MkdirTemp("", "skills-remove-gate-")
		if err != nil {
			skillsRemoveGateErr = err
			return
		}
		out := filepath.Join(dir, "worktree-gate")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = module
		cmd.Env = os.Environ()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			skillsRemoveGateErr = fmt.Errorf("go build worktree-gate: %v: %s", err, stderr.String())
			return
		}
		skillsRemoveGatePath = out
	})
	if skillsRemoveGateErr != nil {
		t.Skipf("worktree-gate build unavailable (the shell leg's lock write needs it pinned through WORKTREE_GATE_BIN): %v", skillsRemoveGateErr)
	}
	return skillsRemoveGatePath
}

// skillsRemoveOracle skips LOUDLY when python3 cannot import tomllib: the
// legacy script's manifest and lock heredocs import it (Python ≥3.11), so on
// a machine whose python3 is older the LEGACY route itself crashes and there
// is no oracle to compare against — an environment gap, never a port defect.
// Same probe, same message shape as the U5a table.
func skillsRemoveOracle(t *testing.T) {
	t.Helper()
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 unavailable (the shell leg parses TOML with it): %v", err)
	}
	probe := exec.Command(python3, "-c", "import tomllib")
	probe.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, perr := probe.CombinedOutput(); perr != nil {
		t.Skipf("python3 CANNOT import tomllib, so lib/skills-remove.sh cannot run and the differential oracle is unavailable — probe `python3 -c 'import tomllib'` said %v: %s (run with a Python ≥3.11 on PATH, e.g. PATH=/tmp/pybin:$PATH go test ./internal/cli/)", perr, out)
	}
}

// The fixture manifests and locks the remove cases start from.
const (
	removeManifestVendored = `[project]
name = "two-deps"

[[deps]]
id = "vendored-skill"
source = "https://github.com/example/vendored.git"
scope = ["root"]

[[deps]]
id = "other-skill"
source = "https://github.com/example/other.git"
scope = ["root"]
`
	removeManifestUnsynced = `[project]
name = "unsynced"

[[deps]]
id = "unsynced-skill"
source = "https://github.com/example/unsynced.git"
scope = ["root"]

[[deps]]
id = "other-skill"
source = "https://github.com/example/other.git"
scope = ["root"]
`
	removeManifestOnly = `[[deps]]
id = "only-skill"
source = "https://github.com/example/only.git"
scope = ["root"]
`
	removeLockBoth = `[deps."vendored-skill".skills."vendored-skill"]
"SKILL.md" = "hash-vendored"

[deps."other-skill".skills."other-skill"]
"SKILL.md" = "hash-other"
`
	removeLockOther = `[deps."other-skill".skills."other-skill"]
"SKILL.md" = "hash-other"
`
)

// buildRemoveFixture materializes one case's project shape under proj:
//   - vendored: two deps, the target vendored in the REAL .deps layout plus a
//     sibling copy (prune selectivity), and a lock carrying hashes for both.
//   - unsynced: two deps, only the sibling vendored, lock with only the
//     sibling's entry — so removing the target must print NO prune line and
//     NO lock line and must leave the lock byte-identical.
//   - onlydep: the [[deps]] block is the only table in the file.
//   - empty: the project directory exists with no ai-specs/ tree.
func buildRemoveFixture(t *testing.T, name, proj string) {
	t.Helper()
	switch name {
	case "vendored":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), removeManifestVendored)
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".deps",
			"vendored-skill", "skills", "vendored-skill", "SKILL.md"),
			"---\nname: vendored-skill\n---\n# vendored-skill\n")
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".deps",
			"other-skill", "skills", "other-skill", "SKILL.md"),
			"---\nname: other-skill\n---\n# other-skill\n")
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".ai-specs.lock"), removeLockBoth)
	case "unsynced":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), removeManifestUnsynced)
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".deps",
			"other-skill", "skills", "other-skill", "SKILL.md"),
			"---\nname: other-skill\n---\n# other-skill\n")
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", ".ai-specs.lock"), removeLockOther)
	case "onlydep":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), removeManifestOnly)
	case "empty":
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown remove fixture %q", name)
	}
}

// skillsRemoveTree snapshots a project and rebases the keys on the project
// root, so two twin projects in different temp dirs compare directly.
func skillsRemoveTree(t *testing.T, proj string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for key, marker := range skillsTreeSnapshot(t, proj) {
		out[strings.TrimPrefix(key, proj+string(filepath.Separator))] = marker
	}
	return out
}

// TestSkillsRemoveDifferentialAgainstShell is T4: every case runs the legacy
// script and the built binary on byte-identical twin projects and asserts
// byte-identical stdout, stderr and exit code (project roots normalized),
// plus identical resulting trees — the manifest, the pruned .deps copy and
// the lock are all compared as bytes/hashes. The CLI home is snapshotted
// around the whole table: the verb must never write into it (S7's cache
// authority is read-only here; the script itself never touches the cache).
func TestSkillsRemoveDifferentialAgainstShell(t *testing.T) {
	skillsRemoveOracle(t)
	bin, root := smokeSetup(t)
	script := filepath.Join(root, "lib", "skills-remove.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy lib/skills-remove.sh unavailable: %v", err)
	}
	gate := skillsRemoveGateBinary(t)
	home := skillsListHome(t, root)
	env := skillsRemoveEnv(home, gate)

	type diffCase struct {
		name      string
		fixture   string   // "" → no project directory is created
		args      []string // "<PROJ>" is replaced by each leg's project root
		cwdProj   bool     // run each leg with cwd = its own project dir
		untouched bool     // both legs must leave the project byte-identical
	}
	cases := []diffCase{
		{"dep exists and vendored", "vendored", []string{"vendored-skill", "<PROJ>"}, false, false},
		{"dep exists but never vendored", "unsynced", []string{"unsynced-skill", "<PROJ>"}, false, false},
		{"dep does not exist", "vendored", []string{"ghost-skill", "<PROJ>"}, false, true},
		{"missing id", "", nil, false, false},
		{"help", "", []string{"--help"}, false, false},
		{"unknown flag", "", []string{"--bogus"}, false, false},
		{"third positional", "", []string{"alpha", "beta", "gamma"}, false, false},
		{"only deps block", "onlydep", []string{"only-skill", "<PROJ>"}, false, false},
		{"manifest not found", "empty", []string{"some-skill", "<PROJ>"}, false, true},
		{"invalid dep id", "vendored", []string{"../escape", "<PROJ>"}, false, true},
		{"positional after dashdash", "vendored", []string{"--", "vendored-skill", "<PROJ>"}, false, false},
		{"target defaults to cwd", "vendored", []string{"vendored-skill"}, true, false},
	}

	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
			if tc.fixture != "" {
				buildRemoveFixture(t, tc.fixture, projA)
				buildRemoveFixture(t, tc.fixture, projB)
			}
			beforeA := skillsRemoveTree(t, projA)
			beforeB := skillsRemoveTree(t, projB)

			legArgs := func(proj string) []string {
				out := make([]string, 0, len(tc.args))
				for _, a := range tc.args {
					out = append(out, strings.ReplaceAll(a, "<PROJ>", proj))
				}
				return out
			}
			dirLeg := func(proj string) string {
				if tc.cwdProj {
					return proj
				}
				return scratch
			}

			legOut, legErr, legCode := runOne("bash",
				append([]string{script}, legArgs(projA)...), env, dirLeg(projA))
			goOut, goErr, goCode := runOne(bin,
				append([]string{"skills", "remove"}, legArgs(projB)...), env, dirLeg(projB))

			normalize := func(b []byte, proj string) string {
				s := string(b)
				// The shell resolves the cwd through getcwd (e.g. /var →
				// /private/var on macOS) when a leg runs with cwd = project;
				// replace the resolved spelling first so both spellings
				// normalize to the same placeholder.
				if resolved, err := filepath.EvalSymlinks(proj); err == nil && resolved != proj {
					s = strings.ReplaceAll(s, resolved, "<PROJ>")
				}
				return strings.ReplaceAll(s, proj, "<PROJ>")
			}
			if got, want := normalize(goOut, projB), normalize(legOut, projA); got != want {
				t.Errorf("stdout mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if got, want := normalize(goErr, projB), normalize(legErr, projA); got != want {
				t.Errorf("stderr mismatch:\n go:   %q\n bash: %q", got, want)
			}
			if goCode != legCode {
				t.Errorf("exit code mismatch: go = %d, bash = %d", goCode, legCode)
			}

			afterA := skillsRemoveTree(t, projA)
			afterB := skillsRemoveTree(t, projB)
			if !reflect.DeepEqual(afterA, afterB) {
				t.Errorf("project trees diverge after the two legs:\n bash: %v\n go:   %v", afterA, afterB)
			}
			if tc.untouched {
				if !reflect.DeepEqual(beforeA, afterA) {
					t.Errorf("bash leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeA, afterA)
				}
				if !reflect.DeepEqual(beforeB, afterB) {
					t.Errorf("go leg mutated a project that must stay untouched:\n before %v\n after  %v", beforeB, afterB)
				}
			}
		})
	}

	if after := skillsTreeSnapshot(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Errorf("the verb wrote into the CLI home (home/cache must stay untouched):\n before %v\n after  %v", homeBefore, after)
	}
}

// TestSkillsRemoveReadOnlyDeviation pins the one behavioural deviation the
// byte differential cannot express (the same class as the U5a cd-prefix
// deviation): the legacy heredoc writes the manifest through mkstemp +
// os.replace, and rename(2) needs write permission on the DIRECTORY, not the
// file — so the script SUCCEEDS on a read-only manifest and completes the
// whole tail (prune + lock). The native leg refuses instead: the shared
// config atomicWrite carries R4-atomicwrite-readonly-refusal (a pinned product
// rule, odd/tasks/go-20-u3b-advisories.md), which the SCRIPT does not have.
// The script is the contract; this divergence is recorded here and reported,
// not hidden — the refusal itself lives in internal/config, outside this
// unit's edit surface.
func TestSkillsRemoveReadOnlyDeviation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the file mode does not stop a rename over the manifest")
	}
	skillsRemoveOracle(t)
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "skills-remove.sh")
	gate := skillsRemoveGateBinary(t)
	// A home WITH lib/: the script's lock heredoc loads lock.py from
	// $AI_SPECS_HOME/lib/_internal, so a bare temp dir would crash the shell
	// leg after the manifest write instead of exercising the lock step.
	home := skillsListHome(t, root)
	env := skillsRemoveEnv(home, gate)

	dirA, dirB := t.TempDir(), t.TempDir()
	projA, projB := filepath.Join(dirA, "proj"), filepath.Join(dirB, "proj")
	buildRemoveFixture(t, "vendored", projA)
	buildRemoveFixture(t, "vendored", projB)
	for _, proj := range []string{projA, projB} {
		if err := os.Chmod(filepath.Join(proj, "ai-specs", "ai-specs.toml"), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	manifestB := filepath.Join(projB, "ai-specs", "ai-specs.toml")
	lockB := filepath.Join(projB, "ai-specs", ".ai-specs.lock")
	manifestBefore, err := os.ReadFile(manifestB)
	if err != nil {
		t.Fatal(err)
	}
	lockBefore, err := os.ReadFile(lockB)
	if err != nil {
		t.Fatal(err)
	}

	// Legacy leg: succeeds end to end and preserves the 0444 mode.
	legOut, legErr, legCode := runOne("bash",
		[]string{script, "vendored-skill", projA}, env, t.TempDir())
	if legCode != 0 {
		t.Errorf("bash exit = %d (want 0: mkstemp+rename bypasses the file mode), stderr = %q", legCode, legErr)
	}
	for _, want := range []string{
		"  ✓ removed [[deps]] 'vendored-skill' from " + projA + "/ai-specs/ai-specs.toml\n",
		"  ✓ pruned ai-specs/.deps/vendored-skill/\n",
		"  ✓ removed lock hashes for 'vendored-skill'\n",
	} {
		if !strings.Contains(string(legOut), want) {
			t.Errorf("bash stdout = %q, want it to contain %q", legOut, want)
		}
	}
	manifestAData, err := os.ReadFile(filepath.Join(projA, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifestAData), "vendored-skill") || !strings.Contains(string(manifestAData), "other-skill") {
		t.Errorf("bash manifest after removal = %q", manifestAData)
	}
	if fi, err := os.Stat(filepath.Join(projA, "ai-specs", "ai-specs.toml")); err != nil || fi.Mode().Perm() != 0o444 {
		t.Errorf("bash leg mode after write: %v, %v; want 0444 preserved", fi, err)
	}
	if isDirectory(filepath.Join(projA, "ai-specs", ".deps", "vendored-skill")) {
		t.Error("bash leg did not prune ai-specs/.deps/vendored-skill/")
	}

	// Native leg: the shared writer refuses; nothing is mutated at all.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "remove", "vendored-skill", projB}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen write-failure exit), stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "refusing to overwrite read-only manifest") {
		t.Errorf("go stderr = %q, want the R4 read-only refusal", stderr.String())
	}
	manifestAfter, err := os.ReadFile(manifestB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifestBefore, manifestAfter) {
		t.Errorf("go leg mutated a refused manifest:\n before %q\n after  %q", manifestBefore, manifestAfter)
	}
	lockAfter, err := os.ReadFile(lockB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lockBefore, lockAfter) {
		t.Errorf("go leg mutated the lock despite the refusal:\n before %q\n after  %q", lockBefore, lockAfter)
	}
	if !isDirectory(filepath.Join(projB, "ai-specs", ".deps", "vendored-skill")) {
		t.Error("go leg pruned .deps despite refusing the manifest write")
	}
}

// TestSkillsRemoveMissingPathDeviation is the U5a deviation's twin for
// remove: a target that is not an existing directory died inside bash's own
// `cd` in the legacy script (lib/skills-remove.sh: line 58: cd: <path>:
// <reason>, exit 1). The port keeps exit 1, the `cd: <path>: <reason>` shape
// and the strerror reason, but does not fabricate the script name and
// line-number prefix.
func TestSkillsRemoveMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "skills-remove.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "remove", "some-id", missing}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen cd exit)", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	want := "cd: " + missing + ": No such file or directory\n"
	if stderr.String() != want {
		t.Errorf("go stderr = %q, want %q", stderr.String(), want)
	}

	legOut, legErr, legCode := runOne("bash", []string{script, "some-id", missing}, env, dir)
	if legCode != 1 {
		t.Errorf("bash exit = %d, want 1", legCode)
	}
	if len(legOut) != 0 {
		t.Errorf("bash stdout = %q, want empty", legOut)
	}
	// Legacy: bash's own cd diagnostic names the path; its
	// `lib/skills-remove.sh: line N: ` prefix is a bash artifact, deliberately
	// not matched (same class as TestSkillsListMissingPathDeviation).
	for _, part := range []string{"cd:", missing, "No such file or directory"} {
		if !strings.Contains(string(legErr), part) {
			t.Errorf("bash stderr = %q, want it to contain %q", legErr, part)
		}
	}
}

// TestSkillsRemoveNativeNeedsNoBashOrPython is the routing proof for the U5b
// goal: with PATH emptied the legacy route could not run at all (the shim
// execs bash lib/skills.sh → skills-remove.sh → its python heredocs), while
// the native route rewrites the manifest, prunes the vendored copy and drops
// the lock hashes — `skills remove` no longer execs the shim, and no Python
// runs on it (S1: zero Python under the product tree).
func TestSkillsRemoveNativeNeedsNoBashOrPython(t *testing.T) {
	bin, _ := smokeSetup(t)
	home := t.TempDir()
	project := t.TempDir()
	buildRemoveFixture(t, "vendored", project)
	env := skillsListEnv(home)
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin,
		[]string{"skills", "remove", "vendored-skill", project}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	for _, want := range []string{
		"  ✓ removed [[deps]] 'vendored-skill' from ",
		"  ✓ pruned ai-specs/.deps/vendored-skill/\n",
		"  ✓ removed lock hashes for 'vendored-skill'\n",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(project, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "vendored-skill") || !strings.Contains(string(manifest), "other-skill") {
		t.Errorf("manifest after native removal = %q", manifest)
	}
	if isDirectory(filepath.Join(project, "ai-specs", ".deps", "vendored-skill")) {
		t.Error("vendored copy was not pruned")
	}
	lock, err := os.ReadFile(filepath.Join(project, "ai-specs", ".ai-specs.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(lock), "vendored-skill") || !strings.Contains(string(lock), "other-skill") {
		t.Errorf("lock after native removal = %q", lock)
	}
}

// TestSkillsListMissingPathDeviation pins the one DOCUMENTED deviation
// between lib/skills-list.sh and the port for a target that is not an
// existing directory, so the differential table above never has to hide it.
// The frozen exit code (1), the empty stdout and the `cd: <path>: <reason>`
// shape survive; only bash's script-name/line-number prefix is dropped (a
// bash artifact the port refuses to fabricate).
func TestSkillsListMissingPathDeviation(t *testing.T) {
	root, err := locateRoot()
	if err != nil {
		t.Skipf("repo root not located: %v", err)
	}
	script := filepath.Join(root, "lib", "skills-list.sh")
	home := t.TempDir()
	env := skillsListEnv(home)
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "list", missing}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen cd exit)", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	want := "cd: " + missing + ": No such file or directory\n"
	if stderr.String() != want {
		t.Errorf("go stderr = %q, want %q", stderr.String(), want)
	}

	legOut, legErr, legCode := runOne("bash", []string{script, missing}, env, dir)
	if legCode != 1 {
		t.Errorf("bash exit = %d, want 1", legCode)
	}
	if len(legOut) != 0 {
		t.Errorf("bash stdout = %q, want empty", legOut)
	}
	// Legacy: bash's own cd diagnostic names the path. The
	// `lib/skills-list.sh: line N: ` prefix is a bash artifact, deliberately
	// not matched.
	for _, part := range []string{"cd:", missing, "No such file or directory"} {
		if !strings.Contains(string(legErr), part) {
			t.Errorf("bash stderr = %q, want it to contain %q", legErr, part)
		}
	}
}
