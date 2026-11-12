package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

// The two legacy oracles of this file were captured once into
// testdata/vendorskills_golden.json: per case, the pre-run project tree the
// fixture wrote (so fixture drift fails as input drift), the legacy exit code,
// stdout and stderr with the temporary project root and cli home tokenized as
// <DIR> and the case's fixture paths as <GIT>/<MISSING>, and the produced tree —
// the project tree for the root [[deps]] route, the cache .deps tree for the
// recipe-dep route — as sorted root-relative paths with their bytes and, for
// regular files, their permission bits.
//
// Directory modes are deliberately not frozen: a vendored target is created with
// MkdirAll(0o777) and CopyTree's Mkdir(0o777), so its directory modes follow the
// process umask, and this package also runs t.Parallel() tests that must not have
// the umask pinned under them. File modes are frozen, with one exception:
// SKILL.md is rendered at the target with a 0o666 create, so its final mode is
// 0o666 &^ umask and the golden freezes the token "umask", which the test
// resolves from a probe file. Every other file is a copy that carries its
// source's explicit mode (copy2 chmods), so it is umask-independent. Runs under
// `umask 022` and `umask 077` prove it.
//
// The test never execs python3 and never reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const vendorGoldenFile = "vendorskills_golden.json"

// vendorFixtureURL is the one source the offline fixture override applies to
// (resolveVendorCloneSource only rewrites the kepano obsidian-skills URL).
const vendorFixtureURL = "https://github.com/kepano/obsidian-skills.git"

// vendorSyncGoldenCases is the hard-coded identity inventory of the frozen
// root-[[deps]] route: the exact case names in capture order. It is an
// independent oracle of coverage — kept in the test, never derived from the
// bundle — so a bundle that is missing, renamed, duplicated or reordered cannot
// validate silently.
var vendorSyncGoldenCases = []string{
	"no deps",
	"dep with ref",
	"dep with path and ancillary dirs",
	"missing source",
}

// vendorRecipeGoldenCases is the hard-coded identity inventory of the frozen
// recipe-dep route (sync_dep_target with in_project=False). The live leg had
// exactly one fixture shape; the freeze keeps it 1:1.
var vendorRecipeGoldenCases = []string{
	"recipe dep skill",
}

// vendorNode is one frozen tree node: a root-relative path, whether it is a
// directory (its mode follows the umask), the tokenized bytes of a regular file,
// and its mode — the octal permission bits, or the token "umask" for the one
// file the port renders with a 0o666 create.
type vendorNode struct {
	Path    string `json:"path"`
	Dir     bool   `json:"dir,omitempty"`
	Content string `json:"content,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// vendorSyncGoldenCase is one frozen fixture of the root-[[deps]] route: the
// pre-run project tree, the legacy exit code, stdout and stderr, and the
// produced project tree. A null stderr means the case's stderr text is git's
// own — version-dependent advisory and detached-HEAD chatter, or a clone
// failure's fatal line — so only a non-empty stderr is required.
type vendorSyncGoldenCase struct {
	Name         string       `json:"name"`
	InputProject []vendorNode `json:"input_project"`
	RC           int          `json:"rc"`
	Stdout       string       `json:"stdout"`
	Stderr       *string      `json:"stderr"`
	Project      []vendorNode `json:"project"`
}

// vendorRecipeGoldenCase is one frozen fixture of the recipe-dep route: the
// pre-run project tree, the legacy exit code, stdout and stderr, and the
// produced cache .deps tree.
type vendorRecipeGoldenCase struct {
	Name         string       `json:"name"`
	InputProject []vendorNode `json:"input_project"`
	RC           int          `json:"rc"`
	Stdout       string       `json:"stdout"`
	Stderr       *string      `json:"stderr"`
	Cache        []vendorNode `json:"cache"`
}

// vendorGolden is the whole frozen bundle: the oracle metadata and the two
// inventories, one per live leg.
type vendorGolden struct {
	Oracle      map[string]string        `json:"oracle"`
	SyncCases   []vendorSyncGoldenCase   `json:"sync_cases"`
	RecipeCases []vendorRecipeGoldenCase `json:"recipe_cases"`
}

// loadVendorGolden reads the frozen bundle and validates its two identity
// inventories, its oracle metadata and the root-relative sorted shape of every
// frozen path before any comparison runs.
func loadVendorGolden(t *testing.T) vendorGolden {
	t.Helper()
	var bundle vendorGolden
	if err := json.Unmarshal(readGoldenBundle(t, vendorGoldenFile), &bundle); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", vendorGoldenFile, err)
	}
	if bundle.Oracle["python"] == "" {
		t.Fatalf("frozen oracle %s carries no python version metadata", vendorGoldenFile)
	}
	syncNames := make([]string, len(bundle.SyncCases))
	for i, rec := range bundle.SyncCases {
		syncNames[i] = rec.Name
		vendorValidateFrozenPaths(t, rec.Name, rec.InputProject, rec.Project)
	}
	validateGoldenIdentity(t, "vendorskills sync", syncNames, vendorSyncGoldenCases)
	recipeNames := make([]string, len(bundle.RecipeCases))
	for i, rec := range bundle.RecipeCases {
		recipeNames[i] = rec.Name
		vendorValidateFrozenPaths(t, rec.Name, rec.InputProject, rec.Cache)
	}
	validateGoldenIdentity(t, "vendorskills recipe", recipeNames, vendorRecipeGoldenCases)
	return bundle
}

// vendorValidateFrozenPaths requires every frozen path to be root-relative and
// every frozen tree to be sorted and free of duplicates.
func vendorValidateFrozenPaths(t *testing.T, name string, trees ...[]vendorNode) {
	t.Helper()
	for _, tree := range trees {
		for i, node := range tree {
			if filepath.IsAbs(node.Path) || strings.HasPrefix(node.Path, "..") {
				t.Fatalf("frozen record %q path %q must be root-relative", name, node.Path)
			}
			if i > 0 && tree[i-1].Path >= node.Path {
				t.Fatalf("frozen record %q paths are not sorted and unique at %q", name, node.Path)
			}
		}
	}
}

// vendorManifest is the fixture manifest every case starts from.
func vendorManifest(extra string) string {
	return "[project]\nname = 'vendor-diff'\n\n[agents]\nenabled = ['claude']\n" + extra
}

// vendorFixtureRoot is the in-repo fixture the offline override serves the
// kepano obsidian-skills URL from.
func vendorFixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(gitignoreRepoRoot(t), "tests", "fixtures", "kepano-obsidian-skills")
}

// vendorSyncCase is one fixture of the root-[[deps]] route: setup writes the
// project manifest and returns the host paths it names, mapped to the tokens the
// golden froze them as. fixture, when set, builds the offline override root the
// case clones from; nil uses the in-repo kepano fixture. The git case clones the
// stable URL from a fixture root that is itself a git repository, so its
// rendered SKILL.md and lock hashes never carry a host path.
type vendorSyncCase struct {
	name    string
	fixture func(t *testing.T) string
	setup   func(t *testing.T, project string) map[string]string
}

// vendorSyncCorpus is the in-test fixture corpus: the four project shapes the
// live differential built, in capture order.
func vendorSyncCorpus() []vendorSyncCase {
	return []vendorSyncCase{
		{
			name: "no deps",
			setup: func(t *testing.T, project string) map[string]string {
				t.Helper()
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), vendorManifest(""), 0o644)
				return nil
			},
		},
		{
			name: "dep with ref",
			fixture: func(t *testing.T) string {
				t.Helper()
				gitRepo := t.TempDir()
				writeRefreshFile(t, filepath.Join(gitRepo, "SKILL.md"), "---\ndescription: git dep\n---\n# git\n", 0o644)
				for _, args := range [][]string{
					{"init", "-q"},
					{"add", "."},
					{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "init"},
					{"tag", "v1"},
				} {
					cmd := exec.Command("git", args...)
					cmd.Dir = gitRepo
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("git %v: %v\n%s", args, err, out)
					}
				}
				return gitRepo
			},
			setup: func(t *testing.T, project string) map[string]string {
				t.Helper()
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
					vendorManifest("\n[[deps]]\nid = 'dep-ref'\nsource = '"+vendorFixtureURL+"'\nref = 'v1'\n"), 0o644)
				return nil
			},
		},
		{
			name: "dep with path and ancillary dirs",
			setup: func(t *testing.T, project string) map[string]string {
				t.Helper()
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
					vendorManifest("\n[[deps]]\nid = 'dep-path'\nsource = '"+vendorFixtureURL+
						"'\npath = 'skills/obsidian-markdown'\n"), 0o644)
				return nil
			},
		},
		{
			name: "missing source",
			setup: func(t *testing.T, project string) map[string]string {
				t.Helper()
				missing := filepath.Join(t.TempDir(), "does-not-exist")
				writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"),
					vendorManifest("\n[[deps]]\nid = 'dep-missing'\nsource = '"+missing+"'\n"), 0o644)
				return map[string]string{missing: "<MISSING>"}
			},
		},
	}
}

// vendorRecipeDep is the single dep the recipe-dep route vendors: the exact
// SkillRef materialize dep from the live leg.
func vendorRecipeDep() *schema.SkillRef {
	return &schema.SkillRef{ID: "dep1", Source: "dep", URL: vendorFixtureURL, Path: "skills/obsidian-markdown"}
}

// vendorTokenize replaces the host paths a case can carry — the temporary
// project root, the cli home and the case's fixture paths — with their tokens.
// Both the path as created and its symlink-resolved form are replaced, because
// the vendoring pipeline resolves paths and macOS resolves /var to /private/var.
// Longest form first, so a nested path is tokenized before its parent.
func vendorTokenize(text string, paths map[string]string) string {
	type form struct{ from, to string }
	forms := make([]form, 0, 2*len(paths))
	for path, token := range paths {
		forms = append(forms, form{path, token})
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			forms = append(forms, form{resolved, token})
		}
	}
	sort.Slice(forms, func(i, j int) bool { return len(forms[i].from) > len(forms[j].from) })
	for _, f := range forms {
		text = strings.ReplaceAll(text, f.from, f.to)
	}
	return text
}

// vendorTokenizer is one case's tokenizer: the project root and the cli home
// both tokenize as <DIR> (neither's identity is part of the contract, and a
// confusion between them would still show up as a tree or stderr mismatch),
// plus the case's own fixture paths.
func vendorTokenizer(project, home string, extra map[string]string) func(string) string {
	paths := map[string]string{project: "<DIR>", home: "<DIR>"}
	for path, token := range extra {
		paths[path] = token
	}
	return func(text string) string { return vendorTokenize(text, paths) }
}

// vendorUmaskMode returns the permission bits a 0o666 create keeps under the
// ambient umask, learned from a probe file rather than by pinning the process
// umask: this package has t.Parallel() tests that must keep running whatever
// umask the caller has.
func vendorUmaskMode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(path, []byte("probe\n"), 0o666); err != nil {
		t.Fatalf("write umask probe: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat umask probe: %v", err)
	}
	return fmt.Sprintf("%04o", info.Mode().Perm())
}

// vendorTree lists one tree as sorted root-relative nodes: a directory as a
// bare path (its mode follows the umask) and a regular file with its tokenized
// bytes and its permission bits. A missing root is an empty tree.
func vendorTree(t *testing.T, root string, tokenize func(string) string) []vendorNode {
	t.Helper()
	out := []vendorNode{}
	if root == "" {
		return out
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
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
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("unexpected symlink %s", path)
		case info.IsDir():
			out = append(out, vendorNode{Path: rel, Dir: true})
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, vendorNode{
				Path:    rel,
				Content: tokenize(string(data)),
				Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// compareVendorTree requires identical node inventories: the same paths in the
// same order, the same directory/file kinds, the same bytes and the same modes.
// The frozen token "umask" resolves to the ambient 0o666 &^ umask bits.
func compareVendorTree(t *testing.T, label string, got, want []vendorNode, umaskMode string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d nodes, want %d\n native = %v\n frozen = %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Path != want[i].Path {
			t.Fatalf("%s: node %d is %q, want %q", label, i, got[i].Path, want[i].Path)
		}
		if got[i].Dir != want[i].Dir {
			t.Errorf("%s: %s is a directory = %v, want %v", label, want[i].Path, got[i].Dir, want[i].Dir)
			continue
		}
		if got[i].Content != want[i].Content {
			t.Errorf("%s: %s bytes:\n native = %q\n frozen = %q", label, want[i].Path, got[i].Content, want[i].Content)
		}
		wantMode := want[i].Mode
		if wantMode == "umask" {
			wantMode = umaskMode
		}
		if got[i].Mode != wantMode {
			t.Errorf("%s: %s mode: native = %q, frozen = %q", label, want[i].Path, got[i].Mode, wantMode)
		}
	}
}

// TestSyncVendoredSkillsDifferential compares the native root-[[deps]] route
// against the frozen legacy oracle for every fixture shape the live differential
// built: the fixture inputs, the exit code, stdout, stderr and the produced
// project tree (bytes and regular-file modes).
func TestSyncVendoredSkillsDifferential(t *testing.T) {
	frozen := loadVendorGolden(t)

	cases := vendorSyncCorpus()
	if len(cases) != len(vendorSyncGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(cases), len(vendorSyncGoldenCases))
	}
	umaskMode := vendorUmaskMode(t)

	for i, tc := range cases {
		if tc.name != vendorSyncGoldenCases[i] {
			t.Fatalf("corpus case %d is %q, inventory %q", i, tc.name, vendorSyncGoldenCases[i])
		}
		rec := frozen.SyncCases[i]

		t.Run(tc.name, func(t *testing.T) {
			project, home := t.TempDir(), t.TempDir()
			tokenize := vendorTokenizer(project, home, tc.setup(t, project))
			fixture := vendorFixtureRoot(t)
			if tc.fixture != nil {
				fixture = tc.fixture(t)
			}

			// The fixture the freeze was captured from is an input of the payload:
			// a drifted setup fails here, not as an opaque payload mismatch.
			compareVendorTree(t, "project input", vendorTree(t, project, tokenize), rec.InputProject, umaskMode)

			t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", fixture)
			var out, errW bytes.Buffer
			rc := SyncVendoredSkills(project, home, &out, &errW)

			if rc != rec.RC {
				t.Errorf("exit code: native = %d, frozen = %d (frozen stderr %v)", rc, rec.RC, rec.Stderr)
			}
			if got := tokenize(out.String()); got != rec.Stdout {
				t.Errorf("stdout mismatch:\n native = %q\n frozen = %q", got, rec.Stdout)
			}
			gotErr := tokenize(stripBridgeFallback(errW.String()))
			if rec.Stderr == nil {
				// A git child's stderr is git's text, not the port's: the record
				// requires it loudly present and does not pin its wording.
				if gotErr == "" {
					t.Errorf("stderr is empty, want git's own line")
				}
			} else if gotErr != *rec.Stderr {
				t.Errorf("stderr mismatch:\n native = %q\n frozen = %q", gotErr, *rec.Stderr)
			}

			compareVendorTree(t, "project", vendorTree(t, project, tokenize), rec.Project, umaskMode)
		})
	}
}

// TestRecipeDepSkillDifferential compares the native recipe-dep route
// (sync_dep_target with in_project=False, the entry the recipe-dep delegation
// uses) against the frozen legacy oracle: the exit code, stdout, stderr and the
// produced cache .deps tree.
func TestRecipeDepSkillDifferential(t *testing.T) {
	frozen := loadVendorGolden(t)

	if len(vendorRecipeGoldenCases) != 1 {
		t.Fatalf("recipe inventory has %d cases, want 1", len(vendorRecipeGoldenCases))
	}
	rec := frozen.RecipeCases[0]
	if rec.Name != vendorRecipeGoldenCases[0] {
		t.Fatalf("recipe case is %q, inventory %q", rec.Name, vendorRecipeGoldenCases[0])
	}
	fixture := vendorFixtureRoot(t)
	umaskMode := vendorUmaskMode(t)

	project, home := t.TempDir(), t.TempDir()
	tokenize := vendorTokenizer(project, home, nil)
	writeRefreshFile(t, filepath.Join(project, "ai-specs", "ai-specs.toml"), "[project]\nname = 'x'\n", 0o644)
	compareVendorTree(t, "project input", vendorTree(t, project, tokenize), rec.InputProject, umaskMode)

	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", fixture)
	var out, errW bytes.Buffer
	rc := 0
	if err := materializeDepSkill(project, home, vendorRecipeDep(), &out, &errW); err != nil {
		rc = 1
		fmt.Fprintln(&errW, err)
	}

	if rc != rec.RC {
		t.Errorf("exit code: native = %d, frozen = %d", rc, rec.RC)
	}
	if got := tokenize(out.String()); got != rec.Stdout {
		t.Errorf("stdout mismatch:\n native = %q\n frozen = %q", got, rec.Stdout)
	}
	if got, want := tokenize(stripBridgeFallback(errW.String())), *rec.Stderr; got != want {
		t.Errorf("stderr mismatch:\n native = %q\n frozen = %q", got, want)
	}

	cache := filepath.Join(refreshCacheRoot(t, home), ".deps")
	compareVendorTree(t, "cache/.deps", vendorTree(t, cache, tokenize), rec.Cache, umaskMode)
}

// TestSyncVendoredSkillsStepIsNative drives the whole sync spine with a
// PATH-shadowed `python3` that logs every invocation, and requires the vendored
// skills step to run without executing vendor-skills.py.
func TestSyncVendoredSkillsStepIsNative(t *testing.T) {
	realPython, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	root := gitignoreRepoRoot(t)

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	logPath := filepath.Join(dir, "python.log")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	wrapper := "#!/bin/bash\nprintf '%s\\n' \"$*\" >> " + shellSingleQuote(logPath) +
		"\nexec " + shellSingleQuote(realPython) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "python3"), []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write python3 shadow: %v", err)
	}

	project := filepath.Join(dir, "project")
	for _, sub := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(project, "ai-specs", sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	manifest := "[project]\nname = 'native-vendor'\n\n[agents]\nenabled = ['claude']\n"
	if err := os.WriteFile(filepath.Join(project, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AI_SPECS_GATE_OFFLINE", "1")
	t.Setenv("AI_SPECS_NO_NETWORK", "1")
	t.Setenv("AI_SPECS_VENDOR_FIXTURE_ROOT", filepath.Join(root, "tests", "fixtures", "kepano-obsidian-skills"))
	t.Setenv("TMPDIR", filepath.Join(dir, "tmp"))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("LC_ALL", "C")
	t.Setenv("PYTHONDONTWRITEBYTECODE", "1")
	for _, p := range []string{os.Getenv("TMPDIR"), os.Getenv("HOME")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}

	// Prove the shadow records invocations before relying on its silence.
	probe := exec.Command("python3", "-c", "pass")
	if out, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("probe shadow: %v\n%s", err, out)
	}
	if log, err := os.ReadFile(logPath); err != nil || len(log) == 0 {
		t.Fatalf("shadow probe logged nothing (%v): %q", err, log)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("clear probe log: %v", err)
	}

	var out, errW bytes.Buffer
	if rc := Run([]string{"sync", project}, root, strings.NewReader(""), &out, &errW); rc != 0 {
		t.Fatalf("sync rc = %d\nstdout:\n%s\nstderr:\n%s", rc, out.String(), errW.String())
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		// No Python child at all is the expected state; an absent log means
		// the shadow was never invoked, which is also a pass.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read python3 invocation log: %v", err)
	}
	if logged := string(log); strings.Contains(logged, "vendor-skills.py") {
		t.Errorf("sync still execs vendor-skills.py; log:\n%s", logged)
	}
}
