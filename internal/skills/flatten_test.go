package skills

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

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

func runLegacyFlatten(t *testing.T, script, root, home, dest string) flattenRun {
	t.Helper()
	cmd := exec.Command("python3", script, root, dest)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "AI_SPECS_HOME="+home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	rc := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("python3 %s: %v", script, err)
		}
		rc = ee.ExitCode()
	}
	return flattenRun{rc, stdout.String(), stderr.String(), snapshotTree(t, dest)}
}

func runGoFlatten(t *testing.T, root, home, dest string) flattenRun {
	t.Helper()
	t.Setenv("AI_SPECS_HOME", home)
	var stdout, stderr bytes.Buffer
	rc := Flatten(root, dest, "", &stdout, &stderr)
	return flattenRun{rc, stdout.String(), stderr.String(), snapshotTree(t, dest)}
}

// TestFlattenDifferential runs the REAL flatten-resolved-skills.py and the Go
// port against the SAME sandbox (the frozen cache key hashes the realpath, so
// sharing the tree avoids any path normalisation). Each leg starts from the
// same pre-state via setup. Success cases pin stdout, stderr, rc and the dest
// projection byte-for-byte; failure cases (Python traceback) pin rc, stdout
// and the partial dest projection.
func TestFlattenDifferential(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	script := legacyModule(t, "flatten-resolved-skills.py")

	cases := []struct {
		name  string
		fails bool
		// build seeds the sources once; reset restores dest before each leg.
		build func(t *testing.T, root, home string) (dest string)
		reset func(t *testing.T, dest string)
	}{
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			dest := tc.build(t, root, home)
			prepare := func() {
				if err := os.RemoveAll(dest); err != nil {
					t.Fatal(err)
				}
				if tc.reset != nil {
					tc.reset(t, dest)
				}
			}

			prepare()
			legacy := runLegacyFlatten(t, script, root, home, dest)
			prepare()
			port := runGoFlatten(t, root, home, dest)

			if (legacy.rc != 0) != tc.fails {
				t.Fatalf("legacy rc=%d, case expects fails=%v\nstderr: %s", legacy.rc, tc.fails, legacy.stderr)
			}
			if port.rc != legacy.rc {
				t.Errorf("rc: port %d, legacy %d\nport stderr: %s", port.rc, legacy.rc, port.stderr)
			}
			if port.stdout != legacy.stdout {
				t.Errorf("stdout:\n port   %q\n legacy %q", port.stdout, legacy.stdout)
			}
			if !tc.fails && port.stderr != legacy.stderr {
				t.Errorf("stderr:\n port   %q\n legacy %q", port.stderr, legacy.stderr)
			}
			if !reflect.DeepEqual(port.tree, legacy.tree) {
				t.Errorf("dest tree:\n port   %s\n legacy %s",
					strings.Join(port.tree, "\n        "), strings.Join(legacy.tree, "\n        "))
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
