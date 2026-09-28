package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustReadTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return string(data)
}

type writeCase struct {
	name string
	// manifest content; when empty, read from testdata[manifestFile]
	manifest     string
	manifestFile string
	refScript    string
	refArgs      func(manifestPath string) []string
	goOp         func(manifestPath string) (string, error)
	// errPrefix: for guard errors the tomllib parse-error detail is
	// diagnostic only (ADR 0002), so compare the message prefix up to and
	// including "invalid TOML:" instead of the full string.
	errPrefix bool
}

func runWriteDiff(t *testing.T, tc writeCase) {
	t.Helper()
	root := repoRoot(t)
	manifest := tc.manifest
	if manifest == "" {
		manifest = mustReadTestdata(t, tc.manifestFile)
	}

	dirA, dirB := t.TempDir(), t.TempDir()
	pathA := filepath.Join(dirA, "manifest.toml")
	pathB := filepath.Join(dirB, "manifest.toml")
	if err := os.WriteFile(pathA, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := os.WriteFile(pathB, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write B: %v", err)
	}

	args := append([]string{filepath.Join("internal", "config", "testdata", tc.refScript)}, tc.refArgs(pathA)...)
	pyOut, pyErr, code := runPython(t, root, args...)
	goMsg, goErr := tc.goOp(pathB)

	norm := func(s, dir string) string {
		return strings.ReplaceAll(strings.TrimSuffix(s, "\n"), dir, "<DIR>")
	}

	if code == 0 {
		if goErr != nil {
			t.Fatalf("python succeeded but Go op failed: %v", goErr)
		}
		if want, got := norm(pyOut, dirA), norm(goMsg, dirB); want != got {
			t.Errorf("message mismatch:\n  go: %q\n  py: %q", got, want)
		}
		compareFileBytes(t, pathA, pathB)
	} else {
		if goErr == nil {
			t.Fatalf("python failed (exit %d) but Go op succeeded with message %q", code, goMsg)
		}
		want, got := norm(pyErr, dirA), norm(goErr.Error(), dirB)
		if tc.errPrefix {
			const cut = "invalid TOML:"
			want = prefixThrough(want, cut)
			got = prefixThrough(got, cut)
		}
		if want != got {
			t.Errorf("error mismatch:\n  go: %q\n  py: %q", got, want)
		}
		// Error path: the file must be untouched on both sides.
		if err := os.WriteFile(filepath.Join(dirA, "untouched-probe"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dirB, "untouched-probe"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		compareFileBytes(t, pathA, filepath.Join(dirA, "untouched-probe"))
		compareFileBytes(t, pathB, filepath.Join(dirB, "untouched-probe"))
	}

	checkNoTempFiles(t, dirA)
	checkNoTempFiles(t, dirB)
}

// prefixThrough returns s truncated after the first occurrence of cut
// (inclusive); if cut is absent, s is returned unchanged.
func prefixThrough(s, cut string) string {
	if i := strings.Index(s, cut); i >= 0 {
		return s[:i+len(cut)]
	}
	return s
}

func compareFileBytes(t *testing.T, pathA, pathB string) {
	t.Helper()
	a, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read %s: %v", pathA, err)
	}
	b, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("read %s: %v", pathB, err)
	}
	if string(a) != string(b) {
		t.Errorf("file bytes diverge:\n  A (%s): %q\n  B (%s): %q", pathA, a, pathB, b)
	}
}

func checkNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".ai-specs.toml.") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s/%s", dir, e.Name())
		}
	}
}

func TestDifferentialWrites(t *testing.T) {
	appendOp := func(depID, url, subdir, scope, trigger, license, attribution, ref string) func(string) (string, error) {
		return func(p string) (string, error) {
			return AppendDepsBlock(p, depID, url, subdir, scope, trigger, license, attribution, ref)
		}
	}

	cases := []writeCase{
		{
			name:         "remove recipe with sub-tables",
			manifestFile: "write_recipe_basic.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "a"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "a")
				return msg, err
			},
		},
		{
			name:         "remove nonexistent recipe",
			manifestFile: "write_recipe_basic.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "missing"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "missing")
				return msg, err
			},
		},
		{
			name:         "remove recipe from already-invalid TOML (guard does not fire)",
			manifestFile: "write_recipe_invalid_base.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "x"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "x")
				return msg, err
			},
		},
		{
			name:         "remove recipe valid→invalid guard fires",
			manifestFile: "write_recipe_guard.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "doom"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "doom")
				return msg, err
			},
			errPrefix: true,
		},
		{
			name:         "remove recipe from CRLF manifest (output LF-normalized)",
			manifestFile: "write_recipe_crlf.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "a"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "a")
				return msg, err
			},
		},
		{
			name:         "remove recipe CRLF valid→invalid guard fires (file untouched)",
			manifestFile: "write_recipe_crlf_guard.toml",
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "doom"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "doom")
				return msg, err
			},
			errPrefix: true,
		},
		{
			name:         "remove dep first-of-two from CRLF manifest",
			manifestFile: "write_dep_crlf.toml",
			refScript:    "skills_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "first"} },
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "first")
			},
		},
		{
			name:      "append to CRLF manifest",
			manifest:  "a = 1\r\nb = 2\r\n",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "dep", "url", "", "", "", "", "", ""}
			},
			goOp: appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:      "append to CRLF manifest without trailing newline",
			manifest:  "a = 1\r\nb = 2\r",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "dep", "url", "", "", "", "", "", ""}
			},
			goOp: appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:         "remove recipe from real repo manifest (sub-table configs)",
			manifestFile: "_repo", // special-cased below
			refScript:    "recipe_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "worktree-flow"} },
			goOp: func(p string) (string, error) {
				_, msg, err := RemoveRecipeSegments(p, "worktree-flow")
				return msg, err
			},
		},
		{
			name:         "remove dep first-of-two",
			manifestFile: "write_deps_two.toml",
			refScript:    "skills_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "first"} },
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "first")
			},
		},
		{
			name:         "remove nonexistent dep",
			manifestFile: "write_deps_two.toml",
			refScript:    "skills_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "missing"} },
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "missing")
			},
		},
		{
			name:         "remove dep valid→invalid guard fires",
			manifestFile: "write_dep_guard.toml",
			refScript:    "skills_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "doom"} },
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "doom")
			},
			errPrefix: true,
		},
		{
			name:         "remove dep from already-invalid TOML (guard does not fire)",
			manifestFile: "write_dep_invalid_base.toml",
			refScript:    "skills_remove_ref.py",
			refArgs:      func(p string) []string { return []string{p, "x"} },
			goOp: func(p string) (string, error) {
				return RemoveDepSegment(p, "x")
			},
		},
		{
			name:      "append with all fields",
			manifest:  "a = 1\n",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "my-dep", "git::https://x/y", "sub/dir", "root,dev", "on-sync", "MIT", "vendor notes", "main"}
			},
			goOp: appendOp("my-dep", "git::https://x/y", "sub/dir", "root,dev", "on-sync", "MIT", "vendor notes", "main"),
		},
		{
			name:      "append minimal",
			manifest:  "a = 1\n",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "dep", "url", "", "", "", "", "", ""}
			},
			goOp: appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:      "append to file not ending in newline",
			manifest:  "a = 1",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "dep", "url", "", "", "", "", "", ""}
			},
			goOp: appendOp("dep", "url", "", "", "", "", "", ""),
		},
		{
			name:      "append scope CSV with spaces and empties",
			manifest:  "a = 1\n",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "dep", "url", "", " root , , dev ,", "", "", "", ""}
			},
			goOp: appendOp("dep", "url", "", " root , , dev ,", "", "", "", ""),
		},
		{
			name:      "append escaping quotes and backslashes",
			manifest:  "a = 1\n",
			refScript: "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, `a"b\c`, `he said "hi" \ ok`, "", "", "", "", "", ""}
			},
			goOp: appendOp(`a"b\c`, `he said "hi" \ ok`, "", "", "", "", "", ""),
		},
		{
			name:         "append onto repo manifest",
			manifestFile: "_repo",
			refScript:    "skills_add_ref.py",
			refArgs: func(p string) []string {
				return []string{p, "new-dep", "git::https://example.com/n", "", "root", "", "", "", ""}
			},
			goOp: appendOp("new-dep", "git::https://example.com/n", "", "root", "", "", "", ""),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.manifestFile == "_repo" {
				root := repoRoot(t)
				data, err := os.ReadFile(filepath.Join(root, "ai-specs", "ai-specs.toml"))
				if err != nil {
					t.Fatalf("read repo manifest: %v", err)
				}
				tc.manifest = string(data)
				tc.manifestFile = ""
			}
			runWriteDiff(t, tc)
		})
	}
}

// TestAtomicReplaceAndMode verifies mode preservation and temp-file hygiene
// for the Go write ops (pure Go; no Python needed).
func TestAtomicReplaceAndMode(t *testing.T) {
	manifest := "[[deps]]\nid = \"x\"\nsource = \"u\"\n\n[after]\ny = 1\n"

	for _, mode := range []os.FileMode{0o755, 0o600} {
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.toml")
		if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}

		if _, err := RemoveDepSegment(path, "x"); err != nil {
			t.Fatalf("mode %o: remove failed: %v", mode, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("mode %o: file mode after write is %o", mode, got)
		}
		checkNoTempFiles(t, dir)
	}

	// Failure path: file and mode untouched, no temp files.
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveDepSegment(path, "missing"); err == nil {
		t.Fatal("expected not-found error")
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	compareFileBytes(t, path, filepath.Join(dir, "probe"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode changed on failure path: %o", got)
	}
	checkNoTempFiles(t, dir)
}
