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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
