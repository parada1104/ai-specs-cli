package cli

// GO-20 U5a (T2): differential for the native `ai-specs skills list`.
// Same discipline as smoke_test.go: the project is read the way the legacy
// route read it, and stdout, stderr and the exit code must be byte-identical
// to the frozen legacy oracle of testdata/skills_list_golden.json
// (odd/tasks/go-20-u5-skills.md S3/S4). The legacy shell route itself is no
// longer executed: GO-20 U2e T5 PR4a captured it once into three bundles (list,
// remove, add) before lib/ is deleted, so a missing bundle FAILS and nothing
// here shells out, reads lib/ or needs python3.
//
// Hermeticity: AI_SPECS_HOME is pinned to an isolated home built like
// tests/_blackbox.py:isolated_home (install entries symlinked in, cache/
// local to the temp dir), so no leg touches the real ~/.cache or the
// repository, and LC_ALL=C pins the glob collation to byte order — the order
// the port (and Python's sorted) always produces.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
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
	// Fixture layout, not a live read: the list mirrors what
	// tests/_blackbox.py:isolated_home links in. "lib" is kept in the list so the
	// legacy home shape stays described, but no test in this file reads through
	// it any more (it disappears with lib/ itself: a missing entry is skipped).
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

// ── Frozen oracles (GO-20 U2e T5 PR4a S4) ──
//
// The three differential tables below used to run the legacy shell route live
// as their oracle. lib/ is being deleted, so each family's legacy output was
// captured ONCE into internal/cli/testdata/ (one bundle per script family) and
// the tables now replay it: no bash, no python3, no lib/ read, and no
// environment skip — a missing bundle FAILS.
//
// What a bundle freezes, per case, in capture order:
//   - input: the pre-run project tree the fixture wrote, so fixture drift
//     fails as input drift instead of an opaque output mismatch;
//   - rc, stdout, stderr: the legacy exit code and both streams;
//   - tree: the project tree the legacy route left behind.
//
// Trees are root-relative paths plus bytes: a directory as a bare path, a
// symlink as its target, a regular file as its bytes. Permission bits are NOT
// frozen: the fixtures are created with os.WriteFile under the process umask
// and this package runs t.Parallel() tests that must not have the umask pinned
// under them. The two read-only cases still assert their 0444 mode natively
// (an explicit chmod), which is umask-independent.
//
// Host paths never reach a bundle: each run's temporary project roots, cli
// homes, fixture source paths and the checkout root (bash prefixes its own
// diagnostics with the script path) are tokenized (<DIR>, <HOME>, <SRC>,
// <ROOT>) in the frozen stdout, stderr and file bytes, both as created and
// symlink-resolved. Two values are host-derived rather than host paths and are
// normalized on both sides instead: the add family's lock `synced_at` wall
// clock and the dep skill hash, which covers a rendered SKILL.md that embeds
// the temp source path (the rendered bytes themselves are frozen). The add
// family also freezes the local source's derived attribution as <OWNER>, which
// the test expands from its own fixture path. Every other lock byte still
// compares.
//
// A bundle also carries its oracle metadata: the python version that produced
// it and the git blob id of every legacy script the family froze, at base
// 233dcd2. Regenerating a bundle needs a human decision.
const (
	skillsListGoldenFile   = "skills_list_golden.json"
	skillsRemoveGoldenFile = "skills_remove_golden.json"
	skillsAddGoldenFile    = "skills_add_golden.json"
)

// skillsListGoldenCases is the hard-coded identity inventory of the frozen
// list bundle: the exact case names in capture order. It is an independent
// oracle of coverage — kept in the test, never derived from the bundle — so a
// bundle that is missing, renamed, duplicated or reordered cannot validate
// silently.
var skillsListGoldenCases = []string{
	"two deps one vendored",
	"no manifest",
	"local skills with and without SKILL.md",
	"local skills fallback bundled ids",
	"help",
	"unknown flag",
	"second positional",
	"positional after dashdash",
	"second positional after dashdash",
	"target defaults to cwd",
	"missing path",
}

// skillsNode is one frozen tree node: a root-relative path, whether it is a
// directory, the tokenized bytes of a regular file, or a symlink's tokenized
// target.
type skillsNode struct {
	Path    string `json:"path"`
	Dir     bool   `json:"dir,omitempty"`
	Content string `json:"content,omitempty"`
	Link    string `json:"link,omitempty"`
}

// skillsFrozenCase is one frozen legacy run of a family.
type skillsFrozenCase struct {
	Name   string       `json:"name"`
	Input  []skillsNode `json:"input"`
	RC     int          `json:"rc"`
	Stdout string       `json:"stdout"`
	Stderr string       `json:"stderr"`
	Tree   []skillsNode `json:"tree"`
}

// skillsToken is one host path and the token it is frozen as.
type skillsToken struct{ path, token string }

type skillsTokens []skillsToken

// skillsTokenSet pairs each host path with the token it is frozen as (paths and
// tokens alternate), adding the symlink-resolved spelling of every path — the
// macOS /var → /private/var case — and sorting longest first so a resolved form
// is replaced before its raw prefix.
func skillsTokenSet(pairs ...string) skillsTokens {
	var out skillsTokens
	for i := 0; i+1 < len(pairs); i += 2 {
		path, token := pairs[i], pairs[i+1]
		if path == "" {
			continue
		}
		out = append(out, skillsToken{path, token})
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			out = append(out, skillsToken{resolved, token})
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].path) > len(out[j].path) })
	return out
}

func (ts skillsTokens) tokenize(text string) string {
	for _, t := range ts {
		text = strings.ReplaceAll(text, t.path, t.token)
	}
	return text
}

// tree lists root as frozen nodes in sorted order: a directory as a bare path,
// a symlink as its tokenized target (never followed), a regular file as its
// tokenized bytes. A missing root is an empty tree.
func (ts skillsTokens) tree(t *testing.T, root string) []skillsNode {
	t.Helper()
	out := []skillsNode{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			out = append(out, skillsNode{Path: rel, Dir: true})
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out = append(out, skillsNode{Path: rel, Link: ts.tokenize(target)})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, skillsNode{Path: rel, Content: ts.tokenize(string(data))})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// skillsCompareNodes requires identical frozen inventories: the same paths in
// the same order with the same kind, bytes or link target.
func skillsCompareNodes(t *testing.T, label string, got, want []skillsNode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d nodes, want %d\n native = %v\n frozen = %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: node %q differs:\n native = %+v\n frozen = %+v", label, want[i].Path, got[i], want[i])
		}
	}
}

// skillsLoadGolden reads one frozen bundle, validates its oracle metadata, the
// root-relative shape of every frozen path and its case inventory, and indexes
// the cases by name. A missing or malformed bundle fails the test: the oracle
// is never recomputed from the live tree.
func skillsLoadGolden(t *testing.T, file string, want []string) map[string]skillsFrozenCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v (the bundle is a snapshot: regenerating it needs a human decision)", file, err)
	}
	var bundle struct {
		Oracle struct {
			Python  string            `json:"python"`
			Base    string            `json:"base"`
			Scripts map[string]string `json:"scripts"`
			Note    string            `json:"note"`
		} `json:"oracle"`
		Cases []skillsFrozenCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", file, err)
	}
	if bundle.Oracle.Python == "" || bundle.Oracle.Base == "" || bundle.Oracle.Note == "" {
		t.Fatalf("frozen oracle %s carries incomplete metadata: %+v", file, bundle.Oracle)
	}
	if len(bundle.Oracle.Scripts) == 0 {
		t.Fatalf("frozen oracle %s names no legacy script blob id", file)
	}
	for script, blob := range bundle.Oracle.Scripts {
		if len(blob) != 40 {
			t.Fatalf("frozen oracle %s: script %s has blob id %q, want a 40-char SHA-1", file, script, blob)
		}
	}
	names := make([]string, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		names[i] = rec.Name
		for _, tree := range [][]skillsNode{rec.Input, rec.Tree} {
			for _, node := range tree {
				if filepath.IsAbs(node.Path) || strings.HasPrefix(node.Path, "../") {
					t.Fatalf("frozen record %q path %q must be root-relative", rec.Name, node.Path)
				}
			}
		}
	}
	if len(names) != len(want) {
		t.Fatalf("%s: %d records, want %d", file, len(names), len(want))
	}
	seen := make(map[string]bool, len(names))
	for i, name := range names {
		if name != want[i] {
			t.Fatalf("%s: record %d is %q, want %q (identity mismatch)", file, i, name, want[i])
		}
		if seen[name] {
			t.Fatalf("%s: duplicate record %q", file, name)
		}
		seen[name] = true
	}
	byName := make(map[string]skillsFrozenCase, len(bundle.Cases))
	for _, rec := range bundle.Cases {
		byName[rec.Name] = rec
	}
	return byName
}

// skillsCheckCase compares one native run against its frozen record: the
// project tree the fixture wrote (input drift first, snapshotted before the
// run), then rc, stdout and stderr in the tokenized space, then the resulting
// project tree.
func skillsCheckCase(t *testing.T, rec skillsFrozenCase, tokens skillsTokens, input []skillsNode, out, errOut []byte, code int, tree []skillsNode) {
	t.Helper()
	skillsCompareNodes(t, "project input", input, rec.Input)
	if got := tokens.tokenize(string(out)); got != rec.Stdout {
		t.Errorf("stdout mismatch:\n native = %q\n frozen = %q", got, rec.Stdout)
	}
	if got := tokens.tokenize(string(errOut)); got != rec.Stderr {
		t.Errorf("stderr mismatch:\n native = %q\n frozen = %q", got, rec.Stderr)
	}
	if code != rec.RC {
		t.Errorf("exit code: native = %d, frozen = %d (frozen stderr %q)", code, rec.RC, rec.Stderr)
	}
	skillsCompareNodes(t, "project", tree, rec.Tree)
}

// TestSkillsListDifferentialAgainstShell is T2: every case replays the frozen
// legacy `bash lib/skills-list.sh` run of testdata/skills_list_golden.json
// against the built binary on the SAME project and asserts byte-identical
// stdout, byte-identical stderr and an identical exit code. The whole table
// is additionally read-only: the project trees and both homes must be
// untouched afterwards (the verb must not create the cache directory or the
// project's ai-specs/ tree).
func TestSkillsListDifferentialAgainstShell(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsListGoldenFile, skillsListGoldenCases)
	bin, root := smokeSetup(t)

	home := skillsListHome(t, root)                           // full isolated home (bundled-skills present)
	fallbackHome := skillsListHome(t, root, "bundled-skills") // home WITHOUT bundled-skills → fallback id pair
	projDeps, projNoManifest, projLocal := skillsListFixtures(t)
	tokens := skillsTokenSet(
		projLocal, "<DIR>", projNoManifest, "<DIR>", projDeps, "<DIR>",
		home, "<HOME>", fallbackHome, "<HOME>")

	// S7: seed the CLI bundle in the cache through the ONE authority. If the
	// port's key drifted from project-cache.py's, the frozen oracle prints the
	// seeded skill and the binary would print `(none)` — the differential
	// fails, which is the proof the two spellings coincide.
	skillsWriteFile(t, filepath.Join(home, "cache", "projects",
		projectcache.CacheKey(projDeps), ".bundled", "skills", "cached-skill", "SKILL.md"),
		"---\nname: cached-skill\ndescription: Bundled description\n---\n# cached-skill\n\nBody.\n")

	type listCase struct {
		name    string
		home    string
		project string   // the project the case targets; "" → none (its input tree is empty)
		args    []string // after `skills list`
		dir     string   // working directory; "" → a fresh temp dir
	}
	cases := []listCase{
		{"two deps one vendored", home, projDeps, []string{projDeps}, ""},
		{"no manifest", home, projNoManifest, []string{projNoManifest}, ""},
		{"local skills with and without SKILL.md", home, projLocal, []string{projLocal}, ""},
		{"local skills fallback bundled ids", fallbackHome, projLocal, []string{projLocal}, ""},
		{"help", home, "", []string{"--help"}, ""},
		{"unknown flag", home, "", []string{"--bogus"}, ""},
		{"second positional", home, projDeps, []string{projDeps, "extra"}, ""},
		{"positional after dashdash", home, projDeps, []string{"--", projDeps}, ""},
		{"second positional after dashdash", home, "", []string{"--", "alpha", "beta"}, ""},
		{"target defaults to cwd", home, projDeps, nil, projDeps},
	}
	// The corpus and the leading inventory must agree case for case before
	// anything is compared; the trailing record is the deviation case its own
	// test reads.
	if len(skillsListGoldenCases) < len(cases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(skillsListGoldenCases))
	}
	for i, tc := range cases {
		if tc.name != skillsListGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, skillsListGoldenCases[i])
		}
	}

	roots := []string{projDeps, projNoManifest, projLocal, home, fallbackHome}
	before := skillsTreeSnapshot(t, roots...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := frozen[tc.name]
			if !ok {
				t.Fatalf("no frozen record %q", tc.name)
			}
			dir := tc.dir
			if dir == "" {
				dir = t.TempDir()
			}
			input := tokens.tree(t, tc.project)
			out, errOut, code := runOne(bin,
				append([]string{"skills", "list"}, tc.args...), skillsListEnv(tc.home), dir)
			skillsCheckCase(t, rec, tokens, input, out, errOut, code, tokens.tree(t, tc.project))
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
// Same discipline as the U5a table above: the built binary runs on the fixture
// project and its stdout, stderr, exit code and resulting tree must be
// byte-identical to the frozen legacy oracle of
// testdata/skills_remove_golden.json (manifest bytes, the pruned .deps copy and
// the lock are all part of the contract — S5, and the D17/D18 steps the script
// performs after the manifest write).

// skillsRemoveEnv is skillsListEnv's pinning plus a WORKTREE_GATE_BIN pin (a
// freshly built worktree-gate, or "" to strip any inherited value). It is
// shared read-only with recipe_test.go, whose live shell legs write their lock
// through the gate's `--write-lock` bridge; the frozen skills tables only ever
// pass "", because the bundles were captured with the bridge pinned and the
// native legs never print its fail-open GO_LOCK_WRITE_BRIDGE_FALLBACK warning
// (same rationale as internal/sync/materialize_test.go).
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

// skillsOracle skips LOUDLY when python3 cannot import tomllib: the legacy
// script's manifest and lock heredocs import it (Python ≥3.11), so on a
// machine whose python3 is older the LEGACY route itself crashes and there is
// no oracle to compare against — an environment gap, never a port defect.
// Same probe, same message shape as the U5a table; script names the legacy
// route this differential runs. Shared read-only with recipe_test.go, whose
// live differentials still run the shell legs.
func skillsOracle(t *testing.T, script string) {
	t.Helper()
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 unavailable (the shell leg parses TOML with it): %v", err)
	}
	probe := exec.Command(python3, "-c", "import tomllib")
	probe.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, perr := probe.CombinedOutput(); perr != nil {
		t.Skipf("python3 CANNOT import tomllib, so %s cannot run and the differential oracle is unavailable — probe `python3 -c 'import tomllib'` said %v: %s (run with a Python ≥3.11 on PATH, e.g. PATH=/tmp/pybin:$PATH go test ./internal/cli/)", script, perr, out)
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

// skillsProjectTree snapshots a project and rebases the keys on the project
// root, so two twin projects in different temp dirs compare directly. Shared
// read-only with recipe_test.go, whose twin-project differentials still use it.
func skillsProjectTree(t *testing.T, proj string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for key, marker := range skillsTreeSnapshot(t, proj) {
		out[strings.TrimPrefix(key, proj+string(filepath.Separator))] = marker
	}
	return out
}

// skillsRemoveGoldenCases is the hard-coded identity inventory of the frozen
// remove bundle: the twelve table cases in capture order, then the two
// deviation records the byte differential cannot express (a read-only manifest
// and a target that is not a directory).
var skillsRemoveGoldenCases = []string{
	"dep exists and vendored",
	"dep exists but never vendored",
	"dep does not exist",
	"missing id",
	"help",
	"unknown flag",
	"third positional",
	"only deps block",
	"manifest not found",
	"invalid dep id",
	"positional after dashdash",
	"target defaults to cwd",
	"read-only manifest",
	"missing path",
}

// TestSkillsRemoveDifferentialAgainstShell is T4: every case replays the frozen
// legacy `bash lib/skills-remove.sh` run of testdata/skills_remove_golden.json
// against the built binary on the same project and asserts byte-identical
// stdout, stderr and exit code, plus an identical resulting tree — the
// manifest, the pruned .deps copy and the lock are all frozen as bytes. Cases
// the oracle left untouched must be left untouched by the port too. The CLI home
// is snapshotted around the whole table: the verb must never write into it
// (S7's cache authority is read-only here).
func TestSkillsRemoveDifferentialAgainstShell(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsRemoveGoldenFile, skillsRemoveGoldenCases)
	bin, root := smokeSetup(t)
	home := skillsListHome(t, root)
	env := skillsRemoveEnv(home, "")

	type diffCase struct {
		name      string
		fixture   string   // "" → no project directory is created
		args      []string // "<PROJ>" is replaced by the project root
		cwdProj   bool     // run with cwd = the project dir
		untouched bool     // the verb must leave the project byte-identical
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
	// The corpus and the leading inventory must agree case for case before
	// anything is compared; the trailing records are the deviation cases their
	// own tests read.
	if len(skillsRemoveGoldenCases) < len(cases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(skillsRemoveGoldenCases))
	}
	for i, tc := range cases {
		if tc.name != skillsRemoveGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, skillsRemoveGoldenCases[i])
		}
	}

	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := frozen[tc.name]
			if !ok {
				t.Fatalf("no frozen record %q", tc.name)
			}
			proj := filepath.Join(t.TempDir(), "proj")
			if tc.fixture != "" {
				buildRemoveFixture(t, tc.fixture, proj)
			}
			tokens := skillsTokenSet(proj, "<DIR>")
			before := tokens.tree(t, proj)

			args := make([]string, 0, len(tc.args))
			for _, a := range tc.args {
				args = append(args, strings.ReplaceAll(a, "<PROJ>", proj))
			}
			dir := scratch
			if tc.cwdProj {
				dir = proj
			}
			out, errOut, code := runOne(bin, append([]string{"skills", "remove"}, args...), env, dir)
			skillsCheckCase(t, rec, tokens, before, out, errOut, code, tokens.tree(t, proj))
			if tc.untouched {
				skillsCompareNodes(t, "untouched project", tokens.tree(t, proj), before)
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
	frozen := skillsLoadGolden(t, skillsRemoveGoldenFile, skillsRemoveGoldenCases)
	home := t.TempDir()
	proj := filepath.Join(t.TempDir(), "proj")
	buildRemoveFixture(t, "vendored", proj)
	manifest := filepath.Join(proj, "ai-specs", "ai-specs.toml")
	lock := filepath.Join(proj, "ai-specs", ".ai-specs.lock")
	if err := os.Chmod(manifest, 0o444); err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	lockBefore, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}

	// The frozen oracle: the legacy route SUCCEEDED end to end on the same 0444
	// input and completed the whole tail — prune and lock included.
	rec, ok := frozen["read-only manifest"]
	if !ok {
		t.Fatal("no frozen record \"read-only manifest\"")
	}
	tokens := skillsTokenSet(proj, "<DIR>")
	skillsCompareNodes(t, "project input", tokens.tree(t, proj), rec.Input)
	if rec.RC != 0 {
		t.Errorf("frozen legacy exit = %d, want 0 (mkstemp+rename bypasses the file mode), frozen stderr = %q", rec.RC, rec.Stderr)
	}
	for _, want := range []string{
		"  ✓ removed [[deps]] 'vendored-skill' from <DIR>/ai-specs/ai-specs.toml\n",
		"  ✓ pruned ai-specs/.deps/vendored-skill/\n",
		"  ✓ removed lock hashes for 'vendored-skill'\n",
	} {
		if !strings.Contains(rec.Stdout, want) {
			t.Errorf("frozen legacy stdout = %q, want it to contain %q", rec.Stdout, want)
		}
	}

	// Native leg: the shared writer refuses; nothing is mutated at all.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "remove", "vendored-skill", proj}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen write-failure exit), stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "refusing to overwrite read-only manifest") {
		t.Errorf("go stderr = %q, want the R4 read-only refusal", stderr.String())
	}
	manifestAfter, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifestBefore, manifestAfter) {
		t.Errorf("go leg mutated a refused manifest:\n before %q\n after  %q", manifestBefore, manifestAfter)
	}
	lockAfter, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lockBefore, lockAfter) {
		t.Errorf("go leg mutated the lock despite the refusal:\n before %q\n after  %q", lockBefore, lockAfter)
	}
	if !isDirectory(filepath.Join(proj, "ai-specs", ".deps", "vendored-skill")) {
		t.Error("go leg pruned .deps despite refusing the manifest write")
	}
}

// TestSkillsRemoveMissingPathDeviation is the U5a deviation's twin for
// remove: a target that is not an existing directory died inside bash's own
// `cd` in the legacy script (lib/skills-remove.sh: line 58: cd: <path>:
// <reason>, exit 1, frozen as the "missing path" record). The port keeps exit
// 1, the `cd: <path>: <reason>` shape and the strerror reason, but does not
// fabricate the script name and line-number prefix.
func TestSkillsRemoveMissingPathDeviation(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsRemoveGoldenFile, skillsRemoveGoldenCases)
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

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

	rec, ok := frozen["missing path"]
	if !ok {
		t.Fatal("no frozen record \"missing path\"")
	}
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1", rec.RC)
	}
	if rec.Stdout != "" {
		t.Errorf("frozen legacy stdout = %q, want empty", rec.Stdout)
	}
	// Legacy: bash's own cd diagnostic names the path; its
	// `lib/skills-remove.sh: line N: ` prefix is a bash artifact, deliberately
	// not matched (same class as TestSkillsListMissingPathDeviation).
	for _, part := range []string{"cd:", "<DIR>/does-not-exist", "No such file or directory"} {
		if !strings.Contains(rec.Stderr, part) {
			t.Errorf("frozen legacy stderr = %q, want it to contain %q", rec.Stderr, part)
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
	frozen := skillsLoadGolden(t, skillsListGoldenFile, skillsListGoldenCases)
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

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

	rec, ok := frozen["missing path"]
	if !ok {
		t.Fatal("no frozen record \"missing path\"")
	}
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1", rec.RC)
	}
	if rec.Stdout != "" {
		t.Errorf("frozen legacy stdout = %q, want empty", rec.Stdout)
	}
	// Legacy: bash's own cd diagnostic names the path. The
	// `lib/skills-list.sh: line N: ` prefix is a bash artifact, deliberately
	// not matched.
	for _, part := range []string{"cd:", "<DIR>/does-not-exist", "No such file or directory"} {
		if !strings.Contains(rec.Stderr, part) {
			t.Errorf("frozen legacy stderr = %q, want it to contain %q", rec.Stderr, part)
		}
	}
}

// ── GO-20 U5c (T6): differential for the native `ai-specs skills add` /
// `add-dep`, and the `skills` dispatcher remainder ──
//
// Same discipline as the U5b table above: the built binary runs on the fixture
// project and its stdout, stderr, exit code and resulting tree must be
// byte-identical to the frozen legacy oracle of testdata/skills_add_golden.json
// (S6: the appended [[deps]] block is the contract, and the .deps copy is what
// sync vendors from it). The oracle covers the legacy `skills.sh add …`,
// `skills-add.sh` under the add-dep label and the `skills.sh` dispatcher arms.
//
// Network hermeticity (T6): every case that reaches `sync` names a LOCAL
// source — a plain directory that vendor-skills copytrees — or the plain
// https kepano URL behind the repo's own stubbed fetcher
// (AI_SPECS_VENDOR_FIXTURE_ROOT, the seam tests/test_skills_add.py's
// AutoSyncRouteTests already drives); no leg ever opens the network.

// skillsAddEnv is skillsListEnv's pinning plus WORKTREE_GATE_BIN (shared with
// the remove table — both legs' Python lock/bridge writes ride it) and the
// vendor fixture root (the stubbed fetcher; empty means "leave it unset").
func skillsAddEnv(home, gate, fixtureRoot string) []string {
	env := skillsRemoveEnv(home, gate)
	if fixtureRoot == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "AI_SPECS_VENDOR_FIXTURE_ROOT=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "AI_SPECS_VENDOR_FIXTURE_ROOT="+fixtureRoot)
}

// skillsAddFixtures builds the shared READ-ONLY sources every leg vendors
// from (copytree, no git, no network) and resolves the repo's stubbed
// fetcher root for the plain-https rows.
func skillsAddFixtures(t *testing.T, root string) (srcOK, srcNoSkill, fixtureRoot string) {
	t.Helper()
	base := t.TempDir()
	srcOK = filepath.Join(base, "src-ok")
	skillsWriteFile(t, filepath.Join(srcOK, "SKILL.md"),
		"---\nname: src-ok\ndescription: Local source skill\n---\n# src-ok\n\nBody.\n")
	srcNoSkill = filepath.Join(base, "src-noskill")
	if err := os.MkdirAll(srcNoSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureRoot = filepath.Join(root, "tests", "fixtures", "kepano-obsidian-skills")
	if _, err := os.Stat(fixtureRoot); err != nil {
		t.Skipf("vendor fixture root unavailable (the plain-https rows stub the fetcher through it): %v", err)
	}
	return srcOK, srcNoSkill, fixtureRoot
}

// The twin manifest shapes the add table starts from.
const (
	addManifestOK = "[project]\nname = \"u5c\"\n"
	// dup seeds the derived id of https://github.com/anthropics/skills so the
	// script's duplicate guard fires before any write.
	addManifestDup = `[project]
name = "u5c"

[[deps]]
id = "skills"
source = "https://github.com/anthropics/skills"
scope = ["root"]
`
	// corrupt is a TOML syntax error: the script's dup-check substitution dies
	// under set -e with the child's status 2 and NO output (its
	// `ERROR: failed to read manifest` branch is unreachable), and the port
	// reproduces that byte for byte.
	addManifestCorrupt = "[project]\nname = \"u5c\"\n[[deps]]\nid = \n"
	// tableDeps is valid TOML whose `deps` is a TABLE, not an array: the
	// script's own reader crashes with a Python traceback at exit 1 (pinned by
	// TestSkillsAddNonListDepsDeviation, not by the byte table).
	addManifestTableDeps = "[project]\nname = \"u5c\"\n[deps]\nid = \"x\"\n"
)

// buildAddFixture materializes one case's project shape under proj:
// ok/dup/corrupt/tabledeps write their manifest, empty only creates the
// project directory (the `not found` branch), and "" creates nothing.
func buildAddFixture(t *testing.T, name, proj string) {
	t.Helper()
	switch name {
	case "":
		return
	case "ok":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), addManifestOK)
	case "dup":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), addManifestDup)
	case "corrupt":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), addManifestCorrupt)
	case "tabledeps":
		skillsWriteFile(t, filepath.Join(proj, "ai-specs", "ai-specs.toml"), addManifestTableDeps)
	case "empty":
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown add fixture %q", name)
	}
}

// skillsSyncedAtRE matches the lock's wall-clock provenance stamp.
var skillsSyncedAtRE = regexp.MustCompile(`synced_at = "[^"]*"`)

// skillsAddGoldenCases is the hard-coded identity inventory of the frozen add
// bundle: the table's own cases in capture order, then the deviation records
// their own tests read (the unbound trailing flag, a target that is not a
// directory, a read-only manifest and a table-shaped deps).
var skillsAddGoldenCases = []string{
	"plain https URL with full sync",
	"local source with full sync",
	"sync failure propagates its exit code",
	"--id override with full sync",
	"--trigger with full sync",
	"--no-sync",
	".git suffix",
	"--subdir",
	"--scope several values",
	"--scope equals empty",
	"--license",
	"--attribution",
	"--ref",
	"third positional",
	"third positional after dashdash is dropped",
	"unknown flag",
	"missing URL",
	"empty last URL component",
	"existing dep id",
	"corrupt manifest dies silent at exit 2",
	"manifest not found",
	"target defaults to cwd",
	"add-dep alias form",
	"add-dep --help carries its label",
	"skills add --help",
	"skills bare usage on stderr",
	"skills --help",
	"skills help keyword",
	"skills unknown subcommand",
	"skills unknown subcommand listx",
	"skills dash subcommand",
	"trailing flag",
	"missing path",
	"read-only manifest",
	"table-shaped deps",
}

// skillsDepHashRE matches the lock's content hash for a vendored skill. That
// hash covers the rendered SKILL.md, whose frontmatter embeds the dep's
// source — a temporary directory in this corpus — so the derived value is
// host-dependent. The rendered bytes themselves are frozen, so the token is a
// narrowing of the one value that could not be: the hash's own input.
var skillsDepHashRE = regexp.MustCompile(`(?m)^("SKILL\.md" = ")[0-9a-f]{64}(")$`)

// skillsAddNodes normalizes the two host-derived values of the add payload:
// the delegating sync's wall-clock provenance stamp (tests/parity/parity.py:
// "ISO-8601 wall-clock instants (lock synced_at, …)" — the sync stamps
// [meta].synced_at with "now" at second granularity, so the frozen oracle and
// the native run land on different seconds through no behaviour of their own)
// and the dep skill hash above. Every OTHER lock byte — cli_version, the
// managed hash, URL-sourced dep hashes — still compares.
func skillsAddNodes(nodes []skillsNode) []skillsNode {
	for i := range nodes {
		for _, re := range []struct {
			pattern *regexp.Regexp
			repl    string
		}{
			{skillsSyncedAtRE, `synced_at = "<wall-clock>"`},
			{skillsDepHashRE, `${1}<dep-hash>${2}`},
		} {
			nodes[i].Content = re.pattern.ReplaceAllString(nodes[i].Content, re.repl)
		}
	}
	return nodes
}

// skillsSourceOwner is the attribution an `add` derives from a local source
// path: its first segment (`/Users/...` → "Users", `/tmp/...` → "tmp"). It is
// the one host-derived value the local-source fixtures print, so the bundle
// freezes it as the <OWNER> placeholder and the test expands it from the
// fixture's own path.
func skillsSourceOwner(source string) string {
	trimmed := strings.TrimPrefix(source, "/")
	if idx := strings.Index(trimmed, "/"); idx > 0 {
		return trimmed[:idx]
	}
	return trimmed
}

// skillsOwnerExpander resolves the add bundle's <OWNER> placeholder from the
// fixture's own local source path, so the frozen payload stays host-free while
// the expected attribution follows the host (see skillsSourceOwner).
func skillsOwnerExpander(source string) func(skillsFrozenCase) skillsFrozenCase {
	owner := skillsSourceOwner(source)
	text := func(s string) string { return strings.ReplaceAll(s, "<OWNER>", owner) }
	nodes := func(in []skillsNode) []skillsNode {
		out := slices.Clone(in)
		for i := range out {
			out[i].Content = text(out[i].Content)
			out[i].Link = text(out[i].Link)
		}
		return out
	}
	return func(rec skillsFrozenCase) skillsFrozenCase {
		rec.Stdout = text(rec.Stdout)
		rec.Stderr = text(rec.Stderr)
		rec.Input = nodes(rec.Input)
		rec.Tree = nodes(rec.Tree)
		return rec
	}
}

// TestSkillsAddDifferentialAgainstShell is T6: every case replays the frozen
// legacy `bash lib/skills.sh add` (or the add-dep / dispatcher arm) run of
// testdata/skills_add_golden.json against the built binary on the same project
// and asserts byte-identical stdout, stderr and exit code, plus an identical
// resulting tree — the appended manifest, the vendored .deps copy, and
// everything the sync delegation derives. Cases the oracle left untouched must
// be left untouched by the port too. The CLI home may gain entries under
// home/cache only: the delegating sync's refresh-bundled step is the ONE
// authority that writes the cache (S7), and nothing else in the home may move.
//
// Network hermeticity (T6): every case that reaches `sync` names a LOCAL source
// — a plain directory that vendor-skills copytrees — or the plain https kepano
// URL behind the repo's own stubbed fetcher (AI_SPECS_VENDOR_FIXTURE_ROOT, the
// seam tests/test_skills_add.py's AutoSyncRouteTests already drives); no leg
// ever opens the network.
func TestSkillsAddDifferentialAgainstShell(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsAddGoldenFile, skillsAddGoldenCases)
	bin, root := smokeSetup(t)
	home := skillsListHome(t, root)
	srcOK, srcNoSkill, fixtureRoot := skillsAddFixtures(t, root)
	env := skillsAddEnv(home, "", fixtureRoot)
	kepano := "https://github.com/kepano/obsidian-skills"
	expand := skillsOwnerExpander(srcOK)

	type addCase struct {
		name      string
		fixture   string   // manifest shape; "" → no project directory
		goCmd     []string // binary argv prefix, shared tail appended
		args      []string // "<PROJ>" is replaced by the project root
		cwdProj   bool     // run with cwd = the project dir
		untouched bool     // the verb must leave the project byte-identical
	}
	cases := []addCase{
		// ── S8: the delegation runs, hermetically, and owns the exit code ──
		{name: "plain https URL with full sync", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{kepano, "<PROJ>", "--subdir", "skills/defuddle"}},
		{name: "local source with full sync", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>"}},
		{name: "sync failure propagates its exit code", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcNoSkill, "<PROJ>"}},
		{name: "--id override with full sync", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "--id", "my-override"}},
		{name: "--trigger with full sync", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{srcOK, "<PROJ>", "--trigger", "Custom trigger phrase"}},
		// ── flag/append contract, without the sync step (fast rows) ──
		{name: "--no-sync", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "--no-sync"}},
		{name: ".git suffix", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{"https://github.com/some-org/my-skill-repo.git", "<PROJ>", "--no-sync"}},
		{name: "--subdir", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{srcOK, "<PROJ>", "--subdir", "nested/dir", "--no-sync"}},
		{name: "--scope several values", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{srcOK, "<PROJ>", "--scope", "root,subrepo,docs", "--no-sync"}},
		{name: "--scope equals empty", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "--scope=", "--no-sync"}},
		{name: "--license", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "--license", "MIT", "--no-sync"}},
		{name: "--attribution", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{srcOK, "<PROJ>", "--attribution", "The Others", "--no-sync"}},
		{name: "--ref", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "--ref", "v2.0.0", "--no-sync"}},
		// ── usage errors and pre-write refusals (nothing may be written) ──
		{name: "third positional", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>", "extra"}, untouched: true},
		{name: "third positional after dashdash is dropped", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{"--no-sync", "--", srcOK, "<PROJ>", "extra"}},
		{name: "unknown flag", fixture: "",
			goCmd: []string{"skills", "add"}, args: []string{"--bogus"}, untouched: true},
		{name: "missing URL", fixture: "",
			goCmd: []string{"skills", "add"}, untouched: true},
		{name: "empty last URL component", fixture: "ok",
			goCmd: []string{"skills", "add"},
			args:  []string{"https://github.com/obra/", "<PROJ>"}, untouched: true},
		{name: "existing dep id", fixture: "dup",
			goCmd: []string{"skills", "add"},
			args:  []string{"https://github.com/anthropics/skills", "<PROJ>"}, untouched: true},
		{name: "corrupt manifest dies silent at exit 2", fixture: "corrupt",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>"}, untouched: true},
		{name: "manifest not found", fixture: "empty",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "<PROJ>"}, untouched: true},
		{name: "target defaults to cwd", fixture: "ok",
			goCmd: []string{"skills", "add"}, args: []string{srcOK, "--no-sync"}, cwdProj: true},
		// ── the add-dep alias: bin/ai-specs's `add-dep)` arm sets the label ──
		{name: "add-dep alias form", fixture: "ok",
			goCmd: []string{"add-dep"}, args: []string{srcOK, "<PROJ>", "--no-sync"}},
		{name: "add-dep --help carries its label", fixture: "",
			goCmd: []string{"add-dep"}, args: []string{"--help"}, untouched: true},
		{name: "skills add --help", fixture: "",
			goCmd: []string{"skills", "add"}, args: []string{"--help"}, untouched: true},
		// ── the lib/skills.sh dispatcher remainder (U5c's second goal) ──
		{name: "skills bare usage on stderr", fixture: "",
			goCmd: []string{"skills"}, untouched: true},
		{name: "skills --help", fixture: "",
			goCmd: []string{"skills"}, args: []string{"--help"}, untouched: true},
		{name: "skills help keyword", fixture: "",
			goCmd: []string{"skills"}, args: []string{"help"}, untouched: true},
		{name: "skills unknown subcommand", fixture: "",
			goCmd: []string{"skills"}, args: []string{"bogus"}, untouched: true},
		{name: "skills unknown subcommand listx", fixture: "",
			goCmd: []string{"skills"}, args: []string{"listx"}, untouched: true},
		{name: "skills dash subcommand", fixture: "",
			goCmd: []string{"skills"}, args: []string{"-list"}, untouched: true},
	}
	// The corpus and the leading inventory must agree case for case before
	// anything is compared; the trailing records are the deviation cases their
	// own tests read.
	if len(skillsAddGoldenCases) < len(cases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(skillsAddGoldenCases))
	}
	for i, tc := range cases {
		if tc.name != skillsAddGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, skillsAddGoldenCases[i])
		}
	}

	// The home may gain cache entries (the delegating sync's refresh-bundled
	// step writes them through the ONE authority) and nothing else.
	homeBefore := skillsTreeSnapshot(t, home)
	scratch := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := frozen[tc.name]
			if !ok {
				t.Fatalf("no frozen record %q", tc.name)
			}
			rec = expand(rec)
			proj := filepath.Join(t.TempDir(), "proj")
			buildAddFixture(t, tc.fixture, proj)
			tokens := skillsTokenSet(srcOK, "<SRC>", srcNoSkill, "<SRC>", proj, "<DIR>")
			before := skillsAddNodes(tokens.tree(t, proj))

			args := make([]string, 0, len(tc.args))
			for _, a := range tc.args {
				args = append(args, strings.ReplaceAll(a, "<PROJ>", proj))
			}
			dir := scratch
			if tc.cwdProj {
				dir = proj
			}
			out, errOut, code := runOne(bin, append(slices.Clone(tc.goCmd), args...), env, dir)

			rec.Tree = skillsAddNodes(slices.Clone(rec.Tree))
			skillsCheckCase(t, rec, tokens, before, out, errOut, code, skillsAddNodes(tokens.tree(t, proj)))
			if tc.untouched {
				skillsCompareNodes(t, "untouched project", skillsAddNodes(tokens.tree(t, proj)), before)
			}
		})
	}

	homeAfter := skillsTreeSnapshot(t, home)
	cachePrefix := filepath.Join(home, "cache") + string(filepath.Separator)
	for key, marker := range homeAfter {
		before, existed := homeBefore[key]
		if existed && before == marker {
			continue
		}
		if !strings.HasPrefix(key, cachePrefix) {
			t.Errorf("the verb wrote outside home/cache: %s: %q -> %q (before existed=%v)", key, before, marker, existed)
		}
	}
	for key := range homeBefore {
		if _, ok := homeAfter[key]; !ok && !strings.HasPrefix(key, cachePrefix) {
			t.Errorf("the verb removed %s from the CLI home", key)
		}
	}
}

// TestSkillsAddNativeNeedsNoBashOrPython is the routing proof for the U5c
// goal: with PATH emptied the legacy route could not run at all (the shim
// execs bash lib/skills.sh → skills-add.sh → its python heredocs), while the
// native route appends the [[deps]] block and answers the dispatcher arms
// itself — `skills add`, `add-dep` and the bare `skills` no longer exec the
// shim, and no Python runs on them (S1: zero Python under the product tree).
func TestSkillsAddNativeNeedsNoBashOrPython(t *testing.T) {
	bin, root := smokeSetup(t)
	home := skillsListHome(t, root)
	srcOK, _, _ := skillsAddFixtures(t, root)
	project := t.TempDir()
	skillsWriteFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), addManifestOK)
	env := skillsAddEnv(home, "", "")
	emptyPath := t.TempDir() // no bash, no python3 reachable for the CHILD
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + emptyPath
		}
	}

	out, errOut, code := runOne(bin,
		[]string{"skills", "add", srcOK, project, "--no-sync"}, env, t.TempDir())
	if code != 0 {
		t.Errorf("exit = %d (want 0 with an empty PATH), stderr = %q", code, errOut)
	}
	if len(errOut) != 0 {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	for _, want := range []string{
		"ai-specs skills add\n",
		"  ✓ appended [[deps]] for 'src-ok' to ",
		"✓ dep registered. Run 'ai-specs sync ",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(project, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id = \"src-ok\"", "source = \"" + srcOK + "\"", "scope = [\"root\"]"} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("manifest = %q, want it to contain %q", manifest, want)
		}
	}

	// The dispatcher arms are native too: bare `skills` → usage on stderr exit
	// 2, `add-dep --help` → its own label, all with the same empty PATH.
	out, errOut, code = runOne(bin, []string{"skills"}, env, t.TempDir())
	if code != 2 || len(out) != 0 {
		t.Errorf("skills bare: exit = %d stdout = %q, want exit 2 and empty stdout", code, out)
	}
	if !strings.HasPrefix(string(errOut), "Usage: ai-specs skills <subcommand> [args]\n") {
		t.Errorf("skills bare stderr = %q, want the dispatcher usage", errOut)
	}
	out, errOut, code = runOne(bin, []string{"add-dep", "--help"}, env, t.TempDir())
	if code != 0 || len(errOut) != 0 {
		t.Errorf("add-dep --help: exit = %d stderr = %q, want exit 0 and empty stderr", code, errOut)
	}
	if !strings.HasPrefix(string(out), "Usage: ai-specs add-dep <git-url> [path] [flags]\n") {
		t.Errorf("add-dep --help stdout = %q, want the add-dep label", out)
	}
}

// TestSkillsAddTrailingFlagDeviation pins a bash artifact the port replaces
// (S12/init's JD-B-001 precedent): a trailing `--id` with no value makes
// skills-add.sh expand `$2` under `set -u` and die with
// `lib/skills-add.sh: line 47: $2: unbound variable`, exit 1, no stdout — the
// frozen "trailing flag" record. The port keeps the observed exit code 1 and
// prints the script's own dead guard text (`ERROR: --id requires a value`, the
// message behind the unreachable `shift 2 || …` fallback) instead of bash's
// line-number noise.
func TestSkillsAddTrailingFlagDeviation(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsAddGoldenFile, skillsAddGoldenCases)
	home := t.TempDir()
	proj := filepath.Join(t.TempDir(), "proj")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "add", "https://example.com/r.git", proj, "--id"}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the observed unbound-variable exit)", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty (dies in the parse loop)", stdout.String())
	}
	if want := "ERROR: --id requires a value\n"; stderr.String() != want {
		t.Errorf("go stderr = %q, want %q", stderr.String(), want)
	}

	rec, ok := frozen["trailing flag"]
	if !ok {
		t.Fatal("no frozen record \"trailing flag\"")
	}
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1", rec.RC)
	}
	if rec.Stdout != "" {
		t.Errorf("frozen legacy stdout = %q, want empty", rec.Stdout)
	}
	// Legacy: bash's own unbound-variable diagnostic (name + line prefix
	// deliberately not matched, same class as the cd deviations).
	if !strings.Contains(rec.Stderr, "$2: unbound variable") {
		t.Errorf("frozen legacy stderr = %q, want the unbound-variable diagnostic", rec.Stderr)
	}
}

// TestSkillsAddMissingPathDeviation is the U5a/U5b cd deviation's twin for
// add: exit 1 and the `cd: <path>: <reason>` shape survive, bash's
// `lib/skills-add.sh: line N: ` prefix is dropped.
func TestSkillsAddMissingPathDeviation(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsAddGoldenFile, skillsAddGoldenCases)
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "add", "https://example.com/r.git", missing}, home, nil, &stdout, &stderr)
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

	rec, ok := frozen["missing path"]
	if !ok {
		t.Fatal("no frozen record \"missing path\"")
	}
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1", rec.RC)
	}
	if rec.Stdout != "" {
		t.Errorf("frozen legacy stdout = %q, want empty", rec.Stdout)
	}
	for _, part := range []string{"cd:", "<DIR>/does-not-exist", "No such file or directory"} {
		if !strings.Contains(rec.Stderr, part) {
			t.Errorf("frozen legacy stderr = %q, want it to contain %q", rec.Stderr, part)
		}
	}
}

// TestSkillsAddReadOnlyDeviation pins BOTH observed sides of the one write
// the shell and the port word differently — and records that add is the
// OPPOSITE of remove's U5b divergence: skills-add.sh appends with
// pathlib.write_text(), which needs the FILE's write bit, so the legacy leg
// dies with a Python PermissionError after printing the summary (exit 1,
// nothing written), while the port reaches the shared atomicWrite and its
// R4-atomicwrite-readonly-refusal rule (rename needs the DIRECTORY's write
// bit). Both legs refuse with exit 1, keep the 0444 mode and the manifest
// bytes, and print the identical summary block; only the failure wording
// differs (ADR 0002 diagnostic class), so this case compares stdout, rc, input
// and tree against the frozen record and asserts each side's own stderr.
func TestSkillsAddReadOnlyDeviation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the file mode does not stop a rename over the manifest")
	}
	frozen := skillsLoadGolden(t, skillsAddGoldenFile, skillsAddGoldenCases)
	root, rerr := locateRoot()
	if rerr != nil {
		t.Skipf("repo root not located: %v", rerr)
	}
	home := t.TempDir()
	srcOK, _, _ := skillsAddFixtures(t, root)
	proj := filepath.Join(t.TempDir(), "proj")
	buildAddFixture(t, "ok", proj)
	manifest := filepath.Join(proj, "ai-specs", "ai-specs.toml")
	if err := os.Chmod(manifest, 0o444); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	// Legacy (frozen): the summary block is printed, then write_text died on
	// the 0444 mode and the script exited 1 without appending.
	rec, ok := frozen["read-only manifest"]
	if !ok {
		t.Fatal("no frozen record \"read-only manifest\"")
	}
	rec = skillsOwnerExpander(srcOK)(rec)
	tokens := skillsTokenSet(srcOK, "<SRC>", proj, "<DIR>")
	skillsCompareNodes(t, "project input", tokens.tree(t, proj), rec.Input)
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1 (write_text on a 0444 manifest)", rec.RC)
	}
	if !strings.Contains(rec.Stderr, "PermissionError") {
		t.Errorf("frozen legacy stderr = %q, want the PermissionError traceback", rec.Stderr)
	}
	if rec.Stdout == "" {
		t.Error("frozen legacy stdout is empty, want the summary block write_text died after")
	}

	// Native: identical stdout, the shared writer's refusal instead.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "add", srcOK, proj, "--no-sync"}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1 (the frozen write-failure exit)", code)
	}
	if !strings.Contains(stderr.String(), "refusing to overwrite read-only manifest") {
		t.Errorf("go stderr = %q, want the R4 read-only refusal", stderr.String())
	}
	if got := tokens.tokenize(stdout.String()); got != rec.Stdout {
		t.Errorf("stdout mismatch (both must print the summary block):\n native = %q\n frozen = %q", got, rec.Stdout)
	}

	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the refusal mutated the manifest:\n before %q\n after  %q", before, after)
	}
	if fi, err := os.Stat(manifest); err != nil || fi.Mode().Perm() != 0o444 {
		t.Errorf("mode after the refusal: %v, %v; want 0444 preserved", fi, err)
	}
}

// TestSkillsAddNonListDepsDeviation pins the second read-failure family:
// skills-add.sh's own dup-check heredoc iterates `data.get("deps")` and
// crashes with a Python traceback at exit 1 when deps is a TABLE (its
// `ERROR: failed to read manifest` branch is unreachable for this shape
// too) — the frozen "table-shaped deps" record. The port keeps exit 1, keeps
// stdout empty and keeps the manifest untouched, and words the script's own
// frozen message instead of the traceback (ADR 0002). The TOML-syntax-error
// sibling of this family dies silently at exit 2 in BOTH legs and is
// byte-compared in the table above ("corrupt manifest dies silent at exit 2").
func TestSkillsAddNonListDepsDeviation(t *testing.T) {
	frozen := skillsLoadGolden(t, skillsAddGoldenFile, skillsAddGoldenCases)
	root, rerr := locateRoot()
	if rerr != nil {
		t.Skipf("repo root not located: %v", rerr)
	}
	home := t.TempDir()
	srcOK, _, _ := skillsAddFixtures(t, root)
	proj := filepath.Join(t.TempDir(), "proj")
	buildAddFixture(t, "tabledeps", proj)

	rec, ok := frozen["table-shaped deps"]
	if !ok {
		t.Fatal("no frozen record \"table-shaped deps\"")
	}
	rec = skillsOwnerExpander(srcOK)(rec)
	if rec.RC != 1 {
		t.Errorf("frozen legacy exit = %d, want 1 (the reader crashes on a table-shaped deps)", rec.RC)
	}
	if rec.Stdout != "" {
		t.Errorf("frozen legacy stdout = %q, want empty (the crash precedes the summary)", rec.Stdout)
	}
	if !strings.Contains(rec.Stderr, "Traceback") {
		t.Errorf("frozen legacy stderr = %q, want the Python traceback", rec.Stderr)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "add", srcOK, proj, "--no-sync"}, home, nil, &stdout, &stderr)
	if code != 1 {
		t.Errorf("go exit = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("go stdout = %q, want empty", stdout.String())
	}
	if want := "ERROR: failed to read manifest — fix ai-specs.toml first.\n"; stderr.String() != want {
		t.Errorf("go stderr = %q, want %q", stderr.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(proj, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != addManifestTableDeps {
		t.Errorf("manifest was mutated:\n%q", data)
	}
}
