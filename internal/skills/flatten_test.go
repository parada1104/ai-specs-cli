package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// The legacy flatten oracle (lib/_internal/flatten-resolved-skills.py main) was
// captured once into testdata/flatten_golden.json. The test never execs python3
// or reads lib/. The golden is a snapshot: regenerating it needs a human
// decision.

const flattenGoldenPath = "testdata/flatten_golden.json"

// corpusMtime pins every fabricated file and directory, because copytree/copy2
// propagate mtimes through copystat: the frozen dest projection then carries
// constants instead of the wall clock the corpus was built at.
var corpusMtime = time.Unix(1700000000, 0)

// flattenRun is one leg's observable outcome: rc, stdout, stderr and the
// projection of everything under dest.
type flattenRun struct {
	rc     int
	stdout string
	stderr string
	tree   []string
}

// snapshotTree projects dest as sorted "kind rel mode mtime [content]" lines.
// mtimes are part of parity: copytree/copy2 propagate them via copystat.
func snapshotTree(t *testing.T, dest string) []string {
	t.Helper()
	var out []string
	if _, err := os.Lstat(dest); err != nil {
		return []string{"<absent>"}
	}
	err := filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dest, p)
		mtime := info.ModTime().UnixNano()
		if rel == "." {
			mtime = 0 // dest itself is created by the run (wall clock), not copied
		}
		line := fmt.Sprintf("%s %o", rel, info.Mode())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			line = "link " + line + " -> " + target
		case info.IsDir():
			line = fmt.Sprintf("dir %s %d", line, mtime)
		default:
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			line = fmt.Sprintf("file %s %d %q", line, mtime, data)
		}
		out = append(out, line)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dest, err)
	}
	sort.Strings(out)
	return out
}

func runGoFlatten(t *testing.T, root, home, dest string) flattenRun {
	t.Helper()
	t.Setenv("AI_SPECS_HOME", home)
	var stdout, stderr bytes.Buffer
	rc := Flatten(root, dest, "", &stdout, &stderr)
	return flattenRun{rc, stdout.String(), stderr.String(), snapshotTree(t, dest)}
}

// pinMtimes fixes the mtime of every entry under the corpus dirs so the frozen
// snapshot is reproducible; setting a child's times never touches its parent.
// Symlinks are skipped: Chtimes follows them and a dangling link has no target.
func pinMtimes(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			return os.Chtimes(path, corpusMtime, corpusMtime)
		})
		if err != nil {
			t.Fatalf("pin mtimes under %s: %v", dir, err)
		}
	}
}

type frozenFlattenCase struct {
	Name          string   `json:"name"`
	CompareStderr bool     `json:"compare_stderr"`
	Exit          int      `json:"exit"`
	Tree          []string `json:"tree"`
	Stdout        string   `json:"stdout"`
	Stderr        string   `json:"stderr"`
	Snapshot      []string `json:"snapshot"`
}

type frozenFlatten struct {
	Oracle map[string]any      `json:"oracle"`
	Cases  []frozenFlattenCase `json:"cases"`
}

// expectedFlattenGoldenCases is the independent inventory of the frozen bundle:
// deleting, duplicating or reordering a golden case fails before any comparison.
var expectedFlattenGoldenCases = []string{
	"four-tiers-duplicates-nested-modes-links",
	"stale-dest-is-wiped",
	"no-skills",
	"dest-through-symlinked-parent",
	"dangling-link-stops-after-failing-skill",
	"dest-is-a-file",
}

type flattenCase struct {
	name  string
	fails bool
	// build seeds the sources once; reset restores dest before each leg.
	build func(t *testing.T, root, home string) (dest string)
	reset func(t *testing.T, dest string)
}

func flattenCases() []flattenCase {
	return []flattenCase{
		{
			name: "four-tiers-duplicates-nested-modes-links",
			build: func(t *testing.T, root, home string) string {
				buildSkillTreeAt(t, root, home)
				cache := projectcache.CacheRoot(root, home)
				local := filepath.Join(root, "ai-specs", "skills", "localonly")
				writeFileT(t, filepath.Join(local, "assets", "deep", "x.md"), "deep\n")
				script := filepath.Join(local, "run.sh")
				writeFileT(t, script, "#!/bin/sh\n")
				if err := os.Chmod(script, 0o755); err != nil {
					t.Fatal(err)
				}
				writeFileT(t, filepath.Join(root, "outside", "shared.txt"), "via link\n")
				writeFileT(t, filepath.Join(root, "outside", "dir", "inner.txt"), "dir link\n")
				symlinkT(t, filepath.Join(root, "outside", "shared.txt"), filepath.Join(local, "filelink.txt"))
				symlinkT(t, filepath.Join(root, "outside", "dir"), filepath.Join(local, "dirlink"))
				return filepath.Join(cache, "resolved-skills")
			},
		},
		{
			name: "stale-dest-is-wiped",
			build: func(t *testing.T, root, home string) string {
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "only"))
				return filepath.Join(root, "dest")
			},
			reset: func(t *testing.T, dest string) {
				writeFileT(t, filepath.Join(dest, "stale", "SKILL.md"), "old\n")
				writeFileT(t, filepath.Join(dest, "loose.txt"), "old\n")
			},
		},
		{
			name: "no-skills",
			build: func(t *testing.T, root, home string) string {
				return filepath.Join(root, "missing-parent", "dest")
			},
		},
		{
			name: "dest-through-symlinked-parent",
			build: func(t *testing.T, root, home string) string {
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "only"))
				if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
					t.Fatal(err)
				}
				symlinkT(t, filepath.Join(root, "real"), filepath.Join(root, "alias"))
				return filepath.Join(root, "alias", "dest")
			},
			reset: func(t *testing.T, dest string) {
				writeFileT(t, filepath.Join(dest, "stale.txt"), "old\n")
			},
		},
		{
			name:  "dangling-link-stops-after-failing-skill",
			fails: true,
			build: func(t *testing.T, root, home string) string {
				skills := filepath.Join(root, "ai-specs", "skills")
				writeSkill(t, filepath.Join(skills, "a-first"))
				writeSkill(t, filepath.Join(skills, "b-broken"))
				writeFileT(t, filepath.Join(skills, "b-broken", "z-after.txt"), "after\n")
				symlinkT(t, filepath.Join(root, "nowhere"), filepath.Join(skills, "b-broken", "m-dangling"))
				writeSkill(t, filepath.Join(skills, "c-never"))
				return filepath.Join(root, "dest")
			},
		},
		{
			name:  "dest-is-a-file",
			fails: true,
			build: func(t *testing.T, root, home string) string {
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "only"))
				return filepath.Join(root, "dest")
			},
			reset: func(t *testing.T, dest string) {
				writeFileT(t, dest, "not a dir\n")
			},
		},
	}
}

func loadFlattenGolden(t *testing.T) frozenFlatten {
	t.Helper()
	raw, err := os.ReadFile(flattenGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", flattenGoldenPath, err)
	}
	var golden frozenFlatten
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", flattenGoldenPath, err)
	}
	return golden
}

// TestFlattenDifferential compares the Go port with the frozen legacy oracle
// across every fabricated case: exit code, stdout, the dest projection, and
// stderr where the two legs agree (the failure cases keep a Python traceback
// that the port replaces with one "error:" line, so stderr is not frozen
// there). Inputs are pinned by a manifest so corpus drift fails as input drift.
func TestFlattenDifferential(t *testing.T) {
	golden := loadFlattenGolden(t)
	corpus := flattenCases()

	if len(corpus) != len(expectedFlattenGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(corpus), len(expectedFlattenGoldenCases))
	}
	if len(golden.Cases) != len(expectedFlattenGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedFlattenGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedFlattenGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || corpus[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, corpus[i].name, want)
		}
	}

	for i, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			frozen := golden.Cases[i]
			if frozen.CompareStderr != !tc.fails {
				t.Fatalf("%s: frozen compare_stderr = %v, corpus fails = %v", tc.name, frozen.CompareStderr, tc.fails)
			}
			if (frozen.Exit != 0) != tc.fails {
				t.Fatalf("%s: frozen exit = %d, corpus fails = %v", tc.name, frozen.Exit, tc.fails)
			}

			root := t.TempDir()
			home := t.TempDir()
			dest := tc.build(t, root, home)
			pinMtimes(t, root, home)

			gotTree := treeManifest(t, root, home)
			if strings.Join(gotTree, "\n") != strings.Join(frozen.Tree, "\n") {
				t.Fatalf("%s: frozen inputs differ from the corpus tree:\n--- frozen ---\n%s\n--- corpus ---\n%s",
					tc.name, strings.Join(frozen.Tree, "\n"), strings.Join(gotTree, "\n"))
			}

			if err := os.RemoveAll(dest); err != nil {
				t.Fatal(err)
			}
			if tc.reset != nil {
				tc.reset(t, dest)
			}
			port := runGoFlatten(t, root, home, dest)
			tokens := frozenTokens(root, home)

			if port.rc != frozen.Exit {
				t.Errorf("rc: port %d, legacy %d\nport stderr: %s", port.rc, frozen.Exit, port.stderr)
			}
			if got := applyTokens(port.stdout, tokens); got != frozen.Stdout {
				t.Errorf("stdout:\n port   %q\n legacy %q", got, frozen.Stdout)
			}
			if frozen.CompareStderr {
				if got := applyTokens(port.stderr, tokens); got != frozen.Stderr {
					t.Errorf("stderr:\n port   %q\n legacy %q", got, frozen.Stderr)
				}
			}
			gotSnapshot := port.tree
			for j, line := range gotSnapshot {
				gotSnapshot[j] = applyTokens(line, tokens)
			}
			if !reflect.DeepEqual(gotSnapshot, frozen.Snapshot) {
				t.Errorf("dest tree:\n port   %s\n legacy %s",
					strings.Join(gotSnapshot, "\n        "), strings.Join(frozen.Snapshot, "\n        "))
			}
		})
	}
}

// TestFlattenRequiresHome pins the S6 accepted deviation: Python falls back to
// its module repo root when AI_SPECS_HOME is unset; the port fails loudly.
func TestFlattenRequiresHome(t *testing.T) {
	t.Setenv("AI_SPECS_HOME", "")
	root := t.TempDir()
	dest := filepath.Join(root, "dest")
	var stdout, stderr bytes.Buffer
	if rc := Flatten(root, dest, "", &stdout, &stderr); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	if stdout.Len() != 0 || stderr.String() != "error: AI_SPECS_HOME is not set\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest must not be created: %v", err)
	}
}

// TestFlattenRefusesEmptyDest pins the S7 safety exception: Python resolves an
// empty dest to the cwd and rmtree's it; the port refuses before any write.
func TestFlattenRefusesEmptyDest(t *testing.T) {
	root, home, _ := buildSkillTree(t)
	t.Setenv("AI_SPECS_HOME", home)
	t.Chdir(root)
	before := snapshotTree(t, root)
	var stdout, stderr bytes.Buffer
	if rc := Flatten(root, "", "", &stdout, &stderr); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	if stdout.Len() != 0 || stderr.String() != "error: empty destination directory\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("cwd changed:\n before %v\n after  %v", before, after)
	}
}

func symlinkT(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
